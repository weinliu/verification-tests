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
	"os/exec"
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

type TestRunDescription struct {
	checked            bool
	catalogSourceName  string
	channel            string
	redirectNeeded     bool
	mustgatherImage    string
	operatorVer        string
	eligibility        bool
	labelSingleNode    bool
	eligibleSingleNode bool
	runtimeClassName   string
	enablePeerPods     bool
	enableGPU          bool
	podvmImageUrl      string
	workloadImage      string
	installKataRPM     bool
	workloadToTest     string
}

// If you changes this please make changes to func createPeerPodSecrets
type PeerpodParam struct {
	AWS_SUBNET_ID            string
	AWS_VPC_ID               string
	PODVM_INSTANCE_TYPE      string
	PROXY_TIMEOUT            string
	VXLAN_PORT               string
	AWS_REGION               string
	AWS_SG_IDS               string
	PODVM_AMI_ID             string
	CLOUD_PROVIDER           string
	AZURE_REGION             string
	AZURE_RESOURCE_GROUP     string
	AZURE_IMAGE_ID           string
	AZURE_INSTANCE_SIZE      string
	AZURE_NSG_ID             string
	AZURE_SUBNET_ID          string
	LIBVIRT_KVM_HOST_ADDRESS string
}

type UpgradeCatalogDescription struct {
	name        string
	namespace   string
	exists      bool
	imageAfter  string
	imageBefore string
	catalogName string
}

var (
	snooze                time.Duration = 2400
	kataSnooze            time.Duration = 5400 // Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	podSnooze             time.Duration = 600  // Peer Pods take longer than 2 minutes
	podRunState                         = "Running"
	featureLabel                        = "feature.node.kubernetes.io/runtime.kata=true"
	workerLabel                         = "node-role.kubernetes.io/worker"
	kataocLabel                         = "node-role.kubernetes.io/kata-oc"
	customLabel                         = "custom-label=test"
	kataconfigStatusQuery               = "-o=jsonpath={.status.conditions[?(@.type=='InProgress')].status}"
	allowedWorkloadTypes                = [3]string{"kata", "peer-pods", "coco"}
)

func ensureNamespaceIsInstalled(oc *exutil.CLI, namespace, namespaceTemplateFile string) (err error) {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", namespace, "--no-headers").Output()
	if err != nil || strings.Contains(msg, "Error from server (NotFound)") {
		namespaceFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", namespaceTemplateFile,
			"-p", "NAME="+namespace).OutputToFile(getRandomString() + "namespaceFile.json")
		if err != nil || namespaceFile == "" {
			if !strings.Contains(namespaceFile, "already exists") {
				_, statErr := os.Stat(namespaceFile)
				if statErr != nil {
					err = fmt.Errorf("ERROR creating the namespace (%v) yaml %s, %v", namespace, namespaceFile, statErr)
					return err
				}
			}
		}

		msg, err = oc.AsAdmin().Run("apply").Args("-f", namespaceFile).Output()
		if strings.Contains(msg, "AlreadyExists") {
			return nil
		}
		if err != nil {
			return fmt.Errorf(" applying namespace file (%v) issue: %v %v", namespaceFile, msg, err)
		}
	}
	return err
}

func ensureOperatorGroupIsInstalled(oc *exutil.CLI, namespace, templateFile string) (err error) {
	msg, err := oc.AsAdmin().Run("get").Args("operatorgroup", "-n", namespace, "--no-headers").Output()
	if err != nil || strings.Contains(msg, "No resources found in") {
		operatorgroupFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", templateFile,
			"-p", "NAME="+namespace, "NAMESPACE="+namespace).OutputToFile(getRandomString() + "operatorgroupFile.json")
		if err != nil || operatorgroupFile != "" {
			if !strings.Contains(operatorgroupFile, "already exists") {
				_, statErr := os.Stat(operatorgroupFile)
				if statErr != nil {
					err = fmt.Errorf("ERROR creating the operatorgroup (%v) yaml %v, %v", namespace, operatorgroupFile, statErr)
					return err
				}
			}
		}
		msg, err = oc.AsAdmin().Run("apply").Args("-f", operatorgroupFile, "-n", namespace).Output()
		if strings.Contains(msg, "AlreadyExists") {
			return nil
		}
		if err != nil {
			return fmt.Errorf("applying operatorgroup file (%v) issue %v %v", operatorgroupFile, msg, err)
		}
	}
	return err
}

func ensureOpenshiftSandboxedContainerOperatorIsSubscribed(oc *exutil.CLI, sub SubscriptionDescription, subTemplate string) (err error) {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "--no-headers").Output()
	if err != nil || strings.Contains(msg, "Error from server (NotFound):") {
		subFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", sub.template, "-p", "SUBNAME="+sub.subName, "SUBNAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
			"APPROVAL="+sub.ipApproval, "OPERATORNAME="+sub.operatorPackage, "SOURCENAME="+sub.catalogSourceName, "SOURCENAMESPACE="+sub.catalogSourceNamespace, "-n", sub.namespace).OutputToFile(getRandomString() + "subscriptionFile.json")
		if err != nil || subFile != "" {
			if !strings.Contains(subFile, "already exists") {
				_, subFileExists := os.Stat(subFile)
				if subFileExists != nil {
					err = fmt.Errorf("ERROR creating the subscription yaml %s, %v", subFile, err)
					return err
				}
			}
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subFile).Output()
		if err != nil || msg == "" {
			err = fmt.Errorf("ERROR applying subscription %v: %v, %v", subFile, msg, err)
			return err
		}
	}
	_, err = subscriptionIsFinished(oc, sub)
	return err
}

func ensureFeatureGateIsApplied(oc *exutil.CLI, sub SubscriptionDescription, featureGatesFile string) (err error) {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "osc-feature-gates", "-n", sub.namespace, "--no-headers").Output()
	if strings.Contains(msg, "Error from server (NotFound)") {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", featureGatesFile).Output()
		if err != nil && !strings.Contains(msg, "already exists exit") {
			err = fmt.Errorf("featureGates cm issue %v %v", msg, err)
		}
	}
	return err
}

// author: tbuskey@redhat.com, abhbaner@redhat.com
func createKataConfig(oc *exutil.CLI, kataconf KataconfigDescription, sub SubscriptionDescription) (msg string, err error) {
	// If this is used, label the caller with [Disruptive][Serial][Slow]
	// If kataconfig already exists, this must not error
	var (
		configFile string
	)

	_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kataconf.name, "--no-headers", "-n", sub.namespace).Output()
	if err == nil {
		// kataconfig exists. Is it finished?
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kataconf.name, "-n", sub.namespace, kataconfigStatusQuery).Output()
		if strings.ToLower(msg) == "false" {
			g.By("(3) kataconfig is previously installed")
			return msg, err // no need to go through the rest
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
	_, _ = checkResourceExists(oc, "kataconfig", kataconf.name, sub.namespace, snooze*time.Second, 10*time.Second)

	g.By("(3.4) Wait for kataconfig to finish install")
	// Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	msg, err = waitForKataconfig(oc, kataconf.name, sub.namespace)
	return msg, err
}
func createKataPodAnnotated(oc *exutil.CLI, podNs, template, basePodName, runtimeClassName, workloadImage string, annotations map[string]string) (msg string, err error) {
	var (
		newPodName string
		configFile string
		phase      = "Running"
	)

	newPodName = getRandomString() + basePodName
	configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", template, "-p", "NAME="+newPodName,
		"-p", "MEMORY="+annotations["MEMORY"], "-p", "CPU="+annotations["CPU"], "-p",
		"INSTANCESIZE="+annotations["INSTANCESIZE"], "-p", "RUNTIMECLASSNAME="+runtimeClassName, "IMAGE="+workloadImage).OutputToFile(getRandomString() + "Pod-common.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	return createKataPodFromTemplate(oc, podNs, newPodName, configFile, runtimeClassName, phase)
}

