//Package kata operator tests
package kata

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type subscriptionDescription struct {
	subName                string `json:"name"`
	namespace              string `json:"namespace"`
	channel                string `json:"channel"`
	ipApproval             string `json:"installPlanApproval"`
	operatorPackage        string `json:"spec.name"`
	catalogSourceName      string `json:"source"`
	catalogSourceNamespace string `json:"sourceNamespace"`
	template               string
}

type testrunConfigmap struct {
	exists            bool
	catalogsourceName string
	channel           string
	icspNeeded        bool
	mustgatherImage   string
	katamonitorImage  string
}

var (
	snooze     time.Duration = 2400
	kataSnooze time.Duration = 5400 // Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node

)

// author: tbuskey@redhat.com,abhbaner@redhat.com
func subscribeFromTemplate(oc *exutil.CLI, sub subscriptionDescription, subTemplate, nsFile, ogFile string) (msg string, err error) {
	g.By(" (1) INSTALLING sandboxed-operator in '" + sub.namespace + "' namespace")
	subFile := ""

	g.By("(1.1) Applying namespace yaml")
	msg, err = oc.AsAdmin().Run("apply").Args("-f", nsFile).Output()
	e2e.Logf("STEP namespace %v %v", msg, err)

	g.By("(1.2)  Applying operatorgroup yaml if needed")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "-n", sub.namespace, "--no-headers").Output()
	if strings.Contains(msg, "No resources found in") {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", ogFile, "-n", sub.namespace).Output()
	}
	e2e.Logf("STEP operator group %v %v", msg, err)

	g.By("(1.3) Creating subscription yaml from template")
	subFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", sub.template, "-p", "SUBNAME="+sub.subName, "SUBNAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"APPROVAL="+sub.ipApproval, "OPERATORNAME="+sub.operatorPackage, "SOURCENAME="+sub.catalogSourceName, "SOURCENAMESPACE="+sub.catalogSourceNamespace, "-n", sub.namespace).OutputToFile(getRandomString() + "subscriptionFile.json")
	// o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Created the subscription yaml %s, %v", subFile, err)

	g.By("(1.4) Applying subscription yaml")
	// no need to check for an existing subscription
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subFile).Output()
	e2e.Logf("Applied subscription %v: %v, %v", subFile, msg, err)

	g.By("(1.5) Verify the operator finished subscribing")
	msg, err = subscriptionIsFinished(oc, sub)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())

	return msg, err
}

// author: tbuskey@redhat.com, abhbaner@redhat.com
func createKataConfig(oc *exutil.CLI, kcTemplate, kcName, kcMonitorImageName, kcLogLevel string, sub subscriptionDescription) (msg string, err error) {
	// If this is used, label the caller with [Disruptive][Serial][Slow]
	// If kataconfig already exists, this must not error
	var (
		configFile string
	)

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", "--no-headers", "-n", sub.namespace).Output()
	if strings.Contains(msg, kcName) {
		g.By("(2) kataconfig is previously installed")
		return msg, err // no need to go through the rest
	}

	g.By("Make sure subscription has finished before kataconfig")
	msg, err = subscriptionIsFinished(oc, sub)
	if err != nil {
		e2e.Logf("The subscription has not finished: %v %v", msg, err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(msg).NotTo(o.BeEmpty())

	g.By("(2) Create kataconfig file")
	configFile, err = oc.AsAdmin().WithoutNamespace().Run("process").Args("--ignore-unknown-parameters=true", "-f", kcTemplate, "-p", "NAME="+kcName, "MONITOR="+kcMonitorImageName, "LOGLEVEL="+kcLogLevel, "-n", sub.namespace).OutputToFile(getRandomString() + "kataconfig-common.json")
	e2e.Logf("the kataconfig file is %s, %v", configFile, err)

	g.By("(2.1) Apply kataconfig file")
	errCheck := wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
		e2e.Logf("%v %v", msg, err)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("applying kataconfig %v failed: %v %v", configFile, msg, err))
	// -o=jsonpath={.status.installationStatus.IsInProgress} "" at this point

	g.By("(2.2) Check kataconfig creation has started")
	errCheck = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", "--no-headers").Output()
		if strings.Contains(msg, kcName) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("kataconfig %v did not get created: %v %v", kcName, msg, err))
	// -o=jsonpath={.status.installationStatus.IsInProgress} "True" at this point

	g.By("(2.3) Wait for kataconfig to finish install")
	// Installing/deleting kataconfig reboots nodes.  AWS BM takes 20 minutes/node
	errCheck = wait.Poll(30*time.Second, kataSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "-o=jsonpath={.status.installationStatus.IsInProgress}").Output()
		if strings.Contains(msg, "false") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("kataconfig %v did not finish install", kcName))

	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig", kcName, "--no-headers").Output()
	msg = "SUCCESS kataconfig is created " + msg
	return msg, err
}

