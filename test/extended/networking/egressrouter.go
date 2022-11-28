package networking

import (
	"fmt"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-egressrouter", exutil.KubeConfigPath())
	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		e2e.Logf("\n\nThe platform is %v\n", platform)
		acceptedPlatform := strings.Contains(platform, "baremetal")
		if !acceptedPlatform {
			g.Skip("Test cases should be run on BareMetal cluster, skip for other platforms!")
		}
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-42340-Egress router redirect mode with multiple destinations.", func() {
		ipStackType := checkIPStackType(oc)
		g.By("Skip testing on ipv6 single stack cluster")
		if ipStackType == "ipv6single" {
			g.Skip("Skip for single stack cluster!!!")
		}
		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking")
			egressBaseDir        = filepath.Join(buildPruningBaseDir, "egressrouter")
			pingPodTemplate      = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			egressRouterTemplate = filepath.Join(egressBaseDir, "egressrouter-multiple-destination-template.yaml")
			egressRouterService  = filepath.Join(egressBaseDir, "serive-egressrouter.yaml")
		)
		g.By("1.Get gateway for one worker node \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		gateway := getIPv4Gateway(oc, nodeList.Items[0].Name)
		o.Expect(gateway).ShouldNot(o.BeEmpty())
		freeIP := findFreeIPs(oc, nodeList.Items[0].Name, 1)
		o.Expect(len(freeIP)).Should(o.Equal(1))
		prefixIP := getInterfacePrefix(oc, nodeList.Items[0].Name)
		o.Expect(prefixIP).ShouldNot(o.BeEmpty())
		reservedIP := fmt.Sprintf("%s/%s", freeIP[0], prefixIP)

		g.By("2. Obtain the namespace \n")
		ns1 := oc.Namespace()

		g.By("3 Create egressrouter \n")
		egressrouter := egressrouterMultipleDst{
			name:           "egressrouter-42430",
			namespace:      ns1,
			reservedip:     reservedIP,
			gateway:        gateway,
			destinationip1: "142.250.188.206",
			destinationip2: "142.250.188.206",
			destinationip3: "142.250.188.206",
			template:       egressRouterTemplate,
		}
		egressrouter.createEgressRouterMultipeDst(oc)
		err = waitForPodWithLabelReady(oc, ns1, "app=egress-router-cni")
		exutil.AssertWaitPollNoErr(err, "EgressRouter pod is not ready!")

		g.By("4. Schedule the worker \n")
		// In rdu1 and rdu2 clusters, there are two sriov nodes with mlx nic, by default, egressrouter case cannot run on it
		// So here exclude sriov nodes in rdu1 and rdu2 clusters, just use the other common worker nodes
		workers := excludeSriovNodes(oc)
		o.Expect(len(workers) > 0).Should(o.BeTrue(), fmt.Sprintf("The number of common worker nodes in the cluster is %v ", len(workers)))
		if len(workers) < nodeList.Size() {
			e2e.Logf("There are sriov workers in the cluster, will schedule the egress router pod to a common node.")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ns1, "deployment/egress-router-cni-deployment", "-p", "{\"spec\":{\"template\":{\"spec\":{\"nodeName\":\""+workers[0]+"\"}}}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			output, err := oc.AsAdmin().WithoutNamespace().Run("rollout").Args("-n", ns1, "status", "deployment/egress-router-cni-deployment").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("successfully rolled out"))
		}

		g.By("5. Create serive for egress router pod! \n")
		createResourceFromFile(oc, ns1, egressRouterService)

		g.By("6. create hello pod in ns1 \n")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("7. Get service IP \n")
		svcIPv4, _ := getSvcIP(oc, ns1, "ovn-egressrouter-multidst-svc")

		g.By("8. Check result,the svc for egessrouter can be accessed \n")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+svcIPv4+":5000 --connect-timeout 5")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to access %s:5000 with error:%v", svcIPv4, err))
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+svcIPv4+":6000 --connect-timeout 5")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to access %s:6000 with error:%v", svcIPv4, err))
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+svcIPv4+":80 --connect-timeout 5")
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to access %s:80 with error:%v", svcIPv4, err))
	})
})