func createKataPodFromTemplate(oc *exutil.CLI, podNs, newPodName, configFile, runtimeClassName, phase string) (msg string, err error) {
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile, "-n", podNs).Output()
	if msg == "" || err != nil {
		return msg, fmt.Errorf("Could not apply configFile %v: %v %v", configFile, msg, err)
	}

	g.By(fmt.Sprintf("Checking if pod %v is ready", newPodName))
	msg, err = checkResourceJsonpath(oc, "pod", newPodName, podNs, "-o=jsonpath={.status.phase}", phase, podSnooze*time.Second, 10*time.Second)
	if msg == "" || err != nil {
		return msg, fmt.Errorf("Could not get pod (%v) status %v: %v %v", newPodName, phase, msg, err)
	}

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", newPodName, "-n", podNs, "-o=jsonpath={.spec.runtimeClassName}").Output()
	if msg != runtimeClassName || err != nil {
		err = fmt.Errorf("pod %v has wrong runtime %v, expecting %v %v", newPodName, msg, runtimeClassName, err)
	}
	return newPodName, err
}

// author: abhbaner@redhat.com
func createKataPod(oc *exutil.CLI, podNs, commonPod, basePodName, runtimeClassName, workloadImage string) string {
	var (
		err        error
		newPodName string
		configFile string
		phase      = "Running"
	)

	newPodName = getRandomString() + basePodName
	configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", commonPod, "-p",
		"NAME="+newPodName, "-p", "RUNTIMECLASSNAME="+runtimeClassName, "-p", "IMAGE="+workloadImage).OutputToFile(getRandomString() + "Pod-common.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	podname, err := createKataPodFromTemplate(oc, podNs, newPodName, configFile, runtimeClassName, phase)
	o.Expect(err).NotTo(o.HaveOccurred())
	return podname
}

