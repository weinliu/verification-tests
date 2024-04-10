package networking

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"regexp"
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
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
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
	g.It("MicroShiftOnly-Author:anusaxen-High-60746-Check nodeport service for external/internal traffic policy and via secondary nic works well on Microshift[Disruptive]", func() {
		var (
			caseID           = "60746"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			nodeName         string
			etp              string
			itp              string
			nodeIP           string
			serviceName      string
			output           string
		)

		exutil.By("Create a namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("Creating hello pod in namespace")
		pod_pmtrs := map[string]string{
			"$podname":   "hello-pod",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod",
		}

		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod")

		exutil.By("Creating test pod in namespace")
		pod_pmtrs = map[string]string{
			"$podname":   "test-pod",
			"$namespace": e2eTestNamespace,
			"$label":     "test-pod",
		}
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "test-pod")

		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP = getNodeIPv4(oc, e2eTestNamespace, nodeName)

		secNICip := getSecondaryNICip(oc)

		//in first iteration we will create Clustr-Cluter ETP and ITP services and in 2nd iteration it will be Local-Local
		for j := 0; j < 2; j++ {
			if j == 0 {
				itp = ""
				etp = ""
				exutil.By("Create NodePort service with ETP and ITP as Cluster")
				serviceName = "nptest-etp-itp-cluster"
			} else {
				etp = "Local"
				itp = "Local"
				exutil.By("Create NodePort service with ETP and ITP as Local")
				serviceName = "nptest-etp-itp-local"
			}

			svc_pmtrs := map[string]string{
				"$servicename":           serviceName,
				"$namespace":             e2eTestNamespace,
				"$label":                 "test-service",
				"$internalTrafficPolicy": itp,
				"$externalTrafficPolicy": etp,
				"$ipFamilyPolicy":        "",
				"$selector":              "hello-pod",
				"$serviceType":           "NodePort",
			}
			createServiceforUshift(oc, svc_pmtrs)

			exutil.By(fmt.Sprintf("Get service port and NodeIP value for service %s", serviceName))
			nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, serviceName, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			svcURL := net.JoinHostPort(nodeIP, nodePort)
			sec_nic_url := net.JoinHostPort(secNICip, nodePort)

			//Check ETP and ITP Cluster and Local type services via debugnode and test pod respectively
			// Access service from nodeIP to validate ETP Cluster/Local. Default emty svc_pmtrs will create both ETP and ITP as Cluster in first iteration
			exutil.By(fmt.Sprintf("Curl NodePort service %s on node IP", serviceName))
			output, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

			//Access service via secondary NIC to simulate ETP Cluster/Local
			exutil.By(fmt.Sprintf("Curl NodePort service %s on secondary node IP", serviceName))
			output, err = exutil.DebugNode(oc, nodeName, "curl", sec_nic_url, "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

			// Access service from cluster's pod network to validate ITP Cluster/Local
			exutil.By(fmt.Sprintf("Curl NodePort Service %s again from a test pod", serviceName))
			output, err = e2eoutput.RunHostCmd(e2eTestNamespace, "test-pod", "curl --connect-timeout 5 -s "+svcURL)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

		}
		//following block of code is to test impact of firewalld reload on any of service created earlier
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "nptest-etp-itp-cluster", "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		svcURL := net.JoinHostPort(nodeIP, nodePort)

		exutil.By("Reload the firewalld and then check nodeport service still can be worked")
		_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "firewall-cmd --reload")
		o.Expect(err).NotTo(o.HaveOccurred())
		firewallState, err := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "firewall-cmd --state")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(firewallState).To(o.ContainSubstring("running"))

		_, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "10")
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	// author: zzhao@redhat.com
	// modified: asood@redhat.com
	g.It("MicroShiftOnly-Author:zzhao-Critical-60968-Check loadbalance service with different external and internal traffic policies works well on Microshift", func() {
		var (
			caseID           = "60968"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			etp              string
			itp              string
			nodeName         string
			nodeIP           string
			serviceName      string
			output           string
		)

		exutil.By("Create a namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("Creating pods in namespace")
		for j := 0; j < 2; j++ {
			pod_pmtrs := map[string]string{
				"$podname":   "hello-pod-" + strconv.Itoa(j),
				"$namespace": e2eTestNamespace,
				"$label":     "hello-pod",
			}

			createPingPodforUshift(oc, pod_pmtrs)
			waitPodReady(oc, e2eTestNamespace, "hello-pod-"+strconv.Itoa(j))
		}

		exutil.By("Creating test pod in namespace")
		pod_pmtrs := map[string]string{
			"$podname":   "test-pod",
			"$namespace": e2eTestNamespace,
			"$label":     "test-pod",
		}
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "test-pod")

		nodeName, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod-0", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		nodeIP = getNodeIPv4(oc, e2eTestNamespace, nodeName)

		policy := [2]string{"Cluster", "Local"}

		for i := 0; i < 2; i++ {
			if i == 0 {
				itp = ""
				etp = policy[i]
				exutil.By(fmt.Sprintf("Create LoadBalance service with ETP and ITP as %s", policy[i]))
				serviceName = "lbtest-etp-itp-" + strings.ToLower(etp)
			} else {
				etp = policy[i]
				itp = policy[i]
				exutil.By(fmt.Sprintf("Create LoadBalance service with ETP and ITP as %s", policy[i]))
				serviceName = "lbtest-etp-itp-" + strings.ToLower(policy[i])
			}

			svc_pmtrs := map[string]string{
				"$servicename":           serviceName,
				"$namespace":             e2eTestNamespace,
				"$label":                 "test-service",
				"$internalTrafficPolicy": itp,
				"$externalTrafficPolicy": etp,
				"$ipFamilyPolicy":        "",
				"$selector":              "hello-pod",
				"$serviceType":           "LoadBalancer",
			}
			createServiceforUshift(oc, svc_pmtrs)

			exutil.By(fmt.Sprintf("Get service port and NodeIP value for service %s", serviceName))

			svcPort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, serviceName, "-o=jsonpath={.spec.ports[*].port}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			svcURL := net.JoinHostPort(nodeIP, svcPort)
			if serviceName == "lbtest-etp-itp-cluster" {
				//Access service from host networked pod
				exutil.By(fmt.Sprintf("Curl LoadBalance service %s", serviceName))
				output, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

				exutil.By(fmt.Sprintf("Delete lb service %s", serviceName))
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", serviceName, "-n", e2eTestNamespace).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				//firewalld entries are removed when service is deleted
				exutil.By(fmt.Sprintf("Curl LoadBalance service %s again", serviceName))
				output, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
				o.Expect(err).To(o.HaveOccurred())
				o.Expect(output).ShouldNot(o.ContainSubstring("Hello OpenShift"))

			} else {
				// Access service from within cluster from pod on cluster network
				exutil.By(fmt.Sprintf("Curl loadbalance Service %s from within cluster", serviceName))
				output, err = e2eoutput.RunHostCmd(e2eTestNamespace, "test-pod", "curl --connect-timeout 5 -s "+svcURL)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

				exutil.By(fmt.Sprintf("Delete lb service %s", serviceName))
				err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", serviceName, "-n", e2eTestNamespace).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				exutil.By(fmt.Sprintf("Curl loadbalance Service %s again from within cluster", serviceName))
				output, err = e2eoutput.RunHostCmd(e2eTestNamespace, "test-pod", "curl --connect-timeout 5 -s "+svcURL)
				o.Expect(err).To(o.HaveOccurred())
				o.Expect(output).ShouldNot(o.ContainSubstring("Hello OpenShift"))

			}
		}

	})
	// author: zzhao@redhat.com
	g.It("MicroShiftOnly-Author:zzhao-Medium-61218-only one loadbalance can be located at same time if creating multi loadbalance service with same port[Serial]", func() {
		var (
			caseID           = "61218"
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

		g.By("Create one loadbalance service")
		svc_pmtrs := map[string]string{
			"$servicename":           "lbtest",
			"$namespace":             e2eTestNamespace,
			"$label":                 "test-service",
			"$internalTrafficPolicy": "",
			"$externalTrafficPolicy": "",
			"$ipFamilyPolicy":        "",
			"$selector":              "hello-pod",
			"$serviceType":           "LoadBalancer",
		}
		createServiceforUshift(oc, svc_pmtrs)

		g.By("Create second loadbalance service")
		svc_pmtrs2 := map[string]string{
			"$servicename":           "lbtest2",
			"$namespace":             e2eTestNamespace,
			"$label":                 "test-service",
			"$internalTrafficPolicy": "",
			"$externalTrafficPolicy": "",
			"$ipFamilyPolicy":        "",
			"$selector":              "hello-pod",
			"$serviceType":           "LoadBalancer",
		}
		createServiceforUshift(oc, svc_pmtrs2)

		g.By("Get service port and NodeIP value")

		svcPort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "lbtest", "-o=jsonpath={.spec.ports[*].port}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		nodeName, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, e2eTestNamespace, nodeName)

		g.By("Check first lb service get node ip")
		lbIngressip, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "lbtest", "-o=jsonpath={.status.loadBalancer.ingress[*].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(lbIngressip).Should(o.ContainSubstring(nodeIP))

		g.By("Check second lb service should't get node ip")
		lbIngressip2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "lbtest2", "-o=jsonpath={.status.loadBalancer.ingress[*].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(lbIngressip2).ShouldNot(o.ContainSubstring(nodeIP))

		svcURL := net.JoinHostPort(nodeIP, svcPort)
		g.By("curl loadbalance Service")
		output, err := exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

		g.By("Delete lb service")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "lbtest", "-n", e2eTestNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		output1 := wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
			lbIngressip2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "lbtest2", "-o=jsonpath={.status.loadBalancer.ingress[*].ip}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(lbIngressip2, nodeIP) {
				return true, nil
			}
			e2e.Logf("second loadbalance still not get node ip")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(output1, fmt.Sprintf("lbtest2 cannot get the nodeip:%s", output1))

		g.By("check lbtest2 ingressip can be accessed")
		output, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

	})

	// author: zzhao@redhat.com
	g.It("MicroShiftOnly-Author:zzhao-Medium-61168-hostnetwork pods and container pods should be able to access kubernets svc api after reboot cluster[Disruptive]", func() {
		var (
			caseID           = "61168"
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
		hellosdnPodName := getPodName(oc, e2eTestNamespace, "name=hello-pod")

		g.By("using dns resolve as hostnetwork pods for checking")
		dnsPodName := getPodName(oc, "openshift-dns", "dns.operator.openshift.io/daemonset-node-resolver=")

		g.By("Check container pod and hostnetwork can access kubernete api")
		output, err := e2eoutput.RunHostCmd(e2eTestNamespace, hellosdnPodName[0], "curl -I --connect-timeout 5 https://10.43.0.1:443 -k")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("HTTP/2 403"))
		output1, err := e2eoutput.RunHostCmd("openshift-dns", dnsPodName[0], "curl -I --connect-timeout 5 https://10.43.0.1:443 -k")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output1).Should(o.ContainSubstring("HTTP/2 403"))

		g.By("reboot node")
		nodeName, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		rebootUshiftNode(oc, nodeName)

		g.By("Check container pod can access kubernete api")
		curlOutput := wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
			output, err = e2eoutput.RunHostCmd(e2eTestNamespace, hellosdnPodName[0], "curl -I --connect-timeout 5 https://10.43.0.1:443 -k")
			if strings.Contains(output, "HTTP/2 403") {
				return true, nil
			}
			e2e.Logf("pods are not ready, try again")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(curlOutput, fmt.Sprintf("Fail to terminate pods:%s", curlOutput))

		g.By("Check hostnetwork can access kubernete api")
		curlHostnetworkOutput := wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
			output, err = e2eoutput.RunHostCmd("openshift-dns", dnsPodName[0], "curl -I --connect-timeout 5 https://10.43.0.1:443 -k")
			if strings.Contains(output, "HTTP/2 403") {
				return true, nil
			}
			e2e.Logf("dns pods are not ready, try again")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(curlHostnetworkOutput, fmt.Sprintf("Fail to terminate pods:%s", curlHostnetworkOutput))

	})

	// author: zzhao@redhat.com
	g.It("MicroShiftOnly-Author:zzhao-Medium-61164-ovn MTU can be updated if it's value is less than default interface mtu[Disruptive]", func() {
		var (
			caseID           = "61164"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			mtu              = "1400"
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
		hellosdnPodName := getPodName(oc, e2eTestNamespace, "name=hello-pod")

		g.By("Update the cluster MTU to 1400")
		nodeName, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		setMTU(oc, nodeName, mtu)
		defer rollbackMTU(oc, nodeName)

		g.By("Create one new pods to check the mtu")
		pod_pmtrs1 := map[string]string{
			"$podname":   "hello-pod2",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod2",
		}

		createPingPodforUshift(oc, pod_pmtrs1)
		waitPodReady(oc, e2eTestNamespace, "hello-pod2")
		hellosdnPodName2 := getPodName(oc, e2eTestNamespace, "name=hello-pod2")

		g.By("Check new created pod mtu changed")
		output, err := e2eoutput.RunHostCmd(e2eTestNamespace, hellosdnPodName2[0], "cat /sys/class/net/eth0/mtu")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(mtu))

		g.By("check existing pod mtu changed")
		output2, err := e2eoutput.RunHostCmd(e2eTestNamespace, hellosdnPodName[0], "cat /sys/class/net/eth0/mtu")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output2).Should(o.ContainSubstring(mtu))

	})
	// author: zzhao@redhat.com
	g.It("MicroShiftOnly-Author:zzhao-Medium-61161-Expose coredns forward as configurable option[Disruptive][Flaky]", func() {

		g.By("Check the default coredns config file")
		dnsConfigMap, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "cm", "dns-default", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(dnsConfigMap).Should(o.ContainSubstring("forward . /etc/resolv.conf"))

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeName := nodeList.Items[0].Name

		g.By("cp the default dns config file to a new path")
		cpNewConfig := "mkdir /run/systemd/resolve && cp /etc/resolv.conf /run/systemd/resolve/resolv.conf && systemctl restart microshift"
		rmDnsConfig := "rm -fr /run/systemd/resolve && systemctl restart microshift"
		defer func() {
			exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", rmDnsConfig)
			output := wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
				dnsConfigMap, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "cm", "dns-default", "-o=jsonpath={.data.Corefile}").Output()
				if strings.Contains(dnsConfigMap, "/etc/resolv.conf") {
					return true, nil
				}
				e2e.Logf("dns config has not been updated")
				return false, nil
			})
			exutil.AssertWaitPollNoErr(output, fmt.Sprintf("Fail to updated dns configmap:%s", output))
		}()
		exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", cpNewConfig)

		g.By("Check the coredns is consuming the new config file")
		output := wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
			dnsConfigMap, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "cm", "dns-default", "-o=jsonpath={.data.Corefile}").Output()
			if strings.Contains(dnsConfigMap, "/run/systemd/resolve/resolv.conf") {
				return true, nil
			}
			e2e.Logf("dns config has not been updated")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(output, fmt.Sprintf("Fail to updated dns configmap:%s", output))

	})

	// author: huirwang@redhat.com
	g.It("MicroShiftOnly-Author:huirwang-High-60969-Blocking external access to the NodePort service on a specific host interface. [Disruptive]", func() {
		var (
			caseID           = "60969"
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
		svc, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "test-service-etp-cluster").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svc).Should(o.ContainSubstring("test-service-etp-cluster"))

		g.By("Get service NodePort and NodeIP value")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "test-service-etp-cluster", "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeName, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, e2eTestNamespace, nodeName)
		svcURL := net.JoinHostPort(nodeIP, nodePort)
		g.By("curl NodePort Service")
		curlNodeCmd := fmt.Sprintf("curl %s -s --connect-timeout 5", svcURL)
		_, err = exec.Command("bash", "-c", curlNodeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Insert a new rule in the nat table PREROUTING chain to drop all packets that match the destination port and IP address")
		defer removeIPRules(oc, nodePort, nodeIP, nodeName)
		ipDropCmd := fmt.Sprintf("nft -a insert rule ip nat PREROUTING tcp dport %v ip daddr %s drop", nodePort, nodeIP)
		_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", ipDropCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify NodePort Service is blocked")
		_, err = exec.Command("bash", "-c", curlNodeCmd).Output()
		o.Expect(err).To(o.HaveOccurred())

		g.By("Remove the added new rule")
		removeIPRules(oc, nodePort, nodeIP, nodeName)

		g.By("Verify the NodePort service can be accessed again.")
		_, err = exec.Command("bash", "-c", curlNodeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: asood@redhat.com
	g.It("MicroShiftOnly-Author:asood-High-64753-Check disabling IPv4 forwarding makes the nodeport service inaccessible. [Disruptive]", func() {
		var (
			caseID           = "64753"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			serviceName      = "test-service-" + caseID
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

		svc_pmtrs := map[string]string{
			"$servicename":           serviceName,
			"$namespace":             e2eTestNamespace,
			"$label":                 "test-service",
			"$internalTrafficPolicy": "",
			"$externalTrafficPolicy": "",
			"$ipFamilyPolicy":        "",
			"$selector":              "hello-pod",
			"$serviceType":           "NodePort",
		}
		createServiceforUshift(oc, svc_pmtrs)
		svc, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, serviceName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svc).Should(o.ContainSubstring(serviceName))

		g.By("Get service NodePort and NodeIP value")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, serviceName, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeName, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod", "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, e2eTestNamespace, nodeName)
		svcURL := net.JoinHostPort(nodeIP, nodePort)
		e2e.Logf("Service URL %s", svcURL)

		g.By("Curl NodePort Service")
		curlNodeCmd := fmt.Sprintf("curl %s -s --connect-timeout 5", svcURL)
		_, err = exec.Command("bash", "-c", curlNodeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Disable IPv4 forwarding")
		enableIPv4ForwardingCmd := fmt.Sprintf("sysctl -w net.ipv4.ip_forward=1")
		disableIPv4ForwardingCmd := fmt.Sprintf("sysctl -w net.ipv4.ip_forward=0")
		defer func() {
			_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", enableIPv4ForwardingCmd)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", disableIPv4ForwardingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify NodePort Service is no longer accessible")
		_, err = exec.Command("bash", "-c", curlNodeCmd).Output()
		o.Expect(err).To(o.HaveOccurred())

		g.By("Enable IPv4 forwarding")
		_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", enableIPv4ForwardingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify the NodePort service can be accessed again.")
		_, err = exec.Command("bash", "-c", curlNodeCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: huirwang@redhat.com
	g.It("MicroShiftOnly-Author:huirwang-Medium-61162-Hostname changes should not block ovn. [Disruptive]", func() {
		var (
			caseID           = "61162"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
		)

		exutil.By("Create a namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(nodeList.Items) > 0).To(o.BeTrue())
		nodeName := nodeList.Items[0].Name

		exutil.By("Change node hostname")
		newHostname := fmt.Sprintf("%v.61162", nodeName)
		e2e.Logf("Changing the host name to %v", newHostname)
		setHostnameCmd := fmt.Sprintf("hostnamectl set-hostname %v", newHostname)
		defer func() {
			_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "hostnamectl set-hostname \"\" ;hostnamectl set-hostname --transient "+nodeName)
			o.Expect(err).NotTo(o.HaveOccurred())
			restartMicroshiftService(oc, nodeName)
		}()
		_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", setHostnameCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		restartMicroshiftService(oc, nodeName)

		exutil.By("Verify the ovn pods running well.")
		err = waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-master")
		exutil.AssertWaitPollNoErr(err, "wait for ovnkube-master pods ready timeout")
		err = waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		exutil.AssertWaitPollNoErr(err, "wait for ovnkube-node pods ready timeout")

		exutil.By("Verify the hostname is new hostname ")
		hostnameOutput, err := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "cat /etc/hostname")
		o.Expect(err).NotTo(o.HaveOccurred())
		pattern := `dhcp.*\.61162`
		re := regexp.MustCompile(pattern)
		cuhostname := re.FindString(hostnameOutput)
		e2e.Logf("Current hostname is %v,expected hostname is %v", cuhostname, newHostname)
		o.Expect(cuhostname == newHostname).To(o.BeTrue())

		exutil.By("Verify test pods working well.")
		pod_pmtrs := map[string]string{
			"$podname":   "hello-pod1",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod",
		}

		exutil.By("create 1st hello pod in namespace")
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod1")

		pod_pmtrs = map[string]string{
			"$podname":   "hello-pod2",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod",
		}
		exutil.By("create 2nd hello pod in same namespace")
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod2")

		exutil.By("curl hello-pod2 to hello-pod1")
		helloPod1IP := getPodIPv4(oc, e2eTestNamespace, "hello-pod1")
		output, err := e2eoutput.RunHostCmd(e2eTestNamespace, "hello-pod2", "curl --connect-timeout 5 -s "+net.JoinHostPort(helloPod1IP, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

	})

	// author: anusaxen@redhat.com
	g.It("MicroShiftOnly-Author:anusaxen-High-64752-Conntrack rule deletion for UDP traffic when NodePort service ep gets deleted", func() {
		var (
			caseID              = "64752"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			e2eTestNamespace    = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			udpListenerPod      = filepath.Join(buildPruningBaseDir, "udp-listener.yaml")
		)

		g.By("Create a namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		pod_pmtrs := map[string]string{
			"$podname":   "hello-pod",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod",
		}

		g.By("creating hello pod client in namespace")
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod")

		g.By("create UDP Listener Pod")
		createResourceFromFile(oc, e2eTestNamespace, udpListenerPod)
		err := waitForPodWithLabelReady(oc, e2eTestNamespace, "name=udp-pod")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=udp-pod not ready")

		//expose udp pod to nodeport service
		err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("pod", "udp-pod", "-n", e2eTestNamespace, "--type=NodePort", "--port=8080", "--protocol=UDP").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		udpListenerPodIP := getPodIPv4(oc, e2eTestNamespace, "udp-pod")

		g.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())

		g.By("Get service NodePort and NodeIP value")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, "udp-pod", "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//send a test packet to udp endpoint which will create an udp conntrack entry on the node
		nodeIP := getNodeIPv4(oc, e2eTestNamespace, nodeList.Items[0].Name)
		cmd_traffic := " for n in {1..3}; do echo $n; sleep 1; done > /dev/udp/" + nodeIP + "/" + nodePort

		_, err = exutil.RemoteShPodWithBash(oc, e2eTestNamespace, "hello-pod", cmd_traffic)
		o.Expect(err).NotTo(o.HaveOccurred())

		//make sure the corresponding conntrack entry exists for the udp endpoint
		output, err := exutil.DebugNodeWithChroot(oc, nodeList.Items[0].Name, "conntrack", "-L", "-p", "udp", "--dport", "8080")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, udpListenerPodIP)).Should(o.BeTrue())

		_, err = oc.WithoutNamespace().Run("delete").Args("pod", "-n", e2eTestNamespace, "udp-pod").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//make sure the corresponding conntrack entry goes away as we deleted udp endpoint above
		output, err = exutil.DebugNodeWithChroot(oc, nodeList.Items[0].Name, "conntrack", "-L", "-p", "udp", "--dport", "8080")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, udpListenerPodIP)).ShouldNot(o.BeTrue())

	})

	// author: asood@redhat.com
	g.It("MicroShiftOnly-Author:asood-Medium-63770-Ensure LoadBalancer service serving pods on hostnetwork or cluster network accessible only from primary node IP address and continues to serve after firewalld reload[Disruptive]", func() {
		var (
			caseID           = "63770"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
		)

		exutil.By("Create a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		err := exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())

		pod_pmtrs := map[string]string{
			"$podname":   "hello-pod-host",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod-host",
			"$nodename":  nodeList.Items[0].Name,
		}
		exutil.By("Creating hello pod on host network in namespace")
		createHostNetworkedPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod-host")

		pod_pmtrs = map[string]string{
			"$podname":   "hello-pod-cluster",
			"$namespace": e2eTestNamespace,
			"$label":     "hello-pod-cluster",
		}

		exutil.By("Creating hello pod on cluster network in namespace")
		createPingPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, "hello-pod-cluster")
		secNICIP := getSecondaryNICip(oc)

		podType := [2]string{"host", "cluster"}
		for _, svcSuffix := range podType {
			exutil.By(fmt.Sprintf("Creating service for hello pod on %s network in namespace", svcSuffix))
			serviceName := "test-service-" + svcSuffix
			svc_pmtrs := map[string]string{
				"$servicename":           serviceName,
				"$namespace":             e2eTestNamespace,
				"$label":                 "test-service",
				"$internalTrafficPolicy": "",
				"$externalTrafficPolicy": "",
				"$ipFamilyPolicy":        "",
				"$selector":              "hello-pod-" + svcSuffix,
				"$serviceType":           "LoadBalancer",
			}
			createServiceforUshift(oc, svc_pmtrs)

			exutil.By(fmt.Sprintf("Construct the URLs for the  %s service", serviceName))
			nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", e2eTestNamespace, "pod", "hello-pod-"+svcSuffix, "-o=jsonpath={.spec.nodeName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeIP := getNodeIPv4(oc, e2eTestNamespace, nodeName)

			svcPort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", e2eTestNamespace, serviceName, "-o=jsonpath={.spec.ports[*].port}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			svcURL := net.JoinHostPort(nodeIP, svcPort)
			secNICURL := net.JoinHostPort(secNICIP, svcPort)

			exutil.By(fmt.Sprintf("Checking service for hello pod on %s network is accessible on primary interface", svcSuffix))
			output, err := exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

			exutil.By(fmt.Sprintf("Checking service for hello pod on %s network is not accessible on secondary interface", svcSuffix))
			output, err = exutil.DebugNode(oc, nodeName, "curl", secNICURL, "-s", "--connect-timeout", "5")
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(output).ShouldNot(o.ContainSubstring("Hello OpenShift"))

			exutil.By("Reload the firewalld")
			_, err = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "firewall-cmd --reload")
			o.Expect(err).NotTo(o.HaveOccurred())
			firewallState, err := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "firewall-cmd --state")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(firewallState).To(o.ContainSubstring("running"))

			exutil.By(fmt.Sprintf("Checking service for hello pod on %s network is accessible after firewalld reload", svcSuffix))
			output, err = exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))

			exutil.By(fmt.Sprintf("Delete LB service %s", serviceName))
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", serviceName, "-n", e2eTestNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

		}

	})

	// author: jechen@redhat.com
	g.It("MicroShiftOnly-Author:jechen-High-65838-br-ex interface should be unmanaged by NetworkManager", func() {

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())

		exutil.By("Check if br-ex on the node is unmanaged")
		e2e.Logf("Check br-ex on node %v", nodeList.Items[0].Name)
		connections, err := exutil.DebugNodeWithChroot(oc, nodeList.Items[0].Name, "bash", "-c", "nmcli conn show")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(connections, "br-ex")).To(o.BeFalse())

	})

	// author: jechen@redhat.com
	g.It("MicroShiftOnly-Author:jechen-High-65840-Killing openvswitch service should reconcile OVN control plane back to normal [Disruptive]", func() {

		exutil.By("Get the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())

		exutil.By("Kill openvswitch on the node")
		e2e.Logf("Kill openvswitch on node %v", nodeList.Items[0].Name)
		_, err := exutil.DebugNodeWithChroot(oc, nodeList.Items[0].Name, "bash", "-c", "pkill -9 -f openvswitch")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check ovs-vswitchd and ovsdb-server are back into active running state")
		output, err := exutil.DebugNodeWithChroot(oc, nodeList.Items[0].Name, "bash", "-c", "systemctl status ovs-vswitchd")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("active (running)"))
		output, err = exutil.DebugNodeWithChroot(oc, nodeList.Items[0].Name, "bash", "-c", "systemctl status ovsdb-server")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("active (running)"))

		exutil.By("Check all pods in openshift-ovn-kubernetes are back to normal in running state")
		statusErr := waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "component=network")
		o.Expect(statusErr).NotTo(o.HaveOccurred())

	})

	g.It("MicroShiftOnly-Author:weliang-Medium-72796-Multus CNI bridge with host-local", func() {
		var (
			nadName          = "bridge-host-local"
			caseID           = "72796"
			e2eTestNamespace = "e2e-ushift-sdn-" + caseID + "-" + getRandomString()
			pod1Name         = "bridge-host-local-pod1"
			pod2Name         = "bridge-host-local-pod2"
		)

		exutil.By("Creating a namespace for the scenario")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		err := exutil.SetNamespacePrivileged(oc, e2eTestNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Configuring a NetworkAttachmentDefinition using bridge with host-local")
		NAD_pmtrs := map[string]string{
			"$nadname":      nadName,
			"$namespace":    e2eTestNamespace,
			"$plugintype":   "bridge",
			"$mode":         " ",
			"$ipamtype":     "host-local",
			"$ipv4range":    "192.168.10.0/24",
			"$ipv6range":    "fd00:dead:beef:10::/64",
			"$v4rangestart": "192.168.10.1",
			"$v4rangeend":   "192.168.10.9",
			"$v6rangestart": "fd00:dead:beef:10::1",
			"$v6rangeend":   "fd00:dead:beef:10::9",
		}
		defer removeResource(oc, true, true, "net-attach-def", nadName, "-n", e2eTestNamespace)
		createMultusNADHostlocalforUshift(oc, NAD_pmtrs)

		exutil.By("Verifying the configued NetworkAttachmentDefinition")
		if checkNAD(oc, e2eTestNamespace, nadName) {
			e2e.Logf("The correct network-attach-defintion: %v is created!", nadName)
		} else {
			e2e.Failf("The correct network-attach-defintion: %v is not created!", nadName)
		}

		exutil.By("Configuring first pod to get additional network")
		pod_pmtrs := map[string]string{
			"$podname":    pod1Name,
			"$namespace":  e2eTestNamespace,
			"$podlabel":   pod1Name,
			"$nadname":    nadName,
			"$podenvname": pod1Name,
		}
		defer removeResource(oc, true, true, "pod", pod1Name, "-n", e2eTestNamespace)
		createMultusPodforUshift(oc, pod_pmtrs)
		waitPodReady(oc, e2eTestNamespace, pod1Name)

		exutil.By("Configuring second pod to get additional network")
		pod2_pmtrs := map[string]string{
			"$podname":    pod2Name,
			"$namespace":  e2eTestNamespace,
			"$podlabel":   pod2Name,
			"$nadname":    nadName,
			"$podenvname": pod2Name,
		}
		defer removeResource(oc, true, true, "pod", pod2Name, "-n", e2eTestNamespace)
		createMultusPodforUshift(oc, pod2_pmtrs)
		waitPodReady(oc, e2eTestNamespace, pod2Name)

		g.By("Get IPs from pod1's secondary interface")
		pod1Net1IPv4, pod1Net1IPv6 := getPodMultiNetworks(oc, e2eTestNamespace, pod1Name, "net1")
		e2e.Logf("The v4 address of pod1's net1 is: %v", pod1Net1IPv4)
		e2e.Logf("The v6 address of pod1's net1 is: %v", pod1Net1IPv6)

		g.By("Get IPs from pod2's secondary interface")
		pod2Net1IPv4, pod2Net1IPv6 := getPodMultiNetworks(oc, e2eTestNamespace, pod2Name, "net1")
		e2e.Logf("The v4 address of pod2's net1 is: %v", pod2Net1IPv4)
		e2e.Logf("The v6 address of pod2's net1 is: %v", pod2Net1IPv6)

		g.By("Checking the connectivity from pod 1 to pod 2 over secondary interface - net1")
		CurlMultusPod2PodPass(oc, e2eTestNamespace, pod1Name, pod2Net1IPv4, "net1", pod2Name)
		CurlMultusPod2PodPass(oc, e2eTestNamespace, pod1Name, pod2Net1IPv6, "net1", pod2Name)
	})
})
