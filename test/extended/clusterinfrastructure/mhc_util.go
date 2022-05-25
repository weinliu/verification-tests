package clusterinfrastructure

import (
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type mhcDescription struct {
	machineset_name string
	clusterid       string
	namespace       string
	maxunhealthy    string
	name            string
	template        string
}

func (mhc *mhcDescription) createMhc(oc *exutil.CLI) {
	e2e.Logf("Creating machine health check ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", mhc.template, "-p", "NAME="+mhc.name, "MAXUNHEALTHY="+mhc.maxunhealthy, "MACHINESET_NAME="+mhc.machineset_name, "CLUSTERID="+mhc.clusterid, "NAMESPACE="+machineAPINamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil {
		e2e.Logf("Please check mhc creation, it has failed")
	}
}

func (mhc *mhcDescription) deleteMhc(oc *exutil.CLI) error {
	e2e.Logf("Deleting machinehealthcheck ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("machinehealthchecks.machine.openshift.io", mhc.name, "-n", mhc.namespace).Execute()
}
