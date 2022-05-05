package disaster_recovery

import (
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
		backupout, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", oc.Namespace(), "node/"+node, "--", "chroot", "/host", "/usr/local/bin/cluster-backup.sh", "/home/core/assets/backup").Output()
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

func getUserNameAndKeyonBationByPlatform(iaasPlatform string, privateKey string) (string, string) {
	user := ""
	keyOnBastion := ""
	switch iaasPlatform {
	case "aws":
		user = os.Getenv("SSH_CLOUD_PRIV_AWS_USER")
		if user == "" {
			user = "ec2-user"
		}
		keyOnBastion = "/home/ec2-user/" + filepath.Base(privateKey)
	case "gcp":
		user = os.Getenv("SSH_CLOUD_PRIV_GCP_USER")
		if user == "" {
			user = "cloud-user"
		}
		keyOnBastion = "/home/cloud-user/" + filepath.Base(privateKey)
	case "azure":
		user = os.Getenv("SSH_CLOUD_PRIV_AZURE_USER")
		if user == "" {
			user = "cloud-user"
		}
		keyOnBastion = "/home/cloud-user/" + filepath.Base(privateKey)
	}
	return user, keyOnBastion
}

func getNodeInternalIpListByLabel(oc *exutil.CLI, labelKey string) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", labelKey, "-o=jsonpath='{.items[*].status.addresses[?(.type==\"InternalIP\")].address}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeInternalIpList := strings.Fields(strings.ReplaceAll(output, "'", ""))
	return nodeInternalIpList
}

// Run the etcdrestroe shell script command on master or node
func runPSCommand(bastionHost string, nodeInternalIp string, command string, privateKeyForClusterNode string, privateKeyForBastion string, userForBastion string) (result string, err error) {
	var msg []byte
	msg, err = exec.Command("bash", "-c", "chmod 600 "+privateKeyForBastion+"; ssh -i "+privateKeyForBastion+" -t -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "+userForBastion+"@"+bastionHost+" sudo -i ssh  -o StrictHostKeyChecking=no  -o UserKnownHostsFile=/dev/null -i "+privateKeyForClusterNode+" core@"+nodeInternalIp+" "+command).CombinedOutput()
	return string(msg), err
}

func waitForOperatorRestart(oc *exutil.CLI, operatorName string) {
	g.By("Check the operator should be in Progressing")
	err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", operatorName).Output()
		if err != nil {
			e2e.Logf("clusteroperator not start new progress, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString("True.*True.*False", output); matched {
			e2e.Logf("clusteroperator is Progressing:\n%s", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "clusteroperator is not Progressing")

	g.By("Wait for the operator to rollout")
	err = wait.Poll(60*time.Second, 900*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", operatorName).Output()
		if err != nil {
			e2e.Logf("Fail to get clusteroperator %s, error: %s. Trying again", operatorName, err)
			return false, nil
		}
		if matched, _ := regexp.MatchString("True.*False.*False", output); matched {
			e2e.Logf("clusteroperator %s is recover to normal:\n%s", operatorName, output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "clusteroperator is not recovered to normal")
}

func waitForContainerDisappear(bastionHost string, nodeInternalIp string, command string, privateKeyForClusterNode string, privateKeyForBastion string, userForBastion string) {
	g.By("Wait for the container to disappear")
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		msg, err := runPSCommand(bastionHost, nodeInternalIp, command, privateKeyForClusterNode, privateKeyForBastion, userForBastion)
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

//Check if the iaasPlatform in the supported list
func in(target string, str_array []string) bool {
	for _, element := range str_array {
		if target == element {
			return true
		}
	}
	return false
}

//make sure all the ectd pods are running
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
