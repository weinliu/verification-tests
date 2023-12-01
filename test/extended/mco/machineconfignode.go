package mco

import (
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

// MachineConfigNode resource type declaration
type MachineConfigNode struct {
	Resource
}

// MachineConfigNodeList resource type declaration
type MachineConfigNodeList struct {
	ResourceList
}

// NewMachineConfigNode constructor to get MCN resource
func NewMachineConfigNode(oc *exutil.CLI, node string) *MachineConfigNode {
	return &MachineConfigNode{Resource: *NewResource(oc, "machineconfignode", node)}
}

// NewMachineConfigNodeList constructor to get MCN list
func NewMachineConfigNodeList(oc *exutil.CLI) *MachineConfigNodeList {
	return &MachineConfigNodeList{ResourceList: *NewResourceList(oc, "machineconfignodes")}
}

// GetAll get list of MachineConfigNode
func (mcnl *MachineConfigNodeList) GetAll() ([]MachineConfigNode, error) {
	resources, err := mcnl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}

	allMCNs := make([]MachineConfigNode, 0, len(resources))
	for _, mcn := range resources {
		allMCNs = append(allMCNs, *NewMachineConfigNode(mcnl.oc, mcn.GetName()))
	}

	return allMCNs, nil
}

// GetDesiredMachineConfig get value of `.spec.configVersion.desired`
func (mcn *MachineConfigNode) GetDesiredMachineConfig() string {
	return mcn.GetOrFail(`{.spec.configVersion.desired}`)
}

// GetPool get value of `.spec.pool.name`
func (mcn *MachineConfigNode) GetPool() string {
	return mcn.GetOrFail(`{.spec.pool.name}`)
}

// GetNode get value of `.spec.node.name`
func (mcn *MachineConfigNode) GetNode() string {
	return mcn.GetOrFail(`{.spec.node.name}`)
}
