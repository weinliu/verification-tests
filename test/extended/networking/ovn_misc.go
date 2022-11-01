package networking

import (
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-ovnkubernetes", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Incompatible networkType, skipping test!!!")
		}
	})

	// author: anusaxen@redhat.com
	g.It("Author:anusaxen-HyperShiftGUEST-Medium-49216-ovnkube-node logs should not print api token in logs. ", func() {
		g.By("it's for bug 2009857")
		workerNode, err := exutil.GetFirstWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ovnkubePod, err := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", workerNode)
		o.Expect(err).NotTo(o.HaveOccurred())
		podlogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(ovnkubePod, "-n", "openshift-ovn-kubernetes", "-c", "ovnkube-node").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podlogs).NotTo(o.ContainSubstring("kube-api-token"))
		g.By("ovnkube-node logs doesn't contain api-token")
	})

	//author: zzhao@redhat.com
	g.It("Author:zzhao-Medium-54742- Completed pod ip can be released. ", func() {
		g.By("it's for bug 2091157,Check the ovnkube-master logs to see if completed pod already release ip")
		result := findLogFromOvnMasterPod(oc, "Releasing IPs for Completed pod")
		o.Expect(result).To(o.BeTrue())
	})

	// author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-55144-Switching OVN gateway modes should not delete custom routes created on node logical routers.[Disruptive] ", func() {
		g.By("it's for bug 2042516")
		var desiredMode string
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("This case requires OVNKubernetes as network backend")
		}
		//need to find out original mode cluster is on so that we can revert back to same post test
		origMode := getOVNGatewayMode(oc)
		if origMode == "local" {
			desiredMode = "shared"
		} else {
			desiredMode = "local"
		}
		e2e.Logf("Cluster is currently on gateway mode %s", origMode)
		e2e.Logf("Desired mode is %s", desiredMode)
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("This case requires at least one schedulable node")
		}
		nodeLogicalRouterName := "GR_" + nodeList.Items[0].Name
		ovnMasterPodName := getOVNLeaderPod(oc, "north")
		lrRouteListDelCmd := "ovn-nbctl lr-route-del " + nodeLogicalRouterName + " 192.168.122.0/24 192.168.122.4"
		lrRouteListAddCmd := "ovn-nbctl lr-route-add " + nodeLogicalRouterName + " 192.168.122.0/24 192.168.122.4"

		lRlOutput, lrlErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListAddCmd)
		o.Expect(lrlErr).NotTo(o.HaveOccurred())
		defer exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)

		defer switchOVNGatewayMode(oc, origMode)
		switchOVNGatewayMode(oc, desiredMode)

		lrRouteListCmd := "ovn-nbctl lr-route-list " + nodeLogicalRouterName
		ovnMasterPodName = getOVNLeaderPod(oc, "north")
		defer exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)

		lRlOutput, lrlErr = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListCmd)
		o.Expect(lrlErr).NotTo(o.HaveOccurred())
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.0/24"))
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.4"))

		//reverting back cluster to original mode it was on and deleting fake route
		switchOVNGatewayMode(oc, origMode)

		ovnMasterPodName = getOVNLeaderPod(oc, "north")
		defer exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)

		lRlOutput, lrlErr = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListCmd)
		o.Expect(lrlErr).NotTo(o.HaveOccurred())
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.0/24"))
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.4"))
		lrRouteListDelCmd = "ovn-nbctl lr-route-del " + nodeLogicalRouterName + " 192.168.122.0/24 192.168.122.4"
		lRlOutput, lrlErr = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)
		o.Expect(lrlErr).NotTo(o.HaveOccurred())
	})
})
