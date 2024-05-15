package mco

import (
	"fmt"
	"os"
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

		initialCMCreationTime := mergedTrustedImageRegistryCACM.GetOrFail(`{.metadata.creationTimestamp}`)
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

		o.Expect(mergedTrustedImageRegistryCACM.Get(`{.metadata.creationTimestamp}`)).To(
			o.Equal(initialCMCreationTime),
			"The %s resource was not patched! it was recreated! The configmap should be patched since https://issues.redhat.com/browse/OCPBUGS-18800")

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

		o.Expect(mergedTrustedImageRegistryCACM.Get(`{.metadata.creationTimestamp}`)).To(
			o.Equal(initialCMCreationTime),
			"The %s resource was not patched! it was recreated! The configmap should be patched since https://issues.redhat.com/browse/OCPBUGS-18800")

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
			EventuallyFileExistsInNode(kubeCloudCertFile, kubeCloudCertContent, mcp.GetNodesOrFail()[0], "3m", "20s")
		} else {
			logger.Infof("No KubeCloud cert was configured and it was not possible to define a new one, we skip the cloudCA validation")
		}

		logger.Infof("OK!\n")

	})

	g.It("Author:rioliu-NonHyperShiftHOST-NonPreRelease-Longduration-High-71991-post action of user-ca-bundle change will skip drain,reboot and restart crio service [Disruptive]", func() {
		var (
			mcName                 = "mco-tc-71991"
			filePath               = "/etc/pki/ca-trust/source/anchors/openshift-config-user-ca-bundle.crt"
			mode                   = 420 // decimal 0644
			objsignCABundlePemPath = "/etc/pki/ca-trust/extracted/pem/objsign-ca-bundle.pem"
			node                   = mcp.GetSortedNodesOrFail()[0]
			behaviourValidator     = UpdateBehaviourValidator{
				RebootNodesShouldBeSkipped: true,
				DrainNodesShoulBeSkipped:   true,
				Checkers: []Checker{
					NodeEventsChecker{
						EventsSequence:        []string{"Reboot", "Drain"},
						EventsAreNotTriggered: true,
					},
				},
			}
		)

		behaviourValidator.Initialize(mcp, nil)

		exutil.By("Removing all MCD pods to clean the logs")
		o.Expect(RemoveAllMCDPods(oc)).To(o.Succeed(), "Error removing all MCD pods in %s namespace", MachineConfigNamespace)
		logger.Infof("OK!\n")

		exutil.By("Create a new certificate")
		_, caPath, err := createCA(createTmpDir(), "newcert.pem")
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating a new random certificate")

		cert, err := os.ReadFile(caPath)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error reading the new random certificate")
		logger.Infof("OK!\n")

		exutil.By("Create the MachineConfig with the new certificate")
		file := ign32File{
			Path: filePath,
			Contents: ign32Contents{
				Source: GetBase64EncodedFileSourceContent(string(cert)),
			},
			Mode: PtrTo(mode),
		}

		mc := NewMachineConfig(oc.AsAdmin(), mcName, mcp.GetName())
		mc.parameters = []string{fmt.Sprintf("FILES=[%s]", string(MarshalOrFail(file)))}
		mc.skipWaitForMcp = true
		defer mc.delete()

		mc.create()
		logger.Infof("OK!\n")

		// Check that the MC is applied according to the expected behaviour
		behaviourValidator.Validate()

		exutil.By("Check that the certificate was created and updated in the cluster by using update-ca-trust command")
		certRemote := NewRemoteFile(node, filePath)
		objsignCABundleRemote := NewRemoteFile(node, objsignCABundlePemPath)

		o.Eventually(certRemote, "5m", "20s").Should(Exist(),
			"The file %s does not exist in the node %s after applying the configuration", certRemote.fullPath, node.GetName())

		o.Eventually(objsignCABundleRemote, "5m", "20s").Should(Exist(),
			"The file %s does not exist in the node %s after applying the configuration", certRemote.fullPath, node.GetName())

		o.Expect(certRemote.Fetch()).To(o.Succeed(),
			"There was an error trying to the the content of file %s in node %s", certRemote.fullPath, node.GetName())

		// diff /etc/pki/ca-trust/extracted/pem/objsign-ca-bundle.pem /etc/pki/ca-trust/source/anchors/openshift-config-user-ca-bundle.crt | less
		// The new certificate should be included in the /etc/pki/ca-trust/extracted/pem/objsign-ca-bundle.pem file when we execute the update-ca-trust command
		o.Expect(objsignCABundleRemote.Read()).To(exutil.Secure(HaveContent(o.ContainSubstring(certRemote.GetTextContent()))),
			"In node %s: The the content of the file %s should have been added to the file %s. Command 'update-ca-trust' was not executed by MCD",
			node.GetName(), certRemote.fullPath, objsignCABundleRemote.fullPath)
		logger.Infof("OK!\n")

		exutil.By("Removing all MCD pods to clean the logs before the MC deletion")
		o.Expect(RemoveAllMCDPods(oc)).To(o.Succeed(), "Error removing all MCD pods in %s namespace", MachineConfigNamespace)
		logger.Infof("OK!\n")

		exutil.By("Delete the MachineConfig")
		behaviourValidator.Initialize(mcp, nil) // re-initialize the validator to ignore previous events
		mc.deleteNoWait()
		logger.Infof("OK!\n")

		// Check that the MC is removed according to the expected behaviour
		behaviourValidator.Validate()

		exutil.By("Check that the certificate file is now empty and the cluster was updated with update-ca-trust")
		// The file is not removed, it is always present but with empty content
		o.Eventually(certRemote.Read, "5m", "20s").Should(exutil.Secure(HaveContent(o.BeEmpty())),
			"The file %s does not exist in the node %s after applying the configuration", certRemote.fullPath, node.GetName())
		o.Eventually(objsignCABundleRemote, "5m", "20s").Should(Exist(),
			"The file %s does not exist in the node %s but it should exist after removing the configuration", certRemote.fullPath, node.GetName())

		o.Expect(objsignCABundleRemote.Read()).NotTo(exutil.Secure(HaveContent(o.ContainSubstring(certRemote.GetTextContent()))),
			"In node %s: The the content of the file %s should have been removed from the file %s. Command 'update-ca-trust' was not executed by MCD after removing the MC",
			node.GetName(), certRemote.fullPath, objsignCABundleRemote.fullPath)
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
