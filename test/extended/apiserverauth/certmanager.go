package apiserverauth

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-auth] CFE", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipMissingQECatalogsource(oc)
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("ROSA-ConnectedOnly-Author:geliu-High-62494-Use explicit credential in ACME dns01 solver with route53 to generate certificate", func() {
		g.By("Check proxy env.")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o", "jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "httpsProxy") {
			g.Skip("The cluster has httpsProxy, ocp-62494 skipped.")
		}

		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		g.By("Check if cluster region is us-gov or not")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(region, "us-gov") {
			g.Skip("Skipping for the aws cluster in us-gov region.")
		}

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
		g.By("Create clusterissuer with route53 as dns01 solver.")
		defer func() {
			e2e.Logf("Delete clusterissuers.cert-manager.io letsencrypt-dns01")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "letsencrypt-dns01").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerFile := filepath.Join(buildPruningBaseDir, "cluster-issuer-acme-dns01-route53.yaml")
		sedCmd := fmt.Sprintf(`sed -i 's/AWS_ACCESS_KEY_ID/%s/g' %s`, accessKeyID, clusterIssuerFile)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterIssuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
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
		certClusterissuerFile := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		// Hard code dns zone to be "qe1.devcluster.openshift.com" temporarily.
		// TODO: when having time in future, change to be: get apps.clustername.yyy...com, trim "apps.clustername.", let dnsZone = yyy...com, implement AWS API for `aws route53 list-hosted-zones | jq -r '.HostedZones[] | select(.Name=="yyyy...com.") | .Id'` to get the hosted zone ID, replace clusterissuer YAML file's hostedZoneID. So even in AWS env not using QE's AWS account, this case can still pass.
		dnsZone := "qe1.devcluster.openshift.com"
		f, err := ioutil.ReadFile(certClusterissuerFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		randomStr := exutil.GetRandomString()
		dnsName := randomStr + "." + dnsZone
		if len(dnsName) > 63 {
			g.Skip("Skip testcase for length of dnsName is beyond 63, and result in err:Failed to create Order, NewOrder request did not include a SAN short enough to fit in CN!!!!")
		}
		e2e.Logf("dnsName=%s", dnsName)
		f1 := strings.ReplaceAll(string(f), "DNS_NAME", dnsName)
		err = ioutil.WriteFile(certClusterissuerFile, []byte(f1), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", certClusterissuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
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
	g.It("ROSA-ARO-ConnectedOnly-Author:geliu-High-62063-Use specified ingressclass in ACME http01 solver to generate certificate [Serial]", func() {
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

		g.By("Check if cluster region is us-gov or not")
		region, err := exutil.GetAWSClusterRegion(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(region, "us-gov") {
			g.Skip("Skipping for the aws cluster in us-gov region.")
		}

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

		g.By("Prepare a clusterissuer which uses AWS hosted zone qe.devcluster.openshift.com as target hosted zone.")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerFile := filepath.Join(buildPruningBaseDir, "clusterissuer-overlapped-zone.yaml")
		f, err := ioutil.ReadFile(clusterIssuerFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		f1 := strings.ReplaceAll(string(f), "AWS_ACCESS_KEY_ID", accessKeyID)
		err = ioutil.WriteFile(clusterIssuerFile, []byte(f1), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Delete clusterissuers.")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "hosted-zone-overlapped").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterIssuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
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
		certClusterissuerFile := filepath.Join(buildPruningBaseDir, "cert-hosted-zone-overlapped.yaml")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ingressDomain=%s", string(output))
		f, err = ioutil.ReadFile(certClusterissuerFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		randomStr := exutil.GetRandomString()
		dnsName := randomStr + "." + output
		f1 = strings.ReplaceAll(string(f), "DNS_NAME", dnsName)
		err = ioutil.WriteFile(certClusterissuerFile, []byte(f1), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", certClusterissuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
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

		g.By("Login with normal user and create issuers.\n")
		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth/certmanager")
		clusterIssuerFile := filepath.Join(buildPruningBaseDir, "cluster-issuer-acme-dns01-route53.yaml")
		f, err := ioutil.ReadFile(clusterIssuerFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		f1 := strings.ReplaceAll(string(f), "AWS_ACCESS_KEY_ID", accessKeyID)
		err = ioutil.WriteFile(clusterIssuerFile, []byte(f1), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Delete clusterissuers.cert-manager.io letsencrypt-dns01")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterissuers.cert-manager.io", "letsencrypt-dns01").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterIssuerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
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
		// Hard code dns zone to be "qe1.devcluster.openshift.com" temporarily.
		// TODO: when having time in future, change to be: get apps.clustername.yyy...com, trim "apps.clustername.", let dnsZone = yyy...com, implement AWS API for `aws route53 list-hosted-zones | jq -r '.HostedZones[] | select(.Name=="yyyy...com.") | .Id'` to get the hosted zone ID, replace clusterissuer YAML file's hostedZoneID. So even in AWS env not using QE's AWS account, this case can still pass.
		dnsZone := "qe1.devcluster.openshift.com"
		buildPruningBaseDir = exutil.FixturePath("testdata", "apiserverauth/certmanager")
		certDNS01File := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		f, err = ioutil.ReadFile(certDNS01File)
		o.Expect(err).NotTo(o.HaveOccurred())
		randomStr := exutil.GetRandomString()
		dnsName := randomStr + "." + dnsZone
		if len(dnsName) > 63 {
			g.Skip("Skip testcase for length of dnsName is beyond 63, and result in err:Failed to create Order, NewOrder request did not include a SAN short enough to fit in CN!!!!")
		}
		e2e.Logf("dnsName=%s", dnsName)
		f1 = strings.ReplaceAll(string(f), "DNS_NAME", dnsName)
		err = ioutil.WriteFile(certDNS01File, []byte(f1), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", certDNS01File).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

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
