package networking

import (
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-multihoming", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60505-Multihoming Verify multihoming pods ipv4 connectivity", func() {
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

		nadName := "layer2ipv4network"
		nsWithnad := ns1 + "/" + nadName

		g.By("Create a custom resource network-attach-defintion in tested namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns1).Execute()
		nad1ns1 := multihomingNAD{
			namespace:      ns1,
			nadname:        nadName,
			subnets:        "192.168.100.0/24",
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
		if checkOVNSwitch(oc, nadName, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch is created")
		} else {
			e2e.Failf("The correct OVN switch is not created")
		}

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
		pod1IPv4, _ := getPodMultiNetwork(oc, ns1, pod1Name[0])
		e2e.Logf("The v4 address of pod1 is: %v", pod1IPv4)

		g.By("Get IPs from the pod2's secondary interface")
		pod2Name := getPodName(oc, ns1, "name=multihoming-pod2")
		pod2IPv4, _ := getPodMultiNetwork(oc, ns1, pod2Name[0])
		e2e.Logf("The v4 address of pod2 is: %v", pod2IPv4)

		g.By("Get IPs from the pod3's secondary interface")
		pod3Name := getPodName(oc, ns1, "name=multihoming-pod3")
		pod3IPv4, _ := getPodMultiNetwork(oc, ns1, pod3Name[0])
		e2e.Logf("The v4 address of pod3 is: %v", pod3IPv4)

		g.By("Check if the new OVN switch ports is created")
		listSWCmd := "ovn-nbctl show | grep port | grep " + nadName + " "
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		podname := []string{pod1Name[0], pod2Name[0], pod3Name[0]}
		if checkOVNswitchPorts(podname, listOutput) {
			e2e.Logf("The correct OVN switch ports are create")
		} else {
			e2e.Failf("The correct OVN switch ports are not created")
		}

		g.By("Checking connectivity from pod1 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv4)

		g.By("Checking connectivity from pod1 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3IPv4)

		g.By("Checking connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv4)

		g.By("Checking connectivity from pod2 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3IPv4)

		g.By("Checking connectivity from pod3 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1IPv4)

		g.By("Checking connectivity from pod3 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2IPv4)

		g.By("Check if the new OVN switch ports are deleted after deleting the pods")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		//After deleting pods, it will take several seconds to delete the switch ports
		o.Eventually(func() bool {
			listOutput, _ := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd)
			return checkOVNswitchPorts(podname, listOutput)
		}, 20*time.Second, 5*time.Second).ShouldNot(o.BeTrue(), "The correct OVN switch ports are not deleted")

		g.By("Check if the network-attach-defintion is deleted")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		if !checkNAD(oc, ns1, nadName) {
			e2e.Logf("The correct network-attach-defintion: %v is deleted!", nadName)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not deleted!", nadName)
		}

		g.By("Check if the new created OVN switch is deleted")
		if !checkOVNSwitch(oc, nadName, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch is deleted")
		} else {
			e2e.Failf("The correct OVN switch is not deleted")
		}
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60506-Multihoming Verify multihoming pods ipv6 connectivity", func() {
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

		nadName := "layer2ipv6network"
		nsWithnad := ns1 + "/" + nadName

		g.By("Create a custom resource network-attach-defintion in tested namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns1).Execute()
		nad1ns1 := multihomingNAD{
			namespace:      ns1,
			nadname:        nadName,
			subnets:        "fd00:dead:beef::0/64",
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
		if checkOVNSwitch(oc, nadName, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch is created")
		} else {
			e2e.Failf("The correct OVN switch is not created")
		}

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
		pod1IPv6 := getPodMultiNetworkIPv6(oc, ns1, pod1Name[0])
		e2e.Logf("The v4 address of pod1 is: %v", pod1IPv6)

		g.By("Get IPs from the pod2's secondary interface")
		pod2Name := getPodName(oc, ns1, "name=multihoming-pod2")
		pod2IPv6 := getPodMultiNetworkIPv6(oc, ns1, pod2Name[0])
		e2e.Logf("The v4 address of pod2 is: %v", pod2IPv6)

		g.By("Get IPs from the pod3's secondary interface")
		pod3Name := getPodName(oc, ns1, "name=multihoming-pod3")
		pod3IPv6 := getPodMultiNetworkIPv6(oc, ns1, pod3Name[0])
		e2e.Logf("The v4 address of pod3 is: %v", pod3IPv6)

		g.By("Check if the new OVN switch ports is created")
		listSWCmd := "ovn-nbctl show | grep port | grep " + nadName + " "
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		podname := []string{pod1Name[0], pod2Name[0], pod3Name[0]}
		if checkOVNswitchPorts(podname, listOutput) {
			e2e.Logf("The correct OVN switch ports are create")
		} else {
			e2e.Failf("The correct OVN switch ports are not created")
		}

		g.By("Checking connectivity from pod1 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv6)

		g.By("Checking connectivity from pod1 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3IPv6)

		g.By("Checking connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv6)

		g.By("Checking connectivity from pod2 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3IPv6)

		g.By("Checking connectivity from pod3 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1IPv6)

		g.By("Checking connectivity from pod3 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2IPv6)

		g.By("Check if the new OVN switch ports are deleted after deleting the pods")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		//After deleting pods, it will take several seconds to delete the switch ports
		o.Eventually(func() bool {
			listOutput, _ := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd)
			return checkOVNswitchPorts(podname, listOutput)
		}, 20*time.Second, 5*time.Second).ShouldNot(o.BeTrue(), "The correct OVN switch ports are not deleted")

		g.By("Check if the network-attach-defintion is deleted")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		if !checkNAD(oc, ns1, nadName) {
			e2e.Logf("The correct network-attach-defintion: %v is deleted!", nadName)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not deleted!", nadName)
		}

		g.By("Check if the new created OVN switch is deleted")
		if !checkOVNSwitch(oc, nadName, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch is deleted")
		} else {
			e2e.Failf("The correct OVN switch is not deleted")
		}
	})
})
