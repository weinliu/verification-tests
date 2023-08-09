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

// waitForClusterHealthy check if new machineconfig is applied successfully
func waitForClusterHealthy(oc *exutil.CLI) {
	e2e.Logf("Waiting for the cluster healthy ...")
	// sleep for 5 minites to make sure related mcp start to update
	time.Sleep(5 * time.Minute)
	timeToWait := time.Duration(getNodeCount(oc)*5) * time.Minute
	pollErr := wait.Poll(1*time.Minute, timeToWait-5, func() (bool, error) {
		master, errMaster := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", "master", "-o", "jsonpath='{.status.conditions[?(@.type==\"Updated\")].status}'").Output()
		worker, errWorker := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", "worker", "-o", "jsonpath='{.status.conditions[?(@.type==\"Updated\")].status}'").Output()
		if errMaster != nil || errWorker != nil {
			e2e.Logf("the err:%v,%v, and try next round", errMaster, errWorker)
			return false, nil
		}
		if strings.Contains(master, "True") && strings.Contains(worker, "True") {
			e2e.Logf("mc operation is completed on mcp")
			return true, nil
		}
		return false, nil
	})
	if pollErr != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Failf("Expected cluster is not healthy after waiting up to %s minutes ...", timeToWait)
	}
	e2e.Logf("Cluster is healthy ...")
}

func getNodeCount(oc *exutil.CLI) int {
	nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeCount := int(strings.Count(nodes, "Ready")) + int(strings.Count(nodes, "NotReady"))
	return nodeCount
}

// SkipIfCloudControllerManagerNotDeployed check if ccm is deployed
func SkipIfCloudControllerManagerNotDeployed(oc *exutil.CLI) {
	var ccm string
	var err error
	ccm, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
	if err == nil {
		if len(ccm) == 0 {
			g.Skip("Skip for cloud-controller-manager is not deployed!")
		}
	}
}
