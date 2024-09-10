package networking

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN udn", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("networking-udn", exutil.KubeConfigPath())
		testDataDirUDN = exutil.FixturePath("testdata", "networking/udn")
	)

	g.BeforeEach(func() {

		SkipIfNoFeatureGate(oc, "NetworkSegmentation")
	})

	g.It("Author:anusaxen-Critical-74921-Check udn pods isolation on user defined networks", func() {
		var (
			udnNadtemplate       = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate       = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			mtu            int32 = 1300
		)

		ipStackType := checkIPStackType(oc)
		g.By("1. Create first namespace")
		ns1 := oc.Namespace()

		g.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		if ipStackType == "ipv4single" {
			g.By("create NAD for ns1")
			nad1 := udnNetDefResource{
				nadname:             "l3-network-" + ns1,
				namespace:           ns1,
				nad_network_name:    "l3-network-" + ns1,
				topology:            "layer3",
				subnet:              "10.150.0.0/16/24",
				mtu:                 mtu,
				net_attach_def_name: ns1 + "/l3-network-" + ns1,
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad1.createUdnNad(oc)

			g.By("create NAD for ns2")
			nad2 := udnNetDefResource{
				nadname:             "l3-network-" + ns2,
				namespace:           ns2,
				nad_network_name:    "l3-network-" + ns2,
				topology:            "layer3",
				subnet:              "10.151.0.0/16/24",
				mtu:                 mtu,
				net_attach_def_name: ns2 + "/l3-network-" + ns2,
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad2.createUdnNad(oc)

		} else {
			if ipStackType == "ipv6single" {
				g.By("create NAD for ns1")
				nad1 := udnNetDefResource{
					nadname:             "l3-network-" + ns1,
					namespace:           ns1,
					nad_network_name:    "l3-network-" + ns1,
					topology:            "layer3",
					subnet:              "2010:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns1 + "/l3-network-" + ns1,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad1.createUdnNad(oc)

				g.By("create NAD for ns2")
				nad2 := udnNetDefResource{
					nadname:             "l3-network-" + ns2,
					namespace:           ns2,
					nad_network_name:    "l3-network-" + ns2,
					topology:            "layer3",
					subnet:              "2011:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns2 + "/l3-network-" + ns2,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad2.createUdnNad(oc)

			} else {
				g.By("create NAD for ns1")
				nad1 := udnNetDefResource{
					nadname:             "l3-network-" + ns1,
					namespace:           ns1,
					nad_network_name:    "l3-network-" + ns1,
					topology:            "layer3",
					subnet:              "10.150.0.0/16/24,2010:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns1 + "/l3-network-" + ns1,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad1.createUdnNad(oc)

				g.By("create NAD for ns2")
				nad2 := udnNetDefResource{
					nadname:             "l3-network-" + ns2,
					namespace:           ns2,
					nad_network_name:    "l3-network-" + ns2,
					topology:            "layer3",
					subnet:              "10.151.0.0/16/24,2011:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns2 + "/l3-network-" + ns2,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad2.createUdnNad(oc)
			}
		}
		g.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("create a udn hello pod in ns2")
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
		g.By("1. Create first namespace")
		ns1 := oc.Namespace()

		g.By("2. Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		if ipStackType == "ipv4single" {
			g.By("create NAD for ns1")
			nad1 := udnNetDefResource{
				nadname:             "l3-network-" + ns1,
				namespace:           ns1,
				nad_network_name:    "l3-network-" + ns1,
				topology:            "layer3",
				subnet:              "10.150.0.0/16/24",
				mtu:                 mtu,
				net_attach_def_name: ns1 + "/l3-network-" + ns1,
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad1.createUdnNad(oc)

			g.By("create NAD for ns2")
			nad2 := udnNetDefResource{
				nadname:             "l3-network-" + ns2,
				namespace:           ns2,
				nad_network_name:    "l3-network-" + ns1, //network name is same as in ns1
				topology:            "layer3",
				subnet:              "10.150.0.0/16/24",
				mtu:                 mtu,
				net_attach_def_name: ns2 + "/l3-network-" + ns2,
				role:                "primary",
				template:            udnNadtemplate,
			}
			nad2.createUdnNad(oc)

		} else {
			if ipStackType == "ipv6single" {
				g.By("create NAD for ns1")
				nad1 := udnNetDefResource{
					nadname:             "l3-network-" + ns1,
					namespace:           ns1,
					nad_network_name:    "l3-network-" + ns1,
					topology:            "layer3",
					subnet:              "2010:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns1 + "/l3-network-" + ns1,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad1.createUdnNad(oc)

				g.By("create NAD for ns2")
				nad2 := udnNetDefResource{
					nadname:             "l3-network-" + ns2,
					namespace:           ns2,
					nad_network_name:    "l3-network-" + ns1, //network name is same as in ns1
					topology:            "layer3",
					subnet:              "2010:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns2 + "/l3-network-" + ns2,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad2.createUdnNad(oc)

			} else {
				g.By("create NAD for ns1")
				nad1 := udnNetDefResource{
					nadname:             "l3-network-" + ns1,
					namespace:           ns1,
					nad_network_name:    "l3-network-" + ns1,
					topology:            "layer3",
					subnet:              "10.150.0.0/16/24,2010:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns1 + "/l3-network-" + ns1,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad1.createUdnNad(oc)

				g.By("create NAD for ns2")
				nad2 := udnNetDefResource{
					nadname:             "l3-network-" + ns2,
					namespace:           ns2,
					nad_network_name:    "l3-network-" + ns1, //network name is same as in ns1
					topology:            "layer3",
					subnet:              "10.150.0.0/16/24,2010:100:200::0/60",
					mtu:                 mtu,
					net_attach_def_name: ns2 + "/l3-network-" + ns2,
					role:                "primary",
					template:            udnNadtemplate,
				}
				nad2.createUdnNad(oc)
			}
		}
		g.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("create a udn hello pod in ns2")
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
		g.By("1. Create first namespace")
		ns1 := oc.Namespace()

		g.By("2. Create 2nd namespace")
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

		g.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("create a udn hello pod in ns2")
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
		g.By("1. Create first namespace")
		ns1 := oc.Namespace()

		g.By("2. Create 2nd namespace")
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
		g.By("create a udn hello pod in ns1")
		pod1 := udnPodResource{
			name:      "hello-pod-ns1",
			namespace: ns1,
			label:     "hello-pod",
			template:  udnPodTemplate,
		}
		pod1.createUdnPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("create a udn hello pod in ns2")
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
			waitForNetworkOperatorState(oc, 100, 5, "True.*True.*False")
			waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")
		}()
		patchResourceAsAdmin(oc, patchSResource, patchInfoTrue)
		waitForNetworkOperatorState(oc, 100, 5, "True.*True.*False")
		waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")

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
})
