package networking

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN bgp", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		ipStackType    string
		host           = "openshift-qe-028.lab.eng.rdu2.redhat.com"
		externalFRRIP1 string
		externalFRRIP2 string
		frrContainerID string
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
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			receiveTemplate     = filepath.Join(buildPruningBaseDir, "bgp/receive_all_template.yaml")
			receiveDSTemplate   = filepath.Join(buildPruningBaseDir, "bgp/receive_all_dualstack_template.yaml")
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			asn                 = 64512
			nodeErr             error
		)

		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		SkipIfNoFeatureGate(oc, "RouteAdvertisements")

		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		if err != nil || !(strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
			g.Skip("This case will only run on rdu1 or rdu2 cluster, skip for other test envrionment!!!")
		}

		if strings.Contains(msg, "offload.openshift-qe.sdn.com") {
			host = "openshift-qe-026.lab.eng.rdu2.redhat.com"
		}

		ipStackType = checkIPStackType(oc)

		exutil.By("check if FRR routeAdvertisements is enabled")
		if !IsFrrRouteAdvertisementEnabled(oc) {
			enableFRRRouteAdvertisement(oc)
			if !IsFrrRouteAdvertisementEnabled(oc) || !areFRRPodsReady(oc, frrNamespace) {
				g.Skip("FRR routeAdvertisement is still not enabled on the cluster, or FRR pods are not ready, skip the test!!!")
			}
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
		if ipStackType == "dualstack" {
			o.Expect(externalFRRIP2).NotTo(o.BeEmpty())
		}

		if ipStackType == "dualstack" || ipStackType == "ipv4single" {
			frrContainerID = createExternalFrrRouter(host, externalFRRIP1, allNodesIP1, allNodesIP2)
		}
		if ipStackType == "ipv6single" {
			frrContainerID = createExternalFrrRouter(host, externalFRRIP1, allNodesIP2, allNodesIP1)
		}

		exutil.By("Get default podNetworks of all cluster nodes")
		podNetwork2Map, podNetwork1Map = getHostPodNetwork(oc, allNodes, "default")
		o.Expect(len(podNetwork2Map)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(podNetwork1Map)).NotTo(o.BeEquivalentTo(0))

		exutil.By("Apply receive_all frrconfiguration and routeAdvertisements yamls to cluster")
		switch ipStackType {
		case "ipv4single":
			frrconfigration1 := frrconfigurationResource{
				name:           "receive-all",
				namespace:      "openshift-frr-k8s",
				asn:            asn,
				externalFRRIP1: externalFRRIP1,
				template:       receiveTemplate,
			}
			frrconfigration1.createFRRconfigration(oc)
			output, frrConfigErr := oc.AsAdmin().Run("get").Args("frrconfiguration", "-n", frrNamespace).Output()
			o.Expect(frrConfigErr).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, frrconfigration1.name)).To(o.BeTrue())
		case "dualstack":
			frrconfigrationDS := frrconfigurationResourceDS{
				name:           "receive-all",
				namespace:      "openshift-frr-k8s",
				asn:            asn,
				externalFRRIP1: externalFRRIP1,
				externalFRRIP2: externalFRRIP2,
				template:       receiveDSTemplate,
			}
			frrconfigrationDS.createFRRconfigrationDS(oc)
			output, frrConfigErr := oc.AsAdmin().Run("get").Args("frrconfiguration", "-n", frrNamespace).Output()
			o.Expect(frrConfigErr).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, frrconfigrationDS.name)).To(o.BeTrue())
		default:
			e2e.Logf("Other ipstack type (i.e singlev6) is currently not supported due to bug in frr.")
			g.Skip("Skip other unsupported ipstack type for now.")
		}

		raName := "default"
		networkSelectorKey := "k8s.ovn.org/default-network"
		networkSelectorValue := ""
		params := []string{"-f", raTemplate, "-p", "NAME=" + raName, "NETWORKSELECTORKEY=" + networkSelectorKey, "NETWORKSELECTORVALUE=" + networkSelectorValue}
		exutil.ApplyNsResourceFromTemplate(oc, "default", params...)
		raErr := checkRAStatus(oc, raName, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - routeAdvertisement applied is accepted")

		exutil.By("Verify default network is advertised")
		o.Eventually(func() bool {
			result := verifyRouteAdvertisement(oc, host, externalFRRIP1, frrContainerID, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map)
			return result
		}, "120s", "15s").Should(o.BeTrue(), "BGP route advertisement of default network did not succeed!!")
		e2e.Logf("SUCCESS - BGP enabled, default network is advertised!!!")

	})

	g.JustAfterEach(func() {
		removeResource(oc, true, true, "frrconfiguration", "receive-all", "-n", frrNamespace)
		removeResource(oc, true, true, "ra", "default")
		sshRunCmd(host, "root", "sudo podman rm -f "+frrContainerID)
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-High-78338-route advertisement and route leaking through VRF-default on default network [Serial]", func() {

		exutil.By("1. From IP routing table, verify cluster default podnetwork routes are advertised to external frr router")
		result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
		o.Expect(result).To(o.BeTrue(), "Not all podNetwork are advertised to external frr router")

		exutil.By("2. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-Longduration-NonPreRelease-High-78342-route advertisement recovery after node reboot [Disruptive]", func() {

		exutil.By("1. From IP routing table, verify cluster default podnetwork routes are advertised to external frr router")
		result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
		o.Expect(result).To(o.BeTrue(), "Not all podNetwork are advertised to external frr router")

		exutil.By("2. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

		exutil.By("3. Reboot one worker node.\n")
		workerNodes, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(workerNodes) < 1 {
			g.Skip("Need at least 1 worker node, not enough worker node, skip the case!!")
		}

		defer checkNodeStatus(oc, workerNodes[0], "Ready")
		rebootNode(oc, workerNodes[0])
		checkNodeStatus(oc, workerNodes[0], "NotReady")
		checkNodeStatus(oc, workerNodes[0], "Ready")

		exutil.By("4. Verify bgp routes in ip routing table of external frr router after node reboot")
		o.Eventually(func() bool {
			result = verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "120s", "5s").Should(o.BeTrue(), "Not all podNetwork are advertised to external frr router after rebooting a node")

		exutil.By("5. Verify bgp routes in ip routing table of each cluster node after node reboot")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table check on node %s failed after rebooting a node", node))
		}

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-78343-route advertisement recovery after OVNK restart [Disruptive]", func() {

		exutil.By("1. From IP routing table, verify cluster default podnetwork routes are advertised to external frr router")
		result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
		o.Expect(result).To(o.BeTrue(), "Not all podNetwork are advertised to external frr router")

		exutil.By("2. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

		exutil.By("3. Restart OVNK.\n")
		defer waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		delPodErr := oc.AsAdmin().Run("delete").Args("pod", "-l", "app=ovnkube-node", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		waitForNetworkOperatorState(oc, 100, 18, "True.*False.*False")

		exutil.By("4. Verify bgp routes in ip routing table of external frr router after OVNK restart")
		o.Consistently(func() bool {
			result = verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "Not all podNetwork are advertised to external frr router after OVNK restart")

		exutil.By("5. Verify bgp routes in ip routing table of each cluster node after OVNK restart")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table check on node %s failed after OVNK restart", node))
		}

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-78344-route advertisement recovery after frr-k8s pods restart [Disruptive]", func() {

		exutil.By("1. From IP routing table, verify cluster default podnetwork routes are advertised to external frr router")
		result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
		o.Expect(result).To(o.BeTrue(), "Not all podNetwork are advertised to external frr router")

		exutil.By("2. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

		exutil.By("3. Restart frr-k8s pods.\n")
		defer waitForPodWithLabelReady(oc, frrNamespace, "app=frr-k8s")
		defer waitForPodWithLabelReady(oc, frrNamespace, "component=frr-k8s-webhook-server")
		delPodErr := oc.AsAdmin().Run("delete").Args("pod", "-l", "app=frr-k8s", "-n", frrNamespace).Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())
		delPodErr = oc.AsAdmin().Run("delete").Args("pod", "-l", "component=frr-k8s-webhook-server", "-n", frrNamespace).Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())

		result = areFRRPodsReady(oc, frrNamespace)
		o.Expect(result).To(o.BeTrue(), "Not all frr-k8s pods fully recovered from restart")

		// Make sure frr-k8s ds successfully rolled out after restart
		status, err := oc.AsAdmin().WithoutNamespace().Run("rollout").Args("status", "-n", frrNamespace, "ds", "frr-k8s", "--timeout", "5m").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(status, "successfully rolled out")).To(o.BeTrue(), "frr-k8s ds did not successfully roll out")

		exutil.By("4. Verify bgp routes in ip routing table of external frr router after frr-k8s pods restart")
		o.Expect(result).To(o.BeTrue(), "Not all podNetwork are advertised to external frr router")
		o.Consistently(func() bool {
			result = verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "Not all podNetwork are advertised to external frr router after frr-k8s pods restart")

		exutil.By("5. Verify bgp routes in ip routing table of each cluster node after frr-k8s pods restart")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table check on node %s failed after frr-k8s pods restart", node))
		}

	})
})
