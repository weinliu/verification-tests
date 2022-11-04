package perfscale

import (
	"fmt"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// author: kkulkarni@redhat.com
var _ = g.Describe("[sig-perfscale] PerfScale oc cli perf", func() {
	defer g.GinkgoRecover()

	oc := exutil.NewCLI("perfscale-cli", exutil.KubeConfigPath())

	// author: kkulkarni@redhat.com
	g.It("Longduration-Author:liqcui-Medium-22140-Create 60 projects and time various oc commands durations[Slow][Serial]", func() {
		deploymentConfigFixture := exutil.FixturePath("testdata", "perfscale", "oc-perf.yaml")
		const projectCount = 60
		start := time.Now()
		g.By("Try to create projects and DC")
		for i := 0; i < projectCount; i++ {
			namespace := fmt.Sprintf("e2e-oc-cli-perf%d", i)
			err := oc.AsAdmin().WithoutNamespace().Run("new-project").Args(namespace).Execute()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "-f", deploymentConfigFixture).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		createDuration := time.Since(start).Seconds()

		e2e.Logf("Duration for creating %d projects and 1 deploymentConfig in each of those is %.2f seconds", projectCount, createDuration)

		start = time.Now()
		g.By("Try to get dcs, sa, and secrets")
		for i := 0; i < projectCount; i++ {
			namespace := fmt.Sprintf("e2e-oc-cli-perf%d", i)
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dc", "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sa", "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "-n", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		getDuration := time.Since(start).Seconds()
		e2e.Logf("Duration for gettings dc, sa, secrets in each of those is %.2f seconds", getDuration)

		start = time.Now()
		g.By("Try to scale the dc replicas to 0")
		for i := 0; i < projectCount; i++ {
			namespace := fmt.Sprintf("e2e-oc-cli-perf%d", i)
			err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("dc", "-n", namespace, "--replicas=0", "--all").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		scaleDuration := time.Since(start).Seconds()
		e2e.Logf("Duration for scale the dc replicas to 0 in each of those is %.2f seconds", scaleDuration)

		start = time.Now()
		g.By("Try to delete project")
		for i := 0; i < projectCount; i++ {
			namespace := fmt.Sprintf("e2e-oc-cli-perf%d", i)
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		deleteDuration := time.Since(start).Seconds()
		e2e.Logf("Duration for deleting %d projects and 1 deploymentConfig in each of those is %.2f seconds", projectCount, deleteDuration)
		// all values in BeNumerically are "Expected" and "Threshold" numbers
		// Expected derived by running this program 5 times against 4.8.0-0.nightly-2021-10-20-155651 and taking median
		// Threshold is set to 20% range of the expected value
		e2e.Logf("createDuration is: %v Expected time is less than 60s.", createDuration)
		o.Expect(createDuration).To(o.BeNumerically("<=", 60))

		e2e.Logf("getDuration is: %v Expected time is less than 60s.", getDuration)
		o.Expect(getDuration).To(o.BeNumerically("<=", 60))

		e2e.Logf("scaleDuration is: %v Expected time is less than 50s.", scaleDuration)
		o.Expect(scaleDuration).To(o.BeNumerically("<=", 50))

		e2e.Logf("deleteDuration is: %v Expected time is less than 500s.", deleteDuration)
		o.Expect(deleteDuration).To(o.BeNumerically("<=", 500))
	})
})
