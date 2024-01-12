package netobserv

import (
	"context"
	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// polls to check ovs-flows-config is created or deleted given shouldExist is true or false
func waitCnoConfigMapUpdate(oc *exutil.CLI, shouldExist bool) {
	err := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 10*time.Minute, false, func(context.Context) (bool, error) {

		// check whether ovs-flows-config config map exists in openshift-network-operator NS
		_, stderr, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "ovs-flows-config", "-n", "openshift-network-operator").Outputs()

		if stderr == "" && shouldExist {
			return true, nil
		}

		if stderr != "" && !shouldExist {
			return true, nil
		}
		return false, nil
	})

	exutil.AssertWaitPollNoErr(err, "ovs-flows-config ConfigMap is not updated")
}

// returns target configured in ovs-flows-config config map
func getOVSFlowsConfigTarget(oc *exutil.CLI, flowlogsPipelineDeployedAs string) (string, error) {

	var template string
	if flowlogsPipelineDeployedAs == "Deployment" {
		template = "{{.data.sharedTarget}}"
	}

	if flowlogsPipelineDeployedAs == "DaemonSet" {
		template = "{{.data.nodePort}}"
	}

	stdout, stderr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "ovs-flows-config", "-n", "openshift-network-operator").Template(template).Outputs()

	if stderr != "" || err != nil {
		e2e.Logf("Fetching ovs-flows-config configmap return err %s", stderr)
	}
	return stdout, err
}

// get flow collector IPs configured in OVS
func getOVSCollectorIP(oc *exutil.CLI) ([]string, error) {
	jsonpath := "{.items[*].spec.containers[*].env[?(@.name==\"IPFIX_COLLECTORS\")].value}"

	var collectors []string
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-ovn-kubernetes", "-o", "jsonpath="+jsonpath).Output()

	if err != nil {
		return collectors, err
	}
	collectors = strings.Split(stdout, " ")

	return collectors, nil
}
