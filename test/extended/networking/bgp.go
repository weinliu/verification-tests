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
		externalFRRIP  string
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
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			asn                 = 64512
			nodeErr             error
		)

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
		externalFRRIP = getExternalFRRIP(oc, allNodesIP1, host)
		o.Expect(externalFRRIP).NotTo(o.BeEmpty())

		if ipStackType == "dualstack" || ipStackType == "ipv4single" {
			frrContainerID = createExternalFrrRouter(host, externalFRRIP, allNodesIP1, allNodesIP2)
		}
		if ipStackType == "ipv6single" {
			frrContainerID = createExternalFrrRouter(host, externalFRRIP, allNodesIP2, allNodesIP1)
		}

		exutil.By("Get default podNetworks of all cluster nodes")
		podNetwork2Map, podNetwork1Map = getDefaultPodNetwork(oc, allNodes)
		o.Expect(len(podNetwork2Map)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(podNetwork1Map)).NotTo(o.BeEquivalentTo(0))

		exutil.By("Apply receive_all frrconfiguration and routeAdvertisements yamls to cluster")
		frrconfigration1 := frrconfigurationResource{
			name:          "receive-all",
			namespace:     "openshift-frr-k8s",
			asn:           asn,
			externalFRRIP: externalFRRIP,
			template:      receiveTemplate,
		}

		frrconfigration1.createFRRconfigration(oc)
		output, frrConfigErr := oc.AsAdmin().Run("get").Args("frrconfiguration", "-n", frrNamespace).Output()
		o.Expect(frrConfigErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, frrconfigration1.name)).To(o.BeTrue())

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
			result := verifyRouteAdvertisement(oc, host, externalFRRIP, frrContainerID, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP route advertisement of default network did not succeed!!")
		e2e.Logf("SUCCESS - BGP enabled, default network is advertised!!!")

	})

	g.JustAfterEach(func() {
		sshRunCmd(host, "root", "sudo podman rm -f "+frrContainerID)
		removeResource(oc, true, true, "frrconfiguration", "receive-all", "-n", frrNamespace)
		removeResource(oc, true, true, "ra", "default")
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-High-78338-route advertisement and route leaking through VRF-default on default network [Serial]", func() {

		exutil.By("1. From IP routing table, verify cluster default podnetwork routes are advertised to external frr router")
		result := verifyIPRoutesOnExternalFrr(host, frrContainerID, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
		o.Expect(result).To(o.BeTrue(), "Not all podNetwork are advertised to external frr router")

		exutil.By("2. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

	})

})
