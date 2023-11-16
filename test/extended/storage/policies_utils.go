package storage

import (
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

// LimitRange struct definition
type LimitRange struct {
	Name            string
	Namespace       string
	Type            string
	Kind            string
	DefaultRequests string
	DefaultLimits   string
	MinRequests     string
	MaxLimits       string
	Template        string
}

// Create creates new LimitRange with customized parameters
func (lr *LimitRange) Create(oc *exutil.CLI) {
	if lr.Namespace == "" {
		lr.Namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", lr.Template, "-p", "LIMITRANGE_NAME="+lr.Name, "LIMITRANGE_NAMESPACE="+lr.Namespace, "LIMIT_TYPE="+lr.Type,
		"LIMIT_KIND="+lr.Kind, "DEFAULT_REQUESTS="+lr.DefaultRequests, "DEFAULT_LIMITS="+lr.DefaultLimits, "MIN_REQUESTS="+lr.MinRequests, "MAX_LIMITS="+lr.MaxLimits)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// DeleteAsAdmin deletes the LimitRange by kubeadmin
func (lr *LimitRange) DeleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("-n", lr.Namespace, "LimitRange/"+lr.Name, "--ignore-not-found").Execute()
}

// ResourceQuota struct definition
type ResourceQuota struct {
	Name             string
	Namespace        string
	Type             string
	HardRequests     string
	HardLimits       string
	PvcLimits        string
	StorageClassName string
	Template         string
}

// Create creates new ResourceQuota with customized parameters
func (rq *ResourceQuota) Create(oc *exutil.CLI) {
	if rq.Namespace == "" {
		rq.Namespace = oc.Namespace()
	}
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", rq.Template, "-p", "RESOURCEQUOTA_NAME="+rq.Name, "RESOURCEQUOTA_NAMESPACE="+rq.Namespace,
		"RESOURCE_TYPE="+rq.Type, "HARD_REQUESTS="+rq.HardRequests, "HARD_LIMITS="+rq.HardLimits, "PVC_LIMITS="+rq.PvcLimits, "STORAGECLASS_NAME="+rq.StorageClassName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// DeleteAsAdmin deletes the ResourceQuota by kubeadmin
func (rq *ResourceQuota) DeleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("-n", rq.Namespace, "ResourceQuota/"+rq.Name, "--ignore-not-found").Execute()
}

// GetValueByJSONPath gets the specified JSONPath value of the ResourceQuota
func (rq *ResourceQuota) GetValueByJSONPath(oc *exutil.CLI, jsonPath string) string {
	output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("-n", rq.Namespace, "ResourceQuota/"+rq.Name, "-o", "jsonpath="+jsonPath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return output
}

// PollGetValueByJSONPath gets the specified JSONPath value of the ResourceQuota satisfy the Eventually check
func (rq *ResourceQuota) PollGetValueByJSONPath(oc *exutil.CLI, jsonPath string) func() string {
	return func() string {
		return rq.GetValueByJSONPath(oc, jsonPath)
	}
}

// PriorityClass struct definition
type PriorityClass struct {
	name        string
	value       string
	description string
	template    string
}

// function option mode to change the default value of PriorityClass parameters,eg. name, value
type priorityClassOption func(*PriorityClass)

// Replace the default value of PriorityClass name parameter
func setPriorityClassName(name string) priorityClassOption {
	return func(this *PriorityClass) {
		this.name = name
	}
}

// Replace the default value of PriorityClass template parameter
func setPriorityClassTemplate(template string) priorityClassOption {
	return func(this *PriorityClass) {
		this.template = template
	}
}

// Replace the default value of PriorityClass Value parameter
func setPriorityClassValue(value string) priorityClassOption {
	return func(this *PriorityClass) {
		this.value = value
	}
}

// Replace the default value of PriorityClass Description parameter
func setPriorityClassDescription(description string) priorityClassOption {
	return func(this *PriorityClass) {
		this.description = description
	}
}

// Creates new PriorityClass with customized parameters
func (pc *PriorityClass) Create(oc *exutil.CLI) {
	err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", pc.template, "-p", "PRIORITYCLASS_NAME="+pc.name,
		"VALUE="+pc.value, "DESCRIPTION="+pc.description)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// DeleteAsAdmin deletes the PriorityClass by kubeadmin
func (pc *PriorityClass) DeleteAsAdmin(oc *exutil.CLI) {
	oc.WithoutNamespace().AsAdmin().Run("delete").Args("PriorityClass/"+pc.name, "--ignore-not-found").Execute()
}

// Create a new customized PriorityClass object
func newPriorityClass(opts ...priorityClassOption) PriorityClass {
	defaultPriorityClass := PriorityClass{
		name:        "my-priorityclass-" + getRandomString(),
		template:    "priorityClass-template.yaml",
		value:       "1000000000",
		description: "Custom priority class",
	}

	for _, o := range opts {
		o(&defaultPriorityClass)
	}

	return defaultPriorityClass
}
