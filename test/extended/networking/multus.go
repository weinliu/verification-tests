package networking

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-multus", exutil.KubeConfigPath())

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

	// author: weliang@redhat.com
	g.It("Author:weliang-High-59440-Whereabouts ip-reconciler should be opt-in and not required. [Disruptive]", func() {
		//Bug: https://bugzilla.redhat.com/show_bug.cgi?id=2101918
		var (
			patchSResource = "networks.operator.openshift.io/cluster"
			patchInfo      = fmt.Sprintf(`{"spec":{ "additionalNetworks": [{"name": "whereabouts-shim", "namespace": "default","rawCNIConfig":"{\"cniVersion\":\"0.3.0\",\"type\":\"bridge\",\"name\":\"cnitest0\",\"ipam\": {\"type\":\"whereabouts\",\"subnet\":\"192.0.2.0/24\"}}","type":"Raw"}]}}`)
			stringInfo1    = "No resources found in openshift-multus namespace"
			stringInfo2    = "ip-reconciler"
		)

		g.By("Check the cronjobs in the openshift-multus namespace")
		cronjobLog1 := getMultusCronJob(oc)
		o.Expect(cronjobLog1).To(o.ContainSubstring(stringInfo1))

		g.By("Add additionalNetworks through network operator")
		patchResourceAsAdmin(oc, patchSResource, patchInfo)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()

		g.By("Check the cronjobs in the openshift-multus namespace")
		o.Eventually(func() string {
			cronjobLog2 := getMultusCronJob(oc)
			return cronjobLog2
		}, "90s", "10s").Should(o.ContainSubstring(stringInfo2), fmt.Sprintf("Failed to get correct multus cronjobs"))

		g.By("Delete additionalNetworks through network operator")
		oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()

		g.By("Check the cronjobs in the openshift-multus namespace")
		o.Eventually(func() string {
			cronjobLog3 := getMultusCronJob(oc)
			return cronjobLog3
		}, "90s", "10s").Should(o.ContainSubstring(stringInfo1), fmt.Sprintf("Failed to get correct multus cronjobs"))
	})

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
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
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
})
