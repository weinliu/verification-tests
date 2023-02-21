package networking

import (
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

	var oc = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())

	g.BeforeEach(func() {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		if err != nil || !strings.Contains(msg, "bm2-zzhao") {
			g.Skip("These cases only can be run on Beijing local baremetal server , skip for other envrionment!!!")
		}
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Author:huirwang-Medium-43466-EgressIP works well with ipv6 address. [Serial]", func() {
		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			g.Skip("Current env is ipv4 single stack cluster, skip this test!!!")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		g.By("create new namespace")
		oc.SetupProject()

		g.By("Label EgressIP node")
		var EgressNodeLabel = "k8s.ovn.org/egress-assignable"
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		if err != nil {
			e2e.Logf("Unexpected error occurred: %v", err)
		}
		g.By("Apply EgressLabel Key for this test on one node.")
		e2e.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel, "true")
		defer e2e.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, EgressNodeLabel)

		g.By("Apply label to namespace")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name=test").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name-").Output()

		g.By("Create an egressip object")
		sub1, _ := getDefaultIPv6Subnet(oc)
		ips, _ := findUnUsedIPv6(oc, sub1, 2)
		egressip1 := egressIPResource1{
			name:      "egressip-43466",
			template:  egressIPTemplate,
			egressIP1: ips[0],
			egressIP2: ips[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		g.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: oc.Namespace(),
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)
		defer pod1.deletePingPod(oc)

		g.By("Check source IP is EgressIP")
		msg, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s -6 "+ipv6EchoServer(true)+" --connect-timeout 5")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, ips[0]) || strings.Contains(msg, ips[1])).To(o.BeTrue())

		g.By("EgressIP works well with IPv6. ")

	})
})
