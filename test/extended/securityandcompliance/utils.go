package securityandcompliance

import (
	"fmt"
	"os"
	"os/exec"
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

type fileintegrity struct {
	name              string
	namespace         string
	configname        string
	configkey         string
	graceperiod       int
	debug             bool
	nodeselectorkey   string
	nodeselectorvalue string
	template          string
}

type podModify struct {
	name      string
	namespace string
	nodeName  string
	args      string
	template  string
}

type seccompProfile struct {
	name            string
	namespace       string
	baseprofilename string
	template        string
}

type podWithProfile struct {
	name             string
	namespace        string
	localhostProfile string
	template         string
}

func createSecurityProfileOperator(oc *exutil.CLI, subD subscriptionDescription, ogD operatorGroupDescription) {
	g.By("Install security profiles operator !!!")
	createOperator(oc, subD, ogD)

	g.By("Check Security Profile Operator &webhook &spod pods are in running state !!!")
	checkReadyPodCountOfDeployment(oc, "security-profiles-operator", subD.namespace, 3)
	checkReadyPodCountOfDeployment(oc, "security-profiles-operator-webhook", subD.namespace, 3)
	checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

	g.By("Security Profiles Operator sucessfully installed !!! ")
}

func subscriptionIsFinished(oc *exutil.CLI, sub subscriptionDescription) (msg string, err error) {
	var (
		csvName   string
		subStatus string
	)

	g.By("Check the operator is AtLatestKnown !!!")
	errCheck := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}").Output()
		if strings.Compare(msg, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	// Expose more info when sub status is not AtLatestKnown
	if errCheck != nil {
		e2e.Logf("The result of \"oc get sub %s -n %s -o=jsonpath={.status.state}\" is: %s", sub.subName, sub.namespace, msg)
		subStatus, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status}").Output()
		e2e.Logf("The result of \"oc get sub %s -n %s -o=jsonpath={.status}\" is: %s", sub.subName, sub.namespace, subStatus)
		source, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.spec.source}").Output()
		soucePodStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "olm.catalogSource="+source, "-n", "openshift-marketplace", "-o=jsonpath={.items[0].status}").Output()
		e2e.Logf("The pod status for catalogsource %s is: %s", source, soucePodStatus)
	}
	// skip test for known OLM bug https://issues.redhat.com/browse/OCPBUGS-19046. More details seen from https://access.redhat.com/solutions/6603001
	if (errCheck != nil) && (strings.Contains(subStatus, "exists and is not referenced by a subscription")) {
		g.Skip("Skip test due to known OLM bug ")
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription %v is not correct status in ns %v", sub.subName, sub.namespace))

	g.By("Get csvName to check its finish !!!")
	csvName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Check the csv '" + csvName + "' has finished !!!")
	errCheck = wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", sub.namespace, "-o=jsonpath={.status.phase}{.status.reason}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(msg, "SucceededInstallSucceeded") == 0 {
			return true, nil
		}
		return false, nil
	})
	// Expose more info when csv not finished
	if errCheck != nil {
		e2e.Logf("The result of \"oc get csv %s -n %s -o=jsonpath={.status.phase}{.status.reason}\" is: %s", csvName, sub.namespace, msg)
		resPod, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", sub.namespace).Output()
		e2e.Logf("The result of \"oc get pod -n %s\" is: %s", sub.namespace, resPod)
		resContainerStatuses, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", sub.namespace, "-o=jsonpath={.items[*].status.containerStatuses[0]}").Output()
		e2e.Logf("The result of \"oc get pod -n %s -o=jsonpath={.items[*].status.containerStatuses[0]}\" is: %s", sub.namespace, resContainerStatuses)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status in ns %v: %v %v", csvName, sub.namespace, msg, err))
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "--no-headers").Output()
	return msg, err
}

