package networking

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
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

})
