package securityandcompliance

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance an end user handle FIO within a namespace", func() {
	defer g.GinkgoRecover()

	var (
		oc                  = exutil.NewCLI("fio-"+getRandomString(), exutil.KubeConfigPath())
		dr                  = make(describerResrouce)
		buildPruningBaseDir string
		ogSingleTemplate    string
		subTemplate         string
		fioTemplate         string
		podModifyTemplate   string
		configFile          string
		configErrFile       string
		configFile1         string
		md5configFile       string
		og                  operatorGroupDescription
		sub                 subscriptionDescription
		fi1                 fileintegrity
		podModifyD          podModify
	)

	g.BeforeEach(func() {
		g.By("Skip test when missingcatalogsource, ARM64, or SkipHetegenous !!!")
		SkipMissingCatalogsource(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)

		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogSingleTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		fioTemplate = filepath.Join(buildPruningBaseDir, "fileintegrity.yaml")
		podModifyTemplate = filepath.Join(buildPruningBaseDir, "pod_modify.yaml")
		configFile = filepath.Join(buildPruningBaseDir, "aide.conf.rhel8")
		configErrFile = filepath.Join(buildPruningBaseDir, "aide.conf.rhel8.err")
		configFile1 = filepath.Join(buildPruningBaseDir, "aide.conf.rhel8.1")
		md5configFile = filepath.Join(buildPruningBaseDir, "md5aide.conf.rhel8")

		og = operatorGroupDescription{
			name:      "openshift-file-integrity-qbcd",
			namespace: "",
			template:  ogSingleTemplate,
		}
		sub = subscriptionDescription{
			subName:                "file-integrity-operator",
			namespace:              "",
			channel:                "stable",
			ipApproval:             "Automatic",
			operatorPackage:        "file-integrity-operator",
			catalogSourceName:      "qe-app-registry",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			template:               subTemplate,
			singleNamespace:        true,
		}
		fi1 = fileintegrity{
			name:              "example-fileintegrity",
			namespace:         "",
			configname:        "",
			configkey:         "",
			graceperiod:       15,
			debug:             false,
			nodeselectorkey:   "node.openshift.io/os_id",
			nodeselectorvalue: "rhcos",
			template:          fioTemplate,
		}
		podModifyD = podModify{
			name:      "",
			namespace: "",
			nodeName:  "",
			args:      "",
			template:  podModifyTemplate,
		}

		itName := g.CurrentSpecReport().FullText()
		dr.addIr(itName)
	})

	g.AfterEach(func() {
		itName := g.CurrentSpecReport().FullText()
		dr.getIr(itName).cleanup()
		dr.rmIr(itName)
	})

	// It will cover test case: OCP-34388 & OCP-27760 , author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-ARO-Author:xiyuan-Critical-34388-High-27760-check file-integrity-operator could report failure and persist the failure logs on to a ConfigMap [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		sub.checkPodFioStatus(oc, "running")
		g.By("Create fileintegrity")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace,
			"GRACEPERIOD="+strconv.Itoa(fi1.graceperiod), "DEBUG="+strconv.FormatBool(fi1.debug), "NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
		o.Expect(err).NotTo(o.HaveOccurred())
		fi1.checkFileintegrityStatus(oc, "running")

		g.By("trigger fileintegrity failure on node")
		var pod1, pod2 podModify = podModifyD, podModifyD
		pod1.namespace = oc.Namespace()
		nodeName := getOneRhcosWorkerNodeName(oc)
		pod1.name = "pod-modify"
		pod1.nodeName = nodeName
		var filePath = "/hostroot/root/test/test" + getRandomString()
		pod1.args = "mkdir -p " + filePath
		defer func() {
			cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod1.name})
			pod2.name = "pod-recover"
			pod2.namespace = oc.Namespace()
			pod2.nodeName = nodeName
			pod2.args = "rm -rf " + filePath
			pod2.doActionsOnNode(oc, "Succeeded", dr)
			cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod2.name})
		}()
		pod1.doActionsOnNode(oc, "Succeeded", dr)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName := fi1.getConfigmapFromFileintegritynodestatus(oc, nodeName)
		fi1.getDataFromConfigmap(oc, cmName, filePath)
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-31979-the enabling debug flag of the logcollector should work [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity with debug=false")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=false")
		var podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordNotExistInLog(oc, podName, "debug:")

		g.By("Configure fileintegrity with debug=true")
		fi1.debug = true
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=true")
		podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordExistInLog(oc, podName, "debug:")

	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-31933-the disabling debug flag of the logcollector should work [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = true

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity with debug=true")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=true")
		var podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordExistInLog(oc, podName, "debug:")

		g.By("Configure fileintegrity with debug=false")
		fi1.debug = false
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=false")
		podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordNotExistInLog(oc, podName, "debug:")

	})

	//author: xiyuan@redhat.com
	g.It("StagerunBoth-NonHyperShiftHOST-ARO-Author:xiyuan-Medium-31873-check the gracePeriod is configurable [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity without gracePeriod")
		fi1.createFIOWithoutKeyword(oc, itName, dr, "gracePeriod")
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=900")

		g.By("create fileintegrity with configmap and gracePeriod")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.graceperiod = 0
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=10")

		fi1.graceperiod = 11
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=11")

		fi1.graceperiod = 120
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=120")

		fi1.graceperiod = -10
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=10")
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-28524-adding invalid configuration should report failure [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		nodeName := getOneRhcosWorkerNodeName(oc)
		fi1.reinitFileintegrity(oc, "annotated")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")

		g.By("Check fileintegritynodestatus becomes Errored")
		fi1.configname = "errfile"
		fi1.configkey = "aideerrconf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configErrFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Error", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "AIDE error: 17 Invalid configureline error", ok, []string{"events", "-n", sub.namespace, "--field-selector",
			"reason=NodeIntegrityStatus"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "AIDE error: 17 Invalid configureline error", ok, []string{"fileintegritynodestatus", "-n", sub.namespace,
			"-o=jsonpath={.items[*].results}"}).check(oc)
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-33177-only one long-running daemonset should be created by FIO [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity without aide config")
		fi1.createFIOWithoutKeyword(oc, itName, dr, "gracePeriod")
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkOnlyOneDaemonset(oc)

		g.By("Create fileintegrity with aide config")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkOnlyOneDaemonset(oc)
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:xiyuan-Medium-33853-check whether aide will not reinit when a fileintegrity recreated after deleted [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity without aide config")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("trigger fileintegrity failure on node")
		var pod1, pod2 podModify = podModifyD, podModifyD
		pod1.namespace = oc.Namespace()
		nodeName := getOneRhcosWorkerNodeName(oc)
		pod1.name = "pod-modify"
		pod1.nodeName = nodeName
		var filePath = "/hostroot/root/test/test" + getRandomString()
		pod1.args = "mkdir -p " + filePath
		defer func() {
			cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod1.name})
			pod2.name = "pod-recover"
			pod2.namespace = oc.Namespace()
			pod2.nodeName = nodeName
			pod2.args = "rm -rf " + filePath
			pod2.doActionsOnNode(oc, "Succeeded", dr)
			cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod2.name})
		}()
		pod1.doActionsOnNode(oc, "Succeeded", dr)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName := fi1.getConfigmapFromFileintegritynodestatus(oc, nodeName)
		fi1.getDataFromConfigmap(oc, cmName, filePath)

		g.By("delete and recreate the fileintegrity")
		fi1.removeFileintegrity(oc, "deleted")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		fi1.getDataFromConfigmap(oc, cmName, filePath)

		g.By("trigger reinit")
		fi1.reinitFileintegrity(oc, "annotated")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		aidpodNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l app=aide-example-fileintegrity", "-n", fi1.namespace,
			"-o=jsonpath={.items[*].metadata.name}").Output()
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("aidepodNames is %s ", aidpodNames))
		aidpodName := strings.Fields(aidpodNames)
		for _, v := range aidpodName {
			newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", v, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		}
		fionodeNames, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", fi1.namespace, "-l node.openshift.io/os_id=rhcos",
			"-o=jsonpath={.items[*].metadata.name}").Output()
		exutil.AssertWaitPollNoErr(err2, fmt.Sprintf("fionodeNames is %s ", fionodeNames))
		fionodeName := strings.Fields(fionodeNames)
		for _, node := range fionodeName {
			fi1.checkFileintegritynodestatus(oc, node, "Succeeded")
			fi1.checkDBBackupResult(oc, node)
		}
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-33332-The fileintegritynodestatuses should show status summary for FIO [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity with aide config")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")

		g.By("Check Data Details in CM and Fileintegritynodestatus Equal or not")
		nodeName := getOneRhcosWorkerNodeName(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName := fi1.getConfigmapFromFileintegritynodestatus(oc, nodeName)
		intFileAddedCM, intFileChangedCM, intFileRemovedCM := fi1.getDetailedDataFromConfigmap(oc, cmName)
		intFileAddedFins, intFileChangedFins, intFileRemovedFins := fi1.getDetailedDataFromFileintegritynodestatus(oc, nodeName)
		checkDataDetailsEqual(intFileAddedCM, intFileChangedCM, intFileRemovedCM, intFileAddedFins, intFileChangedFins, intFileRemovedFins)
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-High-33226-enable configuring tolerations in FileIntegrities [Disruptive]", func() {
		skipForSingleNodeCluster(oc)

		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false
		fi1.nodeselectorkey = "node-role.kubernetes.io/worker"
		fi1.nodeselectorvalue = ""

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create taint")
		nodeName := getOneRhcosWorkerNodeName(oc)
		defer func() {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.taints}").Output()
			if strings.Contains(output, "value1") {
				taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")
			}
		}()
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule")

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerLessThanNodeNumber(oc, "kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		g.By("patch the tolerations and compare again")
		patch := fmt.Sprintf("{\"spec\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"key1\",\"operator\":\"Equal\",\"value\":\"value1\"}]}}")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "merge", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")
		defer taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule-")
		taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule")

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.removeFileintegrity(oc, "deleted")
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerLessThanNodeNumber(oc, "kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		g.By("patch the tolerations and compare again")
		patch = fmt.Sprintf("{\"spec\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"key1\",\"operator\":\"Exists\"}]}}")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "merge", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "kubernetes.io/os=linux,node-role.kubernetes.io/worker=")
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-33254-enable configuring tolerations in FileIntegrities when there is more than one taint on one node [Disruptive]", func() {
		skipForSingleNodeCluster(oc)

		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false
		fi1.nodeselectorkey = "node-role.kubernetes.io/worker"
		fi1.nodeselectorvalue = ""

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create taint")
		nodeName := getOneRhcosWorkerNodeName(oc)
		defer taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-", "key2=value2:NoExecute-")
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule", "key2=value2:NoExecute")

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerLessThanNodeNumber(oc, "kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		g.By("patch the tolerations and compare again")
		patch := fmt.Sprintf("{\"spec\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"key1\",\"operator\":\"Equal\",\"value\":\"value1\"},{\"effect\":\"NoExecute\",\"key\":\"key2\",\"operator\":\"Equal\",\"value\":\"value2\"}]}}")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "merge", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "kubernetes.io/os=linux,node-role.kubernetes.io/worker=")
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-27755-check nodeSelector works for operator file-integrity-operator [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.nodeselectorkey = "node.openshift.io/os_id"
		fi1.nodeselectorvalue = "rhcos"
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "node.openshift.io/os_id=rhcos")

		g.By("Patch fileintegrity with a new nodeselector and compare Aide-scan pod number and Node number")
		patch := fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/nodeSelector/node.openshift.io~1os_id\"},{\"op\":\"add\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1master\",\"value\":\"\"}]")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "json", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/master=")

		g.By("Patch fileintegrity with another nodeselector and compare Aide-scan pod number and Node number")
		patch = fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1master\"},{\"op\":\"add\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1worker\",\"value\":\"\"}]")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "json", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/worker=")

		g.By("Remove nodeselector and compare Aide-scan pod number and Node number")
		patch = fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1worker\"},{\"op\":\"add\",\"path\":\"/spec/nodeSelector/node.openshift.io~1os_id\",\"value\":\"rhcos\"}]")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "json", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "node.openshift.io/os_id=rhcos")
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:xiyuan-Medium-31862-check whether aide config change from non-empty to empty will trigger a re-initialization of the aide database or not [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		nodeName := getOneRhcosWorkerNodeName(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")

		g.By("trigger reinit by changing aide config to empty")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")
	})

	//author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-ARO-Author:xiyuan-High-42026-aide config change will trigger a re-initialization of the aide database [Serial]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity without aide config")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.reinitFileintegrity(oc, "annotated")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		nodeName := getOneRhcosWorkerNodeName(oc)
		fileintegrityNodeStatusName := fi1.name + "-" + nodeName
		assertKeywordsExists(oc, 300, fileintegrityNodeStatusName, "fileintegritynodestatuses", "-o=jsonpath={.items[*].metadata.name}", "-n", fi1.namespace)

		g.By("Check DB backup results")
		dbReinit := true
		dbInitialBackupList, isNewFIO := fi1.getDBBackupLists(oc, nodeName, dbReinit)

		g.By("trigger reinit by applying aide config")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile, "created")
		newCheck("expect", asAdmin, withoutNamespace, contain, fi1.configname, ok, []string{"configmap", "-n", fi1.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		checkDBFilesUpdated(oc, fi1, dbInitialBackupList, nodeName, dbReinit, isNewFIO)
		dbBackupListAfterInit1, isNewFIO := fi1.getDBBackupLists(oc, nodeName, dbReinit)
		assertKeywordsExists(oc, 300, fileintegrityNodeStatusName, "fileintegritynodestatuses", "-o=jsonpath={.items[*].metadata.name}", "-n", fi1.namespace)

		g.By("trigger fileintegrity failure on node")
		var filePath = "/root/test" + getRandomString()
		defer exutil.DebugNodeWithChroot(oc, nodeName, "rm", "-rf", filePath)
		_, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "mkdir", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "Failed", ok, []string{"fileintegritynodestatuses", fileintegrityNodeStatusName, "-n", fi1.namespace, "-o=jsonpath={.lastResult.condition}"}).check(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName := fi1.getConfigmapFromFileintegritynodestatus(oc, nodeName)
		fi1.getDataFromConfigmap(oc, cmName, filePath)

		g.By("trigger reinit by applying aide config")
		fi1.configname = "myconf1"
		fi1.configkey = "aide-conf1"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, configFile1, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		checkDBFilesUpdated(oc, fi1, dbBackupListAfterInit1, nodeName, dbReinit, isNewFIO)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Failed", ok, []string{"fileintegritynodestatuses", fileintegrityNodeStatusName, "-n", fi1.namespace, "-o=jsonpath={.lastResult.condition}"}).check(oc)
		fi1.expectedStringNotExistInConfigmap(oc, cmName, filePath)
	})

	//author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-ARO-Author:pdhamdhe-NonPreRelease-CPaasrunOnly-High-29782-check md5 algorithm could not work for a fips enabled cluster while working well for a fips disabled cluster [Serial][Slow]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")

		g.By("Create fileintegrity with md5 aide config")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.createConfigmapFromFile(oc, itName, dr, fi1.configname, fi1.configkey, md5configFile, "created")
		fi1.createFIOWithConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		nodeName := getOneRhcosWorkerNodeName(oc)

		fipsOut := checkFipsStatus(oc, fi1.namespace)
		if strings.Contains(fipsOut, "FIPS mode is enabled.") {
			fi1.checkFileintegritynodestatus(oc, nodeName, "Errored")
			var podName = fi1.getOneFioPodName(oc)
			fi1.checkKeywordExistInLog(oc, podName, "Use of FIPS disallowed algorithm under FIPS mode exit status 255")
		} else {
			fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")
			var podName = fi1.getOneFioPodName(oc)
			fi1.checkKeywordExistInLog(oc, podName, "running aide check")

			g.By("Check the md5 algorithm for fips disabled cluster")
			var pod1, pod2 podModify = podModifyD, podModifyD
			pod1.namespace = oc.Namespace()
			pod1.name = "pod-modify"
			pod1.nodeName = nodeName
			var filePath = "/hostroot/etc/kubernetes/cloud.conf"
			pod1.args = "echo \"testAAAAAAAAA\" >>" + filePath
			defer func() {
				cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod1.name})
				pod2.name = "pod-recover"
				pod2.namespace = oc.Namespace()
				pod2.nodeName = nodeName
				pod2.args = "sed -i '/testAAAAAAAAA/d' " + filePath
				pod2.doActionsOnNode(oc, "Succeeded", dr)
				cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod2.name})
			}()
			pod1.doActionsOnNode(oc, "Succeeded", dr)
			fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
			cmName := fi1.getConfigmapFromFileintegritynodestatus(oc, nodeName)
			fi1.getDataFromConfigmap(oc, cmName, filePath)
		}
	})

	//author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-ARO-Author:pdhamdhe-NonPreRelease-CPaasrunOnly-High-43136-Check FIO metrics and alerting [Serial][Slow]", func() {
		var itName = g.CurrentSpecReport().FullText()
		oc.SetupProject()
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		g.By("Label the namespace  !!!\n")
		labelNameSpace(oc, sub.namespace, "openshift.io/cluster-monitoring=true")
		fi1.namespace = oc.Namespace()
		fi1.debug = false

		g.By("Create og")
		og.create(oc, itName, dr)
		og.checkOperatorgroup(oc, og.name)
		g.By("Create subscription")
		sub.create(oc, itName, dr)
		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.checkPodFioStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, contain, "openshift.io/cluster-monitoring", ok, []string{"namespace", sub.namespace, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "metrics", ok, []string{"service", "-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create fileintegrity object with default aide config..\n")
		fi1.createFIOWithoutConfig(oc, itName, dr)
		fi1.checkFileintegrityStatus(oc, "running")
		// trigger reinit before checking fileintegritynodestatus, otherwise it could be Failed status
		fi1.reinitFileintegrity(oc, "annotated")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		nodeName := getOneRhcosWorkerNodeName(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")

		g.By("trigger fileintegrity failure on node")
		var pod1, pod2 podModify = podModifyD, podModifyD
		pod1.namespace = oc.Namespace()
		pod1.name = "pod-modify"
		pod1.nodeName = nodeName
		var filePath = "/hostroot/root/test/test" + getRandomString()
		pod1.args = "mkdir -p " + filePath
		defer func() {
			cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod1.name})
			pod2.name = "pod-recover"
			pod2.namespace = oc.Namespace()
			pod2.nodeName = nodeName
			pod2.args = "rm -rf " + filePath
			pod2.doActionsOnNode(oc, "Succeeded", dr)
			cleanupObjects(oc, objectTableRef{"pod", oc.Namespace(), pod2.name})
		}()
		pod1.doActionsOnNode(oc, "Succeeded", dr)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")

		metricsErr := []string{"file_integrity_operator_daemonset_update_total{operation=\"update\"} 1", "file_integrity_operator_node_failed{node=\"" + nodeName + "\"} 1",
			"file_integrity_operator_node_status_total{condition=\"Failed\",node=\"" + nodeName + "\"} 1"}

		checkMetric(oc, metricsErr, sub.namespace, "fio")
		newCheck("expect", asAdmin, withoutNamespace, contain, "file-integrity", ok, []string{"PrometheusRule", "-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NodeHasIntegrityFailure", ok, []string{"PrometheusRule", "file-integrity", "-n", sub.namespace, "-ojsonpath={.spec.groups[0].rules[0].alert}"}).check(oc)
	})
})
