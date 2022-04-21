package mco

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type node struct {
	Resource
}

type nodeList struct {
	ResourceList
}

// NewNode construct a new node struct
func NewNode(oc *exutil.CLI, name string) *node {
	//NewResource(oc, "node", name)
	return &node{*NewResource(oc, "node", name)}
}

// NewNodeList construct a new node list struct to handle all existing nodes
func NewNodeList(oc *exutil.CLI) *nodeList {
	return &nodeList{*NewResourceList(oc, "node")}
}

// String implements the Stringer interface
func (n node) String() string {
	return n.GetName()
}

// DebugNode creates a debugging session of the node with chroot
func (n *node) DebugNodeWithChroot(cmd ...string) (string, error) {
	return exutil.DebugNodeWithChroot(n.oc, n.name, cmd...)
}

// DebugNodeWithOptions launch debug container with options e.g. --image
func (n *node) DebugNodeWithOptions(options []string, cmd ...string) (string, error) {
	return exutil.DebugNodeWithOptions(n.oc, n.name, options, cmd...)
}

// DebugNode creates a debugging session of the node
func (n *node) DebugNode(cmd ...string) (string, error) {
	return exutil.DebugNode(n.oc, n.name, cmd...)
}

// AddCustomLabel add the given label to the node
func (n *node) AddCustomLabel(label string) (string, error) {
	return exutil.AddCustomLabelToNode(n.oc, n.name, label)

}

// DeleteCustomLabel removes the given label from the node
func (n *node) DeleteCustomLabel(label string) (string, error) {
	return exutil.DeleteCustomLabelFromNode(n.oc, n.name, label)

}

// GetMachineConfigDaemon returns the name of the ConfigDaemon pod for this node
func (n *node) GetMachineConfigDaemon() string {
	machineConfigDaemon, err := exutil.GetPodName(n.oc, "openshift-machine-config-operator", "k8s-app=machine-config-daemon", n.name)
	o.Expect(err).NotTo(o.HaveOccurred())
	return machineConfigDaemon
}

// GetNodeHostname returns the cluster node hostname
func (n *node) GetNodeHostname() (string, error) {
	return exutil.GetNodeHostname(n.oc, n.name)
}

// ForceReapplyConfiguration create the file `/run/machine-config-daemon-force` in the node
//  in order to force MCO to reapply the current configuration
func (n *node) ForceReapplyConfiguration() error {
	_, err := n.DebugNodeWithChroot("touch", "/run/machine-config-daemon-force")

	return err
}

// GetUnitStatus executes `systemctl status` command on the node and returns the output
func (n *node) GetUnitStatus(unitName string) (string, error) {
	return n.DebugNodeWithChroot("systemctl", "status", unitName)
}

// UnmaskService executes `systemctl unmask` command on the node and returns the output
func (n *node) UnmaskService(svcName string) (string, error) {
	return n.DebugNodeWithChroot("systemctl", "unmask", svcName)
}

// PollIsCordoned returns a function that can be used by Gomega to poll the if the node is cordoned (with Eventually/Consistently)
func (n *node) PollIsCordoned() func() bool {
	return func() bool {
		key, err := n.Get(`{.spec.taints[?(@.effect=="NoSchedule")].key}`)
		if err != nil {
			return false
		}
		return key == "node.kubernetes.io/unschedulable"
	}
}

// GetCurrentMachineConfig returns the ID of the current machine config used in the node
func (n *node) GetCurrentMachineConfig() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/currentConfig}`)
}

// GetDesiredMachineConfig returns the ID of the machine config that we want the node to use
func (n *node) GetDesiredMachineConfig() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/desiredConfig}`)
}

// GetMachineConfigState returns the State of machineconfiguration process
func (n *node) GetMachineConfigState() string {
	return n.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/state}`)
}

// IsUpdated returns if the node is pending for machineconfig configuration or it is up to date
func (n *node) IsUpdated() bool {
	return (n.GetCurrentMachineConfig() == n.GetDesiredMachineConfig()) && (n.GetMachineConfigState() == "Done")
}

// IsTainted returns if the node hast taints or not
func (n *node) IsTainted() bool {
	taint, err := n.Get("{.spec.taints}")
	return err == nil && taint != ""
}

// IsUpdating returns if the node is currently updating the machine configuration
func (n *node) IsUpdating() bool {
	return n.GetMachineConfigState() == "Working"
}

//GetAll returns a []node list with all existing nodes
func (nl *nodeList) GetAll() ([]node, error) {
	allNodeResources, err := nl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allNodes := make([]node, 0, len(allNodeResources))

	for _, nodeRes := range allNodeResources {
		allNodes = append(allNodes, *NewNode(nl.oc, nodeRes.name))
	}

	return allNodes, nil
}

// GetAllMasterNodes returns a list of master Nodes
func (nl nodeList) GetAllMasterNodes() ([]node, error) {
	nl.ByLabel("node-role.kubernetes.io/master=")

	return nl.GetAll()
}

// GetAllWorkerNodes returns a list of worker Nodes
func (nl nodeList) GetAllWorkerNodes() ([]node, error) {
	nl.ByLabel("node-role.kubernetes.io/worker=")

	return nl.GetAll()
}

// GetAllMasterNodesOrFail returns a list of master Nodes
func (nl nodeList) GetAllMasterNodesOrFail() []node {
	masters, err := nl.GetAllMasterNodes()
	o.Expect(err).NotTo(o.HaveOccurred())
	return masters
}

// GetAllWorkerNodes returns a list of worker Nodes
func (nl nodeList) GetAllWorkerNodesOrFail() []node {
	workers, err := nl.GetAllWorkerNodes()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workers
}

func (nl nodeList) GetAllRhelWokerNodesOrFail() []node {
	nl.ByLabel("node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhel")

	workers, err := nl.GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workers
}

func (nl nodeList) GetAllCoreOsWokerNodesOrFail() []node {
	nl.ByLabel("node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhcos")

	workers, err := nl.GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workers
}

func (nl *nodeList) GetTaintedNodes() []node {
	allNodes, err := nl.GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())

	taintedNodes := []node{}
	for _, node := range allNodes {
		if node.IsTainted() {
			taintedNodes = append(taintedNodes, node)
		}
	}

	return taintedNodes
}
