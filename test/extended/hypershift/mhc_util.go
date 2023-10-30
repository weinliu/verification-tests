package hypershift

import (
	o "github.com/onsi/gomega"
	"os"
	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// utils for the openshift api machines "machinesets.machine.openshift.io" and "machines.machine.openshift.io"

type mhcDescription struct {
	MachinesetName string `json:"MACHINESET_NAME"`
	Clusterid      string `json:"CLUSTERID"`
	Namespace      string `json:"NAMESPACE"`
	Maxunhealthy   string `json:"MAXUNHEALTHY"`
	Name           string `json:"NAME"`
	template       string
}

func (mhc *mhcDescription) createMhc(oc *exutil.CLI, parsedTemplate string) {
	e2e.Logf("Creating machine health check ...")
	vars, err := parseTemplateVarParams(mhc)
	o.Expect(err).NotTo(o.HaveOccurred())

	params := append([]string{"--ignore-unknown-parameters=true", "-f", mhc.template, "-p"}, vars...)
	err = applyResourceFromTemplate(oc, "", parsedTemplate, params...)
	if err != nil {
		e2e.Logf("failed to create machine health check %s", err.Error())
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (mhc *mhcDescription) deleteMhc(oc *exutil.CLI, parsedTemplate string) {
	e2e.Logf("Deleting machinehealthcheck ...")
	defer func() {
		path := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+parsedTemplate)
		os.Remove(path)
	}()
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(mapiMHC, mhc.Name, "--ignore-not-found", "-n", mhc.Namespace).Execute()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func checkMachinesetReplicaStatus(oc *exutil.CLI, machinesetName string) bool {
	desired := doOcpReq(oc, OcpGet, false, "-n", machineAPINamespace, mapiMachineset, machinesetName, `-o=jsonpath={.spec.replicas}`)
	ready := doOcpReq(oc, OcpGet, false, "-n", machineAPINamespace, mapiMachineset, machinesetName, `-o=jsonpath={.status.readyReplicas}`)
	available := doOcpReq(oc, OcpGet, false, "-n", machineAPINamespace, mapiMachineset, machinesetName, `-o=jsonpath={.status.availableReplicas}`)

	e2e.Logf("%s %s desired: %s ready: %s and available: %s", mapiMachineset, machinesetName, desired, ready, available)
	return desired == ready && ready == available
}
