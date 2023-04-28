package networking

import (
	"fmt"
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
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv4, "net1", pod2.podenvname)

		g.By("Checking connectivity from pod1 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3IPv4, "net1", pod3.podenvname)

		g.By("Checking connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv4, "net1", pod1.podenvname)

		g.By("Checking connectivity from pod2 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3IPv4, "net1", pod3.podenvname)

		g.By("Checking connectivity from pod3 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1IPv4, "net1", pod1.podenvname)

		g.By("Checking connectivity from pod3 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2IPv4, "net1", pod2.podenvname)

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
		e2e.Logf("The v6 address of pod1 is: %v", pod1IPv6)

		g.By("Get IPs from the pod2's secondary interface")
		pod2Name := getPodName(oc, ns1, "name=multihoming-pod2")
		pod2IPv6 := getPodMultiNetworkIPv6(oc, ns1, pod2Name[0])
		e2e.Logf("The v6 address of pod2 is: %v", pod2IPv6)

		g.By("Get IPs from the pod3's secondary interface")
		pod3Name := getPodName(oc, ns1, "name=multihoming-pod3")
		pod3IPv6 := getPodMultiNetworkIPv6(oc, ns1, pod3Name[0])
		e2e.Logf("The v6 address of pod3 is: %v", pod3IPv6)

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
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv6, "net1", pod2.podenvname)

		g.By("Checking connectivity from pod1 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3IPv6, "net1", pod3.podenvname)

		g.By("Checking connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv6, "net1", pod1.podenvname)

		g.By("Checking connectivity from pod2 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3IPv6, "net1", pod3.podenvname)

		g.By("Checking connectivity from pod3 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1IPv6, "net1", pod1.podenvname)

		g.By("Checking connectivity from pod3 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2IPv6, "net1", pod2.podenvname)

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
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60507-Multihoming Verify multihoming pods dualstack connectivity", func() {
		var podName, podEnvName, podIPv4, podIPv6 []string
		var ovnMasterPodName, ns, nadName string

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns).Execute()
		podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName = multihomingBeforeCheck(oc)
		multihomingAfterCheck(oc, podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName)
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60508-Multihoming Verify excludeSubnets for multihoming pods’ ipv4 address", func() {
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

		nadName := "layer2excludeipv4network"
		nsWithnad := ns1 + "/" + nadName

		g.By("Create a custom resource network-attach-defintion in tested namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns1).Execute()
		nad1ns1 := multihomingNAD{
			namespace:      ns1,
			nadname:        nadName,
			subnets:        "192.168.10.0/29",
			nswithnadname:  nsWithnad,
			excludeSubnets: "192.168.10.0/30,192.168.10.6/32",
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
		pod3Name := getPodName(oc, ns1, "name=multihoming-pod3")
		o.Eventually(func() string {
			podStatus, _ := getPodStatus(oc, ns1, pod3Name[0])
			return podStatus
		}, 20*time.Second, 5*time.Second).Should(o.Equal("Pending"), fmt.Sprintf("Pod: %s should not be in Running state", pod3Name[0]))

		g.By("Get IPs from the pod1's secondary interface")
		pod1Name := getPodName(oc, ns1, "name=multihoming-pod1")
		pod1IPv4, _ := getPodMultiNetwork(oc, ns1, pod1Name[0])
		e2e.Logf("The v4 address of pod1 is: %v", pod1IPv4)
		if !strings.Contains(pod1IPv4, "192.168.10.4") {
			e2e.Failf("Pod: %s does not get correct ipv4 address", pod1Name[0])
		}

		g.By("Get IPs from the pod2's secondary interface")
		pod2Name := getPodName(oc, ns1, "name=multihoming-pod2")
		pod2IPv4, _ := getPodMultiNetwork(oc, ns1, pod2Name[0])
		e2e.Logf("The v4 address of pod2 is: %v", pod2IPv4)
		if !strings.Contains(pod2IPv4, "192.168.10.5") {
			e2e.Failf("Pod: %s does not get correct ipv4 address", pod2Name[0])
		}

		g.By("Checking connectivity from pod1 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv4, "net1", pod2.podenvname)

		g.By("Checking connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv4, "net1", pod1.podenvname)
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60509-Multihoming Verify excludeSubnets for multihoming pods’ ipv6 address", func() {
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

		nadName := "layer2excludeipv6network"
		nsWithnad := ns1 + "/" + nadName

		g.By("Create a custom resource network-attach-defintion in tested namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns1).Execute()
		nad1ns1 := multihomingNAD{
			namespace:      ns1,
			nadname:        nadName,
			subnets:        "fd00:dead:beef:1::0/126",
			nswithnadname:  nsWithnad,
			excludeSubnets: "fd00:dead:beef:1::0/127",
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
		pod3Name := getPodName(oc, ns1, "name=multihoming-pod3")
		o.Eventually(func() string {
			podStatus, _ := getPodStatus(oc, ns1, pod3Name[0])
			return podStatus
		}, 20*time.Second, 5*time.Second).Should(o.Equal("Pending"), fmt.Sprintf("Pod: %s should not be in Running state", pod3Name[0]))

		g.By("Get IPs from the pod1's secondary interface")
		pod1Name := getPodName(oc, ns1, "name=multihoming-pod1")
		pod1IPv6 := getPodMultiNetworkIPv6(oc, ns1, pod1Name[0])
		e2e.Logf("The v6 address of pod1 is: %v", pod1IPv6)
		if !strings.Contains(pod1IPv6, "fd00:dead:beef:1::2") {
			e2e.Failf("Pod: %s does not get correct ipv4 address", pod1Name[0])
		}

		g.By("Get IPs from the pod2's secondary interface")
		pod2Name := getPodName(oc, ns1, "name=multihoming-pod2")
		pod2IPv6 := getPodMultiNetworkIPv6(oc, ns1, pod2Name[0])
		e2e.Logf("The v6 address of pod2 is: %v", pod2IPv6)
		if !strings.Contains(pod2IPv6, "fd00:dead:beef:1::3") {
			e2e.Failf("Pod: %s does not get correct ipv4 address", pod2Name[0])
		}

		g.By("Checking connectivity from pod1 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2IPv6, "net1", pod2.podenvname)

		g.By("Checking connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1IPv6, "net1", pod1.podenvname)
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-62548-Multihoming Verify multihoming pods with multiple attachments to the different OVN-K networks", func() {
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

		nadName1 := "layer2dualstacknetwork1"
		nsWithnad1 := ns1 + "/" + nadName1
		nadName2 := "layer2dualstacknetwork2"
		nsWithnad2 := ns1 + "/" + nadName2
		nadName3 := nadName1 + "," + nadName2

		g.By("Create two custom resource network-attach-defintions in tested namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName1, "-n", ns1).Execute()
		nad1ns1 := multihomingNAD{
			namespace:      ns1,
			nadname:        nadName1,
			subnets:        "192.168.100.0/24,fd00:dead:beef::0/64",
			nswithnadname:  nsWithnad1,
			excludeSubnets: "",
			topology:       "layer2",
			template:       multihomingNADTemplate,
		}
		nad1ns1.createMultihomingNAD(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName2, "-n", ns1).Execute()
		nad1ns2 := multihomingNAD{
			namespace:      ns1,
			nadname:        nadName2,
			subnets:        "192.168.110.0/24,fd00:dead:beee::0/64",
			nswithnadname:  nsWithnad2,
			excludeSubnets: "",
			topology:       "layer2",
			template:       multihomingNADTemplate,
		}
		nad1ns2.createMultihomingNAD(oc)

		g.By("Check if two network-attach-defintions are created")
		if checkNAD(oc, ns1, nadName1) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nadName1)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nadName1)
		}
		if checkNAD(oc, ns1, nadName2) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nadName2)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nadName2)
		}

		g.By("Check if two new OVN switchs are created")
		ovnMasterPodName := getOVNLeaderPod(oc, "north")
		o.Expect(ovnMasterPodName).ShouldNot(o.Equal(""))
		if checkOVNSwitch(oc, nadName1, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch: %s is created", nadName1)
		} else {
			e2e.Failf("The correct OVN switch: %s is not created", nadName1)
		}
		if checkOVNSwitch(oc, nadName2, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch: %s is created", nadName2)
		} else {
			e2e.Failf("The correct OVN switch: %s is not created", nadName2)
		}

		g.By("Create 1st pod consuming above network-attach-defintions in ns1")
		pod1 := testMultihomingPod{
			name:       "multihoming-pod-1",
			namespace:  ns1,
			podlabel:   "multihoming-pod1",
			nadname:    nadName3,
			nodename:   nodeList.Items[0].Name,
			podenvname: "Hello multihoming-pod-1",
			template:   multihomingPodTemplate,
		}
		pod1.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=multihoming-pod1")).NotTo(o.HaveOccurred())

		g.By("Create 2nd pod consuming above network-attach-defintions in ns1")
		pod2 := testMultihomingPod{
			name:       "multihoming-pod-2",
			namespace:  ns1,
			podlabel:   "multihoming-pod2",
			nadname:    nadName3,
			nodename:   nodeList.Items[0].Name,
			podenvname: "Hello multihoming-pod-2",
			template:   multihomingPodTemplate,
		}
		pod2.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=multihoming-pod2")).NotTo(o.HaveOccurred())

		g.By("Create 3rd pod consuming above network-attach-defintions in ns1")
		pod3 := testMultihomingPod{
			name:       "multihoming-pod-3",
			namespace:  ns1,
			podlabel:   "multihoming-pod3",
			nadname:    nadName3,
			nodename:   nodeList.Items[1].Name,
			podenvname: "Hello multihoming-pod-3",
			template:   multihomingPodTemplate,
		}
		pod3.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=multihoming-pod3")).NotTo(o.HaveOccurred())

		g.By("Get IPs from the pod1's net1 interface")
		pod1Name := getPodName(oc, ns1, "name=multihoming-pod1")
		pod1Net1IPv4, pod1Net1IPv6 := getPodMultiNetworks(oc, ns1, pod1Name[0], "net1")
		e2e.Logf("The v4 address of pod1 is: %v", pod1Net1IPv4)
		e2e.Logf("The v6 address of pod1 is: %v", pod1Net1IPv6)

		g.By("Get IPs from the pod2's net1 interface")
		pod2Name := getPodName(oc, ns1, "name=multihoming-pod2")
		pod2Net1IPv4, pod2Net1IPv6 := getPodMultiNetworks(oc, ns1, pod2Name[0], "net1")
		e2e.Logf("The v4 address of pod1 is: %v", pod2Net1IPv4)
		e2e.Logf("The v6 address of pod1 is: %v", pod2Net1IPv6)

		g.By("Get IPs from the pod3's net1 interface")
		pod3Name := getPodName(oc, ns1, "name=multihoming-pod3")
		pod3Net1IPv4, pod3Net1IPv6 := getPodMultiNetworks(oc, ns1, pod3Name[0], "net1")
		e2e.Logf("The v4 address of pod1 is: %v", pod3Net1IPv4)
		e2e.Logf("The v6 address of pod1 is: %v", pod3Net1IPv6)

		g.By("Check if the new OVN switch ports is created")
		listSWCmd := "ovn-nbctl show | grep port | grep " + nadName1 + " "
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		podname := []string{pod1Name[0], pod2Name[0], pod3Name[0]}
		if checkOVNswitchPorts(podname, listOutput) {
			e2e.Logf("The correct OVN switch ports are create")
		} else {
			e2e.Failf("The correct OVN switch ports are not created")
		}
		listSWCmd1 := "ovn-nbctl show | grep port | grep " + nadName2 + " "
		listOutput1, listErr1 := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd1)
		o.Expect(listErr1).NotTo(o.HaveOccurred())
		if checkOVNswitchPorts(podname, listOutput1) {
			e2e.Logf("The correct OVN switch ports are create")
		} else {
			e2e.Failf("The correct OVN switch ports are not created")
		}

		g.By("Checking net1 connectivity from pod1 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2Net1IPv4, "net1", pod2.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2Net1IPv6, "net1", pod2.podenvname)

		g.By("Checking net1 connectivity from pod1 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3Net1IPv4, "net1", pod3.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3Net1IPv6, "net1", pod3.podenvname)

		g.By("Checking net1 connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1Net1IPv4, "net1", pod1.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1Net1IPv6, "net1", pod1.podenvname)

		g.By("Checking net1 connectivity from pod2 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3Net1IPv4, "net1", pod3.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3Net1IPv6, "net1", pod3.podenvname)

		g.By("Checking net1 connectivity from pod3 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1Net1IPv4, "net1", pod1.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1Net1IPv6, "net1", pod1.podenvname)

		g.By("Checking net1 connectivity from pod3 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2Net1IPv4, "net1", pod2.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2Net1IPv6, "net1", pod2.podenvname)

		g.By("Get IPs from the pod1's net2 interface")
		pod1Net2IPv4, pod1Net2IPv6 := getPodMultiNetworks(oc, ns1, pod1Name[0], "net2")
		e2e.Logf("The v4 address of pod1 is: %v", pod1Net2IPv4, pod1.podenvname)
		e2e.Logf("The v6 address of pod1 is: %v", pod1Net2IPv6, pod1.podenvname)

		g.By("Get IPs from the pod2's net2 interface")
		pod2Net2IPv4, pod2Net2IPv6 := getPodMultiNetworks(oc, ns1, pod2Name[0], "net2")
		e2e.Logf("The v4 address of pod1 is: %v", pod2Net2IPv4, pod2.podenvname)
		e2e.Logf("The v6 address of pod1 is: %v", pod2Net2IPv6, pod2.podenvname)

		g.By("Get IPs from the pod3's net2 interface")
		pod3Net2IPv4, pod3Net2IPv6 := getPodMultiNetworks(oc, ns1, pod3Name[0], "net2")
		e2e.Logf("The v4 address of pod1 is: %v", pod3Net2IPv4)
		e2e.Logf("The v6 address of pod1 is: %v", pod3Net2IPv6)

		g.By("Checking net2 connectivity from pod1 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2Net2IPv4, "net2", pod2.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod2Net2IPv6, "net2", pod2.podenvname)

		g.By("Checking net2 connectivity from pod1 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3Net2IPv4, "net2", pod3.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod1Name[0], pod3Net2IPv6, "net2", pod3.podenvname)

		g.By("Checking net2 connectivity from pod2 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1Net2IPv4, "net2", pod1.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod1Net2IPv6, "net2", pod1.podenvname)

		g.By("Checking net2 connectivity from pod2 to pod3")
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3Net2IPv4, "net2", pod3.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod2Name[0], pod3Net2IPv6, "net2", pod3.podenvname)

		g.By("Checking net2 connectivity from pod3 to pod1")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1Net2IPv4, "net2", pod1.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod1Net2IPv6, "net2", pod1.podenvname)

		g.By("Checking net2 connectivity from pod3 to pod2")
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2Net2IPv4, "net2", pod2.podenvname)
		CurlMultusPod2PodPass(oc, ns1, pod3Name[0], pod2Net2IPv6, "net2", pod2.podenvname)

		//Check no pods connectivity cross two OVN-K networks in layer2 topology
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod2Net1IPv4, "net2", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod2Net1IPv6, "net2", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod2Net2IPv4, "net1", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod2Net2IPv6, "net1", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod3Net1IPv4, "net2", pod3.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod3Net1IPv6, "net2", pod3.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod3Net2IPv4, "net1", pod3.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod1Name[0], pod3Net2IPv6, "net1", pod3.podenvname)

		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod1Net1IPv4, "net2", pod1.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod1Net1IPv6, "net2", pod1.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod1Net2IPv4, "net1", pod1.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod1Net2IPv6, "net1", pod1.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod3Net1IPv4, "net2", pod3.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod3Net1IPv6, "net2", pod3.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod3Net2IPv4, "net1", pod3.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod2Name[0], pod3Net2IPv6, "net1", pod3.podenvname)

		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod2Net1IPv4, "net2", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod2Net1IPv6, "net2", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod2Net2IPv4, "net1", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod2Net2IPv6, "net1", pod2.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod1Net1IPv4, "net2", pod1.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod1Net1IPv6, "net2", pod1.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod1Net2IPv4, "net1", pod1.podenvname)
		CurlMultusPod2PodFail(oc, ns1, pod3Name[0], pod1Net2IPv6, "net1", pod1.podenvname)

		g.By("Check if the new OVN switch ports are deleted after deleting the pods")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		//After deleting pods, it will take several seconds to delete the switch ports
		o.Eventually(func() bool {
			listOutput, _ := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd)
			return checkOVNswitchPorts(podname, listOutput)
		}, 20*time.Second, 5*time.Second).ShouldNot(o.BeTrue(), "The correct OVN switch ports are not deleted")
		o.Eventually(func() bool {
			listOutput, _ := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listSWCmd1)
			return checkOVNswitchPorts(podname, listOutput)
		}, 20*time.Second, 5*time.Second).ShouldNot(o.BeTrue(), "The correct OVN switch ports are not deleted")

		g.By("Check if the network-attach-defintion is deleted")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName1, "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		if !checkNAD(oc, ns1, nadName1) {
			e2e.Logf("The correct network-attach-defintion: %v is deleted!", nadName1)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not deleted!", nadName1)
		}
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName2, "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		if !checkNAD(oc, ns1, nadName2) {
			e2e.Logf("The correct network-attach-defintion: %v is deleted!", nadName2)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not deleted!", nadName2)
		}

		g.By("Check if the new created OVN switch is deleted")
		if !checkOVNSwitch(oc, nadName1, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch: %v is deleted!", nadName1)
		} else {
			e2e.Failf("The correct OVN switch: %v is not deleted!", nadName1)
		}
		if !checkOVNSwitch(oc, nadName2, ovnMasterPodName) {
			e2e.Logf("The correct OVN switch: %v is deleted!", nadName2)
		} else {
			e2e.Failf("The correct OVN switch: %v is not deleted!", nadName2)
		}
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60511-Multihoming Verify multihoming pods’ dualstack connectivity after deleting ovn-northbound-leader pod. [Disruptive]", func() {
		var podName, podEnvName, podIPv4, podIPv6 []string
		var ovnMasterPodName, ns, nadName string

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns).Execute()
		podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName = multihomingBeforeCheck(oc)

		g.By("Delete ovn-northbound-leader pod")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", ovnMasterPodName, "-n", "openshift-ovn-kubernetes").Execute()).NotTo(o.HaveOccurred())

		multihomingAfterCheck(oc, podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName)

	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60512-Multihoming Verify multihoming pods’ dualstack connectivity after deleting all ovnkube-master pods. [Disruptive]", func() {
		var podName, podEnvName, podIPv4, podIPv6 []string
		var ovnMasterPodName, ns, nadName string

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns).Execute()
		podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName = multihomingBeforeCheck(oc)

		g.By("Delete  all ovnkube-master pods")
		ovnMasterPodNames := getPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-master")
		for _, ovnPod := range ovnMasterPodNames {
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", ovnPod, "-n", "openshift-ovn-kubernetes").Execute()).NotTo(o.HaveOccurred())
		}

		multihomingAfterCheck(oc, podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName)
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-60516-Multihoming Verify multihoming pods’ dualstack connectivity after deleting all ovnkube-node pods. [Disruptive]", func() {
		var podName, podEnvName, podIPv4, podIPv6 []string
		var ovnMasterPodName, ns, nadName string

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns).Execute()
		podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName = multihomingBeforeCheck(oc)

		g.By("Delete all ovnkube-node pods")
		ovnNodePodNames := getPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		for _, ovnPod := range ovnNodePodNames {
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", ovnPod, "-n", "openshift-ovn-kubernetes").Execute()).NotTo(o.HaveOccurred())
		}

		multihomingAfterCheck(oc, podName, podEnvName, podIPv4, podIPv6, ovnMasterPodName, ns, nadName)
	})
})
