package clusterinfrastructure

import (
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type pvcDescription struct {
	storageSize string
	template    string
}

func (pvc *pvcDescription) createPvc(oc *exutil.CLI) {
	e2e.Logf("Creating pvc ...")
	exutil.CreateNsResourceFromTemplate(oc, "openshift-machine-api", "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "STORAGESIZE="+pvc.storageSize)
}

func (pvc *pvcDescription) deletePvc(oc *exutil.CLI) error {
	e2e.Logf("Deleting pvc ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", "pvc-cloud", "-n", "openshift-machine-api").Execute()
}
