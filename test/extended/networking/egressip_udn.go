package networking

import (
	"context"
	"fmt"
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
	netutils "k8s.io/utils/net"
)

var _ = g.Describe("[sig-networking] SDN udn EgressIP", func() {
	defer g.GinkgoRecover()

	var (
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {

		SkipIfNoFeatureGate(oc, "NetworkSegmentation")

		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)

		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "azure") || strings.Contains(platform, "none") || strings.Contains(platform, "nutanix") || strings.Contains(platform, "powervs")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS/GCP/Azure/Openstack/Vsphere/BareMetal/Nutanix/Powervs cluster with ovn network plugin, skip for other platforms or other non-OVN network plugin!!")
		}

		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv6single" {
			// Not able to run on IPv6 single cluster for now due to cluster disconnect limiation.
			g.Skip("Skip IPv6 Single cluster.")
		}

		if !(strings.Contains(platform, "none") || strings.Contains(platform, "powervs")) && (checkProxy(oc) || checkDisconnect(oc)) {
			g.Skip("This is proxy/disconnect cluster, skip the test.")
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-High-77654-Validate egressIP with mixed of multiple non-overlapping UDNs and default network(layer3 and IPv4 only) [Serial]", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP2Template         = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1 Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Need 2 nodes for the test, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode := nodeList.Items[0].Name
		theOtherNode := nodeList.Items[1].Name

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")

		exutil.By("2 Create an egressip object, verify egressIP is assigned to egress node")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-77654",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNode))

		exutil.By("3.1 Obtain first namespace, create a second namespace, these two namespaces are for two non-overlapping UDNs")
		ns1 := oc.Namespace()
		oc.SetupProject()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.SetupProject()
		ns3 := oc.Namespace()
		allNS := []string{ns1, ns2, ns3}

		exutil.By("3.3 Apply a label to all namespaces that matches namespaceSelector definied in egressIP object")
		for _, ns := range allNS {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Create two different UDNs in namesapce ns1 and ns2")
		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		cidr := []string{"10.150.0.0/16", "10.151.0.0/16"}
		udncrd := make([]udnCRDResource, 2)
		for i := 0; i < 2; i++ {
			udncrd[i] = udnCRDResource{
				crdname:   udnResourcename[i],
				namespace: udnNS[i],
				role:      "Primary",
				mtu:       mtu,
				cidr:      cidr[i],
				prefix:    24,
				template:  udnCRDSingleStack,
			}
			udncrd[i].createUdnCRDSingleStack(oc)
			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.1 In each namespace, create two test pods, the first one on egressNode, the second one on nonEgressNode ")
		exutil.By("5.2 Apply label to all pods that matches podSelector definied in egressIP object")
		testpods1 := make([]pingPodResourceNode, 3)
		testpods2 := make([]pingPodResourceNode, 3)
		for i := 0; i < 3; i++ {
			// testpods1 are local pods that co-locate on egress node
			testpods1[i] = pingPodResourceNode{
				name:      "hello-pod1-" + allNS[i],
				namespace: allNS[i],
				nodename:  egressNode,
				template:  pingPodNodeTemplate,
			}
			testpods1[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods1[i].name)
			defer exutil.LabelPod(oc, allNS[i], testpods1[i].name, "color-")
			err = exutil.LabelPod(oc, allNS[i], testpods1[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())

			// testpods1 are remote pods on the other non-egress node
			testpods2[i] = pingPodResourceNode{
				name:      "hello-pod2-" + allNS[i],
				namespace: allNS[i],
				nodename:  theOtherNode,
				template:  pingPodNodeTemplate,
			}
			testpods2[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods2[i].name)
			defer exutil.LabelPod(oc, allNS[i], testpods2[i].name, "color-")
			err = exutil.LabelPod(oc, allNS[i], testpods2[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. Verify egressIP from each namespace, egress traffic from these pods should use egressIP as their sourceIP regardless it is from UDN or default network")
		var dstHost, primaryInf string
		var infErr error
		exutil.By("6.1 Use tcpdump to verify egressIP.")
		e2e.Logf("Trying to get physical interface on the egressNode,%s", egressNode)
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost = nslookDomainName("ifconfig.me")
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod := getRequestURL(dstHost)

		exutil.By("6.2 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		for i := 0; i < 3; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, allNS[i], testpods1[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
			tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, allNS[i], testpods2[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-High-77655-Validate egressIP with mixed of multiple overlapping UDNs and default network(layer3 and IPv4 only) [Serial]", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP2Template         = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1 Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Need 2 nodes for the test, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode := nodeList.Items[0].Name
		theOtherNode := nodeList.Items[1].Name

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")

		exutil.By("2 Create an egressip object, verify egressIP is assigned to egress node")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-77655",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNode))

		exutil.By("3.1 Obtain first namespace, create a second namespace, these two namespaces are for two non-overlapping UDNs")
		ns1 := oc.Namespace()
		oc.SetupProject()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.SetupProject()
		ns3 := oc.Namespace()
		allNS := []string{ns1, ns2, ns3}

		exutil.By("3.3 Apply a label to all namespaces that matches namespaceSelector definied in egressIP object")
		for _, ns := range allNS {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Create two overlapping UDNs in namesapce ns1 and ns2")
		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		cidr := []string{"10.150.0.0/16", "10.150.0.0/16"}
		udncrd := make([]udnCRDResource, 2)
		for i := 0; i < 2; i++ {
			udncrd[i] = udnCRDResource{
				crdname:   udnResourcename[i],
				namespace: udnNS[i],
				role:      "Primary",
				mtu:       mtu,
				cidr:      cidr[i],
				prefix:    24,
				template:  udnCRDSingleStack,
			}
			udncrd[i].createUdnCRDSingleStack(oc)
			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.1 In each namespace, create two test pods, the first one on egressNode, the second one on nonEgressNode ")
		exutil.By("5.2 Apply label to all pods that matches podSelector definied in egressIP object")
		testpods1 := make([]pingPodResourceNode, 3)
		testpods2 := make([]pingPodResourceNode, 3)
		for i := 0; i < 3; i++ {
			// testpods1 are lcaol pods that co-locate on egress node
			testpods1[i] = pingPodResourceNode{
				name:      "hello-pod1-" + allNS[i],
				namespace: allNS[i],
				nodename:  egressNode,
				template:  pingPodNodeTemplate,
			}
			testpods1[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods1[i].name)
			defer exutil.LabelPod(oc, allNS[i], testpods1[i].name, "color-")
			err = exutil.LabelPod(oc, allNS[i], testpods1[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())

			// testpods1 are remote pods on the other non-egress node
			testpods2[i] = pingPodResourceNode{
				name:      "hello-pod2-" + allNS[i],
				namespace: allNS[i],
				nodename:  theOtherNode,
				template:  pingPodNodeTemplate,
			}
			testpods2[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods2[i].name)
			defer exutil.LabelPod(oc, allNS[i], testpods2[i].name, "color-")
			err = exutil.LabelPod(oc, allNS[i], testpods2[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. Verify egressIP from each namespace, egress traffic from these pods should use egressIP as their sourceIP regardless it is from UDN or default network")
		var dstHost, primaryInf string
		var infErr error
		exutil.By("6.1 Use tcpdump to verify egressIP.")
		e2e.Logf("Trying to get physical interface on the egressNode %s", egressNode)
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost = nslookDomainName("ifconfig.me")
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod := getRequestURL(dstHost)

		exutil.By("6.2 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		for i := 0; i < 3; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, allNS[i], testpods1[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
			tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, allNS[i], testpods2[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-Longduration-NonPreRelease-High-77744-Validate egressIP Failover with non-overlapping and overlapping UDNs (layer3 and IPv4 only) [Serial]", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP2Template         = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1 Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")

		exutil.By("2 Create an egressip object, verify egressIP is assigned to egress node")
		freeIPs := findFreeIPs(oc, egressNodes[0], 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-77744",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNodes[0]))

		exutil.By("3.1 Obtain first namespace, create two more namespaces")
		ns1 := oc.Namespace()
		oc.SetupProject()
		ns2 := oc.Namespace()
		oc.SetupProject()
		ns3 := oc.Namespace()
		udnNS := []string{ns1, ns2, ns3}

		exutil.By("3.2 Apply a label to all namespaces that matches namespaceSelector definied in egressIP object")
		for _, ns := range udnNS {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Create non-overlapping UDNs between ns1 and ns2, overlapping UDN between ns2 and ns3")
		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2, "l3-network-" + ns3}
		cidr := []string{"10.150.0.0/16", "10.151.0.0/16", "10.151.0.0/16"}
		udncrd := make([]udnCRDResource, 3)
		for i := 0; i < 2; i++ {
			udncrd[i] = udnCRDResource{
				crdname:   udnResourcename[i],
				namespace: udnNS[i],
				role:      "Primary",
				mtu:       mtu,
				cidr:      cidr[i],
				prefix:    24,
				template:  udnCRDSingleStack,
			}
			udncrd[i].createUdnCRDSingleStack(oc)
			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.1 In each namespace, create two test pods, the first one on egressNode, the second one on nonEgressNode ")
		exutil.By("5.2 Apply label to all pods that matches podSelector definied in egressIP object")
		testpods1 := make([]pingPodResourceNode, 3)
		testpods2 := make([]pingPodResourceNode, 3)
		for i := 0; i < 3; i++ {
			// testpods1 are pods on egressNode
			testpods1[i] = pingPodResourceNode{
				name:      "hello-pod1-" + udnNS[i],
				namespace: udnNS[i],
				nodename:  egressNodes[0],
				template:  pingPodNodeTemplate,
			}
			testpods1[i].createPingPodNode(oc)
			waitPodReady(oc, udnNS[i], testpods1[i].name)
			defer exutil.LabelPod(oc, udnNS[i], testpods1[i].name, "color-")
			err = exutil.LabelPod(oc, udnNS[i], testpods1[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())

			// testpods2 are pods on nonEgressNode, egressNodes[1] is currently not a egress node as it is not labelled with egressNodeLabel
			testpods2[i] = pingPodResourceNode{
				name:      "hello-pod2-" + udnNS[i],
				namespace: udnNS[i],
				nodename:  egressNodes[1],
				template:  pingPodNodeTemplate,
			}
			testpods2[i].createPingPodNode(oc)
			waitPodReady(oc, udnNS[i], testpods2[i].name)
			defer exutil.LabelPod(oc, udnNS[i], testpods2[i].name, "color-")
			err = exutil.LabelPod(oc, udnNS[i], testpods2[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. Verify egressIP from each namespace, egress traffic from these pods should use egressIP as their sourceIP regardless it is from overlapping or non-overlapping UDN")
		var dstHost, primaryInf, tcpdumpCmd, cmdOnPod string
		var infErr error
		exutil.By("6.1 Use tcpdump to verify egressIP.")
		e2e.Logf("Trying to get physical interface on the egressNode %s", egressNodes[0])
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost = nslookDomainName("ifconfig.me")
		tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod = getRequestURL(dstHost)

		exutil.By("6.2 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		for i := 0; i < 3; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNodes[0], tcpdumpCmd, udnNS[i], testpods1[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
			tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNodes[0], tcpdumpCmd, udnNS[i], testpods2[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("7. Label the second node with egressNodeLabel, unlabel the first node, verify egressIP still works after failover.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")

		exutil.By("8. Check the egress node was updated in the egressip object.\n")
		egressipErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 360*time.Second, false, func(cxt context.Context) (bool, error) {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps1) != 1 || egressIPMaps1[0]["node"] == egressNodes[0] {
				e2e.Logf("Wait for new egress node applied,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNodes[1]))

		exutil.By("9. Validate egressIP again after egressIP failover \n")
		exutil.By("9.1 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods after egressIP failover")
		for i := 0; i < 3; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNodes[1], tcpdumpCmd, udnNS[i], testpods1[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
			tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNodes[1], tcpdumpCmd, udnNS[i], testpods2[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-Medium-78276-non-overlapping and overlapping UDN egressIP Pods will not be affected by the egressIP set on other netnamespace(layer3 and IPv4 only) [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP2Template   = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		)

		exutil.By("1. Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Need at least 1 node for the test, the prerequirement was not fullfilled, skip the case!!")
		}
		egressNode := nodeList.Items[0].Name

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")

		exutil.By("2. Create 3 namespaces for non-overlapping and overlapping UDNs")
		ns1 := oc.Namespace()
		oc.SetupProject()
		ns2 := oc.Namespace()
		oc.SetupProject()
		ns3 := oc.Namespace()
		allNS := []string{ns1, ns2, ns3}

		exutil.By("3. Get 3 unused IPs from the same subnet of the egressNode,create 3 egressIP objects with same namespaceSelector but different podSelector")
		freeIPs := findFreeIPs(oc, egressNode, 3)
		o.Expect(len(freeIPs)).Should(o.Equal(3))

		podLabelValues := []string{"pink", "blue", "red"}
		egressips := make([]egressIPResource1, 3)
		for i := 0; i < len(allNS); i++ {
			egressips[i] = egressIPResource1{
				name:          "egressip-78276-" + strconv.Itoa(i),
				template:      egressIP2Template,
				egressIP1:     freeIPs[i],
				nsLabelKey:    "org",
				nsLabelValue:  "qe",
				podLabelKey:   "color",
				podLabelValue: podLabelValues[i],
			}
			egressips[i].createEgressIPObject2(oc)
			defer egressips[i].deleteEgressIPObject1(oc)
			egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressips[i].name)
			o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		}

		exutil.By("4. Apply UDN CRD to each namespace,apply to each namespace with label that matches namespaceSelector definied in egressIP object")
		for i, ns := range allNS {
			err = applyL3UDNtoNamespace(oc, ns, i)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5. In each namespace, create a test pod, apply to test pod with label that matches podSelector definied in egressIP object")
		testpods := make([]pingPodResource, 3)
		for i := 0; i < len(allNS); i++ {
			testpods[i] = pingPodResource{
				name:      "hello-pod" + strconv.Itoa(i),
				namespace: allNS[i],
				template:  pingPodTemplate,
			}
			testpods[i].createPingPod(oc)
			waitPodReady(oc, testpods[i].namespace, testpods[i].name)
			defer exutil.LabelPod(oc, allNS[i], testpods[i].name, "color-")
			err = exutil.LabelPod(oc, allNS[i], testpods[i].name, "color="+podLabelValues[i])
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. Verify egressIP from each namespace, egress traffic from each pod should use egressIP defined in the egressIP object the pod qualifies")
		var dstHost, primaryInf string
		var infErr error
		e2e.Logf("Trying to get physical interface on the egressNode,%s", egressNode)
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost = nslookDomainName("ifconfig.me")
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)

		exutil.By("Use tcpdump captured on egressNode to verify egressIP each pod")
		for i := 0; i < len(allNS); i++ {
			_, cmdOnPod := getRequestURL(dstHost)
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, allNS[i], testpods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[i])).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-High-78199-egressIP still works correctly after a UDN network gets deleted then recreated (layer3 and IPv4 only) [Serial]", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP2Template         = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1 Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, nodesToBeUsed := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || nodesToBeUsed == nil || len(nodesToBeUsed) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode := nodesToBeUsed[0]

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")

		exutil.By("2 Create an egressip object, verify egressIP is assigned to egress node")
		freeIPs := findFreeIPs(oc, egressNode, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-78199",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNode))

		exutil.By("3 Obtain a namespace, apply a label to the namespace that matches namespaceSelector definied in egressIP object")
		ns1 := oc.Namespace()

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create an UDN in ns1")
		cidr := []string{"10.150.0.0/16"}
		udncrd := udnCRDResource{
			crdname:   "l3-network-" + ns1,
			namespace: ns1,
			role:      "Primary",
			mtu:       mtu,
			cidr:      cidr[0],
			prefix:    24,
			template:  udnCRDSingleStack,
		}
		udncrd.createUdnCRDSingleStack(oc)
		udnErr := waitUDNCRDApplied(oc, ns1, udncrd.crdname)
		o.Expect(udnErr).NotTo(o.HaveOccurred())

		exutil.By("5.1 In the namespace, create two test pods, the first one on egressNode, the second one on nonEgressNode ")
		exutil.By("5.2 Apply to all pods with label that matches podSelector definied in egressIP object")
		testpods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			testpods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i) + "-" + ns1,
				namespace: ns1,
				nodename:  nodesToBeUsed[i],
				template:  pingPodNodeTemplate,
			}
			testpods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, testpods[i].name)
			defer exutil.LabelPod(oc, ns1, testpods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, testpods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. Verify egressIP from each namespace, egress traffic from these pods should use egressIP as their sourceIP regardless it is from overlapping or non-overlapping UDN")
		var dstHost, primaryInf, tcpdumpCmd, cmdOnPod string
		var infErr error
		exutil.By("6.1 Use tcpdump to verify egressIP.")
		e2e.Logf("Trying to get physical interface on the egressNode %s", egressNode)
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost = nslookDomainName("ifconfig.me")
		tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod = getRequestURL(dstHost)

		exutil.By("6.2 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		for i := 0; i < 2; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, ns1, testpods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("7. Delete local and remote test pods that are associated with UDN in ns1 first, then delete the UDN in ns1.\n")
		for i := 0; i < 2; i++ {
			removeResource(oc, true, true, "pod", testpods[i].name, "-n", testpods[i].namespace)
		}
		removeResource(oc, true, true, "UserDefinedNetwork", udncrd.crdname, "-n", udncrd.namespace)

		exutil.By("8. Recreate the UDN and local/remote test pods in ns1.\n")
		udncrd.createUdnCRDSingleStack(oc)
		for i := 0; i < 2; i++ {
			testpods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, testpods[i].name)
			defer exutil.LabelPod(oc, ns1, testpods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, testpods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		udnErr = waitUDNCRDApplied(oc, ns1, udncrd.crdname)
		o.Expect(udnErr).NotTo(o.HaveOccurred())

		exutil.By("9. Validate egressIP again after recreating UDN in ns1 \n")
		exutil.By("9.1 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods again after UDN recreation")
		for i := 0; i < 2; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, ns1, testpods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}
	})

})

var _ = g.Describe("[sig-networking] SDN udn EgressIP IPv6", func() {
	defer g.GinkgoRecover()

	var (
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		dstHostv6       = "2620:52:0:800:3673:5aff:fe99:92f0"
		ipStackType     string
	)

	g.BeforeEach(func() {

		SkipIfNoFeatureGate(oc, "NetworkSegmentation")

		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		if err != nil || !(strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
			g.Skip("This case will only run on rdu1 or rdu2 dual stack cluster. , skip for other envrionment!!!")
		}

		ipStackType = checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			g.Skip("It is not a dualsatck or singlev6 cluster, skip this test!!!")
		}

		if strings.Contains(msg, "offload.openshift-qe.sdn.com") {
			dstHostv6 = "2620:52:0:800:3673:5aff:fe98:d2d0"
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-77840-Validate egressIP with mixed of multiple non-overlapping UDNs and default network(layer3 and IPv6/dualstack) [Serial]", func() {

		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP1Template         = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			udnCRDL3dualStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1 Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		var egressNode1, egressNode2, nonEgressNode string
		var freeIPs []string
		if ipStackType == "dualstack" && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on dualstack cluster, the prerequirement was not fullfilled, skip the case!!")
		}
		if ipStackType == "ipv6single" && len(nodeList.Items) < 2 {
			g.Skip("Need 2 nodes for the test on singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		if ipStackType == "dualstack" {
			egressNode1 = nodeList.Items[0].Name
			egressNode2 = nodeList.Items[1].Name
			nonEgressNode = nodeList.Items[2].Name
			freeIPs = findFreeIPs(oc, egressNode1, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPv6s := findFreeIPv6s(oc, egressNode2, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPs = append(freeIPs, freeIPv6s[0])
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")
		} else if ipStackType == "ipv6single" {
			egressNode1 = nodeList.Items[0].Name
			nonEgressNode = nodeList.Items[1].Name
			freeIPs = findFreeIPv6s(oc, egressNode1, 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		}
		e2e.Logf("egressIPs to use: %s", freeIPs)

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")

		exutil.By("2. Create an egressip object")
		egressip1 := egressIPResource1{
			name:      "egressip-77840",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		var assignedEIPNodev4, assignedEIPNodev6, assignedEIPv6Addr string
		if ipStackType == "dualstack" {
			o.Expect(len(egressIPMaps1) == 2).Should(o.BeTrue())
			for _, eipMap := range egressIPMaps1 {
				if netutils.IsIPv4String(eipMap["egressIP"]) {
					assignedEIPNodev4 = eipMap["node"]
				}
				if netutils.IsIPv6String(eipMap["egressIP"]) {
					assignedEIPNodev6 = eipMap["node"]
					assignedEIPv6Addr = eipMap["egressIP"]
				}
			}
			o.Expect(assignedEIPNodev4).NotTo(o.Equal(""))
			o.Expect(assignedEIPNodev6).NotTo(o.Equal(""))
			e2e.Logf("For the dualstack EIP,  v4 EIP is currently assigned to node: %s, v6 EIP is currently assigned to node: %s", assignedEIPNodev4, assignedEIPNodev6)
		} else {
			o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue())
			assignedEIPNodev6 = egressNode1
			assignedEIPv6Addr = egressIPMaps1[0]["egressIP"]
		}

		exutil.By("3.1 Obtain first namespace, create a second namespace, these two namespaces are for two non-overlapping UDNs")
		ns1 := oc.Namespace()
		oc.SetupProject()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.SetupProject()
		ns3 := oc.Namespace()
		allNS := []string{ns1, ns2, ns3}

		exutil.By("3.3 Apply a label to all namespaces that matches namespaceSelector definied in egressIP object")
		for _, ns := range allNS {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name=test").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Create two different UDNs in namesapce ns1 and ns2")
		var cidr, ipv4cidr, ipv6cidr []string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv6single" {
			cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			prefix = 64
		} else {
			ipv4cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
			ipv4prefix = 24
			ipv6cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			ipv6prefix = 64
		}

		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		udncrd := make([]udnCRDResource, 2)
		for i := 0; i < 2; i++ {
			if ipStackType == "dualstack" {
				udncrd[i] = udnCRDResource{
					crdname:    udnResourcename[i],
					namespace:  udnNS[i],
					role:       "Primary",
					mtu:        mtu,
					IPv4cidr:   ipv4cidr[i],
					IPv4prefix: ipv4prefix,
					IPv6cidr:   ipv6cidr[i],
					IPv6prefix: ipv6prefix,
					template:   udnCRDL3dualStack,
				}
				udncrd[i].createUdnCRDDualStack(oc)
			} else {
				udncrd[i] = udnCRDResource{
					crdname:   udnResourcename[i],
					namespace: udnNS[i],
					role:      "Primary",
					mtu:       mtu,
					cidr:      cidr[i],
					prefix:    prefix,
					template:  udnCRDSingleStack,
				}
				udncrd[i].createUdnCRDSingleStack(oc)
			}
			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.1 In each namespace, create two test pods, the first one on egressNode, the second one on nonEgressNode ")
		exutil.By("5.2 Apply label to all pods that matches podSelector definied in egressIP object")
		testpods1 := make([]pingPodResourceNode, 3)
		testpods2 := make([]pingPodResourceNode, 3)
		testpods3 := make([]pingPodResourceNode, 3)
		for i := 0; i < 3; i++ {
			if ipStackType == "dualstack" {
				// testpods1 are local pods that co-locate on assignedEIPNodev4 for dualstack
				testpods1[i] = pingPodResourceNode{
					name:      "hello-pod1-" + allNS[i],
					namespace: allNS[i],
					nodename:  assignedEIPNodev4,
					template:  pingPodNodeTemplate,
				}
				testpods1[i].createPingPodNode(oc)
				waitPodReady(oc, allNS[i], testpods1[i].name)
			}

			// testpods2 are local pods that co-locate on assignedEIPNodev6
			testpods2[i] = pingPodResourceNode{
				name:      "hello-pod2-" + allNS[i],
				namespace: allNS[i],
				nodename:  assignedEIPNodev6,
				template:  pingPodNodeTemplate,
			}
			testpods2[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods2[i].name)

			// testpods3 are remote pods on the other non-egress node
			testpods3[i] = pingPodResourceNode{
				name:      "hello-pod3-" + allNS[i],
				namespace: allNS[i],
				nodename:  nonEgressNode,
				template:  pingPodNodeTemplate,
			}
			testpods3[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods3[i].name)
		}

		exutil.By("6. Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		e2e.Logf("Trying to get physical interface on the node,%s", egressNode1)
		primaryInf, infErr := getSnifPhyInf(oc, egressNode1)
		o.Expect(infErr).NotTo(o.HaveOccurred())

		for i := 0; i < 3; i++ {
			if ipStackType == "dualstack" {
				exutil.By("6.1 Verify egressIP from IPv4 perspective")
				dstHostv4 := nslookDomainName("ifconfig.me")
				exutil.SetNamespacePrivileged(oc, oc.Namespace())
				tcpdumpCmdv4 := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHostv4)
				_, cmdOnPodv4 := getRequestURL(dstHostv4)
				exutil.By("6.2 Verify v4 egressIP from test pods local to egress node")
				tcpdumOutputv4 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, allNS[i], testpods1[i].name, cmdOnPodv4)
				o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
				exutil.By("6.3 Verify v4 egressIP from test pods remote to egress node")
				tcpdumOutputv4 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, allNS[i], testpods3[i].name, cmdOnPodv4)
				o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
			}

			exutil.By("6.4 Verify egressIP from IPv6 perspective")
			tcpdumpCmdv6 := fmt.Sprintf("timeout 60s tcpdump -c 3 -nni %s ip6 and host %s", primaryInf, dstHostv6)
			_, cmdOnPodv6 := getRequestURL("[" + dstHostv6 + "]")
			exutil.By("6.5 Verify v6 egressIP from test pods local to egress node")
			tcpdumOutputv6 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, allNS[i], testpods2[i].name, cmdOnPodv6)
			o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
			exutil.By("6.6 Verify v6 egressIP from test pods remote to egress node")
			tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, allNS[i], testpods3[i].name, cmdOnPodv6)
			o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-High-77841-Validate egressIP with mixed of multiple overlapping UDNs and default network(layer3 and IPv6/dualstack) [Serial]", func() {

		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP1Template         = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			udnCRDL3dualStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1 Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		var egressNode1, egressNode2, nonEgressNode string
		var freeIPs []string
		if ipStackType == "dualstack" && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on dualstack cluster, the prerequirement was not fullfilled, skip the case!!")
		}
		if ipStackType == "ipv6single" && len(nodeList.Items) < 2 {
			g.Skip("Need 2 nodes for the test on singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		if ipStackType == "dualstack" {
			egressNode1 = nodeList.Items[0].Name
			egressNode2 = nodeList.Items[1].Name
			nonEgressNode = nodeList.Items[2].Name
			freeIPs = findFreeIPs(oc, egressNode1, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPv6s := findFreeIPv6s(oc, egressNode2, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPs = append(freeIPs, freeIPv6s[0])
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")
		} else if ipStackType == "ipv6single" {
			egressNode1 = nodeList.Items[0].Name
			nonEgressNode = nodeList.Items[1].Name
			freeIPs = findFreeIPv6s(oc, egressNode1, 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		}
		e2e.Logf("egressIPs to use: %s", freeIPs)

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")

		exutil.By("2. Create an egressip object")
		egressip1 := egressIPResource1{
			name:      "egressip-77841",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		var assignedEIPNodev4, assignedEIPNodev6, assignedEIPv6Addr string
		if ipStackType == "dualstack" {
			o.Expect(len(egressIPMaps1) == 2).Should(o.BeTrue())
			for _, eipMap := range egressIPMaps1 {
				if netutils.IsIPv4String(eipMap["egressIP"]) {
					assignedEIPNodev4 = eipMap["node"]
				}
				if netutils.IsIPv6String(eipMap["egressIP"]) {
					assignedEIPNodev6 = eipMap["node"]
					assignedEIPv6Addr = eipMap["egressIP"]
				}
			}
			o.Expect(assignedEIPNodev4).NotTo(o.Equal(""))
			o.Expect(assignedEIPNodev6).NotTo(o.Equal(""))
			e2e.Logf("For the dualstack EIP,  v4 EIP is currently assigned to node: %s, v6 EIP is currently assigned to node: %s", assignedEIPNodev4, assignedEIPNodev6)
		} else {
			o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue())
			assignedEIPNodev6 = egressNode1
			assignedEIPv6Addr = egressIPMaps1[0]["egressIP"]
		}

		exutil.By("3.1 Obtain first namespace, create a second namespace, these two namespaces are for two non-overlapping UDNs")
		ns1 := oc.Namespace()
		oc.SetupProject()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.SetupProject()
		ns3 := oc.Namespace()
		allNS := []string{ns1, ns2, ns3}

		exutil.By("3.3 Apply a label to all namespaces that matches namespaceSelector definied in egressIP object")
		for _, ns := range allNS {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "name=test").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Create two overlapping UDNs in namesapce ns1 and ns2")
		var cidr, ipv4cidr, ipv6cidr []string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv6single" {
			cidr = []string{"2010:100:200::0/60", "2010:100:200::0/60"}
			prefix = 64
		} else {
			ipv4cidr = []string{"10.150.0.0/16", "10.150.0.0/16"}
			ipv4prefix = 24
			ipv6cidr = []string{"2010:100:200::0/60", "2010:100:200::0/60"}
			ipv6prefix = 64
		}

		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		udncrd := make([]udnCRDResource, 2)
		for i := 0; i < 2; i++ {
			if ipStackType == "dualstack" {
				udncrd[i] = udnCRDResource{
					crdname:    udnResourcename[i],
					namespace:  udnNS[i],
					role:       "Primary",
					mtu:        mtu,
					IPv4cidr:   ipv4cidr[i],
					IPv4prefix: ipv4prefix,
					IPv6cidr:   ipv6cidr[i],
					IPv6prefix: ipv6prefix,
					template:   udnCRDL3dualStack,
				}
				udncrd[i].createUdnCRDDualStack(oc)
			} else {
				udncrd[i] = udnCRDResource{
					crdname:   udnResourcename[i],
					namespace: udnNS[i],
					role:      "Primary",
					mtu:       mtu,
					cidr:      cidr[i],
					prefix:    prefix,
					template:  udnCRDSingleStack,
				}
				udncrd[i].createUdnCRDSingleStack(oc)
			}
			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.1 In each namespace, create two test pods, the first one on egressNode, the second one on nonEgressNode ")
		exutil.By("5.2 Apply label to all pods that matches podSelector definied in egressIP object")
		testpods1 := make([]pingPodResourceNode, 3)
		testpods2 := make([]pingPodResourceNode, 3)
		testpods3 := make([]pingPodResourceNode, 3)
		for i := 0; i < 3; i++ {
			if ipStackType == "dualstack" {
				// testpods1 are local pods that co-locate on assignedEIPNodev4 for dualstack
				testpods1[i] = pingPodResourceNode{
					name:      "hello-pod1-" + allNS[i],
					namespace: allNS[i],
					nodename:  assignedEIPNodev4,
					template:  pingPodNodeTemplate,
				}
				testpods1[i].createPingPodNode(oc)
				waitPodReady(oc, allNS[i], testpods1[i].name)
			}

			// testpods2 are local pods that co-locate on assignedEIPNodev6
			testpods2[i] = pingPodResourceNode{
				name:      "hello-pod2-" + allNS[i],
				namespace: allNS[i],
				nodename:  assignedEIPNodev6,
				template:  pingPodNodeTemplate,
			}
			testpods2[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods2[i].name)

			// testpods3 are remote pods on the other non-egress node
			testpods3[i] = pingPodResourceNode{
				name:      "hello-pod3-" + allNS[i],
				namespace: allNS[i],
				nodename:  nonEgressNode,
				template:  pingPodNodeTemplate,
			}
			testpods3[i].createPingPodNode(oc)
			waitPodReady(oc, allNS[i], testpods3[i].name)
		}

		exutil.By("6. Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		e2e.Logf("Trying to get physical interface on the node,%s", egressNode1)
		primaryInf, infErr := getSnifPhyInf(oc, egressNode1)
		o.Expect(infErr).NotTo(o.HaveOccurred())

		for i := 0; i < 3; i++ {
			if ipStackType == "dualstack" {
				exutil.By("6.1 Verify egressIP from IPv4 perspective")
				dstHostv4 := nslookDomainName("ifconfig.me")
				exutil.SetNamespacePrivileged(oc, oc.Namespace())
				tcpdumpCmdv4 := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHostv4)
				_, cmdOnPodv4 := getRequestURL(dstHostv4)
				exutil.By("6.2 Verify v4 egressIP from test pods local to egress node")
				tcpdumOutputv4 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, allNS[i], testpods1[i].name, cmdOnPodv4)
				o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
				exutil.By("6.3 Verify v4 egressIP from test pods remote to egress node")
				tcpdumOutputv4 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, allNS[i], testpods3[i].name, cmdOnPodv4)
				o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
			}

			exutil.By("6.4 Verify egressIP from IPv6 perspective")
			tcpdumpCmdv6 := fmt.Sprintf("timeout 60s tcpdump -c 3 -nni %s ip6 and host %s", primaryInf, dstHostv6)
			_, cmdOnPodv6 := getRequestURL("[" + dstHostv6 + "]")
			exutil.By("6.5 Verify v6 egressIP from test pods local to egress node")
			tcpdumOutputv6 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, allNS[i], testpods2[i].name, cmdOnPodv6)
			o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
			exutil.By("6.6 Verify v6 egressIP from test pods remote to egress node")
			tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, allNS[i], testpods3[i].name, cmdOnPodv6)
			o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-Longduration-NonPreRelease-High-77842-Validate egressIP Failover with non-overlapping and overlapping UDNs (layer3 and IPv6 only) [Serial]", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP2Template         = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1 Get node list, apply EgressLabel Key to one node to make it egressNode")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")

		exutil.By("2 Create an egressip object, verify egressIP is assigned to egress node")
		freeIPs := findFreeIPv6s(oc, egressNodes[0], 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-77842",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNodes[0]))

		exutil.By("3.1 Obtain first namespace, create two more namespaces")
		ns1 := oc.Namespace()
		oc.SetupProject()
		ns2 := oc.Namespace()
		oc.SetupProject()
		ns3 := oc.Namespace()
		udnNS := []string{ns1, ns2, ns3}

		exutil.By("3.2 Apply a label to all namespaces that matches namespaceSelector definied in egressIP object")
		for _, ns := range udnNS {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Create non-overlapping UDNs between ns1 and ns2, overlapping UDN between ns2 and ns3")
		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2, "l3-network-" + ns3}
		cidr := []string{"2010:100:200::0/60", "2011:100:200::0/60", "2011:100:200::0/60"}
		udncrd := make([]udnCRDResource, 3)
		for i := 0; i < 2; i++ {
			udncrd[i] = udnCRDResource{
				crdname:   udnResourcename[i],
				namespace: udnNS[i],
				role:      "Primary",
				mtu:       mtu,
				cidr:      cidr[i],
				prefix:    64,
				template:  udnCRDSingleStack,
			}
			udncrd[i].createUdnCRDSingleStack(oc)
			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("5.1 In each namespace, create two test pods, the first one on egressNode, the second one on nonEgressNode ")
		exutil.By("5.2 Apply label to all pods that matches podSelector definied in egressIP object")
		testpods1 := make([]pingPodResourceNode, 3)
		testpods2 := make([]pingPodResourceNode, 3)
		for i := 0; i < 3; i++ {
			// testpods1 are pods on egressNode
			testpods1[i] = pingPodResourceNode{
				name:      "hello-pod1-" + udnNS[i],
				namespace: udnNS[i],
				nodename:  egressNodes[0],
				template:  pingPodNodeTemplate,
			}
			testpods1[i].createPingPodNode(oc)
			waitPodReady(oc, udnNS[i], testpods1[i].name)
			defer exutil.LabelPod(oc, udnNS[i], testpods1[i].name, "color-")
			err = exutil.LabelPod(oc, udnNS[i], testpods1[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())

			// testpods2 are pods on nonEgressNode, egressNodes[1] is currently not a egress node as it has not been labelled with egressNodeLabel yet
			testpods2[i] = pingPodResourceNode{
				name:      "hello-pod2-" + udnNS[i],
				namespace: udnNS[i],
				nodename:  egressNodes[1],
				template:  pingPodNodeTemplate,
			}
			testpods2[i].createPingPodNode(oc)
			waitPodReady(oc, udnNS[i], testpods2[i].name)
			defer exutil.LabelPod(oc, udnNS[i], testpods2[i].name, "color-")
			err = exutil.LabelPod(oc, udnNS[i], testpods2[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. Verify egressIP from each namespace, egress traffic from these pods should use egressIP as their sourceIP regardless it is from overlapping or non-overlapping UDN")
		var primaryInf, tcpdumpCmd, cmdOnPod string
		var infErr error
		exutil.By("6.1 Use tcpdump to verify egressIP.")
		e2e.Logf("Trying to get physical interface on the egressNode %s", egressNodes[0])
		primaryInf, infErr = getSnifPhyInf(oc, nodeList.Items[0].Name)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 3 -nni %s ip6 and host %s", primaryInf, dstHostv6)
		_, cmdOnPod = getRequestURL("[" + dstHostv6 + "]")

		exutil.By("6.2 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		for i := 0; i < 3; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNodes[0], tcpdumpCmd, udnNS[i], testpods1[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
			tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNodes[0], tcpdumpCmd, udnNS[i], testpods2[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("7. Label the second node with egressNodeLabel, unlabel the first node, verify egressIP still works after failover.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")

		exutil.By("8. Check the egress node was updated in the egressip object.\n")
		egressipErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 360*time.Second, false, func(cxt context.Context) (bool, error) {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps1) != 1 || egressIPMaps1[0]["node"] == egressNodes[0] {
				e2e.Logf("Wait for new egress node applied,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.ContainSubstring(egressNodes[1]))

		exutil.By("9. Validate egressIP again after egressIP failover \n")
		for i := 0; i < 3; i++ {
			exutil.By("9.1 Use tcpdump captured on egressNode to verify egressIP from local pods after egressIP failover")
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNodes[1], tcpdumpCmd, udnNS[i], testpods1[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
			exutil.By("9.2 Use tcpdump captured on egressNode to verify egressIP from remote pods after egressIP failover")
			tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNodes[1], tcpdumpCmd, udnNS[i], testpods2[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-High-78247-egressIP still works correctly after a UDN network gets deleted then recreated (layer3 + v6 or dualstack) [Serial]", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			egressIP1Template         = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			udnCRDL3dualStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			pingPodNodeTemplate       = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			mtu                 int32 = 1300
		)

		exutil.By("1. Get node list, apply EgressLabel Key to one node to make it egressNode, for dualstack, need to label two nodes to be egressNodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		var egressNode1, egressNode2, nonEgressNode string
		var freeIPs []string
		if ipStackType == "dualstack" && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on dualstack cluster, the prerequirement was not fullfilled, skip the case!!")
		}
		if ipStackType == "ipv6single" && len(nodeList.Items) < 2 {
			g.Skip("Need 2 nodes for the test on singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		if ipStackType == "dualstack" {
			egressNode1 = nodeList.Items[0].Name
			egressNode2 = nodeList.Items[1].Name
			nonEgressNode = nodeList.Items[2].Name
			freeIPs = findFreeIPs(oc, egressNode1, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPv6s := findFreeIPv6s(oc, egressNode2, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPs = append(freeIPs, freeIPv6s[0])
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")
		} else if ipStackType == "ipv6single" {
			egressNode1 = nodeList.Items[0].Name
			nonEgressNode = nodeList.Items[1].Name
			freeIPs = findFreeIPv6s(oc, egressNode1, 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		}
		e2e.Logf("egressIPs to use: %s", freeIPs)

		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")

		exutil.By("2. Create an egressip object")
		egressip1 := egressIPResource1{
			name:      "egressip-78247",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)

		// For dualstack, need to find out the actual nodes where v4 and v6 egressIP address are assigned
		var assignedEIPNodev4, assignedEIPNodev6, assignedEIPv6Addr string
		if ipStackType == "dualstack" {
			o.Expect(len(egressIPMaps1) == 2).Should(o.BeTrue())
			for _, eipMap := range egressIPMaps1 {
				if netutils.IsIPv4String(eipMap["egressIP"]) {
					assignedEIPNodev4 = eipMap["node"]
				}
				if netutils.IsIPv6String(eipMap["egressIP"]) {
					assignedEIPNodev6 = eipMap["node"]
					assignedEIPv6Addr = eipMap["egressIP"]
				}
			}
			o.Expect(assignedEIPNodev4).NotTo(o.Equal(""))
			o.Expect(assignedEIPNodev6).NotTo(o.Equal(""))
			e2e.Logf("For the dualstack EIP,  v4 EIP is currently assigned to node: %s, v6 EIP is currently assigned to node: %s", assignedEIPNodev4, assignedEIPNodev6)
		} else {
			o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue())
			assignedEIPNodev6 = egressNode1
			assignedEIPv6Addr = egressIPMaps1[0]["egressIP"]
		}

		exutil.By("3. Obtain a namespace, apply a label to the namespace that matches namespaceSelector definied in egressIP object")
		ns1 := oc.Namespace()

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create an UDN in ns1")
		var cidr, ipv4cidr, ipv6cidr []string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv6single" {
			cidr = []string{"2010:100:200::0/60"}
			prefix = 64
		} else {
			ipv4cidr = []string{"10.150.0.0/16"}
			ipv4prefix = 24
			ipv6cidr = []string{"2010:100:200::0/60"}
			ipv6prefix = 64
		}

		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "l3-network-" + ns1,
				namespace:  ns1,
				role:       "Primary",
				mtu:        mtu,
				IPv4cidr:   ipv4cidr[0],
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr[0],
				IPv6prefix: ipv6prefix,
				template:   udnCRDL3dualStack,
			}
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd = udnCRDResource{
				crdname:   "l3-network-" + ns1,
				namespace: ns1,
				role:      "Primary",
				mtu:       mtu,
				cidr:      cidr[0],
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		udnErr := waitUDNCRDApplied(oc, ns1, udncrd.crdname)
		o.Expect(udnErr).NotTo(o.HaveOccurred())

		exutil.By("5.1 In the namespace, create local test pod on egressNode, create remote test pod on nonEgressNode ")
		var testpod1, testpod2, testpod3 pingPodResourceNode
		var testpods []pingPodResourceNode
		if ipStackType == "dualstack" {
			// testpod1 is local pod on assignedEIPNodev4 for dualstack
			testpod1 = pingPodResourceNode{
				name:      "hello-pod1-" + ns1,
				namespace: ns1,
				nodename:  assignedEIPNodev4,
				template:  pingPodNodeTemplate,
			}
			testpod1.createPingPodNode(oc)
			waitPodReady(oc, ns1, testpod1.name)
			testpods = append(testpods, testpod1)
		}

		// testpod2 is local pod on assignedEIPNodev6 for dualstack
		testpod2 = pingPodResourceNode{
			name:      "hello-pod2-" + ns1,
			namespace: ns1,
			nodename:  assignedEIPNodev6,
			template:  pingPodNodeTemplate,
		}
		testpod2.createPingPodNode(oc)
		waitPodReady(oc, ns1, testpod2.name)
		testpods = append(testpods, testpod2)

		// testpod3 is remote pod on the other non-egress node
		testpod3 = pingPodResourceNode{
			name:      "hello-pod3-" + ns1,
			namespace: ns1,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		testpod3.createPingPodNode(oc)
		waitPodReady(oc, ns1, testpod3.name)
		testpods = append(testpods, testpod3)

		exutil.By("5.2 Apply to all pods with label that matches podSelector definied in egressIP object")
		for i := 0; i < len(testpods); i++ {
			defer exutil.LabelPod(oc, ns1, testpods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, testpods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("6. Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		primaryInf, infErr := getSnifPhyInf(oc, egressNode1)
		o.Expect(infErr).NotTo(o.HaveOccurred())

		var dstHostv4, tcpdumpCmdv4, cmdOnPodv4, tcpdumpCmdv6, cmdOnPodv6 string
		if ipStackType == "dualstack" {
			exutil.By("Verify egressIP from IPv4 perspective")
			dstHostv4 = nslookDomainName("ifconfig.me")
			exutil.SetNamespacePrivileged(oc, oc.Namespace())
			tcpdumpCmdv4 = fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHostv4)
			_, cmdOnPodv4 = getRequestURL(dstHostv4)
			exutil.By("6.1 Verify v4 egressIP from test pods local to egress node")
			tcpdumOutputv4 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod1.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
			exutil.By("6.2 Verify v4 egressIP from test pods remote to egress node")
			tcpdumOutputv4 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod3.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("Verify egressIP from IPv6 perspective")
		tcpdumpCmdv6 = fmt.Sprintf("timeout 60s tcpdump -c 3 -nni %s ip6 and host %s", primaryInf, dstHostv6)
		_, cmdOnPodv6 = getRequestURL("[" + dstHostv6 + "]")
		exutil.By("6.3 Verify v6 egressIP from test pods local to egress node")
		tcpdumOutputv6 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod2.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
		exutil.By("6.4 Verify v6 egressIP from test pods remote to egress node")
		tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod3.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())

		exutil.By("7. Delete local and remote test pods that are associated with UDN in ns1 first, then delete the UDN.\n")
		for i := 0; i < len(testpods); i++ {
			removeResource(oc, true, true, "pod", testpods[i].name, "-n", testpods[i].namespace)
		}
		removeResource(oc, true, true, "UserDefinedNetwork", udncrd.crdname, "-n", udncrd.namespace)

		exutil.By("8. Recreate the UDN and local/remote test pods in ns1.\n")
		if ipStackType == "dualstack" {
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd.createUdnCRDSingleStack(oc)
		}
		udnErr = waitUDNCRDApplied(oc, ns1, udncrd.crdname)
		o.Expect(udnErr).NotTo(o.HaveOccurred())

		for i := 0; i < len(testpods); i++ {
			testpods[i].createPingPodNode(oc)
			waitPodReady(oc, ns1, testpods[i].name)
			defer exutil.LabelPod(oc, ns1, testpods[i].name, "color-")
			err = exutil.LabelPod(oc, ns1, testpods[i].name, "color=pink")
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		err = waitUDNCRDApplied(oc, ns1, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("9. Validate egressIP again from local and remote pods after recreating UDN \n")
		if ipStackType == "dualstack" {
			exutil.By("Verify egressIP from IPv4 perspective")
			exutil.By("9.1 Verify v4 egressIP from test pods local to egress node")
			tcpdumOutputv4 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod1.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
			exutil.By("9.2 Verify v4 egressIP from test pods remote to egress node")
			tcpdumOutputv4 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod3.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("Verify egressIP from IPv6 perspective")
		exutil.By("9.3 Verify v6 egressIP from test pods local to egress node")
		tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod2.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
		exutil.By("9.4 Verify v6 egressIP from test pods remote to egress node")
		tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod3.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
	})

})