func deleteNamespace(oc *exutil.CLI, namespace string) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", namespace, "--ignore-not-found", "--timeout=60s").Execute()
	if err != nil {
		customColumns := "-o=custom-columns=NAME:.metadata.name,CR_NAME:.spec.names.singular,SCOPE:.spec.scope"
		crd, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "-n", namespace, customColumns).Output()
		e2e.Logf("The result of \"oc get crd -n %s %s\" is: %s", namespace, customColumns, crd)
		nsStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", namespace, "-n", namespace, "-o=jsonpath={.status}").Output()
		e2e.Logf("The result of \"oc get ns %s -n %s =-o=jsonpath={.status}\" is: %s", namespace, namespace, nsStatus)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func checkReadyPodCountOfDeployment(oc *exutil.CLI, name string, namespace string, readyCount int) {
	err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		rCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", name, "-n", namespace, "-o=jsonpath={.status.availableReplicas}").Output()
		if strings.Compare(strconv.Itoa(readyCount), rCount) == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check failed: the ready pod count is not expected count [ %d ]", readyCount))
}

func checkPodsStautsOfDaemonset(oc *exutil.CLI, name string, namespace string) {
	var dCount, rCount, misScheduledCount, updatedScheduledCount string
	err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		dCount, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", name, "-n", namespace, "-o=jsonpath={.status.desiredNumberScheduled}").Output()
		rCount, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", name, "-n", namespace, "-o=jsonpath={.status.numberReady}").Output()
		misScheduledCount, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", name, "-n", namespace, "-o=jsonpath={.status.numberMisscheduled}").Output()
		updatedScheduledCount, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", name, "-n", namespace, "-o=jsonpath={.status.updatedNumberScheduled}").Output()
		if strings.Compare(dCount, "0") != 0 && strings.Compare(dCount, rCount) == 0 && strings.Compare(dCount, updatedScheduledCount) == 0 && strings.Compare(misScheduledCount, "0") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check failed: the pods number of desiredNumberScheduled, numberReady, numberMisscheduled, updatedNumberScheduled are: %v, %v, %v, %v",
		dCount, rCount, misScheduledCount, updatedScheduledCount))
}

func (secProfile *seccompProfile) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", secProfile.template, "-p", "NAME="+secProfile.name, "NAMESPACE="+secProfile.namespace, "BASEPROFILENAME="+secProfile.baseprofilename)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (fi1 *fileintegrity) checkFileintegrityStatus(oc *exutil.CLI, expected string) {
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", fi1.namespace, "-l app=aide-"+fi1.name,
			"-o=jsonpath={.items[*].status.containerStatuses[*].state}").Output()
		if strings.Contains(output, expected) && (!(strings.Contains(strings.ToLower(output), "error"))) && (!(strings.Contains(strings.ToLower(output), "crashLoopbackOff"))) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the state of pod with app=aide-example-fileintegrity is not expected %s", expected))
}

