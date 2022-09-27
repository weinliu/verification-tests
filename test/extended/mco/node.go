package mco

import (
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	logger "github.com/openshift/openshift-tests-private/test/extended/mco/logext"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Node is used to handle node OCP resources
type Node struct {
	Resource
}

// NodeList handles list of nodes
type NodeList struct {
	ResourceList
}

// NewNode construct a new node struct
func NewNode(oc *exutil.CLI, name string) *Node {
	return &Node{*NewResource(oc, "node", name)}
}

// NewNodeList construct a new node list struct to handle all existing nodes
func NewNodeList(oc *exutil.CLI) *NodeList {
	return &NodeList{*NewResourceList(oc, "node")}
}

// String implements the Stringer interface
func (n Node) String() string {
	return n.GetName()
}

// DebugNodeWithChroot creates a debugging session of the node with chroot
func (n *Node) DebugNodeWithChroot(cmd ...string) (string, error) {
	return exutil.DebugNodeWithChroot(n.oc, n.name, cmd...)
}

// DebugNodeWithChrootStd creates a debugging session of the node with chroot and only returns separated stdout and stderr
func (n *Node) DebugNodeWithChrootStd(cmd ...string) (string, string, error) {
	setErr := exutil.SetNamespacePrivileged(n.oc, n.oc.Namespace())
	if setErr != nil {
		return "", "", setErr
	}

	cargs := []string{"node/" + n.GetName(), "--", "chroot", "/host"}
	cargs = append(cargs, cmd...)

	stdout, stderr, err := n.oc.Run("debug").Args(cargs...).Outputs()

	recErr := exutil.RecoverNamespaceRestricted(n.oc, n.oc.Namespace())
	if recErr != nil {
		return "", "", recErr
	}

	return stdout, stderr, err
}

// DebugNodeWithOptions launch debug container with options e.g. --image
func (n *Node) DebugNodeWithOptions(options []string, cmd ...string) (string, error) {
	return exutil.DebugNodeWithOptions(n.oc, n.name, options, cmd...)
}

// DebugNode creates a debugging session of the node
func (n *Node) DebugNode(cmd ...string) (string, error) {
	return exutil.DebugNode(n.oc, n.name, cmd...)
}

// AddLabel add the given label to the node
func (n *Node) AddLabel(label, value string) (string, error) {
	return exutil.AddLabelToNode(n.oc, n.name, label, value)

}

// DeleteLabel removes the given label from the node
func (n *Node) DeleteLabel(label string) (string, error) {
	logger.Infof("Delete label %s from node %s", label, n.GetName())
	return exutil.DeleteLabelFromNode(n.oc, n.name, label)
}

// WaitForLabelRemoved waits until the given label is not present in the node.
func (n *Node) WaitForLabelRemoved(label string) error {
	logger.Infof("Waiting for label %s to be removed from node %s", label, n.GetName())
	waitErr := wait.Poll(1*time.Minute, 10*time.Minute, func() (bool, error) {
		labels, err := n.Get(`{.metadata.labels}`)
		if err != nil {
			logger.Infof("Error waiting for labels to be removed:%v, and try next round", err)
			return false, nil
		}
		labelsMap := JSON(labels)
		label, err := labelsMap.GetSafe(label)
		if err == nil && !label.Exists() {
			logger.Infof("Label %s has been removed from node %s", label, n.GetName())
			return true, nil
		}
		return false, nil
	})

	if waitErr != nil {
		logger.Errorf("Timeout while waiting for label %s to be delete from node %s. Error: %s",
			label,
			n.GetName(),
			waitErr)
	}

	return waitErr
}

// GetMachineConfigDaemon returns the name of the ConfigDaemon pod for this node
func (n *Node) GetMachineConfigDaemon() string {
	machineConfigDaemon, err := exutil.GetPodName(n.oc, "openshift-machine-config-operator", "k8s-app=machine-config-daemon", n.name)
	o.Expect(err).NotTo(o.HaveOccurred())
	return machineConfigDaemon
}

// GetNodeHostname returns the cluster node hostname
func (n *Node) GetNodeHostname() (string, error) {
	return exutil.GetNodeHostname(n.oc, n.name)
}

// ForceReapplyConfiguration create the file `/run/machine-config-daemon-force` in the node
//  in order to force MCO to reapply the current configuration
func (n *Node) ForceReapplyConfiguration() error {
	logger.Infof("Forcing reapply configuration in node %s", n.GetName())
	_, err := n.DebugNodeWithChroot("touch", "/run/machine-config-daemon-force")

	return err
}

// GetUnitStatus executes `systemctl status` command on the node and returns the output
func (n *Node) GetUnitStatus(unitName string) (string, error) {
	return n.DebugNodeWithChroot("systemctl", "status", unitName)
}

// UnmaskService executes `systemctl unmask` command on the node and returns the output
func (n *Node) UnmaskService(svcName string) (string, error) {
	return n.DebugNodeWithChroot("systemctl", "unmask", svcName)
}

// GetRpmOstreeStatus returns the rpm-ostree status in json format
func (n Node) GetRpmOstreeStatus(asJSON bool) (string, error) {
	args := []string{"rpm-ostree", "status"}
	if asJSON {
		args = append(args, "--json")
	}
	stringStatus, _, err := n.DebugNodeWithChrootStd(args...)
	logger.Debugf("json rpm-ostree status:\n%s", stringStatus)
	return stringStatus, err
}

// GetBootedOsTreeDeployment returns the ostree deployment currently booted. In json format
func (n Node) GetBootedOsTreeDeployment() (string, error) {
	stringStatus, err := n.GetRpmOstreeStatus(true)
	if err != nil {
		return "", err
	}

	deployments := JSON(stringStatus).Get("deployments")
	for _, item := range deployments.Items() {
		booted := item.Get("booted").ToBool()
		if booted {
			return item.AsJSONString()
		}
	}
	logger.Infof("WARNING! No booted deployment found in node %s", n.GetName())
	return "", nil
}

// PollIsCordoned returns a function that can be used by Gomega to poll the if the node is cordoned (with Eventually/Consistently)
func (n *Node) PollIsCordoned() func() bool {
	return func() bool {
		key, err := n.Get(`{.spec.taints[?(@.key=="node.kubernetes.io/unschedulable")].key}`)
		if err != nil {
			return false
		}

		return key != ""
	}
}

// GetCurrentMachineConfig returns the ID of the current machine config used in the node
func (n Node) GetCurrentMachineConfig() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/currentConfig}`)
}

// GetDesiredMachineConfig returns the ID of the machine config that we want the node to use
func (n Node) GetDesiredMachineConfig() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/desiredConfig}`)
}

