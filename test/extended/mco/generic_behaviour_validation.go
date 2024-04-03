package mco

import (
	"fmt"
	"time"

	o "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type Checker interface {
	Check(checkedNodes ...Node)
}

type PreRebootMCDLogChecker struct {
	Matcher  types.GomegaMatcher
	ErrorMsg string
	Desc     string
}

// CheckLogs  nodeLogs param is a map where the key is the name of the node and the value is the pre-reboot MCD logs
func (preMCDLogChecker PreRebootMCDLogChecker) CheckLogs(nodeLogs map[string]string, checkedNodes ...Node) {
	msg := "Checking pre-reboot MCD logs"
	if preMCDLogChecker.Desc != "" {
		msg = preMCDLogChecker.Desc
	}
	exutil.By(msg)
	o.Expect(checkedNodes).NotTo(o.BeEmpty(), "Refuse to check an empty list of nodes")

	for _, node := range checkedNodes {
		logs, ok := nodeLogs[node.GetName()]
		o.Expect(ok).To(o.BeTrue(), "Something went wrong. There pre-reboot nodes found for node %s", node.GetName())

		o.Expect(logs).Should(preMCDLogChecker.Matcher, preMCDLogChecker.ErrorMsg)
	}

	logger.Infof("OK!\n")
}

type PostRebootMCDLogChecker struct {
	Matcher  types.GomegaMatcher
	ErrorMsg string
	Desc     string
}

func (postMCDLogChecker PostRebootMCDLogChecker) Check(checkedNodes ...Node) {
	msg := "Checking MCD logs after reboot"
	if postMCDLogChecker.Desc != "" {
		msg = postMCDLogChecker.Desc
	}
	exutil.By(msg)
	o.Expect(checkedNodes).NotTo(o.BeEmpty(), "Refuse to check an empty list of nodes")

	for _, node := range checkedNodes {
		logger.Infof("Checking node %s", node.GetName())
		o.Expect(
			exutil.GetSpecificPodLogs(node.oc, MachineConfigNamespace, MachineConfigDaemon, node.GetMachineConfigDaemon(), ""),
		).Should(postMCDLogChecker.Matcher,
			postMCDLogChecker.ErrorMsg)
	}
	logger.Infof("OK!\n")
}

type CommandOutputChecker struct {
	Command  []string
	Matcher  types.GomegaMatcher
	ErrorMsg string
	Desc     string
}

func (cOutChecker CommandOutputChecker) Check(checkedNodes ...Node) {
	msg := "Executing verification commands"
	if cOutChecker.Desc != "" {
		msg = cOutChecker.Desc
	}
	exutil.By(msg)
	o.Expect(checkedNodes).NotTo(o.BeEmpty(), "Refuse to check an empty list of nodes")

	for _, node := range checkedNodes {
		logger.Infof("In node %s. Executing command %s", node.GetName(), cOutChecker.Command)
		o.Expect(
			node.DebugNodeWithChroot(cOutChecker.Command...),
		).To(cOutChecker.Matcher,
			"Command %s validation failed in node %s: %s", cOutChecker.Command, node.GetName(), cOutChecker.ErrorMsg)
	}
	logger.Infof("OK!\n")

}

type NodeEventsChecker struct {
	EventsSequence        []string
	Desc                  string
	EventsAreNotTriggered bool
}

func (ec NodeEventsChecker) Check(nodes ...Node) {
	if ec.EventsAreNotTriggered {
		ec.checkEventsAreTriggeredSequentially(nodes...)
	} else {
		ec.checkEventsAreNotTriggered(nodes...)
	}
}

func (ec NodeEventsChecker) checkEventsAreTriggeredSequentially(checkedNodes ...Node) {
	msg := fmt.Sprintf("Checking triggered events: %s", ec.EventsSequence)
	if ec.Desc != "" {
		msg = ec.Desc
	}
	exutil.By(msg)
	o.Expect(checkedNodes).NotTo(o.BeEmpty(), "Refuse to check an empty list of nodes")

	for _, node := range checkedNodes {
		logger.Infof("Checking node %s", node.GetName())
		o.Expect(node.GetEvents()).To(HaveEventsSequence(ec.EventsSequence...),
			"The expected events sequence did not happen in node %s. Expected: %s", node.GetName(), ec.EventsSequence)
	}
	logger.Infof("OK!\n")
}

