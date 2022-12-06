package hypernto

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// isHyperNTOPodInstalled will return true if any pod is found in the given namespace, and false otherwise
func isHyperNTOPodInstalled(oc *exutil.CLI, hostedClusterName string) bool {

	e2e.Logf("Checking if pod is found in namespace %s...", hostedClusterName)
	deploymentList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", hostedClusterName, "-oname").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	deployNamesReg := regexp.MustCompile("cluster-node-tuning-operator")
	isNTOInstalled := deployNamesReg.MatchString(deploymentList)
	if !isNTOInstalled {
		e2e.Logf("No pod found in namespace %s :(", hostedClusterName)
		return false
	}
	e2e.Logf("Pod found in namespace %s!", hostedClusterName)
	return true
}

// getNodePoolNamebyHostedClusterName used to get nodepool name in clusters
func getNodePoolNamebyHostedClusterName(oc *exutil.CLI, hostedClusterName, hostedClusterNS string) string {

	nodePoolNameList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodepool", "-n", hostedClusterNS, "-ojsonpath='{.items[*].metadata.name}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(nodePoolNameList).NotTo(o.BeEmpty())

	//remove single quota from nodePoolNameList, then replace space with \n
	nodePoolNameStr := strings.Trim(nodePoolNameList, "'")
	nodePoolNameLines := strings.Replace(nodePoolNameStr, " ", "\n", -1)

	e2e.Logf("Hosted Cluster Name is: %s", hostedClusterName)
	hostedClusterNameReg := regexp.MustCompile(".*" + hostedClusterName + ".*")
	nodePoolName := hostedClusterNameReg.FindAllString(nodePoolNameLines, -1)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(nodePoolName).NotTo(o.BeEmpty())
	e2e.Logf("Node Pool Name is: %s", nodePoolName[0])
	return nodePoolName[0]

}

