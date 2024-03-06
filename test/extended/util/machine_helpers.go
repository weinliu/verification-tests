package util

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	MachineAPINamespace = "openshift-machine-api"
	//MapiMachineset means the fullname of mapi machineset
	MapiMachineset = "machinesets.machine.openshift.io"
	//MapiMachine means the fullname of mapi machine
	MapiMachine = "machines.machine.openshift.io"
	//MapiMHC means the fullname of mapi machinehealthcheck
	MapiMHC = "machinehealthchecks.machine.openshift.io"
	//VsphereServer vSphere server hostname
	VsphereServer = "vcenter.sddc-44-236-21-251.vmwarevmc.com"
)

// MachineSetDescription define fields to create machineset
type MachineSetDescription struct {
	Name     string
	Replicas int
}

// CreateMachineSet create a new machineset
func (ms *MachineSetDescription) CreateMachineSet(oc *CLI) {
	e2e.Logf("Creating a new MachineSets ...")
	machinesetName := GetRandomMachineSetName(oc)
	machineSetJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, machinesetName, "-n", MachineAPINamespace, "-o=json").OutputToFile("machineset.json")
	o.Expect(err).NotTo(o.HaveOccurred())

	bytes, _ := ioutil.ReadFile(machineSetJSON)
	value1, _ := sjson.Set(string(bytes), "metadata.name", ms.Name)
	value2, _ := sjson.Set(value1, "spec.selector.matchLabels.machine\\.openshift\\.io/cluster-api-machineset", ms.Name)
	value3, _ := sjson.Set(value2, "spec.template.metadata.labels.machine\\.openshift\\.io/cluster-api-machineset", ms.Name)
	value4, _ := sjson.Set(value3, "spec.replicas", ms.Replicas)
	// Adding taints to machineset so that pods without toleration can not schedule to the nodes we provision
	value5, _ := sjson.Set(value4, "spec.template.spec.taints.0", map[string]interface{}{"effect": "NoSchedule", "key": "mapi", "value": "mapi_test"})
	err = ioutil.WriteFile(machineSetJSON, []byte(value5), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())

	if err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", machineSetJSON).Execute(); err != nil {
		ms.DeleteMachineSet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		WaitForMachinesRunning(oc, ms.Replicas, ms.Name)
	}
}

// CreateMachineSetByArch create a new machineset by arch
func (ms *MachineSetDescription) CreateMachineSetByArch(oc *CLI, arch string) {
	e2e.Logf("Creating a new MachineSets ...")
	machinesetName := GetRandomMachineSetNameByArch(oc, arch)
	machineSetJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, machinesetName, "-n", MachineAPINamespace, "-o=json").OutputToFile("machineset.json")
	o.Expect(err).NotTo(o.HaveOccurred())

	bytes, _ := ioutil.ReadFile(machineSetJSON)
	machinesetjsonWithName, _ := sjson.Set(string(bytes), "metadata.name", ms.Name)
	machinesetjsonWithSelector, _ := sjson.Set(machinesetjsonWithName, "spec.selector.matchLabels.machine\\.openshift\\.io/cluster-api-machineset", ms.Name)
	machinesetjsonWithTemplateLabel, _ := sjson.Set(machinesetjsonWithSelector, "spec.template.metadata.labels.machine\\.openshift\\.io/cluster-api-machineset", ms.Name)
	machinesetjsonWithReplicas, _ := sjson.Set(machinesetjsonWithTemplateLabel, "spec.replicas", ms.Replicas)
	// Adding taints to machineset so that pods without toleration can not schedule to the nodes we provision
	machinesetjsonWithTaints, _ := sjson.Set(machinesetjsonWithReplicas, "spec.template.spec.taints.0", map[string]interface{}{"effect": "NoSchedule", "key": "mapi", "value": "mapi_test"})
	err = ioutil.WriteFile(machineSetJSON, []byte(machinesetjsonWithTaints), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())

	if err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", machineSetJSON).Execute(); err != nil {
		ms.DeleteMachineSet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		WaitForMachinesRunning(oc, ms.Replicas, ms.Name)
	}
}