func (ec NodeEventsChecker) checkEventsAreNotTriggered(checkedNodes ...Node) {
	msg := fmt.Sprintf("Checking that these events were NOT triggered: %s", ec.EventsSequence)
	if ec.Desc != "" {
		msg = ec.Desc
	}
	exutil.By(msg)
	o.Expect(checkedNodes).NotTo(o.BeEmpty(), "Refuse to check an empty list of nodes")

	for _, node := range checkedNodes {
		logger.Infof("Checking node %s", node.GetName())
		for _, eventReason := range ec.EventsSequence {
			o.Expect(node.GetEvents()).NotTo(HaveEventsSequence(eventReason),
				"The event %s should not be triggered in node %s, but it was trggered", node.GetName(), eventReason)
		}
	}
	logger.Infof("OK!\n")
}

type RemoteFileChecker struct {
	FileFullPath string
	Matcher      types.GomegaMatcher
	ErrorMsg     string
	Desc         string
}

func (rfc RemoteFileChecker) Check(checkedNodes ...Node) {
	msg := fmt.Sprintf("Checking file: %s", rfc.FileFullPath)
	if rfc.Desc != "" {
		msg = rfc.Desc
	}
	exutil.By(msg)
	o.Expect(checkedNodes).NotTo(o.BeEmpty(), "Refuse to check an empty list of nodes")

	for _, node := range checkedNodes {
		rf := NewRemoteFile(node, rfc.FileFullPath)
		logger.Infof("Checking remote file %s", rf)
		o.Expect(rf).To(rfc.Matcher,
			"Validation of %s failed: %", rf, rfc.ErrorMsg)
	}
	logger.Infof("OK!\n")
}

type UpdateBehaviourValidator struct {
	mcp          *MachineConfigPool
	checkedNodes []Node
	startTime    time.Time

	PreRebootMCDLogsCheckers []PreRebootMCDLogChecker
	Checkers                 []Checker

	SkipRebootNodesValidation  bool
	RebootNodesShouldBeSkipped bool

	SkipDrainNodesValidation bool
	DrainNodesShoulBeSkipped bool

	SkipRestartCrioValidation  bool
	RestartCrioShouldBeSkipped bool

	SkipReloadCrioValidation bool
	ShouldRestartCrio        bool
}

func (v *UpdateBehaviourValidator) Initialize(mcp *MachineConfigPool, nodes []Node) {
	exutil.By("Gathering initial data needed for the verification steps")
	v.mcp = mcp
	// If no node is provided we test only the first node to be updated in the pool
	if len(nodes) == 0 {
		v.checkedNodes = []Node{mcp.GetSortedNodesOrFail()[0]}
	}

	logger.Infof("Start capturing events in nodes")
	for i := range v.checkedNodes {
		o.Expect(v.checkedNodes[i].IgnoreEventsBeforeNow()).NotTo(o.HaveOccurred(),
			"Error getting the latest event in node %s", v.checkedNodes[i].GetName())
	}

	logger.Infof("Getting starting date")
	// TODO: maybe we should not assume that all nodes are synced
	var dErr error
	v.startTime, dErr = v.checkedNodes[0].GetDate()
	o.Expect(dErr).ShouldNot(o.HaveOccurred(), "Error getting date in node %s", v.checkedNodes[0].GetName())
	logger.Infof("Node %s current date %s", v.checkedNodes[0], v.startTime)
	logger.Infof("OK!\n")
}

