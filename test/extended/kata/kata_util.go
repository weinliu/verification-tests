// Package kata operator tests
package kata

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
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
	name             string `json:"name"`
	logLevel         string `json:"logLevel"`
	eligibility      bool   `json:"checkNodeEligibility"`
	runtimeClassName string `json:"runtimeClassName"`
	enablePeerPods   bool   `json:"enablePeerPods"`
	template         string
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
	operatorVer        string
	eligibility        bool
	labelSingleNode    bool
	eligibleSingleNode bool
	runtimeClassName   string
	enablePeerPods     bool
}

// If you changes this please make changes to func createPeerPodSecrets
type PeerpodParam struct {
	AWS_SUBNET_ID       string
	AWS_VPC_ID          string
	PODVM_INSTANCE_TYPE string
	PROXY_TIMEOUT       string
	VXLAN_PORT          string
	AWS_REGION          string
	AWS_SG_IDS          string
	PODVM_AMI_ID        string
	CLOUD_PROVIDER      string
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
		// kataconfig exists. Is it finished?
		kataconfigStatusQuery, kataconfigStatusQueryChanged, err := kataconfigStatusInUse(oc, sub.namespace, kataconf.name)
		if err != nil {
			e2e.Logf("error with kataconfigStatusInUse: %v, changed %v %v", kataconfigStatusQuery, kataconfigStatusQueryChanged, err)
		} else {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kataconf.name, "-n", sub.namespace, kataconfigStatusQuery).Output()
			if strings.ToLower(msg) == "false" {
				g.By("(3) kataconfig is previously installed")
				return msg, err // no need to go through the rest
			}
		}
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
		"-p", "NAME="+kataconf.name, "LOGLEVEL="+kataconf.logLevel, "PEERPODS="+strconv.FormatBool(kataconf.enablePeerPods), "ELIGIBILITY="+strconv.FormatBool(kataconf.eligibility),
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
	msg, err = checkResourceExists(oc, "kataconfig", kataconf.name, sub.namespace, snooze*time.Second, 10*time.Second)

	g.By("(3.4) Wait for kataconfig to finish install")
	// Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	msg, err = waitForKataconfig(oc, kataconf.name, sub.namespace)
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

	msg = fmt.Sprintf("Checking if pod %v is ready", newPodName)
	g.By(msg)
	msg, err = checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", "Running", podSnooze*time.Second, 10*time.Second)
	if msg == "" || err != nil {
		e2e.Logf("Could not get pod status %v: %v %v", msg, err)
	}

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", newPodName, "-n", podNs, "-o=jsonpath={.spec.runtimeClassName}").Output()
	if msg != runtimeClassName || err != nil {
		e2e.Logf("pod %v has wrong runtime %v %v, expecting %v %v", newPodName, msg, err, runtimeClassName)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).To(o.ContainSubstring(runtimeClassName))
	return newPodName
}

func deleteKataResource(oc *exutil.CLI, res, resNs, resName string) bool {
	_, err := deleteResource(oc, res, resName, resNs, podSnooze*time.Second, 10*time.Second)
	if err != nil {
		return false
	}
	return true
}

