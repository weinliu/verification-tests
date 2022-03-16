package clusterinfrastructure

import (
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	machineAPINamespace = "openshift-machine-api"
)

type MachineSetDescription struct {
	Name     string
	Replicas int
}

// CreateMachineSet create a new machineset
func (ms *MachineSetDescription) CreateMachineSet(oc *exutil.CLI) {
	e2e.Logf("Creating a new MachineSets ...")
	machinesetName := GetRandomMachineSetName(oc)
	machineSetJson, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", machinesetName, "-n", machineAPINamespace, "-o=json").OutputToFile("machineset.json")
	o.Expect(err).NotTo(o.HaveOccurred())

	bytes, _ := ioutil.ReadFile(machineSetJson)
	value1, _ := sjson.Set(string(bytes), "metadata.name", ms.Name)
	value2, _ := sjson.Set(value1, "spec.selector.matchLabels.machine\\.openshift\\.io/cluster-api-machineset", ms.Name)
	value3, _ := sjson.Set(value2, "spec.template.metadata.labels.machine\\.openshift\\.io/cluster-api-machineset", ms.Name)
	value4, _ := sjson.Set(value3, "spec.replicas", ms.Replicas)
	// Adding taints to machineset so that pods without toleration can not schedule to the nodes we provision
	value5, _ := sjson.Set(value4, "spec.template.spec.taints.0", map[string]interface{}{"effect": "NoSchedule", "key": "mapi", "value": "mapi_test"})
	err = ioutil.WriteFile(machineSetJson, []byte(value5), 0644)
	o.Expect(err).NotTo(o.HaveOccurred())

	if err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", machineSetJson).Execute(); err != nil {
		ms.DeleteMachineSet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		WaitForMachinesRunning(oc, ms.Replicas, ms.Name)
	}
}

// DeleteMachineSet delete a machineset
func (ms *MachineSetDescription) DeleteMachineSet(oc *exutil.CLI) error {
	e2e.Logf("Deleting a MachineSets ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("machineset", ms.Name, "-n", machineAPINamespace).Execute()
}

// ListAllMachineNames list all machines
func ListAllMachineNames(oc *exutil.CLI) []string {
	e2e.Logf("Listing all Machines ...")
	machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-o=jsonpath={.items[*].metadata.name}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineNames, " ")
}

// ListWorkerMachineSetNames list all worker machineSets
func ListWorkerMachineSetNames(oc *exutil.CLI) []string {
	e2e.Logf("Listing all MachineSets ...")
	machineSetNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", "-o=jsonpath={.items[*].metadata.name}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineSetNames, " ")
}

// ListWorkerMachineNames list all worker machines
func ListWorkerMachineNames(oc *exutil.CLI) []string {
	e2e.Logf("Listing all Machines ...")
	machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-o=jsonpath={.items[*].metadata.name}", "-l", "machine.openshift.io/cluster-api-machine-type=worker", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineNames, " ")
}

// GetMachineNamesFromMachineSet get all Machines in a Machineset
func GetMachineNamesFromMachineSet(oc *exutil.CLI, machineSetName string) []string {
	e2e.Logf("Getting all Machines in a Machineset ...")
	machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-o=jsonpath={.items[*].metadata.name}", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(machineNames, " ")
}

// GetNodeNameFromMachine get node name for a machine
func GetNodeNameFromMachine(oc *exutil.CLI, machineName string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", machineName, "-o=jsonpath={.status.nodeRef.name}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return nodeName
}

// GetRandomMachineSetName get a random MachineSet name
func GetRandomMachineSetName(oc *exutil.CLI) string {
	e2e.Logf("Getting a random MachineSet ...")
	return ListWorkerMachineSetNames(oc)[0]
}

// ScaleMachineSet scale a MachineSet by replicas
func ScaleMachineSet(oc *exutil.CLI, machineSetName string, replicas int) {
	e2e.Logf("Scaling MachineSets ...")
	_, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas="+strconv.Itoa(replicas), "machineset", machineSetName, "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// WaitForMachinesRunning check if all the machines are Running in a MachineSet
func WaitForMachinesRunning(oc *exutil.CLI, machineNumber int, machineSetName string) {
	e2e.Logf("Waiting for the machines Running ...")
	pollErr := wait.Poll(60*time.Second, 720*time.Second, func() (bool, error) {
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", machineSetName, "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		machinesRunning, _ := strconv.Atoi(msg)
		if machinesRunning != machineNumber {
			e2e.Logf("Expected %v  machine are not Running yet and waiting up to 1 minutes ...", machineNumber)
			return false, nil
		}
		e2e.Logf("Expected %v  machines are Running", machineNumber)
		return true, nil
	})
	if pollErr != nil {
		e2e.Failf("Expected %v  machines are not Running after waiting up to 12 minutes ...", machineNumber)
	}
	e2e.Logf("All machines are Running ...")
}

func WaitForMachineFailed(oc *exutil.CLI, machineSetName string) {
	e2e.Logf("Wait for machine to go into Failed phase")
	err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=jsonpath={.items[0].status.phase}").Output()
		if output != "Failed" {
			e2e.Logf("machine is not in Failed phase and waiting up to 3 seconds ...")
			return false, nil
		}
		e2e.Logf("machine is in Failed phase")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Check machine phase failed")
}

func WaitForMachineProvisioned(oc *exutil.CLI, machineSetName string) {
	e2e.Logf("Wait for machine to go into Provisioned phase")
	err := wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machineSetName, "-o=jsonpath={.items[0].status.phase}").Output()
		if output != "Provisioned" {
			e2e.Logf("machine is not in Provisioned phase and waiting up to 60 seconds ...")
			return false, nil
		}
		e2e.Logf("machine is in Provisioned phase")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Check machine phase failed")
}

func CheckPlatform(oc *exutil.CLI) string {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
	return strings.ToLower(output)
}

// SkipConditionally check the total number of Running machines, if greater than zero, we think machines are managed by machine api operator.
func SkipConditionally(oc *exutil.CLI) {
	msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("machines", "--no-headers", "-n", machineAPINamespace).Output()
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