// getTuningConfigMapNameWithRetry used to get tuned configmap name for specified node pool
func getTuningConfigMapNameWithRetry(oc *exutil.CLI, namespace string, filter string) string {

	var configmapName string
	configmapName = ""
	err := wait.Poll(15*time.Second, 180*time.Second, func() (bool, error) {

		configMaps, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", namespace, "-oname").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configMaps).NotTo(o.BeEmpty())

		//filter the tuned confimap name
		configMapsReg := regexp.MustCompile(".*" + filter + ".*")
		isMatch := configMapsReg.MatchString(configMaps)
		if isMatch {
			tuningConfigMap := configMapsReg.FindAllString(configMaps, -1)
			e2e.Logf("The list of tuned configmap is: \n%v", tuningConfigMap)
			//Node Pool using MC will have two configmap
			if len(tuningConfigMap) == 2 {
				configmapName = tuningConfigMap[0] + " " + tuningConfigMap[1]
			} else {
				configmapName = tuningConfigMap[0]
			}

			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The value sysctl mismatch, please check")
	return configmapName
}

// getTunedSystemSetValueByParamNameInHostedCluster
func getTunedSystemSetValueByParamNameInHostedCluster(oc *exutil.CLI, ntoNamespace, nodeName, oscommand, sysctlparm string) string {

	debugNodeStdout, err := oc.AsAdmin().AsGuestKubeconf().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+nodeName, "--", "chroot", "/host", oscommand, sysctlparm).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	regexpstr, _ := regexp.Compile(sysctlparm + " =.*")
	matchResult := regexpstr.FindString(debugNodeStdout)
	e2e.Logf("The value of %v is %v on %v", sysctlparm, matchResult, nodeName)
	return matchResult
}

// compareSpecifiedValueByNameOnLabelNodewithRetryInHostedCluster
func compareSpecifiedValueByNameOnLabelNodewithRetryInHostedCluster(oc *exutil.CLI, ntoNamespace, nodeName, oscommand, sysctlparm, specifiedvalue string) {

	err := wait.Poll(15*time.Second, 180*time.Second, func() (bool, error) {

		tunedSettings := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, nodeName, oscommand, sysctlparm)
		expectedSettings := sysctlparm + " = " + specifiedvalue
		if strings.Contains(tunedSettings, expectedSettings) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The value sysctl mismatch, please check")
}

// assertIfTunedProfileAppliedOnSpecifiedNode use to check if custom profile applied to a node
func assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc *exutil.CLI, namespace string, tunedNodeName string, tunedName string) {

	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		expectedTunedName, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "profile", tunedNodeName, "-ojsonpath={.status.tunedProfile}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(expectedTunedName).NotTo(o.BeEmpty())
		matchTunedProfile := strings.Contains(expectedTunedName, tunedName)

		expectedAppliedStatus, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "profile", tunedNodeName, `-ojsonpath='{.status.conditions[?(@.type=="Applied")].status}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(expectedAppliedStatus).NotTo(o.BeEmpty())
		matchAppliedStatus := strings.Contains(expectedAppliedStatus, "True")

		if !matchTunedProfile && !matchAppliedStatus {
			e2e.Logf("Profile '%s' has not yet been applied to %s - retrying...", tunedName, tunedNodeName)
			return false, nil
		}
		e2e.Logf("Profile '%s' has been applied to %s - continuing...", tunedName, tunedNodeName)
		tunedProfiles, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "profile").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(expectedAppliedStatus).NotTo(o.BeEmpty())
		e2e.Logf("Current profiles on each node : \n %v ", tunedProfiles)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Profile was not applied to %s within timeout limit (30 seconds)", tunedNodeName))
}

// assertNTOPodLogsLastLinesInHostedCluster
func assertNTOPodLogsLastLinesInHostedCluster(oc *exutil.CLI, namespace string, ntoPod string, lineN string, timeDurationSec int, filter string) {

	var logLineStr []string

	err := wait.Poll(15*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {

		//Remove err assert for SNO, the OCP will can not access temporily when master node restart or certificate key removed
		ntoPodLogs, _ := oc.AsAdmin().AsGuestKubeconf().Run("logs").Args("-n", namespace, ntoPod, "--tail="+lineN).Output()

		regNTOPodLogs, err := regexp.Compile(".*" + filter + ".*")
		o.Expect(err).NotTo(o.HaveOccurred())
		isMatch := regNTOPodLogs.MatchString(ntoPodLogs)
		if isMatch {
			logLineStr = regNTOPodLogs.FindAllString(ntoPodLogs, -1)
			e2e.Logf("The logs of nto pod %v is: \n%v", ntoPod, logLineStr[0])
			return true, nil
		}
		e2e.Logf("The keywords of nto pod isn't found, try next ...")
		return false, nil
	})

	e2e.Logf("The logs of nto pod %v is: \n%v", ntoPod, logLineStr[0])
	exutil.AssertWaitPollNoErr(err, "The tuned pod's log doesn't contain the keywords, please check")
}

// getTunedRenderInHostedCluster returns a string representation of the rendered for tuned in the given namespace
func getTunedRenderInHostedCluster(oc *exutil.CLI, namespace string) (string, error) {
	return oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "tuned", "rendered", "-ojsonpath={.spec.profile[*].name}").Output()
}

// assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster use to check if custom profile applied to a node
func assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc *exutil.CLI, namespace string, nodePoolName string, tunedName string) {

	var (
		matchTunedProfile     bool
		matchAppliedStatus    bool
		matchNum              int
		expectedAppliedStatus string
	)

	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		nodeNames, err := exutil.GetAllNodesByNodePoolNameInHostedCluster(oc, nodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		matchNum = 0
		for i := 0; i < len(nodeNames); i++ {
			expectedTunedName, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "profile", nodeNames[i], "-ojsonpath={.status.tunedProfile}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(expectedTunedName).NotTo(o.BeEmpty())
			matchTunedProfile = strings.Contains(expectedTunedName, tunedName)

			expectedAppliedStatus, err = oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "profile", nodeNames[i], `-ojsonpath='{.status.conditions[?(@.type=="Applied")].status}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(expectedAppliedStatus).NotTo(o.BeEmpty())
			matchAppliedStatus = strings.Contains(expectedAppliedStatus, "True")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(expectedAppliedStatus).NotTo(o.BeEmpty())

			if matchTunedProfile && matchAppliedStatus {
				matchNum++
				e2e.Logf("Profile '%s' matchs on  %s - match times is:%v", tunedName, nodeNames[i], matchNum)

			}
		}

		if matchNum == len(nodeNames) {
			tunedProfiles, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "profile").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Current profiles on each node : \n %v ", tunedProfiles)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Profile was not applied to %s within timeout limit (30 seconds)", nodePoolName))
}

