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
		testFileBasedPolicy(oc, "73368", []Action{NewCommonAction(NodeDisruptionPolicyActionNone)}, []string{`Performing post config change action: None`})
	})

	g.It("Author:rioliu-NonPreRelease-Longduration-High-73374-NodeDisruptionPolicy files with action Reboot [Disruptive]", func() {
		testFileBasedPolicy(oc, "73374", []Action{NewCommonAction(NodeDisruptionPolicyActionReboot)}, []string{})
	})

	g.It("Author:rioliu-NonPreRelease-High-73375-NodeDisruptionPolicy files with action Restart [Disruptive]", func() {
		testFileBasedPolicy(oc, "73375", []Action{NewRestartAction("crio.service")}, []string{`Performing post config change action: Restart`, `crio.service service restarted successfully`})
	})

	g.It("Author:rioliu-NonPreRelease-High-73378-NodeDisruptionPolicy files with action Reload [Disruptive]", func() {
		testFileBasedPolicy(oc, "73378", []Action{NewReloadAction("crio.service")}, []string{`Performing post config change action: Reload`, `crio.service service reloaded successfully`})
	})

	g.It("Author:rioliu-NonPreRelease-High-73385-NodeDisruptionPolicy files with action DaemonReload [Disruptive]", func() {
		testFileBasedPolicy(oc, "73385", []Action{NewCommonAction(NodeDisruptionPolicyActionDaemonReload)}, []string{`Performing post config change action: DaemonReload`, `daemon-reload service reloaded successfully`})
	})

	g.It("Author:rioliu-NonPreRelease-Longduration-High-73388-NodeDisruptionPolicy files with action Drain [Disruptive]", func() {
		testFileBasedPolicy(oc, "73388", []Action{NewCommonAction(NodeDisruptionPolicyActionDrain)}, []string{})
	})

	g.It("Author:rioliu-NonPreRelease-Longduration-High-73389-NodeDisruptionPolicy files with multiple actions [Disruptive]", func() {
		testFileBasedPolicy(oc, "73389", []Action{
			NewCommonAction(NodeDisruptionPolicyActionDrain),
			NewCommonAction(NodeDisruptionPolicyActionDaemonReload),
			NewReloadAction("crio.service"),
			NewRestartAction("crio.service"),
		}, []string{
			`Performing post config change action: Reload`,
			`crio.service service reloaded successfully`,
			`Performing post config change action: Restart`,
			`crio.service service restarted successfully`,
			`Performing post config change action: DaemonReload`,
			`daemon-reload service reloaded successfully`,
		})
	})

})

// test func for file based policy test cases
func testFileBasedPolicy(oc *exutil.CLI, caseID string, actions []Action, expectedLogs []string) {

	var (
		mcName     = fmt.Sprintf("create-test-file-%s-%s", caseID, exutil.GetRandomString())
		filePath   = fmt.Sprintf("/etc/test-file-policy-%s-%s", caseID, exutil.GetRandomString())
		fileConfig = getURLEncodedFileConfig(filePath, fmt.Sprintf("test-%s", caseID), "420")
		workerNode = NewNodeList(oc.AsAdmin()).GetAllCoreOsWokerNodesOrFail()[0]
		workerMcp  = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
	)

	exutil.By("Patch ManchineConfiguration cluster")
	ndp := NewNodeDisruptionPolicy(oc)
	defer ndp.Rollback()
	o.Expect(ndp.AddFilePolicy(filePath, actions...).Apply()).To(o.Succeed(), "Patch ManchineConfiguration failed")

	exutil.By("Check the nodeDisruptionPolicyStatus, new change should be merged")
	o.Expect(ndp.IsUpdated()).To(o.BeTrue(), "New policies are not merged properly")

	exutil.By("Create a test file on worker node")
	mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
	mc.SetParams(fmt.Sprintf("FILES=[%s]", fileConfig))
	mc.skipWaitForMcp = true
	defer mc.delete()
	mc.create()

	// check MCN for reboot and drain
	checkMachineConfigNode(oc, workerNode.GetName(), actions)
	workerMcp.waitForComplete()
	// check MCD logs if expectedLogs is not empty
	checkMachineConfigDaemonLog(workerNode, expectedLogs)
}

// test func used to check expected logs in MCD log
func checkMachineConfigDaemonLog(node Node, expectedLogs []string) {
	if len(expectedLogs) > 0 {
		exutil.By("Check MCD log for post config actions")
		logs, err := node.GetMCDaemonLogs("update.go")
		o.Expect(err).NotTo(o.HaveOccurred(), "Get MCD log failed")
		for _, log := range expectedLogs {
			o.Expect(logs).Should(o.ContainSubstring(log), "Cannot find expected log for post config actions")
		}
	}
}

// test func to check MCN by input actions
func checkMachineConfigNode(oc *exutil.CLI, nodeName string, actions []Action) {

	hasRebootAction := hasAction(NodeDisruptionPolicyActionReboot, actions)
	hasDrainAction := hasAction(NodeDisruptionPolicyActionDrain, actions)

	mcn := NewMachineConfigNode(oc.AsAdmin(), nodeName)
	if hasDrainAction {
		exutil.By("Check whether the node is drained")
		o.Eventually(mcn.GetDrained, "5m", "2s").Should(o.Equal("True"))
	}
	if hasRebootAction {
		exutil.By("Check whether the node is rebooted")
		o.Eventually(mcn.GetRebootedNode, "10m", "6s").Should(o.Equal("True"))
	}
}

func hasAction(actnType string, actions []Action) bool {
	found := false
	for _, a := range actions {
		if a.Type == actnType {
			found = true
			break
		}
	}
	return found
}
