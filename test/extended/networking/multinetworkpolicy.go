package networking

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN multinetworkpolicy", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-multinetworkpolicy", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "sriov.openshift-qe.sdn.com") {
			g.Skip("These cases can only be run on networking team's private RDU2 cluster , skip for other environment!!!")
		}
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-Medium-41168-MultiNetworkPolicy ingress allow same podSelector with same namespaceSelector. [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		policyFile := filepath.Join(buildPruningBaseDir, "ingress-allow-same-podSelector-with-same-namespaceSelector.yaml")
		patchSResource := "networks.operator.openshift.io/cluster"

		ns1 := "project41168a"
		ns2 := "project41168b"
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		defer oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
		defer oc.AsAdmin().Run("delete").Args("project", ns2, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()

		g.By("1. Prepare multus multinetwork including 2 ns,5 pods and 2 NADs")
		prepareMultinetworkTest(oc, ns1, ns2, patchInfo)

		g.By("2. Get IPs of the pod1ns1's secondary interface in first namespace.")
		pod1ns1IPv4, pod1ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns1IPv6)

		g.By("3. Get IPs of the pod2ns1's secondary interface in first namespace.")
		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-2")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns1IPv6)

		g.By("4. Get IPs of the pod3ns1's secondary interface in first namespace.")
		pod3ns1IPv4, pod3ns1IPv6 := getPodMultiNetwork(oc, ns1, "red-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod3ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod3ns1IPv6)

		g.By("5. Get IPs of the pod1ns2's secondary interface in second namespace.")
		pod1ns2IPv4, pod1ns2IPv6 := getPodMultiNetwork(oc, ns2, "blue-pod-3")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns2IPv6)

		g.By("6. Get IPs of the pod2ns2's secondary interface in second namespace.")
		pod2ns2IPv4, pod2ns2IPv6 := getPodMultiNetwork(oc, ns2, "red-pod-2")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns2IPv6)

		g.By("7. All curl should pass before applying policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("8. Create Ingress-allow-same-podSelector-with-same-namespaceSelector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()
		output, err := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingress-allow-same-podselector-with-same-namespaceselector"))

		g.By("9. Same curl testing, one curl pass and three curls will fail after applying policy")
		curlPod2PodMultiNetworkFail(oc, ns1, "red-pod-1", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-3", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "red-pod-2", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-2", pod1ns1IPv4, pod1ns1IPv6)

		g.By("10. Delete ingress-allow-same-podselector-with-same-namespaceselector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns1, "multi-networkpolicy", "ingress-allow-same-podselector-with-same-namespaceselector").Execute()
		output1, _ := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(output1).NotTo(o.ContainSubstring("ingress-allow-same-podselector-with-same-namespaceselector"))

		g.By("11. All curl should pass again after deleting policy")
		//curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)

		g.By("12. Delete two namespaces and disable useMultiNetworkPolicy")
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-41169-MultiNetworkPolicy ingress allow diff podSelector with same namespaceSelector. [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		policyFile := filepath.Join(buildPruningBaseDir, "ingress-allow-diff-podSelector-with-same-namespaceSelector.yaml")
		patchSResource := "networks.operator.openshift.io/cluster"

		ns1 := "project41169a"
		ns2 := "project41169b"
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		defer oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
		defer oc.AsAdmin().Run("delete").Args("project", ns2, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()

		g.By("1. Prepare multus multinetwork including 2 ns,5 pods and 2 NADs")
		prepareMultinetworkTest(oc, ns1, ns2, patchInfo)

		g.By("2. Get IPs of the pod1ns1's secondary interface in first namespace.")
		pod1ns1IPv4, pod1ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns1IPv6)

		g.By("3. Get IPs of the pod2ns1's secondary interface in first namespace.")
		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-2")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns1IPv6)

		g.By("4. Get IPs of the pod3ns1's secondary interface in first namespace.")
		pod3ns1IPv4, pod3ns1IPv6 := getPodMultiNetwork(oc, ns1, "red-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod3ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod3ns1IPv6)

		g.By("5. Get IPs of the pod1ns2's secondary interface in second namespace.")
		pod1ns2IPv4, pod1ns2IPv6 := getPodMultiNetwork(oc, ns2, "blue-pod-3")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns2IPv6)

		g.By("6. Get IPs of the pod2ns2's secondary interface in second namespace.")
		pod2ns2IPv4, pod2ns2IPv6 := getPodMultiNetwork(oc, ns2, "red-pod-2")

		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns2IPv6)

		g.By("7. All curl should pass before applying policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("8. Create Ingress-allow-same-podSelector-with-same-namespaceSelector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()
		output, err := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingress-allow-diff-podselector-with-same-namespaceselector"))

		g.By("9. Same curl testing, one curl fail and three curls will pass after applying policy")
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-2", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "red-pod-1", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-3", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "red-pod-2", pod1ns1IPv4, pod1ns1IPv6)

		g.By("10. Delete ingress-allow-diff-podselector-with-same-namespaceselector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns1, "multi-networkpolicy", "ingress-allow-diff-podselector-with-same-namespaceselector").Execute()
		output1, _ := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(output1).NotTo(o.ContainSubstring("ingress-allow-diff-podselector-with-same-namespaceselector"))

		g.By("11. All curl should pass again after deleting policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("12. Delete two namespaces and disable useMultiNetworkPolicy")
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-41171-MultiNetworkPolicy egress allow same podSelector with same namespaceSelector. [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		policyFile := filepath.Join(buildPruningBaseDir, "egress-allow-same-podSelector-with-same-namespaceSelector.yaml")
		patchSResource := "networks.operator.openshift.io/cluster"

		ns1 := "project41171a"
		ns2 := "project41171b"
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		defer oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
		defer oc.AsAdmin().Run("delete").Args("project", ns2, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()

		g.By("1. Prepare multus multinetwork including 2 ns,5 pods and 2 NADs")
		prepareMultinetworkTest(oc, ns1, ns2, patchInfo)

		g.By("2. Get IPs of the pod1ns1's secondary interface in first namespace.")
		pod1ns1IPv4, pod1ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns1IPv6)

		g.By("3. Get IPs of the pod2ns1's secondary interface in first namespace.")
		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-2")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns1IPv6)

		g.By("4. Get IPs of the pod3ns1's secondary interface in first namespace.")
		pod3ns1IPv4, pod3ns1IPv6 := getPodMultiNetwork(oc, ns1, "red-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod3ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod3ns1IPv6)

		g.By("5. Get IPs of the pod1ns2's secondary interface in second namespace.")
		pod1ns2IPv4, pod1ns2IPv6 := getPodMultiNetwork(oc, ns2, "blue-pod-3")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns2IPv6)

		g.By("6. Get IPs of the pod2ns2's secondary interface in second namespace.")
		pod2ns2IPv4, pod2ns2IPv6 := getPodMultiNetwork(oc, ns2, "red-pod-2")

		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns2IPv6)

		g.By("7. All curl should pass before applying policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("8. Create egress-allow-same-podSelector-with-same-namespaceSelector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()
		output, err := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("egress-allow-same-podselector-with-same-namespaceselector"))

		g.By("9. Same curl testing, one curl pass and three curls will fail after applying policy")
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)

		g.By("10. Delete egress-allow-same-podselector-with-same-namespaceselector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns1, "multi-networkpolicy", "egress-allow-same-podselector-with-same-namespaceselector").Execute()
		output1, _ := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(output1).NotTo(o.ContainSubstring("egress-allow-same-podselector-with-same-namespaceselector"))

		g.By("11. All curl should pass again after deleting policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)

		g.By("12. Delete two namespaces and disable useMultiNetworkPolicy")
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-41172-MultiNetworkPolicy egress allow diff podSelector with same namespaceSelector. [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		policyFile := filepath.Join(buildPruningBaseDir, "egress-allow-diff-podSelector-with-same-namespaceSelector.yaml")
		patchSResource := "networks.operator.openshift.io/cluster"

		ns1 := "project41172a"
		ns2 := "project41172b"
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		defer oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
		defer oc.AsAdmin().Run("delete").Args("project", ns2, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()

		g.By("1. Prepare multus multinetwork including 2 ns,5 pods and 2 NADs")
		prepareMultinetworkTest(oc, ns1, ns2, patchInfo)

		g.By("2. Get IPs of the pod1ns1's secondary interface in first namespace.")
		pod1ns1IPv4, pod1ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns1IPv6)

		g.By("3. Get IPs of the pod2ns1's secondary interface in first namespace.")
		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-2")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns1IPv6)

		g.By("4. Get IPs of the pod3ns1's secondary interface in first namespace.")
		pod3ns1IPv4, pod3ns1IPv6 := getPodMultiNetwork(oc, ns1, "red-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod3ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod3ns1IPv6)

		g.By("5. Get IPs of the pod1ns2's secondary interface in second namespace.")
		pod1ns2IPv4, pod1ns2IPv6 := getPodMultiNetwork(oc, ns2, "blue-pod-3")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns2IPv6)

		g.By("6. Get IPs of the pod2ns2's secondary interface in second namespace.")
		pod2ns2IPv4, pod2ns2IPv6 := getPodMultiNetwork(oc, ns2, "red-pod-2")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod2ns2IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod2ns2IPv6)

		g.By("7. All curl should pass before applying policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("8. Create egress-allow-diff-podSelector-with-same-namespaceSelector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()
		output, err := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("egress-allow-diff-podselector-with-same-namespaceselector"))

		g.By("9. Same curl testing, one curl pass and three curls will fail after applying policy")
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("10. Delete egress-allow-diff-podselector-with-same-namespaceselector policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns1, "multi-networkpolicy", "egress-allow-diff-podselector-with-same-namespaceselector").Execute()
		output1, _ := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(output1).NotTo(o.ContainSubstring("egress-allow-diff-podselector-with-same-namespaceselector"))

		g.By("11. All curl should pass again after deleting policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("12. Delete two namespaces and disable useMultiNetworkPolicy")
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-41170-MultiNetworkPolicy ingress ipblock. [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		patchSResource := "networks.operator.openshift.io/cluster"
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "MultiNetworkPolicy-pod-template.yaml")
		netAttachDefFile := filepath.Join(buildPruningBaseDir, "ipblock-NAD.yaml")
		policyFile := filepath.Join(buildPruningBaseDir, "ingress-ipBlock.yaml")
		ns1 := "project41170a"

		g.By("1. Enable MacvlanNetworkpolicy in the cluster")
		patchResourceAsAdmin(oc, patchSResource, patchInfo)
		defer oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()

		g.By("2. Create a namespace")
		nserr1 := oc.Run("new-project").Args(ns1).Execute()
		o.Expect(nserr1).NotTo(o.HaveOccurred())

		g.By("3. Create MultiNetworkPolicy-NAD in ns1")
		err1 := oc.AsAdmin().Run("create").Args("-f", netAttachDefFile, "-n", ns1).Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())
		output, err2 := oc.Run("get").Args("net-attach-def", "-n", ns1).Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ipblock-net"))

		g.By("4. Create six pods for ip range policy testing")
		pod1ns1 := testPodMultinetwork{
			name:      "blue-pod-1",
			namespace: ns1,
			nodename:  "worker-0",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod1ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		pod2ns1 := testPodMultinetwork{
			name:      "blue-pod-2",
			namespace: ns1,
			nodename:  "worker-0",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod2ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

		pod3ns1 := testPodMultinetwork{
			name:      "blue-pod-3",
			namespace: ns1,
			nodename:  "worker-0",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod3ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod3ns1.namespace, pod3ns1.name)

		pod4ns1 := testPodMultinetwork{
			name:      "blue-pod-4",
			namespace: ns1,
			nodename:  "worker-1",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod4ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod4ns1.namespace, pod4ns1.name)

		pod5ns1 := testPodMultinetwork{
			name:      "blue-pod-5",
			namespace: ns1,
			nodename:  "worker-1",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod5ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod5ns1.namespace, pod5ns1.name)

		pod6ns1 := testPodMultinetwork{
			name:      "blue-pod-6",
			namespace: ns1,
			nodename:  "worker-1",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod6ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod6ns1.namespace, pod6ns1.name)

		g.By("5. Get IPs from all six pod's secondary interfaces")
		pod1ns1IPv4, pod1ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns1IPv6)

		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-2")
		e2e.Logf("The v4 address of pod2ns1is: %v", pod2ns1IPv4)
		e2e.Logf("The v6 address of pod2ns1is: %v", pod2ns1IPv6)

		pod3ns1IPv4, pod3ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-3")
		e2e.Logf("The v4 address of pod3ns1is: %v", pod3ns1IPv4)
		e2e.Logf("The v6 address of pod3ns1is: %v", pod3ns1IPv6)

		pod4ns1IPv4, pod4ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-4")
		e2e.Logf("The v4 address of pod4ns1is: %v", pod4ns1IPv4)
		e2e.Logf("The v6 address of pod4ns1is: %v", pod4ns1IPv6)

		pod5ns1IPv4, pod5ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-5")
		e2e.Logf("The v4 address of pod5ns1is: %v", pod5ns1IPv4)
		e2e.Logf("The v6 address of pod5ns1is: %v", pod5ns1IPv6)

		pod6ns1IPv4, pod6ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-6")
		e2e.Logf("The v4 address of pod6ns1is: %v", pod6ns1IPv4)
		e2e.Logf("The v6 address of pod6ns1is: %v", pod6ns1IPv6)

		g.By("6. All curl should pass before applying policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod6ns1IPv4, pod6ns1IPv6)

		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod6ns1IPv4, pod6ns1IPv6)

		g.By("7. Create ingress-ipBlock policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()
		output, err3 := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("ingress-ipblock"))

		g.By("8. curl should fail for ip range 192.168.0.4-192.168.0.6 after applying policy")
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-6", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-6", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-6", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-6", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-6", pod5ns1IPv4, pod5ns1IPv6)

		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-4", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-5", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-6", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-2", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-3", pod1ns1IPv4, pod1ns1IPv6)

		g.By("9. Delete ingress-ipBlock policy in ns1")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns1, "multi-networkpolicy", "ingress-ipblock").Execute()
		output1, _ := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(output1).NotTo(o.ContainSubstring("ingress-ipblock"))

		g.By("10. All curl should pass again after deleting policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod6ns1IPv4, pod6ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)

		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod6ns1IPv4, pod6ns1IPv6)

		g.By("11. Delete two namespaces and disable useMultiNetworkPolicy")
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-41173-MultiNetworkPolicy egress ipblock. [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		patchSResource := "networks.operator.openshift.io/cluster"
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "MultiNetworkPolicy-pod-template.yaml")
		netAttachDefFile := filepath.Join(buildPruningBaseDir, "ipblock-NAD.yaml")
		policyFile := filepath.Join(buildPruningBaseDir, "egress-ipBlock.yaml")

		g.By("1. Enable MacvlanNetworkpolicy in the cluster")
		patchResourceAsAdmin(oc, patchSResource, patchInfo)
		defer func() {
			policyErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()
			o.Expect(policyErr).NotTo(o.HaveOccurred())
		}()

		g.By("2. Create a namespace")
		originalCtx, contextErr := oc.Run("config").Args("current-context").Output()
		o.Expect(contextErr).NotTo(o.HaveOccurred())
		defer func() {
			rollbackCtxErr := oc.Run("config").Args("set", "current-context", originalCtx).Execute()
			o.Expect(rollbackCtxErr).NotTo(o.HaveOccurred())
		}()
		ns1 := "project41173a"
		nsErr := oc.Run("new-project").Args(ns1).Execute()
		o.Expect(nsErr).NotTo(o.HaveOccurred())
		defer func() {
			proErr := oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
			o.Expect(proErr).NotTo(o.HaveOccurred())
		}()

		g.By("3. Create MultiNetworkPolicy-NAD in ns1")
		policyErr := oc.AsAdmin().Run("create").Args("-f", netAttachDefFile, "-n", ns1).Execute()
		o.Expect(policyErr).NotTo(o.HaveOccurred())
		nadOutput, nadErr := oc.Run("get").Args("net-attach-def", "-n", ns1).Output()
		o.Expect(nadErr).NotTo(o.HaveOccurred())
		o.Expect(nadOutput).To(o.ContainSubstring("ipblock-net"))

		g.By("4. Create six pods for egress ip range policy testing")
		pod1ns1 := testPodMultinetwork{
			name:      "blue-pod-1",
			namespace: ns1,
			nodename:  "worker-0",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod1ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		pod2ns1 := testPodMultinetwork{
			name:      "blue-pod-2",
			namespace: ns1,
			nodename:  "worker-0",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod2ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

		pod3ns1 := testPodMultinetwork{
			name:      "blue-pod-3",
			namespace: ns1,
			nodename:  "worker-0",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod3ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod3ns1.namespace, pod3ns1.name)

		pod4ns1 := testPodMultinetwork{
			name:      "blue-pod-4",
			namespace: ns1,
			nodename:  "worker-1",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod4ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod4ns1.namespace, pod4ns1.name)

		pod5ns1 := testPodMultinetwork{
			name:      "blue-pod-5",
			namespace: ns1,
			nodename:  "worker-1",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod5ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod5ns1.namespace, pod5ns1.name)

		pod6ns1 := testPodMultinetwork{
			name:      "blue-pod-6",
			namespace: ns1,
			nodename:  "worker-1",
			nadname:   "ipblock-net",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod6ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod6ns1.namespace, pod6ns1.name)

		g.By("5. Get IPs from all six pod's secondary interfaces")
		pod1ns1IPv4, pod1ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-1")
		e2e.Logf("The v4 address of pod1ns1is: %v", pod1ns1IPv4)
		e2e.Logf("The v6 address of pod1ns1is: %v", pod1ns1IPv6)

		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-2")
		e2e.Logf("The v4 address of pod2ns1is: %v", pod2ns1IPv4)
		e2e.Logf("The v6 address of pod2ns1is: %v", pod2ns1IPv6)

		pod3ns1IPv4, pod3ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-3")
		e2e.Logf("The v4 address of pod3ns1is: %v", pod3ns1IPv4)
		e2e.Logf("The v6 address of pod3ns1is: %v", pod3ns1IPv6)

		pod4ns1IPv4, pod4ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-4")
		e2e.Logf("The v4 address of pod4ns1is: %v", pod4ns1IPv4)
		e2e.Logf("The v6 address of pod4ns1is: %v", pod4ns1IPv6)

		pod5ns1IPv4, pod5ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-5")
		e2e.Logf("The v4 address of pod5ns1is: %v", pod5ns1IPv4)
		e2e.Logf("The v6 address of pod5ns1is: %v", pod5ns1IPv6)

		pod6ns1IPv4, pod6ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-6")
		e2e.Logf("The v4 address of pod6ns1is: %v", pod6ns1IPv4)
		e2e.Logf("The v6 address of pod6ns1is: %v", pod6ns1IPv6)

		g.By("6. All curl should pass before applying policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod6ns1IPv4, pod6ns1IPv6)

		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod6ns1IPv4, pod6ns1IPv6)

		g.By("7. Create egress-ipBlock policy in ns1")
		policyCreateErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()
		o.Expect(policyCreateErr).NotTo(o.HaveOccurred())
		policyCreOutput, policyCreErr := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(policyCreErr).NotTo(o.HaveOccurred())
		o.Expect(policyCreOutput).To(o.ContainSubstring("egress-ipblock"))

		g.By("8. curl should fail for ip range 192.168.0.4-192.168.0.6 after applying policy")
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod6ns1IPv4, pod6ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)

		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-2", pod6ns1IPv4, pod6ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-2", pod6ns1IPv4, pod6ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-3", pod6ns1IPv4, pod6ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-4", pod6ns1IPv4, pod6ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-5", pod6ns1IPv4, pod6ns1IPv6)

		g.By("9. Delete egress-ipBlock policy in ns1")
		policyDeleteErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns1, "multi-networkpolicy", "egress-ipblock").Execute()
		o.Expect(policyDeleteErr).NotTo(o.HaveOccurred())
		policyDelOutput, policyDelErr := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(policyDelErr).NotTo(o.HaveOccurred())
		o.Expect(policyDelOutput).NotTo(o.ContainSubstring("egress-ipblock"))

		g.By("10. All curl should pass again after deleting policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod6ns1IPv4, pod6ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)

		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod1ns1IPv4, pod1ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod4ns1IPv4, pod4ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod5ns1IPv4, pod5ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-6", pod6ns1IPv4, pod6ns1IPv6)
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-41607-Multinetworkpolicy filter-with-tcpport [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		patchSResource := "networks.operator.openshift.io/cluster"
		tcpportPod := filepath.Join(buildPruningBaseDir, "tcpport-pod.yaml")
		netAttachDefFile := filepath.Join(buildPruningBaseDir, "MultiNetworkPolicy-NAD1.yaml")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "MultiNetworkPolicy-pod-template.yaml")
		policyFile := filepath.Join(buildPruningBaseDir, "policy-tcpport.yaml")

		g.By("1. Enable MacvlanNetworkpolicy in the cluster")
		patchResourceAsAdmin(oc, patchSResource, patchInfo)
		defer func() {
			policyErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()
			o.Expect(policyErr).NotTo(o.HaveOccurred())
		}()

		g.By("2. Create a namespace")
		originalCtx, contextErr := oc.Run("config").Args("current-context").Output()
		o.Expect(contextErr).NotTo(o.HaveOccurred())
		defer func() {
			rollbackCtxErr := oc.Run("config").Args("set", "current-context", originalCtx).Execute()
			o.Expect(rollbackCtxErr).NotTo(o.HaveOccurred())
		}()
		ns := "project41607"
		nsErr := oc.Run("new-project").Args(ns).Execute()
		o.Expect(nsErr).NotTo(o.HaveOccurred())
		defer func() {
			proErr := oc.AsAdmin().Run("delete").Args("project", ns, "--ignore-not-found").Execute()
			o.Expect(proErr).NotTo(o.HaveOccurred())
		}()

		g.By("3. Create MultiNetworkPolicy-NAD in ns1")
		policyErr := oc.AsAdmin().Run("create").Args("-f", netAttachDefFile, "-n", ns).Execute()
		o.Expect(policyErr).NotTo(o.HaveOccurred())
		nadOutput, nadErr := oc.Run("get").Args("net-attach-def", "-n", ns).Output()
		o.Expect(nadErr).NotTo(o.HaveOccurred())
		o.Expect(nadOutput).To(o.ContainSubstring("macvlan-nad1"))

		g.By("4. Create a tcpport pods for ingress tcp port testing")
		createResourceFromFile(oc, ns, tcpportPod)
		podErr := waitForPodWithLabelReady(oc, ns, "name=tcp-port-pod")
		exutil.AssertWaitPollNoErr(podErr, "tcpportPod is not running")
		podIPv4, _ := getPodMultiNetwork(oc, ns, "tcp-port-pod")

		g.By("5. Create a test pods for ingress tcp port testing")
		pod1ns1 := testPodMultinetwork{
			name:      "blue-pod-1",
			namespace: ns,
			nodename:  "worker-1",
			nadname:   "macvlan-nad1",
			labelname: "blue-openshift",
			template:  pingPodTemplate,
		}
		pod1ns1.createTestPodMultinetwork(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("6. curl should pass before applying policy")
		_, curl1Err := e2eoutput.RunHostCmd(ns, "blue-pod-1", "curl --connect-timeout 5 -s "+net.JoinHostPort(podIPv4, "8888"))
		o.Expect(curl1Err).NotTo(o.HaveOccurred())

		g.By("7. Create tcpport policy in ns1")
		policyCreateErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns).Execute()
		o.Expect(policyCreateErr).NotTo(o.HaveOccurred())
		policyCreOutput, policyCreErr := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns).Output()
		o.Expect(policyCreErr).NotTo(o.HaveOccurred())
		o.Expect(policyCreOutput).To(o.ContainSubstring("tcp-port"))

		g.By("8. One curl should fail before applying policy")
		_, curl2Err := e2eoutput.RunHostCmd(ns, "blue-pod-1", "curl --connect-timeout 5 -s "+net.JoinHostPort(podIPv4, "8888"))
		o.Expect(curl2Err).To(o.HaveOccurred())

		g.By("9. Delete tcp-port policy in ns1")
		policyDeleteErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "multi-networkpolicy", "tcp-port").Execute()
		o.Expect(policyDeleteErr).NotTo(o.HaveOccurred())
		policyDelOutput, policyDelErr := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns).Output()
		o.Expect(policyDelErr).NotTo(o.HaveOccurred())
		o.Expect(policyDelOutput).NotTo(o.ContainSubstring("tcp-port"))

		g.By("10. curl should pass after deleting policy")
		_, curl3Err := e2eoutput.RunHostCmd(ns, "blue-pod-1", "curl --connect-timeout 5 -s "+net.JoinHostPort(podIPv4, "8888"))
		o.Expect(curl3Err).NotTo(o.HaveOccurred())
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-55818-Rules are not removed after disabling multinetworkpolicy. [Serial]", func() {
		exutil.SkipBaselineCaps(oc, "None")
		//https://issues.redhat.com/browse/OCPBUGS-977: Rules are not removed after disabling multinetworkpolicy
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
		policyFile := filepath.Join(buildPruningBaseDir, "creat-ten-rules.yaml")
		patchSResource := "networks.operator.openshift.io/cluster"

		ns1 := "project41171a"
		ns2 := "project41171b"
		patchInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		defer oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
		defer oc.AsAdmin().Run("delete").Args("project", ns2, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/useMultiNetworkPolicy"}]`, "--type=json").Execute()

		g.By("1. Prepare multus multinetwork including 2 ns,5 pods and 2 NADs")
		prepareMultinetworkTest(oc, ns1, ns2, patchInfo)

		g.By("2. Get IPs of the pod2ns1's secondary interface in first namespace.")
		pod2ns1IPv4, pod2ns1IPv6 := getPodMultiNetwork(oc, ns1, "blue-pod-2")

		g.By("3. Get IPs of the pod3ns1's secondary interface in first namespace.")
		pod3ns1IPv4, pod3ns1IPv6 := getPodMultiNetwork(oc, ns1, "red-pod-1")

		g.By("4. Get IPs of the pod1ns2's secondary interface in second namespace.")
		pod1ns2IPv4, pod1ns2IPv6 := getPodMultiNetwork(oc, ns2, "blue-pod-3")

		g.By("5. Get IPs of the pod2ns2's secondary interface in second namespace.")
		pod2ns2IPv4, pod2ns2IPv6 := getPodMultiNetwork(oc, ns2, "red-pod-2")

		g.By("6. All curl should pass before applying policy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)

		g.By("7. Create egress-allow-same-podSelector-with-same-namespaceSelector policy in ns1")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		policyList := []string{
			"egress-allow-same-podselector-with-same-namespaceselector1",
			"egress-allow-same-podselector-with-same-namespaceselector2",
			"egress-allow-same-podselector-with-same-namespaceselector3",
			"egress-allow-same-podselector-with-same-namespaceselector4",
			"egress-allow-same-podselector-with-same-namespaceselector5",
			"egress-allow-same-podselector-with-same-namespaceselector6",
			"egress-allow-same-podselector-with-same-namespaceselector7",
			"egress-allow-same-podselector-with-same-namespaceselector8",
			"egress-allow-same-podselector-with-same-namespaceselector9",
			"egress-allow-same-podselector-with-same-namespaceselector10",
		}
		for _, policyRule := range policyList {
			e2e.Logf("The policy rule is: %s", policyRule)
			o.Expect(output).To(o.ContainSubstring(policyRule))
		}

		g.By("8. Same curl testing, one curl pass and three curls will fail after applying policy")
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkFail(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)

		g.By("9. Disable MacvlanNetworkpolicy in the cluster")
		patchDisableInfo := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":false}}")
		patchResourceAsAdmin(oc, patchSResource, patchDisableInfo)

		g.By("10. All curl should pass again after disabling MacvlanNetworkpolicy")
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod3ns1IPv4, pod3ns1IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod1ns2IPv4, pod1ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns2IPv4, pod2ns2IPv6)
		curlPod2PodMultiNetworkPass(oc, ns1, "blue-pod-1", pod2ns1IPv4, pod2ns1IPv6)

		g.By("11. Delete two namespaces and disable useMultiNetworkPolicy")
	})
})