func (v *UpdateBehaviourValidator) Validate() {
	if len(v.PreRebootMCDLogsCheckers) > 0 && v.SkipRebootNodesValidation == false && v.RebootNodesShouldBeSkipped {
		e2e.Failf("Inconsistent behaviour! Nodes are expected to be rebooted because PreRebootMCDLogsCheckers were added, but at the same time RebootNodesShouldBeSkipped is true")
	}

	// Execute the verification of the pre-reboot MCD logs
	if len(v.PreRebootMCDLogsCheckers) > 0 {
		v.checkPreRebootMCDLogs()
	}

	exutil.By("Wait for configuration to be applied")
	v.mcp.waitForComplete()
	logger.Infof("OK!\n")

	// Execute the verification of the rebooted nodes (all nodes should have the same date, since they are synced)
	if !v.SkipRebootNodesValidation {
		v.checkRebootNodes()
	}

	// Execute the verification of the drain nodes behaviour
	if !v.SkipDrainNodesValidation {
		v.checkDrainNodes()
	}

	// Execute the verification of the crio service restart
	if !v.SkipRestartCrioValidation {
		v.checkCrioRestart()
	}

	// Execute the verification of the crio service reload
	if !v.SkipReloadCrioValidation {
		v.checkCrioReload()
	}

	// Execute the generic Checks
	for _, checker := range v.Checkers {
		checker.Check(v.checkedNodes...)
	}
}

// checkCrioReloaded checks if crio was reloaded or not.
func (v *UpdateBehaviourValidator) checkCrioReload() {
	if v.ShouldRestartCrio {
		exutil.By("Checking that crio service was reloaded")
	} else {
		exutil.By("Checking that crio service were NOT reloaded")
	}

	// If the startTime is empty it means that something went wrong in the automation
	// If at any moment we decide to allow an empty value here we can transform this assertion into a warning.
	o.Expect(v.startTime).NotTo(o.Equal(time.Time{}),
		"The provided comparison time was EMPTY while trying to guess if the crio service was restarted")

	for _, node := range v.checkedNodes {
		if v.ShouldRestartCrio {
			o.Expect(node.GetUnitExecReloadStartTime("crio.service")).To(o.BeTemporally(">", v.startTime),
				"Crio service was NOT restarted, but it should be")
		} else {
			o.Expect(node.GetUnitExecReloadStartTime("crio.service")).To(o.BeTemporally("<", v.startTime),
				"Crio service was restarted, but crio restart should have been skipped")
		}
	}

	logger.Infof("OK!\n")
}

// checkCrioRestart checks if crio was restarted or not. It can be restarted because the "systemctl restart crio" command is executed, or because the node was restarted (hence, restarting crio as well)
func (v *UpdateBehaviourValidator) checkCrioRestart() {
	// Since we can check crio restart with reboot is skipped and when reboot is executed, we don't know actually were to
	// look for the log message (pre-reboot log or post-reboot log). So we can't check the crio log message if we dont have more information
	// It is better to actually get the crio.service status and the how much time it has passed since it was restarted
	if v.RestartCrioShouldBeSkipped {
		exutil.By("Checking that crio was NOT restarted")
	} else {
		exutil.By("Checking that crio was restarted")
	}

	// If the startTime is empty it means that something went wrong in the automation
	// If at any moment we decide to allow an empty value here we can transform this assertion into a warning.
	o.Expect(v.startTime).NotTo(o.Equal(time.Time{}),
		"The provided comparison time was EMPTY while trying to guess if the crio service was restarted")

	for _, node := range v.checkedNodes {
		if v.RestartCrioShouldBeSkipped {
			o.Expect(node.GetUnitActiveEnterTime("crio.service")).To(o.BeTemporally("<", v.startTime),
				"Crio service was restarted, but crio restart should have been skipped")
		} else {
			o.Expect(node.GetUnitActiveEnterTime("crio.service")).To(o.BeTemporally(">", v.startTime),
				"Crio service was NOT restarted, but it should be")
		}
	}

	logger.Infof("OK!\n")
}

