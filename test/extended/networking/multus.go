package networking

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN multus", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-multus", exutil.KubeConfigPath())

	// OCP-46387 failed in 4.14 due to https://issues.redhat.com/browse/OCPBUGS-11082 and https://issues.redhat.com/browse/NP-752
	// Enable this case until Dev fix the issue
	/*
		// author: weliang@redhat.com
		g.It("Author:weliang-Medium-46387-[BZ 1896533] network operator degraded due to additionalNetwork in non-existent namespace. [Disruptive]", func() {
			var (
				patchSResource = "networks.operator.openshift.io/cluster"
				patchInfo      = fmt.Sprintf("{\"spec\":{\"additionalNetworks\": [{\"name\": \"secondary\",\"namespace\":\"ocp-46387\",\"simpleMacvlanConfig\": {\"ipamConfig\": {\"staticIPAMConfig\": {\"addresses\": [{\"address\": \"10.1.1.0/24\"}] },\"type\": \"static\"}},\"type\": \"SimpleMacvlan\"}]}}")
			)

			g.By("create new namespace")
			namespace := fmt.Sprintf("ocp-46387")
			err := oc.Run("new-project").Args(namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()

			g.By("Configure network-attach-definition through network operator")
			patchResourceAsAdmin(oc, patchSResource, patchInfo)
			defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()

			//Testing will exit when network operator is in abnormal state during 60 seconding of checking operator.
			g.By("Check NetworkOperatorStatus")
			checkNetworkOperatorState(oc, 10, 60)

			g.By("Delete the namespace")
			nsErr := oc.AsAdmin().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()
			o.Expect(nsErr).NotTo(o.HaveOccurred())

			//Testing will exit when network operator is in abnormal state during 60 seconding of checking operator.
			g.By("Check NetworkOperatorStatus after deleting namespace")
			checkNetworkOperatorState(oc, 10, 60)
		})
	*/

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-High-57589-Whereabouts CNI timesout while iterating exclude range", func() {
		//https://issues.redhat.com/browse/OCPBUGS-2948 : Whereabouts CNI timesout while iterating exclude range

		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			netAttachDefFile1   = filepath.Join(buildPruningBaseDir, "multus/ipv6-excludes-largeranges-NAD.yaml")
			multusPodTemplate   = filepath.Join(buildPruningBaseDir, "multinetworkpolicy/MultiNetworkPolicy-pod-template.yaml")
		)

		ns1 := oc.Namespace()

		g.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}

		g.By("Create a custom resource network-attach-defintion in tested namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", netAttachDefFile1, "-n", ns1).Execute()
		netAttachDefErr := oc.AsAdmin().Run("create").Args("-f", netAttachDefFile1, "-n", ns1).Execute()
		o.Expect(netAttachDefErr).NotTo(o.HaveOccurred())
		netAttachDefOutput, netAttachDefOutputErr := oc.Run("get").Args("net-attach-def", "-n", ns1).Output()
		o.Expect(netAttachDefOutputErr).NotTo(o.HaveOccurred())
		o.Expect(netAttachDefOutput).To(o.ContainSubstring("nad-w-excludes"))

		g.By("Create a multus pod to use above network-attach-defintion")
		ns1MultusPod1 := testPodMultinetwork{
			name:      "ns1-multuspod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			nadname:   "nad-w-excludes",
			labelname: "blue-multuspod",
			template:  multusPodTemplate,
		}
		ns1MultusPod1.createTestPodMultinetwork(oc)
		waitPodReady(oc, ns1MultusPod1.namespace, ns1MultusPod1.name)

		g.By("check the created multus pod to get the right ipv6 CIDR")
		multusPodIPv6 := getPodMultiNetworkIPv6(oc, ns1, ns1MultusPod1.name)
		e2e.Logf("The v6 address of pod's second interface is: %v", multusPodIPv6)
		o.Expect(strings.HasPrefix(multusPodIPv6, "fd43:11f1:3daa:bbaa::")).Should(o.BeTrue())
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-High-59875-Configure ignored namespaces into multus-admission-controller", func() {
		//https://issues.redhat.com/browse/OCPBUGS-6499:Configure ignored namespaces into multus-admission-controller

		ns1 := "openshift-multus"
		expectedOutpu := "-ignore-namespaces"
		g.By("Check multus-admission-controller is configured with ignore-namespaces")
		multusOutput, multusErr := oc.AsAdmin().Run("get").Args("deployment.apps/multus-admission-controller", "-n", ns1, "-o=jsonpath={.spec.template.spec.containers[0].command[2]}").Output()
		exutil.AssertWaitPollNoErr(multusErr, "The deployment.apps/multus-admission-controller is not created")
		o.Expect(multusOutput).To(o.ContainSubstring(expectedOutpu))

		g.By("Check all multus-additional-cni-plugins pods are Running well")
		o.Expect(waitForPodWithLabelReady(oc, ns1, "app=multus-additional-cni-plugins")).NotTo(o.HaveOccurred())
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-59440-Verify whereabouts-reconciler after creating additionalNetworks. [Serial]", func() {
		var (
			patchSResource = "networks.operator.openshift.io/cluster"
			patchInfo      = fmt.Sprintf(`{"spec":{ "additionalNetworks": [{"name": "whereabouts-shim", "namespace": "default","rawCNIConfig":"{\"cniVersion\":\"0.3.0\",\"type\":\"bridge\",\"name\":\"cnitest0\",\"ipam\": {\"type\":\"whereabouts\",\"subnet\":\"192.0.2.0/24\"}}","type":"Raw"}]}}`)
			ns             = "openshift-multus"
		)

		g.By("Check there are no whereabouts-reconciler pods and ds in the openshift-multus namespace before creating additionalNetworks ")
		podStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", "app=whereabouts-reconciler", "-ojsonpath={.items[*].status.conditions[?(@.type==\"Ready\")].status}").Output()
		o.Expect(podStatus).To(o.BeEmpty())
		_, dsErrBefore := oc.AsAdmin().Run("get").Args("daemonset.apps/whereabouts-reconciler", "-n", ns).Output()
		o.Expect(dsErrBefore).To(o.HaveOccurred())

		g.By("Add additionalNetworks through network operator")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()
			g.By("Check NetworkOperatorStatus to ensure the cluster is health after modification")
			checkNetworkOperatorState(oc, 10, 60)
		}()
		patchResourceAsAdmin(oc, patchSResource, patchInfo)

		g.By("Check whereabouts-reconciler pods and ds are created in the openshift-multus namespace after creating additionalNetworks ")
		o.Expect(waitForPodWithLabelReady(oc, ns, "app=whereabouts-reconciler")).NotTo(o.HaveOccurred())
		dsOutput, dsErrAfter := oc.AsAdmin().Run("get").Args("daemonset.apps/whereabouts-reconciler", "-n", ns).Output()
		o.Expect(dsErrAfter).NotTo(o.HaveOccurred())
		o.Expect(dsOutput).To(o.ContainSubstring("whereabouts-reconciler"))

		g.By("Check there are no whereabouts-reconciler pods and ds in the openshift-multus namespace after deleting additionalNetworks ")
		oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()
		o.Eventually(func() bool {
			result := true
			_, err := oc.AsAdmin().Run("get").Args("pod", "-n", ns, "-l", "app=whereabouts-reconciler").Output()
			if err != nil {
				e2e.Logf("Wait for whereabouts-reconciler pods to be deleted")
				result = false
			}
			return result
		}, "60s", "5s").Should(o.BeTrue(), fmt.Sprintf("whereabouts-reconciler pods are not deleted"))
		o.Eventually(func() bool {
			result := true
			_, err := oc.AsAdmin().Run("get").Args("daemonset.apps/whereabouts-reconciler", "-n", ns).Output()
			if err != nil {
				e2e.Logf("Wait for daemonset.apps/whereabouts-reconciler to be deleted")
				result = false
			}
			return result
		}, "60s", "5s").Should(o.BeTrue(), fmt.Sprintf("daemonset.apps/whereabouts-reconciler is not deleted"))
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-64958-Unable to set default-route when istio sidecar is injected. [Serial]", func() {
		//https://issues.redhat.com/browse/OCPBUGS-7844
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			netAttachDefFile    = filepath.Join(buildPruningBaseDir, "multus/istiosidecar-NAD.yaml")
			testPod             = filepath.Join(buildPruningBaseDir, "multus/istiosidecar-pod.yaml")
		)

		exutil.By("Create a new namespace")
		ns1 := "test-64958"
		defer oc.DeleteSpecifiedNamespaceAsAdmin(ns1)
		oc.CreateSpecifiedNamespaceAsAdmin(ns1)

		exutil.By("Create a custom resource network-attach-defintion in the namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", netAttachDefFile, "-n", ns1).Execute()
		netAttachDefErr := oc.AsAdmin().Run("create").Args("-f", netAttachDefFile, "-n", ns1).Execute()
		o.Expect(netAttachDefErr).NotTo(o.HaveOccurred())
		netAttachDefOutput, netAttachDefOutputErr := oc.AsAdmin().Run("get").Args("net-attach-def", "-n", ns1).Output()
		o.Expect(netAttachDefOutputErr).NotTo(o.HaveOccurred())
		o.Expect(netAttachDefOutput).To(o.ContainSubstring("test-nad"))

		exutil.By("Create a pod consuming above network-attach-defintion in ns1")
		createResourceFromFile(oc, ns1, testPod)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=testpod")).NotTo(o.HaveOccurred(), "The test pod in ns/%s is not ready", ns1)

		exutil.By("Check the default-route is created when istio sidecar is injected")
		routeLog, routeErr := execCommandInSpecificPod(oc, ns1, "testpod", "ip route")
		o.Expect(routeErr).NotTo(o.HaveOccurred())
		o.Expect(routeLog).To(o.ContainSubstring("default via 172.19.55.99 dev net1"))
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-Medium-66876-Support Dual Stack IP assignment for whereabouts CNI/IPAM", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			dualstackNADTemplate   = filepath.Join(buildPruningBaseDir, "multus/dualstack-NAD-template.yaml")
			multihomingPodTemplate = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			nadName                = "dualstack"
		)

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has fewer than two nodes")
		}

		exutil.By("Get the name of namespace")
		ns1 := oc.Namespace()

		exutil.By("Create a custom resource network-attach-defintion in the test namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns1).Execute()
		nad1ns1 := dualstackNAD{
			nadname:        nadName,
			namespace:      ns1,
			plugintype:     "macvlan",
			mode:           "bridge",
			ipamtype:       "whereabouts",
			ipv4range:      "192.168.10.0/24",
			ipv6range:      "fd00:dead:beef:10::/64",
			ipv4rangestart: "",
			ipv4rangeend:   "",
			ipv6rangestart: "",
			ipv6rangeend:   "",
			template:       dualstackNADTemplate,
		}
		nad1ns1.createDualstackNAD(oc)

		exutil.By("Check if the network-attach-defintion is created")
		if checkNAD(oc, ns1, nadName) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nadName)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nadName)
		}

		exutil.By("Create 1st pod consuming above network-attach-defintion in ns1")
		pod1 := testMultihomingPod{
			name:       "dualstack-pod-1",
			namespace:  ns1,
			podlabel:   "dualstack-pod1",
			nadname:    nadName,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod1.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=dualstack-pod1")).NotTo(o.HaveOccurred())

		exutil.By("Create 2nd pod consuming above network-attach-defintion in ns1")
		pod2 := testMultihomingPod{
			name:       "dualstack-pod-2",
			namespace:  ns1,
			podlabel:   "dualstack-pod2",
			nadname:    nadName,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod2.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=dualstack-pod2")).NotTo(o.HaveOccurred())

		exutil.By("Get two pods' name")
		podList, podListErr := exutil.GetAllPods(oc, ns1)
		o.Expect(podListErr).NotTo(o.HaveOccurred())
		o.Expect(len(podList)).Should(o.Equal(2))

		exutil.By("Get IPs of the pod1ns1's secondary interface in first namespace.")
		pod1ns1IPv4, pod1ns1IPv6 := getPodMultiNetwork(oc, ns1, podList[0])
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns1IPv6)

		exutil.By("Get IPs of the pod2ns1's secondary interface in first namespace.")
		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, podList[1])
		e2e.Logf("The v4 address of pod2ns1is: %v", pod2ns1IPv4)
		e2e.Logf("The v6 address of pod2ns1is: %v", pod2ns1IPv6)

		g.By("Both ipv4 and ipv6 curl should pass between two pods")
		curlPod2PodMultiNetworkPass(oc, ns1, podList[0], pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[1], pod1ns1IPv4, pod1ns1IPv6)
	})
	g.It("NonHyperShiftHOST-Author:weliang-Medium-69947-The macvlan pod will send Unsolicited Neighbor Advertisements after it is created", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			dualstackNADTemplate   = filepath.Join(buildPruningBaseDir, "multus/dualstack-NAD-template.yaml")
			multihomingPodTemplate = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			nadName                = "whereabouts-dualstack"
			sniffMultusPodTemplate = filepath.Join(buildPruningBaseDir, "multus/sniff-multus-pod-template.yaml")
		)

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("This case requires at least one worker node")
		}

		exutil.By("Get the name of namespace")
		ns := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, ns)
		exutil.SetNamespacePrivileged(oc, ns)

		exutil.By("Create a custom resource network-attach-defintion")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nadName, "-n", ns).Execute()
		nadns := dualstackNAD{
			nadname:        nadName,
			namespace:      ns,
			plugintype:     "macvlan",
			mode:           "bridge",
			ipamtype:       "whereabouts",
			ipv4range:      "192.168.10.0/24",
			ipv6range:      "fd00:dead:beef:10::/64",
			ipv4rangestart: "",
			ipv4rangeend:   "",
			ipv6rangestart: "",
			ipv6rangeend:   "",
			template:       dualstackNADTemplate,
		}
		nadns.createDualstackNAD(oc)

		exutil.By("Check if the network-attach-defintion is created")
		if checkNAD(oc, ns, nadName) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nadName)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nadName)
		}

		exutil.By("Create a sniff pod to capture the traffic from pod's secondary network")
		pod1 := testPodMultinetwork{
			name:      "sniff-pod",
			namespace: ns,
			nodename:  nodeList.Items[0].Name,
			nadname:   nadName,
			labelname: "sniff-pod",
			template:  sniffMultusPodTemplate,
		}
		pod1.createTestPodMultinetwork(oc)
		exutil.AssertWaitPollNoErr(waitForPodWithLabelReady(oc, ns, "name="+pod1.labelname), fmt.Sprintf("Waiting for pod with label name=%s become ready timeout", pod1.labelname))

		exutil.By("The sniff pod start to capture the Unsolicited Neighbor Advertisements from pod's secondary network")
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", ns, pod1.labelname, "bash", "-c",
			`timeout --preserve-status 30 tcpdump -e -i net1 icmp6 and icmp6[0] = 136 -nvvv`).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a pod consuming above network-attach-defintion")
		pod2 := testMultihomingPod{
			name:       "dualstack-pod",
			namespace:  ns,
			podlabel:   "dualstack-pod",
			nadname:    nadName,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod2.createTestMultihomingPod(oc)
		exutil.AssertWaitPollNoErr(waitForPodWithLabelReady(oc, ns, "name="+pod2.podlabel), fmt.Sprintf("Waiting for pod with label name=%s become ready timeout", pod2.podlabel))

		exutil.By("The sniff pod will get Unsolicited Neighbor Advertisements, not neighbor solicitation")
		cmdErr := cmdTcpdump.Wait()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(cmdOutput.String(), "Flags [solicited]")).NotTo(o.BeTrue(), cmdOutput.String())
	})

	g.It("Author:weliang-Medium-72202-[Multus] NAD without configuring network_name. [Disruptive]", func() {
		var (
			buildPruningBaseDir                 = exutil.FixturePath("testdata", "networking")
			nad1Name                            = "ip-overlapping-1"
			nad2Name                            = "ip-overlapping-2"
			pod1Name                            = "ip-overlapping-pod1"
			pod2Name                            = "ip-overlapping-pod2"
			ipv4range1                          = "192.168.20.0/29"
			ipv4range2                          = "192.168.20.0/24"
			interfaceName                       = "net1"
			whereaboutsoverlappingIPNADTemplate = filepath.Join(buildPruningBaseDir, "multus/whereabouts-overlappingIP-NAD-template.yaml")
			multihomingPodTemplate              = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
		)

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("This case requires at least one worker node")
		}

		exutil.By("Get the name of namespace")
		ns := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, ns)
		exutil.SetNamespacePrivileged(oc, ns)

		exutil.By("Configuring first NetworkAttachmentDefinition")
		defer removeResource(oc, true, true, "net-attach-def", nad1Name, "-n", ns)

		nad1 := whereaboutsoverlappingIPNAD{
			nadname:           nad1Name,
			namespace:         ns,
			plugintype:        "macvlan",
			mode:              "bridge",
			ipamtype:          "whereabouts",
			ipv4range:         ipv4range1,
			enableoverlapping: true,
			networkname:       "",
			template:          whereaboutsoverlappingIPNADTemplate,
		}
		nad1.createWhereaboutsoverlappingIPNAD(oc)

		exutil.By("Verifying the configued NetworkAttachmentDefinition")
		if checkNAD(oc, ns, nad1Name) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nad1Name)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nad1Name)
		}

		exutil.By("Configuring pods to get additional network defined in first NAD")
		nad1pod := testMultihomingPod{
			name:       pod1Name,
			namespace:  ns,
			podlabel:   pod1Name,
			nadname:    nad1Name,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		nad1pod.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, "name="+nad1pod.podlabel)).NotTo(o.HaveOccurred())

		exutil.By("Configuring second NetworkAttachmentDefinition with setting true for enable_overlapping_ranges")
		defer removeResource(oc, true, true, "net-attach-def", nad2Name, "-n", ns)
		nad2 := whereaboutsoverlappingIPNAD{
			nadname:           nad2Name,
			namespace:         ns,
			plugintype:        "macvlan",
			mode:              "bridge",
			ipamtype:          "whereabouts",
			ipv4range:         ipv4range2,
			enableoverlapping: true,
			networkname:       "",
			template:          whereaboutsoverlappingIPNADTemplate,
		}
		nad2.createWhereaboutsoverlappingIPNAD(oc)

		exutil.By("Verifying the second NetworkAttachmentDefinition")
		if checkNAD(oc, ns, nad2Name) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nad2Name)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nad2Name)
		}

		exutil.By("Configuring pods for additional network defined in second NAD")
		nad2pod := testMultihomingPod{
			name:       pod2Name,
			namespace:  ns,
			podlabel:   pod2Name,
			nadname:    nad2Name,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		nad2pod.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, "name="+nad2pod.podlabel)).NotTo(o.HaveOccurred())

		ippool1 := "192.168.20.0-29"
		ippool2 := "192.168.20.0-24"
		ipaddress1 := "192.168.20.1"
		ipaddress2 := "192.168.20.2"

		exutil.By("Verifing the correct network_names from ippools")
		ippoolsOutput, ippoolsOutputErr := oc.AsAdmin().Run("get").Args("ippools", "-n", "openshift-multus").Output()
		o.Expect(ippoolsOutputErr).NotTo(o.HaveOccurred())
		o.Expect(ippoolsOutput).To(o.And(o.ContainSubstring(ippool1), o.ContainSubstring(ippool2)))

		exutil.By("Verifing there are no ip overlapping IP addresses from overlappingrangeipreservations")
		overlappingrangeOutput, overlappingrangeOutputErr := oc.AsAdmin().Run("get").Args("overlappingrangeipreservations", "-A", "-n", "openshift-multus").Output()
		o.Expect(overlappingrangeOutputErr).NotTo(o.HaveOccurred())
		o.Expect(overlappingrangeOutput).To(o.And(o.ContainSubstring(ipaddress1), o.ContainSubstring(ipaddress2)))

		exutil.By("Getting IP from pod1's secondary interface")
		pod1List, getPod1Err := exutil.GetAllPodsWithLabel(oc, ns, "name="+nad1pod.podlabel)
		o.Expect(getPod1Err).NotTo(o.HaveOccurred())
		o.Expect(len(pod1List)).NotTo(o.BeEquivalentTo(0))
		pod1Net1IPv4, _ := getPodMultiNetworks(oc, ns, pod1List[0], interfaceName)
		e2e.Logf("The v4 address of pod1's net1 is: %v", pod1Net1IPv4)
		o.Expect(strings.HasPrefix(pod1Net1IPv4, ipaddress1)).Should(o.BeTrue())

		exutil.By("Getting IP from pod2's secondary interface")
		pod2List, getPod2Err := exutil.GetAllPodsWithLabel(oc, ns, "name="+nad2pod.podlabel)
		o.Expect(getPod2Err).NotTo(o.HaveOccurred())
		o.Expect(len(pod2List)).NotTo(o.BeEquivalentTo(0))
		pod2Net1IPv4, _ := getPodMultiNetworks(oc, ns, pod2List[0], interfaceName)
		e2e.Logf("The v4 address of pod2's net1 is: %v", pod2Net1IPv4)
		o.Expect(strings.HasPrefix(pod2Net1IPv4, ipaddress2)).Should(o.BeTrue())

		exutil.By("Deleting the second NetworkAttachmentDefinition and responding pods")
		removeResource(oc, true, true, "net-attach-def", nad2Name, "-n", ns)
		removeResource(oc, true, true, "pod", pod2List[0], "-n", ns)

		exutil.By("Deleting the secondary network_name from ippools")
		removeResource(oc, true, true, "ippools", ippool2, "-n", "openshift-multus")

		exutil.By("Reconfiguring second NetworkAttachmentDefinition with setting false for enable_overlapping_ranges")
		defer removeResource(oc, true, true, "net-attach-def", nad2Name, "-n", ns)
		nad2.enableoverlapping = false
		nad2.createWhereaboutsoverlappingIPNAD(oc)

		exutil.By("Reconfiguring pods for additional network defined in second NAD")
		nad2pod.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, "name="+nad2pod.podlabel)).NotTo(o.HaveOccurred())

		exutil.By("Verifing these is only one IP in overlappingrangeipreservations")
		overlappingrangeOutput1, overlappingrangeOutputErr1 := oc.AsAdmin().Run("get").Args("overlappingrangeipreservations", "-A", "-n", "openshift-multus").Output()
		o.Expect(overlappingrangeOutputErr1).NotTo(o.HaveOccurred())
		o.Expect(overlappingrangeOutput1).To(o.ContainSubstring(ipaddress1))
		o.Expect(overlappingrangeOutput1).NotTo(o.ContainSubstring(ipaddress2))

		exutil.By("Getting IP from pod2's secondary interface")
		podList2, getPod2Err2 := exutil.GetAllPodsWithLabel(oc, ns, "name="+nad2pod.podlabel)
		o.Expect(getPod2Err2).NotTo(o.HaveOccurred())
		o.Expect(len(podList2)).NotTo(o.BeEquivalentTo(0))
		pod3Net1IPv4, _ := getPodMultiNetworks(oc, ns, podList2[0], interfaceName)
		e2e.Logf("The v4 address of pod2's net1 is: %v", pod3Net1IPv4)
		o.Expect(strings.HasPrefix(pod3Net1IPv4, ipaddress1)).Should(o.BeTrue())
	})

	g.It("Author:weliang-Medium-72203-[Multus] NAD using same network_name. [Disruptive]", func() {
		var (
			buildPruningBaseDir                 = exutil.FixturePath("testdata", "networking")
			nad1Name                            = "ip-overlapping-1"
			nad2Name                            = "ip-overlapping-2"
			pod1Name                            = "ip-overlapping-pod1"
			pod2Name                            = "ip-overlapping-pod2"
			ipv4range1                          = "192.168.20.0/29"
			ipv4range2                          = "192.168.20.0/24"
			interfaceName                       = "net1"
			networkName                         = "blue-net"
			whereaboutsoverlappingIPNADTemplate = filepath.Join(buildPruningBaseDir, "multus/whereabouts-overlappingIP-NAD-template.yaml")
			multihomingPodTemplate              = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
		)

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("This case requires at least one worker node")
		}

		exutil.By("Get the name of namespace")
		ns := oc.Namespace()
		defer exutil.RecoverNamespaceRestricted(oc, ns)
		exutil.SetNamespacePrivileged(oc, ns)

		exutil.By("Configuring first NetworkAttachmentDefinition")
		defer removeResource(oc, true, true, "net-attach-def", nad1Name, "-n", ns)

		nad1 := whereaboutsoverlappingIPNAD{
			nadname:           nad1Name,
			namespace:         ns,
			plugintype:        "macvlan",
			mode:              "bridge",
			ipamtype:          "whereabouts",
			ipv4range:         ipv4range1,
			enableoverlapping: true,
			networkname:       networkName,
			template:          whereaboutsoverlappingIPNADTemplate,
		}
		nad1.createWhereaboutsoverlappingIPNAD(oc)

		exutil.By("Verifying the configued NetworkAttachmentDefinition")
		if checkNAD(oc, ns, nad1Name) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nad1Name)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nad1Name)
		}

		exutil.By("Configuring pods to get additional network defined in first NAD")
		nad1pod := testMultihomingPod{
			name:       pod1Name,
			namespace:  ns,
			podlabel:   pod1Name,
			nadname:    nad1Name,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		nad1pod.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, "name="+nad1pod.podlabel)).NotTo(o.HaveOccurred())

		exutil.By("Configuring second NetworkAttachmentDefinition with setting true for enable_overlapping_ranges")
		defer removeResource(oc, true, true, "net-attach-def", nad2Name, "-n", ns)
		nad2 := whereaboutsoverlappingIPNAD{
			nadname:           nad2Name,
			namespace:         ns,
			plugintype:        "macvlan",
			mode:              "bridge",
			ipamtype:          "whereabouts",
			ipv4range:         ipv4range2,
			enableoverlapping: true,
			networkname:       networkName,
			template:          whereaboutsoverlappingIPNADTemplate,
		}
		nad2.createWhereaboutsoverlappingIPNAD(oc)

		exutil.By("Verifying the second NetworkAttachmentDefinition")
		if checkNAD(oc, ns, nad2Name) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nad2Name)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nad2Name)
		}

		exutil.By("Configuring pods for additional network defined in second NAD")
		nad2pod := testMultihomingPod{
			name:       pod2Name,
			namespace:  ns,
			podlabel:   pod2Name,
			nadname:    nad2Name,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		nad2pod.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, "name="+nad2pod.podlabel)).NotTo(o.HaveOccurred())

		ippool1 := "192.168.20.0-29"
		ippool2 := "192.168.20.0-24"
		ipaddress1 := "192.168.20.1"
		ipaddress2 := "192.168.20.2"

		exutil.By("Verifing the correct network_names from ippools")
		ippoolsOutput, ippoolsOutputErr := oc.AsAdmin().Run("get").Args("ippools", "-n", "openshift-multus").Output()
		o.Expect(ippoolsOutputErr).NotTo(o.HaveOccurred())
		o.Expect(ippoolsOutput).To(o.And(o.ContainSubstring(ippool1), o.ContainSubstring(ippool2)))

		exutil.By("Verifing there are no ip overlapping IP addresses from overlappingrangeipreservations")
		overlappingrangeOutput, overlappingrangeOutputErr := oc.AsAdmin().Run("get").Args("overlappingrangeipreservations", "-A", "-n", "openshift-multus").Output()
		o.Expect(overlappingrangeOutputErr).NotTo(o.HaveOccurred())
		o.Expect(overlappingrangeOutput).To(o.And(o.ContainSubstring(ipaddress1), o.ContainSubstring(ipaddress2)))

		exutil.By("Getting IP from pod1's secondary interface")
		pod1List, getPod1Err := exutil.GetAllPodsWithLabel(oc, ns, "name="+nad1pod.podlabel)
		o.Expect(getPod1Err).NotTo(o.HaveOccurred())
		o.Expect(len(pod1List)).NotTo(o.BeEquivalentTo(0))
		pod1Net1IPv4, _ := getPodMultiNetworks(oc, ns, pod1List[0], interfaceName)
		e2e.Logf("The v4 address of pod1's net1 is: %v", pod1Net1IPv4)
		o.Expect(strings.HasPrefix(pod1Net1IPv4, ipaddress1)).Should(o.BeTrue())

		exutil.By("Getting IP from pod2's secondary interface")
		pod2List, getPod2Err := exutil.GetAllPodsWithLabel(oc, ns, "name="+nad2pod.podlabel)
		o.Expect(getPod2Err).NotTo(o.HaveOccurred())
		o.Expect(len(pod2List)).NotTo(o.BeEquivalentTo(0))
		pod2Net1IPv4, _ := getPodMultiNetworks(oc, ns, pod2List[0], interfaceName)
		e2e.Logf("The v4 address of pod2's net1 is: %v", pod2Net1IPv4)
		o.Expect(strings.HasPrefix(pod2Net1IPv4, ipaddress2)).Should(o.BeTrue())

		exutil.By("Deleting the second NetworkAttachmentDefinition and corresponding pods")
		removeResource(oc, true, true, "net-attach-def", nad2Name, "-n", ns)
		removeResource(oc, true, true, "pod", pod2List[0], "-n", ns)

		exutil.By("Deleting the secondary network_name from ippools")
		removeResource(oc, true, true, "ippools", ippool2, "-n", "openshift-multus")

		exutil.By("Reconfiguring second NetworkAttachmentDefinition with setting false for enable_overlapping_ranges")
		defer removeResource(oc, true, true, "net-attach-def", nad2Name, "-n", ns)
		nad2.enableoverlapping = false
		nad2.createWhereaboutsoverlappingIPNAD(oc)

		exutil.By("Reconfiguring pods for additional network defined in second NAD")
		nad2pod.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, "name="+nad2pod.podlabel)).NotTo(o.HaveOccurred())

		exutil.By("Verifing these is only one IP in overlappingrangeipreservations")
		overlappingrangeOutput1, overlappingrangeOutputErr1 := oc.AsAdmin().Run("get").Args("overlappingrangeipreservations", "-A", "-n", "openshift-multus").Output()
		o.Expect(overlappingrangeOutputErr1).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(overlappingrangeOutput1, ipaddress1)).To(o.BeTrue())
		o.Expect(strings.Contains(overlappingrangeOutput1, ipaddress2)).To(o.BeFalse())

		exutil.By("Getting IP from pod2's secondary interface")
		podList2, getPod2Err2 := exutil.GetAllPodsWithLabel(oc, ns, "name="+nad2pod.podlabel)
		o.Expect(getPod2Err2).NotTo(o.HaveOccurred())
		o.Expect(len(podList2)).NotTo(o.BeEquivalentTo(0))
		pod3Net1IPv4, _ := getPodMultiNetworks(oc, ns, podList2[0], interfaceName)
		e2e.Logf("The v4 address of pod2's net1 is: %v", pod3Net1IPv4)
		o.Expect(strings.HasPrefix(pod3Net1IPv4, ipaddress1)).Should(o.BeTrue())
	})
})