// author: abhbaner@redhat.com
// checkKataPodStatus() replaced checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", podRunState, podSnooze*time.Second, 10*time.Second)

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
	msg := ""
	// check sub
	jsonSubStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status}").Output()
	if err != nil || gjson.Get(jsonSubStatus, "state").String() != "AtLatestKnown" {
		e2e.Logf("issue with subscription or state isn't expected: %v, actual: %v error: %v", "AtLatestKnown", jsonSubStatus, err)
	} else {
		if !strings.Contains(gjson.Get(jsonSubStatus, "installedCSV").String(), sub.subName) {
			e2e.Logf("Error: get installedCSV for subscription %v %v", jsonSubStatus, err)
		} else { // check csv
			csvName := gjson.Get(jsonSubStatus, "installedCSV").String()
			jsonCsvStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", sub.namespace, "-o=jsonpath={.status}").Output()
			if err != nil ||
				gjson.Get(jsonCsvStatus, "phase").String() != "Succeeded" ||
				gjson.Get(jsonCsvStatus, "reason").String() != "InstallSucceeded" {
				e2e.Logf("Error: CSV in wrong state, expected: %v actual:\n%v %v", "InstallSucceeded", jsonCsvStatus, err)
			} else { // check kataconfig
				// find out which status query to use
				kataconfigStatusQuery, kataconfigStatusQueryChanged, err := kataconfigStatusInUse(oc, sub.namespace, kcName)
				if err != nil {
					e2e.Logf("error with kataconfigStatusInUse: %v, changed %v %v", kataconfigStatusQuery, kataconfigStatusQueryChanged, err)
				} else {
					msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-n", sub.namespace, kataconfigStatusQuery).Output()
					if err == nil && strings.ToLower(msg) == "false" {
						return true
					}
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
		controlPod string
	)
	g.By("(2) Subscription checking")
	msg, err = checkResourceJsonpath(oc, "sub", sub.subName, sub.namespace, "-o=jsonpath={.status.state}", "AtLatestKnown", snooze*time.Second, 10*time.Second)

	csvName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}").Output()
	if err != nil || csvName == "" {
		e2e.Logf("ERROR: cannot get sub %v installedCSV %v %v", sub.subName, csvName, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())

	g.By("(2.1) Check that the csv '" + csvName + "' has finished")
	msg, err = checkResourceJsonpath(oc, "csv", csvName, sub.namespace, "-o=jsonpath={.status.phase}{.status.reason}", "SucceededInstallSucceeded", snooze*time.Second, 10*time.Second)

	// need controller-manager-service and controller-manager-* pod running before kataconfig
	// oc get pod -o=jsonpath={.items..metadata.name} && find one w/ controller-manager
	g.By("(2.2) Wait for controller manager pod to start")
	// checkResourceJsonpath() needs exact pod name. control-manager deploy does not have full name
	errCheck := wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items..metadata.name}", "-n", sub.namespace).Output()
		if strings.Contains(msg, "controller-manager") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Controller manger pods did not start %v %v", msg, err))

	// what is the pod name?
	for _, controlPod = range strings.Fields(msg) {
		if strings.Contains(controlPod, "controller-manager") {
			break // no need to check the rest
		}
	}

	// controller-podname -o=jsonpath={.status.containerStatuses} && !strings.Contains("false")
	g.By("(2.3) Check that " + controlPod + " is ready")
	// this checks that the 2 containers in the pod are not showing false.  checkResourceJsonpath() cannot be used
	errCheck = wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", controlPod, "-o=jsonpath={.status.containerStatuses}", "-n", sub.namespace).Output()
		if !strings.Contains(strings.ToLower(msg), "false") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("control pod %v did not become ready: %v %v", controlPod, msg, err))

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "--no-headers").Output()
	return msg, err
}