// GetMachineConfigState returns the State of machineconfiguration process
func (n Node) GetMachineConfigState() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/state}`)
}

// GetDesiredConfig returns the desired machine config for this node
func (n Node) GetDesiredConfig() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/desiredConfig}`)
}

// GetDesiredDrain returns the last desired machine config that needed a drain operation in this node
func (n Node) GetDesiredDrain() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/desiredDrain}`)
}

// GetLastAppliedDrain returns the last applied drain in this node
func (n Node) GetLastAppliedDrain() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/lastAppliedDrain}`)
}

// HasBeenDrained returns a true if the desired and the last applied drain annotations have the same value
func (n Node) HasBeenDrained() bool {
	return n.GetLastAppliedDrain() == n.GetDesiredDrain()
}

// IsUpdated returns if the node is pending for machineconfig configuration or it is up to date
func (n *Node) IsUpdated() bool {
	return (n.GetCurrentMachineConfig() == n.GetDesiredMachineConfig()) && (n.GetMachineConfigState() == "Done")
}

// IsTainted returns if the node hast taints or not
func (n *Node) IsTainted() bool {
	taint, err := n.Get("{.spec.taints}")
	return err == nil && taint != ""
}

// IsUpdating returns if the node is currently updating the machine configuration
func (n *Node) IsUpdating() bool {
	return n.GetMachineConfigState() == "Working"
}

// IsReady returns boolean 'true' if the node is ready. Else it retruns 'false'.
func (n Node) IsReady() bool {
	readyCondition := JSON(n.GetOrFail(`{.status.conditions[?(@.type=="Ready")]}`))
	return readyCondition.Get("status").ToString() == "True"
}

// GetMCDaemonLogs returns the logs of the MachineConfig daemonset pod for this node. The logs will be grepped using the 'filter' parameter
func (n Node) GetMCDaemonLogs(filter string) (string, error) {
	return exutil.GetSpecificPodLogs(n.oc, MachineConfigNamespace, "machine-config-daemon", n.GetMachineConfigDaemon(), filter)
}

