package storage

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

// Define configMap struct
type configMap struct {
	name      string
	namespace string
	template  string
}

type configMapVolumeWithPaths struct {
	Name  string `json:"name"`
	Items []struct {
		Key  string `json:"key"`
		Path string `json:"path"`
	} `json:"items"`
}

// function option mode to change the default values of configMap Object attributes, e.g. name, namespace etc.
type configMapOption func(*configMap)

// Replace the default value of configMap name attribute
func setConfigMapName(name string) configMapOption {
	return func(this *configMap) {
		this.name = name
	}
}

// Replace the default value of configMap template attribute
func setConfigMapTemplate(template string) configMapOption {
	return func(this *configMap) {
		this.template = template
	}
}

// Create a new customized configMap object
func newConfigMap(opts ...configMapOption) configMap {
	defaultConfigMap := configMap{
		name:     "storage-cm-" + getRandomString(),
		template: "configmap-template.yaml",
	}
	for _, o := range opts {
		o(&defaultConfigMap)
	}
	return defaultConfigMap
}

// Create new configMap with customized attributes
func (cm *configMap) create(oc *exutil.CLI) {
	if cm.namespace == "" {
		cm.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cm.template, "-p", "CMNAME="+cm.name, "CMNAMESPACE="+cm.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new configMap with extra parameters
func (cm *configMap) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if cm.namespace == "" {
		cm.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", cm.template, "-p", "CMNAME="+cm.name, "CMNAMESPACE="+cm.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the configMap
func (cm *configMap) delete(oc *exutil.CLI) {
	err := oc.WithoutNamespace().Run("delete").Args("-n", cm.namespace, "cm", cm.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the configMap use kubeadmin
func (cm *configMap) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("-n", cm.namespace, "cm", cm.name, "--ignore-not-found").Execute()
}

// Define SharedConfigMap struct
type sharedConfigMap struct {
	name     string
	refCm    *configMap
	template string
}

// Create a new sharedConfigMap
func (scm *sharedConfigMap) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", scm.template, "-p", "NAME="+scm.name, "REF_CM_NAME="+scm.refCm.name, "REF_CM_NAMESPACE="+scm.refCm.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the sharedConfigMap use kubeadmin
func (scm *sharedConfigMap) deleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("sharedconfigmap", scm.name, "--ignore-not-found").Execute()
}
