package oap

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	gcpcrm "google.golang.org/api/cloudresourcemanager/v1"
	gcpiam "google.golang.org/api/iam/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-oap] OAP cert-manager ACME issuer", func() {
	defer g.GinkgoRecover()
	var (
		oc                  = exutil.NewCLI("cert-manager-acme", exutil.KubeConfigPath())
		buildPruningBaseDir = exutil.FixturePath("testdata", "oap/certmanager")
		acmeServerEndpoint  = "https://acme-staging-v02.api.letsencrypt.org/directory"
	)
	g.BeforeEach(func() {
		if !isDeploymentReady(oc, operatorNamespace, operatorDeploymentName) {
			e2e.Logf("Creating Cert Manager Operator...")
			createCertManagerOperator(oc)
		}
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-OSD_CCS-High-62494-dns01 solver should work well with AWS Route53 using explicit credentials", func() {
		var (
			issuerName = "clusterissuer-acme-dns01-route53"
			certName   = "cert-from-" + issuerName
		)

		// applicable variants: AWS, non-STS, non-proxy
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if exutil.IsSTSCluster(oc) {
			g.Skip("Skip for STS cluster")
		}
		exutil.SkipOnProxyCluster(oc)
		if isDisconnected(oc) {
			e2e.Logf("setup a private ACME server for testing in disconnected environment")
			acmeServerEndpoint = setupPebbleServer(oc, oc.Namespace())
		}

		e2e.Logf("Create secret that contains AWS accessKey for issuer to reference")
		defer func() {
			e2e.Logf("Cleanup the created credentials secret")
			err := oc.AsAdmin().Run("delete").Args("-n", operandNamespace, "secret", "test-secret").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		accessKeyID, secureKey := getCredentialFromCluster(oc, "aws")
		oc.NotShowInfo()
		err := oc.AsAdmin().Run("create").Args("-n", operandNamespace, "secret", "generic", "test-secret", "--from-literal=secret-access-key="+secureKey).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Prepare the AWS config client")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsConfig, err := config.LoadDefaultConfig(
			context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
			config.WithRegion(region),
		)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create clusterissuer with route53 as dns01 solver.")
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		baseDomain := getBaseDomain(oc)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("Skipping test case for retreiving Route53 hosted zone ID for current env returns none")
		}
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53.yaml")
		params := []string{"-f", clusterIssuerTemplate, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint, "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "AWS_ACCESS_KEY_ID=" + accessKeyID, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		err = waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("create certificate which references previous clusterissuer")
		dnsName := constructDNSName(dnsZone)
		certTemplate := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certTemplate, "-p", "CERT_NAME=" + certName, "ISSUER_KIND=" + "ClusterIssuer", "ISSUER_NAME=" + issuerName, "DNS_NAME=" + dnsName, "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, certName, oc.Namespace())
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ARO-OSD_CCS-High-62063-http01 solver should work well with default Ingress", func() {
		var (
			issuerName = "letsencrypt-http01"
			certName   = "cert-from-" + issuerName
		)

		// applicable variants: non-proxy
		exutil.SkipOnProxyCluster(oc)
		if isDisconnected(oc) {
			e2e.Logf("setup a private ACME server for testing in disconnected environment")
			acmeServerEndpoint = setupPebbleServer(oc, oc.Namespace())
		}

		e2e.Logf("create an ACME issuer with http01 solver")
		issuerHTTP01File := filepath.Join(buildPruningBaseDir, "issuer-acme-http01.yaml")
		params := []string{"-f", issuerHTTP01File, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err := waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := constructDNSName(ingressDomain)
		certHTTP01File := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certHTTP01File, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "DNS_NAME=" + dnsName, "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, certName, oc.Namespace())
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ARO-OSD_CCS-High-63325-http01 solver should work well with default Ingress using trusted CA in HTTPS proxy env [Serial]", func() {
		var (
			issuerName = "letsencrypt-http01"
			certName   = "cert-from-" + issuerName
		)

		// applicable variants: proxy
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		output0, err0 := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec.trustedCA.name}").Output()
		if !strings.Contains(output, "httpsProxy") || err != nil || output0 == "" || err0 != nil {
			g.Skip("Skip for non-proxy cluster")
		}
		if isDisconnected(oc) {
			e2e.Logf("setup a private ACME server for testing in disconnected environment")
			acmeServerEndpoint = setupPebbleServer(oc, oc.Namespace())
		}

		e2e.Logf("Create configmap trusted-ca.")
		defer func() {
			e2e.Logf("Delete configmap trusted-ca.")
			err = oc.AsAdmin().Run("delete").Args("-n", operandNamespace, "cm", "trusted-ca").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = oc.AsAdmin().Run("create").Args("-n", operandNamespace, "configmap", "trusted-ca").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("label").Args("-n", operandNamespace, "cm", "trusted-ca", "config.openshift.io/inject-trusted-cabundle=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("patch the subscription to inject TRUSTED_CA_CONFIGMAP_NAME env")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"config":{"env":[{"name":"TRUSTED_CA_CONFIGMAP_NAME","value":"trusted-ca"}]}}}`
		err = oc.AsAdmin().Run("patch").Args("sub", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset subscription env")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"config":{"env":[]}}}`
			err = oc.AsAdmin().Run("patch").Args("sub", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("create an ACME issuer with http01 solver")
		issuerHTTP01File := filepath.Join(buildPruningBaseDir, "issuer-acme-http01.yaml")
		params := []string{"-f", issuerHTTP01File, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := constructDNSName(ingressDomain)
		certHTTP01File := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certHTTP01File, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "DNS_NAME=" + dnsName, "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, certName, oc.Namespace())
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-OSD_CCS-ConnectedOnly-Medium-62582-dns01 solver should work well with DNS recursive nameservers when target hosted zone overlaps with cluster default private hosted zone [Serial]", func() {
		var (
			issuerName = "clusterissuer-acme-dns01-hosted-zone-overlapped"
			certName   = "cert-from-" + issuerName
		)

		// applicable variants: AWS, non-STS, non-proxy, non-disconnected
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if exutil.IsSTSCluster(oc) {
			g.Skip("Skip for STS cluster")
		}
		exutil.SkipOnProxyCluster(oc)

		e2e.Logf("Create secret that contains AWS accessKey for issuer to reference")
		defer func() {
			e2e.Logf("Cleanup the created credentials secret")
			err := oc.AsAdmin().Run("delete").Args("-n", operandNamespace, "secret", "test-secret").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		accessKeyID, secureKey := getCredentialFromCluster(oc, "aws")
		oc.NotShowInfo()
		err := oc.AsAdmin().Run("create").Args("-n", operandNamespace, "secret", "generic", "test-secret", "--from-literal=secret-access-key="+secureKey).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Prepare the AWS config client")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsConfig, err := config.LoadDefaultConfig(
			context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
			config.WithRegion(region),
		)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create clusterissuer with route53 as dns01 solver.")
		baseDomain := getBaseDomain(oc)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("Skipping test case for retreiving Route53 hosted zone ID for current env returns none")
		}
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53.yaml")
		params := []string{"-f", clusterIssuerTemplate, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint, "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "AWS_ACCESS_KEY_ID=" + accessKeyID, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("Patch controller args with DNS recursive nameservers for conducting self-check")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"controllerConfig":{"overrideArgs":["--dns01-recursive-nameservers=1.1.1.1:53", "--dns01-recursive-nameservers-only"]}}}`
		err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset controller args")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"controllerConfig":{"overrideArgs":null}}}`
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("Create a certificate")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := constructDNSName(ingressDomain)
		certTemplate := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName, "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "ISSUER_KIND=" + "ClusterIssuer", "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("Check certificate readiness")
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, certName, oc.Namespace())
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-OSD_CCS-ConnectedOnly-Medium-63555-dns01 solver should work well with DNS recursive nameservers in proxy-enabled env [Serial]", func() {
		var (
			issuerName = "clusterissuer-acme-dns01-proxy"
			certName   = "cert-from-" + issuerName
		)

		// applicable variants: AWS, non-STS, proxy, non-disconnected
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if exutil.IsSTSCluster(oc) {
			g.Skip("Skip for STS cluster")
		}
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "httpsProxy") {
			g.Skip("Skip for non-proxy cluster")
		}

		e2e.Logf("Create secret that contains AWS accessKey for issuer to reference")
		defer func() {
			e2e.Logf("Cleanup the created credentials secret")
			err := oc.AsAdmin().Run("delete").Args("-n", operandNamespace, "secret", "test-secret").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		accessKeyID, secureKey := getCredentialFromCluster(oc, "aws")
		oc.NotShowInfo()
		err = oc.AsAdmin().Run("create").Args("-n", operandNamespace, "secret", "generic", "test-secret", "--from-literal=secret-access-key="+secureKey).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Prepare the AWS config client")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsConfig, err := config.LoadDefaultConfig(
			context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
			config.WithRegion(region),
		)
		o.Expect(err).NotTo(o.HaveOccurred())

		baseDomain := getBaseDomain(oc)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("Skipping test case for retreiving Route53 hosted zone ID for current env returns none")
		}
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53.yaml")
		params := []string{"-f", clusterIssuerTemplate, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint, "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "AWS_ACCESS_KEY_ID=" + accessKeyID, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("Patch controller args with DNS recursive nameservers for conducting self-check")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"controllerConfig":{"overrideArgs":["--dns01-recursive-nameservers-only"]}}}`
		err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset controller args")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"controllerConfig":{"overrideArgs":null}}}`
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("Create a certificate")
		dnsName := constructDNSName(dnsZone)
		certTemplate := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName, "CERT_NAME=" + certName, "ISSUER_KIND=" + "ClusterIssuer", "ISSUER_NAME=" + issuerName, "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("Checke certificate readiness")
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, certName, oc.Namespace())
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-OSD_CCS-ConnectedOnly-Medium-69798-dns01 solver should work well with DNS-over-HTTPS to do self-checks in proxy-enabled env [Serial]", func() {
		var (
			minSupportedVersion = "1.13.0"
			issuerName          = "clusterissuer-acme-dns01-proxy-doh"
			certName            = "cert-from-" + issuerName
		)

		skipUnsupportedVersion(oc, minSupportedVersion)

		// applicable variants: AWS, non-STS, proxy, non-disconnected
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if exutil.IsSTSCluster(oc) {
			g.Skip("Skip for STS cluster")
		}
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "httpsProxy") {
			g.Skip("Skip for non-proxy cluster")
		}

		e2e.Logf("Create secret that contains AWS accessKey for issuer to reference")
		defer func() {
			e2e.Logf("Cleanup the created credentials secret")
			err := oc.AsAdmin().Run("delete").Args("-n", operandNamespace, "secret", "test-secret").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		accessKeyID, secureKey := getCredentialFromCluster(oc, "aws")
		oc.NotShowInfo()
		err = oc.AsAdmin().Run("create").Args("-n", operandNamespace, "secret", "generic", "test-secret", "--from-literal=secret-access-key="+secureKey).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Prepare the AWS config client")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsConfig, err := config.LoadDefaultConfig(
			context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
			config.WithRegion(region),
		)
		o.Expect(err).NotTo(o.HaveOccurred())

		baseDomain := getBaseDomain(oc)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("Skipping test case for retreiving Route53 hosted zone ID for current env returns none")
		}
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53.yaml")
		params := []string{"-f", clusterIssuerTemplate, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint, "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "AWS_ACCESS_KEY_ID=" + accessKeyID, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("Patch controller args with valid server for DNS01 self-check")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"controllerConfig":{"overrideArgs":["--dns01-recursive-nameservers-only", "--dns01-recursive-nameservers=https://1.1.1.1/dns-query"]}}}`
		err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset controller args")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"controllerConfig":{"overrideArgs":null}}}`
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("Create a certificate")
		dnsName := constructDNSName(dnsZone)
		certTemplate := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName, "CERT_NAME=" + certName, "COMMON_NAME=" + dnsName, "ISSUER_NAME=" + issuerName, "ISSUER_KIND=" + "ClusterIssuer", "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("Checke certificate readiness")
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		exutil.By("Check and verify issued certificate content")
		verifyCertificate(oc, certName, oc.Namespace())
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ARO-OSD_CCS-Low-63500-multiple solvers mixed with http01 and dns01 should work well", func() {
		var (
			issuerName = "acme-multiple-solvers"
		)

		// applicable variants: non-proxy
		exutil.SkipOnProxyCluster(oc)
		if isDisconnected(oc) {
			e2e.Logf("setup a private ACME server for testing in disconnected environment")
			acmeServerEndpoint = setupPebbleServer(oc, oc.Namespace())
		}

		exutil.By("Create a clusterissuer which has multiple solvers mixed with http01 and dns01")
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-multiple-solvers.yaml")
		params := []string{"-f", clusterIssuerTemplate, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("As normal user, create below 3 certificates in later steps with above clusterissuer.")
		e2e.Logf("Create cert, cert-match-test-1.")
		certFile1 := filepath.Join(buildPruningBaseDir, "cert-match-test-1.yaml")
		err = oc.Run("create").Args("-f", certFile1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ := oc.Run("get").Args("challenge").Output()
			if !strings.Contains(output, "pending") {
				return false, nil
			}
			e2e.Logf("challenge1 has become pending as expected:\n%v", output)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "timeout waiting for challenge1 to become pending")
		challenge1, err := oc.AsAdmin().Run("get").Args("challenge", "-o=jsonpath={.items[*].spec.solver.selector.matchLabels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(challenge1).To(o.ContainSubstring(`"use-http01-solver":"true"`))
		err = oc.Run("delete").Args("cert/cert-match-test-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Create cert, cert-match-test-2.")
		certFile2 := filepath.Join(buildPruningBaseDir, "cert-match-test-2.yaml")
		err = oc.Run("create").Args("-f", certFile2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ := oc.Run("get").Args("challenge").Output()
			if !strings.Contains(output, "pending") {
				return false, nil
			}
			e2e.Logf("challenge2 has become pending as expected:\n%v", output)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "timeout waiting for challenge2 to become pending")
		challenge2, err := oc.Run("get").Args("challenge", "-o=jsonpath={.items[*].spec.solver.selector.dnsNames}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(challenge2).To(o.ContainSubstring("xxia-test-2.test-example.com"))

		e2e.Logf("Create cert, cert-match-test-3.")
		certFile3 := filepath.Join(buildPruningBaseDir, "cert-match-test-3.yaml")
		err = oc.Run("create").Args("-f", certFile3).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ := oc.Run("get").Args("challenge").Output()
			if !strings.Contains(output, "pending") {
				return false, nil
			}
			e2e.Logf("challenge3 has become pending as expected:\n%v", output)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "timeout waiting for challenge3 to become pending")
		challenge3, err := oc.Run("get").Args("challenge", "-o=jsonpath={.items[*].spec.solver.selector.dnsZones}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(challenge3).To(o.ContainSubstring("test-example.com"))
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-High-62500-dns01 solver should work well with AWS Route53 in STS env using IRSA as ambient credentials [Serial]", func() {
		var (
			randomSuffix = getRandomString(4)
			roleName     = "test-private-62500-sts-" + randomSuffix
			policyName   = "test-private-62500-dns01-" + randomSuffix
			issuerName   = "clusterissuer-acme-dns01-route53-ambient"
			certName     = "cert-from-" + issuerName + "-webhook"
		)

		// applicable variants: AWS, STS, non-proxy
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if !exutil.IsSTSCluster(oc) {
			g.Skip("Skip for non-STS cluster")
		}
		exutil.SkipOnProxyCluster(oc)
		if isDisconnected(oc) {
			e2e.Logf("setup a private ACME server for testing in disconnected environment")
			acmeServerEndpoint = setupPebbleServer(oc, oc.Namespace())
		}

		exutil.By("prepare the AWS config, STS and IAM client")
		// AWS config
		// Note that in Prow CI, the credentials source is automatically pre-configured to by the step 'openshift-extended-test'
		// See https://github.com/openshift/release/blob/69b2b9c4f28adcfcc5b9ff4820ecbd8d2582a3d7/ci-operator/step-registry/openshift-extended/test/openshift-extended-test-commands.sh#L41
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsConfig, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		o.Expect(err).NotTo(o.HaveOccurred())
		partition := "aws"
		if strings.Contains(region, "us-gov") {
			e2e.Logf("set AWS partition to 'aws-us-gov' as running on AWS Gov cloud")
			partition = "aws-us-gov"
		}
		// STS client
		stsClient := sts.NewFromConfig(awsConfig)
		getCallerIdentityOutput, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
		o.Expect(err).NotTo(o.HaveOccurred())
		accountID := aws.ToString(getCallerIdentityOutput.Account)
		// IAM client
		iamClient := iam.NewFromConfig(awsConfig)
		// OIDC provider
		oidcProvider, err := exutil.GetOIDCProvider(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create the AWS IAM role with trust relationship policy")
		roleTrustPolicy := `{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Principal": {
						"Federated": "arn:%s:iam::%s:oidc-provider/%s"
					},
					"Action": "sts:AssumeRoleWithWebIdentity",
					"Condition": {
						"StringEquals": {
							"%s:sub": [
								"system:serviceaccount:cert-manager:cert-manager"
							]
						}
					}
				}
			]
		}`
		roleTrustPolicy = fmt.Sprintf(roleTrustPolicy, partition, accountID, oidcProvider, oidcProvider)
		createRoleOutput, err := iamClient.CreateRole(context.TODO(), &iam.CreateRoleInput{
			AssumeRolePolicyDocument: aws.String(roleTrustPolicy),
			RoleName:                 aws.String(roleName),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		roleARN := aws.ToString(createRoleOutput.Role.Arn)
		defer func() {
			e2e.Logf("cleanup the created AWS IAM Role")
			_, err = iamClient.DeleteRole(context.TODO(), &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("create the AWS IAM policy for permissions to operate in Route 53")
		dnsPolicy := `{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": "route53:GetChange",
					"Resource": "arn:%s:route53:::change/*"
				},
				{
					"Effect": "Allow",
					"Action": [
						"route53:ChangeResourceRecordSets",
						"route53:ListResourceRecordSets"
					],
					"Resource": "arn:%s:route53:::hostedzone/*"
				},
				{
					"Effect": "Allow",
					"Action": "route53:ListHostedZonesByName",
					"Resource": "*"
				}
			]
		}`
		dnsPolicy = fmt.Sprintf(dnsPolicy, partition, partition)
		createPolicyOutput, err := iamClient.CreatePolicy(context.TODO(), &iam.CreatePolicyInput{
			PolicyDocument: aws.String(dnsPolicy),
			PolicyName:     aws.String(policyName),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		policyARN := aws.ToString(createPolicyOutput.Policy.Arn)
		defer func() {
			e2e.Logf("cleanup the created AWS IAM Policy")
			_, err = iamClient.DeletePolicy(context.TODO(), &iam.DeletePolicyInput{PolicyArn: aws.String(policyARN)})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("attach the AWS IAM policy to the created role")
		_, err = iamClient.AttachRolePolicy(context.TODO(), &iam.AttachRolePolicyInput{
			PolicyArn: aws.String(policyARN),
			RoleName:  aws.String(roleName),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("detach the AWS IAM Role with Policy")
			_, err = iamClient.DetachRolePolicy(context.TODO(), &iam.DetachRolePolicyInput{
				PolicyArn: aws.String(policyARN),
				RoleName:  aws.String(roleName),
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("annotate the ServiceAccount created by cert-manager")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("sa/cert-manager", "eks.amazonaws.com/role-arn="+roleARN, "-n", controllerNamespace, "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("de-annotate the role-arn from the cert-manager ServiceAccount")
			err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("sa/cert-manager", "eks.amazonaws.com/role-arn-", "-n", controllerNamespace, "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-l", controllerLabel, "-n", controllerNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()

		// delete the old pod and wait for a new one redeployed
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-l", controllerLabel, "-n", controllerNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("create a clusterissuer with route53 as dns01 solver")
		baseDomain := getBaseDomain(oc)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("skipping as retreiving Route53 hosted zone ID for current env returns none")
		}
		issuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53-ambient.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint, "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		err = waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("create a certificate")
		dnsName := getRandomString(4) + "." + dnsZone
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "DNS_NAME=" + dnsName, "ISSUER_NAME=" + issuerName, "ISSUER_KIND=" + "ClusterIssuer", "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-Low-65132-dns01 solver should work well with AWS Route53 in STS env using patched secret as ambient credentials when pod-identity-webhook is not used [Serial]", func() {
		var (
			randomSuffix  = getRandomString(4)
			roleName      = "test-private-65132-sts-" + randomSuffix
			policyName    = "test-private-65132-dns01-" + randomSuffix
			issuerName    = "clusterissuer-acme-dns01-route53-ambient"
			certName      = "cert-from-" + issuerName + "-manual"
			stsSecretName = "aws-sts-creds"
		)

		// applicable variants: AWS, STS, non-proxy
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if !exutil.IsSTSCluster(oc) {
			g.Skip("Skip for non-STS cluster")
		}
		exutil.SkipOnProxyCluster(oc)
		if isDisconnected(oc) {
			e2e.Logf("setup a private ACME server for testing in disconnected environment")
			acmeServerEndpoint = setupPebbleServer(oc, oc.Namespace())
		}

		exutil.By("prepare the AWS config, STS and IAM client")
		// AWS config
		// Note that in Prow CI, the credentials source is automatically pre-configured to by the step 'openshift-extended-test'
		// See https://github.com/openshift/release/blob/69b2b9c4f28adcfcc5b9ff4820ecbd8d2582a3d7/ci-operator/step-registry/openshift-extended/test/openshift-extended-test-commands.sh#L41
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsConfig, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		o.Expect(err).NotTo(o.HaveOccurred())
		partition := "aws"
		if strings.HasPrefix(region, "us-gov") {
			e2e.Logf("set AWS partition to 'aws-us-gov' as running on AWS Gov cloud")
			partition = "aws-us-gov"
		}
		// STS client
		stsClient := sts.NewFromConfig(awsConfig)
		getCallerIdentityOutput, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
		o.Expect(err).NotTo(o.HaveOccurred())
		accountID := aws.ToString(getCallerIdentityOutput.Account)
		// IAM client
		iamClient := iam.NewFromConfig(awsConfig)
		// OIDC provider
		oidcProvider, err := exutil.GetOIDCProvider(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create the AWS IAM role with trust relationship policy")
		roleTrustPolicy := `{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Principal": {
						"Federated": "arn:%s:iam::%s:oidc-provider/%s"
					},
					"Action": "sts:AssumeRoleWithWebIdentity",
					"Condition": {
						"StringEquals": {
							"%s:sub": [
								"system:serviceaccount:cert-manager:cert-manager"
							]
						}
					}
				}
			]
		}`
		roleTrustPolicy = fmt.Sprintf(roleTrustPolicy, partition, accountID, oidcProvider, oidcProvider)
		createRoleOutput, err := iamClient.CreateRole(context.TODO(), &iam.CreateRoleInput{
			AssumeRolePolicyDocument: aws.String(roleTrustPolicy),
			RoleName:                 aws.String(roleName),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		roleARN := aws.ToString(createRoleOutput.Role.Arn)
		defer func() {
			e2e.Logf("cleanup the created AWS IAM Role")
			_, err = iamClient.DeleteRole(context.TODO(), &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("create the AWS IAM policy for permissions to operate in Route 53")
		dnsPolicy := `{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": "route53:GetChange",
					"Resource": "arn:%s:route53:::change/*"
				},
				{
					"Effect": "Allow",
					"Action": [
						"route53:ChangeResourceRecordSets",
						"route53:ListResourceRecordSets"
					],
					"Resource": "arn:%s:route53:::hostedzone/*"
				},
				{
					"Effect": "Allow",
					"Action": "route53:ListHostedZonesByName",
					"Resource": "*"
				}
			]
		}`
		dnsPolicy = fmt.Sprintf(dnsPolicy, partition, partition)
		createPolicyOutput, err := iamClient.CreatePolicy(context.TODO(), &iam.CreatePolicyInput{
			PolicyDocument: aws.String(dnsPolicy),
			PolicyName:     aws.String(policyName),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		policyARN := aws.ToString(createPolicyOutput.Policy.Arn)
		defer func() {
			e2e.Logf("cleanup the created AWS IAM Policy")
			_, err = iamClient.DeletePolicy(context.TODO(), &iam.DeletePolicyInput{PolicyArn: aws.String(policyARN)})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("attach the AWS IAM policy to the created role")
		_, err = iamClient.AttachRolePolicy(context.TODO(), &iam.AttachRolePolicyInput{
			PolicyArn: aws.String(policyARN),
			RoleName:  aws.String(roleName),
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("detach the AWS IAM Role with Policy")
			_, err = iamClient.DetachRolePolicy(context.TODO(), &iam.DetachRolePolicyInput{
				PolicyArn: aws.String(policyARN),
				RoleName:  aws.String(roleName),
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("create the STS config secret manually")
		credContent := fmt.Sprintf("[default]\nsts_regional_endpoints = regional\nrole_arn = %s\nweb_identity_token_file = /var/run/secrets/openshift/serviceaccount/token\nregion = %s", roleARN, region)
		err = oc.AsAdmin().Run("create").Args("-n", operandNamespace, "secret", "generic", stsSecretName, "--from-literal=credentials="+credContent).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("delete the manually created STS secret")
			err := oc.AsAdmin().Run("delete").Args("-n", operandNamespace, "secret", stsSecretName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("patch the subscription to inject CLOUD_CREDENTIALS_SECRET_NAME env")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"config":{"env":[{"name":"CLOUD_CREDENTIALS_SECRET_NAME","value":"` + stsSecretName + `"}]}}}`
		err = oc.AsAdmin().Run("patch").Args("sub", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset subscription env")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"config":{"env":[]}}}`
			err = oc.AsAdmin().Run("patch").Args("sub", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("create a clusterissuer with route53 as dns01 solver")
		baseDomain := getBaseDomain(oc)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("skipping as retreiving Route53 hosted zone ID for current env returns none")
		}
		issuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53-ambient.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint, "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		err = waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("create a certificate")
		dnsName := getRandomString(4) + "." + dnsZone
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "DNS_NAME=" + dnsName, "ISSUER_NAME=" + issuerName, "ISSUER_KIND=" + "ClusterIssuer", "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
			e2e.Logf("listing envs of the controller pod, it should contain 'AWS_SDK_LOAD_CONFIG=1'")
			oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "-l", controllerLabel, "-n", controllerNamespace, "--list").Execute()
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-High-62946-dns01 solver should work well with GCP CloudDNS in STS env using workload identity as ambient credentials [Serial]", func() {
		var (
			serviceAccountPrefix = "test-private-62946-dns01-"
			issuerName           = "clusterissuer-acme-dns01-clouddns-ambient"
			certName             = "cert-from-" + issuerName
			stsSecretName        = "gcp-sts-creds"
		)

		// applicable variants: GCP, STS, non-proxy
		exutil.SkipIfPlatformTypeNot(oc, "GCP")
		if !exutil.IsSTSCluster(oc) {
			g.Skip("Skip for non-STS cluster")
		}
		exutil.SkipOnProxyCluster(oc)
		if isDisconnected(oc) {
			e2e.Logf("setup a private ACME server for testing in disconnected environment")
			acmeServerEndpoint = setupPebbleServer(oc, oc.Namespace())
		}

		exutil.By("create the GCP IAM and CloudResourceManager client")
		// Note that in Prow CI, the credentials source is automatically pre-configured to by the step 'openshift-extended-test'
		// See https://github.com/openshift/release/blob/69b2b9c4f28adcfcc5b9ff4820ecbd8d2582a3d7/ci-operator/step-registry/openshift-extended/test/openshift-extended-test-commands.sh#L43
		iamService, err := gcpiam.NewService(context.Background())
		o.Expect(err).NotTo(o.HaveOccurred())
		crmService, err := gcpcrm.NewService(context.Background())
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("prepare some configs for following WorkloadIdentity authentication")
		// get GCP project ID from cluster
		projectID, err := exutil.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		e2e.Logf("project ID: %s", projectID)

		// retrieve WorkloadIdentity pool ID from OIDC Provider
		oidcProvider, err := exutil.GetOIDCProvider(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		poolID := strings.TrimSuffix(strings.Split(oidcProvider, "/")[1], "-oidc") // trim 'storage.googleapis.com/poolID-oidc' to 'poolID'

		// retrieve projectNumber from project
		project, err := crmService.Projects.Get(projectID).Do()
		o.Expect(err).NotTo(o.HaveOccurred())
		projectNumber := strconv.FormatInt(project.ProjectNumber, 10) // convert int64 to string

		// construct resource identifier
		identifier := fmt.Sprintf("//iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s", projectNumber, poolID)
		e2e.Logf("constructed resource identifier: %s", identifier)

		exutil.By("create a GCP service account")
		serviceAccountName := serviceAccountPrefix + getRandomString(4)
		request := &gcpiam.CreateServiceAccountRequest{
			AccountId: serviceAccountName,
			ServiceAccount: &gcpiam.ServiceAccount{
				DisplayName: "dns01-solver service account for cert-manager",
			},
		}
		result, err := iamService.Projects.ServiceAccounts.Create("projects/"+projectID, request).Do()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("cleanup the created GCP service account")
			_, err = iamService.Projects.ServiceAccounts.Delete(result.Name).Do()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("add IAM policy binding with role 'dns.admin' to GCP project")
		projectRole := "roles/dns.admin"
		projectMember := fmt.Sprintf("serviceAccount:%s", result.Email)
		updateIamPolicyBinding(crmService, projectID, projectRole, projectMember, true)
		defer func() {
			e2e.Logf("cleanup the added IAM policy binding from GCP project")
			updateIamPolicyBinding(crmService, projectID, projectRole, projectMember, false)
		}()

		exutil.By("link cert-manager service account to GCP service account with role 'iam.workloadIdentityUser'")
		resource := fmt.Sprintf("projects/-/serviceAccounts/%s", result.Email)
		serviceAccoutRole := "roles/iam.workloadIdentityUser"
		serviceAccoutMember := fmt.Sprintf("principal:%s/subject/system:serviceaccount:cert-manager:cert-manager", identifier)
		serviceAccountPolicy, err := iamService.Projects.ServiceAccounts.GetIamPolicy(resource).Do()
		o.Expect(err).NotTo(o.HaveOccurred())
		serviceAccountPolicy.Bindings = append(serviceAccountPolicy.Bindings, &gcpiam.Binding{
			Role:    serviceAccoutRole,
			Members: []string{serviceAccoutMember},
		})
		_, err = iamService.Projects.ServiceAccounts.SetIamPolicy(resource, &gcpiam.SetIamPolicyRequest{Policy: serviceAccountPolicy}).Do()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Opt not to use defer here to remove the IAM policy binding from the service account, as it will be cleaned up along with the service account deletion.

		exutil.By("create the GCP STS config secret manually")
		credContent := `{
			"type": "external_account",
			"audience": "%s/providers/%s",
			"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
			"token_url": "https://sts.googleapis.com/v1/token",
			"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/%s:generateAccessToken",
			"credential_source": {
				"file": "/var/run/secrets/openshift/serviceaccount/token",
				"format": {
					"type": "text"
				}
			}
		}`
		credContent = fmt.Sprintf(credContent, identifier, poolID, resource)
		err = oc.AsAdmin().Run("create").Args("-n", operandNamespace, "secret", "generic", stsSecretName, "--from-literal=service_account.json="+credContent).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("cleanup the created GCP STS secret")
			err := oc.AsAdmin().Run("delete").Args("-n", operandNamespace, "secret", stsSecretName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("patch the subscription to inject CLOUD_CREDENTIALS_SECRET_NAME env")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"config":{"env":[{"name":"CLOUD_CREDENTIALS_SECRET_NAME","value":"` + stsSecretName + `"}]}}}`
		err = oc.AsAdmin().Run("patch").Args("sub", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset subscription env")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"config":{"env":[]}}}`
			err = oc.AsAdmin().Run("patch").Args("sub", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("create a clusterissuer with Google Clould DNS as dns01 solver")
		issuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-clouddns-ambient.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "ACME_SERVER=" + acmeServerEndpoint, "PROJECT_ID=" + projectID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		defer func() {
			e2e.Logf("delete the clusterissuer")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuer", issuerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		err = waitForResourceReadiness(oc, "", "clusterissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("create a certificate")
		baseDomain := getBaseDomain(oc)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := getRandomString(4) + "." + dnsZone

		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "DNS_NAME=" + dnsName, "ISSUER_NAME=" + issuerName, "ISSUER_KIND=" + "ClusterIssuer", "COMMON_NAME=" + dnsName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})
})

var _ = g.Describe("[sig-oap] OAP cert-manager Vault issuer", func() {
	defer g.GinkgoRecover()
	var (
		oc                  = exutil.NewCLI("cert-manager-vault", exutil.KubeConfigPath())
		buildPruningBaseDir = exutil.FixturePath("testdata", "oap/certmanager")
	)
	g.BeforeEach(func() {
		if !isDeploymentReady(oc, operatorNamespace, operatorDeploymentName) {
			e2e.Logf("Creating Cert Manager Operator...")
			createCertManagerOperator(oc)
		}
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-High-65028-should work well when authenticating with Vault AppRole", func() {
		var (
			vaultReleaseName = "vault-" + getRandomString(4)
			vaultRoleName    = "cert-manager"
			vaultSecretName  = "cert-manager-vault-approle"
			issuerName       = "issuer-vault-approle"
			certName         = "cert-from-" + issuerName
		)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, vaultRootToken := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)
		configVaultPKI(oc, oc.Namespace(), vaultReleaseName, vaultPodName, vaultRootToken)

		exutil.By("configure auth with Vault AppRole")
		cmd := fmt.Sprintf(`vault auth enable approle && vault write auth/approle/role/%s token_policies="cert-manager" token_ttl=1h token_max_ttl=4h`, vaultRoleName)
		_, err := exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd = fmt.Sprintf(`vault read -format=json auth/approle/role/%s/role-id`, vaultRoleName)
		output, err := exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		vaultRoleID := gjson.Get(output, "data.role_id").String()
		cmd = fmt.Sprintf(`vault write -format=json -force auth/approle/role/%s/secret-id`, vaultRoleName)
		output, err = exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		vaultSecretID := gjson.Get(output, "data.secret_id").String()

		exutil.By("create the auth secret")
		oc.NotShowInfo()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", oc.Namespace(), "secret", "generic", vaultSecretName, "--from-literal=secretId="+vaultSecretID).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer using Vault AppRole")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-vault-approle.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "VAULT_SERVICE=" + vaultReleaseName, "VAULT_NAMESPACE=" + oc.Namespace(), "ROLE_ID=" + vaultRoleID, "SECRET_NAME=" + vaultSecretName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "COMMON_NAME=" + certName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-High-65029-should work well when authenticating with Vault token", func() {
		var (
			vaultReleaseName = "vault-" + getRandomString(4)
			vaultSecretName  = "cert-manager-vault-token"
			issuerName       = "issuer-vault-token"
			certName         = "cert-from-" + issuerName
		)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, vaultToken := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)
		configVaultPKI(oc, oc.Namespace(), vaultReleaseName, vaultPodName, vaultToken)

		exutil.By("configure auth with Vault token")
		cmd := `vault token create -policy=cert-manager -ttl=720h`
		_, err := exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create the auth secret")
		oc.NotShowInfo()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", oc.Namespace(), "secret", "generic", vaultSecretName, "--from-literal=token="+vaultToken).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer using Vault token")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-vault-token.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "VAULT_SERVICE=" + vaultReleaseName, "VAULT_NAMESPACE=" + oc.Namespace(), "SECRET_NAME=" + vaultSecretName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "COMMON_NAME=" + certName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-Low-65030-should work well when authenticating with Kubernetes static service account", func() {
		var (
			vaultReleaseName   = "vault-" + getRandomString(4)
			serviceAccountName = "cert-manager-vault-static-serviceaccount"
			issuerName         = "issuer-vault-static-serviceaccount"
			certName           = "cert-from-" + issuerName
		)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, vaultToken := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)
		configVaultPKI(oc, oc.Namespace(), vaultReleaseName, vaultPodName, vaultToken)

		exutil.By("create a long-lived API token for a service account")
		err := oc.Run("create").Args("serviceaccount", serviceAccountName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		secretFile := filepath.Join(buildPruningBaseDir, "secret-vault-static-sa-token.yaml")
		params := []string{"-f", secretFile, "-p", "SA_NAME=" + serviceAccountName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("configure auth with Kubernetes static service account")
		cmd := fmt.Sprintf(`vault auth enable kubernetes && vault write auth/kubernetes/config kubernetes_host="https://kubernetes.default.svc" kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt && \
vault write auth/kubernetes/role/issuer bound_service_account_names=%s bound_service_account_namespaces=%s token_policies=cert-manager ttl=1h`, serviceAccountName, oc.Namespace())
		_, err = exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer using Kubernetes static service account")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-vault-static-sa.yaml")
		params = []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "VAULT_SERVICE=" + vaultReleaseName, "VAULT_NAMESPACE=" + oc.Namespace(), "SECRET_NAME=" + serviceAccountName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "COMMON_NAME=" + certName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-High-66907-should work well when authenticating with Kubernetes bound service account through Kubernetes auth", func() {
		var (
			minSupportedVersion = "1.12.0"
			vaultReleaseName    = "vault-" + getRandomString(4)
			serviceAccountName  = "cert-manager-vault-bound-serviceaccount"
			issuerName          = "issuer-vault-bound-serviceaccount"
			certName            = "cert-from-" + issuerName
		)

		skipUnsupportedVersion(oc, minSupportedVersion)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, vaultToken := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)
		configVaultPKI(oc, oc.Namespace(), vaultReleaseName, vaultPodName, vaultToken)

		exutil.By("create RBAC resources for the service account to get tokens")
		err := oc.Run("create").Args("serviceaccount", serviceAccountName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		rbacFile := filepath.Join(buildPruningBaseDir, "rbac-vault-bound-sa.yaml")
		params := []string{"-f", rbacFile, "-p", "SA_NAME=" + serviceAccountName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("configure auth with Kubernetes bound service account")
		cmd := fmt.Sprintf(`vault auth enable kubernetes && vault write auth/kubernetes/config kubernetes_host="https://kubernetes.default.svc" kubernetes_ca_cert=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt && \
vault write auth/kubernetes/role/issuer bound_service_account_names=%s bound_service_account_namespaces=%s token_policies=cert-manager ttl=1h`, serviceAccountName, oc.Namespace())
		_, err = exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer using Kubernetes bound service account")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-vault-bound-sa.yaml")
		params = []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "VAULT_SERVICE=" + vaultReleaseName, "VAULT_NAMESPACE=" + oc.Namespace(), "VAULT_AUTH_PATH=kubernetes", "SA_NAME=" + serviceAccountName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "COMMON_NAME=" + certName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-High-76515-should work well when authenticating with Kubernetes bound service account through JWT/OIDC auth", func() {
		var (
			minSupportedVersion = "1.12.0"
			vaultReleaseName    = "vault-" + getRandomString(4)
			serviceAccountName  = "cert-manager-vault-bound-serviceaccount"
			issuerName          = "issuer-vault-bound-serviceaccount"
			certName            = "cert-from-" + issuerName
		)

		skipUnsupportedVersion(oc, minSupportedVersion)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, vaultToken := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)
		configVaultPKI(oc, oc.Namespace(), vaultReleaseName, vaultPodName, vaultToken)

		exutil.By("create RBAC resources for the service account to get tokens")
		err := oc.Run("create").Args("serviceaccount", serviceAccountName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		rbacFile := filepath.Join(buildPruningBaseDir, "rbac-vault-bound-sa.yaml")
		params := []string{"-f", rbacFile, "-p", "SA_NAME=" + serviceAccountName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("configure the JWT auth in Vault")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("--raw", "/.well-known/openid-configuration").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oidcIssuer := gjson.Get(output, "issuer").String()
		cmd := fmt.Sprintf(`vault auth enable jwt && vault write auth/jwt/config oidc_discovery_url=%s`, oidcIssuer)

		if strings.Contains(oidcIssuer, "kubernetes.default.svc") {
			// This is an workaround for non-STS envs, otherwise vault issuer will run into 'fetching keys oidc: get keys failed: 403 Forbidden' error.
			// The public keys under '/openid/v1/jwks' are non-sensitive, so there is no concern about granting access.
			exutil.By("create RBAC resources for anonymous user (vault) to get 'jwks_uri' in non-STS env")
			roleName := "vault-get-jwks-role-76515"
			rolebindingName := "vault-get-jwks-rolebinding-76515"
			err := oc.AsAdmin().WithoutNamespace().Run("create").Args("clusterrole", roleName, "--verb=get", "--non-resource-url=/openid/v1/jwks").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrole", roleName).Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("clusterrolebinding", rolebindingName, "--clusterrole="+roleName, "--group=system:unauthenticated").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding", rolebindingName).Execute()

			// In non-STS envs, OIDC issuer would be the internal URL 'https://kubernetes.default.svc'. Thus explicitly setting the certificate to validate connections is required.
			cmd += " oidc_discovery_ca_pem=@/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
		}
		_, err = exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd = fmt.Sprintf(`vault write auth/jwt/role/issuer role_type=jwt bound_audiences="vault://%s/%s" user_claim=sub bound_subject="system:serviceaccount:%s:%s" token_policies=cert-manager ttl=1m`, oc.Namespace(), issuerName, oc.Namespace(), serviceAccountName)
		_, err = exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer using Kubernetes bound service account")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-vault-bound-sa.yaml")
		params = []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "VAULT_SERVICE=" + vaultReleaseName, "VAULT_NAMESPACE=" + oc.Namespace(), "VAULT_AUTH_PATH=jwt", "SA_NAME=" + serviceAccountName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "COMMON_NAME=" + certName, "SECRET_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})
})

var _ = g.Describe("[sig-oap] OAP cert-manager", func() {
	defer g.GinkgoRecover()
	var (
		oc                  = exutil.NewCLI("cert-manager", exutil.KubeConfigPath())
		buildPruningBaseDir = exutil.FixturePath("testdata", "oap/certmanager")
	)
	g.BeforeEach(func() {
		if !isDeploymentReady(oc, operatorNamespace, operatorDeploymentName) {
			e2e.Logf("Creating Cert Manager Operator...")
			createCertManagerOperator(oc)
		}
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ARO-OSD_CCS-Medium-62006-operator can be uninstalled from CLI and then reinstalled [Serial]", func() {
		e2e.Logf("uninstall the cert-manager operator and cleanup its operand resources")
		cleanupCertManagerOperator(oc)

		e2e.Logf("install the cert-manager operator again")
		createCertManagerOperator(oc)

		e2e.Logf("check if the functionality is normal after the reinstall")
		createIssuer(oc, oc.Namespace())
		createCertificate(oc, oc.Namespace())
		verifyCertificate(oc, "default-selfsigned-cert", oc.Namespace())
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ARO-OSD_CCS-Low-63486-should not delete the TLS secret by default when its Certificate CR is deleted", func() {
		createIssuer(oc, oc.Namespace())
		createCertificate(oc, oc.Namespace())

		e2e.Logf("delete the issued certificate")
		err := oc.Run("delete").Args("certificate", "default-selfsigned-cert").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("waiting upto 30s to check if the TLS secret has not been removed")
		time.Sleep(30 * time.Second)
		err = oc.Run("get").Args("secret", "selfsigned-ca-tls").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-High-74267-can manage Route external TLS secret", func() {
		var (
			appImage        = "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
			serviceName     = "hello-openshift"
			routeType       = "edge"
			routeName       = "myroute"
			issuerName      = "default-selfsigned"
			certName        = "cert-from-" + issuerName
			secretName      = routeName + "-tls"
			podName         = "exec-curl-helper"
			secretMountPath = "/tmp"
		)

		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("Skipping as current cluster is not TechPreviewNoUpgrade")
		}

		exutil.By("create a testing App")
		err := oc.Run("new-app").Args(appImage).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, oc.Namespace(), 10*time.Second, 120*time.Second)

		exutil.By("specify the host name")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		hostName := constructDNSName(ingressDomain)

		exutil.By("create an edge Route for it")
		err = oc.Run("create").Args("route", routeType, routeName, "--service", serviceName, "--hostname", hostName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-selfsigned.yaml")
		err = oc.Run("create").Args("-f", issuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params := []string{"-f", certFile, "-p", "ISSUER_NAME=" + issuerName, "CERT_NAME=" + certName, "SECRET_NAME=" + secretName, "DNS_NAME=" + hostName, "COMMON_NAME=" + hostName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		exutil.By("grant the router service account access to load secret")
		rbacFile := filepath.Join(buildPruningBaseDir, "rbac-secret-reader.yaml")
		params = []string{"-f", rbacFile, "-p", "SECRET_NAME=" + secretName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("patch the issued certificate secret to the Route")
		patchPath := `{"spec":{"tls":{"externalCertificate":{"name":"` + secretName + `"}}}}`
		err = oc.AsAdmin().Run("patch").Args("route", routeName, "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create a helper pod to mount the TLS secret")
		podFile := filepath.Join(buildPruningBaseDir, "exec-curl-helper.yaml")
		params = []string{"-f", podFile, "-p", "POD_NAME=" + podName, "SECRET_NAME=" + secretName, "SECRET_MOUNT_PATH=" + secretMountPath}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, oc.Namespace(), 10*time.Second, 120*time.Second)

		exutil.By("validate the certificate in the helper pod")
		cmd := fmt.Sprintf(`curl -v -sS --cacert %s/ca.crt https://%s`, secretMountPath, hostName)

		statusErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ := exutil.RemoteShPod(oc, oc.Namespace(), podName, "sh", "-c", cmd)
			if strings.Contains(output, "200 OK") && strings.Contains(output, "CN="+hostName) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "timeout waiting for curl validation succeeded")

		exutil.By("verify if the certificate in secret was renewed")
		verifyCertificateRenewal(oc, oc.Namespace(), secretName, 150*time.Second)

		exutil.By("check the route is indeed serving with renewed(unexpired) certificate")
		statusErr = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			output, _ := exutil.RemoteShPod(oc, oc.Namespace(), podName, "sh", "-c", cmd)
			if strings.Contains(output, "200 OK") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "timeout waiting for route serving certificate renewed")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-LEVEL0-Medium-73293-certificates with duplicate secretName should not cause flood of re-issuance attempt", func() {
		var (
			minSupportedVersion = "1.14.0"
			issuerName          = "default-ca"
		)

		skipUnsupportedVersion(oc, minSupportedVersion)

		exutil.By("create a self-signed Issuer and Certificate")
		createIssuer(oc, oc.Namespace())
		createCertificate(oc, oc.Namespace())

		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-ca.yaml")
		certTemplate := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")

		exutil.By("create a CA Issuer")
		err := oc.Run("apply").Args("-f", issuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer 'default-ca' to become Ready")

		exutil.By("create 3 Certificates with the same secretName")
		certNames := []string{"duplicate-cert-1", "duplicate-cert-2", "duplicate-cert-3"}
		for _, name := range certNames {
			params := []string{"-f", certTemplate, "-p", "CERT_NAME=" + name, "ISSUER_NAME=" + issuerName, "SECRET_NAME=" + "secret-duplicate", "COMMON_NAME=" + name}
			exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		}

		// expect only 0 or 1 CertificateRequest to be created
		o.Consistently(func() bool {
			requestNum := 0
			for _, name := range certNames {
				output, err := oc.Run("get").Args("certificaterequest", `-o=jsonpath={.items[?(@.metadata.annotations.cert-manager.io/certificate-name==`+name+`)].spec.uid}`).Output()
				if err != nil {
					e2e.Logf("Error to get certificaterequest: %v", err)
					return false
				}
				if output != "" {
					requestNum++
				}
			}
			return requestNum <= 1
		}, 10*time.Second, 1*time.Second).Should(o.BeTrue(), "expect only 0 or 1 CertificateRequest to be created")

		// expect at most 1 Certificate to be Ready
		o.Consistently(func() bool {
			certReadyNum := 0
			for _, name := range certNames {
				output, err := oc.Run("get").Args("certificate", name, `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`).Output()
				if err != nil {
					e2e.Logf("Error to get certificate: %v", err)
					return false
				}
				if strings.Contains(output, "True") {
					certReadyNum++
				}
			}
			return certReadyNum <= 1
		}, 10*time.Second, 1*time.Second).Should(o.BeTrue(), "expect at most 1 Certificate to be Ready")

		exutil.By("update Certificates to make sure all should have an unique secretName")
		for i, name := range certNames {
			params := []string{"-f", certTemplate, "-p", "CERT_NAME=" + name, "ISSUER_NAME=" + issuerName, "SECRET_NAME=" + "secret-" + strconv.Itoa(i), "COMMON_NAME=" + name}
			exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		}

		// expect all Certificates to be Ready
		o.Eventually(func() bool {
			certReadyNum := 0
			for _, name := range certNames {
				output, err := oc.Run("get").Args("certificate", name, `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`).Output()
				if err != nil {
					e2e.Logf("Error to get certificate: %v", err)
					return false
				}
				if strings.Contains(output, "True") {
					certReadyNum++
				}
			}
			return certReadyNum == len(certNames)
		}, 180*time.Second, 10*time.Second).Should(o.BeTrue(), "expect all Certificates to be Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-Low-63583-operand metrics can be queried by using enabling user-workload-monitoring [Serial]", func() {
		var (
			clusterMonitoringNamespace      = "openshift-monitoring"
			clusterMonitoringConfigMapName  = "cluster-monitoring-config"
			userWorkloadMonitoringNamespace = "openshift-user-workload-monitoring"
			metricsQueryURL                 = "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query"
		)

		exutil.By("Check if the cluster-monitoring ConfigMap exists")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", clusterMonitoringConfigMapName, "-n", clusterMonitoringNamespace).Output()
		if err != nil {
			e2e.Logf("Got error(%v) when trying to get 'configmap/%s', command output: %s", err, clusterMonitoringConfigMapName, output)
			o.Expect(output).To(o.ContainSubstring("not found"))
		} else {
			e2e.Logf("The cluster-monitoring ConfigMap already exists, backup the origin YAML to revert")
			originConfigMapFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", clusterMonitoringConfigMapName, "-n", clusterMonitoringNamespace, "-oyaml").OutputToFile("63583-origin-cm.yaml")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", clusterMonitoringConfigMapName, "-n", clusterMonitoringNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func() {
				e2e.Logf("Revert to the origin ConfigMap")
				err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", originConfigMapFile).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("Delete backup-ed YAML file")
				os.Remove(originConfigMapFile)
			}()
		}

		exutil.By("Enable monitoring for user-defined projects")
		configFile := filepath.Join(buildPruningBaseDir, "cluster-monitoring-config.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Delete created ConfigMap")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", clusterMonitoringConfigMapName, "-n", clusterMonitoringNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, userWorkloadMonitoringNamespace, 10*time.Second, 120*time.Second)

		exutil.By("Create Service Monitor to collect metrics")
		serviceMonitorFile := filepath.Join(buildPruningBaseDir, "servicemonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-n", operandNamespace, "-f", serviceMonitorFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Delete created ServiceMonitor")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("servicemonitor", "cert-manager", "-n", operandNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("Prepare Prometheus SA token for making queries")
		token, err := getSAToken(oc, "prometheus-k8s", clusterMonitoringNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		exutil.By("Query metrics from HTTP API")
		queryString := `query={endpoint="tcp-prometheus-servicemonitor"}`
		cmd := fmt.Sprintf(`curl -s -S -k -H "Authorization: Bearer %s" %s --data-urlencode '%s'`, token, metricsQueryURL, queryString)
		oc.NotShowInfo()
		statusErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			output, err = exutil.RemoteShPod(oc, clusterMonitoringNamespace, "prometheus-k8s-0", "sh", "-c", cmd)
			if !strings.Contains(output, `"status":"success"`) || !strings.Contains(output, `"namespace":"`+operandNamespace+`"`) || err != nil {
				return false, nil
			}
			e2e.Logf("Query succeeded, metrics results: %s\n", output)
			return true, nil
		})
		oc.SetShowInfo()
		if statusErr != nil {
			e2e.Logf("Metrics results are not as expected: %s\n", output)
		}
		exutil.AssertWaitPollNoErr(statusErr, "timed out after 180s waiting query to be success and return expected results")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-Medium-65031-operand and operator log levels can be set [Serial]", func() {
		exutil.By("Set operands log level to an invalid value")
		patchPath := `{"spec":{"logLevel":"xxx"}}`
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("certmanager.operator", "cluster", "--type=merge", "-p", patchPath).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Unsupported value: "xxx"`))

		// The valid values can be "Normal", "Debug", "Trace", and "TraceAll", default is "Normal".
		exutil.By("Set operands log level to a valid value")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, operandNamespace, operandLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath = `{"spec":{"logLevel":"Trace"}}`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("certmanager.operator", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset operands log level")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, operandNamespace, operandLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath := `{"spec":{"logLevel":""}}`
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, operandNamespace, operandLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, operandNamespace, operandLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("Validate the operands log level")
		podList, err := exutil.GetAllPodsWithLabel(oc, operandNamespace, operandLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, pod := range podList {
			// Arg '--v=6' equals to 'Trace'
			args, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", operandNamespace, pod, "-o=jsonpath='{.spec.containers[*].args}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(args).To(o.ContainSubstring("--v=6"))

			// The logs include 'GET https://' means verbosity is indeed increased to '6'
			log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(pod, "-n", operandNamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(log, "GET https://")).To(o.BeTrue())
		}

		// No meaningful negative test for OPERATOR_LOG_LEVEL. Therefore no automation for negative test.

		// The valid values range from 1 to 10, default is 2.
		exutil.By("Set operator log level to a valid value")
		oldPodList, err = exutil.GetAllPodsWithLabel(oc, operatorNamespace, operatorLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath = `{"spec":{"config":{"env":[{"name":"OPERATOR_LOG_LEVEL","value":"6"}]}}}`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset operator log level")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, operatorNamespace, operatorLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"config":{"env":[{"name":"OPERATOR_LOG_LEVEL","value":"2"}]}}}`
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", subscriptionName, "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, operatorNamespace, operatorLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, operatorNamespace, operatorLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("Validate the operator log level")
		podList, err = exutil.GetAllPodsWithLabel(oc, operatorNamespace, operatorLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, pod := range podList {
			env, err := oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "pod", pod, "-n", operatorNamespace, "--list").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(env).To(o.ContainSubstring("OPERATOR_LOG_LEVEL=6"))

			// The logs include 'GET https://' means verbosity is indeed increased to '6'
			log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(pod, "-n", operatorNamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(log, "GET https://")).To(o.BeTrue())
		}
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-CPaasrunOnly-ConnectedOnly-Medium-71327-API groups should pass DAST scan", func() {
		configFile := filepath.Join(buildPruningBaseDir, "rapidast-config.yaml")
		rapidastScan(oc, oc.Namespace(), configFile)
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-NonPreRelease-PreChkUpgrade-ROSA-ARO-OSD_CCS-Medium-65134-needs prepare test data before OCP upgrade", func() {
		var (
			acmeIssuerName  = "letsencrypt-http01"
			sharedNamespace = "ocp-65134-shared-ns"
		)

		exutil.By("create a shared testing namespace")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", sharedNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create a selfsigned issuer and certificate")
		createIssuer(oc, sharedNamespace)
		createCertificate(oc, sharedNamespace)

		e2e.Logf("setup a private ACME server for testing")
		acmeServerEndpoint := setupPebbleServer(oc, oc.Namespace())

		exutil.By("create an ACME http01 issuer")
		acmeIssuerFile := filepath.Join(buildPruningBaseDir, "issuer-acme-http01.yaml")
		params := []string{"-f", acmeIssuerFile, "-p", "ISSUER_NAME=" + acmeIssuerName, "ACME_SERVER=" + acmeServerEndpoint}
		exutil.ApplyNsResourceFromTemplate(oc, sharedNamespace, params...)

		exutil.By("wait for the ACME http01 issuer to become Ready")
		err = waitForResourceReadiness(oc, sharedNamespace, "issuer", acmeIssuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, sharedNamespace, "issuer", acmeIssuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-NonPreRelease-PstChkUpgrade-ROSA-ARO-OSD_CCS-Medium-65134-functions should work normally after OCP upgrade", func() {
		var (
			selfsignedIssuerName = "default-selfsigned"
			selfsignedCertName   = "default-selfsigned-cert"
			acmeIssuerName       = "letsencrypt-http01"
			acmeCertName         = "cert-from-" + acmeIssuerName
			sharedNamespace      = "ocp-65134-shared-ns"
		)

		// check if the shared testing namespace exists first
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("namespace", sharedNamespace).Execute()
		if err != nil {
			g.Skip("Skip the PstChkUpgrade test as namespace '" + sharedNamespace + "' does not exist, PreChkUpgrade test did not finish successfully")
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("namespace", sharedNamespace, "--ignore-not-found").Execute()

		exutil.By("log the CSV post OCP upgrade")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", operatorNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("check the operator and operands pods status, all of them should be Ready")
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, operatorNamespace, 10*time.Second, 120*time.Second)
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, operandNamespace, 10*time.Second, 120*time.Second)

		exutil.By("check the existing issuer and certificate status, all of them should be Ready")
		resources := []struct {
			resourceType string
			resourceName string
		}{
			{"issuer", selfsignedIssuerName},
			{"certificate", selfsignedCertName},
			{"issuer", acmeIssuerName},
		}
		for _, r := range resources {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sharedNamespace, r.resourceType, r.resourceName).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("True"))
		}

		exutil.By("create a new certificate using the ACME http01 issuer")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := constructDNSName(ingressDomain)

		acmeCertFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params := []string{"-f", acmeCertFile, "-p", "ISSUER_NAME=" + acmeIssuerName, "CERT_NAME=" + acmeCertName, "DNS_NAME=" + dnsName, "COMMON_NAME=" + dnsName, "SECRET_NAME=" + acmeCertName}
		exutil.ApplyNsResourceFromTemplate(oc, sharedNamespace, params...)

		exutil.By("wait for the ACME http01 certificate to become Ready")
		err = waitForResourceReadiness(oc, sharedNamespace, "certificate", acmeCertName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, sharedNamespace, "certificate", acmeCertName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-CPaasrunOnly-ConnectedOnly-Low-77811-Google CAS issuer should integrate well as an external issuer", func() {
		var (
			projectID     = "openshift-qe"
			caPoolID      = "google-cas-issuer-cert-manager-sub"
			location      = "us-central1"
			saPrefix      = "test-private-77811-cas-"
			saSecretName  = "google-cas-sa-key"
			issuerName    = "external-issuer-google-cas"
			certName      = "cert-from-" + issuerName
			tlsSecretName = certName + "-tls"
		)

		exutil.SkipIfPlatformTypeNot(oc, "GCP")
		if id, _ := exutil.GetGcpProjectID(oc); id != projectID {
			e2e.Logf("current GCP project ID: %s", id)
			g.Skip("Skip as the CAS testing environment is only pre-setup under 'openshift-qe' project")
		}
		exutil.SkipOnProxyCluster(oc)

		exutil.By("create the GCP IAM and CloudResourceManager client")
		// Note that in Prow CI, the credentials source is automatically pre-configured to by the step 'openshift-extended-test'
		// See https://github.com/openshift/release/blob/69b2b9c4f28adcfcc5b9ff4820ecbd8d2582a3d7/ci-operator/step-registry/openshift-extended/test/openshift-extended-test-commands.sh#L43
		iamService, err := gcpiam.NewService(context.Background())
		o.Expect(err).NotTo(o.HaveOccurred())
		crmService, err := gcpcrm.NewService(context.Background())
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create a GCP service account")
		serviceAccountName := saPrefix + getRandomString(4)
		request := &gcpiam.CreateServiceAccountRequest{
			AccountId: serviceAccountName,
			ServiceAccount: &gcpiam.ServiceAccount{
				DisplayName: "google-cas-issuer service account for cert-manager",
			},
		}
		result, err := iamService.Projects.ServiceAccounts.Create("projects/"+projectID, request).Do()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("cleanup the created GCP service account")
			_, err = iamService.Projects.ServiceAccounts.Delete(result.Name).Do()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("add IAM policy binding with role 'privateca.certificateRequester' to GCP project")
		projectRole := "roles/privateca.certificateRequester"
		projectMember := fmt.Sprintf("serviceAccount:%s", result.Email)
		updateIamPolicyBinding(crmService, projectID, projectRole, projectMember, true)
		defer func() {
			e2e.Logf("cleanup the added IAM policy binding from GCP project")
			updateIamPolicyBinding(crmService, projectID, projectRole, projectMember, false)
		}()

		exutil.By("create key for the GCP service account and store as a secret")
		resource := fmt.Sprintf("projects/-/serviceAccounts/%s", result.Email)
		key, err := iamService.Projects.ServiceAccounts.Keys.Create(resource, &gcpiam.CreateServiceAccountKeyRequest{}).Do()
		o.Expect(err).NotTo(o.HaveOccurred())
		value, err := base64.StdEncoding.DecodeString(key.PrivateKeyData)
		o.Expect(err).NotTo(o.HaveOccurred())
		oc.NotShowInfo()
		err = oc.AsAdmin().Run("create").Args("-n", oc.Namespace(), "secret", "generic", saSecretName, "--from-literal=key.json="+string(value)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		oc.SetShowInfo()

		exutil.By("install the Google Certificate Authority Service Issuer")
		installGoogleCASIssuer(oc, oc.Namespace())

		exutil.By("create a Google CAS issuer")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-google-cas.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "PROJECT=" + projectID, "LOCATION=" + location, "CAPOOL_ID=" + caPoolID, "SA_SECRET=" + saSecretName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "googlecasissuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "googlecasissuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-generic.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName, "ISSUER_KIND=" + "GoogleCASIssuer", "ISSUER_GROUP=" + "cas-issuer.jetstack.io", "SECRET_NAME=" + tlsSecretName, "COMMON_NAME=" + certName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		exutil.By("verify if the issued certificate can be renewed automatically")
		verifyCertificateRenewal(oc, oc.Namespace(), tlsSecretName, 150*time.Second)
	})
})
