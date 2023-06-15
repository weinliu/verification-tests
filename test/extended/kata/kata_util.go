// Package kata operator tests
package kata

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type SubscriptionDescription struct {
	subName                string `json:"name"`
	namespace              string `json:"namespace"`
	channel                string `json:"channel"`
	ipApproval             string `json:"installPlanApproval"`
	operatorPackage        string `json:"spec.name"`
	catalogSourceName      string `json:"source"`
	catalogSourceNamespace string `json:"sourceNamespace"`
	template               string
}

type KataconfigDescription struct {
	name                 string `json:"name"`
	kataMonitorImageName string `json:"kataMonitorImage"`
	logLevel             string `json:"logLevel"`
	eligibility          bool   `json:"checkNodeEligibility"`
	runtimeClassName     string `json:"runtimeClassName"`
	enablePeerPods       bool   `json:"enablePeerPods"`
	template             string
}

// if you change TestrunConfigmap, modify:
// getTestRunConfigmap()
// getTestRunEnvVars()
// testrun-cm-template.yaml
// kata.go:
//
//	testrunDefault
//	53583
type TestrunConfigmap struct {
	exists             bool
	catalogSourceName  string
	channel            string
	icspNeeded         bool
	mustgatherImage    string
	katamonitorImage   string
	operatorVer        string
	eligibility        bool
	labelSingleNode    bool
	eligibleSingleNode bool
	runtimeClassName   string
	enablePeerPods     bool
}

var (
	snooze     time.Duration = 2400
	kataSnooze time.Duration = 5400 // Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	podSnooze  time.Duration = 600  // Peer Pods take longer than 2 minutes
)

// author: tbuskey@redhat.com,abhbaner@redhat.com
func subscribeFromTemplate(oc *exutil.CLI, sub SubscriptionDescription, subTemplate string) (msg string, err error) {
	g.By(" (1) INSTALLING sandboxed-operator in '" + sub.namespace + "' namespace")
	subFile := ""

	g.By("(1.1) Creating subscription yaml from template")
	subFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", sub.template, "-p", "SUBNAME="+sub.subName, "SUBNAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"APPROVAL="+sub.ipApproval, "OPERATORNAME="+sub.operatorPackage, "SOURCENAME="+sub.catalogSourceName, "SOURCENAMESPACE="+sub.catalogSourceNamespace, "-n", sub.namespace).OutputToFile(getRandomString() + "subscriptionFile.json")
	// o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil || subFile != "" {
		if !strings.Contains(subFile, "already exists") {
			_, subFileExists := os.Stat(subFile)
			if subFileExists != nil {
				e2e.Logf("issue creating the subscription yaml %s, %v", subFile, err)
			}
		}
	}

	g.By("(1.2) Applying subscription yaml")
	// no need to check for an existing subscription
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subFile).Output()
	if err != nil || msg == "" {
		e2e.Logf(" issue applying subscription %v: %v, %v", subFile, msg, err)
	}

	g.By("(1.3) Verify the operator finished subscribing")
	msg, err = subscriptionIsFinished(oc, sub)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())

	return msg, err
}

