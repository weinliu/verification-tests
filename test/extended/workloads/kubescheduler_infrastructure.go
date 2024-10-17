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

var _ = g.Describe("[sig-scheduling] Workloads test predicates and priority work well", func() {
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

		// Check if node is a localzone node and adjust the pod yaml
		checkLocalZoneNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node[0], "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/edge") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityllocalzone.yaml")
		} else if strings.Contains(checkLocalZoneNode, "Windows") {
			g.Skip("Skipping the test as node is a windows node")
		} else if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/outposts") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityoutposts.yaml")
		}

		// Create priority pods
		priorityPodl := priorityPodDefinition{
			name:              "priorityl19895",
			label:             "pl19895",
			memory:            memForPod,
			priorityClassName: "priorityl",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		g.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

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
			phase, err := oc.AsAdmin().Run("get").Args("pods", "priorityl198951", "-n", oc.Namespace(), "--template", "{{.status.phase}}").Output()
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

		// Check if node is a localzone node and adjust the pod yaml
		checkLocalZoneNode, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node[1], "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/edge") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityllocalzone.yaml")
		} else if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/outposts") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityoutposts.yaml")
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
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podName, "--grace-period=0", "--force", "-n", oc.Namespace(), "--ignore-not-found").Execute()).NotTo(o.HaveOccurred())
			podGetStatus, podGetErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", oc.Namespace()).Output()
			o.Expect(podGetErr).To(o.HaveOccurred())
			podInfoString := fmt.Sprintf("pods \"%s\" not found", podName)
			o.Expect(podGetStatus).Should(o.ContainSubstring(podInfoString))
		}

	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-High-36108-validate pods with preemptionPolicy set to Never will not preempt any other pods which are running [Disruptive]", func() {
		isSNO := exutil.IsSNOCluster(oc)
		if isSNO {
			g.Skip("Skip Testing on SNO ...")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deploynpc := filepath.Join(buildPruningBaseDir, "non_preempting_priority36108.yaml")
		deploypcT := filepath.Join(buildPruningBaseDir, "priorityclassm.yaml")
		deploypodlT := filepath.Join(buildPruningBaseDir, "priorityl.yaml")

		// Create priorityclassl
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

		// Create nonpreempting priorityclasses
		exutil.By("Create nonpreemptingpriority class")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", deploynpc).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", deploynpc).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Cordon all worker nodes
		exutil.By("Cordon all nodes in the cluster")
		nodeNames, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeNames)

		defer func() {
			for _, v := range nodeNames {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
			for _, v := range nodeNames {
				err = checkNodeUncordoned(oc, v)
				exutil.AssertWaitPollNoErr(err, "node is not ready")
			}
		}()

		for _, v := range nodeNames {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("Uncordon node1")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeNames[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Get allocatable memory on the uncordoned worker node
		allocatableMemory, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeNames[0], "-ojsonpath={.status.allocatable.memory}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Allocatable Memory is %s", allocatableMemory)

		requestedMemCmd := fmt.Sprintf(`oc describe node %s | grep "memory" | awk '{lastLine = $0} END {print $2}'`, nodeNames[0])
		requestedMemory, err := exec.Command("bash", "-c", requestedMemCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("requestedMemory is %s", requestedMemory)

		totalMemoryInBytes := getTotalAllocatableMemory(oc, allocatableMemory, string(requestedMemory))

		if totalMemoryInBytes <= 0 {
			g.Skip("Skipping the test as totalMemoryInBytes is less than or equal to zero")
		}

		// Check if node is a localzone node and adjust the pod yaml
		checkLocalZoneNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", nodeNames[0], "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/edge") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityllocalzone.yaml")
		} else if strings.Contains(checkLocalZoneNode, "Windows") {
			g.Skip("Skipping the test as node is a windows node")
		} else if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/outposts") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityoutposts.yaml")
		}

		g.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

		// Create priority pods
		exutil.By("Create priority podl")
		priorityPodl := priorityPodDefinition{
			name:              "priorityl36108",
			label:             "pl36108",
			memory:            totalMemoryInBytes,
			priorityClassName: "priorityl",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}
		priorityPodl.createPriorityPod(oc)

		labelpl36108 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pl36108"}))
		_, podlReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labelpl36108, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podlReadyErr, "this pod with label env=pl36108 not ready")

		exutil.By("Create priority podm and verify pod is in pending state")
		priorityPodm := priorityPodDefinition{
			name:              "prioritym36108",
			label:             "pm36108",
			memory:            totalMemoryInBytes,
			priorityClassName: "prioritym",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}
		priorityPodm.createPriorityPod(oc)
		assertSpecifiedPodStatus(oc, "prioritym36108", oc.Namespace(), "Pending")

		exutil.By("Verify prioritym pod nominated node is node1")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			nominatedNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym36108", "-n", oc.Namespace(), "-o=jsonpath={.status.nominatedNodeName}").Output()
			e2e.Logf("NominatedNodeName for case OCP-36108 is %s", nominatedNodeName)
			if err != nil {
				e2e.Logf("Trying to get nominatedNode, error: %s. Trying again", err)
				return false, nil
			}
			if nominatedNodeName != nodeNames[0] {
				e2e.Failf("NominatedNode is not equal to node1, trying")
				return false, nil
			}
			return true, nil
		})
		checkPodStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-o", "wide", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Displaying all the pods inside the namespace \n%s", checkPodStatus)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No nominated node even after 180 seconds"))

		exutil.By("Verify podl is terminating and podm is running on node1")
		labelpm36108 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pm36108"}))
		_, podmReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labelpm36108, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podmReadyErr, "this pod with label env=pm36108 not ready")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym36108", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != nodeNames[0] {
			e2e.Failf("Podm is not running on node1, which is not expected")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		// Create non preempting priorityh pod
		nonPreemptingPriorityPodh := priorityPodDefinition{
			name:              "priorityh36108",
			label:             "ph36108",
			memory:            totalMemoryInBytes,
			priorityClassName: "priorityh36108",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		exutil.By("Create non preempting priority pod")
		nonPreemptingPriorityPodh.createPriorityPod(oc)
		assertSpecifiedPodStatus(oc, "priorityh36108", oc.Namespace(), "Pending")

		exutil.By("Verify pod nominated node is nil")
		nominatedNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh36108", "-n", oc.Namespace(), "-o=jsonpath={.status.nominatedNodeName}").Output()
		e2e.Logf("NominatedNodeName is %s", nominatedNodeName)
		if nominatedNodeName != "" {
			e2e.Failf("Nominated node name is not equal to nil which is not expected")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Delete prioritym pod")
		podDelStatus, podDelerr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "prioritym36108", "-n", oc.Namespace(), "--grace-period=0", "--force").Output()
		o.Expect(podDelerr).NotTo(o.HaveOccurred())
		o.Expect(podDelStatus).Should(o.ContainSubstring("pod \"prioritym36108\" force deleted"))

		exutil.By("Verify that prioritym pod no longer exists")
		podInfoStatus, podInfoerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym36108", "-n", oc.Namespace()).Output()
		o.Expect(podInfoerr).To(o.HaveOccurred())
		o.Expect(podInfoStatus).Should(o.ContainSubstring("pods \"prioritym36108\" not found"))

		labelph36108 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "ph36108"}))
		_, podhReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labelph36108, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podhReadyErr, "this pod with label env=ph36108 not ready")

		exutil.By("Delete priorityh pod")
		podDelStatus, podDelerr = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "priorityh36108", "-n", oc.Namespace(), "--grace-period=0", "--force").Output()
		o.Expect(podDelerr).NotTo(o.HaveOccurred())
		o.Expect(podDelStatus).Should(o.ContainSubstring("pod \"priorityh36108\" force deleted"))

		exutil.By("Verify that priorityh pod no longer exists")
		podInfoStatus, podInfoerr = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh36108", "-n", oc.Namespace()).Output()
		o.Expect(podInfoerr).To(o.HaveOccurred())
		o.Expect(podInfoStatus).Should(o.ContainSubstring("pods \"priorityh36108\" not found"))
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-High-36110-validate higher priority pods will preempt pods with lowerprirority when preemtionPolicy is set to Never on them [Disruptive]", func() {
		isSNO := exutil.IsSNOCluster(oc)
		if isSNO {
			g.Skip("Skip Testing on SNO ...")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deploynpc := filepath.Join(buildPruningBaseDir, "non_preempting_priority36110.yaml")
		deploypcT := filepath.Join(buildPruningBaseDir, "priorityclassm.yaml")
		deploypodlT := filepath.Join(buildPruningBaseDir, "priorityl.yaml")

		// Create priorityclassh
		priorityclassh := priorityClassDefinition{
			name:          "priorityh",
			priorityValue: 100,
			template:      deploypcT,
		}

		exutil.By("Create nonpreemptingpriority class")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", deploynpc).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", deploynpc).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create priorityh class")
		defer priorityclassh.deletePriorityClass(oc)
		priorityclassh.createPriorityClass(oc)

		// Cordon all worker nodes
		exutil.By("Cordon all nodes in the cluster")
		nodeNames, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeNames)

		defer func() {
			for _, v := range nodeNames {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
			for _, v := range nodeNames {
				err = checkNodeUncordoned(oc, v)
				exutil.AssertWaitPollNoErr(err, "node is not ready")
			}
		}()

		for _, v := range nodeNames {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Uncordon node1")
		err = oc.AsAdmin().Run("adm").Args("uncordon", nodeNames[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Get allocatable memory on the uncordoned worker node
		allocatableMemory, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeNames[0], "-ojsonpath={.status.allocatable.memory}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Allocatable Memory is %s", allocatableMemory)

		requestedMemCmd := fmt.Sprintf(`oc describe node %s | grep "memory" | awk '{lastLine = $0} END {print $2}'`, nodeNames[0])
		requestedMemory, err := exec.Command("bash", "-c", requestedMemCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("requestedMemory is %s", requestedMemory)

		totalMemoryInBytes := getTotalAllocatableMemory(oc, allocatableMemory, string(requestedMemory))

		if totalMemoryInBytes <= 0 {
			g.Skip("Skipping the test as totalMemoryInBytes is less than or equal to zero")
		}

		// Check if node is a localzone node and adjust the pod yaml
		checkLocalZoneNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", nodeNames[0], "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/edge") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityllocalzone.yaml")
		} else if strings.Contains(checkLocalZoneNode, "Windows") {
			g.Skip("Skipping the test as node is a windows node")
		} else if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/outposts") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityoutposts.yaml")
		}

		exutil.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

		// Create non preempting priority pod
		nonPreemptingPriorityPodm := priorityPodDefinition{
			name:              "prioritym36110",
			label:             "pm36110",
			memory:            totalMemoryInBytes,
			priorityClassName: "prioritym36110",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		exutil.By("Create non preempting priority pod")
		nonPreemptingPriorityPodm.createPriorityPod(oc)

		labelpm36110 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pm36110"}))
		_, podmReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labelpm36110, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podmReadyErr, "this pod with label env=pm36110 not ready")

		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym36110", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != nodeNames[0] {
			e2e.Failf("Podm is not running on node1, which is not expected")
		}

		// Create priorityh pod
		priorityPodh := priorityPodDefinition{
			name:              "priorityh36110",
			label:             "ph36110",
			memory:            totalMemoryInBytes,
			priorityClassName: "priorityh",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}

		exutil.By("Create priorityh pod")
		priorityPodh.createPriorityPod(oc)
		assertSpecifiedPodStatus(oc, "priorityh36110", oc.Namespace(), "Pending")

		g.By("Verify priorityh pod nominated node is node1")
		err = wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			nominatedNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh36110", "-n", oc.Namespace(), "-o=jsonpath={.status.nominatedNodeName}").Output()
			e2e.Logf("NominatedNodeName for case OCP-36110 is %s", nominatedNodeName)
			if err != nil {
				e2e.Logf("Trying to get nominatedNode, error: %s. Trying again", err)
				return false, nil
			}
			if nominatedNodeName != nodeNames[0] {
				return false, fmt.Errorf("NominatedNode is not equal to node1, trying")
			}
			return true, nil
		})
		checkPodStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-o", "wide", "-n", oc.Namespace()).Output()
		e2e.Logf("Displaying all the pods inside the namespace \n%s", checkPodStatus)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No nominated node even after 180 seconds"))

		exutil.By("Verify podm is terminating and podh is running on node1")
		labelph36110 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "ph36110"}))
		_, podhReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labelph36110, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podhReadyErr, "this pod with label env=ph36110 not ready")
		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh36110", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != nodeNames[0] {
			e2e.Failf("Podh is not running on node1, which is not expected")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Delete priorityh pod & verify pod no longer exists")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "priorityh36110", "--grace-period=0", "--force", "-n", oc.Namespace(), "--ignore-not-found").Execute()).NotTo(o.HaveOccurred())
		podInfoStatus, podInfoerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh36110", "-n", oc.Namespace()).Output()
		o.Expect(podInfoerr).To(o.HaveOccurred())
		o.Expect(podInfoStatus).Should(o.ContainSubstring("pods \"priorityh36110\" not found"))
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-Medium-19892-Higher priority pod should preempt the resource even when lower priority pod has nominated node name [Disruptive]", func() {
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

		priorityclassh := priorityClassDefinition{
			name:          "priorityh",
			priorityValue: 100,
			template:      deploypcT,
		}

		exutil.By("Create priorityl class")
		defer priorityclassl.deletePriorityClass(oc)
		priorityclassl.createPriorityClass(oc)

		exutil.By("Create prioritym class")
		defer priorityclassm.deletePriorityClass(oc)
		priorityclassm.createPriorityClass(oc)

		exutil.By("Create priorityh class")
		defer priorityclassh.deletePriorityClass(oc)
		priorityclassh.createPriorityClass(oc)

		// Cordon all worker nodes
		exutil.By("Cordon all nodes in the cluster")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		defer func() {
			for _, v := range node {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", v).Execute()
			}
		}()

		for _, v := range node {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", v).Execute()
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

		// Check if node is a localzone node and adjust the pod yaml
		checkLocalZoneNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node[0], "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/edge") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityllocalzone.yaml")
		} else if strings.Contains(checkLocalZoneNode, "Windows") {
			g.Skip("Skipping the test as node is a windows node")
		} else if strings.Contains(checkLocalZoneNode, "node-role.kubernetes.io/outposts") {
			deploypodlT = filepath.Join(buildPruningBaseDir, "priorityoutposts.yaml")
		}

		exutil.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

		// Create priority pods
		exutil.By("Create priority podl")
		priorityPodl := priorityPodDefinition{
			name:              "priorityl19892",
			label:             "pl19892",
			memory:            totalMemoryInBytes,
			priorityClassName: "priorityl",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}
		priorityPodl.createPriorityPod(oc)

		labell19892 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pl19892"}))
		_, podlReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labell19892, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podlReadyErr, "this pod with label env=pl19892 not ready")

		exutil.By("Create priority podm and verify pod is in pending state")
		priorityPodm := priorityPodDefinition{
			name:              "prioritym19892",
			label:             "pm19892",
			memory:            totalMemoryInBytes,
			priorityClassName: "prioritym",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}
		priorityPodm.createPriorityPod(oc)
		assertSpecifiedPodStatus(oc, "prioritym19892", oc.Namespace(), "Pending")

		exutil.By("Verify prioritym pod nominated node is node1")
		nominatedNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym19892", "-n", oc.Namespace(), "-o=jsonpath={.status.nominatedNodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NominatedNodeName is %s", nominatedNodeName)
		if nominatedNodeName != node[0] {
			e2e.Failf("Nominated node name is not equal to node1")
		}

		exutil.By("Verify podl is terminating and podm is running on node1")
		labelm19892 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "pm19892"}))
		_, podmReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labelm19892, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podmReadyErr, "this pod with label env=pm19892 not ready")

		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "prioritym19892", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != node[0] {
			e2e.Failf("Podm is not running on node1, which is not expected")
		}

		g.By("Create priority podh and verify pod is in pending state")
		priorityPodh := priorityPodDefinition{
			name:              "priorityh19892",
			label:             "ph19892",
			memory:            totalMemoryInBytes,
			priorityClassName: "priorityh",
			namespace:         oc.Namespace(),
			template:          deploypodlT,
		}
		priorityPodh.createPriorityPod(oc)
		assertSpecifiedPodStatus(oc, "priorityh19892", oc.Namespace(), "Pending")

		exutil.By("Verify priorityh pod nominated node is node1")
		nominatedNodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh19892", "-n", oc.Namespace(), "-o=jsonpath={.status.nominatedNodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NominatedNodeName is %s", nominatedNodeName)
		if nominatedNodeName != node[0] {
			e2e.Failf("Nominated node name is not equal to node1")
		}

		exutil.By("Verify podm is terminating and podh is running on node1")
		labelh19892 := labels.SelectorFromSet(labels.Set(map[string]string{"env": "ph19892"}))
		_, podhReadyErr := e2epod.WaitForPodsWithLabelRunningReady(context.Background(), oc.KubeClient(), oc.Namespace(), labelh19892, 1, poddefaultTimeout)
		exutil.AssertWaitPollNoErr(podhReadyErr, "this pod with label env=ph19892 not ready")

		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh19892", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NominatedNodeName is %s", nominatedNodeName)
		if nodeName != node[0] {
			e2e.Failf("Podh is not running on node1, which is not expected")
		}

		exutil.By("Delete priorityh pod")
		podDelStatus, podDelerr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "priorityh19892", "-n", oc.Namespace(), "--grace-period=0", "--force").Output()
		o.Expect(podDelerr).NotTo(o.HaveOccurred())
		o.Expect(podDelStatus).Should(o.ContainSubstring("pod \"priorityh19892\" force deleted"))

		exutil.By("Verify that priorityh pod no longer exists")
		podInfoStatus, podInfoerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "priorityh19892", "-n", oc.Namespace()).Output()
		o.Expect(podInfoerr).To(o.HaveOccurred())
		o.Expect(podInfoStatus).Should(o.ContainSubstring("pods \"priorityh19892\" not found"))
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-High-14479-pod will be scheduled to the node which matches node affinity", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deployNodeAffinity := filepath.Join(buildPruningBaseDir, "node-affinity-required-case14479.yaml")

		// Retreive all worker nodes
		exutil.By("Get all nodes in the cluster")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		// Add label to the node
		g.By("Add label to the node")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node[0], "key14479-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node[0], "key14479=value14479").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check if node is a outposts node and adjust the pod yaml
		checkOutpostsNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node[0], "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(checkOutpostsNode, "node-role.kubernetes.io/outposts") {
			deployNodeAffinity = filepath.Join(buildPruningBaseDir, "node-affinity-required-case14479-outposts.yaml")
		} else if strings.Contains(checkOutpostsNode, "node-role.kubernetes.io/edge") {
			deployNodeAffinity = filepath.Join(buildPruningBaseDir, "node-affinity-required-case14479-edge.yaml")
		}

		exutil.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

		// Create node-affinity-required-case14479 pods
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", deployNodeAffinity, "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", deployNodeAffinity, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify pod is running and it is scheduled on node0")

		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "node-affinity-required-case14479", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			e2e.Logf("output is %s", output)
			if strings.Contains("Running", output) {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod is still not running even after 3 minutes"))

		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "node-affinity-required-case14479", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != node[0] {
			e2e.Failf("Pod is not running on node0, which is not expected")
		}
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-High-14488-pod will still run on the node if labels on the node change and affinity rules no longer met IgnoredDuringExecution", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		nodeAffinityRequiredCase := filepath.Join(buildPruningBaseDir, "node-affinity-required-case14488.yaml")

		// Retreive all worker nodes
		exutil.By("Get all nodes in the cluster")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		// Add label to the node
		g.By("Add label to the node")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node[0], "key14488-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node[0], "key14488=value14488").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check if node is a outposts node and adjust the pod yaml
		checkOutpostsNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node[0], "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(checkOutpostsNode, "node-role.kubernetes.io/outposts") {
			nodeAffinityRequiredCase = filepath.Join(buildPruningBaseDir, "node-affinity-required-case14488-outposts.yaml")
		} else if strings.Contains(checkOutpostsNode, "node-role.kubernetes.io/edge") {
			nodeAffinityRequiredCase = filepath.Join(buildPruningBaseDir, "node-affinity-required-case14488-edge.yaml")
		}

		exutil.By("Set namespace privileged")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())

		// Add node selector to project
		defer func() {
			patchYamlToRestore := `[{"op": "remove", "path": "/metadata/annotations/openshift.io~1node-selector", "value":""}]`
			e2e.Logf("Removing annotation from the user created project")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("namespace", oc.Namespace(), "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		patchYamlTraceAll := `[{"op": "add", "path": "/metadata/annotations/openshift.io~1node-selector", "value":""}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("namespace", oc.Namespace(), "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Create node-affinity-required-case14488 pods
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", nodeAffinityRequiredCase, "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", nodeAffinityRequiredCase, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify pod is running and it is scheduled on node0")

		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "node-affinity-required-case14488", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}").Output()
			if err != nil {
				e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
				return false, nil
			}
			e2e.Logf("output is %s", output)
			if strings.Contains("Running", output) {
				e2e.Logf("deploy is up:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod is still not running even after 3 minutes"))

		nodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "node-affinity-required-case14488", "-n", oc.Namespace(), "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NodeName is %s", nodeName)
		if nodeName != node[0] {
			e2e.Failf("Pod is not running on node0, which is not expected")
		}
	})

})
