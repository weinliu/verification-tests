package nto

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// isPodInstalled will return true if any pod is found in the given namespace, and false otherwise
func isNTOPodInstalled(oc *exutil.CLI, namespace string) bool {

	e2e.Logf("Checking if pod is found in namespace %s...", namespace)

	ntoDeployment, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", namespace, "-ojsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	if len(ntoDeployment) == 0 {
		e2e.Logf("No deployment cluster-node-tuning-operator found in namespace %s :(", namespace)
		return false
	}
	e2e.Logf("Deployment %v found in namespace %s!", ntoDeployment, namespace)
	return true
}

// getNTOPodName checks all pods in a given namespace and returns the first NTO pod name found
func getNTOPodName(oc *exutil.CLI, namespace string) (string, error) {

	podList, err := exutil.GetAllPods(oc, namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
	podListSize := len(podList)
	for i := 0; i < podListSize; i++ {
		if strings.Contains(podList[i], "cluster-node-tuning-operator") {
			return podList[i], nil
		}
	}
	return "", fmt.Errorf("NTO pod was not found in namespace %s", namespace)
}

// getTunedState returns a string representation of the spec.managementState of the specified tuned in a given namespace
func getTunedState(oc *exutil.CLI, namespace string, tunedName string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", tunedName, "-n", namespace, "-o=jsonpath={.spec.managementState}").Output()
}

// patchTunedState will patch the state of the specified tuned to that specified if supported, will throw an error if patch fails or state unsupported
func patchTunedState(oc *exutil.CLI, namespace string, tunedName string, state string) error {

	state = strings.ToLower(state)
	if state == "unmanaged" {
		return oc.AsAdmin().WithoutNamespace().Run("patch").Args("tuned", tunedName, "-p", `{"spec":{"managementState":"Unmanaged"}}`, "--type", "merge", "-n", namespace).Execute()
	} else if state == "managed" {
		return oc.AsAdmin().WithoutNamespace().Run("patch").Args("tuned", tunedName, "-p", `{"spec":{"managementState":"Managed"}}`, "--type", "merge", "-n", namespace).Execute()
	} else if state == "removed" {
		return oc.AsAdmin().WithoutNamespace().Run("patch").Args("tuned", tunedName, "-p", `{"spec":{"managementState":"Removed"}}`, "--type", "merge", "-n", namespace).Execute()
	} else {
		return fmt.Errorf("specified state %s is unsupported", state)
	}
}

// getTunedPriority returns a string representation of the spec.recommend.priority of the specified tuned in a given namespace
func getTunedPriority(oc *exutil.CLI, namespace string, tunedName string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", tunedName, "-n", namespace, "-o=jsonpath={.spec.recommend[*].priority}").Output()
}

// patchTunedPriority will patch the priority of the specified tuned to that specified in a given YAML or JSON file
// we cannot directly patch the value since it is nested within a list, thus the need for a patch file for this function
func patchTunedProfile(oc *exutil.CLI, namespace string, tunedName string, patchFile string) error {
	return oc.AsAdmin().WithoutNamespace().Run("patch").Args("tuned", tunedName, "--patch-file="+patchFile, "--type", "merge", "-n", namespace).Execute()
}

// getTunedProfile returns a string representation of the status.tunedProfile of the given node in the given namespace
func getTunedProfile(oc *exutil.CLI, namespace string, tunedNodeName string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("profiles.tuned.openshift.io", tunedNodeName, "-n", namespace, "-o=jsonpath={.status.tunedProfile}").Output()
}

// assertIfTunedProfileApplied checks the logs for a given tuned pod in a given namespace to see if the expected profile was applied
func assertIfTunedProfileApplied(oc *exutil.CLI, namespace string, tunedNodeName string, tunedName string) {

	o.Eventually(func() bool {
		appliedStatus, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "profiles.tuned.openshift.io", tunedNodeName, `-ojsonpath='{.status.conditions[?(@.type=="Applied")].status}'`).Output()
		tunedProfile, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "profiles.tuned.openshift.io", tunedNodeName, "-ojsonpath={.status.tunedProfile}").Output()
		if err1 != nil || err2 != nil || strings.Contains(appliedStatus, "False") || strings.Contains(appliedStatus, "Unknown") || tunedProfile != tunedName {
			e2e.Logf("failed to apply profile to nodes, the status is %s and profile is %s, check again", appliedStatus, tunedProfile)
		}
		return strings.Contains(appliedStatus, "True") && tunedProfile == tunedName
	}, 15*time.Second, time.Second).Should(o.BeTrue())
}

// assertIfNodeSchedulingDisabled checks all nodes in a cluster to see if 'SchedulingDisabled' status is present on any node
func assertIfNodeSchedulingDisabled(oc *exutil.CLI) string {

	var nodeNames []string
	var nodeNameList []string
	err := wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
		nodeCheck, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		isNodeSchedulingDisabled := strings.Contains(nodeCheck, "SchedulingDisabled")
		isNodeNotReady := strings.Contains(nodeCheck, "NotReady")
		if isNodeSchedulingDisabled || isNodeNotReady {
			e2e.Logf("'SchedulingDisabled' or 'NotReady' status found!")
			if isNodeNotReady {
				e2e.Logf("'NotReady' status found!")
				nodeNameReg := regexp.MustCompile(".*NotReady.*")
				nodeNameList = nodeNameReg.FindAllString(nodeCheck, -1)
			} else if isNodeSchedulingDisabled {
				nodeNameReg := regexp.MustCompile(".*SchedulingDisabled.*")
				nodeNameList = nodeNameReg.FindAllString(nodeCheck, -1)
			} else {
				e2e.Logf("'SchedulingDisabled' or 'NotReady' isn't found!")
			}

			nodeNamestr := nodeNameList[0]
			nodeNames = strings.Split(nodeNamestr, " ")
			e2e.Logf("Node Names is %v", nodeNames)
			return true, nil
		}
		e2e.Logf("'SchedulingDisabled' status not found - retrying...")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "No node was found with 'SchedulingDisabled' status within timeout limit (3 minutes)")
	e2e.Logf("Node Name is %v", nodeNames[0])
	return nodeNames[0]
}

