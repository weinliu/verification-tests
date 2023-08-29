package rosacli

import (
	"bytes"
)

type MachinePoolService interface {
	listMachinePool(clusterID string) (bytes.Buffer, error)
	reflectNodePoolList(result bytes.Buffer) (npl NodePoolList, err error)
	reflectMachinePoolList(result bytes.Buffer) (mpl MachinePoolList, err error)
	createMachinePool(clusterID string, flags ...string) (bytes.Buffer, error)
	editMachinePool(clusterID string, machinePoolName string, flags ...string) (bytes.Buffer, error)
	deleteMachinePool(clusterID string, machinePoolName string) (bytes.Buffer, error)
}

var _ MachinePoolService = &machinepoolService{}

type machinepoolService service

// Struct for the 'rosa list machinepool' output for hosted-cp clusters
type NodePool struct {
	ID               string `json:"ID,omitempty"`
	AutoScaling      string `json:"AUTOSCALING,omitempty"`
	DesiredReplicas  string `json:"DESIRED REPLICAS,omitempty"`
	CurrentReplicas  string `json:"CURRENT REPLICAS,omitempty"`
	InstanceType     string `json:"INSTANCE TYPE,omitempty"`
	Lables           string `json:"LABELS,omitempty"`
	Taints           string `json:"TAINTS,omitempty"`
	AvaliablityZones string `json:"AVAILABILITY ZONES,omitempty"`
	Subnet           string `json:"SUBNET,omitempty"`
	Version          string `json:"VERSION,omitempty"`
	AutoRepair       string `json:"AUTOREPAIR,omitempty"`
	TuningConfigs    string `json:"TUNING CONFIGS,omitempty"`
	Message          string `json:"MESSAGE,omitempty"`
}
type NodePoolList struct {
	NodePools []NodePool `json:"NodePools,omitempty"`
}

// Struct for the 'rosa list machinepool' output for non-hosted-cp clusters
type MachinePool struct {
	ID               string `json:"ID,omitempty"`
	AutoScaling      string `json:"AUTOSCALING,omitempty"`
	Replicas         string `json:"REPLICAS,omitempty"`
	InstanceType     string `json:"INSTANCE TYPE,omitempty"`
	Lables           string `json:"LABELS,omitempty"`
	Taints           string `json:"TAINTS,omitempty"`
	AvaliablityZones string `json:"AVAILABILITY ZONES,omitempty"`
	Subnets          string `json:"SUBNETS,omitempty"`
	SpotInstances    string `json:"SPOT INSTANCES,omitempty"`
	DiskSize         string `json:"DISK SIZE,omitempty"`
}
type MachinePoolList struct {
	MachinePools []MachinePool `json:"MachinePools,omitempty"`
}

// Create MachinePool
func (m *machinepoolService) createMachinePool(clusterID string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	createMachinePool := m.client.Runner.
		Cmd("create", "machinepool").
		CmdFlags(combflags...)

	return createMachinePool.Run()
}

// List MachinePool
func (m *machinepoolService) listMachinePool(clusterID string) (bytes.Buffer, error) {
	listUsers := m.client.Runner.
		Cmd("list", "machinepool").
		CmdFlags("-c", clusterID)
	return listUsers.Run()
}

// Delete MachinePool
func (m *machinepoolService) deleteMachinePool(clusterID string, machinePoolName string) (bytes.Buffer, error) {
	deleteMachinePool := m.client.Runner.
		Cmd("delete", "machinepool").
		CmdFlags("-c", clusterID, machinePoolName, "-y")

	return deleteMachinePool.Run()
}

// Edit MachinePool
func (m *machinepoolService) editMachinePool(clusterID string, machinePoolName string, flags ...string) (bytes.Buffer, error) {
	combflags := append([]string{"-c", clusterID}, flags...)
	combflags = append(combflags, machinePoolName)
	editMachinePool := m.client.Runner.
		Cmd("edit", "machinepool").
		CmdFlags(combflags...)

	return editMachinePool.Run()
}

// Pasrse the result of 'rosa list machinepool' to MachinePoolList struct
func (m *machinepoolService) reflectMachinePoolList(result bytes.Buffer) (mpl MachinePoolList, err error) {
	mpl = MachinePoolList{}
	theMap := m.client.Parser.tableData.Input(result).Parse().output
	for _, machinepoolItem := range theMap {
		mp := &MachinePool{}
		err = mapStructure(machinepoolItem, mp)
		if err != nil {
			return
		}
		mpl.MachinePools = append(mpl.MachinePools, *mp)
	}
	return mpl, err
}

// Pasrse the result of 'rosa list machinepool' to NodePoolList struc
func (m *machinepoolService) reflectNodePoolList(result bytes.Buffer) (npl NodePoolList, err error) {
	npl = NodePoolList{}
	theMap := m.client.Parser.tableData.Input(result).Parse().output
	for _, nodepoolItem := range theMap {
		np := &NodePool{}
		err = mapStructure(nodepoolItem, np)
		if err != nil {
			return
		}
		npl.NodePools = append(npl.NodePools, *np)
	}
	return npl, err
}

// Get specified machinepool by machinepool id
func (mpl MachinePoolList) machinepool(id string) (mp MachinePool, err error) {
	for _, mpItem := range mpl.MachinePools {
		if mpItem.ID == id {
			mp = mpItem
			return
		}
	}
	return
}

// Get specified nodepool by machinepool id
func (npl NodePoolList) nodepool(id string) (np NodePool, err error) {
	for _, npItem := range npl.NodePools {
		if npItem.ID == id {
			np = npItem
			return
		}
	}
	return
}