// DeleteMachineSet delete a machineset
func (ms *MachineSetDescription) DeleteMachineSet(oc *CLI) error {
	e2e.Logf("Deleting a MachineSets ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(MapiMachineset, ms.Name, "-n", MachineAPINamespace).Execute()
}

// ListAllMachineNames list all machines
func ListAllMachineNames(oc *CLI) []string {
	e2e.Logf("Listing all Machines ...")
	machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineNames, " ")
}

// ListWorkerMachineSetNamesByArch list all linux worker machineSets by arch
func ListWorkerMachineSetNamesByArch(oc *CLI, arch string) []string {
	e2e.Logf("Listing all MachineSets by arch ...")
	machineSetNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if machineSetNames == "" {
		g.Skip("Skip this test scenario because there are no machinesets in this cluster")
	}
	workerMachineSetNames := strings.Split(machineSetNames, " ")
	var linuxWorkerMachineSetNames []string
	for _, workerMachineSetName := range workerMachineSetNames {
		machineSetLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, workerMachineSetName, "-o=jsonpath={.spec.template.metadata.labels}", "-n", MachineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		machineSetAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, workerMachineSetName, "-o=jsonpath={.metadata.annotations.capacity\\.cluster-autoscaler\\.kubernetes\\.io/labels}", "-n", MachineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(machineSetAnnotation, "kubernetes.io/arch="+arch) && !strings.Contains(machineSetLabels, `"machine.openshift.io/os-id":"Windows"`) {
			linuxWorkerMachineSetNames = append(linuxWorkerMachineSetNames, workerMachineSetName)
		}
	}
	e2e.Logf("linuxWorkerMachineSetNames: %s", linuxWorkerMachineSetNames)
	return linuxWorkerMachineSetNames
}

// ListWorkerMachineSetNames list all linux worker machineSets
func ListWorkerMachineSetNames(oc *CLI) []string {
	e2e.Logf("Listing all MachineSets ...")
	machineSetNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if machineSetNames == "" {
		g.Skip("Skip this test scenario because there are no machinesets in this cluster")
	}
	workerMachineSetNames := strings.Split(machineSetNames, " ")
	var linuxWorkerMachineSetNames []string
	for _, workerMachineSetName := range workerMachineSetNames {
		machineSetLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, workerMachineSetName, "-o=jsonpath={.spec.template.metadata.labels}", "-n", MachineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(machineSetLabels, `"machine.openshift.io/os-id":"Windows"`) {
			linuxWorkerMachineSetNames = append(linuxWorkerMachineSetNames, workerMachineSetName)
		}
	}
	e2e.Logf("linuxWorkerMachineSetNames: %s", linuxWorkerMachineSetNames)
	return linuxWorkerMachineSetNames
}

// ListWorkerMachineNames list all worker machines
func ListWorkerMachineNames(oc *CLI) []string {
	e2e.Logf("Listing all Machines ...")
	machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-o=jsonpath={.items[*].metadata.name}", "-l", "machine.openshift.io/cluster-api-machine-type=worker", "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineNames, " ")
}

// ListMasterMachineNames list all master machines
func ListMasterMachineNames(oc *CLI) []string {
	e2e.Logf("Listing all Machines ...")
	machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-o=jsonpath={.items[*].metadata.name}", "-l", "machine.openshift.io/cluster-api-machine-type=master", "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineNames, " ")
}

// ListNonOutpostWorkerNodes lists all public nodes in the aws outposts mixed cluster
func ListNonOutpostWorkerNodes(oc *CLI) []string {
	e2e.Logf("Listing all regular nodes ...")
	var nodeNames []string
	var regularNodes []string
	nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if nodes == "" {
		g.Skip("Skip this test scenario because there are no worker nodes in this cluster")
	}
	nodeNames = strings.Split(nodes, " ")
	for _, node := range nodeNames {
		nodeLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(nodeLabels, "topology.ebs.csi.aws.com/outpost-id") {
			regularNodes = append(regularNodes, node)
		}
	}
	return regularNodes
}

// GetMachineNamesFromMachineSet get all Machines in a Machineset
func GetMachineNamesFromMachineSet(oc *CLI, machineSetName string) []string {
	e2e.Logf("Getting all Machines in a Machineset ...")
	machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-o=jsonpath={.items[*].metadata.name}", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineNames, " ")
}

