package networking

import (
	"context"
	"fmt"
	"os"
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
		host           = ""
		externalFRRIP1 string
		externalFRRIP2 string
		allNodes       []string
		podNetwork1Map = make(map[string]string)
		podNetwork2Map = make(map[string]string)
		nodesIP1Map    = make(map[string]string)
		nodesIP2Map    = make(map[string]string)
		allNodesIP2    []string
		allNodesIP1    []string
		frrNamespace   = "openshift-frr-k8s"
	)

	g.JustBeforeEach(func() {
		var (
			nodeErr error
		)

		host = os.Getenv("QE_HYPERVISOR_PUBLIC_ADDRESS")
		if host == "" {
			g.Skip("hypervisorHost is nil, please set env QE_HYPERVISOR_PUBLIC_ADDRESS first!!!")
		}
		SkipIfNoFeatureGate(oc, "RouteAdvertisements")
		if !IsFrrRouteAdvertisementEnabled(oc) || !areFRRPodsReady(oc, frrNamespace) {
			g.Skip("FRR routeAdvertisement is still not enabled on the cluster, or FRR pods are not ready, skip the test!!!")
		}

		raErr := checkRAStatus(oc, "default", "Accepted")
		if raErr != nil {
			g.Skip(("default ra is not accepted. pleaes check the default ra is ready before run the automation"))
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
		externalFRRIP2, externalFRRIP1 = getExternalFRRIP(oc, allNodesIP2, allNodesIP1, host)
		o.Expect(externalFRRIP1).NotTo(o.BeEmpty())
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
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-79089-egressIP without nodeSelector on default network can be advertised correctly and egressIP functions well (singlestack) [Serial]", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP2Template   = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			egressNodeLabel     = "k8s.ovn.org/egress-assignable"
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		ipStackType := checkIPStackType(oc)
		// If cluster is dualstack, it will be used as singlev4 cluster in this case,
		// A full daulstack scenario will be covered in a separate test case because each dualstack egressIP uses two nodes, will need total of 4 nodes for egressIP failover
		if len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test, the prerequirement was not fullfilled, skip the case!!")
		}

		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		var nonEgressNode string
		for _, node := range nodeList.Items {
			if node.Name != egressNodes[0] && node.Name != egressNodes[1] {
				nonEgressNode = node.Name
				break
			}
		}
		o.Expect(nonEgressNode).NotTo(o.Equal(""))

		exutil.By("1. Label an egressNode, get a namespace, label the namespace with org=qe to match namespaceSelector of egressIP object that will be created in step 2")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")

		ns1 := oc.Namespace()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Create an egressip object with an usused IP from same subnet of egress node, verify egressIP is assigned to egress node")
		e2e.Logf("get freeIP from node: %s", egressNodes[0])
		var freeIPs []string
		if ipStackType == "ipv4single" || ipStackType == "dualstack" {
			freeIPs = findFreeIPs(oc, egressNodes[0], 1)
		}
		if ipStackType == "ipv6single" {
			freeIPs = findFreeIPv6s(oc, egressNodes[0], 1)
		}
		o.Expect(len(freeIPs)).Should(o.Equal(1))

		egressip1 := egressIPResource1{
			name:          "egressip-79089",
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
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNodes[0]))

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

		exutil.By("5.1 Create two test pods, add to them with label color=pink to match podSelector of egressIP, one local and another remote to the egress node")
		var podNodes []string
		podNodes = append(podNodes, egressNodes[0])
		podNodes = append(podNodes, nonEgressNode)
		EIPPods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			EIPPods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i) + "-eip-" + ns1,
				namespace: ns1,
				nodename:  podNodes[i],
				template:  pingPodNodeTemplate,
			}
			EIPPods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, EIPPods[i].name)
			defer exutil.LabelPod(oc, ns1, EIPPods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, EIPPods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.2. Verify egressIP works from local or remote pod to egress node")
		e2e.Logf("Trying to get physical interface on the egressNode: %s", egressNodes[0])
		primaryInf, infErr := getSnifPhyInf(oc, egressNodes[0])
		o.Expect(infErr).NotTo(o.HaveOccurred())
		externalPublicHost := "www.google.com"
		var tcpdumpCmd, cmdOnPod string
		if ipStackType == "ipv4single" || ipStackType == "dualstack" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -k " + externalPublicHost
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
			cmdOnPod = "curl -I -6 -k " + externalPublicHost
		}

		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNodes[0], tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("6.1. Create a 3rd test pod on egress node but do not label it so this pod will not use egressIP")
		nonEIPPod := pingPodResourceNode{
			name:      "hello-pod" + "-non-eip-" + ns1,
			namespace: ns1,
			nodename:  egressNodes[0],
			template:  pingPodNodeTemplate,
		}
		nonEIPPod.createPingPodNode(oc)
		waitPodReady(oc, ns1, nonEIPPod.name)

		_, nonEIPPodIP1 := getPodIP(oc, ns1, nonEIPPod.name)

		exutil.By("6.2. Verify the non-EIP pod does not use EIP as source IP in its egressing packets, it directly uses its own podIP as sourceIP")
		tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNodes[0], tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "Pod that unqualified to use egressIP did not use its podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeFalse(), "Pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")

		exutil.By("7. Label the second node with egressNodeLabel, unlabel the first node, verify the new egress node is updated in the egressip object.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")

		egressipErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 360*time.Second, false, func(cxt context.Context) (bool, error) {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps1) != 1 || egressIPMaps1[0]["node"] == egressNodes[0] {
				e2e.Logf("Wait for egressIP being applied to new egress node,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNodes[1]))

		exutil.By("8. From local and remote EIP pods, validate egressIP on new egressNode after egressIP failover \n")
		primaryInf, infErr = getSnifPhyInf(oc, egressNodes[1])
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		for i := 0; i < len(EIPPods); i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNodes[1], tcpdumpCmd, ns1, EIPPods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("9. Verify that after egressIP failover, the non-EIP pod still just uses its own podIP as sourceIP")
		primaryInf, infErr = getSnifPhyInf(oc, egressNodes[0])
		o.Expect(infErr).NotTo(o.HaveOccurred())
		if ipStackType == "ipv4single" || ipStackType == "dualstack" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s host %s", primaryInf, externalPublicHost)
		}
		if ipStackType == "ipv6single" {
			tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 2 -nni %s ip6 host %s", primaryInf, externalPublicHost)
		}
		tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNodes[0], tcpdumpCmd, ns1, nonEIPPod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, nonEIPPodIP1)).To(o.BeTrue(), "After failover, pod that unqualified to use egressIP did not use its podIP as sourceIP!!!")
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeFalse(), "After failover, pod that unqualified to use egressIP should not see egressIP as sourceIP in its egress packets!!!")
	})

})
