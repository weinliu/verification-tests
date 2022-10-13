package mco

import (
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

// KubeletConfig struct is used to handle KubeletConfig resources in OCP
type KubeletConfig struct {
	Resource
	template string
}

// KubeletConfigList handles list of nodes
type KubeletConfigList struct {
	ResourceList
}

// NewKubeletConfig create a NewKubeletConfig struct
func NewKubeletConfig(oc *exutil.CLI, name, template string) *KubeletConfig {
	return &KubeletConfig{Resource: *NewResource(oc, "KubeletConfig", name), template: template}
}

// NewKubeletConfigList create a NewKubeletConfigList struct
func NewKubeletConfigList(oc *exutil.CLI) *KubeletConfigList {
	return &KubeletConfigList{*NewResourceList(oc, "KubeletConfig")}
}

func (kc *KubeletConfig) create() {
	exutil.CreateClusterResourceFromTemplate(kc.oc, "--ignore-unknown-parameters=true", "-f", kc.template, "-p", "NAME="+kc.name)
}

func (kc KubeletConfig) waitUntilSuccess(timeout string) {
	logger.Infof("wait for %s to report success", kc.name)
	o.Eventually(func() map[string]interface{} {
		successCond := JSON(kc.GetConditionByType("Success"))
		if successCond.Exists() {
			return successCond.ToMap()
		}
		logger.Infof("success condition not found, conditions are %s", kc.GetOrFail(`{.status.conditions}`))
		return nil
	},
		timeout, "2s").Should(o.SatisfyAll(o.HaveKeyWithValue("status", "True"),
		o.HaveKeyWithValue("message", "Success")),
		"KubeletConfig '%s' should report Success in status.conditions, but the current status is not success", kc.GetName())
}

func (kc KubeletConfig) waitUntilFailure(expectedMsg, timeout string) {

	logger.Infof("wait for %s to report failure", kc.name)
	o.Eventually(func() map[string]interface{} {
		failureCond := JSON(kc.GetConditionByType("Failure"))
		if failureCond.Exists() {
			return failureCond.ToMap()
		}
		logger.Infof("Failure condition not found, conditions are %s", kc.GetOrFail(`{.status.conditions}`))
		return nil
	},
		timeout, "2s").Should(o.SatisfyAll(o.HaveKeyWithValue("status", "False"), o.HaveKeyWithValue("message", o.ContainSubstring(expectedMsg))),
		"KubeletConfig '%s' should report Failure in status.conditions and report failure message %s. But it doesnt.", kc.GetName(), expectedMsg)
}