// assertIfMasterNodeChangesApplied checks all nodes in a cluster with the master role to see if 'default_hugepagesz=2M' is present on every node in /proc/cmdline
func assertIfMasterNodeChangesApplied(oc *exutil.CLI, masterNodeName string) {

	err := wait.Poll(1*time.Minute, 5*time.Minute, func() (bool, error) {
		output, err := exutil.DebugNode(oc, masterNodeName, "cat", "/proc/cmdline")
		o.Expect(err).NotTo(o.HaveOccurred())

		isMasterNodeChanged := strings.Contains(output, "default_hugepagesz=2M")
		if isMasterNodeChanged {
			e2e.Logf("Node %v has expected changes:\n%v", masterNodeName, output)
			return true, nil
		}
		e2e.Logf("Node %v does not have expected changes - retrying...", masterNodeName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Node"+masterNodeName+"did not have expected changes within timeout limit")
}

// getMaxUserWatchesValue parses out the line determining max_user_watches in inotify.conf
func getMaxUserWatchesValue(inotify string) string {
	reLine := regexp.MustCompile(`fs.inotify.max_user_watches = \d+`)
	reValue := regexp.MustCompile(`\d+`)
	maxUserWatches := reLine.FindString(inotify)
	maxUserWatchesValue := reValue.FindString(maxUserWatches)
	return maxUserWatchesValue
}

// getMaxUserInstancesValue parses out the line determining max_user_instances in inotify.conf
func getMaxUserInstancesValue(inotify string) string {
	reLine := regexp.MustCompile(`fs.inotify.max_user_instances = \d+`)
	reValue := regexp.MustCompile(`\d+`)
	maxUserInstances := reLine.FindString(inotify)
	maxUserInstancesValue := reValue.FindString(maxUserInstances)
	return maxUserInstancesValue
}

// getKernelPidMaxValue parses out the line determining pid_max in the kernel
func getKernelPidMaxValue(kernel string) string {
	reLine := regexp.MustCompile(`kernel.pid_max = \d+`)
	reValue := regexp.MustCompile(`\d+`)
	pidMax := reLine.FindString(kernel)
	pidMaxValue := reValue.FindString(pidMax)
	return pidMaxValue
}

// compareSpecifiedValueByNameOnLabelNode Compare if the sysctl parameter is equal to specified value on labeled node
func compareSpecifiedValueByNameOnLabelNode(oc *exutil.CLI, labelNodeName, sysctlparm, specifiedvalue string) {

	regexpstr, _ := regexp.Compile(sysctlparm + ".*")
	//output, err := exutil.DebugNodeWithChroot(oc, labelNodeName, "sysctl", sysctlparm)
	stdOut, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, labelNodeName, []string{"-q"}, "sysctl", sysctlparm)
	conntrackMax := regexpstr.FindString(stdOut)
	e2e.Logf("The value is %v on %v", conntrackMax, labelNodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(stdOut).To(o.ContainSubstring(sysctlparm + " = " + specifiedvalue))

}

// compareSysctlDifferentFromSpecifiedValueByName compare if the sysctl parameter is not equal to specified value on all the node
func compareSysctlDifferentFromSpecifiedValueByName(oc *exutil.CLI, sysctlparm, specifiedvalue string) {

	nodeList, err := exutil.GetAllNodesbyOSType(oc, "linux")
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeListSize := len(nodeList)

	regexpstr, _ := regexp.Compile(sysctlparm + ".*")
	for i := 0; i < nodeListSize; i++ {
		//output, err := exutil.DebugNodeWithChroot(oc, nodeList[i], "sysctl", sysctlparm)
		stdOut, err := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodeList[i], []string{"-q"}, "sysctl", sysctlparm)
		conntrackMax := regexpstr.FindString(stdOut)
		e2e.Logf("The value is %v on %v", conntrackMax, nodeList[i])
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stdOut).NotTo(o.ContainSubstring(sysctlparm + " = " + specifiedvalue))
	}

}

// compareSysctlValueOnSepcifiedNodeByName compare the sysctl parameter's value on specified node, it should different than other node
func compareSysctlValueOnSepcifiedNodeByName(oc *exutil.CLI, tunedNodeName, sysctlparm, defaultvalue, specifiedvalue string) {

	nodeList, err := exutil.GetAllNodesbyOSType(oc, "linux")
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeListSize := len(nodeList)

	// tuned nodes should have value of 1048578, others should be 1048576
	regexpstr, _ := regexp.Compile(sysctlparm + ".*")
	for i := 0; i < nodeListSize; i++ {
		//output, err := exutil.DebugNodeWithChroot(oc, nodeList[i], "sysctl", sysctlparm)
		stdOut, err := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodeList[i], []string{"-q"}, "sysctl", sysctlparm)
		actualSysctlKeyValue := regexpstr.FindString(stdOut)
		e2e.Logf("The actual value is %v on %v", actualSysctlKeyValue, nodeList[i])
		o.Expect(err).NotTo(o.HaveOccurred())
		if nodeList[i] != tunedNodeName && len(defaultvalue) == 0 {
			e2e.Logf("The expected value of %v shouldn't be %v on %v", sysctlparm, specifiedvalue, nodeList[i])
			o.Expect(stdOut).NotTo(o.ContainSubstring(sysctlparm + " = " + specifiedvalue))
		} else {
			e2e.Logf("The expected value of %v should be %v on %v", sysctlparm, specifiedvalue, nodeList[i])
			o.Expect(stdOut).To(o.ContainSubstring(sysctlparm + " = " + specifiedvalue))
		}
	}
}

// getTunedPodNamebyNodeName
func getTunedPodNamebyNodeName(oc *exutil.CLI, tunedNodeName, namespace string) string {

	podNames, err := exutil.GetPodName(oc, namespace, "", tunedNodeName)
	o.Expect(err).NotTo(o.HaveOccurred())

	//Get Pod name based on node name, and filter tuned pod name when mulitple pod return on the same node
	regexpstr, err := regexp.Compile(`tuned-.*`)
	o.Expect(err).NotTo(o.HaveOccurred())

	tunedPodName := regexpstr.FindString(podNames)
	e2e.Logf("The Tuned Pod Name is: %v", tunedPodName)
	return tunedPodName
}

type ntoResource struct {
	name        string
	namespace   string
	template    string
	sysctlparm  string
	sysctlvalue string
	priority    int
	label       string
}

func (ntoRes *ntoResource) createTunedProfileIfNotExist(oc *exutil.CLI) {

	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", ntoRes.name, "-n", ntoRes.namespace).Output()
	if strings.Contains(output, "NotFound") || strings.Contains(output, "No resources") || err != nil {
		e2e.Logf(fmt.Sprintf("No tuned in project: %s, create one: %s", ntoRes.namespace, ntoRes.name))
		exutil.CreateNsResourceFromTemplate(oc, ntoRes.namespace, "--ignore-unknown-parameters=true", "-f", ntoRes.template, "-p", "TUNED_NAME="+ntoRes.name, "SYSCTLPARM="+ntoRes.sysctlparm, "SYSCTLVALUE="+ntoRes.sysctlvalue)
	} else {
		e2e.Logf(fmt.Sprintf("Already exist %v in project: %s", ntoRes.name, ntoRes.namespace))
	}
}

func (ntoRes *ntoResource) createDebugTunedProfileIfNotExist(oc *exutil.CLI, isDebug bool) {

	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", ntoRes.name, "-n", ntoRes.namespace).Output()
	if strings.Contains(output, "NotFound") || strings.Contains(output, "No resources") || err != nil {
		e2e.Logf(fmt.Sprintf("No tuned in project: %s, create one: %s", ntoRes.namespace, ntoRes.name))
		exutil.CreateNsResourceFromTemplate(oc, ntoRes.namespace, "--ignore-unknown-parameters=true", "-f", ntoRes.template, "-p", "TUNED_NAME="+ntoRes.name, "SYSCTLPARM="+ntoRes.sysctlparm, "SYSCTLVALUE="+ntoRes.sysctlvalue, "ISDEBUG="+strconv.FormatBool(isDebug))
	} else {
		e2e.Logf(fmt.Sprintf("Already exist %v in project: %s", ntoRes.name, ntoRes.namespace))
	}
}

