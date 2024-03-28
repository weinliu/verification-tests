package apiserverauth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-auth] CFE", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("default-"+getRandomString(8), exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("ROSA-ConnectedOnly-Author:geliu-LEVEL0-High-62494-Use explicit credential in ACME dns01 solver with route53 to generate certificate", func() {
		g.By("Check proxy env.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "httpsProxy") {
			g.Skip("The cluster has httpsProxy, ocp-62494 skipped.")
		}

		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		g.By("Check if the cluster is STS or not")
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

		g.By("Create clusterissuer with route53 as dns01 solver.")
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
		hostedZoneID := getRoute53HostedZoneID(accessKeyID, secureKey, region, dnsZone)
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
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterissuer", "-o", "wide").Output()
			if !strings.Contains(output, "True") || err != nil {
				e2e.Logf("clusterissuer is not ready.")
				return false, nil
			}
			e2e.Logf("clusterissuer is ready.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Waiting for get clusterissuer timeout")
		g.By("create certificate which references previous clusterissuer")
		defer func() {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("certificate").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "certificate-from-dns01") {
				e2e.Logf("Remove certificate: certificate-from-dns01.")
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("certificate", "certificate-from-dns01").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()
		e2e.Logf("Create ns with normal user.")
		oc.SetupProject()
		randomStr := getRandomString(4)
		dnsName := randomStr + "." + dnsZone
		if len(dnsName) > 63 {
			g.Skip("Skip testcase for length of dnsName is beyond 63, and result in err:Failed to create Order, NewOrder request did not include a SAN short enough to fit in CN!!!!")
		}
		e2e.Logf("dnsName=%s", dnsName)
		certTemplate := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		statusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("certificate").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			output1, err := oc.Run("get").Args("challenge").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("certificate status is: %v ", output)
			if strings.Contains(output, "True") && !strings.Contains(output1, "certificate-from-dns01") {
				e2e.Logf("certificate status is normal: %v ", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, fmt.Sprintf("certificate is wrong: %v", statusErr))

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "certificate-from-dns01", oc.Namespace())
	})

	// author: geliu@redhat.com
	// This case contains three Polarion cases: 62063, 63325, and 63486. The root case is 62063.
	g.It("ROSA-ARO-ConnectedOnly-Author:geliu-LEVEL0-High-62063-Use specified ingressclass in ACME http01 solver to generate certificate [Serial]", func() {
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
		statusErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {

			output, err := oc.Run("get").Args("issuer", "letsencrypt-http01").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "True") {
				e2e.Logf("Get issuer output is: %v", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, fmt.Sprintf("get issuer is wrong: %v", statusErr))
		e2e.Logf("As the normal user, create certificate.")
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}", "--context=admin").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ingressDomain=%s", ingressDomain)
		dnsName := "t." + ingressDomain
		if len(dnsName) > 63 {
			g.Skip("Skip testcase for length of dnsName is beyond 63, and result in err:Failed to create Order, NewOrder request did not include a SAN short enough to fit in CN!!!!")
		}
		certHTTP01File := filepath.Join(buildPruningBaseDir, "cert-test-http01.yaml")
		sedCmd := fmt.Sprintf(`sed -i 's/DNS_NAME/%s/g' %s`, dnsName, certHTTP01File)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", certHTTP01File).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		statusErr = wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("certificate", "cert-test-http01").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("certificate status is: %v ", output)
			if strings.Contains(output, "True") {
				e2e.Logf("certificate status is normal.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "certificate is wrong.")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "cert-test-http01", oc.Namespace())

		// Low-63486-When a Certificate CR is deleted its certificate secret should not be deleted
		e2e.Logf("Delete certification for ocp-63486.\n")
		err = oc.Run("delete").Args("certificate", "cert-test-http01").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ocp-63486: Waiting 1 min to ensure secret have not be removed.\n")
		time.Sleep(60 * time.Second)
		err = oc.Run("get").Args("secret", "cert-test-http01").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: geliu@redhat.com
	g.It("ROSA-ARO-ConnectedOnly-Author:geliu-Medium-62006-RH cert-manager operator can be uninstalled from CLI and then reinstalled [Serial]", func() {
		e2e.Logf("Login with normal user and create issuer.\n")
		oc.SetupProject()
		createIssuer(oc)
		e2e.Logf("Create certificate.\n")
		createCertificate(oc)
		e2e.Logf("Check issued certificate.\n")
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
		if !strings.Contains(output, "cert-manager") || err != nil {
			e2e.Failf("crd don't contain cert-manager\n.")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("apiservice").Output()
		if !strings.Contains(output, "cert-manager") || err != nil {
			e2e.Failf("apiservice don't contain cert-manager\n.")
		}
		e2e.Logf("Clean up cert-manager-operator NS.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "cert-manager-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Delete operand.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "cert-manager").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Delete cert-manager CRDs.\n")
		e2e.Logf("Patching certmanager/cluster with null finalizers is required, otherwise the delete commands can be stuck.\n")
		patchPath := "{\"metadata\":{\"finalizers\":null}}"
		err = oc.AsAdmin().Run("patch").Args("certmanagers.operator", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Delete certmanagers.operator cluster.\n")
		err = oc.AsAdmin().Run("delete").Args("certmanagers.operator", "cluster").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Delete crd.\n")
		crdList, err := oc.AsAdmin().Run("get").Args("crd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		regexstr, _ := regexp.Compile(".*" + "cert-?manager" + "[0-9A-Za-z-.]*")
		crdListArry := regexstr.FindAllString(crdList, -1)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(append([]string{"crd"}, crdListArry...)...).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("issuer").Output()
		if !strings.Contains(output, "could not find the requested resource") && !strings.Contains(output, `the server doesn't have a resource type "issuer"`) {
			e2e.Failf("issuer is still exist out of expected.\n")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole").Output()
		if !strings.Contains(output, "cert-manager") || err != nil {
			e2e.Failf("clusterrole is not exist.\n")
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrolebinding").Output()
		if !strings.Contains(output, "cert-manager") || err != nil {
			e2e.Failf("clusterrolebinding is not exist.\n")
		}
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
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("ROSA-ConnectedOnly-Author:geliu-Medium-62582-Need override dns args when the target hosted zone in ACME dns01 solver overlaps with the cluster's default private hosted zone [Disruptive]", func() {
		g.By("Check proxy env.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "httpsProxy") {
			g.Skip("The cluster has httpsProxy, ocp-62582 skipped.")
		}

		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		g.By("Skip test when the cluster is with STS credential")
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

		g.By("Create clusterissuer with route53 as dns01 solver.")
		baseDomain := getBaseDomain(oc)
		e2e.Logf("baseDomain=%s", baseDomain)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("dnsZone=%s", dnsZone)
		hostedZoneID := getRoute53HostedZoneID(accessKeyID, secureKey, region, dnsZone)
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
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterissuer", "-o", "wide").Output()
			if !strings.Contains(output, "True") || err != nil {
				e2e.Logf("clusterissuer is not ready.")
				return false, nil
			}
			e2e.Logf("clusterissuer is ready.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Waiting for get clusterissuer timeout")

		g.By("create certificate which references previous clusterissuer")
		e2e.Logf("Create ns with normal user.")
		oc.SetupProject()
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ingressDomain=%s", ingressDomain)
		randomStr := getRandomString(4)
		dnsName := randomStr + "." + ingressDomain
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

		g.By("Apply dns args by patch.")
		certManagerPod0, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "cert-manager", "-l", "app=cert-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		if len(string(certManagerPod0)) == 0 || err != nil {
			e2e.Failf("Fail to get name of cert_manager_pod0.")
		}
		patchPath := "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":[\"--dns01-recursive-nameservers=1.1.1.1:53\",\"--dns01-recursive-nameservers-only\"]}}}"
		var certManagerPod1 string
		defer func() {
			e2e.Logf("patch clusterissuers.cert-manager.io back.")
			patchPath1 := "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":null}}}"
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			statusErr = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				certManagerPod2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "cert-manager", "-l", "app=cert-manager", "-o=jsonpath={.items[*].metadata.name}", "--field-selector=status.phase==Running").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if !strings.Contains(certManagerPod2, certManagerPod1) {
					e2e.Logf("cert-manager pods have been redeployed successfully.")
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(statusErr, "cert-manager pods have NOT been redeployed when recovered.")
		}()
		err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		statusErr = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			certManagerPod1, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "cert-manager", "-l", "app=cert-manager", "-o=jsonpath={.items[*].metadata.name}", "--field-selector=status.phase==Running").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(certManagerPod1, certManagerPod0) {
				e2e.Logf("cert-manager pods have been redeployed successfully.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "cert-manager pods have NOT been redeployed.")

		g.By("Check the certificate content AGAIN.")
		statusErr = wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			output, err = oc.Run("get").Args("certificate", "certificate-hosted-zone-overlapped").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("certificate status is: %v ", output)
			if strings.Contains(output, "True") {
				e2e.Logf("certificate status is normal.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "certificate status is wrong.")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "certificate-hosted-zone-overlapped", oc.Namespace())
	})

	// author: geliu@redhat.com
	// This case contains two Polarion cases: 63555 and 69798. The root case is 63555.
	g.It("ROSA-Author:geliu-Medium-63555-ACME dns01 solver should work in OpenShift proxy env [Serial]", func() {
		g.By("Check proxy env.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "httpsProxy") {
			g.Skip("Fail to check httpsProxy, ocp-63555 skipped.")
		}

		g.By("Skip test when the cluster is with STS credential")
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

		g.By("Login with normal user and create issuers.\n")
		oc.SetupProject()
		baseDomain := getBaseDomain(oc)
		e2e.Logf("baseDomain=%s", baseDomain)
		dnsZone, err := getParentDomain(baseDomain)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("dnsZone=%s", dnsZone)
		hostedZoneID := getRoute53HostedZoneID(accessKeyID, secureKey, region, dnsZone)
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
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterissuer", "-o", "wide").Output()
			if !strings.Contains(output, "True") || err != nil {
				e2e.Logf("clusterissuer is not ready.")
				return false, nil
			}
			e2e.Logf("clusterissuer is ready.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Waiting for clusterissuer ready timeout.")

		g.By("Create the certificate.")
		randomStr := getRandomString(4)
		dnsName := randomStr + "." + dnsZone
		if len(dnsName) > 63 {
			g.Skip("Skip testcase for length of dnsName is beyond 63, and result in err:Failed to create Order, NewOrder request did not include a SAN short enough to fit in CN!!!!")
		}
		e2e.Logf("dnsName=%s", dnsName)
		certTemplate := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName}
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

		g.By("Check the certificate and its challenge")
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
		g.By("patch certmanager/cluster.")
		certManagerPod1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "cert-manager", "-l", "app=cert-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		patchPath := "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":[\"--dns01-recursive-nameservers-only\"]}}}"
		defer func() {
			e2e.Logf("patch certmanager/cluster back.")
			certManagerPod1, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "cert-manager", "-l", "app=cert-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
			patchPath1 := "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":null}}}"
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			statusErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				certManagerPod2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "cert-manager", "-l", "app=cert-manager", "-o=jsonpath={.items[*].metadata.name}", "--field-selector=status.phase==Running").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if !strings.Contains(certManagerPod2, certManagerPod1) {
					e2e.Logf("cert-manager pods have been redeployed successfully.")
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(statusErr, "cert-manager pods have NOT been redeployed after recovery.")
		}()
		err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		statusErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			certManagerPod2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "cert-manager", "-l", "app=cert-manager", "-o=jsonpath={.items[*].metadata.name}", "--field-selector=status.phase==Running").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(certManagerPod2, certManagerPod1) {
				e2e.Logf("cert-manager pods have been redeployed successfully.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "cert-manager pods have NOT been redeployed after patch.")
		g.By("Checke challenge and certificate again.")
		statusErr = wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			output, err = oc.Run("get").Args("certificate", "certificate-from-dns01").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("certificate status is: %v ", output)
			if strings.Contains(output, "True") {
				e2e.Logf("certificate status is normal.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(statusErr, "certificate is wrong.")

		e2e.Logf("Check and verify issued certificate content")
		verifyCertificate(oc, "certificate-from-dns01", oc.Namespace())

		// author: yuewu@redhat.com
		// Medium-69798-ACME dns01 solver should work in OpenShift proxy env with DNS over HTTPS (DoH) for doing the self-checks
		currentVersion, _ := semver.Parse(getCertManagerOperatorVersion(oc))
		minDoHSupportedVersion, _ := semver.Parse("1.13.0")
		// semverA.Compare(semverB) > -1 means 'semverA' greater than or equal to 'semverB', see: https://pkg.go.dev/github.com/blang/semver#Version.Compare
		if currentVersion.Compare(minDoHSupportedVersion) > -1 {
			e2e.Logf("Start to execute test case OCP-69798\n")

			g.By("Configure with an invalid server as negative test.")
			patchPath = "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":[\"--dns01-recursive-nameservers-only\", \"--dns01-recursive-nameservers=https://1.1.1.1/negative-test-dummy-dns-query\"]}}}"
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertAllPodsToBeReadyWithPollerParams(oc, "cert-manager", 10*time.Second, 120*time.Second)

			g.By("Create a new certificate.")
			randomStr = getRandomString(4)
			dnsName = randomStr + "." + dnsZone
			if len(dnsName) > 63 {
				g.Skip("Skip testcase for length of dnsName is beyond 63, and result in err:Failed to create Order, NewOrder request did not include a SAN short enough to fit in CN!!!!")
			}
			e2e.Logf("dnsName=%s", dnsName)
			certTemplate = filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
			params = []string{"-f", certTemplate, "-p", "DNS_NAME=" + dnsName}
			exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)

			g.By("Check if challenge will be pending and show HTTP 403 error")
			statusErr = wait.Poll(10*time.Second, 90*time.Second, func() (bool, error) {
				output, err = oc.Run("get").Args("challenge", "-o", "wide").Output()
				if !strings.Contains(output, "403 Forbidden") || !strings.Contains(output, "pending") || err != nil {
					e2e.Logf("challenge is still in processing, and status is not as expected: %s\n", output)
					return false, nil
				}
				e2e.Logf("challenge's output is as expected: %s\n", output)
				return true, nil
			})
			exutil.AssertWaitPollNoErr(statusErr, "timed out after 90s waiting challenge to be pending state and show HTTP 403 error")

			g.By("Configure with a valid server.")
			patchPath = "{\"spec\":{\"controllerConfig\":{\"overrideArgs\":[\"--dns01-recursive-nameservers-only\", \"--dns01-recursive-nameservers=https://1.1.1.1/dns-query\"]}}}"
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertAllPodsToBeReadyWithPollerParams(oc, "cert-manager", 10*time.Second, 120*time.Second)

			g.By("Check if certificate will be True.")
			statusErr = wait.Poll(10*time.Second, 150*time.Second, func() (bool, error) {
				output, err = oc.Run("get").Args("certificate", "certificate-from-dns01").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if strings.Contains(output, "True") {
					e2e.Logf("certificate status is normal.")
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(statusErr, "timed out after 150s waiting certificate to be True")

			g.By("Check and verify issued certificate content")
			verifyCertificate(oc, "certificate-from-dns01", oc.Namespace())
		} else {
			e2e.Logf("currentVersion(%s) < minDoHSupportedVersion(%s), therefore skipping the DoH checkpoint test (case 69798)", currentVersion, minDoHSupportedVersion)
		}
	})

	// author: geliu@redhat.com
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-Author:geliu-Low-63500-Multiple solvers mixed with http01 and dns01 in ACME issuer should work well", func() {
		g.By("Create a clusterissuer which has multiple solvers mixed with http01 and dns01.")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-acme-multiple-solvers.yaml")
		defer func() {
			e2e.Logf("Delete clusterissuers.")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "acme-multiple-solvers").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterIssuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterissuer", "acme-multiple-solvers", "-o", "wide").Output()
			if !strings.Contains(output, "True") || err != nil {
				e2e.Logf("clusterissuer is not ready.")
				return false, nil
			}
			e2e.Logf("clusterissuer is ready.")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Waiting for get clusterissuer timeout.")

		e2e.Logf("Create ns with normal user.")
		oc.SetupProject()

		g.By("As normal user, create below 3 certificates in later steps with above clusterissuer.")
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
	g.It("Author:yuewu-Low-63583-Check operand metrics by using user-workload-monitoring [Serial]", func() {
		const (
			operandNamespace                = "cert-manager"
			clusterMonitoringNamespace      = "openshift-monitoring"
			clusterMonitoringConfigMapName  = "cluster-monitoring-config"
			userWorkloadMonitoringNamespace = "openshift-user-workload-monitoring"
			metricsQueryURL                 = "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query"
		)
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")

		g.By("Check if the cluster-monitoring ConfigMap exists")
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

		g.By("Enable monitoring for user-defined projects")
		configFile := filepath.Join(buildPruningBaseDir, "cluster-monitoring-config.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Delete created ConfigMap")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", clusterMonitoringConfigMapName, "-n", clusterMonitoringNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, userWorkloadMonitoringNamespace, 10*time.Second, 120*time.Second)

		g.By("Create Service Monitor to collect metrics")
		serviceMonitorFile := filepath.Join(buildPruningBaseDir, "servicemonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-n", operandNamespace, "-f", serviceMonitorFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Delete created ServiceMonitor")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("servicemonitor", "cert-manager", "-n", operandNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Prepare Prometheus SA token for making queries")
		token, err := getSAToken(oc, "prometheus-k8s", clusterMonitoringNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		g.By("Query metrics from HTTP API")
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
	g.It("ROSA-ARO-OSD_CCS-Author:yuewu-Medium-65031-Operand and operator log levels can be set [Serial]", func() {
		const (
			operandNamespace  = "cert-manager"
			operatorNamespace = "cert-manager-operator"
		)

		g.By("Set operands log level to an invalid value")
		patchPath := `{"spec":{"logLevel":"xxx"}}`
		output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("certmanager.operator", "cluster", "--type=merge", "-p", patchPath).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Unsupported value: "xxx"`))

		// The valid values can be "Normal", "Debug", "Trace", and "TraceAll", default is "Normal".
		g.By("Set operands log level to a valid value")
		patchPath = `{"spec":{"logLevel":"Trace"}}`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("certmanager.operator", "cluster", "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset operands log level")
			patchPath := `{"spec":{"logLevel":""}}`
			err = oc.AsAdmin().Run("patch").Args("certmanager", "cluster", "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertAllPodsToBeReadyWithPollerParams(oc, operandNamespace, 10*time.Second, 120*time.Second)
		}()
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, operandNamespace, 10*time.Second, 120*time.Second)

		g.By("Validate the operands log level")
		podList, err := exutil.GetAllPodsWithLabel(oc, operandNamespace, "app.kubernetes.io/instance=cert-manager")
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
		g.By("Set operator log level to a valid value")
		patchPath = `{"spec":{"config":{"env":[{"name":"OPERATOR_LOG_LEVEL","value":"6"}]}}}`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", "openshift-cert-manager-operator", "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("[defer] Unset operator log level")
			patchPath = `{"spec":{"config":{"env":[{"name":"OPERATOR_LOG_LEVEL","value":"2"}]}}}`
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("subscription", "openshift-cert-manager-operator", "-n", operatorNamespace, "--type=merge", "-p", patchPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.AssertAllPodsToBeReadyWithPollerParams(oc, operatorNamespace, 10*time.Second, 120*time.Second)
		}()
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, operatorNamespace, 10*time.Second, 120*time.Second)

		g.By("Validate the operator log level")
		podList, err = exutil.GetAllPodsWithLabel(oc, operatorNamespace, "name=cert-manager-operator")
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
	g.It("CPaasrunOnly-NonPreRelease-Author:yuewu-Medium-71327-cert-manager Operator should pass DAST scan", func() {
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
})
