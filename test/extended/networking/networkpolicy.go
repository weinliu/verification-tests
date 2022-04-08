package networking

import (
	"net"
	"path/filepath"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-networkpolicy", exutil.KubeConfigPath())

	// author: zzhao@redhat.com
	g.It("Author:zzhao-Critical-49076-service domain can be resolved when egress type is enabled", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			helloSdnFile        = filepath.Join(buildPruningBaseDir, "hellosdn.yaml")
			egressTypeFile      = filepath.Join(buildPruningBaseDir, "networkpolicy/egress-allow-all.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/ingress-allow-all.yaml")
		)
		g.By("create new namespace")
		oc.SetupProject()

		g.By("create test pods")
		createResourceFromFile(oc, oc.Namespace(), testPodFile)
		createResourceFromFile(oc, oc.Namespace(), helloSdnFile)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		err = waitForPodWithLabelReady(oc, oc.Namespace(), "name=hellosdn")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=hellosdn not ready")

		g.By("create egress and ingress type networkpolicy")
		createResourceFromFile(oc, oc.Namespace(), egressTypeFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-all-egress"))
		createResourceFromFile(oc, oc.Namespace(), ingressTypeFile)
		output, err = oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-all-ingress"))

		g.By("check hellosdn pods can reolsve the dns after apply the networkplicy")
		helloSdnName := getPodName(oc, oc.Namespace(), "name=hellosdn")
		digOutput, err := e2e.RunHostCmd(oc.Namespace(), helloSdnName[0], "dig kubernetes.default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(digOutput).Should(o.ContainSubstring("Got answer"))
		o.Expect(digOutput).ShouldNot(o.ContainSubstring("connection timed out"))

		g.By("check test-pods can reolsve the dns after apply the networkplicy")
		testPodName := getPodName(oc, oc.Namespace(), "name=test-pods")
		digOutput, err = e2e.RunHostCmd(oc.Namespace(), testPodName[0], "dig kubernetes.default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(digOutput).Should(o.ContainSubstring("Got answer"))
		o.Expect(digOutput).ShouldNot(o.ContainSubstring("connection timed out"))

	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-Critical-49186-[Bug 2035336] Networkpolicy egress rule should work for statefulset pods.", func() {
		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking")
			testPodFile          = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			helloStatefulsetFile = filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
			egressTypeFile       = filepath.Join(buildPruningBaseDir, "networkpolicy/allow-egress-red.yaml")
		)
		g.By("1. Create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("2. Create a statefulset pod in first namespace.")
		createResourceFromFile(oc, ns1, helloStatefulsetFile)
		err := waitForPodWithLabelReady(oc, ns1, "app=hello")
		exutil.AssertWaitPollNoErr(err, "this pod with label app=hello not ready")
		helloPodName := getPodName(oc, ns1, "app=hello")

		g.By("3. Create networkpolicy with egress rule in first namespace.")
		createResourceFromFile(oc, ns1, egressTypeFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-egress-to-red"))

		g.By("4. Create second namespace.")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("5. Create test pods in second namespace.")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, oc.Namespace(), "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("6. Add label to first test pod in second namespace.")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "team=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		testPodName := getPodName(oc, ns2, "name=test-pods")
		err = exutil.LabelPod(oc, ns2, testPodName[0], "type=red")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("6. Get IP of the test pods in second namespace.")
		testPodIP1 := getPodIPv4(oc, ns2, testPodName[0])
		testPodIP2 := getPodIPv4(oc, ns2, testPodName[1])

		g.By("7. Check networkpolicy works.")
		output, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
		output, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5  -s "+net.JoinHostPort(testPodIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))

		g.By("8. Delete statefulset pod for a couple of times.")
		for i := 0; i < 5; i++ {
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", helloPodName[0], "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err := waitForPodWithLabelReady(oc, ns1, "app=hello")
			exutil.AssertWaitPollNoErr(err, "this pod with label app=hello not ready")
		}

		g.By("9. Again checking networkpolicy works.")
		output, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
		output, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))

	})

	// author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-49437-[BZ 2037647] Ingress network policy shouldn't be overruled by egress network policy on another pod", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressTypeFile      = filepath.Join(buildPruningBaseDir, "networkpolicy/default-allow-egress.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node.yaml")
		)
		g.By("Create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("create a hello pod in first namespace")
		podns1 := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		podns1.createPingPodNode(oc)
		waitPodReady(oc, podns1.namespace, podns1.name)

		g.By("create default allow egress type networkpolicy in first namespace")
		createResourceFromFile(oc, ns1, egressTypeFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-allow-egress"))

		g.By("Create Second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()
		g.By("create a hello-pod on 2nd namesapce on same node as first namespace")
		pod1_ns2 := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1_ns2.createPingPodNode(oc)
		waitPodReady(oc, pod1_ns2.namespace, pod1_ns2.name)

		g.By("create another hello-pod on 2nd namesapce but on different node")
		pod2_ns2 := pingPodResourceNode{
			name:      "hello-pod-other-node",
			namespace: ns2,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod2_ns2.createPingPodNode(oc)
		waitPodReady(oc, pod2_ns2.namespace, pod2_ns2.name)

		helloPodName_ns2 := getPodName(oc, ns2, "name=hello-pod")

		g.By("create default deny ingress type networkpolicy in 2nd namespace")
		createResourceFromFile(oc, ns2, ingressTypeFile)
		output, err = oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-deny-ingress"))

		g.By("3. Get IP of the test pods in second namespace.")
		hellopodIP1_ns2 := getPodIPv4(oc, ns2, helloPodName_ns2[0])
		hellopodIP2_ns2 := getPodIPv4(oc, ns2, helloPodName_ns2[1])

		g.By("4. Curl both ns2 pods from ns1.")
		output, err = e2e.RunHostCmd(ns1, podns1.name, "curl --connect-timeout 5  -s "+net.JoinHostPort(hellopodIP1_ns2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))
		output, err = e2e.RunHostCmd(ns1, podns1.name, "curl --connect-timeout 5  -s "+net.JoinHostPort(hellopodIP2_ns2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))
	})

})
