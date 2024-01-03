package workloads

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-scheduling] Workloads", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	const poddefaultTimeout = 3 * time.Minute

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-Low-19895-Preemptor will choose the node with lowest number pods which violated PDBs [Disruptive]", func() {
		isSNO := exutil.IsSNOCluster(oc)
		if isSNO {
			g.Skip("Skip Testing on SNO ...")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deploypcT := filepath.Join(buildPruningBaseDir, "priorityclassm.yaml")
		deploypodlT := filepath.Join(buildPruningBaseDir, "priorityl.yaml")

		// Create priorityclasses
		priorityclassl := priorityClassDefinition{
			name:          "priorityl",
			priorityValue: 1,
			template:      deploypcT,
		}

		priorityclassm := priorityClassDefinition{
			name:          "prioritym",
			priorityValue: 99,
			template:      deploypcT,
		}

		exutil.By("Create priorityl class")
		defer priorityclassl.deletePriorityClass(oc)
		priorityclassl.createPriorityClass(oc)

		exutil.By("Create prioritym class")
		defer priorityclassm.deletePriorityClass(oc)
		priorityclassm.createPriorityClass(oc)

		// Cordon all worker nodes
		exutil.By("Cordon all nodes in the cluster")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		defer func() {
			for _, v := range node {
				err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
			for _, v := range node {
				err = checkNodeUncordoned(oc, v)
				exutil.AssertWaitPollNoErr(err, "node is not ready")
			}
		}()

		for _, v := range node {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("Uncordon node1")
		err = oc.AsAdmin().Run("adm").Args("uncordon", node[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Get allocatable memory on the uncordoned worker node
		allocatableMemory, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node[0], "-ojsonpath={.status.allocatable.memory}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Allocatable Memory is %s", allocatableMemory)

		requestedMemCmd := fmt.Sprintf(`oc describe node %s | grep "memory" | awk '{lastLine = $0} END {print $2}'`, node[0])
		requestedMemory, err := exec.Command("bash", "-c", requestedMemCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("requestedMemory is %s", requestedMemory)

		totalMemoryInBytes := getTotalAllocatableMemory(oc, allocatableMemory, string(requestedMemory))

		if totalMemoryInBytes <= 0 {
			g.Skip("Skipping the test as totalMemoryInBytes is less than or equal to zero")
		}

		// Caluclate the memory with which the pod needs to be created
		memForPod := totalMemoryInBytes / 2

		// Create priority pods
		priorityPodl := priorityPodDefinition{
			name:              "priorityl19895",
			label:             "pl19895",
			memory:            memForPod,
			priorityClassName: "priorityl",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		exutil.By("Create priorityl pod")
		priorityPodl.createPriorityPod(oc)

		labell19895 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pl19895"}))
		_, podlReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labell19895, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podlReadyErr, "this pod with label env=pl19895 not ready")

		exutil.By("Create another priority podl")
		priorityPodl = priorityPodDefinition{
			name:              "priorityl198951",
			label:             "pl19895",
			memory:            memForPod,
			priorityClassName: "priorityl",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		priorityPodl.createPriorityPod(oc)

		e2e.Logf("Waiting for pod running")
		err = wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
			phase, err := oc.AsAdmin().Run("get").Args("pods", "priorityl198951", "--template", "{{.status.phase}}").Output()
			if err != nil {
				return false, nil
			}
			if phase != "Running" {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Second pod with priorityl is not running")

		// Create PodDisruptionBudget
		exutil.By("Create pdb")
		defer oc.AsAdmin().Run("delete").Args("pdb", "pdbocp19895").Execute()
		createPdbErr := oc.AsAdmin().Run("create").Args("poddisruptionbudget", "pdbocp19895", "--min-available=100", "--selector=env=pl19895").Execute()
		o.Expect(createPdbErr).NotTo(o.HaveOccurred())

		// Uncordon node2
		exutil.By("Uncordon node2")
		err = oc.AsAdmin().Run("adm").Args("uncordon", node[1]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Get allocatable memory on the uncordoned worker node
		allocatableMemory, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node[1], "-ojsonpath={.status.allocatable.memory}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Allocatable Memory is %s", allocatableMemory)

		requestedMemCmd = fmt.Sprintf(`oc describe node %s | grep "memory" | awk '{lastLine = $0} END {print $2}'`, node[1])
		requestedMemory, err = exec.Command("bash", "-c", requestedMemCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("requestedMemory is %s", requestedMemory)

		totalMemoryInBytes = getTotalAllocatableMemory(oc, allocatableMemory, string(requestedMemory))

		if totalMemoryInBytes <= 0 {
			g.Skip("Skipping the test as totalMemoryInBytes is less than or equal to zero")
		}

		exutil.By("Create another priority podl")
		priorityPodl = priorityPodDefinition{
			name:              "priorityl198952",
			label:             "pl198952",
			memory:            totalMemoryInBytes,
			priorityClassName: "priorityl",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		priorityPodl.createPriorityPod(oc)

		label198952 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pl198952"}))
		_, podlReadyErr = e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), label198952, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podlReadyErr, "this pod with label env=pl198952 not ready")

		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityl198952", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != node[1] {
			e2e.Failf("Podl is not running on node2, which is not expected")
		}

		// Create PodDisruptionBudget
		exutil.By("Create pdb")
		defer oc.AsAdmin().Run("delete").Args("pdb", "pdbocp198951").Execute()
		createPdbErr = oc.AsAdmin().Run("create").Args("poddisruptionbudget", "pdbocp198951", "--min-available=100", "--selector=env=pl198952").Execute()
		o.Expect(createPdbErr).NotTo(o.HaveOccurred())

		exutil.By("Create priority podm and verify pod is in pending state")
		priorityPodm := priorityPodDefinition{
			name:              "prioritym19895",
			label:             "pm19895",
			memory:            totalMemoryInBytes,
			priorityClassName: "prioritym",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		priorityPodm.createPriorityPod(oc)
		assertSpecifiedPodStatus(oc, "prioritym19895", oc.Namespace(), "Pending")

		exutil.By("Verify prioritym pod nominated node is node2")
		nominatedNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym19895", "-n", oc.Namespace(), "-o=jsonpath={.status.nominatedNodeName}").Output()
		e2e.Logf("NominatedNodeName is %s", nominatedNodeName)
		if nominatedNodeName != node[1] {
			e2e.Failf("Nominated node name is not equal to node2")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify podl is terminating and podm is running on node2")
		label19895 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pm19895"}))
		_, podmReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), label19895, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podmReadyErr, "this pod with label env=pm19895 not ready")
		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym19895", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != node[1] {
			e2e.Failf("Podm is not running on node2, which is not expected")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Delete all the remaining pods & verify pod no longer exists")
		podNames := []string{"priorityl19895", "priorityl198951", "prioritym19895"}
		for _, podName := range podNames {
			podDelStatus, podDelErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podName, "--grace-period=0", "--force", "-n", oc.Namespace()).Output()
			o.Expect(podDelErr).NotTo(o.HaveOccurred())
			podDeletionString := fmt.Sprintf("pod \"%s\" force deleted", podName)
			o.Expect(podDelStatus).Should(o.ContainSubstring(podDeletionString))
			podGetStatus, podGetErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", oc.Namespace()).Output()
			o.Expect(podGetErr).To(o.HaveOccurred())
			podInfoString := fmt.Sprintf("pods \"%s\" not found", podName)
			o.Expect(podGetStatus).Should(o.ContainSubstring(podInfoString))
		}

	})
})