// author: abhbaner@redhat.com
func createKataPod(oc *exutil.CLI, podNs, commonPod, commonPodName string) string {
	//Team - creating unique pod names to avoid pod name clash (named "example") for parallel test execution; pod name eg: e3ytylt9example
	newPodName := getRandomString() + commonPodName
	configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", commonPod, "-p", "NAME="+newPodName).OutputToFile(getRandomString() + "Pod-common.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the file of resource is %s", configFile)

	oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile, "-n", podNs).Execute()

	//validating kata runtime
	podsRuntime, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", newPodName, "-n", podNs, "-o=jsonpath={.spec.runtimeClassName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podsRuntime).To(o.ContainSubstring("kata"))
	e2e.Logf("The runtime used for this pod is %s", podsRuntime)
	return newPodName
}

// author: abhbaner@redhat.com
func deleteKataPod(oc *exutil.CLI, podNs, newPodName string) bool {
	e2e.Logf("delete pod %s in namespace %s", newPodName, podNs)
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", newPodName, "-n", podNs).Execute()
	return true
}

// author: abhbaner@redhat.com
func checkKataPodStatus(oc *exutil.CLI, podNs, newPodName string) {
	errCheck := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		podsStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", newPodName, "-n", podNs, "-o=jsonpath={.status.phase}").Output()
		if strings.Contains(podsStatus, "Running") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Pod %v is not correct status in ns %v", newPodName, podNs))
	e2e.Logf("Pod %s in namespace %s is Running", newPodName, podNs)
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
	g.By("(3) Deleting kataconfig")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("kataconfig", kcName).Output()
	e2e.Logf("%v %v", msg, err)

	g.By("(3.1) Wait for kataconfig to be deleted")
	errCheck := wait.Poll(30*time.Second, kataSnooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("kataconfig").Output()
		if strings.Contains(msg, "No resources found") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("kataconfig %v did not get deleted: %v %v", kcName, msg, err))

	g.By("(3.2) kataconfig is gone")
	return msg, err
}

func getVersionInfo(oc *exutil.CLI, sub subscriptionDescription, opNamespace, subTemplate string) (string, subscriptionDescription) {

	var operatorVer = "1.2.0" //default val
	ConfigMapFound, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap/example-config-env", "-n", "default").Output()
	//If CM exists it means its a Jenkins CI
	if strings.Contains(ConfigMapFound, "example-config-env") && strings.Contains(ConfigMapFound, "4") {
		configMap, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("configmap/example-config-env", "-n", "default").Output()
		versions := strings.Split(configMap, "\n")
		ocpMajorVer := versions[9]
		ocpMinorVer := versions[12]
		channelName := versions[15]
		operatorVer := versions[18]
		e2e.Logf("ocpMajorVer : %s", ocpMajorVer)
		e2e.Logf("ocpMinorVer : %s", ocpMinorVer)
		e2e.Logf("operatorVer : %s", operatorVer)
		e2e.Logf("Channel : %s", channelName)
		//for CI runs - catsrcName set
		catsrcName := "kataci-index"
		e2e.Logf("catalogSourceName : %s", catsrcName)
		os.Setenv("cmMsg", "True")
		sub.catalogSourceName = catsrcName
		sub.channel = channelName
		return operatorVer, sub
	}

	return operatorVer, sub
}

func subscriptionIsFinished(oc *exutil.CLI, sub subscriptionDescription) (msg string, err error) {
	var (
		csvName string
		v       string
	)
	g.By("Check that operator is AtLatestKnown")
	errCheck := wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}").Output()
		// o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(msg, "AtLatestKnown") == 0 {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "--no-headers").Output()
			return true, nil
		}
		return false, nil
	})
	e2e.Logf("Subscription %v %v, %v", msg, err, errCheck)
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription %v is not correct status in ns %v", sub.subName, sub.namespace))

	g.By("Get csvName to check its finish")
	csvName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}").Output()
	// e2e.Logf("csvName %v %v", csvName, err)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())

	g.By("Check that the csv '" + csvName + "' has finished")
	errCheck = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", sub.namespace, "-o=jsonpath={.status.phase}{.status.reason}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(msg, "SucceededInstallSucceeded") == 0 {
			v, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", sub.namespace, "--no-headers").Output()
			msg = fmt.Sprintf("%v state %v", v, msg)
			return true, nil
		}
		return false, nil
	})
	e2e.Logf("csv %v: %v %v", csvName, msg, err)
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status in ns %v: %v %v", csvName, sub.namespace, msg, err))
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.subName, "-n", sub.namespace, "--no-headers").Output()
	return msg, err

}

