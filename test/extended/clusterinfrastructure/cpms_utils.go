package clusterinfrastructure

import (
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// WaitForRollingUpdateCompleted check if the RollingUpdate is completed
func WaitForRollingUpdateCompleted(oc *exutil.CLI, replicas int) {
	e2e.Logf("Waiting for the RollingUpdate completed ...")
	timeToWait := time.Duration(replicas*25) * time.Minute
	err := wait.Poll(1*time.Minute, timeToWait, func() (bool, error) {
		readyReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		currentReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
		desiredReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
		updatedReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.updatedReplicas}", "-n", machineAPINamespace).Output()
		if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas && desiredReplicas == updatedReplicas) {
			e2e.Logf("The RollingUpdate is still ongoing and waiting up to 1 minutes ...")
			return false, nil
		}
		e2e.Logf("The RollingUpdate is completed!")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Wait RollingUpdate failed.")
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
