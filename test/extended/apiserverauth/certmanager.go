package apiserverauth

import (
	"fmt"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var _ = g.Describe("[sig-auth] CFE", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		// TODO: need update code once https://issues.redhat.com/browse/MULTIARCH-3670 is done.
		architecture.SkipNonAmd64SingleArch(oc)
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if strings.Contains(output, "NotFound") {
			g.Skip("Skip since catalogsource/qe-app-registry is not installed")
		}
		e2e.Logf("Prepare cert manager operator.\n")
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("ROSA-ConnectedOnly-Author:geliu-High-62494-Use explicit credential in ACME dns01 solver with route53 to generate certificate [Serial]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Check if the cluster is STS or not")
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system").Execute()
		if err != nil && strings.Contains(err.Error(), "not found") {
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
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")
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
		f, err := ioutil.ReadFile(certClusterissuerFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		randomStr := exutil.GetRandomString()
		f1 := strings.ReplaceAll(string(f), "auth-custom1", randomStr)
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

		g.By("Check the certificate content.")
		defer func() {
			e2e.Logf("Remove the secret_certificate directory")
			_, errCert := exec.Command("bash", "-c", "rm -rf oc_extract_secret_certificate-from-dns01").Output()
			o.Expect(errCert).NotTo(o.HaveOccurred())
		}()
		dirname := "oc_extract_secret_certificate-from-dns01"
		_, err = exec.Command("bash", "-c", "mkdir "+dirname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The dir: %s created successfully...!!\n", dirname)
		_, err = oc.Run("extract").Args("secret/certificate-from-dns01", "--to="+dirname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		opensslCmd := "openssl x509 -noout -text -in " + dirname + "/tls.crt"
		ssloutput, sslerr := exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(sslerr).NotTo(o.HaveOccurred())
		if !strings.Contains(string(ssloutput), "DNS:"+randomStr+".qe1.devcluster.openshift.com") || err != nil {
			e2e.Failf("The certificate indeed issued by Let's Encrypt with SAN failed.")
		}
	})

	// author: geliu@redhat.com
	g.It("ROSA-ARO-ConnectedOnly-Author:geliu-High-62063-Low-63486-Use specified ingressclass in ACME http01 solver to generate certificate [Serial]", func() {
		e2e.Logf("Login with normal user and create new ns.")
		oc.SetupProject()
		e2e.Logf("Create issuer in ns scope created in last step.")
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")
		issuerHttp01File := filepath.Join(buildPruningBaseDir, "issuer-acme-http01.yaml")
		err := oc.Run("create").Args("-f", issuerHttp01File).Execute()
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
		dns_name := "t." + ingressDomain
		if len(dns_name) > 63 {
			g.Skip("Skip testcase for length of dns_name is beyond 63, and result in err:Failed to create Order, NewOrder request did not include a SAN short enough to fit in CN!!!!")
		}
		certHttp01File := filepath.Join(buildPruningBaseDir, "cert-test-http01.yaml")
		sedCmd := fmt.Sprintf(`sed -i 's/DNS_NAME/%s/g' %s`, dns_name, certHttp01File)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", certHttp01File).Execute()
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
		g.By("Check certificate secret.")
		output, err := oc.Run("get").Args("secret", "cert-test-http01").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get secret/cert-test-http01 output: %v", output)
		g.By("Verify the certificate content.")
		defer func() {
			e2e.Logf("Remove certificate dir/files.")
			_, errCert := exec.Command("bash", "-c", "rm -rf oc_extract_secret_certificate-62063").Output()
			o.Expect(errCert).NotTo(o.HaveOccurred())
		}()
		dirname := "oc_extract_secret_certificate-62063"
		_, err = exec.Command("bash", "-c", "mkdir "+dirname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The dir: %s created successfully...!!\n", dirname)
		_, err = oc.Run("extract").Args("secret/cert-test-http01", "--to="+dirname).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		opensslCmd := "openssl x509 -noout -text -in " + dirname + "/tls.crt"
		ssloutput, sslerr := exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(sslerr).NotTo(o.HaveOccurred())
		if !strings.Contains(string(ssloutput), dns_name) {
			e2e.Failf("Failure: The certificate is indeed issued by Let's Encrypt, the Subject Alternative Name is indeed the specified DNS_NAME failed.")
		}
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
		e2e.Logf("Login with normal user and create issuers.\n")
		oc.SetupProject()
		createIssuers(oc)
		e2e.Logf("Create certificate.\n")
		createCertificate(oc)

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
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(append([]string{"clusterrole"}, clusterroleListArry...)...).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterrolebindingList, err := oc.AsAdmin().Run("get").Args("clusterrolebinding").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		regexstr, _ = regexp.Compile("(?m)^[^ ]*cert-?manager[^ ]*")
		clusterrolebindingListArry := regexstr.FindAllString(clusterrolebindingList, -1)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(append([]string{"clusterrolebinding"}, clusterrolebindingListArry...)...).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("ROSA-ConnectedOnly-Author:geliu-Medium-62582-Need override dns args when the target hosted zone in ACME dns01 solver overlaps with the cluster's default private hosted zone [Disruptive]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AWS")

		g.By("Skip test when the cluster is with STS credential")
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system").Execute()
		if err != nil && strings.Contains(err.Error(), "not found") {
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
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")
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
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
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
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system").Execute()
		if err != nil && strings.Contains(err.Error(), "not found") {
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
		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")
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
		ingressDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingress.config", "cluster", "-o=jsonpath={.spec.domain}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ingressDomain=%s", string(ingressDomain))
		buildPruningBaseDir = exutil.FixturePath("testdata", "apiserverauth")
		certDns01File := filepath.Join(buildPruningBaseDir, "certificate-from-clusterissuer-letsencrypt-dns01.yaml")
		f, err = ioutil.ReadFile(certDns01File)
		o.Expect(err).NotTo(o.HaveOccurred())
		randomStr := exutil.GetRandomString()
		dns_name := randomStr + "." + string(ingressDomain)
		e2e.Logf("dns_name=%s", dns_name)
		f1 = strings.ReplaceAll(string(f), "DNS_NAME", dns_name)
		err = ioutil.WriteFile(certDns01File, []byte(f1), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", certDns01File).Execute()
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
	})
})