// PollMCDaemonLogs returns a function that can be used by gomega Eventually/Consistently functions to poll logs results
// If there is an error, it will return empty string, new need to take that into account building our Eventually/Consistently statement
func (n Node) PollMCDaemonLogs(filter string) func() string {
	return func() string {
		logs, err := n.GetMCDaemonLogs(filter)
		if err != nil {
			return ""
		}
		return logs
	}
}

// CaptureMCDaemonLogsUntilRestartWithTimeout captures all the logs in the MachineConfig daemon pod for this node until the daemon pod is restarted
func (n Node) CaptureMCDaemonLogsUntilRestartWithTimeout(timeout string) (string, error) {
	machineConfigDaemon := n.GetMachineConfigDaemon()
	duration, err := time.ParseDuration(timeout)
	if err != nil {
		return "", err
	}

	c := make(chan string, 1)

	go func() {
		logs, err := n.oc.WithoutNamespace().Run("logs").Args("-n", MachineConfigNamespace, machineConfigDaemon, "-c", "machine-config-daemon", "-f").Output()
		if err != nil {
			logger.Errorf("Error getting %s logs: %s", machineConfigDaemon, err)
		}
		c <- logs
	}()

	select {
	case logs := <-c:
		return logs, nil
	case <-time.After(duration):
		errMsg := fmt.Sprintf(`Node "%s". Timeout while waiting for the daemon pod "%s" -n  "%s" to be restarted`,
			n.GetName(), machineConfigDaemon, MachineConfigNamespace)
		logger.Infof(errMsg)
		return "", fmt.Errorf(errMsg)
	}

}

// GetDate executes `date`command and returns the current time in the node
func (n Node) GetDate() (time.Time, error) {

	date, _, err := n.DebugNodeWithChrootStd(`date`, `+%Y-%m-%dT%H:%M:%SZ`)

	logger.Infof("node %s. DATE: %s", n.GetName(), date)
	if err != nil {
		logger.Errorf("Error trying to get date in node %s: %s", n.GetName(), err)
		return time.Time{}, err
	}
	layout := "2006-01-02T15:04:05Z"
	returnTime, perr := time.Parse(layout, date)
	if perr != nil {
		logger.Errorf("Error trying to parsing date %s in node %s: %s", date, n.GetName(), perr)
		return time.Time{}, perr
	}

	return returnTime, nil
}

// GetUptime executes `uptime -s` command and returns the time when the node was booted
func (n Node) GetUptime() (time.Time, error) {

	uptime, _, err := n.DebugNodeWithChrootStd(`uptime`, `-s`)

	logger.Infof("node %s. UPTIME: %s", n.GetName(), uptime)
	if err != nil {
		logger.Errorf("Error trying to get uptime in node %s: %s", n.GetName(), err)
		return time.Time{}, err
	}
	layout := "2006-01-02 15:04:05"
	returnTime, perr := time.Parse(layout, uptime)
	if perr != nil {
		logger.Errorf("Error trying to parsing uptime %s in node %s: %s", uptime, n.GetName(), perr)
		return time.Time{}, perr
	}

	return returnTime, nil
}

// GetEventsByReasonSince returns a list of all the events with the given reason that are related to this node since the provided date
func (n Node) GetEventsByReasonSince(since time.Time, reason string) ([]Event, error) {
	eventList := NewEventList(n.oc, "default")
	eventList.ByFieldSelector(`reason=` + reason + `,involvedObject.name=` + n.GetName())

	return eventList.GetAllSince(since)
}

// GetAllEventsSince returns a list of all the events related to this node since the provided date
func (n Node) GetAllEventsSince(since time.Time) ([]Event, error) {
	eventList := NewEventList(n.oc, "default")
	eventList.ByFieldSelector(`involvedObject.name=` + n.GetName())

	return eventList.GetAllSince(since)
}

// GetDateWithDelta returns the date in the node +delta
func (n Node) GetDateWithDelta(delta string) (time.Time, error) {
	date, err := n.GetDate()
	if err != nil {
		return time.Time{}, err
	}

	timeDuration, terr := time.ParseDuration(delta)
	if terr != nil {
		logger.Errorf("Error getting delta time %s", terr)
		return time.Time{}, terr
	}

	return date.Add(timeDuration), nil
}