func deleteKataResource(oc *exutil.CLI, res, resNs, resName string) bool {
	_, err := deleteResource(oc, res, resName, resNs, podSnooze*time.Second, 10*time.Second)
	if err != nil {
		return false
	}
	return true
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
				msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-n", sub.namespace, kataconfigStatusQuery).Output()
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
		controlPod string
	)
	g.By("(2) Subscription checking")
	msg, _ = checkResourceJsonpath(oc, "sub", sub.subName, sub.namespace, "-o=jsonpath={.status.state}", "AtLatestKnown", snooze*time.Second, 10*time.Second)

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
func applyImageRedirect(oc *exutil.CLI, redirectFile, redirectType, redirectName string) error {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", redirectFile).Output()
	if err != nil {
		return fmt.Errorf("ERROR applying %v: %v %v", redirectType, msg, err)
	}
	_, err = checkResourceExists(oc, redirectType, redirectName, "default", 360*time.Second, 10*time.Second)
	return err
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

func getClusterVersion(oc *exutil.CLI) (clusterVersion, ocpMajorVer, ocpMinorVer string, minorVer int) {
	jsonVersion, err := oc.AsAdmin().WithoutNamespace().Run("version").Args("-o", "json").Output()
	if err != nil || jsonVersion == "" || !gjson.Get(jsonVersion, "openshiftVersion").Exists() {
		e2e.Logf("Error: could not get oc version: %v %v", jsonVersion, err)
	}
	clusterVersion = gjson.Get(jsonVersion, "openshiftVersion").String()
	sa := strings.Split(clusterVersion, ".")
	ocpMajorVer = sa[0]
	ocpMinorVer = sa[1]
	minorVer, _ = strconv.Atoi(ocpMinorVer)
	return clusterVersion, ocpMajorVer, ocpMinorVer, minorVer
}

func waitForKataconfig(oc *exutil.CLI, kcName, opNamespace string) (msg string, err error) {
	// Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node

	errCheck := wait.Poll(30*time.Second, kataSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-n", opNamespace, kataconfigStatusQuery).Output()
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

func changeSubscriptionCatalog(oc *exutil.CLI, subscription SubscriptionDescription, testrun TestRunDescription) (msg string, err error) {
	// check for catsrc existence before calling
	patch := fmt.Sprintf("{\"spec\":{\"source\":\"%v\"}}", testrun.catalogSourceName)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub", subscription.subName, "--type", "merge", "-p", patch, "-n", subscription.namespace).Output()
	return msg, err
}

func changeSubscriptionChannel(oc *exutil.CLI, subscription SubscriptionDescription, testrun TestRunDescription) (msg string, err error) {
	patch := fmt.Sprintf("{\"spec\":{\"channel\":\"%v\"}}", testrun.channel)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub", subscription.subName, "--type", "merge", "-p", patch, "-n", subscription.namespace).Output()
	return msg, err
}

func logErrorAndFail(oc *exutil.CLI, logMsg, msg string, err error) {
	e2e.Logf("%v: %v %v", logMsg, msg, err)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())
}

func checkAndLabelCustomNodes(oc *exutil.CLI, testrun TestRunDescription) {
	e2e.Logf("check and label nodes (or single node for custom label)")
	nodeCustomList := exutil.GetNodeListByLabel(oc, customLabel)
	if len(nodeCustomList) > 0 {
		e2e.Logf("labeled nodes found %v", nodeCustomList)
	} else {
		if testrun.labelSingleNode {
			node, err := exutil.GetFirstWorkerNode(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			LabelNode(oc, node, customLabel)
		} else {
			labelSelectedNodes(oc, workerLabel, customLabel)
		}
	}

}

func labelEligibleNodes(oc *exutil.CLI, testrun TestRunDescription) {
	e2e.Logf("Label worker nodes for eligibility feature")
	if testrun.eligibleSingleNode {
		node, err := exutil.GetFirstWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		LabelNode(oc, node, featureLabel)
	} else {
		labelSelectedNodes(oc, workerLabel, featureLabel)
	}
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
		providerVars = append(providerVars, "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_SUBSCRIPTION_ID", "AZURE_TENANT_ID")
	case "aws":
		providerVars = append(providerVars, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
	case "libvirt":
		providerVars = append(providerVars, "LIBVIRT_URI", "LIBVIRT_POOL", "LIBVIRT_VOL_NAME")
	default:
		msg = fmt.Sprintf("Cloud provider %v is not supported", provider)
		err = fmt.Errorf("%v", msg)
		return msg, err
	}

	jsonData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", ppSecretName, "-n", opNamespace, "-o=jsonpath={.data}").Output()
	if err != nil {
		msg = fmt.Sprintf("Secret for %v not exists", provider)
		err = fmt.Errorf("%v", msg)
		return msg, err
	}
	for index := range providerVars {
		if !gjson.Get(jsonData, providerVars[index]).Exists() || gjson.Get(jsonData, providerVars[index]).String() == "" {
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
		providerVars = append(providerVars, "CLOUD_PROVIDER", "AZURE_NSG_ID", "AZURE_SUBNET_ID", "VXLAN_PORT", "AZURE_REGION", "AZURE_RESOURCE_GROUP")
	case "aws":
		providerVars = append(providerVars, "CLOUD_PROVIDER", "AWS_REGION", "AWS_SG_IDS", "AWS_SUBNET_ID", "AWS_VPC_ID", "VXLAN_PORT")
	case "libvirt":
		providerVars = append(providerVars, "CLOUD_PROVIDER")
	default:
		msg = fmt.Sprintf("Cloud provider %v is not supported", provider)
		err = fmt.Errorf("%v", msg)
		return msg, err
	}

	jsonData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", ppConfigMapName, "-n", opNamespace, "-o=jsonpath={.data}").Output()
	if err != nil {
		msg = fmt.Sprintf("Configmap for %v not exists", provider)
		err = fmt.Errorf("%v", msg)
		return msg, err
	}

	for index := range providerVars {
		if !gjson.Get(jsonData, providerVars[index]).Exists() || gjson.Get(jsonData, providerVars[index]).String() == "" {
			errors++
			errorList = append(errorList, providerVars[index])
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
	var (
		ciCmName     = "peerpods-param-cm"
		ciSecretName = "peerpods-param-secret"
	)

	// Check if the secrets already exist
	g.By("Checking if peer-pods-secret exists")
	msg, err = checkPeerPodSecrets(oc, opNamespace, provider, ppSecretName)
	if err == nil && msg == "" {
		e2e.Logf("peer-pods-secret exists - skipping creating it")
		return msg, err
	}

	//	e2e.Logf("**** peer-pods-secret not found on the cluster - proceeding to create it****")

	//Read params from peerpods-param-cm and store in ppParam struct
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", ciCmName, "-n", "default").Output()
	if err != nil {
		e2e.Logf("%v Configmap created by QE CI not found: msg %v err: %v", ciCmName, msg, err)
	} else {
		configmapData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", ciCmName, "-n", "default", "-o=jsonpath={.data}").Output()
		if err != nil {
			e2e.Failf("%v Configmap created by QE CI has error, no .data: %v %v", ciCmName, configmapData, err)
		}

		e2e.Logf("configmap Data is:\n%v", configmapData)
		ppParam, err := parseCIPpConfigMapData(provider, configmapData)
		if err != nil {
			return msg, err
		}

		var secretFilePath string
		if provider == "aws" {
			secretFilePath, err = createAWSPeerPodSecrets(oc, ppParam, ciSecretName, secretTemplate)
		} else if provider == "azure" {
			secretFilePath, err = createAzurePeerPodSecrets(oc, ppParam, ciSecretName, secretTemplate)
		} else if provider == "libvirt" {
			secretFilePath, err = createLibvirtPeerPodSecrets(oc, ppParam, ciCmName, secretTemplate)
		} else {
			msg = fmt.Sprintf("Cloud provider %v is not supported", provider)
			return msg, fmt.Errorf("%v", msg)
		}

		if err != nil {
			return msg, err
		}

		g.By("(Apply peer-pods-secret file)")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", secretFilePath).Output()
		if err != nil {
			e2e.Logf("Error: applying peer-pods-secret %v failed: %v %v", secretFilePath, msg, err)
		}
		if errRemove := os.Remove(secretFilePath); errRemove != nil {
			e2e.Logf("Error: removing secret file %v failed: %v", secretFilePath, errRemove)
		}

	}

	return msg, err
}

func parseCIPpConfigMapData(provider, configmapData string) (PeerpodParam, error) {
	var ppParam PeerpodParam

	switch provider {
	case "aws":
		return parseAWSCIConfigMapData(configmapData)
	case "azure":
		return parseAzureCIConfigMapData(configmapData)
	case "libvirt":
		return parseLibvirtCIConfigMapData(configmapData)
	default:
		return ppParam, fmt.Errorf("Cloud provider %v is not supported", provider)
	}
}

func parseLibvirtCIConfigMapData(configmapData string) (PeerpodParam, error) {
	var ppParam PeerpodParam

	if gjson.Get(configmapData, "PROXY_TIMEOUT").Exists() {
		ppParam.PROXY_TIMEOUT = gjson.Get(configmapData, "PROXY_TIMEOUT").String()
	}
	if gjson.Get(configmapData, "LIBVIRT_KVM_HOST_ADDRESS").Exists() {
		ppParam.LIBVIRT_KVM_HOST_ADDRESS = gjson.Get(configmapData, "LIBVIRT_KVM_HOST_ADDRESS").String()
	}

	return ppParam, nil
}

func parseAWSCIConfigMapData(configmapData string) (PeerpodParam, error) {
	var ppParam PeerpodParam

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
	if gjson.Get(configmapData, "VXLAN_PORT").Exists() {
		ppParam.VXLAN_PORT = gjson.Get(configmapData, "VXLAN_PORT").String()
	}
	if gjson.Get(configmapData, "PODVM_INSTANCE_TYPE").Exists() {
		ppParam.PODVM_INSTANCE_TYPE = gjson.Get(configmapData, "PODVM_INSTANCE_TYPE").String()
	}
	if gjson.Get(configmapData, "PROXY_TIMEOUT").Exists() {
		ppParam.PROXY_TIMEOUT = gjson.Get(configmapData, "PROXY_TIMEOUT").String()
	}

	return ppParam, nil
}

func parseAzureCIConfigMapData(configmapData string) (PeerpodParam, error) {
	var ppParam PeerpodParam

	if gjson.Get(configmapData, "AZURE_REGION").Exists() {
		ppParam.AZURE_REGION = gjson.Get(configmapData, "AZURE_REGION").String()
	}
	if gjson.Get(configmapData, "AZURE_RESOURCE_GROUP").Exists() {
		ppParam.AZURE_RESOURCE_GROUP = gjson.Get(configmapData, "AZURE_RESOURCE_GROUP").String()
	}
	if gjson.Get(configmapData, "VXLAN_PORT").Exists() {
		ppParam.VXLAN_PORT = gjson.Get(configmapData, "VXLAN_PORT").String()
	}
	if gjson.Get(configmapData, "AZURE_INSTANCE_SIZE").Exists() {
		ppParam.AZURE_INSTANCE_SIZE = gjson.Get(configmapData, "AZURE_INSTANCE_SIZE").String()
	}
	if gjson.Get(configmapData, "AZURE_SUBNET_ID").Exists() {
		ppParam.AZURE_SUBNET_ID = gjson.Get(configmapData, "AZURE_SUBNET_ID").String()
	}
	if gjson.Get(configmapData, "AZURE_NSG_ID").Exists() {
		ppParam.AZURE_NSG_ID = gjson.Get(configmapData, "AZURE_NSG_ID").String()
	}
	if gjson.Get(configmapData, "PROXY_TIMEOUT").Exists() {
		ppParam.PROXY_TIMEOUT = gjson.Get(configmapData, "PROXY_TIMEOUT").String()
	}

	return ppParam, nil
}

func createLibvirtPeerPodSecrets(oc *exutil.CLI, ppParam PeerpodParam, ciSecretName, secretTemplate string) (string, error) {
	configmapDatav2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", ciSecretName, "-n", "default", "-o=jsonpath={.data}").Output()
	if err != nil {
		e2e.Failf("%v Configmap created by QE CI has error, no .data: %v %v", ciSecretName, configmapDatav2, err)
	}

	LIBVIRT_URI := ""
	LIBVIRT_POOL := ""
	LIBVIRT_VOL_NAME := ""
	PROXY_TIMEOUT := ""

	if gjson.Get(configmapDatav2, "LIBVIRT_POOL").Exists() {
		LIBVIRT_POOL = gjson.Get(configmapDatav2, "LIBVIRT_POOL").String()
	}
	if gjson.Get(configmapDatav2, "LIBVIRT_URI").Exists() {
		LIBVIRT_URI = gjson.Get(configmapDatav2, "LIBVIRT_URI").String()
	}
	if gjson.Get(configmapDatav2, "LIBVIRT_VOL_NAME").Exists() {
		LIBVIRT_VOL_NAME = gjson.Get(configmapDatav2, "LIBVIRT_VOL_NAME").String()
	}
	if gjson.Get(configmapDatav2, "PROXY_TIMEOUT").Exists() {
		PROXY_TIMEOUT = gjson.Get(configmapDatav2, "PROXY_TIMEOUT").String()
	}

	// Check for libvirt credentials
	if LIBVIRT_POOL == "" || LIBVIRT_URI == "" || LIBVIRT_VOL_NAME == "" || PROXY_TIMEOUT == "" {
		msg := "Libvirt credentials not found in the data."
		return msg, fmt.Errorf("Libvirt credentials not found")
	}

	// Construct the secretJSON for Libvirt
	secretJSON := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "Opaque",
		"metadata": map[string]string{
			"name":      "peer-pods-secret",
			"namespace": "openshift-sandboxed-containers-operator",
		},
		"stringData": map[string]string{
			"CLOUD_PROVIDER":   "libvirt",
			"LIBVIRT_URI":      LIBVIRT_URI,
			"LIBVIRT_POOL":     LIBVIRT_POOL,
			"LIBVIRT_VOL_NAME": LIBVIRT_VOL_NAME,
		},
	}

	// Marshal the JSON to a string
	secretJSONString, err := json.Marshal(secretJSON)
	if err != nil {
		return "", err
	}

	// Write the JSON string to the secretTemplate file
	err = os.WriteFile(secretTemplate, []byte(secretJSONString), 0644)
	if err != nil {
		return "", err
	}

	return secretTemplate, nil
}

func createAWSPeerPodSecrets(oc *exutil.CLI, ppParam PeerpodParam, ciSecretName, secretTemplate string) (string, error) {
	var (
		secretString  string
		decodedString string
		lines         []string
	)

	// Read peerpods-param-secret to fetch the keys
	secretString, err := oc.AsAdmin().Run("get").Args("secret", ciSecretName, "-n", "default", "-o=jsonpath={.data.aws}").Output()

	if err != nil || secretString == "" {
		e2e.Logf("Error: %v CI provided peer pods secret data empty", err)
		return "", err
	}

	decodedString, err = decodeSecret(secretString)
	if err != nil {
		return "", err
	}

	lines = strings.Split(decodedString, "\n")

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

	// Check for AWS credentials
	if accessKey == "" || secretKey == "" {
		msg := "AWS credentials not found in the data."
		return msg, fmt.Errorf("AWS credentials not found")
	}

	// create AWS specific secret file logic here
	// Construct the secretJSON for AWS
	secretJSON := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "Opaque",
		"metadata": map[string]string{
			"name":      "peer-pods-secret",
			"namespace": "openshift-sandboxed-containers-operator",
		},
		"stringData": map[string]string{
			"AWS_ACCESS_KEY_ID":     accessKey,
			"AWS_SECRET_ACCESS_KEY": secretKey,
		},
	}

	// Marshal the JSON to a string
	secretJSONString, err := json.Marshal(secretJSON)
	if err != nil {
		return "", err
	}

	// Write the JSON string to the secretTemplate file
	err = os.WriteFile(secretTemplate, []byte(secretJSONString), 0644)
	if err != nil {
		return "", err
	}

	return secretTemplate, nil
}

func createAzurePeerPodSecrets(oc *exutil.CLI, ppParam PeerpodParam, ciSecretName, secretTemplate string) (string, error) {
	var (
		secretString  string
		decodedString string
	)

	// Read peerpods-param-secret to fetch the keys
	secretString, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", ciSecretName, "-n", "default", "-o=jsonpath={.data.azure}").Output()

	if err != nil || secretString == "" {
		e2e.Logf("Error: %v CI provided peer pods secret data empty", err)
		return "", err
	}

	decodedString, err = decodeSecret(secretString)
	if err != nil {
		e2e.Logf("Error: %v CI provided peer pods secret data can't be decoded", err)
		return "", err
	}

	//check for all the keys and empty values
	if !(gjson.Get(decodedString, "subscriptionId").Exists() && gjson.Get(decodedString, "clientId").Exists() &&
		gjson.Get(decodedString, "clientSecret").Exists() && gjson.Get(decodedString, "tenantId").Exists()) ||
		gjson.Get(decodedString, "subscriptionId").String() == "" || gjson.Get(decodedString, "clientId").String() == "" ||
		gjson.Get(decodedString, "clientSecret").String() == "" || gjson.Get(decodedString, "tenantId").String() == "" {

		msg := "Azure credentials not found or partial in the data."
		return msg, fmt.Errorf("Azure credentials not found")
	}
	// create Azure specific secret file logic here
	// Construct the secretJSON for Azure
	secretJSON := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "Opaque",
		"metadata": map[string]string{
			"name":      "peer-pods-secret",
			"namespace": "openshift-sandboxed-containers-operator",
		},
		"stringData": map[string]string{
			"AZURE_CLIENT_ID":       gjson.Get(decodedString, "clientId").String(),
			"AZURE_CLIENT_SECRET":   gjson.Get(decodedString, "clientSecret").String(),
			"AZURE_TENANT_ID":       gjson.Get(decodedString, "tenantId").String(),
			"AZURE_SUBSCRIPTION_ID": gjson.Get(decodedString, "subscriptionId").String(),
		},
	}

	// Marshal the JSON to a string
	secretJSONString, err := json.Marshal(secretJSON)
	if err != nil {
		return "", err
	}

	// Write the JSON string to the secretTemplate file
	err = os.WriteFile(secretTemplate, []byte(secretJSONString), 0644)
	if err != nil {
		return "", err
	}

	return secretTemplate, nil
}

// Get the cloud provider type of the test environment copied from test/extended/storage/utils
func getCloudProvider(oc *exutil.CLI) string {
	var (
		errMsg        error
		output        string
		cloudprovider string
	)
	err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		output, errMsg = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if errMsg != nil {
			e2e.Logf("Get cloudProvider *failed with* :\"%v\",wait 5 seconds retry.", errMsg)
			return false, errMsg
		}

		cloudprovider = strings.ToLower(output)
		if cloudprovider == "none" {
			cloudprovider = "libvirt"
		}
		e2e.Logf("The test cluster cloudProvider is :\"%s\".", strings.ToLower(cloudprovider))

		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Waiting for get cloudProvider timeout")
	return strings.ToLower(cloudprovider)
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

func createApplyPeerPodConfigMap(oc *exutil.CLI, provider string, ppParam PeerpodParam, opNamespace, ppConfigMapName, ppConfigMapTemplate string) (msg string, err error) {
	/*
	   Reads the configmap that the CI had applied "peerpods-param-cm"
	   and creates "peer-pods-cm" from it and then applies it on the cluster.

	   Checks if the cluster already has a peer-pods-cm and also for the correct value of the cloud provider
	*/

	var (
		ciCmName   = "peerpods-param-cm"
		configFile string
		imageID    string
	)

	g.By("Checking if peer-pods-cm exists")
	_, err = checkPeerPodConfigMap(oc, opNamespace, provider, ppConfigMapName)
	if err == nil {
		//check for IMAGE ID in the configmap
		msg, err, imageID = CheckPodVMImageID(oc, ppConfigMapName, provider, opNamespace)
		o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("%v imageID: %v err: %v", msg, imageID, err))
		if imageID == "" {
			e2e.Logf("peer-pods-cm in the right state - does not have the IMAGE ID before the kataconfig install , msg: %v", msg)
		} else {
			e2e.Logf("IMAGE ID: %v", imageID)
			msgIfErr := fmt.Sprintf("ERROR: peer-pods-cm has the Image ID before the kataconfig is installed, incorrect state: %v %v %v", imageID, msg, err)
			o.Expect(imageID).NotTo(o.BeEmpty(), msgIfErr)
		}
		e2e.Logf("peer-pods-cm exists - skipping creating it")
		return msg, err

	} else if err != nil {
		e2e.Logf("**** peer-pods-cm not found on the cluster - proceeding to create it****")
	}

	//Read params from peerpods-param-cm and store in ppParam struct
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", ciCmName, "-n", "default").Output()
	if err != nil {
		e2e.Logf("%v Configmap created by QE CI not found: msg %v err: %v", ciCmName, msg, err)
	} else {
		configmapData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", ciCmName, "-n", "default", "-o=jsonpath={.data}").Output()
		if err != nil {
			e2e.Failf("%v Configmap created by QE CI has error, no .data: %v %v", ciCmName, configmapData, err)
		}

		e2e.Logf("configmap Data is:\n%v", configmapData)
		ppParam, err := parseCIPpConfigMapData(provider, configmapData)
		if err != nil {
			return msg, err
		}

		// Create peer-pods-cm file
		if provider == "aws" {
			configFile, err = createAWSPeerPodsConfigMap(oc, ppParam, ppConfigMapTemplate)
		} else if provider == "azure" {
			configFile, err = createAzurePeerPodsConfigMap(oc, ppParam, ppConfigMapTemplate)
		} else if provider == "libvirt" {
			configFile, err = createLibvirtPeerPodsConfigMap(oc, ppParam, ppConfigMapTemplate)
		} else {
			msg = fmt.Sprintf("Cloud provider %v is not supported", provider)
			return msg, fmt.Errorf("%v", msg)
		}

		if err != nil {
			return msg, err
		}

		// Apply peer-pods-cm file
		g.By("(Apply peer-pods-cm file)")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
		if err != nil {
			return fmt.Sprintf("Error: applying peer-pods-cm %v failed: %v %v", configFile, msg, err), err
		}
	}

	return msg, err
}

func createAWSPeerPodsConfigMap(oc *exutil.CLI, ppParam PeerpodParam, ppConfigMapTemplate string) (string, error) {
	g.By("Create peer-pods-cm file")

	// Processing configmap template and create " <randomstring>peer-pods-cm.json"
	configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", ppConfigMapTemplate,
		"-p", "VXLAN_PORT="+ppParam.VXLAN_PORT, "PODVM_INSTANCE_TYPE="+ppParam.PODVM_INSTANCE_TYPE,
		"PROXY_TIMEOUT="+ppParam.PROXY_TIMEOUT, "AWS_REGION="+ppParam.AWS_REGION,
		"AWS_SUBNET_ID="+ppParam.AWS_SUBNET_ID, "AWS_VPC_ID="+ppParam.AWS_VPC_ID,
		"AWS_SG_IDS="+ppParam.AWS_SG_IDS).OutputToFile(getRandomString() + "peer-pods-cm.json")

	if configFile != "" {
		osStatMsg, configFileExists := os.Stat(configFile)
		if configFileExists != nil {
			e2e.Logf("issue creating peer-pods-cm file %s, err: %v , osStatMsg: %v", configFile, err, osStatMsg)
		}
	}

	return configFile, err
}

func createAzurePeerPodsConfigMap(oc *exutil.CLI, ppParam PeerpodParam, ppConfigMapTemplate string) (string, error) {
	g.By("Create peer-pods-cm file")

	// Processing configmap template and create " <randomstring>peer-pods-cm.json"
	configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", ppConfigMapTemplate,
		"-p", "VXLAN_PORT="+ppParam.VXLAN_PORT, "AZURE_INSTANCE_SIZE="+ppParam.AZURE_INSTANCE_SIZE,
		"AZURE_SUBNET_ID="+ppParam.AZURE_SUBNET_ID, "AZURE_NSG_ID="+ppParam.AZURE_NSG_ID,
		"PROXY_TIMEOUT="+ppParam.PROXY_TIMEOUT, "AZURE_REGION="+ppParam.AZURE_REGION,
		"AZURE_RESOURCE_GROUP="+ppParam.AZURE_RESOURCE_GROUP).OutputToFile(getRandomString() + "peer-pods-cm.json")

	if configFile != "" {
		osStatMsg, configFileExists := os.Stat(configFile)
		if configFileExists != nil {
			e2e.Logf("issue creating peer-pods-cm file %s, err: %v , osStatMsg: %v", configFile, err, osStatMsg)
		}
	}

	return configFile, err
}

func createLibvirtPeerPodsConfigMap(oc *exutil.CLI, ppParam PeerpodParam, ppConfigMapTemplate string) (string, error) {
	g.By("Create peer-pods-cm file")

	// Processing configmap template and create " <randomstring>peer-pods-cm.json"
	configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", ppConfigMapTemplate,
		"-p", "PROXY_TIMEOUT="+ppParam.PROXY_TIMEOUT).OutputToFile(getRandomString() + "peer-pods-cm.json")

	if configFile != "" {
		osStatMsg, configFileExists := os.Stat(configFile)
		if configFileExists != nil {
			e2e.Logf("issue creating peer-pods-cm file %s, err: %v , osStatMsg: %v", configFile, err, osStatMsg)
		}
	}

	return configFile, err
}

