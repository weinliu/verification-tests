package etcd

import (
	o "github.com/onsi/gomega"

	"fmt"
	"math/rand"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"encoding/json"
	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// PrometheusQueryResult is struct
type PrometheusQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				To        string `json:"To"`
				Endpoint  string `json:"endpoint"`
				Instance  string `json:"instance"`
				Job       string `json:"job"`
				Namespace string `json:"namespace"`
				Pod       string `json:"pod"`
				Service   string `json:"service"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

func getNodeListByLabel(oc *exutil.CLI, labelKey string) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", labelKey, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeNameList := strings.Fields(output)
	return nodeNameList
}

func getPodListByLabel(oc *exutil.CLI, labelKey string) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-etcd", "-l", labelKey, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podNameList := strings.Fields(output)
	return podNameList
}

func getIpOfMasterNode(oc *exutil.CLI, labelKey string) string {
	ipOfNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", labelKey, "-o=jsonpath={.items[0].status.addresses[0].address}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return ipOfNode
}

func verifyImageIDInDebugNode(oc *exutil.CLI, nodeNameList []string, imageID string, cVersion string) bool {
	found := 0
	for _, node := range nodeNameList {
		resultOutput, err := exutil.DebugNodeWithChroot(oc, node, "oc", "adm", "release", "info", "--registry-config=/var/lib/kubelet/config.json", cVersion, "--image-for=etcd")
		if strings.Contains(resultOutput, imageID) && err == nil {
			e2e.Logf("Image %v successfully deployed on node %v", imageID, node)
			found += 1
		} else {
			e2e.Logf("Failed to deploy image %v on node %v", imageID, node)
		}
	}
	if found == len(nodeNameList) {
		return true
	} else {
		return false

	}

}

func verifySSLHealth(oc *exutil.CLI, ipOfNode string, node string) bool {
	healthCheck := false
	NodeIpAndPort := ipOfNode + ":9979"
	resultOutput, _ := exutil.DebugNodeWithChroot(oc, node, "podman", "run", "--rm", "-ti", "docker.io/drwetter/testssl.sh:latest", NodeIpAndPort)
	outputLines := strings.Split(resultOutput, "\n")
	for _, eachLine := range outputLines {
		if strings.Contains(eachLine, "SWEET32") && strings.Contains(eachLine, "not vulnerable (OK)") {
			healthCheck = true
			break
		}
	}
	if healthCheck {
		e2e.Logf("SWEET32 Vulnerability is secured")
	} else {
		e2e.Logf("SSL op %v ", resultOutput)
	}
	return healthCheck
}

func runDRBackup(oc *exutil.CLI, nodeNameList []string) (nodeName string, etcddb string) {
	var nodeN, etcdDb string
	for nodeindex, node := range nodeNameList {
		backupout, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", oc.Namespace(), "node/"+node, "--", "chroot", "/host", "/usr/local/bin/cluster-backup.sh", "/home/core/assets/backup").Output()
		if strings.Contains(backupout, "Snapshot saved at") && err == nil {
			e2e.Logf("backup on master %v ", node)
			regexp, _ := regexp.Compile("/home/core/assets/backup/snapshot.*db")
			etcdDb = regexp.FindString(backupout)
			nodeN = node
			break
		} else if err != nil && nodeindex < len(nodeNameList) {
			e2e.Logf("Try for next master!")
		} else {
			e2e.Failf("Failed to run the backup!")
		}
	}
	return nodeN, etcdDb
}

func doPrometheusQuery(oc *exutil.CLI, token string, url string, query string) PrometheusQueryResult {
	var data PrometheusQueryResult
	msg, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(
		"-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--",
		"curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token),
		fmt.Sprintf("%s%s", url, query)).Outputs()
	if err != nil {
		e2e.Failf("Failed Prometheus query, error: %v", err)
	}
	o.Expect(msg).NotTo(o.BeEmpty())
	json.Unmarshal([]byte(msg), &data)
	logPrometheusResult(data)
	return data
}

func logPrometheusResult(data PrometheusQueryResult) {
	if len(data.Data.Result) > 0 {
		e2e.Logf("Unexpected metric values.")
		for i, v := range data.Data.Result {
			e2e.Logf(fmt.Sprintf("index: %d value: %s", i, v.Value[1].(string)))
		}
	}
}

func waitForMicroshiftAfterRestart(oc *exutil.CLI, nodename string) {
	exutil.DebugNodeWithOptionsAndChroot(oc, nodename, []string{"-q"}, "bash", "-c", "systemctl restart microshift")
	mStatusErr := wait.Poll(6*time.Second, 300*time.Second, func() (bool, error) {
		output, _ := exutil.DebugNodeWithOptionsAndChroot(oc, nodename, []string{"-q"}, "bash", "-c", "systemctl status microshift")
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift status is: %v ", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(mStatusErr, fmt.Sprintf("Microshift failed to restart: %v", mStatusErr))
}

// make sure the PVC is Bound to the PV
func waitForPvcStatus(oc *exutil.CLI, namespace string, pvcname string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		pvStatus, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pvc", pvcname, "-o=jsonpath='{.status.phase}'").Output()
		if err != nil {
			return false, err
		}
		if match, _ := regexp.MatchString("Bound", pvStatus); match {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The PVC is not Bound as expected")
}

func waitForOneOffBackupToComplete(oc *exutil.CLI, namespace string, bkpname string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		pvStatus, err := oc.AsAdmin().Run("get").Args("-n", namespace, "etcdbackup", bkpname, "-o=jsonpath='{.status.conditions[*].reason}'").Output()
		if err != nil {
			return false, err
		}
		if match, _ := regexp.MatchString("BackupCompleted", pvStatus); match {
			e2e.Logf("OneOffBkpJob status is %v", pvStatus)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The BackupJob is not Completed as expected")
}

func getOneBackupFile(oc *exutil.CLI, namespace string, bkpname string) string {
	bkpfile := ""
	bkpmsg, err := oc.AsAdmin().Run("get").Args("-n", namespace, "etcdbackup", bkpname, "-o=jsonpath='{.status.conditions[*].message}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	bkpmsgList := strings.Fields(bkpmsg)
	for _, bmsg := range bkpmsgList {
		if match, _ := regexp.MatchString("backup-test", bmsg); match {
			e2e.Logf("backupfile is %v", bmsg)
			bkpfile = bmsg
			break
		}
	}
	return bkpfile
}

func verifyBkpFileCreationHost(oc *exutil.CLI, nodeNameList []string, bkpPath string, bkpFile string) bool {
	cmd := "ls -lrt " + bkpPath
	for _, node := range nodeNameList {
		resultOutput, err := exutil.DebugNodeWithChroot(oc, node, "/bin/bash", "-c", cmd)
		if strings.Contains(resultOutput, bkpFile) && err == nil {
			e2e.Logf("OneOffBackupFile %v successfully verified on node %v", bkpFile, node)
			return true
		}
		e2e.Logf("Trying for next node since BackupFile is not found on this node %v", node)
	}
	return false

}

func verifyEtcdClusterMsgStatus(oc *exutil.CLI, msg string, status string) bool {
	etcdStatus, errSt := oc.AsAdmin().WithoutNamespace().Run("get").Args("etcd", "cluster", "-o=jsonpath='{.status.conditions[?(@.reason==\"BootstrapAlreadyRemoved\")].status}'").Output()
	o.Expect(errSt).NotTo(o.HaveOccurred())
	message, errMsg := oc.AsAdmin().WithoutNamespace().Run("get").Args("etcd", "cluster", "-o=jsonpath='{.status.conditions[?(@.reason==\"BootstrapAlreadyRemoved\")].message}'").Output()
	o.Expect(errMsg).NotTo(o.HaveOccurred())
	found := false
	if strings.Contains(message, msg) && strings.Contains(etcdStatus, status) {
		e2e.Logf("message is %v and status is %v", message, etcdStatus)
		found = true
	}
	return found
}

func getIPStackType(oc *exutil.CLI) string {
	svcNetwork, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.serviceNetwork}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	stack := ""
	if strings.Count(svcNetwork, ":") >= 2 && strings.Count(svcNetwork, ".") >= 2 {
		stack = "dualstack"
	} else if strings.Count(svcNetwork, ":") >= 2 {
		stack = "ipv6single"
	} else if strings.Count(svcNetwork, ".") >= 2 {
		stack = "ipv4single"
	}
	return stack
}

func checkOperator(oc *exutil.CLI, operatorName string) {
	err := wait.Poll(60*time.Second, 1500*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("get").Args("clusteroperator", operatorName).Output()
		if err != nil {
			e2e.Logf("get clusteroperator err, will try next time:\n")
			return false, nil
		}
		if matched, _ := regexp.MatchString("True.*False.*False", output); !matched {
			e2e.Logf("clusteroperator %s is abnormal, will try next time:\n", operatorName)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "clusteroperator is abnormal")
}

func verifyRecurBkpFileCreationHost(oc *exutil.CLI, nodeNameList []string, bkpPath string, bkpFile string, count string) bool {
	cmd := "ls -lrt " + bkpPath + " | grep " + bkpFile + "  | wc "
	for _, node := range nodeNameList {
		resultOutput, err := exutil.DebugNodeWithChroot(oc, node, "/bin/bash", "-c", cmd)
		opWords := strings.Split(resultOutput, " ")
		if strings.Contains(opWords[0], count) && err == nil {
			e2e.Logf("Recurring %v successfully verified on node %v", bkpFile, node)
			return true
		}
		e2e.Logf("Trying for next node since expected BackUp files are not found on this node %v", node)
	}
	return false

}

func waitForFirstBackupjobToSchedule(oc *exutil.CLI, namespace string, bkpodname string) string {
	recurPod := ""
	err := wait.Poll(20*time.Second, 120*time.Second, func() (bool, error) {
		podNameOp, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pods", "-o=jsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			return false, err
		}
		podNameList := strings.Fields(podNameOp)
		for _, podName := range podNameList {
			if strings.Contains(podName, bkpodname) && err == nil {
				e2e.Logf("First RecurringBkpPod is %v", podName)
				recurPod = podName
				return true, nil
			}

		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The recurring Backup job is not scheduled")
	return recurPod
}

func waitForRecurBackupJobToComplete(oc *exutil.CLI, namespace string, expectedPod string, expectedState string) {
	firstSchPod := waitForFirstBackupjobToSchedule(oc, namespace, expectedPod)

	err := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		statusOp, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pod", firstSchPod, "-o=jsonpath='{.status.phase}'").Output()
		if err != nil {
			return false, err
		}

		if strings.Contains(statusOp, expectedState) && err == nil {
			e2e.Logf("firstSchPod %v is %v", firstSchPod, statusOp)
			return true, nil
		}
		e2e.Logf("firstSchPod %v is %v, Trying again", firstSchPod, statusOp)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The recurring Backup job is not completed")
}

func isCRDExisting(oc *exutil.CLI, crd string) bool {
	output, err := oc.AsAdmin().Run("get").Args("CustomResourceDefinition", crd, "-o=jsonpath={.metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Compare(output, crd) == 0
}
func createCRD(oc *exutil.CLI, filename string) {
	baseDir := exutil.FixturePath("testdata", "etcd")
	crdTemplate := filepath.Join(baseDir, filename)
	err := oc.AsAdmin().Run("create").Args("-f", crdTemplate).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Successfully created CRD")
}

// make sure all the ectd pods are running
func checkEtcdPodStatus(oc *exutil.CLI) bool {
	output, err := oc.AsAdmin().Run("get").Args("pods", "-l", "app=etcd", "-n", "openshift-etcd", "-o=jsonpath='{.items[*].status.phase}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	statusList := strings.Fields(output)
	for _, podStatus := range statusList {
		if match, _ := regexp.MatchString("Running", podStatus); !match {
			e2e.Logf("Find etcd pod is not running")
			return false
		}
	}
	return true
}

// make sure all the ectd operator pods are running
func checkEtcdOperatorPodStatus(oc *exutil.CLI) bool {
	output, err := oc.AsAdmin().Run("get").Args("pods", "-n", "openshift-etcd-operator", "-o=jsonpath='{.items[*].status.phase}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	statusList := strings.Fields(output)
	for _, podStatus := range statusList {
		if match, _ := regexp.MatchString("Running", podStatus); !match {
			e2e.Logf("etcd operator pod is not running")
			return false
		}
	}
	return true
}

// get the proxies
func getGlobalProxy(oc *exutil.CLI) (string, string) {
	httpProxy, httperr := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status.httpProxy}").Output()
	o.Expect(httperr).NotTo(o.HaveOccurred())
	httpsProxy, httsperr := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status.httpsProxy}").Output()
	o.Expect(httsperr).NotTo(o.HaveOccurred())
	return httpProxy, httpsProxy
}

func verifyImageIDwithProxy(oc *exutil.CLI, nodeNameList []string, httpProxy string, httpsProxy string, imageID string, cVersion string) bool {
	found := 0
	cmd := "export http_proxy=" + httpProxy + ";export https_proxy=" + httpsProxy + ";oc adm release info --registry-config=/var/lib/kubelet/config.json " + cVersion + " --image-for=etcd"
	for _, node := range nodeNameList {
		resultOutput, err := exutil.DebugNodeWithChroot(oc, node, "/bin/bash", "-c", cmd)
		if strings.Contains(resultOutput, imageID) && err == nil {
			e2e.Logf("Image %v successfully deployed on node %v", imageID, node)
			found += 1
		} else {
			e2e.Logf("Failed to deploy Image %v on node %v", imageID, node)
		}
	}
	if found == len(nodeNameList) {
		return true
	} else {
		return false

	}
}

func waitForPodStatus(oc *exutil.CLI, podName string, nameSpace string, podStatus string) {
	err := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		statusOp, err := oc.AsAdmin().Run("get").Args("-n", nameSpace, "pod", podName, "-o=jsonpath='{.status.phase}'").Output()
		if err != nil {
			return false, err
		}

		if strings.Contains(statusOp, podStatus) {
			e2e.Logf("pod %v is %v", podName, podStatus)
			return true, nil
		}
		e2e.Logf("Pod %v is %v, Trying again", podName, statusOp)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The test pod job is not running")
}

func verifyBkpFileCreationOnExternalVol(oc *exutil.CLI, podName string, nameSpace string, bkpPath string, bkpFile string) bool {
	resultOutput, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", nameSpace, podName, "bash", "-c", `ls -lrt `+bkpPath).Output()
	if strings.Contains(resultOutput, bkpFile) && err == nil {
		e2e.Logf("OneOffBackupFile %v successfully verified on exterval volume", bkpFile)
		return true
	} else {
		e2e.Logf("OneOffBackupFile %v not found on exterval volume", bkpFile)
		return false
	}
}

func verifyRecurringBkpFileOnExternalVol(oc *exutil.CLI, podName string, nameSpace string, bkpPath string, bkpFile string, count string) bool {
	cmd := "ls -lrt " + bkpPath + " | grep " + bkpFile + "  | wc "
	resultOutput, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", nameSpace, podName, "bash", "-c", cmd).Output()
	e2e.Logf("resultOutput is %v", resultOutput)
	opWords := strings.Split(resultOutput, " ")
	if strings.Contains(opWords[0], count) && err == nil {
		e2e.Logf("Recurring Backup successfully verified on external volume")
		return true
	}
	return false
}