// IsFIPSEnabled check whether fips is enabled on node
func (n *Node) IsFIPSEnabled() (bool, error) {
	output, err := exutil.DebugNodeWithChroot(n.oc, n.name, "fips-mode-setup", "--check")
	if err != nil {
		logger.Errorf("Error checking fips mode %s", err)
	}

	return strings.Contains(output, "FIPS mode is enabled"), err
}

// IsKernelArgEnabled check whether kernel arg is enabled on node
func (n *Node) IsKernelArgEnabled(karg string) (bool, error) {
	unameOut, unameErr := exutil.DebugNodeWithChroot(n.oc, n.name, "bash", "-c", "uname -a")
	if unameErr != nil {
		logger.Errorf("Error checking kernel arg via uname -a: %v", unameErr)
		return false, unameErr
	}

	cliOut, cliErr := exutil.DebugNodeWithChroot(n.oc, n.name, "cat", "/proc/cmdline")
	if cliErr != nil {
		logger.Errorf("Err checking kernel arg via /proc/cmdline: %v", cliErr)
		return false, cliErr
	}

	return (strings.Contains(unameOut, karg) || strings.Contains(cliOut, karg)), nil
}

// GetAll returns a []Node list with all existing nodes
func (nl *NodeList) GetAll() ([]Node, error) {
	allNodeResources, err := nl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allNodes := make([]Node, 0, len(allNodeResources))

	for _, nodeRes := range allNodeResources {
		allNodes = append(allNodes, *NewNode(nl.oc, nodeRes.name))
	}

	return allNodes, nil
}

// GetAllLinux resturns a list with all linux nodes in the cluster
func (nl NodeList) GetAllLinux() ([]Node, error) {
	nl.ByLabel("kubernetes.io/os=linux")

	return nl.GetAll()
}

// GetAllMasterNodes returns a list of master Nodes
func (nl NodeList) GetAllMasterNodes() ([]Node, error) {
	nl.ByLabel("node-role.kubernetes.io/master=")

	return nl.GetAll()
}

// GetAllWorkerNodes returns a list of worker Nodes
func (nl NodeList) GetAllWorkerNodes() ([]Node, error) {
	nl.ByLabel("node-role.kubernetes.io/worker=")

	return nl.GetAll()
}

// GetAllMasterNodesOrFail returns a list of master Nodes
func (nl NodeList) GetAllMasterNodesOrFail() []Node {
	masters, err := nl.GetAllMasterNodes()
	o.Expect(err).NotTo(o.HaveOccurred())
	return masters
}

// GetAllWorkerNodesOrFail returns a list of worker Nodes. Fail the test case if an error happens.
func (nl NodeList) GetAllWorkerNodesOrFail() []Node {
	workers, err := nl.GetAllWorkerNodes()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workers
}

// GetAllLinuxWorkerNodes returns a list of linux worker Nodes
func (nl NodeList) GetAllLinuxWorkerNodes() ([]Node, error) {
	nl.ByLabel("node-role.kubernetes.io/worker=,kubernetes.io/os=linux")

	return nl.GetAll()
}

// GetAllLinuxWorkerNodesOrFail returns a list of linux worker Nodes. Fail the test case if an error happens.
func (nl NodeList) GetAllLinuxWorkerNodesOrFail() []Node {
	workers, err := nl.GetAllLinuxWorkerNodes()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workers
}

// GetAllRhelWokerNodesOrFail returns a list with all RHEL nodes in the cluster. Fail the test if an error happens.
func (nl NodeList) GetAllRhelWokerNodesOrFail() []Node {
	nl.ByLabel("node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhel")

	workers, err := nl.GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workers
}

// GetAllCoreOsWokerNodesOrFail returns a list with all CoreOs nodes in the cluster. Fail the test case if an error happens.
func (nl NodeList) GetAllCoreOsWokerNodesOrFail() []Node {
	nl.ByLabel("node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhcos")

	workers, err := nl.GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workers
}

// GetTaintedNodes returns a list with all tainted nodes in the cluster. Fail the test if an error happens.
func (nl *NodeList) GetTaintedNodes() []Node {
	allNodes, err := nl.GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())

	taintedNodes := []Node{}
	for _, node := range allNodes {
		if node.IsTainted() {
			taintedNodes = append(taintedNodes, node)
		}
	}

	return taintedNodes
}
