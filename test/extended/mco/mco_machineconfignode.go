package mco

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO MachineConfigNode", func() {

	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("mco-machineconfignode", exutil.KubeConfigPath())
	)

	g.JustBeforeEach(func() {
		preChecks(oc)
		// featureGate MachineConfigNode in included in featureSet: TechPreviewNoUpgrade
		// skip the test if featureSet is not there
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test")
		}
	})

	g.It("Author:rioliu-NonPreRelease-Critical-69184-Enable feature gate MachineConfigNodes [Serial]", func() {
		// need to check whether featureGate MachineConfigNodes is in enabled list
		exutil.By("Check whether featureGate: MachineConfigNodes is enabled")
		featureGate := NewResource(oc.AsAdmin(), "featuregate", "cluster")
		enabled := featureGate.GetOrFail(`{.status.featureGates[*].enabled}`)
		logger.Infof("enabled featuregates: %s", enabled)
		o.Expect(enabled).Should(o.ContainSubstring("MachineConfigNodes"), "featureGate: MachineConfigNodes cannot be found")
	})

})
