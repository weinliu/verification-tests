package securityandcompliance

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance File_Integrity_Operator intra release upgrade", func() {
	defer g.GinkgoRecover()

	var (
		oc                      = exutil.NewCLI("fio-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir     string
		configFile              string
		fioInitialdelayTemplate string
		ogSingleTemplate        string
		subTemplate             string
		og                      operatorGroupDescription
		sub                     subscriptionDescription
		fileIntegritry          fileintegrity
	)

	g.BeforeEach(func() {
		g.By("Skip the test if the cluster has no OLM component")
		exutil.SkipNoOLMCore(oc)

		g.By("Skip test when missingcatalogsource, ARM64, or SkipHetegenous !!!")
		SkipClustersWithRhelNodes(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)

		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		fioInitialdelayTemplate = filepath.Join(buildPruningBaseDir, "fileintegrity_initialdelay.yaml")
		ogSingleTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		configFile = filepath.Join(buildPruningBaseDir, "aide.conf.rhel8")

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
			catalogSourceName:      "redhat-operators",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			template:               subTemplate,
			singleNamespace:        true,
		}
		fileIntegritry = fileintegrity{
			name:              "example-fileintegrity-" + getRandomString(),
			namespace:         "openshift-file-integrity",
			configname:        "cm-upgrade-" + getRandomString(),
			configkey:         "aide-conf",
			graceperiod:       30,
			debug:             false,
			nodeselectorkey:   "node.openshift.io/os_id",
			nodeselectorvalue: "rhcos",
			template:          fioInitialdelayTemplate,
		}

		sub.skipMissingCatalogsources(oc, "file-integrity-operator")
		g.By("Install File Integrity Operator and check it is sucessfully installed !!! ")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("fileintegrity", "--all", "-n", sub.namespace, "--ignore-not-found").Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", sub.namespace, "-n", sub.namespace, "--ignore-not-found").Execute()
		createFileIntegrityOperator(oc, sub, og)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-ARO-OSD_CCS-Critical-42663-Critical-45366-precheck and postcheck for file integrity operator [Serial][Slow]", func() {
		var nodes []string

		defer cleanupObjects(oc, objectTableRef{"fileintegrity", sub.namespace, fileIntegritry.name},
			objectTableRef{"ns", sub.namespace, sub.namespace})

		g.By("Get installed version and check whether upgradable !!!\n")
		csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", sub.namespace, "-l", "operators.coreos.com/file-integrity-operator.openshift-file-integrity=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oldVersion := strings.ReplaceAll(csvName, "file-integrity-operator.v", "")
		oldVersion = strings.Trim(oldVersion, "'")
		upgradable, err := checkUpgradable(oc, "qe-app-registry", "stable", "file-integrity-operator", oldVersion, "file-integrity-operator.v")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The result of upgradable is: %v", upgradable)
		if !upgradable {
			g.Skip("Skip as no new version detected!")
		}

		g.By("Create file integrity object  !!!\n")
		var initialdelay = 60
		fileIntegritry.createConfigmapFromFile(oc, fileIntegritry.configname, fileIntegritry.configkey, configFile, "created")
		errApply := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", fileIntegritry.namespace, "-f", fileIntegritry.template, "-p", "NAME="+fileIntegritry.name,
			"NAMESPACE="+fileIntegritry.namespace, "GRACEPERIOD="+strconv.Itoa(fileIntegritry.graceperiod), "DEBUG="+strconv.FormatBool(fileIntegritry.debug), "CONFNAME="+fileIntegritry.configname,
			"CONFKEY="+fileIntegritry.configkey, "NODESELECTORKEY="+fileIntegritry.nodeselectorkey, "NODESELECTORVALUE="+fileIntegritry.nodeselectorvalue, "INITIALDELAY="+strconv.Itoa(initialdelay))
		o.Expect(errApply).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, fileIntegritry.name, ok, []string{"fileintegrity", "-n", fileIntegritry.namespace,
			"-o=jsonpath={.items[0].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fileIntegritry.name, "-n", fileIntegritry.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check aid pod and file integrity object status.. !!!\n")
		fileIntegritry.assertNodesConditionNotEmpty(oc)
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatuses", "-n", fileIntegritry.namespace, "-o=jsonpath={.items[*].lastResult.condition}").Output()
		if strings.Contains(output, "Failed") {
			fileIntegritry.reinitFileintegrity(oc, "fileintegrity.fileintegrity.openshift.io/"+fileIntegritry.name+" annotate")
			fileIntegritry.checkFileintegrityStatus(oc, "running")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fileIntegritry.name, "-n", fileIntegritry.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", fileIntegritry.namespace, "-l app=aide-"+fileIntegritry.name, "-o=jsonpath={.items[*].spec.nodeName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodes = strings.Fields(output)
			for _, node := range nodes {
				fileIntegritry.checkFileintegritynodestatus(oc, node, "Succeeded")
			}
		}

		g.By("Operator upgrade..!!!\n")
		patchSub := fmt.Sprintf("{\"spec\":{\"source\":\"qe-app-registry\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "--type", "merge", "-p", patchSub, "-n", sub.namespace)
		// Sleep 10 sesonds so that the operator upgrade will be triggered
		time.Sleep(10 * time.Second)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"csv", "-n", sub.namespace,
			"-ojsonpath={.items[0].status.phase}"}).check(oc)
		newCsvName, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", sub.namespace, "-l", "operators.coreos.com/file-integrity-operator.openshift-file-integrity=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		newVersion := strings.ReplaceAll(newCsvName, "file-integrity-operator.v", "")
		o.Expect(newVersion).ShouldNot(o.Equal(oldVersion))

		g.By("Check aid pod and file integrity object status after upgrade.. !!!\n")
		aidpodNames, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l app=aide-example-fileintegrity", "-n", fileIntegritry.namespace,
			"-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		aidpodName := strings.Fields(aidpodNames)
		for _, podName := range aidpodName {
			newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", podName, "-n", fileIntegritry.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		}
		newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fileIntegritry.name, "-n", fileIntegritry.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		fileIntegritry.assertNodesConditionNotEmpty(oc)
		for _, node := range nodes {
			fileIntegritry.checkFileintegritynodestatus(oc, node, "Succeeded")
		}

		g.By("trigger fileintegrity failure on node")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", fileIntegritry.namespace, "-l app=aide-"+fileIntegritry.name, "-o=jsonpath={.items[0].spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var filePath = "/root/test" + getRandomString()
		defer exutil.DebugNodeWithChroot(oc, nodeName, "rm", "-rf", filePath)
		debugNodeStdout, debugNodeErr := exutil.DebugNodeWithChroot(oc, nodeName, "mkdir", filePath)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("The output of creating folder %s is: %s", filePath, debugNodeStdout)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Failed", ok, []string{"fileintegritynodestatus", fileIntegritry.name + "-" + nodeName, "-n", fileIntegritry.namespace, "-o=jsonpath={.lastResult.condition}"}).check(oc)
		cmName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegritynodestatus", fileIntegritry.name+"-"+nodeName, "-n", sub.namespace,
			`-o=jsonpath={.results[?(@.condition=="Failed")].resultConfigMapName}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		fileIntegritry.getDataFromConfigmap(oc, cmName, filePath)
	})
})
