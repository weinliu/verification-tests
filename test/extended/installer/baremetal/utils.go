package baremetal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	machineAPINamespace              = "openshift-machine-api"
	maxCpuUsageAllowed       float64 = 90.0
	minRequiredMemoryInBytes         = 1000000000
)

type Response struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				Instance string `json:"instance"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func checkOperatorsRunning(oc *exutil.CLI) (bool, error) {
	jpath := `{range .items[*]}{.metadata.name}:{.status.conditions[?(@.type=='Available')].status}{':'}{.status.conditions[?(@.type=='Degraded')].status}{'\n'}{end}`
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperators.config.openshift.io", "-o", "jsonpath="+jpath).Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute 'oc get clusteroperators.config.openshift.io' command: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		e2e.Logf("%s", line)
		parts := strings.Split(line, ":")
		available := parts[1] == "True"
		degraded := parts[2] == "False"

		if !available || !degraded {
			return false, nil
		}
	}

	return true, nil
}

func checkNodesRunning(oc *exutil.CLI) (bool, error) {
	nodeNames, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-o=jsonpath={.items[*].metadata.name}").Output()
	if nodeErr != nil {
		return false, fmt.Errorf("failed to execute 'oc get nodes' command: %v", nodeErr)
	}
	nodes := strings.Fields(nodeNames)
	e2e.Logf("\nNode Names are %v", nodeNames)
	for _, node := range nodes {
		nodeStatus, statusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node, "-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}").Output()
		if statusErr != nil {
			return false, fmt.Errorf("failed to execute 'oc get nodes' command: %v", statusErr)
		}
		e2e.Logf("\nNode %s Status is %s\n", node, nodeStatus)

		if nodeStatus != "True" {
			return false, nil
		}
	}
	return true, nil
}

func waitForDeployStatus(oc *exutil.CLI, depName string, nameSpace string, depStatus string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		statusOp, err := oc.AsAdmin().Run("get").Args("-n", nameSpace, "deployment", depName, "-o=jsonpath={.status.conditions[?(@.type=='Available')].status}'").Output()
		if err != nil {
			return false, err
		}

		if strings.Contains(statusOp, depStatus) {
			e2e.Logf("Deployment %v state is %v", depName, depStatus)
			return true, nil
		}
		e2e.Logf("deployment %v is state %v, Trying again", depName, statusOp)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The test deployment job is not running")
}

func getPodName(oc *exutil.CLI, ns string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nPod Name is %v", podName)
	return podName
}

func getPodStatus(oc *exutil.CLI, namespace string, podName string) string {
	podStatus, err := oc.AsAdmin().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s status is %q", podName, podStatus)
	return podStatus
}

func getNodeCpuUsage(oc *exutil.CLI, node string, sampling_time int) float64 {
	samplingTime := strconv.Itoa(sampling_time)

	cpu_sampling := "node_cpu_seconds_total%20%7Binstance%3D%27" + node
	cpu_sampling += "%27%2C%20mode%3D%27idle%27%7D%5B5" + samplingTime + "m%5D"
	query := "query=100%20-%20(avg%20by%20(instance)(irate(" + cpu_sampling + "))%20*%20100)"
	url := "http://localhost:9090/api/v1/query?" + query

	jsonString, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-s", url).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	var response Response
	unmarshalErr := json.Unmarshal([]byte(jsonString), &response)
	o.Expect(unmarshalErr).NotTo(o.HaveOccurred())
	cpuUsage := response.Data.Result[0].Value[1].(string)
	cpu_usage, err := strconv.ParseFloat(cpuUsage, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	return cpu_usage
}

func getClusterUptime(oc *exutil.CLI) (int, error) {
	layout := "2006-01-02T15:04:05Z"
	completionTime, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].status.history[*].completionTime}").Output()
	returnTime, perr := time.Parse(layout, completionTime)
	if perr != nil {
		e2e.Logf("Error trying to parse uptime %s", perr)
		return 0, perr
	}
	now := time.Now()
	uptime := now.Sub(returnTime)
	uptimeByMin := int(uptime.Minutes())
	return uptimeByMin, nil
}

func getNodeavailMem(oc *exutil.CLI, node string) int {
	query := "query=node_memory_MemAvailable_bytes%7Binstance%3D%27" + node + "%27%7D"
	url := "http://localhost:9090/api/v1/query?" + query

	jsonString, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-s", url).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	var response Response
	unmarshalErr := json.Unmarshal([]byte(jsonString), &response)
	o.Expect(unmarshalErr).NotTo(o.HaveOccurred())
	memUsage := response.Data.Result[0].Value[1].(string)
	availableMem, err := strconv.Atoi(memUsage)
	o.Expect(err).NotTo(o.HaveOccurred())
	return availableMem
}

// make sure operator is not processing and degraded
func checkOperator(oc *exutil.CLI, operatorName string) (bool, error) {
	output, err := oc.AsAdmin().Run("get").Args("clusteroperator", operatorName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if matched, _ := regexp.MatchString("True.*False.*False", output); !matched {
		e2e.Logf("clusteroperator %s is abnormal\n", operatorName)
		return false, nil
	}
	return true, nil
}

func waitForPodNotFound(oc *exutil.CLI, podName string, nameSpace string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		out, err := oc.AsAdmin().Run("get").Args("-n", nameSpace, "pods", "-o=jsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			return false, err
		}
		if !strings.Contains(out, podName) {
			e2e.Logf("Pod %v still exists is", podName)
			return true, nil
		}
		e2e.Logf("Pod %v exists, Trying again", podName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The test deployment job is running")
}

func getUserFromSecret(oc *exutil.CLI, namespace string, secretName string) string {
	userbase64, pwderr := oc.AsAdmin().Run("get").Args("secrets", "-n", machineAPINamespace, secretName, "-o=jsonpath={.data.username}").Output()
	o.Expect(pwderr).ShouldNot(o.HaveOccurred())
	user, err := base64.StdEncoding.DecodeString(userbase64)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return string(user)
}

func getPassFromSecret(oc *exutil.CLI, namespace string, secretName string) string {
	pwdbase64, pwderr := oc.AsAdmin().Run("get").Args("secrets", "-n", machineAPINamespace, secretName, "-o=jsonpath={.data.password}").Output()
	o.Expect(pwderr).ShouldNot(o.HaveOccurred())
	pwd, err := base64.StdEncoding.DecodeString(pwdbase64)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return string(pwd)
}

func CopyToFile(fromPath string, toFilename string) string {
	// check if source file is regular file
	srcFileStat, err := os.Stat(fromPath)
	if err != nil {
		e2e.Failf("get source file %s stat failed: %v", fromPath, err)
	}
	if !srcFileStat.Mode().IsRegular() {
		e2e.Failf("source file %s is not a regular file", fromPath)
	}

	// open source file
	source, err := os.Open(fromPath)
	defer source.Close()
	if err != nil {
		e2e.Failf("open source file %s failed: %v", fromPath, err)
	}

	// open dest file
	saveTo := filepath.Join(e2e.TestContext.OutputDir, toFilename)
	dest, err := os.Create(saveTo)
	defer dest.Close()
	if err != nil {
		e2e.Failf("open destination file %s failed: %v", saveTo, err)
	}

	// copy from source to dest
	_, err = io.Copy(dest, source)
	if err != nil {
		e2e.Failf("copy file from %s to %s failed: %v", fromPath, saveTo, err)
	}
	return saveTo
}

func waitForBMHState(oc *exutil.CLI, bmhName string, bmhStatus string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 30*time.Minute, true, func(context.Context) (bool, error) {
		statusOp, err := oc.AsAdmin().Run("get").Args("-n", machineAPINamespace, "bmh", bmhName, "-o=jsonpath={.status.provisioning.state}").Output()
		if err != nil {
			return false, err
		}
		if strings.Contains(statusOp, bmhStatus) {
			e2e.Logf("BMH state %v is %v", bmhName, bmhStatus)
			return true, nil
		}
		e2e.Logf("BMH %v state is %v, Trying again", bmhName, statusOp)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The BMH state of is not as expected")
}

func waitForBMHDeletion(oc *exutil.CLI, bmhName string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 30*time.Minute, true, func(ctx context.Context) (bool, error) {
		out, err := oc.AsAdmin().Run("get").Args("-n", machineAPINamespace, "bmh", "-o=jsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			return false, err
		}
		if !strings.Contains(out, bmhName) {
			e2e.Logf("bmh %v still exists is", bmhName)
			return true, nil
		}
		e2e.Logf("bmh %v exists, Trying again", bmhName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The BMH was not deleted as expected")
}

func getBypathDeviceName(oc *exutil.CLI, bmhName string) string {
	byPath, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", machineAPINamespace, bmhName, "-o=jsonpath={.status.hardware.storage[0].name}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return byPath
}

// clusterOperatorHealthcheck check abnormal operators
func clusterOperatorHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal operators")
	errCo := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		coLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "--no-headers").OutputToFile(dirname)
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -v '.True.*False.*False' || true`, coLogFile)
			coLogs, err := exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(coLogs) > 0 {
				return false, nil
			}
		} else {
			return false, nil
		}
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("No abnormality found in cluster operators...")
		return true, nil
	})
	if errCo != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errCo
}