// author: tbuskey@redhat.com
func waitForNodesInDebug(oc *exutil.CLI, opNamespace string) (msg string, err error) {
	count := 0
	workerNodeList, err := exutil.GetClusterNodesBy(oc, "worker")
	o.Expect(err).NotTo(o.HaveOccurred())
	workerNodeCount := len(workerNodeList)
	if workerNodeCount < 1 {
		e2e.Logf("Error: no worker nodes: %v, %v", workerNodeList, err)
	}
	o.Expect(workerNodeList).NotTo(o.BeEmpty())
	//e2e.Logf("Waiting for %v nodes to enter debug: %v", workerNodeCount, workerNodeList)

	// loop all workers until they all have debug
	errCheck := wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		count = 0
		for index := range workerNodeList {
			msg, err = oc.AsAdmin().Run("debug").Args("-n", opNamespace, "node/"+workerNodeList[index], "--", "chroot", "/host", "crio", "config").Output()
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
	if err != nil {
		e2e.Logf("ERROR applying ImageContentSourcePolicy %v %v", msg, err)
	}
	msg, err = checkResourceExists(oc, "ImageContentSourcePolicy", icspName, "default", 360*time.Second, 10*time.Second)
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
		msg = gjson.Get(msg, "readyReplicas").String()
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Deployment has %v replicas, not %v %v", replicas, msg, err))
	return msg, err
}

func deleteDeployment(oc *exutil.CLI, deployNs, deployName string) bool {
	return deleteKataResource(oc, "deploy", deployNs, deployName)
}

func getTestRunConfigmap(oc *exutil.CLI, testrunDefault TestrunConfigmap, cmNs, cmName string) (testrun TestrunConfigmap, msg string, err error) {
	// set defaults
	testrun = testrunDefault
	testrun.exists = false

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", cmNs, cmName).Output()
	if err != nil {
		e2e.Logf("Configmap is not found: msg %v err: %v", msg, err)
	} else {
		configmapData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", cmNs, cmName, "-o", "jsonpath={.data}").Output()
		if err != nil {
			e2e.Failf("Configmap %v has error, no .data: %v %v", cmName, configmapData, err)
			return testrun, msg, err
		}
		testrun.exists = true
		e2e.Logf("configmap Data is:\n%v", configmapData)
		if gjson.Get(configmapData, "catalogsourcename").Exists() {
			testrun.catalogSourceName = gjson.Get(configmapData, "catalogsourcename").String()
		}

		if gjson.Get(configmapData, "channel").Exists() {
			testrun.channel = gjson.Get(configmapData, "channel").String()
		}

		if gjson.Get(configmapData, "icspneeded").Exists() {
			testrun.icspNeeded = gjson.Get(configmapData, "icspneeded").Bool()
		}

		if gjson.Get(configmapData, "mustgatherimage").Exists() {
			testrun.mustgatherImage = gjson.Get(configmapData, "mustgatherimage").String()
			if strings.Contains(testrun.mustgatherImage, "brew.registry.redhat.io") {
				testrun.icspNeeded = true
			}
		}

		if gjson.Get(configmapData, "eligibility").Exists() {
			testrun.eligibility = gjson.Get(configmapData, "eligibility").Bool()
		}

		if gjson.Get(configmapData, "eligibleSingleNode").Exists() {
			testrun.eligibleSingleNode = gjson.Get(configmapData, "eligibleSingleNode").Bool()
		}

		if gjson.Get(configmapData, "labelsinglenode").Exists() {
			testrun.labelSingleNode = gjson.Get(configmapData, "labelsinglenode").Bool()
		}

		if gjson.Get(configmapData, "operatorVer").Exists() {
			testrun.operatorVer = gjson.Get(configmapData, "operatorVer").String()
		}

		if gjson.Get(configmapData, "runtimeClassName").Exists() {
			testrun.runtimeClassName = gjson.Get(configmapData, "runtimeClassName").String()
		}

		if gjson.Get(configmapData, "enablePeerPods").Exists() {
			testrun.enablePeerPods = gjson.Get(configmapData, "enablePeerPods").Bool()
		}
	}
	return testrun, msg, err
}

func getClusterVersion(oc *exutil.CLI) (clusterVersion, ocpMajorVer, ocpMinorVer string) {
	jsonVersion, err := oc.AsAdmin().WithoutNamespace().Run("version").Args("-o", "json").Output()
	if err != nil || jsonVersion == "" || !gjson.Get(jsonVersion, "openshiftVersion").Exists() {
		e2e.Logf("Error: could not get oc version: %v %v", jsonVersion, err)
	}
	clusterVersion = gjson.Get(jsonVersion, "openshiftVersion").String()
	sa := strings.Split(clusterVersion, ".")
	ocpMajorVer = sa[0]
	ocpMinorVer = sa[1]
	return ocpMajorVer, ocpMinorVer, clusterVersion
}

func waitForKataconfig(oc *exutil.CLI, kcName, opNamespace string) (msg string, err error) {
	// Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	var (
		kataconfigStatusQuery        string
		kataconfigStatusQueryChanged bool
	)

	errCheck := wait.Poll(30*time.Second, kataSnooze*time.Second, func() (bool, error) {
		// find out which status query to use
		kataconfigStatusQuery, kataconfigStatusQueryChanged, err = kataconfigStatusInUse(oc, opNamespace, kcName)
		if err != nil {
			e2e.Logf("error with kataconfigStatusInUse: %v, changed %v %v", kataconfigStatusQuery, kataconfigStatusQueryChanged, err)
		} else {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-n", opNamespace, kataconfigStatusQuery).Output()
			if strings.ToLower(msg) == "false" {
				return true, nil
			}
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

func labelSelectedNodes(oc *exutil.CLI, selectorLabel, customLabel string) {
	nodeList := exutil.GetNodeListByLabel(oc, selectorLabel)
	if len(nodeList) > 0 {
		for _, node := range nodeList {
			LabelNode(oc, node, customLabel)
		}
	}
}

func LabelNode(oc *exutil.CLI, node, customLabel string) {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node, customLabel).Output()
	e2e.Logf("%v applied and output was: %v %v", customLabel, msg, err)
	o.Expect(err).NotTo(o.HaveOccurred())
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

func getAllKataNodes(oc *exutil.CLI, eligibility bool, opNamespace, featureLabel, customLabel string) (nodeNameList []string) {
	actLabel := customLabel
	if eligibility {
		actLabel = featureLabel
	}
	return exutil.GetNodeListByLabel(oc, actLabel)
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
	// oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "-n", podNs, deployName, "--ignore-not-found").Execute()
	// oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", "-n", podNs, deployName, "--ignore-not-found").Execute()
	_, _ = deleteResource(oc, "svc", deployName, podNs, podSnooze*time.Second, 10*time.Second)
	_, _ = deleteResource(oc, "route", deployName, podNs, podSnooze*time.Second, 10*time.Second)

}

func checkPeerPodSecrets(oc *exutil.CLI, opNamespace, provider string, ppSecretName string) (msg string, err error) {
	var (
		errors       = 0
		errorList    []string
		providerVars []string
	)

	switch provider {
	case "azure":
		providerVars = append(providerVars, "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_REGION", "AZURE_RESOURCE_GROUP", "AZURE_SUBSCRIPTION_ID", "AZURE_TENANT_ID")
	case "aws":
		providerVars = append(providerVars, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION", "AWS_SG_IDS", "AWS_SUBNET_ID", "AWS_VPC_ID")
	case "libvirt":
		providerVars = append(providerVars, "LIBVIRT_URI")
	default:
		msg = fmt.Sprintf("Cloud provider %v is not supported", provider)
		err = fmt.Errorf("%v", msg)
		return msg, err
	}

	for index := range providerVars {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", ppSecretName, "-n", opNamespace, "-o=jsonpath={.data."+providerVars[index]+"}").Output()
		if err != nil || msg == "" {
			errors++
			errorList = append(errorList, providerVars[index])
		}
	}

	msg = ""
	if errors != 0 {
		msg = fmt.Sprintf("ERROR missing vars in secret %v %v", errors, errorList)
		err = fmt.Errorf("%v", msg)
	}
	return msg, err
}

func decodeSecret(input string) (msg string, err error) {
	msg = ""

	debase64, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		msg = fmt.Sprintf("Was not able to decode %v.  %v %v", input, debase64, err)
	} else {
		msg = fmt.Sprintf("%s", debase64)
	}
	return msg, err
}

func checkPeerPodConfigMap(oc *exutil.CLI, opNamespace, provider, ppConfigMapName string) (msg string, err error) {
	var (
		errors       = 0
		errorList    []string
		providerVars []string
	)

	switch provider {
	case "azure":
		providerVars = append(providerVars, "CLOUD_PROVIDER", "AZURE_IMAGE_ID", "AZURE_INSTANCE_SIZE", "AZURE_NSG_ID", "AZURE_SUBNET_ID", "VXLAN_PORT")
	case "aws":
		providerVars = append(providerVars, "CLOUD_PROVIDER", "PODVM_AMI_ID", "PODVM_INSTANCE_TYPE", "VXLAN_PORT")
	case "libvirt":
		providerVars = append(providerVars, "CLOUD_PROVIDER")
	default:
		msg = fmt.Sprintf("Cloud provider %v is not supported", provider)
		err = fmt.Errorf("%v", msg)
		return msg, err
	}

	for provider := range providerVars {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", ppConfigMapName, "-n", opNamespace, "-o=jsonpath={.data."+providerVars[provider]+"}").Output()
		if err != nil || msg == "" {
			errors++
			errorList = append(errorList, providerVars[provider])
		}
	}

	msg = ""
	if errors != 0 {
		msg = fmt.Sprintf("ERROR missing vars in configmap %v %v", errors, errorList)
		err = fmt.Errorf("%v", msg)
	}
	return msg, err
}

func checkPeerPodControl(oc *exutil.CLI, opNamespace, expStatus string) (msg string, err error) {
	// This would check peer pod webhook pod , peerpodconfig-ctrl-caa pods , webhook service and endpoints attached to the svc
	var (
		peerpodconfigCtrlCaaPods []string
		webhookPods              []string
		webhooksvc               = "peer-pods-webhook-svc"
	)

	g.By("Check for peer pods webhook pod")
	// checkResourceJsonpath needs a pod name
	errCheck := wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err := oc.AsAdmin().Run("get").Args("pod", "-o=jsonpath={.items..metadata.name}", "-n", opNamespace).Output()
		if err != nil {
			return false, err
		}
		if strings.Contains(msg, "peer-pods-webhook") {
			return true, nil
		}
		return false, nil
	})
	if err != nil || msg == "" || errCheck != nil {
		e2e.Logf(" %v %v, %v", msg, err, errCheck)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("peer pod webhook pod did not start: %v", errCheck))

	//webhook pod names
	msg, err = oc.AsAdmin().Run("get").Args("pod", "-o=jsonpath={.items..metadata.name}", "-n", opNamespace).Output()
	for _, whPod := range strings.Fields(msg) {
		if strings.Contains(whPod, "peer-pods-webhook") {
			webhookPods = append(webhookPods, whPod)
		}
	}

	//count check
	whPodCount := len(webhookPods)
	if whPodCount != 2 {
		e2e.Logf("There should be two webhook pods, instead there are: %v", whPodCount)
		return
	}

	//pod state check
	for _, podName := range webhookPods {
		checkControlPod(oc, podName, opNamespace, expStatus)
	}

	g.By("Check for peer pods ctrl caa pod")
	// checkResourceJsonpath needs a podname
	errCheck = wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().Run("get").Args("pod", "-o=jsonpath={.items..metadata.name}", "-n", opNamespace).Output()
		if strings.Contains(msg, "peerpodconfig-ctrl-caa-daemon") {
			return true, nil
		}
		return false, nil
	})
	if err != nil || msg == "" || errCheck != nil {
		e2e.Logf(" %v %v, %v", msg, err, errCheck)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("peer pod ctrl caa pod did not start %v %v", msg, err))

	//peerpodconfig ctrl CAA pod names
	msg, err = oc.AsAdmin().Run("get").Args("pod", "-o=jsonpath={.items..metadata.name}", "-n", opNamespace).Output()
	for _, ppconfigCaaPod := range strings.Fields(msg) {
		if strings.Contains(ppconfigCaaPod, "peerpodconfig-ctrl-caa") {
			peerpodconfigCtrlCaaPods = append(peerpodconfigCtrlCaaPods, ppconfigCaaPod)
		}
	}

	//pod state check
	for _, podName := range peerpodconfigCtrlCaaPods {
		checkControlPod(oc, podName, opNamespace, expStatus)
	}

	//webhook service
	checkControlSvc(oc, opNamespace, webhooksvc)
	g.By("SUCCESS - peerpod config check passed")
	return msg, err
}

