package securityandcompliance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance File_Integrity_Operator an end user handle FIO within a namespace", func() {
	defer g.GinkgoRecover()

	var (
		oc                  = exutil.NewCLI("fio-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir string
		ogSingleTemplate    string
		subTemplate         string
		fioTemplate         string
		configFile          string
		configErrFile       string
		configFile1         string
		md5configFile       string
		og                  operatorGroupDescription
		sub                 subscriptionDescription
		fi1                 fileintegrity
	)

	g.BeforeEach(func() {
		g.By("Skip the test if the cluster has no OLM component")
		exutil.SkipNoOLMCore(oc)

		g.By("Skip test when missingcatalogsource, ARM64, or SkipHetegenous !!!")
		SkipClustersWithRhelNodes(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)

		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogSingleTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		fioTemplate = filepath.Join(buildPruningBaseDir, "fileintegrity.yaml")
		configFile = filepath.Join(buildPruningBaseDir, "aide.conf.rhel8")
		configErrFile = filepath.Join(buildPruningBaseDir, "aide.conf.rhel8.err")
		configFile1 = filepath.Join(buildPruningBaseDir, "aide.conf.rhel8.1")
		md5configFile = filepath.Join(buildPruningBaseDir, "md5aide.conf.rhel8")

		og = operatorGroupDescription{
			name:      "openshift-file-integrity-qbcd",
			namespace: "openshift-file-integrity",
			template:  ogSingleTemplate,
		}
		sub = subscriptionDescription{
			subName:                "file-integrity-operator",
			namespace:              "openshift-file-integrity",
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
			namespace:         "openshift-file-integrity",
			configname:        "",
			configkey:         "",
			graceperiod:       15,
			debug:             false,
			nodeselectorkey:   "node.openshift.io/os_id",
			nodeselectorvalue: "rhcos",
			template:          fioTemplate,
		}

		sub.skipMissingCatalogsources(oc)
		g.By("Install File Integrity Operator and check it is sucessfully installed !!! ")
		createFileIntegrityOperator(oc, sub, og)
	})

	// It will cover test case: OCP-34388 & OCP-27760 , author: xiyuan@redhat.com
	g.It("Author:xiyuan-LEVEL0-NonHyperShiftHOST-ConnectedOnly-ROSA-ARO-OSD_CCS-WRS-Critical-34388-High-27760-V-EST.01-check file-integrity-operator could report failure and persist the failure logs on to a ConfigMap [Serial]", func() {
		g.By("Create fileintegrity")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.debug = true
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace,
			"GRACEPERIOD="+strconv.Itoa(fi1.graceperiod), "DEBUG="+strconv.FormatBool(fi1.debug), "NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
		o.Expect(err).NotTo(o.HaveOccurred())
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		nodeName := fi1.getNodeName(oc)
		fileintegrityNodeStatusName := fi1.name + "-" + nodeName
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fileintegrityNodeStatusName,
			"-o=jsonpath={.lastResult.condition}").Output()
		if output == "Failed" {
			fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
			fi1.checkFileintegrityStatus(oc, "running")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
			fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")
		}

		g.By("trigger fileintegrity failure on node")
		var filePath = "/root/test" + getRandomString()
		defer exutil.DebugNodeWithChroot(oc, nodeName, "rm", "-rf", filePath)
		debugNodeStdout, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "mkdir", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatus", fileintegrityNodeStatusName, "-n", sub.namespace,
			`-o=jsonpath={.results[?(@.condition=="Failed")].resultConfigMapName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		fi1.getDataFromConfigmap(oc, cmName, filePath)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-ROSA-ARO-OSD_CCS-Longduration-NonPreRelease-CPaasrunOnly-WRS-Critical-27599-V-EST.01-check operator file-integrity-operator could run file integrity checks on the cluster nodes and shows relevant fileintegritynodestatuses [Slow][Serial]", func() {
		g.By("trigger fileintegrity failure on node")
		var filePath = "/root/test27599"
		nodeName := getOneWorkerNodeName(oc)
		createCmd := fmt.Sprintf(`mkdir %s; touch %s/test`, filePath, filePath)
		delCmd := fmt.Sprintf(`if [ -d "%s" ]; then rm -rf %s; fi`, filePath, filePath)
		defer exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", delCmd)
		debugNodeStdout, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", createCmd)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)

		g.By("Create fileintegrity")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace,
			"GRACEPERIOD="+strconv.Itoa(fi1.graceperiod), "DEBUG="+strconv.FormatBool(fi1.debug), "NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
		o.Expect(err).NotTo(o.HaveOccurred())
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		fileintegrityNodeStatusName := fi1.name + "-" + nodeName
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fileintegrityNodeStatusName,
			"-o=jsonpath={.lastResult.condition}").Output()
		if output == "Failed" {
			fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
			fi1.checkFileintegrityStatus(oc, "running")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
			fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")
		}

		g.By("trigger fileintegrity failure on node")
		cmd := fmt.Sprintf(`semanage fcontext -a -t httpd_sys_content_t "%s(/.*)?"; restorecon -Rv %s;
			useradd usr1; useradd usr2; useradd usr3; groupadd test1; gpasswd -a usr1 test1; gpasswd -a usr2 test1;
			chown root:test1 %s; chmod 770 %s; setfacl -m u:usr3:rx %s`, filePath, filePath, filePath, filePath, filePath)
		RecoverCmd := fmt.Sprintf(`rm -rf %s; userdel usr1; userdel usr2; userdel usr3; groupdel test1`, filePath)
		defer exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", RecoverCmd)
		debugNodeStdout, debugNodeErr = exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", cmd)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatus", fileintegrityNodeStatusName, "-n", sub.namespace,
			`-o=jsonpath={.results[?(@.condition=="Failed")].resultConfigMapName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the configmap contains expected string")
		var res string
		defer os.RemoveAll("/tmp/integritylog")
		err = wait.Poll(5*time.Second, 500*time.Second, func() (bool, error) {
			_, errRes := oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", fi1.namespace, "configmap/"+cmName, "--to=/tmp", "--confirm").Output()
			o.Expect(errRes).NotTo(o.HaveOccurred())
			aideResult, err := os.ReadFile("/tmp/integritylog")
			res = string(aideResult)
			o.Expect(err).NotTo(o.HaveOccurred())
			matchedFile, _ := regexp.MatchString(filePath+"/test", res)
			matchedSelinux, _ := regexp.MatchString(`system_u:object_r:httpd_sys_cont`, res)
			matchedGroup, _ := regexp.MatchString(`/hostroot/etc/group`, res)
			matchedShadow, _ := regexp.MatchString("/hostroot/etc/shadow", res)
			matchedPermission, _ := regexp.MatchString("other::---", res)
			e2e.Logf("The result is: matchedFile - %v, matchedSelinux - %v, matchedGroup - %v, matchedShadow - %v, matchedPermission - %v", matchedFile, matchedSelinux, matchedGroup, matchedShadow, matchedPermission)
			if matchedFile && matchedSelinux && matchedGroup && matchedShadow && matchedPermission {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			// Expose more info when configmap not contains the expected string
			e2e.Logf("The aide report details is: %s", res)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cmName %s does not include expected content", cmName))
		}
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-31979-V-EST.01-the enabling debug flag of the logcollector should work [Serial]", func() {
		g.By("Create fileintegrity with debug=false")
		fi1.debug = false
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=false")
		var podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordNotExistInLog(oc, podName, "debug:")

		g.By("Configure fileintegrity with debug=true")
		fi1.debug = true
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=true")
		podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordExistInLog(oc, podName, "debug:")

	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-31933-V-EST.01-the disabling debug flag of the logcollector should work [Serial]", func() {
		fi1.debug = true

		g.By("Create fileintegrity with debug=true")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=true")
		var podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordExistInLog(oc, podName, "debug:")

		g.By("Configure fileintegrity with debug=false")
		fi1.debug = false
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "debug=false")
		podName = fi1.getOneFioPodName(oc)
		fi1.checkKeywordNotExistInLog(oc, podName, "debug:")

	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-StagerunBoth-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-31873-V-EST.01-check the gracePeriod is configurable [Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity without gracePeriod")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutKeyword(oc, "gracePeriod")
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=900")

		g.By("create fileintegrity with configmap and gracePeriod")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		fi1.graceperiod = 0
		defer cleanupObjects(oc, objectTableRef{"configmap", sub.namespace, fi1.configname})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=10")

		fi1.graceperiod = 11
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=11")

		fi1.graceperiod = 120
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=120")

		fi1.graceperiod = -10
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkArgsInPod(oc, "interval=10")
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-28524-V-EST.01-adding invalid configuration should report failure [Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.assertNodesConditionNotEmpty(oc)

		nodeName := fi1.getNodeName(oc)
		fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")

		g.By("Check fileintegritynodestatus becomes Errored")
		fi1.configname = "errfile"
		fi1.configkey = "aideerrconf"
		defer cleanupObjects(oc, objectTableRef{"configmap", sub.namespace, fi1.configname})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configErrFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.assertNodesConditionNotEmpty(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Error", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "AIDE error: 17 Invalid configureline error", ok, []string{"events", "-n", sub.namespace, "--field-selector",
			"reason=NodeIntegrityStatus"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "AIDE error: 17 Invalid configureline error", ok, []string{"fileintegritynodestatus", "-n", sub.namespace,
			"-o=jsonpath={.items[*].results}"}).check(oc)
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-33177-V-EST.01-only one long-running daemonset should be created by FIO [Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity without aide config")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutKeyword(oc, "gracePeriod")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkOnlyOneDaemonset(oc)

		g.By("Create fileintegrity with aide config")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		defer cleanupObjects(oc, objectTableRef{"configmap", sub.namespace, fi1.configname})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkOnlyOneDaemonset(oc)
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-WRS-Medium-33853-V-EST.01-check whether aide will not reinit when a fileintegrity recreated after deleted [Slow][Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity without aide config")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		nodeName := fi1.getNodeName(oc)
		fileintegrityNodeStatusName := fi1.name + "-" + nodeName
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fileintegrityNodeStatusName,
			"-o=jsonpath={.lastResult.condition}").Output()
		if output == "Failed" {
			fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
			fi1.checkFileintegrityStatus(oc, "running")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
			fi1.assertNodesConditionNotEmpty(oc)
			fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")
		}

		g.By("trigger fileintegrity failure on node")
		var filePath = "/root/test" + getRandomString()
		defer exutil.DebugNodeWithChroot(oc, nodeName, "rm", "-rf", filePath)
		debugNodeStdout, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "mkdir", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatus", fi1.name+"-"+nodeName, "-n", sub.namespace,
			`-o=jsonpath={.results[?(@.condition=="Failed")].resultConfigMapName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		fi1.getDataFromConfigmap(oc, cmName, filePath)

		g.By("delete and recreate the fileintegrity")
		fi1.removeFileintegrity(oc, "deleted")
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		fi1.getDataFromConfigmap(oc, cmName, filePath)

		g.By("trigger reinit")
		fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		aidpodNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l app=aide-example-fileintegrity", "-n", fi1.namespace,
			"-o=jsonpath={.items[*].metadata.name}").Output()
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("aidepodNames is %s ", aidpodNames))
		aidpodName := strings.Fields(aidpodNames)
		for _, v := range aidpodName {
			newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", v, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		}
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", fi1.namespace, "-l app=aide-"+fi1.name, "-o=jsonpath={.items[*].spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodes := strings.Fields(output)
		for _, node := range nodes {
			fi1.checkFileintegritynodestatus(oc, node, "Succeeded")
			fi1.checkDBBackupResult(oc, node)
		}
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-DEPRECATED-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-33332-V-EST.01-The fileintegritynodestatuses should show status summary for FIO [Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity with aide config")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		defer cleanupObjects(oc,
			objectTableRef{"configmap", sub.namespace, fi1.configname},
			objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check Data Details in CM and Fileintegritynodestatus Equal or not")
		nodeName := fi1.getNodeName(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
		cmName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatus", fi1.name+"-"+nodeName, "-n", sub.namespace,
			`-o=jsonpath={.results[?(@.condition=="Failed")].resultConfigMapName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		intFileAddedCM, intFileChangedCM, intFileRemovedCM := fi1.getDetailedDataFromConfigmap(oc, cmName)
		intFileAddedFins, intFileChangedFins, intFileRemovedFins := fi1.getDetailedDataFromFileintegritynodestatus(oc, nodeName)
		checkDataDetailsEqual(intFileAddedCM, intFileChangedCM, intFileRemovedCM, intFileAddedFins, intFileChangedFins, intFileRemovedFins)
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-WRS-High-33226-V-EST.01-enable configuring tolerations in FileIntegrities [Disruptive]", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}
		SkipClustersWithRhelNodes(oc)

		fi1.debug = false
		fi1.nodeselectorkey = "node-role.kubernetes.io/worker"
		fi1.nodeselectorvalue = ""

		g.By("Create taint")
		nodeName := getOneWorkerNodeName(oc)
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
		defer cleanupObjects(oc,
			objectTableRef{"configmap", sub.namespace, fi1.configname},
			objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerLessThanNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		g.By("patch the tolerations and compare again")
		patch := fmt.Sprintf("{\"spec\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"key1\",\"operator\":\"Equal\",\"value\":\"value1\"}]}}")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "merge", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")
		defer taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule-")
		taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule")

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.removeFileintegrity(oc, "deleted")
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerLessThanNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		g.By("patch the tolerations and compare again")
		patch = fmt.Sprintf("{\"spec\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"key1\",\"operator\":\"Exists\"}]}}")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "merge", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,kubernetes.io/os=linux,node-role.kubernetes.io/worker=")
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-WRS-Medium-33254-V-EST.01-enable configuring tolerations in FileIntegrities when there is more than one taint on one node [Disruptive]", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}
		SkipClustersWithRhelNodes(oc)

		fi1.debug = false
		fi1.nodeselectorkey = "node-role.kubernetes.io/worker"
		fi1.nodeselectorvalue = ""

		g.By("Create taint")
		nodeName := getOneWorkerNodeName(oc)
		defer taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-", "key2=value2:NoExecute-")
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule", "key2=value2:NoExecute")

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		defer cleanupObjects(oc,
			objectTableRef{"configmap", sub.namespace, fi1.configname},
			objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerLessThanNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,kubernetes.io/os=linux,node-role.kubernetes.io/worker=")

		g.By("patch the tolerations and compare again")
		patch := fmt.Sprintf("{\"spec\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"key1\",\"operator\":\"Equal\",\"value\":\"value1\"},{\"effect\":\"NoExecute\",\"key\":\"key2\",\"operator\":\"Equal\",\"value\":\"value2\"}]}}")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "merge", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,kubernetes.io/os=linux,node-role.kubernetes.io/worker=")
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-27755-V-EST.01-check nodeSelector works for operator file-integrity-operator [Serial]", func() {
		SkipClustersWithRhelNodes(oc)

		fi1.debug = false

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.nodeselectorkey = "node.openshift.io/os_id"
		fi1.nodeselectorvalue = "rhcos"
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,node.openshift.io/os_id=rhcos")

		g.By("Patch fileintegrity with a new nodeselector and compare Aide-scan pod number and Node number")
		patch := fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/nodeSelector/node.openshift.io~1os_id\"},{\"op\":\"add\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1master\",\"value\":\"\"}]")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "json", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/edge!=,node-role.kubernetes.io/master=")

		g.By("Patch fileintegrity with another nodeselector and compare Aide-scan pod number and Node number")
		patch = fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1master\"},{\"op\":\"add\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1worker\",\"value\":\"\"}]")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "json", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,node-role.kubernetes.io/worker=")

		g.By("Remove nodeselector and compare Aide-scan pod number and Node number")
		patch = fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/nodeSelector/node-role.kubernetes.io~1worker\"},{\"op\":\"add\",\"path\":\"/spec/nodeSelector/node.openshift.io~1os_id\",\"value\":\"rhcos\"}]")
		patchResource(oc, asAdmin, withoutNamespace, "fileintegrity", fi1.name, "-n", fi1.namespace, "--type", "json", "-p", patch)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.checkPodNumerEqualNodeNumber(oc, "node-role.kubernetes.io/edge!=,kubernetes.io/os!=windows,node.openshift.io/os_id=rhcos")
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-DEPRECATED-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-WRS-Medium-31862-V-EST.01-check whether aide config change from non-empty to empty will trigger a re-initialization of the aide database or not [Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity with aide config and compare Aide-scan pod number and Node number")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		defer cleanupObjects(oc,
			objectTableRef{"configmap", sub.namespace, fi1.configname},
			objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		nodeName := fi1.getNodeName(oc)
		var filePath = "/root/test" + getRandomString()
		defer exutil.DebugNodeWithChroot(oc, nodeName, "rm", "-rf", filePath)
		_, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "mkdir", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")

		g.By("trigger reinit by changing aide config to empty")
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-ROSA-ARO-OSD_CCS-WRS-High-42026-V-EST.01-aide config change will trigger a re-initialization of the aide database [Slow][Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity without aide config")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)

		g.By("Check DB backup results")
		dbReinit := true
		nodeName := fi1.getNodeName(oc)
		dbInitialBackupList, isNewFIO := fi1.getDBBackupLists(oc, nodeName, dbReinit)

		g.By("trigger reinit by applying aide config")
		fi1.configname = "myconf" + getRandomString()
		fi1.configkey = "aide-conf" + getRandomString()
		fileintegrityNodeStatusName := fi1.name + "-" + nodeName
		defer cleanupObjects(oc, objectTableRef{"configmap", sub.namespace, fi1.configname})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile, "created")
		newCheck("expect", asAdmin, withoutNamespace, contain, fi1.configname, ok, []string{"configmap", "-n", fi1.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		checkDBFilesUpdated(oc, fi1, dbInitialBackupList, nodeName, dbReinit, isNewFIO)
		dbBackupListAfterInit1, isNewFIO := fi1.getDBBackupLists(oc, nodeName, dbReinit)
		fi1.assertNodesConditionNotEmpty(oc)

		g.By("trigger fileintegrity failure on node")
		var filePath = "/root/test" + getRandomString()
		defer exutil.DebugNodeWithChroot(oc, nodeName, "rm", "-rf", filePath)
		debugNodeStdout, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "mkdir", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)
		debugNodeStdout, debugNodeErr = exutil.DebugNodeWithChroot(oc, nodeName, "ls", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of command ls %s is: %s", filePath, debugNodeStdout)
		fi1.assertNodesConditionNotEmpty(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Failed", ok, []string{"fileintegritynodestatuses", fileintegrityNodeStatusName, "-n", fi1.namespace, "-o=jsonpath={.lastResult.condition}"}).check(oc)
		cmName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatus", fi1.name+"-"+nodeName, "-n", sub.namespace,
			`-o=jsonpath={.results[?(@.condition=="Failed")].resultConfigMapName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		fi1.getDataFromConfigmap(oc, cmName, filePath)

		g.By("trigger reinit by applying aide config")
		fi1.configname = "myconf1" + getRandomString()
		fi1.configkey = "aide-conf1" + getRandomString()
		defer cleanupObjects(oc, objectTableRef{"configmap", sub.namespace, fi1.configname})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, configFile1, "created")
		fi1.checkConfigmapCreated(oc)
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		checkDBFilesUpdated(oc, fi1, dbBackupListAfterInit1, nodeName, dbReinit, isNewFIO)
	})

	//author: pdhamdhe@redhat.com
	g.It("Author:pdhamdhe-NonHyperShiftHOST-ConnectedOnly-ROSA-ARO-OSD_CCS-WRS-NonPreRelease-CPaasrunOnly-High-29782-V-EST.01-check md5 algorithm could not work for a fips enabled cluster while working well for a fips disabled cluster [Serial]", func() {
		fi1.debug = false

		g.By("Create fileintegrity with md5 aide config")
		fi1.configname = "myconf"
		fi1.configkey = "aide-conf"
		defer cleanupObjects(oc,
			objectTableRef{"configmap", sub.namespace, fi1.configname},
			objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createConfigmapFromFile(oc, fi1.configname, fi1.configkey, md5configFile, "created")
		fi1.createFIOWithConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		fi1.assertNodesConditionNotEmpty(oc)
		nodeName := fi1.getNodeName(oc)

		fipsOut := checkFipsStatus(oc, fi1.namespace)
		if strings.Contains(fipsOut, "FIPS mode is enabled.") {
			fi1.checkFileintegritynodestatus(oc, nodeName, "Errored")
			var podName = fi1.getOneFioPodName(oc)
			fi1.checkErrorsExistInLog(oc, podName, "Use of FIPS disallowed algorithm under FIPS mode exit status 255")
		} else {
			fileintegrityNodeStatusName := fi1.name + "-" + nodeName
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fi1.namespace, fileintegrityNodeStatusName,
				"-o=jsonpath={.lastResult.condition}").Output()
			if output == "Failed" {
				fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
				fi1.checkFileintegrityStatus(oc, "running")
				newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", fi1.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
				fi1.assertNodesConditionNotEmpty(oc)
				fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")
			}

			g.By("Check the md5 algorithm for fips disabled cluster")
			var filePath = "/etc/kubernetes/cloud.conf"
			createCmd := fmt.Sprintf(`echo testAAAAAAAAA >> %s`, filePath)
			delCmd := fmt.Sprintf(`sed -i '/testAAAAAAAAA/d'  %s`, filePath)
			defer exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", delCmd)
			debugNodeStdout, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", createCmd)
			o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
			e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)
			fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")
			cmName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatus", fi1.name+"-"+nodeName, "-n", sub.namespace,
				`-o=jsonpath={.results[?(@.condition=="Failed")].resultConfigMapName}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			fi1.getDataFromConfigmap(oc, cmName, filePath)
		}
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-ROSA-ARO-OSD_CCS-NonHyperShiftHOST-NonPreRelease-CPaasrunOnly-ConnectedOnly-WRS-High-60960-V-EST.01-check the initialDelay could work as expected [Serial]", func() {
		var (
			fioInitialdelayTemplate = filepath.Join(buildPruningBaseDir, "fileintegrity_initialdelay.yaml")
			cmMaster                = "master-aide-conf"
			cmWorker                = "worker-aide-conf"
			initialdelay            = 100
			fileintegrityMaster     = fileintegrity{
				name:              "fileintegrity-master-" + getRandomString(),
				namespace:         "openshift-file-integrity",
				configname:        cmMaster,
				configkey:         "aide-conf",
				graceperiod:       30,
				debug:             false,
				nodeselectorkey:   "node-role.kubernetes.io/master",
				nodeselectorvalue: "",
				template:          fioInitialdelayTemplate,
			}
			fileintegrityWorker = fileintegrity{
				name:              "fileintegrity-worker" + getRandomString(),
				namespace:         "openshift-file-integrity",
				configname:        cmWorker,
				configkey:         "aide-conf",
				graceperiod:       30,
				debug:             false,
				nodeselectorkey:   "node-role.kubernetes.io/worker",
				nodeselectorvalue: "",
				template:          fioInitialdelayTemplate,
			}
		)

		g.By("Create configmaps and fileintegrites..")
		defer cleanupObjects(oc,
			objectTableRef{"configmap", fileintegrityMaster.namespace, fileintegrityMaster.configname},
			objectTableRef{"fileintegrity", fileintegrityMaster.namespace, fileintegrityMaster.name},
			objectTableRef{"configmap", fileintegrityMaster.namespace, fileintegrityWorker.configname},
			objectTableRef{"fileintegrity", fileintegrityMaster.namespace, fileintegrityWorker.name})
		fileintegrityMaster.createConfigmapFromFile(oc, fileintegrityMaster.configname, fileintegrityMaster.configkey, configFile, "created")
		fileintegrityMaster.createConfigmapFromFile(oc, fileintegrityMaster.configname, fileintegrityMaster.configkey, configFile1, "created")
		fileintegrityMaster.checkConfigmapCreated(oc)
		fileintegrityWorker.checkConfigmapCreated(oc)
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", fileintegrityMaster.namespace, "-f", fileintegrityMaster.template, "-p", "NAME="+fileintegrityMaster.name,
			"NAMESPACE="+fileintegrityMaster.namespace, "GRACEPERIOD="+strconv.Itoa(fileintegrityWorker.graceperiod), "DEBUG="+strconv.FormatBool(fileintegrityWorker.debug), "CONFNAME="+fileintegrityMaster.configname,
			"CONFKEY="+fileintegrityMaster.configkey, "NODESELECTORKEY="+fileintegrityMaster.nodeselectorkey, "INITIALDELAY="+strconv.Itoa(initialdelay))
		o.Expect(err).NotTo(o.HaveOccurred())
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", fileintegrityWorker.namespace, "-f", fileintegrityWorker.template, "-p", "NAME="+fileintegrityWorker.name,
			"NAMESPACE="+fileintegrityWorker.namespace, "GRACEPERIOD="+strconv.Itoa(fileintegrityWorker.graceperiod), "DEBUG="+strconv.FormatBool(fileintegrityWorker.debug), "CONFNAME="+fileintegrityWorker.configname,
			"CONFKEY="+fileintegrityWorker.configkey, "NODESELECTORKEY="+fileintegrityWorker.nodeselectorkey, "INITIALDELAY="+strconv.Itoa(initialdelay))
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, fileintegrityMaster.name, ok, []string{"fileintegrity", "-n", fileintegrityMaster.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, fileintegrityWorker.name, ok, []string{"fileintegrity", "-n", fileintegrityWorker.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check the fileintegrity daemonset will not be ready due to the initialdelay..")
		res, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegrity", "-n", fileintegrityMaster.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		list := strings.Fields(res)
		for _, fileintegrityName := range list {
			err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", "aide-"+fileintegrityName, "-n", fileintegrityMaster.namespace).Output()
				if err != nil && strings.Contains(output, "NotFound") {
					return false, nil
				}
				if err != nil && !strings.Contains(output, "NotFound") {
					return false, err
				}
				return true, nil
			})
			exutil.AssertWaitPollWithErr(err, "The timeout err is expected")
		}

		g.By("Check daemonset and fileintegrity status..")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fileintegrityMaster.name, "-n", fileintegrityMaster.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fileintegrityWorker.name, "-n", fileintegrityWorker.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		checkPodsStautsOfDaemonset(oc, "aide-"+fileintegrityMaster.name, fileintegrityMaster.namespace)
		checkPodsStautsOfDaemonset(oc, "aide-"+fileintegrityWorker.name, fileintegrityWorker.namespace)
		fileintegrityMaster.assertNodesConditionNotEmpty(oc)
		fileintegrityWorker.assertNodesConditionNotEmpty(oc)
	})

	//author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-ROSA-ARO-OSD_CCS-WRS-NonPreRelease-CPaasrunOnly-High-43136-Medium-55781-V-EST.01-Check FIO metrics and alerting [Serial][Slow]", func() {
		// skip test if telemetry not found
		skipNotelemetryFound(oc)
		var alerts []byte
		var errAlert error

		g.By("Label the namespace  !!!\n")
		labelNameSpace(oc, sub.namespace, "openshift.io/cluster-monitoring=true")
		fi1.debug = false

		newCheck("expect", asAdmin, withoutNamespace, contain, "openshift.io/cluster-monitoring", ok, []string{"namespace", sub.namespace, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "metrics", ok, []string{"service", "-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create fileintegrity object with default aide config..\n")
		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fi1.name})
		fi1.createFIOWithoutConfig(oc)
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		// trigger reinit before checking fileintegritynodestatus, otherwise it could be Failed status
		fi1.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fi1.name+" annotate")
		fi1.checkFileintegrityStatus(oc, "running")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fi1.assertNodesConditionNotEmpty(oc)
		nodeName := fi1.getNodeName(oc)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Succeeded")

		g.By("trigger fileintegrity failure on node")
		var filePath = "/root/test" + getRandomString()
		defer exutil.DebugNodeWithChroot(oc, nodeName, "rm", "-rf", filePath)
		debugNodeStdout, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "mkdir", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)
		fi1.checkFileintegritynodestatus(oc, nodeName, "Failed")

		g.By("Check metrics available")
		metricsErr := []string{"file_integrity_operator_daemonset_update_total{operation=\"update\"}", "file_integrity_operator_node_failed{node=\"" + nodeName + "\"}",
			"file_integrity_operator_node_status_total{condition=\"Failed\",node=\"" + nodeName + "\"}"}
		url := fmt.Sprintf("https://metrics." + sub.namespace + ".svc:8585/metrics-fio")
		checkMetric(oc, metricsErr, url)
		newCheck("expect", asAdmin, withoutNamespace, contain, "file-integrity", ok, []string{"PrometheusRule", "-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NodeHasIntegrityFailure", ok, []string{"PrometheusRule", "file-integrity", "-n", sub.namespace, "-ojsonpath={.spec.groups[0].rules[0].alert}"}).check(oc)

		g.By("Curl from the service endpoint")
		podName, err := oc.AsAdmin().Run("get").Args("pods", "-n", sub.namespace, "-l", "name=file-integrity-operator", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podIP, errGet := oc.AsAdmin().Run("get").Args("pod", "-n", sub.namespace, "-l", "name=file-integrity-operator", "-o=jsonpath={.items[0].status.podIP}").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		urlMetrics := "http://" + podIP + ":8383/metrics"
		output, errCurl := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", sub.namespace, "-c", "file-integrity-operator", podName, "--", "curl", "-H", "-ks", urlMetrics).OutputToFile(getRandomString() + "isc-fio-metrics.json")
		o.Expect(errCurl).NotTo(o.HaveOccurred())
		defer exec.Command("rm", output).Output()
		result, errCommand := exec.Command("bash", "-c", "cat "+output+" | grep fileintegrity-controller").Output()
		o.Expect(errCommand).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(result), `controller_runtime_max_concurrent_reconciles{controller="fileintegrity-controller"}`)).To(o.BeTrue())

		g.By("Check there is NodeHasIntegrityFailure alert")
		integrityFailureAlertName := "NodeHasIntegrityFailure"
		alertManagerUrl := getAlertManager(oc)
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		alertManagerVersion, errGetAlertmanagerVersion := getAlertmanagerVersion(oc)
		o.Expect(errGetAlertmanagerVersion).NotTo(o.HaveOccurred())
		alertCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/%s/alerts", token, alertManagerUrl, alertManagerVersion)
		gjsonQueryAlertName, gjsonQueryAlertNameEqual, errGetAlertQueries := getAlertQueries(oc)
		o.Expect(errGetAlertQueries).NotTo(o.HaveOccurred())
		errIntegrityFailureAlert := wait.Poll(3*time.Second, 300*time.Second, func() (bool, error) {
			alerts, errAlert = exec.Command("bash", "-c", alertCMD).Output()
			o.Expect(errAlert).NotTo(o.HaveOccurred())
			if strings.Contains(gjson.Get(string(alerts), gjsonQueryAlertName).String(), integrityFailureAlertName) && strings.Contains(gjson.Get(string(alerts), gjsonQueryAlertNameEqual+integrityFailureAlertName+").labels.node").String(), nodeName) {
				return true, nil
			}
			return false, nil
		})
		if errIntegrityFailureAlert != nil {
			e2e.Logf("The alert is: %s", string(alerts))
		}
		o.Expect(errIntegrityFailureAlert).NotTo(o.HaveOccurred())

		g.By("Check there is no TargetDown alert")
		targetDownAlertName := "TargetDown"
		errGet = wait.Poll(3*time.Second, 150*time.Second, func() (bool, error) {
			alerts, err := exec.Command("bash", "-c", alertCMD).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(gjson.Get(string(alerts), gjsonQueryAlertName).String(), targetDownAlertName) && strings.Contains(gjson.Get(string(alerts), gjsonQueryAlertNameEqual+targetDownAlertName+").labels.namespace").String(), sub.namespace) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollWithErr(errGet, "The timeout err is expected")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-CPaasrunOnly-WRS-High-71796-V-EST.01-file integrity operator should pass DAST test", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)
		configFile := filepath.Join(buildPruningBaseDir, "rapidast/data_rapidastconfig_fileintegrity_v1alpha1.yaml")
		policyFile := filepath.Join(buildPruningBaseDir, "rapidast/customscan.policy")
		_, err := rapidastScan(oc, oc.Namespace(), configFile, policyFile, "fileintegrity.openshift.io_v1alpha1")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: bgudi@redhat.com
	g.It("Author:bgudi-NonHyperShiftHOST-WRS-High-72019-V-EST.01-Check http version for file-integrity-operator", func() {
		g.By("Check http version for metric serive")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		url := fmt.Sprintf("https://metrics.%v.svc:8585/metrics-fio", sub.namespace)
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-i", "-ks", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), `HTTP/1.1 200 OK`)).To(o.BeTrue())
	})
})
