package apiserverauth

import (
	"fmt"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var _ = g.Describe("[sig-auth] Authentication", func() {
	defer g.GinkgoRecover()

	var (
		oc                     = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
		authenticationCoStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		promPod                = "prometheus-k8s-0"
		monitoringns           = "openshift-monitoring"
		queryCredentialMode    = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=cco_credentials_mode"
	)
	g.BeforeEach(func() {
		e2e.Logf("Check for Authentication operator status before test.")
		checkCoStatus(oc, "authentication", authenticationCoStatus)
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if strings.Contains(output, "NotFound") {
			g.Skip("Skip since catalogsource/qe-app-registry is not installed")
		}
		e2e.Logf("Prepare cert manager operator.\n")
		createCertManagerOperator(oc)
	})

	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-Author:geliu-High-62494-Use explicit credential in ACME dns01 solver with route53 to generate certificate [Serial]", func() {
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Skip test when the cluster is with STS credential")
		token, err := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		result, err := getBearerTokenURLViaPod(monitoringns, promPod, queryCredentialMode, token)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(result, "manualpodidentity") {
			g.Skip("Skip for the aws cluster with STS credential")
		}
		defer func() {
			e2e.Logf("Remove the secret generic test-secret.")
			_, errSecret := oc.AsAdmin().Run("delete").Args("-n", "cert-manager", "secret", "test-secret").Output()
			o.Expect(errSecret).NotTo(o.HaveOccurred())
			checkCoStatus(oc, "authentication", authenticationCoStatus)
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
			e2e.Logf("Check for Authentication operator status before test.")
			checkCoStatus(oc, "authentication", authenticationCoStatus)
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
		if !strings.Contains(string(ssloutput), "DNS:auth-custom1.qe1.devcluster.openshift.com") || err != nil {
			e2e.Failf("The certificate indeed issued by Let's Encrypt with SAN failed.")
		}
	})
})
