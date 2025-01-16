package securityandcompliance

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance Compliance_Operator Intra-release upgrade", func() {
	defer g.GinkgoRecover()

	var (
		oc                               = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir              string
		ogCoTemplate                     string
		ogD                              operatorGroupDescription
		confmap                          resourceConfigMapDescription
		scansettingbindingSingleTemplate string
		scansettingTemplate              string
		ss                               scanSettingDescription
		subCoTemplate                    string
		subD                             subscriptionDescription
		ssb                              scanSettingBindingDescription
		ssbMetrics                       scanSettingBindingDescription
		upResourceConfMapTemplate        string
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogCoTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subCoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		scansettingbindingSingleTemplate = filepath.Join(buildPruningBaseDir, "oc-compliance-scansettingbinding.yaml")
		scansettingTemplate = filepath.Join(buildPruningBaseDir, "scansetting.yaml")
		upResourceConfMapTemplate = filepath.Join(buildPruningBaseDir, "upgrade_rsconfigmap.yaml")

		ogD = operatorGroupDescription{
			name:      "openshift-compliance",
			namespace: "openshift-compliance",
			template:  ogCoTemplate,
		}
		subD = subscriptionDescription{
			subName:                "compliance-operator",
			namespace:              "openshift-compliance",
			channel:                "stable",
			ipApproval:             "Automatic",
			operatorPackage:        "compliance-operator",
			catalogSourceName:      "redhat-operators",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			template:               subCoTemplate,
			singleNamespace:        true,
		}
		ss = scanSettingDescription{
			autoapplyremediations:  false,
			autoupdateremediations: false,
			name:                   "test-upg-" + getRandomString(),
			namespace:              subD.namespace,
			roles1:                 "master",
			roles2:                 "worker",
			rotation:               3,
			schedule:               "*/3 * * * *",
			size:                   "2Gi",
			priorityclassname:      "",
			debug:                  false,
			suspend:                false,
			template:               scansettingTemplate,
		}
		ssb = scanSettingBindingDescription{
			name:            "ssb-upg-" + getRandomString(),
			namespace:       subD.namespace,
			profilekind1:    "Profile",
			profilename1:    "ocp4-cis",
			scansettingname: ss.name,
			template:        scansettingbindingSingleTemplate,
		}
		ssbMetrics = scanSettingBindingDescription{
			name:            "ssb-upg-metrics-" + getRandomString(),
			namespace:       subD.namespace,
			profilekind1:    "Profile",
			profilename1:    "ocp4-cis",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}
		confmap = resourceConfigMapDescription{
			name:      "resource-config",
			namespace: "",
			rule:      0,
			variable:  0,
			profile:   0,
			template:  upResourceConfMapTemplate,
		}

		g.By("Skip test when precondition not meet !!!")
		exutil.SkipNoOLMCore(oc)
		subD.skipMissingCatalogsources(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
		skipMissingOrNotApplicableDefaultSC(oc)
		SkipMissingRhcosWorkers(oc)
		SkipClustersWithRhelNodes(oc)

		g.By("Install Compliance Operator and check it is sucessfully installed !!! ")
		uninstallComplianceOperator(oc, subD.namespace)
		createComplianceOperator(oc, subD, ogD)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-CPaasrunOnly-NonPreRelease-High-45014-High-45956-precheck and postcheck for compliance operator resources count and MachineConfigPool status[Slow][Serial]", func() {
		defer uninstallComplianceOperator(oc, subD.namespace)

		g.By("Get installed version and check whether upgradable !!!\n")
		csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/compliance-operator.openshift-compliance=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oldVersion := strings.ReplaceAll(csvName, "compliance-operator.v", "")
		oldVersion = strings.Trim(oldVersion, "'")
		upgradable, err := checkUpgradable(oc, "qe-app-registry", "stable", "compliance-operator", oldVersion, "compliance-operator.v")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The result of upgradable is: %v", upgradable)
		if !upgradable {
			g.Skip("Skip as no new version detected!")
		}

		g.By("Check the MachineConfigPool status after upgrade.. !!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "master", "-n", subD.namespace,
			"-ojsonpath={.spec.paused}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "worker", "-n", subD.namespace,
			"-ojsonpath={.spec.paused}"}).check(oc)

		g.By("Get the compliance operator resources before upgrade..!!!\n")
		ruleCnt := getOperatorResources(oc, "rules", subD.namespace)
		variableCnt := getOperatorResources(oc, "variables", subD.namespace)
		profileCnt := getOperatorResources(oc, "profiles.compliance", subD.namespace)
		confmap.namespace = subD.namespace
		confmap.rule = ruleCnt
		confmap.variable = variableCnt
		confmap.profile = profileCnt
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", confmap.template, "-p", "NAME="+confmap.name, "NAMESPACE="+confmap.namespace,
			"RULE="+strconv.Itoa(confmap.rule), "VARIABLE="+strconv.Itoa(confmap.variable), "PROFILE="+strconv.Itoa(confmap.profile))
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, confmap.name, ok, []string{"configmap", "-n", confmap.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Operator upgrade..!!!\n")
		patchSub := fmt.Sprintf("{\"spec\":{\"source\":\"qe-app-registry\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "sub", subD.subName, "--type", "merge", "-p", patchSub, "-n", subD.namespace)
		// Sleep 10 sesonds so that the operator upgrade will be triggered
		time.Sleep(10 * time.Second)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"csv", "-n", subD.namespace,
			"-ojsonpath={.items[0].status.phase}"}).check(oc)
		newCsvName, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/compliance-operator.openshift-compliance=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		newVersion := strings.ReplaceAll(newCsvName, "compliance-operator.v", "")
		o.Expect(newVersion).ShouldNot(o.Equal(oldVersion))

		g.By("Check the MachineConfigPool status after upgrade.. !!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "master", "-n", subD.namespace,
			"-ojsonpath={.spec.paused}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "worker", "-n", subD.namespace,
			"-ojsonpath={.spec.paused}"}).check(oc)

		g.By("Get the compliance operator resources after upgrade..!!!\n")
		ruleCnt = getOperatorResources(oc, "rules", subD.namespace)
		variableCnt = getOperatorResources(oc, "variables", subD.namespace)
		profileCnt = getOperatorResources(oc, "profiles.compliance", subD.namespace)

		g.By("Compare the compliance operator resource count before and after upgrade.. !!\n")
		readFileLinesToCompare(oc, confmap.name, ruleCnt, subD.namespace, "rule")
		readFileLinesToCompare(oc, confmap.name, variableCnt, subD.namespace, "variable")
		readFileLinesToCompare(oc, confmap.name, profileCnt, subD.namespace, "profile")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-CPaasrunOnly-NonPreRelease-High-37721-High-56351-precheck and postchck for compliance operator upgrade [Slow][Serial]", func() {
		defer uninstallComplianceOperator(oc, subD.namespace)

		g.By("Get installed version and check whether upgradable !!!\n")
		csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/compliance-operator.openshift-compliance=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oldVersion := strings.ReplaceAll(csvName, "compliance-operator.v", "")
		oldVersion = strings.Trim(oldVersion, "'")
		upgradable, err := checkUpgradable(oc, "qe-app-registry", "stable", "compliance-operator", oldVersion, "compliance-operator.v")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The result of upgradable is: %v", upgradable)
		if !upgradable {
			g.Skip("Skip as no new version detected!")
		}

		g.By("Create scansetting !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		err3 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ssb.template, "-p", "NAME="+ssb.name, "NAMESPACE="+ssb.namespace,
			"PROFILENAME1="+ssb.profilename1, "PROFILEKIND1="+ssb.profilekind1, "PROFILENAME2="+ssb.profilename2, "SCANSETTINGNAME="+ssb.scansettingname)
		o.Expect(err3).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status, name and result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		checkComplianceSuiteResult(oc, subD.namespace, ssb.name, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.schedule, ok, []string{"cronjob", ssb.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.schedule}"}).check(oc)

		g.By("Operator upgrade..!!!\n")
		patchSub := fmt.Sprintf("{\"spec\":{\"source\":\"qe-app-registry\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "sub", subD.subName, "--type", "merge", "-p", patchSub, "-n", subD.namespace)
		// Sleep 10 sesonds so that the operator upgrade will be triggered
		time.Sleep(10 * time.Second)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"csv", "-n", subD.namespace,
			"-ojsonpath={.items[0].status.phase}"}).check(oc)
		newCsvName, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/compliance-operator.openshift-compliance=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		newVersion := strings.ReplaceAll(newCsvName, "compliance-operator.v", "")
		o.Expect(newVersion).ShouldNot(o.Equal(oldVersion))

		g.By("Check ComplianceSuite status, name and result after first rescan.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		checkComplianceSuiteResult(oc, subD.namespace, ssb.name, "NON-COMPLIANT")
		lastSuccessfulTime, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "-n", subD.namespace, ssb.name+"-rerunner",
			"-o=jsonpath={.status.lastSuccessfulTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Patch the ss scansettingbinding to suspend the scan and check ssb status !!!\n")
		patchSuspendTrue := fmt.Sprintf("{\"suspend\":true}")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", patchSuspendTrue, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "SUSPENDED", ok, []string{"ssb", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"cronjob", ssb.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		lastSuccessfulTimeSuspended, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "-n", subD.namespace, ssb.name+"-rerunner",
			"-o=jsonpath={.status.lastSuccessfulTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(lastSuccessfulTimeSuspended).Should(o.Equal(lastSuccessfulTime))

		g.By("Patch the ss scansettingbinding to resume the scan and check ssb status !!!\n")
		patchSuspendFalse := fmt.Sprintf("{\"suspend\":false}")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", patchSuspendFalse, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"cronjob", ssb.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")
		lastSuccessfulTimeResumed, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "-n", subD.namespace, ssb.name+"-rerunner",
			"-o=jsonpath={.status.lastSuccessfulTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(lastSuccessfulTimeResumed).ShouldNot(o.Equal(lastSuccessfulTime))
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-CPaasrunOnly-NonPreRelease-Medium-56353-Check no TargetDown alert is raised after upgrading compliance operator to new version [Slow][Serial]", func() {
		metricSsbStr := []string{
			"compliance_operator_compliance_scan_status_total{name=\"ocp4-cis\",phase=\"DONE\",result=",
			"compliance_operator_compliance_state{name=\"" + ssbMetrics.name + "\"}"}

		defer uninstallComplianceOperator(oc, subD.namespace)

		g.By("Get installed version and check whether upgradable !!!\n")
		csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/compliance-operator.openshift-compliance=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oldVersion := strings.ReplaceAll(csvName, "compliance-operator.v", "")
		oldVersion = strings.Trim(oldVersion, "'")
		upgradable, err := checkUpgradable(oc, "qe-app-registry", "stable", "compliance-operator", oldVersion, "compliance-operator.v")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The result of upgradable is: %v", upgradable)
		if !upgradable {
			g.Skip("Skip as no new version detected!")
		}

		g.By("Operator upgrade..!!!\n")
		patchSub := fmt.Sprintf("{\"spec\":{\"source\":\"qe-app-registry\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "sub", subD.subName, "--type", "merge", "-p", patchSub, "-n", subD.namespace)
		// Sleep 10 sesonds so that the operator upgrade will be triggered
		time.Sleep(10 * time.Second)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"csv", "-n", subD.namespace,
			"-ojsonpath={.items[0].status.phase}"}).check(oc)
		newCsvName, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/compliance-operator.openshift-compliance=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		newVersion := strings.ReplaceAll(newCsvName, "compliance-operator.v", "")
		o.Expect(newVersion).ShouldNot(o.Equal(oldVersion))

		g.By("Check compliance operator status !!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=name=compliance-operator", "-n",
			subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "VALID", ok, []string{"profilebundle", "ocp4", "-n", subD.namespace,
			"-ojsonpath={.status.dataStreamStatus}"}).check(oc)
		arch := architecture.ClusterArchitecture(oc)
		switch arch {
		case architecture.PPC64LE:
		case architecture.S390X:
		case architecture.AMD64:
			newCheck("expect", asAdmin, withoutNamespace, compare, "VALID", ok, []string{"profilebundle", "rhcos4", "-n", subD.namespace,
				"-ojsonpath={.status.dataStreamStatus}"}).check(oc)
		default:
			e2e.Logf("Architecture %s is not supported", arch.String())
		}

		g.By("Check ComplianceSuite status, name and result after first rescan.. !!!\n")
		defer cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbMetrics.name})
		ssbMetrics.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbMetrics.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbMetrics.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		checkComplianceSuiteResult(oc, subD.namespace, ssbMetrics.name, "NON-COMPLIANT")

		g.By("Check metrics.. !!!\n")
		url := fmt.Sprintf("https://metrics." + subD.namespace + ".svc:8585/metrics-co")
		checkMetric(oc, metricSsbStr, url)
		newCheck("expect", asAdmin, withoutNamespace, contain, "compliance", ok, []string{"PrometheusRule", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NonCompliant", ok, []string{"PrometheusRule", "compliance", "-n", subD.namespace, "-ojsonpath={.spec.groups[0].rules[0].alert}"}).check(oc)

		g.By("check alerts")
		alertString := fmt.Sprintf("The compliance suite %s returned as NON-COMPLIANT, ERROR, or INCONSISTENT", ssbMetrics.name)
		checkAlert(oc, ssbMetrics.name, alertString, 300)

		g.By("check no targetdown alert")
		checkAlertNotExist(oc, "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{namespace=\"openshift-compliance\"}'", "TargetDown", 180)
	})
})
