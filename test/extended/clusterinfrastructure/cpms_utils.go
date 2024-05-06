package clusterinfrastructure

import (
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// waitForCPMSUpdateCompleted wait the Update to complete
func waitForCPMSUpdateCompleted(oc *exutil.CLI, replicas int) {
	e2e.Logf("Waiting for the Update completed ...")
	timeToWait := time.Duration(replicas*50) * time.Minute
	count := 0
	err := wait.Poll(1*time.Minute, timeToWait, func() (bool, error) {
		count++
		if count == 1 {
			e2e.Logf("Wait for the update to start and waiting up to 1 minutes ... count %d", count)
			return false, nil
		}
		desiredReplicas, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
		if err1 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ... count %d", count)
			return false, nil
		}
		readyReplicas, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		if err2 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ... count %d", count)
			return false, nil
		}
		currentReplicas, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
		if err3 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ... count %d", count)
			return false, nil
		}
		updatedReplicas, err4 := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.updatedReplicas}", "-n", machineAPINamespace).Output()
		if err4 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 1 minutes ... count %d", count)
			return false, nil
		}
		if desiredReplicas == currentReplicas && desiredReplicas == readyReplicas && desiredReplicas == updatedReplicas {
			e2e.Logf("The Update is completed! desiredReplicas is %s, count %d", desiredReplicas, count)
			return true, nil
		}
		e2e.Logf("The Update is still ongoing and waiting up to 1 minutes ... count %d, desiredReplicas is %s,currentReplicas is %s,readyReplicas is %s,updatedReplicas is %s", count, desiredReplicas, currentReplicas, readyReplicas, updatedReplicas)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Wait Update failed.")
}

// skipForCPMSNotStable skip the test if the cpms is not stable
func skipForCPMSNotStable(oc *exutil.CLI) {
	readyReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	currentReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	desiredReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	updatedReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.updatedReplicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas && desiredReplicas == updatedReplicas) {
		g.Skip("Skip for cpms is not stable!")
	}
}

// printNodeInfo print the output of oc get node
func printNodeInfo(oc *exutil.CLI) {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
	e2e.Logf("%v", output)
}

// getMachineSuffix get the machine suffix
func getMachineSuffix(oc *exutil.CLI, machineName string) string {
	start := strings.LastIndex(machineName, "-")
	suffix := machineName[start:]
	return suffix
}

// checkIfCPMSIsStable check if the Update is completed
func checkIfCPMSIsStable(oc *exutil.CLI) bool {
	err := wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
		readyReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		currentReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		desiredReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if desiredReplicas == currentReplicas && desiredReplicas == readyReplicas {
			e2e.Logf("cpms is stable!")
			return true, nil
		}
		e2e.Logf("cpms is not stable, desiredReplicas :%s, currentReplicas:%s, readyReplicas:%s", desiredReplicas, currentReplicas, readyReplicas)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "CPMS is not stable!!!.")
	return err == nil
}

// getCPMSAvailabilityZones get zones from cpms
func getCPMSAvailabilityZones(oc *exutil.CLI, iaasPlatform string) []string {
	var getCPMSAvailabilityZonesJSON string
	switch iaasPlatform {
	case clusterinfra.AWS:
		getCPMSAvailabilityZonesJSON = "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[*].placement.availabilityZone}"
	case clusterinfra.AZURE, clusterinfra.GCP:
		getCPMSAvailabilityZonesJSON = "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains." + iaasPlatform + "[*].zone}"
	default:
		e2e.Logf("The " + iaasPlatform + " Platform is not supported for now.")
	}
	availabilityZonesStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getCPMSAvailabilityZonesJSON, "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	availabilityZones := strings.Split(availabilityZonesStr, " ")
	e2e.Logf("availabilityZones:%s", availabilityZones)
	return availabilityZones
}

// getZoneAndMachineFromCPMSZones get the zone only have one machine and return the machine name
func getZoneAndMachineFromCPMSZones(oc *exutil.CLI, availabilityZones []string) (int, string, string) {
	var key int
	var value, machineName string
	for key, value = range availabilityZones {
		labels := "machine.openshift.io/zone=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		machineNamesStr, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("machines.machine.openshift.io", "-l", labels, "-o=jsonpath={.items[*].metadata.name}", "-n", machineAPINamespace).Output()
		if machineNamesStr == "" {
			continue
		}
		machineNames := strings.Split(machineNamesStr, " ")
		machineName = machineNames[0]
		number := len(machineNames)
		if number == 1 {
			e2e.Logf("key:%s, failureDomain:%s, master machine name:%s", key, value, machineName)
			break
		}
	}
	return key, value, machineName
}