// author: tbuskey@redhat.com, abhbaner@redhat.com
func createKataConfig(oc *exutil.CLI, kataconf KataconfigDescription, sub SubscriptionDescription) (msg string, err error) {
	// If this is used, label the caller with [Disruptive][Serial][Slow]
	// If kataconfig already exists, this must not error
	var (
		configFile string
	)

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kataconf.name, "--no-headers", "-n", sub.namespace).Output()
	if err == nil {
		g.By("(3) kataconfig is previously installed")
		return msg, err // no need to go through the rest
	}

	g.By("(3) Make sure subscription has finished before kataconfig")
	msg, err = subscriptionIsFinished(oc, sub)
	if err != nil {
		e2e.Logf("The subscription has not finished: %v %v", msg, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())

	g.By("(3.1) Create kataconfig file")
	configFile, err = oc.AsAdmin().WithoutNamespace().Run("process").Args("--ignore-unknown-parameters=true", "-f", kataconf.template,
		"-p", "NAME="+kataconf.name, "MONITOR="+kataconf.kataMonitorImageName, "LOGLEVEL="+kataconf.logLevel, "PEERPODS="+strconv.FormatBool(kataconf.enablePeerPods), "ELIGIBILITY="+strconv.FormatBool(kataconf.eligibility),
		"-n", sub.namespace).OutputToFile(getRandomString() + "kataconfig-common.json")
	if err != nil || configFile == "" {
		_, configFileExists := os.Stat(configFile)
		if configFileExists != nil {
			e2e.Logf("issue creating kataconfig file is %s, %v", configFile, err)
		}
	}

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "controller-manager-service", "-n", sub.namespace).Output()
	e2e.Logf("Controller-manager-service: %v %v", msg, err)

	g.By("(3.2) Apply kataconfig file")
	// -o=jsonpath={.status.installationStatus.IsInProgress} "" at this point
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
	if err != nil {
		e2e.Logf("Error: applying kataconfig %v failed: %v %v", configFile, msg, err)
	}
	// If it is already applied by a parallel test there will be an err

	g.By("(3.3) Check kataconfig creation has started")
	errCheck := wait.PollImmediate(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kataconf.name, "--no-headers").Output()
		if strings.Contains(msg, kataconf.name) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("kataconfig %v did not get created: %v %v", kataconf.name, msg, err))

	g.By("(3.4) Wait for kataconfig to finish install")
	// Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	msg, err = waitForKataconfig(oc, kataconf.name)
	return msg, err
}

// author: abhbaner@redhat.com
func createKataPod(oc *exutil.CLI, podNs, commonPod, commonPodName, runtimeClassName string) string {
	var (
		msg        string
		err        error
		newPodName string
		configFile string
	)

	newPodName = getRandomString() + commonPodName
	configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", commonPod, "-p", "NAME="+newPodName, "-p", "RUNTIMECLASSNAME="+runtimeClassName).OutputToFile(getRandomString() + "Pod-common.json")
	o.Expect(err).NotTo(o.HaveOccurred())

	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile, "-n", podNs).Output()
	if msg == "" || err != nil {
		e2e.Logf("Could not apply configFile %v: %v %v", configFile, msg, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())

	msg = fmt.Sprintf("Checking for %v runtime of pod %v", runtimeClassName, newPodName)
	g.By(msg)
	errCheck := wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", newPodName, "-n", podNs, "-o=jsonpath={.spec.runtimeClassName}").Output()
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Creating pod %v with %v failed: %v %v", newPodName, configFile, msg, err))

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", newPodName, "-n", podNs, "-o=jsonpath={.spec.runtimeClassName}").Output()
	if msg != runtimeClassName || err != nil {
		e2e.Logf("pod %v has wrong runtime %v %v, expecting %v %v", newPodName, msg, err, runtimeClassName)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).To(o.ContainSubstring(runtimeClassName))
	return newPodName
}

