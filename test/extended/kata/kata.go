//Package kata operator tests
package kata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-kata] Kata", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("kata", exutil.KubeConfigPath())
		opNamespace          = "openshift-sandboxed-containers-operator"
		commonKataConfigName = "example-kataconfig"
		testDataDir          = exutil.FixturePath("testdata", "kata")
		iaasPlatform         string
		kcTemplate           = filepath.Join(testDataDir, "kataconfig.yaml")
		defaultDeployment    = filepath.Join(testDataDir, "deployment-example.yaml")
		subTemplate          = filepath.Join(testDataDir, "subscription_template.yaml")
		kcLogLevel           = "info"
		kcMonitorImageName   = "registry.redhat.io/openshift-sandboxed-containers/osc-monitor-rhel8:1.2.0"
		mustGatherImage      = "registry.redhat.io/openshift-sandboxed-containers/osc-must-gather-rhel8:1.3.0"
		icspName             = "kata-brew-registry"
		icspFile             = filepath.Join(testDataDir, "ImageContentSourcePolicy-brew.yaml")
		testrunInitial       testrunConfigmap
		clusterVersion       string
		ocpMajorVer          string
		ocpMinorVer          string
		operatorVer          = "1.2.0"
		testrun              testrunConfigmap
	)

	subscription := subscriptionDescription{
		subName:                "sandboxed-containers-operator",
		namespace:              opNamespace,
		catalogSourceName:      "redhat-operators",
		catalogSourceNamespace: "openshift-marketplace",
		channel:                "stable-1.2",
		ipApproval:             "Automatic",
		operatorPackage:        "sandboxed-containers-operator",
		template:               subTemplate,
	}

	testrunInitial.exists = false // no overrides yet

	testrunDefault := testrunConfigmap{
		exists:            false,
		catalogSourceName: subscription.catalogSourceName,
		channel:           subscription.channel,
		icspNeeded:        false,
		mustgatherImage:   mustGatherImage,
		katamonitorImage:  kcMonitorImageName,
	}

	g.BeforeEach(func() {
		// Creating/deleting kataconfig reboots all worker node and extended-platform-tests may timeout.
		// --------- AWS baremetal may take >20m per node ----------------
		// add --timeout 70m
		// tag with [Slow][Serial][Disruptive] when deleting/recreating kataconfig

		var (
			err error
			msg string
		)

		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		iaasPlatform = strings.ToLower(msg)
		e2e.Logf("the current platform is %v", iaasPlatform)

		ocpMajorVer, ocpMinorVer, clusterVersion = getClusterVersion(oc)
		e2e.Logf("Running %v.%v on: %v", ocpMajorVer, ocpMinorVer, clusterVersion)

		// check if there is a CM override
		testrunInitial, msg, err = getTestRunConfigmap(oc, testrunDefault, "default", "osc-config")
		if testrunInitial.exists { // then override
			subscription.catalogSourceName = testrunInitial.catalogSourceName
			subscription.channel = testrunInitial.channel
			mustGatherImage = testrunInitial.mustgatherImage
			kcMonitorImageName = testrunInitial.katamonitorImage
			operatorVer = testrunInitial.operatorVer
			e2e.Logf("subscription after testrun cm osc-config: %v", subscription)
		}

		operatorVer, sub := getVersionInfo(oc, subscription, operatorVer)
		if os.Getenv("cmMsg") != "" { //env var cmMsg will have no value if configmap is not found
			subscription.catalogSourceName = sub.catalogSourceName
			subscription.channel = sub.channel
			kcMonitorImageName = "registry.redhat.io/openshift-sandboxed-containers/osc-monitor-rhel8:" + operatorVer
			e2e.Logf("subscription after Jenkins cm example-config-env: %v", subscription)
			e2e.Logf("operatorVer : %s", operatorVer)
			e2e.Logf("monitor : %s", kcMonitorImageName)
		}

		// check env to override defaults or CM
		testrun, msg = getTestRunEnvVars("OSCS", testrunDefault)
		// change subscription to match testrun.  env options override default and CM values
		if testrun.exists {
			testrunInitial = testrun
			subscription.catalogSourceName = testrunInitial.catalogSourceName
			subscription.channel = testrunInitial.channel
			mustGatherImage = testrunInitial.mustgatherImage
			kcMonitorImageName = testrunInitial.katamonitorImage
			operatorVer = testrunInitial.operatorVer
			e2e.Logf("environment OSCS found. subscription: %v", subscription)
		}

		if testrunInitial.icspNeeded {
			e2e.Logf("An ICSP is being applied to allow %v and %v to work", testrunInitial.katamonitorImage, testrunInitial.mustgatherImage)
			msg, err = imageContentSourcePolicy(oc, icspFile, icspName)
			if err != nil || msg == "" {
				logErrorAndFail(oc, fmt.Sprintf("Error: applying ICSP"), msg, err)
			}
		}

		ns := filepath.Join(testDataDir, "namespace.yaml")
		og := filepath.Join(testDataDir, "operatorgroup.yaml")

		msg, err = subscribeFromTemplate(oc, subscription, subTemplate, ns, og)
		e2e.Logf("---------- subscription %v succeeded with channel %v %v", subscription.subName, subscription.channel, err)

		msg, err = createKataConfig(oc, kcTemplate, commonKataConfigName, kcMonitorImageName, kcLogLevel, subscription)
		e2e.Logf("---------- kataconfig %v create succeeded %v %v", commonKataConfigName, msg, err)
	})

	g.It("Author:abhbaner-High-39499-Operator installation", func() {
		g.By("Checking sandboxed-operator operator installation")
		e2e.Logf("Operator install check successfull as part of setup !!!!!")
		g.By("SUCCESSS - sandboxed-operator operator installed")

	})

	g.It("Author:abhbaner-High-43522-Common Kataconfig installation", func() {
		g.By("Install Common kataconfig and verify it")
		e2e.Logf("common kataconfig %v is installed", commonKataConfigName)
		g.By("SUCCESSS - kataconfig installed")

	})

	g.It("Author:abhbaner-High-41566-High-41574-deploy & delete a pod with kata runtime", func() {
		commonPodName := "example"
		commonPod := filepath.Join(testDataDir, "example.yaml")

		oc.SetupProject()
		podNs := oc.Namespace()

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, commonPod, commonPodName)
		defer deleteKataPod(oc, podNs, newPodName)
		checkKataPodStatus(oc, podNs, newPodName)
		e2e.Logf("Pod (with Kata runtime) with name -  %v , is installed", newPodName)
		g.By("SUCCESS - Pod with kata runtime installed")
		g.By("TEARDOWN - deleting the kata pod")
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-43238-Operator prohibits creation of multiple kataconfigs", func() {
		var (
			kataConfigName2 = commonKataConfigName + "2"
			configFile      string
			msg             string
			err             error
			kcTemplate      = filepath.Join(testDataDir, "kataconfig.yaml")
		)
		g.By("Create 2nd kataconfig file")
		configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", kcTemplate, "-p", "NAME="+kataConfigName2, "-n", subscription.namespace).OutputToFile(getRandomString() + "kataconfig-common.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the file of resource is %s", configFile)

		g.By("Apply 2nd kataconfig")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
		o.Expect(msg).To(o.ContainSubstring("KataConfig instance already exists"))
		e2e.Logf("err %v, msg %v", err, msg)

		g.By("Success - cannot apply 2nd kataconfig")

	})

	g.It("Author:abhbaner-High-41263-Namespace check", func() {
		g.By("Checking if ns 'openshift-sandboxed-containers-operator' exists")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("namespaces").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring(subscription.namespace))
		g.By("SUCCESS - Namespace check complete")

	})

	g.It("Author:abhbaner-High-43620-validate podmetrics for pod running kata", func() {
		commonPodName := "example"
		commonPod := filepath.Join(testDataDir, "example.yaml")

		oc.SetupProject()
		podNs := oc.Namespace()

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, commonPod, commonPodName)
		defer deleteKataPod(oc, podNs, newPodName)
		checkKataPodStatus(oc, podNs, newPodName)

		errCheck := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			podMetrics, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("podmetrics", newPodName, "-n", podNs).Output()
			if err != nil {
				e2e.Logf("error  %v, please try next round", err)
				return false, nil
			}
			e2e.Logf("Pod metrics output below  \n %s ", podMetrics)
			o.Expect(podMetrics).To(o.ContainSubstring("Cpu"))
			o.Expect(podMetrics).To(o.ContainSubstring("Memory"))
			o.Expect(podMetrics).To(o.ContainSubstring("Events"))
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("can not describe podmetrics %v in ns %v", newPodName, podNs))
		g.By("SUCCESS - Podmetrics for pod with kata runtime validated")
		g.By("TEARDOWN - deleting the kata pod")
	})

	g.It("Author:abhbaner-High-43617-High-43616-CLI checks pod logs & fetching pods in podNs", func() {
		commonPodName := "example"
		commonPod := filepath.Join(testDataDir, "example.yaml")

		oc.SetupProject()
		podNs := oc.Namespace()

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, commonPod, commonPodName)
		defer deleteKataPod(oc, podNs, newPodName)

		/* checkKataPodStatus prints the pods with the podNs and validates if
		its running or not thus verifying OCP-43616 */

		checkKataPodStatus(oc, podNs, newPodName)
		e2e.Logf("Pod (with Kata runtime) with name -  %v , is installed", newPodName)
		errCheck := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
			podlogs, err := oc.AsAdmin().Run("logs").WithoutNamespace().Args("pod/"+newPodName, "-n", podNs).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(podlogs).NotTo(o.BeEmpty())
			if strings.Contains(podlogs, "httpd") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Pod logs are not getting generated"))
		g.By("SUCCESS - Logs for pods with kata validated")
		g.By("TEARDOWN - deleting the kata pod")
	})

	g.It("Author:abhbaner-High-43514-kata pod displaying correct overhead", func() {
		commonPodName := "example"
		commonPod := filepath.Join(testDataDir, "example.yaml")

		oc.SetupProject()
		podNs := oc.Namespace()

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, commonPod, commonPodName)
		defer deleteKataPod(oc, podNs, newPodName)
		checkKataPodStatus(oc, podNs, newPodName)
		e2e.Logf("Pod (with Kata runtime) with name -  %v , is installed", newPodName)

		g.By("Checking Pod Overhead")
		podoverhead, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("runtimeclass", "kata").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podoverhead).NotTo(o.BeEmpty())
		o.Expect(podoverhead).To(o.ContainSubstring("Overhead"))
		o.Expect(podoverhead).To(o.ContainSubstring("Cpu"))
		o.Expect(podoverhead).To(o.ContainSubstring("Memory"))
		g.By("SUCCESS - kata pod overhead verified")
		g.By("TEARDOWN - deleting the kata pod")
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-43619-oc admin top pod works for pods that use kata runtime", func() {

		oc.SetupProject()
		var (
			commonPodTemplate = filepath.Join(testDataDir, "example.yaml")
			podNs             = oc.Namespace()
			podName           string
			err               error
			msg               string
			waitErr           error
			metricCount       = 0
		)

		g.By("Deploy a pod with kata runtime")
		podName = createKataPod(oc, podNs, commonPodTemplate, "admtop")
		defer deleteKataPod(oc, podNs, podName)
		checkKataPodStatus(oc, podNs, podName)

		g.By("Get oc top adm metrics for the pod")
		snooze = 360
		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "pod", "-n", podNs, podName, "--no-headers").Output()
			if err == nil { // Will get error with msg: error: metrics not available yet
				metricCount = len(strings.Fields(msg))
			}
			if metricCount == 3 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "metrics never appeared")
		if metricCount == 3 {
			e2e.Logf("metrics for pod %v", msg)
		}
		o.Expect(metricCount).To(o.Equal(3))

		g.By("Success")

	})

	g.It("Author:abhbaner-High-43516-operator is available in CatalogSource", func() {

		g.By("Checking catalog source for the operator")
		opMarketplace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", "-n", "openshift-marketplace").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(opMarketplace).NotTo(o.BeEmpty())
		o.Expect(opMarketplace).To(o.ContainSubstring("sandboxed-containers-operator"))
		o.Expect(opMarketplace).To(o.ContainSubstring("Red Hat Operators"))
		g.By("SUCCESS -  'sandboxed-containers-operator' is present in packagemanifests")

	})

	g.It("Longduration-NonPreRelease-Author:abhbaner-High-43523-Monitor Kataconfig deletion[Disruptive][Serial][Slow]", func() {
		g.By("Delete kataconfig and verify it")
		msg, err := deleteKataConfig(oc, commonKataConfigName)
		e2e.Logf("kataconfig %v was deleted\n--------- %v %v", commonKataConfigName, msg, err)

		g.By("Recreating kataconfig in 43523 for the remaining test cases")
		msg, err = createKataConfig(oc, kcTemplate, commonKataConfigName, kcMonitorImageName, kcLogLevel, subscription)
		e2e.Logf("recreated kataconfig %v: %v %v", commonKataConfigName, msg, err)

		g.By("SUCCESS")
	})

	g.It("Longduration-NonPreRelease-Author:abhbaner-High-41813-Build Acceptance test[Disruptive][Serial][Slow]", func() {
		//This test will install operator,kataconfig,pod with kata - delete pod, delete kataconfig
		commonPodName := "example"
		commonPod := filepath.Join(testDataDir, "example.yaml")

		oc.SetupProject()
		podNs := oc.Namespace()

		g.By("Deploying pod with kata runtime and verify it")
		newPodName := createKataPod(oc, podNs, commonPod, commonPodName)
		checkKataPodStatus(oc, podNs, newPodName)
		e2e.Logf("Pod (with Kata runtime) with name -  %v , is installed", newPodName)
		deleteKataPod(oc, podNs, newPodName)
		g.By("Kata Pod deleted - now deleting kataconfig")

		msg, err := deleteKataConfig(oc, commonKataConfigName)
		e2e.Logf("common kataconfig %v was deleted %v %v", commonKataConfigName, msg, err)
		g.By("SUCCESSS - build acceptance passed")

		g.By("Recreating kataconfig for the remaining test cases")
		msg, err = createKataConfig(oc, kcTemplate, commonKataConfigName, kcMonitorImageName, kcLogLevel, subscription)
		e2e.Logf("recreated kataconfig %v: %v %v", commonKataConfigName, msg, err)
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-46235-Kata Metrics Verify that Namespace is labeled to enable monitoring", func() {
		var (
			err        error
			msg        string
			s          string
			label      = ""
			hasMetrics = false
		)

		g.By("Get labels of openshift-sandboxed-containers-operator namespace to check for monitoring")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "openshift-sandboxed-containers-operator", "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, s = range strings.SplitAfter(msg, ",") {
			if strings.Contains(s, "openshift.io/cluster-monitoring") {
				label = s
				if strings.Contains(strings.SplitAfter(s, ":")[1], "true") {
					hasMetrics = true
				}
			}
		}
		o.Expect(strings.Contains(msg, "openshift.io/cluster-monitoring")).To(o.BeTrue())
		e2e.Logf("Label is %v", label)
		o.Expect(hasMetrics).To(o.BeTrue())

		g.By("Success")
	})

	g.It("Author:abhbaner-High-43524-Existing deployments (with runc) should restart normally after kata runtime install", func() {

		oc.SetupProject()
		var (
			podNs      = oc.Namespace()
			deployName = "dep-43524"
			msg        string
			podName    string
			newPodName string
		)

		g.By("Create deployment config from template")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", defaultDeployment, "-p", "NAME="+deployName).OutputToFile(getRandomString() + "dep-common.json")
		if err != nil {
			e2e.Logf("Could not create configFile %v %v", configFile, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Applying deployment file " + configFile)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile, "-n", podNs).Output()
		if err != nil {
			e2e.Logf("Could not apply configFile %v %v", msg, err)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for deployment to be ready")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("deploy", "-n", podNs, deployName, "--ignore-not-found").Execute()
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())

		g.By("Wait for pods to be ready")
		errCheck := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", podNs, "--no-headers").Output()
			if !strings.Contains(msg, "No resources found") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Timed out waiting for pods %v %v", msg, err))

		g.By("Get pod name")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", podNs, "--no-headers").Output()
		podName = strings.Split(msg, " ")[0]
		e2e.Logf("podname %v %v", msg, err)

		msg = fmt.Sprintf("Deleting pod %v from deployment", podName)
		g.By(msg)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podName, "-n", podNs).Output()
		e2e.Logf("%v pod deleted: %v %v", podName, msg, err)

		g.By("Wait for deployment to re-replicate")
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())

		g.By("Get new pod name")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", podNs, "--no-headers").Output()
		newPodName = strings.Split(msg, " ")[0]
		e2e.Logf("new podname %v %v", msg, err)
		if newPodName == podName {
			e2e.Failf("A new pod did not get created")
		}

		g.By("SUCCESSS - kataconfig installed and post that pod with runc successfully restarted ")
	})

	// author: tbuskey@redhat.com
	g.It("Longduration-NonPreRelease-Author:tbuskey-High-42167-Must-gather collects sandboxed operator logs[Serial]", func() {

		type counts struct {
			audits           int
			crio             int
			qemuLogs         int
			qemuVersion      int
			describeCsv      int
			describeKc       int
			describeServices int
			describeSub      int
			describeVwebhook int
		}

		oc.SetupProject()

		var (
			crioFile              string
			crioRuntimeConfigName = "crio-debug-42167"
			crioRuntimeLogLevel   = "debug"
			crioTemplate          = filepath.Join(testDataDir, "containerruntimeconfig_template.yaml")
			deployConfigFile      = ""
			deployName            = "mg-42167"
			deploymentTemplate    = filepath.Join(testDataDir, "deployment-example.yaml")
			deploymentFile        = getRandomString() + "dep-common.json"
			err                   error
			fails                 = 0
			kcLogLevel            = "{\"spec\":{\"logLevel\":\"debug\"}}"
			logFile               string
			mustgatherFiles       = []string{""}
			mustgatherName        = "mustgather" + getRandomString()
			mustgatherDir         = "/tmp/" + mustgatherName
			mustgatherLog         = mustgatherName + ".log"
			mustgatherTopdir      string
			msg                   string
			nodeControlCount      = 0
			nodeWorkerCount       = 0
			podNs                 = oc.Namespace()
			singleNode            = false
		)

		mustgatherChecks := counts{
			audits:           0,
			crio:             0,
			qemuLogs:         0,
			qemuVersion:      0,
			describeCsv:      0,
			describeKc:       0,
			describeServices: 0,
			describeSub:      0,
			describeVwebhook: 0,
		}

		nodeControlList, msg, err := getNodeListByLabel(oc, subscription.namespace, "node-role.kubernetes.io/master=")
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeControlCount = len(nodeControlList)

		nodeWorkerList, msg, err := getNodeListByLabel(oc, subscription.namespace, "node-role.kubernetes.io/worker=")
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeWorkerCount = len(nodeWorkerList)

		mustgatherExpected := counts{
			audits:           nodeWorkerCount,
			crio:             nodeWorkerCount + nodeControlCount,
			qemuLogs:         nodeWorkerCount, // Need to change from deployment
			qemuVersion:      nodeWorkerCount,
			describeCsv:      1,
			describeKc:       1,
			describeServices: 1,
			describeSub:      1,
			describeVwebhook: 1,
		}

		// for SNO
		if nodeWorkerCount == 1 && !strings.Contains(nodeWorkerList[0], "worker") {
			singleNode = true
			mustgatherExpected.crio = nodeWorkerCount
		}

		g.By("Create ContainerRuntimeConfig to put worker nodes into debug mode")
		// or logLevel: debug in kataconfig for 1.3 will already do it
		crioFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", crioTemplate, "-p", "NAME="+crioRuntimeConfigName, "LOGLEVEL="+crioRuntimeLogLevel, "-n", subscription.namespace).OutputToFile(getRandomString() + "-crioRuntimeConfigFile.json")
		e2e.Logf("Created the ContainerRuntimeConfig yaml %s, %v", crioFile, err)

		g.By("Applying ContainerRuntimeConfig yaml")
		// no need to check for an existing one
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", crioFile).Output()
		e2e.Logf("Applied ContainerRuntimeConfig %v: %v, %v", crioFile, msg, err)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("containerruntimeconfig", crioRuntimeConfigName, "-n", subscription.namespace, "--ignore-not-found").Execute()
		// 4.12 needs the loglevel
		msg, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kataconfig", commonKataConfigName, "-n", subscription.namespace, "--type", "merge", "--patch", kcLogLevel).Output()
		e2e.Logf("kcLogLevel patched: %v %v", msg, err)

		// oc patch kataconfig example-kataconfig --type merge --patch '{"spec":{"logLevel":"debug"}}'

		g.By("Wait for worker nodes to be in crio debug mode")
		msg, err = waitForNodesInDebug(oc, subscription.namespace)
		e2e.Logf("%v", msg)

		g.By("Create a deployment file from template")
		// This creates N replicas where N=worker node
		// It does not ensure that there is a replica on each worker node.
		/* Loop because on 4.12 SNO, nodes might not respond at 1st
		error: unable to process template
		service unavailable
		exit status 1
		*/
		errCheck := wait.Poll(10*time.Second, 360*time.Second, func() (bool, error) {
			deployConfigFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", deploymentTemplate, "-p", "NAME="+deployName, "-p", "NAMESPACE="+podNs, "-p", "REPLICAS="+fmt.Sprintf("%v", nodeWorkerCount)).OutputToFile(deploymentFile)
			if strings.Contains(deployConfigFile, deploymentFile) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Error: Unable to create deployment file from template: %v %v", deployConfigFile, err))
		o.Expect(deployConfigFile).NotTo(o.BeEmpty())

		g.By("Apply deployment " + deployConfigFile)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", deployConfigFile, "-n", podNs).Output()
		e2e.Logf("Applied deployment %v: %v %v", deployName, msg, err)

		g.By("Wait for deployment to be ready")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("deploy", "-n", podNs, deployName, "--ignore-not-found").Execute()
		msg, err = waitForDeployment(oc, podNs, deployName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())

		g.By("run must-gather")
		defer os.RemoveAll(mustgatherDir)
		logFile, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", subscription.namespace, "must-gather", "--image="+mustGatherImage, "--dest-dir="+mustgatherDir).OutputToFile(mustgatherLog)
		e2e.Logf("created mustgather from image %v in %v logged to %v,%v %v", mustGatherImage, mustgatherDir, mustgatherLog, logFile, err)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("look in " + mustgatherDir)
		files, err := ioutil.ReadDir(mustgatherDir)
		e2e.Logf("%v contents %v\n", mustgatherDir, err)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, file := range files {
			if strings.Contains(file.Name(), "sha256") {
				mustgatherTopdir = mustgatherDir + "/" + file.Name()
				break
			}
		}

		g.By("Walk through " + mustgatherTopdir)
		err = filepath.Walk(mustgatherTopdir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				e2e.Logf("Error on %v: %v", path, err)
				return err
			}

			// qemu will create a directory but might not create files
			if info.IsDir() {
				if strings.Contains(path, "worker") && strings.Contains(path, "/run/vc/crio/fifo") && !strings.Contains(path, "/run/vc/crio/fifo/io") {
					mustgatherChecks.qemuLogs++
				}
				if strings.Contains(path, "audit") {
					e2e.Logf("AUDIT directory %v", path)
				}
			} else {
				mustgatherFiles = append(mustgatherFiles, path)
				if strings.Contains(path, "audit") {
					if strings.Contains(path, "audit.log") {
						e2e.Logf("AUDIT file %v", path)
					}
					if strings.Contains(path, "audit_logs_listing") {
						output, _ := ioutil.ReadFile(path)
						e2e.Logf("AUDIT logs listing\n%v", string(output))
						e2e.Logf("nodeWorkerList %v", nodeWorkerList)
					}
				}

				if strings.Contains(path, "audit.log") {
					mustgatherChecks.audits++
				}

				if strings.Contains(path, "/nodes/") {
					if strings.Contains(path, "_logs_crio") {
						mustgatherChecks.crio++
					}
					// in SNO, no worker, just master
					if (strings.Contains(path, "worker") || (singleNode == true && strings.Contains(path, "master"))) && strings.Contains(path, "/version") {
						mustgatherChecks.qemuVersion++
						// read file to extract kata-containers-* and qemu-kvm-core-* ?
					}
				}

				if strings.Contains(path, "/sandboxed-containers") {
					if strings.Contains(path, "/clusterserviceversion_description") {
						mustgatherChecks.describeCsv++
					}
					if strings.Contains(path, "/kataconfig_description") {
						mustgatherChecks.describeKc++
					}
					if strings.Contains(path, "/services_description") {
						mustgatherChecks.describeServices++
					}
					if strings.Contains(path, "/subscription_description") {
						mustgatherChecks.describeSub++
					}
					if strings.Contains(path, "/validatingwebhookconfigurations_description") {
						mustgatherChecks.describeVwebhook++
					}
				}
			}
			return nil
		})
		e2e.Logf("%v files in must-gather dir %v", len(mustgatherFiles), mustgatherTopdir)
		e2e.Logf("counts %v, expected %v", mustgatherChecks, mustgatherExpected)

		g.By("Compare walkthrough counts to expected from " + mustgatherTopdir)

		e2e.Logf("mustgatherChecks.audits : %v", mustgatherChecks.audits)
		if mustgatherChecks.audits != mustgatherExpected.audits {
			e2e.Logf("Audit logs (%v) did not exist on all worker nodes (%v)", mustgatherChecks.audits, mustgatherExpected.audits)
			fails++
		}
		e2e.Logf("mustgatherChecks.crio : %v", mustgatherChecks.crio)
		if mustgatherChecks.crio != (mustgatherExpected.crio) {
			e2e.Logf("crio logs (%v) did exist on all nodes (%v)", mustgatherChecks.crio, (mustgatherExpected.crio))
			fails++
		}

		// A deployment will place VMs based on loads
		// to ensure a VM is on each node another method is needed
		e2e.Logf("mustgatherChecks.qemuLogs : %v", mustgatherChecks.qemuLogs)
		if mustgatherChecks.qemuLogs != mustgatherExpected.qemuLogs {
			e2e.Logf("qemu log directory (%v) does not exist on all worker nodes (%v), is ok", mustgatherChecks.qemuLogs, mustgatherExpected.qemuLogs)
			// VMs should be 1 on each worker node but k8s might put 2 on a node & 0 on another per node load
			if !singleNode && mustgatherChecks.qemuLogs < 1 { // because deployment is used
				fails++
			}
		}

		e2e.Logf("mustgatherChecks.qemuVersion : %v", mustgatherChecks.qemuVersion)
		if mustgatherChecks.qemuVersion != mustgatherExpected.qemuVersion {
			e2e.Logf("rpm version log (%v) did not exist on worker nodes (%v)", mustgatherChecks.qemuVersion, mustgatherExpected.qemuVersion)
			fails++
		}

		e2e.Logf("mustgatherChecks.describeCsv : %v", mustgatherChecks.describeCsv)
		if mustgatherChecks.describeCsv != mustgatherExpected.describeCsv {
			e2e.Logf("describeCsv (%v) did not exist", mustgatherChecks.describeCsv)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeKc : %v", mustgatherChecks.describeKc)
		if mustgatherChecks.describeKc != mustgatherExpected.describeKc {
			e2e.Logf("describeKc (%v) did not exist", mustgatherChecks.describeKc)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeServices : %v", mustgatherChecks.describeServices)
		if mustgatherChecks.describeServices != mustgatherExpected.describeServices {
			e2e.Logf("describeServices (%v) did not exist", mustgatherChecks.describeServices)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeSub : %v", mustgatherChecks.describeSub)
		if mustgatherChecks.describeSub != mustgatherExpected.describeSub {
			e2e.Logf("describeSub (%v) did not exist", mustgatherChecks.describeSub)
			fails++
		}
		e2e.Logf("mustgatherChecks.describeVwebhook : %v", mustgatherChecks.describeVwebhook)
		if mustgatherChecks.describeVwebhook != mustgatherExpected.describeVwebhook {
			e2e.Logf("describeVwebhook (%v) did not exist", mustgatherChecks.describeVwebhook)
			fails++
		}

		if fails != 0 {
			e2e.Logf("%v logs did not match expectd results", fails)
		}
		o.Expect(fails).To(o.Equal(0))

		g.By("Tear down pod")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("deploy", "-n", podNs, deployName).Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("containerruntimeconfig", crioRuntimeConfigName, "-n", subscription.namespace).Execute()
		os.RemoveAll(mustgatherDir)

		g.By("SUCCESS")
	})

	// author: tbuskey@redhat.com
	g.It("Longduration-Author:tbuskey-High-53583-upgrade osc operator [Disruptive][Serial]", func() {
		var (
			testrunUpgrade testrunConfigmap
			testrun        testrunConfigmap
			cmNs           = "default"
			cmName         = "osc-config-upgrade"
			subUpgrade     = subscription
			label          string
			msg            string
			err            error
			podsChanged    = false
		)

		g.By("Checking for configmap " + cmName)
		testrunUpgrade, msg, err = getTestRunConfigmap(oc, testrunDefault, cmNs, cmName)

		g.By("Checking for OSCU environment vars") // env options override default and CM values
		testrun, msg = getTestRunEnvVars("OSCU", testrunDefault)
		if testrun.exists {
			testrunUpgrade = testrun
			e2e.Logf("environment OSCU found. subscription: %v", subscription)
		}

		if testrunUpgrade.exists {
			msg = fmt.Sprintf("Upgrade with testrun will be performed with %v", testrunUpgrade)
			g.By(msg)

			if testrunUpgrade.icspNeeded {
				msg = fmt.Sprintf("Installing ImageContentSourcePolicy to allow %v and %v to work", testrunUpgrade.katamonitorImage, testrunUpgrade.mustgatherImage)
				g.By(msg)
				// apply icsp.  Do not delete or pods can get ImagePullBackoff
				msg, err = imageContentSourcePolicy(oc, icspFile, icspName)
				if err != nil || msg == "" {
					logErrorAndFail(oc, "Error: applying ICSP", msg, err)
				}
			}

			if testrunUpgrade.catalogSourceName != subUpgrade.catalogSourceName {
				// catalog should already exist, but verify to ensure it is ready
				g.By("Check for existence of catalog " + testrunUpgrade.catalogSourceName)
				errCheck := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
					msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catsrc", testrunUpgrade.catalogSourceName, "-n", subUpgrade.catalogSourceNamespace, "-o=jsonpath={.status.connectionState.lastObservedState}").Output()
					if msg == "READY" {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("catalog %v is not found.  Will not upgrade: %v %v", testrunUpgrade.catalogSourceName, msg, err))

				g.By("Check catalog for " + subUpgrade.subName)
				label = fmt.Sprintf("catalog=%v", testrunUpgrade.catalogSourceName)
				errCheck = wait.Poll(10*time.Second, 240*time.Second, func() (bool, error) {
					msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-l", label, "-n", subUpgrade.catalogSourceNamespace).Output()
					if strings.Contains(msg, subUpgrade.subName) {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v is not in the %v catalog. Cannot change subscription: %v %v", subUpgrade.subName, testrunUpgrade.catalogSourceName, msg, err))

				g.By("Changing catalogsource in subscription")
				msg, err = changeSubscriptionCatalog(oc, subUpgrade, testrunUpgrade)
				if err != nil || msg == "" {
					logErrorAndFail(oc, fmt.Sprintf("Error: patching the subscription catalog %v", subUpgrade), msg, err)
				}

				// wait for subscription to finish
				msg, err = subscriptionIsFinished(oc, subUpgrade)
				if err != nil || msg == "" {
					logErrorAndFail(oc, fmt.Sprintf("Error: subscription wait failed %v", subUpgrade), msg, err)
				}
			}

			if testrunUpgrade.channel != subUpgrade.channel {
				g.By("Changing the subscription channel")
				msg, err = changeSubscriptionChannel(oc, subUpgrade, testrunUpgrade)
				if err != nil || msg == "" {
					logErrorAndFail(oc, fmt.Sprintf("Error: patching the subscription channel %v", subUpgrade), msg, err)
				}

				// all pods restart & subscription gets recreated
				msg, err = subscriptionIsFinished(oc, subUpgrade)
				if err != nil || msg == "" {
					logErrorAndFail(oc, fmt.Sprintf("Error: subscription wait failed for %v", subUpgrade), msg, err)
				}
				// check that controller manager pod is running?
			}

			if testrunUpgrade.katamonitorImage != kcMonitorImageName {
				g.By("Changing the monitor image & pods")
				msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", subUpgrade.namespace, "-l", "name=openshift-sandboxed-containers-monitor", "-o=jsonpath={.items..metadata.name}").Output()
				if err != nil || msg == "" {
					logErrorAndFail(oc, "Error: cannot get the pod info before patching kataconfig monitor images", msg, err)
				}
				oldpods := strings.Fields(msg)

				msg, err = changeKataMonitorImage(oc, subUpgrade, testrunUpgrade, commonKataConfigName)
				if err != nil || msg == "" {
					logErrorAndFail(oc, "Error: cannot patch kataconfig with monitor image", msg, err)
				}

				g.By("Wait & check for kata monitor image change")
				// starts changing 40s after
				errCheck := wait.Poll(30*time.Second, 120*time.Second, func() (bool, error) {
					msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", subUpgrade.namespace, "-l", "name=openshift-sandboxed-containers-monitor", "-o=jsonpath={.items..metadata.name}").Output()
					for _, pod := range oldpods {
						if strings.Contains(msg, pod) {
							podsChanged = false
							break // no use checking the rest
						} else {
							podsChanged = true
						}
					}
					if podsChanged {
						return true, nil
					}
					return false, nil
				})
				if !podsChanged {
					e2e.Logf("monitor pods did not upgrade from %v to %v %v", oldpods, msg, err)
					o.Expect(podsChanged).To(o.BeTrue())
				}
				exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("monitor pods did not change %v", msg))

			}

		} else {
			e2e.Logf("Upgrade will not be done: %v\n%v %v", testrunUpgrade, msg, err)
		}

		g.By("SUCCESS")
	})
})