func createSSHPeerPodsKeys(oc *exutil.CLI, ppParam PeerpodParam, provider string) error {
	g.By("Create ssh keys")

	keyName := "id_rsa_" + getRandomString()
	pubKeyName := keyName + ".pub"
	fromFile := "--from-file=id_rsa.pub=./" + pubKeyName

	shredRMCmd := fmt.Sprintf(`shred -f --remove ./%v ./%v`, keyName, pubKeyName)
	defer exec.Command("bash", "-c", shredRMCmd).CombinedOutput()

	sshKeyGenCmd := fmt.Sprintf(`ssh-keygen -f ./%v -N ""`, keyName)
	retCmd, err := exec.Command("bash", "-c", sshKeyGenCmd).CombinedOutput()
	if err != nil {
		e2e.Logf("the error: %v", string(retCmd))
		return err
	}

	if provider == "libvirt" {
		fromFile = fromFile + " --from-file=id_rsa=./" + keyName
		sshCopyIdCmd := fmt.Sprintf(`ssh-copy-id -i ./%v %v`, pubKeyName, ppParam.LIBVIRT_KVM_HOST_ADDRESS)
		retCmd, err = exec.Command("bash", "-c", sshCopyIdCmd).CombinedOutput()
		if err != nil {
			e2e.Logf("the error: %v", string(retCmd))
			return err
		}
	}
	secretMsg, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-sandboxed-containers-operator",
		"secret", "generic", "ssh-key-secret", fromFile).Output()
	if strings.Contains(secretMsg, "already exists") {
		e2e.Logf(secretMsg)
		return nil
	}
	return err
}

