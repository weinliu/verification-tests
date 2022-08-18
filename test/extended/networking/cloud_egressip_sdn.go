package networking

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var (
		ipEchoURL string
		a         *exutil.AwsClient

		oc = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp")
		if !acceptedPlatform || !strings.Contains(networkType, "sdn") {
			g.Skip("Test cases should be run on AWS or GCP cluster with Openshift-SDN network plugin, skip for other platforms or other network plugin!!")
		}

		switch platform {
		case "aws":
			e2e.Logf("\n AWS is detected, running the case on AWS\n")
			if ipEchoURL == "" {
				getAwsCredentialFromCluster(oc)
				a = exutil.InitAwsSession()
				_, err := getAwsIntSvcInstanceID(a, oc)
				if err != nil {
					e2e.Logf("There is no int svc instance in this cluster, %v", err)
					g.Skip("There is no int svc instance in this cluster, skip the cases!!")
				}

				ipEchoURL, err = installIPEchoServiceOnAWS(a, oc)
				if err != nil {
					e2e.Logf("No ip-echo service installed on the bastion host, %v", err)
					g.Skip("No ip-echo service installed on the bastion host, skip the cases!!")
				}
			}
		case "gcp":
			e2e.Logf("\n GCP is detected, running the case on GCP\n")
			if ipEchoURL == "" {
				// If an int-svc instance with external IP found, IpEcho service will be installed on the int-svc instance
				// otherwise, just give error message and skip the test
				infraID, err := exutil.GetInfraID(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				host, err := getIntSvcExternalIPFromGcp(oc, infraID)
				if err != nil {
					e2e.Logf("There is no int svc instance in this cluster, %v", err)
					g.Skip("There is no int svc instance in this cluster, skip the cases!!")
				}
				ipEchoURL, err = installIPEchoServiceOnGCP(oc, infraID, host)
				if err != nil {
					e2e.Logf("No ip-echo service installed on the bastion host, %v", err)
					g.Skip("No ip-echo service installed on the bastion host, skip the cases!!")
				}
			}
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-46701-High-47470-Pods will lose external access if same egressIP is assigned to different netnamespace, error should be logged on master node. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Pick a node as egressIP node, add egressCIDRs to it")
		// get CIDR on the node
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		sub1 := getIfaddrFromNode(egressNode, oc)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub1+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")

		g.By("2. Create first namespace then create a test pod in it")
		oc.SetupProject()
		ns1 := oc.Namespace()

		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}

		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Find an unused IP on the node, use it as egressIP address, add it to netnamespace of the first project")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub1, 1)

		patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[]}")

		g.By("4. Verify egressCIDRs and egressIPs on the node")
		output := getEgressCIDRs(oc, egressNode)
		o.Expect(output).To(o.ContainSubstring(sub1))

		ip, err := getEgressIPByKind(oc, "hostsubnet", nodeList.Items[0].Name, 1)
		e2e.Logf("\n\n\n got egressIP as -->%v<--\n\n\n", ip)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs[0]).Should(o.BeElementOf(ip))

		g.By("5.From first namespace, check source IP is EgressIP")
		e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
		sourceIP, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.BeElementOf(freeIPs[0]))

		g.By("6. Create second namespace, create a test pod in it")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}

		pod2.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod2.name, "-n", pod2.namespace).Execute()
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("7. Add same egressIP address to netnamespace of the second project")
		patchResourceAsAdmin(oc, "netnamespace/"+pod2.namespace, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod2.namespace, "{\"egressIPs\":[]}")

		g.By("8. check egressIP again after second netnamespace is patched with same egressIP")
		ip, err = getEgressIPByKind(oc, "hostsubnet", nodeList.Items[0].Name, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs[0]).Should(o.BeElementOf(ip))

		g.By("9.Check source IP again from first project, curl command should not succeed, error is expected")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("10.Check source IP again from second project, curl command should not succeed, error is expected")
		_, err = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("11.Error would be logged into SDN pod log on master node")
		sdnPodName, err := exutil.GetPodName(oc, "openshift-sdn", "app=sdn", egressNode)
		e2e.Logf("\n Got sdn pod name for egressNode %v: %v\n", egressNode, sdnPodName)

		podLogs, err := exutil.GetSpecificPodLogs(oc, "openshift-sdn", "sdn", sdnPodName, "'Error processing egress IPs'")
		e2e.Logf("podLogs is %v", podLogs)
		exutil.AssertWaitPollNoErr(err, "Did not get log for from SDN pod of the egressNode")
		o.Expect(podLogs).To(o.ContainSubstring("Error processing egress IPs: Multiple namespaces"))
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-46709-Master balance egressIPs across nodes when there are multiple nodes handling egressIP. [Disruptive]", func() {
		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getIfaddrFromNode(egressNode1, oc)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("2. Find 6 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 6)

		g.By("3. Create 6 namespaces, apply one egressIP from the freeIPs to each namespace")
		var ns = [6]string{}
		for i := 0; i < 6; i++ {
			oc.SetupProject()
			ns[i] = oc.Namespace()
			patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[\""+freeIPs[i]+"\"]}")
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[]}")
		}

		g.By("4. check egressIP for each node, each node should have 3 egressIP addresses assigned from the pool")
		ip1, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 3)
		o.Expect(err).NotTo(o.HaveOccurred())
		ip2, err := getEgressIPByKind(oc, "hostsubnet", egressNode2, 3)
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < 3; i++ {
			o.Expect(ip1[i]).Should(o.BeElementOf(freeIPs))
			o.Expect(ip2[i]).Should(o.BeElementOf(freeIPs))
		}
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-46554-[Automatic EgressIP] no more than one egress IP per node for each namespace. [Disruptive]", func() {
		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getIfaddrFromNode(egressNode1, oc)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("2. Find 3 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 3)

		g.By("3. Create one namespace, apply 3 egressIP to the namespace")
		oc.SetupProject()
		ns := oc.Namespace()
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\",\""+freeIPs[1]+"\",\""+freeIPs[2]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("4. Check egressIP for each node, each node should have 1 egressIP addresses assigned from the pool")
		ip1, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		ip2, err := getEgressIPByKind(oc, "hostsubnet", egressNode2, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip1[0]).Should(o.BeElementOf(freeIPs))
		o.Expect(ip2[0]).Should(o.BeElementOf(freeIPs))
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-46556-[Automatic EgressIP] A pod that is on a node hosting egressIP, it will always use the egressIP of the node . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getIfaddrFromNode(egressNode1, oc)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("2. Find 2 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("3. Create a namespaces, apply the both egressIPs to the namespace, and create a test pod on the egress node")

		oc.SetupProject()
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  egressNode1,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		// check the source IP that the test pod uses
		g.By("4. get egressIP on the node where test pod resides")
		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs))

		g.By("5.Check source IP from the test pod for 5 times, it should always use the egressIP of the egressNode that it resides on")
		for i := 0; i < 5; i++ {
			sourceIP, err := e2e.RunHostCmd(ns, podns.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			e2e.Logf("sourceIP is %v", sourceIP)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.BeElementOf(ip))
		}
	})

	// author: jechen@redhat.com
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-46705-The egressIP should still work fine after the node or network service restarted. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Pick a node as egressIP node")
		// get CIDR on the node
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		sub1 := getIfaddrFromNode(egressNode, oc)

		g.By("2. Create first namespace then create a test pod in it")
		oc.SetupProject()
		ns1 := oc.Namespace()

		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}

		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Find an unused IP on the node, use it as egressIP address, add it to the egress node and netnamespace of the project")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub1, 1)

		patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[]}")

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")

		g.By("4. Verify egressIPs on the node")
		ip, err := getEgressIPByKind(oc, "hostsubnet", nodeList.Items[0].Name, 1)
		e2e.Logf("\n\n\n got egressIP as -->%v<--\n\n\n", ip)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIPs[0]).Should(o.BeElementOf(ip))

		g.By("5.From the namespace, check source IP is EgressIP")
		e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
		sourceIPErr := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			sourceIP, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			if !contains(freeIPs, sourceIP) || err != nil {
				e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, ip, err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))

		g.By("6.Reboot egress node.\n")
		defer checkNodeStatus(oc, egressNode, "Ready")
		rebootNode(oc, egressNode)
		checkNodeStatus(oc, egressNode, "NotReady")
		checkNodeStatus(oc, egressNode, "Ready")

		g.By("7.check source IP is EgressIP again")
		sourceIPErr = wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			sourceIP, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			if !contains(freeIPs, sourceIP) || err != nil {
				e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n", sourceIP, ip, err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(sourceIPErr, fmt.Sprintf("Failed to get sourceIP:%s", sourceIPErr))
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-46555-Medium-46962-[Automatic EgressIP] Random egressIP is used on a pod that is not on a node hosting an egressIP, and random outages with egressIP . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Identify two worker nodes with same subnet as egressIP nodes, pick a third node as non-egressIP node")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("Need at least 3 worker nodes, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}

		e2e.Logf("\nEgressNode1: %v\n", egressNode1)
		e2e.Logf("\nEgressNode2: %v\n", egressNode2)
		e2e.Logf("\nnonEgressNode: %v\n", nonEgressNode)

		g.By("2. Get subnet from egressIP node, add egressCIDRs to both egressIP nodes")
		sub := getIfaddrFromNode(egressNode1, oc)
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("3. Find 2 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("4. Create a namespaces, apply the both egressIPs to the namespace, but create a test pod on the third non-egressIP node")
		oc.SetupProject()
		ns := oc.Namespace()
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		g.By("5.Check SDN pod log of the non-egress node, there should be no 'may be offline' error log ")
		sdnPodName, err := exutil.GetPodName(oc, "openshift-sdn", "app=sdn", nonEgressNode)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sdnPodName).NotTo(o.BeEmpty())
		podlogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(sdnPodName, "-n", "openshift-sdn", "-c", "sdn").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podlogs).NotTo(o.ContainSubstring("may be offline"))

		g.By("6.Check source IP from the test pod for 10 times, it should use either egressIP address as its sourceIP")
		sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[1]))
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-46557-[Manual EgressIP] Random egressIP is used on a pod that is not on a node hosting an egressIP . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Identify two worker nodes with same subnet as egressIP nodes, pick a third node as non-egressIP node")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("Need at least 3 worker nodes, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}

		e2e.Logf("\nEgressNode1: %v\n", egressNode1)
		e2e.Logf("\nEgressNode2: %v\n", egressNode2)
		e2e.Logf("\nnonEgressNode: %v\n", nonEgressNode)

		g.By("2. Find 2 unused IPs from one egress node")
		sub := getIfaddrFromNode(egressNode1, oc)
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("3. Patch egressIP address to egressIP nodes")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[]}")

		g.By("4. Create a namespaces, apply the both egressIPs to the namespace, but create a test pod on the third non-egressIP node")
		oc.SetupProject()
		ns := oc.Namespace()
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		g.By("5.Check source IP from the test pod for 10 times, it should use either egressIP address as its sourceIP")
		sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[1]))
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-46558-[Manual EgressIP] A pod that is on a node hosting egressIP, it will always use the egressIP of the node . [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		sub := getIfaddrFromNode(egressNode1, oc)

		g.By("2. Find 2 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 2)

		g.By("3. Patch egressIP address to egressIP nodes")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[]}")

		g.By("3. Create a namespaces, apply the both egressIPs to the namespace, and create a test pod on the egress node")

		oc.SetupProject()
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  egressNode1,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		// check the source IP that the test pod uses
		g.By("4. get egressIP on the node where test pod resides")
		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode1, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs))

		g.By("5.Check source IP from the test pod for 10 times, it should always use the egressIP of the egressNode that it resides on")
		sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
		o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs[1]))
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-Medium-46963-Should remove the egressIP from the array if it was not being used. [Disruptive]", func() {
		g.By("1. Pick a node as egressIP node, add egressCIDRs to it")
		// get CIDR on the node
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		sub := getIfaddrFromNode(egressNode, oc)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")

		g.By("2. Find 5 unused IPs from the egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 5)

		g.By("3. Create a namespace, patch one egressIP from the freeIPs to the netnamespace, repeat 5 times, replaces the egressIP each time")
		oc.SetupProject()
		ns := oc.Namespace()

		for i := 0; i < 5; i++ {
			patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[i]+"\"]}")
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		}

		g.By("4. check egressIP for the node, it should have only the last egressIP from the freeIPs as its egressIP address")
		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[4]))
		for i := 0; i < 4; i++ {
			o.Expect(ip[0]).ShouldNot(o.BeElementOf(freeIPs[i]))
		}
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47054-The egressIP can be HA if netnamespace has single egressIP . [Disruptive][Slow]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet, add egressCIDRs to them")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		// find a node that is not egress node
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}
		e2e.Logf("\n non-egress node: %v\n", nonEgressNode)

		g.By("2. Get subnet from egressIP node, add egressCIDRs to both egressIP nodes")
		sub := getIfaddrFromNode(egressNode1, oc)
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")

		g.By("3. Find 1 unused IPs from one egress node")
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 1)

		g.By("4. Create a namespaces, apply the egressIP to the namespace, and create a test pod on a non-egress node")
		oc.SetupProject()
		ns := oc.Namespace()

		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podns.name, "-n", podns.namespace).Execute()
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("5. Find the host that is assigned the egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs[0])
		e2e.Logf("\n\n\n foundHost that has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		g.By("6. reboot the host that has egressIP address")
		defer checkNodeStatus(oc, foundHost, "Ready")
		rebootNode(oc, foundHost)

		g.By("7. Get the egressNode that does not have egressIP address assigned previously")
		// remove the host with egressIP address from the egress node list, get the egressNode that does not have egressIP address assigned
		var hostLeft []string
		for i, v := range egressNodes {
			if v == foundHost {
				hostLeft = append(egressNodes[:i], egressNodes[i+1:]...)
				break
			}
		}
		e2e.Logf("\n Get the egressNode that did not have egressIP address previously: %v\n\n\n", hostLeft)

		g.By("8. check if egressIP address is moved to the other egressIP node after original host is rebooted")
		ip, err := getEgressIPByKind(oc, "hostsubnet", hostLeft[0], 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs))

		g.By("9.From the namespace, check source IP is egressIP address")
		sourceIP, err := e2e.RunHostCmd(podns.namespace, podns.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		if !contains(freeIPs, sourceIP) || err != nil {
			err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
				sourceIP, err := e2e.RunHostCmd(podns.namespace, podns.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
				if !contains(freeIPs, sourceIP) || err != nil {
					e2e.Logf("\n got sourceIP as %v while egressIP is %v, or got the error: %v\n, try again", sourceIP, ip, err)
					return false, nil
				}
				return true, nil
			})
		}
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to get sourceIP:%s", err))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.BeElementOf(freeIPs[0]))
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-46559-[Automatic EgressIP] If some egress node is unavailable, pods continue use other available egressIPs after a short delay. [Disruptive][Slow]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet as egressCIDR and an unused ip address from each node that have same subnet, add egressCIDR to each egress node")
		var egressNodes []string
		nodeNum := 4
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < nodeNum {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		// choose first 3 nodes as egress nodes
		for i := 0; i < (nodeNum - 1); i++ {
			egressNodes = append(egressNodes, nodeList.Items[i].Name)
		}
		sub1 := getIfaddrFromNode(nodeList.Items[0].Name, oc)
		sub2 := getIfaddrFromNode(nodeList.Items[1].Name, oc)
		sub3 := getIfaddrFromNode(nodeList.Items[2].Name, oc)
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressCIDRs\":[\""+sub1+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressCIDRs\":[\""+sub2+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressCIDRs\":[\""+sub3+"\"]}")

		// Find 1 unused IPs from each egress node
		freeIPs1 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[0].Name, sub1, 1)
		freeIPs2 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[1].Name, sub2, 1)
		freeIPs3 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[2].Name, sub3, 1)

		g.By("2. Create a namespaces, patch all egressIPs to the namespace, and create a test pod on a non-egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nodeList.Items[nodeNum-1].Name,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[0]+"\", \""+freeIPs2[0]+"\", \""+freeIPs3[0]+"\"]}")

		g.By("3.Check source IP from the test pod for 10 times, it should use any egressIP address as its sourceIP")
		sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs1[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))

		g.By("4. Find the host that is assigned the first egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs1[0])
		e2e.Logf("\n\n\n foundHost that has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		g.By("5. Get the zone info for the host, shutdown the host that has the first egressIP address")
		var instance []string
		var zone string
		switch exutil.CheckPlatform(oc) {
		case "aws":
			e2e.Logf("\n AWS is detected \n")
			defer checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
			defer startInstanceOnAWS(a, nodeList.Items[1].Name)
			stopInstanceOnAWS(a, nodeList.Items[1].Name)
			checkNodeStatus(oc, nodeList.Items[1].Name, "NotReady")
		case "gcp":
			// for gcp, remove the postfix "c.openshift-qe.internal" to get its instance name
			instance = strings.Split(foundHost, ".")
			e2e.Logf("\n\n\n the worker node to be shutdown is: %v\n\n\n", instance[0])
			infraID, err := exutil.GetInfraID(oc)
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("6.From the namespace, check source IP is egressIP address")
		sourceIP, err = execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
		o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs1[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))

		g.By("7. Bring the host back up")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
			startInstanceOnAWS(a, nodeList.Items[1].Name)
			checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-46561-[Manual EgressIP] If some egress node is unavailable, pods continue use other available egressIPs after a short delay. [Disruptive][Slow]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, get subnet and an unused ip address from each node, add egressIP to each egress node")
		var egressNodes []string
		nodeNum := 4
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < nodeNum {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		// choose first 3 nodes as egress nodes
		for i := 0; i < (nodeNum - 1); i++ {
			egressNodes = append(egressNodes, nodeList.Items[i].Name)
		}
		sub1 := getIfaddrFromNode(nodeList.Items[0].Name, oc)
		sub2 := getIfaddrFromNode(nodeList.Items[1].Name, oc)
		sub3 := getIfaddrFromNode(nodeList.Items[2].Name, oc)

		// Find 1 unused IPs from each egress node
		freeIPs1 := findUnUsedIPsOnNodeOrFail(oc, egressNodes[0], sub1, 1)
		freeIPs2 := findUnUsedIPsOnNodeOrFail(oc, egressNodes[1], sub2, 1)
		freeIPs3 := findUnUsedIPsOnNodeOrFail(oc, egressNodes[2], sub3, 1)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressIPs\":[\""+freeIPs1[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[0], "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressIPs\":[\""+freeIPs2[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[1], "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressIPs\":[\""+freeIPs3[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNodes[2], "{\"egressIPs\":[]}")

		g.By("2. Create a namespaces, patch all egressIPs to the namespace, and create a test pod on a non-egress node")
		oc.SetupProject()
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nodeList.Items[nodeNum-1].Name,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[0]+"\", \""+freeIPs2[0]+"\", \""+freeIPs3[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("3.Check source IP from the test pod for 10 times, it should use any egressIP address as its sourceIP")
		sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs1[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))

		g.By("4. Find the host that is assigned the first egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs1[0])
		e2e.Logf("\n\n\n foundHost that has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		g.By("5. Get the zone info for the host, shutdown the host that has the first egressIP address")
		var instance []string
		var zone string
		switch exutil.CheckPlatform(oc) {
		case "aws":
			e2e.Logf("\n AWS is detected \n")
			defer checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
			defer startInstanceOnAWS(a, nodeList.Items[1].Name)
			stopInstanceOnAWS(a, nodeList.Items[1].Name)
			checkNodeStatus(oc, nodeList.Items[1].Name, "NotReady")
		case "gcp":
			// for gcp, remove the postfix "c.openshift-qe.internal" to get its instance name
			instance = strings.Split(foundHost, ".")
			e2e.Logf("\n\n\n the worker node to be shutdown is: %v\n\n\n", instance[0])
			infraID, err := exutil.GetInfraID(oc)
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("6.From the namespace, check source IP is egressIP address")
		sourceIP, err = execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
		o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs1[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs2[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs3[0]))

		g.By("7. Bring the host back up")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
			startInstanceOnAWS(a, nodeList.Items[1].Name)
			checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47455-The egressIP could be assigned to project automatically once it is defined in hostsubnet egressCIDR. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, use the first node as egressNode, get subnet and an unused ip address from the node, apply egressCIDRs to the nod")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		sub := getIfaddrFromNode(nodeList.Items[0].Name, oc)

		// Find 3 unused IPs from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[0].Name, sub, 3)

		patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[0].Name, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[0].Name, "{\"egressCIDRs\":[]}")

		g.By("2. Create a namespaces, patch the first egressIP to the namespace, create a test pod in the namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		pod := pingPodResource{
			name:      "hello-pod1",
			namespace: ns,
			template:  pingPodTemplate,
		}

		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		g.By("3.Check the first egressIP is added to node's primary NIC, verify source IP from this namespace is the first EgressIP")
		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[0], true)

		pod.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod.name, "-n", pod.namespace).Execute()
		waitPodReady(oc, pod.namespace, pod.name)

		var expectedEgressIP = []string{freeIPs[0]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		sourceIP, err := e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))

		g.By("4.Unpatch egressIP to the namespace, Check egressIP is removed from node's primary NIC, verify source IP from this namespace is node's IP address")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[0], false)

		PodNodeName, err := exutil.GetPodNodeName(oc, pod.namespace, pod.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, ns, PodNodeName)
		sourceIP, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(nodeIP))

		g.By("5.Patch the second egressIP to the namespace, verify it is added to node's primary NIC, verify source IP from this namespace is the second EgressIP now")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		expectedEgressIP = []string{freeIPs[1]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[1], true)

		sourceIP, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[1]))

		g.By("6.Patch the third egressIP to the namespace, verify it is added to node's primary NIC, verify source IP from this namespace is the third EgressIP now")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[2]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		expectedEgressIP = []string{freeIPs[2]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		checkPrimaryNIC(oc, nodeList.Items[0].Name, freeIPs[2], true)

		sourceIP, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[2]))

		g.By("7.Patch the namespace with a private IP that is definitely not within the CIDR, verify it is not added to node's primary NIC, curl command should not succeed, error is expected ")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\"192.168.1.100\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		checkPrimaryNIC(oc, nodeList.Items[0].Name, "192.168.1.100", false)

		_, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47456-High-47457-Can change egressIP of project when there are multiple egressIP, can access outside with nodeIP after egressIP is removed. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, get subnets and unused ip addresses from first two nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough nodes for the test, need at least 2 nodes, skip the case!!")
		}

		sub1 := getIfaddrFromNode(nodeList.Items[0].Name, oc)
		sub2 := getIfaddrFromNode(nodeList.Items[1].Name, oc)

		// Find 2 unused IPs from the first egress node and 1 unused IP from the second egress node
		freeIPs1 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[0].Name, sub1, 2)
		freeIPs2 := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[1].Name, sub2, 1)

		g.By("Patch 2 egressIPs to the first egressIP node, patch 1 egressIP to the second egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[0].Name, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[0].Name, "{\"egressIPs\":[\""+freeIPs1[0]+"\", \""+freeIPs1[1]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[1].Name, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[1].Name, "{\"egressIPs\":[\""+freeIPs2[0]+"\"]}")

		g.By("2. Create a namespace, patch the first egressIP from first node to the namespace, create a test pod in the namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		pod := pingPodResource{
			name:      "hello-pod1",
			namespace: ns,
			template:  pingPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[0]+"\"]}")

		g.By("3. Verify source IP from this namespace is the first EgressIP of first node")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		pod.createPingPod(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		var expectedEgressIP = []string{freeIPs1[0], freeIPs1[1]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		sourceIP, err := e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs1[0]))

		g.By("4.Patch the second egressIP of the first node to the namespace, verify source IP from this namespace is the second egressIP from the first node now")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs1[1]+"\"]}")

		expectedEgressIP = []string{freeIPs1[0], freeIPs1[1]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		sourceIP, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs1[1]))

		g.By("5.Patch the egressIP from the second node to the namespace, verify source IP from this namespace is the egressIP from the second node now")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs2[0]+"\"]}")

		expectedEgressIP = []string{freeIPs2[0]}
		checkEgressIPonSDNHost(oc, nodeList.Items[1].Name, expectedEgressIP)

		sourceIP, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs2[0]))

		g.By("6.Unpatch egressIP to the namespace, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")

		PodNodeName, err := exutil.GetPodNodeName(oc, pod.namespace, pod.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, ns, PodNodeName)
		sourceIP, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(nodeIP))

		g.By("7.Unpatch egressIP to the hostsubnet, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[0].Name, "{\"egressIPs\":[]}")

		sourceIP, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(nodeIP))
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47458-High-47459-EgressIP works when reusing the egressIP that was held by a deleted project, EgressIP works well after removed egressIP is added back. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, use the first node as egressIP node, get subnet and 1 unused ip addresses from the node")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		sub := getIfaddrFromNode(nodeList.Items[0].Name, oc)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, nodeList.Items[0].Name, sub, 1)

		g.By("2. Patch the egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[0].Name, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+nodeList.Items[0].Name, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("3. Create first namespace, patch the egressIP to the namespace, create a test pod in the namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Verify source IP from this namespace is the EgressIP")
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		var expectedEgressIP = []string{freeIPs[0]}
		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		sourceIP, CurlErr := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))

		g.By("5.Unpatch egressIP from the first namespace, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")

		PodNodeName, err := exutil.GetPodNodeName(oc, pod1.namespace, pod1.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, ns1, PodNodeName)
		sourceIP, CurlErr = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(nodeIP))

		g.By("6. Create a second namespace, patch the egressIP to the namespace, create a test pod in the namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("7. Verify source IP from the second namespace is the EgressIP that is being reused")
		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		checkEgressIPonSDNHost(oc, nodeList.Items[0].Name, expectedEgressIP)

		sourceIP, CurlErr = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))

		g.By("8.Unpatch egressIP from the second namespace, verify source IP from this namespace is node's IP address where the test pod resides on")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")

		PodNodeName, err = exutil.GetPodNodeName(oc, pod2.namespace, pod2.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeIP = getNodeIPv4(oc, ns2, PodNodeName)
		sourceIP, CurlErr = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(nodeIP))

		g.By("9.Patch the removed egressIP back to the second namespace, verify source IP from this namespace is the egressIP that is added back")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		sourceIP, CurlErr = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47463-Pod will not be affected by the egressIP set on other netnamespace. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, choose first node as egressNode, get subnet and 1 unused ip address from the egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough nodes for the test, need at least 2 nodes, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name
		nonEgressNode := nodeList.Items[1].Name
		sub := getIfaddrFromNode(egressNode, oc)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		g.By("2. Patch the egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("3. Create first namespace, patch the egressIP to the namespace")
		ns1 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Create a second namespace, create two test pods in the second namespace, one on the egressNode, another on a non-egressNode")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns2,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns2,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("5. Curl from the first test pod of the second namespace, verify its sourceIP is its nodeIP address, not the egressIP associated with first namespace")
		nodeIP1 := getNodeIPv4(oc, ns2, egressNode)
		sourceIP, CurlErr := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(nodeIP1))
		o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs[0]))

		g.By("6. Curl from the second test pod of the second namespace, verify its sourceIP is its nodeIP address. not the egressIP associated with first namespace")
		nodeIP2 := getNodeIPv4(oc, ns2, nonEgressNode)
		sourceIP, CurlErr = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(nodeIP2))
		o.Expect(sourceIP).ShouldNot(o.ContainSubstring(freeIPs[0]))
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47464-The egressIP will be unavailable if it is set to multiple hostsubnets. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 := egressNodes[0]
		egressNode2 := egressNodes[1]

		g.By("2. Get subnet from egressIP node, find 1 unused IPs from one egress node")
		sub := getIfaddrFromNode(egressNode1, oc)
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 1)

		g.By("3. Patch the same egressIP to both egress nodes")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Create a namespaces, apply the egressIP to the namespace, and create a test pod in it")
		ns := oc.Namespace()

		pod := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}

		pod.createPingPod(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("5.Verify the egressIP is not added to any egress node's primary NIC")
		checkPrimaryNIC(oc, egressNode1, freeIPs[0], false)
		checkPrimaryNIC(oc, egressNode2, freeIPs[0], false)

		g.By("6. Curl from the test pod should not succeed, error is expected")
		_, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47468-High-47469-Pod access external through egressIP if egress node hosts the egressIP that assigned to netns, or it lose access to external if no node hosts the egressIP that assigned to netns. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, choose first node as egressNode, get subnet and 1 unused ip address from the egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough nodes for the test, need at least 2 nodes, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name
		nonEgressNode := nodeList.Items[1].Name
		sub := getIfaddrFromNode(egressNode, oc)

		// Find 3 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 3)

		g.By("2. Patch first two unused IP above as egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\", \""+freeIPs[1]+"\"]}")

		g.By("3. Create first namespace, patch the first egressIP to the namespace")
		ns1 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Create two test pods in the first namespace, one on the egressNode, another on a non-egressNode")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns1,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("5. Create a second namespace, patch the second egressIP to the namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		g.By("6. Create two test pods in the second namespace, one on the egressNode, another on a non-egressNode")
		pod3 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns2,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod3.createPingPodNode(oc)
		waitPodReady(oc, pod3.namespace, pod3.name)

		pod4 := pingPodResourceNode{
			name:      "hello-pod4",
			namespace: ns2,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod4.createPingPodNode(oc)
		waitPodReady(oc, pod4.namespace, pod4.name)

		g.By("7. Curl from test pods of the first namespace, verify their sourceIP is the egressIP associated with first namespace regardless the test pod is on egressNode or not")
		sourceIP, CurlErr := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Curl from first pod on egressNode, get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))

		sourceIP, CurlErr = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Curl from second pod on Non-egress Node, get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))

		g.By("8. Curl from test pods of the second namespace, verify their sourceIP is the egressIP associated with second namespace regardless the test pod is on egressNode or not")
		sourceIP, CurlErr = e2e.RunHostCmd(pod3.namespace, pod3.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Curl from third pod on egressNode, get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[1]))

		sourceIP, CurlErr = e2e.RunHostCmd(pod4.namespace, pod4.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		e2e.Logf("\n Curl from forth pod on Non-egress Node, get sourceIP as %v \n", sourceIP)
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(sourceIP).Should(o.Equal(freeIPs[1]))

		g.By("9. Create a third namespace, patch the third egressIP to the namespace without it being assigned to an egressNode")
		oc.SetupProject()
		ns3 := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns3, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns3, "{\"egressIPs\":[\""+freeIPs[2]+"\"]}")

		g.By("10. Create two test pods in the third namespace, one on the egressNode, another on a non-egressNode")
		pod5 := pingPodResourceNode{
			name:      "hello-pod5",
			namespace: ns3,
			nodename:  egressNode,
			template:  pingPodNodeTemplate,
		}
		pod5.createPingPodNode(oc)
		waitPodReady(oc, pod5.namespace, pod5.name)

		pod6 := pingPodResourceNode{
			name:      "hello-pod6",
			namespace: ns3,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod6.createPingPodNode(oc)
		waitPodReady(oc, pod6.namespace, pod6.name)

		g.By("11. Curl from test pods of the third namespace, verify they can not access external because there is no node hosts the egressIP, even though it is assigned to third netnamespace, error is expected for curl command")
		sourceIP, CurlErr = e2e.RunHostCmd(pod5.namespace, pod5.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(CurlErr).To(o.HaveOccurred())

		sourceIP, CurlErr = e2e.RunHostCmd(pod6.namespace, pod6.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(CurlErr).To(o.HaveOccurred())
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47055-Should be able to access to the service's externalIP with egressIP [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		externalIPServiceTemplate := filepath.Join(buildPruningBaseDir, "externalip_service1-template.yaml")
		externalIPPodTemplate := filepath.Join(buildPruningBaseDir, "externalip_pod-template.yaml")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, choose first node as egressNode, get 2 unused IP from the node")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}
		egressNode := nodeList.Items[0].Name
		sub := getIfaddrFromNode(egressNode, oc)

		// Find 2 unused IP from the egress node, first one is used as externalIP, second one is used as egressIP
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 2)

		g.By("2. In first namespace, create externalIP service and pod")
		ns1 := oc.Namespace()
		service := externalIPService{
			name:       "service-unsecure",
			namespace:  ns1,
			externalIP: freeIPs[0],
			template:   externalIPServiceTemplate,
		}

		pod1 := externalIPPod{
			name:      "externalip-pod",
			namespace: ns1,
			template:  externalIPPodTemplate,
		}

		defer patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[]}}}}")
		patchResourceAsAdmin(oc, "network/cluster", "{\"spec\":{\"externalIP\":{\"policy\":{\"allowedCIDRs\":[\""+sub+"\"]}}}}")

		defer removeResource(oc, true, true, "service", service.name, "-n", service.namespace)
		parameters := []string{"--ignore-unknown-parameters=true", "-f", service.template, "-p", "NAME=" + service.name, "EXTERNALIP=" + service.externalIP}
		exutil.ApplyNsResourceFromTemplate(oc, service.namespace, parameters...)

		pod1.createExternalIPPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Patch egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		g.By("4. Create a second namespace, patch the first egressIP to the namespace, create a test pod in it")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns2, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("5. Curl the externalIP service from test pod of 2nd namespace")
		curlURL := freeIPs[0] + ":27017"
		output, CurlErr := e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+curlURL+" --connect-timeout 5")
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Hello OpenShift!`))

		g.By("6. Create a third namespace with no egressIP patched, create a test pod in it")
		oc.SetupProject()
		ns3 := oc.Namespace()

		pod3 := pingPodResource{
			name:      "hello-pod3",
			namespace: ns3,
			template:  pingPodTemplate,
		}

		pod3.createPingPod(oc)
		waitPodReady(oc, pod3.namespace, pod3.name)

		g.By("7. Curl the externalIP service from test pod of 3rd namespace")
		output, CurlErr = e2e.RunHostCmd(pod3.namespace, pod3.name, "curl -s "+curlURL+" --connect-timeout 5")
		o.Expect(CurlErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Hello OpenShift!`))
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-47057-NodePort works when configuring an egressIP address [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		nodeServiceTemplate := filepath.Join(buildPruningBaseDir, "nodeservice-template.yaml")

		g.By("1. Get list of worker nodes, choose first node as egressNode, get 1 unused IP from the node")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name
		sub := getIfaddrFromNode(egressNode, oc)
		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		g.By("2. Choose a second worker node, create nodeport service on it")
		nonEgressNode := nodeList.Items[1].Name
		nonEgressNodeHostIP := checkParameter(oc, "default", "hostsubnet", nonEgressNode, "-o=jsonpath={.hostIP}")
		e2e.Logf("nonEgressNodeHostIP is: %v ", nonEgressNodeHostIP)

		g.By("3. create a namespace, create nodeport service on the nonEgress node")
		ns1 := oc.Namespace()
		service := nodePortService{
			name:      "hello-pod",
			namespace: ns1,
			nodeName:  nonEgressNode,
			template:  nodeServiceTemplate,
		}

		defer removeResource(oc, true, true, "service", service.name, "-n", service.namespace)
		parameters := []string{"--ignore-unknown-parameters=true", "-f", service.template, "-p", "NAME=" + service.name, "NODENAME=" + service.nodeName}
		exutil.ApplyNsResourceFromTemplate(oc, service.namespace, parameters...)
		waitPodReady(oc, ns1, "hello-pod")

		g.By("4. Patch egressIP to the egressIP node and the namespace")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns1, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("5. Access the nodeport service from a master node")
		result := checkParameter(oc, ns1, "service", "hello-pod", "-o=jsonpath={.spec.type}")

		// get first master node that is available
		firstMasterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		e2e.Logf("First master node is: %v ", firstMasterNode)

		if strings.Contains(result, "NodePort") {
			curlURL := nonEgressNodeHostIP + ":30012"
			curlCommand := "curl -s " + curlURL + " --connect-timeout 5"
			curlErr := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
				output, err := exutil.DebugNodeWithChroot(oc, firstMasterNode, "bash", "-c", curlCommand)
				if !strings.Contains(output, "Hello OpenShift!") || err != nil {
					e2e.Logf("\n Output when accesing the nodeport service: ---->%v<----, or got the error: %v\n", output, err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(curlErr, fmt.Sprintf("Failed to access nodeport service:%s", curlErr))

		} else {
			g.Fail("Did not find NodePort service")
		}
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-High-47462-EgressNetworkPolicy should work well with egressIP [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressPolicyTemplate := filepath.Join(buildPruningBaseDir, "egress-limit-policy-template.yaml")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")

		g.By("1. Get list of nodes, choose first node as egressNode, get 1 unused IP from the node that will be used as egressIP")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name
		sub := getIfaddrFromNode(egressNode, oc)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		// set the cidrSelector for the network policy to be the internalIP of the int-svc instance
		internalIP := strings.Split(ipEchoURL, ":")[0]
		cidrSelector := internalIP + "/32"
		e2e.Logf("\n cideSelector: %v \n", cidrSelector)

		g.By("2. create an egress network policy to deny ipecho service's IP which is the internalIP of the int-svc")
		ns := oc.Namespace()
		policy := egressPolicy{
			name:         "policy1",
			namespace:    ns,
			cidrSelector: cidrSelector,
			template:     egressPolicyTemplate,
		}

		defer removeResource(oc, true, true, "egressnetworkpolicy", policy.name, "-n", policy.namespace)
		parameters := []string{"--ignore-unknown-parameters=true", "-f", policy.template, "-p", "NAME=" + policy.name, "CIDRSELECTOR=" + policy.cidrSelector}
		exutil.ApplyNsResourceFromTemplate(oc, policy.namespace, parameters...)

		//check if deny policy is in place
		result := checkParameter(oc, ns, "egressnetworkpolicy", policy.name, "-o=jsonpath={.spec.egress[0].type}")
		if !strings.Contains(result, "Deny") {
			g.Fail("No deny network policy is in place as expected, fail the test now")
		}

		g.By("3. Patch egressIP to the egressIP node and the namespace")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		pod := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}

		pod.createPingPod(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		g.By("4. Curl from the test pod should not succeed, error is expected as there is deny network policy in place")
		_, err = e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("5. Patch change the network policy to allow the ipecho service IP address (which is internalIP of the int-svc)")
		change := "[{\"op\":\"replace\", \"path\":\"/spec/egress/0/type\", \"value\":\"Allow\"}]"
		patchReplaceResourceAsAdmin(oc, oc.Namespace(), "egressnetworkpolicy", policy.name, change)

		g.By("6. Curl external after network policy is changed to allow the IP, verify sourceIP is the egressIP")
		result = checkParameter(oc, ns, "egressnetworkpolicy", policy.name, "-o=jsonpath={.spec.egress[0].type}")
		if strings.Contains(result, "Allow") {
			sourceIP, CurlErr := e2e.RunHostCmd(pod.namespace, pod.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			e2e.Logf("\n Get sourceIP as %v \n", sourceIP)
			o.Expect(CurlErr).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))
		} else {
			g.Fail("Network policy was not changed to allow the ip")
		}
	})

	// author: jechen@redhat.com
	g.It("ConnectedOnly-Author:jechen-Medium-47461-Should not be able to access the node via the egressIP [Disruptive]", func() {

		g.By("1. Get list of nodes, choose first node as egressNode, get 1 unused IP from the node that will be used as egressIP")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough node available, need at least one node for the test, skip the case!!")
		}

		g.By("2. Find 1 unused IP from the egressNode as egressIP, patch egressIP to the egressIP node and the namespace")
		ns := oc.Namespace()
		egressNode := nodeList.Items[0].Name

		// get subnet and unused IP from the egressNode
		sub := getIfaddrFromNode(egressNode, oc)
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("2. timeout test from the int-svc instance to egressNode through egressIP")
		nodeIP := getNodeIPv4(oc, ns, egressNode)
		e2e.Logf("\n\n NodeIP: -->%v<--, or error: %v \n\n", nodeIP)
		switch exutil.CheckPlatform(oc) {
		case "aws":
			// timeout test to egressNode through nodeIP is expected to succeed
			result, timeoutTestErr := accessEgressNodeFromIntSvcInstanceOnAWS(a, oc, nodeIP)
			e2e.Logf("\n\n timeout test ssh connection to node through nodeIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
			o.Expect(timeoutTestErr).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.Equal("0"))

			// timeout test to egressNode through egressIP is expected to fail
			result, timeoutTestErr = accessEgressNodeFromIntSvcInstanceOnAWS(a, oc, freeIPs[0])
			e2e.Logf("\n\n timeout test ssh connection to node through egressIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
			o.Expect(timeoutTestErr).To(o.HaveOccurred())
			o.Expect(result).NotTo(o.Equal("0"))
		case "gcp":
			infraID, err := exutil.GetInfraID(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			host, err := getIntSvcExternalIPFromGcp(oc, infraID)
			o.Expect(err).NotTo(o.HaveOccurred())
			// timeout test to egressNode through nodeIP is expected to succeed
			result, timeoutTestErr := accessEgressNodeFromIntSvcInstanceOnGCP(host, nodeIP)
			e2e.Logf("\n\n timeout test ssh connection to node through nodeIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
			o.Expect(timeoutTestErr).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.Equal("0"))

			// timeout test to egressNode through egressIP is expected to fail
			result, timeoutTestErr = accessEgressNodeFromIntSvcInstanceOnGCP(host, freeIPs[0])
			e2e.Logf("\n\n timeout test ssh connection to node through egressIP, got result as -->%v<--, or error: %v \n\n", result, timeoutTestErr)
			o.Expect(timeoutTestErr).To(o.HaveOccurred())
			o.Expect(result).NotTo(o.Equal("0"))
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-46960- EgressIP can failover if the node is NotReady. [Disruptive][Slow]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		g.By("1. Get list of nodes, find two nodes that have same subnet, use them as egressNodes, use the subnet as egressCIDR to be assigned to egressNodes")
		var egressNode1, egressNode2, nonEgressNode string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Find two nodes with same subnet as egressNodes, total number of nodes needs to be at least 3, as test pod will be created on the third non-egressNode
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		// Find the non-egressNode
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != egressNode1 && nodeList.Items[i].Name != egressNode2 {
				nonEgressNode = nodeList.Items[i].Name
				break
			}
		}
		if nonEgressNode == "" {
			g.Skip("Did not get a node that is not egressIP node, skip the case!!")
		}

		e2e.Logf("\nEgressNode1: %v\n", egressNode1)
		e2e.Logf("\nEgressNode2: %v\n", egressNode2)
		e2e.Logf("\nnonEgressNode: %v\n", nonEgressNode)

		sub := getIfaddrFromNode(egressNode1, oc)

		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode1, "{\"egressCIDRs\":[\""+sub+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode2, "{\"egressCIDRs\":[\""+sub+"\"]}")

		// Find 1 unused IPs to be used as egressIP
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode1, sub, 1)

		g.By("2. Create a namespaces, patch egressIP to the namespace, create a test pod on a non-egress node")
		ns := oc.Namespace()
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("3.Check source IP from the test pod for 10 times before failover, it should use the egressIP address as its sourceIP")
		sourceIP, err := execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n Before failover, get sourceIP as: %v\n", sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))

		g.By("4. Find the egressNode that is initially assigned the egressIP address")
		foundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs[0])
		e2e.Logf("\n\n\n foundHost that initially has the egressIP: %v\n\n\n", foundHost)
		o.Expect(foundHost).NotTo(o.BeEmpty())

		g.By("5. Get the zone info for the host, shutdown the host that has the egressIP address to cause failover")
		var instance []string
		var zone string
		switch exutil.CheckPlatform(oc) {
		case "aws":
			e2e.Logf("\n AWS is detected \n")
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnAWS(a, foundHost)
			stopInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "NotReady")
		case "gcp":
			// for gcp, remove the postfix "c.openshift-qe.internal" to get its instance name
			instance = strings.Split(foundHost, ".")
			e2e.Logf("\n\n\n the worker node to be shutdown is: %v\n\n\n", instance[0])
			infraID, err := exutil.GetInfraID(oc)
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, foundHost, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("6.After first egressNode becomes NotReady, from the namespace, check source IP is still egressIP address")
		sourceIP, err = execCommandInSpecificPod(oc, podns.namespace, podns.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n Get sourceIP as: %v\n", sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))

		g.By("7. Find the new host that hosts the egressIP now")
		newFoundHost := SDNHostwEgressIP(oc, egressNodes, freeIPs[0])
		e2e.Logf("\n\n\n The new foundHost that has the egressIP: %v\n\n\n", newFoundHost)
		o.Expect(newFoundHost).Should(o.BeElementOf(egressNodes))
		o.Expect(newFoundHost).ShouldNot(o.Equal(foundHost))

		g.By("8. Bring the host back up")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, foundHost, "Ready")
			startInstanceOnAWS(a, foundHost)
			checkNodeStatus(oc, foundHost, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, foundHost, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, foundHost, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

	})
})