// author: abhbaner@redhat.com, vvoronko@redhat.com
func deleteKataPod(oc *exutil.CLI, podNs, delPodName string) bool {
	output, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", delPodName, "-n", podNs).Output()
	if err != nil {
		e2e.Logf("issue deleting pod %v in namespace %v, output: %v/nerror: %v", delPodName, podNs, output, err)
		return false
	}

	errCheck := wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", delPodName, "-n", podNs).Output()
		if err != nil {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Pod %v was not finally deleted in ns %v", delPodName, podNs))
	return true
}

// author: abhbaner@redhat.com
func checkKataPodStatus(oc *exutil.CLI, podNs, podName, expStatus string) {
	var actualStatus string
	errCheck := wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		actualStatus, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", podName, "-n", podNs, "-o=jsonpath={.status.phase}").Output()
		if strings.Contains(actualStatus, expStatus) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Pod %v is not correct status in ns %v. Expected status: %v, Actual status: %v", podName, podNs, expStatus, actualStatus))
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

func deleteKataConfig(oc *exutil.CLI, kcName string) (msg string, err error) {
	g.By("(4.1) Trigger kataconfig deletion")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("kataconfig", kcName).Output()
	if err != nil || msg == "" {
		e2e.Logf("Unexpected error while trying to delete kataconfig: %v\nerror: %v", msg, err)
	}
	//SNO could become unavailable while restarting
	//o.Expect(err).NotTo(o.HaveOccurred())

	g.By("(4.2) Wait for kataconfig to be deleted")
	errCheck := wait.Poll(30*time.Second, kataSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig").Output()
		if strings.Contains(msg, "No resources found") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("kataconfig %v did not get deleted: %v %v", kcName, msg, err))

	g.By("(4.3) kataconfig is gone")
	return msg, err
}

func checkKataInstalled(oc *exutil.CLI, sub SubscriptionDescription, kcName string) bool {
	var (
		jsonpathSubState   = "-o=jsonpath={.status.state}"
		jsonpathCsv        = "-o=jsonpath={.status.installedCSV}"
		jsonpathCsvState   = "-o=jsonpath={.status.phase}{.status.reason}"
		jsonpathKataconfig = "-o=jsonpath={.status.installationStatus.IsInProgress}{.status.unInstallationStatus.inProgress.status}"
		expectSubState     = "AtLatestKnown"
		expectCsvState     = "SucceededInstallSucceeded"
	)
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, jsonpathSubState).Output()
	if err != nil || msg != expectSubState {
		e2e.Logf("issue with subscription or state isn't expected: %v, actual: %v error: %v", expectSubState, msg, err)
	} else {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, jsonpathCsv).Output()
		if err != nil || !strings.Contains(msg, sub.subName) {
			e2e.Logf("Error: get installedCSV for subscription %v %v", msg, err)
		} else {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", msg, "-n", sub.namespace, jsonpathCsvState).Output()
			if err != nil || msg != expectCsvState {
				e2e.Logf("Error: CSV in wrong state, expected: %v actual: %v %v", expectCsvState, msg, err)
			} else {
				msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-n", sub.namespace, jsonpathKataconfig).Output()
				// DEBUG upper or lowercase the msg so just 1 comparision
				if err == nil && strings.ToLower(msg) == "false" {
					return true
				}
				e2e.Logf("Error: Kataconfig in wrong state, expected: false actual: %v error: %v", msg, err)
			}
		}
	}
	return false
}

func subscriptionIsFinished(oc *exutil.CLI, sub SubscriptionDescription) (msg string, err error) {
	var (
		csvName    string
		v          string
		controlPod string
	)
	g.By("(2) Subscription checking")
	errCheck := wait.PollImmediate(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}").Output()
		// o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(msg, "AtLatestKnown") == 0 {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "--no-headers").Output()
			return true, nil
		}
		return false, nil
	})
	if err != nil || msg == "" || errCheck != nil {
		e2e.Logf("issue with subscription %v %v, %v", msg, err, errCheck)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription %v is not correct status in ns %v", sub.subName, sub.namespace))

	csvName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}").Output()
	if err != nil || csvName == "" {
		e2e.Logf("Error: get sub for installedCSV %v %v", csvName, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())

	g.By("(2.1) Check that the csv '" + csvName + "' has finished")
	errCheck = wait.PollImmediate(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", sub.namespace, "-o=jsonpath={.status.phase}{.status.reason}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(msg, "SucceededInstallSucceeded") == 0 {
			v, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", sub.namespace, "--no-headers").Output()
			msg = fmt.Sprintf("%v state %v", v, msg)
			return true, nil
		}
		return false, nil
	})
	if err != nil || msg == "" || errCheck != nil {
		e2e.Logf("issue with csv finish %v: %v %v", csvName, msg, err)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status in ns %v: %v %v", csvName, sub.namespace, msg, err))

	// need controller-manager-service and controller-manager-* pod running before kataconfig
	// oc get pod -o=jsonpath={.items..metadata.name} && find one w/ controller-manager
	g.By("(2.2) Wait for controller manager pod to start")
	errCheck = wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items..metadata.name}", "-n", sub.namespace).Output()
		if strings.Contains(msg, "controller-manager") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Controller manger pod did not start: %v: %v %v", msg, err))

	// what is the pod name?
	for _, controlPod = range strings.Fields(msg) {
		if strings.Contains(controlPod, "controller-manager") {
			break // no need to check the rest
		}
	}

	// controller-podname -o=jsonpath={.status.containerStatuses} && !strings.Contains("false")
	g.By("(2.3) Check that " + controlPod + " is ready")
	errCheck = wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", controlPod, "-o=jsonpath={.status.containerStatuses}", "-n", sub.namespace).Output()
		if !strings.Contains(strings.ToLower(msg), "false") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v pod did not become ready: %v: %v %v", controlPod, msg, err))

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "--no-headers").Output()
	return msg, err
}

