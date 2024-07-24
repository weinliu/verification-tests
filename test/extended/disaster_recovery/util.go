package disasterrecovery

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"bufio"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

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

func runDRBackup(oc *exutil.CLI, nodeNameList []string) (nodeName string, etcddb string) {
	var nodeN, etcdDb string
	succBackup := false
	for _, node := range nodeNameList {
		backupout, err := exutil.DebugNodeWithOptionsAndChroot(oc, node, []string{"-q"}, "/usr/local/bin/cluster-backup.sh", "/home/core/assets/backup")
		if err != nil {
			e2e.Logf("Try for next master!")
			continue
		}
		if strings.Contains(backupout, "Snapshot saved at") && err == nil {
			e2e.Logf("backup on master %v ", node)
			regexp, _ := regexp.Compile("/home/core/assets/backup/snapshot.*db")
			etcdDb = regexp.FindString(backupout)
			nodeN = node
			succBackup = true
			break
		}
	}
	if !succBackup {
		e2e.Failf("Failed to run the backup!")
	}
	return nodeN, etcdDb
}

func getUserNameAndKeyonBationByPlatform(iaasPlatform string) string {
	user := ""
	switch iaasPlatform {
	case "aws":
		user = os.Getenv("SSH_CLOUD_PRIV_AWS_USER")
	case "gcp":
		user = os.Getenv("SSH_CLOUD_PRIV_GCP_USER")
	case "azure":
		user = os.Getenv("SSH_CLOUD_PRIV_AZURE_USER")
	}
	return user
}
func getNewMastermachine(masterMachineStatus []string, masterMachineNameList []string, desiredStatus string) string {
	newMasterMachine := ""
	for p, v := range masterMachineStatus {
		if strings.Contains(v, desiredStatus) {
			newMasterMachine = masterMachineNameList[p]
			break
		}
	}
	e2e.Logf("New machine is %s", newMasterMachine)
	return newMasterMachine
}

func getNodeInternalIPListByLabel(oc *exutil.CLI, labelKey string) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", labelKey, "-o=jsonpath='{.items[*].status.addresses[?(.type==\"InternalIP\")].address}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeInternalIPList := strings.Fields(strings.ReplaceAll(output, "'", ""))
	return nodeInternalIPList
}

// Run the etcdrestroe shell script command on master or node
func runPSCommand(bastionHost string, nodeInternalIP string, command string, privateKeyForBastion string, userForBastion string) (result string, err error) {
	var msg []byte
	if bastionHost != "" {
		msg, err = exec.Command("bash", "-c", "chmod 600 "+privateKeyForBastion+";ssh -i "+privateKeyForBastion+" -o StrictHostKeyChecking=no  -o ProxyCommand=\"ssh -o IdentityFile="+privateKeyForBastion+" -o StrictHostKeyChecking=no -W %h:%p "+userForBastion+"@"+bastionHost+"\""+" core@"+nodeInternalIP+" "+command).CombinedOutput()
	} else {
		msg, err = exec.Command("bash", "-c", "chmod 600 "+privateKeyForBastion+";ssh -i "+privateKeyForBastion+" -o StrictHostKeyChecking=no core@"+nodeInternalIP+" "+command).CombinedOutput()
	}
	return string(msg), err
}