// author: valiev@redhat.com
func getNodeListByLabel(oc *exutil.CLI, labelKey string) (nodeNameList []string, msg string, err error) {
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", labelKey, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeNameList = strings.Fields(msg)
	return nodeNameList, msg, err
}

// author: tbuskey@redhat.com
func waitForNodesInDebug(oc *exutil.CLI) (msg string, err error) {
	count := 0
	workerNodeList, msg, err := getNodeListByLabel(oc, "node-role.kubernetes.io/worker=")
	o.Expect(err).NotTo(o.HaveOccurred())
	workerNodeCount := len(workerNodeList)
	if workerNodeCount < 1 {
		e2e.Logf("ERROR no worker nodes: %v, %v %v", workerNodeList, msg, err)
	}
	o.Expect(workerNodeList).NotTo(o.BeEmpty())
	e2e.Logf("Waiting for %v nodes to enter debug: %v", workerNodeCount, workerNodeList)

	// loop all workers until they all have debug
	errCheck := wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		count = 0
		for index := range workerNodeList {
			msg, err = oc.AsAdmin().Run("debug").Args("node/"+workerNodeList[index], "--", "chroot", "/host", "crio", "config").Output()
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
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("only %v of %v total worker nodes are in debug: %v\n %v", count, workerNodeCount, workerNodeList, msg, err))
	msg = fmt.Sprintf("All %v worker nodes are in debug mode: %v", workerNodeCount, workerNodeList)
	err = nil
	return msg, err
}

// author: tbuskey@redhat.com
func imageContentSourcePolicy(oc *exutil.CLI, configFile, name string) (msg string, err error) {
	g.By("Applying ImageContentSourcePolicy")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
	errCheck := wait.Poll(10*time.Second, 360*time.Second, func() (bool, error) {
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--no-headers").Output()
		if strings.Contains(msg, name) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("applying ImageContentSourcePolicy %v failed: %v %v", configFile, msg, err))
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

func getTestRunInput(oc *exutil.CLI, subscription subscriptionDescription, katamonitorImage, mustgatherImage, cmNs, cmName string) (testrun testrunConfigmap, msg string, err error) {
	testrun = testrunConfigmap{
		exists:            false,
		catalogsourceName: subscription.catalogSourceName,
		channel:           subscription.channel,
		icspNeeded:        false,
		mustgatherImage:   mustgatherImage,
		katamonitorImage:  katamonitorImage,
	}

	// is it there?
	msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", cmNs, cmName).Output()
	if err != nil {
		e2e.Logf("STEP Configmap is not found: msg %v err: %v", msg, err)
		testrun.exists = false
	} else {
		testrun.exists = true

		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", cmNs, cmName, "-o=jsonpath={.data}").Output()
		e2e.Logf("STEP .data from %v: %v %v", cmName, msg, err)

		// look at all the items for a value.  If they are not empty, change the defaults
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.catalogsourcename}", "-n", cmNs).Output()
		if err == nil {
			e2e.Logf("STEP testrun catalogsourcename %v, err %v", msg, err)
			testrun.catalogsourceName = msg
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.channel}", "-n", cmNs).Output()
		if err == nil {
			e2e.Logf("STEP testrun channel %v, err %v", msg, err)
			testrun.channel = msg
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.icspneeded}", "-n", cmNs).Output()
		if err == nil {
			e2e.Logf("STEP testrun icspneeded %v, err %v", msg, err)
			testrun.icspNeeded, err = strconv.ParseBool(msg)
			if err != nil {
				e2e.Failf("Error in %v config map.  icspneeded must be a golang true or false string", cmName)
			}
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.katamonitormage}", "-n", cmNs).Output()
		if err == nil {
			e2e.Logf("STEP testrun katamonitormage %v, err %v", msg, err)
			testrun.katamonitorImage = msg
		}
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-o=jsonpath={.data.mustgatherimage}", "-n", cmNs).Output()
		if err == nil {
			e2e.Logf("STEP testrun mustgatherimage %v, err %v", msg, err)
			testrun.mustgatherImage = msg
		}
	}
	return testrun, msg, err
}
