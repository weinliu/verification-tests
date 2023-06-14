package cpu

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

func cpuManagerStatebyNode(oc *exutil.CLI, namespace string, nodeName string, ContainerName string) (string, string) {

	var (
		PODCUPs string
		CPUNums string
	)

	cpuManagerStateStdOut, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("node/"+nodeName, "-n", namespace, "-q", "--", "chroot", "host", "cat", "/var/lib/kubelet/cpu_manager_state").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(cpuManagerStateStdOut).NotTo(o.BeEmpty())

	var cpuManagerStateInfo map[string]interface{}
	json.Unmarshal([]byte(cpuManagerStateStdOut), &cpuManagerStateInfo)

	defaultCpuSet := fmt.Sprint(cpuManagerStateInfo["defaultCpuSet"])
	o.Expect(defaultCpuSet).NotTo(o.BeEmpty())

	Entries := fmt.Sprint(cpuManagerStateInfo["entries"])
	o.Expect(Entries).NotTo(o.BeEmpty())

	PODUUIDMapCPUs := strings.Split(Entries, " ")
	Len := len(PODUUIDMapCPUs)
	for i := 0; i < Len; i++ {
		if strings.Contains(PODUUIDMapCPUs[i], ContainerName) {
			PODUUIDMapCPU := strings.Split(PODUUIDMapCPUs[i], ":")
			CPUNums = strings.Trim(PODUUIDMapCPU[len(PODUUIDMapCPU)-1], "]")
		}
		PODCUPs += CPUNums + " "
	}
	return defaultCpuSet, PODCUPs
}

func getContainerIDByPODName(oc *exutil.CLI, podName string, namespace string) string {
	var containerID string
	containerIDStdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, `-ojsonpath='{.status.containerStatuses[?(@.name=="etcd")].containerID}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(containerIDStdOut).NotTo(o.BeEmpty())

	containerIDArr := strings.Split(containerIDStdOut, "/")
	Len := len(containerIDArr)
	if Len > 0 {
		containerID = containerIDArr[Len-1]
	}
	return containerID
}

func getPODCPUSet(oc *exutil.CLI, namespace string, nodeName string, containerID string) string {

	podCPUSetStdDir, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("node/"+nodeName, "-n", namespace, "-q", "--", "chroot", "host", "find", "/sys/fs/cgroup/cpuset/", "-name", "*"+containerID+"*").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podCPUSetStdDir).NotTo(o.BeEmpty())
	podCPUSet, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("node/"+nodeName, "-n", namespace, "-q", "--", "chroot", "host", "cat", podCPUSetStdDir+"/cpuset.cpus").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podCPUSet).NotTo(o.BeEmpty())
	return podCPUSet
}

func getFirstDrainedMasterNode(oc *exutil.CLI) string {

	var (
		nodeName string
	)
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		nodeHostNameStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", `-ojsonpath={.items[?(@.spec.unschedulable==true)].metadata.name}`).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		nodeHostName := strings.Trim(nodeHostNameStr, "'")

		masterNodeNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/master", "-oname").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if len(nodeHostName) > 0 && strings.Contains(masterNodeNames, nodeHostName) {
			nodeName = nodeHostName
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "No any node's status is SchedulingDisabled was found")
	return nodeName
}