// deleteControlPlaneMachineSet delete the ControlPlaneMachineSet to make it Inactive
func deleteControlPlaneMachineSet(oc *exutil.CLI) {
	e2e.Logf("Deleting ControlPlaneMachineSet ...")
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("controlplanemachineset", "cluster", "-n", machineAPINamespace, "--wait=false").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// activeControlPlaneMachineSet active the ControlPlaneMachineSet
func activeControlPlaneMachineSet(oc *exutil.CLI) {
	e2e.Logf("Active ControlPlaneMachineSet ...")
	err := wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
		cpmsState, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=jsonpath={.spec.state}").Output()
		if cpmsState != "Active" {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"state":"Active"}}`, "--type=merge", "-n", machineAPINamespace).Output()
			e2e.Logf("controlplanemachineset status is: %s, waiting up to 2 seconds, then patch output: %s", cpmsState, output)
			return false, nil
		}
		e2e.Logf("controlplanemachineset is in Active state")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "controlplanemachineset is not in Active state")
}

// replaceOneMasterMachine create a new master machine and delete the old master machine
func replaceOneMasterMachine(oc *exutil.CLI, oldMachineName, newMachineName string) {
	e2e.Logf("Creating a new master machine ...")
	machineJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machines.machine.openshift.io", oldMachineName, "-n", machineAPINamespace, "-o=json").OutputToFile("mastermachine.json")
	o.Expect(err).NotTo(o.HaveOccurred())

	bytes, _ := ioutil.ReadFile(machineJSON)
	value1, _ := sjson.Set(string(bytes), "metadata.name", newMachineName)
	value2, _ := sjson.Set(value1, "spec.providerID", nil)
	err = os.WriteFile(machineJSON, []byte(value2), 0o644)
	o.Expect(err).NotTo(o.HaveOccurred())

	if err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", machineJSON).Execute(); err != nil {
		clusterinfra.DeleteMachine(oc, newMachineName)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		clusterinfra.DeleteMachine(oc, oldMachineName)
		clusterinfra.WaitForMachineRunningByName(oc, newMachineName)
		clusterinfra.WaitForMachineDisappearByName(oc, oldMachineName)
	}
}

// randomMasterMachineName randomly generate a master machine name
func randomMasterMachineName(oldMachineName string) (string, string) {
	start := strings.LastIndex(oldMachineName, "-")
	newIndex := strconv.Itoa(rand.Intn(100) + 3)
	newMachineName := oldMachineName[0:start+1] + newIndex
	return "-" + newIndex, newMachineName
}

// getMasterMachineNameBySuffix get the master machine name by suffix
func getMasterMachineNameBySuffix(oc *exutil.CLI, suffix string) string {
	currentMasterMachineNames := clusterinfra.ListMasterMachineNames(oc)
	for _, value := range currentMasterMachineNames {
		if suffix == getMachineSuffix(oc, value) {
			return value
		}
	}
	return ""
}

// waitForClusterStable wait cluster to stabilize
func waitForClusterStable(oc *exutil.CLI) {
	e2e.Logf("Wait cluster to stabilize ...")
	err := wait.Poll(2*time.Minute, 40*time.Minute, func() (bool, error) {
		authenticationState, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/authentication", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		if err1 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 2 minutes ...")
			return false, nil
		}
		etcdState, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/etcd", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		if err2 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 2 minutes ...")
			return false, nil
		}
		kubeapiserverState, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/kube-apiserver", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		if err3 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 2 minutes ...")
			return false, nil
		}
		openshiftapiserverState, err4 := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/openshift-apiserver", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		if err4 != nil {
			e2e.Logf("The server was unable to return a response and waiting up to 2 minutes ...")
			return false, nil
		}
		if strings.Contains(authenticationState, "TrueFalseFalse") && strings.Contains(etcdState, "TrueFalseFalse") && strings.Contains(kubeapiserverState, "TrueFalseFalse") && strings.Contains(openshiftapiserverState, "TrueFalseFalse") {
			e2e.Logf("The cluster is stable!")
			return true, nil
		}
		e2e.Logf("The cluster is not stable and waiting up to 2 minutes ...")
		return false, nil
	})
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Output()
	e2e.Logf("%v", output)
	exutil.AssertWaitPollNoErr(err, "Wait cluster to stabilize failed.")
}

// getCPMSState get CPMS state is Active or Inactive
func getCPMSState(oc *exutil.CLI) string {
	cpmsState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=jsonpath={.spec.state}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return cpmsState
}

// getArchitectureType get the architecture is arm64 or amd64
func getArchitectureType(oc *exutil.CLI) string {
	architecture, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", clusterinfra.GetNodeNameFromMachine(oc, clusterinfra.ListMasterMachineNames(oc)[0]), "-o=jsonpath={.status.nodeInfo.architecture}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return architecture
}

// skipForClusterNotStable skip the test if the cluster is not stable
func skipForClusterNotStable(oc *exutil.CLI) {
	authenticationState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/authentication", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	etcdState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/etcd", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeapiserverState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/kube-apiserver", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	openshiftapiserverState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/openshift-apiserver", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !(strings.Contains(authenticationState, "TrueFalseFalse") && strings.Contains(etcdState, "TrueFalseFalse") && strings.Contains(kubeapiserverState, "TrueFalseFalse") && strings.Contains(openshiftapiserverState, "TrueFalseFalse")) {
		g.Skip("Skip for cluster is not stable!")
	}
}

// checkIfCPMSCoIsStable check if some replicas need update, if no replicas need update, return true, else return false
func checkIfCPMSCoIsStable(oc *exutil.CLI) bool {
	cpmsState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/control-plane-machine-set", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(cpmsState, "TrueFalseFalse") {
		return true
	}
	message, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/control-plane-machine-set").Output()
	e2e.Logf("%v", message)
	return false
}

// waitMasterNodeReady wait all master node ready
func waitMasterNodeReady(oc *exutil.CLI) {
	err := wait.Poll(1*time.Minute, 5*time.Minute, func() (bool, error) {
		masterMachineList := clusterinfra.ListMasterMachineNames(oc)
		for _, masterMachineName := range masterMachineList {
			nodeName := clusterinfra.GetNodeNameFromMachine(oc, masterMachineName)
			readyStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
			if readyStatus != "True" {
				e2e.Logf("node %s is not ready, status:%s", nodeName, readyStatus)
				return false, nil
			}
		}
		e2e.Logf("All master node are ready!")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "wait master node ready failed!")
}
