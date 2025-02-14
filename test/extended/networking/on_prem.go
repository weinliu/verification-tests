package networking

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN on-prem", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("networking-cno", exutil.KubeConfigPath())
	)

	//author: zzhao@redhat.com
	g.It("Author:zzhao-Medium-77042-Add annotation in the on-prem namespace static pods for workload partitioning", func() {
		// Skip this case for un-supported platform

		g.By("Check platforms")
		platformtype := exutil.CheckPlatform(oc)
		nsForPlatforms := map[string]string{
			"baremetal": "openshift-kni-infra",
			"vsphere":   "openshift-vsphere-infra",
			"nutanix":   "openshift-nutanix-infra",
		}
		ns := nsForPlatforms[platformtype]
		if ns == "" {
			g.Skip("Skip for non-supported platform")
		}
		appLabel := strings.Replace(ns, "openshift-", "", -1)
		lbappLable := appLabel + "-api-lb"
		dnsappLable := appLabel + "-coredns"
		kaappLabel := appLabel + "-vrrp"

		allLabels := []string{lbappLable, dnsappLable, kaappLabel}

		exutil.By("check all pods annotation")
		for _, label := range allLabels {
			podNames, error := oc.WithoutNamespace().AsAdmin().Run("get").Args("po", "-n", ns, "-l=app="+label, `-ojsonpath={.items[?(@.status.phase=="Running")].metadata.name}`).Output()
			o.Expect(error).NotTo(o.HaveOccurred())
			if podNames == "" {
				g.Skip("no related pods are running, so it's maybe use ELB, skip this testing")
			}
			podName := strings.Fields(podNames)
			// Check if workload partioning annotation is added
			podAnnotation, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("po", "-n", ns, podName[0], `-ojsonpath={.metadata.annotations}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(podAnnotation).To(o.ContainSubstring(`"target.workload.openshift.io/management":"{\"effect\": \"PreferredDuringScheduling\"}"`))
		}
	})

	//author: qiowang@redhat.com
	g.It("Author:qiowang-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-49841-Medium-50215-IPI on vSphere configures keepalived in unicast mode for API/INGRESS by default [Disruptive]", func() {
		platform := exutil.CheckPlatform(oc)
		if !strings.Contains(platform, "vsphere") {
			g.Skip("Test case should be run on vSphere, skip for other platforms!!")
		}
		apiVIPs := GetVIPOnCluster(oc, platform, "apiVIP")
		ingressVIPs := GetVIPOnCluster(oc, platform, "ingressVIP")
		ipStackType := checkIPStackType(oc)
		if len(apiVIPs) == 0 || len(ingressVIPs) == 0 {
			g.Skip("Found none AIP/INGRESS VIP on the cluster, skip the testing!!")
		}
		nodes, getNodeErr := exutil.GetAllNodesbyOSType(oc, "linux")
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		masterNodes, getMasterNodeErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getMasterNodeErr).NotTo(o.HaveOccurred())

		var (
			vipNode     string
			newVIPNode  string
			vipTypes    = []string{"apiVIP", "ingressVIP"}
			vips        = [][]string{apiVIPs, ingressVIPs}
			vipNodeSets = [][]string{masterNodes, nodes}
			cmds        = []string{"cat /etc/keepalived/monitor.conf", "cat /etc/keepalived/keepalived.conf"}
			expResults  = []string{"mode: unicast", "unicast_src_ip"}
		)
		for i, vipType := range vipTypes {
			exutil.By("1. Get the node which holds the " + vipType)
			e2e.Logf("The %s is: %s", vipType, vips[i])
			vipNode = FindVIPNode(oc, vips[i][0])
			o.Expect(vipNode).NotTo(o.Equal(""))
			vipNodeIP1, vipNodeIP2 := getNodeIP(oc, vipNode)
			e2e.Logf("%s is on node %s, the node's IP address is: %s, %s", vipType, vipNode, vipNodeIP1, vipNodeIP2)

			exutil.By("2. Check the keepalived monitor file and config file on the " + vipType + " node")
			e2e.Logf("Check on the %s node %s", vipType, vipNode)
			for j, cmd := range cmds {
				datas, err := exutil.DebugNodeWithChroot(oc, vipNode, "bash", "-c", cmd)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(strings.Contains(datas, expResults[j])).Should(o.BeTrue())
			}

			exutil.By("3. Capture vrrp advertisement packets on the " + vipType + " node")
			tcpdumpCmd := "timeout 10s tcpdump -nn -i any proto 112"
			runCmd, cmdOutput, _, err := oc.WithoutNamespace().AsAdmin().Run("debug").Args("-n", "default", "node/"+vipNode, "--", "bash", "-c", tcpdumpCmd).Background()
			defer runCmd.Process.Kill()
			o.Expect(err).NotTo(o.HaveOccurred())
			runCmd.Wait()
			for _, node := range vipNodeSets[i] {
				if node != vipNode {
					nodeIP1, nodeIP2 := getNodeIP(oc, node)
					if ipStackType == "dualstack" {
						o.Expect(strings.Contains(cmdOutput.String(), vipNodeIP1+" > "+nodeIP1+": VRRPv3, Advertisement")).Should(o.BeTrue())
						o.Expect(strings.Contains(cmdOutput.String(), vipNodeIP2+" > "+nodeIP2+": VRRPv2, Advertisement")).Should(o.BeTrue())
					} else if ipStackType == "ipv6single" {
						o.Expect(strings.Contains(cmdOutput.String(), vipNodeIP2+" > "+nodeIP2+": VRRPv3, Advertisement")).Should(o.BeTrue())
					} else {
						o.Expect(strings.Contains(cmdOutput.String(), vipNodeIP2+" > "+nodeIP2+": VRRPv2, Advertisement")).Should(o.BeTrue())
					}
				}
			}

			exutil.By("4. Reboot the " + vipType + " node, check there will be new node holds the " + vipType)
			defer checkNodeStatus(oc, vipNode, "Ready")
			rebootNode(oc, vipNode)
			checkNodeStatus(oc, vipNode, "NotReady")
			checkNodeStatus(oc, vipNode, "Ready")
			newVIPNode = FindVIPNode(oc, vips[i][0])
			o.Expect(newVIPNode).NotTo(o.Equal(""))
			e2e.Logf("%s is on node %s", vipType, newVIPNode)
		}
	})
})