// GetNodeNamesFromMachineSet get all Nodes in a Machineset
func GetNodeNamesFromMachineSet(oc *CLI, machineSetName string) []string {
	e2e.Logf("Getting all Nodes in a Machineset ...")
	nodeNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-o=jsonpath={.items[*].status.nodeRef.name}", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(nodeNames, " ")
}

// GetNodeNameFromMachine get node name for a machine
func GetNodeNameFromMachine(oc *CLI, machineName string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, machineName, "-o=jsonpath={.status.nodeRef.name}", "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return nodeName
}

// GetRandomMachineSetName get a random RHCOS MachineSet name, if it's aws outpost cluster, return a outpost machineset
func GetRandomMachineSetName(oc *CLI) string {
	e2e.Logf("Getting a random MachineSet ...")
	if IsAwsOutpostCluster(oc) {
		return GetOneOutpostMachineSet(oc)
	}
	allMachineSetNames := ListWorkerMachineSetNames(oc)
	var filteredMachineSetNames []string

	// Filter out MachineSet names containing 'rhel'
	for _, name := range allMachineSetNames {
		if !strings.Contains(name, "rhel") {
			filteredMachineSetNames = append(filteredMachineSetNames, name)
		}
	}

	// Check if there are any machine sets left after filtering
	if len(filteredMachineSetNames) == 0 {
		g.Skip("Skip this test scenario because there are no suitable machinesets in this cluster to copy")
	}

	// Return a random MachineSet name from the filtered list
	return filteredMachineSetNames[rand.Int31n(int32(len(filteredMachineSetNames)))]
}

// GetRandomMachineSetNameByArch get a random MachineSet name by arch
func GetRandomMachineSetNameByArch(oc *CLI, arch string) string {
	e2e.Logf("Getting a random MachineSet by arch ...")
	machinesetNames := ListWorkerMachineSetNamesByArch(oc, arch)
	if len(machinesetNames) == 0 {
		g.Skip(fmt.Sprintf("Skip this test scenario because there are no linux/%s machinesets in this cluster", arch))
	}
	return machinesetNames[rand.Int31n(int32(len(machinesetNames)))]
}

// GetMachineSetReplicas get MachineSet replicas
func GetMachineSetReplicas(oc *CLI, machineSetName string) int {
	e2e.Logf("Getting MachineSets replicas ...")
	replicasVal, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, machineSetName, "-o=jsonpath={.spec.replicas}", "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	replicas, _ := strconv.Atoi(replicasVal)
	return replicas
}

// ScaleMachineSet scale a MachineSet by replicas
func ScaleMachineSet(oc *CLI, machineSetName string, replicas int) {
	e2e.Logf("Scaling MachineSets ...")
	_, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas="+strconv.Itoa(replicas), MapiMachineset, machineSetName, "-n", MachineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	WaitForMachinesRunning(oc, replicas, machineSetName)
}

// DeleteMachine delete a machine
func DeleteMachine(oc *CLI, machineName string) error {
	e2e.Logf("Deleting Machine ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(MapiMachine, machineName, "-n", MachineAPINamespace).Execute()
}

// WaitForMachinesRunning check if all the machines are Running in a MachineSet
func WaitForMachinesRunning(oc *CLI, machineNumber int, machineSetName string) {
	e2e.Logf("Waiting for the machines Running ...")
	pollErr := wait.Poll(60*time.Second, 960*time.Second, func() (bool, error) {
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, machineSetName, "-o=jsonpath={.status.readyReplicas}", "-n", MachineAPINamespace).Output()
		machinesRunning, _ := strconv.Atoi(msg)
		if machinesRunning != machineNumber {
			phase, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=jsonpath={.items[*].status.phase}").Output()
			if strings.Contains(phase, "Failed") {
				output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=yaml").Output()
				e2e.Logf("%v", output)
				return false, fmt.Errorf("Some machine go into Failed phase!")
			}
			if strings.Contains(phase, "Provisioning") {
				output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=yaml").Output()
				if strings.Contains(output, "InsufficientInstanceCapacity") {
					e2e.Logf("%v", output)
					return false, fmt.Errorf("InsufficientInstanceCapacity")
				}
			}
			e2e.Logf("Expected %v  machine are not Running yet and waiting up to 1 minutes ...", machineNumber)
			return false, nil
		}
		e2e.Logf("Expected %v  machines are Running", machineNumber)
		return true, nil
	})
	if pollErr != nil {
		if pollErr.Error() == "InsufficientInstanceCapacity" {
			g.Skip("InsufficientInstanceCapacity, skip this test")
		}
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=yaml").Output()
		e2e.Logf("%v", output)
		e2e.Failf("Expected %v  machines are not Running after waiting up to 16 minutes ...", machineNumber)
	}
	e2e.Logf("All machines are Running ...")
}

