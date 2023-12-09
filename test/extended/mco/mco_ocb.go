package mco

import (
	"fmt"
	"regexp"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO ocb", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("mco-ocb", exutil.KubeConfigPath())
	)

	g.JustBeforeEach(func() {
		preChecks(oc)
		// According to https://issues.redhat.com/browse/MCO-831, featureSet:TechPreviewNoUpgrade is required
		// xref: featureGate: OnClusterBuild
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test")
		}
	})

	g.It("Author:sregidor-NonPreRelease-High-66567-OCB Wiring up Productionalized Build Controller [Disruptive]", func() {

		infraMcpName := "infra"

		exutil.By("Creating necessary resources to enable OCB functionality")

		defer func() { _ = cleanOCBTestConfigResources(oc.AsAdmin()) }()
		o.Expect(createOCBDefaultTestConfigResourcesFromPullSecret(oc.AsAdmin())).NotTo(o.BeNil(),
			"Error creating the necessary CMs and Secrets to enable OCB functionality")

		logger.Infof("OK!\n")

		exutil.By("Create custom infra MCP")
		infraMcp := NewMachineConfigPool(oc.AsAdmin(), infraMcpName)
		infraMcp.template = generateTemplateAbsolutePath("custom-machine-config-pool.yaml")
		defer infraMcp.delete()
		infraMcp.create()

		logger.Infof("OK!\n")

		exutil.By("Label the infra MCP to enable the OCB functionality")
		infraMcp.EnableOnClusterBuild()
		logger.Infof("OK!\n")

		exutil.By("Check that the deployment machine-os-builder is created")
		mOSBuilder := NewNamespacedResource(oc.AsAdmin(), "deployment", MachineConfigNamespace, "machine-os-builder")

		o.Eventually(mOSBuilder, "5m", "30s").Should(Exist(),
			"The machine-os-builder deployment was not created when the OCB functionality was enabled in the infra pool")

		o.Expect(mOSBuilder.Get(`{.spec.template.spec.containers[?(@.name=="machine-os-builder")].command}`)).To(o.ContainSubstring("machine-os-builder"),
			"Error the machine-os-builder is not invoking the machine-os-builder binary")

		o.Eventually(mOSBuilder.Get, "3m", "30s").WithArguments(`{.spec.replicas}`).Should(o.Equal("1"),
			"The machine-os-builder deployment was created but the configured number of replicas is not the expected one")
		o.Eventually(mOSBuilder.Get, "2m", "30s").WithArguments(`{.status.availableReplicas}`).Should(o.Equal("1"),
			"The machine-os-builder deployment was created but the available number of replicas is not the expected one")

		exutil.AssertAllPodsToBeReady(oc.AsAdmin(), MachineConfigNamespace)
		logger.Infof("OK!\n")

		exutil.By("Remove the OCB label from the infra pool")
		infraMcp.DisableOnClusterBuild()

		o.Eventually(mOSBuilder, "5m", "30s").ShouldNot(Exist(),
			"The machine-os-builder deployment was not removed when the infra pool was unlabeled")
		exutil.AssertAllPodsToBeReady(oc.AsAdmin(), MachineConfigNamespace)
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonPreRelease-Medium-66661-OCB validate configmap on-cluster-build-config [Disruptive]", func() {

		var (
			expectedErrorMessage string
			ocpConfig            *OCBConfigMapValues
			infraMcpName         = "infra"
		)

		// Make sure that no initial config is present
		exutil.By("Clean OCB configuration")
		logger.Infof("If there is any previous OCB configuraion resources, we clean them")
		o.Expect(cleanOCBTestConfigResources(oc.AsAdmin())).To(o.Succeed(),
			"Error cleaning the OCB config resources")
		logger.Infof("OK!\n")

		exutil.By("Create custom infra MCP")
		infraMcp := NewMachineConfigPool(oc.AsAdmin(), infraMcpName)
		infraMcp.template = generateTemplateAbsolutePath("custom-machine-config-pool.yaml")
		defer infraMcp.delete()
		infraMcp.create()

		logger.Infof("OK!\n")

		// NO CONFIGMAP
		exutil.By("Check no configmap scenario")
		ocpConfig = nil
		expectedErrorMessage = regexp.QuoteMeta(`could not update Machine OS Builder deployment: on-cluster-build-config ConfigMap missing, did you create it?`)
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, "", expectedErrorMessage)
		logger.Infof("OK!\n")

		// MISSING KEYS
		exutil.By("Check missing key in configmap: baseImagePullSecretName")
		ocpConfig = &OCBConfigMapValues{
			finalImagePushSecretName: PtrStr(OCBDefaultFinalImagePushSecretName),
			finalImagePullspec:       PtrStr(DefaultLayeringQuayRepository),
		}
		expectedErrorMessage = regexp.QuoteMeta(`could not update Machine OS Builder deployment: missing required key "baseImagePullSecretName" in configmap on-cluster-build-config`)
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, "", expectedErrorMessage)
		logger.Infof("OK!\n")

		exutil.By("Check missing key in configmap: finalImagePushSecretName")
		ocpConfig = &OCBConfigMapValues{
			baseImagePullSecretName: PtrStr(OCBDefaultBaseImagePullSecretName),
			finalImagePullspec:      PtrStr(DefaultLayeringQuayRepository),
		}
		expectedErrorMessage = regexp.QuoteMeta(`could not update Machine OS Builder deployment: missing required key "finalImagePushSecretName" in configmap on-cluster-build-config`)
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, "", expectedErrorMessage)
		logger.Infof("OK!\n")

		exutil.By("Check missing key in configmap: finalImagePullspec")
		ocpConfig = &OCBConfigMapValues{
			finalImagePushSecretName: PtrStr(OCBDefaultFinalImagePushSecretName),
			baseImagePullSecretName:  PtrStr(OCBDefaultBaseImagePullSecretName),
		}
		expectedErrorMessage = regexp.QuoteMeta(`could not update Machine OS Builder deployment: missing required key "finalImagePullspec" in configmap on-cluster-build-config`)
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, "", expectedErrorMessage)
		logger.Infof("OK!\n")

		// EMPTY KEYS
		exutil.By("Check empty key in configmap: baseImagePullSecretName")
		ocpConfig = &OCBConfigMapValues{
			baseImagePullSecretName:  PtrStr(""),
			finalImagePushSecretName: PtrStr(OCBDefaultFinalImagePushSecretName),
			finalImagePullspec:       PtrStr(DefaultLayeringQuayRepository),
		}
		expectedErrorMessage = regexp.QuoteMeta(`could not update Machine OS Builder deployment: key "baseImagePullSecretName" in configmap on-cluster-build-config has an empty value`)
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, "", expectedErrorMessage)
		logger.Infof("OK!\n")

		exutil.By("Check empty key in configmap: finalImagePushSecretName")
		ocpConfig = &OCBConfigMapValues{
			baseImagePullSecretName:  PtrStr(OCBDefaultBaseImagePullSecretName),
			finalImagePushSecretName: PtrStr(""),
			finalImagePullspec:       PtrStr(DefaultLayeringQuayRepository),
		}
		expectedErrorMessage = regexp.QuoteMeta(`could not update Machine OS Builder deployment: key "finalImagePushSecretName" in configmap on-cluster-build-config has an empty value`)
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, "", expectedErrorMessage)
		logger.Infof("OK!\n")

		exutil.By("Check empty key in configmap: finalImagePullspec")
		ocpConfig = &OCBConfigMapValues{
			baseImagePullSecretName:  PtrStr(OCBDefaultBaseImagePullSecretName),
			finalImagePushSecretName: PtrStr(OCBDefaultFinalImagePushSecretName),
			finalImagePullspec:       PtrStr(""),
		}
		expectedErrorMessage = regexp.QuoteMeta(`could not update Machine OS Builder deployment: key "finalImagePullspec" in configmap on-cluster-build-config has an empty value`)
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, "", expectedErrorMessage)
		logger.Infof("OK!\n")

		// SECRETS NOT FOUND
		exutil.By("Check secret not found: baseImagePullSecretName")
		ocpConfig = &OCBConfigMapValues{
			baseImagePullSecretName:  PtrStr("fake-base-image-secret"),
			finalImagePushSecretName: PtrStr(OCBDefaultFinalImagePushSecretName),
			finalImagePullspec:       PtrStr(DefaultLayeringQuayRepository),
		}
		message := `could not update Machine OS Builder deployment: secret %s from on-cluster-build-config is not found. Did you use the right secret name?`

		expectedErrorMessage = regexp.QuoteMeta(fmt.Sprintf(message, *ocpConfig.baseImagePullSecretName))
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, *ocpConfig.baseImagePullSecretName, expectedErrorMessage)
		logger.Infof("OK!\n")

		exutil.By("Check secret not found: finalImagePushSecretName")
		ocpConfig = &OCBConfigMapValues{
			baseImagePullSecretName:  PtrStr(OCBDefaultBaseImagePullSecretName),
			finalImagePushSecretName: PtrStr("fake-final-image-secret"),
			finalImagePullspec:       PtrStr(DefaultLayeringQuayRepository),
		}
		expectedErrorMessage = regexp.QuoteMeta(fmt.Sprintf(message, *ocpConfig.finalImagePushSecretName))
		CheckOCBConfigError(oc.AsAdmin(), infraMcp, ocpConfig, *ocpConfig.finalImagePushSecretName, expectedErrorMessage)
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonPreRelease-High-66573-OCB configure image builder type for config map on-cluster-build-config [Disruptive]", func() {
		var (
			infraMcpName = "infra"
			cm           = NewNamespacedResource(oc.AsAdmin(), "cm", MachineConfigNamespace, OCBConfigmapName)
			MCOperator   = NewResource(oc.AsAdmin(), "ClusterOperator", "machine-config")
		)
		g.Skip("Test case skipped because of issues https://issues.redhat.com/browse/OCPBUGS-18991 and https://issues.redhat.com/browse/OCPBUGS-18955")

		exutil.By("Creating necessary resources to enable OCB functionality")

		defer func() { _ = cleanOCBTestConfigResources(oc.AsAdmin()) }()
		o.Expect(createOCBDefaultTestConfigResourcesFromPullSecret(oc.AsAdmin())).NotTo(o.BeNil(),
			"Error creating the necessary CMs and Secrets to enable OCB functionality")

		logger.Infof("OK!\n")

		exutil.By("Create custom infra MCP")
		infraMcp := NewMachineConfigPool(oc.AsAdmin(), infraMcpName)
		infraMcp.template = generateTemplateAbsolutePath("custom-machine-config-pool.yaml")
		defer infraMcp.delete()
		infraMcp.create()

		logger.Infof("OK!\n")

		exutil.By("Label the infra MCP to enable the OCB functionality")
		infraMcp.EnableOnClusterBuild()
		logger.Infof("OK!\n")

		exutil.By("Check the default builder type")
		o.Eventually(exutil.GetAllPodsWithLabel,
			"5m", "1m").WithArguments(oc.AsAdmin(), MachineConfigNamespace, OCBMachineOsBuilderLabel).ShouldNot(
			o.BeEmpty(),
			"The machine-os-builder pod was not started when the on-cluster-build functionality was enabled")

		mosBuilderPods, err := exutil.GetAllPodsWithLabel(oc.AsAdmin(), MachineConfigNamespace, OCBMachineOsBuilderLabel)
		mosBuilderPod := mosBuilderPods[0]
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the machine-os-builder pod")
		logger.Infof("New machine-os-builder pod: %s", mosBuilderPod)

		o.Eventually(exutil.GetSpecificPodLogs,
			"5m", "1m").WithArguments(oc, MachineConfigNamespace, OCBMachineOsBuilderContainer, mosBuilderPod, "").Should(
			o.ContainSubstring(`imageBuilderType not set, defaulting to "openshift-image-builder"`),
			"The machine-os-builder pod is not reporting the use of the right builder type")
		logger.Infof("OK!\n")

		exutil.By("Check the openshift-image-builder builder type")
		logger.Infof("Set the openshift-image-builder builder type")
		o.Expect(cm.Patch("merge", `{"data":{"imageBuilderType": "openshift-image-builder"}}`)).To(o.Succeed(),
			"Error patching the on-cluster-build configmap")

		logger.Infof("Checking that the machine-os-builder pod is restarted")
		o.Eventually(exutil.GetAllPodsWithLabel,
			"5m", "1m").WithArguments(oc.AsAdmin(), MachineConfigNamespace, OCBMachineOsBuilderLabel).ShouldNot(
			o.ContainElement(mosBuilderPod),
			"The machine-os-builder pod was not restarted after the builder type was reconfigured")

		mosBuilderPods, err = exutil.GetAllPodsWithLabel(oc.AsAdmin(), MachineConfigNamespace, OCBMachineOsBuilderLabel)
		mosBuilderPod = mosBuilderPods[0]
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the machine-os-builder pod")
		logger.Infof("New machine-os-builder pod: %s", mosBuilderPod)

		logger.Infof("Checking that the right builder type is reported in the machine-os-builder logs")

		o.Eventually(exutil.GetSpecificPodLogs,
			"5m", "1m").WithArguments(oc, MachineConfigNamespace, OCBMachineOsBuilderContainer, mosBuilderPod, "").Should(
			o.ContainSubstring(`imageBuilderType set to "openshift-image-builder"`),
			"The machine-os-builder pod is not reporting the use of the right builder type")
		logger.Infof("OK!\n")

		exutil.By("Check the custom-pod-builder builder type")
		logger.Infof("Set the custom-pod-builder builder type")
		o.Expect(cm.Patch("merge", `{"data":{"imageBuilderType": "custom-pod-builder"}}`)).To(o.Succeed(),
			"Error patching the on-cluster-build configmap")

		logger.Infof("Checking that the machine-os-builder pod is restarted")
		o.Eventually(exutil.GetAllPodsWithLabel,
			"5m", "1m").WithArguments(oc.AsAdmin(), MachineConfigNamespace, OCBMachineOsBuilderLabel).ShouldNot(
			o.ContainElement(mosBuilderPod),
			"The machine-os-builder pod was not restarted after the builder type was reconfigured")

		mosBuilderPods, err = exutil.GetAllPodsWithLabel(oc.AsAdmin(), MachineConfigNamespace, OCBMachineOsBuilderLabel)
		mosBuilderPod = mosBuilderPods[0]
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the machine-os-builder pod")
		logger.Infof("New machine-os-builder pod: %s", mosBuilderPod)

		logger.Infof("Checking that the right builder type is reported in the machine-os-builder logs")

		o.Eventually(exutil.GetSpecificPodLogs,
			"5m", "1m").WithArguments(oc, MachineConfigNamespace, OCBMachineOsBuilderContainer, mosBuilderPod, "").Should(
			o.ContainSubstring(`imageBuilderType set to "custom-pod-builder"`),
			"The machine-os-builder pod is not reporting the use of the right builder type")
		logger.Infof("OK!\n")

		exutil.By("Check invalid builder type")
		logger.Infof("Set the invalid builder type")
		o.Expect(cm.Patch("merge", `{"data":{"imageBuilderType": "custom-pod-builder"}}`)).To(o.Succeed(),
			"Error patching the on-cluster-build configmap")

		o.Eventually(MCOperator, "5m", "1m").Should(
			BeDegraded(),
			"The machine-config ClusterOperator should become degraded when an invalid builder type is configured")
		o.Eventually(MCOperator, "5m", "1m").Should(
			HaveDegradedMessage(`invalid image builder type "test-builder", valid types: [custom-pod-builder openshift-image-builder]`),
			"The machine-config ClusterOperator should become degraded when an invalid builder type is configured.\n%s", MCOperator.PrettyString())

		logger.Infof("OK!\n")
	})
})

