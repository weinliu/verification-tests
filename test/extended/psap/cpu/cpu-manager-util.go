package cpu

import (
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

func getFirstDrainedMasterNode(oc *exutil.CLI) string {

	var (
		nodeName string
	)
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		nodeHostNameStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", `-ojsonpath={.items[?(@.spec.unschedulable==true)].metadata.name}`).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		nodeHostName := strings.Trim(nodeHostNameStr, "'")

		masterNodeNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/master", "-oname").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if len(nodeHostName) > 0 && strings.Contains(masterNodeNames, nodeHostName) {
			nodeName = nodeHostName
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "No any node's status is SchedulingDisabled was found")
	return nodeName
}
