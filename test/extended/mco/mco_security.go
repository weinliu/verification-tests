package mco

import (
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO security", func() {
	defer g.GinkgoRecover()

	var (
		oc   = exutil.NewCLI("mco-security", exutil.KubeConfigPath())
		wMcp *MachineConfigPool
		mMcp *MachineConfigPool
		// Compact compatible MCP. If the node is compact/SNO this variable will be the master pool, else it will be the worker pool
		mcp *MachineConfigPool
		cc  *ControllerConfig
	)

	g.JustBeforeEach(func() {
		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
		mcp = GetCompactCompatiblePool(oc.AsAdmin())
		cc = NewControllerConfig(oc.AsAdmin(), "machine-config-controller")
		logger.Infof("%s %s %s", wMcp, mMcp, mcp)

		preChecks(oc)
	})
	g.It("Author:sregidor-NonHyperShiftHOST-Medium-66048-Check image registry user bundle certificate [Disruptive]", func() {

		if !IsCapabilityEnabled(oc.AsAdmin(), "ImageRegistry") {
			g.Skip("ImageRegistry is not installed, skip this test")
		}

		var (
			mergedTrustedImageRegistryCACM = NewConfigMap(oc.AsAdmin(), "openshift-config-managed", "merged-trusted-image-registry-ca")
			imageConfig                    = NewResource(oc.AsAdmin(), "image.config.openshift.io", "cluster")
			certFileName                   = "caKey.pem"
			cmName                         = "cm-test-ca"
		)

		exutil.By("Get current image.config spec")
		initImageConfigSpec := imageConfig.GetOrFail(`{.spec}`)

		defer func() {
			logger.Infof("Restore original image.config spec: %s", initImageConfigSpec)
			_ = imageConfig.Patch("json", `[{ "op": "add", "path": "/spec", "value": `+initImageConfigSpec+`}]`)
		}()
		logger.Infof("OK!\n")

		exutil.By("Add new  additionalTrustedCA to the image.config resource")
		logger.Infof("Creating new config map with a new CA")
		additionalTrustedCM, err := CreateConfigMapWithRandomCert(oc.AsAdmin(), "openshift-config", cmName, certFileName)
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error creating a configmap with a CA")

		defer additionalTrustedCM.Delete()

		newCertificate := additionalTrustedCM.GetDataValueOrFail(certFileName)

		logger.Infof("Configure the image.config resource to use the new configmap")
		o.Expect(imageConfig.Patch("merge", fmt.Sprintf(`{"spec": {"additionalTrustedCA": {"name": "%s"}}}`, cmName))).To(
			o.Succeed(),
			"Error setting the new image.config spec")

		logger.Infof("OK!\n")

		exutil.By("Check that the ControllerConfig has been properly synced")
		o.Eventually(cc.GetImageRegistryBundleUserDataByFileName,
			"3m", "20s").WithArguments(certFileName).Should(
			exutil.Secure(o.Equal(newCertificate)),
			"The new certificate was not properly added to the controller config imageRegistryBundleUserData")

		usrDataInfo, err := GetCertificatesInfoFromPemBundle(certFileName, []byte(newCertificate))
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error extracting certificate info from the new additional trusted CA")

		o.Expect(cc.GetCertificatesInfoByBundleFileName(certFileName)).To(
			o.Equal(usrDataInfo),
			"The information reported in the ControllerConfig for bundle file %s is wrong", certFileName)

		logger.Infof("OK!\n")

		exutil.By("Check that the merged-trusted-image-registry-ca configmap has been properly synced")
		o.Expect(mergedTrustedImageRegistryCACM.GetDataValueOrFail(certFileName)).To(
			exutil.Secure(o.Equal(newCertificate)),
			"The configmap -n  openshift-config-managed merged-trusted-image-registry-ca was not properly synced")

		logger.Infof("OK!\n")

		// We verify that all nodes in the pools have the new certificate (be aware that windows nodes do not belong to any pool, we are skipping them)
		for _, node := range append(wMcp.GetNodesOrFail(), mMcp.GetNodesOrFail()...) {
			exutil.By(fmt.Sprintf("Check that the certificate was correctly deployed in node %s", node.GetName()))

			EventuallyImageRegistryCertificateExistsInNode(certFileName, newCertificate, node, "5m", "30s")
			logger.Infof("OK!\n")
		}

		exutil.By("Configure an empty image.config spec")
		o.Expect(imageConfig.Patch("json", `[{ "op": "add", "path": "/spec", "value": {}}]`)).To(
			o.Succeed(),
			"Error configuring an empty image.config spec")
		logger.Infof("OK!\n")

		exutil.By("Check that the ControllerConfig was properly synced")

		o.Eventually(cc.GetImageRegistryBundleUserData, "45s", "20s").ShouldNot(
			exutil.Secure(o.HaveKey(certFileName)),
			"The new certificate was not properly removed from the ControllerConfig imageRegistryBundleUserData")

		o.Expect(cc.GetCertificatesInfoByBundleFileName(certFileName)).To(
			exutil.Secure(o.BeEmpty()),
			"The information reported in the ControllerConfig for bundle file %s was not removed", certFileName)

		logger.Infof("OK!\n")

		exutil.By("Check that the merged-trusted-image-registry-ca configmap has been properly synced")
		o.Expect(mergedTrustedImageRegistryCACM.GetDataMap()).NotTo(
			exutil.Secure(o.HaveKey(newCertificate)),
			"The certificate was not removed from the configmap -n  openshift-config-managed merged-trusted-image-registry-ca")

		logger.Infof("OK!\n")

		// We verify that the certificate was removed from all nodes in the pools (be aware that windows nodes do not belong to any pool, we are skipping them)
		for _, node := range append(wMcp.GetNodesOrFail(), mMcp.GetNodesOrFail()...) {
			exutil.By(fmt.Sprintf("Check that the certificate was correctly removed from node %s", node.GetName()))

			certPath := filepath.Join(ImageRegistryCertificatesDir, certFileName, ImageRegistryCertificatesFileName)
			rfCert := NewRemoteFile(node, certPath)

			logger.Infof("Checking certificate file %s", certPath)

			o.Eventually(rfCert.Exists, "5m", "20s").Should(
				o.BeFalse(),
				"The certificate %s was not removed from the node %s. But it should have been removed after the image.config reconfiguration",
				certPath, node.GetName())

			logger.Infof("OK!\n")
		}
	})
	g.It("Author:sregidor-NonHyperShiftHOST-High-67660-MCS generates ignition configs with certs [Disruptive]", func() {
		var (
			proxy                      = NewResource(oc.AsAdmin(), "proxy", "cluster")
			certFileKey                = "ca-bundle.crt"
			cloudCertFileKey           = "ca-bundle.pem"
			userCABundleConfigMap      = NewConfigMap(oc.AsAdmin(), "openshift-config", "user-ca-bundle")
			cmName                     = "test-proxy-config"
			cmNamespace                = "openshift-config"
			proxyConfigMap             *ConfigMap
			kubeCloudProviderConfigMap = GetCloudProviderConfigMap(oc.AsAdmin())
			kubeCloudManagedConfigMap  = NewConfigMap(oc.AsAdmin(), "openshift-config-managed", "kube-cloud-config")
			kubeCertFile               = "/etc/kubernetes/kubelet-ca.crt"
			userCABundleCertFile       = "/etc/pki/ca-trust/source/anchors/openshift-config-user-ca-bundle.crt"
			kubeCloudCertFile          = "/etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem"
			ignitionConfig             = "3.4.0"
		)

		logger.Infof("Using pool %s for testing", mcp.GetName())

		// Create a new config map and configure the proxy additional trusted CA if necessary
		proxyConfigMapName := proxy.GetOrFail(`{.spec.trustedCA.name}`)
		if proxyConfigMapName == "" {
			var err error
			exutil.By("Configure the proxy with an additional trusted CA")
			logger.Infof("Create a configmap with the CA")
			proxyConfigMap, err = CreateConfigMapWithRandomCert(oc.AsAdmin(), cmNamespace, cmName, certFileKey)
			o.Expect(err).NotTo(o.HaveOccurred(),
				"Error creating a configmap with a CA")
			defer proxyConfigMap.Delete()

			logger.Infof("Patch the proxy resource to use the new configmap")
			initProxySpec := proxy.GetOrFail(`{.spec}`)
			defer func() {
				logger.Infof("Restore original proxy spec: %s", initProxySpec)
				_ = proxy.Patch("json", `[{ "op": "add", "path": "/spec", "value": `+initProxySpec+`}]`)
			}()
			proxy.Patch("merge", fmt.Sprintf(`{"spec": {"trustedCA": {"name": "%s"}}}`, cmName))
			logger.Infof("OK!\n")
		} else {
			logger.Infof("The proxy is already configured to use the CA inside this configmap: %s", proxyConfigMapName)
			proxyConfigMap = NewConfigMap(oc.AsAdmin(), "openshift-config", proxyConfigMapName)
		}

		exutil.By(fmt.Sprintf(`Check that the "%s" is in the ignition config`, kubeCertFile))
		jsonPath := fmt.Sprintf(`storage.files.#(path=="%s")`, kubeCertFile)
		o.Eventually(mcp.GetMCSIgnitionConfig,
			"1m", "20s").WithArguments(true, ignitionConfig).ShouldNot(
			HavePathWithValue(jsonPath, o.BeEmpty()),
			"The file %s is not served in the ignition config", kubeCertFile)

		logger.Infof("OK!\n")

		exutil.By(fmt.Sprintf(`Check that the "%s" is in the ignition config`, userCABundleCertFile))
		logger.Infof("Check that the file is served in the ignition config")
		jsonPath = fmt.Sprintf(`storage.files.#(path=="%s")`, userCABundleCertFile)
		o.Eventually(mcp.GetMCSIgnitionConfig,
			"1m", "20s").WithArguments(true, ignitionConfig).ShouldNot(
			HavePathWithValue(jsonPath, o.BeEmpty()),
			"The file %s is not served in the ignition config", userCABundleCertFile)

		logger.Infof("Check that the file has the right content in the nodes")

		certContent := ""
		if userCABundleConfigMap.Exists() {
			userCABundleCert, exists, err := userCABundleConfigMap.HasKey(certFileKey)
			o.Expect(err).NotTo(o.HaveOccurred(), "Error checking if %s contains key '%s'", userCABundleConfigMap, certFileKey)
			if exists {
				certContent = userCABundleCert
			}
		} else {
			logger.Infof("%s does not exist. We don't take it into account", userCABundleConfigMap)
		}

		// OCPQE-17800 only merge the cert contents when trusted CA in proxy/cluster is not cm/user-ca-bundle
		if proxyConfigMap.GetName() != userCABundleConfigMap.GetName() {
			certContent += proxyConfigMap.GetDataValueOrFail(certFileKey)
		}

		EventuallyFileExistsInNode(userCABundleCertFile, certContent, mcp.GetNodesOrFail()[0], "3m", "20s")

		logger.Infof("OK!\n")

		exutil.By(fmt.Sprintf(`Check that the "%s" is in the ignition config`, kubeCloudCertFile))
		kubeCloudCertContent, err := kubeCloudManagedConfigMap.GetDataValue("ca-bundle.pem")
		if err != nil {
			logger.Infof("No KubeCloud cert configured, configuring a new value")
			if kubeCloudProviderConfigMap != nil && kubeCloudProviderConfigMap.Exists() {
				_, caPath, err := createCA(createTmpDir(), cloudCertFileKey)
				o.Expect(err).NotTo(o.HaveOccurred(), "Error creating a new random certificate")
				defer kubeCloudProviderConfigMap.RemoveDataKey(cloudCertFileKey)
				kubeCloudProviderConfigMap.SetData("--from-file=" + cloudCertFileKey + "=" + caPath)
				kubeCloudCertContent = kubeCloudManagedConfigMap.GetDataValueOrFail(cloudCertFileKey)

			} else {
				logger.Infof("It is not possible to configure a new CloudCA. CloudProviderConfig configmap is not defined in the infrastructure resource or it does not exist")
				kubeCloudCertContent = ""
			}

		}

		if kubeCloudCertContent != "" {
			logger.Infof("Check that the file is served in the ignition config")
			jsonPath = fmt.Sprintf(`storage.files.#(path=="%s")`, kubeCloudCertFile)
			o.Eventually(mcp.GetMCSIgnitionConfig,
				"3m", "20s").WithArguments(true, ignitionConfig).ShouldNot(
				HavePathWithValue(jsonPath, o.BeEmpty()),
				"The file %s is not served in the ignition config", kubeCloudCertFile)

			logger.Infof("Check that the file has the right content in the nodes")
			EventuallyFileExistsInNode(kubeCloudCertFile, kubeCloudCertContent, mcp.GetNodesOrFail()[0], "20m", "20s")
		} else {
			logger.Infof("No KubeCloud cert was configured and it was not possible to define a new one, we skip the cloudCA validation")
		}

		logger.Infof("OK!\n")

	})
})