// OCBConfigMapValues struct that stores the values to configure the OCB functionality in the "on-cluster-build-config" configmap
type OCBConfigMapValues struct {
	imageBuilderType,
	baseImagePullSecretName,
	finalImagePushSecretName,
	finalImagePullspec *string
}

// CheckOCBConfigError validate the OCB configs in TC-66661
func CheckOCBConfigError(oc *exutil.CLI, mcp *MachineConfigPool, ocbConfig *OCBConfigMapValues, removeSecretName, expectedErrorMessage string) {

	MCOperator := NewResource(oc.AsAdmin(), "ClusterOperator", "machine-config")

	if ocbConfig != nil {
		o.Expect(createOCBTestConfigResourcesFromPullSecret(oc, *ocbConfig)).To(o.Succeed(),
			"We could not create the resources for  configmap %v", *ocbConfig)
	}

	// Sometimes we need to remove a secret to check the error when a needed secret is not found
	if removeSecretName != "" {
		logger.Infof("Removing secret %s to force an error", removeSecretName)
		secret := NewSecret(oc.AsAdmin(), MachineConfigNamespace, removeSecretName)
		o.Expect(secret.Delete()).To(o.Succeed(),
			"Error removing secret %s", secret)
	}

	o.Eventually(MCOperator, "5m", "20s").ShouldNot(BeDegraded(),
		"The MachineConfigOperator is already degraded before starting to check the scenario")

	defer func() {
		_ = mcp.DisableOnClusterBuild()
		_ = cleanOCBTestConfigResources(oc.AsAdmin())
	}()

	logger.Infof("Enable OCB functionality in MCP: %s", mcp.GetName())
	mcp.EnableOnClusterBuild()

	logger.Infof("Checking the MCO status")
	o.Eventually(MCOperator, "1m", "20s").Should(BeDegraded(),
		"The MachineConfigOperator resource should be degraded when the current configuration is applied")
	o.Eventually(MCOperator, "1m", "20s").ShouldNot(BeAvailable(),
		"The MachineConfigOperator resource should NOT be available when the current configuration is applied")

	o.Eventually(MCOperator, "1m", "20s").Should(HaveDegradedMessage(o.MatchRegexp(expectedErrorMessage)),
		"The MachineConfigOperator is not reporting the right Degraded message")
	o.Eventually(MCOperator, "1m", "20s").Should(HaveAvailableMessage(o.MatchRegexp(expectedErrorMessage)),
		"The MachineConfigOperator is not reporting the right Available message")
}

