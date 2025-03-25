package networking

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN OVN ibgp", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		ipStackType          string
		host                 = ""
		externalFRRIP1       string
		externalFRRIP2       string
		frrContainerID       string
		allNodes             []string
		podNetwork1Map       = make(map[string]string)
		podNetwork2Map       = make(map[string]string)
		nodesIP1Map          = make(map[string]string)
		nodesIP2Map          = make(map[string]string)
		allNodesIP2          []string
		allNodesIP1          []string
		frrNamespace         = "openshift-frr-k8s"
		raName               string
		frrConfigurationName string
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

		host = os.Getenv("QE_HYPERVISOR_PUBLIC_ADDRESS")
		if host == "" {
			g.Skip("hypervisorHost is nil, please set env QE_HYPERVISOR_PUBLIC_ADDRESS first!!!")
		}

		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		SkipIfNoFeatureGate(oc, "RouteAdvertisements")

		SkipIfExternalFRRExists(host)

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

		emptyArray := []string{}
		if ipStackType == "dualstack" {
			frrContainerID = createExternalFrrRouter(host, "", allNodesIP1, allNodesIP2, emptyArray, emptyArray)
		} else if ipStackType == "ipv4single" {
			frrContainerID = createExternalFrrRouter(host, "", allNodesIP1, emptyArray, emptyArray, emptyArray)
		} else if ipStackType == "ipv6single" {
			frrContainerID = createExternalFrrRouter(host, "", emptyArray, allNodesIP1, emptyArray, emptyArray)
		}

		exutil.By("Get default podNetworks of all cluster nodes")
		podNetwork2Map, podNetwork1Map = getHostPodNetwork(oc, allNodes, "default")
		o.Expect(len(podNetwork2Map)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(podNetwork1Map)).NotTo(o.BeEquivalentTo(0))

		exutil.By("Apply receive_all frrconfiguration and routeAdvertisements yamls to cluster")
		frrConfigurationName = "receive-all"
		switch ipStackType {
		case "ipv4single":
			frrconfigration1 := frrconfigurationResource{
				name:           frrConfigurationName,
				namespace:      frrNamespace,
				asnLocal:       asn,
				asnRemote:      asn,
				externalFRRIP1: externalFRRIP1,
				template:       receiveTemplate,
			}
			frrconfigration1.createFRRconfigration(oc)
			output, frrConfigErr := oc.AsAdmin().Run("get").Args("frrconfiguration", "-n", frrNamespace).Output()
			o.Expect(frrConfigErr).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, frrconfigration1.name)).To(o.BeTrue())
		case "dualstack":
			frrconfigrationDS := frrconfigurationResourceDS{
				name:           frrConfigurationName,
				namespace:      frrNamespace,
				asnLocal:       asn,
				asnRemote:      asn,
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

		raName = "default"
		networkSelectorKey := "k8s.ovn.org/default-network"
		networkSelectorValue := ""
		params := []string{"-f", raTemplate, "-p", "NAME=" + raName, "NETWORKSELECTORKEY=" + networkSelectorKey, "NETWORKSELECTORVALUE=" + networkSelectorValue}
		exutil.ApplyNsResourceFromTemplate(oc, "default", params...)
		raErr := checkRAStatus(oc, raName, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - routeAdvertisement applied is accepted")

		exutil.By("Verify default network is advertised")
		o.Eventually(func() bool {
			result := verifyRouteAdvertisement(oc, host, externalFRRIP2, externalFRRIP1, frrContainerID, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map)
			return result
		}, "120s", "15s").Should(o.BeTrue(), "BGP route advertisement of default network did not succeed!!")
		e2e.Logf("SUCCESS - BGP enabled, default network is advertised!!!")

	})

	g.JustAfterEach(func() {
		removeResource(oc, true, true, "ra", raName)
		removeResource(oc, true, true, "frrconfiguration", frrConfigurationName, "-n", frrNamespace)
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

})

