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
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN ovn-kubernetes ibgp-udn", func() {
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

		exutil.By("Get external FRR IP")
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

		//label userdefinednetwork with label app=udn
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

		//first add iptables in external router to farwarding the traffic
		udnIp1, udnIp2 := getPodIPUDN(oc, ns1, testpodNS1Names[0], "ovn-udn1")

		exutil.By("from UDN pod egress outside before adding iptables, should be failed")
		_, err = e2eoutput.RunHostCmd(ns1, testpodNS1Names[0], "curl www.google.com --connect-timeout 5 -I")
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("from UDN pod egress outside after adding iptables, should be pass")

		defer restoreIptablesRules(host)
		err = addIPtablesRules(host, udnIp1, udnIp2, externalFRRIP1, externalFRRIP2)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = e2eoutput.RunHostCmd(ns1, testpodNS1Names[0], "curl www.google.com --connect-timeout 5 -I")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Delete the RA for the udn and check the traffic again")
		ra.deleteRA(oc)

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, false)
			return result
		}, "60s", "5s").Should(o.BeTrue(), "BGP UDN route advertisement did not be removed!!")
		Curlexternal2UDNPodFail(oc, host, ns1, testpodNS1Names[1])
		_, err = e2eoutput.RunHostCmd(ns1, testpodNS1Names[0], "curl www.google.com --connect-timeout 5 -I")
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-High-78339-High-78348-route advertisement for UDN networks through VRF-default and route filtering with networkSelector [Serial]", func() {

		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking")
			raTemplate           = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			pingPodTemplate      = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			networkselectorkey   = "app"
			networkselectorvalue = "udn"
			udnNames             = []string{"layer3-udn-78339-1", "layer3-udn-78339-2", "layer3-udn-78339-3"}
			udnNS                []string
		)

		exutil.By("1. Create two UDN namespaces, create a layer3 UDN in each UDN namespace, the two UDNs should NOT be overlapping")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr []string
		ipv4cidr = []string{"10.150.0.0/16", "20.150.0.0/16", "30.150.0.0/16"}
		ipv6cidr = []string{"2010:100:200::0/48", "2011:100:200::0/48", "2012:100:200::0/48"}
		cidr = []string{"10.150.0.0/16", "20.150.0.0/16", "30.150.0.0/16"}
		if ipStackType == "ipv6single" {
			cidr = []string{"2010:100:200::0/48", "2011:100:200::0/48", "2012:100:200::0/48"}
		}

		for i := 0; i < 3; i++ {
			oc.CreateNamespaceUDN()
			ns := oc.Namespace()
			udnNS = append(udnNS, ns)
			createGeneralUDNCRD(oc, ns, udnNames[i], ipv4cidr[i], ipv6cidr[i], cidr[i], "layer3")
		}

		exutil.By("2. Only label the first two UDNs with label that matches networkSelector in routeAdvertisement")
		for i := 0; i < 2; i++ {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", udnNS[i], "userdefinednetwork", udnNames[i], networkselectorkey+"-").Execute()
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", udnNS[i], "userdefinednetwork", udnNames[i], networkselectorkey+"="+networkselectorvalue).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("3. Apply a routeAdvertisement with matching networkSelector")
		raname := "ra-udn"
		params := []string{"-f", raTemplate, "-p", "NAME=" + raname, "NETWORKSELECTORKEY=" + networkselectorkey, "NETWORKSELECTORVALUE=" + networkselectorvalue}
		defer removeResource(oc, true, true, "ra", raname)
		exutil.ApplyNsResourceFromTemplate(oc, oc.Namespace(), params...)
		raErr := checkRAStatus(oc, raname, "Accepted")
		exutil.AssertWaitPollNoErr(raErr, "routeAdvertisement applied does not have the right condition status")
		e2e.Logf("SUCCESS - UDN routeAdvertisement applied is accepted")

		exutil.By("4. Verify the first two UDNs with matching networkSelector are advertised")
		var UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 map[string]string
		for i := 0; i < 2; i++ {
			UDNnetwork_ipv6_ns, UDNnetwork_ipv4_ns := getHostPodNetwork(oc, allNodes, udnNS[i]+"_"+udnNames[i])

			// save pod nework info of first UDN, it will be be used in step 8
			if i == 0 {
				UDNnetwork_ipv6_ns1 = UDNnetwork_ipv6_ns
				UDNnetwork_ipv4_ns1 = UDNnetwork_ipv4_ns
			}
			o.Eventually(func() bool {
				result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns, UDNnetwork_ipv6_ns, nodesIP1Map, nodesIP2Map, true)
				return result
			}, "60s", "10s").Should(o.BeTrue(), "UDN with matching networkSelector was not advertised as expected!!")
		}

		exutil.By("5. Verify the third UDN without matching networkSelector is NOT advertised")
		UDNnetwork_ipv6_ns3, UDNnetwork_ipv4_ns3 := getHostPodNetwork(oc, allNodes, udnNS[2]+"_"+udnNames[2])
		result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns3, UDNnetwork_ipv6_ns3, nodesIP1Map, nodesIP2Map, false)
		o.Expect(result).To(o.BeTrue(), "Unlablled UDN should not be advertised, but their routes are in routing table")

		exutil.By("6.1 Create a UDN pod in each UDN namespace associating with its UDN")
		testpods := make([]pingPodResource, len(udnNS))
		for i := 0; i < len(udnNS); i++ {
			testpods[i] = pingPodResource{
				name:      "hello-pod" + udnNS[i],
				namespace: udnNS[i],
				template:  pingPodTemplate,
			}
			testpods[i].createPingPod(oc)
			waitPodReady(oc, testpods[i].namespace, testpods[i].name)
		}

		exutil.By("6.2 Verify UDN pod in first two UDN namespaces can be accessed from external but the UDN pod in 3rd UDN namespace is not accessible as its UDN was not advertised")
		Curlexternal2UDNPodPass(oc, host, udnNS[0], testpods[0].name)
		Curlexternal2UDNPodPass(oc, host, udnNS[1], testpods[1].name)
		Curlexternal2UDNPodFail(oc, host, udnNS[2], testpods[2].name)

		// comment out the rest of test steps due to https://issues.redhat.com/browse/OCPBUGS-51142, will add it back after the bug is fixed
		// exutil.By("7.1 Unlabel the second UDN, verify the second UDN is not longer advertised")
		// err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", udnNS[1], "userdefinednetwork", udnNames[1], networkselectorkey+"-").Execute()
		// o.Expect(err).NotTo(o.HaveOccurred())
		// UDNnetwork_ipv6_ns2, UDNnetwork_ipv4_ns2 := getHostPodNetwork(oc, allNodes, udnNS[1]+"_"+udnNames[1])
		// o.Eventually(func() bool {
		// 	result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns2, UDNnetwork_ipv6_ns2, nodesIP1Map, nodesIP2Map, false)
		// 	return result
		// }, "60s", "10s").Should(o.BeTrue(), "advertised routes for unlabelled UDN were not cleaned up as expected!!")

		// exutil.By("7.2 UDN pod in second UDN should not be accessible from external any more")
		// time.Sleep(60 * time.Second)
		// Curlexternal2UDNPodFail(oc, host, udnNS[1], testpods[1].name)

		exutil.By("8. Delete the UDN pod of first UDN, then delete the first UDN, verify the first UDN is not longer advertised")
		removeResource(oc, true, true, "pod", testpods[0].name, "-n", testpods[0].namespace)
		removeResource(oc, true, true, "userdefinednetwork", udnNames[0], "-n", udnNS[0])

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, false)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "advertised routes for deleted UDN were not cleaned up as expected!!")

		e2e.Logf("SUCCESS - UDN route advertisement through VRF-default and route filtering through networkSelector work correctly!!!")
	})

	g.It("Author:zzhao-NonHyperShiftHOST-ConnectedOnly-Critical-78809-UDN pod can access same node and different node when BGP is advertise in LGW and SGW mode [Serial]", func() {

		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			testPodFile            = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			hostNetworkPodTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-hostnetwork-specific-node-template.yaml")
			raTemplate             = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			udnName                = "udn-network-78809"
		)
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This test requires at least 2 worker nodes which is not fulfilled. ")
		}
		ipStackType := checkIPStackType(oc)

		exutil.By("Create hostnetwork pod in ns")
		ns_hostnetwork := oc.Namespace()
		err := exutil.SetNamespacePrivileged(oc, ns_hostnetwork)
		o.Expect(err).NotTo(o.HaveOccurred())

		hostpod := pingPodResourceNode{
			name:      "hostnetwork-pod",
			namespace: ns_hostnetwork,
			nodename:  nodeList.Items[0].Name,
			template:  hostNetworkPodTemplate,
		}
		hostpod.createPingPodNode(oc)
		waitPodReady(oc, ns_hostnetwork, hostpod.name)
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

		//label userdefinednetwork with label app=udn
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

		exutil.By("Check the UDN network was advertised to external router")

		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns1+"_"+udnName)
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "5s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")

		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertise!!!", udnName, ns1)

		exutil.By("Create replica pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		//testpodNS1Names := getPodName(oc, ns1, "name=test-pods")

		//nodeIPv6, nodeIPv4 := getNodeIP(oc, hostpod.nodename)
		exutil.By("check from the UDN pod can access same/different host service")

		//comment due to https: //issues.redhat.com/browse/OCPBUGS-51165
		//CurlUDNPod2hostServicePASS(oc, ns1, testpodNS1Names[1], nodeIPv4, nodeIPv6, "8080")
		//CurlUDNPod2hostServicePASS(oc, ns1, testpodNS1Names[0], nodeIPv4, nodeIPv6, "8080")

		exutil.By("Delete the RA for the udn and check the traffic again, which should be failed as UDN host isolation")
		ra.deleteRA(oc)

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, false)
			return result
		}, "60s", "5s").Should(o.BeTrue(), "BGP UDN route advertisement did not be removed!!")

		//comment due to https: //issues.redhat.com/browse/OCPBUGS-51165
		//CurlUDNPod2hostServiceFail(oc, ns1, testpodNS1Names[1], nodeIPv4, nodeIPv6, "8080")
		//CurlUDNPod2hostServiceFail(oc, ns1, testpodNS1Names[0], nodeIPv4, nodeIPv6, "8080")

	})

	g.It("Author:zzhao-NonHyperShiftHOST-ConnectedOnly-Critical-78810-Same host and different host can access the UDN pod when BGP route is advertised on both SGW and LGW [Serial]", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			udnName             = "udn-network-78810"
		)
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This test requires at least 2 worker nodes which is not fulfilled. ")
		}
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

		//label userdefinednetwork with label app=udn
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

		exutil.By("Check the UDN network was advertised on worker node")

		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns1+"_"+udnName)
		o.Eventually(func() bool {
			result := verifyBGPRoutesOnClusterNode(oc, nodeList.Items[0].Name, externalFRRIP2, externalFRRIP1, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")

		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertise!!!", udnName, ns1)

		exutil.By("Create replica pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		err := waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS1Names := getPodName(oc, ns1, "name=test-pods")

		exutil.By("Get the pod located node name")
		nodeName, nodeNameErr := exutil.GetPodNodeName(oc, ns1, testpodNS1Names[1])
		o.Expect(nodeNameErr).NotTo(o.HaveOccurred())

		exutil.By("Validate pod to pod on different workers")
		CurlPod2PodPassUDN(oc, ns1, testpodNS1Names[0], ns1, testpodNS1Names[1])

		exutil.By("check from same host to access udn pod")
		// comment this due to bug https://issues.redhat.com/browse/OCPBUGS-51165
		//CurlNode2PodFailUDN(oc, nodeName, ns1, testpodNS1Names[1])

		exutil.By("check from the UDN pod can access different host service")
		differentHostName := nodeList.Items[0].Name
		if differentHostName == nodeName {
			differentHostName = nodeList.Items[1].Name
		}
		// comment this due to bug https://issues.redhat.com/browse/OCPBUGS-51165
		//CurlNode2PodPassUDN(oc, differentHostName, ns1, testpodNS1Names[1])

		exutil.By("Delete the RA for the udn and check the traffic again, host to UDN should be isolation")
		ra.deleteRA(oc)

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, false)
			return result
		}, "60s", "5s").Should(o.BeTrue(), "BGP UDN route advertisement did not be removed!!")
		CurlNode2PodFailUDN(oc, nodeName, ns1, testpodNS1Names[1])
		CurlNode2PodFailUDN(oc, differentHostName, ns1, testpodNS1Names[1])

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-Longduration-NonPreRelease-High-78342-route advertisement recovery for default network and UDN after node reboot [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			udnName             = "udn-network-78342"
			ipStackType         = checkIPStackType(oc)
		)

		exutil.By("1. Get worker nodes")
		workerNodes, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(workerNodes) < 1 {
			g.Skip("Need at least 1 worker node, not enough worker node, skip the case!!")
		}

		exutil.By("2. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

		exutil.By("3. Get first namespace for default network, create an UDN namespace and UDN in UDN namespace")
		ns1 := oc.Namespace()

		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		allNS := []string{ns1, ns2}

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
		createGeneralUDNCRD(oc, ns2, udnName, ipv4cidr, ipv6cidr, cidr, "layer3")

		//label userdefinednetwork with label app=udn
		setUDNLabel(oc, ns2, udnName, "app=udn")

		exutil.By("4. Create RA to advertise the UDN network")
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

		exutil.By("5.  Verify  UDN network is advertised to external and other cluster nodes")
		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns2+"_"+udnName)
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertised to external !!!", udnName, ns2)

		for _, node := range allNodes {
			result := verifyBGPRoutesOnClusterNode(oc, node, externalFRRIP2, externalFRRIP1, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertised to other cluster nodes !!!", udnName, ns2)

		exutil.By("6. Create a test pod in each namespace, verify each pod can be accessed from external because their default network and UDN are advertised")
		testpods := make([]pingPodResource, len(allNS))
		for i := 0; i < len(allNS); i++ {
			testpods[i] = pingPodResource{
				name:      "hello-pod" + allNS[i],
				namespace: allNS[i],
				template:  pingPodTemplate,
			}
			testpods[i].createPingPod(oc)
			waitPodReady(oc, testpods[i].namespace, testpods[i].name)
		}
		Curlexternal2PodPass(oc, host, testpods[0].namespace, testpods[0].name)
		Curlexternal2UDNPodPass(oc, host, testpods[1].namespace, testpods[1].name)

		exutil.By("7. Reboot one worker node.\n")
		defer checkNodeStatus(oc, workerNodes[0], "Ready")
		rebootNode(oc, workerNodes[0])
		checkNodeStatus(oc, workerNodes[0], "NotReady")
		checkNodeStatus(oc, workerNodes[0], "Ready")

		exutil.By("8.  Verify default network and UDN advertisements after node reboot")
		exutil.By("8.1.  Verify from external frr container")
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "120s", "5s").Should(o.BeTrue(), "Not all podNetwork are advertised to external frr router after rebooting a node")
		e2e.Logf("SUCCESS - BGP default network is still correctly advertised to external after node reboot !!!")

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s is still correctly advertised to external after node reboot !!!", udnName, ns2)

		exutil.By("8.2. Verify from cluster nodes")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table check for routes of defaulet network on node %s failed after rebooting a node", node))
			result = verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes for UDN %s as expected", node, udnName))
		}
		e2e.Logf("SUCCESS - BGP default network and UDN are still correctly advertised to other cluster nodes after node reboot !!!")

		exutil.By("9. Verify each pod can still be accessed from external after node reboot because their default network and UDN remain advertised")
		Curlexternal2PodPass(oc, host, allNS[0], testpods[0].name)
		Curlexternal2UDNPodPass(oc, host, allNS[1], testpods[1].name)

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-78343-route advertisement recovery for default network and UDN after OVNK restart [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			udnName             = "udn-network-78343"
			ipStackType         = checkIPStackType(oc)
		)

		exutil.By("1. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

		exutil.By("2. Get first namespace for default network, create an UDN namespace and UDN in UDN namespace")
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		allNS := []string{ns1, ns2}

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

		createGeneralUDNCRD(oc, ns2, udnName, ipv4cidr, ipv6cidr, cidr, "layer3")

		//label userdefinednetwork with label app=udn
		setUDNLabel(oc, ns2, udnName, "app=udn")

		exutil.By("3. Create RA to advertise the UDN network")

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

		exutil.By("4.  Verify  UDN network is advertised to external and other cluster nodes")
		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns2+"_"+udnName)
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertised to external !!!", udnName, ns2)

		for _, node := range allNodes {
			result := verifyBGPRoutesOnClusterNode(oc, node, externalFRRIP2, externalFRRIP1, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertised to other cluster nodes !!!", udnName, ns2)

		exutil.By("5. Create a test pod in each namespace, verify each pod can be accessed from external because their default network and UDN are advertised")
		testpods := make([]pingPodResource, len(allNS))
		for i := 0; i < len(allNS); i++ {
			testpods[i] = pingPodResource{
				name:      "hello-pod" + allNS[i],
				namespace: allNS[i],
				template:  pingPodTemplate,
			}
			testpods[i].createPingPod(oc)
			waitPodReady(oc, testpods[i].namespace, testpods[i].name)
		}
		Curlexternal2PodPass(oc, host, testpods[0].namespace, testpods[0].name)
		Curlexternal2UDNPodPass(oc, host, testpods[1].namespace, testpods[1].name)

		exutil.By("6. Restart OVNK.\n")
		defer waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		delPodErr := oc.AsAdmin().Run("delete").Args("pod", "-l", "app=ovnkube-node", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		waitForNetworkOperatorState(oc, 100, 18, "True.*False.*False")

		exutil.By("7.  Verify default network and UDN advertisements after OVNK restart")
		exutil.By("7.1.  Verify from external frr container")
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "120s", "5s").Should(o.BeTrue(), "Not all podNetwork are advertised to external frr router after OVNK restart")
		e2e.Logf("SUCCESS - BGP default network is still correctly advertised to external after OVNK restart !!!")

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s is still correctly advertised to external after OVNK restart !!!", udnName, ns2)

		exutil.By("7.2. Verify from cluster nodes")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table check for routes of defaulet network on node %s failed after OVNK restart", node))
			result = verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes for UDN %s as expected after OVNK restart", node, udnName))
		}
		e2e.Logf("SUCCESS - BGP default network and UDN are still correctly advertised to other cluster nodes after OVNK restart !!!")

		exutil.By("8. Verify each pod can still be accessed from external after OVNK restart because their default network and UDN remain advertised")
		Curlexternal2PodPass(oc, host, allNS[0], testpods[0].name)
		Curlexternal2UDNPodPass(oc, host, allNS[1], testpods[1].name)

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-78344-route advertisement recovery for default network and UDN after frr-k8s pods restart [Disruptive]", func() {

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			raTemplate          = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			statefulSetHelloPod = filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
			udnName             = "udn-network-78344"
			ipStackType         = checkIPStackType(oc)
		)

		exutil.By("1. From IP routing table, verify external routes and other cluster nodes' default podnetwork are learned to each cluster node")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}

		exutil.By("2. Get first namespace for default network, create an UDN namespace and UDN in UDN namespace")
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		allNS := []string{ns1, ns2}

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

		createGeneralUDNCRD(oc, ns2, udnName, ipv4cidr, ipv6cidr, cidr, "layer3")

		//label userdefinednetwork with label app=udn
		setUDNLabel(oc, ns2, udnName, "app=udn")

		exutil.By("3. Create RA to advertise the UDN network")
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

		exutil.By("4.  Verify  UDN network is advertised to external and other cluster nodes")
		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns2+"_"+udnName)
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertised to external !!!", udnName, ns2)

		for _, node := range allNodes {
			result := verifyBGPRoutesOnClusterNode(oc, node, externalFRRIP2, externalFRRIP1, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes as expected", node))
		}
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertised to other cluster nodes !!!", udnName, ns2)

		exutil.By("5. Create a test pod in each namespace, verify each pod can be accessed from external because their default network and UDN are advertised")
		var testpods []string
		for _, ns := range allNS {
			createResourceFromFile(oc, ns, statefulSetHelloPod)
			podErr := waitForPodWithLabelReady(oc, ns, "app=hello")
			exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
			helloPodname := getPodName(oc, ns, "app=hello")
			o.Expect(len(helloPodname)).Should(o.Equal(1))
			testpods = append(testpods, helloPodname[0])
		}
		Curlexternal2PodPass(oc, host, allNS[0], testpods[0])
		Curlexternal2UDNPodPass(oc, host, allNS[1], testpods[1])

		exutil.By("6. Restart frr-k8s pods.\n")
		defer waitForPodWithLabelReady(oc, frrNamespace, "app=frr-k8s")
		defer waitForPodWithLabelReady(oc, frrNamespace, "component=frr-k8s-webhook-server")
		delPodErr := oc.AsAdmin().Run("delete").Args("pod", "-l", "app=frr-k8s", "-n", frrNamespace).Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())
		delPodErr = oc.AsAdmin().Run("delete").Args("pod", "-l", "component=frr-k8s-webhook-server", "-n", frrNamespace).Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())

		result := areFRRPodsReady(oc, frrNamespace)
		o.Expect(result).To(o.BeTrue(), "Not all frr-k8s pods fully recovered from restart")

		// Make sure frr-k8s ds successfully rolled out after restart
		status, err := oc.AsAdmin().WithoutNamespace().Run("rollout").Args("status", "-n", frrNamespace, "ds", "frr-k8s", "--timeout", "5m").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(status, "successfully rolled out")).To(o.BeTrue(), "frr-k8s ds did not successfully roll out")

		// wait for routs to be re-advertised
		time.Sleep(60 * time.Second)

		exutil.By("7.  Verify default network and UDN advertisements after frr-k8s pods restart")
		exutil.By("7.1.  Verify from external frr container")
		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "120s", "5s").Should(o.BeTrue(), "Not all podNetwork are advertised to external frr router after frr-k8s pods restart")
		e2e.Logf("SUCCESS - BGP default network is still correctly advertised to external after frr-k8s pods restart !!!")

		o.Eventually(func() bool {
			result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")
		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s is still correctly advertised to external after frr-k8s pods restart !!!", udnName, ns2)

		exutil.By("7.2. Verify from cluster nodes")
		for _, node := range allNodes {
			result := verifyIPRoutesOnClusterNode(oc, node, externalFRRIP1, allNodes, podNetwork1Map, podNetwork2Map, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table check for routes of defaulet network on node %s failed after frr-k8s pods restart", node))
			result = verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			o.Expect(result).To(o.BeTrue(), fmt.Sprintf("ip routing table of node %s did not have all bgp routes for UDN %s as expected after frr-k8s pods restart", node, udnName))
		}
		e2e.Logf("SUCCESS - BGP default network and UDN are still correctly advertised to other cluster nodes after frr-k8s pods restart !!!")

		exutil.By("8. Verify each pod can still be accessed from external after frr-k8s pods restart because their default network and UDN remain advertised")
		// If stateful test pod(s) happen to be on rebooted node, pods would be recreated, wait for pods to be ready
		var testpods2 []string
		for _, ns := range allNS {
			podErr := waitForPodWithLabelReady(oc, ns, "app=hello")
			exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
			helloPodname := getPodName(oc, ns, "app=hello")
			testpods2 = append(testpods2, helloPodname[0])
		}
		Curlexternal2PodPass(oc, host, allNS[0], testpods2[0])
		Curlexternal2UDNPodPass(oc, host, allNS[1], testpods2[1])
	})
	g.It("Author:zzhao-Critical-79214-UDN pod with NodePort and externalTrafficPolicy is Local/cluster service when BGP is advertise in LGW and SGW mode (UDN layer3)", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			raTemplate             = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			ipFamilyPolicy         = "SingleStack"
			udnName                = "udn-79214-l3"
		)

		exutil.By("0. Get three worker nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 3 {
			g.Skip("This case requires 3 nodes, but the cluster has less than three nodes")
		}

		exutil.By("1. Create two namespaces, first one is for default network and second is for UDN and then label namespaces")
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		ns := []string{ns1, ns2}
		for _, namespace := range ns {
			err = exutil.SetNamespacePrivileged(oc, namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("2. Create UDN CRD in ns2")
		var cidr, ipv4cidr, ipv6cidr string
		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
				ipFamilyPolicy = "PreferDualStack"
			}
		}
		createGeneralUDNCRD(oc, ns2, udnName, ipv4cidr, ipv6cidr, cidr, "layer3")

		exutil.By("Create RA to advertise the UDN")

		setUDNLabel(oc, ns2, udnName, "app=udn")
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

		exutil.By("Check the UDN network was advertised on worker node")

		UDNnetwork_ipv6_ns1, UDNnetwork_ipv4_ns1 := getHostPodNetwork(oc, allNodes, ns2+"_"+udnName)
		o.Eventually(func() bool {
			result := verifyBGPRoutesOnClusterNode(oc, nodeList.Items[0].Name, externalFRRIP2, externalFRRIP1, allNodes, UDNnetwork_ipv4_ns1, UDNnetwork_ipv6_ns1, nodesIP1Map, nodesIP2Map, true)
			return result
		}, "60s", "10s").Should(o.BeTrue(), "BGP UDN route advertisement did not succeed!!")

		e2e.Logf("SUCCESS - BGP UDN network %s for namespace %s advertise!!!", udnName, ns2)
		exutil.By("3. Create two pods and nodeport service with externalTrafficPolicy=Local in ns1 and ns2")
		nodeportsLocal := []string{}
		pods := make([]pingPodResourceNode, 2)
		svcs := make([]genericServiceResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("3.%d Create pod and nodeport service with externalTrafficPolicy=Local in %s", i, ns[i]))
			for j := 0; j < 2; j++ {
				pods[j] = pingPodResourceNode{
					name:      "hello-pod" + strconv.Itoa(j),
					namespace: ns[i],
					nodename:  nodeList.Items[j].Name,
					template:  pingPodNodeTemplate,
				}
				pods[j].createPingPodNode(oc)
				waitPodReady(oc, ns[i], pods[j].name)
			}
			svcs[i] = genericServiceResource{
				servicename:           "test-service" + strconv.Itoa(i),
				namespace:             ns[i],
				protocol:              "TCP",
				selector:              "hello-pod",
				serviceType:           "NodePort",
				ipFamilyPolicy:        ipFamilyPolicy,
				internalTrafficPolicy: "Cluster",
				externalTrafficPolicy: "Local",
				template:              genericServiceTemplate,
			}
			svcs[i].createServiceFromParams(oc)
			nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns[i], svcs[i].servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeportsLocal = append(nodeportsLocal, nodePort)
		}

		exutil.By("4. Validate pod/host to nodeport service with externalTrafficPolicy=Local traffic")
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("4.1.%d Validate pod to nodeport service with externalTrafficPolicy=Local traffic in %s", i, ns[i]))
			//comment due to bug https://issues.redhat.com/browse/OCPBUGS-50636
			//CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[0].Name, nodeportsLocal[i])
			//CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[1].Name, nodeportsLocal[i])
			//CurlPod2NodePortFail(oc, ns[i], pods[i].name, nodeList.Items[2].Name, nodeportsLocal[i])
		}
		exutil.By("4.2 Validate host to nodeport service with externalTrafficPolicy=Local traffic on default network")
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[0].Name, nodeportsLocal[0])
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[1].Name, nodeportsLocal[0])
		exutil.By("4.3 Validate UDN pod to default network nodeport service with externalTrafficPolicy=Local traffic")

		//comment due to bug: https://issues.redhat.com/browse/OCPBUGS-52278
		//CurlPod2NodePortFail(oc, ns[1], pods[1].name, nodeList.Items[0].Name, nodeportsLocal[0])
		CurlPod2NodePortPass(oc, ns[1], pods[1].name, nodeList.Items[0].Name, nodeportsLocal[0])

		exutil.By("4.4 Validate host to nodeport service with externalTrafficPolicy=Local traffic on UDN network")
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[0].Name, nodeportsLocal[1])
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[1].Name, nodeportsLocal[1])

		exutil.By("5. Create nodeport service with externalTrafficPolicy=Cluster in ns1 and ns2")
		nodeportsCluster := []string{}
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("5.%d Create pod and nodeport service with externalTrafficPolicy=Cluster in %s", i, ns[i]))
			removeResource(oc, true, true, "svc", "test-service"+strconv.Itoa(i), "-n", ns[i])
			svcs[i].externalTrafficPolicy = "Cluster"
			svcs[i].createServiceFromParams(oc)
			nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns[i], svcs[i].servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeportsCluster = append(nodeportsCluster, nodePort)
		}

		exutil.By("6. Validate pod/host to nodeport service with externalTrafficPolicy=Cluster traffic")
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("6.1.%d Validate pod to nodeport service with externalTrafficPolicy=Cluster traffic in %s", i, ns[i]))
			CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[0].Name, nodeportsCluster[i])
			CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[1].Name, nodeportsCluster[i])
			CurlPod2NodePortPass(oc, ns[i], pods[i].name, nodeList.Items[2].Name, nodeportsCluster[i])
		}
		exutil.By("6.2 Validate host to nodeport service with externalTrafficPolicy=Cluster traffic on default network")
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[0].Name, nodeportsCluster[0])
		CurlNodePortPass(oc, nodeList.Items[2].Name, nodeList.Items[1].Name, nodeportsCluster[0])
		exutil.By("6.3 Validate UDN pod to default network nodeport service with externalTrafficPolicy=Cluster traffic")
		CurlPod2NodePortFail(oc, ns[1], pods[1].name, nodeList.Items[0].Name, nodeportsLocal[0])
		CurlPod2NodePortFail(oc, ns[1], pods[1].name, nodeList.Items[1].Name, nodeportsLocal[0])
	})

	g.It("Author:meinli-Critical-79212-Validate pod2Service by BGP UDN in LGW and SGW (Layer3)", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			testPodFile            = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			raTemplate             = filepath.Join(buildPruningBaseDir, "bgp/ra_template.yaml")
			udnNames               = []string{"udn-network-ns1", "udn-network-ns2"}
			ipFamilyPolicy         = "SingleStack"
		)

		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This test requires at least 2 worker nodes which is not fulfilled.")
		}

		exutil.By("1. Obtain three namespaces, first and second for UDN, third for default network")
		oc.CreateNamespaceUDN()
		udnNS := []string{oc.Namespace()}
		oc.CreateNamespaceUDN()
		udnNS = append(udnNS, oc.Namespace())
		oc.SetupProject()
		ns3 := oc.Namespace()

		exutil.By("2. Create UDN CRD in udnNS")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr []string
		cidr = []string{"10.150.0.0/16", "10.160.0.0/16"}
		ipv4cidr = []string{"10.150.0.0/16", "10.160.0.0/16"}
		ipv6cidr = []string{"2010:100:200::0/48", "2010:200:200::0/48"}
		if ipStackType == "dualstack" {
			ipFamilyPolicy = "PreferDualStack"
		}
		if ipStackType == "ipv6single" {
			cidr = []string{"2010:100:200::0/48", "2010:200:200::0/48"}
		}

		for i, ns := range udnNS {
			createGeneralUDNCRD(oc, ns, udnNames[i], ipv4cidr[i], ipv6cidr[i], cidr[i], "layer3")
			//label userdefinednetwork with label app=udn
			setUDNLabel(oc, ns, udnNames[i], "app=udn")
		}

		exutil.By("3. Create RA to advertise the UDN network")
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

		exutil.By("4. Verify two UDNs with matching networkSelector are advertised")
		for i := 0; i < 2; i++ {
			UDNnetwork_ipv6_ns, UDNnetwork_ipv4_ns := getHostPodNetwork(oc, allNodes, udnNS[i]+"_"+udnNames[i])
			o.Eventually(func() bool {
				result := verifyIPRoutesOnExternalFrr(host, allNodes, UDNnetwork_ipv4_ns, UDNnetwork_ipv6_ns, nodesIP1Map, nodesIP2Map, true)
				return result
			}, "60s", "10s").Should(o.BeTrue(), "UDN with matching networkSelector was not advertised as expected!!")
		}

		exutil.By("5. Create three pods: one as a backend pod and the other two as client pods on the same/different nodes in ns1.")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod",
			namespace: udnNS[0],
			nodename:  nodeList.Items[0].Name,
			template:  pingPodTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		pods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			pods[i] = pingPodResourceNode{
				name:      "hello-pod-" + strconv.Itoa(i),
				namespace: udnNS[0],
				nodename:  nodeList.Items[i].Name,
				template:  pingPodTemplate,
			}
			pods[i].createPingPodNode(oc)
			waitPodReady(oc, pods[i].namespace, pods[i].name)
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("-n", udnNS[0], "pod", pods[i].name, "name=hello-pod-"+strconv.Itoa(i), "--overwrite=true").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. create a ClusterIP service in ns1")
		svc := genericServiceResource{
			servicename:           "test-service-udn",
			namespace:             udnNS[0],
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        ipFamilyPolicy,
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)
		svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", udnNS[0], svc.servicename).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svcOutput).Should(o.ContainSubstring(svc.servicename))

		exutil.By("7. Verify ClusterIP service can be accessed from pods on same/different nodes in ns1.")
		for _, pod := range pods {
			CurlPod2SvcPass(oc, udnNS[0], udnNS[0], pod.name, svc.servicename)
		}

		exutil.By("8. Create udn pods in ns2")
		createResourceFromFile(oc, udnNS[1], testPodFile)
		err = waitForPodWithLabelReady(oc, udnNS[1], "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNameNS2 := getPodName(oc, udnNS[1], "name=test-pods")

		//exutil.By("9. Validate same/different host to pod")
		//https://issues.redhat.com/browse/OCPBUGS-51165
		//CurlNode2PodPassUDN(oc, nodeList.Items[0].Name, udnNS[0], pods[0].name)
		//CurlNode2PodPassUDN(oc, nodeList.Items[1].Name, udnNS[0], pods[0].name)

		//exutil.By("10. Validate pod was isolated with different udn network")
		// https://issues.redhat.com/browse/OCPBUGS-52462
		//CurlPod2PodFailUDN(oc, udnNS[1], testPodNameNS2[0], udnNS[0], pods[0].name)

		exutil.By("11. Verify different udn network, service was isolated")
		CurlPod2SvcFail(oc, udnNS[1], udnNS[0], testPodNameNS2[0], svc.servicename)

		exutil.By("12. Create service and pods on default network")
		createResourceFromFile(oc, ns3, testPodFile)
		err = waitForPodWithLabelReady(oc, ns3, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNameNS3 := getPodName(oc, ns3, "name=test-pods")

		exutil.By("13. Not be able to access udn service from default network.")
		CurlPod2SvcFail(oc, ns3, udnNS[0], testPodNameNS3[0], svc.servicename)
		//exutil.By("14. Not be able to access default network service from udn network.")
		//https://issues.redhat.com/browse/OCPBUGS-52278
		//CurlPod2SvcFail(oc, udnNS[0], ns3, pods[0].name, "test-service")
		exutil.By("15. Validate that the UDN pod is isolated from the default network pod.")
		CurlPod2PodFail(oc, udnNS[0], pods[0].name, ns3, testPodNameNS3[0])

		exutil.By("16. Update internalTrafficPolicy as Local for udn service in ns1.")
		patch := `[{"op": "replace", "path": "/spec/internalTrafficPolicy", "value": "Local"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("service", svc.servicename, "-n", udnNS[0], "-p", patch, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("16.1. Verify ClusterIP service can be accessed from pods[0] which is deployed on same node as service back-end pod.")
		CurlPod2SvcPass(oc, udnNS[0], udnNS[0], pods[0].name, svc.servicename)
		exutil.By("16.2. Verify ClusterIP service can NOT be accessed from pods[1] which is deployed on different node as service back-end pod.")
		CurlPod2SvcFail(oc, udnNS[0], udnNS[0], pods[1].name, svc.servicename)
	})

})