// author: valiev@redhat.com
func getNodeListByLabel(oc *exutil.CLI, opNamespace, labelKey string) (nodeNameList []string, msg string, err error) {
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-n", opNamespace, "-l", labelKey, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeNameList = strings.Fields(msg)
	return nodeNameList, msg, err
}

// author: tbuskey@redhat.com
func waitForNodesInDebug(oc *exutil.CLI, opNamespace, nodesLabel string) (msg string, err error) {
	count := 0
	workerNodeList, msg, err := getNodeListByLabel(oc, opNamespace, nodesLabel)
	o.Expect(err).NotTo(o.HaveOccurred())
	workerNodeCount := len(workerNodeList)
	if workerNodeCount < 1 {
		e2e.Logf("Error: no worker nodes: %v, %v %v", workerNodeList, msg, err)
	}
	o.Expect(workerNodeList).NotTo(o.BeEmpty())
	//e2e.Logf("Waiting for %v nodes to enter debug: %v", workerNodeCount, workerNodeList)

	// loop all workers until they all have debug
	errCheck := wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		count = 0
		for index := range workerNodeList {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", opNamespace, "node/"+workerNodeList[index], "--", "chroot", "/host", "crio", "config").Output()
			if strings.Contains(msg, "log_level = \"debug") {
				count++
				o.Expect(msg).To(o.ContainSubstring("log_level = \"debug"))
			}
		}
		if count == workerNodeCount {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Error: only %v of %v total worker nodes are in debug: %v\n %v", count, workerNodeCount, workerNodeList, msg))
	msg = fmt.Sprintf("All %v worker nodes are in debug mode: %v", workerNodeCount, workerNodeList)
	err = nil
	return msg, err
}

// author: tbuskey@redhat.com
func imageContentSourcePolicy(oc *exutil.CLI, icspFile, icspName string) (msg string, err error) {
	g.By("Applying ImageContentSourcePolicy")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", icspFile).Output()
	errCheck := wait.Poll(10*time.Second, 360*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--no-headers").Output()
		if strings.Contains(msg, icspName) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Error: applying ImageContentSourcePolicy %v failed: %v %v", icspFile, msg, err))
	return msg, err
}

func waitForDeployment(oc *exutil.CLI, podNs, deployName string) (msg string, err error) {
	var (
		snooze   time.Duration = 300
		replicas string
	)

	replicas, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", podNs, deployName, "-o=jsonpath={.spec.replicas}").Output()
	if err != nil {
		e2e.Logf("replica fetch failed %v %v", replicas, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(replicas).NotTo(o.BeEmpty())

	errCheck := wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", podNs, deployName, "-o=jsonpath={.status.readyReplicas}").Output()
		if msg == replicas {
			return true, nil
		}
		return false, nil
	})

	if errCheck != nil {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", podNs, deployName, "-o=jsonpath={.status}").Output()
		e2e.Logf("timed out %v != %v %v", replicas, msg, err)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", podNs, deployName, "-o=jsonpath={.status.readyReplicas}").Output()
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Deployment has %v replicas, not %v %v", replicas, msg, err))
	return msg, err
}

func getTestRunConfigmap(oc *exutil.CLI, testrunDefault TestrunConfigmap, cmNs, cmName string) (testrun TestrunConfigmap, msg string, err error) {
	// set defaults
	testrun = testrunDefault
	testrun.exists = false

	// icspNeeded is set if either of the Images has "brew.registry.redhat.io" in it

	// is a configmap there?  IFF not, don't put an error in the log!
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", cmNs).Output()
	if err != nil || !strings.Contains(msg, cmName) {
		testrun.exists = false
		return testrun, msg, err
	}

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", cmNs, cmName).Output()
	if err != nil {
		e2e.Logf("Configmap is not found: msg %v err: %v", msg, err)
		testrun.exists = false
	} else {
		testrun.exists = true
		// cm should have a data section
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", cmNs, cmName, "-o=jsonpath={.data}").Output()
		if err != nil {
			e2e.Failf("Configmap %v has error, no .data: %v %v", cmName, msg, err)
		}

		// look at all the items for a value.  If they are not empty, change the defaults
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.catalogsourcename}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.catalogSourceName = msg
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.channel}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.channel = msg
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.icspneeded}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.icspNeeded, err = strconv.ParseBool(msg)
			if err != nil {
				e2e.Failf("Error in %v config map.  icspneeded must be a golang true or false string", cmName)
			}
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.katamonitormage}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.katamonitorImage = msg
			if strings.Contains(msg, "brew.registry.redhat.io") {
				testrun.icspNeeded = true
			}
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.mustgatherimage}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.mustgatherImage = msg
			if strings.Contains(msg, "brew.registry.redhat.io") {
				testrun.icspNeeded = true
			}
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.eligibility}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.eligibility, err = strconv.ParseBool(msg)
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.eligibleSingleNode}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.eligibleSingleNode, err = strconv.ParseBool(msg)
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.labelsinglenode}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.labelSingleNode, err = strconv.ParseBool(msg)
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.operatorVer}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.operatorVer = msg
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.runtimeClassName}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.runtimeClassName = msg
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.enablePeerPods}", "-n", cmNs).Output()
		if err == nil && len(msg) > 0 {
			testrun.enablePeerPods, err = strconv.ParseBool(msg)
		}
	}
	return testrun, msg, err
}