// WaitForMachineFailed check if all the machines are Failed in a MachineSet
func WaitForMachineFailed(oc *CLI, machineSetName string) {
	e2e.Logf("Wait for machine to go into Failed phase")
	err := wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=jsonpath={.items[0].status.phase}").Output()
		if output != "Failed" {
			e2e.Logf("machine is not in Failed phase and waiting up to 30 seconds ...")
			return false, nil
		}
		e2e.Logf("machine is in Failed phase")
		return true, nil
	})
	AssertWaitPollNoErr(err, "Check machine phase failed")
}

// WaitForMachineProvisioned check if all the machines are Provisioned in a MachineSet
func WaitForMachineProvisioned(oc *CLI, machineSetName string) {
	e2e.Logf("Wait for machine to go into Provisioned phase")
	err := wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=jsonpath={.items[0].status.phase}").Output()
		if output != "Provisioned" {
			e2e.Logf("machine is not in Provisioned phase and waiting up to 60 seconds ...")
			return false, nil
		}
		e2e.Logf("machine is in Provisioned phase")
		return true, nil
	})
	AssertWaitPollNoErr(err, "Check machine phase failed")
}

// WaitForMachinesDisapper check if all the machines are Dissappered in a MachineSet
func WaitForMachinesDisapper(oc *CLI, machineSetName string) {
	e2e.Logf("Waiting for the machines Dissapper ...")
	err := wait.Poll(60*time.Second, 1200*time.Second, func() (bool, error) {
		machineNames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-o=jsonpath={.items[*].metadata.name}", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-n", MachineAPINamespace).Output()
		if machineNames != "" {
			e2e.Logf(" Still have machines are not Disappered yet and waiting up to 1 minutes ...")
			return false, nil
		}
		e2e.Logf("All machines are Disappered")
		return true, nil
	})
	AssertWaitPollNoErr(err, "Wait machine disappear failed.")
}

// WaitForMachinesRunningByLabel check if all the machines with the specific labels are Running
func WaitForMachinesRunningByLabel(oc *CLI, machineNumber int, labels string) []string {
	e2e.Logf("Waiting for the machines Running ...")
	err := wait.Poll(60*time.Second, 960*time.Second, func() (bool, error) {
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-l", labels, "-o=jsonpath={.items[*].status.phase}", "-n", MachineAPINamespace).Output()
		machinesRunning := strings.Count(msg, "Running")
		if machinesRunning == machineNumber {
			e2e.Logf("Expected %v machines are Running", machineNumber)
			return true, nil
		}
		e2e.Logf("Expected %v machine are not Running yet and waiting up to 1 minutes ...", machineNumber)
		return false, nil
	})
	AssertWaitPollNoErr(err, "Wait machine running failed.")
	msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-l", labels, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
	return strings.Split(msg, " ")
}

// WaitForMachineRunningByField check if the machine is Running by field
func WaitForMachineRunningByField(oc *CLI, field string, fieldValue string, labels string) string {
	e2e.Logf("Waiting for the machine Running ...")
	var newMachineName string
	err := wait.Poll(60*time.Second, 960*time.Second, func() (bool, error) {
		msg, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-l", labels, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
		if err2 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ...")
			return false, nil
		}
		for _, machineName := range strings.Split(msg, " ") {
			phase, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, machineName, "-o=jsonpath={.status.phase}", "-n", MachineAPINamespace).Output()
			machineFieldValue, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, machineName, field, "-n", MachineAPINamespace).Output()
			if phase == "Running" && machineFieldValue == fieldValue {
				e2e.Logf("The machine with field %s = %s is Running %s", field, fieldValue, machineName)
				newMachineName = machineName
				return true, nil
			}
		}
		e2e.Logf("The machine with field %s = %s is not Running and waiting up to 1 minutes ...", field, fieldValue)
		return false, nil
	})
	AssertWaitPollNoErr(err, "Wait machine Running failed.")
	return newMachineName
}

