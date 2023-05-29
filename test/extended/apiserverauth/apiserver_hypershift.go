package apiserverauth

import (
	"fmt"
	"strconv"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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
	)

	g.BeforeEach(func() {
		guestClusterName, guestClusterKube, hostedClusterNS = exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		e2e.Logf("%s, %s, %s", guestClusterName, guestClusterKube, hostedClusterNS)
		oc.SetGuestKubeconf(guestClusterKube)

		iaasPlatform = exutil.CheckPlatform(oc)

	})

	// author: kewang@redhat.com
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-Longduration-NonPreRelease-Author:kewang-Medium-62093-[Apiserver] Wire tlsSecurityProfile cipher config from apiservers/cluster into apiservers of hosted cluster [Slow][Disruptive]", func() {

		var (
			apiserverConfigPatch = `[{"op": "replace", "path": "/spec/configuration/apiServer", "value": {}}]`
			defaultCipherPatch   = `[{"op": "replace", "path": "/spec/configuration/apiServer/tlsSecurityProfile", "value":}]`
			defaultCipherSuite   = `["TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"] VersionTLS12`
			cipherItems          = []struct {
				cipherType  string
				cipherSuite string
				patch       string
			}{
				{
					cipherType:  "custom",
					cipherSuite: `["TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"] VersionTLS11`,
					patch:       `[{"op": "replace", "path":  "/spec/configuration/apiServer/tlsSecurityProfile", "value":{"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256"],"minTLSVersion":"VersionTLS11"},"type":"Custom"}}]`,
				},
				{
					cipherType:  "Old",
					cipherSuite: `["TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256","TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA","TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA","TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA","TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA","TLS_RSA_WITH_AES_128_GCM_SHA256","TLS_RSA_WITH_AES_256_GCM_SHA384","TLS_RSA_WITH_AES_128_CBC_SHA256","TLS_RSA_WITH_AES_128_CBC_SHA","TLS_RSA_WITH_AES_256_CBC_SHA","TLS_RSA_WITH_3DES_EDE_CBC_SHA"] VersionTLS10`,
					patch:       `[{"op": "replace", "path":  "/spec/configuration/apiServer/tlsSecurityProfile", "value":{"old":{},"type":"Old"}}]`,
				},
				{
					cipherType:  "Intermediate",
					cipherSuite: defaultCipherSuite,
					patch:       `[{"op": "replace", "path":  "/spec/configuration/apiServer/tlsSecurityProfile", "value":{"intermediate":{},"type":"Intermediate"}}]`,
				},
			}
		)

		// Currently, the test is only supported on AWSd on AWS
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		guestClusterNS = hostedClusterNS + "-" + guestClusterName
		e2e.Logf("HostedClusterControlPlaneNS: %v", guestClusterNS)

		defer func() {
			g.By("-->> Restoring cluster's ciphers")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "--type=json", "-p", defaultCipherPatch).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Checking if apiservers are restarted
			errKas := waitApiserverRestartOfHypershift(oc, "kube-apiserver", guestClusterNS, 480)
			o.Expect(errKas).NotTo(o.HaveOccurred())
			errOas := waitApiserverRestartOfHypershift(oc, "openshift-apiserver", guestClusterNS, 180)
			o.Expect(errOas).NotTo(o.HaveOccurred())
			errOauth := waitApiserverRestartOfHypershift(oc, "oauth-openshift", guestClusterNS, 180)
			o.Expect(errOauth).NotTo(o.HaveOccurred())
			e2e.Logf("#### Check cipherSuites and minTLSVersion of oauth, openshift-apiserver and kubeapiservers config.")
			errChipher := verifyHypershiftCiphers(oc, defaultCipherSuite, guestClusterNS)
			if errChipher != nil {
				exutil.AssertWaitPollNoErr(errChipher, "Ciphers are not matched the expected Intermediate type!")
			}

		}()

		g.By("-->> 1.) Check the default cipherSuites and minTLSVersion of oauth, openshift-apiserver and kubeapiservers config.")
		errChipher := verifyHypershiftCiphers(oc, defaultCipherSuite, guestClusterNS)
		if errChipher != nil {
			exutil.AssertWaitPollNoErr(errChipher, fmt.Sprintf("The ciphers are not matched : %s", defaultCipherSuite))
		}
		e2e.Logf(`The ciphers type are matched default "Intermediate".`)

		// The Kubernetes API server will not recursively create nested objects for a JSON patch input. This behaviour is consistent with the JSON Patch specification in RFC 6902, section A.12
		outCfg, errCfg := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "-o", `jsonpath={.spec.configuration.apiServer}`).Output()
		o.Expect(errCfg).NotTo(o.HaveOccurred())
		if len(outCfg) == 0 {
			e2e.Logf("Creating the parent path of Apiserver configuration if not existed.")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "--type=json", "-p", apiserverConfigPatch).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		// Apply supported chipher types
		for i, cipherItem := range cipherItems {
			i += 2
			oldVer, errOldrVer := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "-o", `jsonpath={.status.conditions[?(@.type=="KubeAPIServerAvailable")].observedGeneration}`).Output()
			o.Expect(errOldrVer).NotTo(o.HaveOccurred())
			intOldVer, _ := strconv.Atoi(oldVer)
			o.Expect(intOldVer).To(o.BeNumerically(">", 0))
			e2e.Logf("observedGeneration: %v", intOldVer)

			g.By(fmt.Sprintf("-->> %d.1) Patching the apiserver cluster with ciphers:  %s", i, cipherItem.cipherType))
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "--type=json", "-p", cipherItem.patch).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Checking if apiservers are restarted
			errKas := waitApiserverRestartOfHypershift(oc, "kube-apiserver", guestClusterNS, 480)
			o.Expect(errKas).NotTo(o.HaveOccurred())
			errOas := waitApiserverRestartOfHypershift(oc, "openshift-apiserver", guestClusterNS, 180)
			o.Expect(errOas).NotTo(o.HaveOccurred())
			errOauth := waitApiserverRestartOfHypershift(oc, "oauth-openshift", guestClusterNS, 180)
			o.Expect(errOauth).NotTo(o.HaveOccurred())

			newVer, errNewVer := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", guestClusterName, "-n", hostedClusterNS, "-o", `jsonpath={.status.conditions[?(@.type=="KubeAPIServerAvailable")].observedGeneration}`).Output()
			o.Expect(errNewVer).NotTo(o.HaveOccurred())
			e2e.Logf("observedGeneration: %v", newVer)
			o.Expect(strconv.Atoi(newVer)).To(o.BeNumerically(">", intOldVer))

			g.By(fmt.Sprintf("-->> %d.2) Check cipherSuites and minTLSVersion of oauth, openshift-apiserver and kubeapiservers config.", i))
			errChipher := verifyHypershiftCiphers(oc, cipherItem.cipherSuite, guestClusterNS)
			if errChipher != nil {
				exutil.AssertWaitPollNoErr(errChipher, fmt.Sprintf("Ciphers are not matched : %s", cipherItem.cipherType))
			}
			e2e.Logf("#### Ciphers are matched: %s", cipherItem.cipherType)
		}
	})
})