func getClusterVersion(oc *exutil.CLI) (clusterVersion, ocpMajorVer, ocpMinorVer string) {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("version").Args("-o", "yaml").Output()
	if err != nil || msg == "" {
		e2e.Logf("Error: could not get oc version: %v %v", msg, err)
	}
	for _, s := range strings.Split(msg, "\n") {
		if strings.Contains(s, "openshiftVersion") {
			sa := strings.Split(s, " ")
			clusterVersion = sa[1]
			break
		}
	}
	sa := strings.Split(clusterVersion, ".")
	ocpMajorVer = sa[0]
	ocpMinorVer = sa[1]
	return ocpMajorVer, ocpMinorVer, clusterVersion
}

func waitForKataconfig(oc *exutil.CLI, kcName string) (msg string, err error) {
	// Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	errCheck := wait.Poll(30*time.Second, kataSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-o=jsonpath={.status.installationStatus.IsInProgress}{.status.unInstallationStatus.inProgress.status}").Output()
		// false || False, "" is done
		// true || True, "" install is in progress
		// FalseTrue uninstall (delete) is in progress
		if strings.ToLower(msg) == "false" {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("kataconfig %v did not finish install", kcName))

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "--no-headers").Output()
	msg = "SUCCESS kataconfig is created " + msg
	return msg, err
}

func changeSubscriptionCatalog(oc *exutil.CLI, subscription SubscriptionDescription, testrun TestrunConfigmap) (msg string, err error) {
	// check for catsrc existence before calling
	patch := fmt.Sprintf("{\"spec\":{\"source\":\"%v\"}}", testrun.catalogSourceName)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub", subscription.subName, "--type", "merge", "-p", patch, "-n", subscription.namespace).Output()
	return msg, err
}

