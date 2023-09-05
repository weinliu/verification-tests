package networking

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
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
	g.It("Author:anusaxen-Medium-49216-ovnkube-node logs should not print api token in logs. ", func() {
		g.By("it's for bug 2009857")
		workerNode, err := exutil.GetFirstWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ovnkubePod, err := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", workerNode)
		o.Expect(err).NotTo(o.HaveOccurred())
		podlogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(ovnkubePod, "-n", "openshift-ovn-kubernetes", "-c", "ovnkube-controller").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podlogs).NotTo(o.ContainSubstring("kube-api-token"))
		g.By("ovnkube-node logs doesn't contain api-token")
	})

	//author: zzhao@redhat.com
	g.It("NonHyperShiftHOST-Author:zzhao-Medium-54742- Completed pod ip can be released. ", func() {
		g.By("it's for bug 2091157,Check the ovnkube-master logs to see if completed pod already release ip")
		result := findLogFromPod(oc, "Releasing IPs for Completed pod", "openshift-ovn-kubernetes", "app=ovnkube-node", "ovnkube-controller")
		o.Expect(result).To(o.BeTrue())
	})

	// author: anusaxen@redhat.com
	g.It("NonHyperShiftHOST-Author:anusaxen-High-55144-Switching OVN gateway modes should not delete custom routes created on node logical routers.[Disruptive] ", func() {
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
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 1 {
			g.Skip("This case requires at least one schedulable node")
		}
		nodeLogicalRouterName := "GR_" + nodeList.Items[0].Name
		ovnMasterPodName := getOVNKMasterOVNkubeNode(oc)
		lrRouteListDelCmd := "ovn-nbctl lr-route-del " + nodeLogicalRouterName + " 192.168.122.0/24 192.168.122.4"
		lrRouteListAddCmd := "ovn-nbctl lr-route-add " + nodeLogicalRouterName + " 192.168.122.0/24 192.168.122.4"

		_, lrlErr1 := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListAddCmd)
		o.Expect(lrlErr1).NotTo(o.HaveOccurred())
		defer exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)

		defer switchOVNGatewayMode(oc, origMode)
		switchOVNGatewayMode(oc, desiredMode)

		lrRouteListCmd := "ovn-nbctl lr-route-list " + nodeLogicalRouterName
		ovnMasterPodName = getOVNKMasterOVNkubeNode(oc)
		defer exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)

		lRlOutput, lrlErr2 := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListCmd)
		o.Expect(lrlErr2).NotTo(o.HaveOccurred())
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.0/24"))
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.4"))

		//reverting back cluster to original mode it was on and deleting fake route
		switchOVNGatewayMode(oc, origMode)

		ovnMasterPodName = getOVNKMasterOVNkubeNode(oc)
		defer exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)

		_, lrlErr3 := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListCmd)
		o.Expect(lrlErr3).NotTo(o.HaveOccurred())
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.0/24"))
		o.Expect(lRlOutput).To(o.ContainSubstring("192.168.122.4"))
		lrRouteListDelCmd = "ovn-nbctl lr-route-del " + nodeLogicalRouterName + " 192.168.122.0/24 192.168.122.4"
		_, lrlErr4 := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, lrRouteListDelCmd)
		o.Expect(lrlErr4).NotTo(o.HaveOccurred())
	})

	// author: jechen@redhat.com
	g.It("Author:jechen-Medium-61312-Unsupported scenarios in expanding cluster networks should be denied. [Disruptive]", func() {

		ipStackType := checkIPStackType(oc)
		if ipStackType != "ipv4single" {
			g.Skip("The feature is currently supported on IPv4 cluster only, skip for other IP stack type for now")
		}

		origNetworkCIDR, orighostPrefix := getClusterNetworkInfo(oc)
		origNetAddress := strings.Split(origNetworkCIDR, "/")[0]
		origNetMaskVal, _ := strconv.Atoi(strings.Split(origNetworkCIDR, "/")[1])
		origHostPrefixVal, _ := strconv.Atoi(orighostPrefix)
		e2e.Logf("Original netAddress:%v, netMask:%v, hostPrefix: %v", origNetAddress, origNetMaskVal, origHostPrefixVal)

		g.By("1. Verify that decreasing IP space by larger CIDR mask is not allowed")
		newCIDR := origNetAddress + "/" + strconv.Itoa(origNetMaskVal+1)
		e2e.Logf("Attempt to change to newCIDR: %v", newCIDR)

		// patch command will be executed even though invalid config is supplied, so still call patchResourceAsAdmin function
		restorePatchValue := "{\"spec\":{\"clusterNetwork\":[{\"cidr\":\"" + origNetworkCIDR + "\", \"hostPrefix\":" + orighostPrefix + "}],\"networkType\":\"OVNKubernetes\"}}"
		defer patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", restorePatchValue)
		patchValue := "{\"spec\":{\"clusterNetwork\":[{\"cidr\":\"" + newCIDR + "\", \"hostPrefix\":" + orighostPrefix + "}],\"networkType\":\"OVNKubernetes\"}}"
		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", patchValue)

		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).Should(o.ContainSubstring(`invalid configuration: [reducing IP range with a larger CIDR mask for clusterNetwork CIDR is unsupported]`))

		// restore to original valid config before next step
		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", restorePatchValue)
		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).ShouldNot(o.ContainSubstring(`invalid configuration: [reducing IP range with a larger CIDR mask for clusterNetwork CIDR is unsupported]`))

		g.By("2. Verify that changing hostPrefix is not allowed")
		newHostPrefix := strconv.Itoa(origHostPrefixVal + 1)
		e2e.Logf("Attempt to change to newHostPrefix: %v", newHostPrefix)

		// patch command will be executed even though invalid config is supplied, so still call patchResourceAsAdmin function
		patchValue = "{\"spec\":{\"clusterNetwork\":[{\"cidr\":\"" + origNetworkCIDR + "\", \"hostPrefix\":" + newHostPrefix + "}],\"networkType\":\"OVNKubernetes\"}}"
		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", patchValue)
		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).Should(o.ContainSubstring(`invalid configuration: [modifying a clusterNetwork's hostPrefix value is unsupported]`))

		// restore to original valid config before next step
		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", restorePatchValue)
		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).ShouldNot(o.ContainSubstring(`invalid configuration: [modifying a clusterNetwork's hostPrefix value is unsupported]`))

		newHostPrefix = strconv.Itoa(origHostPrefixVal - 1)
		e2e.Logf("Attempt to change to newHostPrefix: %v", newHostPrefix)

		// patch command will be executed even though invalid config is supplied, so still call patchResourceAsAdmin function
		patchValue = "{\"spec\":{\"clusterNetwork\":[{\"cidr\":\"" + origNetworkCIDR + "\", \"hostPrefix\":" + newHostPrefix + "}],\"networkType\":\"OVNKubernetes\"}}"
		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", patchValue)
		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).Should(o.ContainSubstring(`invalid configuration: [modifying a clusterNetwork's hostPrefix value is unsupported]`))

		// restore to original valid config before next step
		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", restorePatchValue)
		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).ShouldNot(o.ContainSubstring(`invalid configuration: [modifying a clusterNetwork's hostPrefix value is unsupported]`))

		g.By("3. Verify that changing network IP is not allowed")
		subAddress := strings.Split(origNetAddress, ".")
		subAddressB, _ := strconv.Atoi(subAddress[1])
		newSubAddressB := strconv.Itoa(subAddressB + 1)
		newNetAddress := subAddress[0] + "." + newSubAddressB + "." + subAddress[2] + "." + subAddress[3]
		newCIDR = newNetAddress + "/" + strconv.Itoa(origNetMaskVal)
		e2e.Logf("Attempt to change to newCIDR: %v", newCIDR)

		// patch command will be executed even though invalid config is supplied, so still call patchResourceAsAdmin function
		patchValue = "{\"spec\":{\"clusterNetwork\":[{\"cidr\":\"" + newCIDR + "\", \"hostPrefix\":" + orighostPrefix + "}],\"networkType\":\"OVNKubernetes\"}}"
		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", patchValue)
		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).Should(o.ContainSubstring(`invalid configuration: [modifying IP network value for clusterNetwork CIDR is unsupported]`))

		patchResourceAsAdmin(oc, "Network.config.openshift.io/cluster", restorePatchValue)
		o.Eventually(func() string {
			return getCNOStatusCondition(oc)
		}, 30*time.Second, 3*time.Second).ShouldNot(o.ContainSubstring(`invalid configuration: [modifying IP network value for clusterNetwork CIDR is unsupported]`))
	})

	//author: zzhao@redhat.com
	//bug: https://issues.redhat.com/browse/OCPBUGS-2827
	g.It("NonHyperShiftHOST-ConnectedOnly-ROSA-OSD_CCS-Author:zzhao-Medium-64297- check nodeport service with large mtu.[Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			hostPortServiceFile = filepath.Join(buildPruningBaseDir, "ocpbug-2827/hostport.yaml")
			mtuTestFile         = filepath.Join(buildPruningBaseDir, "ocpbug-2827/mtutest.yaml")
			ns1                 = "openshift-kube-apiserver"
		)
		platform := exutil.CheckPlatform(oc)
		acceptedPlatform := strings.Contains(platform, "aws")
		if !acceptedPlatform {
			g.Skip("Test cases should be run on AWS cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}

		g.By("create nodeport service in namespace")
		defer removeResource(oc, true, true, "-f", hostPortServiceFile, "-n", ns1)
		createResourceFromFile(oc, ns1, hostPortServiceFile)

		g.By("create mtutest pod")
		defer removeResource(oc, true, true, "-f", mtuTestFile, "-n", ns1)
		createResourceFromFile(oc, ns1, mtuTestFile)
		err := waitForPodWithLabelReady(oc, ns1, "app=mtu-tester")
		exutil.AssertWaitPollNoErr(err, "this pod with label app=mtu-tester not ready")
		mtuTestPod := getPodName(oc, ns1, "app=mtu-tester")

		g.By("get one nodeip")
		PodNodeName, nodeErr := exutil.GetPodNodeName(oc, ns1, mtuTestPod[0])
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		nodeIp := getNodeIPv4(oc, ns1, PodNodeName)

		output, err := e2eoutput.RunHostCmd(ns1, mtuTestPod[1], "curl --connect-timeout 5 -s "+net.JoinHostPort(nodeIp, "31251")+"?mtu=8849 2>/dev/null | cut -b-10")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "Terminated")).To(o.BeFalse())
		output, err = e2eoutput.RunHostCmd(ns1, mtuTestPod[1], "curl --connect-timeout 5 -s "+net.JoinHostPort(nodeIp, "31251")+"?mtu=8850 2>/dev/null | cut -b-10")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "Terminated")).To(o.BeFalse())
	})

	// author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-64151-check node healthz port is enabled for ovnk in CNO for GCP", func() {
		e2e.Logf("It is for OCPBUGS-7158")
		platform := checkPlatform(oc)
		if !strings.Contains(platform, "gcp") {
			g.Skip("Skip for un-expected platform,not GCP!")
		}
		g.By("Expect healtz-bind-address to be present in ovnkube-config config map")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-ovn-kubernetes", "ovnkube-config", "-ojson").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "0.0.0.0:10256")).To(o.BeTrue())

		g.By("Make sure healtz-bind-address is reachable via nodes")
		worker_node, err := exutil.GetFirstLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = exutil.DebugNode(oc, worker_node, "bash", "-c", "curl -v http://0.0.0.0:10256/healthz")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("HTTP/1.1 200 OK"))
	})
})
