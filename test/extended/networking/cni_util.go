package networking

import (
	"fmt"
	"net"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
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

func CurlMultusPod2PodPass(oc *exutil.CLI, namespaceSrc string, podNameSrc string, podIPDst string, outputInt string, podEnvName string) {
	output, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+outputInt+" --connect-timeout 5 -s "+net.JoinHostPort(podIPDst, "8080"))
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(strings.Contains(output, podEnvName)).To(o.BeTrue())
}

func CurlMultusPod2PodFail(oc *exutil.CLI, namespaceSrc string, podNameSrc string, podIPDst string, outputInt string, podEnvName string) {
	output, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+outputInt+" --connect-timeout 5 -s "+net.JoinHostPort(podIPDst, "8080"))
	o.Expect(err).To(o.HaveOccurred())
	o.Expect(strings.Contains(output, podEnvName)).NotTo(o.BeTrue())
}

// Using getPodMultiNetworks when pods consume multiple NADs
// Using getPodMultiNetwork when pods consume single NAD
func getPodMultiNetworks(oc *exutil.CLI, namespace string, podName string, netName string) (string, string) {
	cmd1 := "ip a sho " + netName + " | awk 'NR==3{print $2}' |grep -Eo '((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])'"
	cmd2 := "ip a sho " + netName + " | awk 'NR==5{print $2}' |grep -Eo '([A-Fa-f0-9]{1,4}::?){1,7}[A-Fa-f0-9]{1,4}'"
	podv4Output, err := e2eoutput.RunHostCmd(namespace, podName, cmd1)
	o.Expect(err).NotTo(o.HaveOccurred())
	podIPv4 := strings.TrimSpace(podv4Output)
	podv6Output, err1 := e2eoutput.RunHostCmd(namespace, podName, cmd2)
	o.Expect(err1).NotTo(o.HaveOccurred())
	podIPv6 := strings.TrimSpace(podv6Output)
	return podIPv4, podIPv6
}