func changeSubscriptionChannel(oc *exutil.CLI, subscription SubscriptionDescription, testrun TestrunConfigmap) (msg string, err error) {
	patch := fmt.Sprintf("{\"spec\":{\"channel\":\"%v\"}}", testrun.channel)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub", subscription.subName, "--type", "merge", "-p", patch, "-n", subscription.namespace).Output()
	return msg, err
}

func changeKataMonitorImage(oc *exutil.CLI, subscription SubscriptionDescription, testrun TestrunConfigmap, kcName string) (msg string, err error) {
	patch := fmt.Sprintf("{\"spec\":{\"kataMonitorImage\":\"%v\"}}", testrun.katamonitorImage)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kataconfig", kcName, "--type", "merge", "-p", patch, "-n", subscription.namespace).Output()
	return msg, err
}

func logErrorAndFail(oc *exutil.CLI, logMsg, msg string, err error) {
	e2e.Logf("%v: %v %v", logMsg, msg, err)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())

}

func getTestRunEnvVars(envPrefix string, testrunDefault TestrunConfigmap) (testrunEnv TestrunConfigmap, msg string) {

	var (
		err error
		val string
	)
	testrunEnv = testrunDefault
	testrunEnv.exists = false

	switch envPrefix {
	case "OSCS":
		msg = fmt.Sprintf("Looking for %v prefixed environment variables for starting OSC version", envPrefix)
	case "OSCU":
		msg = fmt.Sprintf("Looking for %v prefixed environment variables for upgrading OSC version", envPrefix)
	default:
		msg = fmt.Sprintf("Cannot look for %v prefixed environment variables \nValid prefixes are OSCS or OSCU", envPrefix)
		return testrunEnv, msg
	}

	val = os.Getenv(envPrefix + "OSCCHANNEL")
	if val != "" {
		testrunEnv.channel = val
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "ICSPNEEDED")
	if val != "" {
		testrunEnv.icspNeeded, err = strconv.ParseBool(val)
		if err != nil {
			e2e.Failf("Error: %v must be a golang true or false string", envPrefix+"ICSPNEEDED")
		}
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "CATSOURCENAME")
	if val != "" {
		testrunEnv.catalogSourceName = val
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "KATAMONITORIMAGE")
	if val != "" {
		testrunEnv.katamonitorImage = val
		testrunEnv.exists = true
		if strings.Contains(val, "brew.registry.redhat.io") {
			testrunEnv.icspNeeded = true
		}
	}

	val = os.Getenv(envPrefix + "MUSTGATHERIMAGE")
	if val != "" {
		testrunEnv.mustgatherImage = val
		testrunEnv.exists = true
		if strings.Contains(val, "brew.registry.redhat.io") {
			testrunEnv.icspNeeded = true
		}
	}

	val = os.Getenv(envPrefix + "OPERATORVER")
	if val != "" {
		testrunEnv.operatorVer = val
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "RUNTIMECLASSNAME")
	if val != "" {
		testrunEnv.runtimeClassName = val
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "ENABLEPEERPODS")
	if val != "" {
		testrunEnv.enablePeerPods, err = strconv.ParseBool(val)
		if err != nil {
			e2e.Failf("Error: %v must be a golang true or false string", envPrefix+"ENABLEPEERPODS")
		}
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "ELIGIBILITY")
	if val != "" {
		testrunEnv.eligibility, err = strconv.ParseBool(msg)
		if err != nil {
			e2e.Failf("Error: %v must be a golang true or false string", envPrefix+"ELIGIBILITY")
		}
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "ELIGIBLESINGLENODE")
	if val != "" {
		testrunEnv.eligibleSingleNode, err = strconv.ParseBool(msg)
		if err != nil {
			e2e.Failf("Error: %v must be a golang true or false string", envPrefix+"ELIGIBLESINGLENODE")
		}
		testrunEnv.exists = true
	}

	val = os.Getenv(envPrefix + "LABELSINGLENODE")
	if val != "" {
		testrunEnv.labelSingleNode, err = strconv.ParseBool(msg)
		if err != nil {
			e2e.Failf("Error: %v must be a golang true or false string", envPrefix+"LABELSINGLENODE")
		}
		testrunEnv.exists = true
	}

	return testrunEnv, msg
}