// EventuallyFileExistsInNode fails the test if the certificate file does not exist in the node after the time specified as parameters
func EventuallyImageRegistryCertificateExistsInNode(certFileName, certContent string, node Node, timeout, poll string) {
	certPath := filepath.Join(ImageRegistryCertificatesDir, certFileName, ImageRegistryCertificatesFileName)
	EventuallyFileExistsInNode(certPath, certContent, node, timeout, poll)
}

// EventuallyFileExistsInNode fails the test if the file does not exist in the node after the time specified as parameters
func EventuallyFileExistsInNode(filePath, expectedContent string, node Node, timeout, poll string) {
	logger.Infof("Checking file %s in node %s", filePath, node.GetName())
	rfCert := NewRemoteFile(node, filePath)
	o.Eventually(func(gm o.Gomega) { // Passing o.Gomega as parameter we can use assertions inside the Eventually function without breaking the retries.
		gm.Expect(rfCert.Fetch()).To(o.Succeed(),
			"Cannot read the certificate file %s in node:%s ", rfCert.fullPath, node.GetName())

		gm.Expect(rfCert.GetTextContent()).To(exutil.Secure(o.Equal(expectedContent)),
			"the certificate stored in file %s does not match the expected value", rfCert.fullPath)
	}, timeout, poll).
		Should(o.Succeed(),
			"The file %s in node %s does not contain the expected certificate.", rfCert.GetFullPath(), node.GetName())
}
