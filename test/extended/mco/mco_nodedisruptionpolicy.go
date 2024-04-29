package mco

import (
	"fmt"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-mco] MCO NodeDisruptionPolicy", func() {

	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("mco-nodedisruptionpolicy", exutil.KubeConfigPath())
	)

	g.JustBeforeEach(func() {
		preChecks(oc)
		// skip the test if featureSet is not there
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test")
		}
		// check featureGate NodeDisruptionPolicy is enabled
		enabledFeatureGates := NewResource(oc.AsAdmin(), "featuregate", "cluster").GetOrFail(`{.status.featureGates[*].enabled}`)
		o.Expect(enabledFeatureGates).Should(o.ContainSubstring("NodeDisruptionPolicy"), "featureGate: NodeDisruptionPolicy is not in enabled list")
	})

	g.It("Author:rioliu-NonPreRelease-High-73368-NodeDisruptionPolicy files with action None [Disruptive]", func() {

		var (
			mcName     = "create-test-file-73368"
			filePath   = "/etc/test-action-none-73368"
			fileConfig = getURLEncodedFileConfig(filePath, "test-73368", "420")
		)

		exutil.By("Patch ManchineConfiguration cluster")
		ndp := NewNodeDisruptionPolicy(oc)
		defer ndp.Rollback()
		o.Expect(ndp.Patch("merge", fmt.Sprintf(`{"spec":{"nodeDisruptionPolicy":{"files":[{"actions":[{"type": "None"}],"path":"%s"}]}}}`, filePath))).To(o.Succeed(), "Patch ManchineConfiguration failed")

		exutil.By("Check the nodeDisruptionPolicyStatus, new change should be merged")
		policies := ndp.GetPolicies("files")
		foundMergedPolicy := false
		for _, policy := range policies {
			if policy.GetPath() == filePath && policy.GetActions()[0].GetType() == "None" {
				foundMergedPolicy = true
				break
			}
		}
		o.Expect(foundMergedPolicy).To(o.BeTrue(), "Cannot find merge policy")

		exutil.By("Create a test file on worker node")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.SetParams(fmt.Sprintf("FILES=[%s]", fileConfig))
		defer mc.delete()
		mc.create()

		exutil.By("Check MCD log no post config action is taken")
		worker := NewNodeList(oc.AsAdmin()).GetAllCoreOsWokerNodesOrFail()[0]
		o.Expect(worker.GetMCDaemonLogs("Executing")).Should(
			o.And(
				o.ContainSubstring(`performPostConfigChangeNodeDisruptionAction(drain already complete/skipped)`),
				o.ContainSubstring("postconfig action: None")),
			"Cannot find expected log for post config action None")

	})

})
