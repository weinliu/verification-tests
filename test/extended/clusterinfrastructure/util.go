package clusterinfrastructure

import (
	"math/rand"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	machineAPINamespace      = "openshift-machine-api"
	clusterAPINamespace      = "openshift-cluster-api"
	machineApproverNamespace = "openshift-cluster-machine-approver"
	mapiMachineset           = "machinesets.machine.openshift.io"
	mapiMachine              = "machines.machine.openshift.io"
	mapiMHC                  = "machinehealthchecks.machine.openshift.io"
	capiMachineset           = "machinesets.cluster.x-k8s.io"
	capiMachine              = "machines.cluster.x-k8s.io"
	defaultTimeout           = 300 * time.Second
)

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var jsonCfg string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "cloud.json")
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		jsonCfg = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Applying resources from template is failed")
	e2e.Logf("The resource is %s", jsonCfg)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", jsonCfg).Execute()
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

func skipTestIfSpotWorkers(oc *exutil.CLI) {
	machines, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-o=jsonpath={.items[*].metadata.name}", "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if machines != "" {
		g.Skip("This case cannot be tested using spot instance!")
	}
}

// Get the cluster history versions
func getClusterHistoryVersions(oc *exutil.CLI) string {
	historyVersions, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.history[*].version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Cluster history versions are %s", historyVersions)
	return historyVersions
}

// To be used if sensitive data is present in template
func applyResourceFromTemplateWithoutInfo(oc *exutil.CLI, parameters ...string) error {
	var jsonCfg string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().NotShowInfo().Run("process").Args(parameters...).OutputToFile(getRandomString() + "cloud.json")
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		jsonCfg = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Applying resources from template is failed")
	e2e.Logf("The resource is %s", jsonCfg)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", jsonCfg).Execute()
}

func getClusterRegion(oc *exutil.CLI) string {
	region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", `-ojsonpath={.items[].metadata.labels.topology\.kubernetes\.io/region}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return region
}