func cleanOCBTestConfigResources(oc *exutil.CLI) error {

	var cmErr, pushErr, pullErr, errGetPull, errGetPush error

	logger.Infof("Cleaning OCB test config resources")
	cm := NewNamespacedResource(oc.AsAdmin(), "cm", MachineConfigNamespace, OCBConfigmapName)
	if !cm.Exists() {
		logger.Infof("The configmap %s does not exists. Nothing to clean.", OCBConfigmapName)
		return nil
	}

	logger.Infof("CM: %s\n", cm.PrettyString())

	pullSecretName, errGetPull := cm.Get(`{.data.baseImagePullSecretName}`)
	if errGetPull != nil {
		logger.Infof("Error getting OCB base image pull secret name. ERROR:%v", errGetPull)
	}

	if pullSecretName != "" {
		pullSecret := NewSecret(oc.AsAdmin(), MachineConfigNamespace, pullSecretName)
		if pullSecret.Exists() {
			pullErr := pullSecret.Delete()
			if pullErr != nil {
				logger.Infof("Error deleting %s. ERROR: %v", pullSecret, pullErr)
			}
		} else {
			logger.Infof("OCB base image pull secret name %s does not exist. Nothing to delete", pullSecretName)
		}
	}

	pushSecretName, errGetPush := cm.Get(`{.data.finalImagePushSecretName}`)
	if errGetPush != nil {
		logger.Infof("Error getting OCB final image push secret name. ERROR: %v", errGetPush)
	}

	if pushSecretName != "" {
		pushSecret := NewSecret(oc.AsAdmin(), MachineConfigNamespace, pushSecretName)
		if pullSecretName != pushSecretName {
			if pushSecret.Exists() {
				pushErr := pushSecret.Delete()
				if pushErr != nil {
					logger.Infof("Error deleting push secret %s. ERROR: %v", pushSecret, pushErr)
				}
			} else {
				logger.Infof("OCB final image push secret name %s does not exist. Nothing to delete", pushSecretName)
			}

		} else {
			logger.Infof("Push secret is the same secret as pull secret, so we don't need to delete it")
		}
	}

	cmErr = cm.Delete()
	if cmErr != nil {
		logger.Infof("Error deleting configmap %s. ERROR: %v", cm, cmErr)
	}

	if cmErr != nil || pushErr != nil || pullErr != nil || errGetPull != nil || errGetPush != nil {
		return fmt.Errorf("There were errors while cleaning the OCB test config resources. Please, review the logs")
	}

	return nil
}

