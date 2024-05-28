package networking

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN network.node-identity", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("networking-node-identity", exutil.KubeConfigPath())
		notFountMsg = fmt.Sprintf("\"network-node-identity.openshift.io\" not found")
		opNamespace = "openshift-network-operator"
		cmName      = "network-node-identity"
	)

	g.BeforeEach(func() {
		// Check network node identity webhook is enabled on cluster
		webhook, err := checkNodeIdentityWebhook(oc)
		if err != nil || strings.Contains(webhook, notFountMsg) {
			g.Skip("The cluster does not have node identity webhook enabled, skipping tests")
		}
		e2e.Logf("The Node Identity webhook enabled on the cluster : %s", webhook)
		o.Expect(strings.Split(webhook, " ")).Should(o.HaveLen(2))

	})

	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:asood-High-68157-Node identity validating webhook can be disabled and enabled successfully [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodTemplate     = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			patchEnableWebhook  = fmt.Sprintf("{\"data\":{\"enabled\":\"true\"}}")
		)
		exutil.By("Get namespace")
		ns := oc.Namespace()

		exutil.By("Create config map to disable webhook")
		_, err := disableNodeIdentityWebhook(oc, opNamespace, cmName)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			patchResourceAsAdmin(oc, "configmap/"+cmName, patchEnableWebhook, opNamespace)
			waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", cmName, "-n", opNamespace).Execute()
			webhook, err := checkNodeIdentityWebhook(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Split(webhook, " ")).Should(o.HaveLen(2))

		}()

		exutil.By("NetworkOperatorStatus should back to normal after webhook is disabled")
		waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")

		exutil.By("Verify the webhook is disabled")
		webhook, _ := checkNodeIdentityWebhook(oc)
		o.Expect(strings.Contains(webhook, notFountMsg)).To(o.BeTrue())

		exutil.By("Verify pod is successfully scheduled on a node without the validating webhook")
		pod1 := pingPodResource{
			name:      "hello-pod-1",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("Enable the webhook again")
		patchResourceAsAdmin(oc, "configmap/"+cmName, patchEnableWebhook, opNamespace)

		exutil.By("NetworkOperatorStatus should back to normal after webhook is enabled")
		waitForNetworkOperatorState(oc, 100, 15, "True.*False.*False")
		webhook, err = checkNodeIdentityWebhook(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Split(webhook, " ")).Should(o.HaveLen(2))

		exutil.By("Verify pod is successfully scheduled on a node after the webhook is enabled")
		pod2 := pingPodResource{
			name:      "hello-pod-2",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

	})

	g.It("NonHyperShiftHOST-Author:asood-High-68156-ovnkube-node should be modifying annotations on its own node and pods only.[Serial]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			caseID              = "68156"
			kubeconfigFilePath  = "/tmp/kubeconfig-" + caseID
			userContext         = "default-context"
		)
		exutil.By("Get list of nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		workerNodeCount := len(nodeList.Items)
		o.Expect(workerNodeCount == 0).ShouldNot(o.BeTrue())

		exutil.By("Get namespace")
		ns := oc.Namespace()

		exutil.By(fmt.Sprintf("Get ovnkube-node pod name for a node %s", nodeList.Items[0].Name))
		ovnKubeNodePodName, err := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", nodeList.Items[0].Name)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ovnKubeNodePodName).NotTo(o.BeEmpty())

		defer func() {
			err := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("node", nodeList.Items[0].Name, "k8s.ovn.org/node-mgmt-port-").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			_, cmdErr := exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnKubeNodePodName, "ovnkube-controller", "rm -f /tmp/*.yaml")
			o.Expect(cmdErr).NotTo(o.HaveOccurred())
			_, cmdErr = exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnKubeNodePodName, "ovnkube-controller", fmt.Sprintf("rm -f %s", kubeconfigFilePath))
			o.Expect(cmdErr).NotTo(o.HaveOccurred())
		}()
		exutil.By(fmt.Sprintf("Create a kubeconfig file on the node %s", nodeList.Items[0].Name))
		o.Expect(generateKubeConfigFileForContext(oc, nodeList.Items[0].Name, ovnKubeNodePodName, kubeconfigFilePath, userContext)).To(o.BeTrue())

		exutil.By("Verify pod is successfully scheduled on a node")
		podns := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		podns.createPingPodNode(oc)
		waitPodReady(oc, podns.namespace, podns.name)

		exutil.By("Generate YAML for the pod and save it on node")
		_, podFileErr := exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnKubeNodePodName, "ovnkube-controller", fmt.Sprintf("export KUBECONFIG=%s; oc -n %s get pod %s -o json > /tmp/%s-%s.yaml", kubeconfigFilePath, podns.namespace, podns.name, podns.name, caseID))
		o.Expect(podFileErr).NotTo(o.HaveOccurred())

		for i := 0; i < 2; i++ {
			exutil.By(fmt.Sprintf("Generate YAML for the node %s and save it on node", nodeList.Items[i].Name))
			_, cmdErr := exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnKubeNodePodName, "ovnkube-controller", fmt.Sprintf("export KUBECONFIG=%s; oc get node %s -o json > /tmp/node-%s-%s.yaml", kubeconfigFilePath, nodeList.Items[i].Name, caseID, strconv.Itoa(i)))
			o.Expect(cmdErr).NotTo(o.HaveOccurred())
			//single node cluster case
			if workerNodeCount == 1 {
				break
			}
		}

		exutil.By("Verify the annotation can be added to the node where ovnkube-node is impersonated")
		patchNodePayload := `[{"op": "add", "path": "/metadata/annotations/k8s.ovn.org~1node-mgmt-port", "value":"{\"PfId\":1, \"FuncId\":1}"}]`
		patchNodeCmd := fmt.Sprintf("export KUBECONFIG=%s; kubectl patch -f /tmp/node-%s-0.yaml --type='json' --subresource=status -p='%s'", kubeconfigFilePath, caseID, patchNodePayload)
		cmdOutput, cmdErr := exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnKubeNodePodName, "ovnkube-controller", fmt.Sprintf("export KUBECONFIG=%s;  %s", kubeconfigFilePath, patchNodeCmd))
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		e2e.Logf(cmdOutput)

		if workerNodeCount > 1 {
			exutil.By("Verify the annotation cannot be added to the node where ovnkube-node is not impersonated")
			patchNodeCmd = fmt.Sprintf("export KUBECONFIG=%s; kubectl patch -f /tmp/node-%s-1.yaml --type='json' --subresource=status -p='%s'", kubeconfigFilePath, caseID, patchNodePayload)
			_, cmdErr = exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnKubeNodePodName, "ovnkube-controller", fmt.Sprintf("export KUBECONFIG=%s;  %s", kubeconfigFilePath, patchNodeCmd))
			o.Expect(cmdErr).To(o.HaveOccurred())
		}

		exutil.By("Verify ovnkube-node is not allowed to add the annotation to pod")
		patchPodDisallowedPayload := `[{"op": "add", "path": "/metadata/annotations/description", "value":"{\"hello-pod\"}"}]`
		patchPodCmd := fmt.Sprintf("export KUBECONFIG=%s; kubectl -n %s patch -f /tmp/%s-%s.yaml --type='json' --subresource=status -p='%s'", kubeconfigFilePath, podns.namespace, podns.name, caseID, patchPodDisallowedPayload)
		_, cmdErr = exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnKubeNodePodName, "ovnkube-controller", fmt.Sprintf("export KUBECONFIG=%s;  %s", kubeconfigFilePath, patchPodCmd))
		o.Expect(cmdErr).To(o.HaveOccurred())

	})

})

var _ = g.Describe("[sig-networking] SDN node", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("node", exutil.KubeConfigPath())
	)

	g.It("Longduration-NonPreRelease-Author:asood-Critical-68690-When adding nodes, the overlapped node-subnet should not be allocated. [Disruptive]", func() {

		exutil.By("1. Create a new machineset, get the new node created\n")
		clusterinfra.SkipConditionally(oc)
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-68690"
		machineSet := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 2}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer machineSet.DeleteMachineSet(oc)
		machineSet.CreateMachineSet(oc)

		machineName := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)
		o.Expect(len(machineName)).ShouldNot(o.Equal(0))
		for i := 0; i < 2; i++ {
			nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineName[i])
			e2e.Logf("Node with name %v added to cluster", nodeName)
		}

		exutil.By("2. Check host subnet is not over lapping for the nodes\n")
		nodeList, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		similarSubnetNodesFound, _ := findNodesWithSameSubnet(oc, nodeList)
		o.Expect(similarSubnetNodesFound).To(o.BeFalse())

	})

})
