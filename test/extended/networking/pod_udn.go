package networking

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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
})