// WaitForMachineRunningBySuffix check if the machine is Running by suffix
func WaitForMachineRunningBySuffix(oc *CLI, machineNameSuffix string, labels string) string {
	e2e.Logf("Waiting for the machine Running ...")
	var newMachineName string
	err := wait.Poll(60*time.Second, 960*time.Second, func() (bool, error) {
		msg, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-l", labels, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
		if err2 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ...")
			return false, nil
		}
		for _, machineName := range strings.Split(msg, " ") {
			phase, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, machineName, "-o=jsonpath={.status.phase}", "-n", MachineAPINamespace).Output()
			if phase == "Running" && strings.HasSuffix(machineName, machineNameSuffix) {
				e2e.Logf("The machine with suffix %s is Running %s", machineNameSuffix, machineName)
				newMachineName = machineName
				return true, nil
			}
		}
		e2e.Logf("The machine with suffix %s is not Running and waiting up to 1 minutes ...", machineNameSuffix)
		return false, nil
	})
	AssertWaitPollNoErr(err, "Wait machine Running failed.")
	return newMachineName
}

// WaitForMachineRunningByName check if the machine is Running by name
func WaitForMachineRunningByName(oc *CLI, machineName string) {
	e2e.Logf("Waiting for %s machine Running ...", machineName)
	err := wait.Poll(60*time.Second, 960*time.Second, func() (bool, error) {
		phase, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, machineName, "-o=jsonpath={.status.phase}", "-n", MachineAPINamespace).Output()
		if phase == "Running" {
			e2e.Logf("The machine %s is Running", machineName)
			return true, nil
		}
		e2e.Logf("The machine %s is not Running and waiting up to 1 minutes ...", machineName)
		return false, nil
	})
	AssertWaitPollNoErr(err, "Wait machine Running failed.")
}

// WaitForMachineDisappearBySuffix check if the machine is disappear by machine suffix
func WaitForMachineDisappearBySuffix(oc *CLI, machineNameSuffix string, labels string) {
	e2e.Logf("Waiting for the machine disappear by suffix ...")
	err := wait.Poll(60*time.Second, 1800*time.Second, func() (bool, error) {
		msg, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-l", labels, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
		if err2 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ...")
			return false, nil
		}
		for _, machineName := range strings.Split(msg, " ") {
			if strings.HasSuffix(machineName, machineNameSuffix) {
				e2e.Logf("The machine %s is not disappear and waiting up to 1 minutes ...", machineName)
				return false, nil
			}
		}
		e2e.Logf("The machine with suffix %s is disappear", machineNameSuffix)
		return true, nil
	})
	AssertWaitPollNoErr(err, "Wait machine disappear by suffix failed.")
}

// WaitForMachineDisappearBySuffixAndField check if the machine is disappear by machine suffix and field
func WaitForMachineDisappearBySuffixAndField(oc *CLI, machineNameSuffix string, field string, fieldValue string, labels string) {
	e2e.Logf("Waiting for the machine disappear by suffix and field...")
	err := wait.Poll(60*time.Second, 1800*time.Second, func() (bool, error) {
		msg, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "-l", labels, "-o=jsonpath={.items[*].metadata.name}", "-n", MachineAPINamespace).Output()
		if err2 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ...")
			return false, nil
		}
		for _, machineName := range strings.Split(msg, " ") {
			machineFieldValue, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, machineName, field, "-n", MachineAPINamespace).Output()
			if strings.HasSuffix(machineName, machineNameSuffix) && machineFieldValue == fieldValue {
				e2e.Logf("The machine %s is not disappear and waiting up to 1 minutes ...", machineName)
				return false, nil
			}
		}
		e2e.Logf("The machine with suffix %s and %s = %s is disappear", machineNameSuffix, field, fieldValue)
		return true, nil
	})
	AssertWaitPollNoErr(err, "Wait machine disappear by suffix and field failed.")
}

