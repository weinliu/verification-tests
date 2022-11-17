package clusterinfrastructure

import (
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// waitForCPMSUpdateCompleted check if the Update is completed
func waitForCPMSUpdateCompleted(oc *exutil.CLI, replicas int) {
	e2e.Logf("Waiting for the Update completed ...")
	timeToWait := time.Duration(replicas*25) * time.Minute
	err := wait.Poll(1*time.Minute, timeToWait, func() (bool, error) {
		readyReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		currentReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
		desiredReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
		updatedReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.updatedReplicas}", "-n", machineAPINamespace).Output()
		if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas && desiredReplicas == updatedReplicas) {
			e2e.Logf("The Update is still ongoing and waiting up to 1 minutes ...")
			return false, nil
		}
		e2e.Logf("The Update is completed!")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Wait Update failed.")
}

// skipForCPMSNotExist skip the test if controlplanemachineset doesn't exist
func skipForCPMSNotExist(oc *exutil.CLI) {
	controlplanemachineset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
	if err != nil || len(controlplanemachineset) == 0 {
		g.Skip("Skip for controlplanemachineset doesn't exist!")
	}
}

// skipForCPMSNotStable skip the test if the cpms is not stable
func skipForCPMSNotStable(oc *exutil.CLI) {
	readyReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	currentReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	desiredReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas) {
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
	suffix := machineName[start+1:]
	return suffix
}

// checkIfCPMSIsStable check if the Update is completed
func checkIfCPMSIsStable(oc *exutil.CLI) bool {
	readyReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	currentReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	desiredReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas) {
		e2e.Logf("cpms is not stable, desiredReplicas :%s, currentReplicas:%s, readyReplicas:%s", desiredReplicas, currentReplicas, readyReplicas)
		return false
	}
	return true
}

// getCPMSAvailabilityZones get zones from cpms
func getCPMSAvailabilityZones(oc *exutil.CLI) []string {
	getAvailabilityZonesJSON := "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[*].placement.availabilityZone}"
	availabilityZonesStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getAvailabilityZonesJSON, "-n", machineAPINamespace).Output()
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
		machineNames := strings.Split(machineNamesStr, " ")
		machineName = machineNames[0]
		number := len(machineNames)
		if number == 1 {
			e2e.Logf("failureDomain:%s, master machine name:%s", value, machineName)
			break
		}
	}
	return key, value, machineName
}
