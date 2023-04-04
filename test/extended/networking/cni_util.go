package networking

import (
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type multihomingNAD struct {
	namespace      string
	nadname        string
	subnets        string
	nswithnadname  string
	excludeSubnets string
	topology       string
	template       string
}

type testMultihomingPod struct {
	name       string
	namespace  string
	podlabel   string
	nadname    string
	podenvname string
	nodename   string
	template   string
}

func (nad *multihomingNAD) createMultihomingNAD(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", nad.template, "-p", "NAMESPACE="+nad.namespace, "NADNAME="+nad.nadname, "SUBNETS="+nad.subnets, "NSWITHNADNAME="+nad.nswithnadname, "EXCLUDESUBNETS="+nad.excludeSubnets, "TOPOLOGY="+nad.topology)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to net attach definition %v", nad.nadname))
}

func (pod *testMultihomingPod) createTestMultihomingPod(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "PODLABEL="+pod.podlabel, "NADNAME="+pod.nadname, "PODENVNAME="+pod.podenvname, "NODENAME="+pod.nodename)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func checkOVNSwitch(oc *exutil.CLI, nad string, leaderPod string) bool {
	listSWCmd := "ovn-nbctl show | grep switch"
	listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", leaderPod, listSWCmd)
	o.Expect(listErr).NotTo(o.HaveOccurred())
	return strings.Contains(listOutput, nad)
}

func checkNAD(oc *exutil.CLI, ns string, nad string) bool {
	nadOutput, nadOutputErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("net-attach-def", "-n", ns).Output()
	o.Expect(nadOutputErr).NotTo(o.HaveOccurred())
	return strings.Contains(nadOutput, nad)
}

func checkOVNswitchPorts(podName []string, outPut string) bool {
	result := true
	for _, pod := range podName {
		if !strings.Contains(outPut, pod) {
			result = false
		}
	}
	return result
}
