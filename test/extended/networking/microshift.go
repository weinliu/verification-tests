package networking

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("SDN-microshift")

	// author: anusaxen@redhat.com
	g.It("MicroShiftOnly-Author:anusaxen-Critical-60331-mixed ingress and egress policies can work well", func() {
		var (
			caseID              = "60331"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			e2eTestNamespace1   = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			e2eTestNamespace2   = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			helloSdnFile        = filepath.Join(buildPruningBaseDir, "hellosdn.yaml")
			egressTypeFile      = filepath.Join(buildPruningBaseDir, "networkpolicy/egress_49696.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/ingress_49696.yaml")
		)
		g.By("Create 1st namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace1)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace1)

		g.By("create test pods")
		createResourceFromFile(oc, e2eTestNamespace1, testPodFile)
		createResourceFromFile(oc, e2eTestNamespace1, helloSdnFile)
		exutil.AssertWaitPollNoErr(waitForPodWithLabelReady(oc, e2eTestNamespace1, "name=test-pods"), fmt.Sprintf("this pod with label name=test-pods in ns/%s not ready", e2eTestNamespace1))
		exutil.AssertWaitPollNoErr(waitForPodWithLabelReady(oc, e2eTestNamespace1, "name=hellosdn"), fmt.Sprintf("this pod with label name=hellosdn in ns/%s not ready", e2eTestNamespace1))
		hellosdnPodNameNs1 := getPodName(oc, e2eTestNamespace1, "name=hellosdn")

		g.By("create egress type networkpolicy in ns1")
		createResourceFromFile(oc, e2eTestNamespace1, egressTypeFile)

		g.By("create ingress type networkpolicy in ns1")
		createResourceFromFile(oc, e2eTestNamespace1, ingressTypeFile)

		g.By("#. Create 2nd namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace2)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace2)

		g.By("create test pods in second namespace")
		createResourceFromFile(oc, e2eTestNamespace2, helloSdnFile)
		exutil.AssertWaitPollNoErr(waitForPodWithLabelReady(oc, e2eTestNamespace2, "name=hellosdn"), fmt.Sprintf("this pod with label name=hellosdn in ns/%s not ready", e2eTestNamespace2))

		g.By("Get IP of the test pods in second namespace.")
		hellosdnPodNameNs2 := getPodName(oc, e2eTestNamespace2, "name=hellosdn")
		hellosdnPodIP1Ns2 := getPodIPv4(oc, e2eTestNamespace2, hellosdnPodNameNs2[0])

		g.By("curl from ns1 hellosdn pod to ns2 pod")
		_, err := e2eoutput.RunHostCmd(e2eTestNamespace1, hellosdnPodNameNs1[0], "curl --connect-timeout 5  -s "+net.JoinHostPort(hellosdnPodIP1Ns2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))

	})

	// author: anusaxen@redhat.com
	g.It("MicroShiftOnly-Author:anusaxen-High-60332-Network Policies should work with OVNKubernetes when traffic hairpins back to the same source through a service", func() {
		var (
			caseID           = "60332"
			baseDir          = exutil.FixturePath("testdata", "networking")
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			allowfromsameNS  = filepath.Join(baseDir, "networkpolicy/allow-from-same-namespace.yaml")
		)

		g.By("Create a namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		pod_pmtrs := map[string]string{
			"$podname":   "hello-pod1",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod",
		}

		g.By("create 1st hello pod in namespace")
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod1")

		pod_pmtrs = map[string]string{
			"$podname":   "hello-pod2",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod",
		}
		g.By("create 2nd hello pod in same namespace")
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod2")

		//ipFamilyPolicy and externalTrafficPolicy are left blank which would get default values
		svc_pmtrs := map[string]string{
			"$servicename":           "test-service",
			"$namespace":             e2eTestNamespace,
			"$label":                 "test-service",
			"$internalTrafficPolicy": "Cluster",
			"$externalTrafficPolicy": "",
			"$ipFamilyPolicy":        "",
			"$selector":              "hello-pod",
			"$serviceType":           "ClusterIP",
		}
		createServiceforUshift(oc, svc_pmtrs)

		g.By("create allow-from-same-namespace ingress networkpolicy in ns")
		createResourceFromFile(oc, e2eTestNamespace, allowfromsameNS)

		g.By("Get Pod IPs")
		helloPod1IP := getPodIPv4(oc, e2eTestNamespace, "hello-pod1")
		helloPod2IP := getPodIPv4(oc, e2eTestNamespace, "hello-pod2")

		g.By("Get svc IP")
		svcIP := getSvcIPv4(oc, e2eTestNamespace, "test-service")

		g.By("curl hello-pod1 to hello-pod2")
		output, err := e2eoutput.RunHostCmd(e2eTestNamespace, "hello-pod1", "curl --connect-timeout 5 -s "+net.JoinHostPort(helloPod1IP, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

		g.By("curl hello-pod2 to hello-pod1")
		output, err = e2eoutput.RunHostCmd(e2eTestNamespace, "hello-pod2", "curl --connect-timeout 5 -s "+net.JoinHostPort(helloPod2IP, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

		for i := 0; i < 5; i++ {

			g.By("curl hello-pod1 to service:port")
			output, err = e2eoutput.RunHostCmd(e2eTestNamespace, "hello-pod1", "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP, "27017"))
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

			g.By("curl hello-pod2 to service:port")
			output, err = e2eoutput.RunHostCmd(e2eTestNamespace, "hello-pod2", "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP, "27017"))
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
		}
	})

	// author: anusaxen@redhat.com
	g.It("MicroShiftOnly-Author:anusaxen-High-60426-podSelector allow-to and allow-from can work together", func() {
		var (
			caseID            = "60426"
			baseDir           = exutil.FixturePath("testdata", "networking")
			e2eTestNamespace1 = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			e2eTestNamespace2 = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			ingressTypeFile   = filepath.Join(baseDir, "networkpolicy/default-deny-ingress.yaml")
			allowfromRed      = filepath.Join(baseDir, "microshift/np-allow-from-red.yaml")
			allowtoBlue       = filepath.Join(baseDir, "microshift/np-allow-to-blue.yaml")
		)

		g.By("Create a namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace1)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace1)

		g.By("create 4 test pods in e2enamespace1")
		for i := 0; i < 4; i++ {
			pod_pmtrs := map[string]string{
				"$podname":   "test-pod" + strconv.Itoa(i),
				"$namespace": e2eTestNamespace1,
				"$label":     "test-pod" + strconv.Itoa(i),
			}
			createPingPodforUshift(oc, pod_pmtrs)
			waitPodReady(oc, e2eTestNamespace1, "test-pod"+strconv.Itoa(i))
		}

		var testPodNS1 [4]string
		var testPodIPNS1 [4]string
		g.By("Get IP of the test pods in e2eTestNamespace1 namespace.")
		for i := 0; i < 4; i++ {
			testPodNS1[i] = strings.Join(getPodName(oc, e2eTestNamespace1, "name=test-pod"+strconv.Itoa(i)), "")
			testPodIPNS1[i] = getPodIPv4(oc, e2eTestNamespace1, testPodNS1[i])
		}

		// label pod0 and pod1 with type=red and type=blue respectively in e2eTestNamespace1
		err := exutil.LabelPod(oc, e2eTestNamespace1, testPodNS1[0], "type=red")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.LabelPod(oc, e2eTestNamespace1, testPodNS1[1], "type=blue")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create 2nd namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace2)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace2)

		g.By("create 2 test pods in e2enamespace1")
		for i := 0; i < 2; i++ {
			pod_pmtrs := map[string]string{
				"$podname":   "test-pod" + strconv.Itoa(i),
				"$namespace": e2eTestNamespace2,
				"$label":     "test-pod" + strconv.Itoa(i),
			}
			createPingPodforUshift(oc, pod_pmtrs)
			waitPodReady(oc, e2eTestNamespace2, "test-pod"+strconv.Itoa(i))
		}
		var testPodNS2 [2]string
		var testPodIPNS2 [2]string
		g.By("Get IP of the test pods in e2eTestNamespace2 namespace.")
		for i := 0; i < 2; i++ {
			testPodNS2[i] = strings.Join(getPodName(oc, e2eTestNamespace2, "name=test-pod"+strconv.Itoa(i)), "")
			testPodIPNS2[i] = getPodIPv4(oc, e2eTestNamespace2, testPodNS2[i])
		}

		// label pod0 with type=red in e2eTestNamespace2
		err = exutil.LabelPod(oc, e2eTestNamespace2, testPodNS2[0], "type=red")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create default deny ingress type networkpolicy in 1st namespace")
		createResourceFromFile(oc, e2eTestNamespace1, ingressTypeFile)

		g.By("create allow-from-red and allow-from-blue type networkpolicy in 1st namespace")
		createResourceFromFile(oc, e2eTestNamespace1, allowfromRed)
		createResourceFromFile(oc, e2eTestNamespace1, allowtoBlue)

		g.By("Try to access the pod in e2eTestNamespace1 from each pod")
		g.By("curl testPodNS10 to testPodNS13")
		output, err := e2eoutput.RunHostCmd(e2eTestNamespace1, testPodNS1[0], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIPNS1[3], "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

		g.By("curl testPodNS12 to testPodNS11")
		output, err = e2eoutput.RunHostCmd(e2eTestNamespace1, testPodNS1[2], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIPNS1[1], "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

		g.By("curl testPodNS12 to testPodNS13")
		output, err = e2eoutput.RunHostCmd(e2eTestNamespace1, testPodNS1[2], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIPNS1[3], "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).ShouldNot(o.ContainSubstring("Hello OpenShift"))

		g.By("Try to access the pod from e2eTestNamespace2 now")
		g.By("curl testPodNS20 to testPodNS13")
		output, err = e2eoutput.RunHostCmd(e2eTestNamespace2, testPodNS2[1], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIPNS1[3], "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).ShouldNot(o.ContainSubstring("Hello OpenShift"))

		g.By("curl testPodNS21 to testPodNS11")
		output, err = e2eoutput.RunHostCmd(e2eTestNamespace2, testPodNS2[1], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIPNS1[1], "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

		g.By("curl testPodNS21 to testPodNS13")
		output, err = e2eoutput.RunHostCmd(e2eTestNamespace2, testPodNS2[1], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIPNS1[3], "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).ShouldNot(o.ContainSubstring("Hello OpenShift"))

	})

	// author: qiowang@redhat.com
	g.It("MicroShiftOnly-Author:qiowang-High-60290-Idling/Unidling services", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testSvcFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			namespace           = "test-60290"
		)

		g.By("create namespace")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(namespace)
		oc.CreateSpecifiedNamespaceAsAdmin(namespace)

		g.By("create test pods with rc and service")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", testSvcFile, "-n", namespace).Execute()
		createResourceFromFile(oc, namespace, testSvcFile)
		waitForPodWithLabelReady(oc, namespace, "name=test-pods")
		svcOutput, svcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace).Output()
		o.Expect(svcErr).NotTo(o.HaveOccurred())
		o.Expect(svcOutput).To(o.ContainSubstring("test-service"))

		g.By("idle test-service")
		idleOutput, idleErr := oc.AsAdmin().WithoutNamespace().Run("idle").Args("-n", namespace, "test-service").Output()
		o.Expect(idleErr).NotTo(o.HaveOccurred())
		o.Expect(idleOutput).To(o.ContainSubstring("The service \"%v/test-service\" has been marked as idled", namespace))

		g.By("check test pods are terminated")
		getPodOutput := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			output, getPodErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace).Output()
			o.Expect(getPodErr).NotTo(o.HaveOccurred())
			e2e.Logf("pods status: %s", output)
			if strings.Contains(output, "No resources found") {
				return true, nil
			}
			e2e.Logf("pods are not terminated, try again")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(getPodOutput, fmt.Sprintf("Fail to terminate pods:%s", getPodOutput))

		// for micorshift: unidling is not supported now, and manual re-scaling the replicas is required
		// https://issues.redhat.com/browse/USHIFT-503
		g.By("re-scaling the replicas")
		_, rescaleErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("replicationcontroller/test-rc", "-n", namespace, "-p", "{\"spec\":{\"replicas\":2}}", "--type=merge").Output()
		o.Expect(rescaleErr).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, namespace, "name=test-pods")
	})

	// author: weliang@redhat.com
	g.It("MicroShiftOnly-Author:weliang-Medium-60550-Pod should be accessible via node ip and host port", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "hostport-pod.yaml")
			ns                  = "test-ocp-60550"
		)

		g.By("create a test namespace")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(ns)
		oc.CreateSpecifiedNamespaceAsAdmin(ns)
		defer exutil.RecoverNamespaceRestricted(oc, ns)
		exutil.SetNamespacePrivileged(oc, ns)

		g.By("create a test pod")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", testPodFile, "-n", ns).Execute()
		createResourceFromFile(oc, ns, testPodFile)
		waitForPodWithLabelReady(oc, ns, "name=hostport-pod")

		g.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())

		g.By("Get the IP address from the worker node")
		nodeIP := getNodeIPv4(oc, ns, nodeList.Items[0].Name)

		g.By("Verify the pod should be accessible via nodeIP:hostport")
		ipv4URL := net.JoinHostPort(nodeIP, "9500")
		curlOutput, err := exutil.DebugNode(oc, nodeList.Items[0].Name, "curl", ipv4URL, "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(curlOutput).To(o.ContainSubstring("Hello Hostport Pod"))
	})

	// author: anusaxen@redhat.com
	g.It("MicroShiftOnly-Author:anusaxen-High-60746-Check nodeport service works well on Microshift", func() {
		var (
			caseID           = "60746"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
		)

		g.By("Create a namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		pod_pmtrs := map[string]string{
			"$podname":   "hello-pod",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod",
		}

		g.By("creating hello pod in namespace")
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod")

		//ipFamilyPolicy, externalTrafficPolicy and internalTrafficPolicy are left blank which would get default values, ETP will be Cluster type in that case
		svc_pmtrs := map[string]string{
			"$servicename":           "test-service-etp-cluster",
			"$namespace":             e2eTestNamespace,
			"$label":                 "test-service",
			"$internalTrafficPolicy": "",
			"$externalTrafficPolicy": "",
			"$ipFamilyPolicy":        "",
			"$selector":              "hello-pod",
			"$serviceType":           "NodePort",
		}
		createServiceforUshift(oc, svc_pmtrs)

		g.By("Get service NodePort and NodeIP value")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "test-service-etp-cluster", "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeName, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, e2eTestNamespace, nodeName)
		svcURL := net.JoinHostPort(nodeIP, nodePort)
		g.By("curl NodePort Service")
		_, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete test-service-etp-cluster from ns and rec-reate it with ETP type Local")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service-etp-cluster", "-n", e2eTestNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		svc_pmtrs = map[string]string{
			"$servicename":           "test-service-etp-local",
			"$namespace":             e2eTestNamespace,
			"$label":                 "test-service",
			"$internalTrafficPolicy": "",
			"$externalTrafficPolicy": "Local",
			"$ipFamilyPolicy":        "",
			"$selector":              "hello-pod",
			"$serviceType":           "NodePort",
		}
		createServiceforUshift(oc, svc_pmtrs)
		g.By("Get new service's NodePort")
		nodePort, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "test-service-etp-local", "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		svcURL = net.JoinHostPort(nodeIP, nodePort)
		g.By("curl NodePort Service")
		_, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())

	})
})