func checkControlPod(oc *exutil.CLI, podName, podNs, expStatus string) (msg string, err error) {
	msg, err = checkResourceJsonpath(oc, "pods", podName, podNs, "-o=jsonpath={.status.phase}", expStatus, podSnooze*time.Second, 10*time.Second)
	return msg, err
}

func checkControlSvc(oc *exutil.CLI, svcNs, svcName string) (msg string, err error) {
	g.By("Check for " + svcName + "service")
	msg, err = checkResourceJsonpath(oc, "service", svcName, svcNs, "-o=jsonpath={.metadata.name}", svcName, podSnooze*time.Second, 10*time.Second)

	g.By("Check for " + svcName + "service endpoints")
	// checkResourceJsonpath does strings.Contains not ContainsAny
	errCheck := wait.PollImmediate(10*time.Second, podSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().Run("get").Args("ep", svcName, "-n", svcNs, "-o=jsonpath={.subsets..addresses..ip}").Output()
		if strings.ContainsAny(msg, "0123456789") {
			return true, nil
		}
		return false, nil
	})
	if err != nil || msg == "" || errCheck != nil {
		e2e.Logf(" %v %v, %v", msg, err, errCheck)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v does not have endpoints attached to it;   err: %v", svcName, err))

	g.By("SUCCESS - service check passed")
	return msg, err
}

