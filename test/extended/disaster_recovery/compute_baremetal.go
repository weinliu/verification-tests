package disasterrecovery

import (
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type baremetalIPIInstance struct {
	instance
}

func newBaremetalIPIInstance(oc *exutil.CLI, nodeName string) *baremetalIPIInstance {
	return &baremetalIPIInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
	}
}

// GetBaremetalMasterNodes get master nodes.
func GetBaremetalMasterNodes(oc *exutil.CLI) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, "master")
	o.Expect(err).NotTo(o.HaveOccurred())
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newBaremetalIPIInstance(oc, nodeName))
	}
	return results, nil
}

func (ipi *baremetalIPIInstance) GetInstanceID() (string, error) {
	var masterNodeMachineConfig string
	bmhOutput, bmhErr := ipi.oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", "openshift-machine-api", "-o", `jsonpath='{.items[*].metadata.name}'`).Output()
	o.Expect(bmhErr).NotTo(o.HaveOccurred())
	machineConfigOutput := strings.Fields(bmhOutput)
	for i := 0; i < len(machineConfigOutput); i++ {
		if strings.Contains(machineConfigOutput[i], ipi.nodeName) {
			masterNodeMachineConfig = strings.ReplaceAll(machineConfigOutput[i], "'", "")
		}
	}
	return masterNodeMachineConfig, bmhErr
}

func (ipi *baremetalIPIInstance) Start() error {
	errVM := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		vmInstance, err := GetBmhNodeMachineConfig(ipi.oc, ipi.nodeName)
		if err != nil {
			return false, nil
		}
		patch := `[{"op": "replace", "path": "/spec/online", "value": true}]`
		startErr := ipi.oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", "openshift-machine-api", vmInstance, "--type=json", "-p", patch).Execute()
		if startErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVM, fmt.Sprintf("Not able to start %s", ipi.nodeName))
	return errVM
}

func (ipi *baremetalIPIInstance) Stop() error {
	errVM := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		vmInstance, err := GetBmhNodeMachineConfig(ipi.oc, ipi.nodeName)
		if err != nil {
			return false, nil
		}
		patch := `[{"op": "replace", "path": "/spec/online", "value": false}]`
		stopErr := ipi.oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", "openshift-machine-api", vmInstance, "--type=json", "-p", patch).Execute()
		if stopErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVM, fmt.Sprintf("Not able to stop %s", ipi.nodeName))
	return errVM
}

func (ipi *baremetalIPIInstance) State() (string, error) {
	nodeStatus, statusErr := ipi.oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", ipi.nodeName, "-o", `jsonpath={.status.conditions[3].type}`).Output()
	o.Expect(statusErr).NotTo(o.HaveOccurred())
	return strings.ToLower(nodeStatus), statusErr
}

// GetBmhNodeMachineConfig get bmh machineconfig name
func GetBmhNodeMachineConfig(oc *exutil.CLI, vmInstance string) (string, error) {
	var masterNodeMachineConfig string
	bmhOutput, bmhErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", "openshift-machine-api", "-o", `jsonpath='{.items[*].metadata.name}'`).Output()
	o.Expect(bmhErr).NotTo(o.HaveOccurred())
	machineConfigOutput := strings.Fields(bmhOutput)
	for i := 0; i < len(machineConfigOutput); i++ {
		if strings.Contains(machineConfigOutput[i], vmInstance) {
			masterNodeMachineConfig = strings.ReplaceAll(machineConfigOutput[i], "'", "")
		}
	}
	return masterNodeMachineConfig, bmhErr
}
