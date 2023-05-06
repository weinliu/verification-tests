package networking

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
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

func multihomingBeforeCheck(oc *exutil.CLI) ([]string, []string, []string, []string, string, string, string) {
	var (
		buildPruningBaseDir    = exutil.FixturePath("testdata", "networking/multihoming")
		multihomingNADTemplate = filepath.Join(buildPruningBaseDir, "multihoming-NAD-template.yaml")
		multihomingPodTemplate = filepath.Join(buildPruningBaseDir, "multihoming-pod-template.yaml")
	)

	g.By("Get the ready-schedulable worker nodes")
	nodeList, nodeErr := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
	o.Expect(nodeErr).NotTo(o.HaveOccurred())
	if len(nodeList.Items) < 2 {
		g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
	}

	g.By("Create a test namespace")
	ns1 := oc.Namespace()

	nadName := "layer2dualstacknetwork"
	nsWithnad := ns1 + "/" + nadName

	g.By("Create a custom resource network-attach-defintion in tested namespace")
	nad1ns1 := multihomingNAD{
		namespace:      ns1,
		nadname:        nadName,
		subnets:        "192.168.100.0/24,fd00:dead:beef::0/64",
		nswithnadname:  nsWithnad,
		excludeSubnets: "",
		topology:       "layer2",
		template:       multihomingNADTemplate,
	}
	nad1ns1.createMultihomingNAD(oc)

	g.By("Check if the network-attach-defintion is created")
	if checkNAD(oc, ns1, nadName) {
		e2e.Logf("The correct network-attach-defintion: %v is created!", nadName)
	} else {
		e2e.Failf("The correct network-attach-defintion: %v is not created!", nadName)
	}

	g.By("Check if the new OVN switch is created")
	ovnMasterPodName := getOVNLeaderPod(oc, "north")
	o.Expect(ovnMasterPodName).ShouldNot(o.Equal(""))
	o.Eventually(func() bool {
		return checkOVNSwitch(oc, nadName, ovnMasterPodName)
	}, 30*time.Second, 5*time.Second).Should(o.BeTrue(), "The correct OVN switch is not created")

	g.By("Create 1st pod consuming above network-attach-defintion in ns1")
	pod1 := testMultihomingPod{
		name:       "multihoming-pod-1",
		namespace:  ns1,
		podlabel:   "multihoming-pod1",
		nadname:    nadName,
		nodename:   nodeList.Items[0].Name,
		podenvname: "Hello multihoming-pod-1",
		template:   multihomingPodTemplate,
	}
	pod1.createTestMultihomingPod(oc)
	o.Expect(waitForPodWithLabelReady(oc, ns1, "name=multihoming-pod1")).NotTo(o.HaveOccurred())

	g.By("Create 2nd pod consuming above network-attach-defintion in ns1")
	pod2 := testMultihomingPod{
		name:       "multihoming-pod-2",
		namespace:  ns1,
		podlabel:   "multihoming-pod2",
		nadname:    nadName,
		nodename:   nodeList.Items[0].Name,
		podenvname: "Hello multihoming-pod-2",
		template:   multihomingPodTemplate,
	}
	pod2.createTestMultihomingPod(oc)
	o.Expect(waitForPodWithLabelReady(oc, ns1, "name=multihoming-pod2")).NotTo(o.HaveOccurred())

	g.By("Create 3rd pod consuming above network-attach-defintion in ns1")
	pod3 := testMultihomingPod{
		name:       "multihoming-pod-3",
		namespace:  ns1,
		podlabel:   "multihoming-pod3",
		nadname:    nadName,
		nodename:   nodeList.Items[1].Name,
		podenvname: "Hello multihoming-pod-3",
		template:   multihomingPodTemplate,
	}
	pod3.createTestMultihomingPod(oc)
	o.Expect(waitForPodWithLabelReady(oc, ns1, "name=multihoming-pod3")).NotTo(o.HaveOccurred())

	g.By("Get IPs from the pod1's secondary interface")
	pod1Name := getPodName(oc, ns1, "name=multihoming-pod1")
	pod1IPv4, pod1IPv6 := getPodMultiNetwork(oc, ns1, pod1Name[0])
	e2e.Logf("The v4 address of pod1 is: %v", pod1IPv4, "net1", pod1.podenvname)
	e2e.Logf("The v6 address of pod1 is: %v", pod1IPv6, "net1", pod1.podenvname)

	g.By("Get IPs from the pod2's secondary interface")
	pod2Name := getPodName(oc, ns1, "name=multihoming-pod2")
	pod2IPv4, pod2IPv6 := getPodMultiNetwork(oc, ns1, pod2Name[0])
	e2e.Logf("The v4 address of pod1 is: %v", pod2IPv4, "net1", pod2.podenvname)
	e2e.Logf("The v6 address of pod1 is: %v", pod2IPv6, "net1", pod2.podenvname)

	g.By("Get IPs from the pod3's secondary interface")
	pod3Name := getPodName(oc, ns1, "name=multihoming-pod3")
	pod3IPv4, pod3IPv6 := getPodMultiNetwork(oc, ns1, pod3Name[0])
	e2e.Logf("The v4 address of pod1 is: %v", pod3IPv4, "net1", pod3.podenvname)
	e2e.Logf("The v6 address of pod1 is: %v", pod3IPv6, "net1", pod3.podenvname)

	g.By("Check if the new OVN switch ports is created")
	listSWCmd := "ovn-nbctl show | grep port | grep " + nadName + " "
	podName := []string{pod1Name[0], pod2Name[0], pod3Name[0]}
	o.Eventually(func() bool {
		listOutput, _ := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd)
		return checkOVNswitchPorts(podName, listOutput)
	}, 30*time.Second, 5*time.Second).Should(o.BeTrue(), "The correct OVN switch ports are not created")

	g.By("Checking connectivity from pod1 to pod2")
	CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv4, "net1", pod2.podenvname)
	CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv6, "net1", pod2.podenvname)

	g.By("Checking connectivity from pod1 to pod3")
	CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3IPv4, "net1", pod3.podenvname)
	CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3IPv6, "net1", pod3.podenvname)

	g.By("Checking connectivity from pod2 to pod1")
	CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv4, "net1", pod1.podenvname)
	CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv6, "net1", pod1.podenvname)

	g.By("Checking connectivity from pod2 to pod3")
	CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3IPv4, "net1", pod3.podenvname)
	CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3IPv6, "net1", pod3.podenvname)

	g.By("Checking connectivity from pod3 to pod1")
	CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1IPv4, "net1", pod1.podenvname)
	CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1IPv6, "net1", pod1.podenvname)

	g.By("Checking connectivity from pod3 to pod2")
	CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2IPv4, "net1", pod2.podenvname)
	CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2IPv6, "net1", pod2.podenvname)

	podEnvName := []string{pod1.podenvname, pod2.podenvname, pod3.podenvname}
	podIPv4 := []string{pod1IPv4, pod2IPv4, pod3IPv4}
	podIPv6 := []string{pod1IPv6, pod2IPv6, pod3IPv6}
	return podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns1, nadName
}