func kataconfigStatusInUse(oc *exutil.CLI, opNamespace, kcName string) (kataconfigStatusQuery string, kataconfigStatusQueryChanged bool, err error) {
	// author: tbuskey@redhat.com
	// detect kataconfig status changes in 1.5 and use them
	var json string

	kataconfigStatusQuery = "-o=jsonpath={.status.installationStatus.IsInProgress}{.status.unInstallationStatus.inProgress.status}"
	kataconfigStatusQueryChanged = false

	json, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-n", opNamespace, "-o=jsonpath={.status}").Output()
	if err != nil || json == "" {
		kataconfigStatusQuery = fmt.Sprintf("Could not get status of kataconfig %v %v", json, err)
	} else {
		if gjson.Get(json, "conditions").Exists() {
			kataconfigStatusQuery = "-o=jsonpath={.status.conditions[?(@.type=='InProgress')].status}"
			kataconfigStatusQueryChanged = true
		}
	}
	return kataconfigStatusQuery, kataconfigStatusQueryChanged, err
}

func checkResourceExists(oc *exutil.CLI, resType, resName, resNs string, duration, interval time.Duration) (msg string, err error) {
	// working: pod, deploy, service, route, ep, ds
	errCheck := wait.PollImmediate(interval, duration, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(resType, resName, "-n", resNs, "--no-headers").Output()
		if strings.Contains(msg, resName) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v %v was not found in ns %v after %v sec: %v %v", resType, resName, resNs, duration, msg, err))
	return msg, nil
}

func checkResourceJsonpath(oc *exutil.CLI, resType, resName, resNs, jsonpath, expected string, duration, interval time.Duration) (msg string, err error) {
	// resType=pod,    -o=jsonpath='{.status.phase}',                                               expected="Running"
	// resType=deploy, -o=jsonpath='{.status.conditions[?(@.type=="Available")].status}',           expected="True"
	// resType=route,  -o=jsonpath='{.status.ingress..conditions[?(@.type==\"Admitted\")].status}', expected="True"
	// resType=ds,     -o=jsonpath='{.status.ingress..conditions[?(@.type==\"Admitted\")].status}'", expected= number of nodes w/ kata-oc
	//   fmt.Sprintf("%v", len(exutil.GetNodeListByLabel(oc, kataocLabel)))

	/* readyReplicas might not exist in .status!
	// resType=deploy, -o=jsonpath='{.status.readyReplicas}',                                       expected = spec.replicas
	*/

	errCheck := wait.PollImmediate(interval, duration, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(resType, resName, "-n", resNs, jsonpath).Output()
		if strings.Contains(msg, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v %v in ns %v is not in %v state after %v sec: %v %v", resType, resName, resNs, expected, duration, msg, err))
	return msg, nil
}

func deleteResource(oc *exutil.CLI, res, resName, resNs string, duration, interval time.Duration) (msg string, err error) {
	msg, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(res, resName, "-n", resNs, "--ignore-not-found").Output()
	if err != nil {
		msg = fmt.Sprintf("ERROR: Cannot start deleting %v %v -n %v: %v %v", res, resName, resNs, msg, err)
		e2e.Failf(msg)
	}

	// make sure it doesn't exist
	errCheck := wait.PollImmediate(interval, duration, func() (bool, error) {
		msg, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(res, resName, "-n", resNs, "--no-headers").Output()
		if strings.Contains(msg, "not found") {
			return true, nil
		}
		return false, nil
	})
	if errCheck != nil {
		e2e.Logf("ERROR: Timeout waiting for delete to finish on %v %v -n %v: %v", res, resName, resNs, msg)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v %v was not finally deleted in ns %v", res, resName, resNs))

	msg = fmt.Sprintf("deleted %v %v -n %v: %v %v", res, resName, resNs, msg, err)
	err = nil
	return msg, err
}

func createApplyPeerPodSecrets(oc *exutil.CLI, provider string, ppParam PeerpodParam, opNamespace, ppSecretName, secretTemplate string) (msg string, err error) {
	/*
		Reads the configmap and the secret that the CI had applied "peerpods-param-cm" and "peerpods-param-secret"
		and creates "peer-pods-secret" from it and then applies it on the cluster.

		Checks if the cluster already has a peer-pods-secret and also for the correct value of the cloud provider
	*/

	var (
		secretString  string
		decodedString string
		ciCmName      = "peerpods-param-cm"
		ciSecretName  = "peerpods-param-secret"
	)

	// Check if the secrets already exist
	g.By("Checking if peer-pods-secret exists")
	msg, err = checkPeerPodSecrets(oc, opNamespace, provider, ppSecretName)
	if err == nil && msg == "" {
		e2e.Logf("peer-pods-secret exists - skipping creating it")
		return msg, err
	} else if err != nil {
		e2e.Logf("**** peer-pods-secret not found on the cluster - proceeding to create it****")
	}

	// Check if provider is correct
	if provider == "aws" || provider == "azure" {
		ppParam.CLOUD_PROVIDER = provider
	} else {
		msg = fmt.Sprintf("Cloud provider %v is not supported", provider)
		return msg, fmt.Errorf("%v", msg)
	}

	//Read params from peerpods-param-cm and store in ppParam struct
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", ciCmName).Output()
	if err != nil {
		e2e.Logf("peerpods-param-cm Configmap created by QE CI  not found: msg %v err: %v", msg, err)
	} else {
		configmapData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", ciCmName, "-o=jsonpath={.data}").Output()
		if err != nil {
			e2e.Failf("peerpods-param-cm Configmap created by QE CI %v has error, no .data: %v %v", ciCmName, configmapData, err)
		}

		e2e.Logf("configmap Data is:\n%v", configmapData)
		if gjson.Get(configmapData, "AWS_REGION").Exists() {
			ppParam.AWS_REGION = gjson.Get(configmapData, "AWS_REGION").String()
		}
		if gjson.Get(configmapData, "AWS_SUBNET_ID").Exists() {
			ppParam.AWS_SUBNET_ID = gjson.Get(configmapData, "AWS_SUBNET_ID").String()
		}
		if gjson.Get(configmapData, "AWS_VPC_ID").Exists() {
			ppParam.AWS_VPC_ID = gjson.Get(configmapData, "AWS_VPC_ID").String()
		}
		if gjson.Get(configmapData, "AWS_SG_IDS").Exists() {
			ppParam.AWS_SG_IDS = gjson.Get(configmapData, "AWS_SG_IDS").String()
		}
	}

	//Read peerpods-param-secret to fetch the access key and secret key
	secretString, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", ciSecretName, "-o=jsonpath={.data.aws}").Output()
	if err != nil || secretString == "" {
		e2e.Logf(" Error:%v CI provided peer pods secret data empty ", err)
	}
	decodedString, err = decodeSecret(secretString)
	if err != nil {
		return msg, err
	}

	lines := strings.Split(decodedString, "\n")
	accessKey := ""
	secretKey := ""

	for _, line := range lines {
		parts := strings.Split(line, "=")
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key == "aws_access_key_id" {
				accessKey = value
			} else if key == "aws_secret_access_key" {
				secretKey = value
			}
		}
	}

	if accessKey == "" || secretKey == "" {
		msg = "AWS credentials not found in the data."
		err = fmt.Errorf("AWS credentials not found")
		return msg, err
	}

	g.By("Create peer-pods-secret file")

	// Create a JSON file that represents the secret file format
	secretJSON := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "Opaque",
		"metadata": map[string]string{
			"name":      ppSecretName,
			"namespace": opNamespace,
		},
		"stringData": map[string]string{
			"AWS_ACCESS_KEY_ID":     accessKey,
			"AWS_SECRET_ACCESS_KEY": secretKey,
			"AWS_REGION":            ppParam.AWS_REGION,
			"AWS_SUBNET_ID":         ppParam.AWS_SUBNET_ID,
			"AWS_VPC_ID":            ppParam.AWS_VPC_ID,
			"AWS_SG_IDS":            ppParam.AWS_SG_IDS,
		},
	}

	// Marshal the JSON to a string
	secretJSONString, err := json.Marshal(secretJSON)
	if err != nil {
		return msg, err
	}

	// Write the JSON string to a file
	secretFile, err := os.Create(secretTemplate)
	if err != nil {
		return msg, err
	}
	defer secretFile.Close()

	secretFile.Write(secretJSONString)

	configFile := filepath.Dir(secretTemplate)
	if configFile != "" {
		osStatMsg, configFileExists := os.Stat(configFile)
		if configFileExists != nil {
			e2e.Logf("issue creating peer-pods-secret file is %s, err: %v , msg: %v' , osStatMsg: %v", configFile, err, msg, osStatMsg)
		}
	}

	g.By("(Apply peer-pods-secret  file")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
	if err != nil {
		e2e.Logf("Error: applying peer-pods-secret %v failed: %v %v", configFile, msg, err)
	}

	return msg, err
}

// Get the cloud provider type of the test environment copied from test/extended/storage/utils
func getCloudProvider(oc *exutil.CLI) string {
	var (
		errMsg error
		output string
	)
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, errMsg = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if errMsg != nil {
			e2e.Logf("Get cloudProvider *failed with* :\"%v\",wait 5 seconds retry.", errMsg)
			return false, errMsg
		}
		e2e.Logf("The test cluster cloudProvider is :\"%s\".", strings.ToLower(output))
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Waiting for get cloudProvider timeout")
	return strings.ToLower(output)
}

func createRWOfilePVC(oc *exutil.CLI, opNamespace, pvcName, capacity string) (err error) {
	// author: vvoronko@redhat.com
	// creates a PVC using as much calculated and default paramers as possible, leaving only:
	// napespace
	// Capacity in Gigs
	// Name
	// returns err
	accessMode := "ReadWriteOnce" //ReadWriteOnce, ReadOnlyMany or ReadWriteMany
	volumeMode := "Filesystem"    //Filesystem, Block
	return createPVC(oc, opNamespace, pvcName, capacity, volumeMode, accessMode)
}

func createPVC(oc *exutil.CLI, opNamespace, pvcName, capacity, volumeMode, accessMode string) (err error) {
	// just single Storage class per platform, block will be supported later?
	const jsonCsiClass = `{"azure":{"Filesystem":"azurefile-csi","Block":"managed-csi"},
		"gcp":{"Filesystem":"standard-csi","Block":"standard-csi"},
		"aws":{"Filesystem":"gp3-csi","Block":"gp3-csi"}}`
	cloudPlatform := getCloudProvider(oc)
	scName := gjson.Get(jsonCsiClass, strings.Join([]string{cloudPlatform, volumeMode}, `.`)).String()

	pvcDataDir := exutil.FixturePath("testdata", "storage")
	pvcTemplate := filepath.Join(pvcDataDir, "pvc-template.yaml")

	//validate provided capacity is a valid integer
	_, err = strconv.Atoi(capacity)
	if err != nil {
		return err
	}

	g.By("Create pvc from template")
	pvcFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", pvcTemplate,
		"-p", "SCNAME="+scName, "-p", "PVCNAME="+pvcName, "-p", "PVCNAMESPACE="+opNamespace,
		"-p", "ACCESSMODE="+accessMode, "-p", "VOLUMEMODE="+volumeMode, "-p", "PVCCAPACITY="+capacity).OutputToFile(getRandomString() + "pvc-default.json")
	if err != nil {
		e2e.Logf("Could not create pvc %v %v", pvcFile, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Applying pvc " + pvcFile)
	msg, err := oc.AsAdmin().Run("apply").Args("-f", pvcFile, "-n", opNamespace).Output()
	if err != nil {
		e2e.Logf("Could not apply pvc %v %v", msg, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("pvc apply output: %v", msg)
	return err
}