func checkLabeledPodsExpectedRunning(oc *exutil.CLI, resNs, label, expectedRunning string) (msg string, err error) {
	// the inputs are strings to be consistant with other check....() functions.  This is also what the oc command returns
	var (
		resType  = "pod"
		jsonpath = "-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}"
		podList  []string
		podName  string
		number   int
		failMsg  []string
	)

	podList, err = exutil.GetAllPodsWithLabel(oc, resNs, label)
	if err != nil || len(podList) == 0 {
		e2e.Failf("Could not get pod names with %v label: %v %v", label, podList, err)
	}
	number, err = strconv.Atoi(expectedRunning)
	if number != len(podList) || err != nil {
		e2e.Failf("ERROR: Number of pods %v does not match %v expected pods: %v %v", number, expectedRunning, msg, err)
	}

	for _, podName = range podList {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(resType, podName, "-n", resNs, jsonpath).Output()
		if err != nil || strings.ToLower(msg) != "true" {
			failMsg = append(failMsg, fmt.Sprintf("ERROR: %v is not ready: %v %v", podName, msg, err))
		}
	}
	if len(failMsg) != 0 {
		e2e.Failf("%v pods are not ready: %v", len(failMsg), failMsg)
	}
	err = nil
	msg = fmt.Sprintf("All %v pods ready %v)", expectedRunning, podList)
	return msg, err
}

