package perfscale

import (
	"fmt"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// author: kkulkarni@redhat.com
var _ = g.Describe("[sig-perfscale] PerfScale oc cli perf", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("perfscale-cli", exutil.KubeConfigPath())
		ocpPerfAppDeployment string
		ocpPerfAppService    string
		ocPerfAppImageName   string
		iaasPlatform         string
		isSNO                bool
		namespace            string
	)

	g.BeforeEach(func() {
		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		e2e.Logf("Cloud provider is: %v", iaasPlatform)
		ocpPerfAppDeployment = exutil.FixturePath("testdata", "perfscale", "oc-perf-deployment.yaml")
		ocpPerfAppService = exutil.FixturePath("testdata", "perfscale", "oc-perf-service.yaml")
		isSNO = exutil.IsSNOCluster(oc)
	})

	// author: kkulkarni@redhat.com
	g.It("Longduration-Author:liqcui-Medium-22140-Create 60 projects and time various oc commands durations[Slow][Serial]", func() {

		if isSNO {
			g.Skip("Skip Testing on SNO ...")
		}

		//First choice to use [tests] image, the image mirrored by default in disconnected cluster
		//if don't have [tests] image in some environment, we can use hello-openshift as image
		//usually test imagestream shipped in all ocp and mirror the image in disconnected cluster by default
		ocPerfAppImageName = getImagestreamImageName(oc, "tests")
		if len(ocPerfAppImageName) == 0 {
			ocPerfAppImageName = "quay.io/openshifttest/hello-openshift:multiarch"
		}
		//ocPerfAppImageName = "quay.io/openshifttest/hello-openshift:multiarch"
		e2e.Logf("ocp perfscale test case ocp-22140 will use below image to test:\n[Image Name]:%s", ocPerfAppImageName)

		const projectCount = 60

		start := time.Now()
		g.By("Try to create projects and deployments")
		nsPattern := "e2e-oc-cli-perf-%d"
		for i := 0; i < projectCount; i++ {
			namespace = fmt.Sprintf(nsPattern, i)
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()
			err := oc.AsAdmin().WithoutNamespace().Run("new-project").Args(namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.ApplyNsResourceFromTemplate(oc, namespace, "--ignore-unknown-parameters=true", "-f", ocpPerfAppDeployment, "-p", "IMAGENAME="+ocPerfAppImageName)
			err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", ocpPerfAppService, "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		createDuration := time.Since(start).Seconds()

		e2e.Logf("Duration for creating %d projects and 1 deploymentConfig in each of those is %.2f seconds", projectCount, createDuration)

		start = time.Now()
		g.By("Try to get deployment, sa, and secrets")
		for i := 0; i < projectCount; i++ {
			namespace = fmt.Sprintf(nsPattern, i)
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sa", "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		getDuration := time.Since(start).Seconds()
		e2e.Logf("Duration for gettings deployment, sa, secrets in each of those is %.2f seconds", getDuration)

		start = time.Now()
		g.By("Try to scale the dc replicas to 0")
		for i := 0; i < projectCount; i++ {
			namespace = fmt.Sprintf(nsPattern, i)
			err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", "-n", namespace, "--replicas=0", "--all").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		scaleDuration := time.Since(start).Seconds()
		e2e.Logf("Duration for scale the dc replicas to 0 in each of those is %.2f seconds", scaleDuration)

		start = time.Now()
		g.By("Try to delete project")
		for i := 0; i < projectCount; i++ {
			namespace = fmt.Sprintf(nsPattern, i)
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		deleteDuration := time.Since(start).Seconds()
		e2e.Logf("Duration for deleting %d projects and 1 deploymentConfig in each of those is %.2f seconds", projectCount, deleteDuration)
		// all values in BeNumerically are "Expected" and "Threshold" numbers
		// Expected derived by running this program 5 times against 4.8.0-0.nightly-2021-10-20-155651 and taking median
		// Threshold is set to lower than the expected value
		e2e.Logf("createDuration is: %v Expected time is less than 350s.", createDuration)
		o.Expect(createDuration).To(o.BeNumerically("<=", 350))

		e2e.Logf("getDuration is: %v Expected time is less than 90s.", getDuration)
		o.Expect(getDuration).To(o.BeNumerically("<=", 90))

		e2e.Logf("scaleDuration is: %v Expected time is less than 75s.", scaleDuration)
		o.Expect(scaleDuration).To(o.BeNumerically("<=", 75))

		e2e.Logf("deleteDuration is: %v Expected time is less than 570s.", deleteDuration)
		o.Expect(deleteDuration).To(o.BeNumerically("<=", 570))
	})
})