func createOCBDefaultTestConfigResourcesFromPullSecret(oc *exutil.CLI) (*OCBConfigMapValues, error) {
	ocbConfig := newTestDefaultsOCBConfig()
	return ocbConfig, createOCBTestConfigResourcesFromPullSecret(oc, *ocbConfig)
}

func createOCBTestConfigResourcesFromPullSecret(oc *exutil.CLI, ocbConfig OCBConfigMapValues) error {

	logger.Infof("Creating OCB test config resources")

	if ocbConfig.baseImagePullSecretName != nil && *ocbConfig.baseImagePullSecretName != "" {
		logger.Infof("Create the OCB base image pull secret")
		err := createOCBSecretFromPullSecret(oc, *ocbConfig.baseImagePullSecretName)
		if err != nil {
			logger.Infof("Error creating the OCB base image pull secret")
			return err
		}
	} else {
		logger.Infof("Skipping baseImagePullSecretName creation. Nil value provided")
	}

	if ocbConfig.finalImagePushSecretName != nil && *ocbConfig.finalImagePushSecretName != "" {
		logger.Infof("Create the OCB final image push secret")
		err := createOCBSecretFromPullSecret(oc, *ocbConfig.finalImagePushSecretName)
		if err != nil {
			logger.Infof("Error creating the OCB final image push secret")
			return err
		}
	} else {
		logger.Infof("Skipping finalImagePushSecretName creation. Nil value provided")
	}

	logger.Infof("Create the OCB on-cluster-build-config configmap")
	err := createOCBConfigMap(oc, ocbConfig)
	if err != nil {
		logger.Infof("Error creating the OCB on-cluster-build-config configmap")
		return err
	}

	return nil
}