var _ = g.Describe("[sig-networking] SDN EgressIPs Basic", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp")
		if !acceptedPlatform || !strings.Contains(networkType, "sdn") {
			g.Skip("Test cases should be run on AWS or GCP cluster with Openshift-SDN network plugin, skip for other platforms or other network plugin!!")
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-Low-47460-Invalid egressIP should not be acceptable", func() {

		g.By("1. Get list of nodes, use the first node as egressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2. Patch invalid egressIP or invalid egressCIDRs to the egressIP node")
		output, patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"a.b.c.d\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"fe80::5054:ff:fedd:3698\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"256.256.256.256\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"test-value\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressIPs\":[\"10.10.10.-1\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressCIDRs\":[\"10.0.0.1/64\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressCIDRs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostsubnet/"+nodeList.Items[0].Name, "-p", "{\"egressCIDRs\":[\"10.1.1/24\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressCIDRs"))

		g.By("3. Create a namespace, patch invalid egressIP to the namespace, they should not be accepted")
		ns := oc.Namespace()

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"a.b.c.d\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"fe80::5054:ff:fedd:3698\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"256.256.256.256\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"test-value\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))

		output, patchErr = oc.AsAdmin().WithoutNamespace().Run("patch").Args("netnamespace/"+ns, "-p", "{\"egressIPs\":[\"10.10.10.-1\"]}", "--type=merge").Output()
		o.Expect(patchErr).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("invalid: egressIPs"))
	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47466-High-47467-Related iptables/openflow and egressIP to node's primary NIC will be added/removed once egressIP is added/removed to/from netnamespace. [Disruptive]", func() {

		g.By("1. Get list of nodes, choose first node as egressNode, get subnet and 1 unused ip address from the egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		egressNode := nodeList.Items[0].Name
		sub := getIfaddrFromNode(egressNode, oc)

		// Find 1 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 1)

		g.By("2. Patch the egressIP to the egressIP node")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("3. Create first namespace, patch the egressIP to the namespace")
		ns := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		g.By("4. Verify that iptable rule is added on egressNode")
		IPtableCmd := "iptables-save  |grep " + freeIPs[0]
		CmdOutput, CmdErr := execCommandInSDNPodOnNode(oc, egressNode, IPtableCmd)
		o.Expect(CmdErr).NotTo(o.HaveOccurred())
		o.Expect(CmdOutput).Should(
			o.And(
				o.ContainSubstring("OPENSHIFT-MASQUERADE"),
				o.ContainSubstring("OPENSHIFT-FIREWALL-ALLOW")))

		g.By("5. Verify that openflow rule is added on egressNode")
		OpenflowCmd := "ovs-ofctl dump-flows br0 -O OpenFlow13  |grep table=101"
		CmdOutput, CmdErr = execCommandInSDNPodOnNode(oc, egressNode, OpenflowCmd)
		o.Expect(CmdErr).NotTo(o.HaveOccurred())
		o.Expect(CmdOutput).Should(o.ContainSubstring("reg0=0x"))

		g.By("6. Verify that the egressIP is added to egressNode's primary NIC")
		checkPrimaryNIC(oc, egressNode, freeIPs[0], true)

		g.By("7.Unpatch egressIP to the namespace, verify iptable rule is removed from egressNode")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		CmdOutput, CmdErr = execCommandInSDNPodOnNode(oc, egressNode, IPtableCmd)
		o.Expect(CmdErr).To(o.HaveOccurred())

		g.By("8.Verify openflow rule is removed from egressNode")
		CmdOutput, CmdErr = execCommandInSDNPodOnNode(oc, egressNode, OpenflowCmd)
		o.Expect(CmdErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n Get CmdOutput as %v \n", CmdOutput)
		o.Expect(CmdOutput).ShouldNot(o.ContainSubstring("reg0=0x"))

		g.By("9.Verify egressIP is also removed from egressNode's primary NIC")
		checkPrimaryNIC(oc, egressNode, freeIPs[0], false)
	})

	g.It("NonPreRelease-ConnectedOnly-Author:jechen-Medium-47472-Meduim-47473-Cluster admin can add/remove egressIPs on netnamespace and hostsubnet. [Disruptive]", func() {

		g.By("1. Get list of nodes, use the first node as egressIP node")
		nodeList, getNodeErr := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())

		egressNode := nodeList.Items[0].Name
		sub := getIfaddrFromNode(egressNode, oc)

		// Find 2 unused IP from the egress node
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, 2)

		g.By("2. Patch the egressIP to the egressIP node, verify the egressIP is added to the hostsubnet, use oc describe command to verify egressIP for hostsubnet as well")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		ip, err := getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[0]))

		ipReturned := describeCheckEgressIPByKind(oc, "hostsubnet", egressNode)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[0]))

		g.By("3. Create first namespace, patch the egressIP to the namespace, verify the egressIP is added to the netnamespace, use oc describe command to verify egressIP for netnamespace as well")
		ns := oc.Namespace()

		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[0]+"\"]}")

		ip, err = getEgressIPByKind(oc, "netnamespace", ns, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[0]))

		ipReturned = describeCheckEgressIPByKind(oc, "netnamespace", ns)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[0]))

		g.By("4. Patch a new egressIP to hostsubnet, verify hostsubnet is updated with new egressIP, use oc describe command to verify egressIP for hostsubnet as well")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		ip, err = getEgressIPByKind(oc, "hostsubnet", egressNode, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[1]))

		ipReturned = describeCheckEgressIPByKind(oc, "hostsubnet", egressNode)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[1]))

		g.By("5. Patch a new egressIP to namespace, verify namespace is updated with new egressIP, use oc describe command to verify egressIP for netnamespace as well")
		defer patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[\""+freeIPs[1]+"\"]}")

		ip, err = getEgressIPByKind(oc, "netnamespace", ns, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ip[0]).Should(o.BeElementOf(freeIPs[1]))

		ipReturned = describeCheckEgressIPByKind(oc, "netnamespace", ns)
		o.Expect(ipReturned).Should(o.BeElementOf(freeIPs[1]))

		g.By("6. Unpatch egressIP from hostsubnet, verify egressIP is removed from hostsubnet, use oc describe command to verify that for hostsubnet as well")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressIPs\":[]}")
		ip, err = getEgressIPByKind(oc, "hostsubnet", egressNode, 0)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(ip) == 0).Should(o.BeTrue())

		ipReturned = describeCheckEgressIPByKind(oc, "hostsubnet", egressNode)
		o.Expect(ipReturned == "<none>").Should(o.BeTrue())

		g.By("7. Unpatch egressIP from hostsubnet, verify egressIP is removed from hostsubnet, , use oc describe command to verify that for hostsubnet as well")
		patchResourceAsAdmin(oc, "netnamespace/"+ns, "{\"egressIPs\":[]}")
		ip, err = getEgressIPByKind(oc, "netnamespace", ns, 0)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(ip) == 0).Should(o.BeTrue())

		ipReturned = describeCheckEgressIPByKind(oc, "netnamespace", ns)
		o.Expect(ipReturned == "<none>").Should(o.BeTrue())
	})

	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-47570-EgressIP capacity test. [Disruptive]", func() {
		g.By("1. Get list of nodes, use the first node as egressIP node, patch egressCIDRs to the egressNode")
		nodeList, getNodeErr := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Not enough nodes for the test, need at least 1 nodes, skip the case!!")
		}

		egressNode := nodeList.Items[0].Name
		sub := getIfaddrFromNode(egressNode, oc)

		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")
		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub+"\"]}")

		g.By("2 Get IP capacity of the node, find exceedNum (ipCap+1) number of unused IPs from the egress node. \n")
		ipCapacity := getIPv4Capacity(oc, egressNode)
		o.Expect(ipCapacity != "").Should(o.BeTrue())
		ipCap, _ := strconv.Atoi(ipCapacity)
		e2e.Logf("\n The egressIP capacity for this cloud provider is found as %v \n", ipCap)
		if ipCap > 14 {
			g.Skip("This is not the general IP capacity, will skip it.")
		}

		exceedNum := ipCap + 1
		freeIPs := findUnUsedIPsOnNodeOrFail(oc, egressNode, sub, exceedNum)

		g.By("3. Create up to exceedNum number of namespaces, patch one egressIP to each namespace.")
		var ns = make([]string, exceedNum)
		for i := 0; i < exceedNum; i++ {
			oc.SetupProject()
			ns[i] = oc.Namespace()
			defer patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[]}")
			patchResourceAsAdmin(oc, "netnamespace/"+ns[i], "{\"egressIPs\":[\""+freeIPs[i]+"\"]}")
		}

		g.By("4. Verify only ipCap number of egressIP are allowed to the hostsubnet.")
		ipReturned, checkErr := getEgressIPByKind(oc, "hostsubnet", nodeList.Items[0].Name, ipCap)
		o.Expect(checkErr).NotTo(o.HaveOccurred())
		o.Expect(len(ipReturned) == ipCap).Should(o.BeTrue())

		// Note: Due to bug with JIRA ticket OCPBUGS-69, currently, we only check if the number of egressIP assigned to hostsubnet is equal to capacity limit, it does not exceeds capacity limit.
		// Will add a check to verify event log for warning message after OCPBUGS-69 is fixed

	})

})
