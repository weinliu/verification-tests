package mco

import (
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// ContainerRuntimeConfig struct is used to handle ContainerRuntimeConfig resources in OCP
type ContainerRuntimeConfig struct {
	*Resource
	template string
}

// ContainerRuntimeConfigList handles list of nodes
type ContainerRuntimeConfigList struct {
	ResourceList
}

// NewContainerRuntimeConfig creates a ContainerRuntimeConfig struct
func NewContainerRuntimeConfig(oc *exutil.CLI, name string, template string) *ContainerRuntimeConfig {
	return &ContainerRuntimeConfig{Resource: NewResource(oc, "ContainerRuntimeConfig", name), template: template}
}

// NewContainerRuntimeConfigList create a NewKubeletConfigList struct
func NewContainerRuntimeConfigList(oc *exutil.CLI) *ContainerRuntimeConfigList {
	return &ContainerRuntimeConfigList{*NewResourceList(oc, "ContainerRuntimeConfig")}
}

func (cr *ContainerRuntimeConfig) create() {
	exutil.CreateClusterResourceFromTemplate(cr.oc, "--ignore-unknown-parameters=true", "-f", cr.template, "-p", "NAME="+cr.name)
}

func (cr ContainerRuntimeConfig) waitUntilSuccess(timeout string) {
	e2e.Logf("wait for %s to report success", cr.name)
	o.Eventually(func() map[string]interface{} {
		successCond := JSON(cr.GetConditionByType("Success"))
		if successCond.Exists() {
			return successCond.ToMap()
		}
		e2e.Logf("success condition not found, conditions are %s", cr.GetOrFail(`{.status.conditions}`))
		return nil
	},
		timeout, "2s").Should(o.SatisfyAll(o.HaveKeyWithValue("status", "True"),
		o.HaveKeyWithValue("message", "Success")))
}

func (cr ContainerRuntimeConfig) waitUntilFailure(expectedMsg string, timeout string) {
	e2e.Logf("wait for %s to report failure", cr.name)
	o.Eventually(func() map[string]interface{} {
		failureCond := JSON(cr.GetConditionByType("Failure"))
		if failureCond.Exists() {
			return failureCond.ToMap()
		}
		e2e.Logf("Failure condition not found, conditions are %s", cr.GetOrFail(`{.status.conditions}`))
		return nil
	},
		timeout, "2s").Should(o.SatisfyAll(o.HaveKeyWithValue("status", "False"), o.HaveKeyWithValue("message", o.ContainSubstring(expectedMsg))))
}