var _ = g.Describe("[sig-networking] SDN bgp ebgp", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		ipStackType          string
		host                 = ""
		externalFRRIP1       string
		externalFRRIP2       string
		frrContainerID       string
		allNodes             []string
		podNetwork1Map       = make(map[string]string)
		podNetwork2Map       = make(map[string]string)
		nodesIP1Map          = make(map[string]string)
		nodesIP2Map          = make(map[string]string)
		allNodesIP2          []string
		allNodesIP1          []string
		frrNamespace         = "openshift-frr-k8s"
		raName               string
		frrConfigurationName string
	)

	g.JustBeforeEach(func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			receiveTemplate     = filepath.Join(buildPruningBaseDir, "bgp/receive_all_template.yaml")
			receiveDSTemplate   = filepath.Join(buildPruningBaseDir, "bgp/receive_all_dualstack_template.yaml")
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			localASN            = 64512
			externalASN         = 64515
			nodeErr             error
		)

		host = os.Getenv("QE_HYPERVISOR_PUBLIC_ADDRESS")
		if host == "" {
			g.Skip("hypervisorHost is nil, please set env QE_HYPERVISOR_PUBLIC_ADDRESS first!!!")
		}

		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		SkipIfNoFeatureGate(oc, "RouteAdvertisements")

		SkipIfExternalFRRExists(host)

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

		emptyArray := []string{}
		if ipStackType == "dualstack" {
			frrContainerID = createExternalFrrRouterEBGP(host, "", allNodesIP1, allNodesIP2, emptyArray, emptyArray)
		} else if ipStackType == "ipv4single" {
			frrContainerID = createExternalFrrRouterEBGP(host, "", allNodesIP1, emptyArray, emptyArray, emptyArray)
		} else if ipStackType == "ipv6single" {
			frrContainerID = createExternalFrrRouterEBGP(host, "", emptyArray, allNodesIP1, emptyArray, emptyArray)
		}

		exutil.By("Get default podNetworks of all cluster nodes")
		podNetwork2Map, podNetwork1Map = getHostPodNetwork(oc, allNodes, "default")
		o.Expect(len(podNetwork2Map)).NotTo(o.BeEquivalentTo(0))
		o.Expect(len(podNetwork1Map)).NotTo(o.BeEquivalentTo(0))

		exutil.By("Apply receive_all frrconfiguration and routeAdvertisements yamls to cluster")
		frrConfigurationName = "receive-all"
		switch ipStackType {
		case "ipv4single":
			frrconfigration1 := frrconfigurationResource{
				name:           frrConfigurationName,
				namespace:      frrNamespace,
				asnLocal:       localASN,
				asnRemote:      externalASN,
				externalFRRIP1: externalFRRIP1,
				template:       receiveTemplate,
			}
			frrconfigration1.createFRRconfigration(oc)
			output, frrConfigErr := oc.AsAdmin().Run("get").Args("frrconfiguration", "-n", frrNamespace).Output()
			o.Expect(frrConfigErr).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, frrconfigration1.name)).To(o.BeTrue())
		case "dualstack":
			frrconfigrationDS := frrconfigurationResourceDS{
				name:           frrConfigurationName,
				namespace:      frrNamespace,
				asnLocal:       localASN,
				asnRemote:      externalASN,
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

		raName = "default"
		networkSelectorKey := "k8s.ovn.org/default-network"
		networkSelectorValue := ""
		params := []string{"-f", raTemplate, "-p", "NAME=" + raName, "NETWORKSELECTORKEY=" + networkSelectorKey, "NETWORKSELECTORVALUE=" + networkSelectorValue}
		exutil.ApplyNsResourceFromTemplate(oc, "default", params...)
		raErr := checkRAStatus(oc, raName, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - routeAdvertisement applied is accepted")

		// wait some time for routeAdvertisement to occur
		time.Sleep(60 * time.Second)
		exutil.By("Verify default network is advertised")
		o.Eventually(func() bool {
			result := verifyRouteAdvertisementEBGP(oc, host, externalFRRIP2, externalFRRIP1, frrContainerID, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map)
			return result
		}, "90s", "5s").Should(o.BeTrue(), "BGP route advertisement of default network did not succeed!!")
		e2e.Logf("SUCCESS - BGP enabled, default network is advertised!!!")

	})

	g.JustAfterEach(func() {
		removeResource(oc, true, true, "ra", raName)
		removeResource(oc, true, true, "frrconfiguration", frrConfigurationName, "-n", frrNamespace)
		sshRunCmd(host, "root", "sudo podman rm -f "+frrContainerID)
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-Longduration-NonPreRelease-High-78349-route advertisement on default network for eBGP scenrio [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		)
		exutil.By("1. Get first namespace for default network")
		ns1 := oc.Namespace()

		exutil.By("2. Create a test pod in each namespace, verify each pod can be accessed from external because their default network")
		testpod := pingPodResource{
			name:      "hello-pod" + ns1,
			namespace: ns1,
			template:  pingPodTemplate,
		}
		testpod.createPingPod(oc)
		waitPodReady(oc, testpod.namespace, testpod.name)

		Curlexternal2PodPass(oc, host, testpod.namespace, testpod.name)

	})

})