func (v *UpdateBehaviourValidator) checkDrainNodes() {
	var (
		skipDrainLogMsg = "Changes do not require drain, skipping"
		execDrainLogMsg = "requesting cordon and drain via annotation to controller"
	)
	// We could check the "Drain" event in this function, but sometimes the events are not triggered and we don't know why
	// Until events are not more stable we should not force even validation in ALL our tests, only in those that we want to expose to this instability

	if v.DrainNodesShoulBeSkipped {
		exutil.By("Checking that nodes were NOT drained")
	} else {
		exutil.By("Checking that nodes were drained")
	}

	for _, node := range v.checkedNodes {
		if v.DrainNodesShoulBeSkipped {
			logger.Infof("Checking that node %s was NOT drained", node.GetName())
			o.Expect(
				exutil.GetSpecificPodLogs(node.oc, MachineConfigNamespace, MachineConfigDaemon, node.GetMachineConfigDaemon(), ""),
			).Should(o.ContainSubstring(skipDrainLogMsg),
				"Error! The node %s was drained, but the drain operation should have been skipped", node.GetName())
		} else {
			logger.Infof("Checking that node %s was drained", node.GetName())
			o.Expect(
				exutil.GetSpecificPodLogs(node.oc, MachineConfigNamespace, MachineConfigDaemon, node.GetMachineConfigDaemon(), ""),
			).Should(o.ContainSubstring(execDrainLogMsg),
				"Error! The node %s was NOT drained, but it should be", node.GetName())
		}
	}

	logger.Infof("OK!\n")
}

func (v *UpdateBehaviourValidator) checkRebootNodes() {
	var (
		skipRebootLogMsg = "skipping reboot"
	)

	// We could check the "Reboot" event in this function, but sometimes the events are not triggered and we don't know why
	// Until events are not more stable we should not force even validation in ALL our tests, only in those that we want to expose to this instability
	if v.RebootNodesShouldBeSkipped {
		exutil.By("Checking that nodes were NOT rebooted")
	} else {
		exutil.By("Checking that nodes were rebooted")
	}

	// If the startTime is empty it means that something went wrong in the automation
	// If at any moment we decide to allow an empty value here we can transform this assertion into a warning.
	o.Expect(v.startTime).NotTo(o.Equal(time.Time{}),
		"The provided comparison time was EMPTY while trying to guess if the nodes were rebooted")

	for _, node := range v.checkedNodes {
		if v.RebootNodesShouldBeSkipped {
			logger.Infof("Checking that node %s was NOT rebooted", node.GetName())
			o.Expect(node.GetUptime()).Should(o.BeTemporally("<", v.startTime),
				"The node %s must NOT be rebooted, but it was rebooted. Uptime date happened after the start config time.", node.GetName())

			o.Expect(
				exutil.GetSpecificPodLogs(node.oc, MachineConfigNamespace, MachineConfigDaemon, node.GetMachineConfigDaemon(), ""),
			).Should(o.ContainSubstring(skipRebootLogMsg),
				"Error! The node %s was rebooted, but the reboot operation should have been skipped. Cannot find the 'skipping reboot' log msg", node.GetName())
		} else {
			logger.Infof("Checking that node %s was rebooted", node.GetName())
			o.Expect(node.GetUptime()).Should(o.BeTemporally(">", v.startTime),
				"The node %s must be rebooted, but it was not. Uptime date happened before the start config time.", node.GetName())

			o.Expect(
				exutil.GetSpecificPodLogs(node.oc, MachineConfigNamespace, MachineConfigDaemon, node.GetMachineConfigDaemon(), ""),
			).ShouldNot(o.ContainSubstring(skipRebootLogMsg),
				"Error! The node %s was NOT rebooted, logs are reporting a 'skipping reboot' message.", node.GetName())
		}
	}

	logger.Infof("OK!\n")
}

func (v *UpdateBehaviourValidator) checkPreRebootMCDLogs() {
	preRebootLogs, err := v.mcp.CaptureAllNodeLogsBeforeRestart()
	o.Expect(err).NotTo(o.HaveOccurred(), "Error capturing get MCD logs before the nodes reboot")

	for _, preRebootMCDLogsChecker := range v.PreRebootMCDLogsCheckers {
		preRebootMCDLogsChecker.CheckLogs(preRebootLogs, v.checkedNodes...)
	}
	logger.Infof("OK!\n")
}
