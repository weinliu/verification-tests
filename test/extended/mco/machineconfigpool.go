package mco

import (
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
}

// NewMachineConfigPool create a NewMachineConfigPool struct
func NewMachineConfigPool(oc *exutil.CLI, name string) *MachineConfigPool {
	return &MachineConfigPool{Resource: *NewResource(oc, "mcp", name)}
}

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
		"5m").Should(o.BeNumerically(">=", 0), fmt.Sprintf("machineCount field has no value in MCP %s", mcp.name))

	return totalNodes * 10

}

// GetNodes returns a list with the nodes that belong to the machine config pool
func (mcp *MachineConfigPool) GetNodes() ([]Node, error) {
	labels := JSON(mcp.GetOrFail(`{.spec.nodeSelector.matchLabels}`))
	o.Expect(labels.Exists()).Should(o.BeTrue(), fmt.Sprintf("The pool %s has no machLabels value defined", mcp.GetName()))

	nodeList := NewNodeList(mcp.oc)
	// Never select windows nodes
	requiredLabel := "kubernetes.io/os!=windows"
	for k, v := range labels.ToMap() {
		requiredLabel += fmt.Sprintf(",%s=%s", k, v.(string))
	}
	nodeList.ByLabel(requiredLabel)

	return nodeList.GetAll()
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

// GetSortedUpdatedNodes returns the list of the UpdatedNodes sorted by the time when they started to be updated.
// If maxUnavailable>0, then the function will fail if more that maxUpdatingNodes are being updated at the same time
func (mcp *MachineConfigPool) GetSortedUpdatedNodes(maxUnavailable int) []Node {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s in pool %s for all nodes to start updating.", timeToWait, mcp.name)

	poolNodes, errget := mcp.GetNodes()
	o.Expect(errget).NotTo(o.HaveOccurred(), fmt.Sprintf("Cannot get nodes in pool %s", mcp.GetName()))

	pendingNodes := poolNodes
	updatedNodes := []Node{}
	err := wait.Poll(20*time.Second, timeToWait, func() (bool, error) {
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

	err := wait.Poll(1*time.Minute, timeToWait, func() (bool, error) {
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

	err := wait.Poll(1*time.Minute, timeToWait, func() (bool, error) {
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

	err := wait.Poll(1*time.Minute, timeToWait, func() (bool, error) {
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
