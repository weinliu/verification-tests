package networking

import (
	"fmt"
	"path/filepath"
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
		masterNode, getMasterNodeErr := exutil.GetFirstMasterNode(oc)
		o.Expect(getMasterNodeErr).NotTo(o.HaveOccurred())

		ovnkubePod, getPodErr := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", masterNode)
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
					nad_network_name:    "l3-network-" + ns1, //network name is same as in ns1
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
					nad_network_name:    "l3-network-" + ns1, //network name is same as in ns1
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
})
