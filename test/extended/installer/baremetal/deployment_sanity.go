package baremetal

import (
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const maxCpuUsageAllowed float64 = 90.0

var _ = g.Describe("[sig-baremetal] INSTALLER IPI on BareMetal", func() {
	defer g.GinkgoRecover()
	var (
		oc               = exutil.NewCLI("baremetal-deployment-sanity", exutil.KubeConfigPath())
		iaasPlatform     string
		goodPodStates    = []string{"Running", "Succeeded", "Completed", "NodeAffinity"}
		warningPodStates = []string{"Pending", "Terminating"}
		issueReported    bool
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		if !(iaasPlatform == "baremetal") {
			e2e.Logf("Cluster is: %s", iaasPlatform)
			g.Skip("For Non-baremetal cluster , this is not supported!")
		}
	})
	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-29146-Verify that all clusteroperators are Available", func() {
		g.By("Running oc get clusteroperators")
		res, _ := checkOperatorsRunning(oc)
		o.Expect(res).To(o.BeTrue())
	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-29719-Verify that all nodes are up and running", func() {
		g.By("Running oc get nodes")
		res, _ := checkNodesRunning(oc)
		o.Expect(res).To(o.BeTrue())

	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-32361-Verify that deployment exists and is not empty", func() {
		g.By("Create new namespace")
		oc.SetupProject()
		ns32361 := oc.Namespace()

		g.By("Create deployment")
		deployCreationErr := oc.Run("create").Args("deployment", "deploy32361", "-n", ns32361, "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Execute()
		o.Expect(deployCreationErr).NotTo(o.HaveOccurred())

		g.By("Check deployment status is available")
		waitForDeployStatus(oc, "deploy32361", ns32361, "True")
		status, err := oc.AsAdmin().Run("get").Args("deployment", "-n", ns32361, "deploy32361", "-o=jsonpath={.status.conditions[?(@.type=='Available')].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nDeployment %s Status is %s\n", "deploy32361", status)
		o.Expect(status).To(o.Equal("True"))

		g.By("Check pod is in Running state")
		podName := getPodName(oc, ns32361)
		podStatus := getPodStatus(oc, ns32361, podName)
		o.Expect(podStatus).To(o.Equal("Running"))
	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-32195-Verify that all pods in all namespaces are in good state", func() {
		runningPods := []string{}
		warnningPods := []string{}
		failedPods := []string{}

		g.By("Running oc get pods -A")
		allNamespaces, err := oc.AsAdmin().Run("get").Args("namespaces", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nameSpaces := strings.Fields(allNamespaces)
		for _, ns := range nameSpaces {
			allPods, err := oc.AsAdmin().Run("get").Args("pods", "-n", ns, "-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			pods := strings.Fields(allPods)
			for _, pod := range pods {
				podStatus := getPodStatus(oc, ns, pod)
				if stringInSlice(podStatus, goodPodStates) {
					runningPods = append(runningPods, pod)
				} else if stringInSlice(podStatus, warningPodStates) {
					warnningPods = append(warnningPods, pod+"/"+ns)
				} else {
					failedPods = append(failedPods, pod+"/"+ns)
				}
			}
		}

		if len(runningPods) == 0 {
			e2e.Logf("\nList of running pods is empty %s\n", runningPods)
			issueReported = true
		}
		if len(failedPods) != 0 {
			e2e.Logf("\nFailed pods are: %s\n", failedPods)
			issueReported = true
		}
		if len(warnningPods) != 0 {
			e2e.Logf("\nWarning pods are: %s\n", warnningPods)
			issueReported = true
		}
		o.Expect(issueReported).NotTo(o.BeTrue())
	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-34195-Verify all pods replicas are running on workers only", func() {
		g.By("Create new namespace")
		oc.SetupProject()
		ns34195 := oc.Namespace()

		g.By("Create deployment with num of workers + 1 replicas")
		workerNodes, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		replicasNum := len(workerNodes) + 1
		deployCreationErr := oc.Run("create").Args("deployment", "deploy34195", "-n", ns34195, fmt.Sprintf("--replicas=%d", replicasNum), "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Execute()
		o.Expect(deployCreationErr).NotTo(o.HaveOccurred())
		waitForDeployStatus(oc, "deploy34195", ns34195, "True")

		g.By("Check deployed pods number is as expected")
		pods, err := oc.AsAdmin().Run("get").Args("pods", "-n", ns34195, "--field-selector=status.phase=Running", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podList := strings.Fields(pods)
		o.Expect(len(podList)).To(o.Equal(replicasNum))

		g.By("Check pods are deployed on worker nodes only")
		for _, pod := range podList {
			podNodeName, err := exutil.GetPodNodeName(oc, ns34195, pod)
			o.Expect(err).NotTo(o.HaveOccurred())
			res := exutil.IsWorkerNode(oc, podNodeName)
			if !res {
				e2e.Logf("\nPod %s was deployed on non worker node  %s\n", pod, podNodeName)
			}
			o.Expect(res).To(o.BeTrue())
		}
	})

	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-39126-Verify maximum CPU usage limit hasn't reached on each of the nodes", func() {
		g.By("Running oc get nodes")
		cpuExceededNodes := []string{}
		sampling_time, err := getClusterUptime(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeNames, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(nodeErr).NotTo(o.HaveOccurred(), "Failed to execute oc get nodes")
		nodes := strings.Fields(nodeNames)
		for _, node := range nodes {
			cpuUsage := getNodeCpuUsage(oc, node, sampling_time)
			if cpuUsage > maxCpuUsageAllowed {
				cpuExceededNodes = append(cpuExceededNodes, node)
				e2e.Logf("\ncpu usage of exceeded node: %s is %.2f%%", node, cpuUsage)
			}
		}
		o.Expect(cpuExceededNodes).Should(o.BeEmpty(), "These nodes exceed max CPU usage allowed: %s", cpuExceededNodes)
	})
})