func checkResourceJsonpathMatch(oc *exutil.CLI, resType, resName, resNs, jsonPath1, jsonPath2 string) (expectedMatch, msg string, err error) {
	// the inputs are strings to be consistant with other check....() functions.  This is also what the oc command returns
	var (
		duration time.Duration = 300
		interval time.Duration = 10
	)

	_, _ = checkResourceExists(oc, resType, resName, resNs, duration, interval)

	expectedMatch, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(resType, resName, "-n", resNs, jsonPath1).Output()
	if err != nil || expectedMatch == "" {
		e2e.Failf("ERROR: could not get %v from %v %v: %v %v", jsonPath1, resType, resName, expectedMatch, err)
	}

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(resType, resName, "-n", resNs, jsonPath2).Output()
	if err != nil || msg == "" {
		e2e.Failf("ERROR: could not get %v from %v %v: %v %v", jsonPath2, resType, resName, msg, err)
	}
	if expectedMatch != msg {
		e2e.Failf("ERROR: %v (%) does not match %v (%v)", jsonPath1, expectedMatch, jsonPath2, msg)
	}
	err = nil
	msg = fmt.Sprintf("%v (%v) == %v (%v)", jsonPath1, expectedMatch, jsonPath2, msg)
	return expectedMatch, msg, err
}

func clusterHasEnabledFIPS(oc *exutil.CLI, subscriptionNamespace string) bool {

	firstNode, err := exutil.GetFirstMasterNode(oc)
	msgIfErr := fmt.Sprintf("ERROR Could not get first node to check FIPS '%v' %v", firstNode, err)
	o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
	o.Expect(firstNode).NotTo(o.BeEmpty(), msgIfErr)

	fipsModeStatus, err := oc.AsAdmin().Run("debug").Args("-n", subscriptionNamespace, "node/"+firstNode, "--", "chroot", "/host", "fips-mode-setup", "--check").Output()
	msgIfErr = fmt.Sprintf("ERROR Could not check FIPS on node %v: '%v' %v", firstNode, fipsModeStatus, err)
	o.Expect(err).NotTo(o.HaveOccurred(), msgIfErr)
	o.Expect(fipsModeStatus).NotTo(o.BeEmpty(), msgIfErr)

	// This will be true or false
	return strings.Contains(fipsModeStatus, "FIPS mode is enabled.")
}

func patchPeerPodLimit(oc *exutil.CLI, opNamespace, newLimit string) {
	patchLimit := "{\"spec\":{\"limit\":\"" + newLimit + "\"}}"
	msg, err := oc.AsAdmin().Run("patch").Args("peerpodconfig", "peerpodconfig-openshift", "-n",
		opNamespace, "--type", "merge", "--patch", patchLimit).Output()

	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not patch podvm limit to %v\n error: %v %v", newLimit, msg, err))

	currentLimit := getPeerPodLimit(oc, opNamespace)
	o.Expect(currentLimit).To(o.Equal(newLimit))

	//check node untill the new value is propagated
	jsonpath := "-o=jsonpath='{.status.allocatable.kata\\.peerpods\\.io/vm}'"
	nodeName, _ := exutil.GetFirstWorkerNode(oc)
	nodeLimit, _ := checkResourceJsonpath(oc, "node", nodeName, opNamespace, jsonpath, newLimit, 30*time.Second, 5*time.Second)

	e2e.Logf("node podvm limit is %v", nodeLimit)
	o.Expect(strings.Trim(nodeLimit, "'")).To(o.Equal(newLimit))
}

func getPeerPodLimit(oc *exutil.CLI, opNamespace string) (podLimit string) {
	jsonpathLimit := "-o=jsonpath={.spec.limit}"
	podLimit, err := oc.AsAdmin().Run("get").Args("peerpodconfig", "peerpodconfig-openshift", "-n", opNamespace, jsonpathLimit).Output()
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not find %v in %v\n Error: %v", jsonpathLimit, "peerpodconfig-openshift", err))
	e2e.Logf("peerpodconfig podvm limit is %v", podLimit)
	return podLimit
}

func getPeerPodMetadataInstanceType(oc *exutil.CLI, opNamespace, podName, provider string) (string, error) {
	metadataCurl := map[string][]string{
		"aws":   {"http://169.254.169.254/latest/meta-data/instance-type"},
		"azure": {"-H", "Metadata:true", "\\*", "http://169.254.169.254/metadata/instance/compute/vmSize?api-version=2023-07-01&format=text"},
	}
	podCmd := []string{"-n", opNamespace, podName, "--", "curl", "-s"}
	msg, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(append(podCmd, metadataCurl[provider]...)...).Output()
	return msg, err
}