func waitForOperatorRestart(oc *exutil.CLI, operatorName string) {
	g.By("Check the operator should be in Progressing")
	err := wait.Poll(20*time.Second, 600*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", operatorName).Output()
		if err != nil {
			e2e.Logf("clusteroperator %s has not started new progress, error: %s. Trying again", operatorName, err)
			return false, nil
		}
		if matched, _ := regexp.MatchString("True.*True.*False", output); matched {
			e2e.Logf("clusteroperator %s is Progressing:\n%s", operatorName, output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "clusteroperator is not Progressing")

	g.By("Wait for the operator to rollout")
	err = wait.Poll(60*time.Second, 1500*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", operatorName).Output()
		if err != nil {
			e2e.Logf("Fail to get clusteroperator %s, error: %s. Trying again", operatorName, err)
			return false, nil
		}
		if matched, _ := regexp.MatchString("True.*False.*False", output); matched {
			e2e.Logf("clusteroperator %s has recovered to normal:\n%s", operatorName, output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "clusteroperator has not recovered to normal")
}

func waitForContainerDisappear(bastionHost string, nodeInternalIP string, command string, privateKeyForBastion string, userForBastion string) {
	g.By("Wait for the container to disappear")
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		msg, err := runPSCommand(bastionHost, nodeInternalIP, command, privateKeyForBastion, userForBastion)
		if err != nil {
			e2e.Logf("Fail to get container, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString("", msg); matched {
			e2e.Logf("The container has disappeared")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The pod is not disappeared as expected")
}

// Check if the iaasPlatform in the supported list
func in(target string, strArray []string) bool {
	for _, element := range strArray {
		if target == element {
			return true
		}
	}
	return false
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

// make sure all the machine are running
func waitMachineStatusRunning(oc *exutil.CLI, newMasterMachineName string) {
	err := wait.Poll(30*time.Second, 600*time.Second, func() (bool, error) {
		machineStatus, errSt := oc.AsAdmin().Run("get").Args("-n", "openshift-machine-api", exutil.MapiMachine, newMasterMachineName, "-o=jsonpath='{.status.phase}'").Output()
		if errSt != nil {
			e2e.Logf("Failed to get machineStatus, error: %s. Trying again", errSt)
			return false, nil
		}
		if match, _ := regexp.MatchString("Running", machineStatus); match {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Sadly the machine is not Running.")
}

// make sure correct number of machines are present
func waitforDesiredMachineCount(oc *exutil.CLI, machineCount int) {
	err := wait.Poll(60*time.Second, 1500*time.Second, func() (bool, error) {
		output, errGetMachine := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o=jsonpath='{.items[*].metadata.name}'").Output()
		if errGetMachine != nil {
			e2e.Logf("Failed to get machinecount, error: %s. Trying again", errGetMachine)
			return false, nil
		}
		machineNameList := strings.Fields(output)
		if len(machineNameList) == machineCount {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Sadly the machine count didn't match")
}

// update new machine file
func updateMachineYmlFile(machineYmlFile string, oldMachineName string, newMasterMachineName string) bool {
	fileName := machineYmlFile
	in, err := os.OpenFile(fileName, os.O_RDONLY, 0666)
	if err != nil {
		e2e.Logf("open machineYaml file fail:", err)
		return false
	}
	defer in.Close()

	out, err := os.OpenFile(strings.Replace(fileName, "machine.yaml", "machineUpd.yaml", -1), os.O_RDWR|os.O_CREATE, 0766)
	if err != nil {
		e2e.Logf("Open write file fail:", err)
		return false
	}
	defer out.Close()

	br := bufio.NewReader(in)
	index := 1
	matchTag := false
	newLine := ""

	for {
		line, _, err := br.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			e2e.Logf("read err:", err)
			return false
		}
		if strings.Contains(string(line), "providerID: ") {
			matchTag = true
		} else if strings.Contains(string(line), "status:") {
			break
		} else if strings.Contains(string(line), "generation: ") {
			matchTag = true
		} else if strings.Contains(string(line), "machine.openshift.io/instance-state: ") {
			matchTag = true
		} else if strings.Contains(string(line), "resourceVersion: ") {
			matchTag = true
		} else if strings.Contains(string(line), oldMachineName) {
			newLine = strings.Replace(string(line), oldMachineName, newMasterMachineName, -1)
		} else {
			newLine = string(line)
		}
		if !matchTag {
			_, err = out.WriteString(newLine + "\n")
			if err != nil {
				e2e.Logf("Write to file fail:", err)
				return false
			}
		} else {
			matchTag = false
		}
		index++
	}
	e2e.Logf("Update Machine FINISH!")
	return true
}

// make sure operator is not processing and degraded
func checkOperator(oc *exutil.CLI, operatorName string) {
	var output string
	var err error
	var split []string
	if operatorName == "" {
		output, err = oc.AsAdmin().Run("get").Args("clusteroperator", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		split = strings.Split(output, " ")
	} else {
		split = append(split, operatorName)
	}
	err = wait.Poll(60*time.Second, 1500*time.Second, func() (bool, error) {
		for _, item := range split {
			output, err = oc.AsAdmin().Run("get").Args("clusteroperator", item).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matched, _ := regexp.MatchString("True.*False.*False", output); !matched {
				e2e.Logf("clusteroperator %s is abnormal, will try next time:\n", item)
				return false, nil
			}
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "clusteroperator abnormal")
}

func waitMachineDesiredStatus(oc *exutil.CLI, newMasterMachineName string, desiredState string) {
	err := wait.Poll(60*time.Second, 480*time.Second, func() (bool, error) {
		machineStatus, err := oc.AsAdmin().Run("get").Args("-n", "openshift-machine-api", exutil.MapiMachine, newMasterMachineName, "-o=jsonpath='{.status.phase}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if match, _ := regexp.MatchString(desiredState, machineStatus); match {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "sorry the machine is not in desired state")
}

func waitForDesiredStateOfCR(oc *exutil.CLI, desiredState string) {
	err := wait.Poll(60*time.Second, 480*time.Second, func() (bool, error) {
		statusOfCR, err := oc.AsAdmin().Run("get").Args("controlplanemachineset.machine.openshift.io", "cluster", "-n", "openshift-machine-api", "-o=jsonpath={.spec.state}").Output()
		if err != nil {
			e2e.Logf("Failed to get CR status, error: %s. Trying again", err)
			return false, nil
		}
		e2e.Logf("statusOfCR is %v ", statusOfCR)
		if statusOfCR == desiredState {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "sorry the CR is not in desired state")
}

// Checks whether cluster operator is healthy.
func IsCOHealthy(oc *exutil.CLI, operatorName string) bool {
	output, err := oc.AsAdmin().Run("get").Args("clusteroperator", operatorName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if matched, _ := regexp.MatchString("True.*False.*False", output); !matched {
		e2e.Logf("clusteroperator %s is abnormal", operatorName)
		return false
	}
	return true
}

// Checks cluster operator is healthy
func healthyCheck(oc *exutil.CLI) bool {
	e2e.Logf("make sure all the etcd pods are running")
	podAllRunning := checkEtcdPodStatus(oc)
	if podAllRunning != true {
		e2e.Logf("The ectd pods are not running")
		return false
	}
	e2e.Logf("Check all oprators status")
	checkOperator(oc, "")

	e2e.Logf("Make sure all the nodes are normal")
	out, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
	checkMessage := []string{
		"SchedulingDisabled",
		"NotReady",
	}
	for _, v := range checkMessage {
		if strings.Contains(out, v) {
			e2e.Logf("The cluster nodes is abnormal.")
			return false
		}
	}
	return true
}
