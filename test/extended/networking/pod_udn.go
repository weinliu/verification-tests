package networking

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN udn", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("networking-udn", exutil.KubeConfigPath())
		testDataDirUDN = exutil.FixturePath("testdata", "networking/udn")
	)

	g.BeforeEach(func() {

		networkType := checkNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("This is required to run on OVNKubernetes Network Backened")
		}
		workerNode, getWorkerNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getWorkerNodeErr).NotTo(o.HaveOccurred())

		ovnkubePod, getPodErr := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", workerNode)
		o.Expect(getPodErr).NotTo(o.HaveOccurred())

		//following checks are needed until udn feature gets GA'ed
		expectedString := "EnableNetworkSegmentation"
		podLogs, LogErr := checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-controller", ovnkubePod, expectedString)
		o.Expect(LogErr).NotTo(o.HaveOccurred())

		if !strings.Contains(podLogs, "EnableNetworkSegmentation:true") {
			g.Skip("This case is required to run on network segmentation enabled cluster")
		}
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

	g.It("Author:huirwang-Medium-75658-Check sctp traffic work well via udn pods user defined networks.	[Disruptive]", func() {
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
})