func (ntoRes *ntoResource) createIRQSMPAffinityProfileIfNotExist(oc *exutil.CLI) {

	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", ntoRes.name, "-n", ntoRes.namespace).Output()
	if strings.Contains(output, "NotFound") || strings.Contains(output, "No resources") || err != nil {
		e2e.Logf(fmt.Sprintf("No tuned in project: %s, create one: %s", ntoRes.namespace, ntoRes.name))
		exutil.CreateNsResourceFromTemplate(oc, ntoRes.namespace, "--ignore-unknown-parameters=true", "-f", ntoRes.template, "-p", "TUNED_NAME="+ntoRes.name, "SYSCTLPARM="+ntoRes.sysctlparm, "SYSCTLVALUE="+ntoRes.sysctlvalue)
	} else {
		e2e.Logf(fmt.Sprintf("Already exist %v in project: %s", ntoRes.name, ntoRes.namespace))
	}
}

func (ntoRes *ntoResource) delete(oc *exutil.CLI) {
	_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoRes.namespace, "tuned", ntoRes.name, "--ignore-not-found").Execute()
}

func (ntoRes *ntoResource) assertTunedProfileApplied(oc *exutil.CLI, workerNodeName string) {

	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {

		appliedStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoRes.namespace, "profiles.tuned.openshift.io", workerNodeName, `-ojsonpath='{.status.conditions[?(@.type=="Applied")].status}'`).Output()
		if err == nil && strings.Contains(appliedStatus, "True") {
			e2e.Logf("Tuned custom profile applied to nodes, the status is %s", appliedStatus)
			//Check if the new profiles name applied on a node
			return true, nil
		}
		e2e.Logf("The profile [ %v ] is not applied on node [ %v ], try next around \n", ntoRes.name, workerNodeName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "New tuned profile isn't applied correctly, please check")
}

func (ntoRes *ntoResource) applyNTOTunedProfile(oc *exutil.CLI) {
	exutil.ApplyNsResourceFromTemplate(oc, ntoRes.namespace, "--ignore-unknown-parameters=true", "-f", ntoRes.template, "-p", "TUNED_PROFILE="+ntoRes.name, "-p", "SYSCTL_NAME="+ntoRes.sysctlparm, "-p", "SYSCTL_VALUE="+ntoRes.sysctlvalue, "-p", "LABEL_NAME="+ntoRes.label)
}

