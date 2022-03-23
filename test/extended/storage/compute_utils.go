package storage

import (
	"fmt"
	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	o "github.com/onsi/gomega"
)

// Execute command in node
func execCommandInSpecificNode(oc *exutil.CLI, nodeHostName string, command string) (string, error) {
	nodeHostName = "node/" + nodeHostName
	command1 := []string{nodeHostName, "-q", "--", "chroot", "/host", "bin/sh", "-c", command}
	msg, err := oc.AsAdmin().Run("debug").Args(command1...).Output()
	if err != nil {
		e2e.Logf("Execute \""+command+"\" on node \"%s\" *failed with* : \"%v\".", nodeHostName, err)
		return msg, err
	} else {
		e2e.Logf("Executed \""+command+"\" on node \"%s\" *Successed* ", nodeHostName)
		debugLogf("Executed \""+command+"\" on node \"%s\" *Output is* : \"%v\".", nodeHostName, msg)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

// Check the Volume mounted on the Node
func checkVolumeMountOnNode(oc *exutil.CLI, volumeName string, nodeName string) {
	command := "mount | grep " + volumeName
	err := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		_, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" mount on node: \"%s\" failed", volumeName, nodeName))
}

// Check the Volume not mounted on the Node
func checkVolumeNotMountOnNode(oc *exutil.CLI, volumeName string, nodeName string) {
	command := "mount | grep -c \"" + volumeName + "\" || true"
	err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
		count, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			e2e.Logf("Err Occurred: %v", err)
			return false, err
		}
		if count == "0" {
			e2e.Logf("Volume: \"%s\" umount from node \"%s\" successfully", volumeName, nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" unmount from node: \"%s\" timeout", volumeName, nodeName))
}

// Check the Volume not detached from the Node
func checkVolumeDetachedFromNode(oc *exutil.CLI, volumeName string, nodeName string) {
	command := "lsblk | grep -c \"" + volumeName + "\" || true"
	err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
		count, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			e2e.Logf("Err Occurred: %v", err)
			return false, err
		}
		if count == "0" {
			e2e.Logf("Volume: \"%s\" detached from node \"%s\" successfully", volumeName, nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" detached from node: \"%s\" timeout", volumeName, nodeName))
}

// Check the mounted volume on the Node contains content by cmd
func checkVolumeMountCmdContain(oc *exutil.CLI, volumeName string, nodeName string, content string) {
	command := "mount | grep " + volumeName
	err := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		msg, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			return false, nil
		}
		return strings.Contains(msg, content), nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" mount in node : \"%s\" contains  \"%s\" failed", volumeName, nodeName, content))
}

// Get the Node List for pod with label
func getNodeListForPodByLabel(oc *exutil.CLI, namespace string, labelName string) ([]string, error) {
	podsList, err := getPodsListByLabel(oc, namespace, labelName)
	o.Expect(err).NotTo(o.HaveOccurred())
	var nodeList []string
	for _, pod := range podsList {
		nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", pod, "-n", namespace, "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s is on Node:\"%s\"", pod, nodeName)
		nodeList = append(nodeList, nodeName)
	}
	return nodeList, err
}

func getNodeNameByPod(oc *exutil.CLI, namespace string, podName string) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The node name is %s", nodeName)
	return nodeName
}

// Get the cluster wokernodes info
func getWorkersInfo(oc *exutil.CLI) string {
	workersInfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker", "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workersInfo
}

// Get the cluster schedulable woker nodes names with the same avaiable zone
func getSchedulableWorkersWithSameAz(oc *exutil.CLI) (schedulableWorkersWithSameAz []string, azName string) {
	var (
		workersInfo              = getWorkersInfo(oc)
		workers                  = strings.Split(strings.Trim(strings.Trim(gjson.Get(workersInfo, "items.#.metadata.name").String(), "["), "]"), ",")
		schedulableWorkersWithAz = make(map[string]string)
		zonePath                 = `metadata.labels.topology\.kubernetes\.io\/zone`
	)
	for _, worker := range workers {
		scheduleFlag := gjson.Get(workersInfo, "items.#(metadata.name="+worker+").spec.unschedulable").String()
		workerOS := gjson.Get(workersInfo, "items.#(metadata.name="+worker+").metadata.labels.kubernetes\\.io\\/os").String()
		if scheduleFlag != "true" && workerOS == "linux" {
			azName = gjson.Get(workersInfo, "items.#(metadata.name="+worker+")."+zonePath).String()
			if azName == "" {
				azName = "noneAzCluster"
			}
			if _, ok := schedulableWorkersWithAz[azName]; ok {
				e2e.Logf("Schedulable workers %s,%s in the same az %s", worker, schedulableWorkersWithAz[azName], azName)
				return append(schedulableWorkersWithSameAz, worker, schedulableWorkersWithAz[azName]), azName
			}
			schedulableWorkersWithAz[azName] = worker
		}
	}
	e2e.Logf("*** The test cluster has less than two schedulable linux workers in each avaiable zone! ***")
	return nil, azName
}

// Drain specified node
func drainSpecificNode(oc *exutil.CLI, nodeName string) {
	e2e.Logf("oc adm drain nodes/" + nodeName + " --ignore-daemonsets --delete-emptydir-data --force --timeout=600s")
	err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("drain", "nodes/"+nodeName, "--ignore-daemonsets", "--delete-emptydir-data", "--force", "--timeout=600s").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Uncordon specified node
func uncordonSpecificNode(oc *exutil.CLI, nodeName string) error {
	e2e.Logf("oc adm uncordon nodes/" + nodeName)
	return oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", "nodes/"+nodeName).Execute()
}

// Waiting specified node avaiable: scheduleable and ready
func waitNodeAvaiable(oc *exutil.CLI, nodeName string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		nodeInfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes/"+nodeName, "-o", "json").Output()
		if err != nil {
			e2e.Logf("Err Occurred: %v", err)
			return false, err
		}
		if !gjson.Get(nodeInfo, `spec.unschedulable`).Exists() && gjson.Get(nodeInfo, `status.conditions.#(type=Ready).status`).String() == "True" {
			e2e.Logf("Node: \"%s\" is ready to use", nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting Node: \"%s\" become ready to use timeout", nodeName))
}
