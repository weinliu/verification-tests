package etcd

import (
	o "github.com/onsi/gomega"

	"fmt"
	"math/rand"
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
	exutil.AssertWaitPollNoErr(err, "clusteroperator abnormal")
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