func newTestDefaultsOCBConfig() *OCBConfigMapValues {
	return &OCBConfigMapValues{
		baseImagePullSecretName:  PtrStr(OCBDefaultBaseImagePullSecretName),
		finalImagePushSecretName: PtrStr(OCBDefaultFinalImagePushSecretName),
		finalImagePullspec:       PtrStr(DefaultLayeringQuayRepository),
	}
}

func createOCBConfigMap(oc *exutil.CLI, ocbConfig OCBConfigMapValues) error {

	cm := NewNamespacedResource(oc.AsAdmin(), "cm", MachineConfigNamespace, OCBConfigmapName)
	if cm.Exists() {
		logger.Infof("The %s configmap already exists. We proceed to delete it and recreate it.", OCBConfigmapName)
		cm.DeleteOrFail()
	}

	params := []string{cm.GetKind(), cm.GetName(), "-n", cm.GetNamespace()}

	if ocbConfig.finalImagePullspec != nil {
		params = append(params, []string{"--from-literal", "finalImagePullspec=" + *ocbConfig.finalImagePullspec}...)
	}

	if ocbConfig.imageBuilderType != nil {
		params = append(params, []string{"--from-literal", "imageBuilderType=" + *ocbConfig.imageBuilderType}...)
	}

	if ocbConfig.baseImagePullSecretName != nil {
		params = append(params, []string{"--from-literal", "baseImagePullSecretName=" + *ocbConfig.baseImagePullSecretName}...)
	}

	if ocbConfig.finalImagePushSecretName != nil {
		params = append(params, []string{"--from-literal", "finalImagePushSecretName=" + *ocbConfig.finalImagePushSecretName}...)
	}

	return oc.WithoutNamespace().Run("create").Args(params...).Execute()
}

func createOCBSecretFromPullSecret(oc *exutil.CLI, name string) error {
	pullSecret := NewSecret(oc.AsAdmin(), "openshift-config", "pull-secret")
	tmpDir, err := pullSecret.Extract()
	if err != nil {
		return err
	}

	return createOCBSecret(oc, name, tmpDir+"/.dockerconfigjson")
}

func createOCBSecret(oc *exutil.CLI, name, dockerConfigFile string) error {

	if name == "" {
		return fmt.Errorf("Refuse to create a secret with an empty name")
	}

	secret := NewSecret(oc.AsAdmin(), MachineConfigNamespace, name)
	if secret.Exists() {
		logger.Infof("The %s secret already exists. We proceed to delete it and recreate it.", name)
		secret.DeleteOrFail()
	}

	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "-n", MachineConfigNamespace,
		"docker-registry", name, "--from-file=.dockerconfigjson="+dockerConfigFile).Execute()
}
