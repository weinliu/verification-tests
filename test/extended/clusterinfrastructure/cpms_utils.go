package clusterinfrastructure

import (
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// WaitForUpdateCompleted check if the Update is completed
func WaitForUpdateCompleted(oc *exutil.CLI, replicas int) {
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

// SkipForCPMSNotExist skip the test if controlplanemachineset doesn't exist
func SkipForCPMSNotExist(oc *exutil.CLI) {
	controlplanemachineset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(controlplanemachineset) == 0 {
		g.Skip("Skip for controlplanemachineset doesn't exist!")
	}
}

// SkipForUpdateIsOngoing skip the test if the previous Update is onging
func SkipForUpdateIsOngoing(oc *exutil.CLI) {
	readyReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	currentReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	desiredReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas) {
		g.Skip("Skip for the previous Update is onging!")
	}
}

// PrintNodeInfo print the output of oc get node
func PrintNodeInfo(oc *exutil.CLI) {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
	e2e.Logf("%v", output)
}

// GetMachineSuffix get the machine suffix
func GetMachineSuffix(oc *exutil.CLI, machineName string) string {
	start := strings.LastIndex(machineName, "-")
	suffix := machineName[start+1:]
	return suffix
}

// CheckIfUpdateIsCompleted check if the Update is completed
func CheckIfUpdateIsCompleted(oc *exutil.CLI) bool {
	readyReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	currentReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	desiredReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	updatedReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.updatedReplicas}", "-n", machineAPINamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas && desiredReplicas == updatedReplicas) {
		return false
	}
	return true
}
