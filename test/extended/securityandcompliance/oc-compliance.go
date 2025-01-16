package securityandcompliance

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance oc_compliance_plugin The OC Compliance plugin makes compliance operator easy to use", func() {
	defer g.GinkgoRecover()

	var (
		oc                         = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir        string
		ogCoTemplate               string
		subCoTemplate              string
		scansettingTemplate        string
		scansettingbindingTemplate string
		tprofileWithoutVarTemplate string
		ogD                        operatorGroupDescription
		subD                       subscriptionDescription
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogCoTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subCoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		scansettingTemplate = filepath.Join(buildPruningBaseDir, "oc-compliance-scansetting.yaml")
		scansettingbindingTemplate = filepath.Join(buildPruningBaseDir, "oc-compliance-scansettingbinding.yaml")
		tprofileWithoutVarTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-withoutvariable.yaml")
		ns := "openshift-compliance"

		ogD = operatorGroupDescription{
			name:      "openshift-compliance",
			namespace: ns,
			template:  ogCoTemplate,
		}
		subD = subscriptionDescription{
			subName:                "compliance-operator",
			namespace:              ns,
			channel:                "stable",
			ipApproval:             "Automatic",
			operatorPackage:        "compliance-operator",
			catalogSourceName:      "qe-app-registry",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			template:               subCoTemplate,
			singleNamespace:        true,
		}

		g.By("Skip test when precondition not meet !!!")
		exutil.SkipNoOLMCore(oc)
		subD.skipMissingCatalogsources(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
		skipMissingOrNotApplicableDefaultSC(oc)
		SkipMissingRhcosWorkers(oc)
		SkipClustersWithRhelNodes(oc)

		g.By("Install Compliance Operator and check it is sucessfully installed !!! ")
		createComplianceOperator(oc, subD, ogD)
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:pdhamdhe-High-40681-The oc compliance plugin rerun set of scans on command [Serial][Slow]", func() {

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "master-scansetting" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				rotation:               10,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "co-requirement" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis-node",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name})

		g.By("Check default profiles name ocp4-cis-node .. !!!\n")
		subD.getProfileName(oc, "ocp4-cis-node")

		ssb.namespace = subD.namespace
		ss.namespace = subD.namespace
		ssb.scansettingname = ss.name

		g.By("Create scansetting !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status, name and result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check ComplianceSuite status, name and result after first rescan.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)

		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		_, err1 := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())

		g.By("Check ComplianceSuite status, name and result after second rescan.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)

		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		_, err2 := OcComplianceCLI().Run("rerun-now").Args("compliancescan", "ocp4-cis-node-master", "-n", subD.namespace).Output()
		o.Expect(err2).NotTo(o.HaveOccurred())

		g.By("Check ComplianceScan status, name and result after third rescan.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancescan", "ocp4-cis-node-master", "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancescan", "ocp4-cis-node-master", "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)

		subD.complianceScanName(oc, "ocp4-cis-node-master")
		subD.getComplianceScanResult(oc, "ocp4-cis-node-master", "NON-COMPLIANT")

		g.By("The ocp-40681 The oc compliance plugin has performed rescan on command successfully... !!!!\n ")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Longduration-NonPreRelease-High-41185-The oc compliance controls command reports the compliance standards and controls that is benchmark fulfil for profiles [Slow]", func() {
		arch := architecture.ClusterArchitecture(oc)

		g.By("Check default profilebundles name and status.. !!!\n")

		subD.getProfileBundleNameandStatus(oc, "ocp4", "VALID")
		switch arch {
		case architecture.AMD64:
			subD.getProfileBundleNameandStatus(oc, "rhcos4", "VALID")
		case architecture.S390X, architecture.PPC64LE:
		default:
			e2e.Logf("Architecture %s is not supported", arch.String())
		}

		g.By("Check default profiles name.. !!!\n")

		subD.getProfileName(oc, "ocp4-cis")
		subD.getProfileName(oc, "ocp4-cis-node")
		subD.getProfileName(oc, "ocp4-moderate")
		subD.getProfileName(oc, "ocp4-moderate-node")
		switch arch {
		case architecture.PPC64LE:
			subD.getProfileName(oc, "ocp4-pci-dss")
			subD.getProfileName(oc, "ocp4-pci-dss-node")
			subD.getProfileName(oc, "ocp4-pci-dss-node-3-2")
			subD.getProfileName(oc, "ocp4-pci-dss-node-4-0")
		case architecture.AMD64:
			subD.getProfileName(oc, "ocp4-e8")
			subD.getProfileName(oc, "ocp4-nerc-cip")
			subD.getProfileName(oc, "ocp4-nerc-cip-node")
			subD.getProfileName(oc, "ocp4-pci-dss")
			subD.getProfileName(oc, "ocp4-pci-dss-node")
			subD.getProfileName(oc, "ocp4-pci-dss-node-3-2")
			subD.getProfileName(oc, "ocp4-pci-dss-node-4-0")
			subD.getProfileName(oc, "rhcos4-e8")
			subD.getProfileName(oc, "rhcos4-moderate")
			subD.getProfileName(oc, "rhcos4-nerc-cip")
		default:
			e2e.Logf("Architecture %s is not supported", arch.String())
		}

		g.By("Check profile standards and controls.. !!!\n")

		assertCheckProfileControls(oc, subD.namespace, "ocp4-cis", "CIP-003-8.*R4.1")
		assertCheckProfileControls(oc, subD.namespace, "ocp4-moderate-node", "AU-12.*c")
		switch arch {
		case architecture.PPC64LE:
			assertCheckProfileControls(oc, subD.namespace, "ocp4-pci-dss-node", "Req-10.5.2")
			assertCheckProfileControls(oc, subD.namespace, "ocp4-pci-dss-node-4-0", "10.3.2")
		case architecture.AMD64:
			assertCheckProfileControls(oc, subD.namespace, "ocp4-e8", "NIST-800-53")
			assertCheckProfileControls(oc, subD.namespace, "ocp4-high-node", "CIP-007-3.*R5.1.1")
			assertCheckProfileControls(oc, subD.namespace, "ocp4-nerc-cip", "PCI-DSS")
			assertCheckProfileControls(oc, subD.namespace, "ocp4-pci-dss-node", "Req-10.5.2")
			assertCheckProfileControls(oc, subD.namespace, "ocp4-pci-dss-node-4-0", "10.3.2")
			assertCheckProfileControls(oc, subD.namespace, "rhcos4-high", "CIP-003-8.*R3")
		default:
			e2e.Logf("Architecture %s is not supported", arch.String())
		}

		g.By("The ocp-41185 Successfully verify compliance standards and controls for all profiles ... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:pdhamdhe-High-41190-The view result command of oc compliance plugin exposes more information about a compliance result [Serial][Slow]", func() {

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "master-scansetting" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				rotation:               10,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "co-requirement" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name})

		g.By("Check default profiles name ocp4-cis.. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")

		ssb.namespace = subD.namespace
		ss.namespace = subD.namespace
		ssb.scansettingname = ss.name

		g.By("Create scansetting !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify rules status and result through oc-compliance view-result command.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-cis-api-server-admission-control-plugin-alwaysadmit", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		assertRuleResult(oc, "ocp4-cis-api-server-admission-control-plugin-alwaysadmit", subD.namespace,
			[...]string{"Status               | PASS", "Result Object Name   | ocp4-cis-api-server-admission-control-plugin-alwaysadmit"})

		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-cis-audit-log-forwarding-enabled", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		assertRuleResult(oc, "ocp4-cis-audit-log-forwarding-enabled", subD.namespace,
			[...]string{"Status               | FAIL", "Result Object Name   | ocp4-cis-audit-log-forwarding-enabled"})

		g.By("The ocp-41190 Successfully verify oc-compliance view-result reports result in details... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-High-41182-The bind command of oc compliance plugin will take the given parameters and create a ScanSettingBinding object [Serial][Slow]", func() {
		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "master-scansetting" + getRandomString(),
				namespace:              "",
				priorityclassname:      "",
				roles1:                 "master",
				rotation:               10,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				template:               scansettingTemplate,
			}
			tp = tailoredProfileWithoutVarDescription{
				name:         "ocp4-cis-custom",
				namespace:    "",
				extends:      "ocp4-cis",
				title:        "new profile from scratch",
				description:  "new profile with specific rules",
				enrulename1:  "ocp4-scc-limit-root-containers",
				enrulename2:  "ocp4-api-server-insecure-bind-address",
				disrulename1: "ocp4-api-server-encryption-provider-cipher",
				disrulename2: "ocp4-scc-drop-container-capabilities",
				template:     tprofileWithoutVarTemplate,
			}
			ssbName = "my-binding" + getRandomString()
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssbName},
			objectTableRef{"tailoredprofile", subD.namespace, tp.name},
			objectTableRef{"scansetting", subD.namespace, ss.name})

		g.By("Check default profiles name ocp4-cis.. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbName, "profile/ocp4-moderate", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify scansettingbinding, ScanSetting, profile objects created..!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbName, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ocp4-moderate", ok, []string{"scansettingbinding", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.profiles}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "default", ok, []string{"scansettingbinding", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.settingsRef}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		subD.complianceSuiteResult(oc, ssbName, "NON-COMPLIANT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbName, "2")

		cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbName})

		g.By("Create scansetting and tailoredProfile objects.. !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		tp.namespace = subD.namespace
		tp.create(oc)
		subD.getTailoredProfileNameandStatus(oc, tp.name)
		_, err1 := OcComplianceCLI().Run("bind").Args("-N", ssbName, "-S", ss.name, "tailoredprofile/ocp4-cis-custom", "-n", subD.namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())

		g.By("Verify scansettingbinding, ScanSetting, profile objects created..!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbName, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ocp4-cis-custom", ok, []string{"scansettingbinding", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.profiles}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansettingbinding", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.settingsRef}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		checkComplianceSuiteStatus(oc, ssbName, subD.namespace, "DONE")
		subD.complianceSuiteResult(oc, ssbName, "NON-COMPLIANT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbName, "2")

		assertDryRunBind(oc, "profile/ocp4-cis", subD.namespace, "name: ocp4-cis")
		assertDryRunBind(oc, "profile/ocp4-cis-node", subD.namespace, "name: ocp4-cis-node")
		assertDryRunBind(oc, "tailoredprofile/ocp4-cis-custom", subD.namespace, "name: ocp4-cis-custom")

		g.By("The ocp-41182 verify oc-compliance bind command works as per desing... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:pdhamdhe-High-40714-The oc compliance helps to download the raw compliance results from the Persistent Volume [Serial][Slow]", func() {
		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "master-scansetting",
				namespace:              "",
				roles1:                 "master",
				rotation:               10,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "co-requirement" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				scansettingname: "master-scansetting",
				template:        scansettingbindingTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name})

		g.By("Check default profiles name ocp4-cis.. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")

		g.By("Create scansetting !!!\n")
		ssb.namespace = subD.namespace
		ss.namespace = subD.namespace
		ssb.scansettingname = ss.name
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		assertfetchRawResult(oc, ssb.name, subD.namespace)

		g.By("The ocp-40714 fetches raw result of scan successfully... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-High-41195-The oc compliance plugin fetches the fixes or remediations from a rule profile or remediation objects [Serial][Slow]", func() {
		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "master-scansetting" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				rotation:               10,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "co-requirement" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name})

		g.By("Check default profiles name ocp4-cis.. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")

		g.By("Create scansetting !!!\n")
		ssb.namespace = subD.namespace
		ss.namespace = subD.namespace
		ssb.scansettingname = ss.name
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace, ss.name,
			"-o=jsonpath={.metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		assertfetchFixes(oc, "rule", "ocp4-api-server-encryption-provider-cipher", subD.namespace)
		assertfetchFixes(oc, ssb.profilekind1, ssb.profilename1, subD.namespace)

		g.By("The ocp-41195 fetches fixes from a rule profile or remediation objects successfully... !!!!\n ")
	})
})
