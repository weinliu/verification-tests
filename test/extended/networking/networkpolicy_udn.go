package networking

import (
	"fmt"
	"path/filepath"
	"strconv"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN udn networkpolicy", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("networking-udn", exutil.KubeConfigPath())
		testDataDirUDN = exutil.FixturePath("testdata", "networking/udn")
	)

	g.BeforeEach(func() {

		SkipIfNoFeatureGate(oc, "NetworkSegmentation")
	})

	g.It("Author:asood-High-78292-Validate ingress allow-same-namespace and allow-all-namespaces network policies in Layer 3 NAD.", func() {
		var (
			testID                       = "78292"
			testDataDir                  = exutil.FixturePath("testdata", "networking")
			udnNADTemplate               = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate               = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			ingressDenyFile              = filepath.Join(testDataDir, "networkpolicy/default-deny-ingress.yaml")
			ingressAllowSameNSFile       = filepath.Join(testDataDir, "networkpolicy/allow-from-same-namespace.yaml")
			ingressAllowAllNSFile        = filepath.Join(testDataDir, "networkpolicy/allow-from-all-namespaces.yaml")
			mtu                    int32 = 1300
			nsPodMap                     = make(map[string][]string)
			nadResourcename              = "l3-network-"
			topology                     = "layer3"
		)

		ipStackType := checkIPStackType(oc)
		var nadName string
		var nadNS []string = make([]string, 0, 4)
		nsDefaultNetwork := oc.Namespace()
		nadNetworkName := []string{"l3-network-test-1", "l3-network-test-2"}

		exutil.By("1.0 Create 4 UDN namespaces")
		for i := 0; i < 4; i++ {
			oc.CreateNamespaceUDN()
			nadNS = append(nadNS, oc.Namespace())
		}
		nadNS = append(nadNS, nsDefaultNetwork)
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16/24", "10.152.0.0/16/24"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2010:100:200::0/60", "2012:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16/24,2010:100:200::0/60", "10.152.0.0/16/24,2012:100:200::0/60"}
			}
		}
		exutil.By("2. Create Layer 3 NAD in first two namespaces")
		// Same network name in both namespaces
		nad := make([]udnNetDefResource, 4)
		for i := 0; i < 2; i++ {
			nadName = nadResourcename + strconv.Itoa(i) + "-" + testID
			if i == 1 {
				o.Expect(oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", nadNS[i], "team=ocp").Execute()).NotTo(o.HaveOccurred())

			}
			exutil.By(fmt.Sprintf("Create NAD %s in namespace %s", nadName, nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadName,
				namespace:           nadNS[i],
				nad_network_name:    nadNetworkName[0],
				topology:            topology,
				subnet:              subnet[0],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadName,
				role:                "primary",
				template:            udnNADTemplate,
			}
			nad[i].createUdnNad(oc)

		}
		exutil.By("3. Create two pods in each namespace")
		pod := make([]udnPodResource, 4)
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				pod[j] = udnPodResource{
					name:      "hello-pod-" + testID + "-" + strconv.Itoa(i) + "-" + strconv.Itoa(j),
					namespace: nadNS[i],
					label:     "hello-pod",
					template:  udnPodTemplate,
				}
				pod[j].createUdnPod(oc)
				waitPodReady(oc, pod[j].namespace, pod[j].name)
				nsPodMap[pod[j].namespace] = append(nsPodMap[pod[j].namespace], pod[j].name)
			}
		}
		CurlPod2PodPassUDN(oc, nadNS[1], nsPodMap[nadNS[1]][0], nadNS[0], nsPodMap[nadNS[0]][0])
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][1], nadNS[0], nsPodMap[nadNS[0]][0])

		exutil.By("4. Create default deny ingress type networkpolicy in first namespace")
		createResourceFromFile(oc, nadNS[0], ingressDenyFile)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", nadNS[0]).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-deny-ingress"))

		exutil.By("5. Validate traffic between pods in first namespace and from pods in second namespace")
		CurlPod2PodFailUDN(oc, nadNS[1], nsPodMap[nadNS[1]][0], nadNS[0], nsPodMap[nadNS[0]][0])
		CurlPod2PodFailUDN(oc, nadNS[0], nsPodMap[nadNS[0]][1], nadNS[0], nsPodMap[nadNS[0]][0])

		exutil.By("6. Create allow same namespace ingress type networkpolicy in first namespace")
		createResourceFromFile(oc, nadNS[0], ingressAllowSameNSFile)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", nadNS[0]).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-from-same-namespace"))

		exutil.By("7. Validate traffic between pods in first namespace works but traffic from pod in second namespace is blocked")
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][1], nadNS[0], nsPodMap[nadNS[0]][0])
		CurlPod2PodFailUDN(oc, nadNS[1], nsPodMap[nadNS[1]][0], nadNS[0], nsPodMap[nadNS[0]][0])

		exutil.By("8. Create allow ingress from all namespaces networkpolicy in first namespace")
		createResourceFromFile(oc, nadNS[0], ingressAllowAllNSFile)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", nadNS[0]).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-from-all-namespaces"))

		exutil.By("9. Validate traffic from pods in second namespace")
		CurlPod2PodPassUDN(oc, nadNS[1], nsPodMap[nadNS[1]][0], nadNS[0], nsPodMap[nadNS[0]][0])

		exutil.By(fmt.Sprintf("10. Create NAD with same network %s in namespace %s as the first two namespaces and %s  (different network) in %s", nadNetworkName[0], nadNS[2], nadNetworkName[1], nadNS[3]))
		for i := 2; i < 4; i++ {
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename + strconv.Itoa(i) + "-" + testID,
				namespace:           nadNS[i],
				nad_network_name:    nadNetworkName[i-2],
				topology:            topology,
				subnet:              subnet[i-2],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename + strconv.Itoa(i) + "-" + testID,
				role:                "primary",
				template:            udnNADTemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("11. Create one pod each in last three namespaces, last one being without NAD")
		pod = make([]udnPodResource, 6)
		for i := 2; i < 5; i++ {
			for j := 0; j < 1; j++ {
				pod[j] = udnPodResource{
					name:      "hello-pod-" + testID + "-" + strconv.Itoa(i) + "-" + strconv.Itoa(j),
					namespace: nadNS[i],
					label:     "hello-pod",
					template:  udnPodTemplate,
				}
				pod[j].createUdnPod(oc)
				waitPodReady(oc, pod[j].namespace, pod[j].name)
				nsPodMap[pod[j].namespace] = append(nsPodMap[pod[j].namespace], pod[j].name)
			}
		}
		exutil.By("12. Validate traffic from pods in third and fourth namespace works but not from pod in fifth namespace (default)")
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[2], nsPodMap[nadNS[2]][0])
		CurlPod2PodFailUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[3], nsPodMap[nadNS[3]][0])
		CurlPod2PodFail(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[4], nsPodMap[nadNS[4]][0])

		exutil.By("13. Update allow-all-namespaces policy with label to allow ingress traffic from pod in second namespace only")
		npPatch := `[{"op": "replace", "path": "/spec/ingress/0/from/0/namespaceSelector", "value": {"matchLabels": {"team": "ocp" }}}]`
		patchReplaceResourceAsAdmin(oc, "networkpolicy/allow-from-all-namespaces", npPatch, nadNS[0])
		npRules, npErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "allow-from-all-namespaces", "-n", nadNS[0], "-o=jsonpath={.spec}").Output()
		o.Expect(npErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n Network policy after update: %s", npRules)

		exutil.By("14. Validate traffic from pods in second namespace works but fails from pod in third namespace")
		CurlPod2PodPassUDN(oc, nadNS[1], nsPodMap[nadNS[1]][0], nadNS[0], nsPodMap[nadNS[0]][0])
		CurlPod2PodFailUDN(oc, nadNS[2], nsPodMap[nadNS[2]][0], nadNS[0], nsPodMap[nadNS[0]][0])

	})

	g.It("Author:asood-High-79092-Validate egress allow-same-namespace and allow-all-namespaces network policies in Layer 2 NAD.", func() {
		var (
			testID                      = "79092"
			testDataDir                 = exutil.FixturePath("testdata", "networking")
			udnNADTemplate              = filepath.Join(testDataDirUDN, "udn_nad_template.yaml")
			udnPodTemplate              = filepath.Join(testDataDirUDN, "udn_test_pod_template.yaml")
			egressDenyFile              = filepath.Join(testDataDir, "networkpolicy/default-deny-egress.yaml")
			egressAllowSameNSFile       = filepath.Join(testDataDir, "networkpolicy/allow-to-same-namespace.yaml")
			egressAllowAllNSFile        = filepath.Join(testDataDir, "networkpolicy/allow-to-all-namespaces.yaml")
			mtu                   int32 = 1300
			nsPodMap                    = make(map[string][]string)
			nadResourcename             = "l2-network-"
			topology                    = "layer2"
		)

		ipStackType := checkIPStackType(oc)
		var nadName string
		var nadNS []string = make([]string, 0, 4)
		nadNetworkName := []string{"l2-network-test-1", "l2-network-test-2"}
		nsDefaultNetwork := oc.Namespace()

		exutil.By("1.0 Create 4 UDN namespaces")
		for i := 0; i < 4; i++ {
			oc.CreateNamespaceUDN()
			nadNS = append(nadNS, oc.Namespace())
		}
		nadNS = append(nadNS, nsDefaultNetwork)
		var subnet []string
		if ipStackType == "ipv4single" {
			subnet = []string{"10.150.0.0/16", "10.152.0.0/16"}
		} else {
			if ipStackType == "ipv6single" {
				subnet = []string{"2012:100:200::0/60"}
			} else {
				subnet = []string{"10.150.0.0/16,2010:100:200::0/60", "10.152.0.0/16,2012:100:200::0/60"}
			}
		}

		exutil.By("2. Create Layer 2 NAD in first two namespaces")
		nad := make([]udnNetDefResource, 4)
		for i := 0; i < 2; i++ {
			nadName = nadResourcename + strconv.Itoa(i) + "-" + testID
			if i == 1 {
				o.Expect(oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", nadNS[i], "team=ocp").Execute()).NotTo(o.HaveOccurred())

			}
			exutil.By(fmt.Sprintf("Create NAD %s in namespace %s", nadName, nadNS[i]))
			nad[i] = udnNetDefResource{
				nadname:             nadName,
				namespace:           nadNS[i],
				nad_network_name:    nadNetworkName[0],
				topology:            topology,
				subnet:              subnet[0],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadName,
				role:                "primary",
				template:            udnNADTemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("3. Create two pods in each namespace")
		pod := make([]udnPodResource, 4)
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				pod[j] = udnPodResource{
					name:      "hello-pod-" + testID + "-" + strconv.Itoa(i) + "-" + strconv.Itoa(j),
					namespace: nadNS[i],
					label:     "hello-pod",
					template:  udnPodTemplate,
				}
				pod[j].createUdnPod(oc)
				waitPodReady(oc, pod[j].namespace, pod[j].name)
				nsPodMap[pod[j].namespace] = append(nsPodMap[pod[j].namespace], pod[j].name)
			}
		}
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[1], nsPodMap[nadNS[1]][0])
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[0], nsPodMap[nadNS[0]][1])

		exutil.By("4. Create default deny egresss type networkpolicy in first namespace")
		createResourceFromFile(oc, nadNS[0], egressDenyFile)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", nadNS[0]).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-deny-egress"))

		exutil.By("5. Validate traffic between pods in first namespace and from pods in second namespace")
		CurlPod2PodFailUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[1], nsPodMap[nadNS[1]][0])
		CurlPod2PodFailUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[0], nsPodMap[nadNS[0]][1])

		exutil.By("6. Create allow egress to same namespace networkpolicy in first namespace")
		createResourceFromFile(oc, nadNS[0], egressAllowSameNSFile)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", nadNS[0]).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-to-same-namespace"))

		exutil.By("7. Validate traffic between pods in first namespace works but traffic from pod in second namespace is blocked")
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[0], nsPodMap[nadNS[0]][1])
		CurlPod2PodFailUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[1], nsPodMap[nadNS[1]][0])

		exutil.By("8. Create allow all namespaces egress type networkpolicy in first namespace")
		createResourceFromFile(oc, nadNS[0], egressAllowAllNSFile)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "-n", nadNS[0]).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-to-all-namespaces"))

		exutil.By("9. Validate traffic to pods in second namespace")
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[1], nsPodMap[nadNS[1]][0])

		exutil.By(fmt.Sprintf("10. Create NAD with same network %s in namespace %s as the first two namespaces and %s  (different network) in %s", nadNetworkName[0], nadNS[2], nadNetworkName[1], nadNS[3]))
		for i := 2; i < 4; i++ {
			nad[i] = udnNetDefResource{
				nadname:             nadResourcename + strconv.Itoa(i) + "-" + testID,
				namespace:           nadNS[i],
				nad_network_name:    nadNetworkName[i-2],
				topology:            topology,
				subnet:              subnet[i-2],
				mtu:                 mtu,
				net_attach_def_name: nadNS[i] + "/" + nadResourcename + strconv.Itoa(i) + "-" + testID,
				role:                "primary",
				template:            udnNADTemplate,
			}
			nad[i].createUdnNad(oc)
		}

		exutil.By("11. Create one pod each in last three namespaces, last one being without NAD")
		pod = make([]udnPodResource, 6)
		for i := 2; i < 5; i++ {
			for j := 0; j < 1; j++ {
				pod[j] = udnPodResource{
					name:      "hello-pod-" + testID + "-" + strconv.Itoa(i) + "-" + strconv.Itoa(j),
					namespace: nadNS[i],
					label:     "hello-pod",
					template:  udnPodTemplate,
				}
				pod[j].createUdnPod(oc)
				waitPodReady(oc, pod[j].namespace, pod[j].name)
				nsPodMap[pod[j].namespace] = append(nsPodMap[pod[j].namespace], pod[j].name)
			}
		}

		exutil.By("12. Validate traffic to pods in third and fourth namespace works but not to pod in fifth namespace (default)")
		CurlPod2PodPassUDN(oc, nadNS[2], nsPodMap[nadNS[2]][0], nadNS[0], nsPodMap[nadNS[0]][0])
		CurlPod2PodFailUDN(oc, nadNS[3], nsPodMap[nadNS[3]][0], nadNS[0], nsPodMap[nadNS[0]][0])
		CurlPod2PodFail(oc, nadNS[4], nsPodMap[nadNS[4]][0], nadNS[0], nsPodMap[nadNS[0]][0])

		exutil.By("13. Update allow-all-namespaces policy with label to allow ingress traffic to pod in second namespace only")
		npPatch := `[{"op": "replace", "path": "/spec/egress/0/to/0/namespaceSelector", "value": {"matchLabels": {"team": "ocp" }}}]`
		patchReplaceResourceAsAdmin(oc, "networkpolicy/allow-to-all-namespaces", npPatch, nadNS[0])
		npRules, npErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("networkpolicy", "allow-to-all-namespaces", "-n", nadNS[0], "-o=jsonpath={.spec}").Output()
		o.Expect(npErr).NotTo(o.HaveOccurred())
		e2e.Logf("\n Network policy after update: %s", npRules)

		exutil.By("14. Validate traffic to pods in second namespace works but fails to pod in third namespace")
		CurlPod2PodPassUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[1], nsPodMap[nadNS[1]][0])
		CurlPod2PodFailUDN(oc, nadNS[0], nsPodMap[nadNS[0]][0], nadNS[2], nsPodMap[nadNS[2]][0])

	})

})
