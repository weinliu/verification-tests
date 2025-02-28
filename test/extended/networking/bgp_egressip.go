package networking

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN bgp egressIP", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		host           = "openshift-qe-028.lab.eng.rdu2.redhat.com"
		externalFRRIP  string
		externalFRRIP2 string
		allNodes       []string
		podNetwork1Map = make(map[string]string)
		podNetwork2Map = make(map[string]string)
		nodesIP1Map    = make(map[string]string)
		nodesIP2Map    = make(map[string]string)
		allNodesIP2    []string
		allNodesIP1    []string
		frrNamespace   = "openshift-frr-k8s"
		intf           string
	)

	g.JustBeforeEach(func() {
		var (
			nodeErr error
		)

		SkipIfNoFeatureGate(oc, "RouteAdvertisements")
		if !IsFrrRouteAdvertisementEnabled(oc) || !areFRRPodsReady(oc, frrNamespace) {
			g.Skip("FRR routeAdvertisement is still not enabled on the cluster, or FRR pods are not ready, skip the test!!!")
		}

		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		if err != nil || !(strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
			g.Skip("This case will only run on rdu1 or rdu2 cluster, skip for other test envrionment!!!")
		}
		intf = "sriovbm"

		if strings.Contains(msg, "offload.openshift-qe.sdn.com") {
			host = "openshift-qe-026.lab.eng.rdu2.redhat.com"
			intf = "offloadbm"
		}

		exutil.By("Get IPs of all cluster nodes, and IP map of all nodes")
		allNodes, nodeErr = exutil.GetAllNodes(oc)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		o.Expect(len(allNodes)).NotTo(o.BeEquivalentTo(0))
		nodesIP2Map, nodesIP1Map, allNodesIP2, allNodesIP1 = getNodeIPMAP(oc, allNodes)
		o.Expect(len(nodesIP2Map)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(nodesIP1Map)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(allNodesIP2)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(allNodesIP1)).NotTo(o.BeEquivalentTo(0))

		exutil.By("Get external FRR IP, create external FRR container on the host with external FRR IP and cluster nodes' IPs")
		externalFRRIP2, externalFRRIP = getExternalFRRIP(oc, allNodesIP2, allNodesIP1, host)
		o.Expect(externalFRRIP).NotTo(o.BeEmpty())
		ipStackType := checkIPStackType(oc)
		if ipStackType == "dualstack" {
			o.Expect(externalFRRIP2).NotTo(o.BeEmpty())
		}

		exutil.By("Get default podNetworks of all cluster nodes")
		podNetwork2Map, podNetwork1Map = getHostPodNetwork(oc, allNodes, "default")
		o.Expect(len(podNetwork2Map)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(podNetwork1Map)).NotTo(o.BeEquivalentTo(0))

		exutil.By("Verify default network is advertised")
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "30s", "5s").Should(o.BeTrue(), "BGP route advertisement of default network did not succeed!!")
		e2e.Logf("SUCCESS - BGP enabled, default network is advertised!!!")

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-79715-Legacy egressIP (using unused IP from same node subnet) on default network is adverised and works as expected (singlestack) [Serial]", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP2Template   = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressNodeLabel     = "k8s.ovn.org/egress-assignable"
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		ipStackType := checkIPStackType(oc)
		// dualstack scenario will be covered in a separate test case
		if (ipStackType == "ipv4single" || ipStackType == "ipv6single") && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on singlev4 or singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("1. Label an egressNode, get a namespace, label the namespace with org=qe to match namespaceSelector of egressIP object that will be created in step 6")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")

		ns1 := oc.Namespace()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Create an egressip object with an usused IP from same subnet of egress node, verify egressIP is assigned to egress node")
		var freeIPs []string
		if ipStackType == "ipv4single" {
			freeIPs = findFreeIPs(oc, nodeList.Items[0].Name, 1)
		}
		if ipStackType == "ipv6single" {
			freeIPs = findFreeIPv6s(oc, nodeList.Items[0].Name, 1)
		}
		o.Expect(len(freeIPs)).Should(o.Equal(1))

		egressip1 := egressIPResource1{
			name:          "egressip-79715",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[0].Name))

		// Due to https://issues.redhat.com/browse/OCPBUGS-45933, temporarily comment out the verification of advertisement of legacy egressIP
		// exutil.By("3. Verify egressIP address is advertised to external frr router")
		// nodeIPEgressNode := getNodeIPv4(oc, "default", nodeList.Items[0].Name)
		// o.Eventually(func() bool {
		// 	result := verifySingleIPRouteOnExternalFrr(host, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
		// 	return result
		// }, "90s", "15s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to external frr!!")

		// exutil.By("4. Verify egressIP address is advertised to all other cluster nodes")
		// o.Eventually(func() bool {
		// 	result := verifySingleIPRoutesOnClusterNode(oc, egressIPMaps1[0]["node"], allNodes, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
		// 	return result
		// }, "90s", "15s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to all other cluster nodes!!")

		// we need to give some time to let egressIP route advertisement take effect while we skip its route advertisement check due to OCPBUGS-45933
		time.Sleep(60 * time.Second)

		exutil.By("5.1 Create two test pods, add to them with label color=pink to match podSelector of egressIP, one local and another remote to the egress node")
		EIPPods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			EIPPods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i) + "-eip-" + ns1,
				namespace: ns1,
				nodename:  nodeList.Items[i].Name,
				template:  pingPodNodeTemplate,
			}
			EIPPods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, EIPPods[i].name)
			defer exutil.LabelPod(oc, ns1, EIPPods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, EIPPods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.2. Verify egressIP works from local or remote pod to egress node")
		e2e.Logf("Trying to get physical interface on the egressNode: %s", nodeList.Items[0].Name)
		primaryInf, infErr := getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		externalPublicHost := "www.google.com"
		var tcpdumpCmd, cmdOnPod string
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -k " + externalPublicHost
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -6 -k " + externalPublicHost
		}

		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("6.1. Create a 3rd test pod on egress node but do not label it so this pod will not use egressIP")
		nonEIPPod := pingPodResourceNode{
			name:      "hello-pod" + "-non-eip-" + ns1,
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		nonEIPPod.createPingPodNode(oc)
		waitPodReady(oc, ns1, nonEIPPod.name)

		_, nonEIPPodIP1 := getPodIP(oc, ns1, nonEIPPod.name)

		exutil.By("6.2. Verify the non-EIP pod does not use EIP as source IP in its egressing packets, it directly uses its own podIP as sourceIP")
		tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "Pod that unqualified to use egressIP did not use its podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeFalse(), "Pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")

		exutil.By("7. Label the second node with egressNodeLabel, unlabel the first node, verify the new egress node is updated in the egressip object.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel, "true")

		egressipErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 360*time.Second, false, func(cxt context.Context) (bool, error) {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps1) != 1 || egressIPMaps1[0]["node"] == nodeList.Items[0].Name {
				e2e.Logf("Wait for egressIP being applied to new egress node,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[2].Name))

		exutil.By("8. From local and remote EIP pods, validate egressIP on new egressNode after egressIP failover \n")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[2].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[2].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("9. Verify that after egressIP failover, the non-EIP pod still just uses its own podIP as sourceIP")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "After failover, pod that unqualified to use egressIP did not use its podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeFalse(), "After failover, pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-79089-egressIP using unused random IP on default network is adverised and works as expected (singlestack) [Serial]", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP2Template   = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressNodeLabel     = "k8s.ovn.org/egress-assignable"
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		ipStackType := checkIPStackType(oc)
		// dualstack scenario will be covered in a separate test case
		if (ipStackType == "ipv4single" || ipStackType == "ipv6single") && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on singlev4 or singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("1. Label an egressNode, get a namespace, label the namespace with org=qe to match namespaceSelector of egressIP object that will be created in step 6")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")

		ns1 := oc.Namespace()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Create an egressip object with a random unused IP, verify egressIP is assigned to egress node")
		var freeIP string
		if ipStackType == "ipv4single" {
			freeIP = generateRandomUnusedPublicIPv4(oc)
		}
		if ipStackType == "ipv6single" {
			freeIP = generateRandomGlobalIPv6(oc)
		}

		egressip1 := egressIPResource1{
			name:          "egressip-79089",
			template:      egressIP2Template,
			egressIP1:     freeIP,
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[0].Name))

		exutil.By("3. Verify egressIP address is advertised to external frr router")
		nodeIPEgressNode := getNodeIPv4(oc, "default", nodeList.Items[0].Name)
		o.Eventually(func() bool {
			result := verifySingleBGPRouteOnExternalFrr(host, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
			return result
		}, "90s", "15s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to external frr!!")

		exutil.By("4. Verify egressIP address is advertised to all other cluster nodes")
		o.Eventually(func() bool {
			result := verifySingleBGPRouteOnClusterNode(oc, egressIPMaps1[0]["node"], allNodes, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
			return result
		}, "90s", "5s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to all other cluster nodes!!")

		exutil.By("5. Add iptables rules to hypervisor to assist the test")
		var ipnet *net.IPNet
		if ipStackType == "ipv4single" {
			_, ipnet, err = net.ParseCIDR(freeIP + "/32")
		}
		if ipStackType == "ipv6single" {
			_, ipnet, err = net.ParseCIDR(freeIP + "/128")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		ruleDel1 := "sudo iptables -t filter -D FORWARD -s " + ipnet.String() + " -i " + intf + " -j ACCEPT"
		ruleDel2 := "sudo iptables -t filter -D FORWARD -d " + ipnet.String() + " -o " + intf + " -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
		var ruleDel3 string
		if ipStackType == "ipv4single" {
			ruleDel3 = "sudo iptables -t nat -D POSTROUTING -s " + ipnet.String() + " ! -d " + externalFRRIP + "/24 -j MASQUERADE"
		}
		if ipStackType == "ipv6single" {
			ruleDel3 = "sudo iptables -t nat -D POSTROUTING -s " + ipnet.String() + " ! -d " + externalFRRIP + "/64 -j MASQUERADE"
		}

		defer sshRunCmd(host, "root", ruleDel1)
		defer sshRunCmd(host, "root", ruleDel2)
		defer sshRunCmd(host, "root", ruleDel3)
		err = addIPtablesRules(host, intf, freeIP, externalFRRIP)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6.1 Create two test pods, add to them with label color=pink to match podSelector of egressIP, one local and another remote to the egress node")
		EIPPods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			EIPPods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i) + "-eip-" + ns1,
				namespace: ns1,
				nodename:  nodeList.Items[i].Name,
				template:  pingPodNodeTemplate,
			}
			EIPPods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, EIPPods[i].name)
			defer exutil.LabelPod(oc, ns1, EIPPods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, EIPPods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6.2. Verify egressIP works from local or remote pod to egress node")
		e2e.Logf("Trying to get physical interface on the egressNode: %s", nodeList.Items[0].Name)
		primaryInf, infErr := getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		externalPublicHost := "www.google.com"

		var tcpdumpCmd, cmdOnPod string
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -k " + externalPublicHost
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -6 -k " + externalPublicHost
		}

		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeTrue())
		}

		exutil.By("7.1. Create a 3rd test pod on egress node but do not label it so this pod will not use egressIP")
		nonEIPPod := pingPodResourceNode{
			name:      "hello-pod" + "-non-eip-" + ns1,
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		nonEIPPod.createPingPodNode(oc)
		waitPodReady(oc, ns1, nonEIPPod.name)

		_, nonEIPPodIP1 := getPodIP(oc, ns1, nonEIPPod.name)

		exutil.By("7.2. Verify the non-EIP pod does not use EIP as source IP in its egressing packets, it directly uses its own podIP as sourceIP")
		tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "Pod that unqualified to use egressIP did not use its podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeFalse(), "Pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")

		exutil.By("8. Label the second node with egressNodeLabel, unlabel the first node, verify the new egress node is updated in the egressip object.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel, "true")

		egressipErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 360*time.Second, false, func(cxt context.Context) (bool, error) {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps1) != 1 || egressIPMaps1[0]["node"] == nodeList.Items[0].Name {
				e2e.Logf("Wait for egressIP being applied to new egress node,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[2].Name))

		exutil.By("9. From local and remote EIP pods, validate egressIP on new egressNode after egressIP failover \n")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[2].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[2].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeTrue())
		}

		exutil.By("10. Verify that after egressIP failover, the non-EIP pod still just uses its own podIP as sourceIP")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "After failover, pod that unqualified to use egressIP did not use its podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeFalse(), "After failover, pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-79766-Legacy egressIP (using unused IP from same node subnet) on UDN is adverised and works as expected (singlestack) [Serial]", func() {

		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking")
			raTemplate           = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			egressIP2Template    = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			pingPodNodeTemplate  = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressNodeLabel      = "k8s.ovn.org/egress-assignable"
			udnName              = "layer3-udn-79766"
			networkselectorkey   = "app"
			networkselectorvalue = "udn"
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		ipStackType := checkIPStackType(oc)
		// dualstack scenario will be covered in a separate test case
		if (ipStackType == "ipv4single" || ipStackType == "ipv6single") && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on singlev4 or singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("1. Create a UDN namespace, create a layer3 UDN in the UDN namespace")
		exutil.By("Create namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		exutil.By("Create CRD for UDN")
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
			}
		}

		createGeneralUDNCRD(oc, ns1, udnName, ipv4cidr, ipv6cidr, cidr, "layer3")

		exutil.By("2. Label the UDN with label that matches networkSelector in routeAdvertisement")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "userdefinednetwork", udnName, networkselectorkey+"-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "userdefinednetwork", udnName, networkselectorkey+"="+networkselectorvalue).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Apply a routeAdvertisement with matching networkSelector")
		raname := "ra-udn"
		params := []string{"-f", raTemplate, "-p", "NAME=" + raname, "NETWORKSELECTORKEY=" + networkselectorkey, "NETWORKSELECTORVALUE=" + networkselectorvalue}
		defer removeResource(oc, true, true, "ra", raname)
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		raErr := checkRAStatus(oc, raname, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - UDN routeAdvertisement applied is accepted")

		exutil.By("4. Label an egressNode, get a namespace, label the namespace with org=qe to match namespaceSelector of egressIP object that will be created in next step")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. Create an egressip object with an unused IP from same subnet of egress node, verify egressIP is assigned to egress node")
		var freeIPs []string
		if ipStackType == "ipv4single" {
			freeIPs = findFreeIPs(oc, nodeList.Items[0].Name, 1)
		}
		if ipStackType == "ipv6single" {
			freeIPs = findFreeIPv6s(oc, nodeList.Items[0].Name, 1)
		}
		o.Expect(len(freeIPs)).Should(o.Equal(1))

		egressip1 := egressIPResource1{
			name:          "egressip-79766",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[0].Name))

		// Due to https://issues.redhat.com/browse/OCPBUGS-45933, temporarily comment out the verification of advertisement of legacy egressIP
		// exutil.By("6. Verify egressIP address is advertised to external frr router")
		// nodeIPEgressNode := getNodeIPv4(oc, "default", nodeList.Items[0].Name)
		// o.Eventually(func() bool {
		// 	result := verifySingleIPRouteOnExternalFrr(host, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
		// 	return result
		// }, "90s", "15s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to external frr!!")

		// exutil.By("4. Verify egressIP address is advertised to all other cluster nodes")
		// o.Eventually(func() bool {
		// 	result := verifySingleIPRoutesOnClusterNode(oc, egressIPMaps1[0]["node"], allNodes, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
		// 	return result
		// }, "90s", "15s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to all other cluster nodes!!")

		// we need to give some time to let egressIP route advertisement take effect while we skip its route advertisement check due to OCPBUGS-45933
		time.Sleep(60 * time.Second)

		exutil.By("7.1 Create two test pods, add to them with label color=pink to match podSelector of egressIP, one local and another remote to the egress node")
		EIPPods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			EIPPods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i) + "-eip-" + ns1,
				namespace: ns1,
				nodename:  nodeList.Items[i].Name,
				template:  pingPodNodeTemplate,
			}
			EIPPods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, EIPPods[i].name)
			defer exutil.LabelPod(oc, ns1, EIPPods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, EIPPods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("7.2. Verify egressIP works from local or remote pod to egress node")
		e2e.Logf("Trying to get physical interface on the egressNode: %s", nodeList.Items[0].Name)
		primaryInf, infErr := getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		externalPublicHost := "www.google.com"
		var tcpdumpCmd, cmdOnPod string
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -k " + externalPublicHost
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -6 -k " + externalPublicHost
		}

		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("8.1. Create a 3rd test pod on egress node but do not label it so this pod will not use egressIP")
		nonEIPPod := pingPodResourceNode{
			name:      "hello-pod" + "-non-eip-" + ns1,
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		nonEIPPod.createPingPodNode(oc)
		waitPodReady(oc, ns1, nonEIPPod.name)

		_, nonEIPPodIP1 := getPodIPUDN(oc, ns1, nonEIPPod.name, "ovn-udn1")

		exutil.By("8.2. Verify the non-EIP pod does not use EIP as source IP in its egressing packets, it directly uses its own UDN podIP as sourceIP")
		tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "Pod that unqualified to use egressIP did not use its UDN podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeFalse(), "Pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")

		exutil.By("9. Label the second node with egressNodeLabel, unlabel the first node, verify the new egress node is updated in the egressip object.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel, "true")

		egressipErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 360*time.Second, false, func(cxt context.Context) (bool, error) {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps1) != 1 || egressIPMaps1[0]["node"] == nodeList.Items[0].Name {
				e2e.Logf("Wait for egressIP being applied to new egress node,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[2].Name))

		exutil.By("10. From local and remote EIP pods, validate egressIP on new egressNode after egressIP failover \n")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[2].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[2].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("11. Verify that after egressIP failover, the non-EIP pod still just uses its own UDN podIP as sourceIP")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}

		tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "After failover, pod that unqualified to use egressIP did not use its UDN podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeFalse(), "After failover, pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-79767-egressIP using unused random IP on UDN is adverised and works as expected (singlestack) [Serial]", func() {

		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking")
			raTemplate           = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			egressIP2Template    = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			pingPodNodeTemplate  = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressNodeLabel      = "k8s.ovn.org/egress-assignable"
			udnName              = "layer3-udn-79767"
			networkselectorkey   = "app"
			networkselectorvalue = "udn"
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		ipStackType := checkIPStackType(oc)
		// dualstack scenario will be covered in a separate test case
		if (ipStackType == "ipv4single" || ipStackType == "ipv6single") && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on singlev4 or singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("1.1 Create a UDN namespace, create a layer3 UDN in the UDN namespace")
		exutil.By("Create namespace")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		exutil.By("Create CRD for UDN")
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
			}
		}

		createGeneralUDNCRD(oc, ns1, udnName, ipv4cidr, ipv6cidr, cidr, "layer3")

		exutil.By("1.2 Label the UDN with label that matches networkSelector in routeAdvertisement")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "userdefinednetwork", udnName, networkselectorkey+"-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "userdefinednetwork", udnName, networkselectorkey+"="+networkselectorvalue).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Apply a routeAdvertisement with matching networkSelector")
		raname := "ra-udn"
		params := []string{"-f", raTemplate, "-p", "NAME=" + raname, "NETWORKSELECTORKEY=" + networkselectorkey, "NETWORKSELECTORVALUE=" + networkselectorvalue}
		defer removeResource(oc, true, true, "ra", raname)
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		raErr := checkRAStatus(oc, raname, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - UDN routeAdvertisement applied is accepted")

		exutil.By("3.1 Label an egressNode, get a namespace, label the namespace with org=qe to match namespaceSelector of egressIP object that will be created in step 4")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.2. Label an egressNode, get a namespace, label the namespace with org=qe to match namespaceSelector of egressIP object that will be created in next step")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.3. Create an egressip object with a random unused IP, verify egressIP is assigned to egress node")
		var freeIP string
		if ipStackType == "ipv4single" {
			freeIP = generateRandomUnusedPublicIPv4(oc)
		}
		if ipStackType == "ipv6single" {
			freeIP = generateRandomGlobalIPv6(oc)
		}

		egressip1 := egressIPResource1{
			name:          "egressip-79767",
			template:      egressIP2Template,
			egressIP1:     freeIP,
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[0].Name))

		exutil.By("3.4. Verify egressIP address is advertised to external frr router")
		// Wait some time for EIP on UDN to be advertised
		time.Sleep(60 * time.Second)
		nodeIPEgressNode := getNodeIPv4(oc, "default", nodeList.Items[0].Name)
		o.Eventually(func() bool {
			result := verifySingleBGPRouteOnExternalFrr(host, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
			return result
		}, "60s", "15s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to external frr!!")

		exutil.By("4. Verify egressIP address is advertised to all other cluster nodes")
		o.Eventually(func() bool {
			result := verifySingleBGPRouteOnClusterNode(oc, egressIPMaps1[0]["node"], allNodes, egressIPMaps1[0]["egressIP"], nodeIPEgressNode, true)
			return result
		}, "60s", "5s").Should(o.BeTrue(), "egressIP on default network with random unused IP was not advertised to all other cluster nodes!!")

		exutil.By("5. Add iptables rules to hypervisor to assist the test")
		var ipnet *net.IPNet
		if ipStackType == "ipv4single" {
			_, ipnet, err = net.ParseCIDR(freeIP + "/32")
		}
		if ipStackType == "ipv6single" {
			_, ipnet, err = net.ParseCIDR(freeIP + "/128")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		ruleDel1 := "sudo iptables -t filter -D FORWARD -s " + ipnet.String() + " -i " + intf + " -j ACCEPT"
		ruleDel2 := "sudo iptables -t filter -D FORWARD -d " + ipnet.String() + " -o " + intf + " -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
		var ruleDel3 string
		if ipStackType == "ipv4single" {
			ruleDel3 = "sudo iptables -t nat -D POSTROUTING -s " + ipnet.String() + " ! -d " + externalFRRIP + "/24 -j MASQUERADE"
		}
		if ipStackType == "ipv6single" {
			ruleDel3 = "sudo iptables -t nat -D POSTROUTING -s " + ipnet.String() + " ! -d " + externalFRRIP + "/64 -j MASQUERADE"
		}
		defer sshRunCmd(host, "root", ruleDel1)
		defer sshRunCmd(host, "root", ruleDel2)
		defer sshRunCmd(host, "root", ruleDel3)
		err = addIPtablesRules(host, intf, freeIP, externalFRRIP)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6.1 Create two test pods, add to them with label color=pink to match podSelector of egressIP, one local and another remote to the egress node")
		EIPPods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			EIPPods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i) + "-eip-" + ns1,
				namespace: ns1,
				nodename:  nodeList.Items[i].Name,
				template:  pingPodNodeTemplate,
			}
			EIPPods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, EIPPods[i].name)
			defer exutil.LabelPod(oc, ns1, EIPPods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, EIPPods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6.2. Verify egressIP works from local or remote pod to egress node")
		e2e.Logf("Trying to get physical interface on the egressNode: %s", nodeList.Items[0].Name)
		primaryInf, infErr := getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		externalPublicHost := "www.google.com"

		var tcpdumpCmd, cmdOnPod string
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -k " + externalPublicHost
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -6 -k " + externalPublicHost
		}

		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeTrue())
		}

		exutil.By("7.1 Create a 3rd test pod on egress node but do not label it so this pod will not use egressIP")
		nonEIPPod := pingPodResourceNode{
			name:      "hello-pod" + "-non-eip-" + ns1,
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		nonEIPPod.createPingPodNode(oc)
		waitPodReady(oc, ns1, nonEIPPod.name)

		_, nonEIPPodIP1 := getPodIPUDN(oc, ns1, nonEIPPod.name, "ovn-udn1")

		exutil.By("7.2. Verify the non-EIP pod does not use EIP as source IP in its egressing packets, it directly uses its own UDN podIP as sourceIP")
		tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "Pod that unqualified to use egressIP did not use its UDN podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeFalse(), "Pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")

		exutil.By("8. Label the second node with egressNodeLabel, unlabel the first node, verify the new egress node is updated in the egressip object.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel, "true")

		egressipErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 360*time.Second, false, func(cxt context.Context) (bool, error) {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps1) != 1 || egressIPMaps1[0]["node"] == nodeList.Items[0].Name {
				e2e.Logf("Wait for egressIP being applied to new egress node,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(nodeList.Items[2].Name))

		exutil.By("9. From local and remote EIP pods, validate egressIP on new egressNode after egressIP failover \n")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[2].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[2].Name, tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeTrue())
		}

		exutil.By("10. Verify that after egressIP failover, the non-EIP pod still just uses its own UDN podIP as sourceIP")
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, nodeList.Items[0].Name, tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "After failover, pod that unqualified to use egressIP did not use its UDN podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIP)).To(o.BeFalse(), "After failover, pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")
	})

})
