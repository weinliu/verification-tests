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
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN udn pods", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("networking-udn", exutil.KubeConfigPath())
		testDataDirUDN = exutil.FixturePath("testdata", "networking/udn")
	)

	g.BeforeEach(func() {

		SkipIfNoFeatureGate(oc, "NetworkSegmentation")

		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
	})

	g.It("Author:anusaxen-Critical-74921-Check udn pods isolation on user defined networks", func() {
		var (
			udnNadtemplate       = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate       = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			mtu            int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		nadNS := []string{ns1, ns2}
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.151.0.0/16/24"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.151.0.0/16/24,2011:100:200::0/60"}
			}
		}

		nad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[i],
				topology:            "layer3",
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("create a udn hello pod in ns2")
		pod2 := udnPodResource{
			name:      "hello-pod-ns2",
			namespace: ns2,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}

		pod2.createUdnPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		//udn network connectivity should be isolated
		CurlPod2PodFailUDN(oc, ns1, pod1.name, ns2, pod2.name)
		//default network connectivity should also be isolated
		CurlPod2PodFail(oc, ns1, pod1.name, ns2, pod2.name)

	})

	g.It("Author:anusaxen-Critical-75236-Check udn pods are not isolated if same nad network is shared across two namespaces", func() {
		var (
			udnNadtemplate       = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate       = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			mtu            int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		nadNS := []string{ns1, ns2}
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.150.0.0/16/24"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2010:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.150.0.0/16/24,2010:100:200::0/60"}
			}
		}

		nad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    "l3-network-ns1", //Keeping same nad network name across all which is l3-network-ns1
				topology:            "layer3",
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("create a udn hello pod in ns2")
		pod2 := udnPodResource{
			name:      "hello-pod-ns2",
			namespace: ns2,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}

		pod2.createUdnPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		//udn network connectivity should NOT be isolated
		CurlPod2PodPassUDN(oc, ns1, pod1.name, ns2, pod2.name)
		//default network connectivity should be isolated
		CurlPod2PodFail(oc, ns1, pod1.name, ns2, pod2.name)
	})

	g.It("Author:huirwang-High-75223-Restarting ovn pods should not break UDN primary network traffic.[Disruptive]", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnNadtemplate            = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			testPodFile               = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			mtu                 int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		nadNS := []string{ns1, ns2}
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.151.0.0/16/24"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.151.0.0/16/24,2011:100:200::0/60"}
			}
		}

		nad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[i],
				topology:            "layer3",
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
			exutil.By("Verifying the configued NetworkAttachmentDefinition")
			if checkNAD(oc, nadNS[i], nadResourcename[i]) {
				e2e.Logf("The correct network-attach-defintion: %v is created!", nadResourcename[i])
			} else {
				e2e.Failf("The correct network-attach-defintion: %v is not created!", nadResourcename[i])
			}
		}

		exutil.By("Create replica pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		err := waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS1Names := getPodName(oc, ns1, "name=test-pods")
		CurlPod2PodPassUDN(oc, ns1, testpodNS1Names[0], ns1, testpodNS1Names[1])

		exutil.By("create replica pods in ns2")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS2Names := getPodName(oc, ns2, "name=test-pods")
		CurlPod2PodPassUDN(oc, ns2, testpodNS2Names[0], ns2, testpodNS2Names[1])

		exutil.By("Restart OVN pods")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "--all", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertAllPodsToBeReady(oc, "openshift-ovn-kubernetes")

		exutil.By("Verify the connection in UDN primary network not broken.")
		CurlPod2PodPassUDN(oc, ns1, testpodNS1Names[0], ns1, testpodNS1Names[1])
		CurlPod2PodPassUDN(oc, ns2, testpodNS2Names[0], ns2, testpodNS2Names[1])
	})

	g.It("Author:huirwang-Medium-75238-NAD can be created with secondary role with primary UDN in same namespace.", func() {
		var (
			udnNadtemplate  = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate  = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			pingPodTemplate = filepath.Join(testDataDirUDN, "udn_test_pod_annotation_template.yaml")

			mtu int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l3-network-2-" + ns1}
		role := []string{"primary", "secondary"}
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.161.0.0/16/24"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.151.0.0/16/24,2011:100:200::0/60"}
			}
		}

		nad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], ns1))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           ns1,
				nad_network_name:    nadResourcename[i],
				topology:            "layer3",
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: ns1 + "/" + nadResourcename[i],
				role:                role[i],
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
			exutil.By("Verifying the configued NetworkAttachmentDefinition")
			if checkNAD(oc, ns1, nadResourcename[i]) {
				e2e.Logf("The correct network-attach-defintion: %v is created!", nadResourcename[i])
			} else {
				e2e.Failf("The correct network-attach-defintion: %v is not created!", nadResourcename[i])
			}
		}

		exutil.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("create a hello pod in ns1 refers to secondary udn network")
		pod2 := udnPodSecNADResource{
			name:       "hello-pod-ns1-2",
			namespace:  ns1,
			label:      "hello-pod",
			annotation: "/l3-network-2-" + ns1,
			template:   pingPodTemplate,
		}
		pod2.createUdnPodWithSecNAD(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		exutil.By("Verify the two pods between primary and udn networks work well")
		CurlPod2PodPassUDN(oc, ns1, pod1.name, ns1, pod2.name)

		exutil.By("Verify the pod2 has secondary network, but pod1 doesn't. ")
		pod1IPs, err := execCommandInSpecificPod(oc, ns1, pod1.name, "ip a")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(pod1IPs, "net1@")).NotTo(o.BeTrue())
		pod2IPs, err := execCommandInSpecificPod(oc, ns1, pod2.name, "ip a")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(pod2IPs, "net1@")).To(o.BeTrue())
	})

	g.It("Author:huirwang-Medium-75658-Check sctp traffic work well via udn pods user defined networks for laye3.	[Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			sctpClientPod       = filepath.Join(buildPruningBaseDir, "sctp/sctpclient.yaml")
			sctpServerPod       = filepath.Join(buildPruningBaseDir, "sctp/sctpserver.yaml")
			sctpModule          = filepath.Join(buildPruningBaseDir, "sctp/load-sctp-module.yaml")
			udnCRDdualStack     = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			udnCRDSingleStack   = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			sctpServerPodName   = "sctpserver"
			sctpClientPodname   = "sctpclient"
		)
		exutil.By("Preparing the nodes for SCTP")
		prepareSCTPModule(oc, sctpModule)

		ipStackType := checkIPStackType(oc)

		exutil.By("Setting privileges on the namespace")
		ns := oc.Namespace()

		var cidr, ipv4cidr, ipv6cidr string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
				prefix = 64
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv4prefix = 24
				ipv6cidr = "2010:100:200::0/48"
				ipv6prefix = 64
			}
		}

		exutil.By("Create CRD for UDN")
		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "udn-network-75658",
				namespace:  ns,
				role:       "Primary",
				mtu:        1400,
				IPv4cidr:   ipv4cidr,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr,
				IPv6prefix: ipv6prefix,
				template:   udnCRDdualStack,
			}
			udncrd.createUdnCRDDualStack(oc)

		} else {
			udncrd = udnCRDResource{
				crdname:   "udn-network-75658",
				namespace: ns,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		err := waitUDNCRDApplied(oc, ns, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer exutil.RecoverNamespaceRestricted(oc, ns)
		exutil.SetNamespacePrivileged(oc, ns)

		exutil.By("create sctpClientPod")
		createResourceFromFile(oc, ns, sctpClientPod)
		err1 := waitForPodWithLabelReady(oc, ns, "name=sctpclient")
		exutil.AssertWaitPollNoErr(err1, "sctpClientPod is not running")

		exutil.By("create sctpServerPod")
		createResourceFromFile(oc, ns, sctpServerPod)
		err2 := waitForPodWithLabelReady(oc, ns, "name=sctpserver")
		exutil.AssertWaitPollNoErr(err2, "sctpServerPod is not running")

		exutil.By("Verify sctp server pod can be accessed for UDN network.")
		if ipStackType == "dualstack" {
			sctpServerIPv6, sctpServerIPv4 := getPodIPUDN(oc, ns, sctpServerPodName, "ovn-udn1")
			verifySctpConnPod2IP(oc, ns, sctpServerIPv4, sctpServerPodName, sctpClientPodname, true)
			verifySctpConnPod2IP(oc, ns, sctpServerIPv6, sctpServerPodName, sctpClientPodname, true)
		} else {
			sctpServerIP, _ := getPodIPUDN(oc, ns, sctpServerPodName, "ovn-udn1")
			verifySctpConnPod2IP(oc, ns, sctpServerIP, sctpServerPodName, sctpClientPodname, true)
		}
	})

	g.It("Author:weliang-Medium-75623-Feature Integration UDN with multus. [Disruptive]", func() {
		var (
			udnNadtemplate               = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			mtu                    int32 = 1300
			buildPruningBaseDir          = exutil.FixturePath("testdata", "networking")
			dualstackNADTemplate         = filepath.Join(buildPruningBaseDir, "multus/dualstack-NAD-template.yaml")
			multihomingPodTemplate       = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
		)

		exutil.By("Getting the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("The cluster has no ready node for the testing")
		}

		ns1 := oc.Namespace()

		exutil.By("Creating NAD1 for ns1")
		nad1 := udnNetDefResource{
			nadname:             "udn-primary-net",
			namespace:           ns1,
			nad_network_name:    "udn-primary-net",
			topology:            "layer3",
			subnet:              "10.100.0.0/16/24",
			mtu:                 mtu,
			net_attach_def_name: ns1 + "/" + "udn-primary-net",
			role:                "primary",
			template:            udnNadtemplate,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nad1.nadname, "-n", ns1).Execute()
		nad1.createUdnNad(oc)

		exutil.By("Verifying the configured NAD1")
		if checkNAD(oc, ns1, nad1.nadname) {
			e2e.Logf("The correct network-attach-definition: %v is created!", nad1.nadname)
		} else {
			e2e.Failf("The correct network-attach-definition: %v is not created!", nad1.nadname)
		}

		exutil.By("Creating NAD2 for ns1")
		nad2 := dualstackNAD{
			nadname:        "dualstack",
			namespace:      ns1,
			plugintype:     "macvlan",
			mode:           "bridge",
			ipamtype:       "whereabouts",
			ipv4range:      "192.168.10.0/24",
			ipv6range:      "fd00:dead:beef:10::/64",
			ipv4rangestart: "",
			ipv4rangeend:   "",
			ipv6rangestart: "",
			ipv6rangeend:   "",
			template:       dualstackNADTemplate,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nad2.nadname, "-n", ns1).Execute()
		nad2.createDualstackNAD(oc)

		exutil.By("Verifying the configured NAD2")
		if checkNAD(oc, ns1, nad2.nadname) {
			e2e.Logf("The correct network-attach-definition: %v is created!", nad2.nadname)
		} else {
			e2e.Failf("The correct network-attach-definition: %v is not created!", nad2.nadname)
		}

		exutil.By("Configuring pod1 for additional network using NAD2")
		pod1 := testMultihomingPod{
			name:       "dualstack-pod-1",
			namespace:  ns1,
			podlabel:   "dualstack-pod1",
			nadname:    "dualstack",
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod1.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=dualstack-pod1")).NotTo(o.HaveOccurred())

		exutil.By("Configuring pod2 for additional network using NAD2")
		pod2 := testMultihomingPod{
			name:       "dualstack-pod-2",
			namespace:  ns1,
			podlabel:   "dualstack-pod2",
			nadname:    "dualstack",
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod2.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=dualstack-pod2")).NotTo(o.HaveOccurred())

		exutil.By("Getting two pods' names")
		podList, podListErr := exutil.GetAllPods(oc, ns1)
		o.Expect(podListErr).NotTo(o.HaveOccurred())
		o.Expect(len(podList)).Should(o.Equal(2))

		exutil.By("Verifying the pod1 has the primary network")
		pod1IPs, err := execCommandInSpecificPod(oc, ns1, podList[0], "ip a")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(pod1IPs, "ovn-udn1@")).To(o.BeTrue())

		exutil.By("Verifying the pod2 has the primary network")
		pod2IPs, err := execCommandInSpecificPod(oc, ns1, podList[1], "ip a")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(pod2IPs, "ovn-udn1@")).To(o.BeTrue())

		exutil.By("Getting IPs from pod1's secondary interface")
		pod1v4, pod1v6 := getPodMultiNetwork(oc, ns1, podList[0])

		exutil.By("Getting IPs from pod2's secondary interface")
		pod2v4, pod2v6 := getPodMultiNetwork(oc, ns1, podList[1])

		exutil.By("Verifying both ipv4 and ipv6 communication between two pods through their secondary interface")
		curlPod2PodMultiNetworkPass(oc, ns1, podList[0], pod2v4, pod2v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[1], pod1v4, pod1v6)
	})

	g.It("Author:huirwang-Medium-75239-Check sctp traffic work well via udn pods user defined networks for layer2.	[Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			sctpClientPod       = filepath.Join(buildPruningBaseDir, "sctp/sctpclient.yaml")
			sctpServerPod       = filepath.Join(buildPruningBaseDir, "sctp/sctpserver.yaml")
			sctpModule          = filepath.Join(buildPruningBaseDir, "sctp/load-sctp-module.yaml")
			udnCRDdualStack     = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnCRDSingleStack   = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_singlestack_template.yaml")
			sctpServerPodName   = "sctpserver"
			sctpClientPodname   = "sctpclient"
		)
		exutil.By("Preparing the nodes for SCTP")
		prepareSCTPModule(oc, sctpModule)

		ipStackType := checkIPStackType(oc)

		exutil.By("Setting privileges on the namespace")
		ns := oc.Namespace()

		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
			}
		}

		exutil.By("Create CRD for UDN")
		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:   "udn-network-75239",
				namespace: ns,
				role:      "Primary",
				mtu:       1400,
				IPv4cidr:  ipv4cidr,
				IPv6cidr:  ipv6cidr,
				template:  udnCRDdualStack,
			}
			udncrd.createLayer2DualStackUDNCRD(oc)

		} else {
			udncrd = udnCRDResource{
				crdname:   "udn-network-75658",
				namespace: ns,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				template:  udnCRDSingleStack,
			}
			udncrd.createLayer2SingleStackUDNCRD(oc)
		}

		err := waitUDNCRDApplied(oc, ns, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("create sctpClientPod")
		createResourceFromFile(oc, ns, sctpClientPod)
		err1 := waitForPodWithLabelReady(oc, ns, "name=sctpclient")
		exutil.AssertWaitPollNoErr(err1, "sctpClientPod is not running")

		exutil.By("create sctpServerPod")
		createResourceFromFile(oc, ns, sctpServerPod)
		err2 := waitForPodWithLabelReady(oc, ns, "name=sctpserver")
		exutil.AssertWaitPollNoErr(err2, "sctpServerPod is not running")

		exutil.By("Verify sctp server pod can be accessed for UDN network.")
		if ipStackType == "dualstack" {
			sctpServerIPv6, sctpServerIPv4 := getPodIPUDN(oc, ns, sctpServerPodName, "ovn-udn1")
			verifySctpConnPod2IP(oc, ns, sctpServerIPv4, sctpServerPodName, sctpClientPodname, true)
			verifySctpConnPod2IP(oc, ns, sctpServerIPv6, sctpServerPodName, sctpClientPodname, true)
		} else {
			sctpServerIP, _ := getPodIPUDN(oc, ns, sctpServerPodName, "ovn-udn1")
			verifySctpConnPod2IP(oc, ns, sctpServerIP, sctpServerPodName, sctpClientPodname, true)
		}

	})

	g.It("Author:qiowang-High-75254-Check kubelet probes are allowed via default network's LSP for the UDN pods", func() {
		var (
			udnCRDdualStack         = filepath.Join(testDataDirUDN, "udn_crd_dualstack2_template.yaml")
			udnCRDSingleStack       = filepath.Join(testDataDirUDN, "udn_crd_singlestack_template.yaml")
			udnPodLivenessTemplate  = filepath.Join(testDataDirUDN, "udn_test_pod_liveness_template.yaml")
			udnPodReadinessTemplate = filepath.Join(testDataDirUDN, "udn_test_pod_readiness_template.yaml")
			udnPodStartupTemplate   = filepath.Join(testDataDirUDN, "udn_test_pod_startup_template.yaml")
			livenessProbePort       = 8080
			readinessProbePort      = 8081
			startupProbePort        = 1234
		)

		exutil.By("1. Create privileged namespace")
		ns := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns)

		exutil.By("2. Create CRD for UDN")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
				prefix = 64
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv4prefix = 24
				ipv6cidr = "2010:100:200::0/48"
				ipv6prefix = 64
			}
		}
		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "udn-network-ds-75254",
				namespace:  ns,
				role:       "Primary",
				mtu:        1400,
				IPv4cidr:   ipv4cidr,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr,
				IPv6prefix: ipv6prefix,
				template:   udnCRDdualStack,
			}
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd = udnCRDResource{
				crdname:   "udn-network-ss-75254",
				namespace: ns,
				role:      "Primary",
				mtu:       1400,
				cidr:      cidr,
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		err := waitUDNCRDApplied(oc, ns, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Create a udn hello pod with liveness probe in ns1")
		pod1 := udnPodWithProbeResource{
			name:             "hello-pod-ns1-liveness",
			namespace:        ns,
			label:            "hello-pod",
			port:             livenessProbePort,
			failurethreshold: 1,
			periodseconds:    1,
			template:         udnPodLivenessTemplate,
		}
		pod1.createUdnPodWithProbe(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("4. Capture packets in pod " + pod1.name + ", check liveness probe traffic is allowed via default network")
		tcpdumpCmd1 := fmt.Sprintf("timeout 5s tcpdump -nni eth0 port %v", pod1.port)
		cmdTcpdump1, cmdOutput1, _, err1 := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, pod1.name, "--", "bash", "-c", tcpdumpCmd1).Background()
		defer cmdTcpdump1.Process.Kill()
		o.Expect(err1).NotTo(o.HaveOccurred())
		cmdTcpdump1.Wait()
		e2e.Logf("The captured packet is %s", cmdOutput1.String())
		expPacket1 := strconv.Itoa(pod1.port) + ": Flags [S]"
		o.Expect(strings.Contains(cmdOutput1.String(), expPacket1)).To(o.BeTrue())

		exutil.By("5. Create a udn hello pod with readiness probe in ns1")
		pod2 := udnPodWithProbeResource{
			name:             "hello-pod-ns1-readiness",
			namespace:        ns,
			label:            "hello-pod",
			port:             readinessProbePort,
			failurethreshold: 1,
			periodseconds:    1,
			template:         udnPodReadinessTemplate,
		}
		pod2.createUdnPodWithProbe(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		exutil.By("6. Capture packets in pod " + pod2.name + ", check readiness probe traffic is allowed via default network")
		tcpdumpCmd2 := fmt.Sprintf("timeout 5s tcpdump -nni eth0 port %v", pod2.port)
		cmdTcpdump2, cmdOutput2, _, err2 := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, pod2.name, "--", "bash", "-c", tcpdumpCmd2).Background()
		defer cmdTcpdump2.Process.Kill()
		o.Expect(err2).NotTo(o.HaveOccurred())
		cmdTcpdump2.Wait()
		e2e.Logf("The captured packet is %s", cmdOutput2.String())
		expPacket2 := strconv.Itoa(pod2.port) + ": Flags [S]"
		o.Expect(strings.Contains(cmdOutput2.String(), expPacket2)).To(o.BeTrue())

		exutil.By("7. Create a udn hello pod with startup probe in ns1")
		pod3 := udnPodWithProbeResource{
			name:             "hello-pod-ns1-startup",
			namespace:        ns,
			label:            "hello-pod",
			port:             startupProbePort,
			failurethreshold: 100,
			periodseconds:    2,
			template:         udnPodStartupTemplate,
		}
		pod3.createUdnPodWithProbe(oc)
		waitPodReady(oc, pod3.namespace, pod3.name)

		exutil.By("8. Capture packets in pod " + pod3.name + ", check readiness probe traffic is allowed via default network")
		tcpdumpCmd3 := fmt.Sprintf("timeout 10s tcpdump -nni eth0 port %v", pod3.port)
		cmdTcpdump3, cmdOutput3, _, err3 := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, pod3.name, "--", "bash", "-c", tcpdumpCmd3).Background()
		defer cmdTcpdump3.Process.Kill()
		o.Expect(err3).NotTo(o.HaveOccurred())
		cmdTcpdump3.Wait()
		e2e.Logf("The captured packet is %s", cmdOutput3.String())
		expPacket3 := strconv.Itoa(pod3.port) + ": Flags [S]"
		o.Expect(strings.Contains(cmdOutput3.String(), expPacket3)).To(o.BeTrue())
	})

	g.It("Author:anusaxen-Critical-75876-Check udn pods are not isolated if same nad network is shared across two namespaces(layer 2)", func() {
		var (
			udnNadtemplate       = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate       = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			mtu            int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l2-network-" + ns1, "l2-network-" + ns2}
		nadNS := []string{ns1, ns2}
		var subnet string
		if ipStackType == "ipv4single" {
			subnet = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				subnet = "2010:100:200::0/60"
			} else {
				subnet = "10.150.0.0/16,2010:100:200::0/60"
			}
		}

		nad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    "l2-network",
				topology:            "layer2",
				subnet:              subnet,
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("create a udn hello pod in ns2")
		pod2 := udnPodResource{
			name:      "hello-pod-ns2",
			namespace: ns2,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}

		pod2.createUdnPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		//udn network connectivity should NOT be isolated
		CurlPod2PodPassUDN(oc, ns1, pod1.name, ns2, pod2.name)
		//default network connectivity should be isolated
		CurlPod2PodFail(oc, ns1, pod1.name, ns2, pod2.name)
	})

	g.It("Author:anusaxen-Critical-75875-Check udn pods isolation on user defined networks (layer 2)", func() {
		var (
			udnNadtemplate       = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate       = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			mtu            int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l2-network-" + ns1, "l2-network-" + ns2}
		nadNS := []string{ns1, ns2}
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16", "10.151.0.0/16"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16,2010:100:200::0/60", "10.151.0.0/16,2011:100:200::0/60"}
			}
		}

		nad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[i],
				topology:            "layer2",
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}
		exutil.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("create a udn hello pod in ns2")
		pod2 := udnPodResource{
			name:      "hello-pod-ns2",
			namespace: ns2,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}

		pod2.createUdnPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		//udn network connectivity should be isolated
		CurlPod2PodFailUDN(oc, ns1, pod1.name, ns2, pod2.name)
		//default network connectivity should also be isolated
		CurlPod2PodFail(oc, ns1, pod1.name, ns2, pod2.name)
	})

	g.It("Author:weliang-NonPreRelease-Longduration-Medium-75624-Feture intergration UDN with multinetworkpolicy. [Disruptive]", func() {
		var (
			udnNadtemplate               = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			mtu                    int32 = 1300
			buildPruningBaseDir          = exutil.FixturePath("testdata", "networking")
			dualstackNADTemplate         = filepath.Join(buildPruningBaseDir, "multus/dualstack-NAD-template.yaml")
			multihomingPodTemplate       = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			policyFile                   = filepath.Join(testDataDirUDN, "udn_with_multiplenetworkpolicy.yaml")
			patchSResource               = "networks.operator.openshift.io/cluster"
		)

		exutil.By("Getting the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("The cluster has no ready node for the testing")
		}

		exutil.By("Enabling useMultiNetworkPolicy in the cluster")
		patchInfoTrue := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		patchInfoFalse := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":false}}")
		defer func() {
			patchResourceAsAdmin(oc, patchSResource, patchInfoFalse)
			exutil.By("NetworkOperatorStatus should back to normal after disable useMultiNetworkPolicy")
			waitForNetworkOperatorState(oc, 10, 5, "True.*True.*False")
			waitForNetworkOperatorState(oc, 60, 15, "True.*False.*False")
		}()
		patchResourceAsAdmin(oc, patchSResource, patchInfoTrue)
		waitForNetworkOperatorState(oc, 10, 5, "True.*True.*False")
		waitForNetworkOperatorState(oc, 60, 15, "True.*False.*False")

		exutil.By("Creating a new namespace for this MultiNetworkPolicy testing")
		origContxt, contxtErr := oc.Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		defer func() {
			useContxtErr := oc.Run("config").Args("use-context", origContxt).Execute()
			o.Expect(useContxtErr).NotTo(o.HaveOccurred())
		}()
		ns1 := "project75624"
		defer oc.AsAdmin().Run("delete").Args("project", ns1, "--ignore-not-found").Execute()
		nserr1 := oc.Run("new-project").Args(ns1).Execute()
		o.Expect(nserr1).NotTo(o.HaveOccurred())
		_, proerr1 := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "user="+ns1).Output()
		o.Expect(proerr1).NotTo(o.HaveOccurred())

		exutil.By("Creating NAD1 for ns1")
		nad1 := udnNetDefResource{
			nadname:             "udn-primary-net",
			namespace:           ns1,
			nad_network_name:    "udn-primary-net",
			topology:            "layer3",
			subnet:              "10.100.0.0/16/24",
			mtu:                 mtu,
			net_attach_def_name: ns1 + "/" + "udn-primary-net",
			role:                "primary",
			template:            udnNadtemplate,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nad1.nadname, "-n", ns1).Execute()
		nad1.createUdnNad(oc)

		exutil.By("Verifying the configured NAD1")
		if checkNAD(oc, ns1, nad1.nadname) {
			e2e.Logf("The correct network-attach-definition: %v is created!", nad1.nadname)
		} else {
			e2e.Failf("The correct network-attach-definition: %v is not created!", nad1.nadname)
		}

		exutil.By("Creating NAD2 for ns1")
		nad2 := dualstackNAD{
			nadname:        "dualstack",
			namespace:      ns1,
			plugintype:     "macvlan",
			mode:           "bridge",
			ipamtype:       "whereabouts",
			ipv4range:      "192.168.10.0/24",
			ipv6range:      "fd00:dead:beef:10::/64",
			ipv4rangestart: "",
			ipv4rangeend:   "",
			ipv6rangestart: "",
			ipv6rangeend:   "",
			template:       dualstackNADTemplate,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("net-attach-def", nad2.nadname, "-n", ns1).Execute()
		nad2.createDualstackNAD(oc)

		exutil.By("Verifying the configured NAD2")
		if checkNAD(oc, ns1, nad2.nadname) {
			e2e.Logf("The correct network-attach-definition: %v is created!", nad2.nadname)
		} else {
			e2e.Failf("The correct network-attach-definition: %v is not created!", nad2.nadname)
		}

		nadName := "dualstack"
		nsWithnad := ns1 + "/" + nadName

		exutil.By("Configuring pod1 for additional network using NAD2")
		pod1 := testMultihomingPod{
			name:       "blue-pod-1",
			namespace:  ns1,
			podlabel:   "blue-pod",
			nadname:    nsWithnad,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod1.createTestMultihomingPod(oc)

		exutil.By("Configuring pod2 for additional network using NAD2")
		pod2 := testMultihomingPod{
			name:       "blue-pod-2",
			namespace:  ns1,
			podlabel:   "blue-pod",
			nadname:    nsWithnad,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod2.createTestMultihomingPod(oc)

		exutil.By("Verifying both pods with same label of blue-pod are ready for testing")
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=blue-pod")).NotTo(o.HaveOccurred())

		exutil.By("Configuring pod3 for additional network using NAD2")
		pod3 := testMultihomingPod{
			name:       "red-pod-1",
			namespace:  ns1,
			podlabel:   "red-pod",
			nadname:    nsWithnad,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod3.createTestMultihomingPod(oc)

		exutil.By("Configuring pod4 for additional network NAD2")
		pod4 := testMultihomingPod{
			name:       "red-pod-2",
			namespace:  ns1,
			podlabel:   "red-pod",
			nadname:    nsWithnad,
			nodename:   nodeList.Items[0].Name,
			podenvname: "",
			template:   multihomingPodTemplate,
		}
		pod4.createTestMultihomingPod(oc)

		exutil.By("Verifying both pods with same label of red-pod are ready for testing")
		o.Expect(waitForPodWithLabelReady(oc, ns1, "name=red-pod")).NotTo(o.HaveOccurred())

		exutil.By("Getting the deployed pods' names")
		podList, podListErr := exutil.GetAllPods(oc, ns1)
		o.Expect(podListErr).NotTo(o.HaveOccurred())

		exutil.By("Getting the IPs of the pod1's secondary interface")
		pod1v4, pod1v6 := getPodMultiNetwork(oc, ns1, podList[0])

		exutil.By("Getting the IPs of the pod2's secondary interface")
		pod2v4, pod2v6 := getPodMultiNetwork(oc, ns1, podList[1])

		exutil.By("Getting the IPs of the pod3's secondary interface")
		pod3v4, pod3v6 := getPodMultiNetwork(oc, ns1, podList[2])

		exutil.By("Getting the IPs of the pod4's secondary interface")
		pod4v4, pod4v6 := getPodMultiNetwork(oc, ns1, podList[3])

		exutil.By("Verifying the curling should pass before applying multinetworkpolicy")
		curlPod2PodMultiNetworkPass(oc, ns1, podList[2], pod1v4, pod1v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[2], pod2v4, pod2v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[3], pod1v4, pod1v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[3], pod2v4, pod2v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[2], pod4v4, pod4v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[3], pod3v4, pod3v6)

		exutil.By("Creating the ingress-allow-same-podSelector-with-same-namespaceSelector policy in ns1")
		defer removeResource(oc, true, true, "multi-networkpolicy", "ingress-allow-same-podselector-with-same-namespaceselector", "-n", ns1)
		oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", policyFile, "-n", ns1).Execute()
		output, err := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verifying the ingress-allow-same-podSelector-with-same-namespaceSelector policy is created in ns1")
		o.Expect(output).To(o.ContainSubstring("ingress-allow-same-podselector-with-same-namespaceselector"))

		exutil.By("Verifying the configured multinetworkpolicy will deny or allow the traffics as policy defined")
		curlPod2PodMultiNetworkFail(oc, ns1, podList[2], pod1v4, pod1v6)
		curlPod2PodMultiNetworkFail(oc, ns1, podList[2], pod2v4, pod2v6)
		curlPod2PodMultiNetworkFail(oc, ns1, podList[3], pod1v4, pod1v6)
		curlPod2PodMultiNetworkFail(oc, ns1, podList[3], pod2v4, pod2v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[2], pod4v4, pod4v6)
		curlPod2PodMultiNetworkPass(oc, ns1, podList[3], pod3v4, pod3v6)
	})

	g.It("Author:huirwang-NonPreRelease-Longduration-High-75503-Overlapping pod CIDRs/IPs are allowed in different primary NADs.", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnNadtemplate            = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			testPodFile               = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			mtu                 int32 = 1300
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has fewer than two nodes.")
		}

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Obtain first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Obtain 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		nadNS := []string{ns1, ns2}
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/26/29", "10.150.0.0/26/29"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2010:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/26/29,2010:100:200::0/60", "10.150.0.0/26/29,2010:100:200::0/60"}
			}
		}

		nad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[i],
				topology:            "layer3",
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
			exutil.By("Verifying the configued NetworkAttachmentDefinition")
			if checkNAD(oc, nadNS[i], nadResourcename[i]) {
				e2e.Logf("The correct network-attach-defintion: %v is created!", nadResourcename[i])
			} else {
				e2e.Failf("The correct network-attach-defintion: %v is not created!", nadResourcename[i])
			}
		}

		exutil.By("Create replica pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		numberOfPods := "8"
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas="+numberOfPods, "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS1Names := getPodName(oc, ns1, "name=test-pods")
		e2e.Logf("Collect all the pods IPs in namespace %s", ns1)
		var podsNS1IP1, podsNS1IP2 []string
		for i := 0; i < len(testpodNS1Names); i++ {
			podIP1, podIP2 := getPodIPUDN(oc, ns1, testpodNS1Names[i], "ovn-udn1")
			if podIP2 != "" {
				podsNS1IP2 = append(podsNS1IP2, podIP2)
			}
			podsNS1IP1 = append(podsNS1IP1, podIP1)
		}
		e2e.Logf("The IPs of pods in first namespace %s for UDN:\n %v %v", ns1, podsNS1IP1, podsNS1IP2)

		exutil.By("create replica pods in ns2")
		createResourceFromFile(oc, ns2, testPodFile)
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas="+numberOfPods, "-n", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS2Names := getPodName(oc, ns2, "name=test-pods")
		e2e.Logf("Collect all the pods IPs in namespace %s", ns2)
		var podsNS2IP1, podsNS2IP2 []string
		for i := 0; i < len(testpodNS2Names); i++ {
			podIP1, podIP2 := getPodIPUDN(oc, ns2, testpodNS2Names[i], "ovn-udn1")
			if podIP2 != "" {
				podsNS2IP2 = append(podsNS2IP2, podIP2)
			}
			podsNS2IP1 = append(podsNS2IP1, podIP1)
		}
		e2e.Logf("The IPs of pods in second namespace %s for UDN:\n %v %v", ns2, podsNS2IP1, podsNS2IP2)

		testpodNS1NamesLen := len(testpodNS1Names)
		podsNS1IP1Len := len(podsNS1IP1)
		podsNS1IP2Len := len(podsNS1IP2)
		exutil.By("Verify udn network should be able to access in same network.")
		for i := 0; i < testpodNS1NamesLen; i++ {
			for j := 0; j < podsNS1IP1Len; j++ {
				if podsNS1IP2Len > 0 && podsNS1IP2[j] != "" {
					_, err = e2eoutput.RunHostCmd(ns1, testpodNS1Names[i], "curl --connect-timeout 5 -s "+net.JoinHostPort(podsNS1IP2[j], "8080"))
					o.Expect(err).NotTo(o.HaveOccurred())
				}
				_, err = e2eoutput.RunHostCmd(ns1, testpodNS1Names[i], "curl --connect-timeout 5 -s "+net.JoinHostPort(podsNS1IP1[j], "8080"))
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}

		podsNS2IP1Len := len(podsNS2IP1)
		podsNS2IP2Len := len(podsNS2IP2)
		exutil.By("Verify udn network should be isolated in different network.")
		for i := 0; i < testpodNS1NamesLen; i++ {
			for j := 0; j < podsNS2IP1Len; j++ {
				if podsNS2IP2Len > 0 && podsNS2IP2[j] != "" {
					if contains(podsNS1IP2, podsNS2IP2[j]) {
						// as the destination IP in ns2 is same as one in NS1, then it will be able to access that IP and has been executed in previous steps.
						continue
					} else {
						_, err = e2eoutput.RunHostCmd(ns1, testpodNS1Names[i], "curl --connect-timeout 5 -s "+net.JoinHostPort(podsNS2IP2[j], "8080"))
						o.Expect(err).To(o.HaveOccurred())
					}
				}
				if contains(podsNS1IP1, podsNS2IP1[j]) {
					// as the destination IP in ns2 is same as one in NS1, then  it will be able to access that IP and has been executed in previous steps..
					continue
				} else {
					_, err = e2eoutput.RunHostCmd(ns1, testpodNS1Names[i], "curl --connect-timeout 5 -s "+net.JoinHostPort(podsNS2IP1[j], "8080"))
					o.Expect(err).To(o.HaveOccurred())
				}
			}
		}
	})

	g.It("Author:meinli-High-75880-Check udn pods connection and isolation on user defined networks when NADs are created via CRD(Layer 3)", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack           = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			testPodFile               = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			mtu                 int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		exutil.By("3. Create CRD for UDN")
		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		udnNS := []string{ns1, ns2}

		var cidr, ipv4cidr, ipv6cidr []string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
				prefix = 64
			} else {
				ipv4cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
				ipv4prefix = 24
				ipv6cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
				ipv6prefix = 64
			}
		}
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
					template:   udnCRDdualStack,
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

		exutil.By("4. Create replica pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		err := waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS1Names := getPodName(oc, ns1, "name=test-pods")
		CurlPod2PodPassUDN(oc, ns1, testpodNS1Names[0], ns1, testpodNS1Names[1])

		exutil.By("5. create replica pods in ns2")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS2Names := getPodName(oc, ns2, "name=test-pods")

		exutil.By("6. verify isolation on user defined networks")
		//udn network connectivity should be isolated
		CurlPod2PodFailUDN(oc, ns1, testpodNS1Names[0], ns2, testpodNS2Names[0])
		//default network connectivity should also be isolated
		CurlPod2PodFail(oc, ns1, testpodNS1Names[0], ns2, testpodNS2Names[0])
	})

	g.It("Author:meinli-High-75881-Check udn pods connection and isolation on user defined networks when NADs are created via CRD(Layer 2)", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack           = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_singlestack_template.yaml")
			testPodFile               = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			mtu                 int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		exutil.By("3. Create CRD for UDN")
		udnResourcename := []string{"l2-network-" + ns1, "l2-network-" + ns2}
		udnNS := []string{ns1, ns2}

		var cidr, ipv4cidr, ipv6cidr []string
		if ipStackType == "ipv4single" {
			cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
		} else {
			if ipStackType == "ipv6single" {
				cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			} else {
				ipv4cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
				ipv6cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
			}
		}
		udncrd := make([]udnCRDResource, 2)
		for i := 0; i < 2; i++ {
			if ipStackType == "dualstack" {
				udncrd[i] = udnCRDResource{
					crdname:   udnResourcename[i],
					namespace: udnNS[i],
					role:      "Primary",
					mtu:       mtu,
					IPv4cidr:  ipv4cidr[i],
					IPv6cidr:  ipv6cidr[i],
					template:  udnCRDdualStack,
				}
				udncrd[i].createLayer2DualStackUDNCRD(oc)

			} else {
				udncrd[i] = udnCRDResource{
					crdname:   udnResourcename[i],
					namespace: udnNS[i],
					role:      "Primary",
					mtu:       mtu,
					cidr:      cidr[i],
					template:  udnCRDSingleStack,
				}
				udncrd[i].createLayer2SingleStackUDNCRD(oc)
			}

			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Create replica pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		err := waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS1Names := getPodName(oc, ns1, "name=test-pods")
		CurlPod2PodPassUDN(oc, ns1, testpodNS1Names[0], ns1, testpodNS1Names[1])

		exutil.By("5. create replica pods in ns2")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testpodNS2Names := getPodName(oc, ns2, "name=test-pods")

		exutil.By("6. verify isolation on user defined networks")
		//udn network connectivity should be isolated
		CurlPod2PodFailUDN(oc, ns1, testpodNS1Names[0], ns2, testpodNS2Names[0])
		//default network connectivity should also be isolated
		CurlPod2PodFail(oc, ns1, testpodNS1Names[0], ns2, testpodNS2Names[0])
	})

	g.It("Author:asood-High-75899-Validate L2 and L3 Pod2Egress traffic in shared and local gateway mode", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnCRDL2dualStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnCRDL2SingleStack       = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_singlestack_template.yaml")
			udnCRDL3dualStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			udnCRDL3SingleStack       = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			udnNadtemplate            = filepath.Join(buildPruningBaseDir, "udn/udn_nad_template.yaml")
			testPodFile               = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			mtu                 int32 = 1300
			pingIPv4Cmd               = "ping -c 2 8.8.8.8"
			pingIPv6Cmd               = "ping6 -c 2 2001:4860:4860::8888"
			udnNS                     = []string{}
			pingCmds                  = []string{}
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Obtain first namespace and create three others")
		udnNS = append(udnNS, oc.Namespace())
		for i := 0; i < 3; i++ {
			oc.SetupProject()
			udnNS = append(udnNS, oc.Namespace())
		}

		var cidr, ipv4cidr, ipv6cidr []string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
			prefix = 24
			pingCmds = append(pingCmds, pingIPv4Cmd)
		} else {
			if ipStackType == "ipv6single" {
				cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
				prefix = 64
				pingCmds = append(pingCmds, pingIPv6Cmd)
			} else {
				ipv4cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
				ipv4prefix = 24
				ipv6cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
				ipv6prefix = 64
				pingCmds = append(pingCmds, pingIPv4Cmd)
				pingCmds = append(pingCmds, pingIPv6Cmd)
			}
		}

		exutil.By("2. Create CRD for UDN in first two namespaces")
		udnResourcename := []string{"l2-network-" + udnNS[0], "l3-network-" + udnNS[1]}
		udnDSTemplate := []string{udnCRDL2dualStack, udnCRDL3dualStack}
		udnSSTemplate := []string{udnCRDL2SingleStack, udnCRDL3SingleStack}

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
					template:   udnDSTemplate[i],
				}
				switch i {
				case 0:
					udncrd[0].createLayer2DualStackUDNCRD(oc)
				case 1:
					udncrd[1].createUdnCRDDualStack(oc)
				}

			} else {
				udncrd[i] = udnCRDResource{
					crdname:   udnResourcename[i],
					namespace: udnNS[i],
					role:      "Primary",
					mtu:       mtu,
					cidr:      cidr[i],
					prefix:    prefix,
					template:  udnSSTemplate[i],
				}
				switch i {
				case 0:
					udncrd[0].createLayer2SingleStackUDNCRD(oc)
				case 1:
					udncrd[1].createUdnCRDSingleStack(oc)
				}
			}

			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		exutil.By("3. Create NAD for UDN in last two namespaces")
		udnNADResourcename := []string{"l2-network-" + udnNS[2], "l3-network-" + udnNS[3]}
		topology := []string{"layer2", "layer3"}
		udnnad := make([]udnNetDefResource, 2)
		for i := 0; i < 2; i++ {
			udnnad[i] = udnNetDefResource{
				nadname:             udnNADResourcename[i],
				namespace:           udnNS[i+2],
				nad_network_name:    udnNADResourcename[i],
				topology:            topology[i],
				subnet:              "",
				mtu:                 mtu,
				net_attach_def_name: fmt.Sprintf("%s/%s", udnNS[i+2], udnNADResourcename[i]),
				role:                "primary",
				template:            udnNadtemplate,
			}
			if ipStackType == "dualstack" {
				udnnad[i].subnet = fmt.Sprintf("%s,%s", ipv4cidr[i], ipv6cidr[i])
			} else {
				udnnad[i].subnet = cidr[i]
			}
			udnnad[i].createUdnNad(oc)
		}

		exutil.By("4. Create replica pods in namespaces")
		for _, ns := range udnNS {
			e2e.Logf("Validating in %s namespace", ns)
			createResourceFromFile(oc, ns, testPodFile)
			err := waitForPodWithLabelReady(oc, ns, "name=test-pods")
			exutil.AssertWaitPollNoErr(err, "Pods with label name=test-pods not ready")
			testpodNSNames := getPodName(oc, ns, "name=test-pods")
			CurlPod2PodPassUDN(oc, ns, testpodNSNames[0], ns, testpodNSNames[1])
			for _, pingCmd := range pingCmds {
				pingResponse, err := execCommandInSpecificPod(oc, ns, testpodNSNames[0], pingCmd)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(strings.Contains(pingResponse, "0% packet loss")).To(o.BeTrue())
			}
		}
	})

	g.It("Author:meinli-High-75955-Verify UDN failed message when user defined join subnet overlaps user defined subnet (Layer3)", func() {
		var (
			buildPruningBaseDir                         = exutil.FixturePath("testdata", "networking")
			udnCRDL3dualStack                           = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			udnCRDL3SingleStack                         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			UserDefinedPrimaryNetworkJoinSubnetV4       = "100.65.0.0/16"
			UserDefinedPrimaryNetworkJoinSubnetV6       = "fd99::/48"
			mtu                                   int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create namespace")
		ns := oc.Namespace()

		exutil.By("2. Create CRD for UDN")
		var udncrd udnCRDResource
		var cidr string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = UserDefinedPrimaryNetworkJoinSubnetV4
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = UserDefinedPrimaryNetworkJoinSubnetV6
				prefix = 64
			} else {
				ipv4prefix = 24
				ipv6prefix = 64
			}
		}
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "udn-network-75995",
				namespace:  ns,
				role:       "Primary",
				mtu:        mtu,
				IPv4cidr:   UserDefinedPrimaryNetworkJoinSubnetV4,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   UserDefinedPrimaryNetworkJoinSubnetV6,
				IPv6prefix: ipv6prefix,
				template:   udnCRDL3dualStack,
			}
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd = udnCRDResource{
				crdname:   "udn-network-75995",
				namespace: ns,
				role:      "Primary",
				mtu:       mtu,
				cidr:      cidr,
				prefix:    prefix,
				template:  udnCRDL3SingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		err := waitUDNCRDApplied(oc, ns, udncrd.crdname)
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("3. Check UDN failed message")
		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("userdefinednetwork.k8s.ovn.org", udncrd.crdname, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.Or(
			o.ContainSubstring(fmt.Sprintf("illegal network configuration: user defined join subnet \"100.65.0.0/16\" overlaps user defined subnet \"%s\"", UserDefinedPrimaryNetworkJoinSubnetV4)),
			o.ContainSubstring(fmt.Sprintf("illegal network configuration: user defined join subnet \"fd99::/64\" overlaps user defined subnet \"%s\"", UserDefinedPrimaryNetworkJoinSubnetV6))))
	})

	g.It("Author:anusaxen-Critical-75984-Check udn pods isolation on user defined networks post OVN gateway migration", func() {
		var (
			udnNadtemplate       = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate       = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			mtu            int32 = 1300
		)

		ipStackType := checkIPStackType(oc)

		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()
		exutil.By("3. Create 3rd namespace")
		oc.SetupProject()
		ns3 := oc.Namespace()

		exutil.By("4. Create 4th namespace")
		oc.SetupProject()
		ns4 := oc.Namespace()

		nadResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2, "l2-network-" + ns3, "l2-network-" + ns4}
		nadNS := []string{ns1, ns2, ns3, ns4}
		topo := []string{"layer3", "layer3", "layer2", "layer2"}

		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.151.0.0/16/24", "10.152.0.0/16", "10.153.0.0/16"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2011:100:200::0/60", "2012:100:200::0/60", "2013:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.151.0.0/16/24,2011:100:200::0/60", "10.152.0.0/16,2012:100:200::0/60", "10.153.0.0/16,2013:100:200::0/60"}
			}
		}

		nad := make([]udnNetDefResource, 4)
		for i := 0; i < 4; i++ {
			exutil.By(fmt.Sprintf("create NAD %s in namespace %s", nadResourcename[i], nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename[i],
				namespace:           nadNS[i],
				nad_network_name:    nadResourcename[i],
				topology:            topo[i],
				subnet:              subnet[i],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename[i],
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad[i].createUdnNad(oc)
		}

		pod := make([]udnPodResource, 4)
		for i := 0; i < 4; i++ {
			exutil.By("create a udn hello pods in ns1 ns2 ns3 and ns4")
			pod[i] = udnPodResource{
				name:      "hello-pod",
				namespace: nadNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)
		}

		exutil.By("create another udn hello pod in ns1 to ensure layer3 conectivity post migration among'em")
		pod_ns1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: nadNS[0],
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod_ns1.createUdnPod(oc)
		waitPodReady(oc, pod_ns1.namespace, pod_ns1.name)

		exutil.By("create another udn hello pod in ns3 to ensure layer2 conectivity post migration among'em")
		pod_ns3 := udnPodResource{
			name:      "hello-pod-ns3",
			namespace: nadNS[2],
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod_ns3.createUdnPod(oc)
		waitPodReady(oc, pod_ns3.namespace, pod_ns3.name)

		//need to find out original mode cluster is on so that we can revert back to same post test
		var desiredMode string
		origMode := getOVNGatewayMode(oc)
		if origMode == "local" {
			desiredMode = "shared"
		} else {
			desiredMode = "local"
		}
		e2e.Logf("Cluster is currently on gateway mode %s", origMode)
		e2e.Logf("Desired mode is %s", desiredMode)

		defer switchOVNGatewayMode(oc, origMode)
		switchOVNGatewayMode(oc, desiredMode)

		//udn network connectivity for layer3 should be isolated
		CurlPod2PodFailUDN(oc, ns1, pod[0].name, ns2, pod[1].name)
		//default network connectivity for layer3 should also be isolated
		CurlPod2PodFail(oc, ns1, pod[0].name, ns2, pod[1].name)

		//udn network connectivity for layer2 should be isolated
		CurlPod2PodFailUDN(oc, ns3, pod[2].name, ns4, pod[3].name)
		//default network connectivity for layer2 should also be isolated
		CurlPod2PodFail(oc, ns3, pod[2].name, ns4, pod[3].name)

		//ensure udn network connectivity for layer3 should be there
		CurlPod2PodPassUDN(oc, ns1, pod[0].name, ns1, pod_ns1.name)
		//ensure udn network connectivity for layer2 should be there
		CurlPod2PodPassUDN(oc, ns3, pod[2].name, ns3, pod_ns3.name)
	})

	g.It("Author:anusaxen-NonPreRelease-Longduration-Critical-76939-Check udn pods isolation on a scaled node [Disruptive]", func() {
		var (
			udnPodTemplate           = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			udnPodTemplateNode       = filepath.Join(testDataDirUDN, "udn_test_pod_template_node.yaml")
			udnCRDSingleStack        = filepath.Join(testDataDirUDN, "udn_crd_singlestack_template.yaml")
			mtu                int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		o.Expect(ipStackType).NotTo(o.BeEmpty())
		if ipStackType != "ipv4single" {
			g.Skip("This case requires IPv4 single stack cluster")
		}
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.VSphere, clusterinfra.IBMCloud, clusterinfra.OpenStack)

		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		udnResourcename := []string{"l3-network-" + ns1, "l3-network-" + ns2}
		udnNS := []string{ns1, ns2}
		var cidr []string
		var prefix int32

		cidr = []string{"10.150.0.0/16", "10.151.0.0/16"}
		prefix = 24

		udncrd := make([]udnCRDResource, 2)
		for i := 0; i < 2; i++ {
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
			err := waitUDNCRDApplied(oc, udnNS[i], udncrd[i].crdname)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		//following code block to scale up a node on cluster
		exutil.By("1. Create a new machineset, get the new node created\n")
		clusterinfra.SkipConditionally(oc)
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-76939"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
		machineName := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)
		o.Expect(len(machineName)).ShouldNot(o.Equal(0))
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineName[0])
		e2e.Logf("Get nodeName: %v", nodeName)

		checkNodeStatus(oc, nodeName, "Ready")

		exutil.By("create a udn hello pod in ns2")
		pod2 := udnPodResourceNode{
			name:      "hello-pod-ns2",
			namespace: ns2,
			label:     "hello-pod",
			nodename:  nodeName,
			template:  udnPodTemplateNode,
		}

		pod2.createUdnPodNode(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		//udn network connectivity should be isolated
		CurlPod2PodFailUDN(oc, ns1, pod1.name, ns2, pod2.name)
		//default network connectivity should also be isolated
		CurlPod2PodFail(oc, ns1, pod1.name, ns2, pod2.name)
	})

	g.It("Author:meinli-High-77517-Validate pod2pod connection within and across node when creating UDN with Secondary role from same namespace (Layer3)", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			udnCRDdualStack           = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			udnPodTemplate            = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			mtu                 int32 = 9000
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Get namespace and worker node")
		ns := oc.Namespace()
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}

		exutil.By("2. create CUDN with Secondary role")
		var cidr, ipv4cidr, ipv6cidr string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
				prefix = 64
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv4prefix = 24
				ipv6cidr = "2010:100:200::0/60"
				ipv6prefix = 64
			}
		}

		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "l3-secondary",
				namespace:  ns,
				role:       "Secondary",
				mtu:        mtu,
				IPv4cidr:   ipv4cidr,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr,
				IPv6prefix: ipv6prefix,
				template:   udnCRDdualStack,
			}
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd = udnCRDResource{
				crdname:   "l3-secondary",
				namespace: ns,
				role:      "Secondary",
				mtu:       mtu,
				cidr:      cidr,
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		err = waitUDNCRDApplied(oc, udncrd.namespace, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. validate Layer3 router is created in OVN")
		ovnMasterPodName := getOVNKMasterOVNkubeNode(oc)
		o.Expect(ovnMasterPodName).NotTo(o.BeEmpty())
		o.Eventually(func() bool {
			return checkOVNRouter(oc, "l3.secondary_ovn_cluster_router", ovnMasterPodName)
		}, 20*time.Second, 5*time.Second).Should(o.BeTrue(), "The correct OVN router is not created")

		exutil.By("4. Create 2 pods within the same node and 1 pod across with different nodes")
		pods := make([]testMultihomingPod, 3)
		var podNames []string
		for i := 0; i < 2; i++ {
			pods[i] = testMultihomingPod{
				name:       "multihoming-pod-" + strconv.Itoa(i),
				namespace:  ns,
				podlabel:   "multihoming-pod-" + strconv.Itoa(i),
				nadname:    udncrd.crdname,
				nodename:   nodeList.Items[i].Name,
				podenvname: "Hello multihoming-pod",
				template:   udnPodTemplate,
			}
			pods[i].createTestMultihomingPod(oc)
			o.Expect(waitForPodWithLabelReady(oc, ns, fmt.Sprintf("name=%s", pods[i].podlabel))).NotTo(o.HaveOccurred())
			podNames = append(podNames, getPodName(oc, pods[i].namespace, fmt.Sprintf("name=%s", pods[i].podlabel))[0])
		}
		pods[2] = testMultihomingPod{
			name:       "multihoming-pod-2",
			namespace:  ns,
			podlabel:   "multihoming-pod-2",
			nadname:    udncrd.crdname,
			nodename:   nodeList.Items[1].Name,
			podenvname: "Hello multihoming-pod",
			template:   udnPodTemplate,
		}
		pods[2].createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, fmt.Sprintf("name=%s", pods[2].podlabel))).NotTo(o.HaveOccurred())
		podNames = append(podNames, getPodName(oc, pods[2].namespace, fmt.Sprintf("name=%s", pods[2].podlabel))[0])

		exutil.By("5. Check pods subnet overlap within and across nodes")
		o.Expect(checkPodCIDRsOverlap(oc, ns, ipStackType, []string{podNames[2], podNames[0]}, "net1")).Should(o.BeFalse())
		o.Expect(checkPodCIDRsOverlap(oc, ns, ipStackType, []string{podNames[2], podNames[1]}, "net1")).Should(o.BeTrue())

		exutil.By("6. Validate pod2pod connection within the same node and across with different nodes")
		CurlUDNPod2PodPassMultiNetwork(oc, ns, ns, podNames[2], "net1", podNames[0], "net1")
		CurlUDNPod2PodPassMultiNetwork(oc, ns, ns, podNames[2], "net1", podNames[1], "net1")
	})

	g.It("Author:meinli-High-77519-Validate pod2pod isolation within and across nodes when creating UDN with Secondary role from different namespaces (Layer3)", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			udnCRDdualStack           = filepath.Join(buildPruningBaseDir, "udn/udn_crd_dualstack2_template.yaml")
			udnPodTemplate            = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			mtu                 int32 = 9000
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Get namespace and worker node")
		ns1 := oc.Namespace()
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}

		exutil.By("2. create CUDN with Secondary role in ns1")
		var cidr, ipv4cidr, ipv6cidr []string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = []string{"10.150.0.0/16", "10.200.0.0/16"}
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
				prefix = 64
			} else {
				ipv4cidr = []string{"10.150.0.0/16", "10.200.0.0/16"}
				ipv4prefix = 24
				ipv6cidr = []string{"2010:100:200::0/60", "2011:100:200::0/60"}
				ipv6prefix = 64
			}
		}

		var udncrd udnCRDResource
		if ipStackType == "dualstack" {
			udncrd = udnCRDResource{
				crdname:    "l3-secondary",
				namespace:  ns1,
				role:       "Secondary",
				mtu:        mtu,
				IPv4cidr:   ipv4cidr[0],
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr[0],
				IPv6prefix: ipv6prefix,
				template:   udnCRDdualStack,
			}
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd = udnCRDResource{
				crdname:   "l3-secondary",
				namespace: ns1,
				role:      "Secondary",
				mtu:       mtu,
				cidr:      cidr[0],
				prefix:    prefix,
				template:  udnCRDSingleStack,
			}
			udncrd.createUdnCRDSingleStack(oc)
		}
		err = waitUDNCRDApplied(oc, udncrd.namespace, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. create 1 pod with secondary annotation in ns1")
		var podNames []string
		// create 1 pod in ns1
		pod1 := testMultihomingPod{
			name:       "multihoming-pod-ns1",
			namespace:  ns1,
			podlabel:   "multihoming-pod-ns1",
			nadname:    udncrd.crdname,
			nodename:   nodeList.Items[0].Name,
			podenvname: "Hello multihoming-pod",
			template:   udnPodTemplate,
		}
		pod1.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, fmt.Sprintf("name=%s", pod1.podlabel))).NotTo(o.HaveOccurred())
		podNames = append(podNames, getPodName(oc, pod1.namespace, fmt.Sprintf("name=%s", pod1.podlabel))[0])

		exutil.By("4. create CUDN with secondary role in ns2")
		// create 2nd namespace
		oc.SetupProject()
		ns2 := oc.Namespace()
		udncrd.namespace = ns2
		if ipStackType == "dualstack" {
			udncrd.IPv4cidr = ipv4cidr[1]
			udncrd.IPv6cidr = ipv6cidr[1]
			udncrd.createUdnCRDDualStack(oc)
		} else {
			udncrd.cidr = cidr[1]
			udncrd.createUdnCRDSingleStack(oc)
		}
		err = waitUDNCRDApplied(oc, udncrd.namespace, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create 2 pods with secondary annotation in ns2")
		pods := make([]testMultihomingPod, 2)
		//create 2 pods in ns2
		for i := 0; i < 2; i++ {
			pods[i] = testMultihomingPod{
				name:       "multihoming-pod-" + strconv.Itoa(i),
				namespace:  ns2,
				podlabel:   "multihoming-pod-" + strconv.Itoa(i),
				nadname:    udncrd.crdname,
				nodename:   nodeList.Items[i].Name,
				podenvname: "Hello multihoming-pod",
				template:   udnPodTemplate,
			}
			pods[i].createTestMultihomingPod(oc)
			o.Expect(waitForPodWithLabelReady(oc, ns2, fmt.Sprintf("name=%s", pods[i].podlabel))).NotTo(o.HaveOccurred())
			podNames = append(podNames, getPodName(oc, pods[i].namespace, fmt.Sprintf("name=%s", pods[i].podlabel))[0])
		}

		exutil.By("6. Validate pod2pod isolation from secondary network in different namespaces")
		CurlUDNPod2PodFailMultiNetwork(oc, ns1, ns2, podNames[0], "net1", podNames[1], "net1")
		CurlUDNPod2PodFailMultiNetwork(oc, ns1, ns2, podNames[0], "net1", podNames[2], "net1")
		CurlUDNPod2PodPassMultiNetwork(oc, ns2, ns2, podNames[1], "net1", podNames[2], "net1")
	})

	g.It("Author:meinli-High-77563-Validate pod2pod connection within and across node when creating UDN with Secondary role from same namespace (Layer2)", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack           = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnPodTemplate            = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			mtu                 int32 = 9000
		)
		exutil.By("1. Get namespace and worker node")
		ns := oc.Namespace()
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}

		exutil.By("2. create Layer2 CUDN with Secondary role")
		ipv4cidr := "10.150.0.0/16"
		ipv6cidr := "2010:100:200::0/60"
		udncrd := udnCRDResource{
			crdname:   "l2-secondary",
			namespace: ns,
			role:      "Secondary",
			mtu:       mtu,
			IPv4cidr:  ipv4cidr,
			IPv6cidr:  ipv6cidr,
			template:  udnCRDdualStack,
		}
		udncrd.createLayer2DualStackUDNCRD(oc)
		err = waitUDNCRDApplied(oc, udncrd.namespace, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. validate Layer2 switch is created in OVN")
		ovnMasterPodName := getOVNKMasterOVNkubeNode(oc)
		o.Expect(ovnMasterPodName).NotTo(o.BeEmpty())
		o.Eventually(func() bool {
			return checkOVNSwitch(oc, "l2.secondary_ovn_layer2_switch", ovnMasterPodName)
		}, 20*time.Second, 5*time.Second).Should(o.BeTrue(), "The correct OVN switch is not created")

		exutil.By("4. create 2 pods within the same node and 1 pod across with different nodes")
		pods := make([]testMultihomingPod, 3)
		var podNames []string
		for i := 0; i < 2; i++ {
			pods[i] = testMultihomingPod{
				name:       "multihoming-pod-" + strconv.Itoa(i),
				namespace:  ns,
				podlabel:   "multihoming-pod-" + strconv.Itoa(i),
				nadname:    udncrd.crdname,
				nodename:   nodeList.Items[i].Name,
				podenvname: "Hello multihoming-pod",
				template:   udnPodTemplate,
			}
			pods[i].createTestMultihomingPod(oc)
			o.Expect(waitForPodWithLabelReady(oc, ns, fmt.Sprintf("name=%s", pods[i].podlabel))).NotTo(o.HaveOccurred())
			podNames = append(podNames, getPodName(oc, pods[i].namespace, fmt.Sprintf("name=%s", pods[i].podlabel))[0])
		}
		pods[2] = testMultihomingPod{
			name:       "multihoming-pod-2",
			namespace:  ns,
			podlabel:   "multihoming-pod-2",
			nadname:    udncrd.crdname,
			nodename:   nodeList.Items[1].Name,
			podenvname: "Hello multihoming-pod",
			template:   udnPodTemplate,
		}
		pods[2].createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns, fmt.Sprintf("name=%s", pods[2].podlabel))).NotTo(o.HaveOccurred())
		podNames = append(podNames, getPodName(oc, pods[2].namespace, fmt.Sprintf("name=%s", pods[2].podlabel))[0])

		exutil.By("5. Check pods subnet overlap within and across nodes")
		o.Expect(checkPodCIDRsOverlap(oc, ns, "dualstack", []string{podNames[2], podNames[0]}, "net1")).Should(o.BeTrue())
		o.Expect(checkPodCIDRsOverlap(oc, ns, "dualstack", []string{podNames[2], podNames[1]}, "net1")).Should(o.BeTrue())

		exutil.By("6. Validate pod2pod connection (dual stack) within the same node")
		pod0IPv4, pod0IPv6 := getPodMultiNetwork(oc, ns, podNames[0])
		e2e.Logf("Pod0 IPv4 address is: %v, IPv6 address is: %v", pod0IPv4, pod0IPv6)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod0IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod0IPv6, "net1", pods[0].podenvname)

		exutil.By("7. Validate pod2pod connection (dual stack) across with different nodes")
		pod1IPv4, pod1IPv6 := getPodMultiNetwork(oc, ns, podNames[1])
		e2e.Logf("Pod1 IPv4 address is: %v, IPv6 address is: %v", pod1IPv4, pod1IPv6)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv6, "net1", pods[1].podenvname)
	})

	g.It("Author:meinli-High-77564-Validate pod2pod isolation within and across node when creating UDN with Secondary role from different namespaces (Layer2)", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack           = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnPodTemplate            = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			mtu                 int32 = 9000
			podenvname                = "Hello multihoming-pod"
		)
		exutil.By("1. Get namespace and worker node")
		ns1 := oc.Namespace()
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}

		exutil.By("2. Create Layer2 CUDN with Secondary role in ns1")
		ipv4cidr := []string{"10.150.0.0/16", "10.200.0.0/16"}
		ipv6cidr := []string{"2010:100:200::0/60", "2011:100:200::0/60"}
		udncrd1 := udnCRDResource{
			crdname:   "l2-secondary-ns1",
			namespace: ns1,
			role:      "Secondary",
			mtu:       mtu,
			IPv4cidr:  ipv4cidr[0],
			IPv6cidr:  ipv6cidr[0],
			template:  udnCRDdualStack,
		}
		udncrd1.createLayer2DualStackUDNCRD(oc)
		err = waitUDNCRDApplied(oc, udncrd1.namespace, udncrd1.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. create 1 pod with secondary annotation in ns1")
		var podNames []string
		// create 1 pod in ns1
		pod1 := testMultihomingPod{
			name:       "multihoming-pod-ns1",
			namespace:  ns1,
			podlabel:   "multihoming-pod-ns1",
			nadname:    udncrd1.crdname,
			nodename:   nodeList.Items[0].Name,
			podenvname: podenvname,
			template:   udnPodTemplate,
		}
		pod1.createTestMultihomingPod(oc)
		o.Expect(waitForPodWithLabelReady(oc, ns1, fmt.Sprintf("name=%s", pod1.podlabel))).NotTo(o.HaveOccurred())
		podNames = append(podNames, getPodName(oc, pod1.namespace, fmt.Sprintf("name=%s", pod1.podlabel))[0])

		exutil.By("4. create Layer2 CUDN with secondary role in ns2")
		// create 2nd namespace
		oc.SetupProject()
		ns2 := oc.Namespace()
		udncrd2 := udnCRDResource{
			crdname:   "l2-secondary-ns2",
			namespace: ns2,
			role:      "Secondary",
			mtu:       mtu,
			IPv4cidr:  ipv4cidr[1],
			IPv6cidr:  ipv6cidr[1],
			template:  udnCRDdualStack,
		}
		udncrd2.createLayer2DualStackUDNCRD(oc)
		err = waitUDNCRDApplied(oc, udncrd2.namespace, udncrd2.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. create pods with secondary annotation in ns2")
		pods := make([]testMultihomingPod, 2)
		//create 2 pods in ns2
		for i := 0; i < 2; i++ {
			pods[i] = testMultihomingPod{
				name:       "multihoming-pod-" + strconv.Itoa(i),
				namespace:  ns2,
				podlabel:   "multihoming-pod-" + strconv.Itoa(i),
				nadname:    udncrd2.crdname,
				nodename:   nodeList.Items[i].Name,
				podenvname: podenvname,
				template:   udnPodTemplate,
			}
			pods[i].createTestMultihomingPod(oc)
			o.Expect(waitForPodWithLabelReady(oc, ns2, fmt.Sprintf("name=%s", pods[i].podlabel))).NotTo(o.HaveOccurred())
			podNames = append(podNames, getPodName(oc, pods[i].namespace, fmt.Sprintf("name=%s", pods[i].podlabel))[0])
		}

		exutil.By("6. validate pod2pod isolation (dual stack) within the same node")
		pod0IPv4, pod0IPv6 := getPodMultiNetwork(oc, ns2, podNames[1])
		e2e.Logf("Pod0 IPv4 address is: %v, IPv6 address is: %v", pod0IPv4, pod0IPv6)
		CurlMultusPod2PodFail(oc, ns1, podNames[0], pod0IPv4, "net1", podenvname)
		CurlMultusPod2PodFail(oc, ns1, podNames[0], pod0IPv6, "net1", podenvname)

		exutil.By("7. validate pod2pod isolation (dual stack) across with different node")
		pod1IPv4, pod1IPv6 := getPodMultiNetwork(oc, ns2, podNames[2])
		e2e.Logf("Pod1 IPv4 address is: %v, IPv6 address is: %v", pod1IPv4, pod1IPv6)
		CurlMultusPod2PodFail(oc, ns1, podNames[0], pod1IPv4, "net1", podenvname)
		CurlMultusPod2PodFail(oc, ns1, podNames[0], pod1IPv6, "net1", podenvname)
		CurlMultusPod2PodPass(oc, ns2, podNames[1], pod1IPv4, "net1", podenvname)
		CurlMultusPod2PodPass(oc, ns2, podNames[1], pod1IPv6, "net1", podenvname)
	})

	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-High-77656-Verify ingress-ipblock policy for UDN pod's secondary interface (Layer2). [Disruptive]", func() {
		var (
			buildPruningBaseDir                          = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack                              = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnPodTemplate                               = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			multinetworkipBlockIngressTemplateDual       = filepath.Join(buildPruningBaseDir, "multihoming/multiNetworkPolicy_ingress_ipblock_template.yaml")
			patchSResource                               = "networks.operator.openshift.io/cluster"
			mtu                                    int32 = 9000
		)
		exutil.By("Getting the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("The cluster has no ready node for the testing")
		}

		exutil.By("Getting the namespace name")
		ns := oc.Namespace()
		nadName := "ipblockingress77656"
		nsWithnad := ns + "/" + nadName

		exutil.By("Enabling useMultiNetworkPolicy in the cluster")
		patchInfoTrue := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		patchInfoFalse := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":false}}")
		defer func() {
			patchResourceAsAdmin(oc, patchSResource, patchInfoFalse)
			exutil.By("NetworkOperatorStatus should back to normal after disable useMultiNetworkPolicy")
			waitForNetworkOperatorState(oc, 10, 5, "True.*True.*False")
			waitForNetworkOperatorState(oc, 60, 15, "True.*False.*False")
		}()
		patchResourceAsAdmin(oc, patchSResource, patchInfoTrue)

		exutil.By("Wait for the NetworkOperator to become functional after enabling useMultiNetworkPolicy")
		waitForNetworkOperatorState(oc, 10, 5, "True.*True.*False")
		waitForNetworkOperatorState(oc, 60, 15, "True.*False.*False")

		exutil.By("Creating Layer2 UDN CRD with Secondary role")
		ipv4cidr := "20.200.200.0/24"
		ipv6cidr := "2000:200:200::0/64"
		udncrd := udnCRDResource{
			crdname:   nadName,
			namespace: ns,
			role:      "Secondary",
			mtu:       mtu,
			IPv4cidr:  ipv4cidr,
			IPv6cidr:  ipv6cidr,
			template:  udnCRDdualStack,
		}
		udncrd.createLayer2DualStackUDNCRD(oc)
		err := waitUDNCRDApplied(oc, udncrd.namespace, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Creating three testing pods consuming above network-attach-definition in ns")
		var podName, podLabel, podenvName string
		pods := make([]testMultihomingPod, 3)
		var podNames []string
		for i := 0; i < 3; i++ {
			podName = "multihoming-pod-" + strconv.Itoa(i+1)
			podLabel = "multihoming-pod" + strconv.Itoa(i+1)
			podenvName = "Hello multihoming-pod-" + strconv.Itoa(i+1)
			pods[i] = testMultihomingPod{
				name:       podName,
				namespace:  ns,
				podlabel:   podLabel,
				nadname:    udncrd.crdname,
				nodename:   "",
				podenvname: podenvName,
				template:   udnPodTemplate,
			}
			pods[i].createTestMultihomingPod(oc)
			o.Expect(waitForPodWithLabelReady(oc, ns, fmt.Sprintf("name=%s", pods[i].podlabel))).NotTo(o.HaveOccurred())
			podNames = append(podNames, getPodName(oc, pods[i].namespace, fmt.Sprintf("name=%s", pods[i].podlabel))[0])
		}

		exutil.By("Verifying the all pods get dual IPs")
		pod1IPv4, pod1IPv6 := getPodMultiNetwork(oc, ns, podNames[0])
		pod2IPv4, pod2IPv6 := getPodMultiNetwork(oc, ns, podNames[1])
		pod3IPv4, pod3IPv6 := getPodMultiNetwork(oc, ns, podNames[2])
		pod3IPv4WithCidr := pod3IPv4 + "/32"
		pod3IPv6WithCidr := pod3IPv6 + "/128"

		exutil.By("Verifying that there is no traffic blocked between pods")
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv6, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod2IPv6, "net1", pods[1].podenvname)

		exutil.By("Creating ipBlock Ingress Dual CIDRs Policy to allow traffic only from pod3")
		defer removeResource(oc, true, true, "multi-networkpolicy", "multinetworkipblock-dual-cidrs-ingress", "-n", ns)
		IPBlock := multinetworkipBlockCIDRsDual{
			name:      "multinetworkipblock-dual-cidrs-ingress",
			namespace: ns,
			cidrIpv4:  pod3IPv4WithCidr,
			cidrIpv6:  pod3IPv6WithCidr,
			policyfor: nsWithnad,
			template:  multinetworkipBlockIngressTemplateDual,
		}
		IPBlock.createMultinetworkipBlockCIDRDual(oc)
		policyoutput, policyerr := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns).Output()
		o.Expect(policyerr).NotTo(o.HaveOccurred())
		o.Expect(policyoutput).To(o.ContainSubstring("multinetworkipblock-dual-cidrs-ingress"))

		exutil.By("Verifying the ipBlock Ingress Dual CIDRs policy ensures that only traffic from pod3 is allowed")
		CurlMultusPod2PodFail(oc, ns, podNames[0], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodFail(oc, ns, podNames[0], pod2IPv6, "net1", pods[1].podenvname)
		CurlMultusPod2PodFail(oc, ns, podNames[1], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodFail(oc, ns, podNames[1], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod2IPv6, "net1", pods[1].podenvname)

		exutil.By("Deleting ipBlock Ingress Dual CIDRs Policy")
		removeResource(oc, true, true, "multi-networkpolicy", "multinetworkipblock-dual-cidrs-ingress", "-n", ns)
		policyoutput1, policyerr1 := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns).Output()
		o.Expect(policyerr1).NotTo(o.HaveOccurred())
		o.Expect(policyoutput1).NotTo(o.ContainSubstring("multinetworkipblock-dual-cidrs-ingress"))

		exutil.By("Verifying that there is no traffic blocked between pods after deleting policy")
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv6, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[2], pod2IPv6, "net1", pods[1].podenvname)
	})

	g.It("Author:weliang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-78125-Verify egress-ipblock policy for UDN pod's secondary interface (Layer2). [Disruptive]", func() {
		var (
			buildPruningBaseDir                         = exutil.FixturePath("testdata", "networking")
			udnCRDdualStack                             = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnPodTemplate                              = filepath.Join(buildPruningBaseDir, "multihoming/multihoming-pod-template.yaml")
			multinetworkipBlockegressTemplateDual       = filepath.Join(buildPruningBaseDir, "multihoming/multiNetworkPolicy_egress_ipblock_template.yaml")
			patchSResource                              = "networks.operator.openshift.io/cluster"
			mtu                                   int32 = 9000
		)
		exutil.By("Getting the ready-schedulable worker nodes")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("The cluster has no ready node for the testing")
		}

		exutil.By("Getting the namespace name")
		ns := oc.Namespace()
		nadName := "ipblockegress78125"
		nsWithnad := ns + "/" + nadName

		exutil.By("Enabling useMultiNetworkPolicy in the cluster")
		patchInfoTrue := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":true}}")
		patchInfoFalse := fmt.Sprintf("{\"spec\":{\"useMultiNetworkPolicy\":false}}")
		defer func() {
			patchResourceAsAdmin(oc, patchSResource, patchInfoFalse)
			exutil.By("NetworkOperatorStatus should back to normal after disable useMultiNetworkPolicy")
			waitForNetworkOperatorState(oc, 10, 5, "True.*True.*False")
			waitForNetworkOperatorState(oc, 60, 15, "True.*False.*False")
		}()
		patchResourceAsAdmin(oc, patchSResource, patchInfoTrue)

		exutil.By("Wait for the NetworkOperator to become functional after enabling useMultiNetworkPolicy")
		waitForNetworkOperatorState(oc, 10, 5, "True.*True.*False")
		waitForNetworkOperatorState(oc, 60, 15, "True.*False.*False")

		exutil.By("Creating Layer2 UDN CRD with Secondary role")
		ipv4cidr := "20.200.200.0/24"
		ipv6cidr := "2000:200:200::0/64"
		udncrd := udnCRDResource{
			crdname:   nadName,
			namespace: ns,
			role:      "Secondary",
			mtu:       mtu,
			IPv4cidr:  ipv4cidr,
			IPv6cidr:  ipv6cidr,
			template:  udnCRDdualStack,
		}
		udncrd.createLayer2DualStackUDNCRD(oc)
		err := waitUDNCRDApplied(oc, udncrd.namespace, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Creating three testing pods consuming above network-attach-definition in ns")
		var podName, podLabel, podenvName string
		pods := make([]testMultihomingPod, 3)
		var podNames []string
		for i := 0; i < 3; i++ {
			podName = "multihoming-pod-" + strconv.Itoa(i+1)
			podLabel = "multihoming-pod" + strconv.Itoa(i+1)
			podenvName = "Hello multihoming-pod-" + strconv.Itoa(i+1)
			pods[i] = testMultihomingPod{
				name:       podName,
				namespace:  ns,
				podlabel:   podLabel,
				nadname:    udncrd.crdname,
				nodename:   "",
				podenvname: podenvName,
				template:   udnPodTemplate,
			}
			pods[i].createTestMultihomingPod(oc)
			o.Expect(waitForPodWithLabelReady(oc, ns, fmt.Sprintf("name=%s", pods[i].podlabel))).NotTo(o.HaveOccurred())
			podNames = append(podNames, getPodName(oc, pods[i].namespace, fmt.Sprintf("name=%s", pods[i].podlabel))[0])
		}

		exutil.By("Verifying the all pods get dual IPs")
		pod1IPv4, pod1IPv6 := getPodMultiNetwork(oc, ns, podNames[0])
		pod2IPv4, pod2IPv6 := getPodMultiNetwork(oc, ns, podNames[1])
		pod3IPv4, pod3IPv6 := getPodMultiNetwork(oc, ns, podNames[2])
		pod3IPv4WithCidr := pod3IPv4 + "/32"
		pod3IPv6WithCidr := pod3IPv6 + "/128"

		exutil.By("Verifying that there is no traffic blocked between pods")
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv6, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod3IPv4, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod3IPv6, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod3IPv4, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod3IPv6, "net1", pods[2].podenvname)

		exutil.By("Creating ipBlock egress Dual CIDRs Policy to allow traffic only to pod3")
		defer removeResource(oc, true, true, "multi-networkpolicy", "multinetworkipblock-dual-cidrs-egress", "-n", ns)
		IPBlock := multinetworkipBlockCIDRsDual{
			name:      "multinetworkipblock-dual-cidrs-egress",
			namespace: ns,
			cidrIpv4:  pod3IPv4WithCidr,
			cidrIpv6:  pod3IPv6WithCidr,
			policyfor: nsWithnad,
			template:  multinetworkipBlockegressTemplateDual,
		}
		IPBlock.createMultinetworkipBlockCIDRDual(oc)
		policyoutput, policyerr := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns).Output()
		o.Expect(policyerr).NotTo(o.HaveOccurred())
		o.Expect(policyoutput).To(o.ContainSubstring("multinetworkipblock-dual-cidrs-egress"))

		exutil.By("Verifying the ipBlock egress Dual CIDRs policy ensures that only traffic to pod3 is allowed")
		CurlMultusPod2PodFail(oc, ns, podNames[0], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodFail(oc, ns, podNames[0], pod2IPv6, "net1", pods[1].podenvname)
		CurlMultusPod2PodFail(oc, ns, podNames[1], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodFail(oc, ns, podNames[1], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod3IPv4, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod3IPv6, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod3IPv4, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod3IPv6, "net1", pods[2].podenvname)

		exutil.By("Deleting ipBlock egress Dual CIDRs Policy")
		removeResource(oc, true, true, "multi-networkpolicy", "multinetworkipblock-dual-cidrs-egress", "-n", ns)
		policyoutput1, policyerr1 := oc.AsAdmin().Run("get").Args("multi-networkpolicy", "-n", ns).Output()
		o.Expect(policyerr1).NotTo(o.HaveOccurred())
		o.Expect(policyoutput1).NotTo(o.ContainSubstring("multinetworkipblock-dual-cidrs-egress"))

		exutil.By("Verifying that there is no traffic blocked between pods after deleting policy")
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv4, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod2IPv6, "net1", pods[1].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv4, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod1IPv6, "net1", pods[0].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod3IPv4, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[0], pod3IPv6, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod3IPv4, "net1", pods[2].podenvname)
		CurlMultusPod2PodPass(oc, ns, podNames[1], pod3IPv6, "net1", pods[2].podenvname)
	})

	g.It("Author:meinli-Medium-78329-Validate pod2pod on diff workers and host2pod on same/diff workers (UDN Layer3 with Primary role)", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)
		exutil.By("1. Get worker node and namespace")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		ns := oc.Namespace()

		exutil.By("2. Create UDN CRD Layer3 with Primary role")
		err = applyL3UDNtoNamespace(oc, ns, 0)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. Create two pods on diff workers in ns")
		pods := make([]pingPodResourceNode, 2)
		for i := 0; i < 2; i++ {
			pods[i] = pingPodResourceNode{
				name:      "hello-pod" + strconv.Itoa(i),
				namespace: ns,
				nodename:  nodeList.Items[i].Name,
				template:  pingPodNodeTemplate,
			}
			pods[i].createPingPodNode(oc)
			waitPodReady(oc, ns, pods[i].name)
		}

		exutil.By("4. Validate pod to pod on different workers")
		CurlPod2PodPassUDN(oc, ns, pods[0].name, ns, pods[1].name)

		exutil.By("5. validate host to pod on same and diff workers")
		CurlNode2PodFailUDN(oc, nodeList.Items[0].Name, ns, pods[0].name)
		CurlNode2PodFailUDN(oc, nodeList.Items[0].Name, ns, pods[1].name)
	})

	g.It("Author:qiowang-High-77542-Check default network ports can be exposed on UDN pods(layer3) [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			sctpModule          = filepath.Join(buildPruningBaseDir, "sctp/load-sctp-module.yaml")
			statefulSetHelloPod = filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
			tcpPort             = 8080
			udpPort             = 6000
			sctpPort            = 30102
		)

		exutil.By("Preparing the nodes for SCTP")
		prepareSCTPModule(oc, sctpModule)

		exutil.By("1. Create the first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create a hello pod in ns1")
		createResourceFromFile(oc, ns1, statefulSetHelloPod)
		pod1Err := waitForPodWithLabelReady(oc, ns1, "app=hello")
		exutil.AssertWaitPollNoErr(pod1Err, "The statefulSet pod is not ready")
		pod1Name := getPodName(oc, ns1, "app=hello")[0]

		exutil.By("3. Create the 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		exutil.By("4. Create CRD for UDN in ns2")
		err := applyL3UDNtoNamespace(oc, ns2, 0)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. Create a udn hello pod in ns2")
		createResourceFromFile(oc, ns2, statefulSetHelloPod)
		pod2Err := waitForPodWithLabelReady(oc, ns2, "app=hello")
		exutil.AssertWaitPollNoErr(pod2Err, "The statefulSet pod is not ready")
		pod2Name := getPodName(oc, ns2, "app=hello")[0]

		exutil.By("6. Check ICMP/TCP/UDP/SCTP traffic between pods in ns1 and ns2, should not be able to access")
		PingPod2PodFail(oc, ns1, pod1Name, ns2, pod2Name)
		CurlPod2PodFail(oc, ns1, pod1Name, ns2, pod2Name)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "UDP", udpPort, false)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "SCTP", sctpPort, false)

		exutil.By("7. Add annotation to expose default network port on udn pod")
		annotationConf := `k8s.ovn.org/open-default-ports=[{"protocol":"icmp"}, {"protocol":"tcp","port":` + strconv.Itoa(tcpPort) + `}, {"protocol":"udp","port":` + strconv.Itoa(udpPort) + `}, {"protocol":"sctp","port":` + strconv.Itoa(sctpPort) + `}]`
		err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("pod", pod2Name, "-n", ns2, "--overwrite", annotationConf).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8. Check ICMP/TCP/UDP/SCTP traffic between pods in ns1 and ns2, should be able to access")
		PingPod2PodPass(oc, ns1, pod1Name, ns2, pod2Name)
		CurlPod2PodPass(oc, ns1, pod1Name, ns2, pod2Name)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "UDP", udpPort, true)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "SCTP", sctpPort, true)
	})

	g.It("Author:qiowang-High-77742-Check default network ports can be exposed on UDN pods(layer2) [Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			sctpModule          = filepath.Join(buildPruningBaseDir, "sctp/load-sctp-module.yaml")
			udnCRDdualStack     = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_dualstack_template.yaml")
			udnCRDSingleStack   = filepath.Join(buildPruningBaseDir, "udn/udn_crd_layer2_singlestack_template.yaml")
			statefulSetHelloPod = filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
			tcpPort             = 8080
			udpPort             = 6000
			sctpPort            = 30102
		)

		exutil.By("Preparing the nodes for SCTP")
		prepareSCTPModule(oc, sctpModule)

		exutil.By("1. Create the first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create a hello pod in ns1")
		createResourceFromFile(oc, ns1, statefulSetHelloPod)
		pod1Err := waitForPodWithLabelReady(oc, ns1, "app=hello")
		exutil.AssertWaitPollNoErr(pod1Err, "The statefulSet pod is not ready")
		pod1Name := getPodName(oc, ns1, "app=hello")[0]

		exutil.By("3. Create the 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		exutil.By("4. Create CRD for UDN in ns2")
		var cidr, ipv4cidr, ipv6cidr string
		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/48"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/48"
			}
		}
		udncrd := udnCRDResource{
			crdname:   "udn-l2-network-77742",
			namespace: ns2,
			role:      "Primary",
			mtu:       1300,
		}
		if ipStackType == "dualstack" {
			udncrd.IPv4cidr = ipv4cidr
			udncrd.IPv6cidr = ipv6cidr
			udncrd.template = udnCRDdualStack
			udncrd.createLayer2DualStackUDNCRD(oc)
		} else {
			udncrd.cidr = cidr
			udncrd.template = udnCRDSingleStack
			udncrd.createLayer2SingleStackUDNCRD(oc)
		}
		err := waitUDNCRDApplied(oc, ns2, udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. Create a udn hello pod in ns2")
		createResourceFromFile(oc, ns2, statefulSetHelloPod)
		pod2Err := waitForPodWithLabelReady(oc, ns2, "app=hello")
		exutil.AssertWaitPollNoErr(pod2Err, "The statefulSet pod is not ready")
		pod2Name := getPodName(oc, ns2, "app=hello")[0]

		exutil.By("6. Check ICMP/TCP/UDP/SCTP traffic between pods in ns1 and ns2, should not be able to access")
		PingPod2PodFail(oc, ns1, pod1Name, ns2, pod2Name)
		CurlPod2PodFail(oc, ns1, pod1Name, ns2, pod2Name)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "UDP", udpPort, false)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "SCTP", sctpPort, false)

		exutil.By("7. Add annotation to expose default network port on udn pod")
		annotationConf := `k8s.ovn.org/open-default-ports=[{"protocol":"icmp"}, {"protocol":"tcp","port":` + strconv.Itoa(tcpPort) + `}, {"protocol":"udp","port":` + strconv.Itoa(udpPort) + `}, {"protocol":"sctp","port":` + strconv.Itoa(sctpPort) + `}]`
		err = oc.AsAdmin().WithoutNamespace().Run("annotate").Args("pod", pod2Name, "-n", ns2, "--overwrite", annotationConf).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8. Check ICMP/TCP/UDP/SCTP traffic between pods in ns1 and ns2, should be able to access")
		PingPod2PodPass(oc, ns1, pod1Name, ns2, pod2Name)
		CurlPod2PodPass(oc, ns1, pod1Name, ns2, pod2Name)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "UDP", udpPort, true)
		verifyConnPod2Pod(oc, ns1, pod1Name, ns2, pod2Name, "SCTP", sctpPort, true)
	})

	g.It("Author:meinli-Medium-78492-[CUDN layer3] Validate CUDN enable creating shared OVN network across multiple namespaces", func() {
		var (
			cudnCRDL3dualStack         = filepath.Join(testDataDirUDN, "cudn_crd_dualstack_template.yaml")
			cudnCRDL3SingleStack       = filepath.Join(testDataDirUDN, "cudn_crd_singlestack_template.yaml")
			udnPodTemplate             = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			matchLabelKey              = "test.io"
			matchValue                 = "cudn-network"
			mtu                  int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create three namespaces and label two of them as cudn selector")
		var allNS []string
		for i := 0; i < 3; i++ {
			if i == 0 {
				allNS = append(allNS, oc.Namespace())
			} else {
				oc.SetupProject()
				allNS = append(allNS, oc.Namespace())
			}
			if i < 2 {
				ns := allNS[i]
				defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, fmt.Sprintf("%s-", matchLabelKey)).Execute()
				err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, fmt.Sprintf("%s=%s", matchLabelKey, matchValue)).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}

		exutil.By("2. create CUDN with two namespaces")
		var cudncrd cudnCRDResource
		var cidr, ipv4cidr, ipv6cidr string
		var prefix, ipv4prefix, ipv6prefix int32
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
			prefix = 24
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
				prefix = 64
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv4prefix = 24
				ipv6cidr = "2010:100:200::0/60"
				ipv6prefix = 64
			}
		}
		if ipStackType == "dualstack" {
			cudncrd = cudnCRDResource{
				crdname:    "cudn-network-78492",
				labelvalue: matchValue,
				labelkey:   matchLabelKey,
				role:       "Primary",
				mtu:        mtu,
				IPv4cidr:   ipv4cidr,
				IPv4prefix: ipv4prefix,
				IPv6cidr:   ipv6cidr,
				IPv6prefix: ipv6prefix,
				template:   cudnCRDL3dualStack,
			}
			defer removeResource(oc, true, true, "clusteruserdefinednetwork", cudncrd.crdname)
			cudncrd.createCUDNCRDDualStack(oc)
		} else {
			cudncrd = cudnCRDResource{
				crdname:    "cudn-network-78492",
				labelvalue: matchValue,
				labelkey:   matchLabelKey,
				role:       "Primary",
				mtu:        mtu,
				cidr:       cidr,
				prefix:     prefix,
				template:   cudnCRDL3SingleStack,
			}
			defer removeResource(oc, true, true, "clusteruserdefinednetwork", cudncrd.crdname)
			cudncrd.createCUDNCRDSingleStack(oc)
		}
		err := waitCUDNCRDApplied(oc, cudncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. create pods in ns1 and ns2, one pod in ns3")
		pods := make([]udnPodResource, 3)
		for i := 0; i < 3; i++ {
			pods[i] = udnPodResource{
				name:      "hello-pod-" + allNS[i],
				namespace: allNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			defer removeResource(oc, true, true, "pod", pods[i].name, "-n", pods[i].namespace)
			pods[i].createUdnPod(oc)
			waitPodReady(oc, pods[i].namespace, pods[i].name)
		}

		exutil.By("4. check pods' interfaces")
		for i := 0; i < 2; i++ {
			podIP, _ := getPodIPUDN(oc, pods[i].namespace, pods[i].name, "ovn-udn1")
			o.Expect(podIP).NotTo(o.BeEmpty())
		}
		output, err := e2eoutput.RunHostCmd(pods[2].namespace, pods[2].name, "ip -o link show")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).ShouldNot(o.ContainSubstring("ovn-udn1"))

		exutil.By("5. Validate CUDN pod traffic")
		CurlPod2PodPassUDN(oc, pods[0].namespace, pods[0].name, pods[1].namespace, pods[1].name)
	})

	g.It("Author:meinli-Medium-78598-[CUDN layer2] Validate CUDN enable creating shared OVN network across multiple namespaces", func() {
		var (
			cudnCRDL2dualStack         = filepath.Join(testDataDirUDN, "cudn_crd_layer2_dualstack_template.yaml")
			cudnCRDL2SingleStack       = filepath.Join(testDataDirUDN, "cudn_crd_layer2_singlestack_template.yaml")
			udnPodTemplate             = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			matchLabelKey              = "test.io"
			matchValue                 = "cudn-network"
			mtu                  int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		exutil.By("1. Create three namespaces and label two of them as cudn selector")
		var allNS []string
		for i := 0; i < 3; i++ {
			if i == 0 {
				allNS = append(allNS, oc.Namespace())
			} else {
				oc.SetupProject()
				allNS = append(allNS, oc.Namespace())
			}
			if i < 2 {
				ns := allNS[i]
				defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, fmt.Sprintf("%s-", matchLabelKey)).Execute()
				err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, fmt.Sprintf("%s=%s", matchLabelKey, matchValue)).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}

		exutil.By("2. create CUDN with two namespaces")
		var cudncrd cudnCRDResource
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/60"
			}
		}
		if ipStackType == "dualstack" {
			cudncrd = cudnCRDResource{
				crdname:    "cudn-network-78598",
				labelvalue: matchValue,
				labelkey:   matchLabelKey,
				role:       "Primary",
				mtu:        mtu,
				IPv4cidr:   ipv4cidr,
				IPv6cidr:   ipv6cidr,
				template:   cudnCRDL2dualStack,
			}
			defer removeResource(oc, true, true, "clusteruserdefinednetwork", cudncrd.crdname)
			cudncrd.createLayer2DualStackCUDNCRD(oc)
		} else {
			cudncrd = cudnCRDResource{
				crdname:    "cudn-network-78598",
				labelvalue: matchValue,
				labelkey:   matchLabelKey,
				role:       "Primary",
				mtu:        mtu,
				cidr:       cidr,
				template:   cudnCRDL2SingleStack,
			}
			defer removeResource(oc, true, true, "clusteruserdefinednetwork", cudncrd.crdname)
			cudncrd.createLayer2SingleStackCUDNCRD(oc)
		}
		err := waitCUDNCRDApplied(oc, cudncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. create pods in ns1 and ns2, one pod in ns3")
		pods := make([]udnPodResource, 3)
		for i := 0; i < 3; i++ {
			pods[i] = udnPodResource{
				name:      "hello-pod-" + allNS[i],
				namespace: allNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			defer removeResource(oc, true, true, "pod", pods[i].name, "-n", pods[i].namespace)
			pods[i].createUdnPod(oc)
			waitPodReady(oc, pods[i].namespace, pods[i].name)
		}

		exutil.By("4. check pods' interfaces")
		for i := 0; i < 2; i++ {
			podIP, _ := getPodIPUDN(oc, pods[i].namespace, pods[i].name, "ovn-udn1")
			o.Expect(podIP).NotTo(o.BeEmpty())
		}
		output, err := e2eoutput.RunHostCmd(pods[2].namespace, pods[2].name, "ip -o link show")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).ShouldNot(o.ContainSubstring("ovn-udn1"))

		exutil.By("5. Validate CUDN pod traffic")
		CurlPod2PodPassUDN(oc, pods[0].namespace, pods[0].name, pods[1].namespace, pods[1].name)
	})

	g.It("Author:anusaxen-Low-77752-Check udn pods isolation with udn crd and native NAD integration", func() {
		var (
			buildPruningBaseDir       = exutil.FixturePath("testdata", "networking")
			udnNadtemplate            = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate            = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			udnCRDSingleStack         = filepath.Join(buildPruningBaseDir, "udn/udn_crd_singlestack_template.yaml")
			mtu                 int32 = 1300
		)
		ipStackType := checkIPStackType(oc)
		o.Expect(ipStackType).NotTo(o.BeEmpty())
		if ipStackType != "ipv4single" {
			g.Skip("This case requires IPv4 single stack cluster")
		}

		var cidr string
		var prefix int32
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
			prefix = 24
		}

		exutil.By("1. Create first namespace")
		ns1 := oc.Namespace()

		exutil.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		nadNS := []string{ns1, ns2}
		nadResourcename := []string{"l3-network-" + nadNS[0], "l3-network-" + nadNS[1]}

		exutil.By(fmt.Sprintf("create native NAD %s in namespace %s", nadResourcename[0], nadNS[0]))
		nad := udnNetDefResource{
			nadname:             nadResourcename[0],
			namespace:           nadNS[0],
			nad_network_name:    nadResourcename[0],
			topology:            "layer3",
			subnet:              "10.150.0.0/16/24",
			mtu:                 mtu,
			net_attach_def_name: nadNS[0] + "/" + nadResourcename[0],
			role:                "primary",
			template:            udnNadtemplate,
		}
		nad.createUdnNad(oc)

		exutil.By(fmt.Sprintf("create crd NAD %s in namespace %s", nadResourcename[1], nadNS[1]))
		udncrd := udnCRDResource{
			crdname:   nadResourcename[1],
			namespace: nadNS[1],
			role:      "Primary",
			mtu:       mtu,
			cidr:      cidr,
			prefix:    prefix,
			template:  udnCRDSingleStack,
		}
		udncrd.createUdnCRDSingleStack(oc)

		err := waitUDNCRDApplied(oc, nadNS[1], udncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())

		pod := make([]udnPodResource, 2)
		for i := 0; i < 2; i++ {
			exutil.By("create a udn hello pod in ns1 and ns2")
			pod[i] = udnPodResource{
				name:      "hello-pod",
				namespace: nadNS[i],
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			pod[i].createUdnPod(oc)
			waitPodReady(oc, pod[i].namespace, pod[i].name)
		}

		//udn network connectivity should be isolated
		CurlPod2PodFailUDN(oc, nadNS[0], pod[0].name, nadNS[1], pod[1].name)
		//default network connectivity should also be isolated
		CurlPod2PodFail(oc, nadNS[0], pod[0].name, nadNS[1], pod[1].name)
	})

	g.It("Author:meinli-Medium-79003-[CUDN layer3] Verify that patching namespaces for existing CUDN functionality operate as intended", func() {
		var (
			udnPodTemplate = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			key            = "test.cudn.layer3"
			crdName        = "cudn-network-79003"
			values         = []string{"value-79003-1", "value-79003-2"}
		)

		exutil.By("1. create two namespaces and label them")
		allNS := []string{oc.Namespace()}
		oc.SetupProject()
		allNS = append(allNS, oc.Namespace())
		for i := 0; i < 2; i++ {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", allNS[i], fmt.Sprintf("%s-", key)).Execute()
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", allNS[i], fmt.Sprintf("%s=%s", key, values[i])).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("2. create CUDN in ns1")
		ipStackType := checkIPStackType(oc)
		var cidr, ipv4cidr, ipv6cidr string
		if ipStackType == "ipv4single" {
			cidr = "10.150.0.0/16"
		} else {
			if ipStackType == "ipv6single" {
				cidr = "2010:100:200::0/60"
			} else {
				ipv4cidr = "10.150.0.0/16"
				ipv6cidr = "2010:100:200::0/60"
			}
		}

		defer removeResource(oc, true, true, "clusteruserdefinednetwork", crdName)
		cudncrd, err := createCUDNCRD(oc, key, crdName, ipv4cidr, ipv6cidr, cidr, "layer3", []string{values[0], ""})
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3. patch namespaces for CUDN")
		patchCmd := fmt.Sprintf("{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"%s\", \"operator\": \"In\", \"values\": [\"%s\", \"%s\"]}]}}}", key, values[0], values[1])
		patchResourceAsAdmin(oc, fmt.Sprintf("clusteruserdefinednetwork.k8s.ovn.org/%s", cudncrd.crdname), patchCmd)

		err = waitCUDNCRDApplied(oc, cudncrd.crdname)
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteruserdefinednetwork.k8s.ovn.org", cudncrd.crdname, "-ojsonpath={.status.conditions[*].message}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(allNS[1]))

		exutil.By("4. create pods in ns1 and ns2")
		pods := make([]udnPodResource, 2)
		for i, ns := range allNS {
			pods[i] = udnPodResource{
				name:      "hello-pod-" + ns,
				namespace: ns,
				label:     "hello-pod",
				template:  udnPodTemplate,
			}
			defer removeResource(oc, true, true, "pod", pods[i].name, "-n", pods[i].namespace)
			pods[i].createUdnPod(oc)
			waitPodReady(oc, pods[i].namespace, pods[i].name)
		}

		exutil.By("5. validate connection from CUDN pod to CUDN pod")
		CurlPod2PodPassUDN(oc, pods[0].namespace, pods[0].name, pods[1].namespace, pods[1].name)

		exutil.By("6. unlabel ns2")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", allNS[1], fmt.Sprintf("%s-", key)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCUDNCRDApplied(oc, cudncrd.crdname)
		o.Expect(err).To(o.HaveOccurred())
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteruserdefinednetwork.k8s.ovn.org", cudncrd.crdname, "-ojsonpath={.status.conditions[*].message}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(fmt.Sprintf("failed to delete NetworkAttachmentDefinition [%s/%s]", allNS[1], cudncrd.crdname)))

		exutil.By("7. validate connection from CUDN pod to CUDN pod")
		CurlPod2PodPassUDN(oc, pods[0].namespace, pods[0].name, pods[1].namespace, pods[1].name)
	})
})
