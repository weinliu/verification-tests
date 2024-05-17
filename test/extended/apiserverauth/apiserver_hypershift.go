package apiserverauth

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-api-machinery] API_Server on hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc               = exutil.NewCLI("hp-apiserver-test", exutil.KubeConfigPath())
		guestClusterName string
		guestClusterNS   string
		guestClusterKube string
		hostedClusterNS  string
		iaasPlatform     string
		tmpdir           string
	)

	g.BeforeEach(func() {
		guestClusterName, guestClusterKube, hostedClusterNS = exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		e2e.Logf("#### %s, %s, %s", guestClusterName, guestClusterKube, hostedClusterNS)
		oc.SetGuestKubeconf(guestClusterKube)

		iaasPlatform = exutil.CheckPlatform(oc)
		// Currently, the test is only supported on AWS
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}
		guestClusterNS = hostedClusterNS + "-" + guestClusterName
		e2e.Logf("HostedClusterControlPlaneNS: %v", guestClusterNS)
		tmpdir = "/tmp/-OCP-apisever-cases-" + exutil.GetRandomString() + "/"
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.JustAfterEach(func() {
		if err := os.RemoveAll(tmpdir); err == nil {
			e2e.Logf("test dir %s is cleaned up", tmpdir)
		}
	})

	// author: kewang@redhat.com
	// The case always runs into failure due to regression bug https://issues.redhat.com/browse/OCPBUGS-30986, add Flaky tag to skip execution until the bug get fixed.
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Longduration-NonPreRelease-Author:kewang-Medium-62093-[Apiserver] Wire tlsSecurityProfile cipher config from apiservers/cluster into apiservers of hosted cluster [Flaky][Slow][Disruptive]", func() {

		var (
			defaultCipherPatch = `{"spec": {"configuration": {"apiServer": null}}}`
			defaultCipherSuite = `["TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"] VersionTLS12`
			cipherItems        = []struct {
				cipherType  string
				cipherSuite string
				patch       string
			}{
				{
					cipherType:  "custom",
					cipherSuite: `["TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"] VersionTLS11`,
					patch:       `{"spec": {"configuration": {"apiServer": {"tlsSecurityProfile":{"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256"],"minTLSVersion":"VersionTLS11"},"type":"Custom"}}}}}`,
				},
				{
					cipherType:  "Old",
					cipherSuite: `["TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256","TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA","TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA","TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA","TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA","TLS_RSA_WITH_AES_128_GCM_SHA256","TLS_RSA_WITH_AES_256_GCM_SHA384","TLS_RSA_WITH_AES_128_CBC_SHA256","TLS_RSA_WITH_AES_128_CBC_SHA","TLS_RSA_WITH_AES_256_CBC_SHA","TLS_RSA_WITH_3DES_EDE_CBC_SHA"] VersionTLS10`,
					patch:       `{"spec": {"configuration": {"apiServer": {"tlsSecurityProfile":{"old":{},"type":"Old"}}}}}`,
				},
				{
					cipherType:  "Intermediate",
					cipherSuite: defaultCipherSuite,
					patch:       `{"spec": {"configuration": {"apiServer": {"tlsSecurityProfile":{"intermediate":{},"type":"Intermediate"}}}}}`,
				},
			}
		)

		defer func() {
			exutil.By("-->> Restoring cluster's ciphers")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "--type=merge", "-p", defaultCipherPatch).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Checking if apiservers are restarted
			errKas := waitApiserverRestartOfHypershift(oc, "kube-apiserver", guestClusterNS, 480)
			o.Expect(errKas).NotTo(o.HaveOccurred())
			errOas := waitApiserverRestartOfHypershift(oc, "openshift-apiserver", guestClusterNS, 180)
			o.Expect(errOas).NotTo(o.HaveOccurred())
			errOauth := waitApiserverRestartOfHypershift(oc, "oauth-openshift", guestClusterNS, 120)
			o.Expect(errOauth).NotTo(o.HaveOccurred())
			e2e.Logf("#### Check cipherSuites and minTLSVersion of oauth, openshift-apiserver and kubeapiservers config.")
			errChipher := verifyHypershiftCiphers(oc, defaultCipherSuite, guestClusterNS)
			if errChipher != nil {
				exutil.AssertWaitPollNoErr(errChipher, "Ciphers are not matched the expected Intermediate type!")
			}

		}()

		exutil.By("-->> 1.) Check the default cipherSuites and minTLSVersion of oauth, openshift-apiserver and kubeapiservers config.")
		errChipher := verifyHypershiftCiphers(oc, defaultCipherSuite, guestClusterNS)
		if errChipher != nil {
			exutil.AssertWaitPollNoErr(errChipher, fmt.Sprintf("The ciphers are not matched : %s", defaultCipherSuite))
		}
		e2e.Logf(`The ciphers type are matched default "Intermediate".`)

		// Apply supported chipher types
		for i, cipherItem := range cipherItems {
			i += 2
			oldVer, errOldrVer := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "-o", `jsonpath={.status.conditions[?(@.type=="KubeAPIServerAvailable")].observedGeneration}`).Output()
			o.Expect(errOldrVer).NotTo(o.HaveOccurred())
			intOldVer, _ := strconv.Atoi(oldVer)
			o.Expect(intOldVer).To(o.BeNumerically(">", 0))
			e2e.Logf("observedGeneration: %v", intOldVer)

			exutil.By(fmt.Sprintf("-->> %d.1) Patching the apiserver cluster with ciphers:  %s", i, cipherItem.cipherType))
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "--type=merge", "-p", cipherItem.patch).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Checking if apiservers are restarted
			errKas := waitApiserverRestartOfHypershift(oc, "kube-apiserver", guestClusterNS, 480)
			o.Expect(errKas).NotTo(o.HaveOccurred())
			errOas := waitApiserverRestartOfHypershift(oc, "openshift-apiserver", guestClusterNS, 180)
			o.Expect(errOas).NotTo(o.HaveOccurred())
			errOauth := waitApiserverRestartOfHypershift(oc, "oauth-openshift", guestClusterNS, 120)
			o.Expect(errOauth).NotTo(o.HaveOccurred())

			newVer, errNewVer := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "-o", `jsonpath={.status.conditions[?(@.type=="KubeAPIServerAvailable")].observedGeneration}`).Output()
			o.Expect(errNewVer).NotTo(o.HaveOccurred())
			e2e.Logf("observedGeneration: %v", newVer)
			o.Expect(strconv.Atoi(newVer)).To(o.BeNumerically(">", intOldVer))

			exutil.By(fmt.Sprintf("-->> %d.2) Check cipherSuites and minTLSVersion of oauth, openshift-apiserver and kubeapiservers config.", i))
			errChipher := verifyHypershiftCiphers(oc, cipherItem.cipherSuite, guestClusterNS)
			if errChipher != nil {
				exutil.AssertWaitPollNoErr(errChipher, fmt.Sprintf("Ciphers are not matched : %s", cipherItem.cipherType))
			}
			e2e.Logf("#### Ciphers are matched: %s", cipherItem.cipherType)
		}
	})

	// author: kewang@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:kewang-High-64000-Check the http accessible /readyz for kube-apiserver [Serial]", func() {
		exutil.By("1) Check if port 6081 is available")
		err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
			checkOutput, _ := exec.Command("bash", "-c", "lsof -i:6081").Output()
			// no need to check error since some system output stderr for valid result
			if len(checkOutput) == 0 {
				return true, nil
			}
			e2e.Logf("Port 6081 is occupied, trying again")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Port 6081 is available")

		exutil.By("2) Get kube-apiserver pods")
		err = oc.AsAdmin().Run("project").Args(guestClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		podList, err := exutil.GetAllPodsWithLabel(oc, guestClusterNS, "app=kube-apiserver")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podList).ShouldNot(o.BeEmpty())
		defer oc.AsAdmin().Run("project").Args("default").Execute() // switch to default project

		exutil.By("3) Perform port-forward on the first pod available")
		exutil.AssertPodToBeReady(oc, podList[0], guestClusterNS)
		_, _, _, err = oc.AsAdmin().Run("port-forward").Args("-n", guestClusterNS, podList[0], "6081:6443").Background()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer exec.Command("bash", "-c", "kill -HUP $(lsof -t -i:6081)").Output()

		err1 := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
			checkOutput, _ := exec.Command("bash", "-c", "lsof -i:6081").Output()
			// no need to check error since some system output stderr for valid result
			if len(checkOutput) != 0 {
				e2e.Logf("#### Port-forward 6081:6443 is in use")
				return true, nil
			}
			e2e.Logf("#### Waiting for port-forward applying ...")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err1, "#### Port-forward 6081:6443 doesn't work")

		// kube-apiserver of hosted clsuter doesn't use insecure port 6081
		exutil.By("4) check if port forward succeed")
		out, err := exec.Command("bash", "-c", "curl -ks https://127.0.0.1:6081/readyz --noproxy \"127.0.0.1\"").Output()
		outStr := string(out)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outStr).Should(o.ContainSubstring("ok"), fmt.Sprintf("Output from kube-apiserver pod readyz :: %s", outStr))
		e2e.Logf("Port forwarding works fine, case ran passed!")
	})

	// author: kewang@redhat.com
	// The case always runs into failure with OCP 4.15 and later due to https://issues.redhat.com/browse/OCPBUGS-28866, add Flaky tag to skip execution until the bug get fixed.
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Author:kewang-Medium-64076-Init container setup should have the proper securityContext [Flaky]", func() {
		var (
			apiserverItems = []struct {
				label     string
				apiserver string
			}{
				{
					label:     "kube-apiserver",
					apiserver: "kube-apiserver",
				},
				{
					label:     "openshift-apiserver",
					apiserver: "openshift-apiserver",
				},
				{
					label:     "oauth-openshift",
					apiserver: "oauth-server",
				},
			}
			sc = `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"runAsNonRoot":true,"runAsUser":`
		)

		for i, apiserverItem := range apiserverItems {
			exutil.By(fmt.Sprintf("%v.1 Get one pod name of %s", i+1, apiserverItem.label))
			e2e.Logf("namespace is: %s", guestClusterNS)
			podList, err := exutil.GetAllPodsWithLabel(oc, guestClusterNS, "app="+apiserverItem.label)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(podList).ShouldNot(o.BeEmpty())
			e2e.Logf("Get the %s pod name: %s", apiserverItem.label, podList[0])

			containerList, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", guestClusterNS, podList[0], "-o", `jsonpath={.spec.containers[*].name}`).Output()
			o.Expect(err1).NotTo(o.HaveOccurred())
			o.Expect(containerList).ShouldNot(o.BeEmpty())
			containers := strings.Split(containerList, " ")

			exutil.By(fmt.Sprintf("%v.2 Checking the securityContext of containers of %s pod %s:", i+1, apiserverItem.apiserver, podList[0]))
			for _, container := range containers {
				e2e.Logf("#### Checking the container %s of pod: %s", container, podList[0])
				jsonpath := fmt.Sprintf(`jsonpath={range .spec.containers[?(@.name=="%s")]}{.securityContext}`, container)
				out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", guestClusterNS, podList[0], "-o", jsonpath).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(out).To(o.ContainSubstring(sc))
				e2e.Logf("#### The securityContext of container %s matched the expected result.", container)
			}

			exutil.By(fmt.Sprintf("%v.3 Checking the securityContext of init-container %s of pod %s", i+1, apiserverItem.apiserver, podList[0]))
			jsonpath := `jsonpath={.spec.initContainers[].securityContext}`
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", guestClusterNS, podList[0], "-o", jsonpath).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(out).To(o.ContainSubstring(sc))
			e2e.Logf("#### The securityContext of init-container matched the expected result.")

			exutil.By(fmt.Sprintf("%v.4 Checking one container %s of %s pod %s is not allowed to access any devices on the host", i+1, containers[0], apiserverItem.apiserver, podList[0]))
			cmd := []string{"-n", guestClusterNS, podList[0], "-c", containers[0], "--", "sysctl", "-w", "kernel.msgmax=65536"}
			cmdOut, errCmd := oc.AsAdmin().WithoutNamespace().Run("exec").Args(cmd...).Output()
			o.Expect(errCmd).To(o.HaveOccurred())
			o.Expect(cmdOut).Should(o.ContainSubstring("Read-only file system"))
		}
	})

	// author: rgangwar@redhat.com
	g.It("HyperShiftMGMT-ARO-OSD_CCS-NonPreRelease-Longduration-Author:rgangwar-High-70020-Add new custom certificate for the cluster API [Disruptive] [Slow]", func() {
		var (
			patchToRecover           = `{"spec": {"configuration": {"apiServer": {"servingCerts": {"namedCertificates": []}}}}}`
			originHostdKubeconfigBkp = "kubeconfig.origin"
			originHostedKube         = guestClusterKube
			originCA                 = tmpdir + "certificate-authority-data-origin.crt"
			newCA                    = tmpdir + "certificate-authority-data-origin-new.crt"
			CN_BASE                  = "kas-test-cert"
			caKeypem                 = tmpdir + "/caKey.pem"
			caCertpem                = tmpdir + "/caCert.pem"
			serverKeypem             = tmpdir + "/serverKey.pem"
			serverconf               = tmpdir + "/server.conf"
			serverWithSANcsr         = tmpdir + "/serverWithSAN.csr"
			serverCertWithSAN        = tmpdir + "/serverCertWithSAN.pem"
			originHostedKubeconfPath string
		)

		restoreCluster := func(oc *exutil.CLI) {
			err := oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run("adm").Args("wait-for-stable-cluster").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			output := getResourceToBeReady(oc, asAdmin, withoutNamespace, "secret", "-n", hostedClusterNS)
			if strings.Contains(output, "custom-api-cert") {
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "custom-api-cert", "-n", hostedClusterNS, "--ignore-not-found").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("Cluster openshift-config secret reset to default values")
			}
		}

		updateKubeconfigWithConcatenatedCert := func(caCertPath, originCertPath, kubeconfigPath string, newCertPath string) error {
			caCert, err := ioutil.ReadFile(caCertPath)
			o.Expect(err).NotTo(o.HaveOccurred())

			originCert, err := ioutil.ReadFile(originCertPath)
			o.Expect(err).NotTo(o.HaveOccurred())

			concatenatedCert := append(caCert, originCert...)
			err = ioutil.WriteFile(newCertPath, concatenatedCert, 0644)
			o.Expect(err).NotTo(o.HaveOccurred())

			base64EncodedCert := base64.StdEncoding.EncodeToString(concatenatedCert)
			updateCmdKubeconfg := fmt.Sprintf(`sed -i "s/certificate-authority-data: .*/certificate-authority-data: %s/" %s`, base64EncodedCert, kubeconfigPath)
			_, err = exec.Command("bash", "-c", updateCmdKubeconfg).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Kubeconfig file updated successfully.")
			return nil
		}

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "custom-api-cert", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Taking backup of old hosted kubeconfig to restore old kubeconfig
		exutil.By("1. Get the original hosted kubeconfig backup")
		originHostedKubeconfPath = CopyToFile(originHostedKube, originHostdKubeconfigBkp)

		defer func() {
			exutil.By("Restoring hosted hypershift cluster")
			_, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "--type=merge", "-p", patchToRecover).Output()
			errKas := waitApiserverRestartOfHypershift(oc, "kube-apiserver", guestClusterNS, 480)
			o.Expect(errKas).NotTo(o.HaveOccurred())
			e2e.Logf("Restore original hosted hypershift kubeconfig")
			bkpCmdKubeConf := fmt.Sprintf("cp -f %s %s", originHostedKubeconfPath, originHostedKube)
			_, err := exec.Command("bash", "-c", bkpCmdKubeConf).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			restoreCluster(oc)
			e2e.Logf("Cluster recovered")
		}()

		apiServerURL, err := oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run("config").Args("view", "-ojsonpath={.clusters[0].cluster.server}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		fqdnName, err := url.Parse(apiServerURL)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Get the original CA")
		caCmd := fmt.Sprintf(`grep certificate-authority-data %s | grep -Eo "[^ ]+$" | base64 -d > %s`, originHostedKube, originCA)
		_, err = exec.Command("bash", "-c", caCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Create certificates with SAN.")
		opensslCMD := fmt.Sprintf("openssl genrsa -out %v 2048", caKeypem)
		_, caKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caKeyErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf(`openssl req -x509 -new -nodes -key %v -days 100000 -out %v -subj "/CN=%s_ca"`, caKeypem, caCertpem, CN_BASE)
		_, caCertErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(caCertErr).NotTo(o.HaveOccurred())
		opensslCMD = fmt.Sprintf("openssl genrsa -out %v 2048", serverKeypem)
		_, serverKeyErr := exec.Command("bash", "-c", opensslCMD).Output()
		o.Expect(serverKeyErr).NotTo(o.HaveOccurred())
		serverconfCMD := fmt.Sprintf(`cat > %v << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = %s
EOF`, serverconf, fqdnName.Hostname())
		_, serverconfErr := exec.Command("bash", "-c", serverconfCMD).Output()
		o.Expect(serverconfErr).NotTo(o.HaveOccurred())
		serverWithSANCMD := fmt.Sprintf(`openssl req -new -key %v -out %v -subj "/CN=%s_server" -config %v`, serverKeypem, serverWithSANcsr, CN_BASE, serverconf)
		_, serverWithSANErr := exec.Command("bash", "-c", serverWithSANCMD).Output()
		o.Expect(serverWithSANErr).NotTo(o.HaveOccurred())
		serverCertWithSANCMD := fmt.Sprintf(`openssl x509 -req -in %v -CA %v -CAkey %v -CAcreateserial -out %v -days 100000 -extensions v3_req -extfile %s`, serverWithSANcsr, caCertpem, caKeypem, serverCertWithSAN, serverconf)
		_, serverCertWithSANErr := exec.Command("bash", "-c", serverCertWithSANCMD).Output()
		o.Expect(serverCertWithSANErr).NotTo(o.HaveOccurred())

		exutil.By("4. Creating custom secret using server certificate")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "tls", "custom-api-cert", "--cert="+serverCertWithSAN, "--key="+serverKeypem, "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. Add new certificate to apiserver")
		patchCmd := fmt.Sprintf(`{"spec": {"configuration": {"apiServer": {"servingCerts": {"namedCertificates": [{"names": ["%s"], "servingCertificate": {"name": "custom-api-cert"}}]}}}}}`, fqdnName.Hostname())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "--type=merge", "-p", patchCmd).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. Add new certificates to hosted kubeconfig")
		// To avoid error "Unable to connect to the server: tls: failed to verify certificate: x509: certificate signed by unknown authority." updating kubeconfig
		updateKubeconfigWithConcatenatedCert(caCertpem, originCA, originHostedKube, newCA)

		exutil.By("7. Checking if kube-apiserver is restarted")
		waitApiserverRestartOfHypershift(oc, "kube-apiserver", guestClusterNS, 480)

		exutil.By("8. Validate new certificates")
		returnValues := []string{"Subject", "Issuer"}
		certDetails, err := urlHealthCheck(fqdnName.Hostname(), caCertpem, returnValues)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(certDetails.Subject)).To(o.ContainSubstring("CN=kas-test-cert_server"))
		o.Expect(string(certDetails.Issuer)).To(o.ContainSubstring("CN=kas-test-cert_ca"))

		exutil.By("9. Validate old certificates should not work")
		certDetails, err = urlHealthCheck(fqdnName.Hostname(), originCA, returnValues)
		o.Expect(err).To(o.HaveOccurred())
	})
})
