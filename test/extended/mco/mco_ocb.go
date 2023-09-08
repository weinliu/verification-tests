package mco

import (
	"fmt"

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
	})

	g.It("Author:sregidor-NonPreRelease-High-66567-OCB Wiring up Productionalized Build Controller [Disruptive]", func() {

		infraMcpName := "infra"

		exutil.By("Creating necessary resources to enable OCB functionality")

		defer func() { _ = cleanOCBTestConfigResources(oc.AsAdmin()) }()
		o.Expect(createOCBDefaultTestConfigResources(oc.AsAdmin())).NotTo(o.BeNil(),
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
})

// OCBConfigMapValues struct that stores the values to configure the OCB functionality in the "on-cluster-build-config" configmap
type OCBConfigMapValues struct {
	imageBuilderType,
	baseImagePullSecretName,
	finalImagePushSecretName,
	finalImagePullspec string
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

	pullSecret := NewSecret(oc.AsAdmin(), MachineConfigNamespace, pullSecretName)
	if pullSecret.Exists() {
		pullErr := pullSecret.Delete()
		if pullErr != nil {
			logger.Infof("Error deleting %s. ERROR: %v", pullSecret, pullErr)
		}
	} else {
		logger.Infof("OCB base image pull secret name %s does not exist. Nothing to delete", pullSecretName)
	}

	pushSecretName, errGetPush := cm.Get(`{.data.finalImagePushSecretName}`)
	if errGetPush != nil {
		logger.Infof("Error getting OCB final image push secret name. ERROR: %v", errGetPush)
	}

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

	cmErr = cm.Delete()
	if cmErr != nil {
		logger.Infof("Error deleting configmap %s. ERROR: %v", cm, cmErr)
	}

	if cmErr != nil || pushErr != nil || pullErr != nil || errGetPull != nil || errGetPush != nil {
		return fmt.Errorf("There were errors while cleaning the OCB test config resources. Please, review the logs")
	}

	return nil
}

func createOCBDefaultTestConfigResources(oc *exutil.CLI) (*OCBConfigMapValues, error) {

	ocbConfig := newTestDefaultsOCBConfig()

	logger.Infof("Create the OCB base image pull secret")
	err := createOCBSecretFromPullSecret(oc, ocbConfig.baseImagePullSecretName)
	if err != nil {
		logger.Infof("Error creating the OCB base image pull secret")
		return nil, err
	}

	logger.Infof("Create the OCB final image push secret")
	err = createOCBSecretFromPullSecret(oc, ocbConfig.finalImagePushSecretName)
	if err != nil {
		logger.Infof("Error creating the OCB final image push secret")
		return nil, err
	}

	logger.Infof("Create the OCB on-cluster-build-config configmap")
	err = createOCBConfigMap(oc, ocbConfig)
	if err != nil {
		logger.Infof("Error creating the OCB on-cluster-build-config configmap")
		return nil, err
	}

	return ocbConfig, nil
}

func newTestDefaultsOCBConfig() *OCBConfigMapValues {
	return &OCBConfigMapValues{
		baseImagePullSecretName:  OCBDefaultBaseImagePullSecretName,
		finalImagePushSecretName: OCBDefaultFinalImagePushSecretName,
		finalImagePullspec:       DefaultLayeringQuayRepository,
	}
}

func createOCBConfigMap(oc *exutil.CLI, ocbConfig *OCBConfigMapValues) error {

	cm := NewNamespacedResource(oc.AsAdmin(), "cm", MachineConfigNamespace, OCBConfigmapName)
	if cm.Exists() {
		logger.Infof("The %s configmap already exists. We proceed to delete it and recreate it.", OCBConfigmapName)
		cm.DeleteOrFail()
	}

	template := NewMCOTemplate(oc.AsAdmin(), "generic-on-cluster-build-cm.yaml")

	params := []string{"-p", "NAME=" + OCBConfigmapName, "-p", "NAMESPACE=" + MachineConfigNamespace, "-p", "FINAL_IMAGE_PULL_SPEC=" + ocbConfig.finalImagePullspec}

	if ocbConfig.imageBuilderType != "" {
		params = append(params, []string{"-p", "IMAGE_BUILDER_TYPE=" + ocbConfig.imageBuilderType}...)
	}

	if ocbConfig.baseImagePullSecretName != "" {
		params = append(params, []string{"-p", "BASE_IMAGE_PULL_SECRET_NAME=" + ocbConfig.baseImagePullSecretName}...)
	}

	if ocbConfig.finalImagePushSecretName != "" {
		params = append(params, []string{"-p", "FINAL_IMAGE_PUSH_SECRET_NAME=" + ocbConfig.finalImagePushSecretName}...)
	}

	return template.Create(params...)
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

	secret := NewSecret(oc.AsAdmin(), MachineConfigNamespace, name)
	if secret.Exists() {
		logger.Infof("The %s secret already exists. We proceed to delete it and recreate it.", name)
		secret.DeleteOrFail()
	}

	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "-n", MachineConfigNamespace,
		"docker-registry", name, "--from-file=.dockerconfigjson="+dockerConfigFile).Execute()
}
