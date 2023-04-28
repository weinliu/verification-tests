package networking

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-services", exutil.KubeConfigPath())
	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-50347-internalTrafficPolicy set Local for pod/node to service access", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Create a namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("create 1st hello pod in ns1")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Local",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		svc.ipFamilyPolicy = "SingleStack"
		svc.createServiceFromParams(oc)

		g.By("Create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("Create a pod hello-pod2 in second namespace, pod located the same node")
		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod2.name)

		g.By("Create second pod hello-pod3 in second namespace, pod located on the different node")
		pod3 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns2,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod3.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod3.name)

		g.By("curl from hello-pod2 to service:port")
		CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

		g.By("curl from hello-pod3 to service:port should be failling")
		CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

		g.By("Curl from node0 to service:port")
		//Due to bug 2078691,skip below step for now.
		//CurlNode2SvcPass(oc, pod1.nodename, ns1,"test-service")
		g.By("Curl from node1 to service:port")
		CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		ipStackType := checkIPStackType(oc)

		if ipStackType == "dualstack" {
			g.By("Delete testservice from ns")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Checking pod to svc:port behavior now on with PreferDualStack Service")
			svc.ipFamilyPolicy = "PreferDualStack"
			svc.createServiceFromParams(oc)
			g.By("curl from hello-pod2 to service:port")
			CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

			g.By("curl from hello-pod3 to service:port should be failling")
			CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

			g.By("Curl from node0 to service:port")
			//Due to bug 2078691,skip below step for now.
			//CurlNode2SvcPass(oc, pod1.nodename, ns1,"test-service")
			g.By("Curl from node1 to service:port")
			CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		}
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-50348-internalTrafficPolicy set Local for pod/node to service access with hostnetwork pod backend. [Serial]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			hostNetworkPodTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-hostnetwork-specific-node-template.yaml")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Create a namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()
		//Required for hostnetwork pod
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-group", "privileged", "system:serviceaccounts:"+ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create 1st hello pod in ns1")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  hostNetworkPodTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Local",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		svc.ipFamilyPolicy = "SingleStack"
		svc.createServiceFromParams(oc)

		g.By("Create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("Create a pod hello-pod2 in second namespace, pod located the same node")
		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod2.name)

		g.By("Create second pod hello-pod3 in second namespace, pod located on the different node")
		pod3 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns2,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod3.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod3.name)

		g.By("curl from hello-pod2 to service:port")
		CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

		g.By("curl from hello-pod3 to service:port should be failing")
		CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

		g.By("Curl from node1 to service:port")
		CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		ipStackType := checkIPStackType(oc)

		if ipStackType == "dualstack" {
			g.By("Delete testservice from ns")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Checking pod to svc:port behavior now on with PreferDualStack Service")
			svc.ipFamilyPolicy = "PreferDualStack"
			svc.createServiceFromParams(oc)
			g.By("curl from hello-pod2 to service:port")
			CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

			g.By("curl from hello-pod3 to service:port should be failing")
			CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

			g.By("Curl from node1 to service:port")
			CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		}
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-Medium-57344-Add support for service session affinity timeout", func() {
		//Bug: https://issues.redhat.com/browse/OCPBUGS-4502
		var (
			buildPruningBaseDir         = exutil.FixturePath("testdata", "networking")
			servicesBaseDir             = exutil.FixturePath("testdata", "networking/services")
			pingPodTemplate             = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			sessionAffinitySvcv4        = filepath.Join(servicesBaseDir, "sessionaffinity-svcv4.yaml")
			sessionAffinitySvcdualstack = filepath.Join(servicesBaseDir, "sessionaffinity-svcdualstack.yaml")
			sessionAffinityPod1         = filepath.Join(servicesBaseDir, "sessionaffinity-pod1.yaml")
			sessionAffinityPod2         = filepath.Join(servicesBaseDir, "sessionaffinity-pod2.yaml")
		)

		ns1 := oc.Namespace()

		g.By("create two pods which will be the endpoints for sessionaffinity service in ns1")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinityPod1, "-n", ns1).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinityPod2, "-n", ns1).Execute()
		createResourceFromFile(oc, ns1, sessionAffinityPod1)
		waitPodReady(oc, ns1, "blue-pod-1")
		createResourceFromFile(oc, ns1, sessionAffinityPod2)
		waitPodReady(oc, ns1, "blue-pod-2")

		g.By("create a testing pod in ns1")
		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		pod1.createPingPod(oc)
		waitPodReady(oc, ns1, pod1.name)

		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			g.By("test ipv4 singlestack cluster")
			g.By("create a sessionaffinity service in ns1")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinitySvcv4, "-n", ns1).Execute()
			createsvcerr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sessionAffinitySvcv4, "-n", ns1).Execute()
			o.Expect(createsvcerr).NotTo(o.HaveOccurred())
			svcoutput, svcerr := oc.AsAdmin().Run("get").Args("service", "-n", ns1).Output()
			o.Expect(svcerr).NotTo(o.HaveOccurred())
			o.Expect(svcoutput).To(o.ContainSubstring("sessionaffinitysvcv4"))
			serviceIPv4 := getSvcIPv4(oc, ns1, "sessionaffinitysvcv4")

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will LB after curl sleep more than 10s
			g.By("Traffic will LB to two endpoints with sleep 15s in curl")
			trafficoutput, trafficerr := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 11; done")
			o.Expect(trafficerr).NotTo(o.HaveOccurred())
			if strings.Contains(trafficoutput, "Hello Blue Pod-1") && strings.Contains(trafficoutput, "Hello Blue Pod-2") {
				e2e.Logf("Pass : Traffic LB to two endpoints when curl sleep more than 10s")
			} else {
				e2e.Failf("Fail: Traffic does not LB to two endpoints when curl sleep more than 10s")
			}

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will not LB after curl sleep less than 10s
			g.By("Traffic will not LB to two endpoints without sleep 15s in curl")
			trafficoutput1, trafficerr1 := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 9; done")
			o.Expect(trafficerr1).NotTo(o.HaveOccurred())
			if (strings.Contains(trafficoutput1, "Hello Blue Pod-1") && !strings.Contains(trafficoutput1, "Hello Blue Pod-2")) || (strings.Contains(trafficoutput1, "Hello Blue Pod-2") && !strings.Contains(trafficoutput1, "Hello Blue Pod-1")) {
				e2e.Logf("Pass : Traffic does not LB to two endpoints when curl sleep less than 10s")
			} else {
				e2e.Failf("Fail: Traffic LB to two endpoints when curl sleep less than 10s")
			}
		}

		if ipStackType == "dualstack" {
			g.By("test dualstack cluster")
			g.By("create a sessionaffinity service in ns1")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinitySvcdualstack, "-n", ns1).Execute()
			createsvcerr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sessionAffinitySvcdualstack, "-n", ns1).Execute()
			o.Expect(createsvcerr).NotTo(o.HaveOccurred())
			svcoutput, svcerr := oc.AsAdmin().Run("get").Args("service", "-n", ns1).Output()
			o.Expect(svcerr).NotTo(o.HaveOccurred())
			o.Expect(svcoutput).To(o.ContainSubstring("sessionaffinitysvcdualstack"))
			serviceIPv4 := getSvcIPv4(oc, ns1, "sessionaffinitysvcdualstack")
			serviceIPv6 := getSvcIPv6(oc, ns1, "sessionaffinitysvcdualstack")

			// Test ipv4 traffic in dualstack cluster
			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will LB after curl sleep more than 10s
			g.By("Traffic will LB to two endpoints with sleep 15s in curl")
			trafficoutput, trafficerr := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 11; done")
			o.Expect(trafficerr).NotTo(o.HaveOccurred())
			if strings.Contains(trafficoutput, "Hello Blue Pod-1") && strings.Contains(trafficoutput, "Hello Blue Pod-2") {
				e2e.Logf("Pass : Traffic LB to two endpoints when curl sleep more than 10s")
			} else {
				e2e.Failf("Fail: Traffic does not LB to two endpoints when curl sleep more than 10s")
			}

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will not LB after curl sleep less than 10s
			g.By("Traffic will not LB to two endpoints without sleep 15s in curl")
			trafficoutput1, trafficerr1 := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 9; done")
			o.Expect(trafficerr1).NotTo(o.HaveOccurred())
			if (strings.Contains(trafficoutput1, "Hello Blue Pod-1") && !strings.Contains(trafficoutput1, "Hello Blue Pod-2")) || (strings.Contains(trafficoutput1, "Hello Blue Pod-2") && !strings.Contains(trafficoutput1, "Hello Blue Pod-1")) {
				e2e.Logf("Pass : Traffic does not LB to two endpoints when curl sleep less than 10s")
			} else {
				e2e.Failf("Fail: Traffic LB to two endpoints when curl sleep less than 10s")
			}

			// Tes ipv6 traffic in dualstack cluster
			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will LB after curl sleep more than 10s
			g.By("Traffic will LB to two endpoints with sleep 15s in curl")
			v6trafficoutput, v6trafficerr := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl -g -6 ["+serviceIPv6+"]:8080; sleep 11; done")
			o.Expect(v6trafficerr).NotTo(o.HaveOccurred())
			if strings.Contains(v6trafficoutput, "Hello Blue Pod-1") && strings.Contains(v6trafficoutput, "Hello Blue Pod-2") {
				e2e.Logf("Pass : Traffic LB to two endpoints when curl sleep more than 10s")
			} else {
				e2e.Failf("Fail: Traffic does not LB to two endpoints when curl sleep more than 10s")
			}

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will not LB after curl sleep less than 10s
			g.By("Traffic will not LB to two endpoints without sleep 15s in curl")
			v6trafficoutput1, v6trafficerr1 := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl -g -6 ["+serviceIPv6+"]:8080; sleep 9; done")
			o.Expect(v6trafficerr1).NotTo(o.HaveOccurred())
			if (strings.Contains(v6trafficoutput1, "Hello Blue Pod-1") && !strings.Contains(v6trafficoutput1, "Hello Blue Pod-2")) || (strings.Contains(v6trafficoutput1, "Hello Blue Pod-2") && !strings.Contains(v6trafficoutput1, "Hello Blue Pod-1")) {
				e2e.Logf("Pass : Traffic does not LB to two endpoints when curl sleep less than 10s")
			} else {
				e2e.Failf("Fail: Traffic LB to two endpoints when curl sleep less than 10s")
			}
		}
	})
	// author: asood@redhat.com
	g.It("Longduration-NonPreRelease-Author:asood-High-62293-Validate all the constructs are created on logical routers and logical switches for a service type loadbalancer. [Disruptive]", func() {
		// Bug: https://issues.redhat.com/browse/OCPBUGS-5930 (Duplicate bug https://issues.redhat.com/browse/OCPBUGS-7000)
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			svcEndpoints           []string
			lsConstructs           []string
			lrConstructs           []string
		)
		platform := exutil.CheckPlatform(oc)
		//vSphere does not have LB service support yet
		e2e.Logf("platform %s", platform)
		if !(strings.Contains(platform, "gcp") || strings.Contains(platform, "aws") || strings.Contains(platform, "azure")) {
			g.Skip("Skip for non-supported auto scaling machineset platforms!!")
		}
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("OVN constructs would not be on the cluster")
		}
		workerNodes, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		allNodes, errNodes := exutil.GetAllNodes(oc)
		o.Expect(errNodes).NotTo(o.HaveOccurred())

		if len(workerNodes) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Get namespace")
		ns := oc.Namespace()

		g.By("create 1st hello pod in ns1")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns,
			nodename:  workerNodes[0],
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns, pod1.name)

		g.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "LoadBalancer",
			ipFamilyPolicy:        "SingleStack",
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "Cluster",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		g.By("Get endpoints of loadbalancer services")
		svcEndpoints = append(svcEndpoints, getServiceEndpoints(oc, svc.servicename, svc.namespace))
		svcEndpoints = append(svcEndpoints, getServiceEndpoints(oc, "kubernetes", "default"))

		g.By("Get logical routes and switches existing nodes")
		lsConstructs = getOVNConstruct(oc, "ls-list", allNodes)
		o.Expect(lsConstructs).NotTo(o.BeEmpty())
		o.Expect(len(lsConstructs) == len(allNodes)).Should(o.BeTrue())
		//Get logical routes only for worker nodes as kube API service does not have entries for master nodes
		lrConstructs = getOVNConstruct(oc, "lr-list", workerNodes)
		o.Expect(lrConstructs).NotTo(o.BeEmpty())
		o.Expect(len(lsConstructs) == len(allNodes)).Should(o.BeTrue())

		g.By("Validate all the entries are created for user created service and kubernetes/ kube API")
		for i := 0; i < len(svcEndpoints); i++ {
			o.Expect(getOVNLBContructs(oc, "ls-lb-list", svcEndpoints[i], lsConstructs)).To(o.BeTrue())
			o.Expect(getOVNLBContructs(oc, "lr-lb-list", svcEndpoints[i], lrConstructs)).To(o.BeTrue())
		}

		g.By("Create a new machineset with 2")
		var newNodes []string
		exutil.SkipConditionally(oc)
		machinesetName := "machineset-62293"
		ms := exutil.MachineSetDescription{machinesetName, 2}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		exutil.WaitForMachinesRunning(oc, 2, machinesetName)
		machineName := exutil.GetMachineNamesFromMachineSet(oc, machinesetName)
		nodeName0 := exutil.GetNodeNameFromMachine(oc, machineName[0])
		nodeName1 := exutil.GetNodeNameFromMachine(oc, machineName[1])
		newNodes = append(newNodes, nodeName0)
		newNodes = append(newNodes, nodeName1)
		e2e.Logf("The nodes %s and %s added successfully", nodeName0, nodeName1)

		g.By("Get logical routes and switches new nodes")
		lsConstructs = getOVNConstruct(oc, "ls-list", newNodes)
		o.Expect(lsConstructs).NotTo(o.BeEmpty())
		o.Expect(len(lsConstructs) == len(newNodes)).Should(o.BeTrue())
		lrConstructs = getOVNConstruct(oc, "lr-list", newNodes)
		o.Expect(lrConstructs).NotTo(o.BeEmpty())
		o.Expect(len(lrConstructs) == len(newNodes)).Should(o.BeTrue())

		g.By("Validate all the entries are created for service new nodes")
		for i := 0; i < len(svcEndpoints); i++ {
			o.Expect(getOVNLBContructs(oc, "ls-lb-list", svcEndpoints[i], lsConstructs)).To(o.BeTrue())
			o.Expect(getOVNLBContructs(oc, "lr-lb-list", svcEndpoints[i], lrConstructs)).To(o.BeTrue())
		}
		g.By("Validate kubernetes service is reachable from new nodes")
		for i := 0; i < len(newNodes); i++ {
			output, err := exutil.DebugNodeWithChroot(oc, newNodes[i], "bash", "-c", "curl -s -k https://172.30.0.1/healthz --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, "ok")).To(o.BeTrue())
		}

	})
})