// clusterNodesHealthcheck check abnormal nodes
func clusterNodesHealthcheck(oc *exutil.CLI, waitTime int) error {
	errNode := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
		if err == nil {
			if strings.Contains(output, "NotReady") || strings.Contains(output, "SchedulingDisabled") {
				return false, nil
			}
		} else {
			return false, nil
		}
		e2e.Logf("Nodes are normal...")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		return true, nil
	})
	if errNode != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errNode
}

// checkNodeStatus
func checkNodeStatus(oc *exutil.CLI, pollIntervalSec time.Duration, pollDurationMinute time.Duration, nodeName string, nodeStatus string) error {
	e2e.Logf("Check status of node %s", nodeName)
	errNode := wait.PollUntilContextTimeout(context.Background(), pollIntervalSec, pollDurationMinute, false, func(ctx context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", nodeName, "-o=jsonpath={.status.conditions[3].status}").Output()
		if err != nil || string(output) != nodeStatus {
			e2e.Logf("Node status: %s. Trying again", output)
			return false, nil
		}
		if string(output) == nodeStatus {
			e2e.Logf("Node status: %s", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errNode, "Node did not change state as expected")
	return errNode
}

func buildFirmwareURL(vendor, currentVersion string) (string, string) {
	var url, fileName string

	iDRAC_71070 := "https://dl.dell.com/FOLDER11965413M/1/iDRAC_7.10.70.00_A00.exe"
	iDRAC_71030 := "https://dl.dell.com/FOLDER11319105M/1/iDRAC_7.10.30.00_A00.exe"
	ilo5_305 := "https://downloads.hpe.com/pub/softlib2/software1/fwpkg-ilo/p991377599/v247527/ilo5_305.fwpkg"
	ilo5_302 := "https://downloads.hpe.com/pub/softlib2/software1/fwpkg-ilo/p991377599/v243854/ilo5_302.fwpkg"
	ilo6_157 := "https://downloads.hpe.com/pub/softlib2/software1/fwpkg-ilo/p788720876/v247531/ilo6_160.fwpkg"
	ilo6_160 := "https://downloads.hpe.com/pub/softlib2/software1/fwpkg-ilo/p788720876/v243858/ilo6_157.fwpkg"

	switch vendor {
	case "Dell Inc.":
		fileName = "firmimgFIT.d9"
		if currentVersion == "7.10.70.00" {
			url = iDRAC_71030
		} else if currentVersion == "7.10.30.00" {
			url = iDRAC_71070
		} else {
			url = iDRAC_71070 // Default to 7.10.70.00
		}
	case "HPE":
		// Extract the iLO version and assign the file name accordingly
		if strings.Contains(currentVersion, "iLO 5") {
			if currentVersion == "iLO 5 v3.05" {
				url = ilo5_302
				fileName = "ilo5_302.bin"
			} else if currentVersion == "iLO 5 v3.02" {
				url = ilo5_305
				fileName = "ilo5_305.bin"
			} else {
				url = ilo5_305 // Default to v3.05
				fileName = "ilo5_305.bin"
			}
		} else if strings.Contains(currentVersion, "iLO 6") {
			if currentVersion == "iLO 6 v1.57" {
				url = ilo6_160
				fileName = "ilo6_160.bin"
			} else if currentVersion == "iLO 6 v1.60" {
				url = ilo6_157
				fileName = "ilo6_157.bin"
			} else {
				url = ilo6_157 // Default to 1.57
				fileName = "ilo6_157.bin"
			}
		} else {
			g.Skip("Unsupported HPE BMC version")
		}
	default:
		g.Skip("Unsupported vendor")
	}

	return url, fileName
}
