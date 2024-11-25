package networking

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	netutils "k8s.io/utils/net"
)

var _ = g.Describe("[sig-networking] SDN network-tools ovnkube-trace", func() {
	defer g.GinkgoRecover()

	var (
		oc               = exutil.NewCLI("networking-tools", exutil.KubeConfigPath())
		expPod2PodResult = []string{"ovn-trace source pod to destination pod indicates success",
			"ovn-trace destination pod to source pod indicates success",
			"ovs-appctl ofproto/trace source pod to destination pod indicates success",
			"ovs-appctl ofproto/trace destination pod to source pod indicates success",
			"ovn-detrace source pod to destination pod indicates success",
			"ovn-detrace destination pod to source pod indicates success"}
		expPod2PodRemoteResult = []string{"ovn-trace (remote) source pod to destination pod indicates success",
			"ovn-trace (remote) destination pod to source pod indicates success"}
		expPod2SvcResult = []string{"ovn-trace source pod to service clusterIP indicates success"}
		expPod2IPResult  = []string{"ovn-trace from pod to IP indicates success",
			"ovs-appctl ofproto/trace pod to IP indicates success",
			"ovn-detrace pod to external IP indicates success"}
	)

	g.BeforeEach(func() {
		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
	})

	// author: qiowang@redhat.com
	g.It("Author:qiowang-Medium-67625-Medium-67648-Check ovnkube-trace - pod2pod traffic and pod2hostnetworkpod traffic", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			hostNetworkPodTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-hostnetwork-specific-node-template.yaml")
		)
		nodeList, getNodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough nodes available for the test, skip the case!!")
		}
		workerNode1 := nodeList.Items[0].Name
		workerNode2 := nodeList.Items[1].Name
		tmpPath := "/tmp/ocp-67625-67648"
		defer os.RemoveAll(tmpPath)
		cpOVNKubeTraceToLocal(oc, tmpPath)

		exutil.By("1. Create hello-pod1, pod located on the first node")
		ns := oc.Namespace()
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns,
			nodename:  workerNode1,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("2. Create hello-pod2 and hostnetwork hostnetwork-hello-pod2, pod located on the first node")
		//Required for hostnetwork pod
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-group", "privileged", "system:serviceaccounts:"+ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns,
			nodename:  workerNode1,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)
		hostnetworkPod2 := pingPodResourceNode{
			name:      "hostnetwork-hello-pod2",
			namespace: ns,
			nodename:  workerNode1,
			template:  hostNetworkPodTemplate,
		}
		hostnetworkPod2.createPingPodNode(oc)
		waitPodReady(oc, hostnetworkPod2.namespace, hostnetworkPod2.name)

		exutil.By("3. Create hello-pod3 and hostnetwork hostnetwork-hello-pod3, pod located on the second node")
		pod3 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns,
			nodename:  workerNode2,
			template:  pingPodNodeTemplate,
		}
		pod3.createPingPodNode(oc)
		waitPodReady(oc, pod3.namespace, pod3.name)
		hostnetworkPod3 := pingPodResourceNode{
			name:      "hostnetwork-hello-pod3",
			namespace: ns,
			nodename:  workerNode2,
			template:  hostNetworkPodTemplate,
		}
		hostnetworkPod3.createPingPodNode(oc)
		waitPodReady(oc, hostnetworkPod3.namespace, hostnetworkPod3.name)

		exutil.By("4. Simulate traffic between pod and pod when they land on the same node")
		podIP1 := getPodIPv4(oc, ns, pod1.name)
		addrFamily := "ip4"
		if netutils.IsIPv6String(podIP1) {
			addrFamily = "ip6"
		}
		cmd := tmpPath + "/ovnkube-trace -src-namespace " + ns + " -src " + pod1.name + " -dst-namespace " + ns + " -dst " + pod2.name + " -tcp -addr-family " + addrFamily
		traceOutput, cmdErr := exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		for _, expResult := range expPod2PodResult {
			o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
		}

		exutil.By("5. Simulate traffic between pod and pod when they land on different nodes")
		cmd = tmpPath + "/ovnkube-trace -src-namespace " + ns + " -src " + pod1.name + " -dst-namespace " + ns + " -dst " + pod3.name + " -tcp -addr-family " + addrFamily
		traceOutput, cmdErr = exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		for _, expResult := range expPod2PodResult {
			o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
		}
		for _, expResult := range expPod2PodRemoteResult {
			o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
		}

		exutil.By("6. Simulate traffic between pod and hostnetwork pod when they land on the same node")
		cmd = tmpPath + "/ovnkube-trace -src-namespace " + ns + " -src " + pod1.name + " -dst-namespace " + ns + " -dst " + hostnetworkPod2.name + " -udp -addr-family " + addrFamily
		traceOutput, cmdErr = exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		for _, expResult := range expPod2PodResult {
			o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
		}

		exutil.By("7. Simulate traffic between pod and hostnetwork pod when they land on different nodes")
		cmd = tmpPath + "/ovnkube-trace -src-namespace " + ns + " -src " + pod1.name + " -dst-namespace " + ns + " -dst " + hostnetworkPod3.name + " -udp -addr-family " + addrFamily
		traceOutput, cmdErr = exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		for _, expResult := range expPod2PodResult {
			o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
		}
		o.Expect(strings.Contains(string(traceOutput), expPod2PodRemoteResult[1])).Should(o.BeTrue())
	})

	g.It("Author:qiowang-Medium-67649-Check ovnkube-trace - pod2service traffic", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		)
		nodeList, getNodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough nodes available for the test, skip the case!!")
		}
		tmpPath := "/tmp/ocp-67649"
		defer os.RemoveAll(tmpPath)
		cpOVNKubeTraceToLocal(oc, tmpPath)

		exutil.By("1. Create hello-pod")
		ns := oc.Namespace()
		pod := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod.createPingPod(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		exutil.By("2. Simulate traffic between pod and service")
		podIP1 := getPodIPv4(oc, ns, pod.name)
		addrFamily := "ip4"
		if netutils.IsIPv6String(podIP1) {
			addrFamily = "ip6"
		}
		cmd := tmpPath + "/ovnkube-trace -src-namespace " + ns + " -src " + pod.name + " -dst-namespace openshift-dns -service dns-default -tcp -addr-family " + addrFamily
		traceOutput, cmdErr := exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		for _, expResult := range expPod2PodResult {
			o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
		}
		for _, expResult := range expPod2SvcResult {
			o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
		}
	})

	g.It("Author:qiowang-NonPreRelease-Medium-55180-Check ovnkube-trace - pod2external traffic [Disruptive]", func() {
		var (
			testScope           = []string{"without egressip", "with egressip"}
			egressNodeLabel     = "k8s.ovn.org/egress-assignable"
			externalIPv4        = "8.8.8.8"
			externalIPv6        = "2001:4860:4860::8888"
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIPTemplate    = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		)
		nodeList, getNodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough nodes available for the test, skip the case!!")
		}

		// check if the cluster supported for test steps related to egressip
		// focus on RDU dualstack/ipv6single cluster for ipv6 traffic, and other supported platforms for ipv4 traffic
		testList := []string{testScope[0]}
		addrFamily := "ip4"
		externalIP := externalIPv4
		ipStackType := checkIPStackType(oc)
		if ipStackType == "dualstack" || ipStackType == "ipv6single" {
			addrFamily = "ip6"
			externalIP = externalIPv6
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
			if err != nil || !(strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
				e2e.Logf("Test steps related egressip will only run on rdu1 or rdu2 dualstack/ipv6single cluster, skip for other envrionment!!")
			} else {
				testList = append(testList, testScope[1])
			}
		} else {
			platform := exutil.CheckPlatform(oc)
			acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "azure") || strings.Contains(platform, "nutanix")
			if !acceptedPlatform {
				e2e.Logf("Test steps related egressip should be run on AWS/GCP/Azure/Openstack/Vsphere/BareMetal/Nutanix cluster, will skip for other platforms!!")
			} else {
				testList = append(testList, testScope[1])
			}
		}

		tmpPath := "/tmp/ocp-55180"
		defer os.RemoveAll(tmpPath)
		cpOVNKubeTraceToLocal(oc, tmpPath)

		var nsList, podList []string
		for _, testItem := range testList {
			exutil.By("Verify pod2external traffic when the pod associate " + testItem)
			exutil.By("Create namespace")
			oc.SetupProject()
			ns := oc.Namespace()
			nsList = append(nsList, ns)

			if testItem == "with egressip" {
				exutil.By("Label namespace with name=test")
				defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name-").Execute()
				nsLabelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name=test").Execute()
				o.Expect(nsLabelErr).NotTo(o.HaveOccurred())

				exutil.By("Label EgressIP node")
				egressNode := nodeList.Items[0].Name
				defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)
				e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")

				exutil.By("Create egressip object")
				var freeIPs []string
				if ipStackType == "dualstack" || ipStackType == "ipv6single" {
					freeIPs = findFreeIPv6s(oc, egressNode, 2)
				} else {
					freeIPs = findFreeIPs(oc, egressNode, 2)
				}
				o.Expect(len(freeIPs)).Should(o.Equal(2))
				egressip := egressIPResource1{
					name:      "egressip-55180",
					template:  egressIPTemplate,
					egressIP1: freeIPs[0],
					egressIP2: freeIPs[1],
				}
				defer egressip.deleteEgressIPObject1(oc)
				egressip.createEgressIPObject1(oc)
				egressIPMaps := getAssignedEIPInEIPObject(oc, egressip.name)
				o.Expect(len(egressIPMaps)).Should(o.Equal(1))
			}

			exutil.By("Create test pod in the namespace")
			pod := pingPodResource{
				name:      "hello-pod",
				namespace: ns,
				template:  pingPodTemplate,
			}
			pod.createPingPod(oc)
			waitPodReady(oc, pod.namespace, pod.name)
			podList = append(podList, pod.name)

			exutil.By("Simulate traffic between pod and external IP, pod associate " + testItem)
			cmd := tmpPath + "/ovnkube-trace -src-namespace " + ns + " -src " + pod.name + " -dst-ip " + externalIP + " -tcp -addr-family " + addrFamily
			e2e.Logf("ovnkube-trace command: %s", cmd)
			traceOutput, cmdErr := exec.Command("bash", "-c", cmd).Output()
			o.Expect(cmdErr).NotTo(o.HaveOccurred())
			for _, expResult := range expPod2IPResult {
				o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
			}
		}

		exutil.By("Switch gateway mode")
		origMode := getOVNGatewayMode(oc)
		var desiredMode string
		if origMode == "local" {
			desiredMode = "shared"
		} else {
			desiredMode = "local"
		}
		e2e.Logf("Cluster is currently on gateway mode %s", origMode)
		e2e.Logf("Desired mode is %s", desiredMode)
		defer switchOVNGatewayMode(oc, origMode)
		switchOVNGatewayMode(oc, desiredMode)

		for i, testItem := range testList {
			exutil.By("Simulate traffic between pod and external IP, pod associate " + testItem)
			cmd := tmpPath + "/ovnkube-trace -src-namespace " + nsList[i] + " -src " + podList[i] + " -dst-ip " + externalIP + " -tcp -addr-family " + addrFamily
			e2e.Logf("ovnkube-trace command: %s", cmd)
			traceOutput, cmdErr := exec.Command("bash", "-c", cmd).Output()
			o.Expect(cmdErr).NotTo(o.HaveOccurred())
			for _, expResult := range expPod2IPResult {
				o.Expect(strings.Contains(string(traceOutput), expResult)).Should(o.BeTrue())
			}
		}
	})
})