func (fi1 *fileintegrity) getDataFromConfigmap(oc *exutil.CLI, cmName string, expected string) {
	var res string
	err := wait.Poll(5*time.Second, 500*time.Second, func() (bool, error) {
		_, err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", fi1.namespace, "configmap/"+cmName, "--to=/tmp", "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		aideResult, err := os.ReadFile("/tmp/integritylog")
		res = string(aideResult)
		o.Expect(err).NotTo(o.HaveOccurred())
		matched, _ := regexp.MatchString(expected, res)
		if matched {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		// Expose more info when configmap not contains the expected string
		e2e.Logf("The aide report details is: %s", res)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmName %s does not include %s", cmName, expected))
}

func getOneWorkerNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l node-role.kubernetes.io/edge!=,node-role.kubernetes.io/worker=",
		"-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return nodeName
}

func getOneMasterNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l node-role.kubernetes.io/master=",
		"-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return nodeName
}

func (fi1 *fileintegrity) getOneFioPodName(oc *exutil.CLI) string {
	fioPodName, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l file-integrity.openshift.io/pod=",
		"-n", fi1.namespace, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	if strings.Compare(fioPodName, "") != 0 {
		return fioPodName
	}
	return fioPodName
}

func (fi1 *fileintegrity) checkKeywordNotExistInLog(oc *exutil.CLI, podName string, expected string) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		logs, err1 := oc.AsAdmin().WithoutNamespace().Run("logs").Args(podName, "-n", fi1.namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if strings.Compare(logs, "") != 0 && !strings.Contains(logs, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s includes %s", podName, expected))
}

func (fi1 *fileintegrity) checkKeywordExistInLog(oc *exutil.CLI, podName string, expected string) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		logs, err1 := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+podName, "-n", fi1.namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if strings.Contains(logs, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s does not include %s", podName, expected))
}

func (fi1 *fileintegrity) checkErrorsExistInLog(oc *exutil.CLI, podName string, expected string) {
	var logs []byte
	var errGrep error
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		logFile, errLogs := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+podName, "-n", fi1.namespace).OutputToFile(getRandomString() + "isc-audit.log")
		if errLogs != nil {
			return false, errLogs
		}
		logs, errGrep = exec.Command("bash", "-c", "cat "+logFile+" | grep -i error; rm -rf "+logFile).Output()
		if errGrep != nil {
			return false, errGrep
		}
		if strings.Contains(string(logs), expected) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		// Expose more info when the logs didn't contain expected string
		e2e.Logf("The logs for pod %s is: %s", podName, string(logs))
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s does not include %s", podName, expected))
}

func (fi1 *fileintegrity) checkArgsInPod(oc *exutil.CLI, expected string) {
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		fioPodArgs, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l file-integrity.openshift.io/pod=",
			"-n", fi1.namespace, "-o=jsonpath={.items[0].spec.containers[].args}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		if strings.Contains(fioPodArgs, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("args of does not include %s", expected))
}

func (pod *podModify) doActionsOnNode(oc *exutil.CLI, expected string) {
	err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", pod.namespace, "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
		"NODENAME="+pod.nodeName, "PARAC="+pod.args)
	o.Expect(err1).NotTo(o.HaveOccurred())
	newCheck("expect", asAdmin, withoutNamespace, contain, expected, ok, []string{"pod", "-n", pod.namespace, pod.name,
		"-o=jsonpath={.status.phase}"}).check(oc)
}

func (fi1 *fileintegrity) createFIOWithoutConfig(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", fi1.namespace, "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace,
		"GRACEPERIOD="+strconv.Itoa(fi1.graceperiod), "DEBUG="+strconv.FormatBool(fi1.debug), "NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (fi1 *fileintegrity) createFIOWithoutKeyword(oc *exutil.CLI, keyword string) {
	err := applyResourceFromTemplateWithoutKeyword(oc, keyword, "--ignore-unknown-parameters=true", "-n", fi1.namespace, "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace,
		"CONFNAME="+fi1.configname, "CONFKEY="+fi1.configkey, "DEBUG="+strconv.FormatBool(fi1.debug), "NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (fi1 *fileintegrity) createFIOWithConfig(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", fi1.namespace, "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace,
		"GRACEPERIOD="+strconv.Itoa(fi1.graceperiod), "DEBUG="+strconv.FormatBool(fi1.debug), "CONFNAME="+fi1.configname, "CONFKEY="+fi1.configkey,
		"NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (sub *subscriptionDescription) checkPodFioStatus(oc *exutil.CLI, expected string) {
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", sub.namespace, "-l", "name=file-integrity-operator",
			"-o=jsonpath={.items[*].status.containerStatuses[*].state}").Output()
		if strings.Contains(strings.ToLower(output), expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("state of pod with name=file-integrity-operator is not expected %s", expected))
}

func (fi1 *fileintegrity) createConfigmapFromFile(oc *exutil.CLI, cmName string, aideKey string, aideFile string, expected string) (bool, error) {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", cmName, "-n", fi1.namespace, "--from-file="+aideKey+"="+aideFile).Output()
	if strings.Contains(strings.ToLower(output), expected) {
		return true, nil
	}
	return false, nil
}