func multihomingAfterCheck(oc *exutil.CLI, podName []string, podEnvName []string, podIPv4 []string, podIPv6 []string, ovnMasterPodName string, ns string, nadName string) {
	pod1Name := podName[0]
	pod2Name := podName[1]
	pod3Name := podName[2]
	pod1envname := podEnvName[0]
	pod2envname := podEnvName[1]
	pod3envname := podEnvName[2]
	pod1IPv4 := podIPv4[0]
	pod2IPv4 := podIPv4[1]
	pod3IPv4 := podIPv4[2]
	pod1IPv6 := podIPv6[0]
	pod2IPv6 := podIPv6[1]
	pod3IPv6 := podIPv6[2]

	g.By("Wait for the new ovn-northbound-leader pod is created")
	o.Eventually(func() string {
		podName := getOVNLeaderPod(oc, "north")
		return podName
	}, "120s", "5s").Should(o.ContainSubstring("ovnkube-master"), fmt.Sprintf("Failed to get correct OVN leader pod"))

	g.By("Checking connectivity from pod to pod after deleting  all ovnkube-node pods")
	e2e.Logf("pod1Name is %s", pod1Name)
	e2e.Logf("pod2IPv4 is  %s", pod2IPv4)
	e2e.Logf("pod2IPv4 is %s", pod2envname)
	e2e.Logf("ns is %s", ns)

	CurlMultusPod2PodPass(oc, ns, pod1Name, pod2IPv4, "net1", pod2envname)
	CurlMultusPod2PodPass(oc, ns, pod1Name, pod2IPv6, "net1", pod2envname)
	CurlMultusPod2PodPass(oc, ns, pod1Name, pod3IPv4, "net1", pod3envname)
	CurlMultusPod2PodPass(oc, ns, pod1Name, pod3IPv6, "net1", pod3envname)
	CurlMultusPod2PodPass(oc, ns, pod2Name, pod1IPv4, "net1", pod1envname)
	CurlMultusPod2PodPass(oc, ns, pod2Name, pod1IPv6, "net1", pod1envname)
	CurlMultusPod2PodPass(oc, ns, pod2Name, pod3IPv4, "net1", pod3envname)
	CurlMultusPod2PodPass(oc, ns, pod2Name, pod3IPv6, "net1", pod3envname)
	CurlMultusPod2PodPass(oc, ns, pod3Name, pod1IPv4, "net1", pod1envname)
	CurlMultusPod2PodPass(oc, ns, pod3Name, pod1IPv6, "net1", pod1envname)
	CurlMultusPod2PodPass(oc, ns, pod3Name, pod2IPv4, "net1", pod2envname)
	CurlMultusPod2PodPass(oc, ns, pod3Name, pod2IPv6, "net1", pod2envname)

	g.By("Check if the new OVN switch ports are deleted after deleting the pods")
	o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", ns).Execute()).NotTo(o.HaveOccurred())
	//After deleting pods, it will take several seconds to delete the switch ports
	ovnMasterPodNewName := getOVNLeaderPod(oc, "north")
	o.Expect(ovnMasterPodNewName).ShouldNot(o.Equal(""))
	listSWCmd := "ovn-nbctl show | grep port | grep " + nadName + " "
	o.Eventually(func() bool {
		listOutput, _ := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodNewName, listSWCmd)
		return checkOVNswitchPorts(podName, listOutput)
	}, 30*time.Second, 5*time.Second).ShouldNot(o.BeTrue(), "The correct OVN switch ports are not deleted")

	g.By("Check if the network-attach-defintion is deleted")
	o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns).Execute()).NotTo(o.HaveOccurred())
	if !checkNAD(oc, ns, nadName) {
		e2e.Logf("The correct network-attach-defintion: %v is deleted!", nadName)
	} else {
		e2e.Failf("The correct network-attach-defintion: %v is not deleted!", nadName)
	}

	g.By("Check if the new created OVN switch is deleted")
	o.Eventually(func() bool {
		return checkOVNSwitch(oc, nadName, ovnMasterPodNewName)
	}, 30*time.Second, 5*time.Second).ShouldNot(o.BeTrue(), "The correct OVN switch is not deleted")
}