func labelSelectedNodes(oc *exutil.CLI, opNamespace, selectorLabel, customLabel string) {
	nodeList, _, err := getNodeListByLabel(oc, opNamespace, selectorLabel)
	if err == nil && len(nodeList) > 0 {
		for _, node := range nodeList {
			LabelNode(oc, opNamespace, node, customLabel)
		}
	}
}

func LabelNode(oc *exutil.CLI, opNamespace, node, customLabel string) {
	//check if node has the label already
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o=jsonpath={.metadata.labels}").Output()
	if err == nil && !strings.Contains(msg, customLabel) {
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node, customLabel).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func getInstancesOnNode(oc *exutil.CLI, opNamespace, node string) (instances int, err error) {

	cmd := fmt.Sprintf("ps -ef | grep uuid | grep -v grep | wc -l")
	msg, err := exutil.DebugNodeWithOptionsAndChroot(oc, node, []string{"-q"}, "bin/sh", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	instances, err = strconv.Atoi(strings.TrimSpace(msg))
	if err != nil {
		instances = 0
	}
	return instances, err
}

func getTotalInstancesOnNodes(oc *exutil.CLI, opNamespace string, nodeList []string) (total int) {
	total = 0
	count := 0
	for _, node := range nodeList {
		count, _ = getInstancesOnNode(oc, opNamespace, node)
		e2e.Logf("found %v VMs on node %v", count, node)
		total += count
	}
	e2e.Logf("Total %v VMs on all nodes", total)
	return total
}

func getAllKataNodes(oc *exutil.CLI, eligibility bool, opNamespace, featureLabel, customLabel string) (nodeNameList []string, msg string, err error) {
	actLabel := customLabel
	if eligibility {
		actLabel = featureLabel
	}
	nodeList, msg, err := getNodeListByLabel(oc, opNamespace, actLabel)
	return nodeList, msg, err
}

func getHttpResponse(url string, expStatusCode int) (resp string, err error) {
	resp = ""
	res, err := http.Get(url)
	if err == nil {
		defer res.Body.Close()
		if res.StatusCode != expStatusCode {
			err = fmt.Errorf("Response from url=%v\n actual status code=%d doesn't match expected %d\n", url, res.StatusCode, expStatusCode)
		} else {
			body, err := io.ReadAll(res.Body)
			if err == nil {
				resp = string(body)
			}
		}
	}
	return resp, err
}

// create a service and route for the deployment, both with the same name as deployment itself
// require defer deleteRouteAndService to cleanup
func createServiceAndRoute(oc *exutil.CLI, deployName, podNs string) (host string, err error) {
	msg, err := oc.WithoutNamespace().Run("expose").Args("deployment", deployName, "-n", podNs).Output()
	if err != nil {
		e2e.Logf("Expose deployment failed with: %v %v", msg, err)
	} else {
		msg, err = oc.Run("expose").Args("service", deployName, "-n", podNs).Output()
		if err != nil {
			e2e.Logf("Expose service failed with: %v %v", msg, err)
		} else {
			host, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", deployName, "-n", podNs, "-o=jsonpath={.spec.host}").Output()
			if err != nil || host == "" {
				e2e.Logf("Failed to get host from route, actual host=%v\n error %v", host, err)
			}
			host = strings.Trim(host, "'")
		}
	}
	return host, err
}

// cleanup for createServiceAndRoute func
func deleteRouteAndService(oc *exutil.CLI, deployName, podNs string) {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "-n", podNs, deployName, "--ignore-not-found").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", "-n", podNs, deployName, "--ignore-not-found").Execute()
}
