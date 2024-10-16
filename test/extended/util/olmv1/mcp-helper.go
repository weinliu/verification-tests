package olmv1util

import (
	"context"
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

func AssertMCPCondition(oc *exutil.CLI, name, conditionType, field, expect string, checkInterval, checkTimeout, consistentTime int) {
	e2e.Logf("========= assert mcp %v %s %s expect is %s =========", name, conditionType, field, expect)
	err := CheckMCPCondition(oc, name, conditionType, field, expect, checkInterval, checkTimeout)
	o.Expect(err).NotTo(o.HaveOccurred())
	if consistentTime != 0 {
		e2e.Logf("make sure mcp %s expect is %s consistently for %ds", conditionType, expect, consistentTime)
		jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].%s}`, conditionType, field)
		o.Consistently(func() string {
			output, _ := GetNoEmpty(oc, "mcp", name, "-o", jsonpath)
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(expect)),
			"mcp %s expected is not %s", conditionType, expect)
	}
}

func CheckMCPCondition(oc *exutil.CLI, name, conditionType, field, expect string, checkInterval, checkTimeout int) error {
	e2e.Logf("========= check mcp %v %s %s expect is %s =========", name, conditionType, field, expect)
	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].%s}`, conditionType, field)
	errWait := wait.PollUntilContextTimeout(context.TODO(), time.Duration(checkInterval)*time.Second, time.Duration(checkTimeout)*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := GetNoEmpty(oc, "mcp", name, "-o", jsonpath)
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", output, err)
			return false, nil
		}
		if !strings.Contains(strings.ToLower(output), strings.ToLower(expect)) {
			e2e.Logf("got is %v, not %v, and try next", output, expect)
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		GetNoEmpty(oc, "mcp", name, "-o=jsonpath-as-json={.status}")
		errWait = fmt.Errorf("error happen: %v\n mcp %s expected is not %s in %v seconds", errWait, conditionType, expect, checkTimeout)
	}
	return errWait
}

func HealthyMCP4OLM(oc *exutil.CLI) bool {
	return HealthyMCP4Module(oc, "OLM")
}

func HealthyMCP4Module(oc *exutil.CLI, module string) bool {
	output, err := GetNoEmpty(oc, "mcp", "-ojsonpath={.items..metadata.name}")
	if err != nil {
		e2e.Logf("output is %v, error is %v, and try next", output, err)
		return false
	}
	// if your moudle has specific checking or not same checking with OLM. you could add your module branch
	// and please keep OLM logic
	if module == "OLM" {
		mcpNames := strings.Fields(output)
		// check if there is 2
		if len(mcpNames) > 2 {
			e2e.Logf("there is unexpect mcp: %v", mcpNames)
			return false
		}
		for _, name := range mcpNames {
			if name != "worker" && name != "master" {
				e2e.Logf("there is mcp %v which is not expected", name)
				return false
			}
		}

		workerStatus, err := GetMCPStatus(oc, "worker")
		if err != nil {
			e2e.Logf("error is %v", err)
			return false
		}
		if !(strings.Contains(workerStatus.UpdatingStatus, "False") &&
			// strings.Contains(workerStatus.UpdatedStatus, "True") &&
			strings.Compare(workerStatus.MachineCount, workerStatus.ReadyMachineCount) == 0 &&
			strings.Compare(workerStatus.UnavailableMachineCount, workerStatus.DegradedMachineCount) == 0 &&
			strings.Compare(workerStatus.DegradedMachineCount, "0") == 0) {
			e2e.Logf("mcp worker's status is not correct: %v", workerStatus)
			return false
		}
		masterStatus, err := GetMCPStatus(oc, "master")
		if err != nil {
			e2e.Logf("error is %v", err)
			return false
		}
		if !(strings.Contains(masterStatus.UpdatingStatus, "False") &&
			// strings.Contains(masterStatus.UpdatedStatus, "True") &&
			strings.Compare(masterStatus.MachineCount, masterStatus.ReadyMachineCount) == 0 &&
			strings.Compare(masterStatus.UnavailableMachineCount, masterStatus.DegradedMachineCount) == 0 &&
			strings.Compare(masterStatus.DegradedMachineCount, "0") == 0) {
			e2e.Logf("mcp master's status is not correct:%v", masterStatus)
			return false
		}
	}

	return true
}

type McpStatus struct {
	MachineCount            string
	ReadyMachineCount       string
	UnavailableMachineCount string
	DegradedMachineCount    string
	UpdatingStatus          string
	UpdatedStatus           string
}

func GetMCPStatus(oc *exutil.CLI, name string) (McpStatus, error) {
	updatingStatus, err := GetNoEmpty(oc, "mcp", name, `-ojsonpath='{.status.conditions[?(@.type=="Updating")].status}'`)
	if err != nil {
		return McpStatus{}, err
	}
	updatedStatus, err := GetNoEmpty(oc, "mcp", name, `-ojsonpath='{.status.conditions[?(@.type=="Updated")].status}'`)
	if err != nil {
		return McpStatus{}, err
	}
	machineCount, err := GetNoEmpty(oc, "mcp", name, "-o=jsonpath={..status.machineCount}")
	if err != nil {
		return McpStatus{}, err
	}
	readyMachineCount, err := GetNoEmpty(oc, "mcp", name, "-o=jsonpath={..status.readyMachineCount}")
	if err != nil {
		return McpStatus{}, err
	}
	unavailableMachineCount, err := GetNoEmpty(oc, "mcp", name, "-o=jsonpath={..status.unavailableMachineCount}")
	if err != nil {
		return McpStatus{}, err
	}
	degradedMachineCount, err := GetNoEmpty(oc, "mcp", name, "-o=jsonpath={..status.degradedMachineCount}")
	if err != nil {
		return McpStatus{}, err
	}
	return McpStatus{
		MachineCount:            machineCount,
		ReadyMachineCount:       readyMachineCount,
		UnavailableMachineCount: unavailableMachineCount,
		DegradedMachineCount:    degradedMachineCount,
		UpdatingStatus:          updatingStatus,
		UpdatedStatus:           updatedStatus,
	}, nil
}