// compareSpecifiedValueByNameOnNodePoolLevelwithRetryInHostedCluster
func compareSpecifiedValueByNameOnNodePoolLevelwithRetryInHostedCluster(oc *exutil.CLI, ntoNamespace, nodePoolName, oscommand, sysctlparm, specifiedvalue string) {

	var (
		isMatch  bool
		matchNum int
	)

	err := wait.Poll(15*time.Second, 180*time.Second, func() (bool, error) {
		nodeNames, err := exutil.GetAllNodesByNodePoolNameInHostedCluster(oc, nodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodesNum := len(nodeNames)
		matchNum = 0
		//all worker node in the nodepool should match the tuned profile settings
		for i := 0; i < nodesNum; i++ {
			tunedSettings := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, nodeNames[i], oscommand, sysctlparm)
			expectedSettings := sysctlparm + " = " + specifiedvalue
			if strings.Contains(tunedSettings, expectedSettings) {
				matchNum++
				isMatch = true
			}
		}
		if isMatch && matchNum == nodesNum {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The value sysctl mismatch, please check")
}

// assertMisMatchTunedSystemSettingsByParmNameOnNodePoolLevelInHostedCluster used to compare the the value shouldn't match specified name
func assertMisMatchTunedSystemSettingsByParmNameOnNodePoolLevelInHostedCluster(oc *exutil.CLI, ntoNamespace, nodePoolName, oscommand, sysctlparm, expectedMisMatchValue string) {
	nodeNames, err := exutil.GetAllNodesByNodePoolNameInHostedCluster(oc, nodePoolName)
	o.Expect(err).NotTo(o.HaveOccurred())
	nodesNum := len(nodeNames)
	for i := 0; i < nodesNum; i++ {
		stdOut := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, nodeNames[i], oscommand, sysctlparm)
		o.Expect(stdOut).NotTo(o.BeEmpty())
		o.Expect(stdOut).NotTo(o.ContainSubstring(expectedMisMatchValue))
	}
}

// assertIfMatchKenelBootOnNodePoolLevelInHostedCluster used to compare if match the keywords
func assertIfMatchKenelBootOnNodePoolLevelInHostedCluster(oc *exutil.CLI, ntoNamespace, nodePoolName, expectedMisMatchValue string, isMatch bool) {
	nodeNames, err := exutil.GetAllNodesByNodePoolNameInHostedCluster(oc, nodePoolName)
	o.Expect(err).NotTo(o.HaveOccurred())
	nodesNum := len(nodeNames)
	for i := 0; i < nodesNum; i++ {
		debugNodeStdout, err := oc.AsAdmin().AsGuestKubeconf().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+nodeNames[i], "--", "cat", "/proc/cmdline").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(debugNodeStdout).NotTo(o.BeEmpty())
		if isMatch {
			o.Expect(debugNodeStdout).To(o.ContainSubstring(expectedMisMatchValue))
		} else {
			o.Expect(debugNodeStdout).NotTo(o.ContainSubstring(expectedMisMatchValue))
		}

	}
}
