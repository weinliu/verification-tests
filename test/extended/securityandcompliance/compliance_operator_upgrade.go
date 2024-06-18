package securityandcompliance

import (
	"path/filepath"
	"strconv"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance Compliance_Operator Pre-check and post-check for compliance operator upgrade", func() {
	defer g.GinkgoRecover()
	const (
		coNamspace = "openshift-compliance"
	)
	var (
		oc = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.Context("When the compliance-operator is installed", func() {
		var (
			buildPruningBaseDir        = exutil.FixturePath("testdata", "securityandcompliance")
			scansettingbindingTemplate = filepath.Join(buildPruningBaseDir, "scansettingbinding.yaml")
			upResourceConfMapTemplate  = filepath.Join(buildPruningBaseDir, "upgrade_rsconfigmap.yaml")

			ssb = scanSettingBindingDescription{
				name:            "cossb",
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis-node",
				profilename2:    "ocp4-cis",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			confmap = resourceConfigMapDescription{
				name:      "resource-config",
				namespace: "",
				rule:      0,
				variable:  0,
				profile:   0,
				template:  upResourceConfMapTemplate,
			}
		)

		g.BeforeEach(func() {
			g.By("Skip test when precondition not meet !!!")
			exutil.SkipNoOLMCore(oc)
			SkipMissingCatalogsource(oc)
			architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
			SkipMissingDefaultSC(oc)
			SkipMissingRhcosWorkers(oc)

			g.By("Check csv and pods for coNamspace !!!")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "-n", coNamspace, "-l",
				"operators.coreos.com/compliance-operator.openshift-compliance", "-o=jsonpath={.items[0].status.phase}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "ocp4", ok, []string{"pod", "--selector=profile-bundle=ocp4", "-n",
				coNamspace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "rhcos4", ok, []string{"pod", "--selector=profile-bundle=rhcos4", "-n",
				coNamspace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

			g.By("Check Compliance Operator & profileParser pods are in running state !!!")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=name=compliance-operator", "-n",
				coNamspace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=profile-bundle=ocp4", "-n",
				coNamspace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=profile-bundle=rhcos4", "-n",
				coNamspace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)

			g.By("Check profilebundle status and metrics service !!!")
			newCheck("expect", asAdmin, withoutNamespace, compare, "VALID", ok, []string{"profilebundle", "ocp4", "-n", coNamspace,
				"-ojsonpath={.status.dataStreamStatus}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, compare, "VALID", ok, []string{"profilebundle", "rhcos4", "-n", coNamspace,
				"-ojsonpath={.status.dataStreamStatus}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "metrics", ok, []string{"service", "-n", coNamspace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		})

		// author: xiyuan@redhat.com
		g.It("NonHyperShiftHOST-Author:xiyuan-CPaasrunOnly-NonPreRelease-High-37721-High-37824-High-45014-precheck for compliance operator", func() {
			g.By("Create scansettingbinding !!!\n")
			ssb.namespace = coNamspace

			err3 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ssb.template, "-p", "NAME="+ssb.name, "NAMESPACE="+ssb.namespace,
				"PROFILENAME1="+ssb.profilename1, "PROFILEKIND1="+ssb.profilekind1, "PROFILENAME2="+ssb.profilename2, "SCANSETTINGNAME="+ssb.scansettingname)
			o.Expect(err3).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
				"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

			g.By("Check ComplianceSuite status, name and result.. !!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssb.name, "-n", coNamspace,
				"-o=jsonpath={.status.phase}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", coNamspace,
				"-o=jsonpath={.status.phase}"}).check(oc)
			checkComplianceSuiteResult(oc, coNamspace, ssb.name, "NON-COMPLIANT INCONSISTENT")
		})

		// author: pdhamdhe@redhat.com
		g.It("NonHyperShiftHOST-Author:pdhamdhe-CPaasrunOnly-NonPreRelease-High-45014-High-45956-precheck for compliance operator resources count and MachineConfigPool status", func() {
			g.By("Check the MachineConfigPool status after upgrade.. !!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "master", "-n", coNamspace,
				"-ojsonpath={.spec.paused}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "worker", "-n", coNamspace,
				"-ojsonpath={.spec.paused}"}).check(oc)

			g.By("Get the compliance operator resources before upgrade..!!!\n")
			ruleCnt := getOperatorResources(oc, "rules", coNamspace)
			variableCnt := getOperatorResources(oc, "variables", coNamspace)
			profileCnt := getOperatorResources(oc, "profiles.compliance", coNamspace)

			g.By("Create confimap to store data before upgrade.. !!\n")
			confmap.namespace = coNamspace
			confmap.rule = ruleCnt
			confmap.variable = variableCnt
			confmap.profile = profileCnt
			err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", confmap.template, "-p", "NAME="+confmap.name, "NAMESPACE="+confmap.namespace,
				"RULE="+strconv.Itoa(confmap.rule), "VARIABLE="+strconv.Itoa(confmap.variable), "PROFILE="+strconv.Itoa(confmap.profile))
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, confmap.name, ok, []string{"configmap", "-n", confmap.namespace,
				"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		})

		// author: xiyuan@redhat.com
		g.It("NonHyperShiftHOST-Author:xiyuan-CPaasrunOnly-NonPreRelease-High-37721-High-37824-postcheck for compliance operator", func() {
			defer cleanupObjects(oc,
				objectTableRef{"scansettingbinding", coNamspace, ssb.name})

			g.By("Trigger rescan using oc-compliance plugin.. !!")
			_, err := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", coNamspace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check ComplianceSuite status, name and result after first rescan.. !!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssb.name, "-n", coNamspace,
				"-o=jsonpath={.status.phase}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", coNamspace,
				"-o=jsonpath={.status.phase}"}).check(oc)
			checkComplianceSuiteResult(oc, coNamspace, ssb.name, "NON-COMPLIANT INCONSISTENT")
		})

		// author: pdhamdhe@redhat.com
		g.It("NonHyperShiftHOST-Author:pdhamdhe-CPaasrunOnly-NonPreRelease-High-45014-High-45956-postcheck for compliance operator resources count and MachineConfigPool status", func() {
			confmap.namespace = coNamspace
			defer cleanupObjects(oc, objectTableRef{"configmap", confmap.namespace, confmap.name})
			g.By("Check the MachineConfigPool status after upgrade.. !!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "master", "-n", coNamspace,
				"-ojsonpath={.spec.paused}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"machineconfigpool", "worker", "-n", coNamspace,
				"-ojsonpath={.spec.paused}"}).check(oc)

			g.By("Get the compliance operator resources after upgrade..!!!\n")
			ruleCnt := getOperatorResources(oc, "rules", coNamspace)
			variableCnt := getOperatorResources(oc, "variables", coNamspace)
			profileCnt := getOperatorResources(oc, "profiles.compliance", coNamspace)

			g.By("Compare the compliance operator resource count before and after upgrade.. !!\n")
			readFileLinesToCompare(oc, confmap.name, ruleCnt, coNamspace, "rule")
			readFileLinesToCompare(oc, confmap.name, variableCnt, coNamspace, "variable")
			readFileLinesToCompare(oc, confmap.name, profileCnt, coNamspace, "profile")
		})
	})
})
