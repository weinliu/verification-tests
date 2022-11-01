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
		ipEchoURL       string
		a               *exutil.AwsClient
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		flag            string

		oc = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS/GCP/Openstack/Vsphere/BareMetal cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}

		switch platform {
		case "aws":
			e2e.Logf("\n AWS is detected, running the case on AWS\n")
			if ipEchoURL == "" {
				getAwsCredentialFromCluster(oc)
				a = exutil.InitAwsSession()
				_, err := getAwsIntSvcInstanceID(a, oc)
				if err != nil {
					flag = "tcpdump"
					e2e.Logf("There is no int svc instance in this cluster: %v, try tcpdump way", err)
				} else {
					ipEchoURL, err = installIPEchoServiceOnAWS(a, oc)
					if ipEchoURL != "" && err == nil {
						flag = "ipecho"
						e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
					} else {
						flag = "tcpdump"
						e2e.Logf("No ip-echo service installed on the bastion host, change to use tcpdump way %v", err)
					}
				}
			}
		case "gcp":
			e2e.Logf("\n GCP is detected, running the case on GCP\n")
			if ipEchoURL == "" {
				// If an int-svc instance with external IP found, IpEcho service will be installed on the int-svc instance
				// otherwise, use tcpdump to verify egressIP
				infraID, err := exutil.GetInfraID(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				host, err := getIntSvcExternalIPFromGcp(oc, infraID)
				if host == "" || err != nil {
					flag = "tcpdump"
					e2e.Logf("There is no int svc instance in this cluster: %v, try tcpdump way", err)
				} else {
					ipEchoURL, err = installIPEchoServiceOnGCP(oc, infraID, host)
					if ipEchoURL != "" && err == nil {
						flag = "ipecho"
						e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
					} else {
						e2e.Logf("No ip-echo service installed on the bastion host, %v, change to use tcpdump to verify", err)
						flag = "tcpdump"
					}
				}
			}
		case "openstack":
			e2e.Logf("\n OpenStack is detected, running the case on OpenStack\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on OpenStack")
		case "vsphere":
			e2e.Logf("\n Vsphere is detected, running the case on Vsphere\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on Vsphere")
		case "baremetal":
			e2e.Logf("\n BareMetal is detected, running the case on BareMetal\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on BareMetal")
		default:
			e2e.Logf("Not support cloud provider for  egressip cases for now.")
			g.Skip("Not support cloud provider for  egressip cases for now.")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47272-Pods will not be affected by the egressIP set on other netnamespace. [Serial]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		g.By("1.1 Label EgressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		g.By("1.2 Apply EgressLabel Key to one node.")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)

		g.By("2.1 Create first egressip object")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:          "egressip-47272-1",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		g.By("2.2 Create second egressip object")
		egressip2 := egressIPResource1{
			name:          "egressip-47272-2",
			template:      egressIP2Template,
			egressIP1:     freeIPs[1],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "blue",
		}
		egressip2.createEgressIPObject2(oc)
		defer egressip2.deleteEgressIPObject1(oc)
		egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip2.name)
		o.Expect(len(egressIPMaps2)).Should(o.Equal(1))

		g.By("3.1 create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("3.2 Apply a label to first namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("3.3 Create a pod in first namespace. ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3.4 Apply label to pod in first namespace")
		err = exutil.LabelPod(oc, ns1, pod1.name, "color=pink")
		defer exutil.LabelPod(oc, ns1, pod1.name, "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4.1 create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("4.2 Apply a label to second namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4.3 Create a pod in second namespace ")
		pod2 := pingPodResource{
			name:      "hello-pod",
			namespace: ns2,
			template:  pingPodTemplate,
		}
		pod2.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod2.name, "-n", pod2.namespace).Execute()
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("4.4 Apply label to pod in second namespace")
		err = exutil.LabelPod(oc, ns2, pod2.name, "color=blue")
		defer exutil.LabelPod(oc, ns2, pod2.name, "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("5.1 Check source IP in first namespace using first egressip object")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			sourceIP, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))

			g.By("5.2 Check source IP in second namespace using second egressip object")
			sourceIP, err = e2e.RunHostCmd(pod2.namespace, pod2.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(freeIPs[1]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47272", ns2)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47272", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			g.By("5.2 Check source IP in second namespace using second egressip object")
			egressErr2 := verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, freeIPs[1], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr2).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get expected egressip:%s", freeIPs[1]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		g.By("Pods will not be affected by the egressIP set on other netnamespace.!!! ")
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47164-Medium-47025-Be able to update egressip object,The pods removed matched labels will not use EgressIP [Serial]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		g.By("1.1 Label EgressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		g.By("1.2 Apply EgressLabel Key to one node.")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)

		g.By("2.1 Create first egressip object")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:          "egressip-47164",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		g.By("3.1 create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("3.2 Apply a label to first namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("3.3 Create a pod in first namespace. ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3.4 Apply label to pod in first namespace")
		err = exutil.LabelPod(oc, ns1, pod1.name, "color=pink")
		defer exutil.LabelPod(oc, ns1, pod1.name, "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4. Update the egressip in egressip object")
		updateEgressIPObject(oc, egressip1.name, freeIPs[1])

		g.By("5. Check source IP is updated IP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			sourceIP, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(freeIPs[1]))

			g.By("6. Remove labels from test pod.")
			err = exutil.LabelPod(oc, ns1, pod1.name, "color-")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("7. Check source IP is not EgressIP")
			sourceIP, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).ShouldNot(o.Equal(freeIPs[1]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47164", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47164", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[1], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get expected egressip:%s", freeIPs[1]))

			g.By("6. Remove labels from test pod.")
			err = exutil.LabelPod(oc, ns1, pod1.name, "color-")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("7. Check source IP is not EgressIP")
			egressErr = verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[1], dstHost, ns1, tcpdumpDS.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Should not get egressip:%s", freeIPs[1]))

		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47030-An EgressIP object can not have multiple egress IP assignments on the same node. [Serial]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("1. Label EgressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("2. Apply EgressLabel Key for this test on one node.")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)

		g.By("3. Create an egressip object")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47030",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("4. Check only one EgressIP assigned in the object.")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47028-After remove EgressIP node tag, EgressIP will failover to other availabel egress nodes. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet \n")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		g.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)

		g.By("3.1 Create new namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()
		g.By("3.2 Apply label to namespace\n")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Output()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4. Create a pod in first namespace. \n")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("5. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47028",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("4. Check EgressIP assigned in the object.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		g.By("5. Update Egress node to egressNode2.\n")
		e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)

		g.By("6. Check the egress node was updated in the egressip object.\n")
		egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			egressIPMaps = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps) != 1 || egressIPMaps[0]["node"] == egressNode1 {
				e2e.Logf("Wait for new egress node applied,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps[0]["node"]).Should(o.ContainSubstring(egressNode2))

		g.By("7. Check the source ip.\n")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			sourceIP, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(egressIPMaps[0]["egressIP"]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode2)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47028", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47028", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Longduration-NonPreRelease-High-47031-After reboot egress node EgressIP still work.  [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		g.By("1.1 Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		g.By("1.2 Apply EgressLabel Key to one node.\n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		g.By("2.1 Create first egressip object\n")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-47031",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("3.1 create first namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("3.2 Apply a label to test namespace.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("3.3 Create pods in test namespace. \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("3.4 Apply label to one pod in test namespace\n")
		testPodName := getPodName(oc, ns1, "name=test-pods")
		err = exutil.LabelPod(oc, ns1, testPodName[0], "color=pink")
		defer exutil.LabelPod(oc, ns1, testPodName[0], "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4. Check only one EgressIP assigned in the object.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		g.By("5.Reboot egress node.\n")
		defer checkNodeStatus(oc, egressNode, "Ready")
		rebootNode(oc, egressNode)
		checkNodeStatus(oc, egressNode, "NotReady")
		checkNodeStatus(oc, egressNode, "Ready")
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("7. Check EgressIP assigned in the object.\n")
		egressipObjErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps2) == 1 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(egressipObjErr, fmt.Sprintf("Failed to apply egressIPs:%s", egressipObjErr))

		g.By("8. Check source IP is egressIP \n")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf(" ipEchoURL is %v", ipEchoURL)
			sourceIP, err := e2e.RunHostCmd(ns1, testPodName[0], "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(freeIPs[0]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47031", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47031", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			g.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, testPodName[0], ns1, freeIPs[0], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Longduration-NonPreRelease-Critical-47032-High-47034-Traffic is load balanced between egress nodes,multiple EgressIP objects can have multiple egress IPs.[Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("create new namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		g.By("Apply EgressLabel Key for this test on one node.\n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)

		g.By("Apply label to namespace\n")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an egressip object\n")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 4)
		o.Expect(len(freeIPs)).Should(o.Equal(4))
		egressip1 := egressIPResource1{
			name:      "egressip-47032",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("Create another egressip object\n")

		egressip2 := egressIPResource1{
			name:      "egressip-47034",
			template:  egressIPTemplate,
			egressIP1: freeIPs[2],
			egressIP2: freeIPs[3],
		}
		egressip2.createEgressIPObject1(oc)
		defer egressip2.deleteEgressIPObject1(oc)
		//Update label in egressip2 object to a different one from egressip1
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-47034", "-p", "{\"spec\":{\"namespaceSelector\":{\"matchLabels\":{\"name\":\"qe\"}}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("Create sencond namespace.")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("Apply label to second namespace\n")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a pod in second namespace")
		pod2 := pingPodResource{
			name:      "hello-pod",
			namespace: ns2,
			template:  pingPodTemplate,
		}
		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("Check source IP is randomly one of egress ips.\n")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			sourceIP, err := execCommandInSpecificPod(oc, pod2.namespace, pod2.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[2]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[3]))
			sourceIP, err = execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[1]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump", "true")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNodes[0])
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47032", ns2)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47032", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("Check source IP is randomly one of egress ips for both namespaces.")
			egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, err := execCommandInSpecificPod(oc, pod2.namespace, pod2.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				if checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[2], true) != nil || checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[3], true) != nil || err != nil {
					e2e.Logf("No matched egressIPs in tcpdump log, try next round.")
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get both EgressIPs %s,%s in tcpdump", freeIPs[2], freeIPs[3]))

			egressipErr2 := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				if checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[0], true) != nil || checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[1], true) != nil || err != nil {
					e2e.Logf("No matched egressIPs in tcpdump log, try next round.")
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr2, fmt.Sprintf("Failed to get both EgressIPs %s,%s in tcpdump", freeIPs[0], freeIPs[1]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-47019-High-47023-EgressIP works well with networkpolicy and egressFirewall. [Serial]", func() {
		//EgressFirewall case cannot run in proxy cluster, skip if proxy cluster.
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		networkPolicyFile := filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall2-template.yaml")

		g.By("1. Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		egressNode := nodeList.Items[0].Name
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		g.By("3. create new namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("4. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()

		g.By("5. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47019",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		g.By("6. Create test pods \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = waitForPodWithLabelReady(oc, oc.Namespace(), "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("7. Create default deny ingress type networkpolicy in test namespace\n")
		createResourceFromFile(oc, ns1, networkPolicyFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-deny-ingress"))

		g.By("8. Create an EgressFirewall object with rule deny.")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns1,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)
		defer egressFW2.deleteEgressFW2Object(oc)

		g.By("9. Get test pods IP and test pod name in test namespace\n")
		testPodName := getPodName(oc, oc.Namespace(), "name=test-pods")

		g.By("10. Check network policy works. \n")
		CurlPod2PodFail(oc, ns1, testPodName[0], ns1, testPodName[1])

		g.By("11. Check EgressFirewall policy works. \n")
		_, err = e2e.RunHostCmd(ns1, testPodName[0], "curl -s ifconfig.me --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("12.Update EgressFirewall to allow")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressfirewall.k8s.ovn.org/default", "-n", ns1, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Allow\",\"to\":{\"cidrSelector\":\"0.0.0.0/0\"}}]}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By("13. Check EgressFirewall Allow rule works and EgressIP works.\n")
			egressipErr := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
				sourceIP, err := e2e.RunHostCmd(ns1, testPodName[0], "curl -s "+ipEchoURL+" --connect-timeout 5")
				if err != nil {
					e2e.Logf("Wait for EgressFirewall taking effect. %v", err)
					return false, nil
				}
				if !contains(freeIPs, sourceIP) {
					eip, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip", "-o=jsonpath={.}").Output()
					e2e.Logf(eip)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("The source Ip is not same as the egressIP expected!"))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47023", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47023", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("13. Verify from tcpdump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, testPodName[0], ns1, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47018-Medium-47017-Multiple projects use same EgressIP,EgressIP works for all pods in the namespace with matched namespaceSelector. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("1. Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		egressNode := nodeList.Items[0].Name
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		g.By("3. create first namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("4. Create test pods in first namespace. \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNs1Name := getPodName(oc, ns1, "name=test-pods")

		g.By("5. Apply label to ns1 namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()

		g.By("6. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47018",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		g.By("7. create new namespace\n")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("8. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name-").Execute()

		g.By("9. Create test pods in second namespace  \n")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNs2Name := getPodName(oc, ns2, "name=test-pods")

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By("10. Check source IP from both namespace, should be egressip.  \n")
			sourceIP, err := e2e.RunHostCmd(ns1, testPodNs1Name[0], "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.BeElementOf(freeIPs))
			sourceIP, err = e2e.RunHostCmd(ns1, testPodNs1Name[1], "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.BeElementOf(freeIPs))
			sourceIP, err = e2e.RunHostCmd(ns2, testPodNs2Name[0], "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.BeElementOf(freeIPs))
			sourceIP, err = e2e.RunHostCmd(ns2, testPodNs2Name[1], "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.BeElementOf(freeIPs))

			g.By("11. Remove matched labels from namespace ns1  \n")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("12.  Check source IP from namespace ns1, should not be egressip. \n")
			sourceIP, err = e2e.RunHostCmd(ns1, testPodNs1Name[0], "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).ShouldNot(o.BeElementOf(freeIPs))
			sourceIP, err = e2e.RunHostCmd(ns1, testPodNs1Name[1], "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).ShouldNot(o.BeElementOf(freeIPs))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47017", ns2)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47017", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("10.Check source IP from both namespace, should be egressip. ")
			egressErr := verifyEgressIPinTCPDump(oc, testPodNs1Name[0], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs1Name[1], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs2Name[0], ns2, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs2Name[0], ns2, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

			g.By("11. Remove matched labels from namespace ns1  \n")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("12.  Check source IP from namespace ns1, should not be egressip. \n")
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs1Name[0], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs1Name[1], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Longduration-NonPreRelease-Medium-47033-If an egress node is NotReady traffic is still load balanced between available egress nodes. [Disruptive]", func() {
		platform := exutil.CheckPlatform(oc)
		e2e.Logf("\n\nThe platform is %v,\n", platform)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp")
		if !acceptedPlatform || ipEchoURL == "" {
			g.Skip("Test cases should be run on AWS/GCP with int-svc host, skip for other platforms!!")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("1. create new namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("2. Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 3 {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		g.By("3. Apply EgressLabel Key for this test on 3 nodes.\n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[2].Name, egressNodeLabel)

		g.By("4. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()

		g.By("5. Create an egressip object\n")
		sub1 := getIfaddrFromNode(nodeList.Items[0].Name, oc)
		freeIP1 := findUnUsedIPsOnNode(oc, nodeList.Items[0].Name, sub1, 1)
		o.Expect(len(freeIP1) == 1).Should(o.BeTrue())
		sub2 := getIfaddrFromNode(nodeList.Items[1].Name, oc)
		freeIP2 := findUnUsedIPsOnNode(oc, nodeList.Items[1].Name, sub2, 1)
		o.Expect(len(freeIP2) == 1).Should(o.BeTrue())
		sub3 := getIfaddrFromNode(nodeList.Items[2].Name, oc)
		freeIP3 := findUnUsedIPsOnNode(oc, nodeList.Items[2].Name, sub3, 1)
		o.Expect(len(freeIP3) == 1).Should(o.BeTrue())

		egressip1 := egressIPResource1{
			name:      "egressip-47033",
			template:  egressIPTemplate,
			egressIP1: freeIP1[0],
			egressIP2: freeIP2[0],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("6. Update an egressip object with three egressips.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-47033", "-p", "{\"spec\":{\"egressIPs\":[\""+freeIP1[0]+"\",\""+freeIP2[0]+"\",\""+freeIP3[0]+"\"]}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("7. Create a pod \n")
		pod1 := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("8. Check source IP is randomly one of egress ips.\n")
		e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
		egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps2) == 3).Should(o.BeTrue())
		sourceIP, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..15}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIP1[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIP2[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIP3[0]))

		g.By("9. Stop one egress node.\n")
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
			instance = strings.Split(nodeList.Items[1].Name, ".")
			e2e.Logf("\n\n\n the worker node to be shutdown is: %v\n\n\n", instance[0])
			infraID, err := exutil.GetInfraID(oc)
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeList.Items[1].Name, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("10. Check EgressIP updated in EIP object, sourceIP contains 2 IPs. \n")
		egressipErr := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			egressIPMaps2 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps2) != 2 {
				return false, nil
			}
			sourceIP, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..15}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			e2e.Logf(sourceIP)
			if err != nil {
				return false, nil
			}
			if strings.Contains(sourceIP, egressIPMaps2[0]["egressIP"]) && strings.Contains(sourceIP, egressIPMaps2[1]["egressIP"]) {
				sourceIPSlice := findIP(sourceIP)
				if len(unique(sourceIPSlice)) == 2 {
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("The source Ip is not same as the egressIP expected!"))

		g.By("11. Start the stopped egress node \n")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
			startInstanceOnAWS(a, nodeList.Items[1].Name)
			checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeList.Items[1].Name, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		g.By("12. Check source IP is randomly one of 3 egress IPs.\n")
		e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
		sourceIP, err = execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..15}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(sourceIP)
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIP1[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIP2[0]))
		o.Expect(sourceIP).Should(o.ContainSubstring(freeIP3[0]))
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-53069-[Bug2097243] EgressIP should work for recreated same name pod. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("1. Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		g.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		g.By("3.1 Get temp namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("3.2 Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4. Create a pod in temp namespace. \n")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("5. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-53069",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("4. Check EgressIP assigned in the object.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			g.By("5. Check the source ip.\n")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			sourceIP, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(egressIPMaps[0]["egressIP"]))

			g.By("6. Delete the test pod and recreate it. \n")
			pod1.deletePingPod(oc)
			pod1.createPingPod(oc)
			waitPodReady(oc, pod1.namespace, pod1.name)

			g.By("7. Check the source ip.\n")
			sourceIP, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoURL+" --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(sourceIP).Should(o.Equal(egressIPMaps[0]["egressIP"]))
		case "tcpdump":
			g.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-53069", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-53069", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			g.By("5. Verify from tcpdump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

			g.By("6. Delete the test pod and recreate it. \n")
			pod1.deletePingPod(oc)
			pod1.createPingPod(oc)
			waitPodReady(oc, pod1.namespace, pod1.name)

			g.By("7. Verify from tcpdump that source IP is EgressIP")
			egressErr = verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

		default:
			g.Skip("Skip for not support scenarios!")
		}
	})
})

var _ = g.Describe("[sig-networking] SDN OVN EgressIP Basic", func() {
	//Cases in this function, do not need curl ip-echo
	defer g.GinkgoRecover()

	var (
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS or GCP cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-NonPreRelease-Medium-47029-Low-47024-Any egress IP can only be assigned to one node only. Warning event will be triggered if applying EgressIP object but no EgressIP nodes. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		g.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		g.By("2 Create first egressip object \n")
		freeIPs := findFreeIPs(oc, egressNodes[0], 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:          "egressip-47029",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("3. Check warning event. \n")
		warnErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			warningEvent, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", "default").Output()
			if err != nil {
				e2e.Logf("Wait for waring event generated.%v", err)
				return false, nil
			}
			if !strings.Contains(warningEvent, "NoMatchingNodeFound") {
				e2e.Logf("Wait for waring event generated. ")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(warnErr, "Warning event doesn't conclude: NoMatchingNodeFound.")

		g.By("4 Apply EgressLabel Key to nodes. \n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)

		g.By("5. Check EgressIP assigned in the object.\n")
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1), "EgressIP object should get one applied item, but actually not.")

		g.By("6 Create second egressip object with same egressIP \n")
		egressip2 := egressIPResource1{
			name:          "egressip-47024",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip2.createEgressIPObject2(oc)
		defer egressip2.deleteEgressIPObject1(oc)

		g.By("7 Check the second egressIP object, no egressIP assigned  .\n")
		egressIPStatus, egressIPerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip", egressip2.name, "-ojsonpath={.status.items}").Output()
		o.Expect(egressIPerr).NotTo(o.HaveOccurred())
		o.Expect(egressIPStatus).To(o.Equal(""))

		g.By("8. Edit the second egressIP object to another IP\n")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/"+egressip2.name, "-p", "{\"spec\":{\"egressIPs\":[\""+freeIPs[1]+"\"]}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("9. Check egressIP assigned in the second object.\n")
		egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip2.name)
		o.Expect(len(egressIPMaps2)).Should(o.Equal(1), "EgressIP object should get one applied item, but actually not.")

	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-47021-lr-policy-list and snat should be updated correctly after remove pods. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP1Template := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")

		g.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		g.By("2 Apply EgressLabel Key to one node. \n")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")

		g.By("3. create new namespace\n")
		ns1 := oc.Namespace()

		g.By("4. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("5. Create test pods and scale test pods to 10 \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=10", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("6. Create an egressip object\n")
		sub1 := getIfaddrFromNode(egressNode, oc)
		freeIPs := findUnUsedIPsOnNode(oc, egressNode, sub1, 2)
		o.Expect(len(freeIPs) == 2).Should(o.BeTrue())
		egressip1 := egressIPResource1{
			name:      "egressip-47021",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue())

		g.By("7.Scale down CNO to 0 \n")
		defer oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", "network-operator", "--replicas=1", "-n", "openshift-network-operator").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", "network-operator", "--replicas=0", "-n", "openshift-network-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("8.Delete ovnkube-master pods \n")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-l", "app=ovnkube-master", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("9. Scale test pods to 1 \n")
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=1", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podsErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			podsOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns1).Output()
			e2e.Logf(podsOutput)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Count(podsOutput, "test") == 1 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(podsErr, fmt.Sprintf("The pods were not scaled to the expected number!"))
		testPodName := getPodName(oc, ns1, "name=test-pods")
		_, testPodIPv4 := getPodIP(oc, ns1, testPodName[0])

		g.By("10. Scale up CNO to 1 \n")
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", "network-operator", "--replicas=1", "-n", "openshift-network-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-master")
		exutil.AssertWaitPollNoErr(err, "ovnkube-master pods are not ready")

		g.By("11. Check lr-policy-list and snat in northdb. \n")
		ovnPod := getOVNLeaderPod(oc, "north")
		o.Expect(ovnPod != "").Should(o.BeTrue())
		lspCmd := "ovn-nbctl lr-policy-list ovn_cluster_router | grep -v inport"
		checkLspErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			lspOutput, lspErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, lspCmd)
			if lspErr != nil {
				e2e.Logf("%v,Waiting for lr-policy-list to be synced, try next ...,", lspErr)
				return false, nil
			}
			e2e.Logf(lspOutput)
			if strings.Contains(lspOutput, testPodIPv4) && strings.Count(lspOutput, "100 ") == 1 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(checkLspErr, fmt.Sprintf("lr-policy-list was not synced correctly!"))

		snatCmd := "ovn-nbctl --format=csv --no-heading find nat external_ids:name=" + egressip1.name
		checkSnatErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			snatOutput, snatErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, snatCmd)
			if snatErr != nil {
				e2e.Logf("%v,Waiting for snat to be synced, try next ...,", snatErr)
				return false, nil
			}
			e2e.Logf(snatOutput)
			if strings.Contains(snatOutput, testPodIPv4) && strings.Count(snatOutput, egressip1.name) == 1 {
				e2e.Logf("The snat for egressip is as expected!")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(checkSnatErr, fmt.Sprintf("snat was not synced correctly!"))
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-HyperShiftGUEST-Longduration-NonPreRelease-Medium-47208-The configured EgressIPs exceeds IP capacity. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		g.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		g.By("2 Apply EgressLabel Key to one node. \n")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		g.By("3 Get IP capacity of the node. \n")
		ipCapacity := getIPv4Capacity(oc, egressNode)
		o.Expect(ipCapacity != "").Should(o.BeTrue())
		ipCap, _ := strconv.Atoi(ipCapacity)
		if ipCap > 14 {
			g.Skip("This is not the general IP capacity, will skip it.")
		}
		exceedNum := ipCap + 1

		g.By("4 Create egressip objects \n")
		sub1 := getIfaddrFromNode(egressNode, oc)
		freeIPs := findUnUsedIPsOnNode(oc, egressNode, sub1, exceedNum)
		o.Expect(len(freeIPs) == exceedNum).Should(o.BeTrue())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("egressip", "--all").Execute()
		egressIPConfig := make([]egressIPResource1, exceedNum)
		for i := 0; i <= ipCap; i++ {
			iVar := strconv.Itoa(i)
			egressIPConfig[i] = egressIPResource1{
				name:          "egressip-47208-" + iVar,
				template:      egressIP2Template,
				egressIP1:     freeIPs[i],
				nsLabelKey:    "org",
				nsLabelValue:  "qe",
				podLabelKey:   "color",
				podLabelValue: "pink",
			}
			egressIPConfig[i].createEgressIPObject2(oc)
		}

		g.By("5 Check ipCapacity+1 number egressIP created,but one is not assigned egress node \n")
		egressIPErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			egressIPOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip").Output()
			e2e.Logf(egressIPOutput)
			if err != nil {
				e2e.Logf("Wait for egressip assigned.%v", err)
				return false, nil
			}
			if strings.Count(egressIPOutput, "egressip-47208") == exceedNum {
				e2e.Logf("The %v number egressIP object created.", exceedNum)
				if strings.Count(egressIPOutput, egressNode) == ipCap {
					e2e.Logf("The %v number egressIPs were assigned.", ipCap)
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(egressIPErr, fmt.Sprintf(" Error at getting EgressIPs or EgressIPs were not assigned corrently."))

		g.By("6. Check warning event. \n")
		warnErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			warningEvent, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", "default").Output()
			if err != nil {
				e2e.Logf("Wait for warning event generated.%v", err)
				return false, nil
			}
			if !strings.Contains(warningEvent, "NoMatchingNodeFound") {
				e2e.Logf("Expected warning message is not found, try again ")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(warnErr, fmt.Sprintf("Warning event doesn't conclude: NoMatchingNodeFound."))

	})

	// author: jechen@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jechen-High-54045-EgressIP health check through monitoring port over GRPC on OCP OVN cluster. [Disruptive]", func() {

		ipStackType := checkIPStackType(oc)
		if ipStackType != "ipv4single" {
			g.Skip("This case requires IPv4 cluster only")
		}

		g.By("1 check ovnkube-config configmap if egressip-node-healthcheck-port=9107 is in it \n")
		configmapName := "ovnkube-config"
		envString := " egressip-node-healthcheck-port=9107"
		cmCheckErr := checkEnvInConfigMap(oc, "openshift-ovn-kubernetes", configmapName, envString)
		o.Expect(cmCheckErr).NotTo(o.HaveOccurred())

		g.By("2 get leader ovnkube-master pod and ovnkube-node pods \n")
		readyErr := waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-master")
		exutil.AssertWaitPollNoErr(readyErr, "ovnkube-master pods are not ready")
		ovnMasterPodName := getOVNLeaderPod(oc, "north")

		readyErr = waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		exutil.AssertWaitPollNoErr(readyErr, "ovnkube-node pods are not ready")
		ovnkubeNodePods := getPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		g.By("3 Check each ovnkube-node pod's log that health check server is started on it \n")
		expectedString := "Starting Egress IP Health Server on "
		for _, ovnkubeNodePod := range ovnkubeNodePods {
			podLogs, LogErr := checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-node", ovnkubeNodePod, "'egress ip'")
			o.Expect(LogErr).NotTo(o.HaveOccurred())
			o.Expect(podLogs).To(o.ContainSubstring(expectedString))
		}

		g.By("4 Get list of nodes, pick one as egressNode, apply EgressLabel Key to it \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		nodeOVNK8sMgmtIP := getOVNK8sNodeMgmtIPv4(oc, egressNode)

		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		g.By("5 Check leader ovnkube-master pod's log that health check connection has been made to the egressNode on port 9107 \n")
		expectedString = "Connected to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr := checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-master", ovnMasterPodName, "'"+expectedString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString))

		g.By("6. Add iptables on to block port 9107 on egressNode, verify from log of ovnkube-master pod that the health check connection is closed.\n")
		defer exutil.DebugNodeWithChroot(oc, egressNode, "iptables", "-D", "INPUT", "-p", "tcp", "--destination-port", "9107", "-j", "DROP")
		_, debugNodeErr := exutil.DebugNodeWithChroot(oc, egressNode, "iptables", "-I", "INPUT", "1", "-p", "tcp", "--destination-port", "9107", "-j", "DROP")
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

		expectedString1 := "Closing connection with " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-master", ovnMasterPodName, "'"+expectedString1+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString1))
		expectedString2 := "Could not connect to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-master", ovnMasterPodName, "'"+expectedString2+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString2))

		g.By("7. Delete the iptables rule, verify from log of ovnkube-master pod that the health check connection is re-established.\n")
		_, debugNodeErr = exutil.DebugNodeWithChroot(oc, egressNode, "iptables", "-D", "INPUT", "-p", "tcp", "--destination-port", "9107", "-j", "DROP")
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

		expectedString = "Connected to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-master", ovnMasterPodName, "'"+expectedString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString))

		g.By("8. Unlabel the egressNoe egressip-assignable, verify from log of ovnkube-master pod that the health check connection is closed.\n")
		e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		expectedString = "Closing connection with " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"

		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-master", ovnMasterPodName, "'"+expectedString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString))
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-Longduration-NonPreRelease-55030-After reboot egress node, lr-policy-list and snat should keep correct. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP1Template := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")

		g.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		g.By("2 Apply EgressLabel Key to one node. \n")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")

		g.By("3. create new namespace\n")
		ns1 := oc.Namespace()

		g.By("4. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("5. Create test pods and scale test pods to 10 \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=10", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("6. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNodes[0], 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-55030",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(2), "EgressIP object expected 2 items!!")

		g.By("5.Reboot egress node.\n")
		defer checkNodeStatus(oc, egressNodes[0], "Ready")
		rebootNode(oc, egressNodes[0])
		checkNodeStatus(oc, egressNodes[0], "NotReady")
		checkNodeStatus(oc, egressNodes[0], "Ready")
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("6. Check lr-policy-list and snat in northdb. \n")
		lspCmd := "ovn-nbctl lr-policy-list ovn_cluster_router | grep -v inport"
		lspErr := searchOVNDBForSpecCmd(oc, lspCmd, "100 ", 10)
		exutil.AssertWaitPollNoErr(lspErr, ("The lr-policy-list was not synced correctly! "))
		snatCmd := "ovn-nbctl --format=csv --no-heading find nat external_ids:name=" + egressip1.name
		snatErr := searchOVNDBForSpecCmd(oc, snatCmd, egressip1.name, 20)
		exutil.AssertWaitPollNoErr(snatErr, ("Snat rules for egressIP was not synced correctly! "))
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-55632-After enable egress node, egress node shouldn't generate broadcast ARP for service IPs. [Serial]", func() {
		e2e.Logf("This case is from customer bug: https://bugzilla.redhat.com/show_bug.cgi?id=2052975")
		g.By("1 Get list of nodes \n")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		egessNode := nodeList.Items[0].Name

		g.By("2 Apply EgressLabel Key to one node. \n")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel)
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel, "true")

		g.By("3. Check no ARP broadcast for service IPs\n")
		e2e.Logf("Trying to get physical interface on the node,%s", egessNode)
		phyInf, nicError := getSnifPhyInf(oc, egessNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("tcpdump -c 10 -nni %s arp", phyInf)
		outPut, tcpdumpErr := exutil.DebugNode(oc, egessNode, "bash", "-c", tcpdumpCmd)
		o.Expect(tcpdumpErr).NotTo(o.HaveOccurred())
		o.Expect(outPut).NotTo(o.ContainSubstring("172.30"), fmt.Sprintf("The output of tcpdump is %s", outPut))
	})

})

var _ = g.Describe("[sig-networking] SDN OVN EgressIP", func() {
	//Cases in this function, do not need curl ip-echo
	defer g.GinkgoRecover()

	var (
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS or GCP cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-47163-High-47026-Deleting EgressIP object and recreating it works,EgressIP was removed after delete egressIP object. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("Get the temporary namespace")
		ns := oc.Namespace()

		g.By("Get schedulable worker nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")

		g.By("Create tcpdump sniffer Daemonset.")
		primaryInf, infErr := getSnifPhyInf(oc, egressNode)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost := nslookDomainName("ifconfig.me")
		defer deleteTcpdumpDS(oc, "tcpdump-47163", ns)
		tcpdumpDS, snifErr := createSnifferDaemonset(oc, ns, "tcpdump-47163", "tcpdump", "true", dstHost, primaryInf, 80)
		o.Expect(snifErr).NotTo(o.HaveOccurred())

		g.By("Apply EgressLabel Key for this test on one node.")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		g.By("Apply label to namespace")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name=test").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name-").Output()

		g.By("Create an egressip object")
		sub1 := getIfaddrFromNode(egressNode, oc)
		freeIps := findUnUsedIPsOnNode(oc, egressNode, sub1, 2)
		o.Expect(len(freeIps) == 2).Should(o.BeTrue())
		egressip1 := egressIPResource1{
			name:      "egressip-47163",
			template:  egressIPTemplate,
			egressIP1: freeIps[0],
			egressIP2: freeIps[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue(), fmt.Sprintf("The egressIP was not assigned correctly!"))

		g.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		defer pod1.deletePingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("Check source IP is EgressIP")
		egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps1[0]["egressIP"], dstHost, ns, tcpdumpDS.name, true)
		o.Expect(egressErr).NotTo(o.HaveOccurred())

		g.By("Deleting egressip object")
		egressip1.deleteEgressIPObject1(oc)
		waitCloudPrivateIPconfigUpdate(oc, egressIPMaps1[0]["egressIP"], false)
		egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			randomStr, url := getRequestURL(dstHost)
			_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, url)
			if checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, egressIPMaps1[0]["egressIP"], false) != nil {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to clear egressip:%s", egressipErr))

		g.By("Recreating egressip object")
		egressip1.createEgressIPObject1(oc)
		egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps2) == 1).Should(o.BeTrue(), fmt.Sprintf("The egressIP was not assigned correctly!"))

		g.By("Check source IP is EgressIP")
		egressErr = verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps2[0]["egressIP"], dstHost, ns, tcpdumpDS.name, true)
		o.Expect(egressErr).NotTo(o.HaveOccurred())

		g.By("Deleting EgressIP object and recreating it works!!! ")

	})

})
