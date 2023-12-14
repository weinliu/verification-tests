package mco

import (
	"fmt"

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

	g.It("Author:rioliu-NonPreRelease-High-69187-validate MachineConfigNodes properties [Serial]", func() {

		nodes := NewNodeList(oc.AsAdmin()).GetAllLinuxWorkerNodesOrFail()

		for _, node := range nodes {
			mcn := NewMachineConfigNode(oc.AsAdmin(), node.GetName())

			exutil.By(fmt.Sprintf("Check MachineConfigNode properties for %s", node.GetName()))

			logger.Infof("Check spec.configVersion.desired")
			desiredOfNode := node.GetDesiredMachineConfig()
			desiredOfMCNSpec := mcn.GetDesiredMachineConfigOfSpec()
			o.Expect(desiredOfNode).Should(o.Equal(desiredOfMCNSpec), "desired config of node is not same as machineconfignode.spec")

			logger.Infof("Check spec.pool")
			poolOfNode := node.GetPrimaryPoolOrFail().GetName()
			poolOfMCNSpec := mcn.GetPool()
			o.Expect(poolOfNode).Should(o.Equal(poolOfMCNSpec), "pool of node is not same as machineconfignode.spec")

			logger.Infof("Check spec.node")
			nodeOfMCNSpec := mcn.GetNode()
			o.Expect(node.GetName()).Should(o.Equal(nodeOfMCNSpec), "node name is not same as machineconfignode.spec")

			logger.Infof("Check status.configVersion.current")
			currentOfNode := node.GetCurrentMachineConfig()
			currentOfMCNStatus := mcn.GetCurrentMachineConfigOfStatus()
			o.Expect(currentOfNode).Should(o.Equal(currentOfMCNStatus), "current config of node is not same as machineconfignode.status")

			logger.Infof("Check status.configVersion.desired")
			desiredOfMCNStatus := mcn.GetDesiredMachineConfigOfStatus()
			o.Expect(desiredOfNode).Should(o.Equal(desiredOfMCNStatus), "desired config of node is not same as machineconfignode.status")

			logger.Infof("OK\n")
		}

	})

	g.It("Author:rioliu-NonPreRelease-Longduration-High-69197-validate MachineConfigNode condition status transition [Disruptive]", func() {

		var (
			mcName     = "create-test-file"
			fileConfig = getURLEncodedFileConfig("/etc/test-file", "hello", "420")
			workerMcp  = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		)

		// create machine config to apply a file change
		exutil.By("Create a test file on node")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.SetMCOTemplate(GenericMCTemplate)
		mc.SetParams(fmt.Sprintf("FILES=[%s]", fileConfig))
		mc.skipWaitForMcp = true

		defer mc.delete()
		mc.create()

		exutil.By("Check conidition status of MachineConfigNode")
		// get 1st updating worker nodes
		workerNode := workerMcp.GetCordonedNodes()[0]
		// if test fail, need to waiting for mcp to complete, then rollback the change
		defer workerMcp.waitForComplete()

		mcn := NewMachineConfigNode(oc.AsAdmin(), workerNode.GetName())
		o.Eventually(mcn.GetUpdated, "1m", "5s").Should(o.Equal("False"))
		o.Eventually(mcn.GetUpdatePrepared, "1m", "3s").Should(o.Equal("True"))
		o.Eventually(mcn.GetUpdateCompatible, "3m", "3s").Should(o.Equal("True"))
		o.Eventually(mcn.GetUpdateExecuted, "1m", "3s").Should(o.Equal("Unknown"))
		o.Eventually(mcn.GetCordoned, "30s", "3s").Should(o.Equal("True"))
		o.Eventually(mcn.GetDrained, "30s", "2s").Should(o.Equal("Unknown"))
		o.Eventually(mcn.GetDrained, "1m", "2s").Should(o.Equal("True"))
		o.Eventually(mcn.GetAppliedFilesAndOS, "1m", "1s").Should(o.Equal("Unknown"))
		o.Eventually(mcn.GetAppliedFilesAndOS, "3m", "2s").Should(o.Equal("True"))
		o.Eventually(mcn.GetUpdateExecuted, "20s", "5s").Should(o.Equal("True"))
		o.Eventually(mcn.GetUpdatePostActionComplete, "30m", "5s").Should(o.Equal("Unknown"))
		o.Eventually(mcn.GetRebootedNode, "15s", "3s").Should(o.Equal("Unknown"))
		o.Eventually(mcn.GetRebootedNode, "5m", "5s").Should(o.Equal("True"))
		o.Eventually(mcn.GetResumed, "15s", "5s").Should(o.Equal("True"))
		o.Eventually(mcn.GetUpdateComplete, "10s", "5s").Should(o.Equal("True"))
		o.Eventually(mcn.GetUncordoned, "10s", "2s").Should(o.Equal("True"))
		o.Eventually(mcn.GetUpdated, "1m", "5s").Should(o.Equal("True"))

	})

})
