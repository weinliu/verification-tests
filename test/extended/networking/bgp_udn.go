package networking

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN ovn-kubernetes bgp-udn", func() {
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

		if strings.Contains(msg, "offload.openshift-qe.sdn.com") {
			host = "openshift-qe-026.lab.eng.rdu2.redhat.com"
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

	// author: zzhao@redhat.com
	g.It("Author:zzhao-NonHyperShiftHOST-ConnectedOnly-Critical-78806-UDN network can be accessed once the it's advertised by BGP in LGW and SGW [Serial]", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			udnName             = "udn-network-78806"
		)

		ipStackType := checkIPStackType(oc)
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

		//label userdefinednetwork with label app=blue
		setUDNLabel(oc, ns1, udnName, "app=udn")

		exutil.By("Create RA to advertise the UDN network")

		ra := routeAdvertisement{
			name:              "udn",
			networkLabelKey:   "app",
			networkLabelVaule: "udn",
			template:          raTemplate,
		}
		defer func() {
			ra.deleteRA(oc)
		}()
		ra.createRA(oc)
		raErr := checkRAStatus(oc, ra.name, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - UDN routeAdvertisement applied is accepted")

		exutil.By("Check the UDN network was advertised to external router")

		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns1+"_"+udnName)
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")

		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertise!!!", udnName, ns1)

		exutil.By("Create replica pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		err := waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS1Names := getPodName(oc, ns1, "name=test-pods")
		Curlexternal2UDNPodPass(oc, host, ns1, testpodNS1Names[1])

		exutil.By("Delete the RA for the udn and check the traffic again")
		ra.deleteRA(oc)

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "5s").ShouldNot(o.BeTrue(), "BGP UDN route advertisement did not be removed!!")
		Curlexternal2UDNPodFail(oc, host, ns1, testpodNS1Names[1])

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-High-78339-High-78348-route advertisement for UDN networks through VRF-default and route filtering with networkSelector [Serial]", func() {

		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking")
			raTemplate           = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			pingPodTemplate      = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			networkselectorkey   = "app"
			networkselectorvalue = "udn"
			udnNames             = []string{"layer3-udn-78339-1", "layer3-udn-78339-2"}
		)

		exutil.By("1. Create two UDN namespaces, create a layer3 UDN in each UDN namespace, the two UDNs should NOT be overlapping")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr []string
		ipv4cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
		ipv6cidr = []string{"2010:100:200::0/48", "2011:100:200::0/48"}
		cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
		if ipStackType == "ipv6single" {
			cidr = []string{"2010:100:200::0/48", "2011:100:200::0/48"}
		}

		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		createGeneralUDNCRD(oc, ns1, udnNames[0], ipv4cidr[0], ipv6cidr[0], cidr[0], "layer3")
		createGeneralUDNCRD(oc, ns2, udnNames[1], ipv4cidr[1], ipv6cidr[1], cidr[1], "layer3")

		exutil.By("2. Only label the first UDN with label that matches networkSelector in routeAdvertisement")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "userdefinednetwork", udnNames[0], networkselectorkey+"-").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", ns1, "userdefinednetwork", udnNames[0], networkselectorkey+"="+networkselectorvalue).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Apply a routeAdvertisement with matching networkSelector")
		raname := "ra-udn"
		params := []string{"-f", raTemplate, "-p", "NAME=" + raname, "NETWORKSELECTORKEY=" + networkselectorkey, "NETWORKSELECTORVALUE=" + networkselectorvalue}
		defer removeResource(oc, true, true, "ra", raname)
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		raErr := checkRAStatus(oc, raname, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - UDN routeAdvertisement applied is accepted")

		exutil.By("4. Verify the first UDN with matching networkSelector is advertised")
		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns1+"_"+udnNames[0])
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "UDN with matching networkSelector was not advertised as expected!!")

		exutil.By("5. Verify the second UDN without matching networkSelector is NOT advertised")
		UDNnetwork_ipv6_ns2, UDNnetwork_ipv4_ns2 := getHostPodNetwork(oc, allNodes, ns2+"_"+udnNames[1])
		result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns2, UDNnetwork_ipv6_ns2, nodesIP1Map, nodesIP2Map, false)
		o.Expect(result).To(o.BeTrue())

		testpods := make([]pingPodResource, len(udnNS))
		for i := 0; i < len(udnNS); i++ {
			testpods[i] = pingPodResource{
				name:      "hello-pod" + udnNS[i],
				namespace: udnNS[i],
				template:  pingPodTemplate,
			}
			testpods[i].createPingPod(oc)
			waitPodReady(oc, testpods[i].namespace, testpods[i].name)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5. Verify UDN pod in first UDN namespace can be accessed from external but the UDN pod in second UDN namespace is not accessible as its UDN was not advertised")
		Curlexternal2UDNPodPass(oc, host, ns1, testpods[0].name)
		Curlexternal2UDNPodFail(oc, host, ns2, testpods[1].name)

		e2e.Logf("SUCCESS - UDN route advertisement through VRF-default and route filtering through networkSelector work correctly!!!")
	})

})
