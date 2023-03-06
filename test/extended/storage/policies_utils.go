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