func CheckPodVMImageID(oc *exutil.CLI, ppConfigMapName, provider, opNamespace string) (msg string, err error, imageID string) {

	cloudProviderMap := map[string]string{
		"aws":   "PODVM_AMI_ID",
		"azure": "AZURE_IMAGE_ID",
	}

	// Fetch the configmap details
	msg, err = oc.AsAdmin().Run("get").Args("configmap", ppConfigMapName, "-n", opNamespace, "-o=jsonpath={.data}").Output()
	if err != nil {
		return "Error fetching configmap details", err, ""
	}

	imageIDParam := cloudProviderMap[provider]

	if !gjson.Get(msg, imageIDParam).Exists() {
		// Handle the case when imageIDParam is not found
		e2e.Logf("Image ID parameter '%s' not found in the config map", imageIDParam)
		return fmt.Sprintf("CM created does not have: %s", imageIDParam), nil, ""
	}

	imageID = gjson.Get(msg, imageIDParam).String()
	if imageID == "" {
		// Handle the case when imageIDParam is an empty string
		e2e.Logf("Image ID parameter found in the config map but is an empty string; Image ID :%s", imageIDParam)
		return fmt.Sprintf("CM created has an empty value for Image ID : %s", imageIDParam), nil, ""
	}
	return "CM does have the Image ID", nil, imageID
}

