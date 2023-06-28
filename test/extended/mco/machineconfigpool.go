package mco

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

// MachineConfigPool struct is used to handle MachineConfigPool resources in OCP
type MachineConfigPool struct {
	template string
	Resource
	MinutesWaitingPerNode int
}

// MachineConfigPoolList struct handles list of MCPs
type MachineConfigPoolList struct {
	ResourceList
}

// NewMachineConfigPool create a NewMachineConfigPool struct
func NewMachineConfigPool(oc *exutil.CLI, name string) *MachineConfigPool {
	return &MachineConfigPool{Resource: *NewResource(oc, "mcp", name), MinutesWaitingPerNode: DefaultMinutesWaitingPerNode}
}

// MachineConfigPoolList construct a new node list struct to handle all existing nodes
func NewMachineConfigPoolList(oc *exutil.CLI) *MachineConfigPoolList {
	return &MachineConfigPoolList{*NewResourceList(oc, "mcp")}
}

// String implements the Stringer interface

func (mcp *MachineConfigPool) create() {
	exutil.CreateClusterResourceFromTemplate(mcp.oc, "--ignore-unknown-parameters=true", "-f", mcp.template, "-p", "NAME="+mcp.name)
	mcp.waitForComplete()
}

func (mcp *MachineConfigPool) delete() {
	logger.Infof("deleting custom mcp: %s", mcp.name)
	err := mcp.oc.AsAdmin().WithoutNamespace().Run("delete").Args("mcp", mcp.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (mcp *MachineConfigPool) pause(enable bool) {
	logger.Infof("patch mcp %v, change spec.paused to %v", mcp.name, enable)
	err := mcp.Patch("merge", `{"spec":{"paused": `+strconv.FormatBool(enable)+`}}`)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// SetMaxUnavailable sets the value for maxUnavailable
func (mcp *MachineConfigPool) SetMaxUnavailable(maxUnavailable int) {
	logger.Infof("patch mcp %v, change spec.maxUnavailable to %d", mcp.name, maxUnavailable)
	err := mcp.Patch("merge", fmt.Sprintf(`{"spec":{"maxUnavailable": %d}}`, maxUnavailable))
	o.Expect(err).NotTo(o.HaveOccurred())
}

// RemoveMaxUnavailable removes spec.maxUnavailable attribute from the pool config
func (mcp *MachineConfigPool) RemoveMaxUnavailable() {
	logger.Infof("patch mcp %v, removing spec.maxUnavailable", mcp.name)
	err := mcp.Patch("json", `[{ "op": "remove", "path": "/spec/maxUnavailable" }]`)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (mcp *MachineConfigPool) getConfigNameOfSpec() (string, error) {
	output, err := mcp.Get(`{.spec.configuration.name}`)
	logger.Infof("spec.configuration.name of mcp/%v is %v", mcp.name, output)
	return output, err
}

func (mcp *MachineConfigPool) getConfigNameOfStatus() (string, error) {
	output, err := mcp.Get(`{.status.configuration.name}`)
	logger.Infof("status.configuration.name of mcp/%v is %v", mcp.name, output)
	return output, err
}

func (mcp *MachineConfigPool) getMachineCount() (int, error) {
	machineCountStr, ocErr := mcp.Get(`{.status.machineCount}`)
	if ocErr != nil {
		logger.Infof("Error getting machineCount: %s", ocErr)
		return -1, ocErr
	}

	if machineCountStr == "" {
		return -1, fmt.Errorf(".status.machineCount value is not already set in MCP %s", mcp.GetName())
	}

	machineCount, convErr := strconv.Atoi(machineCountStr)

	if convErr != nil {
		logger.Errorf("Error converting machineCount to integer: %s", ocErr)
		return -1, convErr
	}

	return machineCount, nil
}

func (mcp *MachineConfigPool) getDegradedMachineCount() (int, error) {
	dmachineCountStr, ocErr := mcp.Get(`{.status.degradedMachineCount}`)
	if ocErr != nil {
		logger.Errorf("Error getting degradedmachineCount: %s", ocErr)
		return -1, ocErr
	}
	dmachineCount, convErr := strconv.Atoi(dmachineCountStr)

	if convErr != nil {
		logger.Errorf("Error converting degradedmachineCount to integer: %s", ocErr)
		return -1, convErr
	}

	return dmachineCount, nil
}

func (mcp *MachineConfigPool) pollMachineCount() func() string {
	return mcp.Poll(`{.status.machineCount}`)
}

func (mcp *MachineConfigPool) pollReadyMachineCount() func() string {
	return mcp.Poll(`{.status.readyMachineCount}`)
}

func (mcp *MachineConfigPool) pollDegradedMachineCount() func() string {
	return mcp.Poll(`{.status.degradedMachineCount}`)
}

// GetDegradedStatus returns the value of the 'Degraded' condition in the MCP
func (mcp *MachineConfigPool) GetDegradedStatus() (string, error) {
	return mcp.Get(`{.status.conditions[?(@.type=="Degraded")].status}`)
}

func (mcp *MachineConfigPool) pollDegradedStatus() func() string {
	return mcp.Poll(`{.status.conditions[?(@.type=="Degraded")].status}`)
}

// GetUpdatedStatus returns the value of the 'Updated' condition in the MCP
func (mcp *MachineConfigPool) GetUpdatedStatus() (string, error) {
	return mcp.Get(`{.status.conditions[?(@.type=="Updated")].status}`)
}

func (mcp *MachineConfigPool) pollUpdatedStatus() func() string {
	return mcp.Poll(`{.status.conditions[?(@.type=="Updated")].status}`)
}

func (mcp *MachineConfigPool) estimateWaitTimeInMinutes() int {
	var totalNodes int

	o.Eventually(func() int {
		var err error
		totalNodes, err = mcp.getMachineCount()
		if err != nil {
			return -1
		}
		return totalNodes
	},
		"5m", "5s").Should(o.BeNumerically(">=", 0), fmt.Sprintf("machineCount field has no value in MCP %s", mcp.name))

	// If the pool has no node configured, we wait at least 1 minute.
	// There are tests that create pools with 0 nodes and wait for the pool to be updated. They cant wait 0 minutes.
	if totalNodes == 0 {
		return 1
	}

	return totalNodes * mcp.MinutesWaitingPerNode
}

// SetWaitingTimeForRTKernel increases the time that the MCP will wait for the update to be executed
func (mcp *MachineConfigPool) SetWaitingTimeForRTKernel() {
	mcp.MinutesWaitingPerNode = DefaultMinutesWaitingPerNode + RTKernelIncWait
}

// SetDefaultWaitingTime restore the default waiting time that the MCP will wait for the update to be executed
func (mcp *MachineConfigPool) SetDefaultWaitingTime() {
	mcp.MinutesWaitingPerNode = DefaultMinutesWaitingPerNode
}

// getNodesWithLabels returns a list with the nodes that belong to the machine config pool and has the provided labels
func (mcp *MachineConfigPool) getNodesWithLabels(extraLabels string) ([]Node, error) {
	mcp.oc.NotShowInfo()
	defer mcp.oc.SetShowInfo()

	labelsString, err := mcp.Get(`{.spec.nodeSelector.matchLabels}`)
	if err != nil {
		return nil, err
	}
	labels := JSON(labelsString)
	o.Expect(labels.Exists()).Should(o.BeTrue(), fmt.Sprintf("The pool %s has no machLabels value defined", mcp.GetName()))

	nodeList := NewNodeList(mcp.oc)
	// Never select windows nodes
	requiredLabel := "kubernetes.io/os!=windows"
	if extraLabels != "" {
		requiredLabel += ","
		requiredLabel += extraLabels
	}
	for k, v := range labels.ToMap() {
		requiredLabel += fmt.Sprintf(",%s=%s", k, v.(string))
	}
	nodeList.ByLabel(requiredLabel)

	return nodeList.GetAll()
}

// GetNodes returns a list with the nodes that belong to the machine config pool
func (mcp *MachineConfigPool) GetNodes() ([]Node, error) {
	mcp.oc.NotShowInfo()
	defer mcp.oc.SetShowInfo()

	// A node can belong to several pools
	// In the case of "worker" pool, if a node belongs to both "worker" and "master" pool
	// then the node is considered to belong to "master" node and not to "worker" pool.
	if mcp.GetName() == MachineConfigPoolWorker {
		nodes, err := mcp.getNodesWithLabels("")
		if err != nil {
			return nil, err
		}
		nodesNotMaster := []Node{}

		masterPool := NewMachineConfigPool(mcp.oc, MachineConfigPoolMaster)
		for _, node := range nodes {
			nodeIsMaster, err := node.IsInPool(masterPool)
			if err != nil {
				return nil, err
			}
			if !nodeIsMaster {
				nodesNotMaster = append(nodesNotMaster, node)
			}
		}

		return nodesNotMaster, nil
	}
	return mcp.getNodesWithLabels("")
}

// GetNodesOrFail returns a list with the nodes that belong to the machine config pool and fail the test if any error happened
func (mcp *MachineConfigPool) GetNodesOrFail() []Node {
	ns, err := mcp.GetNodes()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Cannot get the nodes in %s MCP", mcp.GetName())
	return ns
}

// GetCoreOsNodes returns a list with the CoreOs nodes that belong to the machine config pool
func (mcp *MachineConfigPool) GetCoreOsNodes() ([]Node, error) {
	return mcp.getNodesWithLabels("node.openshift.io/os_id=rhcos")
}

// GetCoreOsNodesOrFail returns a list with the nodes that belong to the machine config pool and fail the test if any error happened
func (mcp *MachineConfigPool) GetCoreOsNodesOrFail() []Node {
	ns, err := mcp.GetCoreOsNodes()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Cannot get the coreOS nodes in %s MCP", mcp.GetName())
	return ns
}

// GetSortedNodes returns a list with the nodes that belong to the machine config pool in the same order used to update them
// when a configuration is applied
func (mcp *MachineConfigPool) GetSortedNodes() ([]Node, error) {

	poolNodes, err := mcp.GetNodes()
	if err != nil {
		return nil, err
	}

	return sortNodeList(poolNodes), nil

}

// GetSortedNodesOrFail returns a list with the nodes that belong to the machine config pool in the same order used to update them
// when a configuration is applied. If any error happens while getting the list, then the test is failed.
func (mcp *MachineConfigPool) GetSortedNodesOrFail() []Node {
	nodes, err := mcp.GetSortedNodes()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(),
		"Cannot get the list of nodes that belong to '%s' MCP", mcp.GetName())

	return nodes
}

// GetSortedUpdatedNodes returns the list of the UpdatedNodes sorted by the time when they started to be updated.
// If maxUnavailable>0, then the function will fail if more that maxUpdatingNodes are being updated at the same time
func (mcp *MachineConfigPool) GetSortedUpdatedNodes(maxUnavailable int) []Node {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s in pool %s for all nodes to start updating.", timeToWait, mcp.name)

	poolNodes, errget := mcp.GetNodes()
	o.Expect(errget).NotTo(o.HaveOccurred(), fmt.Sprintf("Cannot get nodes in pool %s", mcp.GetName()))

	pendingNodes := poolNodes
	updatedNodes := []Node{}
	immediate := false
	err := wait.PollUntilContextTimeout(context.TODO(), 20*time.Second, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		// If there are degraded machines, stop polling, directly fail
		degradedstdout, degradederr := mcp.getDegradedMachineCount()
		if degradederr != nil {
			logger.Errorf("the err:%v, and try next round", degradederr)
			return false, nil
		}

		if degradedstdout != 0 {
			logger.Errorf("Degraded MC:\n%s", mcp.PrettyString())
			exutil.AssertWaitPollNoErr(fmt.Errorf("Degraded machines"), fmt.Sprintf("mcp %s has degraded %d machines", mcp.name, degradedstdout))
		}

		// Check that there aren't more thatn maxUpdatingNodes updating at the same time
		if maxUnavailable > 0 {
			totalUpdating := 0
			for _, node := range poolNodes {
				if node.IsUpdating() {
					totalUpdating++
				}
			}
			if totalUpdating > maxUnavailable {
				exutil.AssertWaitPollNoErr(fmt.Errorf("maxUnavailable Not Honored"), fmt.Sprintf("Pool %s, error: %d nodes were updating at the same time. Only %d nodes should be updating at the same time.", mcp.GetName(), totalUpdating, maxUnavailable))
			}
		}

		remainingNodes := []Node{}
		for _, node := range pendingNodes {
			if node.IsUpdating() {
				logger.Infof("Node %s is UPDATING", node.GetName())
				updatedNodes = append(updatedNodes, node)
			} else {
				remainingNodes = append(remainingNodes, node)
			}
		}

		if len(remainingNodes) == 0 {
			logger.Infof("All nodes have started to be updated on mcp %s", mcp.name)
			return true, nil

		}
		logger.Infof(" %d remaining nodes", len(remainingNodes))
		pendingNodes = remainingNodes
		return false, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Could not get the list of updated nodes on mcp %s", mcp.name))
	return updatedNodes
}

// WaitForNotDegradedStatus waits until MCP is not degraded, if the condition times out the returned error is != nil
func (mcp MachineConfigPool) WaitForNotDegradedStatus() error {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s for MCP %s status to be not degraded.", timeToWait, mcp.name)

	immediate := false
	err := wait.PollUntilContextTimeout(context.TODO(), 1*time.Minute, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		stdout, err := mcp.GetDegradedStatus()
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, "False") {
			logger.Infof("MCP degraded status is False %s", mcp.name)
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		logger.Errorf("MCP: %s .Error waiting for not degraded status: %s", mcp.GetName(), err)
	}

	return err
}

// WaitForUpdatedStatus waits until MCP is rerpoting updated status, if the condition times out the returned error is != nil
func (mcp MachineConfigPool) WaitForUpdatedStatus() error {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s for MCP %s status to be updated.", timeToWait, mcp.name)

	immediate := false
	err := wait.PollUntilContextTimeout(context.TODO(), 1*time.Minute, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		stdout, err := mcp.GetUpdatedStatus()
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, "True") {
			logger.Infof("MCP Updated status is True %s", mcp.name)
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		logger.Errorf("MCP: %s .Error waiting for updated status: %s", mcp.GetName(), err)
	}

	return err
}

func (mcp *MachineConfigPool) waitForComplete() {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s for MCP %s to be completed.", timeToWait, mcp.name)

	immediate := false
	err := wait.PollUntilContextTimeout(context.TODO(), 1*time.Minute, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		// If there are degraded machines, stop polling, directly fail
		degradedstdout, degradederr := mcp.getDegradedMachineCount()
		if degradederr != nil {
			logger.Errorf("the err:%v, and try next round", degradederr)
			return false, nil
		}

		if degradedstdout != 0 {
			logger.Errorf("Degraded MC:\n%s", mcp.PrettyString())
			exutil.AssertWaitPollNoErr(fmt.Errorf("Degraded machines"), fmt.Sprintf("mcp %s has degraded %d machines", mcp.name, degradedstdout))
		}

		stdout, err := mcp.Get(`{.status.conditions[?(@.type=="Updated")].status}`)
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, "True") {
			// i.e. mcp updated=true, mc is applied successfully
			logger.Infof("The new MC has been successfully applied to MCP '%s'", mcp.name)
			return true, nil
		}
		return false, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("mc operation is not completed on mcp %s", mcp.name))
}

// GetReportedOsImageOverrideValue returns the value of the os_image_url_override prometheus metric for this pool
func (mcp *MachineConfigPool) GetReportedOsImageOverrideValue() (string, error) {
	query := fmt.Sprintf(`os_image_url_override{pool="%s"}`, strings.ToLower(mcp.GetName()))

	mon, err := exutil.NewMonitor(mcp.oc.AsAdmin())
	if err != nil {
		return "", err
	}

	osImageOverride, err := mon.SimpleQuery(query)
	if err != nil {
		return "", err
	}

	jsonOsImageOverride := JSON(osImageOverride)
	status := jsonOsImageOverride.Get("status").ToString()
	if status != "success" {
		return "", fmt.Errorf("Query %s execution failed: %s", query, osImageOverride)
	}

	logger.Infof("%s metric is:%s", query, osImageOverride)

	metricValue := JSON(osImageOverride).Get("data").Get("result").Item(0).Get("value").Item(1).ToString()
	return metricValue, nil
}

// RecoverFromDegraded updates the current and desired machine configs so that the pool can recover from degraded state once the offending MC is deleted
func (mcp *MachineConfigPool) RecoverFromDegraded() error {
	logger.Infof("Recovering %s pool from degraded status", mcp.GetName())
	mcpNodes, _ := mcp.GetNodes()
	for _, node := range mcpNodes {
		logger.Infof("Restoring desired config in node: %s", node)
		err := node.RestoreDesiredConfig()
		if err != nil {
			return fmt.Errorf("Error restoring desired config in node %s. Error: %s",
				mcp.GetName(), err)
		}
	}

	derr := mcp.WaitForNotDegradedStatus()
	if derr != nil {
		logger.Infof("Could not recover from the degraded status: %s", derr)
		return derr
	}

	uerr := mcp.WaitForUpdatedStatus()
	if uerr != nil {
		logger.Infof("Could not recover from the degraded status: %s", uerr)
		return uerr
	}

	return nil
}

// IsRealTimeKernel returns true if the pool is using a realtime kernel
func (mcp *MachineConfigPool) IsRealTimeKernel() (bool, error) {
	nodes, err := mcp.GetNodes()
	if err != nil {
		logger.Errorf("Error getting the nodes in pool %s", mcp.GetName())
		return false, err
	}

	return nodes[0].IsRealTimeKernel()
}

// GetConfiguredMachineConfig return the MachineConfig currently configured in the pool
func (mcp *MachineConfigPool) GetConfiguredMachineConfig() (*MachineConfig, error) {
	currentMcName, err := mcp.Get("{.status.configuration.name}")
	if err != nil {
		logger.Errorf("Error getting the currently configured MC in pool %s: %s", mcp.GetName(), err)
		return nil, err
	}

	logger.Debugf("The currently configured MC in pool %s is: %s", mcp.GetName(), currentMcName)
	return NewMachineConfig(mcp.oc, currentMcName, mcp.GetName()), nil
}

// SanityCheck returns an error if the MCP is Degraded or Updating.
// We can't use WaitForUpdatedStatus or WaitForNotDegradedStatus because they always wait the interval. In a sanity check we want a fast response.
func (mcp *MachineConfigPool) SanityCheck() error {
	timeToWait := (time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute) / 13
	logger.Infof("Waiting %s for MCP %s to be completed.", timeToWait.Round(time.Second), mcp.name)

	const trueStatus = "True"
	var message string

	immediate := true
	err := wait.PollUntilContextTimeout(context.TODO(), 1*time.Minute, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		// If there are degraded machines, stop polling, directly fail
		degraded, degradederr := mcp.GetDegradedStatus()
		if degradederr != nil {
			message = fmt.Sprintf("Error gettting Degraded status: %s", degradederr)
			return false, nil
		}

		if degraded == trueStatus {
			message = fmt.Sprintf("MCP '%s' is degraded", mcp.GetName())
			return false, nil
		}

		updated, err := mcp.GetUpdatedStatus()
		if err != nil {
			message = fmt.Sprintf("Error gettting Updated status: %s", err)
			return false, nil
		}
		if updated == trueStatus {
			logger.Infof("MCP '%s' is ready for testing", mcp.name)
			return true, nil
		}
		message = fmt.Sprintf("MCP '%s' is not updated", mcp.GetName())
		return false, nil
	})

	if err != nil {
		return fmt.Errorf(message)
	}

	return nil
}

// GetAll returns a []MachineConfigPool list with all existing machine config pools sorted by creation time
func (mcpl *MachineConfigPoolList) GetAll() ([]MachineConfigPool, error) {
	mcpl.ResourceList.SortByTimestamp()
	allMCPResources, err := mcpl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allMCPs := make([]MachineConfigPool, 0, len(allMCPResources))

	for _, mcpRes := range allMCPResources {
		allMCPs = append(allMCPs, *NewMachineConfigPool(mcpl.oc, mcpRes.name))
	}

	return allMCPs, nil
}

// GetCompactCompatiblePool returns worker pool if the cluster is not compact/SNO. Else it will return master pool.
func GetCompactCompatiblePool(oc *exutil.CLI) *MachineConfigPool {
	var (
		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
	)
	if len(wMcp.GetNodesOrFail()) == 0 {
		logger.Infof("Running in SNO/Compact cluster. Using master pool for testing")
		return mMcp
	}

	return wMcp
}

// GetCoreOsCompatiblePool returns worker pool if it has CoreOs nodes. If there is no CoreOs node in the worker pool, then it returns master pool.
func GetCoreOsCompatiblePool(oc *exutil.CLI) *MachineConfigPool {
	var (
		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
	)
	if len(wMcp.GetCoreOsNodesOrFail()) == 0 {
		logger.Infof("No CoreOs nodes in the worker pool. Using master pool for testing")
		return mMcp
	}

	return wMcp
}
