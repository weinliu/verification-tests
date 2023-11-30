package baremetal

import (
	"context"
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

func checkOperatorsRunning(oc *exutil.CLI) (bool, error) {
	jpath := `{range .items[*]}{.metadata.name}:{.status.conditions[?(@.type=='Available')].status}{':'}{.status.conditions[?(@.type=='Degraded')].status}{'\n'}{end}`
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperators.config.openshift.io", "-o", "jsonpath="+jpath).Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute 'oc get clusteroperators.config.openshift.io' command: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		e2e.Logf("%s", line)
		parts := strings.Split(line, ":")
		available := parts[1] == "True"
		degraded := parts[2] == "False"

		if !available || !degraded {
			return false, nil
		}
	}

	return true, nil
}

func checkNodesRunning(oc *exutil.CLI) (bool, error) {
	nodeNames, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-o=jsonpath={.items[*].metadata.name}").Output()
	if nodeErr != nil {
		return false, fmt.Errorf("failed to execute 'oc get nodes' command: %v", nodeErr)
	}
	nodes := strings.Fields(nodeNames)
	e2e.Logf("\nNode Names are %v", nodeNames)
	for _, node := range nodes {
		nodeStatus, statusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node, "-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}").Output()
		if statusErr != nil {
			return false, fmt.Errorf("failed to execute 'oc get nodes' command: %v", statusErr)
		}
		e2e.Logf("\nNode %s Status is %s\n", node, nodeStatus)

		if nodeStatus != "True" {
			return false, nil
		}
	}
	return true, nil
}

func waitForDeployStatus(oc *exutil.CLI, depName string, nameSpace string, depStatus string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		statusOp, err := oc.AsAdmin().Run("get").Args("-n", nameSpace, "deployment", depName, "-o=jsonpath={.status.conditions[?(@.type=='Available')].status}'").Output()
		if err != nil {
			return false, err
		}

		if strings.Contains(statusOp, depStatus) {
			e2e.Logf("Deployment %v state is %v", depName, depStatus)
			return true, nil
		}
		e2e.Logf("deployment %v is state %v, Trying again", depName, statusOp)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The test deployment job is not running")
}

func getPodName(oc *exutil.CLI, ns string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nPod Name is %v", podName)
	return podName
}

func getPodStatus(oc *exutil.CLI, namespace string, podName string) string {
	podStatus, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status is %q", podName, podStatus)
	return podStatus
}