func getTestRunConfigmap(oc *exutil.CLI, testrun *TestRunDescription, testrunConfigmapNs, testrunConfigmapName string) (configmapExists bool, err error) {
	configmapExists = true
	if testrun.checked { // its been checked
		return configmapExists, nil
	}

	errorMessage := ""
	// testrun.checked should == false

	configmapJson, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", testrunConfigmapNs, testrunConfigmapName, "-o", "json").Output()
	if err != nil {
		e2e.Logf("Configmap is not found: %v %v", configmapJson, err)
		testrun.checked = true // we checked, it doesn't exist
		return false, nil
	}

	// testrun.checked should still == false
	configmapData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", testrunConfigmapNs, testrunConfigmapName, "-o", "jsonpath={.data}").Output()
	if err != nil {
		e2e.Logf("Configmap %v has error %v, no .data: %v %v", testrunConfigmapName, configmapJson, configmapData, err)
		return configmapExists, err
	}
	e2e.Logf("configmap file %v found. Data is:\n%v", testrunConfigmapName, configmapData)

	if gjson.Get(configmapData, "catalogsourcename").Exists() {
		testrun.catalogSourceName = gjson.Get(configmapData, "catalogsourcename").String()
	} else {
		errorMessage = fmt.Sprintf("catalogsourcename is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "channel").Exists() {
		testrun.channel = gjson.Get(configmapData, "channel").String()
	} else {
		errorMessage = fmt.Sprintf("channel is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "redirectNeeded").Exists() {
		testrun.redirectNeeded = gjson.Get(configmapData, "redirectNeeded").Bool()
	} else {
		errorMessage = fmt.Sprintf("redirectNeeded is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "mustgatherimage").Exists() {
		testrun.mustgatherImage = gjson.Get(configmapData, "mustgatherimage").String()
		if strings.Contains(testrun.mustgatherImage, "brew.registry.redhat.io") {
			testrun.redirectNeeded = true
		}
	} else {
		errorMessage = fmt.Sprintf("mustgatherimage is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "eligibility").Exists() {
		testrun.eligibility = gjson.Get(configmapData, "eligibility").Bool()
	} else {
		errorMessage = fmt.Sprintf("eligibility is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "eligibleSingleNode").Exists() {
		testrun.eligibleSingleNode = gjson.Get(configmapData, "eligibleSingleNode").Bool()
	} else {
		errorMessage = fmt.Sprintf("eligibleSingleNode is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "labelSingleNode").Exists() {
		testrun.labelSingleNode = gjson.Get(configmapData, "labelsinglenode").Bool()
	} else {
		errorMessage = fmt.Sprintf("labelSingleNode is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "operatorVer").Exists() {
		testrun.operatorVer = gjson.Get(configmapData, "operatorVer").String()
	} else {
		errorMessage = fmt.Sprintf("operatorVer is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "runtimeClassName").Exists() {
		testrun.runtimeClassName = gjson.Get(configmapData, "runtimeClassName").String()
	} else {
		errorMessage = fmt.Sprintf("runtimeClassName is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "enablePeerPods").Exists() {
		testrun.enablePeerPods = gjson.Get(configmapData, "enablePeerPods").Bool()
	} else {
		errorMessage = fmt.Sprintf("enablePeerPods is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "enableGPU").Exists() {
		testrun.enableGPU = gjson.Get(configmapData, "enableGPU").Bool()
	} else {
		errorMessage = fmt.Sprintf("enableGPU is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "podvmImageUrl").Exists() {
		testrun.podvmImageUrl = gjson.Get(configmapData, "podvmImageUrl").String()
	} else {
		errorMessage = fmt.Sprintf("podvmImageUrl is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "workloadImage").Exists() {
		testrun.workloadImage = gjson.Get(configmapData, "workloadImage").String()
	} else {
		errorMessage = fmt.Sprintf("workloadImage is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "installKataRPM").Exists() {
		testrun.installKataRPM = gjson.Get(configmapData, "installKataRPM").Bool()
	} else {
		errorMessage = fmt.Sprintf("installKataRPM is missing from data\n%v", errorMessage)
	}

	if gjson.Get(configmapData, "workloadToTest").Exists() {
		testrun.workloadToTest = gjson.Get(configmapData, "workloadToTest").String()
		workloadAllowed := false
		for _, v := range allowedWorkloadTypes {
			if v == testrun.workloadToTest {
				workloadAllowed = true
			}
		}
		if !workloadAllowed {
			errorMessage = fmt.Sprintf("workloadToTest (%v) is not one of the allowed workloads (%v)\n%v", testrun.workloadToTest, allowedWorkloadTypes, errorMessage)
		}
	} else {
		errorMessage = fmt.Sprintf("workloadToTest is missing from data\n%v", errorMessage)
	}

	if errorMessage != "" {
		err = fmt.Errorf("%v", errorMessage)
		// testrun.checked still == false. Setup is wrong & all tests will fail
	} else {
		testrun.checked = true // No errors, we checked
	}

	return configmapExists, err
}

func getTestRunParameters(oc *exutil.CLI, subscription *SubscriptionDescription, kataconfig *KataconfigDescription, testrun *TestRunDescription, testrunConfigmapNs, testrunConfigmapName string) (configmapExists bool, err error) {

	configmapExists = true

	if testrun.checked { // already have everything & final values == Input values
		return configmapExists, nil
	}

	configmapExists, err = getTestRunConfigmap(oc, testrun, testrunConfigmapNs, testrunConfigmapName)
	if err != nil {
		// testrun.checked should be false
		return configmapExists, err
	}

	// no errors testrun.checked should be true
	if configmapExists { // Then testrun changed & subscription & kataconfig should too
		subscription.catalogSourceName = testrun.catalogSourceName
		subscription.channel = testrun.channel
		kataconfig.eligibility = testrun.eligibility
		kataconfig.runtimeClassName = testrun.runtimeClassName
		kataconfig.enablePeerPods = testrun.enablePeerPods
	}
	return configmapExists, nil
}

func getUpgradeCatalogConfigMap(oc *exutil.CLI, upgradeCatalog *UpgradeCatalogDescription) (err error) {

	upgradeCatalog.exists = false

	// need a checkResourceExists that doesn't fail when not found.
	configMaps, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", upgradeCatalog.namespace, "-o=jsonpath={.items..metadata.name}").Output()
	if err != nil {
		err = fmt.Errorf("cannot get configmaps in ns %v: Configmaps=[%v] Error:%w", upgradeCatalog.namespace, configMaps, err)
		upgradeCatalog.exists = true // override skip if there is an error
		return err
	}

	if strings.Contains(configMaps, upgradeCatalog.name) {
		upgradeCatalog.exists = true
	}

	if !upgradeCatalog.exists { // no cm is not error
		return nil
	}

	upgradeCatalog.imageAfter, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", upgradeCatalog.namespace, upgradeCatalog.name, "-o=jsonpath={.data.imageAfter}").Output()
	if err != nil || upgradeCatalog.imageAfter == "" {
		err = fmt.Errorf("The %v configmap is missing the imageAfter: %v %v", upgradeCatalog.name, upgradeCatalog.imageAfter, err)
		return err
	}

	upgradeCatalog.imageBefore, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catsrc", "-n", "openshift-marketplace", upgradeCatalog.catalogName, "-o=jsonpath={.spec.image}").Output()
	if err != nil {
		err = fmt.Errorf("Could not get the current image from the %v catsrc %v %v", upgradeCatalog.catalogName, upgradeCatalog.imageBefore, err)
		return err
	}

	return nil
}

func changeCatalogImage(oc *exutil.CLI, catalogName, catalogImage string) (err error) {

	patch := fmt.Sprintf("{\"spec\":{\"image\":\"%v\"}}", catalogImage)
	msg, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("catsrc", catalogName, "--type", "merge", "-p", patch, "-n", "openshift-marketplace").Output()
	if err != nil {
		err = fmt.Errorf("Could not patch %v %v %v", catalogName, msg, err)
		return err
	}

	msg, err = oc.AsAdmin().Run("get").Args("catsrc", catalogName, "-n", "openshift-marketplace", "-o=jsonpath={.spec.image}").Output()
	if err != nil || msg != catalogImage {
		err = fmt.Errorf("Catalog patch did not change image to %v %v %v", catalogImage, msg, err)
		return err
	}

	waitForCatalogReadyOrFail(oc, catalogName)

	return nil
}

func waitForCatalogReadyOrFail(oc *exutil.CLI, catalogName string) {
	_, _ = checkResourceJsonpath(oc, "catsrc", catalogName, "openshift-marketplace", "-o=jsonpath={.status.connectionState.lastObservedState}", "READY", 300*time.Second, 10*time.Second)
}

func checkResourceJsonPathChanged(oc *exutil.CLI, resType, resName, resNs, jsonpath, currentValue string, duration, interval time.Duration) (newValue string, err error) {
	// watch a resource that has a known value until it changes.  Return the new value
	errCheck := wait.PollImmediate(interval, duration, func() (bool, error) {
		newValue, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(resType, resName, "-n", resNs, jsonpath).Output()
		if newValue != currentValue && err == nil {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v %v in ns %v is not in %v state after %v sec: %v %v", resType, resName, resNs, currentValue, duration, newValue, err))
	return newValue, nil
}

func waitForPodsToTerminate(oc *exutil.CLI, namespace, listOfPods string) {
	var (
		podStillRunning bool
		currentPods     string
	)

	errCheck := wait.PollImmediate(10*time.Second, snooze*time.Second, func() (bool, error) {
		podStillRunning = false
		currentPods, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-o=jsonpath={.items..metadata.name}").Output()
		for _, pod := range strings.Fields(listOfPods) {
			if strings.Contains(currentPods, pod) {
				podStillRunning = true
				break
			}
		}
		if podStillRunning {
			return false, nil
		}
		return true, nil
	})
	currentPods, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-o=jsonpath={.items..metadata.name}").Output()
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Timeout waiting for a (%v) pods to terminate.  Current pods %v running", listOfPods, currentPods))
}

func patchPodvmEnableGPU(oc *exutil.CLI, opNamespace, cmName, enableGpu string) {
	patchGPU := "{\"data\":{\"ENABLE_NVIDIA_GPU\":\"" + enableGpu + "\"}}"
	msg, err := oc.AsAdmin().Run("patch").Args("configmap", cmName, "-n",
		opNamespace, "--type", "merge", "--patch", patchGPU).Output()

	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not patch ENABLE_NVIDIA_GPU to %v\n error: %v %v", enableGpu, msg, err))

	currentGPU := getPodvmEnableGPU(oc, opNamespace, cmName)
	o.Expect(currentGPU).To(o.Equal(enableGpu))
}

func getPodvmEnableGPU(oc *exutil.CLI, opNamespace, cmName string) (enGPU string) {
	jsonpath := "-o=jsonpath={.data.ENABLE_NVIDIA_GPU}"
	msg, err := oc.AsAdmin().Run("get").Args("configmap", cmName, "-n", opNamespace, jsonpath).Output()
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Could not find %v in %v\n Error: %v", jsonpath, cmName, err))
	e2e.Logf("ENABLE_NVIDIA_GPU is %v", msg)
	return msg
}

func installKataContainerRPM(oc *exutil.CLI, testrun *TestRunDescription) (rpmName string, err error) {

	workerNodeList, err := exutil.GetClusterNodesBy(oc, "worker")
	if err != nil || len(workerNodeList) < 1 {
		err = fmt.Errorf("Error: no worker nodes: %v, %v", workerNodeList, err)
		return rpmName, err
	}

	rpmName, err = checkNodesForKataContainerRPM(oc, testrun, workerNodeList)
	if err != nil {
		return rpmName, err
	}

	errors := ""
	cmd := fmt.Sprintf("mount -o remount,rw /usr; rpm -Uvh /var/local/%v", rpmName)
	for index := range workerNodeList {
		msg, err := exutil.DebugNodeWithOptionsAndChroot(oc, workerNodeList[index], []string{"-q"}, "/bin/sh", "-c", cmd)
		if !(strings.Contains(msg, "already installed") || strings.Contains(msg, "installing")) {
			if err != nil {
				errors = fmt.Sprintf("%vError trying to rpm -Uvh %v on %v: %v %v\n", errors, rpmName, workerNodeList[index], msg, err)
			}
		}
	}

	if errors != "" {
		err = fmt.Errorf("Error: Scratch rpm errors: %v", errors)
	}
	return rpmName, err
}

func checkNodesForKataContainerRPM(oc *exutil.CLI, testrun *TestRunDescription, workerNodeList []string) (rpmName string, err error) {
	// check if rpm exists
	errors := ""
	msg := ""
	cmd := fmt.Sprintf("ls -1 /var/local | grep '^kata-containers.*rpm$'")
	for index := range workerNodeList {
		msg, err = exutil.DebugNodeWithOptionsAndChroot(oc, workerNodeList[index], []string{"-q"}, "/bin/sh", "-c", cmd)
		if strings.Contains(msg, "kata-containers") && strings.Contains(msg, ".rpm") {
			rpmName = strings.TrimRight(msg, "\n") // need test
		}
		if rpmName == "" {
			errors = fmt.Sprintf("%vError finding /var/local/kata-containers.*rpm on %v: %v %v\n", errors, workerNodeList[index], msg, err)
		}
	}

	if errors != "" {
		err = fmt.Errorf("Errors finding rpm in /var/local: %v", errors)
	}
	return rpmName, err

}
