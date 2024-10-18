package apiserverauth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/blang/semver"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	gcpcrm "google.golang.org/api/cloudresourcemanager/v1"
	gcpiam "google.golang.org/api/iam/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-auth] CFE cert-manager", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("default-"+getRandomString(8), exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ConnectedOnly-High-62494-Use explicit credential in ACME dns01 solver with route53 to generate certificate", func() {
		exutil.By("Check proxy env.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "httpsProxy") {
			g.Skip("The cluster has httpsProxy, ocp-62494 skipped.")
		}

		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		exutil.By("Check if the cluster is STS or not")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system").Output()
		if err != nil && strings.Contains(output, "not found") {
			g.Skip("Skipping for the aws cluster without credential in cluster")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Remove the secret generic test-secret.")
			_, errSecret := oc.AsAdmin().Run("delete").Args("-n", "cert-manager", "secret", "test-secret").Output()
			o.Expect(errSecret).NotTo(o.HaveOccurred())
		}()
		e2e.Logf("Create secret generic test-secret.")
		cloudProvider := getCloudProvider(oc)
		accessKeyID, secureKey := getCredentialFromCluster(oc, cloudProvider)
		oc.NotShowInfo()
		_, errSec := oc.AsAdmin().Run("create").Args("-n", "cert-manager", "secret", "generic", "test-secret", "--from-literal=secret-access-key="+secureKey).Output()
		oc.SetShowInfo()
		o.Expect(errSec).NotTo(o.HaveOccurred())
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
			e2e.Logf("Delete clusterissuers.cert-manager.io letsencrypt-dns01")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "letsencrypt-dns01").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		baseDomain := getBaseDomain(oc)
		e2e.Logf("baseDomain=%s", baseDomain)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("dnsZone=%s", dnsZone)
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("Skipping test case for retreiving Route53 hosted zone ID for current env returns none")
		}
		e2e.Logf("Route53 HostedZoneID=%s", hostedZoneID)
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53.yaml")
		oc.NotShowInfo()
		params := []string{"-f", clusterIssuerTemplate, "-p", "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "AWS_ACCESS_KEY_ID=" + accessKeyID, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		oc.SetShowInfo()
		err = waitForResourceReadiness(oc, "", "clusterissuer", "letsencrypt-dns01", 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", "letsencrypt-dns01", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("create certificate which references previous clusterissuer")
		e2e.Logf("Create ns with normal user.")
		oc.SetupProject()
		dnsName := constructDNSName(dnsZone)
		certTemplate := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName, "ISSUER_NAME=" + "letsencrypt-dns01"}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", "certificate-from-dns01", 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", "certificate-from-dns01", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "certificate-from-dns01", oc.Namespace())
	})

	// author: geliu@redhat.com
	// This case contains three Polarion cases: 62063, 63325, and 63486. The root case is 62063.
	g.It("Author:geliu-ROSA-ARO-ConnectedOnly-High-62063-Use specified ingressclass in ACME http01 solver to generate certificate [Serial]", func() {
		skipIfRouteUnreachable(oc)

		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		output0, err0 := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec.trustedCA.name}").Output()
		if !strings.Contains(output, "httpsProxy") || err != nil || output0 == "" || err0 != nil {
			e2e.Logf("Fail to check httpsProxy, ocp-63325 skipped.")
		} else {
			// High-63325-Configure cert-manager to work in https proxy OpenShift env with trusted certificate authority
			defer func() {
				e2e.Logf("Delete configmap trusted-ca.")
				err = oc.AsAdmin().Run("delete").Args("-n", "cert-manager", "cm", "trusted-ca").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}()

			e2e.Logf("Create configmap trusted-ca.")
			_, err := oc.AsAdmin().Run("create").Args("-n", "cert-manager", "configmap", "trusted-ca").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().Run("label").Args("-n", "cert-manager", "cm", "trusted-ca", "config.openshift.io/inject-trusted-cabundle=true").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func() {
				e2e.Logf("Patch subscription for recovery.")
				patchPath1 := "{\"spec\":{\"config\":{\"env\":[]}}}"
				err0 := oc.AsAdmin().Run("patch").Args("-n", "cert-manager-operator", "sub", "openshift-cert-manager-operator", "--type=merge", "-p", patchPath1).Execute()
				o.Expect(err0).NotTo(o.HaveOccurred())
			}()
			e2e.Logf("patch sub openshift-cert-manager-operator.")
			patchPath := "{\"spec\":{\"config\":{\"env\":[{\"name\":\"TRUSTED_CA_CONFIGMAP_NAME\",\"value\":\"trusted-ca\"}]}}}"
			err0 := oc.AsAdmin().Run("patch").Args("-n", "cert-manager-operator", "sub", "openshift-cert-manager-operator", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err0).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", "cert-manager", "cert-manager", "-o=jsonpath={.spec.template.spec.containers[0].volumeMounts}").Output()
				output1, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", "cert-manager", "cert-manager", "-o=jsonpath={.spec.template.spec.volumes}").Output()
				if !strings.Contains(output, "trusted-ca") || err != nil || !strings.Contains(output1, "trusted-ca") || err1 != nil {
					e2e.Logf("cert-manager deployment is not ready.")
					return false, nil
				}
				e2e.Logf("cert-manager deployment is ready.")
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "Waiting for deployment times out.")
		}

		e2e.Logf("Login with normal user and create new ns.")
		oc.SetupProject()
		e2e.Logf("Create issuer in ns scope created in last step.")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		issuerHTTP01File := filepath.Join(buildPruningBaseDir, "issuer-acme-http01.yaml")
		err = oc.Run("create").Args("-f", issuerHTTP01File).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", "letsencrypt-http01", 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", "letsencrypt-http01", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		e2e.Logf("As the normal user, create certificate.")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}", "--context=admin").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := constructDNSName(ingressDomain)
		certHTTP01File := filepath.Join(buildPruningBaseDir, "cert-test-http01.yaml")
		params := []string{"-f", certHTTP01File, "-p", "ISSUER_NAME=" + "letsencrypt-http01", "CERT_NAME=" + "cert-test-http01", "DNS_NAME=" + dnsName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", "cert-test-http01", 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", "cert-test-http01", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "cert-test-http01", oc.Namespace())

		// Low-63486-When a Certificate CR is deleted its certificate secret should not be deleted
		e2e.Logf("Delete certification for ocp-63486.\n")
		err = oc.Run("delete").Args("certificate", "cert-test-http01").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ocp-63486: Waiting 1 min to ensure secret have not be removed.\n")
		time.Sleep(60 * time.Second)
		err = oc.Run("get").Args("secret", "cert-test-http01-tls").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ARO-ConnectedOnly-Medium-62006-RH cert-manager operator can be uninstalled from CLI and then reinstalled [Serial]", func() {
		e2e.Logf("Create an issuer and certificate before performing deletion")
		createIssuer(oc, oc.Namespace())
		createCertificate(oc, oc.Namespace())
		e2e.Logf("Verify the issued certificate")
		verifyCertificate(oc, "default-selfsigned-cert", oc.Namespace())

		e2e.Logf("Delete subscription and csv")
		csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "openshift-cert-manager-operator", "-n", "cert-manager-operator", "-o=jsonpath={.status.installedCSV}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("sub", "openshift-cert-manager-operator", "-n", "cert-manager-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("csv", csvName, "-n", "cert-manager-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get certmanager operator pods, it should be gone.\n")
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "cert-manager-operator", "pod").Output()
			if !strings.Contains(output, "No resources found") || err != nil {
				e2e.Logf("operator pod still exist\n.")
				return false, nil
			}
			e2e.Logf("operator pod deleted as expected.\n")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "operator pod have not been deleted.")

		e2e.Logf("Check cert-manager CRDs and apiservices still exist as expected.\n")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "cert-manager") {
			e2e.Logf("existing crds:\n%v", output)
			e2e.Failf("crds don't contain cert-manager")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("apiservice").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "cert-manager") {
			e2e.Logf("existing apiservices:\n%v", output)
			e2e.Failf("apiservices don't contain cert-manager")
		}
		e2e.Logf("Clean up cert-manager-operator NS.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "cert-manager-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Delete operand.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "cert-manager").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Patching certmanager/cluster with null finalizers is required, otherwise the delete commands can be stuck.\n")
		patchPath := "{\"metadata\":{\"finalizers\":null}}"
		err = oc.AsAdmin().Run("patch").Args("certmanagers.operator", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Delete certmanagers.operator cluster.\n")
		err = oc.AsAdmin().Run("delete").Args("certmanagers.operator", "cluster").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Delete cert-manager CRDs")
		crdList, err := oc.AsAdmin().Run("get").Args("crd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		regexstr, _ := regexp.Compile(".*" + "cert-?manager" + "[0-9A-Za-z-.]*")
		crdListArry := regexstr.FindAllString(crdList, -1)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(append([]string{"crd"}, crdListArry...)...).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		statusErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
			err = oc.AsAdmin().Run("get").Args("issuer").Execute()
			if err != nil { // We expect the err to be not nil
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "timeout waiting for the cert-manager's CRDs deletion to take effect")

		e2e.Logf("Check the clusterroles and clusterrolebindings remainders")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "cert-manager") {
			e2e.Logf("existing clusterrole:\n%v", output)
			e2e.Failf("clusterroles don't contain cert-manager")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrolebinding").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "cert-manager") {
			e2e.Logf("existing clusterrolebinding:\n%v", output)
			e2e.Failf("clusterrolebindings don't contain cert-manager")
		}

		e2e.Logf("Clean up the clusterroles and clusterrolebindings remainders")
		clusterroleList, err := oc.AsAdmin().Run("get").Args("clusterrole").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		regexstr, _ = regexp.Compile(".*" + "cert-?manager" + "[0-9A-Za-z-.:]*")
		clusterroleListArry := regexstr.FindAllString(clusterroleList, -1)
		_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args(append([]string{"clusterrole"}, clusterroleListArry...)...).Execute()
		// Some clusterrole resources returned by `oc get` may be automatically deleted. In such case, `NotTo(o.HaveOccurred())` assertion may fail with "xxxx" not found for those resources. So comment out the assertion.
		// o.Expect(err).NotTo(o.HaveOccurred())
		clusterrolebindingList, err := oc.AsAdmin().Run("get").Args("clusterrolebinding").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		regexstr, _ = regexp.Compile("(?m)^[^ ]*cert-?manager[^ ]*")
		clusterrolebindingListArry := regexstr.FindAllString(clusterrolebindingList, -1)
		_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args(append([]string{"clusterrolebinding"}, clusterrolebindingListArry...)...).Execute()
		// Some clusterrolebinding resources returned by `oc get` may be automatically deleted. In such case, `NotTo(o.HaveOccurred())` assertion may fail with "xxxx" not found for those resources. So comment out the assertion.
		// o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Install the cert-manager operator again")
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ConnectedOnly-Medium-62582-Need override dns args when the target hosted zone in ACME dns01 solver overlaps with the cluster's default private hosted zone [Disruptive]", func() {
		exutil.By("Check proxy env.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "httpsProxy") {
			g.Skip("The cluster has httpsProxy, ocp-62582 skipped.")
		}

		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		exutil.By("Skip test when the cluster is with STS credential")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system").Output()
		if err != nil && strings.Contains(output, "not found") {
			g.Skip("Skipping for the aws cluster without credential in cluster")
		}
		e2e.Logf("Create secret generic test-secret.")
		cloudProvider := getCloudProvider(oc)
		accessKeyID, secureKey := getCredentialFromCluster(oc, cloudProvider)
		oc.NotShowInfo()
		defer func() {
			e2e.Logf("Remove the secret generic test-secret.")
			_, errSecret := oc.AsAdmin().Run("delete").Args("-n", "cert-manager", "secret", "test-secret").Output()
			o.Expect(errSecret).NotTo(o.HaveOccurred())
		}()
		_, errSec := oc.AsAdmin().Run("create").Args("-n", "cert-manager", "secret", "generic", "test-secret", "--from-literal=secret-access-key="+secureKey).Output()
		oc.SetShowInfo()
		o.Expect(errSec).NotTo(o.HaveOccurred())
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
		e2e.Logf("baseDomain=%s", baseDomain)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("dnsZone=%s", dnsZone)
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("Skipping test case for retreiving Route53 hosted zone ID for current env returns none")
		}
		e2e.Logf("Route53 HostedZoneID=%s", hostedZoneID)
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-overlapped-zone.yaml")
		oc.NotShowInfo()
		params := []string{"-f", clusterIssuerTemplate, "-p", "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "AWS_ACCESS_KEY_ID=" + accessKeyID, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		oc.SetShowInfo()
		defer func() {
			e2e.Logf("Delete clusterissuers.")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "hosted-zone-overlapped").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = waitForResourceReadiness(oc, "", "clusterissuer", "hosted-zone-overlapped", 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", "hosted-zone-overlapped", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("create certificate which references previous clusterissuer")
		e2e.Logf("Create ns with normal user.")
		oc.SetupProject()
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := constructDNSName(ingressDomain)
		certTemplate := filepath.Join(buildPruningBaseDir, "cert-hosted-zone-overlapped.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		statusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("challenge", "-o", "wide").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "returned REFUSED") {
				e2e.Logf("challenge output return 'REFUSED' as expected. %v ", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "challenge/certificate is wrong.")

		exutil.By("Apply dns args by patch.")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, "cert-manager", "app=cert-manager")
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"controllerConfig":{"overrideArgs":["--dns01-recursive-nameservers=1.1.1.1:53", "--dns01-recursive-nameservers-only"]}}}`
		err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unpatch dns args")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, "cert-manager", "app=cert-manager")
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"controllerConfig":{"overrideArgs":null}}}`
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, "cert-manager", "app=cert-manager", oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, "cert-manager", "app=cert-manager", oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("Check the certificate content AGAIN.")
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", "certificate-hosted-zone-overlapped", 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", "certificate-hosted-zone-overlapped", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "certificate-hosted-zone-overlapped", oc.Namespace())
	})

	// author: geliu@redhat.com
	// This case contains two Polarion cases: 63555 and 69798. The root case is 63555.
	g.It("Author:geliu-ROSA-ConnectedOnly-Medium-63555-ACME dns01 solver should work in OpenShift proxy env [Serial]", func() {
		exutil.By("Check proxy env.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "httpsProxy") {
			g.Skip("Fail to check httpsProxy, ocp-63555 skipped.")
		}

		exutil.By("Skip test when the cluster is with STS credential")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system").Output()
		if err != nil && strings.Contains(output, "not found") {
			g.Skip("Skipping for the aws cluster without credential in cluster")
		}
		e2e.Logf("Create secret generic test-secret.")
		cloudProvider := getCloudProvider(oc)
		accessKeyID, secureKey := getCredentialFromCluster(oc, cloudProvider)
		oc.NotShowInfo()
		defer func() {
			e2e.Logf("Remove the secret generic test-secret.")
			_, errSecret := oc.AsAdmin().Run("delete").Args("-n", "cert-manager", "secret", "test-secret").Output()
			o.Expect(errSecret).NotTo(o.HaveOccurred())
		}()
		_, errSec := oc.AsAdmin().Run("create").Args("-n", "cert-manager", "secret", "generic", "test-secret", "--from-literal=secret-access-key="+secureKey).Output()
		oc.SetShowInfo()
		o.Expect(errSec).NotTo(o.HaveOccurred())
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		awsConfig, err := config.LoadDefaultConfig(
			context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
			config.WithRegion(region),
		)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Login with normal user and create issuers.\n")
		oc.SetupProject()
		baseDomain := getBaseDomain(oc)
		e2e.Logf("baseDomain=%s", baseDomain)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("dnsZone=%s", dnsZone)
		hostedZoneID := getRoute53HostedZoneID(awsConfig, dnsZone)
		if len(hostedZoneID) == 0 {
			g.Skip("Skipping test case for retreiving Route53 hosted zone ID for current env returns none")
		}
		e2e.Logf("Route53 HostedZoneID=%s", hostedZoneID)
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerTemplate := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-dns01-route53.yaml")
		oc.NotShowInfo()
		params := []string{"-f", clusterIssuerTemplate, "-p", "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "AWS_ACCESS_KEY_ID=" + accessKeyID, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
		exutil.ApplyClusterResourceFromTemplate(oc, params...)
		oc.SetShowInfo()
		defer func() {
			e2e.Logf("Delete clusterissuers.cert-manager.io letsencrypt-dns01")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "letsencrypt-dns01").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = waitForResourceReadiness(oc, "", "clusterissuer", "letsencrypt-dns01", 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", "letsencrypt-dns01", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		exutil.By("Create the certificate.")
		dnsName := constructDNSName(dnsZone)
		certTemplate := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName, "ISSUER_NAME=" + "letsencrypt-dns01"}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		exutil.By("Check the certificate and its challenge")
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("challenge").Output()
			if !strings.Contains(output, "pending") || err != nil {
				e2e.Logf("challenge is not become pending.%v", output)
				return false, nil
			}
			e2e.Logf("challenge is become pending status.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Fail to wait challenge become pending status.")
		err = wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
			challenge, err := oc.Run("get").Args("challenge", "-o", "wide").Output()
			if !strings.Contains(challenge, "i/o timeout") || err != nil {
				e2e.Logf("challenge has not output as expected.")
				return false, nil
			}
			e2e.Logf("challenge have output as expected.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failure: challenge has not output as expected.")
		exutil.By("Apply dns args by patch.")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, "cert-manager", "app=cert-manager")
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"controllerConfig":{"overrideArgs":["--dns01-recursive-nameservers-only"]}}}`
		err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unpatch dns args")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, "cert-manager", "app=cert-manager")
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"controllerConfig":{"overrideArgs":null}}}`
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, "cert-manager", "app=cert-manager", oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, "cert-manager", "app=cert-manager", oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("Checke certificate again.")
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", "certificate-from-dns01", 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", "certificate-from-dns01", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "certificate-from-dns01", oc.Namespace())

		// author: yuewu@redhat.com
		// Medium-69798-ACME dns01 solver should work in OpenShift proxy env with DNS over HTTPS (DoH) for doing the self-checks
		currentVersion, _ := semver.Parse(getCertManagerOperatorVersion(oc))
		minDoHSupportedVersion, _ := semver.Parse("1.13.0")
		// semverA.Compare(semverB) > -1 means 'semverA' greater than or equal to 'semverB', see: https://pkg.go.dev/github.com/blang/semver#Version.Compare
		if currentVersion.Compare(minDoHSupportedVersion) > -1 {
			e2e.Logf("Start to execute test case OCP-69798\n")

			exutil.By("Configure with an invalid server as negative test.")
			patchPath = "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":[\"--dns01-recursive-nameservers-only\", \"--dns01-recursive-nameservers=https://1.1.1.1/negative-test-dummy-dns-query\"]}}}"
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertAllPodsToBeReadyWithPollerParams(oc, "cert-manager", 10*time.Second, 120*time.Second)

			exutil.By("Create a new certificate.")
			dnsName := constructDNSName(dnsZone)
			certTemplate = filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
			params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName, "ISSUER_NAME=" + "letsencrypt-dns01"}
			exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

			exutil.By("Check if challenge will be pending and show HTTP 403 error")
			statusErr := wait.Poll(10*time.Second, 90*time.Second, func() (bool, error) {
				output, err = oc.Run("get").Args("challenge", "-o", "wide").Output()
				if !strings.Contains(output, "403 Forbidden") || !strings.Contains(output, "pending") || err != nil {
					e2e.Logf("challenge is still in processing, and status is not as expected: %s\n", output)
					return false, nil
				}
				e2e.Logf("challenge's output is as expected: %s\n", output)
				return true, nil
			})
			exutil.AssertWaitPollNoErr(statusErr, "timed out after 90s waiting challenge to be pending state and show HTTP 403 error")

			exutil.By("Configure with a valid server.")
			patchPath = "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":[\"--dns01-recursive-nameservers-only\", \"--dns01-recursive-nameservers=https://1.1.1.1/dns-query\"]}}}"
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertAllPodsToBeReadyWithPollerParams(oc, "cert-manager", 10*time.Second, 120*time.Second)

			exutil.By("Check if certificate will be True.")
			err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", "certificate-from-dns01", 10*time.Second, 300*time.Second)
			if err != nil {
				dumpResource(oc, oc.Namespace(), "certificate", "certificate-from-dns01", "-o=yaml")
			}
			exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

			exutil.By("Check and verify issued certificate content")
			verifyCertificate(oc, "certificate-from-dns01", oc.Namespace())
		} else {
			e2e.Logf("currentVersion(%s) < minDoHSupportedVersion(%s), therefore skipping the DoH checkpoint test (case 69798)", currentVersion, minDoHSupportedVersion)
		}
	})

	// author: geliu@redhat.com
	g.It("Author:geliu-ROSA-ARO-OSD_CCS-ConnectedOnly-Low-63500-Multiple solvers mixed with http01 and dns01 in ACME issuer should work well", func() {
		exutil.By("Create a clusterissuer which has multiple solvers mixed with http01 and dns01.")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-multiple-solvers.yaml")
		defer func() {
			e2e.Logf("Delete clusterissuers.")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "acme-multiple-solvers").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterIssuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = waitForResourceReadiness(oc, "", "clusterissuer", "acme-multiple-solvers", 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, "", "clusterissuer", "acme-multiple-solvers", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for clusterissuer to become Ready")

		e2e.Logf("Create ns with normal user.")
		oc.SetupProject()

		exutil.By("As normal user, create below 3 certificates in later steps with above clusterissuer.")
		e2e.Logf("Create cert, cert-match-test-1.")
		buildPruningBaseDir = exutil.FixturePath("testdata", "apiserverauth/certmanager")
		certFile1 := filepath.Join(buildPruningBaseDir, "cert-match-test-1.yaml")
		err = oc.Run("create").Args("-f", certFile1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("challenge").Output()
			if !strings.Contains(output, "pending") || err != nil {
				e2e.Logf("challenge1 is not become pending.%v", output)
				return false, nil
			}
			e2e.Logf("challenge1 is become pending status.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Fail to wait challenge1 become pending status.")
		challenge1, err := oc.AsAdmin().Run("get").Args("challenge", "-o=jsonpath={.items[*].spec.solver.selector.matchLabels}").Output()
		if !strings.Contains(challenge1, `"use-http01-solver":"true"`) || err != nil {
			e2e.Failf("challenge1 has not output as expected.")
		}
		err = oc.Run("delete").Args("cert/cert-match-test-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Create cert, cert-match-test-2.")
		buildPruningBaseDir = exutil.FixturePath("testdata", "apiserverauth/certmanager")
		certFile2 := filepath.Join(buildPruningBaseDir, "cert-match-test-2.yaml")
		err = oc.Run("create").Args("-f", certFile2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("challenge").Output()
			if !strings.Contains(output, "pending") || err != nil {
				e2e.Logf("challenge2 is not become pending.%v", output)
				return false, nil
			}
			e2e.Logf("challenge2 is become pending status.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Fail to wait challenge2 become pending status.")
		challenge2, err := oc.Run("get").Args("challenge", "-o=jsonpath={.items[*].spec.solver.selector.dnsNames}").Output()
		if !strings.Contains(challenge2, "xxia-test-2.test-example.com") || err != nil {
			e2e.Failf("challenge2 has not output as expected.")
		}
		err = oc.Run("delete").Args("cert/cert-match-test-2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Create cert, cert-match-test-3.")
		buildPruningBaseDir = exutil.FixturePath("testdata", "apiserverauth/certmanager")
		certFile3 := filepath.Join(buildPruningBaseDir, "cert-match-test-3.yaml")
		err = oc.Run("create").Args("-f", certFile3).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("challenge").Output()
			if !strings.Contains(output, "pending") || err != nil {
				e2e.Logf("challenge3 is not become pending.%v", output)
				return false, nil
			}
			e2e.Logf("challenge3 is become pending status.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Fail to wait challenge3 become pending status.")
		challenge3, err := oc.Run("get").Args("challenge", "-o=jsonpath={.items[*].spec.solver.selector.dnsZones}").Output()
		if !strings.Contains(challenge3, "test-example.com") || err != nil {
			e2e.Failf("challenge3 has not output as expected.")
		}
	})

	// author: yuewu@redhat.com
	// This case contains two Polarion cases: 62500 and 65132. The root case is 62500.
	g.It("Author:yuewu-ROSA-ConnectedOnly-High-62500-Use IRSA as ambient credential in AWS STS env for ACME dns01 route53 solver to generate certificate [Serial] [Flaky]", func() {
		const (
			rolePrefix          = "test-private-62500-sts-"
			policyPrefix        = "test-private-62500-dns01-"
			controllerNamespace = "cert-manager"
			controllerLabel     = "app.kubernetes.io/name=cert-manager"
			issuerName          = "route53-ambient"
			certName62500       = "certificate-from-dns01-route53-pod-identity-webhook"
			certName65132       = "certificate-from-dns01-route53-manual-patch"
			stsSecretName       = "aws-sts-creds"
		)

		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "httpsProxy") || strings.Contains(output, "httpProxy") {
			g.Skip("This case can run in STS proxy env. Handling proxy env needs to use cert-manager flag '--dns01-recursive-nameservers-only', which is already covered in OCP-63555. For simplicity, skipping proxy configured cluster for this case.")
		}

		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		if !exutil.IsSTSCluster(oc) {
			g.Skip("Skip for non-STS cluster")
		}

		exutil.By("prepare the AWS config, STS and IAM client")
		// AWS config
		// Note that in Prow CI, the credentials source is automatically pre-configured to by the step 'openshift-extended-test'
		// See https://github.com/openshift/release/blob/69b2b9c4f28adcfcc5b9ff4820ecbd8d2582a3d7/ci-operator/step-registry/openshift-extended/test/openshift-extended-test-commands.sh#L41
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		awsConfig, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
		o.Expect(err).NotTo(o.HaveOccurred())

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
						"Federated": "arn:aws:iam::%s:oidc-provider/%s"
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
		roleTrustPolicy = fmt.Sprintf(roleTrustPolicy, accountID, oidcProvider, oidcProvider)

		roleName := rolePrefix + getRandomString(4)
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
					"Resource": "arn:aws:route53:::change/*"
				},
				{
					"Effect": "Allow",
					"Action": [
						"route53:ChangeResourceRecordSets",
						"route53:ListResourceRecordSets"
					],
					"Resource": "arn:aws:route53:::hostedzone/*"
				},
				{
					"Effect": "Allow",
					"Action": "route53:ListHostedZonesByName",
					"Resource": "*"
				}
			]
		}`
		policyName := policyPrefix + getRandomString(4)
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
		// record old controller pod's name
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("sa/cert-manager", "eks.amazonaws.com/role-arn="+roleARN, "-n", controllerNamespace, "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			// double check if the anno was removed, in case any interruptions occurred before case 65132.
			output, err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("sa/cert-manager", "-n", controllerNamespace, "--list").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "eks.amazonaws.com/role-arn") {
				oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("de-annotate the role-arn from the cert-manager ServiceAccount")
				err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("sa/cert-manager", "eks.amazonaws.com/role-arn-", "-n", controllerNamespace, "--overwrite").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-l", controllerLabel, "-n", controllerNamespace).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
			}
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

		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		issuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-route53-ambient-credential.yaml")
		params := []string{"-f", issuerFile, "-p", "DNS_ZONE=" + dnsZone, "AWS_REGION=" + region, "ROUTE53_HOSTED_ZONE_ID=" + hostedZoneID}
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

		certFile := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName62500, "DNS_NAME=" + dnsName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName62500, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName62500, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for 62500's certificate to become Ready")

		// author: yuewu@redhat.com
		// Low-65132-CLOUD_CREDENTIALS_SECRET_NAME should work in AWS STS cluster when pod identity webhook is not used
		exutil.By("de-annotate the role-arn from the cert-manager ServiceAccount (used by case 62500)")
		oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("sa/cert-manager", "eks.amazonaws.com/role-arn-", "-n", controllerNamespace, "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-l", controllerLabel, "-n", controllerNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("create the STS config secret manually")
		credContent := fmt.Sprintf("[default]\nsts_regional_endpoints = regional\nrole_arn = %s\nweb_identity_token_file = /var/run/secrets/openshift/serviceaccount/token", roleARN)
		err = oc.AsAdmin().Run("create").Args("-n", "cert-manager", "secret", "generic", stsSecretName, "--from-literal=credentials="+credContent).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("delete the manually created STS secret")
			err := oc.AsAdmin().Run("delete").Args("-n", "cert-manager", "secret", stsSecretName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("patch the subscription to inject CLOUD_CREDENTIALS_SECRET_NAME env")
		oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"config":{"env":[{"name":"CLOUD_CREDENTIALS_SECRET_NAME","value":"` + stsSecretName + `"}]}}}`
		err = oc.AsAdmin().Run("patch").Args("sub", "openshift-cert-manager-operator", "-n", "cert-manager-operator", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("un-patch the subscription")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"config":{"env":[]}}}`
			err = oc.AsAdmin().Run("patch").Args("sub", "openshift-cert-manager-operator", "-n", "cert-manager-operator", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("create another certificate")
		dnsName = getRandomString(4) + "." + dnsZone

		certFile = filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName65132, "DNS_NAME=" + dnsName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName65132, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName65132, "-o=yaml")
			e2e.Logf("listing envs of the controller pod, it should contain 'AWS_SDK_LOAD_CONFIG=1'")
			_ = oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "-l", controllerLabel, "-n", controllerNamespace, "--list").Execute()
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for 65132's certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ConnectedOnly-High-62946-Use Google workload identity federation as ambient credential in GCP STS env for ACME dns01 cloudDNS solver to generate certificate [Serial]", func() {
		const (
			serviceAccountPrefix = "test-private-62946-dns01-"
			controllerNamespace  = "cert-manager"
			controllerLabel      = "app.kubernetes.io/name=cert-manager"
			issuerName           = "google-clouddns-ambient"
			certName             = "certificate-from-dns01-clouddns"
			stsSecretName        = "gcp-sts-creds"
		)

		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "httpsProxy") || strings.Contains(output, "httpProxy") {
			g.Skip("This case can run in STS proxy env. Handling proxy env needs to use cert-manager flag '--dns01-recursive-nameservers-only', which is already covered in OCP-63555. For simplicity, skipping proxy configured cluster for this case.")
		}

		exutil.SkipIfPlatformTypeNot(oc, "GCP")
		if !exutil.IsSTSCluster(oc) {
			g.Skip("Skip for non-STS cluster")
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
		err = oc.AsAdmin().Run("create").Args("-n", "cert-manager", "secret", "generic", stsSecretName, "--from-literal=service_account.json="+credContent).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("cleanup the created GCP STS secret")
			err := oc.AsAdmin().Run("delete").Args("-n", "cert-manager", "secret", stsSecretName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("patch the subscription to inject CLOUD_CREDENTIALS_SECRET_NAME env")
		oldPodList, err := exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		patchPath := `{"spec":{"config":{"env":[{"name":"CLOUD_CREDENTIALS_SECRET_NAME","value":"` + stsSecretName + `"}]}}}`
		err = oc.AsAdmin().Run("patch").Args("sub", "openshift-cert-manager-operator", "-n", "cert-manager-operator", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("un-patch the subscription")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, controllerNamespace, controllerLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"config":{"env":[]}}}`
			err = oc.AsAdmin().Run("patch").Args("sub", "openshift-cert-manager-operator", "-n", "cert-manager-operator", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)
		}()
		waitForPodsToBeRedeployed(oc, controllerNamespace, controllerLabel, oldPodList, 10*time.Second, 120*time.Second)

		exutil.By("create a clusterissuer with Google Clould DNS as dns01 solver")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		issuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-clouddns-ambient-credential.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "PROJECT_ID=" + projectID}
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

		certFile := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "DNS_NAME=" + dnsName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-High-74267-Route TLS secret can be managed by cert-manager", func() {
		const (
			appImage        = "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
			serviceName     = "hello-openshift"
			routeType       = "edge"
			routeName       = "myroute"
			certName        = "myroute-cert"
			secretName      = "myroute-tls"
			issuerName      = "default-selfsigned"
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
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-selfsigned.yaml")
		err = oc.Run("create").Args("-f", issuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-selfsigned-route.yaml")
		params := []string{"-f", certFile, "-p", "ISSUER_NAME=" + issuerName, "CERT_NAME=" + certName, "SECRET_NAME=" + secretName, "DNS_NAME=" + hostName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")

		// store the initial cert expire time for verifying renewal in the end
		initialExpireTime, _ := getCertificateExpireTime(oc, oc.Namespace(), secretName)
		e2e.Logf("certificate initial expire time: %v ", initialExpireTime)

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
		var currentExpireTime time.Time
		statusErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			currentExpireTime, err = getCertificateExpireTime(oc, oc.Namespace(), secretName)
			if err != nil {
				e2e.Logf("got error in func 'getCertificateExpireTime':\n%v", err)
				return false, nil
			}

			// returns Ture if currentExpireTime > initialExpireTime, indicates cert got renewed.
			if currentExpireTime.After(initialExpireTime) {
				return true, nil
			}
			return false, nil
		})
		e2e.Logf("certificate current expire time: %v ", currentExpireTime)
		exutil.AssertWaitPollNoErr(statusErr, "timeout waiting for certificate in secret to be renewed")

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
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-LEVEL0-Medium-73293-Certificates with duplicate secretName should not cause flood of re-issuance attempt", func() {
		const (
			minSupportedVersion = "1.14.0"
		)

		skipUnsupportedVersion(oc, minSupportedVersion)

		exutil.By("create a self-signed Issuer and Certificate")
		createIssuer(oc, oc.Namespace())
		createCertificate(oc, oc.Namespace())

		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-ca.yaml")
		certTemplate := filepath.Join(buildPruningBaseDir, "cert-duplicate-secret.yaml")

		exutil.By("create a CA Issuer")
		err := oc.Run("apply").Args("-f", issuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", "default-ca", 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", "default-ca", "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer 'default-ca' to become Ready")

		exutil.By("create 3 Certificates with the same secretName")
		certNames := []string{"duplicate-cert-1", "duplicate-cert-2", "duplicate-cert-3"}
		for _, name := range certNames {
			params := []string{"-f", certTemplate, "-p", "CERT_NAME=" + name, "SECRET_NAME_SUFFIX=" + "duplicate"}
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
			params := []string{"-f", certTemplate, "-p", "CERT_NAME=" + name, "SECRET_NAME_SUFFIX=" + strconv.Itoa(i)}
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
	g.It("Author:yuewu-ConnectedOnly-Low-63583-Check operand metrics by using user-workload-monitoring [Serial]", func() {
		const (
			operandNamespace                = "cert-manager"
			clusterMonitoringNamespace      = "openshift-monitoring"
			clusterMonitoringConfigMapName  = "cluster-monitoring-config"
			userWorkloadMonitoringNamespace = "openshift-user-workload-monitoring"
			metricsQueryURL                 = "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query"
		)
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")

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
		statusErr := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
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
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-65031-Operand and operator log levels can be set [Serial]", func() {
		const (
			operandNamespace  = "cert-manager"
			operandLabel      = "app.kubernetes.io/instance=cert-manager"
			operatorNamespace = "cert-manager-operator"
			operatorLabel     = "name=cert-manager-operator"
		)

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
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", "openshift-cert-manager-operator", "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset operator log level")
			oldPodList, err = exutil.GetAllPodsWithLabel(oc, operatorNamespace, operatorLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			patchPath = `{"spec":{"config":{"env":[{"name":"OPERATOR_LOG_LEVEL","value":"2"}]}}}`
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", "openshift-cert-manager-operator", "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
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
	g.It("Author:yuewu-CPaasrunOnly-NonPreRelease-ConnectedOnly-Medium-71327-cert-manager Operator should pass DAST scan", func() {
		// ensure componentName and apiGroupName to follow the file naming conventions
		const (
			componentName = "cert-manager"
			apiGroupName  = "cert-manager.io_v1"
		)

		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		configFile := filepath.Join(buildPruningBaseDir, "rapidast-config.yaml")
		policyFile := filepath.Join(buildPruningBaseDir, "rapidast-scan-policy.xml")

		oc.SetupProject()
		rapidastScan(oc, oc.Namespace(), componentName, apiGroupName, configFile, policyFile)
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-NonPreRelease-PreChkUpgrade-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-65134-Prepare cert-manager test data before OCP upgrade", func() {
		const (
			acmeIssuerName  = "letsencrypt-http01"
			sharedNamespace = "ocp-65134-shared-ns"
		)

		exutil.By("create a shared testing namespace")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", sharedNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create a selfsigned issuer and certificate")
		createIssuer(oc, sharedNamespace)
		createCertificate(oc, sharedNamespace)

		exutil.By("create an ACME http01 issuer")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		acmeIssuerFile := filepath.Join(buildPruningBaseDir, "issuer-acme-http01.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", sharedNamespace, "-f", acmeIssuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("wait for the ACME http01 issuer to become Ready")
		err = waitForResourceReadiness(oc, sharedNamespace, "issuer", acmeIssuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, sharedNamespace, "issuer", acmeIssuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-NonPreRelease-PstChkUpgrade-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-65134-cert-manager functions should work normally after OCP upgrade", func() {
		const (
			selfsignedIssuerName = "default-selfsigned"
			selfsignedCertName   = "default-selfsigned-cert"
			acmeIssuerName       = "letsencrypt-http01"
			acmeCertName         = "letsencrypt-http01-cert"
			operatorNamespace    = "cert-manager-operator"
			operandNamespace     = "cert-manager"
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

		exutil.By("check if the http01 solver is applicable in current env")
		skipIfRouteUnreachable(oc)

		exutil.By("create a new certificate using the ACME http01 issuer")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dnsName := constructDNSName(ingressDomain)

		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		acmeCertFile := filepath.Join(buildPruningBaseDir, "cert-test-http01.yaml")
		params := []string{"-f", acmeCertFile, "-p", "ISSUER_NAME=" + acmeIssuerName, "CERT_NAME=" + acmeCertName, "DNS_NAME=" + dnsName}
		exutil.ApplyNsResourceFromTemplate(oc, sharedNamespace, params...)

		exutil.By("wait for the ACME http01 certificate to become Ready")
		err = waitForResourceReadiness(oc, sharedNamespace, "certificate", acmeCertName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, sharedNamespace, "certificate", acmeCertName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-65028-Vault issuer should work well when authenticating with Vault AppRole", func() {
		const (
			vaultReleaseName = "vault-65028"
			vaultRoleName    = "cert-manager"
			vaultSecretName  = "cert-manager-vault-approle"
			issuerName       = "issuer-vault-approle"
			certName         = "cert-from-issuer-vault-approle"
		)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, _ := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)

		exutil.By("configure auth with Vault AppRole")
		cmd := fmt.Sprintf(`vault auth enable approle && vault write auth/approle/role/%s token_policies="cert-manager" token_ttl=1h token_max_ttl=4h`, vaultRoleName)
		_, err := exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd = fmt.Sprintf(`vault read -format=json auth/approle/role/%s/role-id`, vaultRoleName)
		output, err := exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		vaultRoleId := gjson.Get(output, "data.role_id").String()
		cmd = fmt.Sprintf(`vault write -format=json -force auth/approle/role/%s/secret-id`, vaultRoleName)
		output, err = exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		vaultSecretId := gjson.Get(output, "data.secret_id").String()

		exutil.By("create the auth secret")
		oc.NotShowInfo()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", oc.Namespace(), "secret", "generic", vaultSecretName, "--from-literal=secretId="+vaultSecretId).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer using Vault AppRole")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-vault-approle.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "VAULT_SERVICE=" + vaultReleaseName, "VAULT_NAMESPACE=" + oc.Namespace(), "ROLE_ID=" + vaultRoleId, "SECRET_NAME=" + vaultSecretName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-from-vault.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-65029-Vault issuer should work well when authenticating with Vault token", func() {
		const (
			vaultReleaseName = "vault-65029"
			vaultSecretName  = "cert-manager-vault-token"
			issuerName       = "issuer-vault-token"
			certName         = "cert-from-issuer-vault-token"
		)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, vaultToken := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)

		exutil.By("configure auth with Vault token")
		cmd := fmt.Sprintf(`vault token create -policy=cert-manager -ttl=720h`)
		_, err := exutil.RemoteShPod(oc, oc.Namespace(), vaultPodName, "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create the auth secret")
		oc.NotShowInfo()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", oc.Namespace(), "secret", "generic", vaultSecretName, "--from-literal=token="+vaultToken).Execute()
		oc.SetShowInfo()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create an issuer using Vault token")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		issuerFile := filepath.Join(buildPruningBaseDir, "issuer-vault-token.yaml")
		params := []string{"-f", issuerFile, "-p", "ISSUER_NAME=" + issuerName, "VAULT_SERVICE=" + vaultReleaseName, "VAULT_NAMESPACE=" + oc.Namespace(), "SECRET_NAME=" + vaultSecretName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "issuer", issuerName, 10*time.Second, 120*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "issuer", issuerName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for issuer to become Ready")

		exutil.By("create a certificate")
		certFile := filepath.Join(buildPruningBaseDir, "cert-from-vault.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-Low-65030-Vault issuer should work well when authenticating with Kubernetes static service account", func() {
		const (
			vaultReleaseName   = "vault-65030"
			serviceAccountName = "cert-manager-vault-static-serviceaccount"
			issuerName         = "issuer-vault-static-serviceaccount"
			certName           = "cert-from-issuer-vault-static-serviceaccount"
		)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, _ := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)

		exutil.By("create a long-lived API token for a service account")
		err := oc.Run("create").Args("serviceaccount", serviceAccountName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
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
		certFile := filepath.Join(buildPruningBaseDir, "cert-from-vault.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-66907-Vault issuer should work well when authenticating with Kubernetes bound service account through Kubernetes auth", func() {
		const (
			minSupportedVersion = "1.12.0"
			vaultReleaseName    = "vault-66907"
			serviceAccountName  = "cert-manager-vault-bound-serviceaccount"
			issuerName          = "issuer-vault-bound-serviceaccount"
			certName            = "cert-from-issuer-vault-bound-serviceaccount"
		)

		skipUnsupportedVersion(oc, minSupportedVersion)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, _ := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)

		exutil.By("create RBAC resources for the service account to get tokens")
		err := oc.Run("create").Args("serviceaccount", serviceAccountName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
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
		certFile := filepath.Join(buildPruningBaseDir, "cert-from-vault.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})

	// author: yuewu@redhat.com
	g.It("Author:yuewu-ROSA-ARO-OSD_CCS-ConnectedOnly-Medium-76515-Vault issuer should work well when authenticating with Kubernetes bound service account through JWT/OIDC auth", func() {
		const (
			minSupportedVersion = "1.12.0"
			vaultReleaseName    = "vault-76515"
			serviceAccountName  = "cert-manager-vault-bound-serviceaccount"
			issuerName          = "issuer-vault-bound-serviceaccount"
			certName            = "cert-from-issuer-vault-bound-serviceaccount"
		)

		skipUnsupportedVersion(oc, minSupportedVersion)

		exutil.By("setup an in-cluster Vault server with PKI secrets enigne enabled")
		vaultPodName, _ := setupVaultServer(oc, oc.Namespace(), vaultReleaseName)

		exutil.By("create RBAC resources for the service account to get tokens")
		err := oc.Run("create").Args("serviceaccount", serviceAccountName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
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
		certFile := filepath.Join(buildPruningBaseDir, "cert-from-vault.yaml")
		params = []string{"-f", certFile, "-p", "CERT_NAME=" + certName, "ISSUER_NAME=" + issuerName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		err = waitForResourceReadiness(oc, oc.Namespace(), "certificate", certName, 10*time.Second, 300*time.Second)
		if err != nil {
			dumpResource(oc, oc.Namespace(), "certificate", certName, "-o=yaml")
		}
		exutil.AssertWaitPollNoErr(err, "timeout waiting for certificate to become Ready")
	})
})
