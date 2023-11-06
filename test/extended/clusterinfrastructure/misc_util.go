package clusterinfrastructure

import (
	"fmt"
	"strconv"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type defaultMachinesetAzureDescription struct {
	name                 string
	namespace            string
	template             string
	clustername          string
	location             string
	vnet                 string
	subnet               string
	networkResourceGroup string
}

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

func (defaultMachinesetAzure *defaultMachinesetAzureDescription) createDefaultMachineSetOnAzure(oc *exutil.CLI) {
	e2e.Logf("Creating gcpMachineSet ...")
	if err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", defaultMachinesetAzure.template, "-p", "NAME="+defaultMachinesetAzure.name, "NAMESPACE="+machineAPINamespace, "CLUSTERNAME="+defaultMachinesetAzure.clustername, "LOCATION="+defaultMachinesetAzure.location, "VNET="+defaultMachinesetAzure.vnet, "SUBNET="+defaultMachinesetAzure.subnet, "NETWORKRG="+defaultMachinesetAzure.networkResourceGroup); err != nil {
		defaultMachinesetAzure.deleteDefaultMachineSetOnAzure(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		waitForDefaultMachinesRunning(oc, 1, defaultMachinesetAzure.name)
	}
}

func (defaultMachinesetAzure *defaultMachinesetAzureDescription) deleteDefaultMachineSetOnAzure(oc *exutil.CLI) error {
	e2e.Logf("Deleting gcpMachineSet ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(mapiMachineset, defaultMachinesetAzure.name, "-n", machineAPINamespace).Execute()
}

// waitForDefaultMachinesRunning check if all the machines are Running in a MachineSet
func waitForDefaultMachinesRunning(oc *exutil.CLI, machineNumber int, machineSetName string) {
	e2e.Logf("Waiting for the machines Running ...")
	pollErr := wait.Poll(60*time.Second, 960*time.Second, func() (bool, error) {
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machineSetName, "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		machinesRunning, _ := strconv.Atoi(msg)
		if machinesRunning != machineNumber {
			e2e.Logf("Expected %v  machine are not Running yet and waiting up to 1 minutes ...", machineNumber)
			return false, nil
		}
		e2e.Logf("Expected %v  machines are Running", machineNumber)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Expected %v  machines are not Running after waiting up to 16 minutes ...", machineNumber))
	e2e.Logf("All machines are Running ...")
}