// assertDebugSettings
func assertDebugSettings(oc *exutil.CLI, tunedNodeName string, ntoNamespace string, isDebug string) bool {

	nodeProfile, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("profiles.tuned.openshift.io", tunedNodeName, "-n", ntoNamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	regDebugCheck, err := regexp.Compile(".*Debug:.*" + isDebug)
	o.Expect(err).NotTo(o.HaveOccurred())

	isMatch := regDebugCheck.MatchString(nodeProfile)
	loglines := regDebugCheck.FindAllString(nodeProfile, -1)
	e2e.Logf("The result is: %v", loglines[0])
	return isMatch
}

func getDefaultSMPAffinityBitMaskbyCPUCores(oc *exutil.CLI, workerNodeName string) string {
	//Get CPU number in specified worker nodes
	cpuCoresStdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", workerNodeName, "-ojsonpath={.status.capacity.cpu}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(cpuCoresStdOut).NotTo(o.BeEmpty())

	cpuCores, err := strconv.Atoi(cpuCoresStdOut)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(cpuCoresStdOut).NotTo(o.BeEmpty())

	cpuHexMask := make([]byte, 0, 2)
	if cpuCores%4 != 0 {

		modCPUCoresby4 := int(cpuCores % 4)
		var cpuCoresMask int
		switch modCPUCoresby4 {
		case 3:
			cpuCoresMask = 7
		case 2:
			cpuCoresMask = 3
		case 1:
			cpuCoresMask = 1
		}
		cpuHexMask = append(cpuHexMask, byte(cpuCoresMask))
	}

	for i := 0; i < cpuCores/4; i++ {
		cpuHexMask = append(cpuHexMask, 15)
	}

	cpuHexMaskStr := fmt.Sprintf("%x", cpuHexMask)
	cpuHexMaskFmt := strings.ReplaceAll(cpuHexMaskStr, "0", "")
	e2e.Logf("There are %v cores on worker node %v, the hex mask is %v\n", cpuHexMaskFmt)

	return cpuHexMaskFmt
}

func convertCPUBitMaskToByte(cpuHexMask string) []byte {

	cpuHexMaskChars := []rune(cpuHexMask)
	cpuBitsMask := make([]byte, 0)
	cpuNum := 0

	for i := 0; i < len(cpuHexMaskChars); i++ {

		switch cpuHexMaskChars[i] {
		case 'f':
			cpuBitsMask = append(cpuBitsMask, 15)
			cpuNum = cpuNum + 4
		case '7':
			cpuBitsMask = append(cpuBitsMask, 7)
			cpuNum = cpuNum + 3
		case '3':
			cpuBitsMask = append(cpuBitsMask, 3)
			cpuNum = cpuNum + 2
		case '1':
			cpuBitsMask = append(cpuBitsMask, 1)
			cpuNum = cpuNum + 1
		}

	}

	e2e.Logf("The total CPU number is %v\nThe CPU HexMask is:\n%s\nThe CPU BitsMask is:\n%b\n", cpuNum, cpuHexMask, cpuBitsMask)
	return cpuBitsMask
}

func convertIsolatedCPURange2CPUList(isolatedCPURange string) []byte {

	//Get a separated cpu number list
	cpuList := make([]byte, 0, 8)
	//From [1,2,4-5,12-17,24-28,30-32]
	//To   [1 2 4 5 12 13 14 15 16 17 24 25 26 27 28 30 31 32]

	cpuRangeList := strings.Split(isolatedCPURange, ",")

	for i := 0; i < len(cpuRangeList); i++ {
		//if CPU range is 12-17 which contain "-"

		if strings.Contains(cpuRangeList[i], "-") {

			//Ignore such senario when cpu setting as 45-,-46
			if strings.HasPrefix(cpuRangeList[i], "-") {
				continue
			}
			//startCPU is 12
			//endCPU is 17
			//the CPU range must be two numbers
			cpuRange := strings.Split(cpuRangeList[i], "-")
			endCPU, _ := strconv.Atoi(cpuRange[1])
			startCPU, _ := strconv.Atoi(cpuRange[0])
			for i := 0; i <= endCPU-startCPU; i++ {
				cpus := startCPU + i
				cpuList = append(cpuList, byte(cpus))
			}

		} else {
			cpus, _ := strconv.Atoi(cpuRangeList[i])

			//Ignore 1,2,<no number>
			if len(cpuRangeList[i]) != 0 {
				cpuList = append(cpuList, byte(cpus))
			}
		}
	}
	return cpuList
}

func assertIsolateCPUCoresAffectedBitMask(cpuBitsMask []byte, isolatedCPU []byte) string {

	//Isolated CPU Range, 0,1,3-4,11-16,23-27
	//          27 26 25 24 ---------------------------------3 2 1 0
	//          27%6=3
	//[1111     1111         1111 1111 1111 1111 1111         1111] cpuBitMask
	//[0000     1111         1000 0001 1111 1000 0001         1011] isolatedCPU
	//--------------------------------------------------------------
	//[1111     0000         0111 1110 0000 0111 1110         0100] affinityCPUMask
	//  0         1            2    3   4     5   6             7    cpuBitMaskGroupsIndex
	//            6            5    4   3     2   1             0    isolatedCPUIndex
	//     maxValueOfIsolatedCPUIndex
	var affinityCPUMask string
	totalCPUBitMaskGroups := len(cpuBitsMask)
	totalIsolatedCPUNum := len(isolatedCPU)

	e2e.Logf("The total isolated CPUs is: %v\n", totalIsolatedCPUNum)
	e2e.Logf("The max CPU that isolated is : %v\n", int(isolatedCPU[totalIsolatedCPUNum-1]))

	//The max CPU number is 27, Index is 15
	maxValueOfIsolatedCPUIndex := int(isolatedCPU[totalIsolatedCPUNum-1]) / 4
	e2e.Logf("totalCPUGroupNum is: %v\nmaxCPUGroupIndex is: %v\n", totalCPUBitMaskGroups, maxValueOfIsolatedCPUIndex)
	maxValueOfCPUBitMaskGroupsIndex := totalCPUBitMaskGroups - 1
	for i := totalIsolatedCPUNum - 1; i >= 0; i-- {

		isolatedCPUIndex := int(isolatedCPU[i]) / 4

		cpuBitsMaskIndex := maxValueOfCPUBitMaskGroupsIndex - isolatedCPUIndex
		// 3 => 1000 2=>0100 1=>0010 0=>0000
		modIsolatedCPUby4 := int(isolatedCPU[i] % 4)
		var isolatedCPUMask int
		switch modIsolatedCPUby4 {
		case 3:
			isolatedCPUMask = 8
		case 2:
			isolatedCPUMask = 4
		case 1:
			isolatedCPUMask = 2
		case 0:
			isolatedCPUMask = 1
		}

		valueOfCPUBitsMaskOnIndex := int(cpuBitsMask[cpuBitsMaskIndex]) ^ isolatedCPUMask
		e2e.Logf("%04b ^ %04b = %04b\n", cpuBitsMask[cpuBitsMaskIndex], isolatedCPUMask, valueOfCPUBitsMaskOnIndex)
		cpuBitsMask[cpuBitsMaskIndex] = byte(valueOfCPUBitsMaskOnIndex)
	}
	cpuBitsMaskStr := fmt.Sprintf("%x", cpuBitsMask)
	affinityCPUMask = strings.ReplaceAll(cpuBitsMaskStr, "0", "")
	e2e.Logf("affinityCPUMask is: %s\n", affinityCPUMask)
	return affinityCPUMask
}

func assertDefaultIRQSMPAffinityAffectedBitMask(cpuBitsMask []byte, isolatedCPU []byte, defaultIRQSMPAffinity string) bool {

	//Isolated CPU Range, 0,1,3-4,11-16,23-27
	//          27 26 25 24 ---------------------------------3 2 1 0
	//          27%6=3
	//[1111     1111         1111 1111 1111 1111 1111         1111] cpuBitMask
	//[0000     1111         1000 0001 1111 1000 0001         1011] isolatedCPU
	//--------------------------------------------------------------
	//[0000     1111         1000 0001 1111 1000 0001         1011] affinityCPUMask
	//  0         1            2    3   4     5   6             7    cpuBitMaskGroupsIndex
	//            6            5    4   3     2   1             0    isolatedCPUIndex
	//     maxValueOfIsolatedCPUIndex

	var affinityCPUMask string
	var isMatch bool
	totalCPUBitMaskGroups := len(cpuBitsMask)
	totalIsolatedCPUNum := len(isolatedCPU)

	e2e.Logf("The total isolated CPUs is: %v\n", totalIsolatedCPUNum)
	e2e.Logf("The max CPU that isolated is : %v\n", int(isolatedCPU[totalIsolatedCPUNum-1]))
	isolatedCPUMaskGroup := make([]byte, 0, 8)

	for i := 0; i < totalCPUBitMaskGroups; i++ {
		//Initial all bits to zero of isolatedCPUMask first
		isolatedCPUMaskGroup = append(isolatedCPUMaskGroup, byte(int(cpuBitsMask[i])&0))
	}

	e2e.Logf("The initial isolatedCPUMask is %04b\n", isolatedCPUMaskGroup)

	maxValueOfCPUBitMaskGroupsIndex := totalCPUBitMaskGroups - 1
	for i := totalIsolatedCPUNum - 1; i >= 0; i-- {

		isolatedCPUIndex := int(isolatedCPU[i]) / 4

		cpuBitsMaskIndex := maxValueOfCPUBitMaskGroupsIndex - isolatedCPUIndex

		// 3 => 1000 2=>0100 1=>0010 0=>0000
		modIsolatedCPUby4 := int(isolatedCPU[i] % 4)
		var isolatedCPUMask int
		switch modIsolatedCPUby4 {
		case 3:
			isolatedCPUMask = 8
		case 2:
			isolatedCPUMask = 4
		case 1:
			isolatedCPUMask = 2
		case 0:
			isolatedCPUMask = 1
		}

		e2e.Logf("%04b | %04b = %04b\n", isolatedCPUMaskGroup[cpuBitsMaskIndex], isolatedCPUMask, int(isolatedCPUMaskGroup[cpuBitsMaskIndex])|isolatedCPUMask)
		valueOfCPUBitsMaskOnIndex := int(isolatedCPUMaskGroup[cpuBitsMaskIndex]) | isolatedCPUMask
		isolatedCPUMaskGroup[cpuBitsMaskIndex] = byte(valueOfCPUBitsMaskOnIndex)

	}

	//Remove additional 0 in the isolatedCPUMaskGroup
	e2e.Logf("cpuBitsMask is: %04b\n", isolatedCPUMaskGroup)
	cpuBitsMaskStr := fmt.Sprintf("%x", isolatedCPUMaskGroup)
	cpuBitsMaskRune := []rune(cpuBitsMaskStr)
	bitsMaskChars := make([]byte, 0, 2)

	for i := 1; i < len(cpuBitsMaskRune); i = i + 2 {
		bitsMaskChars = append(bitsMaskChars, byte(cpuBitsMaskRune[i]))
	}
	affinityCPUMask = string(bitsMaskChars)

	//If defaultIRQSMPAffinity start with 0, ie, 00020, remove 000 and change to 20
	if strings.HasPrefix(defaultIRQSMPAffinity, "0") || strings.HasPrefix(affinityCPUMask, "0") {
		defaultIRQSMPAffinity = strings.TrimLeft(defaultIRQSMPAffinity, "0")
		affinityCPUMask = strings.TrimLeft(affinityCPUMask, "0")
	}

	e2e.Logf("affinityCPUMask is: -%s-, defaultIRQSMPAffinity is -%s-\n", affinityCPUMask, defaultIRQSMPAffinity)
	if affinityCPUMask == defaultIRQSMPAffinity {
		isMatch = true
	}
	return isMatch
}

// AssertTunedAppliedMC Check if customed tuned applied via MCP
func AssertTunedAppliedMC(oc *exutil.CLI, mcNamePrefix string, filter string) {

	mcNameList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mc", "--no-headers", "-oname").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The name of mcName is: %v", mcNameList)

	mcNameReg, _ := regexp.Compile(".*" + mcNamePrefix)
	mcName := mcNameReg.FindAllString(mcNameList, -1)
	e2e.Logf("The expected names of mcName is: %v", mcName)

	mcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mcName[0], "-oyaml").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(mcOutput).To(o.ContainSubstring(filter))

	//Print machineconfig content by filter
	mccontent, _ := regexp.Compile(".*" + filter + ".*")
	contentLines := mccontent.FindAllString(mcOutput, -1)
	e2e.Logf("The result is: %v", contentLines[0])
	o.Expect(mcOutput).To(o.ContainSubstring(filter))
}