func (fi1 *fileintegrity) checkConfigmapCreated(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", fi1.configname, "-n", fi1.namespace).Output()
		if strings.Contains(output, fi1.configname) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cm %s is not created", fi1.configname))
}

func (fi1 *fileintegrity) checkFileintegritynodestatus(oc *exutil.CLI, nodeName string, expected string) {
	var output string
	fileintegrityName := fi1.name + "-" + nodeName
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fileintegrityName,
			"-o=jsonpath={.lastResult.condition}").Output()
		return output == expected, nil
	})
	if err != nil {
		// Expose more info when not get the expected condition of the fileintegritynodestatuses
		e2e.Logf("The fileintegritynodestatuses %s for node %s is: %s", fileintegrityName, nodeName, output)
		res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace).Output()
		e2e.Logf("The fileintegritynodestatuses for all nodes are: %s", res)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fileintegritynodestatuses %s is not expected %s", fi1.name+"-"+nodeName, expected))
}

func (fi1 *fileintegrity) checkOnlyOneDaemonset(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 45*time.Second, func() (bool, error) {
		daemonsetPodNumber, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", "-n", fi1.namespace, "-o=jsonpath={.items[].status.numberReady}").Output()
		podNameString, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l file-integrity.openshift.io/pod=", "-n", fi1.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		intDaemonsetPodNumber, _ := strconv.Atoi(daemonsetPodNumber)
		intPodNumber := len(strings.Fields(podNameString))
		if intPodNumber == intDaemonsetPodNumber {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "daemonset number is not expted ")
}

func (fi1 *fileintegrity) removeFileintegrity(oc *exutil.CLI, expected string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("fileintegrity", fi1.name, "-n", fi1.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (fi1 *fileintegrity) reinitFileintegrity(oc *exutil.CLI, expected string) {
	res, err := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("fileintegrity", fi1.name, "-n", fi1.namespace, "file-integrity.openshift.io/re-init=").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(res).To(o.ContainSubstring(expected))
}

func (fi1 *fileintegrity) getDetailedDataFromFileintegritynodestatus(oc *exutil.CLI, nodeName string) (int, int, int) {
	var intFilesAdded, intFilesChanged, intFilesRemoved int
	err := wait.Poll(5*time.Second, 150*time.Second, func() (bool, error) {
		filesAdded, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fi1.name+"-"+nodeName,
			"-o=jsonpath={.results[-1].filesAdded}").Output()
		filesChanged, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fi1.name+"-"+nodeName,
			"-o=jsonpath={.results[-1].filesChanged}").Output()
		filesRemoved, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fi1.name+"-"+nodeName,
			"-o=jsonpath={.results[-1].filesRemoved}").Output()
		if filesAdded == "" && filesChanged == "" && filesRemoved == "" {
			return false, nil
		}
		if filesAdded == "" {
			intFilesAdded = 0
		} else {
			intFilesAdded, _ = strconv.Atoi(filesAdded)
		}
		if filesChanged == "" {
			intFilesChanged = 0
		} else {
			intFilesChanged, _ = strconv.Atoi(filesChanged)
		}
		if filesRemoved == "" {
			intFilesRemoved = 0
		} else {
			intFilesRemoved, _ = strconv.Atoi(filesRemoved)
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("file of fileintegritynodestatuses  %s is not added, changed or removed", fi1.name+"-"+nodeName))
	return intFilesAdded, intFilesChanged, intFilesRemoved
}

func (fi1 *fileintegrity) getDetailedDataFromConfigmap(oc *exutil.CLI, cmName string) (int, int, int) {
	var intFilesAdded, intFilesChanged, intFilesRemoved int
	err := wait.Poll(5*time.Second, 150*time.Second, func() (bool, error) {
		annotations, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-n", fi1.namespace,
			"-o=jsonpath={.metadata.annotations}").Output()
		e2e.Logf("the result of annotations in configmap:%v", annotations)
		filesAdded, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-n", fi1.namespace,
			"-o=jsonpath={.metadata.annotations.file-integrity\\.openshift\\.io/files-added}").Output()
		filesChanged, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-n", fi1.namespace,
			"-o=jsonpath={.metadata.annotations.file-integrity\\.openshift\\.io/files-changed}").Output()
		filesRemoved, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-n", fi1.namespace,
			"-o=jsonpath={.metadata.annotations.file-integrity\\.openshift\\.io/files-removed}").Output()
		if (filesAdded == "" && filesChanged == "" && filesRemoved == "") || (filesAdded == "0" && filesChanged == "0" && filesRemoved == "0") {
			return false, nil
		}
		if filesAdded == "" {
			intFilesAdded = 0
		} else {
			intFilesAdded, _ = strconv.Atoi(filesAdded)
		}
		if filesChanged == "" {
			intFilesChanged = 0
		} else {
			intFilesChanged, _ = strconv.Atoi(filesChanged)
		}
		if filesRemoved == "" {
			intFilesRemoved = 0
		} else {
			intFilesRemoved, _ = strconv.Atoi(filesRemoved)
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("file of cm  %s is not added, changed or removed", cmName))
	return intFilesAdded, intFilesChanged, intFilesRemoved
}

