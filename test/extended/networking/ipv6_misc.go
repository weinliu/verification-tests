package networking

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-networking] SDN misc", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-ipv6", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("This case requires OVNKubernetes as network plugin, skip the test as the cluster does not have OVN network plugin")
		}
	})

	// author: weliang@redhat.com
	g.It("NonHyperShiftHOST-Author:weliang-High-55193-Dual stack cluster fails on installation when multi-path routing entries exist. [Disruptive]", func() {
		// Customer bug https://issues.redhat.com/browse/OCPBUGS-1318
		ipStackType := checkIPStackType(oc)
		g.By("Skip testing on ipv4 or ipv6 single stack cluster")
		if ipStackType == "ipv4single" || ipStackType == "ipv6single" {
			g.Skip("The case only can be run on dualstack cluster , skip for single stack cluster!!!")
		}

		g.By("Test on dualstack cluster")
		if ipStackType == "dualstack" {
			ns := "openshift-ovn-kubernetes"
			g.By("Create multihop routes in one of ovnkubenode")
			workerNode, nodeErr := exutil.GetFirstWorkerNode(oc)
			o.Expect(nodeErr).NotTo(o.HaveOccurred())
			ovnkubePod, podErr := exutil.GetPodName(oc, ns, "app=ovnkube-node", workerNode)
			o.Expect(podErr).NotTo(o.HaveOccurred())
			_, routeErr1 := execCommandInSpecificPod(oc, ns, ovnkubePod, "ip -6 r flush default")
			o.Expect(routeErr1).NotTo(o.HaveOccurred())
			defaultRoute1, routeErr2 := execCommandInSpecificPod(oc, ns, ovnkubePod, "ip -6 r show default")
			o.Expect(routeErr2).NotTo(o.HaveOccurred())
			o.Expect(defaultRoute1).To(o.ContainSubstring(""))
			_, routeErr3 := execCommandInSpecificPod(oc, ns, ovnkubePod, "ip -6 r add default metric 48 nexthop via fe80::cee1:9402:8c35:be41 dev br-ex nexthop via fe80::cee1:9402:8c35:be42 dev br-ex")
			o.Expect(routeErr3).NotTo(o.HaveOccurred())
			defaultRoute2, routeErr4 := execCommandInSpecificPod(oc, ns, ovnkubePod, "ip -6 r show default")
			o.Expect(routeErr4).NotTo(o.HaveOccurred())
			o.Expect(defaultRoute2).To(o.ContainSubstring("nexthop via fe80::cee1:9402:8c35:be42 dev br-ex weight 1"))
			o.Expect(defaultRoute2).To(o.ContainSubstring("nexthop via fe80::cee1:9402:8c35:be41 dev br-ex weight 1"))

			g.By("Delete this ovnkubenode pod and restart a new one")
			delErr := oc.WithoutNamespace().AsAdmin().Run("delete").Args("pod", ovnkubePod, "-n", ns).Execute()
			o.Expect(delErr).NotTo(o.HaveOccurred())
			podName, podErr1 := oc.AsAdmin().Run("get").Args("-n", ns, "pod", "-l=app=ovnkube-node", "--sort-by=metadata.creationTimestamp", "-o=jsonpath={.items[-1:].metadata.name}").Output()
			o.Expect(podErr1).NotTo(o.HaveOccurred())
			waitPodReady(oc, ns, podName)

			g.By("Get correct log information about default gateway from the new ovnkubenode pod")
			expectedString := "Found default gateway interface br-ex fe80::cee1:9402:8c35:be41"
			podLogs, LogErr := checkLogMessageInPod(oc, ns, "ovnkube-node", podName, "'"+expectedString+"'"+"| tail -1")
			o.Expect(LogErr).NotTo(o.HaveOccurred())
			o.Expect(podLogs).To(o.ContainSubstring(expectedString))

		}
	})
})