// WaitForMachineDisappearByName check if the machine is disappear by machine name
func WaitForMachineDisappearByName(oc *CLI, machineName string) {
	e2e.Logf("Waiting for the machine disappear by name ...")
	err := wait.Poll(60*time.Second, 1800*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, machineName, "-n", MachineAPINamespace).Output()
		if strings.Contains(output, "not found") {
			e2e.Logf("machine %s is disappear", machineName)
			return true, nil
		}
		e2e.Logf("machine %s is not disappear and waiting up to 1 minutes ...", machineName)
		return false, nil
	})
	AssertWaitPollNoErr(err, "Wait machine disappear by name failed.")
}

// CheckPlatform check the cluster's platform
func CheckPlatform(oc *CLI) string {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
	return strings.ToLower(output)
}

// SkipTestIfNotSupportedPlatform skip the test if current platform matches one of the provided not supported platforms
func SkipTestIfNotSupportedPlatform(oc *CLI, notsupported ...string) {
	p := CheckPlatform(oc)
	for _, nsp := range notsupported {
		if strings.EqualFold(nsp, p) {
			g.Skip("Skip this test scenario because it is not supported on the " + p + " platform")
		}
	}
}

// SkipTestIfSupportedPlatformNotMatched skip the test if supported platforms are not matched
func SkipTestIfSupportedPlatformNotMatched(oc *CLI, supported ...string) {
	var match bool
	p := CheckPlatform(oc)
	for _, sp := range supported {
		if strings.EqualFold(sp, p) {
			match = true
			break
		}
	}

	if !match {
		g.Skip("Skip this test scenario because it is not supported on the " + p + " platform")
	}
}

// SkipConditionally check the total number of Running machines, if greater than zero, we think machines are managed by machine api operator.
func SkipConditionally(oc *CLI) {
	msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachine, "--no-headers", "-n", MachineAPINamespace).Output()
	machinesRunning := strings.Count(msg, "Running")
	if machinesRunning == 0 {
		g.Skip("Expect at least one Running machine. Found none!!!")
	}
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

// GetAwsVolumeInfoAttachedToInstanceID get detail info of the volume attached to the instance id
func GetAwsVolumeInfoAttachedToInstanceID(instanceID string) (string, error) {
	mySession := session.Must(session.NewSession())
	svc := ec2.New(mySession, aws.NewConfig())
	input := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("attachment.instance-id"),
				Values: []*string{
					aws.String(instanceID),
				},
			},
		},
	}
	volumeInfo, err := svc.DescribeVolumes(input)
	newValue, _ := json.Marshal(volumeInfo)
	return string(newValue), err
}

// GetAwsCredentialFromCluster get aws credential from cluster
func GetAwsCredentialFromCluster(oc *CLI) {
	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", "json").Output()
	// Skip for sts and c2s clusters.
	if err != nil {
		g.Skip("Did not get credential to access aws, skip the testing.")

	}
	o.Expect(err).NotTo(o.HaveOccurred())
	accessKeyIDBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).String(), gjson.Get(credential, `data.aws_secret_access_key`).String()
	accessKeyID, err1 := base64.StdEncoding.DecodeString(accessKeyIDBase64)
	o.Expect(err1).NotTo(o.HaveOccurred())
	secureKey, err2 := base64.StdEncoding.DecodeString(secureKeyBase64)
	o.Expect(err2).NotTo(o.HaveOccurred())
	clusterRegion, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
	o.Expect(err3).NotTo(o.HaveOccurred())
	os.Setenv("AWS_ACCESS_KEY_ID", string(accessKeyID))
	os.Setenv("AWS_SECRET_ACCESS_KEY", string(secureKey))
	os.Setenv("AWS_REGION", clusterRegion)
}

// SkipForSNOCluster skip for SNO cluster
func SkipForSNOCluster(oc *CLI) {
	//Only 1 master, 1 worker node and with the same hostname.
	masterNodes, _ := GetClusterNodesBy(oc, "master")
	workerNodes, _ := GetClusterNodesBy(oc, "worker")
	if len(masterNodes) == 1 && len(workerNodes) == 1 && masterNodes[0] == workerNodes[0] {
		g.Skip("Skip for SNO cluster.")
	}
}