func checkDataDetailsEqual(intFileAddedCM int, intFileChangedCM int, intFileRemovedCM int, intFileAddedFins int, intFileChangedFins int, intFileRemovedFins int) {
	if intFileAddedCM != intFileAddedFins || intFileChangedCM != intFileChangedFins || intFileRemovedCM != intFileRemovedFins {
		e2e.Failf("the data datails in configmap and fileintegrity not equal!")
	}
}

func (fi1 *fileintegrity) checkPodNumerLessThanNodeNumber(oc *exutil.CLI, label string) {
	err := wait.Poll(5*time.Second, 100*time.Second, func() (bool, error) {
		intNodeNumber := getNodeNumberPerLabel(oc, label)
		daemonsetPodNumber, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", "-n", fi1.namespace, "-o=jsonpath={.items[].status.numberReady}").Output()
		intDaemonsetPodNumber, _ := strconv.Atoi(daemonsetPodNumber)
		if intNodeNumber != intDaemonsetPodNumber+1 {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "daemonset pod number greater than node number")
}

func (fi1 *fileintegrity) checkPodNumerEqualNodeNumber(oc *exutil.CLI, label string) {
	err := wait.Poll(5*time.Second, 100*time.Second, func() (bool, error) {
		intNodeNumber := getNodeNumberPerLabel(oc, label)
		daemonsetPodNumber, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", "-n", fi1.namespace, "-o=jsonpath={.items[].status.numberReady}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		intDaemonsetPodNumber, _ := strconv.Atoi(daemonsetPodNumber)
		if intNodeNumber != intDaemonsetPodNumber {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "daemonset pod number not equal to node number")
}

func (fi1 *fileintegrity) recreateFileintegrity(oc *exutil.CLI) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegrity", fi1.name, "-n", fi1.namespace, "-ojson").OutputToFile(getRandomString() + "isc-config.json")
		if err1 != nil {
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fileintegrity %s is not got", fi1.name))
	fi1.removeFileintegrity(oc, "deleted")
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func setLabelToSpecificNode(oc *exutil.CLI, nodeName string, label string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nodeName, label).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (fi1 *fileintegrity) expectedStringNotExistInConfigmap(oc *exutil.CLI, cmName string, expected string) {
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		_, err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", oc.Namespace(), "configmap/"+cmName, "--to=/tmp", "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		aideResult, err := os.ReadFile("/tmp/integritylog")
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(string(aideResult), expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cm %s contains %s", cmName, expected))
}

func (fi1 *fileintegrity) checkDBBackupResult(oc *exutil.CLI, nodeName string) {
	errWait := wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
		dbBackupResult, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args(`node/`+nodeName, "-n", fi1.namespace, "--", "chroot", "/host", "find", "/etc/kubernetes/", "-maxdepth", "1", "-mmin", "-5").Output()
		if err != nil {
			return false, nil
		}
		if strings.Contains(dbBackupResult, "aide.db.gz.backup") && strings.Contains(dbBackupResult, "aide.log.backup") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("the DB backup result for node %s does not exist", nodeName))
}

func (fi1 *fileintegrity) getDBBackupLists(oc *exutil.CLI, nodeName string, dbReinitiated bool) ([]string, bool) {
	var dbGzBackupList []string
	isNewFIO := false
	maxBackups, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegrity", fi1.name, "-n", fi1.namespace, "-o=jsonpath={.spec.config.maxBackups}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	maxBackupsInt, _ := strconv.Atoi(maxBackups)

	errWait := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		dbBackup, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args(`node/`+nodeName, "-n", fi1.namespace, "--", "chroot", "/host", "find", "/etc/kubernetes/", "-maxdepth", "1").Output()
		if err != nil {
			return false, nil
		}
		dbGzBackupList = getMatchedFiles("aide.db.gz.backup.*", dbBackup)
		if dbReinitiated == false {
			dbGzFilesList := getMatchedFiles("aide.db.gz.*", dbBackup)
			if len(dbGzFilesList) == 0 {
				isNewFIO = true
			}
		}

		if len(dbGzBackupList) > maxBackupsInt && dbReinitiated == true {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("the DB backup result for node %s is invalid", nodeName))
	return dbGzBackupList, isNewFIO
}

func getMatchedFiles(pattern string, filesList string) []string {
	var matchedFileList []string
	regex, err := regexp.Compile(pattern)
	if err != nil {
		e2e.Failf("Error while extracting files using pattern %s", pattern)
	}
	matchedFileList = regex.FindAllString(filesList, -1)
	return matchedFileList
}

func checkDBFilesUpdated(oc *exutil.CLI, fi1 fileintegrity, oldDbBackupfiles []string, nodeName string, dbReinitiated bool, isNewFIO bool) {
	var newBackupFilesCount int
	var foundCommonFile bool
	var errorMsg string
	errWait := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		newDBBackupfiles, _ := fi1.getDBBackupLists(oc, nodeName, dbReinitiated)
		for i := range newDBBackupfiles {
			foundCommonFile = false
			for j := range oldDbBackupfiles {
				if oldDbBackupfiles[j] == newDBBackupfiles[i] {
					foundCommonFile = true
				}
			}
			if foundCommonFile == false {
				newBackupFilesCount++
				e2e.Logf("the DB backup files for node %s has updated", nodeName)
				break
			}
		}
		if newBackupFilesCount == 0 {
			if dbReinitiated == true {
				errorMsg = "DB files not updated. It should be updated"
				return false, nil
			}
			e2e.Logf("the DB backup files for node %s has not updated", nodeName)
		} else if newBackupFilesCount != 0 && dbReinitiated == false && isNewFIO == false {
			errorMsg = "DB files updated. It should not be updated"
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("%s", errorMsg))
}

func (fi1 *fileintegrity) assertNodeConditionNotEmpty(oc *exutil.CLI, nodeName string) {
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		fileintegrityName := fi1.name + "-" + nodeName
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fileintegrityName,
			"-o=jsonpath={.lastResult.condition}").Output()
		if err != nil {
			return false, nil
		}
		return output != "", nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fileintegritynodestatuses %s is empty", fi1.name+"-"+nodeName))
}

func (fi1 *fileintegrity) assertNodesConditionNotEmpty(oc *exutil.CLI) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", fi1.namespace, "-l app=aide-"+fi1.name, "-o=jsonpath={.items[*].spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodes := strings.Fields(output)
	for _, node := range nodes {
		fi1.assertNodeConditionNotEmpty(oc, node)
	}
}

func (fi1 *fileintegrity) getNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", fi1.namespace, "-l app=aide-"+fi1.name, "-o=jsonpath={.items[0].spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return nodeName
}
