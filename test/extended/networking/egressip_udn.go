package networking

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.CreateNamespaceUDN()
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.CreateNamespaceUDN()
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		oc.CreateNamespaceUDN()
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		oc.CreateNamespaceUDN()
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
		oc.CreateNamespaceUDN()
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

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-High-78200-egressIP still works correctly after OVNK restarted on local and remote client host  (layer3 and IPv4 only) [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP2Template   = filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
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
			name:          "egressip-78200",
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create an UDN in ns1")
		err = applyL3UDNtoNamespace(oc, ns1, 0)
		o.Expect(err).NotTo(o.HaveOccurred())

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

		exutil.By("6. Verify egress traffic from these local or remote egressIP pods should use egressIP as their sourceIP")
		var dstHost, primaryInf, tcpdumpCmd, cmdOnPod string
		var infErr error
		exutil.By("6.1 Use tcpdump to verify egressIP.")
		e2e.Logf("Trying to get physical interface on the egressNode %s", egressNode)
		primaryInf, infErr = getSnifPhyInf(oc, egressNode)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost = nslookDomainName("ifconfig.me")
		tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod = getRequestURL(dstHost)

		exutil.By("6.2 Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods")
		for i := 0; i < 2; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, ns1, testpods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("7. Restart ovnkube-node pod of client host that local egressIP pod is on.\n")
		// Since local egressIP pod is on egress node, restart ovnkube-pod of egress node
		ovnkPod := ovnkubeNodePod(oc, egressNode)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", ovnkPod, "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		exutil.By("8. Validate egressIP again after restarting ovnkude-node pod of client host that local egressIP pod is on \n")
		exutil.By("Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods again after UDN recreation")
		for i := 0; i < 2; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, ns1, testpods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("9. Restart ovnkube-node pod of client host that remote egressIP pod is on.\n")
		// Since remote egressIP pod is on non-egress node, restart ovnkube-pod of the non-egress node nodesToBeUsed[1]
		ovnkPod = ovnkubeNodePod(oc, nodesToBeUsed[1])
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", ovnkPod, "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		exutil.By("10. Validate egressIP again after restarting ovnkude-node pod of client host that remote egressIP pod is on \n")
		exutil.By("Use tcpdump captured on egressNode to verify egressIP from local pods and remote pods again after UDN recreation")
		for i := 0; i < 2; i++ {
			tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, ns1, testpods[i].name, cmdOnPod)
			o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())
		}
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-Longduration-NonPreRelease-High-78293-After reboot egress node EgressIP on UDN still work (layer3 and IPv4). [Disruptive]", func() {
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

		exutil.By("2. Get 1 unused IPs from the same subnet of the egressNode,create an egressIP object")
		freeIPs := findFreeIPs(oc, egressNode, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))

		egressip := egressIPResource1{
			name:          "egressip-78293",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip.createEgressIPObject2(oc)
		defer egressip.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("3. Get a namespace and apply UDN CRD to it")
		oc.CreateNamespaceUDN()
		ns := oc.Namespace()
		oc.CreateNamespaceUDN()

		exutil.By("4. Apply UDN CRD to each namespace,apply to each namespace with label that matches namespaceSelector definied in egressIP object")
		err = applyL3UDNtoNamespace(oc, ns, 0)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. In the namespace, create a test pod, apply to test pod with label that matches podSelector definied in egressIP object")
		testpod := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		testpod.createPingPod(oc)
		waitPodReady(oc, testpod.namespace, testpod.name)
		defer exutil.LabelPod(oc, ns, testpod.name, "color-")
		err = exutil.LabelPod(oc, ns, testpod.name, "color=pink")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. Verify that egress traffic from pod use egressIP as its sourceIP")
		primaryInf, infErr := getSnifPhyInf(oc, egressNode)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost := nslookDomainName("ifconfig.me")
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod := getRequestURL(dstHost)
		tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, ns, testpod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())

		exutil.By("7.Reboot egress node.\n")
		defer checkNodeStatus(oc, egressNode, "Ready")
		rebootNode(oc, egressNode)
		checkNodeStatus(oc, egressNode, "NotReady")
		checkNodeStatus(oc, egressNode, "Ready")
		waitPodReady(oc, testpod.namespace, testpod.name)
		err = exutil.LabelPod(oc, ns, testpod.name, "color=pink")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8. Check EgressIP is assigned again after reboot.\n")
		verifyExpectedEIPNumInEIPObject(oc, egressip.name, 1)

		exutil.By("8. Validate egressIP after node reboot \n")
		tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, egressNode, tcpdumpCmd, ns, testpod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-Longduration-NonPreRelease-High-78422-EgressIP on UDN still works on next available egress node after previous assigned egress node was deleted (layer3 and IPv4 only). [Disruptive]", func() {

		platform := exutil.CheckPlatform(oc)
		if strings.Contains(platform, "baremetal") || strings.Contains(platform, "none") {
			g.Skip("Skip for non-supported auto scaling machineset platforms!!")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		exutil.By("1. Get an existing worker node to be non-egress node.")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("Need at least 1 worker node, skip the test as the requirement was not fulfilled.")
		}
		nonEgressNode := nodeList.Items[0].Name

		exutil.By("2.Create a new machineset with 2 nodes")
		clusterinfra.SkipConditionally(oc)
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-78422"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 2}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		clusterinfra.WaitForMachinesRunning(oc, 2, machinesetName)
		machineNames := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)
		nodeName0 := clusterinfra.GetNodeNameFromMachine(oc, machineNames[0])
		nodeName1 := clusterinfra.GetNodeNameFromMachine(oc, machineNames[1])

		exutil.By("3. Get a namespace and apply UDN CRD to it, apply to the namespace with a label that matches namespaceSelector in egressIP object in step 5")
		oc.CreateNamespaceUDN()
		ns := oc.Namespace()
		err = applyL3UDNtoNamespace(oc, ns, 0)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Apply EgressLabel to the first node created by the new machineset\n")
		// No need to defer unlabeling the node, as the node will be defer deleted with machineset before end of the test case
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeName0, egressNodeLabel, "true")

		exutil.By("5. Get an unused IP address from the first node, create an egressip object with the IP\n")
		freeIPs := findFreeIPs(oc, nodeName0, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))

		egressip := egressIPResource1{
			name:          "egressip-78422",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip.createEgressIPObject2(oc)
		defer egressip.deleteEgressIPObject1(oc)
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))
		o.Expect(egressIPMaps[0]["node"]).Should(o.Equal(nodeName0))

		exutil.By("6. Create a test pod on the non-egress node, apply to the pod with a label that matches podSelector in egressIP object \n")
		testpod := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		testpod.createPingPodNode(oc)
		waitPodReady(oc, ns, testpod.name)
		defer exutil.LabelPod(oc, ns, testpod.name, "color-")
		err = exutil.LabelPod(oc, ns, testpod.name, "color=pink")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7. Get tcpdump on first egress node, verify that egressIP works on first egress node")
		primaryInf, infErr := getSnifPhyInf(oc, nodeName0)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost := nslookDomainName("ifconfig.me")
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod := getRequestURL(dstHost)
		tcpdumOutput := getTcpdumpOnNodeCmdFromPod(oc, machineNames[0], tcpdumpCmd, ns, testpod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())

		exutil.By("8. Apply EgressLabel to the second node created by the new machineset.\n")
		// No need to defer unlabeling the node, as the node will be deleted with machineset before the end of the test case
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeName1, egressNodeLabel, "true")

		exutil.By("9. Delete the first egress node, verify egressIP migrates to the second egress node.\n")
		removeResource(oc, true, true, "machines.machine.openshift.io", machineNames[0], "-n", "openshift-machine-api")

		o.Eventually(func() bool {
			egressIPMaps := getAssignedEIPInEIPObject(oc, egressip.name)
			return len(egressIPMaps) == 1 && egressIPMaps[0]["node"] == nodeName1
		}, "120s", "10s").Should(o.BeTrue(), "egressIP was not migrated to next available egress node!!")

		exutil.By("10. Get tcpdump on second egress node, verify that egressIP still works after migrating to second egress node")
		primaryInf, infErr = getSnifPhyInf(oc, nodeName1)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		tcpdumpCmd = fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s host %s", primaryInf, dstHost)
		_, cmdOnPod = getRequestURL(dstHost)
		tcpdumOutput = getTcpdumpOnNodeCmdFromPod(oc, nodeName1, tcpdumpCmd, ns, testpod.name, cmdOnPod)
		o.Expect(strings.Contains(tcpdumOutput, freeIPs[0])).To(o.BeTrue())

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-High-78453-Traffic is load balanced between egress nodes for egressIP UDN (layer3 and IPv4 only) .[Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		exutil.By("1. Get two worker nodes that are in same subnet, they will be used as egress-assignable nodes, get a third node as non-egress node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 || len(nodeList.Items) < 3 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		var nonEgressNode string
		for _, node := range nodeList.Items {
			if !contains(egressNodes, node.Name) {
				nonEgressNode = node.Name
				break
			}
		}

		exutil.By("2. Apply EgressLabel Key to nodes.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)

		exutil.By("3 Obtain first namespace, apply CRD and label to it ")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		err = applyL3UDNtoNamespace(oc, ns1, 0)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNodes[0], 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-78453",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		//Replce matchLabel with matchExpressions
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-78453", "-p", "{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"name\", \"operator\": \"In\", \"values\": [\"test\"]}]}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-78453", "-p", "{\"spec\":{\"namespaceSelector\":{\"matchLabels\":null}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 2)

		exutil.By("5. Create two pods, one pod is local to egress node, another pod is remote to egress node ")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1-" + ns1,
			namespace: ns1,
			nodename:  egressNodes[0],
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		pod2 := pingPodResourceNode{
			name:      "hello-pod2-" + ns1,
			namespace: ns1,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod2.name)

		exutil.By("6. Check source IP is randomly one of egress ips.\n")
		exutil.By("6.1 Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump", "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump", "true")
		primaryInf, infErr := getSnifPhyInf(oc, egressNodes[0])
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost := nslookDomainName("ifconfig.me")
		defer deleteTcpdumpDS(oc, "tcpdump-78453", ns1)
		tcpdumpDS, snifErr := createSnifferDaemonset(oc, ns1, "tcpdump-78453", "tcpdump", "true", dstHost, primaryInf, 80)
		o.Expect(snifErr).NotTo(o.HaveOccurred())

		exutil.By("6.2 Verify egressIP load balancing from local pod.")
		egressipErr := wait.PollUntilContextTimeout(context.Background(), 100*time.Second, 100*time.Second, false, func(cxt context.Context) (bool, error) {
			randomStr, url := getRequestURL(dstHost)
			_, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			if checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIPs[0], true) != nil || checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIPs[1], true) != nil || err != nil {
				e2e.Logf("No matched egressIPs in tcpdump log, try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get both EgressIPs %s,%s in tcpdump for local pod %s", freeIPs[0], freeIPs[1], pod1.name))

		exutil.By("6.3 Verify egressIP load balancing from remote pod.")
		egressipErr = wait.PollUntilContextTimeout(context.Background(), 100*time.Second, 100*time.Second, false, func(cxt context.Context) (bool, error) {
			randomStr, url := getRequestURL(dstHost)
			_, err := execCommandInSpecificPod(oc, pod2.namespace, pod2.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			if checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIPs[0], true) != nil || checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIPs[1], true) != nil || err != nil {
				e2e.Logf("No matched egressIPs in tcpdump log, try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get both EgressIPs %s,%s in tcpdump for local pod %s", freeIPs[0], freeIPs[1], pod2.name))
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.CreateNamespaceUDN()
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
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		udnNS := []string{ns1, ns2}

		exutil.By("3.2 Create a third namespace, it will be used to validate egressIP from default network")
		oc.CreateNamespaceUDN()
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		oc.CreateNamespaceUDN()
		ns2 := oc.Namespace()
		oc.CreateNamespaceUDN()
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
		oc.CreateNamespaceUDN()
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

	// author: jechen@redhat.com
	g.It("Author:jechen-ConnectedOnly-NonPreRelease-High-78274-egressIP still works correctly after OVNK restarted on local and remote client host (layer3 + v6 or dualstack) [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP1Template   = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
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
			name:      "egressip-78274",
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
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create an UDN in ns1")
		err = applyL3UDNtoNamespace(oc, ns1, 0)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5.1 In the namespace, create local test pod on egressNode, create remote test pod on nonEgressNode ")
		var testpod1, testpod2, testpod3 pingPodResourceNode
		// var testpods []pingPodResourceNode
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
			// testpods = append(testpods, testpod1)
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
		// testpods = append(testpods, testpod2)

		// testpod3 is remote pod on the other non-egress node
		testpod3 = pingPodResourceNode{
			name:      "hello-pod3-" + ns1,
			namespace: ns1,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		testpod3.createPingPodNode(oc)
		waitPodReady(oc, ns1, testpod3.name)
		// testpods = append(testpods, testpod3)

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

		exutil.By("7. Restart ovnkube-node pod of client host that local egressIP pod is on.\n")
		// Since local egressIP pod is on egress node, so just to restart ovnkube-pod of egress node
		ovnkPod := ovnkubeNodePod(oc, egressNode1)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", ovnkPod, "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		if ipStackType == "dualstack" {
			ovnkPod := ovnkubeNodePod(oc, egressNode2)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", ovnkPod, "-n", "openshift-ovn-kubernetes").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		}

		exutil.By("8. Validate egressIP again from local and remote pods after recreating UDN \n")
		if ipStackType == "dualstack" {
			exutil.By("Verify egressIP from IPv4 perspective")
			exutil.By("8.1 Verify v4 egressIP from test pods local to egress node")
			tcpdumOutputv4 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod1.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
			exutil.By("8.2 Verify v4 egressIP from test pods remote to egress node")
			tcpdumOutputv4 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod3.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("Verify egressIP from IPv6 perspective")
		exutil.By("8.3 Verify v6 egressIP from test pods local to egress node")
		tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod2.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
		exutil.By("8.4 Verify v6 egressIP from test pods remote to egress node")
		tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod3.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())

		exutil.By("9. Restart ovnkube-node pod of client host that remote egressIP pod is on.\n")
		// Since local egressIP pod is on egress node, so just to restart ovnkube-pod of egress node
		ovnkPod = ovnkubeNodePod(oc, nonEgressNode)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", ovnkPod, "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		exutil.By("10. Validate egressIP again from local and remote pods after recreating UDN \n")
		if ipStackType == "dualstack" {
			exutil.By("Verify egressIP from IPv4 perspective")
			exutil.By("10.1 Verify v4 egressIP from test pods local to egress node")
			tcpdumOutputv4 := getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod1.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
			exutil.By("10.2 Verify v4 egressIP from test pods remote to egress node")
			tcpdumOutputv4 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev4, tcpdumpCmdv4, ns1, testpod3.name, cmdOnPodv4)
			o.Expect(strings.Contains(tcpdumOutputv4, freeIPs[0])).To(o.BeTrue())
		}

		exutil.By("Verify egressIP from IPv6 perspective")
		exutil.By("10.3 Verify v6 egressIP from test pods local to egress node")
		tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod2.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
		exutil.By("10.4 Verify v6 egressIP from test pods remote to egress node")
		tcpdumOutputv6 = getTcpdumpOnNodeCmdFromPod(oc, assignedEIPNodev6, tcpdumpCmdv6, ns1, testpod3.name, cmdOnPodv6)
		o.Expect(strings.Contains(tcpdumOutputv6, assignedEIPv6Addr)).To(o.BeTrue())
	})
})

var _ = g.Describe("[sig-networking] SDN udn EgressIP genetic", func() {
	//Test cases in this function do not need external bashion host
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
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-NonHyperShiftHOST-ConnectedOnly-High-78663-Pods on default network and UDNs can access k8s service when its node is egressIP node [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressIP1Template   = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)

		exutil.By("1. Get node list")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		var egressNode1, egressNode2, nonEgressNode string
		var freeIPs []string
		ipStackType := checkIPStackType(oc)
		if ipStackType == "dualstack" && len(nodeList.Items) < 3 {
			g.Skip("Need 3 nodes for the test on dualstack cluster, the prerequirement was not fullfilled, skip the case!!")
		} else if (ipStackType == "ipv4single" || ipStackType == "ipv6single") && len(nodeList.Items) < 2 {
			g.Skip("Need 2 nodes for the test on singlev4 or singlev6 cluster, the prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("2.1 Get a namespace for default network, Create two more namespaces for two overlapping UDN")
		oc.CreateNamespaceUDN()
		ns1 := oc.Namespace()
		var udnNS []string
		for i := 0; i < 2; i++ {
			oc.CreateNamespaceUDN()
			ns := oc.Namespace()
			udnNS = append(udnNS, ns)
		}
		allNS := append(udnNS, ns1)
		e2e.Logf("allNS: %v", allNS)

		exutil.By("2.2 Apply a label to all namespaces that matches namespaceSelector defined in egressIP object")
		for _, ns := range allNS {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.By("2.2. Create two overlapping UDNs between two UDN namespaces")
			if ns != ns1 {
				err = applyL3UDNtoNamespace(oc, ns, 0)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}

		exutil.By("3. Apply EgressLabel Key to egressNode.  Two egress nodes are needed for dualstack egressIP object")
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
		} else if ipStackType == "ipv4single" {
			egressNode1 = nodeList.Items[0].Name
			nonEgressNode = nodeList.Items[1].Name
			freeIPs = findFreeIPs(oc, egressNode1, 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		}
		e2e.Logf("egressIPs to use: %s", freeIPs)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")

		exutil.By("4. Create an egressip object")
		egressip1 := egressIPResource1{
			name:      "egressip-78663",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)

		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		var assignedEIPNodev4, assignedEIPNodev6, assignedEIPNode string
		if ipStackType == "dualstack" {
			o.Expect(len(egressIPMaps1) == 2).Should(o.BeTrue())
			for _, eipMap := range egressIPMaps1 {
				if netutils.IsIPv4String(eipMap["egressIP"]) {
					assignedEIPNodev4 = eipMap["node"]
				}
				if netutils.IsIPv6String(eipMap["egressIP"]) {
					assignedEIPNodev6 = eipMap["node"]
				}
			}
			o.Expect(assignedEIPNodev4).NotTo(o.Equal(""))
			o.Expect(assignedEIPNodev6).NotTo(o.Equal(""))
			e2e.Logf("For the dualstack EIP,  v4 EIP is currently assigned to node: %s, v6 EIP is currently assigned to node: %s", assignedEIPNodev4, assignedEIPNodev6)
		} else {
			o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue())
			assignedEIPNode = egressNode1
		}

		exutil.By("5. On each of egress node(s) and nonEgressNode, create a test pod, curl k8s service from each pod")
		var nodeNames []string
		if ipStackType == "dualstack" {
			nodeNames = []string{assignedEIPNodev4, assignedEIPNodev6, nonEgressNode}
		} else {
			nodeNames = []string{assignedEIPNode, nonEgressNode}
		}
		e2e.Logf("nodeNames: %s , length of nodeName is: %d", nodeNames, len(nodeNames))

		var testpods [3][3]pingPodResourceNode
		for j := 0; j < len(allNS); j++ {
			for i := 0; i < len(nodeNames); i++ {
				testpods[j][i] = pingPodResourceNode{
					name:      "hello-pod" + strconv.Itoa(i) + "-" + allNS[j],
					namespace: allNS[j],
					nodename:  nodeNames[i],
					template:  pingPodNodeTemplate,
				}
				testpods[j][i].createPingPodNode(oc)
				waitPodReady(oc, allNS[j], testpods[j][i].name)
			}
		}

		svcIP1, svcIP2 := getSvcIP(oc, "default", "kubernetes")
		e2e.Logf("k8s service has IP(s) as svcIP1: %s, svcIP2: %s", svcIP1, svcIP2)

		var curlCmd string
		if svcIP2 != "" {
			curlCmdv6 := fmt.Sprintf("curl -I -k -v https://[%s]:443/api?timeout=32s", svcIP1)
			curlCmdv4 := fmt.Sprintf("curl -I -k -v https://%s:443/api?timeout=32s", svcIP2)
			for j := 0; j < len(allNS); j++ {
				for i := 0; i < len(nodeNames); i++ {
					_, curlErr := e2eoutput.RunHostCmd(testpods[j][i].namespace, testpods[j][i].name, curlCmdv6)
					o.Expect(curlErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to curl k8s service from pod %s", testpods[j][i].name))
					_, curlErr = e2eoutput.RunHostCmd(testpods[j][i].namespace, testpods[j][i].name, curlCmdv4)
					o.Expect(curlErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to curl k8s service from pod %s", testpods[j][i].name))
				}
			}
		} else {
			curlCmd = fmt.Sprintf("curl -I -k -v https://%s/api?timeout=32s", net.JoinHostPort(svcIP1, "443"))
			for j := 0; j < len(allNS); j++ {
				for i := 0; i < len(nodeNames); i++ {
					_, curlErr := e2eoutput.RunHostCmd(testpods[j][i].namespace, testpods[j][i].name, curlCmd)
					o.Expect(curlErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to curl k8s service from pod %s", testpods[j][i].name))
				}
			}
		}
	})
})