// AssertTunedAppliedToNode Check if customed tuned applied to a certain node
func AssertTunedAppliedToNode(oc *exutil.CLI, tunedNodeName string, filter string) bool {

	cmdLineOutput, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, tunedNodeName, []string{"-q"}, "cat", "/proc/cmdline")
	o.Expect(err).NotTo(o.HaveOccurred())
	var isMatch bool
	if strings.Contains(cmdLineOutput, filter) {
		//Print machineconfig content by filter
		cmdLineReg, _ := regexp.Compile(".*" + filter + ".*")
		contentLines := cmdLineReg.FindAllString(cmdLineOutput, -1)
		e2e.Logf("The result is: %v", contentLines[0])
		isMatch = true
	} else {
		e2e.Logf("the result mismatch the filter: %v", filter)
		isMatch = false
	}
	return isMatch
}

// assertNTOPodLogsLastLines     s
func assertNTOPodLogsLastLines(oc *exutil.CLI, namespace string, ntoPod string, lineN string, timeDurationSec int, filter string) {

	err := wait.Poll(15*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {

		//Remove err assert for SNO, the OCP will can not access temporily when master node restart or certificate key removed
		ntoPodLogs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", namespace, ntoPod, "--tail="+lineN).Output()

		regNTOPodLogs, err := regexp.Compile(".*" + filter + ".*")
		o.Expect(err).NotTo(o.HaveOccurred())
		isMatch := regNTOPodLogs.MatchString(ntoPodLogs)
		if isMatch {
			loglines := regNTOPodLogs.FindAllString(ntoPodLogs, -1)
			e2e.Logf("The logs of nto pod %v is: \n%v", ntoPod, loglines[0])
			return true, nil
		}
		e2e.Logf("The keywords of nto pod isn't found, try next ...")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The tuned pod's log doesn't contain the keywords, please check")
}

// getServiceENDPoint
func getServiceENDPoint(oc *exutil.CLI, namespace string) string {
	var endPointIP string
	ipFamilyPolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "service/node-tuning-operator", "-ojsonpath={.spec.ipFamilyPolicy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(ipFamilyPolicy).NotTo(o.BeEmpty())

	serviceOutput, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", namespace, "service/node-tuning-operator").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(serviceOutput).NotTo(o.BeEmpty())

	endPointReg, _ := regexp.Compile(".*Endpoints.*")
	endPointIPStr := endPointReg.FindString(serviceOutput)
	o.Expect(endPointIPStr).NotTo(o.BeEmpty())

	if ipFamilyPolicy == "SingleStack" {
		endPointStr := strings.ReplaceAll(endPointIPStr, "         ", ",")
		o.Expect(endPointStr).NotTo(o.BeEmpty())

		endPointStrArr := strings.Split(endPointStr, ",")
		o.Expect(endPointStrArr).NotTo(o.BeEmpty())

		endPointStrArrLen := len(endPointStrArr)
		endPointIP = endPointStrArr[endPointStrArrLen-1]

	} else {
		endPointIPStrNoSpace := strings.ReplaceAll(endPointIPStr, " ", "")
		o.Expect(endPointIPStrNoSpace).NotTo(o.BeEmpty())

		endPointIPArr := strings.Split(endPointIPStrNoSpace, ":")
		o.Expect(endPointIPArr).NotTo(o.BeEmpty())

		endPointIP = endPointIPArr[1] + ":" + endPointIPArr[2]
	}

	return endPointIP
}

// AssertNTOCertificateRotate used for check if NTO certificate rotate
func AssertNTOCertificateRotate(oc *exutil.CLI, ntoNamespace string, tunedNodeName string, encodeBase64OpenSSLOutputBefore string, encodeBase64OpenSSLExpireDateBefore string) {

	metricEndpoint := getServiceENDPoint(oc, ntoNamespace)
	err := wait.Poll(15*time.Second, 300*time.Second, func() (bool, error) {

		openSSLOutputAfter, err := exutil.DebugNodeWithOptions(oc, tunedNodeName, []string{"--quiet=true"}, "/bin/bash", "-c", "/bin/openssl s_client -connect "+metricEndpoint+" 2>/dev/null </dev/null")
		o.Expect(err).NotTo(o.HaveOccurred())

		openSSLExpireDateAfter, err := exutil.DebugNodeWithOptions(oc, tunedNodeName, []string{"--quiet=true"}, "/bin/bash", "-c", "/bin/openssl s_client -connect "+metricEndpoint+" 2>/dev/null </dev/null  | /bin/openssl x509 -noout -dates")
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("The openSSL Expired Date information of NTO openSSL after rotate as below: \n%v", openSSLExpireDateAfter)

		encodeBase64OpenSSLOutputAfter := exutil.StringToBASE64(openSSLOutputAfter)
		encodeBase64OpenSSLExpireDateAfter := exutil.StringToBASE64(openSSLExpireDateAfter)

		if encodeBase64OpenSSLOutputBefore != encodeBase64OpenSSLOutputAfter && encodeBase64OpenSSLExpireDateBefore != encodeBase64OpenSSLExpireDateAfter {
			e2e.Logf("The certificate has been updated ...")
			return true, nil
		}
		e2e.Logf("The certificate isn't updated, try next round ...")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The NTO certificate isn't rotate, please check")
}

// compareCertificateBetweenOpenSSLandTLSSecret
func compareCertificateBetweenOpenSSLandTLSSecret(oc *exutil.CLI, ntoNamespace string, tunedNodeName string) {

	metricEndpoint := getServiceENDPoint(oc, ntoNamespace)
	err := wait.Poll(15*time.Second, 180*time.Second, func() (bool, error) {

		//Extract certificate from openssl that nto operator service endpoint
		openSSLOutputAfter, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "/bin/bash", "-c", "/bin/openssl s_client -connect "+metricEndpoint+" 2>/dev/null </dev/null | sed -ne '/-BEGIN CERTIFICATE-/,/-END CERTIFICATE-/p'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Extract tls.crt from secret node-tuning-operator-tls
		encodeBase64tlsCertOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "secret", "node-tuning-operator-tls", `-ojsonpath='{ .data.tls\.crt }'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		tmpTLSCertOutput := strings.Trim(encodeBase64tlsCertOutput, "'")
		tlsCertOutput := exutil.BASE64DecodeStr(tmpTLSCertOutput)

		if strings.Contains(tlsCertOutput, openSSLOutputAfter) {
			e2e.Logf("The certificate is the same ...")
			return true, nil
		}
		e2e.Logf("The certificate is different, try next round ...")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The certificate is different, please check")
}

// assertIFChannel
func assertIFChannelQueuesStatus(oc *exutil.CLI, namespace string, tunedNodeName string) bool {

	var isMatch bool
	findStr := fmt.Sprintf(`find /sys/class/net -type l -not -lname *virtual* -a -not -name enP* -printf %%f"\n"`)
	ifNameList, _, err := exutil.DebugNodeRetryWithOptionsAndChrootWithStdErr(oc, tunedNodeName, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", findStr)
	//ifNameList, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", namespace, "--quiet=true", "node/"+tunedNodeName, "--", "find", "/sys/class/net", "-type", "l", "-not", "-lname", "*virtual*", "-a", "-not", "-name", "enP*", "-printf", `%f"\n"`).Output()
	e2e.Logf("Physical network list is: %v", ifNameList)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(ifNameList).NotTo(o.BeEmpty())

	//Remove double quotes
	ifNameStr := strings.ReplaceAll(ifNameList, "\"", "")
	o.Expect(ifNameStr).NotTo(o.BeEmpty())
	//Check all physical nic
	ifNames := strings.Split(ifNameStr, "\n")
	e2e.Logf("ifNames is: %v", ifNames)
	o.Expect(ifNames).NotTo(o.BeEmpty())

	for i := 0; i < len(ifNames); {
		if len(ifNames[i]) > 0 {
			ethToolsOutput, _, err := exutil.DebugNodeRetryWithOptionsAndChrootWithStdErr(oc, tunedNodeName, []string{"--quiet=true", "--to-namespace=" + namespace}, "ethtool", "-l", ifNames[i])
			//ethToolsOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", namespace, "--quiet=true", "node/"+tunedNodeName, "--", "ethtool", "-l", ifNames[i]).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ethToolsOutput).NotTo(o.BeEmpty())
			e2e.Logf("ethtool -l %v:, \n%v", ifNames[i], ethToolsOutput)

			regChannel, err := regexp.Compile("Combined:.*1")
			o.Expect(err).NotTo(o.HaveOccurred())
			isMatch = regChannel.MatchString(ethToolsOutput)
			if isMatch {
				break
			}
		}
		i++
	}
	return isMatch
}

// compareSpecifiedValueByNameOnLabelNodewithRetry
func compareSpecifiedValueByNameOnLabelNodewithRetry(oc *exutil.CLI, ntoNamespace, nodeName, sysctlparm, specifiedvalue string) {

	err := wait.Poll(15*time.Second, 180*time.Second, func() (bool, error) {

		sysctlOutput, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, nodeName, []string{"--quiet=true", "--to-namespace=" + ntoNamespace}, "sysctl", sysctlparm)
		e2e.Logf("The actual value is [ %v ] on %v", sysctlOutput, nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())

		regexpstr, _ := regexp.Compile(sysctlparm + " = " + specifiedvalue)
		matchStr := regexpstr.FindString(sysctlOutput)
		e2e.Logf("The match value is [ %v ] on %v", matchStr, nodeName)

		isMatch := regexpstr.MatchString(sysctlOutput)
		if isMatch {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The certificate is different, please check")
}

// skipDeployPAO
func skipDeployPAO(oc *exutil.CLI) bool {

	skipPAO := true
	clusterVersion, _, err := exutil.GetClusterVersion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Cluster Version: %v", clusterVersion)
	paoDeployOCPVersionList := []string{"4.6", "4.7", "4.8", "4.9", "4.10"}

	for _, v := range paoDeployOCPVersionList {
		if strings.Contains(clusterVersion, v) {
			skipPAO = false
			break
		}
	}
	return skipPAO
}

// assertIOTimeOutandMaxRetries
func assertIOTimeOutandMaxRetries(oc *exutil.CLI, ntoNamespace string) {

	nodeList, err := exutil.GetAllNodesbyOSType(oc, "linux")
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeListSize := len(nodeList)

	for i := 0; i < nodeListSize; i++ {
		timeoutOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+nodeList[i], "--", "chroot", "/host", "cat", "/sys/module/nvme_core/parameters/io_timeout").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The value of io_timeout is : %v on node %v", timeoutOutput, nodeList[i])
		o.Expect(timeoutOutput).To(o.ContainSubstring("4294967295"))
	}
}

// confirmedTunedReady
func confirmedTunedReady(oc *exutil.CLI, ntoNamespace string, tunedName string, timeDurationSec int) {

	err := wait.Poll(10*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {

		tunedStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if strings.Contains(tunedStatus, tunedName) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "tuned is not ready")
}

// switchThrottlectlOnOff
func switchThrottlectlOnOff(oc *exutil.CLI, ntoNamespace, tunedNodeName string, throttlectlState string, timeDurationSec int) {

	err := wait.Poll(10*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {

		//_, err := exutil.DebugNodeWithChroot(oc, tunedNodeName, "/usr/bin/throttlectl", throttlectlState)
		err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "host", "/usr/bin/throttlectl", throttlectlState).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		//schedRTRuntimeStatus, err := exutil.DebugNode(oc, tunedNodeName, "cat", "/proc/sys/kernel/sched_rt_runtime_us")
		schedRTRuntimeStatus, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "cat", "/proc/sys/kernel/sched_rt_runtime_us").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Sleep 10s each time and retry two times to improve sucessful rate of restarting stalld
		if strings.Contains(schedRTRuntimeStatus, "-1") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "throttlectl status isn't correct, retry")
}

// assertCProccessNOTInCgroupSchedulerBlacklist
func assertProcessNOTInCgroupSchedulerBlacklist(oc *exutil.CLI, tunedNodeName string, namespace string, processFilter string, nodeCPUCores int) bool {

	//pIDCpusAllowedList, err := exutil.RemoteShPodWithBash(oc, namespace, tunedPodName, "grep ^Cpus_allowed_list /proc/`pgrep "+processFilter+"`/status")
	pIDCpusAllowedList, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", namespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "grep ^Cpus_allowed_list /proc/`pgrep "+processFilter+"`/status").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Actually Process's Cpus_allowed_list in /proc/$PID/status on worker nodes is: \n%v", pIDCpusAllowedList)

	//CPU = 2
	//The CPU Allow List of Process In Cgroup Blacklist is 0-1(N is CPU Cores -1 )
	//The CPU Allow List of Process Not In Cgroup Blacklist is 0

	//CPU > 2
	//The CPU Allow List of Process In Cgroup Blacklist is 0-N(N is CPU Cores -1 )
	//The CPU Allow List of Process Not In Cgroup Blacklist is 0,2-N
	nodeCPUCores = nodeCPUCores - 1
	e2e.Logf("Expected Process's Cpus_allowed_list in /proc/$PID/status on worker nodes is: \n%v", "Cpus_allowed_list:	0 or 0,2-"+strconv.Itoa(nodeCPUCores))

	regPIDCpusAllowedList0, err := regexp.Compile(`.*0$`)
	o.Expect(err).NotTo(o.HaveOccurred())

	regPIDCpusAllowedList1, err := regexp.Compile(`.*0,2-.*`)
	o.Expect(err).NotTo(o.HaveOccurred())

	isMatch0 := regPIDCpusAllowedList0.MatchString(pIDCpusAllowedList)
	isMatch1 := regPIDCpusAllowedList1.MatchString(pIDCpusAllowedList)

	e2e.Logf("Match cgroup Cpus_allowed_list for process %v is: %v", processFilter, isMatch0 || isMatch1)
	return isMatch0 || isMatch1
}

// assertCpusAllowedListNOTInCgroupSchedulerBlacklist
func assertProcessInCgroupSchedulerBlacklist(oc *exutil.CLI, tunedNodeName string, namespace string, processFilter string, nodeCPUCores int) bool {

	pIDCpusAllowedList, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", namespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "grep ^Cpus_allowed_list /proc/`pgrep "+processFilter+"`/status").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Actually Process's Cpus_allowed_list in /proc/$PID/status on worker nodes is: \n%v", pIDCpusAllowedList)

	//CPU = 2
	//The CPU Allow List of Process In Cgroup Blacklist is 0-1
	//The CPU Allow List of Process Not In Cgroup Blacklist is 0

	//CPU > 2
	//The CPU Allow List of Process In Cgroup Blacklist is 0-N(N is CPU Cores -1 )
	//The CPU Allow List of Process Not In Cgroup Blacklist is 0,2-N(N is CPU Cores -1 )
	nodeCPUCores = nodeCPUCores - 1
	e2e.Logf("Expected Process's Cpus_allowed_list in /proc/$PID/status on worker nodes is: \n%v", "Cpus_allowed_list:	0-"+strconv.Itoa(nodeCPUCores))
	regPIDCpusAllowedList, err := regexp.Compile(".*0-" + strconv.Itoa(nodeCPUCores))
	o.Expect(err).NotTo(o.HaveOccurred())

	isMatch := regPIDCpusAllowedList.MatchString(pIDCpusAllowedList)

	e2e.Logf("Match cgroup Cpus_allowed_list for process %v is: %v", processFilter, isMatch)
	return isMatch
}

// getNTOOperatorPodName retrun NTO operator POD name
func getNTOOperatorPodName(oc *exutil.CLI, namespace string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "pods", "-lname=cluster-node-tuning-operator", "-ojsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podName).NotTo(o.BeEmpty())
	return podName
}

// assertCONodeTuningStatusWithoutWARNWithRetry sometime the WARN messages disappear with delay, so need to retry checking
func assertCONodeTuningStatusWithoutWARNWithRetry(oc *exutil.CLI, timeDurationSec int, filter string) {

	err := wait.Poll(time.Duration(timeDurationSec/10)*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {

		coNodeTuningStdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/node-tuning").Output()
		if err != nil {
			e2e.Logf("the status of co/node-tuning abnormal, please check")
			return false, err
		}

		regCONodeTuningStdOut, err := regexp.Compile(".*" + filter + ".*")
		if err != nil {
			e2e.Logf("Un-supported filter %v, please check", filter)
			return false, err
		}

		isMatch := regCONodeTuningStdOut.MatchString(coNodeTuningStdOut)
		if isMatch {
			loglines := regCONodeTuningStdOut.FindAllString(coNodeTuningStdOut, -1)
			e2e.Logf("The status of co/node-tuning is:%v \n%v\n", coNodeTuningStdOut, loglines[0])
			e2e.Logf("The keywords of co/node-tuning still found, try next ...")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "The checking of co/node-tuning met with unexpected error, please check")
}

// getValueOfSysctlByName parses out the line determining sysctl in the kernel
func getValueOfSysctlByName(oc *exutil.CLI, ntoNamespace, tunedNodeName, sysctlparm string) string {

	var sysctlValue string
	defaultValues, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "sysctl", sysctlparm).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(defaultValues).NotTo(o.BeEmpty())
	sysctlArray := strings.Split(defaultValues, "=")
	sysctlLen := len(sysctlArray)
	if sysctlLen == 2 {
		sysctlValue = strings.TrimSpace(sysctlArray[1])
	}
	return sysctlValue
}

// assertNTOCustomProfileStatus return correct profile status
func assertNTOCustomProfileStatus(oc *exutil.CLI, ntoNamespace string, tunedNodeName string, expectedProfile string, expectedAppliedStatus string, expectedDegradedStatus string) bool {

	currentProfile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io", tunedNodeName, `-ojsonpath={.status.tunedProfile}`).Output()
	currentProfile = strings.Trim(currentProfile, "'")
	e2e.Logf("currentProfile is %v", currentProfile)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(currentProfile).NotTo(o.BeEmpty())

	appliedStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io", tunedNodeName, `-ojsonpath='{.status.conditions[?(@.type=="Applied")].status}'`).Output()
	appliedStatus = strings.Trim(appliedStatus, "'")
	e2e.Logf("appliedStatus is %v", appliedStatus)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(appliedStatus).NotTo(o.BeEmpty())

	degradedStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io", tunedNodeName, `-ojsonpath='{.status.conditions[?(@.type=="Degraded")].status}'`).Output()
	degradedStatus = strings.Trim(degradedStatus, "'")
	e2e.Logf("degradedStatus is %v", degradedStatus)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(degradedStatus).NotTo(o.BeEmpty())

	return appliedStatus == expectedAppliedStatus && degradedStatus == expectedDegradedStatus && currentProfile == expectedProfile
}

func isROSAHostedCluster(oc *exutil.CLI) bool {

	var clusterType string
	//Check if it's ROSA hosted cluster
	sharedDir := os.Getenv("SHARED_DIR")
	if len(sharedDir) != 0 {
		fmt.Println("SHARED_DIR was found ")
		byteArray, err := ioutil.ReadFile(sharedDir + "/cluster-type")
		if err != nil {
			clusterType = ""
		} else {
			clusterType = string(byteArray)
			clusterType = strings.ToLower(clusterType)
		}

	}
	return strings.Contains(clusterType, "rosa")
}

func getFirstMasterNodeName(oc *exutil.CLI) string {
	var firstMasterNodeName string
	//ibmcloud don't have {.items[*].status.addresses[?(@.type=="Hostname")].address}'
	masterNodeNamesStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l node-role.kubernetes.io/control-plane=", `-oname`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(masterNodeNamesStr).NotTo(o.BeEmpty())
	masterNodeNamesArray := strings.Split(masterNodeNamesStr, "\n")

	if len(masterNodeNamesArray) > 0 {
		firstMasterNodeNameArr := strings.Split(masterNodeNamesArray[0], "/")
		if len(firstMasterNodeNameArr) > 1 {
			firstMasterNodeName = firstMasterNodeNameArr[1]
		}
	}
	return firstMasterNodeName
}

func getDefaultProfileNameOnMaster(oc *exutil.CLI, masterNodeName string) string {

	var defaultProfileName string

	defaultProfileName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-node-tuning-operator", "profiles.tuned.openshift.io", masterNodeName, "-ojsonpath={.status.tunedProfile}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(defaultProfileName).NotTo(o.BeEmpty())

	e2e.Logf("defaultProfileName is %v on %v ", defaultProfileName, masterNodeName)
	return defaultProfileName
}

func assertCoStatusWithKeywords(oc *exutil.CLI, keywords string) {

	o.Eventually(func() bool {
		coStatus, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Output()
		if err1 != nil || !strings.Contains(coStatus, keywords) {
			e2e.Logf("failed to find the keywords, the status of co is %s , check again", coStatus)
		}
		return strings.Contains(coStatus, keywords)
	}, 60*time.Second, time.Second).Should(o.BeTrue())
}

func getWorkerMachinesetName(oc *exutil.CLI, machineseetSN int) string {

	var (
		machinesetName  string
		linuxMachineset = make([]string, 0, 3)
	)
	machinesetList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-machine-api", "machineset", "-ojsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	//workerMachineSets := strings.ReplaceAll(machinesetList, " ", "\n")
	workerMachineSets := strings.Split(machinesetList, " ")
	e2e.Logf("workerMachineSets is %v in getWorkerMachinesetName", workerMachineSets)

	if len(workerMachineSets) > 0 {
		for i := 0; i < len(workerMachineSets); i++ {
			//Skip windows worker node
			// if (!strings.Contains(workerMachineSets[i], "windows") || !strings.Contains(workerMachineSets[i], "edge")) && strings.Contains(workerMachineSets[i], "worker") {
			if strings.Contains(workerMachineSets[i], "windows") || strings.Contains(workerMachineSets[i], "edge") {
				e2e.Logf("skip windows or egde node [ %v ] in getWorkerMachinesetName", workerMachineSets[i])
			} else {
				linuxMachineset = append(linuxMachineset, workerMachineSets[i])
			}
		}
		e2e.Logf("linuxMachineset is %v in getWorkerMachinesetName", linuxMachineset)
		if machineseetSN < len(linuxMachineset) {
			machinesetName = linuxMachineset[machineseetSN]
		}
	}

	e2e.Logf("machinesetName is %v in getWorkerMachinesetName", machinesetName)
	return machinesetName
}

func choseOneWorkerNodeNotByMachineset(oc *exutil.CLI, choseBy int) string {
	//0 means the first worker node, 1 means the last worker node
	var tunedNodeName string
	var err error
	if choseBy == 0 {
		tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
		e2e.Logf("the tunedNodeName that we get inside choseOneWorkerNodeNotByMachineset when choseBy 0 is %v ", tunedNodeName)
		o.Expect(tunedNodeName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
	} else if choseBy == 1 {
		tunedNodeName, err = exutil.GetLastLinuxWorkerNode(oc)
		e2e.Logf("the tunedNodeName that we get inside choseOneWorkerNodeNotByMachineset when choseBy 1 is %v ", tunedNodeName)
		o.Expect(tunedNodeName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		e2e.Logf("Invalid parameter for choseBy is %v ", choseBy)
	}
	return tunedNodeName
}

func choseOneWorkerNodeToRunCase(oc *exutil.CLI, choseBy int) string {
	//Prior to choose worker nodes with machineset
	var tunedNodeName string
	if exutil.IsMachineSetExist(oc) {
		machinesetName := getWorkerMachinesetName(oc, choseBy)
		e2e.Logf("machinesetName is %v in choseOneWorkerNodeToRunCase", machinesetName)

		if len(machinesetName) != 0 {
			machinesetReplicas := exutil.GetRelicasByMachinesetName(oc, machinesetName)
			if !strings.Contains(machinesetReplicas, "0") {
				tunedNodeName = exutil.GetNodeNameByMachineset(oc, machinesetName)
				o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			} else {
				tunedNodeName = choseOneWorkerNodeNotByMachineset(oc, choseBy)
			}
		} else {
			tunedNodeName = choseOneWorkerNodeNotByMachineset(oc, choseBy)
		}
	} else {
		tunedNodeName = choseOneWorkerNodeNotByMachineset(oc, choseBy)
		e2e.Logf("the tunedNodeName that we get inside choseOneWorkerNodeToRunCase when choseBy %v is %v ", choseBy, tunedNodeName)
	}
	return tunedNodeName
}

func getTotalLinuxMachinesetNum(oc *exutil.CLI) int {

	var (
		machinesetNum   int
		linuxMachineset = make([]string, 0, 3)
	)
	machinesetList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-machine-api", "machineset", "-ojsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	//workerMachineSets := strings.ReplaceAll(machinesetList, " ", "\n")
	workerMachineSets := strings.Split(machinesetList, " ")
	e2e.Logf("workerMachineSets is %v in getWorkerMachinesetName", workerMachineSets)

	if len(workerMachineSets) > 0 {
		for i := 0; i < len(workerMachineSets); i++ {
			//Skip windows worker node
			// if (!strings.Contains(workerMachineSets[i], "windows") || !strings.Contains(workerMachineSets[i], "edge")) && strings.Contains(workerMachineSets[i], "worker") {
			if strings.Contains(workerMachineSets[i], "windows") || strings.Contains(workerMachineSets[i], "edge") {
				e2e.Logf("skip windows or egde node [ %v ] in getWorkerMachinesetName", workerMachineSets[i])
			} else {
				linuxMachineset = append(linuxMachineset, workerMachineSets[i])
			}
		}
		e2e.Logf("linuxMachineset is %v in getWorkerMachinesetName", linuxMachineset)
		machinesetNum = len(linuxMachineset)

	}
	e2e.Logf("machinesetNum is %v in getWorkerMachinesetName", machinesetNum)
	return machinesetNum
}
