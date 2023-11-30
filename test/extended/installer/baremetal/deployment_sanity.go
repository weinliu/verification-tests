package baremetal

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-baremetal] INSTALLER IPI on BareMetal", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("baremetal-deployment-sanity", exutil.KubeConfigPath())
		iaasPlatform string
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
})