// GetVsphereCredentialFromCluster retrieves vSphere credentials as env variables
func GetVsphereCredentialFromCluster(oc *CLI) {

	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/vsphere-creds", "-n", "kube-system", "-o", "json").Output()
	// Skip for sts and c2s clusters.
	if err != nil {
		g.Skip("Did not get credential to access vSphere, skip the testing.")

	}

	// Scape the dots in the vsphere server hostname to access the json value
	scapedVsphereName := strings.ReplaceAll(VsphereServer, ".", "\\.")
	usernameBase64, passwordBase64 := gjson.Get(credential, `data.`+scapedVsphereName+`\.username`).String(), gjson.Get(credential, `data.`+scapedVsphereName+`\.password`).String()

	username, err := base64.StdEncoding.DecodeString(usernameBase64)
	o.Expect(err).NotTo(o.HaveOccurred())
	password, err := base64.StdEncoding.DecodeString(passwordBase64)
	o.Expect(err).NotTo(o.HaveOccurred())

	os.Setenv("VSPHERE_USER", string(username))
	os.Setenv("VSPHERE_PASSWORD", string(password))
	os.Setenv("VSPHERE_SERVER", VsphereServer)

}

// GetGcpCredentialFromCluster retrieves vSphere credentials as env variables
func GetGcpCredentialFromCluster(oc *CLI) {

	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/gcp-credentials", "-n", "kube-system", "-o", "json").Output()
	// Skip for sts and c2s clusters.
	if err != nil {
		g.Skip("Did not get credential to access GCP, skip the testing.")

	}

	serviceAccountBase64 := gjson.Get(credential, `data.service_account\.json`).String()

	serviceAccount, err := base64.StdEncoding.DecodeString(serviceAccountBase64)
	o.Expect(err).NotTo(o.HaveOccurred())

	os.Setenv("GOOGLE_CREDENTIALS", string(serviceAccount))

}

// Check if the cluster uses spot instances
func UseSpotInstanceWorkersCheck(oc *CLI) bool {
	machines, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machines.machine.openshift.io", "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-machine-api", "-l", "machine.openshift.io/interruptible-instance=").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if machines != "" {
		e2e.Logf("\nSpot instance workers are used\n")
		return true
	}
	return false
}

// IsAwsOutpostCluster judges whether the aws test cluster has outpost workers
func IsAwsOutpostCluster(oc *CLI) bool {
	if CheckPlatform(oc) != "aws" {
		return false
	}
	workersLabel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker", "--show-labels").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Contains(workersLabel, `topology.ebs.csi.aws.com/outpost-id`)
}

// SkipForAwsOutpostCluster skip for Aws Outpost cluster
func SkipForAwsOutpostCluster(oc *CLI) {
	if IsAwsOutpostCluster(oc) {
		g.Skip("Skip for Aws Outpost cluster.")
	}
}

// IsAwsOutpostMixedCluster check whether the cluster is aws outpost mixed workers cluster
func IsAwsOutpostMixedCluster(oc *CLI) bool {
	return IsAwsOutpostCluster(oc) && len(ListNonOutpostWorkerNodes(oc)) > 0
}

// SkipForNotAwsOutpostMixedCluster skip for not Aws Outpost Mixed cluster
func SkipForNotAwsOutpostMixedCluster(oc *CLI) {
	if !IsAwsOutpostMixedCluster(oc) {
		g.Skip("Skip for not Aws Outpost Mixed cluster.")
	}
}

// GetOneOutpostMachineSet return one outpost machineset name
func GetOneOutpostMachineSet(oc *CLI) string {
	outpostMachines, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker", "-l", "topology.ebs.csi.aws.com/outpost-id", "-o=jsonpath={.items[*].metadata.annotations.machine\\.openshift\\.io\\/machine}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	oneOutpostMachine := strings.Split(outpostMachines, " ")[0]
	start := strings.Index(oneOutpostMachine, "openshift-machine-api/")
	suffix := strings.LastIndex(oneOutpostMachine, "-")
	oneOutpostMachineSet := oneOutpostMachine[start+22 : suffix]
	e2e.Logf("oneOutpostMachineSet: %s", oneOutpostMachineSet)
	return oneOutpostMachineSet
}
