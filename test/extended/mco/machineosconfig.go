package mco

import (
	"encoding/json"
	"fmt"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

type ContainerFile struct {
	ContainerfileArch string `json:"containerfileArch"` // TODO: CURRENTLY MCO DOES NOT SUPPORT DIFFERENT ARCHITECTURES, BUT IT WILL
	Content           string `json:"content"`
}

// MachineOSConfig resource type declaration
type MachineOSConfig struct {
	Resource
}

// MachineOSConfigList handles list of MachineOSConfig
type MachineOSConfigList struct {
	ResourceList
}

// MachineOSConfig constructor to get MachineOSConfig resource
func NewMachineOSConfig(oc *exutil.CLI, name string) *MachineOSConfig {
	return &MachineOSConfig{Resource: *NewResource(oc, "machineosconfig", name)}
}

// NewMachineOSConfigList construct a new MachineOSConfig list struct to handle all existing MachineOSConfig
func NewMachineOSConfigList(oc *exutil.CLI) *MachineOSConfigList {
	return &MachineOSConfigList{*NewResourceList(oc, "machineosconfig")}
}

// CreateMachineOSConfig creates a MOSC resource using the information provided in the arguments
func CreateMachineOSConfig(oc *exutil.CLI, name, pool, currentImagePullSecret, baseImagePullSecret, renderedImagePushSecret, pushSpec string, containerFile []ContainerFile) (*MachineOSConfig, error) {
	var (
		containerFilesString = "[]"
	)
	logger.Infof("Creating MachineOSConfig %s in pool %s with pullSecret %s pushSecret %s and pushSpec %s", name, pool, baseImagePullSecret, renderedImagePushSecret, pushSpec)
	newMOSC := NewMachineOSConfig(oc, name)

	if len(containerFile) > 0 {
		containerFilesBytes, err := json.Marshal(containerFile)
		if err != nil {
			return newMOSC, err
		}
		containerFilesString = string(containerFilesBytes)
	}
	logger.Infof("Using custom Containerfile %s", containerFilesString)

	err := NewMCOTemplate(oc, "generic-machine-os-config.yaml").Create("-p", "NAME="+name, "POOL="+pool, "CURRENTIMAGEPULLSECRET="+currentImagePullSecret, "BASEIMAGEPULLSECRET="+baseImagePullSecret,
		"RENDEREDIMAGEPUSHSECRET="+renderedImagePushSecret, "PUSHSPEC="+pushSpec, "CONTAINERFILE="+containerFilesString)
	return newMOSC, err
}

func CreateMachineOSConfigUsingInternalRegistry(oc *exutil.CLI, name, pool string, containerFile []ContainerFile) (*MachineOSConfig, error) {
	// We use a copy of the cluster's pull secret to pull the images
	pullSecret := NewSecret(oc.AsAdmin(), "openshift-config", "pull-secret")
	baseImagePullSecret, err := CloneResource(pullSecret, "cloned-pull-secret-"+exutil.GetRandomString(), MachineConfigNamespace, nil)
	if err != nil {
		return NewMachineOSConfig(oc, name), err
	}

	// We use the builder SA secret in MCO to push the images to the internal registry
	saBuilder := NewNamespacedResource(oc, "sa", MachineConfigNamespace, "builder")
	renderedImagePushSecretName := saBuilder.GetOrFail(`{.secrets[0].name}`)
	if renderedImagePushSecretName == "" {
		return NewMachineOSConfig(oc, name), fmt.Errorf("Rendered image push secret cannot have an empty value")
	}

	// We use the default SA secret in MCO to pull the current image from the internal registry
	saDefault := NewNamespacedResource(oc, "sa", MachineConfigNamespace, "default")
	currentImagePullSecret := saDefault.GetOrFail(`{.secrets[0].name}`)
	if currentImagePullSecret == "" {
		return NewMachineOSConfig(oc, name), fmt.Errorf("Current image pull secret cannot have an empty value")
	}

	// We use a push spec stored in the internal registry in the MCO namespace. We use a different image for every pool
	pushSpec := fmt.Sprintf("%s/openshift-machine-config-operator/ocb-%s-image:latest", InternalRegistrySvcURL, pool)

	return CreateMachineOSConfig(oc, name, pool, currentImagePullSecret, baseImagePullSecret.GetName(), renderedImagePushSecretName, pushSpec, containerFile)
}

// GetPullSecret returns the pull secret configured in this MOSC
func (mosc MachineOSConfig) GetPullSecret() (*Secret, error) {
	pullSecretName, err := mosc.Get(`{.spec.buildInputs.baseImagePullSecret.name}`)
	if err != nil {
		return nil, err
	}
	if pullSecretName == "" {
		logger.Warnf("%s has an empty pull secret!! GetPullSecret will return nil", mosc)
		return nil, nil
	}

	return NewSecret(mosc.oc, MachineConfigNamespace, pullSecretName), nil
}

// GetPushSecret returns the push secret configured in this MOSC
func (mosc MachineOSConfig) GetPushSecret() (*Secret, error) {
	pushSecretName, err := mosc.Get(`{.spec.buildInputs.renderedImagePushSecret.name}`)
	if err != nil {
		return nil, err
	}
	if pushSecretName == "" {
		logger.Warnf("%s has an empty push secret!! GetPushSecret will return nil", mosc)
		return nil, nil
	}

	return NewSecret(mosc.oc, MachineConfigNamespace, pushSecretName), nil
}

// CleanupAndDelete removes the secrets in the MachineOSConfig resource and the removes the MachoneOSConfig resource itself
func (mosc MachineOSConfig) CleanupAndDelete() error {
	if !mosc.Exists() {
		logger.Infof("%s does not exist. No need to delete it", mosc)
		return nil
	}

	pullSecret, err := mosc.GetPullSecret()
	if err != nil {
		logger.Errorf("Error getting %s in %s. We continue cleaning.", pullSecret, mosc)
	}
	if pullSecret == nil {
		logger.Infof("Pull secret is empty in %s, skipping pull secret cleanup", mosc)
	} else {
		err := cleanupMOSCSecret(*pullSecret)
		if err != nil {
			logger.Errorf("An error happened cleaning %s in %s. We continue cleaning.\nErr:%s", pullSecret, mosc, err)
		}
	}

	pushSecret, err := mosc.GetPushSecret()
	if err != nil {
		logger.Errorf("Error getting %s in %s. We continue cleaning.", pushSecret, mosc)
	}

	if pushSecret == nil {
		logger.Infof("Push secret is empty in %s, skipping pull secret cleanup", mosc)
	} else {
		cleanupMOSCSecret(*pushSecret)
		if err != nil {
			logger.Errorf("An error happened cleaning %s in %s. We continue cleaning.\nErr:%s", pushSecret, mosc, err)
		}
	}

	return mosc.Delete()
}

// GetMachineConfigPool returns the MachineConfigPool for this MOSC
func (mosc MachineOSConfig) GetMachineConfigPool() (*MachineConfigPool, error) {
	poolName, err := mosc.Get(`{.spec.machineConfigPool.name}`)
	if err != nil {
		return nil, err
	}
	if poolName == "" {
		logger.Errorf("Empty MachineConfigPool configured in %s", mosc)
		return nil, fmt.Errorf("Empty MachineConfigPool configured in %s", mosc)
	}

	return NewMachineConfigPool(mosc.oc, poolName), nil
}

// GetMachineOSBuildList returns a list of all MOSB linked to this MOSC
func (mosc MachineOSConfig) GetMachineOSBuildList() ([]MachineOSBuild, error) {
	mosbList := NewMachineOSBuildList(mosc.GetOC())
	mosbList.SetItemsFilter(fmt.Sprintf(`?(@.spec.machineOSConfig.name=="%s")`, mosc.GetName()))
	return mosbList.GetAll()
}

// GetAll returns a []MachineOSConfig list with all existing pinnedimageset sorted by creation timestamp
func (moscl *MachineOSConfigList) GetAll() ([]MachineOSConfig, error) {
	moscl.ResourceList.SortByTimestamp()
	allMOSCResources, err := moscl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allMOSCs := make([]MachineOSConfig, 0, len(allMOSCResources))

	for _, moscRes := range allMOSCResources {
		allMOSCs = append(allMOSCs, *NewMachineOSConfig(moscl.oc, moscRes.name))
	}

	return allMOSCs, nil
}

// GetAllOrFail returns a []MachineOSConfig list with all existing pinnedimageset sorted by creation time, if any error happens it fails the test
func (moscl *MachineOSConfigList) GetAllOrFail() []MachineOSConfig {
	moscs, err := moscl.GetAll()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error getting the list of existing MachineOSConfig in the cluster")
	return moscs
}

// cleanupMOSCSecret helper function to clean the secrets configured in MachineOSConfig resources
func cleanupMOSCSecret(secret Secret) error {
	if !secret.Exists() {
		logger.Infof("%s does not exist. Not need to delete it.", secret)
		return nil
	}

	hasOwner, err := secret.HasOwner()
	if err != nil {
		logger.Errorf("There was an error looking for the owner of %s. We will not delete it.\nErr:%s", secret, err)
		return err
	}

	if hasOwner {
		logger.Infof("%s is owned by other resources, skipping deletion", secret)
		return nil
	}

	return secret.Delete()
}
