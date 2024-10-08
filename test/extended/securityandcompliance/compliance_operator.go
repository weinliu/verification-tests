package securityandcompliance

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance Compliance_Operator The Compliance Operator automates compliance check for OpenShift and CoreOS", func() {
	defer g.GinkgoRecover()

	var (
		oc                               = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir              string
		ogCoTemplate                     string
		subCoTemplate                    string
		csuiteTemplate                   string
		csuiteRemTemplate                string
		csuitetpcmTemplate               string
		csuitetaintTemplate              string
		csuitenodeTemplate               string
		csuiteSCTemplate                 string
		cscanTemplate                    string
		cscantaintTemplate               string
		cscantaintsTemplate              string
		cscanSCTemplate                  string
		errGetImageProfile               error
		relatedImageProfile              string
		tprofileManualRuleTemplate       string
		tprofileTemplate                 string
		tprofileWithoutVarTemplate       string
		scansettingTemplate              string
		scansettingLimitTemplate         string
		scansettingSingleTemplate        string
		scansettingbindingTemplate       string
		scansettingbindingSingleTemplate string
		sccTemplate                      string
		profilebundleTemplate            string
		pvExtractPodTemplate             string
		storageClassTemplate             string
		fioTemplate                      string
		fluentdCmYAML                    string
		fluentdDmYAML                    string
		clusterLogForYAML                string
		clusterLoggingYAML               string
		ldapConfigMapYAML                string
		motdConfigMapYAML                string
		consoleNotificationYAML          string
		networkPolicyYAML                string
		machineConfigPoolYAML            string
		mcTemplate                       string
		mcWithoutIgnitionEnableAuditYAML string
		prometheusAuditRuleYAML          string
		wordpressRouteYAML               string
		resourceQuotaYAML                string
		tprofileHypershfitTemplate       string
		tpSingleVariableTemplate         string
		tprofileTwoVariablesTemplate     string
		tprofileWithoutDescriptionYAML   string
		tprofileWithoutTitleYAML         string
		serviceYAML                      string
		ogD                              operatorGroupDescription
		subD                             subscriptionDescription
		storageClass                     storageClassDescription
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogCoTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subCoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		csuiteTemplate = filepath.Join(buildPruningBaseDir, "compliancesuite.yaml")
		csuiteRemTemplate = filepath.Join(buildPruningBaseDir, "compliancesuiterem.yaml")
		csuitetpcmTemplate = filepath.Join(buildPruningBaseDir, "compliancesuitetpconfmap.yaml")
		csuitetaintTemplate = filepath.Join(buildPruningBaseDir, "compliancesuitetaint.yaml")
		csuitenodeTemplate = filepath.Join(buildPruningBaseDir, "compliancesuitenodes.yaml")
		csuiteSCTemplate = filepath.Join(buildPruningBaseDir, "compliancesuiteStorageClass.yaml")
		cscanTemplate = filepath.Join(buildPruningBaseDir, "compliancescan.yaml")
		cscantaintTemplate = filepath.Join(buildPruningBaseDir, "compliancescantaint.yaml")
		cscantaintsTemplate = filepath.Join(buildPruningBaseDir, "compliancescantaints.yaml")
		cscanSCTemplate = filepath.Join(buildPruningBaseDir, "compliancescanStorageClass.yaml")
		tprofileManualRuleTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-manual-rule.yaml")
		tprofileTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile.yaml")
		tprofileHypershfitTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-hypershift.yaml")
		tpSingleVariableTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-single-variable.yaml")
		tprofileTwoVariablesTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-two-variables.yaml")
		tprofileWithoutVarTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-withoutvariable.yaml")
		scansettingTemplate = filepath.Join(buildPruningBaseDir, "scansetting.yaml")
		scansettingLimitTemplate = filepath.Join(buildPruningBaseDir, "scansettingLimit.yaml")
		scansettingSingleTemplate = filepath.Join(buildPruningBaseDir, "scansettingsingle.yaml")
		scansettingbindingTemplate = filepath.Join(buildPruningBaseDir, "scansettingbinding.yaml")
		scansettingbindingSingleTemplate = filepath.Join(buildPruningBaseDir, "oc-compliance-scansettingbinding.yaml")
		sccTemplate = filepath.Join(buildPruningBaseDir, "scc.yaml")
		profilebundleTemplate = filepath.Join(buildPruningBaseDir, "profilebundle.yaml")
		pvExtractPodTemplate = filepath.Join(buildPruningBaseDir, "pvextractpod.yaml")
		storageClassTemplate = filepath.Join(buildPruningBaseDir, "storage_class.yaml")
		fioTemplate = filepath.Join(buildPruningBaseDir, "fileintegrity.yaml")
		fluentdCmYAML = filepath.Join(buildPruningBaseDir, "fluentdConfigMap.yaml")
		fluentdDmYAML = filepath.Join(buildPruningBaseDir, "fluentdDeployment.yaml")
		clusterLogForYAML = filepath.Join(buildPruningBaseDir, "ClusterLogForwarder.yaml")
		clusterLoggingYAML = filepath.Join(buildPruningBaseDir, "ClusterLogging.yaml")
		ldapConfigMapYAML = filepath.Join(buildPruningBaseDir, "ldap_configmap.yaml")
		motdConfigMapYAML = filepath.Join(buildPruningBaseDir, "motd_configmap.yaml")
		consoleNotificationYAML = filepath.Join(buildPruningBaseDir, "consolenotification.yaml")
		networkPolicyYAML = filepath.Join(buildPruningBaseDir, "network-policy.yaml")
		machineConfigPoolYAML = filepath.Join(buildPruningBaseDir, "machineConfigPool.yaml")
		mcTemplate = filepath.Join(buildPruningBaseDir, "machine-config.yaml")
		mcWithoutIgnitionEnableAuditYAML = filepath.Join(buildPruningBaseDir, "machineconfig-no-igntion-enable-audit.yaml")
		prometheusAuditRuleYAML = filepath.Join(buildPruningBaseDir, "prometheus-audit.yaml")
		wordpressRouteYAML = filepath.Join(buildPruningBaseDir, "wordpress-route.yaml")
		resourceQuotaYAML = filepath.Join(buildPruningBaseDir, "resource-quota.yaml")
		tprofileWithoutDescriptionYAML = filepath.Join(buildPruningBaseDir, "tailoredprofile-withoutdescription.yaml")
		tprofileWithoutTitleYAML = filepath.Join(buildPruningBaseDir, "tailoredprofile-withouttitle.yaml")
		serviceYAML = filepath.Join(buildPruningBaseDir, "service-unsecure.yaml")

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
			catalogSourceName:      "qe-app-registry",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			template:               subCoTemplate,
			singleNamespace:        true,
		}
		storageClass = storageClassDescription{
			name:              "",
			provisioner:       "",
			reclaimPolicy:     "",
			volumeBindingMode: "",
			template:          storageClassTemplate,
		}

		g.By("Skip test when precondition not meet !!!")
		exutil.SkipNoOLMCore(oc)
		subD.skipMissingCatalogsources(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
		SkipMissingDefaultSC(oc)
		SkipMissingRhcosWorkers(oc)
		SkipClustersWithRhelNodes(oc)

		g.By("Install Compliance Operator and check it is sucessfully installed !!! ")
		createComplianceOperator(oc, subD, ogD)

		g.By("Get content image !!! ")
		relatedImageProfile, errGetImageProfile = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "name=compliance-operator", "-o=jsonpath={.items[*].spec.containers[0].env[?(.name == 'RELATED_IMAGE_PROFILE')].value}").Output()
		o.Expect(errGetImageProfile).NotTo(o.HaveOccurred())
	})

	// author: pdhamdhe@redhat.com
	g.It("Author:pdhamdhe-LEVEL0-NonHyperShiftHOST-ARO-ConnectedOnly-Critical-27649-The ComplianceSuite reports the scan result as Compliant or Non-Compliant [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector: "wscan",
				template:     csuiteTemplate,
			}
			csuiteMD = complianceSuiteDescription{
				name:         "master-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "master-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
				nodeSelector: "master",
				template:     csuiteTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
			objectTableRef{"compliancesuite", subD.namespace, csuiteMD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create worker-compliancesuite !!!\n")
		csuiteD.create(oc)
		csuiteMD.namespace = subD.namespace
		g.By("Create master-compliancesuite !!!\n")
		csuiteMD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check master-compliancesuite name and result..!!!\n")
		subD.complianceSuiteResult(oc, csuiteMD.name, "NON-COMPLIANT")
		g.By("Check master-compliancesuite result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteMD.name, "2")

		g.By("Check worker-compliancesuite name and result..!!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		g.By("The ocp-27649 ComplianceScan has performed successfully... !!!!\n ")

	})

	/* Disabling the test case, it might be required in future release
	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Medium-32082-The ComplianceSuite shows the scan result NOT-APPLICABLE after all rules are skipped to scan", func() {

		var (
			csuite = complianceSuiteDescription{
				name:                "worker-compliancesuite",
				namespace:           "",
				scanname:            "worker-scan",
				profile:             "xccdf_org.ssgproject.content_profile_ncp",
				content:             "ssg-rhel7-ds.xml",
				contentImage:        relatedImageProfile,
				noExternalResources: true,
				nodeSelector:        "wscan",
				template:            csuiteTemplate,
			}
			itName = g.CurrentSpecReport().FullText()
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, "worker-compliancesuite"})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuite.namespace = subD.namespace
		g.By("Create compliancesuite !!!\n")
		e2e.Logf("Here namespace : %v\n", catSrc.namespace)
		csuite.create(oc)

		g.By("Check complianceSuite Status !!!\n")
		csuite.checkComplianceSuiteStatus(oc, "DONE")

		g.By("Check worker scan pods status !!! \n")
		subD.scanPodStatus(oc, "Succeeded")

		g.By("Check complianceSuite name and result..!!!\n")
		subD.complianceSuiteName(oc, "worker-compliancesuite")
		subD.complianceSuiteResult(oc, csuite.name, "NOT-APPLICABLE")

		g.By("Check rule status through complianceCheckResult.. !!!\n")
		subD.getRuleStatus(oc, "SKIP")

		g.By("The ocp-32082 complianceScan has performed successfully....!!!\n")

	})*/

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:pdhamdhe-High-33398-The Compliance Operator supports to variables in tailored profile [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			tprofileD = tailoredProfileDescription{
				name:         "rhcos-tailoredprofile",
				namespace:    "",
				extends:      "rhcos4-moderate",
				enrulename1:  "rhcos4-sshd-disable-root-login",
				disrulename1: "rhcos4-audit-rules-dac-modification-chmod",
				disrulename2: "rhcos4-audit-rules-etc-group-open",
				varname:      "rhcos4-var-selinux-state",
				value:        "permissive",
				template:     tprofileTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"tailoredprofile", subD.namespace, tprofileD.name})

		tprofileD.namespace = subD.namespace
		g.By("Create tailoredprofile !!!\n")
		tprofileD.create(oc)
		g.By("Check tailoredprofile name and status !!!\n")
		subD.getTailoredProfileNameandStatus(oc, tprofileD.name)

		g.By("Verify the tailoredprofile details through configmap ..!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "xccdf_org.ssgproject.content_rule_sshd_disable_root_login", ok,
			[]string{"configmap", "rhcos-tailoredprofile-tp", "-n", subD.namespace, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "xccdf_org.ssgproject.content_rule_audit_rules_dac_modification_chmod", ok,
			[]string{"configmap", "rhcos-tailoredprofile-tp", "-n", subD.namespace, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "xccdf_org.ssgproject.content_value_var_selinux_state", ok,
			[]string{"configmap", "rhcos-tailoredprofile-tp", "-n", subD.namespace, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "permissive", ok, []string{"configmap", "rhcos-tailoredprofile-tp", "-n", subD.namespace,
			"-o=jsonpath={.data}"}).check(oc)

		g.By("ocp-33398 The Compliance Operator supported variables in tailored profile... !!!\n")

	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Author:xiyuan-Medium-61342-Verify the rule ocp4-cis-scc-limit-container-allowed-capabilities could be set to MANUAL [Serial]", func() {
		g.By("Create tailoredprofile !!!\n")
		var (
			tpName = "manual-" + getRandomString()
			ssb    = scanSettingBindingDescription{
				name:            "manual-rule-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "TailoredProfile",
				profilename1:    tpName,
				profilename2:    "ocp4-cis",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
		)

		g.By("Create tp... !!!\n")
		defer cleanupObjects(oc,
			objectTableRef{"tailoredprofile", subD.namespace, tpName},
			objectTableRef{"ssb", subD.namespace, ssb.name})
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tprofileManualRuleTemplate, "-p", "NAME="+tpName, "NAMESPACE="+subD.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, tpName, ok, []string{"tailoredprofile", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", tpName, "-n", subD.namespace, "-o=jsonpath={.status.state}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check test results !!!\n")
		assertKeywordsExists(oc, 300, "DONE", "compliancesuite", ssb.name, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancesuite",
			ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "MANUAL", ok, []string{"ccr", tpName + "-scc-limit-container-allowed-capabilities", "-n", subD.namespace,
			"-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"ccr", ssb.profilename2 + "-scc-limit-container-allowed-capabilities", "-n", subD.namespace,
			"-o=jsonpath={.status}"}).check(oc)
		g.By("ocp-61342 Verify the rule ocp4-cis-scc-limit-container-allowed-capabilities could be set to MANUAL... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:pdhamdhe-High-32840-The ComplianceSuite generates through ScanSetting CR [Serial][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			tprofileD = tailoredProfileDescription{
				name:         "rhcos-tp",
				namespace:    "",
				extends:      "rhcos4-e8",
				enrulename1:  "rhcos4-sshd-disable-root-login",
				disrulename1: "rhcos4-no-empty-passwords",
				disrulename2: "rhcos4-audit-rules-dac-modification-chown",
				varname:      "rhcos4-var-selinux-state",
				value:        "permissive",
				template:     tprofileTemplate,
			}
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "myss",
				namespace:              "",
				roles1:                 "master",
				roles2:                 "worker",
				rotation:               10,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "master-compliancesuite" + getRandomString(),
				namespace:       "",
				profilekind1:    "TailoredProfile",
				profilename1:    "rhcos-tp",
				profilename2:    "ocp4-moderate",
				scansettingname: "",
				template:        scansettingbindingTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name},
			objectTableRef{"tailoredprofile", subD.namespace, ssb.profilename1})

		g.By("Check default profiles name rhcos4-e8 .. !!!\n")
		subD.getProfileName(oc, tprofileD.extends)

		tprofileD.namespace = subD.namespace
		ssb.namespace = subD.namespace
		ss.namespace = subD.namespace
		ssb.scansettingname = ss.name
		g.By("Create tailoredprofile rhcos-tp !!!\n")
		tprofileD.create(oc)
		g.By("Verify tailoredprofile name and status !!!\n")
		subD.getTailoredProfileNameandStatus(oc, ssb.profilename1)

		g.By("Create scansetting !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace, ss.name,
			"-o=jsonpath={.metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertKeywordsExists(oc, 300, "DONE", "compliancesuite", ssb.name, "-o=jsonpath={.status.phase}", "-n", subD.namespace)

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")
		g.By("Check complianceSuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("Verify the disable rules are not available in compliancecheckresult.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, tprofileD.name+"-"+ss.roles1+"-sshd-disable-root-login", ok, []string{"compliancecheckresult", "-l", "compliance.openshift.io/suite=" + ssb.name, "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, tprofileD.name+"-"+ss.roles1+"-no-empty-passwords", nok, []string{"compliancecheckresult", "-l", "compliance.openshift.io/suite=" + ssb.name, "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, tprofileD.name+"-"+ss.roles1+"-rules-dac-modification-chown", nok, []string{"compliancecheckresult", "-n", subD.namespace}).check(oc)

		g.By("ocp-32840 The ComplianceSuite generated successfully using scansetting... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Medium-33381-Verify the ComplianceSuite could be generated from Tailored profiles [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			tprofileD = tailoredProfileDescription{
				name:         "rhcos-e8-tp",
				namespace:    "",
				extends:      "rhcos4-e8",
				enrulename1:  "rhcos4-sshd-disable-root-login",
				disrulename1: "rhcos4-no-empty-passwords",
				disrulename2: "rhcos4-audit-rules-dac-modification-chown",
				varname:      "rhcos4-var-selinux-state",
				value:        "permissive",
				template:     tprofileTemplate,
			}
			csuiteD = complianceSuiteDescription{
				name:               "rhcos-csuite" + getRandomString(),
				namespace:          "",
				scanname:           "rhcos-scan" + getRandomString(),
				profile:            "xccdf_compliance.openshift.io_profile_rhcos-e8-tp",
				content:            "ssg-rhcos4-ds.xml",
				contentImage:       relatedImageProfile,
				nodeSelector:       "wscan",
				tailoringConfigMap: "rhcos-e8-tp-tp",
				template:           csuitetpcmTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
			objectTableRef{"tailoredprofile", subD.namespace, tprofileD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Check default profiles name rhcos4-e8 .. !!!\n")
		subD.getProfileName(oc, tprofileD.extends)
		tprofileD.namespace = subD.namespace
		g.By("Create tailoredprofile !!!\n")
		tprofileD.create(oc)
		g.By("Check tailoredprofile name and status !!!\n")
		subD.getTailoredProfileNameandStatus(oc, tprofileD.name)

		csuiteD.namespace = subD.namespace
		g.By("Create compliancesuite !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check rhcos-csuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")
		g.By("Check rhcos-csuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "2")

		g.By("Verify the enable and disable rules.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-sshd-disable-root-login", ok, []string{"compliancecheckresult", "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-no-empty-passwords", nok, []string{"compliancecheckresult", "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-audit-rules-dac-modification-chown", nok, []string{"compliancecheckresult", "-n", subD.namespace}).check(oc)

		g.By("ocp-33381 The ComplianceSuite performed scan successfully using tailored profile... !!!\n")

	})
	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:xiyuan-Medium-33611-Verify the tolerations could work for compliancescan when there is more than one taint on node [Disruptive]", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var cscanD = complianceScanDescription{
			name:         "example-compliancescan3" + getRandomString(),
			namespace:    "",
			profile:      "xccdf_org.ssgproject.content_profile_moderate",
			content:      "ssg-rhcos4-ds.xml",
			contentImage: relatedImageProfile,
			rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
			key:          "key1",
			value:        "value1",
			operator:     "Equal",
			key1:         "key2",
			value1:       "value2",
			operator1:    "Equal",
			nodeSelector: "wscan",
			debug:        true,
			template:     cscantaintsTemplate,
		}

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Label and set taint value to one worker node.. !!!\n")
		nodeName := getOneWorkerNodeName(oc)
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule", "key2=value2:NoExecute")
		labelTaintNode(oc, "node", nodeName, "taint=true")
		defer func() {
			taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-", "key2=value2:NoExecute-")
			labelTaintNode(oc, "node", nodeName, "taint-")
			cleanupObjects(oc, objectTableRef{"compliancescan", subD.namespace, cscanD.name})
		}()

		cscanD.namespace = subD.namespace
		g.By("Create compliancescan.. !!!\n")
		cscanD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceScan name and result.. !!!\n")
		subD.complianceScanName(oc, cscanD.name)
		subD.getComplianceScanResult(oc, cscanD.name, "COMPLIANT")

		g.By("Check complianceScan result exit-code through configmap.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanD.name, "0")

		g.By("Verify if scan pod generated for tainted node.. !!!\n")
		assertCoPodNumerEqualNodeNumber(oc, cscanD.namespace, "node-role.kubernetes.io/wscan=", cscanD.name)

		g.By("ocp-33611 Verify the tolerations could work for compliancescan when there is more than one taints on node successfully.. !!!\n")

	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-High-37121-High-61422-High-66790-The ComplianceSuite generates through ScanSettingBinding CR with cis profile and default scansetting [Serial][Slow]", func() {
		var (
			ssb = scanSettingBindingDescription{
				name:            "cis-test" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				profilename2:    "ocp4-cis-node",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			ssbWithVersionSuffix14 = scanSettingBindingDescription{
				name:            "cis-test-1-4-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis-1-4",
				profilename2:    "ocp4-cis-node-1-4",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			ssbWithVersionSuffix15 = scanSettingBindingDescription{
				name:            "cis-test-1-5-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis-1-5",
				profilename2:    "ocp4-cis-node-1-5",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			ccrsShouldPass = []string{
				"ocp4-cis-node-master-file-groupowner-etcd-pki-cert-files",
				"ocp4-cis-node-master-file-groupowner-openshift-pki-cert-files",
				"ocp4-cis-node-master-file-groupowner-openshift-pki-key-files",
				"ocp4-cis-node-master-file-owner-etcd-pki-cert-files",
				"ocp4-cis-node-master-file-owner-openshift-pki-cert-files",
				"ocp4-cis-node-master-file-owner-openshift-pki-key-files",
				"ocp4-cis-node-master-file-permissions-etcd-pki-cert-files",
				"ocp4-cis-node-master-file-permissions-openshift-pki-cert-files",
				"ocp4-cis-node-master-file-permissions-openshift-pki-key-files",
				"ocp4-cis-node-master-file-permissions-controller-manager-kubeconfig",
				"ocp4-cis-node-master-file-permissions-etcd-data-dir",
				"ocp4-cis-node-master-file-permissions-etcd-data-files",
				"ocp4-cis-node-master-file-permissions-etcd-member",
				"ocp4-cis-node-master-file-permissions-kube-apiserver",
				"ocp4-cis-node-master-file-permissions-kube-controller-manager",
				"ocp4-cis-node-master-file-permissions-kubelet-conf",
				"ocp4-cis-node-master-file-permissions-master-admin-kubeconfigs",
				"ocp4-cis-node-master-file-permissions-multus-conf",
				"ocp4-cis-node-master-file-permissions-ovs-conf-db",
				"ocp4-cis-node-master-file-permissions-ovs-conf-db-lock",
				"ocp4-cis-node-master-file-permissions-ovs-pid",
				"ocp4-cis-node-master-file-permissions-ovs-sys-id-conf",
				"ocp4-cis-node-master-file-permissions-ovs-vswitchd-pid",
				"ocp4-cis-node-master-file-permissions-ovsdb-server-pid",
				"ocp4-cis-node-master-file-permissions-scheduler",
				"ocp4-cis-node-master-file-permissions-scheduler-kubeconfig",
				"ocp4-cis-node-master-file-permissions-worker-ca",
				"ocp4-cis-node-master-file-permissions-worker-kubeconfig",
				"ocp4-cis-node-master-file-permissions-worker-service",
				"ocp4-cis-node-master-kubelet-enable-streaming-connections",
				"ocp4-cis-node-master-kubelet-configure-event-creation",
				"ocp4-cis-node-master-kubelet-configure-tls-cipher-suites",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-hard-imagefs-available",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-hard-memory-available",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-hard-nodefs-available",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-hard-nodefs-inodesfree"}
			ccrsShouldNotExist = []string{
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-hard-imagefs-inodesfree",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-soft-imagefs-available",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-soft-imagefs-inodesfree",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-soft-memory-available",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-soft-nodefs-available",
				"ocp4-cis-node-master-kubelet-eviction-thresholds-set-soft-nodefs-inodesfree"}
			cniConfFilePermissionRule = "ocp4-cis-node-master-file-permissions-cni-conf"
		)

		g.By("Check default profiles name ocp4-cis .. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")
		g.By("Check default profiles name ocp4-cis-node .. !!!\n")
		subD.getProfileName(oc, "ocp4-cis-node")

		g.By("Create scansettingbinding !!!\n")
		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansettingbinding", subD.namespace, ssbWithVersionSuffix14.name},
			objectTableRef{"scansettingbinding", subD.namespace, ssbWithVersionSuffix15.name})
		ssb.create(oc)
		ssbWithVersionSuffix14.create(oc)
		ssbWithVersionSuffix15.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbWithVersionSuffix14.name, ok, []string{"scansettingbinding", "-n", ssbWithVersionSuffix14.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbWithVersionSuffix15.name, ok, []string{"scansettingbinding", "-n", ssbWithVersionSuffix15.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssb.name, subD.namespace, "DONE")
		checkComplianceSuiteStatus(oc, ssbWithVersionSuffix14.name, subD.namespace, "DONE")
		checkComplianceSuiteStatus(oc, ssbWithVersionSuffix15.name, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteName(oc, ssbWithVersionSuffix14.name)
		subD.complianceSuiteName(oc, ssbWithVersionSuffix15.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")
		subD.complianceSuiteResult(oc, ssbWithVersionSuffix14.name, "NON-COMPLIANT INCONSISTENT")
		subD.complianceSuiteResult(oc, ssbWithVersionSuffix15.name, "NON-COMPLIANT INCONSISTENT")
		g.By("Check complianceSuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbWithVersionSuffix14.name, "2")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbWithVersionSuffix15.name, "2")

		g.By("Check the rule count for each scan.. !!!\n")
		scanNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("scan", "-n", ssb.namespace, "-l", "compliance.openshift.io/suite="+ssb.name,
			"-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		scanNameList := strings.Fields(scanNames)
		for _, scanName := range scanNameList {
			checkRuleCountMatch(oc, ssb.namespace, scanName)
		}

		g.By("Check ccr should exist and pass by default.. !!!\n")
		timestamp, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("scan/ocp4-cis-node-master", "-n", ssb.namespace, "-ojsonpath={.status.startTimestamp}").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		for _, ccrShouldPass := range ccrsShouldPass {
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"ccr", ccrShouldPass, "-n", ssb.namespace,
				"-o=jsonpath={.status}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, timestamp, ok, []string{"ccr", ccrShouldPass, "-n", ssb.namespace,
				"-o=jsonpath={.metadata.annotations.compliance\\.openshift\\.io/last-scanned-timestamp}"}).check(oc)
		}

		g.By("Check cni rule.. !!!\n")
		version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-ojsonpath={.status.desired.version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		major := strings.Split(version, ".")[0]
		minor := strings.Split(version, ".")[1]
		minorInt, err := strconv.Atoi(minor)
		o.Expect(err).NotTo(o.HaveOccurred())
		switch {
		case major == "4" && minorInt >= 16:
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"ccr", cniConfFilePermissionRule, "-n", ssb.namespace,
				"-o=jsonpath={.status}"}).check(oc)
		case major == "4" && minorInt < 16:
		default:
			e2e.Logf("skip test for rule %s", cniConfFilePermissionRule)
		}

		g.By("Check ccr should not exist.. !!!\n")
		for _, ccrNotExist := range ccrsShouldNotExist {
			newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"ccr", ccrNotExist, "-n", subD.namespace}).check(oc)
		}

		g.By("Check rule ocp4-kubeadmin-removed.. !!!\n")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "kubeadmin", "-n", "kube-system").Output()
		if err != nil && strings.Contains(output, "NotFound") {
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"ccr", "ocp4-cis-kubeadmin-removed", "-n", ssb.namespace,
				"-o=jsonpath={.status}"}).check(oc)
		} else if err == nil && strings.Contains(output, "Opaque") {
			newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"ccr", "ocp4-cis-kubeadmin-removed", "-n", ssb.namespace,
				"-o=jsonpath={.status}"}).check(oc)
		} else {
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Diff the failed rules between profiles without suffix and profiles of latest version.. !!!\n")
		checkFailedRulesForTwoProfiles(oc, subD.namespace, ssb.name, ssbWithVersionSuffix15.name, "-1-5")

		g.By("ocp-37121-61422 The ComplianceSuite generated successfully using scansetting CR and cis profile and default scansetting... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Medium-33713-The ComplianceSuite reports the scan result as Error", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_coreos-ncp",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				nodeSelector: "wscan",
				template:     csuiteTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create compliancesuite !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "ERROR")

		g.By("Check complianceScan result through configmap exit-code and result from error-msg..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "1")
		subD.getScanResultFromConfigmap(oc, csuiteD.scanname, "No profile matching suffix \"xccdf_org.ssgproject.content_profile_coreos-ncp\" was found.")

		g.By("The ocp-33713 complianceScan has performed successfully....!!!\n")

	})

	// author: pdhamdhe@redhat.com
	g.It("StagerunBoth-NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Critical-27705-The ComplianceScan reports the scan result Compliant or Non-Compliant", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			cscanD = complianceScanDescription{
				name:         "worker-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector: "wscan",
				template:     cscanTemplate,
			}

			cscanMD = complianceScanDescription{
				name:         "master-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
				nodeSelector: "master",
				template:     cscanTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc,
			objectTableRef{"compliancescan", subD.namespace, cscanD.name},
			objectTableRef{"compliancescan", subD.namespace, cscanMD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		cscanD.namespace = subD.namespace
		g.By("Create worker-scan !!!\n")
		cscanD.create(oc)

		cscanMD.namespace = subD.namespace
		g.By("Create master-scan !!!\n")
		cscanMD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check master-scan name and result..!!!\n")
		subD.complianceScanName(oc, cscanD.name)
		subD.getComplianceScanResult(oc, cscanD.name, "NON-COMPLIANT")

		g.By("Check master-scan result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanD.name, "2")

		g.By("Check worker-scan name and result..!!!\n")
		subD.complianceScanName(oc, cscanMD.name)
		subD.getComplianceScanResult(oc, cscanMD.name, "COMPLIANT")

		g.By("Check worker-scan result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanMD.name, "0")

		g.By("The ocp-27705 ComplianceScan has performed successfully... !!!! ")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Medium-27762-The ComplianceScan reports the scan result Error", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			cscanD = complianceScanDescription{
				name:         "worker-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_coreos-ncp",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				nodeSelector: "wscan",
				template:     cscanTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"compliancescan", subD.namespace, cscanD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		cscanD.namespace = subD.namespace
		g.By("Create compliancescan !!!\n")
		cscanD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceScan name and result..!!!\n")
		subD.complianceScanName(oc, cscanD.name)
		subD.getComplianceScanResult(oc, cscanD.name, "ERROR")

		g.By("Check complianceScan result through configmap exit-code and result from error-msg..!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanD.name, "1")
		subD.getScanResultFromConfigmap(oc, cscanD.name, "No profile matching suffix \"xccdf_org.ssgproject.content_profile_coreos-ncp\" was found.")

		g.By("The ocp-27762 complianceScan has performed successfully....!!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:pdhamdhe-Medium-27968-Perform scan only on a subset of nodes using ComplianceScan object [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			cscanMD = complianceScanDescription{
				name:         "master-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
				nodeSelector: "master",
				template:     cscanTemplate,
			}
		)
		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"compliancescan", subD.namespace, cscanMD.name})

		cscanMD.namespace = subD.namespace
		g.By("Create master-scan !!!\n")
		cscanMD.create(oc)
		checkComplianceScanStatus(oc, cscanMD.name, subD.namespace, "DONE")

		g.By("Check master-scan name and result..!!!\n")
		subD.complianceScanName(oc, cscanMD.name)
		subD.getComplianceScanResult(oc, cscanMD.name, "NON-COMPLIANT")
		g.By("Check master-scan result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanMD.name, "2")

		g.By("The ocp-27968 ComplianceScan has performed successfully on a subset of nodes... !!!! ")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-High-33230-The compliance-operator raw result storage size is configurable", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector: "wscan",
				size:         "2Gi",
				template:     csuiteTemplate,
			}
			cscanMD = complianceScanDescription{
				name:         "master-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
				nodeSelector: "master",
				size:         "3Gi",
				template:     cscanTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
			objectTableRef{"compliancescan", subD.namespace, cscanMD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create worker-compliancesuite.. !!!\n")
		csuiteD.create(oc)

		cscanMD.namespace = subD.namespace
		g.By("Create master-scan.. !!!\n")
		cscanMD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")

		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		g.By("Check pvc name and storage size for worker-scan.. !!!\n")
		subD.getPVCName(oc, csuiteD.name)
		subD.getPVCSize(oc, "2Gi")

		g.By("Check master-scan name and result..!!!\n")
		subD.complianceScanName(oc, cscanMD.name)
		subD.getComplianceScanResult(oc, cscanMD.name, "NON-COMPLIANT")

		g.By("Check master-scan result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanMD.name, "2")

		g.By("Check pvc name and storage size for master-scan ..!!!\n")
		subD.getPVCName(oc, "master-scan")
		subD.getPVCSize(oc, "3Gi")

		g.By("The ocp-33230 complianceScan has performed successfully and storage size verified ..!!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-NonPreRelease-Longduration-ConnectedOnly-Author:pdhamdhe-High-33609-Verify the tolerations could work for compliancesuite [Disruptive]", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				key:          "key1",
				value:        "value1",
				operator:     "Equal",
				nodeSelector: "wscan",
				debug:        true,
				template:     csuitetaintTemplate,
			}
			csuite = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				key:          "key1",
				value:        "",
				operator:     "Exists",
				nodeSelector: "wscan",
				debug:        true,
				template:     csuitetaintTemplate,
			}
		)

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Label and set taint to one worker node.. !!!\n")
		//	setTaintLabelToWorkerNode(oc)
		//	setTaintToWorkerNodeWithValue(oc)
		nodeName := getOneWorkerNodeName(oc)
		defer func() {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.taints}").Output()
			if strings.Contains(output, "value1") {
				taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")
			}
			output1, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.taints[0].key}").Output()
			if strings.Contains(output1, "key1") {
				taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule-")
			}
			output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.labels.taint}").Output()
			if strings.Contains(output2, "true") {
				labelTaintNode(oc, "node", nodeName, "taint-")
			}
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancesuite", csuiteD.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancesuite", csuite.name, "-n", subD.namespace, "--ignore-not-found").Execute()
		}()
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule")
		labelTaintNode(oc, "node", nodeName, "taint=true")

		csuiteD.namespace = subD.namespace
		g.By("Create compliancesuite.. !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")

		g.By("Check complianceScan result exit-code through configmap.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		g.By("Verify if pod generated for tainted node.. !!!\n")
		assertCoPodNumerEqualNodeNumber(oc, subD.namespace, "node-role.kubernetes.io/wscan=", csuiteD.scanname)

		g.By("Remove csuite and taint label from worker node.. !!!\n")
		cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")

		g.By("Taint worker node without value.. !!!\n")
		/*	defer func() {
			taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule-")
		}()*/
		taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule")

		csuite.namespace = subD.namespace
		g.By("Create compliancesuite.. !!!\n")
		csuite.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuite.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, "worker-compliancesuite")
		subD.complianceSuiteResult(oc, csuite.name, "COMPLIANT")

		g.By("Check complianceScan result exit-code through configmap...!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuite.name, "0")

		g.By("Verify if pod generated for tainted node.. !!!\n")
		assertCoPodNumerEqualNodeNumber(oc, subD.namespace, "node-role.kubernetes.io/wscan=", csuite.scanname)

		g.By("ocp-33609 The compliance scan performed on tained node successfully.. !!!\n")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:pdhamdhe-High-33610-Verify the tolerations could work for compliancescan [Disruptive]", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			cscanD = complianceScanDescription{
				name:         "worker-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				key:          "key1",
				value:        "value1",
				operator:     "Equal",
				nodeSelector: "wscan",
				debug:        true,
				template:     cscantaintTemplate,
			}
			cscan = complianceScanDescription{
				name:         "worker-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				key:          "key1",
				value:        "",
				operator:     "Exists",
				nodeSelector: "wscan",
				debug:        true,
				template:     cscantaintTemplate,
			}
		)

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Label and set taint value to one worker node.. !!!\n")
		nodeName := getOneWorkerNodeName(oc)
		defer func() {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.taints}").Output()
			if strings.Contains(output, "value1") {
				taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")
			}
			output1, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.taints[0].key}").Output()
			if strings.Contains(output1, "key1") {
				taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule-")
			}
			output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.labels.taint}").Output()
			if strings.Contains(output2, "true") {
				labelTaintNode(oc, "node", nodeName, "taint-")
			}
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancescan", cscanD.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancescan", cscan.name, "-n", subD.namespace, "--ignore-not-found").Execute()
		}()
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule")
		labelTaintNode(oc, "node", nodeName, "taint=true")

		cscanD.namespace = subD.namespace
		g.By("Create compliancescan.. !!!\n")
		cscanD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceScan name and result.. !!!\n")
		subD.complianceScanName(oc, cscanD.name)
		subD.getComplianceScanResult(oc, cscanD.name, "COMPLIANT")

		g.By("Check complianceScan result exit-code through configmap.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanD.name, "0")

		g.By("Verify if scan pod generated for tainted node.. !!!\n")
		assertCoPodNumerEqualNodeNumber(oc, subD.namespace, "node-role.kubernetes.io/wscan=", cscanD.name)

		g.By("Remove compliancescan object and recover tainted worker node.. !!!\n")
		cleanupObjects(oc, objectTableRef{"compliancescan", subD.namespace, cscanD.name})
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")

		g.By("Set taint to worker node without value.. !!!\n")
		taintNode(oc, "taint", "node", nodeName, "key1=:NoSchedule")

		cscan.namespace = subD.namespace
		g.By("Create compliancescan.. !!!\n")
		cscan.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscan.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceScan name and result.. !!!\n")
		subD.complianceScanName(oc, cscan.name)
		subD.getComplianceScanResult(oc, cscan.name, "COMPLIANT")

		g.By("Check complianceScan result exit-code through configmap...!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscan.name, "0")

		g.By("Verify if the scan pod generated for tainted node...!!!\n")
		assertCoPodNumerEqualNodeNumber(oc, subD.namespace, "node-role.kubernetes.io/wscan=", cscan.name)

		g.By("ocp-33610 The compliance scan performed on tained node successfully.. !!!\n")

	})

	// author: pdhamdhe@redhat.com
	g.It("StagerunBoth-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:pdhamdhe-Critical-28949-The complianceSuite and ComplianeScan perform scan using Platform scan type", func() {

		var (
			csuiteD = complianceSuiteDescription{
				name:         "platform-compliancesuite" + getRandomString(),
				namespace:    "",
				scanType:     "platform",
				scanname:     "platform-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				template:     csuiteTemplate,
			}
			cscanMD = complianceScanDescription{
				name:         "platform-new-scan" + getRandomString(),
				namespace:    "",
				scanType:     "platform",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				template:     cscanTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
			objectTableRef{"compliancescan", subD.namespace, cscanMD.name})

		csuiteD.namespace = subD.namespace
		g.By("Create platform-compliancesuite.. !!!\n")
		csuiteD.create(oc)

		cscanMD.namespace = subD.namespace
		g.By("Create platform-new-scan.. !!!\n")
		cscanMD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check platform-scan pod.. !!!\n")
		subD.scanPodName(oc, csuiteD.scanname+"-api-checks-pod")

		g.By("Check platform-new-scan pod.. !!!\n")
		subD.scanPodName(oc, cscanMD.name+"-api-checks-pod")

		g.By("Check platform scan pods status.. !!! \n")
		subD.scanPodStatusWithLabel(oc, cscanMD.name, "Succeeded")

		g.By("Check platform-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")

		g.By("Check platform-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, csuiteD.name, "2")

		g.By("Check platform-new-scan name and result..!!!\n")
		subD.complianceScanName(oc, cscanMD.name)
		subD.getComplianceScanResult(oc, cscanMD.name, "NON-COMPLIANT")

		g.By("Check platform-new-scan result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, cscanMD.name, "2")

		g.By("The ocp-28949 complianceScan for platform has performed successfully ..!!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:pdhamdhe-Critical-36988-The ComplianceScan could be triggered for cis profile for platform scanType", func() {

		var (
			cscanMD = complianceScanDescription{
				name:         "platform-new-scan" + getRandomString(),
				namespace:    "",
				scanType:     "platform",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				template:     cscanTemplate,
			}
		)

		defer cleanupObjects(oc, objectTableRef{"compliancescan", subD.namespace, cscanMD.name})

		cscanMD.namespace = subD.namespace
		g.By("Create platform-new-scan.. !!!\n")
		cscanMD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check platform-new-scan pod.. !!!\n")
		subD.scanPodName(oc, cscanMD.name+"-api-checks-pod")

		g.By("Check platform scan pods status.. !!! \n")
		subD.scanPodStatusWithLabel(oc, cscanMD.name, "Succeeded")

		g.By("Check platform-new-scan name and result..!!!\n")
		subD.complianceScanName(oc, cscanMD.name)
		subD.getComplianceScanResult(oc, cscanMD.name, "NON-COMPLIANT")

		g.By("Check platform-new-scan result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanMD.name, "2")

		g.By("The ocp-36988 complianceScan for platform has performed successfully ..!!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:pdhamdhe-Critical-36990-The ComplianceSuite could be triggered for cis profiles for platform scanType", func() {

		var (
			csuiteD = complianceSuiteDescription{
				name:         "platform-compliancesuite" + getRandomString(),
				namespace:    "",
				scanType:     "platform",
				scanname:     "platform-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_cis",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				template:     csuiteTemplate,
			}
		)

		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})

		csuiteD.namespace = subD.namespace
		g.By("Create platform-compliancesuite.. !!!\n")
		csuiteD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check platform-scan pod.. !!!\n")
		subD.scanPodName(oc, csuiteD.scanname+"-api-checks-pod")

		g.By("Check platform scan pods status.. !!! \n")
		subD.scanPodStatusWithLabel(oc, csuiteD.scanname, "Succeeded")

		g.By("Check platform-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")

		g.By("Check platform-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "2")

		g.By(" ocp-36990 The complianceSuite object successfully performed platform scan for cis profile ..!!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Critical-37063-The ComplianceSuite could be triggered for cis profiles for node scanType", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_cis",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				nodeSelector: "wscan",
				template:     csuiteTemplate,
			}

			csuiteMD = complianceSuiteDescription{
				name:         "master-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "master-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_cis-node",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				nodeSelector: "master",
				template:     csuiteTemplate,
			}

			csuiteRD = complianceSuiteDescription{
				name:         "cis-node-" + getRandomString(),
				namespace:    "",
				scanname:     "cis-node-" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_cis-node",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				template:     csuitenodeTemplate,
			}
		)

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Remove all suites in case the test case failed in the middle !!!\n")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancesuite", csuiteD.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancesuite", csuiteMD.name, "-n", subD.namespace, "--ignore-not-found").Execute()
		}()

		csuiteD.namespace = subD.namespace
		g.By("Create worker-compliancesuite !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check worker-compliancesuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")

		g.By("Check worker-compliancesuite result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "2")

		g.By("Remove worker-compliancesuite object.. !!!\n")
		cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})

		csuiteMD.namespace = subD.namespace
		g.By("Create master-compliancesuite !!!\n")
		csuiteMD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check master-compliancesuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteMD.name)
		subD.complianceSuiteResult(oc, csuiteMD.name, "NON-COMPLIANT")

		g.By("Check master-compliancesuite result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteMD.name, "2")

		g.By("Verify compliance scan result compliancecheckresult through label ...!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			csuiteMD.scanname + "-etcd-unique-ca", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Remove master-compliancesuite object.. !!!\n")
		cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteMD.name})

		csuiteRD.namespace = subD.namespace
		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteRD.name})
		g.By("Create rhcos-compliancesuite !!!\n")
		csuiteRD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteRD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check master-compliancesuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteRD.name)
		subD.complianceSuiteResult(oc, csuiteRD.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Check master-compliancesuite result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteRD.name, "2")

		g.By("Verify compliance scan result compliancecheckresult through label ...!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteRD.scanname+"-file-owner-ovs-conf-db", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/check-status=PASS,compliance.openshift.io/scan-name=" + csuiteRD.scanname, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			csuiteRD.scanname + "-file-owner-ovs-conf-db", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By(" ocp-37063 The complianceSuite object successfully triggered scan for cis node profile.. !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:pdhamdhe-NonPreRelease-Longduration-High-32120-The ComplianceSuite performs schedule scan for Platform scan type [Slow]", func() {
		var (
			csuiteD = complianceSuiteDescription{
				name:         "platform-compliancesuite" + getRandomString(),
				namespace:    "",
				schedule:     "*/4 * * * *",
				scanType:     "platform",
				scanname:     "platform-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				debug:        true,
				template:     csuiteTemplate,
			}
		)

		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})

		csuiteD.namespace = subD.namespace
		g.By("Create platform-compliancesuite.. !!!\n")
		csuiteD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check platform-scan pod.. !!!\n")
		subD.scanPodName(oc, csuiteD.scanname+"-api-checks-pod")

		g.By("Check platform scan pods status.. !!! \n")
		subD.scanPodStatusWithLabel(oc, csuiteD.scanname, "Succeeded")
		g.By("Check platform-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")
		g.By("Check platform-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "2")

		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.name+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "*/4 * * * *", ok, []string{"cronjob", csuiteD.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.schedule}"}).check(oc)

		checkComplianceSuiteStatus(oc, csuiteD.name, subD.namespace, "RUNNING")
		newCheck("expect", asAdmin, withoutNamespace, contain, "1", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[*].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"pod", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check platform-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "2")

		g.By("The ocp-32120 The complianceScan object performed Platform schedule scan successfully.. !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-High-33418-Medium-44062-The ComplianceSuite performs the schedule scan through cron job and also verify the suitererunner resources are doubled [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				schedule:     "*/4 * * * *",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector: "wscan",
				debug:        true,
				template:     csuiteTemplate,
			}
		)

		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create worker-compliancesuite.. !!!\n")
		csuiteD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.name+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "*/4 * * * *", ok, []string{"cronjob", csuiteD.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.schedule}"}).check(oc)

		checkComplianceSuiteStatus(oc, csuiteD.name, subD.namespace, "RUNNING")
		newCheck("expect", asAdmin, withoutNamespace, contain, "1", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[*].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"pod", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Verify the suitererunner resource requests and limits doubled through jobs.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "{\"cpu\":\"50m\",\"memory\":\"100Mi\"}", ok, []string{"jobs", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].spec.template.spec.containers[0].resources.limits}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "{\"cpu\":\"10m\",\"memory\":\"20Mi\"}", ok, []string{"jobs", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].spec.template.spec.containers[0].resources.requests}"}).check(oc)

		g.By("Verify the suitererunner resource requests and limits doubled through pods.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "{\"cpu\":\"50m\",\"memory\":\"100Mi\"}", ok, []string{"pods", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].spec.containers[0].resources.limits}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "{\"cpu\":\"10m\",\"memory\":\"20Mi\"}", ok, []string{"pods", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].spec.containers[0].resources.requests}"}).check(oc)

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		g.By("ocp-33418 ocp-44062 The ComplianceSuite object performed schedule scan and verify the suitererunner resources requests & limit successfully.. !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:xiyuan-NonPreRelease-Longduration-Medium-33456-The Compliance-Operator edits the scheduled cron job to scan from ComplianceSuite [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "example-compliancesuite1" + getRandomString(),
				namespace:    "",
				schedule:     "*/4 * * * *",
				scanname:     "worker-scan",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector: "wscan",
				debug:        true,
				template:     csuiteTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
		)

		// adding label to rhcos worker node to skip non-rhcos worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create worker-compliancesuite.. !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.name+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "*/4 * * * *", ok, []string{"cronjob", csuiteD.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.schedule}"}).check(oc)

		g.By("edit schedule by patch.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"schedule\":\"*/4 * * * *\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "compliancesuites", csuiteD.name, "-n", csuiteD.namespace, "--type", "merge", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, "*/4 * * * *", ok, []string{"cronjob", csuiteD.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.schedule}"}).check(oc)

		checkComplianceSuiteStatus(oc, csuiteD.name, subD.namespace, "RUNNING")
		newCheck("expect", asAdmin, withoutNamespace, contain, "1", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[*].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"pod", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		g.By("The ocp-33456-The Compliance-Operator could edit scheduled cron job to scan from ComplianceSuite successfully.. !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-NonPreRelease-Longduration-High-33453-High-54066-The Compliance Operator rotates the raw scan results [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				schedule:     "*/4 * * * *",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector: "wscan",
				rotation:     2,
				debug:        true,
				template:     csuiteTemplate,
			}
			pvExtract = pvExtractDescription{
				name:      "pv-extract" + getRandomString(),
				scanname:  "",
				namespace: subD.namespace,
				template:  pvExtractPodTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
			objectTableRef{"pod", subD.namespace, pvExtract.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Create a compliancesuite anc check the status.. !!!\n")
		csuiteD.namespace = subD.namespace
		pvExtract.scanname = csuiteD.scanname
		csuiteD.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Processing\")].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Processing", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].reason}"}).check(oc)
		timestampRunning, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancesuite", csuiteD.name, "-n", subD.namespace,
			"-o=jsonpath={.status.conditions[?(@.type==\"Processing\")].lastTransitionTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NotRunning", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Processing\")].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Done", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].reason}"}).check(oc)
		timestampDone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancesuite", csuiteD.name, "-n", subD.namespace,
			"-o=jsonpath={.status.conditions[?(@.type==\"Processing\")].lastTransitionTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(timestampDone).ShouldNot(o.Equal(timestampRunning))

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		//Verifying rotation policy and cronjob
		newCheck("expect", asAdmin, withoutNamespace, contain, "2", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.spec.scans[0].rawResultStorage.rotation}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.name+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "*/4 * * * *", ok, []string{"cronjob", csuiteD.name + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.schedule}"}).check(oc)

		//Second round of scan and check
		checkComplianceSuiteStatus(oc, csuiteD.name, subD.namespace, "RUNNING")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Processing\")].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Processing", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].reason}"}).check(oc)
		timestampReRunning, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancesuite", csuiteD.name, "-n", subD.namespace,
			"-ojsonpath={.status.conditions[?(@.type==\"Processing\")].lastTransitionTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(timestampReRunning).ShouldNot(o.Equal(timestampDone))
		newCheck("expect", asAdmin, withoutNamespace, contain, "1", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[*].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"pod", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NotRunning", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Processing\")].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Done", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].reason}"}).check(oc)
		timestampRerunDone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancesuite", csuiteD.name, "-n", subD.namespace,
			"-o=jsonpath={.status.conditions[?(@.type==\"Processing\")].lastTransitionTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(timestampRerunDone).ShouldNot(o.Equal(timestampReRunning))

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		//Third round of scan and check
		checkComplianceSuiteStatus(oc, csuiteD.name, subD.namespace, "RUNNING")
		newCheck("expect", asAdmin, withoutNamespace, contain, "2", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[*].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"pod", "-l", "compliance.openshift.io/suite=" + csuiteD.name + ",workload=suitererunner", "-n",
			subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		g.By("Check worker-compliancesuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		g.By("Create pv-extract pod and verify arfReport result directories.. !!!\n")
		pvExtract.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pod", pvExtract.name, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		commands := []string{"-n", subD.namespace, "exec", "pod/" + pvExtract.name, "--", "ls", "/workers-scan-results"}
		arfReportDir, err := oc.AsAdmin().WithoutNamespace().Run(commands...).Args().Output()
		e2e.Logf("The arfReport result dir:\n%v", arfReportDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(arfReportDir, "0") && (strings.Contains(arfReportDir, "1") && strings.Contains(arfReportDir, "2")) {
			g.By("The ocp-33453 The ComplianceSuite object performed schedule scan and rotates the raw scan results successfully.. !!!\n")
		}
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-High-33660-Verify the differences in nodes from the same role could be handled [Serial]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_direct_root_logins",
				nodeSelector: "wscan",
				template:     csuiteTemplate,
			}
		)

		nodeName := getOneRhcosWorkerNodeName(oc)
		defer func() {
			cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})
			_, _ = exutil.DebugNodeWithOptionsAndChroot(oc, nodeName, []string{"--to-namespace=" + subD.namespace}, "rm", "-rf", "/etc/securetty")
		}()
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, nodeName, []string{"--to-namespace=" + subD.namespace}, "touch", "/etc/securetty")
		o.Expect(err).NotTo(o.HaveOccurred())
		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create compliancesuite !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "INCONSISTENT")

		g.By("Verify compliance scan result compliancecheckresult through label ...!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-no-direct-root-logins", ok, []string{"compliancecheckresult",
			"-l", "compliance.openshift.io/inconsistent-check,compliance.openshift.io/suite=" + csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "INCONSISTENT", ok, []string{"compliancecheckresult",
			csuiteD.scanname + "-no-direct-root-logins", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ocp-33660 The compliance scan successfully handled the differences from the same role nodes ...!!!\n")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:pdhamdhe-Medium-32814-High-45729-The compliance operator by default creates ProfileBundles and profiles", func() {
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
		// The supported profiles are different for different archs.
		// Details seen from: https://docs.openshift.com/container-platform/latest/security/compliance_operator/co-scans/compliance-operator-supported-profiles.html#compliance-supported-profiles_compliance-operator-supported-profiles
		profilesByArch := map[architecture.Architecture][]string{
			architecture.S390X: {
				"ocp4-cis",
				"ocp4-cis-1-4",
				"ocp4-cis-1-5",
				"ocp4-cis-node",
				"ocp4-cis-node-1-4",
				"ocp4-cis-node-1-5",
				"ocp4-moderate",
				"ocp4-moderate-node",
				"ocp4-moderate-node-rev-4",
				"ocp4-moderate-rev-4",
				"ocp4-pci-dss",
				"ocp4-pci-dss-3-2",
				"ocp4-pci-dss-4-0",
				"ocp4-pci-dss-node",
				"ocp4-pci-dss-node-3-2",
				"ocp4-pci-dss-node-4-0"},
			architecture.PPC64LE: {
				"ocp4-cis",
				"ocp4-cis-1-4",
				"ocp4-cis-1-5",
				"ocp4-cis-node",
				"ocp4-cis-node-1-4",
				"ocp4-cis-node-1-5",
				"ocp4-moderate",
				"ocp4-moderate-node",
				"ocp4-moderate-node-rev-4",
				"ocp4-moderate-rev-4",
				"ocp4-pci-dss",
				"ocp4-pci-dss-3-2",
				"ocp4-pci-dss-4-0",
				"ocp4-pci-dss-node",
				"ocp4-pci-dss-node-3-2",
				"ocp4-pci-dss-node-4-0"},
			architecture.AMD64: {
				"ocp4-cis",
				"ocp4-cis-1-4",
				"ocp4-cis-1-5",
				"ocp4-cis-node",
				"ocp4-cis-node-1-4",
				"ocp4-cis-node-1-5",
				"ocp4-e8",
				"ocp4-high",
				"ocp4-high-node",
				"ocp4-high-node-rev-4",
				"ocp4-high-rev-4",
				"ocp4-moderate",
				"ocp4-moderate-node",
				"ocp4-moderate-node-rev-4",
				"ocp4-moderate-rev-4",
				"ocp4-nerc-cip",
				"ocp4-nerc-cip-node",
				"ocp4-pci-dss",
				"ocp4-pci-dss-3-2",
				"ocp4-pci-dss-4-0",
				"ocp4-pci-dss-node",
				"ocp4-pci-dss-node-3-2",
				"ocp4-pci-dss-node-4-0",
				"ocp4-stig",
				"ocp4-stig-node-v2r1",
				"ocp4-stig-node",
				"ocp4-stig-node-v1r1",
				"ocp4-stig-node-v2r1",
				"ocp4-stig-v1r1",
				"rhcos4-e8",
				"rhcos4-high",
				"rhcos4-high-rev-4",
				"rhcos4-moderate",
				"rhcos4-moderate-rev-4",
				"rhcos4-nerc-cip",
				"rhcos4-stig",
				"rhcos4-stig-v1r1",
				"rhcos4-stig-v2r1"},
		}

		if profiles, ok := profilesByArch[arch]; ok {
			for _, profile := range profiles {
				subD.getProfileName(oc, profile)
			}
		} else {
			e2e.Logf("Architecture %s is not supported", arch.String())
		}

		g.By("The Compliance Operator by default created ProfileBundles and profiles are verified successfully.. !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Medium-33431-Verify compliance check result shows in ComplianceCheckResult label for compliancesuite", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector: "wscan",
				template:     csuiteTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})
		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create compliancesuite !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")

		g.By("Check complianceScan result exit-code through configmap...!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "0")

		g.By("Verify compliance scan result compliancecheckresult through label ...!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-no-netrc-files", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/check-severity=medium", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-no-netrc-files", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/check-status=PASS", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-no-netrc-files", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/scan-name=" + csuiteD.scanname, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.scanname+"-no-netrc-files", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/suite=" + csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("ocp-33431 The compliance scan result verified through ComplianceCheckResult label successfully....!!!\n")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Medium-33435-Verify the compliance scan result shows in ComplianceCheckResult label for compliancescan", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			cscanD = complianceScanDescription{
				name:         "rhcos-scan" + getRandomString(),
				namespace:    "",
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
				nodeSelector: "wscan",
				template:     cscanTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"compliancescan", subD.namespace, cscanD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		cscanD.namespace = subD.namespace
		g.By("Create compliancescan !!!\n")
		cscanD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result..!!!\n")
		subD.complianceScanName(oc, "rhcos-scan")
		subD.getComplianceScanResult(oc, cscanD.name, "NON-COMPLIANT")

		g.By("Check complianceScan result exit-code through configmap...!!!\n")
		subD.getScanExitCodeFromConfigmapWithScanName(oc, cscanD.name, "2")

		g.By("Verify compliance scan result compliancecheckresult through label ...!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, cscanD.name+"-no-empty-passwords", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/check-severity=high", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, cscanD.name+"-no-empty-passwords", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/check-status=FAIL", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, cscanD.name+"-no-empty-passwords", ok, []string{"compliancecheckresult",
			"--selector=compliance.openshift.io/scan-name=" + cscanD.name, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("ocp-33435 The compliance scan result verified through ComplianceCheckResult label successfully....!!!\n")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-Medium-33449-The compliance-operator raw results store in ARF format on a PVC", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
				nodeSelector: "wscan",
				template:     csuiteTemplate,
			}
			pvExtract = pvExtractDescription{
				name:      "pv-extract" + getRandomString(),
				scanname:  "",
				namespace: subD.namespace,
				template:  pvExtractPodTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
			objectTableRef{"pod", subD.namespace, pvExtract.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		csuiteD.namespace = subD.namespace
		g.By("Create worker-compliancesuite !!!\n")
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")

		g.By("Check worker-compliancesuite result through exit-code ..!!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "2")

		g.By("Create pv-extract pod and check status.. !!!\n")
		pvExtract.scanname = csuiteD.scanname
		pvExtract.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pod", pvExtract.name, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check ARF report generates in xml format.. !!!\n")
		subD.getARFreportFromPVC(oc, pvExtract.name, ".xml.bzip2")

		g.By("The ocp-33449 complianceScan raw result successfully stored in ARF format on the PVC... !!!!\n")

	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:xiyuan-Medium-37171-Check compliancesuite status when there are multiple rhcos4 profiles added in scansettingbinding object [Serial][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var ssb = scanSettingBindingDescription{
			name:            "rhcos4" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "rhcos4-e8",
			profilename2:    "rhcos4-moderate",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		g.By("Check default profiles name rhcos4-e8 .. !!!\n")
		subD.getProfileName(oc, ssb.profilename1)
		g.By("Check default profiles name rhcos4-moderate .. !!!\n")
		subD.getProfileName(oc, ssb.profilename2)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", ssb.namespace, ssb.name})
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssb.name, ssb.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")
		g.By("Check complianceSuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("ocp-37171 Check compliancesuite status when there are multiple rhcos4 profiles added in scansettingbinding object successfully... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:xiyuan-High-37084-The ComplianceSuite generates through ScanSettingBinding CR with tailored cis profile [Serial]", func() {
		var (
			tp = tailoredProfileWithoutVarDescription{
				name:         "ocp4-cis-custom",
				namespace:    "",
				extends:      "ocp4-cis",
				title:        "My little profile",
				description:  "cis profile rules",
				enrulename1:  "ocp4-scc-limit-root-containers",
				rationale1:   "None",
				enrulename2:  "ocp4-api-server-insecure-bind-address",
				rationale2:   "Platform",
				disrulename1: "ocp4-api-server-encryption-provider-cipher",
				drationale1:  "Platform",
				disrulename2: "ocp4-scc-drop-container-capabilities",
				drationale2:  "None",
				template:     tprofileWithoutVarTemplate,
			}
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "myss" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				roles2:                 "worker",
				rotation:               5,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "my-companys-compliance-requirements" + getRandomString(),
				namespace:       "",
				profilekind1:    "TailoredProfile",
				profilename1:    "ocp4-cis-custom",
				profilename2:    "ocp4-cis-node",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name},
			objectTableRef{"tailoredprofile", subD.namespace, tp.name})

		g.By("Check default profiles name ocp4-cis and ocp4-cis-node.. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")
		subD.getProfileName(oc, "ocp4-cis-node")

		tp.namespace = subD.namespace
		g.By("Create tailoredprofile !!!\n")
		tp.create(oc)
		subD.getTailoredProfileNameandStatus(oc, tp.name)

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Check complianceSuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("ocp-37084 The ComplianceSuite generates through ScanSettingBinding CR with tailored cis profile successfully... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:pdhamdhe-High-34928-access modes and Storage class are configurable through ComplianceSuite and ComplianceScan", func() {
		SkipForIBMCloud(oc)
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:             "worker-compliancesuite" + getRandomString(),
				namespace:        "",
				scanname:         "worker-scan" + getRandomString(),
				profile:          "xccdf_org.ssgproject.content_profile_moderate",
				content:          "ssg-rhcos4-ds.xml",
				contentImage:     relatedImageProfile,
				rule:             "xccdf_org.ssgproject.content_rule_no_netrc_files",
				nodeSelector:     "worker",
				storageClassName: "gold",
				pvAccessModes:    "ReadWriteOnce",
				template:         csuiteSCTemplate,
			}
			cscanMD = complianceScanDescription{
				name:             "master-scan" + getRandomString(),
				namespace:        "",
				profile:          "xccdf_org.ssgproject.content_profile_e8",
				content:          "ssg-rhcos4-ds.xml",
				contentImage:     relatedImageProfile,
				rule:             "xccdf_org.ssgproject.content_rule_accounts_no_uid_except_zero",
				nodeSelector:     "master",
				storageClassName: "gold",
				pvAccessModes:    "ReadWriteOnce",
				template:         cscanSCTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name},
			objectTableRef{"compliancescan", subD.namespace, cscanMD.name},
			objectTableRef{"storageclass", subD.namespace, "gold"})

		g.By("Get the default storageClass provisioner & volumeBindingMode from cluster .. !!!\n")
		storageClass.name = "gold"
		scName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o=jsonpath={.items[?(@.metadata.annotations.storageclass\\.kubernetes\\.io/is-default-class==\"true\")].metadata.name}", "-n", oc.Namespace()).Output()
		provisioner, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", string(scName), "-n", subD.namespace, "-o=jsonpath={.provisioner}").Output()
		storageClass.provisioner = provisioner
		o.Expect(err).NotTo(o.HaveOccurred())
		storageClass.reclaimPolicy = "Delete"
		volumeBindingMode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", string(scName), "-n", subD.namespace, "-o=jsonpath={.volumeBindingMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		storageClass.volumeBindingMode = volumeBindingMode
		storageClass.create(oc)

		csuiteD.namespace = subD.namespace
		g.By("Create worker-compliancesuite.. !!!\n")
		csuiteD.create(oc)

		cscanMD.namespace = subD.namespace
		g.By("Create master-scan.. !!!\n")
		cscanMD.create(oc)

		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancescan", cscanMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check worker-compliancesuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT INCONSISTENT")

		g.By("Check pvc name and storage size for worker-scan.. !!!\n")
		subD.getPVCName(oc, csuiteD.scanname)
		newCheck("expect", asAdmin, withoutNamespace, contain, "gold", ok, []string{"pvc", csuiteD.scanname, "-n",
			subD.namespace, "-o=jsonpath={.spec.storageClassName}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ReadWriteOnce", ok, []string{"pvc", csuiteD.scanname, "-n",
			subD.namespace, "-o=jsonpath={.status.accessModes[]}"}).check(oc)

		g.By("Check master-scan name and result..!!!\n")
		subD.complianceScanName(oc, cscanMD.name)
		subD.getComplianceScanResult(oc, cscanMD.name, "COMPLIANT")

		g.By("Check pvc name and storage size for master-scan ..!!!\n")
		subD.getPVCName(oc, cscanMD.name)
		newCheck("expect", asAdmin, withoutNamespace, contain, "gold", ok, []string{"pvc", cscanMD.name, "-n",
			subD.namespace, "-o=jsonpath={.spec.storageClassName}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ReadWriteOnce", ok, []string{"pvc", cscanMD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.accessModes[]}"}).check(oc)

		g.By("ocp-34928 Storage class and access modes are successfully configurable through ComplianceSuite and ComplianceScan ..!!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:xiyuan-Medium-40372-Use a separate SA for resultserver", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var csuiteMD = complianceSuiteDescription{
			name:         "master-compliancesuite" + getRandomString(),
			namespace:    "",
			scanname:     "master-scan" + getRandomString(),
			profile:      "xccdf_org.ssgproject.content_profile_moderate",
			content:      "ssg-rhcos4-ds.xml",
			contentImage: relatedImageProfile,
			rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
			nodeSelector: "master",
			debug:        true,
			template:     csuiteTemplate,
		}

		fsGroup, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", subD.namespace, "-lname=compliance-operator", "-o=jsonpath={.items[0].spec.securityContext.fsGroup}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		securityContextLevel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", subD.namespace, "-lname=compliance-operator", "-o=jsonpath={.items[0].spec.securityContext.seLinuxOptions.level}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		keyword := fmt.Sprintf(`{"fsGroup":%s,"runAsNonRoot":true,"runAsUser":%s,"seLinuxOptions":{"level":%s},"seccompProfile":{"type":"RuntimeDefault"}}`, fsGroup, fsGroup, securityContextLevel)

		// These are special steps to overcome problem which are discussed in [1] so that namespace should not stuck in 'Terminating' state
		// [1] https://bugzilla.redhat.com/show_bug.cgi?id=1858186
		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteMD.name})

		g.By("create compliancesuite")
		csuiteMD.namespace = subD.namespace
		csuiteMD.create(oc)

		g.By("check the scc and securityContext for the rs pod")
		label := "compliance.openshift.io/scan-name=" + csuiteMD.scanname + ",workload=resultserver"
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"pod", "-l", label, "-n", subD.namespace}).check(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[0].spec.securityContext}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), keyword))
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:xiyuan-Medium-40280-The infrastructure feature should show the Compliance operator when the disconnected filter gets applied", func() {
		g.By("check the infrastructure-features for csv!!!\n")
		csvName, err := getResource(oc, asAdmin, withoutNamespace, "csv", "-n", subD.namespace, "-l", "operators.coreos.com/compliance-operator.openshift-compliance=", "-o=jsonpath={.items[0].metadata.name}")
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "[\"disconnected\", \"fips\", \"proxy-aware\"]", ok, []string{"csv",
			csvName, "-n", subD.namespace, "-o=jsonpath={.metadata.annotations.operators\\.openshift\\.io/infrastructure-features}"}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:xiyuan-Medium-41769-The compliance operator could get HTTP_PROXY and HTTPS_PROXY environment from OpenShift has global proxy settings	", func() {
		g.By("Get the httpPoxy and httpsProxy info!!!\n")
		httpProxy, _ := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-n", subD.namespace, "-o=jsonpath={.spec.httpProxy}")
		httpsProxy, _ := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-n", subD.namespace, "-o=jsonpath={.spec.httpsProxy}")

		if len(httpProxy) == 0 && len(httpsProxy) == 0 {
			g.Skip("Skip for non-proxy cluster! This case intentionally runs nothing!")
		} else {
			g.By("check the proxy info for the compliance-operator deployment!!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "\"name\":\"HTTP_PROXY\",\"value\":\""+httpProxy+"\"", ok, []string{"deployment", "compliance-operator",
				"-n", subD.namespace, "-o=jsonpath={.spec.template.spec.containers[0].env}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "\"name\":\"HTTPS_PROXY\",\"value\":\""+httpsProxy+"\"", ok, []string{"deployment", "compliance-operator",
				"-n", subD.namespace, "-o=jsonpath={.spec.template.spec.containers[0].env}"}).check(oc)

			g.By("create a compliancesuite!!!\n")
			var csuiteMD = complianceSuiteDescription{
				name:         "master-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "master-scan",
				profile:      "xccdf_org.ssgproject.content_profile_cis-node",
				content:      "ssg-ocp4-ds.xml",
				contentImage: relatedImageProfile,
				nodeSelector: "master",
				template:     csuiteTemplate,
			}
			defer cleanupObjects(oc,
				objectTableRef{"compliancesuite", subD.namespace, csuiteMD.name})
			csuiteMD.namespace = subD.namespace
			g.By("Create master-compliancesuite !!!\n")
			csuiteMD.create(oc)
			assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteMD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

			g.By("check the https proxy in the configmap!!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, httpsProxy, ok, []string{"cm", csuiteMD.scanname + "-openscap-env-map", "-n",
				subD.namespace, "-o=jsonpath={.data.HTTPS_PROXY}"}).check(oc)
		}
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:xiyuan-High-33859-Verify if the profileparser enables to get content updates when the image digest updated [Serial][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			pb = profileBundleDescription{
				name:         "test1",
				namespace:    "",
				contentimage: "quay.io/openshifttest/ocp4-openscap-content@sha256:392b0a67e4386a7450b0bb0c9353231563b7ab76056d215f10e6f5ffe0a2cbad",
				contentfile:  "ssg-rhcos4-ds.xml",
				template:     profilebundleTemplate,
			}
			tprofileD = tailoredProfileDescription{
				name:         "example-tailoredprofile",
				namespace:    "",
				extends:      "test1-moderate",
				enrulename1:  "test1-account-disable-post-pw-expiration",
				disrulename1: "test1-account-unique-name",
				disrulename2: "test1-account-use-centralized-automated-auth",
				varname:      "test1-var-selinux-state",
				value:        "permissive",
				template:     tprofileTemplate,
			}
			tprofileD2 = tailoredProfileDescription{
				name:         "example-tailoredprofile2",
				namespace:    "",
				extends:      "test1-moderate",
				enrulename1:  "test1-wireless-disable-in-bios",
				disrulename1: "test1-account-unique-name",
				disrulename2: "test1-account-use-centralized-automated-auth",
				varname:      "test1-var-selinux-state",
				value:        "permissive",
				template:     tprofileTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"profilebundle", subD.namespace, pb.name},
			objectTableRef{"tailoredprofile", subD.namespace, tprofileD.name},
			objectTableRef{"tailoredprofile", subD.namespace, tprofileD2.name})

		g.By("create profilebundle!!!\n")
		pb.namespace = subD.namespace
		pb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, pb.name, ok, []string{"profilebundle", pb.name, "-n", pb.namespace,
			"-o=jsonpath={.metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", pb.name, "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "test1-chronyd-no-chronyc-network", ok, []string{"rules", "test1-chronyd-no-chronyc-network",
			"-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "test1-chronyd-client-only", ok, []string{"rules", "test1-chronyd-client-only",
			"-n", subD.namespace}).check(oc)

		g.By("check the default profilebundle !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", "ocp4", "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", "rhcos4", "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		g.By("Create tailoredprofile !!!\n")
		tprofileD.namespace = subD.namespace
		tprofileD.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", tprofileD.name, "-n", tprofileD.namespace,
			"-o=jsonpath={.status.state}"}).check(oc)

		g.By("Patch the profilebundle with a different image !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"contentImage\":\"quay.io/openshifttest/ocp4-openscap-content@sha256:3778c668f462424552c15c6c175704b64270ea06183fd034aa264736f1ec45a9\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "profilebundle", pb.name, "-n", pb.namespace, "--type", "merge", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Pending", ok, []string{"profilebundle", pb.name, "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", pb.name, "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"rules", "test1-chronyd-no-chronyc-network", "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "test1-chronyd-client-only", ok, []string{"rules", "test1-chronyd-client-only",
			"-n", subD.namespace}).check(oc)

		g.By("Create tailoredprofile !!!\n")
		tprofileD2.namespace = subD.namespace
		tprofileD2.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", tprofileD2.name, "-n", tprofileD.namespace,
			"-o=jsonpath={.status.state}"}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:xiyuan-Medium-33578-Verify if the profileparser enables to get content updates when add a new ProfileBundle [Serial][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			pb = profileBundleDescription{
				name:         "test1",
				namespace:    "",
				contentimage: "quay.io/openshifttest/ocp4-openscap-content@sha256:392b0a67e4386a7450b0bb0c9353231563b7ab76056d215f10e6f5ffe0a2cbad",
				contentfile:  "ssg-rhcos4-ds.xml",
				template:     profilebundleTemplate,
			}
			pb2 = profileBundleDescription{
				name:         "test2",
				namespace:    "",
				contentimage: "quay.io/openshifttest/ocp4-openscap-content@sha256:3778c668f462424552c15c6c175704b64270ea06183fd034aa264736f1ec45a9",
				contentfile:  "ssg-rhcos4-ds.xml",
				template:     profilebundleTemplate,
			}
			tprofileD = tailoredProfileDescription{
				name:         "example-tailoredprofile",
				namespace:    "",
				extends:      "test1-moderate",
				enrulename1:  "test1-account-disable-post-pw-expiration",
				disrulename1: "test1-account-unique-name",
				disrulename2: "test1-account-use-centralized-automated-auth",
				varname:      "test1-var-selinux-state",
				value:        "permissive",
				template:     tprofileTemplate,
			}
			tprofileD2 = tailoredProfileDescription{
				name:         "example-tailoredprofile2",
				namespace:    "",
				extends:      "test2-moderate",
				enrulename1:  "test2-wireless-disable-in-bios",
				disrulename1: "test2-account-unique-name",
				disrulename2: "test2-account-use-centralized-automated-auth",
				varname:      "test2-var-selinux-state",
				value:        "permissive",
				template:     tprofileTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"profilebundle", subD.namespace, pb.name},
			objectTableRef{"profilebundle", subD.namespace, pb2.name},
			objectTableRef{"tailoredprofile", subD.namespace, tprofileD.name},
			objectTableRef{"tailoredprofile", subD.namespace, tprofileD2.name})

		g.By("create profilebundle!!!\n")
		pb.namespace = subD.namespace
		pb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, pb.name, ok, []string{"profilebundle", pb.name, "-n", pb.namespace,
			"-o=jsonpath={.metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", pb.name, "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		g.By("check the default profilebundle !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", "ocp4", "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", "rhcos4", "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		g.By("Create tailoredprofile !!!\n")
		tprofileD.namespace = subD.namespace
		tprofileD.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", tprofileD.name, "-n", tprofileD.namespace,
			"-o=jsonpath={.status.state}"}).check(oc)

		g.By("Create another profilebundle with a different image !!!\n")
		pb2.namespace = subD.namespace
		pb2.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, pb2.name, ok, []string{"profilebundle", pb2.name, "-n", pb2.namespace,
			"-o=jsonpath={.metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", pb2.name, "-n", pb2.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		g.By("check the default profilebundle !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", "ocp4", "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Valid", ok, []string{"profilebundle", "rhcos4", "-n", pb.namespace,
			"-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		g.By("Create tailoredprofile !!!\n")
		tprofileD2.namespace = subD.namespace
		tprofileD2.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", tprofileD2.name, "-n", tprofileD.namespace,
			"-o=jsonpath={.status.state}"}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:xiyuan-High-33429-The Compliance Operator performs scan successfully on taint node without tolerations [Disruptive] [Slow]", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var csuiteD = complianceSuiteDescription{
			name:         "example-compliancesuite" + getRandomString(),
			namespace:    "",
			scanname:     "rhcos-scan" + getRandomString(),
			schedule:     "* 1 * * *",
			profile:      "xccdf_org.ssgproject.content_profile_moderate",
			content:      "ssg-rhcos4-ds.xml",
			contentImage: relatedImageProfile,
			rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
			nodeSelector: "wscan",
			size:         "2Gi",
			template:     csuiteTemplate,
		}

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Get one worker node.. !!!\n")
		nodeName := getOneRhcosWorkerNodeName(oc)

		g.By("cleanup !!!\n")
		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})
		defer func() {
			taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")
		}()

		g.By("Taint node and create compliancesuite.. !!!\n")
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule")
		csuiteD.namespace = subD.namespace
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancesuite", csuiteD.name, "-n",
			subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)

		g.By("Check complianceScan result exit-code through configmap.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, csuiteD.name, "2 unschedulable")
	})

	/* Disabling the test, it may be needed in future
	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-Author:xiyuan-Medium-40226-NOT APPLICABLE rule should report NOT APPLICABLE status in 'ComplianceCheckResult' instead of SKIP [Slow]", func() {
		var (
			ssb = scanSettingBindingDescription{
				name:            "cis-test",
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				profilename2:    "ocp4-cis-node",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			itName = g.CurrentSpecReport().FullText()
		)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[0].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace,  "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Check ComplianceSuite status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NOT-APPLICABLE", ok, []string{"compliancecheckresult", "ocp4-cis-node-worker-etcd-unique-ca", "-n", ssb.namespace,
			"-o=jsonpath={.status}"}).check(oc)

		g.By("Check the number of compliancecheckresult in NOT-APPLICABLE !!!\n")
		checkResourceNumber(oc, 37, "compliancecheckresult", "-l", "compliance.openshift.io/check-status=NOT-APPLICABLE", "--no-headers", "-n", subD.namespace)

		g.By("Check the warnings of NOT-APPLICABLE rules !!!\n")
		checkWarnings(oc, "rule is only applicable", "compliancecheckresult", "-l", "compliance.openshift.io/check-status=NOT-APPLICABLE", "--no-headers", "-n", subD.namespace)
	})*/

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ARO-ConnectedOnly-Author:pdhamdhe-High-41861-Verify fips mode checking rules are working as expected [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_enable_fips_mode",
				nodeSelector: "wscan",
				template:     csuiteTemplate,
			}

			csuiteMD = complianceSuiteDescription{
				name:         "master-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "master-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_enable_fips_mode",
				nodeSelector: "master",
				template:     csuiteTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"compliancesuite", subD.namespace, csuiteMD.name},
			objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})

		// adding label to rhcos worker node to skip rhel worker node if any
		g.By("Label all rhcos worker nodes as wscan.. !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		g.By("Create compliancesuite objects !!!\n")
		csuiteD.namespace = subD.namespace
		csuiteD.create(oc)
		csuiteMD.namespace = subD.namespace
		csuiteMD.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteD.name, ok, []string{"compliancesuite", "-n", csuiteD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteMD.name, ok, []string{"compliancesuite", "-n", csuiteMD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", csuiteD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteMD.name, "-n", csuiteMD.namespace, "-o=jsonpath={.status.phase}")

		fipsOut := checkFipsStatus(oc, subD.namespace)
		if strings.Contains(fipsOut, "FIPS mode is enabled.") {
			g.By("Check complianceSuite result.. !!!\n")
			subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
			subD.complianceSuiteResult(oc, csuiteMD.name, "COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult", csuiteMD.scanname + "-enable-fips-mode", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult", csuiteD.scanname + "-enable-fips-mode", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		} else {
			g.By("Check complianceSuite result.. !!!\n")
			subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")
			subD.complianceSuiteResult(oc, csuiteMD.name, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult", csuiteMD.scanname + "-enable-fips-mode", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult", csuiteD.scanname + "-enable-fips-mode", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		}

		g.By("ocp-41861 Successfully verify fips mode checking rules are working as expected ..!!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:pdhamdhe-High-41093-Medium-44944-The instructions should be available for all rules in cis profiles and The nodeName shows in target and fact:identifier elements of complianceScan XCCDF format result [Serial][Slow]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "cis-instruction" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-cis",
			profilename2:    "ocp4-cis-node",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Check the instructions exists for cis manual rules.. !!!\n")
		checkInstructionsForManualRules(oc, ssb.name)

		g.By("Verify the nodeName shows in target & fact:identifier elements of complianceScan XCCDF format result.. !!!\n")
		extractResultFromConfigMap(oc, "worker", ssb.namespace)
		extractResultFromConfigMap(oc, "master", ssb.namespace)
		g.By("All CIS rules has instructions & nodeName is available in target & identifier elements of complianceScan XCCDF format result..!!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:xiyuan-High-47044-High-74104-Verify the moderate profiles perform scan as expected with default scanSettings [Serial][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		ssbModerate := "ssb-moderate-" + getRandomString()

		g.By("Check the annotations for all profiles exists .. !!!\n")
		profiles, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "profile.compliance", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		profilesList := strings.Fields(profiles)
		for _, profile := range profilesList {
			newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profile.compliance", profile, "-n", subD.namespace,
				`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		}

		g.By("Get guid for moderate profiles.. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")
		subD.getProfileName(oc, "ocp4-moderate-node")
		subD.getProfileName(oc, "rhcos4-moderate")
		ocp4ModerateGuid, errGetOcp4ModerateGuid := oc.AsAdmin().Run("get").Args("profile.compliance", "ocp4-moderate", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`).Output()
		o.Expect(errGetOcp4ModerateGuid).NotTo(o.HaveOccurred())
		ocp4ModerateNodeGuid, errGetOcp4ModerateNodeGuid := oc.AsAdmin().Run("get").Args("profile.compliance", "ocp4-moderate-node", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`).Output()
		o.Expect(errGetOcp4ModerateNodeGuid).NotTo(o.HaveOccurred())
		rhcos4ModerateGuid, errGetRhcos4ModerateGuid := oc.AsAdmin().Run("get").Args("profile.compliance", "rhcos4-moderate", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`).Output()
		o.Expect(errGetRhcos4ModerateGuid).NotTo(o.HaveOccurred())

		g.By("Create scansettingbinding... !!!\n")
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbModerate})
		_, errBind := OcComplianceCLI().Run("bind").Args("-N", ssbModerate, "profile/ocp4-moderate", "profile/ocp4-moderate-node", "profile/rhcos4-moderate", "-n", subD.namespace).Output()
		o.Expect(errBind).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbModerate, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbModerate, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbModerate, "NON-COMPLIANT INCONSISTENT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbModerate, "2")

		g.By("Check compliance scan should have labels with profile guid !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, ocp4ModerateGuid, ok, []string{"scan", "ocp4-moderate", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ocp4ModerateNodeGuid, ok, []string{"scan", "ocp4-moderate-node-master", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ocp4ModerateNodeGuid, ok, []string{"scan", "ocp4-moderate-node-worker", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, rhcos4ModerateGuid, ok, []string{"scan", "rhcos4-moderate-master", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, rhcos4ModerateGuid, ok, []string{"scan", "rhcos4-moderate-worker", "-n", subD.namespace,
			`-o=jsonpath={.metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)

		g.By("Check rules  should have labels with profile guid !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, ocp4ModerateGuid, ok, []string{"ccr", "-l", "compliance.openshift.io/scan-name=ocp4-moderate", "-n", subD.namespace,
			`-o=jsonpath={.items[0].metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ocp4ModerateNodeGuid, ok, []string{"ccr", "-l", "compliance.openshift.io/scan-name=ocp4-moderate-node-master", "-n", subD.namespace,
			`-o=jsonpath={.items[0].metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ocp4ModerateNodeGuid, ok, []string{"ccr", "-l", "compliance.openshift.io/scan-name=ocp4-moderate-node-worker", "-n", subD.namespace,
			`-o=jsonpath={.items[0].metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, rhcos4ModerateGuid, ok, []string{"ccr", "-l", "compliance.openshift.io/scan-name=rhcos4-moderate-master", "-n", subD.namespace,
			`-o=jsonpath={.items[0].metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, rhcos4ModerateGuid, ok, []string{"ccr", "-l", "compliance.openshift.io/scan-name=rhcos4-moderate-worker", "-n", subD.namespace,
			`-o=jsonpath={.items[0].metadata.labels.compliance\.openshift\.io/profile-guid}`}).check(oc)

		g.By("ocp-47044 The ocp4 moderate profiles perform scan as expected with default scanSettings... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:xiyuan-High-50524-The instructions should be available for all rules in high profiles [Serial][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		ssbHigh := "ssb-high-" + getRandomString()

		g.By("Check moderate profiles .. !!!\n")
		subD.getProfileName(oc, "ocp4-high")
		subD.getProfileName(oc, "ocp4-high-node")
		subD.getProfileName(oc, "rhcos4-high")

		g.By("Create scansettingbinding... !!!\n")
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbHigh})
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbHigh, "profile/ocp4-high", "profile/ocp4-high-node", "profile/rhcos4-high", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbHigh, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbHigh, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbHigh, "NON-COMPLIANT INCONSISTENT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbHigh, "2")

		g.By("Check the instructions exists for cis manual rules.. !!!\n")
		checkInstructionsForManualRules(oc, ssbHigh)

		g.By("ocp-50524 The instructions should be available for all rules in high profiles..!!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:pdhamdhe-Low-42695-Verify the manual remediation for rule ocp4-moderate-scansettingbinding-exists works as expected [Serial]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "moderate-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-cis",
			profilename2:    "ocp4-moderate",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name})

		g.By("Check default profiles name ocp4-moderate .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "moderate-test", ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify 'ocp4-moderate-scansettingbinding-exists' rule status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-scansettingbinding-exists", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("The ocp-42695 The ScanSettingBinding object exist and operator is successfully installed... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Longduration-CPaasrunOnly-NonPreRelease-Low-42719-Low-42810-Low-42834-Check manual remediation works for TokenMaxAge TokenInactivityTimeout and no-ldap-insecure rules for oauth cluster object [Disruptive][Slow]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "moderate-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-moderate",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}
		defer func() {
			g.By("Check pod status from 'openshift-authentication' namespace during pod reboot.. !!!\n")
			expectedStatus := map[string]string{"Progressing": "True"}
			err := waitCoBecomes(oc, "authentication", 240, expectedStatus)
			exutil.AssertWaitPollNoErr(err, `authentication status has not yet changed to {"Progressing": "True"} in 240 seconds`)
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "authentication", 240, expectedStatus)
			exutil.AssertWaitPollNoErr(err, `authentication status has not yet changed to {"Available": "True", "Progressing": "False", "Degraded": "False"} in 240 seconds`)
		}()
		defer func() {
			g.By("Remove TokenMaxAge, TokenInactivityTimeout parameters and ldap configuration by patching.. !!!\n")
			patch1 := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/tokenConfig/accessTokenMaxAgeSeconds\"}]")
			patch2 := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/tokenConfig/accessTokenInactivityTimeout\"}]")
			patch3 := fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/identityProviders\",\"value\":[{\"ldap\":{\"attributes\":{\"id\":[\"dn\"],\"name\":[\"cn\"],\"preferredUsername\":[\"uid\"]},\"bindDN\":\"\",\"bindPassword\":{\"name\":\"\"},\"ca\":{\"name\":\"ad-ldap\"},\"insecure\":false,\"url\":\"ldaps://10.66.147.179/cn=users,dc=ad-example,dc=com?uid\"},\"mappingMethod\":\"claim\",\"name\":\"AD_ldaps_provider\",\"type\":\"LDAP\"}]}]")
			patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type", "json", "-p", patch1)
			patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type", "json", "-p", patch2)
			patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type=json", "-p", patch3)
			newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"oauth", "cluster", "-o=jsonpath={.spec.tokenConfig.accessTokenMaxAgeSeconds}"}).check(oc)
			newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"oauth", "cluster", "-o=jsonpath={.spec.tokenConfig.accessTokenInactivityTimeout}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "ldap", nok, []string{"oauth", "cluster", "-o=jsonpath={.spec.identityProviders}"}).check(oc)
			cleanupObjects(oc, objectTableRef{"configmap", "openshift-config", "ca.crt"})
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
		}()

		g.By("Check default profiles name ocp4-moderate .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify TokenMaxAge, TokenInactivityTimeout and no-ldap-insecure rules status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-ocp-no-ldap-insecure", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		ruleOauthMaxageRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ccr", "-n", subD.namespace, "ocp4-moderate-oauth-or-oauthclient-token-maxage", "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ruleOauthInactivateTimeoutRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ccr", "-n", subD.namespace, "ocp4-moderate-oauth-or-oauthclient-inactivity-timeout", "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if ruleOauthMaxageRes != "FAIL" || ruleOauthInactivateTimeoutRes != "FAIL" {
			g.Skip(fmt.Sprintf("The precondition not matched! The scan result for rule ocp4-moderate-oauth-or-oauthclient-token-maxage is: %s! And scan result for rule ocp4-moderate-oauth-or-oauthclient-inactivity-timeout is: %s", ruleOauthMaxageRes, ruleOauthInactivateTimeoutRes))
		}

		g.By("Set TokenMaxAge, TokenInactivityTimeout parameters and ldap configuration by patching.. !!!\n")
		patch1 := fmt.Sprintf("[{\"op\":\"add\",\"path\":\"/spec/identityProviders\",\"value\":[{\"ldap\":{\"attributes\":{\"email\":[\"mail\"],\"id\":[\"dn\"],\"name\":[\"uid\"],\"preferredUsername\":[\"uid\"]},\"insecure\":true,\"url\":\"ldap://10.66.147.104:389/ou=People,dc=my-domain,dc=com?uid\"},\"mappingMethod\":\"add\",\"name\":\"openldapidp\",\"type\":\"LDAP\"}]}]")
		patch2 := fmt.Sprintf("[{\"op\":\"remove\",\"path\":\"/spec/identityProviders\",\"value\":[{\"ldap\":{\"attributes\":{\"email\":[\"mail\"],\"id\":[\"dn\"],\"name\":[\"uid\"],\"preferredUsername\":[\"uid\"]},\"insecure\":true,\"url\":\"ldap://10.66.147.104:389/ou=People,dc=my-domain,dc=com?uid\"},\"mappingMethod\":\"add\",\"name\":\"openldapidp\",\"type\":\"LDAP\"}]}]")
		patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type", "merge", "-p",
			"{\"spec\":{\"tokenConfig\":{\"accessTokenMaxAgeSeconds\":28800}}}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "28800", ok, []string{"oauth", "cluster",
			"-o=jsonpath={.spec.tokenConfig.accessTokenMaxAgeSeconds}"}).check(oc)
		patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type", "merge", "-p",
			"{\"spec\":{\"tokenConfig\":{\"accessTokenInactivityTimeout\":\"10m0s\"}}}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "10m0s", ok, []string{"oauth", "cluster",
			"-o=jsonpath={.spec.tokenConfig.accessTokenInactivityTimeout}"}).check(oc)
		patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type=json", "-p", patch1)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ldap", ok, []string{"oauth", "cluster", "-o=jsonpath={.spec.identityProviders}"}).check(oc)

		g.By("Check pod status from 'openshift-authentication' namespace during pod reboot.. !!!\n")
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "authentication", 240, expectedStatus)
		exutil.AssertWaitPollNoErr(err, `authentication status has not yet changed to {"Progressing": "True"} in 240 seconds`)
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "authentication", 240, expectedStatus)
		exutil.AssertWaitPollNoErr(err, `authentication status has not yet changed to {"Available": "True", "Progressing": "False", "Degraded": "False"} in 240 seconds`)

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err = OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Verify TokenMaxAge, TokenInactivityTimeout and no-ldap-insecure rules status through compliancecheck result after rescan.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-oauth-or-oauthclient-token-maxage", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-oauth-or-oauthclient-inactivity-timeout", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-moderate-ocp-no-ldap-insecure", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type=json", "-p", patch2)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ldap", nok, []string{"oauth", "cluster", "-o=jsonpath={.spec.identityProviders}"}).check(oc)

		g.By("Apply secure ldap to oauth cluster object by patching.. !!!\n")
		patch3 := fmt.Sprintf("[{\"op\":\"add\",\"path\":\"/spec/identityProviders\",\"value\":[{\"ldap\":{\"attributes\":{\"id\":[\"dn\"],\"name\":[\"cn\"],\"preferredUsername\":[\"uid\"]},\"bindDN\":\"\",\"bindPassword\":{\"name\":\"\"},\"ca\":{\"name\":\"ad-ldap\"},\"insecure\":false,\"url\":\"ldaps://10.66.147.179/cn=users,dc=ad-example,dc=com?uid\"},\"mappingMethod\":\"claim\",\"name\":\"AD_ldaps_provider\",\"type\":\"LDAP\"}]}]")
		patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type=json", "-p", patch3)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ldap", ok, []string{"oauth", "cluster", "-o=jsonpath={.spec.identityProviders}"}).check(oc)
		g.By("Configured ldap to oauth cluster object by patching.. !!!\n")
		_, err2 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-config", "-f", ldapConfigMapYAML).Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "ca.crt", ok, []string{"configmap", "-n", "openshift-config", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err3 := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		g.By("Verify 'ocp4-moderate-ocp-no-ldap-insecure' rule status again through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-ocp-no-ldap-insecure", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ocp-42719-42810-42834 The manual remediation successfully applied for TokenMaxAge, TokenInactivityTimeout and no-ldap-insecure rules... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:pdhamdhe-Low-42960-Low-43098-Check that TokenMaxAge and TokenInactivityTimeout are configurable for oauthclient objects [Disruptive][Slow]", func() {
		var (
			tp = tailoredProfileDescription{
				name:         "tp-moderate-" + getRandomString(),
				namespace:    subD.namespace,
				extends:      "ocp4-moderate",
				enrulename1:  "ocp4-oauth-or-oauthclient-token-maxage",
				disrulename1: "ocp4-ocp-no-ldap-insecure",
				disrulename2: "ocp4-openshift-motd-exists",
				varname:      "ocp4-var-oauth-token-maxage",
				value:        "28800",
				template:     tprofileTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "moderate-test" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "TailoredProfile",
				profilename1:    tp.name,
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"tp", subD.namespace, tp.name})

		defer func() {
			g.By("Remove TokenMaxAge parameter by patching oauthclient objects.. !!!\n")
			oauthclients, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("oauthclient", "-n", subD.namespace,
				"-o=jsonpath={.items[*].metadata.name}").Output()
			oauthclient := strings.Fields(oauthclients)
			for _, v := range oauthclient {
				patchResource(oc, asAdmin, withoutNamespace, "oauthclient", v, "--type=json", "-p",
					"[{\"op\": \"remove\",\"path\": \"/accessTokenMaxAgeSeconds\"}]")
				newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"oauthclient", v,
					"-o=jsonpath={.accessTokenMaxAgeSeconds}"}).check(oc)
				patchResource(oc, asAdmin, withoutNamespace, "oauthclient", v, "--type=json", "-p",
					"[{\"op\": \"remove\",\"path\": \"/accessTokenInactivityTimeoutSeconds\"}]")
				newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"oauthclient", v,
					"-o=jsonpath={.accessTokenInactivityTimeoutSeconds}"}).check(oc)
			}
		}()

		g.By("Check default profiles name ocp4-moderate .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		g.By("Create tailoredprofile !!!\n")
		tp.namespace = subD.namespace
		tp.create(oc)
		subD.getTailoredProfileNameandStatus(oc, tp.name)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify 'ocp4-oauth-or-oauthclient-token-maxage' rule status through compliancecheck result.. !!!\n")
		ruleOauthMaxageRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ccr", "-n", subD.namespace, tp.name+"-oauth-or-oauthclient-token-maxage", "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ruleOauthInactivateTimeoutRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ccr", "-n", subD.namespace, tp.name+"-oauth-or-oauthclient-inactivity-timeout", "-o=jsonpath={.status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if ruleOauthMaxageRes != "FAIL" || ruleOauthInactivateTimeoutRes != "FAIL" {
			g.Skip(fmt.Sprintf("The precondition not matched! The scan result for rule ocp4-oauth-or-oauthclient-token-maxage is: %s! And scan result for rule ocp4-moderate-oauth-or-oauthclient-inactivity-timeout is: %s", ruleOauthMaxageRes, ruleOauthInactivateTimeoutRes))
		}

		g.By("Set TokenMaxAge parameter to console oauthclient object by patching.. !!!\n")
		patchResource(oc, asAdmin, withoutNamespace, "oauthclient", "console", "--type", "merge", "-p", "{\"accessTokenMaxAgeSeconds\":28800}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "28800", ok, []string{"oauthclient", "console",
			"-o=jsonpath={.accessTokenMaxAgeSeconds}"}).check(oc)

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err = OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check ComplianceSuite status.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		g.By("Verify 'ocp4-oauth-or-oauthclient-token-maxage' rule status again through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			tp.name + "-oauth-or-oauthclient-token-maxage", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Set TokenMaxAge & TokenInactivityTimeout parameters to all remaining oauthclient objects by patching.. !!!\n")
		oauthclients, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("oauthclient", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		oauthclient := strings.Fields(oauthclients)
		for _, v := range oauthclient {
			patchResource(oc, asAdmin, withoutNamespace, "oauthclient", v, "--type", "merge", "-p", "{\"accessTokenMaxAgeSeconds\":28800}")
			newCheck("expect", asAdmin, withoutNamespace, contain, "28800", ok, []string{"oauthclient", v,
				"-o=jsonpath={.accessTokenMaxAgeSeconds}"}).check(oc)
			patchResource(oc, asAdmin, withoutNamespace, "oauthclient", v, "--type", "merge", "-p", "{\"accessTokenInactivityTimeoutSeconds\":600}")
			newCheck("expect", asAdmin, withoutNamespace, contain, "600", ok, []string{"oauthclient", v,
				"-o=jsonpath={.accessTokenInactivityTimeoutSeconds}"}).check(oc)
		}

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err1 := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		g.By("Check ComplianceSuite status.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Verify TokenMaxAge & TokenInactivityTimeout rules status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			tp.name + "-oauth-or-oauthclient-token-maxage", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			tp.name + "-oauth-or-oauthclient-inactivity-timeout", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ocp-42960-43098 The TokenMaxAge & TokenInactivityTimeout parameters are configured for oauthclient objects successfully... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-ROSA-ARO-OSD_CCS-Author:pdhamdhe-Low-42685-Low-46927-check the manual remediation for rules file-integrity-exists and file-integrity-notification-enabled working as expected [Serial][Slow]", func() {
		var (
			ssb = scanSettingBindingDescription{
				name:            "moderate-test" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-moderate",
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
			fioNamespace = "fio" + getRandomString()
			og           = operatorGroupDescription{
				name:      "openshift-file-integrity-qbcd",
				namespace: fioNamespace,
				template:  ogCoTemplate,
			}
			sub = subscriptionDescription{
				subName:                "file-integrity-operator",
				namespace:              fioNamespace,
				channel:                "stable",
				ipApproval:             "Automatic",
				operatorPackage:        "file-integrity-operator",
				catalogSourceName:      "redhat-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subCoTemplate,
				singleNamespace:        true,
			}
			fi1 = fileintegrity{
				name:              "example-fileintegrity",
				namespace:         fioNamespace,
				configname:        "",
				configkey:         "",
				graceperiod:       15,
				debug:             false,
				nodeselectorkey:   "node.openshift.io/os_id",
				nodeselectorvalue: "rhcos",
				template:          fioTemplate,
			}
		)
		defer func() {
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("fileintegrity", "--all", "-n", sub.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", sub.namespace, "-n", sub.namespace, "--ignore-not-found").Execute()
		}()

		g.By("Check default profiles name ocp4-moderate .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		fioObj, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("fileintegrities", "--all-namespaces",
			"-o=jsonpath={.items[0].status.phase}").Output()

		if strings.Contains(fioObj, "Active") {
			g.By("The fileintegrity operator is installed, let's verify the rule status through compliancecheck result.. !!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
				"ocp4-moderate-file-integrity-exists", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
				"ocp4-moderate-file-integrity-notification-enabled", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
			g.By("The file-integrity-exists and file-integrity-notification-enabled rules are verified successfully... !!!!\n ")
		} else {
			g.By("\n\n Let's installed fileintegrity operator... !!!\n")
			createFileIntegrityOperator(oc, sub, og)

			g.By("Create File Integrity object.. !!!\n")
			fi1.namespace = sub.namespace
			err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", fi1.template, "-p", "NAME="+fi1.name, "NAMESPACE="+fi1.namespace,
				"GRACEPERIOD="+strconv.Itoa(fi1.graceperiod), "DEBUG="+strconv.FormatBool(fi1.debug), "NODESELECTORKEY="+fi1.nodeselectorkey, "NODESELECTORVALUE="+fi1.nodeselectorvalue)
			o.Expect(err).NotTo(o.HaveOccurred())
			fi1.checkFileintegrityStatus(oc, "running")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Active", ok, []string{"fileintegrity", fi1.name, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

			g.By("Rerun scan and check ComplianceSuite status & result.. !!!\n")
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssb.name, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssb.name, "-n", subD.namespace,
				"-o=jsonpath={.status.phase}"}).check(oc)
			subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

			g.By("Verify 'file-integrity-exists' & 'file-integrity-notification-enabled' rules status again through compliancecheck result.. !!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
				"ocp4-moderate-file-integrity-exists", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
				"ocp4-moderate-file-integrity-notification-enabled", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

			g.By("The file-integrity-exists and file-integrity-notification-enabled rules are verified successfully... !!!!\n ")
		}
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-NonPreRelease-CPaasrunOnly-Medium-40660-Low-42874-Check whether the audit logs are getting forwarded using TLS protocol [Disruptive][Slow]", func() {
		var (
			ogL = operatorGroupDescription{
				name:      "openshift-logging",
				namespace: "openshift-logging",
				template:  ogCoTemplate,
			}
			subL = subscriptionDescription{
				subName:                "cluster-logging",
				namespace:              "openshift-logging",
				channel:                "stable",
				ipApproval:             "Automatic",
				operatorPackage:        "cluster-logging",
				catalogSourceName:      "redhat-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subCoTemplate,
				singleNamespace:        true,
			}
			ssb = scanSettingBindingDescription{
				name:            "moderate-test" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				profilename2:    "ocp4-moderate",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"ClusterLogForwarder", subL.namespace, "instance"},
			objectTableRef{"ClusterLogging", subL.namespace, "instance"},
			objectTableRef{"configmap", subL.namespace, "fluentdserver"},
			objectTableRef{"deployment", subL.namespace, "fluentdserver"},
			objectTableRef{"service", subL.namespace, "fluentdserver"},
			objectTableRef{"serviceaccount", subL.namespace, "fluentdserver"},
			objectTableRef{"secret", subL.namespace, "fluentdserver"},
			objectTableRef{"project", subL.namespace, subL.namespace})

		g.By("Check default profiles are available 'ocp4-cis' and 'ocp4-moderate' .. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")
		subD.getProfileName(oc, "ocp4-moderate")
		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify audit rules status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-moderate-audit-log-forwarding-uses-tls", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-cis-audit-log-forwarding-enabled", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Check if the openshift-logging namespace is already available.. !!!\n")
		namespace, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "-ojsonpath={.items[*].metadata.name}").Output()
		if !strings.Contains(namespace, "openshift-logging") {
			g.By("Install logging operator and check it is installed successfully.. !!!\n")
			createOperator(oc, subL, ogL)
			newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "-l operators.coreos.com/cluster-logging.openshift-logging=", "-n", subL.namespace,
				"-o=jsonpath={.items[0].status.phase}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", "-l name=cluster-logging-operator", "-n", subL.namespace,
				"-o=jsonpath={.items[0].status.phase}"}).check(oc)
		} else {
			g.By("The openshift-logging namespace is already available.. !!!\n")
			podStat := checkOperatorPodStatus(oc, "openshift-logging")
			if !strings.Contains(podStat, "Running") {
				createOperator(oc, subL, ogL)
				newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "-l operators.coreos.com/cluster-logging.openshift-logging=", "-n", subL.namespace,
					"-o=jsonpath={.items[0].status.phase}"}).check(oc)
				newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", "-l name=cluster-logging-operator", "-n", subL.namespace,
					"-o=jsonpath={.items[0].status.phase}"}).check(oc)
			} else {
				g.By("The cluster-logging operator is installed in openshift-logging namespace and pod is running.. !!!\n")
			}
		}

		g.By("Generate the secret key for fluentdserver.. !!!\n")
		genFluentdSecret(oc, subL.namespace, "fluentdserver")
		newCheck("expect", asAdmin, withoutNamespace, contain, "fluentdserver", ok, []string{"secret", "-n", subL.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create service accound for fluentd receiver.. !!!\n")
		_, err2 := oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", "fluentdserver", "-n", subL.namespace).Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "fluentdserver", ok, []string{"sa", "-n", subL.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		_, err3 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "privileged", "system:serviceaccount:openshift-logging:fluentdserver", "-n", subL.namespace).Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		g.By("Create Fluentd ConfigMap .. !!!\n")
		_, err4 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subL.namespace, "-f", fluentdCmYAML).Output()
		o.Expect(err4).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "fluentdserver", ok, []string{"cm", "-n", subL.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Create Fluentd Deployment .. !!!\n")
		_, err5 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subL.namespace, "-f", fluentdDmYAML).Output()
		o.Expect(err5).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "fluentdserver", ok, []string{"deployment", "-n", subL.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Expose Fluentd Deployment .. !!!\n")
		_, err6 := oc.AsAdmin().WithoutNamespace().Run("expose").Args("deployment", "fluentdserver", "-n", subL.namespace).Output()
		o.Expect(err6).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "fluentdserver", ok, []string{"deployment", "-n", subL.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create ClusterForward & ClusterLogging instances.. !!!\n")
		_, err7 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subL.namespace, "-f", clusterLogForYAML).Output()
		o.Expect(err7).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "instance", ok, []string{"ClusterLogForwarder", "-n", subL.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		_, err8 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subL.namespace, "-f", clusterLoggingYAML).Output()
		o.Expect(err8).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "instance", ok, []string{"ClusterLogging", "-n", subL.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check fluentdserver is running state.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pod", "-l logging-infra=fluentdserver", "-n",
			subL.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)

		g.By("Rerun scan and check ComplianceSuite status & result.. !!!\n")
		_, err9 := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err9).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify audit rules status again through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-audit-log-forwarding-uses-tls", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-cis-audit-log-forwarding-enabled", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		//	csvname, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		//	assertCheckAuditLogsForword(oc, subL.namespace, csvname)

		g.By("ocp-40660 and ocp-42874 the audit logs are getting forwarded using TLS protocol successfully... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-NonPreRelease-CPaasrunOnly-Low-42700-Check that a login banner is configured and login screen customised [Disruptive][Slow]", func() {

		var ssb = scanSettingBindingDescription{
			name:            "moderate-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-moderate",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}

		defer func() {
			g.By("Remove motd, ConsoleNotification and scansettingbinding objects.. !!!\n")
			patch := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/templates/login\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type", "json", "-p", patch)
			newCheck("expect", asAdmin, withoutNamespace, contain, "login-secret", nok, []string{"oauth", "cluster", "-o=jsonpath={.spec.templates.login}"}).check(oc)
			cleanupObjects(oc, objectTableRef{"secret", "openshift-config", "login-secret"})
			cleanupObjects(oc, objectTableRef{"configmap", "openshift", "motd"})
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
		}()

		g.By("Check default profiles name ocp4-moderate .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify motd and banner or login rules status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-moderate-openshift-motd-exists", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-moderate-banner-or-login-template-set", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Create motd configMap and consoleNotification objects.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift", "-f", motdConfigMapYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "motd", ok, []string{"configmap", "-n", "openshift", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		_, err1 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", consoleNotificationYAML).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "classification-banner", ok, []string{"ConsoleNotification", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err2 := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Verify motd and banner or login rules status again through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-openshift-motd-exists", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-banner-or-login-template-set", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		cleanupObjects(oc, objectTableRef{"ConsoleNotification", subD.namespace, "classification-banner"})
		createLoginTemp(oc, "openshift-config")
		newCheck("expect", asAdmin, withoutNamespace, contain, "login-secret", ok, []string{"secret", "-n", "openshift-config", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Set login-secret template to oauth cluster object by patching.. !!!\n")
		patchResource(oc, asAdmin, withoutNamespace, "oauth", "cluster", "--type", "merge", "-p", "{\"spec\":{\"templates\":{\"login\":{\"name\":\"login-secret\"}}}}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "login-secret", ok, []string{"oauth", "cluster", "-o=jsonpath={.spec.templates.login}"}).check(oc)

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err3 := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Verify ocp4-moderate-banner-or-login-template-set rule status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-banner-or-login-template-set", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ocp-42700 The login banner is configured and login screen customised successfully... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-NonPreRelease-CPaasrunOnly-Low-42720-check manual remediation for rule ocp4-moderate-configure-network-policies-namespaces working as expected [Disruptive][Slow]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "moderate-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-moderate",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}

		defer func() {
			g.By("Remove NetworkPolicy object from all non-control plane namespace.. !!!\n")
			nsName := "ns-42720-1"
			cleanupObjects(oc, objectTableRef{"ns", nsName, nsName})
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
			nonControlNamespacesList := getNonControlNamespaces(oc, "Active")
			for _, v := range nonControlNamespacesList {
				cleanupObjects(oc, objectTableRef{"NetworkPolicy", v, "allow-same-namespace"})
				newCheck("expect", asAdmin, withoutNamespace, contain, "allow-same-namespace", nok, []string{"NetworkPolicy", "-n", v, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			}
		}()

		g.By("Check default profiles name ocp4-moderate .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")
		ssb.namespace = subD.namespace

		nonControlNamespacesTerList := getNonControlNamespaces(oc, "Terminating")
		e2e.Logf("Terminating Non Control Namespaces List: %s", nonControlNamespacesTerList)
		if len(nonControlNamespacesTerList) != 0 {
			for _, v := range nonControlNamespacesTerList {
				scanName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("scan", "-n", v, "-ojsonpath={.items[*].metadata.name}").Output()
				scans := strings.Fields(scanName)
				patch := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/metadata/finalizers\"}]")
				if len(scans) != 0 {
					for _, scanname := range scans {
						e2e.Logf("The %s scan patched to remove finalizers from namespace %s \n", scanname, v)
						patchResource(oc, asAdmin, withoutNamespace, "scan", scanname, "--type", "json", "-p", patch, "-n", v)
					}
				}
				suiteName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("suite", "-n", v, "-ojsonpath={.items[*].metadata.name}").Output()
				suites := strings.Fields(suiteName)
				if len(suites) != 0 {
					for _, suitename := range suites {
						e2e.Logf("The %s suite patched to remove finalizers from namespace %s \n", suitename, v)
						patchResource(oc, asAdmin, withoutNamespace, "suite", suitename, "--type", "json", "-p", patch, "-n", v)
					}
				}
				profbName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("profilebundles", "-n", v, "-ojsonpath={.items[*].metadata.name}").Output()
				profb := strings.Fields(profbName)
				if len(profb) != 0 {
					for _, profbname := range profb {
						e2e.Logf("The %s profilebundle patched to remove finalizers from namespace %s \n", profbname, v)
						patchResource(oc, asAdmin, withoutNamespace, "profilebundles", profbname, "--type", "json", "-p", patch, "-n", v)
					}
				}
			}
		}

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")
		g.By("Verify ocp4-moderate-configure-network-policies-namespaces rule status through compliancecheck result.. !!!\n")
		ruleCheckResult, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("ccr", "-n", subD.namespace, "ocp4-moderate-configure-network-policies-namespaces", "-o=jsonpath={.status}").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		if ruleCheckResult != "FAIL" {
			g.Skip(fmt.Sprintf("The precondition not matched! The scan result for rule ocp4-moderate-configure-network-policies-namespaces is: %s! The nonControlNamespacesTerList is: %s", ruleCheckResult, nonControlNamespacesTerList))
		}

		nonControlNamespacesTerList1 := getNonControlNamespaces(oc, "Terminating")
		if len(nonControlNamespacesTerList1) == 0 {
			g.By("Create NetworkPolicy in all non-control plane namespace .. !!!\n")
			nonControlNamespacesList := getNonControlNamespaces(oc, "Active")
			e2e.Logf("Here namespace : %v\n", nonControlNamespacesList)
			for _, v := range nonControlNamespacesList {
				_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", v, "-f", networkPolicyYAML).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				newCheck("expect", asAdmin, withoutNamespace, contain, "allow-same-namespace", ok, []string{"NetworkPolicy", "-n", v, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			}
		}

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", ssb.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		g.By("Verify ocp4-moderate-configure-network-policies-namespaces rule status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-configure-network-policies-namespaces", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Create one more non-control plane namespace .. !!!\n")
		nsName := "ns-42720-1"
		_, err1 := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", nsName).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err2 := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", ssb.namespace).Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		g.By("Verify motd and banner or login rules status again through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-moderate-configure-network-policies-namespaces", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ocp-42720 The manual remediation works for network-policies-namespaces rule... !!!!\n ")

	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:pdhamdhe-High-41153-There are OpenSCAP checks created to verify that the cluster is compliant  for the section 5 of the Kubernetes CIS profile [Serial]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "cis-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-cis",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}

		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})

		g.By("Check default profiles name ocp4-cis .. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")
		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		cisRlueList := []string{"ocp4-cis-rbac-limit-cluster-admin", "ocp4-cis-rbac-limit-secrets-access", "ocp4-cis-rbac-wildcard-use", "ocp4-cis-rbac-pod-creation-access",
			"ocp4-cis-accounts-unique-service-account", "ocp4-cis-accounts-restrict-service-account-tokens", "ocp4-cis-scc-limit-privileged-containers", "ocp4-cis-scc-limit-process-id-namespace",
			"ocp4-cis-scc-limit-ipc-namespace", "ocp4-cis-scc-limit-network-namespace", "ocp4-cis-scc-limit-privilege-escalation", "ocp4-cis-scc-limit-root-containers",
			"ocp4-cis-scc-limit-net-raw-capability", "ocp4-cis-scc-limit-container-allowed-capabilities", "ocp4-cis-scc-drop-container-capabilities", "ocp4-cis-configure-network-policies",
			"ocp4-cis-configure-network-policies-namespaces", "ocp4-cis-secrets-no-environment-variables", "ocp4-cis-secrets-consider-external-storage", "ocp4-cis-ocp-allowed-registries",
			"ocp4-cis-ocp-allowed-registries-for-import", "ocp4-cis-ocp-insecure-registries", "ocp4-cis-ocp-insecure-allowed-registries-for-import", "ocp4-cis-general-namespaces-in-use",
			"ocp4-cis-general-default-seccomp-profile", "ocp4-cis-general-apply-scc", "ocp4-cis-general-default-namespace-use"}
		checkRulesExistInComplianceCheckResult(oc, cisRlueList, subD.namespace)

		g.By("ocp-41153 There are OpenSCAP checks created to verify that the cluster is compliant for the section 5 of the Kubernetes CIS profile... !!!!\n ")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Longduration-CPaasrunOnly-NonPreRelease-High-27967-High-33782-Medium-33711-Medium-47346-The ComplianceSuite performs scan on a subset of nodes with autoApplyRemediations enable and ComplianceCheckResult shows remediation rule result in details and also supports array of values for remediation [Disruptive][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			csuiteD = complianceSuiteDescription{
				name:         "worker-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "worker-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_audit_rules_dac_modification_chmod",
				nodeSelector: "wrscan",
				template:     csuiteTemplate,
			}
			csuite = complianceSuiteDescription{
				name:         "example-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "example-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_no_empty_passwords",
				nodeSelector: "wrscan",
				template:     csuiteRemTemplate,
			}
			csuiteCD = complianceSuiteDescription{
				name:         "chronyd-compliancesuite" + getRandomString(),
				namespace:    "",
				scanname:     "rhcos4-scan" + getRandomString(),
				profile:      "xccdf_org.ssgproject.content_profile_moderate",
				content:      "ssg-rhcos4-ds.xml",
				contentImage: relatedImageProfile,
				rule:         "xccdf_org.ssgproject.content_rule_chronyd_or_ntpd_specify_multiple_servers",
				nodeSelector: "wrscan",
				template:     csuiteRemTemplate,
			}
		)
		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove compliancesuite, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + csuiteD.name})
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + csuite.name})
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + csuiteCD.name})
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancesuite", csuiteD.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancesuite", csuite.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("compliancesuite", csuiteCD.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("mcp", csuiteD.nodeSelector, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, csuiteD.nodeSelector)

		g.By("Create compliancesuite objects !!!\n")
		csuiteD.namespace = subD.namespace
		csuite.namespace = subD.namespace
		csuiteCD.namespace = subD.namespace
		csuiteD.create(oc)
		csuite.create(oc)
		csuiteCD.create(oc)
		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", csuiteD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuiteD.name, "NON-COMPLIANT")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuite.name, "-n", csuite.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuite.name, "NON-COMPLIANT")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteCD.name, "-n", csuiteCD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuiteCD.name, "NON-COMPLIANT")

		g.By("Verify worker-scan-audit-rules-dac-modification-chmod rule status through compliancecheckresult & complianceremediations.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			csuiteD.scanname + "-audit-rules-dac-modification-chmod", "-n", csuiteD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NotApplied", ok, []string{"complianceremediations",
			csuiteD.scanname + "-audit-rules-dac-modification-chmod", "-n", csuiteD.namespace, "-o=jsonpath={.status.applicationState}"}).check(oc)

		g.By("ClusterOperator should be healthy before applying remediation")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Apply remediation by patching rule.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"apply\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "complianceremediations", csuiteD.scanname+"-audit-rules-dac-modification-chmod", "-n", csuiteD.namespace, "--type", "merge", "-p", patch)

		g.By("Verify rules status through compliancecheckresult !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			csuite.scanname + "-no-empty-passwords", "-n", csuite.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			csuiteCD.scanname + "-chronyd-or-ntpd-specify-multiple-servers", "-n", csuiteCD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Verified autoremediation applied for those rules and machineConfig gets created.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Applied", ok, []string{"complianceremediations", csuiteD.scanname + "-audit-rules-dac-modification-chmod", "-n", csuiteD.namespace,
			"-o=jsonpath={.status.applicationState}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "75-"+csuiteD.scanname+"-audit-rules-dac-modification-chmod", ok, []string{"mc", "-n", csuiteD.namespace,
			"--selector=compliance.openshift.io/scan-name=" + csuiteD.scanname, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Applied", ok, []string{"complianceremediations", csuite.scanname + "-no-empty-passwords", "-n", csuiteD.namespace,
			"-o=jsonpath={.status.applicationState}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "75-"+csuite.scanname+"-no-empty-passwords", ok, []string{"mc", "-n", csuiteD.namespace,
			"--selector=compliance.openshift.io/scan-name=" + csuite.scanname, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		newCheck("expect", asAdmin, withoutNamespace, contain, "Applied", ok, []string{"complianceremediations", csuiteCD.scanname + "-chronyd-or-ntpd-specify-multiple-servers", "-n", csuiteCD.namespace,
			"-o=jsonpath={.status.applicationState}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "75-"+csuiteCD.scanname+"-chronyd-or-ntpd-specify-multiple-servers", ok, []string{"mc", "-n", csuiteD.namespace,
			"--selector=compliance.openshift.io/scan-name=" + csuiteCD.scanname, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check worker machineconfigpool status.. !!!\n")
		checkMachineConfigPoolStatus(oc, csuiteD.nodeSelector)

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Rerun scan using oc-compliance plugin.. !!!\n")
		_, err1 := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", csuiteD.name, "-n", csuiteD.namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		_, err2 := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", csuite.name, "-n", csuite.namespace).Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		_, err3 := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", csuiteCD.name, "-n", csuiteCD.namespace).Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", csuiteD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuiteD.name, "COMPLIANT")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuite.name, "-n", csuite.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuite.name, "COMPLIANT")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteCD.name, "-n", csuiteCD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuiteCD.name, "COMPLIANT")

		g.By("Verify rules status through compliancecheck result again.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			csuiteD.scanname + "-audit-rules-dac-modification-chmod", "-n", csuiteD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			csuite.scanname + "-no-empty-passwords", "-n", csuite.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			csuiteCD.scanname + "-chronyd-or-ntpd-specify-multiple-servers", "-n", csuiteCD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Verify the ntp settings from node contents.. !!!\n")
		contList := []string{"server 0.pool.ntp.org minpoll 4 maxpoll 10", "server 1.pool.ntp.org minpoll 4 maxpoll 10", "server 2.pool.ntp.org minpoll 4 maxpoll 10", "server 3.pool.ntp.org minpoll 4 maxpoll 10"}
		checkNodeContents(oc, workerNodeName, subD.namespace, contList, "cat", "-n", "/etc/chrony.d/ntp-server.conf", "server")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-CPaasrunOnly-NonPreRelease-High-45421-Verify the scan scheduling option strict or not strict are configurable through scan objects [Disruptive][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "nodestrict",
				namespace:              "",
				roles1:                 "worker",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "rhcos4-test" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "rhcos4-moderate",
				scansettingname: "nodestrict",
				template:        scansettingbindingSingleTemplate,
			}
		)

		g.By("Get one worker node and mark that unschedulable.. !!!\n")
		nodeName := getOneRhcosWorkerNodeName(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", nodeName).Output()
		_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", nodeName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"node", nodeName, "-o=jsonpath={.spec.unschedulable}"}).check(oc)

		defer func() {
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
			cleanupObjects(oc, objectTableRef{"scansetting", subD.namespace, ss.name})
		}()

		g.By("Check default profiles name rhcos4-moderate.. !!!\n")
		subD.getProfileName(oc, "rhcos4-moderate")

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Remove scansettingbinding object !!!\n")
		cleanupObjects(oc, objectTableRef{"scansettingbinding", ssb.namespace, ssb.name})

		g.By("Patch scansetting object and verify.. !!!\n")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", "{\"strictNodeScan\":true}", "-n", ss.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"ss", ss.name, "-n", ss.namespace, "-o=jsonpath={..strictNodeScan}"}).check(oc)

		g.By("Again create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PENDING", ok, []string{"compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("ocp-45421 Successfully verify the scan scheduling option strict or not strict are configurable through scan objects... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-NonPreRelease-CPaasrunOnly-Longduration-High-45692-Verify scan and manual fix work as expected for NERC CIP profiles with default scanSettings [Disruptive][Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "nerc-ss",
				namespace:              "",
				roles1:                 "worker",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbNerc = "nerc-cip-test" + getRandomString()
		)

		defer func() {
			g.By("Remove motd configmap and scansettingbinding objects.. !!!\n")
			cleanupObjects(oc, objectTableRef{"configmap", "openshift", "motd"})
			cleanupObjects(oc, objectTableRef{"scansetting", subD.namespace, ss.name})
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbNerc})
		}()

		g.By("Check default NERC profiles.. !!!\n")
		subD.getProfileName(oc, "ocp4-nerc-cip")
		subD.getProfileName(oc, "ocp4-nerc-cip-node")
		subD.getProfileName(oc, "rhcos4-nerc-cip")

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.metadata.name}"}).check(oc)

		g.By("Create scansettingbindings !!!\n")
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbNerc, "-S", ss.name, "profile/ocp4-nerc-cip", "profile/ocp4-nerc-cip-node", "profile/rhcos4-nerc-cip", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNerc, ok, []string{"scansettingbinding", ssbNerc, "-n", subD.namespace,
			"-o=jsonpath={.metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbNerc, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssbNerc)
		subD.complianceSuiteResult(oc, ssbNerc, "NON-COMPLIANT INCONSISTENT")

		g.By("Create motd configMap object.. !!!\n")
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift", "-f", motdConfigMapYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "motd", ok, []string{"configmap", "-n", "openshift", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Rerun scan using oc-compliance plugin.. !!")
		_, err1 := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssbNerc, "-n", subD.namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbNerc, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Verify motd and banner or login rules status again through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-nerc-cip-openshift-motd-exists", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ocp-45692 The NERC CIP compliance profiles perform scan and manual fix as expected with default scanSettings... !!!\n")
	})

	// author: pdhamdhe@redhat.com/bgudi@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Author:pdhamdhe-High-46991-High-66791-Check the PCI DSS compliance profiles perform scan as expected with default scanSettings and check for rule ocp4-routes-protected-by-tls [Serial]", func() {
		var (
			ns1    = "mytest" + getRandomString()
			route1 = "myedge" + getRandomString()
			ssb    = scanSettingBindingDescription{
				name:            "pci-dss-test" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-pci-dss",
				profilename2:    "ocp4-pci-dss-node",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			ssbWithVersion32 = scanSettingBindingDescription{
				name:            "pci-dss-3-2-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-pci-dss-3-2",
				profilename2:    "ocp4-pci-dss-node-3-2",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			ssbWithVersion40 = scanSettingBindingDescription{
				name:            "pci-dss-4-0-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-pci-dss-4-0",
				profilename2:    "ocp4-pci-dss-node-4-0",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			serviceTemplate = serviceDescription{
				name:        "service-unsecure" + getRandomString(),
				namespace:   ns1,
				profilekind: "Service",
				template:    serviceYAML,
			}
		)

		g.By("Check default profiles name ocp4-pci-dss .. !!!\n")
		subD.getProfileName(oc, "ocp4-pci-dss")
		g.By("Check default profiles name ocp4-pci-dss-node .. !!!\n")
		subD.getProfileName(oc, "ocp4-pci-dss-node")

		g.By("Create new namespace  !!!")
		defer func() { cleanupObjects(oc, objectTableRef{"ns", serviceTemplate.namespace, serviceTemplate.namespace}) }()
		msg, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", serviceTemplate.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s", msg)

		g.By("Create service !!!")
		defer func() { cleanupObjects(oc, objectTableRef{"service", serviceTemplate.namespace, serviceTemplate.name}) }()
		serviceTemplate.create(oc)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", serviceTemplate.namespace, "service", serviceTemplate.name, "-o=jsonpath={.metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s", msg)

		g.By("Create route !!!")
		defer func() { cleanupObjects(oc, objectTableRef{"route", serviceTemplate.namespace, route1}) }()
		msg, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("route", "edge", route1, "--"+"service="+serviceTemplate.name, "-n", serviceTemplate.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s", msg)

		g.By("Create scansettingbinding !!!\n")
		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansettingbinding", subD.namespace, ssbWithVersion32.name},
			objectTableRef{"scansettingbinding", subD.namespace, ssbWithVersion40.name})
		ssb.create(oc)
		ssbWithVersion32.create(oc)
		ssbWithVersion40.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbWithVersion32.name, ok, []string{"scansettingbinding", "-n", ssbWithVersion32.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbWithVersion40.name, ok, []string{"scansettingbinding", "-n", ssbWithVersion40.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssb.name, subD.namespace, "DONE")
		checkComplianceSuiteStatus(oc, ssbWithVersion32.name, subD.namespace, "DONE")
		checkComplianceSuiteStatus(oc, ssbWithVersion40.name, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteName(oc, ssbWithVersion32.name)
		subD.complianceSuiteName(oc, ssbWithVersion40.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")
		subD.complianceSuiteResult(oc, ssbWithVersion32.name, "NON-COMPLIANT INCONSISTENT")
		subD.complianceSuiteResult(oc, ssbWithVersion40.name, "NON-COMPLIANT INCONSISTENT")
		g.By("Check complianceSuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbWithVersion32.name, "2")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbWithVersion40.name, "2")

		g.By("Diff the failed rules between profiles without suffix and profiles of latest version.. !!!\n")
		checkFailedRulesForTwoProfiles(oc, subD.namespace, ssb.name, ssbWithVersion40.name, "-4-0")

		g.By("ocp-66791 Check whether ocp4-pci-dss-routes-protected-by-tls ccr pass")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-pci-dss-routes-protected-by-tls", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
	})

	// author: pdhamdhe@redhat.com
	g.It("Author:pdhamdhe-ConnectedOnly-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-High-43066-check the metrics and alerts are available for Compliance Operator [Serial]", func() {
		// skip test if telemetry not found
		skipNotelemetryFound(oc)

		var ssb = scanSettingBindingDescription{
			name:            "cis-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-cis",
			profilename2:    "ocp4-cis-node",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})

		metricSsbStr := []string{
			"compliance_operator_compliance_scan_status_total{name=\"ocp4-cis-node-master\",phase=\"DONE\",result=",
			"compliance_operator_compliance_scan_status_total{name=\"ocp4-cis-node-worker\",phase=\"DONE\",result=",
			"compliance_operator_compliance_state{name=\"" + ssb.name + "\"}"}

		newCheck("expect", asAdmin, withoutNamespace, contain, "openshift.io/cluster-monitoring", ok, []string{"namespace", subD.namespace, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "metrics", ok, []string{"service", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check default profiles name ocp4-cis .. !!!\n")
		subD.getProfileName(oc, "ocp4-cis")
		g.By("Check default profiles name ocp4-cis-node .. !!!\n")
		subD.getProfileName(oc, "ocp4-cis-node")

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssb.name, subD.namespace, "DONE")
		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Check metrics !!!\n")
		url := fmt.Sprintf("https://metrics." + subD.namespace + ".svc:8585/metrics-co")
		checkMetric(oc, metricSsbStr, url)
		newCheck("expect", asAdmin, withoutNamespace, contain, "compliance", ok, []string{"PrometheusRule", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NonCompliant", ok, []string{"PrometheusRule", "compliance", "-n", subD.namespace, "-ojsonpath={.spec.groups[0].rules[0].alert}"}).check(oc)

		g.By("ocp-43066 The metrics and alerts are getting reported for Compliance Operator ... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("Author:pdhamdhe-ConnectedOnly-NonHyperShiftHOST-ARO-ConnectedOnly-Low-43072-check the metrics and alerts are available for compliance_operator_compliance_scan_error_total [Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)
		// skip test if telemetry not found
		skipNotelemetryFound(oc)

		var csuiteD = complianceSuiteDescription{
			name:         "worker-compliancesuite" + getRandomString(),
			namespace:    "",
			scanname:     "worker-scan" + getRandomString(),
			profile:      "xccdf_org.ssgproject.content_profile_coreos-ncp",
			content:      "ssg-rhcos4-ds.xml",
			contentImage: relatedImageProfile,
			nodeSelector: "wscan",
			template:     csuiteTemplate,
		}

		defer cleanupObjects(oc, objectTableRef{"compliancesuite", subD.namespace, csuiteD.name})

		g.By("Label all rhcos worker nodes as wscan !!!\n")
		setLabelToNode(oc, "node-role.kubernetes.io/wscan=")

		metricsErr := []string{
			"compliance_operator_compliance_scan_status_total{name=\"" + csuiteD.scanname + "\",phase=\"DONE\",result=\"ERROR\"}",
			"compliance_operator_compliance_state{name=\"" + csuiteD.name + "\"}"}

		newCheck("expect", asAdmin, withoutNamespace, contain, "openshift.io/cluster-monitoring", ok, []string{"namespace", subD.namespace, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "metrics", ok, []string{"service", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create compliancesuite !!!\n")
		csuiteD.namespace = subD.namespace
		csuiteD.create(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteD.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result..!!!\n")
		subD.complianceSuiteName(oc, csuiteD.name)
		subD.complianceSuiteResult(oc, csuiteD.name, "ERROR")

		url := fmt.Sprintf("https://metrics." + subD.namespace + ".svc:8585/metrics-co")
		checkMetric(oc, metricsErr, url)
		newCheck("expect", asAdmin, withoutNamespace, contain, "compliance", ok, []string{"PrometheusRule", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NonCompliant", ok, []string{"PrometheusRule", "compliance", "-n", subD.namespace, "-ojsonpath={.spec.groups[0].rules[0].alert}"}).check(oc)

		g.By("ocp-43072 The metrics and alerts are getting reported for Compliance Operator error ... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-CPaasrunOnly-NonPreRelease-Medium-74227-Medium-74437-Test variables ocp4-var-network-policies-namespaces-exempt-regex and ocp4-var-sccs-with-allowed-capabilities-regex [Serial]", func() {
		var (
			sccName   = "scc-test-" + getRandomString()
			tprofileD = tailoredProfileTwoVarsDescription{
				name:      "tp-variables-" + getRandomString(),
				namespace: subD.namespace,
				extends:   "ocp4-cis",
				varname1:  "ocp4-var-network-policies-namespaces-exempt-regex",
				value1:    "",
				varname2:  "ocp4-var-sccs-with-allowed-capabilities-regex",
				value2:    "",
				template:  tprofileTwoVariablesTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "variables-test-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "TailoredProfile",
				profilename1:    tprofileD.name,
				profilename2:    "ocp4-cis",
				scansettingname: "default",
				template:        scansettingbindingTemplate,
			}
			nsTest1 = "ns-74227-" + getRandomString()
			nsTest2 = "ns-74227-" + getRandomString()
		)

		defer func() {
			g.By("Remove ssb and tailoredprofile... !!!\n")
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
				objectTableRef{"tailoredprofile", subD.namespace, tprofileD.name},
				objectTableRef{"scc", subD.namespace, sccName},
				objectTableRef{"ns", nsTest1, nsTest1},
				objectTableRef{"ns", nsTest2, nsTest2})
		}()

		g.By("Set value1 for tprofileD .. !!!\n")
		errGetNs1 := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", nsTest1).Execute()
		o.Expect(errGetNs1).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, nsTest1, ok, []string{"ns", nsTest1, "-n", nsTest1, "-o=jsonpath={.metadata.name}"}).check(oc)
		errGetNs2 := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", nsTest2).Execute()
		o.Expect(errGetNs2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, nsTest2, ok, []string{"ns", nsTest2, "-n", nsTest2, "-o=jsonpath={.metadata.name}"}).check(oc)
		nonControlNamespacesList := getNonControlNamespacesWithoutStatusChecking(oc)
		length := len(nonControlNamespacesList)
		for i, ns := range nonControlNamespacesList {
			if i == length-1 {
				tprofileD.value1 = tprofileD.value1 + ns
			} else {
				tprofileD.value1 = tprofileD.value1 + ns + "|"
			}
		}

		g.By("Set value2 for tprofileD .. !!!\n")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sccTemplate, "-p", "NAME="+sccName)
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultValue, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("variable", "-n", subD.namespace, tprofileD.varname2, "-o=jsonpath={.value}").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		tprofileD.value2 = defaultValue + "|^" + sccName + "$"

		g.By("Create the tailoredprofile !!!\n")
		errApply := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tprofileD.template, "-p", "NAME="+tprofileD.name, "NAMESPACE="+tprofileD.namespace,
			"EXTENDS="+tprofileD.extends, "VARNAME1="+tprofileD.varname1, "VALUE1="+tprofileD.value1, "VARNAME2="+tprofileD.varname2, "VALUE2="+tprofileD.value2)
		o.Expect(errApply).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", "-n", subD.namespace, tprofileD.name,
			"-o=jsonpath={.status.state}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("Verify the rule status through compliancecheckresult and value through apiservers cluster object... !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			tprofileD.name + "-configure-network-policies-namespaces", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-cis" + "-configure-network-policies-namespaces", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			tprofileD.name + "-scc-limit-container-allowed-capabilities", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-cis" + "-scc-limit-container-allowed-capabilities", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Medium-74227-Medium-74437-Test variables ocp4-var-network-policies-namespaces-exempt-regex and ocp4-var-sccs-with-allowed-capabilities-regex done... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-NonPreRelease-Medium-76105-Check the variable ocp4-var-resource-requests-quota-per-project-exempt-regex works as expected for rule ocp4-resource-requests-quota-per-project [Serial]", func() {
		var (
			tpStig = tailoredProfileTwoVarsDescription{
				name:      "tp-quota-" + getRandomString(),
				namespace: subD.namespace,
				extends:   "ocp4-stig",
				varname1:  "ocp4-var-resource-requests-quota-per-project-exempt-regex",
				value1:    "",
				template:  tpSingleVariableTemplate,
			}
			tpModerate = tailoredProfileTwoVarsDescription{
				name:      "tp-quota-m-" + getRandomString(),
				namespace: subD.namespace,
				extends:   "ocp4-moderate",
				varname1:  "ocp4-var-resource-requests-quota-per-project-exempt-regex",
				value1:    "",
				template:  tpSingleVariableTemplate,
			}
			ssbStig = scanSettingBindingDescription{
				name:            "moderate-test" + getRandomString(),
				namespace:       "",
				profilekind1:    "TailoredProfile",
				profilename1:    tpStig.name,
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
			ssbModerate = scanSettingBindingDescription{
				name:            "moderate-test" + getRandomString(),
				namespace:       "",
				profilekind1:    "TailoredProfile",
				profilename1:    tpModerate.name,
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
			nsTest1 = "ns-76105-" + getRandomString()
			nsTest2 = "ns-76105-" + getRandomString()
		)

		defer func() {
			g.By("Remove ssb and tailoredprofile... !!!\n")
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbStig.name},
				objectTableRef{"scansettingbinding", subD.namespace, ssbModerate.name},
				objectTableRef{"tailoredprofile", subD.namespace, tpStig.name},
				objectTableRef{"tailoredprofile", subD.namespace, tpModerate.name},
				objectTableRef{"ns", nsTest1, nsTest1},
				objectTableRef{"ns", nsTest2, nsTest2})
		}()

		g.By("Set value1 for tpStig .. !!!\n")
		errGetNs1 := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", nsTest1).Execute()
		o.Expect(errGetNs1).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, nsTest1, ok, []string{"ns", nsTest1, "-n", nsTest1, "-o=jsonpath={.metadata.name}"}).check(oc)
		errGetNs2 := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", nsTest2).Execute()
		o.Expect(errGetNs2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, nsTest2, ok, []string{"ns", nsTest2, "-n", nsTest2, "-o=jsonpath={.metadata.name}"}).check(oc)
		nonControlNamespacesList := getNonControlNamespacesWithoutStatusChecking(oc)
		length := len(nonControlNamespacesList)
		for i, ns := range nonControlNamespacesList {
			if i == length-1 {
				tpStig.value1 = tpStig.value1 + ns
			} else {
				tpStig.value1 = tpStig.value1 + ns + "|"
			}
		}
		tpModerate.value1 = tpStig.value1

		g.By("Create the tailoredprofile !!!\n")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tpStig.template, "-p", "NAME="+tpStig.name, "NAMESPACE="+tpStig.namespace,
			"EXTENDS="+tpStig.extends, "VARNAME1="+tpStig.varname1, "VALUE1="+tpStig.value1)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", "-n", subD.namespace, tpStig.name,
			"-o=jsonpath={.status.state}"}).check(oc)
		errApply := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tpModerate.template, "-p", "NAME="+tpModerate.name, "NAMESPACE="+tpModerate.namespace,
			"EXTENDS="+tpModerate.extends, "VARNAME1="+tpModerate.varname1, "VALUE1="+tpModerate.value1)
		o.Expect(errApply).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", "-n", subD.namespace, tpModerate.name,
			"-o=jsonpath={.status.state}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssbStig.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbStig.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		ssbModerate.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbModerate.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbStig.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbModerate.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbStig.name, "NON-COMPLIANT INCONSISTENT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbStig.name, "2")
		subD.complianceSuiteResult(oc, ssbModerate.name, "NON-COMPLIANT INCONSISTENT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbModerate.name, "2")

		g.By("Verify the rule status through compliancecheckresult and value through apiservers cluster object... !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			tpStig.name + "-resource-requests-quota-per-project", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			tpModerate.name + "-resource-requests-quota", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Medium-76105-Check the variable ocp4-var-resource-requests-quota-per-project-exempt-regex works as expected for rule ocp4-resource-requests-quota-per-project done... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Longduration-CPaasrunOnly-NonPreRelease-High-46419-Compliance operator supports remediation templating by setting custom variables in the tailored profile [Disruptive][Slow]", func() {
		var (
			tprofileD = tailoredProfileDescription{
				name:         "ocp4-audit-tailored",
				namespace:    "",
				extends:      "ocp4-cis",
				enrulename1:  "ocp4-audit-profile-set",
				disrulename1: "ocp4-audit-error-alert-exists",
				disrulename2: "ocp4-audit-log-forwarding-uses-tls",
				varname:      "ocp4-var-openshift-audit-profile",
				value:        "WriteRequestBodies",
				template:     tprofileTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "ocp4-audit-ssb" + getRandomString(),
				namespace:       "",
				profilekind1:    "TailoredProfile",
				profilename1:    "ocp4-audit-tailored",
				scansettingname: "default-auto-apply",
				template:        scansettingbindingSingleTemplate,
			}
		)

		defer func() {
			g.By("Reset the audit profile value to Default and remove resources... !!!\n")
			patchResource(oc, asAdmin, withoutNamespace, "apiservers", "cluster", "--type", "merge", "-p",
				"{\"spec\":{\"audit\":{\"profile\":\"Default\"}}}")
			newCheck("expect", asAdmin, withoutNamespace, contain, "Default", ok, []string{"apiservers", "cluster",
				"-o=jsonpath={.spec.audit.profile}"}).check(oc)
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
			cleanupObjects(oc, objectTableRef{"tailoredprofile", subD.namespace, tprofileD.name})
		}()

		g.By("Check default profiles name ocp4-cis .. !!!\n")
		subD.getProfileName(oc, tprofileD.extends)

		g.By("Create tailoredprofile ocp4-audit-tailored !!!\n")
		tprofileD.namespace = subD.namespace
		ssb.namespace = subD.namespace
		tprofileD.create(oc)
		g.By("Verify tailoredprofile name and status !!!\n")
		subD.getTailoredProfileNameandStatus(oc, tprofileD.name)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")
		g.By("Check complianceSuite result through exit-code.. !!!\n")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("Verify the rule status through compliancecheckresult and complianceremediations... !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-audit-tailored-audit-profile-set", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Applied", ok, []string{"complianceremediations",
			"ocp4-audit-tailored-audit-profile-set", "-n", subD.namespace, "-o=jsonpath={.status.applicationState}"}).check(oc)

		g.By("Verify the audit profile set value through complianceremediations... !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "WriteRequestBodies", ok, []string{"complianceremediations",
			"ocp4-audit-tailored-audit-profile-set", "-n", subD.namespace, "-o=jsonpath={.spec.current.object.spec.audit.profile}"}).check(oc)

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Rerun the scan and check result... !!")
		_, err := OcComplianceCLI().Run("rerun-now").Args("scansettingbinding", ssb.name, "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "COMPLIANT")

		g.By("Verify the rule status through compliancecheckresult and value through apiservers cluster object... !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-audit-tailored-audit-profile-set", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "WriteRequestBodies", ok, []string{"apiservers", "cluster",
			"-ojsonpath={.spec.audit.profile}"}).check(oc)

		g.By("The compliance operator supports remediation templating by setting custom variables in the tailored profile... !!!\n")
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Longduration-CPaasrunOnly-NonPreRelease-High-46100-High-54323-Verify autoremediations works for CIS profiles [Disruptive][Slow]", func() {
		//skip cluster when apiserver encryption type is eqauls to aescbc as enable/disable encryption is destructive and time consuming
		g.By("Check if cluster is Etcd Encryption On")
		skipEtcdEncryptionOff(oc)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  true,
				autoupdateremediations: true,
				name:                   "auto-rem-ss",
				namespace:              "",
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbCis = "cis-test" + getRandomString()
		)

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbCis, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ss", ss.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + ssbCis})
		}()
		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.machineCount}"}).check(oc)
		}()
		defer func() {
			g.By("Patch all complianceremediations to false .. !!!\n")
			patchPaused := fmt.Sprintf("{\"spec\":{\"paused\":true}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchPaused)
			setApplyToFalseForAllCrs(oc, subD.namespace, ssbCis)
			patchUnpaused := fmt.Sprintf("{\"spec\":{\"paused\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchUnpaused)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create scansetting... !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbCis, "-S", ss.name, "profile/ocp4-cis", "profile/ocp4-cis-node", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbCis, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbCis, subD.namespace, "DONE")
		subD.complianceSuiteResult(oc, ssbCis, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, contain, "COMPLIANT", ok, []string{"compliancescan",
			"ocp4-cis-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-cis", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)

		g.By("Check rules and remediation status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-cis-api-server-encryption-provider-cipher", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crResult, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crResult != "" {
			checkMachineConfigPoolStatus(oc, ss.roles1)
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbCis, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subD.complianceSuiteResult(oc, ssbCis, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, contain, "COMPLIANT", ok, []string{"compliancescan",
				"ocp4-cis-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
				"ocp4-cis", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		}

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check all rules with autoremations PASS !!!\n")
		result, err := oc.AsAdmin().Run("get").Args("ccr", "-n", subD.namespace, "-l", "compliance.openshift.io/automated-remediation=,compliance.openshift.io/check-status=FAIL").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).Should(o.MatchRegexp("No resources found.*"), fmt.Sprintf("%s is NOT expected result for the final result", result))
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Longduration-CPaasrunOnly-NonPreRelease-High-46995-Verify autoremediations works for PCI-DSS profiles [Disruptive][Slow]", func() {
		//skip cluster when apiserver encryption type is eqauls to aescbc as enable/disable encryption is destructive and time consuming
		g.By("Check if cluster is Etcd Encryption On")
		skipEtcdEncryptionOff(oc)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  true,
				autoupdateremediations: true,
				name:                   "auto-rem-ss",
				namespace:              "",
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbPci = "pci-test" + getRandomString()
		)

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbPci, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ss", ss.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + ssbPci})
		}()
		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.machineCount}"}).check(oc)
		}()
		defer func() {
			g.By("Patch all complianceremediations to false .. !!!\n")
			patchPaused := fmt.Sprintf("{\"spec\":{\"paused\":true}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchPaused)
			setApplyToFalseForAllCrs(oc, subD.namespace, ssbPci)
			patchUnpaused := fmt.Sprintf("{\"spec\":{\"paused\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchUnpaused)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create scansetting... !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbPci, "-S", ss.name, "profile/ocp4-pci-dss", "profile/ocp4-pci-dss-node", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbPci, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbPci, subD.namespace, "DONE")
		subD.complianceSuiteResult(oc, ssbPci, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, contain, "COMPLIANT", ok, []string{"compliancescan",
			"ocp4-pci-dss-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-pci-dss", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)

		g.By("Check rules and remediation status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-pci-dss-api-server-encryption-provider-cipher", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crResult, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crResult != "" {
			checkMachineConfigPoolStatus(oc, ss.roles1)
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbPci, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkComplianceSuiteStatus(oc, ssbPci, subD.namespace, "DONE")
			subD.complianceSuiteResult(oc, ssbPci, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, contain, "COMPLIANT", ok, []string{"compliancescan",
				"ocp4-pci-dss-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
				"ocp4-pci-dss", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		}

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check all rules with autoremations PASS !!!\n")
		result, err := oc.AsAdmin().Run("get").Args("ccr", "-n", subD.namespace, "-l", "compliance.openshift.io/automated-remediation=,compliance.openshift.io/check-status=FAIL").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).Should(o.MatchRegexp("No resources found.*"), fmt.Sprintf("%s is NOT expected result for the final result", result))
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-Longduration-CPaasrunOnly-NonPreRelease-High-50518-High-49698-Check remediation works for ocp4-high ocp4-high-node and rhcos4-high profiles [Disruptive][Slow]", func() {
		g.By("Check if precondition not matched!")
		skipEtcdEncryptionOff(oc)
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  true,
				autoupdateremediations: true,
				name:                   "auto-rem-ss",
				namespace:              "",
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbHigh = "high-test" + getRandomString()
		)

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)
		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbHigh, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ss", ss.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + ssbHigh})
		}()

		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.machineCount}"}).check(oc)
		}()
		defer func() {
			g.By("Patch all complianceremediations to false .. !!!\n")
			patchPaused := fmt.Sprintf("{\"spec\":{\"paused\":true}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchPaused)
			setApplyToFalseForAllCrs(oc, subD.namespace, ssbHigh)
			patchUnpaused := fmt.Sprintf("{\"spec\":{\"paused\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchUnpaused)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create scansetting... !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbHigh, "-S", ss.name, "profile/ocp4-high", "profile/ocp4-high-node", "profile/rhcos4-high", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbHigh, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbHigh, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssbHigh, "NON-COMPLIANT INCONSISTENT")

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crResult, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-l", "compliance.openshift.io/suite="+ssbHigh, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crResult != "" {
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crMissingDependencies, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l", "compliance.openshift.io/has-unmet-dependencies=",
			"-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crMissingDependencies != "" {
			g.By("Perform scan using oc-compliance plugin.. !!!\n")
			_, err := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbHigh, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssbHigh, "-n", subD.namespace,
				"-o=jsonpath={.status.phase}"}).check(oc)

			g.By("Check complianceSuite name and result.. !!!\n")
			subD.complianceSuiteResult(oc, ssbHigh, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger the third round of rescan if needed !!!\n")
		crMissingDependencies, err = oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l", "compliance.openshift.io/has-unmet-dependencies=",
			"-o=jsonpath={.items[*].metadata.name}").Output()
		e2e.Logf("The crMissingDependencies is: %s and the err is: %v", crMissingDependencies, err)
		//o.Expect(len(crMissingDependencies)).Should(o.BeNumerically("==", 0))
		if crResult != "" {
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbHigh, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkComplianceSuiteStatus(oc, ssbHigh, subD.namespace, "DONE")
			subD.complianceSuiteResult(oc, ssbHigh, "NON-COMPLIANT INCONSISTENT")
		}

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check rules and remediation status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-high", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-high-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"rhcos4-high-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-high-oauth-or-oauthclient-inactivity-timeout", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-high-node-wrscan-kubelet-enable-protect-kernel-defaults", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-high-node-wrscan-kubelet-enable-protect-kernel-sysctl", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		result, _ := oc.AsAdmin().Run("get").Args("ccr", "-n", subD.namespace, "-l", "compliance.openshift.io/automated-remediation=,compliance.openshift.io/check-status=FAIL").Output()
		if !strings.Contains(result, "No resources found") {
			e2e.Failf("%s is NOT expected result for the final result", result)
		}
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-Author:xiyuan-Longduration-CPaasrunOnly-NonPreRelease-High-47051-Verify the autoremediations works for ocp4-nerc-cip, ocp4-nerc-cip-node and rhcos4-nerc-cip profiles [Disruptive][Slow]", func() {
		g.By("Check if cluster is Etcd Encryption On")
		skipEtcdEncryptionOff(oc)

		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  true,
				autoupdateremediations: true,
				name:                   "auto-rem-ss",
				namespace:              "",
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbNercCip = "nerc-cip-test" + getRandomString()
		)

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)
		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbNercCip, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ss", ss.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + ssbNercCip})
		}()
		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.machineCount}"}).check(oc)
		}()
		defer func() {
			g.By("Patch all complianceremediations to false .. !!!\n")
			patchPaused := fmt.Sprintf("{\"spec\":{\"paused\":true}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchPaused)
			setApplyToFalseForAllCrs(oc, subD.namespace, ssbNercCip)
			patchUnpaused := fmt.Sprintf("{\"spec\":{\"paused\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchUnpaused)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create scansetting... !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbNercCip, "-S", ss.name, "profile/ocp4-nerc-cip", "profile/ocp4-nerc-cip-node", "profile/rhcos4-nerc-cip", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNercCip, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbNercCip, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssbNercCip, "NON-COMPLIANT INCONSISTENT")
		crResult, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-l", "compliance.openshift.io/suite="+ssbNercCip, "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crResult != "" {
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crMissingDependencies, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l",
			"compliance.openshift.io/has-unmet-dependencies=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crMissingDependencies != "" {
			g.By("Performing second scan using oc-compliance plugin.. !!!\n")
			_, err := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbNercCip, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssbNercCip, "-n", subD.namespace,
				"-o=jsonpath={.status.phase}"}).check(oc)

			g.By("Check complianceSuite name and result.. !!!\n")
			subD.complianceSuiteResult(oc, ssbNercCip, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger the third round of rescan if needed !!!\n")
		crMissingDependencies, err = oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l",
			"compliance.openshift.io/has-unmet-dependencies=", "-o=jsonpath={.items[*].metadata.name}").Output()
		e2e.Logf("The crMissingDependencies is: %s and the err is: %v", crMissingDependencies, err)
		o.Expect(len(crMissingDependencies)).Should(o.BeNumerically("==", 0))
		if crResult != "" {
			checkMachineConfigPoolStatus(oc, ss.roles1)
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbNercCip, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkComplianceSuiteStatus(oc, ssbNercCip, subD.namespace, "DONE")
			subD.complianceSuiteResult(oc, ssbNercCip, "NON-COMPLIANT INCONSISTENT")
		}

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check rules and remediation status !!!\n")
		checkComplianceSuiteStatus(oc, ssbNercCip, subD.namespace, "DONE")
		subD.complianceSuiteResult(oc, ssbNercCip, "NON-COMPLIANT INCONSISTENT")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-nerc-cip-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-nerc-cip", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"rhcos4-nerc-cip-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)

		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-nerc-cip-oauth-or-oauthclient-inactivity-timeout", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-nerc-cip-node-wrscan-kubelet-enable-protect-kernel-defaults", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-nerc-cip-node-wrscan-kubelet-enable-protect-kernel-sysctl", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"rhcos4-nerc-cip-wrscan-banner-etc-issue", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)

		result, _ := oc.AsAdmin().Run("get").Args("ccr", "-n", subD.namespace, "-l", "compliance.openshift.io/automated-remediation=,compliance.openshift.io/check-status=FAIL").Output()
		if !strings.Contains(result, "No resources found") {
			e2e.Failf("%s is NOT expected result for the final result", result)
		}
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-Longduration-CPaasrunOnly-NonPreRelease-High-71274-High-41010-Verify the autoremediations works for ocp4-moderate ocp4-moderate-node and rhcos4-moderate profiles [Disruptive][Slow]", func() {
		g.By("Check if cluster is Etcd Encryption On")
		skipEtcdEncryptionOff(oc)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  true,
				autoupdateremediations: true,
				name:                   "auto-rem-ss",
				namespace:              "",
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbModerate = "moderate-test" + getRandomString()
		)

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbModerate, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ss", ss.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + ssbModerate})
		}()

		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.machineCount}"}).check(oc)
		}()
		defer func() {
			g.By("Patch all complianceremediations to false .. !!!\n")
			patchPaused := fmt.Sprintf("{\"spec\":{\"paused\":true}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchPaused)
			setApplyToFalseForAllCrs(oc, subD.namespace, ssbModerate)
			patchUnpaused := fmt.Sprintf("{\"spec\":{\"paused\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchUnpaused)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create scansetting... !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbModerate, "-S", ss.name, "profile/ocp4-moderate", "profile/ocp4-moderate-node", "profile/rhcos4-moderate", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbModerate, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbModerate, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssbModerate, "NON-COMPLIANT INCONSISTENT")

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crResult, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-l", "compliance.openshift.io/suite="+ssbModerate, "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crResult != "" {
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crMissingDependencies, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l", "compliance.openshift.io/has-unmet-dependencies=",
			"-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crMissingDependencies != "" {
			g.By("Performing second scan using oc-compliance plugin.. !!!\n")
			_, err := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbModerate, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssbModerate, "-n", subD.namespace,
				"-o=jsonpath={.status.phase}"}).check(oc)

			g.By("Check complianceSuite name and result.. !!!\n")
			subD.complianceSuiteResult(oc, ssbModerate, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger the third round of rescan if needed !!!\n")
		crMissingDependencies, err = oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l", "compliance.openshift.io/has-unmet-dependencies=",
			"-o=jsonpath={.items[*].metadata.name}").Output()
		e2e.Logf("The crMissingDependencies is: %s and the err is: %v", crMissingDependencies, err)
		o.Expect(len(crMissingDependencies)).Should(o.BeNumerically("==", 0))

		if crResult != "" {
			checkMachineConfigPoolStatus(oc, ss.roles1)
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbModerate, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkComplianceSuiteStatus(oc, ssbModerate, subD.namespace, "DONE")
			subD.complianceSuiteResult(oc, ssbModerate, "NON-COMPLIANT INCONSISTENT")
		}

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check rules and remediation status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-moderate", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-moderate-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"rhcos4-moderate-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-audit-profile-set", "-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		result, _ := oc.AsAdmin().Run("get").Args("ccr", "-n", subD.namespace, "-l", "compliance.openshift.io/automated-remediation=,compliance.openshift.io/check-status=FAIL").Output()
		if !strings.Contains(result, "No resources found") {
			e2e.Failf("%s is NOT expected result for the final result", result)
		}
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-Author:pdhamdhe-Longduration-CPaasrunOnly-NonPreRelease-High-47147-Check rules work if all non-openshift namespaces has resourcequota and route rate limit [Disruptive][Slow]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "moderate-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-moderate",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}

		defer func() {
			g.By("Remove route and resourcequota objects from all non-control plane namespace.. !!!\n")
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})
			nsName := "ns-47147-1"
			cleanupObjects(oc, objectTableRef{"route", nsName, "wordpress"})
			cleanupObjects(oc, objectTableRef{"ns", nsName, nsName})
			newCheck("expect", asAdmin, withoutNamespace, contain, "wordpress", nok, []string{"route", "-n", nsName, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			nonControlNamespacesList := getNonControlNamespaces(oc, "Active")
			for _, v := range nonControlNamespacesList {
				cleanupObjects(oc, objectTableRef{"resourcequota", v, "example"})
				newCheck("expect", asAdmin, withoutNamespace, contain, "example", nok, []string{"resourcequota", "-n", v, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			}
		}()

		g.By("Check for default profile 'ocp4-moderate' .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		g.By("Create route in non-control namespace .. !!!\n")
		nsName := "ns-47147-1"
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", nsName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err1 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", nsName, "-f", wordpressRouteYAML).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "wordpress", ok, []string{"route", "-n", nsName, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify rules status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-moderate-resource-requests-quota", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
			"ocp4-moderate-routes-rate-limit", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)

		g.By("Annotate route in non-control namespace.. !!!\n")
		_, err2 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("-n", nsName, "route", "wordpress", "haproxy.router.openshift.io/rate-limit-connections=true").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())

		nonControlNamespacesTerList1 := getNonControlNamespaces(oc, "Terminating")
		if len(nonControlNamespacesTerList1) == 0 {
			nonControlNamespacesList := getNonControlNamespaces(oc, "Active")
			e2e.Logf("Here namespace : %v\n", nonControlNamespacesList)
			g.By("Create resourcequota in all non-control plane namespace .. !!!\n")
			for _, v := range nonControlNamespacesList {
				_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", v, "-f", resourceQuotaYAML).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				newCheck("expect", asAdmin, withoutNamespace, contain, "example", ok, []string{"resourcequota", "-n", v,
					"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			}
		}

		g.By("Rerun scan using oc-compliance plugin.. !!!\n")
		_, err3 := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssb.name, "-n", ssb.namespace).Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Verify rules status through compliancecheck result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-resource-requests-quota", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"ocp4-moderate-routes-rate-limit", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:pdhamdhe-Medium-47173-Verify the rule that check for API Server audit error alerts [Serial][Slow]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "ocp4-moderate-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-moderate",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}

		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})

		g.By("Check for default profile 'ocp4-moderate' .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate")

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Check if audit prometheusrules is already available.. !!!\n")
		prometheusrules, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheusrules", "-n", "openshift-kube-apiserver", "-ojsonpath={.items[*].metadata.name}").Output()
		if strings.Contains(prometheusrules, "audit-errors") {
			newCheck("expect", asAdmin, withoutNamespace, contain, "audit-errors", ok, []string{"prometheusrules", "-n", "openshift-kube-apiserver",
				"-ojsonpath={.items[*].metadata.name}"}).check(oc)
			g.By("Verify audit rule status through compliancecheck result.. !!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
				"ocp4-moderate-audit-error-alert-exists", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		} else {
			g.By("Verify audit rule status through compliancecheck result.. !!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult",
				"ocp4-moderate-audit-error-alert-exists", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
			g.By("Create the prometheusrules for audit.. !!!\n")
			_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-kube-apiserver", "-f", prometheusAuditRuleYAML).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "audit-errors", ok, []string{"prometheusrules", "-n", "openshift-kube-apiserver",
				"-ojsonpath={.items[*].metadata.name}"}).check(oc)
			defer cleanupObjects(oc, objectTableRef{"prometheusrules", "openshift-kube-apiserver", "audit-errors"})

			g.By("Rerun scan using oc-compliance plugin.. !!!\n")
			_, err1 := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssb.name, "-n", ssb.namespace).Output()
			o.Expect(err1).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
				"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
			g.By("Check ComplianceSuite status and result.. !!!\n")
			assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
			subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")
			g.By("Verify audit rule status through compliancecheck result.. !!!\n")
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
				"ocp4-moderate-audit-error-alert-exists", "-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		}
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:pdhamdhe-Medium-47373-Low-47371-Enable TailoredProfiles without extending a Profile and also validate that title and description [Serial][Slow]", func() {
		var (
			tprofileDN = tailoredProfileWithoutVarDescription{
				name:         "new-profile-node",
				namespace:    "",
				extends:      "",
				title:        "new profile with node rules from scratch",
				description:  "new profile with node rules",
				enrulename1:  "ocp4-file-groupowner-cni-conf",
				rationale1:   "Node",
				enrulename2:  "ocp4-accounts-restrict-service-account-tokens",
				rationale2:   "None",
				disrulename1: "ocp4-file-groupowner-etcd-data-dir",
				drationale1:  "Node",
				disrulename2: "ocp4-accounts-unique-service-account",
				drationale2:  "None",
				template:     tprofileWithoutVarTemplate,
			}
			tprofileDP = tailoredProfileWithoutVarDescription{
				name:         "new-profile-platform",
				namespace:    "",
				extends:      "",
				title:        "new profile with platform rules from scratch",
				description:  "new profile with platform rules",
				enrulename1:  "ocp4-api-server-admission-control-plugin-alwaysadmit",
				rationale1:   "Platform",
				enrulename2:  "ocp4-accounts-restrict-service-account-tokens",
				rationale2:   "None",
				disrulename1: "ocp4-api-server-admission-control-plugin-alwayspullimages",
				drationale1:  "Platform",
				disrulename2: "ocp4-configure-network-policies",
				drationale2:  "None",
				template:     tprofileWithoutVarTemplate,
			}
			tprofileDNP = tailoredProfileWithoutVarDescription{
				name:         "new-profile-both",
				namespace:    "",
				extends:      "",
				title:        "new profile with both checktype rules from scratch",
				description:  "new profile with both checkType rules",
				enrulename1:  "ocp4-file-groupowner-cni-conf",
				rationale1:   "Node",
				enrulename2:  "ocp4-api-server-admission-control-plugin-alwayspullimages",
				rationale2:   "Platform",
				disrulename1: "ocp4-file-groupowner-etcd-data-dir",
				drationale1:  "Node",
				disrulename2: "ocp4-api-server-admission-control-plugin-alwaysadmit",
				drationale2:  "Platform",
				template:     tprofileWithoutVarTemplate,
			}
			ssbN = scanSettingBindingDescription{
				name:            "tailor-profile-node" + getRandomString(),
				namespace:       "",
				profilekind1:    "TailoredProfile",
				profilename1:    "new-profile-node",
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
			ssbP = scanSettingBindingDescription{
				name:            "tailor-profile-platform" + getRandomString(),
				namespace:       "",
				profilekind1:    "TailoredProfile",
				profilename1:    "new-profile-platform",
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
		)

		var errmsg = "didn't match expected type"

		defer func() {
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbN.name})
			cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbP.name})
			cleanupObjects(oc, objectTableRef{"tailoredprofile", subD.namespace, tprofileDN.name})
			cleanupObjects(oc, objectTableRef{"tailoredprofile", subD.namespace, tprofileDP.name})
			cleanupObjects(oc, objectTableRef{"tailoredprofile", subD.namespace, tprofileDNP.name})
		}()

		g.By("Create tailoredprofiles !!!\n")
		tprofileDN.namespace = subD.namespace
		tprofileDP.namespace = subD.namespace
		tprofileDNP.namespace = subD.namespace
		tprofileDN.create(oc)
		tprofileDP.create(oc)
		tprofileDNP.create(oc)
		subD.getTailoredProfileNameandStatus(oc, tprofileDN.name)
		subD.getTailoredProfileNameandStatus(oc, tprofileDP.name)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ERROR", ok, []string{"tailoredprofile", tprofileDNP.name, "-n", tprofileDNP.namespace,
			"-o=jsonpath={.status.state}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, errmsg, ok, []string{"tailoredprofile", tprofileDNP.name, "-n", tprofileDNP.namespace,
			"-o=jsonpath={.status.errorMessage}"}).check(oc)

		errorMsg := []string{"The TailoredProfile \"profile-description\" is invalid: spec.description: Required value", "The TailoredProfile \"profile-title\" is invalid: spec.title: Required value"}
		verifyTailoredProfile(oc, errorMsg, subD.namespace, tprofileWithoutDescriptionYAML)
		verifyTailoredProfile(oc, errorMsg, subD.namespace, tprofileWithoutTitleYAML)

		g.By("Create scansettingbindings !!!\n")
		ssbN.namespace = subD.namespace
		ssbP.namespace = subD.namespace
		ssbN.create(oc)
		ssbP.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbN.name, ok, []string{"scansettingbinding", "-n", ssbN.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbP.name, ok, []string{"scansettingbinding", "-n", ssbP.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbN.name, "-n", ssbN.namespace, "-o=jsonpath={.status.phase}")
		g.By("Check complianceSuite name and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbP.name, "-n", ssbP.namespace, "-o=jsonpath={.status.phase}")
		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssbN.name, "COMPLIANT")
		subD.complianceSuiteResult(oc, ssbP.name, "COMPLIANT")

		g.By("Verify the rule status through compliancecheckresult... !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"new-profile-node-master-file-groupowner-cni-conf", "-n", ssbN.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "MANUAL", ok, []string{"compliancecheckresult",
			"new-profile-node-worker-accounts-restrict-service-account-tokens", "-n", ssbN.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult",
			"new-profile-platform-api-server-admission-control-plugin-alwaysadmit", "-n", ssbP.namespace, "-o=jsonpath={.status}"}).check(oc)
	})

	// author: pdhamdhe@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:pdhamdhe-Medium-47148-Check file and directory permissions for apiserver audit logs [Serial][Slow]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "ocp4-moderate-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-moderate-node",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})

		g.By("Check for default profile 'ocp4-moderate-node' .. !!!\n")
		subD.getProfileName(oc, "ocp4-moderate-node")

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		auditRules := []string{"master-directory-permissions-var-log-kube-audit", "master-directory-permissions-var-log-oauth-audit", "master-directory-permissions-var-log-ocp-audit",
			"master-file-ownership-var-log-kube-audit", "master-file-ownership-var-log-oauth-audit", "master-file-ownership-var-log-ocp-audit", "master-file-permissions-var-log-kube-audit",
			"master-file-permissions-var-log-oauth-audit", "master-file-permissions-var-log-ocp-audit"}

		g.By("Check audit rules status !!!\n")
		getRuleStatus(oc, auditRules, "PASS", ssb.profilename1, ssb.namespace)
		g.By("Get one master node.. !!!\n")
		masterNodeName := getOneMasterNodeName(oc)
		contList1 := []string{"drwx------.  2 root        root ", "kube-apiserver"}
		contList2 := []string{"drwx------.  2 root        root ", "oauth-apiserver"}
		contList3 := []string{"drwx------.  2 root        root ", "openshift-apiserver"}
		contList4 := []string{"-rw-------", "audit.log"}
		checkNodeContents(oc, masterNodeName, ssb.namespace, contList1, "ls", "-l", "/var/log", "kube-apiserver")
		checkNodeContents(oc, masterNodeName, ssb.namespace, contList2, "ls", "-l", "/var/log", "oauth-apiserver")
		checkNodeContents(oc, masterNodeName, ssb.namespace, contList3, "ls", "-l", "/var/log", "openshift-apiserver")
		checkNodeContents(oc, masterNodeName, ssb.namespace, contList4, "ls", "-l", "/var/log/kube-apiserver", "audit.log")
		checkNodeContents(oc, masterNodeName, ssb.namespace, contList4, "ls", "-l", "/var/log/oauth-apiserver", "audit.log")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:xiyuan-Medium-47162-Check if the XCCDF variable values are getting render in the compliance rules", func() {
		keywordsInstr := "min-request-timeout.*3600"
		keywordsAnnot := "compliance.openshift.io/rule-variable.*var-api-min-request-timeout"
		keywordsVariableValue := "3600"
		assertKeywordsExists(oc, 10, keywordsInstr, "rule", "ocp4-api-server-request-timeout", "-o=jsonpath={.description}", "-n", subD.namespace)
		assertKeywordsExists(oc, 10, keywordsAnnot, "rule", "ocp4-api-server-request-timeout", "-o=jsonpath={.metadata.annotations}", "-n", subD.namespace)
		assertKeywordsExists(oc, 10, keywordsVariableValue, "variables", "ocp4-var-api-min-request-timeout", "-o=jsonpath={.value}", "-n", subD.namespace)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-ConnectedOnly-NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Medium-48643-Check if the prometheusRule that verifies the Compliance alerts [Serial]", func() {
		// skip test if telemetry not found
		skipNotelemetryFound(oc)

		var (
			ssb = scanSettingBindingDescription{
				name:            "pci-test",
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-pci-dss",
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
		)
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssb.name})

		newCheck("expect", asAdmin, withoutNamespace, contain, "openshift.io/cluster-monitoring", ok, []string{"namespace", subD.namespace, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "metrics", ok, []string{"service", "-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssb.name, subD.namespace, "DONE")
		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("check alerts")
		alertString := "The compliance suite pci-test returned as NON-COMPLIANT, ERROR, or INCONSISTENT"
		checkAlert(oc, ssb.name, alertString, 300)
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ARO-OSD_CCS-Author:xiyuan-Medium-53303-create a scansettingbinding with or without priorityClass defined in scansetting [Serial]", func() {
		var (
			priorityClassTemplate = filepath.Join(buildPruningBaseDir, "priorityclass.yaml")
			prioritym             = priorityClassDescription{
				name:         "prioritym" + getRandomString(),
				namespace:    subD.namespace,
				prirotyValue: 99,
				template:     priorityClassTemplate,
			}
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "ss-with-pc" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				roles2:                 "worker",
				rotation:               5,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				priorityclassname:      prioritym.name,
				debug:                  true,
				suspend:                false,
				template:               scansettingTemplate,
			}
			ssbWithPC    = "ssb-with-pc-" + getRandomString()
			ssbWithoutPC = "ssb-without-pc-" + getRandomString()
		)

		defer func() {
			cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbWithPC})
			cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbWithoutPC})
			cleanupObjects(oc, objectTableRef{"priorityclass", subD.namespace, prioritym.name})
			cleanupObjects(oc, objectTableRef{"scansetting", subD.namespace, ss.name})
		}()

		g.By("Create priorityclass !!!\n")
		prioritym.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, prioritym.name, ok, []string{"priorityclass", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbWithPC, "-S", ss.name, "profile/ocp4-cis", "profile/ocp4-cis-node", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbWithPC, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbWithPC, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbWithPC, "NON-COMPLIANT INCONSISTENT")
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.priorityclassname, ok, []string{"compliancesuite", ssbWithPC, "-n", subD.namespace,
			"-o=jsonpath={.spec.scans[0].priorityClass}"}).check(oc)

		g.By("Check priorityname and priority for pods.. !!!\n")
		assertParameterValueForBulkPods(oc, ss.priorityclassname, "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, strconv.Itoa(prioritym.prirotyValue), "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
		assertParameterValueForBulkPods(oc, ss.priorityclassname, "pod", "-l", "workload=aggregator", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, strconv.Itoa(prioritym.prirotyValue), "pod", "-l", "workload=aggregator", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")

		g.By("Remove ssb and patch ss !!!\n")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbWithPC, "-n", subD.namespace, "--ignore-not-found").Execute()
		patch := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/priorityClass\"}]")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "-n", subD.namespace, "--type", "json", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"scansetting", ss.name, "-n", subD.namespace,
			"-o=jsonpath={.priorityClass}"}).check(oc)

		g.By("Create scansettingbinding without priorityClass !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbWithoutPC, "-S", ss.name, "profile/ocp4-cis", "profile/ocp4-cis-node", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbWithoutPC, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbWithoutPC, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbWithoutPC, "NON-COMPLIANT INCONSISTENT")
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"compliancesuite", ssbWithoutPC, "-n", subD.namespace,
			"-o=jsonpath={.spec.scans[0].priorityClass}"}).check(oc)

		g.By("Check priorityname and priority for pods.. !!!\n")
		assertParameterValueForBulkPods(oc, "", "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, "0", "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
		assertParameterValueForBulkPods(oc, "", "pod", "-l", "workload=aggregator", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, "0", "pod", "-l", "workload=aggregator", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ARO-OSD_CCS-Author:xiyuan-Medium-53301-create a compliancesuite with or without priorityClass", func() {
		var (
			priorityClassTemplate = filepath.Join(buildPruningBaseDir, "priorityclass.yaml")
			prioritym             = priorityClassDescription{
				name:         "prioritym" + getRandomString(),
				namespace:    subD.namespace,
				prirotyValue: 99,
				template:     priorityClassTemplate,
			}
			csuite = complianceSuiteDescription{
				name:          "suite-with-pc" + getRandomString(),
				namespace:     subD.namespace,
				scanname:      "cis-scan" + getRandomString(),
				profile:       "xccdf_org.ssgproject.content_profile_cis",
				content:       "ssg-ocp4-ds.xml",
				contentImage:  relatedImageProfile,
				nodeSelector:  "master",
				debug:         true,
				priorityClass: prioritym.name,
				template:      csuiteTemplate,
			}
			csuiteWithoutPC = complianceSuiteDescription{
				name:          "suite-without-pc" + getRandomString(),
				namespace:     subD.namespace,
				scanname:      "cis-scan" + getRandomString(),
				profile:       "xccdf_org.ssgproject.content_profile_cis",
				content:       "ssg-ocp4-ds.xml",
				contentImage:  relatedImageProfile,
				nodeSelector:  "master",
				debug:         true,
				priorityClass: "",
				template:      csuiteTemplate,
			}
		)

		defer func() {
			cleanupObjects(oc, objectTableRef{"suite", subD.namespace, csuite.name})
			cleanupObjects(oc, objectTableRef{"suite", subD.namespace, csuiteWithoutPC.name})
			cleanupObjects(oc, objectTableRef{"priorityclass", subD.namespace, prioritym.name})
		}()

		g.By("Create priorityclass !!!\n")
		prioritym.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, prioritym.name, ok, []string{"priorityclass", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create compliancesuite and check compliancesuite status !!!\n")
		csuite.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuite.name, ok, []string{"compliancesuite", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuite.name, "-n", csuite.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuite.name, "NON-COMPLIANT")

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuite.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuite.name, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, contain, prioritym.name, ok, []string{"compliancesuite", csuite.name, "-n", subD.namespace,
			"-o=jsonpath={.spec.scans[0].priorityClass}"}).check(oc)

		g.By("Check priorityname and priority for pods.. !!!\n")
		assertParameterValueForBulkPods(oc, prioritym.name, "pod", "-l", "compliance.openshift.io/scan-name="+csuite.scanname+",workload=scanner", "-n", subD.namespace,
			"-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, strconv.Itoa(prioritym.prirotyValue), "pod", "-l", "compliance.openshift.io/scan-name="+csuite.scanname+",workload=scanner",
			"-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
		assertParameterValueForBulkPods(oc, prioritym.name, "pod", "-l", "compliance.openshift.io/scan-name="+csuite.scanname+",workload=aggregator", "-n", subD.namespace,
			"-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, strconv.Itoa(prioritym.prirotyValue), "pod", "-l", "compliance.openshift.io/scan-name="+csuite.scanname+",workload=aggregator",
			"-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")

		g.By("Create compliancesuite without priorityclass and check compliancesuite status !!!\n")
		csuiteWithoutPC.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuiteWithoutPC.name, ok, []string{"compliancesuite", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteWithoutPC.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuite.name, "NON-COMPLIANT")

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuiteWithoutPC.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuiteWithoutPC.name, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"compliancesuite", csuiteWithoutPC.name, "-n", subD.namespace,
			"-o=jsonpath={.spec.scans[0].priorityClass}"}).check(oc)

		g.By("Check priorityname and priority for pods.. !!!\n")
		assertParameterValueForBulkPods(oc, "", "pod", "-l", "compliance.openshift.io/scan-name="+csuiteWithoutPC.scanname+",workload=scanner", "-n", subD.namespace,
			"-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, "0", "pod", "-l", "compliance.openshift.io/scan-name="+csuiteWithoutPC.scanname+",workload=scanner",
			"-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
		assertParameterValueForBulkPods(oc, "", "pod", "-l", "compliance.openshift.io/scan-name="+csuiteWithoutPC.scanname+",workload=aggregator", "-n", subD.namespace,
			"-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, "0", "pod", "-l", "compliance.openshift.io/scan-name="+csuiteWithoutPC.scanname+",workload=aggregator",
			"-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ARO-OSD_CCS-Author:xiyuan-Medium-53321-create a compliancesuite/scansettingbinding with a non-exist priorityClass [Serial]", func() {
		var (
			priorityClassName = "pc-not-exists" + getRandomString()
			csuite            = complianceSuiteDescription{
				name:          "suite-with-pc" + getRandomString(),
				namespace:     subD.namespace,
				scanname:      "cis-scan" + getRandomString(),
				profile:       "xccdf_org.ssgproject.content_profile_cis",
				content:       "ssg-ocp4-ds.xml",
				contentImage:  relatedImageProfile,
				nodeSelector:  "master",
				debug:         true,
				priorityClass: priorityClassName,
				template:      csuiteTemplate,
			}
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "ss-with-pc" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				roles2:                 "worker",
				rotation:               5,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				priorityclassname:      priorityClassName,
				debug:                  true,
				suspend:                false,
				template:               scansettingTemplate,
			}
			ssbWithPC = "ssb-with-pc-" + getRandomString()
		)

		defer func() {
			cleanupObjects(oc, objectTableRef{"suite", subD.namespace, csuite.name})
			cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbWithPC})
			cleanupObjects(oc, objectTableRef{"ss", subD.namespace, ss.name})
		}()

		g.By("Check priorityclass not exist !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"priorityclass", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create compliancesuite and check compliancesuite status !!!\n")
		csuite.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, csuite.name, ok, []string{"compliancesuite", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuite.name, "-n", csuite.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuite.name, "NON-COMPLIANT")

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", csuite.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, csuite.name, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"compliancesuite", csuite.name, "-n", subD.namespace,
			"-o=jsonpath={.spec.scans[0].priorityClass}"}).check(oc)

		g.By("Check event message.. !!!\n")
		commonMessage := ".*Error while getting priority class.*" + priorityClassName + ".*"
		assertEventMessageRegexpMatch(oc, commonMessage, "event", "-n", subD.namespace, "--field-selector", "involvedObject.name="+csuite.name+",involvedObject.kind=ComplianceSuite", "-o=jsonpath={.items[*].message}")
		assertEventMessageRegexpMatch(oc, commonMessage, "event", "-n", subD.namespace, "--field-selector", "involvedObject.name="+csuite.scanname+",involvedObject.kind=ComplianceScan", "-o=jsonpath={.items[*].message}")

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbWithPC, "-S", ss.name, "profile/ocp4-cis", "profile/ocp4-cis-node", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbWithPC, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbWithPC, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbWithPC, "NON-COMPLIANT INCONSISTENT")
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.priorityclassname, ok, []string{"compliancesuite", ssbWithPC, "-n", subD.namespace,
			"-o=jsonpath={.spec.scans[0].priorityClass}"}).check(oc)

		g.By("Check priorityname and priority for pods.. !!!\n")
		assertParameterValueForBulkPods(oc, "", "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, "0", "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
		assertParameterValueForBulkPods(oc, "", "pod", "-l", "workload=aggregator", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, "0", "pod", "-l", "workload=aggregator", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")

		g.By("Check event message for ssb.. !!!\n")
		assertEventMessageRegexpMatch(oc, commonMessage, "event", "-n", subD.namespace, "--field-selector", "involvedObject.name="+ssbWithPC+",involvedObject.kind=ComplianceSuite", "-o=jsonpath={.items[*].message}")
		assertEventMessageRegexpMatch(oc, commonMessage, "event", "-n", subD.namespace, "--field-selector", "involvedObject.name=ocp4-cis,involvedObject.kind=ComplianceScan", "-o=jsonpath={.items[*].message}")
		assertEventMessageRegexpMatch(oc, commonMessage, "event", "-n", subD.namespace, "--field-selector", "involvedObject.name=ocp4-cis-node-master,involvedObject.kind=ComplianceScan", "-o=jsonpath={.items[*].message}")
		assertEventMessageRegexpMatch(oc, commonMessage, "event", "-n", subD.namespace, "--field-selector", "involvedObject.name=ocp4-cis-node-worker,involvedObject.kind=ComplianceScan", "-o=jsonpath={.items[*].message}")
	})

	// author: xiyuan@redhat.com
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-ROSA-ARO-OSD_CCS-Author:xiyuan-High-60854-High-60864-Check the scan for ocp4-cis/ocp4-pci-dss with tailored profile works for hypershift cluster [Serial][Slow]", func() {
		var (
			tprofileCis = tailoredProfileDescription{
				name:      "hypershift-cis" + getRandomString(),
				namespace: subD.namespace,
				extends:   "ocp4-cis",
				value:     "",
				template:  tprofileHypershfitTemplate,
			}
			tprofilePcidss = tailoredProfileDescription{
				name:      "hypershift-pci-dss" + getRandomString(),
				namespace: subD.namespace,
				extends:   "ocp4-pci-dss",
				value:     "",
				template:  tprofileHypershfitTemplate,
			}
			ssbCis                   = "test-cis" + getRandomString()
			ssbPcidss                = "test-pci-dss" + getRandomString()
			ccrsCisShouldFailList    = []string{tprofileCis.name + "-audit-log-forwarding-webhook"}
			ccrsPcidssShouldFailList = []string{tprofilePcidss.name + "-audit-log-forwarding-webhook"}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssbCis},
			objectTableRef{"scansettingbinding", subD.namespace, ssbPcidss},
			objectTableRef{"tailoredprofile", subD.namespace, tprofileCis.name},
			objectTableRef{"tailoredprofile", subD.namespace, tprofilePcidss.name})

		g.By("Create the name of hosted cluster !!!\n")
		hypershift_namespace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "-l", "hypershift.openshift.io/hosted-control-plane=true", "-n", subD.namespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		hostedClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedcluster", "-A", "-n", subD.namespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		subString := "-" + hostedClusterName
		hypershift_namespace_prefix := strings.TrimSuffix(hypershift_namespace, subString)

		g.By("Create the tailoredprofiels !!!\n")
		tprofileCis.value = hostedClusterName
		tprofilePcidss.value = hostedClusterName
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tprofileCis.template, "-p", "NAME="+tprofileCis.name, "NAMESPACE="+tprofileCis.namespace,
			"EXTENDS="+tprofileCis.extends, "VALUE="+tprofileCis.value, "HYPERSHIFT_NAMESPACE_PREFIX="+hypershift_namespace_prefix)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", tprofilePcidss.template, "-p", "NAME="+tprofilePcidss.name, "NAMESPACE="+tprofilePcidss.namespace,
			"EXTENDS="+tprofilePcidss.extends, "VALUE="+tprofilePcidss.value, "HYPERSHIFT_NAMESPACE_PREFIX="+hypershift_namespace_prefix)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"tp", tprofileCis.name, "-n", subD.namespace, "-o=jsonpath={.status.state}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"tp", tprofilePcidss.name, "-n", subD.namespace, "-o=jsonpath={.status.state}"}).check(oc)

		g.By("Rerun scan and check ComplianceSuite status & result.. !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbCis, "-n", subD.namespace, "tailoredprofile/"+tprofileCis.name).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbPcidss, "-n", subD.namespace, "tailoredprofile/"+tprofilePcidss.name).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbCis, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbPcidss, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertKeywordsExists(oc, 400, "DONE", "compliancesuite", ssbCis, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		assertKeywordsExists(oc, 400, "DONE", "compliancesuite", ssbPcidss, "-o=jsonpath={.status.phase}", "-n", subD.namespace)

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssbCis, "NON-COMPLIANT")
		subD.complianceSuiteResult(oc, ssbPcidss, "NON-COMPLIANT")

		g.By("Verify the failed ccr number and failed ccr list.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, compare, "PASS", ok, []string{"compliancecheckresult", tprofileCis.name + "-api-server-encryption-provider-cipher",
			"-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "PASS", ok, []string{"compliancecheckresult", tprofilePcidss.name + "-api-server-encryption-provider-cipher",
			"-n", subD.namespace, "-o=jsonpath={.status}"}).check(oc)
		ccrCisFailList, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", "-l", "compliance.openshift.io/check-status=FAIL,"+"compliance.openshift.io/suite="+ssbCis,
			"-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		ccrPcidssFailList, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("compliancecheckresult", "-l", "compliance.openshift.io/check-status=FAIL,"+"compliance.openshift.io/suite="+ssbPcidss,
			"-n", subD.namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		for _, ccrCis := range ccrsCisShouldFailList {
			o.Expect(strings.Fields(ccrCisFailList)).Should(o.ContainElement(ccrCis), fmt.Sprintf("The %s NOT in the failed ccr list %s", ccrCis, ccrCisFailList))

		}
		for _, ccrPcidss := range ccrsPcidssShouldFailList {
			o.Expect(strings.Fields(ccrPcidssFailList)).Should(o.ContainElement(ccrPcidss), fmt.Sprintf("The %s NOT in the failed ccr list %s", ccrPcidss, ccrPcidssFailList))

		}
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-Author:xiyuan-High-54055-check compliance operator could be deleted successfully [Serial]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "cis-test" + getRandomString(),
			namespace:       "",
			profilekind1:    "Profile",
			profilename1:    "ocp4-cis",
			scansettingname: "default",
			template:        scansettingbindingSingleTemplate,
		}

		defer func() {
			g.By("delete Compliance Operator !!!")
			deleteNamespace(oc, subD.namespace)
			g.By("the Compliance Operator is deleted successfully !!!")
		}()

		defer func() {
			g.By("Check node labels and the ssb/suite/scan/pod/pb are deleted sucessfully !!!")
			setLabelToNode(oc, "node-role.kubernetes.io/wscan-")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", "--all", "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("suite", "--all", "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("scan", "--all", "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pb", "--all", "-n", subD.namespace, "--ignore-not-found").Execute()
		}()

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Author:xiyuan-Medium-55355-Check the operator's resources limit is configurable [Serial]", func() {
		g.By("Check the default resource requirements for compliance operator pod !!!\n")
		assertEventMessageRegexpMatch(oc, "cpu.*200m.*memory.*500Mi", "pod", "-l", "name=compliance-operator", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="compliance-operator")].resources.limits}`)
		assertEventMessageRegexpMatch(oc, "cpu.*10m.*memory.*20Mi", "pod", "-l", "name=compliance-operator", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="compliance-operator")].resources.requests}`)

		g.By("Patch the daemonResourceRequirements !!!\n")
		defer func() {
			g.By("Recover the default daemonResourceRequirements.. !!!\n")
			patchRecover := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/config/resources\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "sub", subD.subName, "--type", "json", "--patch", patchRecover, "-n", subD.namespace)
			newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=name=compliance-operator", "-n",
				subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
			assertEventMessageRegexpMatch(oc, "cpu.*200m.*memory.*500Mi", "pod", "-l", "name=compliance-operator", "-n", subD.namespace,
				`-o=jsonpath={.items[*].spec.containers[?(@.name=="compliance-operator")].resources.limits}`)
			assertEventMessageRegexpMatch(oc, "cpu.*10m.*memory.*20Mi", "pod", "-l", "name=compliance-operator", "-n", subD.namespace,
				`-o=jsonpath={.items[*].spec.containers[?(@.name=="compliance-operator")].resources.requests}`)
		}()
		patchResourceReq := fmt.Sprintf("{\"spec\":{\"config\":{\"resources\":{\"limits\":{\"memory\":\"512Mi\",\"cpu\":\"256m\"},\"requests\":{\"memory\":\"52Mi\",\"cpu\":\"25m\"}}}}}")
		patchResource(oc, asAdmin, withoutNamespace, "sub", subD.subName, "--type", "merge", "-p", patchResourceReq, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", "--selector=name=compliance-operator", "-n",
			subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		assertEventMessageRegexpMatch(oc, "cpu.*256m.*memory.*512Mi", "pod", "-l", "name=compliance-operator", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="compliance-operator")].resources.limits}`)
		assertEventMessageRegexpMatch(oc, "cpu.*25m.*memory.*52Mi", "pod", "-l", "name=compliance-operator", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="compliance-operator")].resources.requests}`)

		g.By("ocp-55355 Check the operator's memory or CPU limits is configurable... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-High-61324-Low-61325-Check scans and instructions work for ocp4-stig, ocp4-stig-node and rhcos4-stig profiles for v1r1 and v2r1 versions[Slow]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		ssbNameStig := "ocp4-stig-" + getRandomString()
		ssbNameStigv2r1 := "ocp4-stig-v2r1-" + getRandomString()
		ssbNameStigv1r1 := "ocp4-stig-v1r1-" + getRandomString()
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbNameStig},
			objectTableRef{"scansettingbinding", subD.namespace, ssbNameStigv2r1},
			objectTableRef{"scansettingbinding", subD.namespace, ssbNameStigv1r1})

		g.By("Verify scansettingbinding, ScanSetting, profile objects created..!!!\n")
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbNameStig, "profile/ocp4-stig", "profile/ocp4-stig-node", "profile/rhcos4-stig", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbNameStigv2r1, "profile/ocp4-stig-v2r1", "profile/ocp4-stig-node-v2r1", "profile/rhcos4-stig-v2r1", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbNameStigv1r1, "profile/ocp4-stig-v1r1", "profile/ocp4-stig-node-v1r1", "profile/rhcos4-stig-v1r1", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify scansettingbinding, ScanSetting, profile objects created..!!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameStig, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameStigv2r1, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameStigv1r1, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameStig, ok, []string{"compliancesuite", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameStigv2r1, ok, []string{"compliancesuite", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameStigv1r1, ok, []string{"compliancesuite", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertKeywordsExists(oc, 300, "DONE", "compliancesuite", ssbNameStig, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		assertKeywordsExists(oc, 300, "DONE", "compliancesuite", ssbNameStigv2r1, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		assertKeywordsExists(oc, 300, "DONE", "compliancesuite", ssbNameStigv1r1, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		subD.complianceSuiteResult(oc, ssbNameStig, "NON-COMPLIANT INCONSISTENT")
		subD.complianceSuiteResult(oc, ssbNameStigv2r1, "NON-COMPLIANT INCONSISTENT")
		subD.complianceSuiteResult(oc, ssbNameStigv1r1, "NON-COMPLIANT INCONSISTENT")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbNameStig, "2")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbNameStigv2r1, "2")
		subD.getScanExitCodeFromConfigmapWithSuiteName(oc, ssbNameStigv1r1, "2")

		g.By("Diff the failed rules between profiles without suffix and profiles of latest version.. !!!\n")
		checkFailedRulesForTwoProfiles(oc, subD.namespace, ssbNameStig, ssbNameStigv2r1, "-v2r1")

		g.By("Check the instructions available for all rules.. !!")
		//Add two rules in exemptRulesList due to bug https://issues.redhat.com/browse/OCPBUGS-18789
		exemptRulesList := []string{"rhcos4-stig-master-audit-rules-session-events", "rhcos4-stig-worker-audit-rules-session-events"}
		checkInstructionNotEmpty(oc, subD.namespace, ssbNameStig, exemptRulesList)

		g.By("ocp-61324-61325-Check scans and instructions work for ocp4-stig, ocp4-stig-node and rhco4-stig profiles successfully... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-Author:xiyuan-Longduration-CPaasrunOnly-NonPreRelease-High-61323-High-67355-Check remediation works for ocp4-stig, ocp4-stig-node and rhcos4-stig profiles [Disruptive][Slow]", func() {
		g.By("Check if cluster is Etcd Encryption On")
		skipEtcdEncryptionOff(oc)

		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  true,
				autoupdateremediations: true,
				name:                   "auto-rem-ss",
				namespace:              "",
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbStig = "stig-test" + getRandomString()
		)

		g.By("Check stig profiles .. !!!\n")
		subD.getProfileName(oc, "ocp4-stig")
		subD.getProfileName(oc, "ocp4-stig-node")
		subD.getProfileName(oc, "rhcos4-stig")

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbStig, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ss", ss.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + ssbStig})
		}()

		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.machineCount}"}).check(oc)
		}()
		defer func() {
			g.By("Patch all complianceremediations to false .. !!!\n")
			patchPaused := fmt.Sprintf("{\"spec\":{\"paused\":true}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchPaused)
			setApplyToFalseForAllCrs(oc, subD.namespace, ssbStig)
			patchUnpaused := fmt.Sprintf("{\"spec\":{\"paused\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchUnpaused)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create scansetting... !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbStig, "-S", ss.name, "profile/ocp4-stig", "profile/ocp4-stig-node", "profile/rhcos4-stig", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbStig, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbStig, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssbStig, "NON-COMPLIANT INCONSISTENT")
		crResult, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-l", "compliance.openshift.io/suite="+ssbStig, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crResult != "" {
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crMissingDependencies, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l",
			"compliance.openshift.io/has-unmet-dependencies=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crMissingDependencies != "" {
			g.By("Performing second scan using oc-compliance plugin.. !!!\n")
			_, err := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbStig, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssbStig, "-n", subD.namespace,
				"-o=jsonpath={.status.phase}"}).check(oc)

			g.By("Check complianceSuite name and result.. !!!\n")
			subD.complianceSuiteResult(oc, ssbStig, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger the third round of rescan if needed !!!\n")
		crMissingDependencies, err = oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l",
			"compliance.openshift.io/has-unmet-dependencies=", "-o=jsonpath={.items[*].metadata.name}").Output()
		e2e.Logf("The crMissingDependencies is: %s and the err is: %v", crMissingDependencies, err)
		o.Expect(len(crMissingDependencies)).Should(o.BeNumerically("==", 0))

		if crResult != "" {
			checkMachineConfigPoolStatus(oc, ss.roles1)
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbStig, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkComplianceSuiteStatus(oc, ssbStig, subD.namespace, "DONE")
			subD.complianceSuiteResult(oc, ssbStig, "NON-COMPLIANT INCONSISTENT")
		}

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check rules and remediation status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-stig-node-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-stig", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "COMPLIANT", ok, []string{"compliancescan",
			"rhcos4-stig-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)

		result, _ := oc.AsAdmin().Run("get").Args("ccr", "-n", subD.namespace, "-l", "compliance.openshift.io/automated-remediation=,compliance.openshift.io/check-status=FAIL").Output()
		if !strings.Contains(result, "No resources found") {
			e2e.Failf("%s is NOT expected result for the final result", result)
		}
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-Author:xiyuan-Longduration-CPaasrunOnly-NonPreRelease-High-69602-Check http version for compliance operator's service endpoints [Serial]", func() {
		g.By("Check http version for metric serive")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		url := fmt.Sprintf("https://metrics.%v.svc:8585/metrics-co", subD.namespace)
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-i", "-ks", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), `HTTP/1.1 200 OK`)).To(o.BeTrue())

		g.By("Create scansettingbinding... !!!\n")
		ocp4ProfileparserPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", subD.namespace, "-l", "profile-bundle=ocp4,workload=profileparser", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ssbName := "ocp4-cis-" + getRandomString()
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", subD.namespace, ssbName})
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbName, "profile/ocp4-cis", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check http version for result server endpoint")
		label := "compliance.openshift.io/scan-name=ocp4-cis,workload=resultserver"
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"pod", "-l", label, "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		podIp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[0].status.podIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The result of podIp: %s", podIp)
		url = fmt.Sprintf("https://" + podIp + ":8443")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", subD.namespace, "-c", "pauser", ocp4ProfileparserPodName, "curl", "-Iksv", url).Output()
		o.Expect(strings.Contains(string(output), `HEAD / HTTP/1.1`)).To(o.BeTrue())
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:xiyuan-High-70206-Check the profiles annotation avaiable in the rules", func() {
		g.By("Check rule ocp4-accounts-restrict-service-account-tokens has the profiles annotation")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", subD.namespace, "rule", "ocp4-accounts-restrict-service-account-tokens", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), `"compliance.openshift.io/profiles":"ocp4-cis,ocp4-pci-dss,ocp4-high,ocp4-moderate,ocp4-stig-v1r1,ocp4-moderate-rev-4,ocp4-nerc-cip,ocp4-pci-dss-3-2,ocp4-stig,ocp4-cis-1-4,ocp4-high-rev-4"`))

		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("rule", `-o=jsonpath={.items[?(@.metadata.annotations.compliance\.openshift\.io/profiles!="")].metadata.name}`, "-n", subD.namespace).Output()
		o.Expect(strings.Contains(string(output), `ocp4-file-groupowner-ovs-conf-db`))
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("rule", `-o=jsonpath={.items[?(@.metadata.annotations.compliance\.openshift\.io/profiles=="")].metadata.name}`, "-n", subD.namespace).Output()
		o.Expect(strings.Contains(string(output), `rhcos4-wireless-disable-interfaces`))
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:xiyuan-NonPreRelease-High-67425-An administrator can suspend the scansettingbinding and resume the scan schedule [Serial][Slow]", func() {
		var ss = scanSettingDescription{
			autoapplyremediations:  false,
			autoupdateremediations: false,
			name:                   "test-suspend-" + getRandomString(),
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
		var ssbName string = "test-suspend-" + getRandomString()

		defer cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbName},
			objectTableRef{"ss", subD.namespace, ss.name})

		g.By("Create the default value for suspend field in default and default-auto-apply ss !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"scansetting", "default", "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"scansetting", "default", "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)

		g.By("Create scansetting !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbName, "-S", ss.name, "profile/ocp4-cis", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssbName, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Patch the ss scansettingbinding to suspend the scan and check ssb status !!!\n")
		patchSuspendTrue := fmt.Sprintf("{\"suspend\":true}")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", patchSuspendTrue, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "SUSPENDED", ok, []string{"ssb", ssbName, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check compliancesuite status and make sure ongoing compliancesuite will not be impacted.. !!! \n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbName, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbName, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbName+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"cronjob", ssbName + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbName, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Patch the ss scansettingbinding to resume the scan and check ssb status !!!\n")
		patchSuspendFalse := fmt.Sprintf("{\"suspend\":false}")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", patchSuspendFalse, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssbName, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"cronjob", ssbName + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, present, "", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbName, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbName, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbName, "NON-COMPLIANT")

		g.By("The ocp-67425 An administrator can suspend the scansettingbinding and resume the scan schedule.. !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:xiyuan-NonPreRelease-Medium-68179-The scansettingbinding will be suspended when the supend field setting to true in the corresponding scansetting [Serial]", func() {
		var ss = scanSettingDescription{
			autoapplyremediations:  false,
			autoupdateremediations: false,
			name:                   "test-suspend-" + getRandomString(),
			namespace:              subD.namespace,
			roles1:                 "master",
			roles2:                 "worker",
			rotation:               3,
			schedule:               "*/3 * * * *",
			size:                   "2Gi",
			priorityclassname:      "",
			debug:                  false,
			suspend:                true,
			template:               scansettingTemplate,
		}
		var ssbName string = "test-suspend-" + getRandomString()

		defer cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbName},
			objectTableRef{"ss", subD.namespace, ss.name})

		g.By("Create scansetting !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbName, "-S", ss.name, "profile/ocp4-cis", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "SUSPENDED", ok, []string{"ssb", ssbName, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"compliancesuite", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbName, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Patch the ss scansettingbinding to unsuspend the scan and check ssb status !!!\n")
		patchSuspendTrue := fmt.Sprintf("{\"suspend\":false}")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", patchSuspendTrue, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssbName, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check compliancesuite status.. !!! \n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbName, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbName, "-n", subD.namespace, "-o=jsonpath={.status.phase}")

		subD.complianceSuiteResult(oc, ssbName, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbName+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"cronjob", ssbName + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, present, "", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbName, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("The ocp-68179 The scansettingbinding will be suspended when the supend field setting to true in the corresponding scansetting.. !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-ROSA-ARO-OSD_CCS-Author:xiyuan-NonPreRelease-Medium-68189-Verify multiple ScanSettingBindings using the same ScanSetting could be paused together [Serial][Slow]", func() {
		var ss = scanSettingDescription{
			autoapplyremediations:  false,
			autoupdateremediations: false,
			name:                   "test-suspend-" + getRandomString(),
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
		var ssbNameCis string = "test-cis-" + getRandomString()
		var ssbNamePcidss string = "test-pci-dss-" + getRandomString()

		defer cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbNameCis},
			objectTableRef{"ssb", subD.namespace, ssbNamePcidss},
			objectTableRef{"ss", subD.namespace, ss.name})

		g.By("Create scansetting !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		_, err := OcComplianceCLI().Run("bind").Args("-N", ssbNameCis, "-S", ss.name, "profile/ocp4-cis", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbNamePcidss, "-S", ss.name, "profile/ocp4-pci-dss", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssbNameCis, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssbNamePcidss, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbNameCis, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbNamePcidss, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Patch the ss scansettingbinding to suspend the scan and check ssb status !!!\n")
		patchSuspendTrue := fmt.Sprintf("{\"suspend\":true}")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", patchSuspendTrue, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "SUSPENDED", ok, []string{"ssb", ssbNameCis, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "SUSPENDED", ok, []string{"ssb", ssbNamePcidss, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check compliancesuite status and make sure ongoing compliancesuite will not be impacted.. !!! \n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbNameCis, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbNamePcidss, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssbNameCis, "NON-COMPLIANT")
		subD.complianceSuiteResult(oc, ssbNamePcidss, "NON-COMPLIANT")
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameCis+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNamePcidss+"-rerunner", ok, []string{"cronjob", "-n",
			subD.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"cronjob", ssbNameCis + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "true", ok, []string{"cronjob", ssbNamePcidss + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbNameCis, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, notPresent, "", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbNamePcidss, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Patch the ss scansettingbinding to resume the scan and check ssb status !!!\n")
		patchSuspendFalse := fmt.Sprintf("{\"suspend\":false}")
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "--type", "merge", "-p", patchSuspendFalse, "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssbNameCis, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"ssb", ssbNamePcidss, "-n",
			subD.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"cronjob", ssbNameCis + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "false", ok, []string{"cronjob", ssbNamePcidss + "-rerunner",
			"-n", subD.namespace, "-o=jsonpath={.spec.suspend}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNameCis+"-rerunner-", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbNameCis, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbNamePcidss+"-rerunner-", ok, []string{"job", "-l", "compliance.openshift.io/suite=" + ssbNamePcidss, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check compliancesuite will become running again.. !!! \n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbNameCis, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbNamePcidss, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)

		g.By("The ocp-68189 Verify multiple ScanSettingBindings using the same ScanSetting could be paused together.. !!!\n")
	})

	// author: vahirwad@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:vahirwad-Medium-53920-Check the scanner's and api-resource-collector's limits configurable [Serial]", func() {

		var (
			ss = scanSettingDescription{
				name:           "scan-limit-" + getRandomString(),
				namespace:      "",
				roles1:         "master",
				roles2:         "worker",
				rotation:       3,
				schedule:       "0 1 * * *",
				strictnodescan: true,
				size:           "1Gi",
				debug:          true,
				cpu_limit:      "150m",
				memory_limit:   "512Mi",
				template:       scansettingLimitTemplate,
			}

			ssb = scanSettingBindingDescription{
				name:            "cis-scan-limit-test" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				profilename2:    "ocp4-cis-node",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		defer func() {
			cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssb.name})
			cleanupObjects(oc, objectTableRef{"scansetting", subD.namespace, ss.name})
		}()

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.phase}")
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Check the scanLimit for the ComplianceSuite.. !!!\n")
		assertParameterValueForBulkPods(oc, "150m", "suite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.spec.scans[*].scanLimits.cpu}")
		assertParameterValueForBulkPods(oc, "512Mi", "suite", ssb.name, "-n", subD.namespace, "-o=jsonpath={.spec.scans[*].scanLimits.memory}")

		g.By("Check the scanLimit for the scanner.. !!!\n")
		assertParameterValueForBulkPods(oc, "150m", "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.containers[?(@.name==\"scanner\")].resources.limits.cpu}")
		assertParameterValueForBulkPods(oc, "512Mi", "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.containers[?(@.name==\"scanner\")].resources.limits.memory}")

		g.By("ocp-53920-Check the scanner's and api-resource-collector's limits configurable ..!!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Author:xiyuan-High-60340-Check the timeout and maxRetryOnTimeout parameter work as expected [Serial]", func() {
		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "myss" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				roles2:                 "worker",
				rotation:               5,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "timeout-" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				profilename2:    "ocp4-cis-node",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name})

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		patch := `{"maxRetryOnTimeout":2,"timeout":"10s"}`
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "-n", subD.namespace, "--type", "merge", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, "2", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.maxRetryOnTimeout}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "10s", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.timeout}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "ERROR")

		g.By("Check complianceSuite result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "2", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis\")].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Timeout while waiting for the scan pod to be finished", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis\")].errormsg}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "2", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis-node-master\")].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Timeout while waiting for the scan pod to be finished", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis-node-master\")].errormsg}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "2", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis-node-worker\")].currentIndex}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Timeout while waiting for the scan pod to be finished", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis-node-worker\")].errormsg}"}).check(oc)

		g.By("ocp-60340 Check the timeout and maxRetryOnTimeout parameter work as expected... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-Author:xiyuan-High-60342-Check the timeout will be diabled when timeout was set to 0s [Serial]", func() {
		var (
			ss = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "myss" + getRandomString(),
				namespace:              "",
				roles1:                 "master",
				roles2:                 "worker",
				rotation:               5,
				schedule:               "0 1 * * *",
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "non-timeout-" + getRandomString(),
				namespace:       "",
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis",
				profilename2:    "ocp4-cis-node",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", subD.namespace, ssb.name},
			objectTableRef{"scansetting", subD.namespace, ss.name})

		g.By("Create scansetting !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		patch := `{"maxRetryOnTimeout":2,"timeout":"0s"}`
		patchResource(oc, asAdmin, withoutNamespace, "ss", ss.name, "-n", subD.namespace, "--type", "merge", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, "2", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.maxRetryOnTimeout}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "0s", ok, []string{"scansetting", ss.name, "-n", ss.namespace,
			"-o=jsonpath={.timeout}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.namespace = subD.namespace
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-n", ssb.namespace, "-o=jsonpath={.status.phase}")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteName(oc, ssb.name)
		subD.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")

		g.By("Check complianceSuite result.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Compliance scan run is done and has results", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis\")].conditions[?(@.type==\"Ready\")].message}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Compliance scan run is done and has results", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis-node-master\")].conditions[?(@.type==\"Ready\")].message}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Compliance scan run is done and has results", ok, []string{"compliancesuite", ssb.name, "-n",
			subD.namespace, "-o=jsonpath={.status.scanStatuses[?(@.name==\"ocp4-cis-node-worker\")].conditions[?(@.type==\"Ready\")].message}"}).check(oc)

		g.By("ocp-60342 Check the timeout will be diabled when timeout was set to 0s... !!!\n")
	})

	// author: bgudi@redhat.com
	g.It("NonHyperShiftHOST-Author:bgudi-Longduration-CPaasrunOnly-NonPreRelease-High-72713-Verify autoremediations works for e8 profiles [Disruptive][Slow]", func() {
		g.By("Check if cluster is Etcd Encryption On")
		skipEtcdEncryptionOff(oc)

		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		var (
			ss = scanSettingDescription{
				autoapplyremediations:  true,
				autoupdateremediations: true,
				name:                   "auto-rem-ss",
				namespace:              "",
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssbE8 = "e8-test" + getRandomString()
		)

		g.By("Check e8 profiles .. !!!\n")
		subD.getProfileName(oc, "ocp4-e8")
		subD.getProfileName(oc, "rhcos4-e8")

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ssb", ssbE8, "-n", subD.namespace, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ss", ss.name, "-n", subD.namespace, "--ignore-not-found").Execute()
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkNodeStatus(oc)
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, "-l compliance.openshift.io/suite=" + ssbE8})
		}()

		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.machineCount}"}).check(oc)
		}()
		defer func() {
			g.By("Patch all complianceremediaiton to false .. !!!\n")
			patchPaused := fmt.Sprintf("{\"spec\":{\"paused\":true}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchPaused)
			setApplyToFalseForAllCrs(oc, subD.namespace, ssbE8)
			patchUnpaused := fmt.Sprintf("{\"spec\":{\"paused\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "mcp", ss.roles1, "-n", subD.namespace, "--type", "merge", "-p", patchUnpaused)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create scansetting... !!!\n")
		ss.namespace = subD.namespace
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create scansettingbinding... !!!\n")
		_, err = OcComplianceCLI().Run("bind").Args("-N", ssbE8, "-S", ss.name, "profile/ocp4-e8", "profile/rhcos4-e8", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbE8, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		checkComplianceSuiteStatus(oc, ssbE8, subD.namespace, "DONE")

		g.By("Check complianceSuite name and result.. !!!\n")
		subD.complianceSuiteResult(oc, ssbE8, "NON-COMPLIANT INCONSISTENT")
		crResult, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-l", "compliance.openshift.io/suite="+ssbE8, "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crResult != "" {
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger another round of rescan if needed !!!\n")
		crMissingDependencies, err := oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l",
			"compliance.openshift.io/has-unmet-dependencies=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if crMissingDependencies != "" {
			g.By("Performing second scan using oc-compliance plugin.. !!!\n")
			_, err := OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbE8, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "DONE", ok, []string{"compliancesuite", ssbE8, "-n", subD.namespace,
				"-o=jsonpath={.status.phase}"}).check(oc)

			g.By("Check complianceSuite name and result.. !!!\n")
			subD.complianceSuiteResult(oc, ssbE8, "NON-COMPLIANT")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace,
				"-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, ss.roles1)
		}

		g.By("ClusterOperator should be healthy before running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Trigger the third round of rescan if needed !!!\n")
		crMissingDependencies, err = oc.AsAdmin().Run("get").Args("complianceremediation", "-n", subD.namespace, "-l",
			"compliance.openshift.io/has-unmet-dependencies=", "-o=jsonpath={.items[*].metadata.name}").Output()
		e2e.Logf("The crMissingDependencies is: %s and the err is: %v", crMissingDependencies, err)
		o.Expect(len(crMissingDependencies)).Should(o.BeNumerically("==", 0))
		if crResult != "" {
			checkMachineConfigPoolStatus(oc, ss.roles1)
			_, err = OcComplianceCLI().Run("rerun-now").Args("compliancesuite", ssbE8, "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			checkComplianceSuiteStatus(oc, ssbE8, subD.namespace, "DONE")
			subD.complianceSuiteResult(oc, ssbE8, "NON-COMPLIANT INCONSISTENT")
		}

		g.By("ClusterOperator should be healthy after running rescan")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check rules and remediation status !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancescan",
			"ocp4-e8", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "COMPLIANT", ok, []string{"compliancescan",
			"rhcos4-e8-wrscan", "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)

		result, _ := oc.AsAdmin().Run("get").Args("ccr", "-n", subD.namespace, "-l",
			"compliance.openshift.io/automated-remediation=,compliance.openshift.io/check-status=FAIL").Output()
		if !strings.Contains(result, "No resources found") {
			e2e.Failf("%s is NOT expected result for the final result", result)
		}
	})

	// author: xiyuan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-ROSA-ARO-OSD_CCS-ConnectedOnly-Author:xiyuan-High-45414-Verify the nodeSelector and tolerations are configurable for result server pod [Slow][Disruptive]", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}

		var (
			nodeName               = getOneWorkerNodeName(oc)
			ssbDefaultNodeselector = scanSettingBindingDescription{
				name:            "default-nodeselector-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-cis-node",
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
			ssbCustomNodeselector = scanSettingBindingDescription{
				name:            "custom-nodeselector-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-high-node",
				scansettingname: "default",
				template:        scansettingbindingSingleTemplate,
			}
		)

		g.By("Create the first scansetting and scansettingbinding!!!\n")
		defer cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbDefaultNodeselector.name})
		ssbDefaultNodeselector.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbDefaultNodeselector.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check the nodeselector for resultserver pods.. !!!\n")
		label := "workload=resultserver"
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"pod", "-l", label, "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running Running", ok, []string{"pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		assertParameterValueForBulkPods(oc, `{"node-role.kubernetes.io/master":""}`, "pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[*].spec.nodeSelector}")

		g.By("Check the scan result pods for the first round scan.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbDefaultNodeselector.name, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		subD.complianceSuiteResult(oc, ssbDefaultNodeselector.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Create the sedond scansetting and scansettingbinding !!!\n")
		defer func() {
			cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbCustomNodeselector.name})
			patch := `[{"op": "replace", "path": "/rawResultStorage/nodeSelector", "value":{"node-role.kubernetes.io/master":""}}]`
			patchResource(oc, asAdmin, withoutNamespace, "ss", "default", "-n", subD.namespace, "--type", "json", "-p", patch)
		}()
		cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbDefaultNodeselector.name})
		patch := `[{"op":"replace","path":"/rawResultStorage/nodeSelector","value":{"node-role.kubernetes.io/worker":""}}]`
		patchResource(oc, asAdmin, withoutNamespace, "ss", "default", "-n", subD.namespace, "--type", "json", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, "{\"node-role.kubernetes.io/worker\":\"\"}", ok, []string{"ss", "default",
			"-n", subD.namespace, "-o=jsonpath={.rawResultStorage.nodeSelector}"}).check(oc)
		ssbCustomNodeselector.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbCustomNodeselector.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check the nodeselector for resultserver pods for the second round scan.. !!!\n")
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"pod", "-l", label, "-n", subD.namespace}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Running Running", ok, []string{"pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		assertParameterValueForBulkPods(oc, `{"node-role.kubernetes.io/worker":""}`, "pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[*].spec.nodeSelector}")

		g.By("Check the scan result pods.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbCustomNodeselector.name, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		subD.complianceSuiteResult(oc, ssbCustomNodeselector.name, "NON-COMPLIANT INCONSISTENT")

		g.By("Patch the ss to apply toleration !!!\n")
		defer func() {
			cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbCustomNodeselector.name})
			patch := `{"rawResultStorage":{"tolerations":[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"},{"effect":"NoExecute","key":"node.kubernetes.io/not-ready","operator":"Exists","tolerationSeconds":300},{"effect":"NoExecute","key":"node.kubernetes.io/unreachable","operator":"Exists","tolerationSeconds":300},{"effect":"NoSchedule","key":"node.kubernetes.io/memory-pressure","operator":"Exists"}]}}`
			patchResource(oc, asAdmin, withoutNamespace, "ss", "default", "-n", subD.namespace, "--type", "merge", "-p", patch)
			newCheck("expect", asAdmin, withoutNamespace, contain, `[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"},{"effect":"NoExecute","key":"node.kubernetes.io/not-ready","operator":"Exists","tolerationSeconds":300},{"effect":"NoExecute","key":"node.kubernetes.io/unreachable","operator":"Exists","tolerationSeconds":300},{"effect":"NoSchedule","key":"node.kubernetes.io/memory-pressure","operator":"Exists"}]`,
				ok, []string{"ss", "default", "-n", subD.namespace, "-o=jsonpath={.rawResultStorage.tolerations}"}).check(oc)
		}()
		patch = `{"rawResultStorage":{"tolerations":[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"},{"effect":"NoExecute","key":"node.kubernetes.io/not-ready","operator":"Exists","tolerationSeconds":300},{"effect":"NoExecute","key":"node.kubernetes.io/unreachable","operator":"Exists","tolerationSeconds":300},{"effect":"NoSchedule","key":"node.kubernetes.io/memory-pressure","operator":"Exists"},{"effect":"NoExecute","key":"key1","value":"value1","operator":"Equal"}]}}`
		patchResource(oc, asAdmin, withoutNamespace, "ss", "default", "-n", subD.namespace, "--type", "merge", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, `{"effect":"NoExecute","key":"key1","operator":"Equal","value":"value1"}`, ok, []string{"ss", "default",
			"-n", subD.namespace, "-o=jsonpath={.rawResultStorage.tolerations}"}).check(oc)

		g.By("Label and set taint value to one worker node.. !!!\n")
		taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule")
		labelTaintNode(oc, "node", nodeName, "taint=true")
		defer func() {
			taintNode(oc, "taint", "node", nodeName, "key1=value1:NoSchedule-")
			labelTaintNode(oc, "node", nodeName, "taint-")
		}()

		g.By("Create a third ssb.. !!!\n")
		cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssbCustomNodeselector.name})
		ssbCustomNodeselector.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssbCustomNodeselector.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RUNNING", ok, []string{"compliancesuite", ssbCustomNodeselector.name, "-n", subD.namespace,
			"-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check the nodeselector for resultserver pods.. !!!\n")
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"pod", "-l", label, "-n", subD.namespace}).check(oc)
		assertParameterValueForBulkPods(oc, `{"node-role.kubernetes.io/worker":""}`, "pod", "-l", label, "-n", subD.namespace, "-o=jsonpath={.items[*].spec.nodeSelector}")

		g.By("Check the scan result pods.. !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssbCustomNodeselector.name, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		subD.complianceSuiteResult(oc, ssbCustomNodeselector.name, "NON-COMPLIANT INCONSISTENT")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-NonPreRelease-Medium-53667-Check the ocp4-pci-dss-modified-api-checks-pod will not in CrashLoopBackoff state with large scale of mc [Disruptive][Slow]", func() {
		g.By("Set initial value !!!\n")
		var (
			cnt           int
			mcPrefix      = "isc-test-mc-53667-"
			mcCount       = int64(150)
			mcEnableAudit = "isc-mc-53667-enable-audit" + getRandomString()
			ss            = scanSettingDescription{
				autoapplyremediations:  false,
				autoupdateremediations: false,
				name:                   "auto-rem-ss" + getRandomString(),
				namespace:              subD.namespace,
				roles1:                 "wrscan",
				rotation:               5,
				schedule:               "0 1 * * *",
				strictnodescan:         false,
				size:                   "2Gi",
				priorityclassname:      "",
				debug:                  false,
				suspend:                false,
				template:               scansettingSingleTemplate,
			}
			ssb = scanSettingBindingDescription{
				name:            "pci-dss-" + getRandomString(),
				namespace:       subD.namespace,
				profilekind1:    "Profile",
				profilename1:    "ocp4-pci-dss-node",
				profilename2:    "ocp4-pci-dss",
				scansettingname: ss.name,
				template:        scansettingbindingTemplate,
			}
		)

		// checking all nodes are in Ready state before the test case starts
		checkNodeStatus(oc)
		// adding label to one rhcos worker node to skip rhel and other RHCOS worker nodes
		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove scansettingbinding, machineconfig, machineconfigpool objects.. !!!\n")
			cleanupObjects(oc, objectTableRef{"ssb", subD.namespace, ssb.name})
			cleanupObjects(oc, objectTableRef{"ss", subD.namespace, ss.name})
			checkMachineConfigPoolStatus(oc, "worker")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, ss.roles1})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
		}()
		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", ss.roles1, "-n", subD.namespace, "-o=jsonpath={.status.machineCount}"}).check(oc)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Update kenel audit... !!!\n")
		defer func() {
			cleanupObjects(oc, objectTableRef{"mc", subD.namespace, mcEnableAudit})
		}()
		errEnableAudit := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", mcWithoutIgnitionEnableAuditYAML, "-p", "NAME="+mcEnableAudit, "ROLE="+ss.roles1)
		o.Expect(errEnableAudit).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, ss.roles1)

		g.By("Create a large scale of machineconfigs... !!!\n")
		defer func() {
			for i := int64(0); i < mcCount; i++ {
				go func(i int64) {
					defer g.GinkgoRecover()
					cleanupObjects(oc, objectTableRef{"mc", subD.namespace, mcPrefix + strconv.Itoa(int(i))})
				}(i)
			}
			err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
				nsCnt := getResouceCnt(oc, "mc", mcPrefix)
				if nsCnt == int(0) {
					return true, nil
				}
				return false, nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		for mcId := int64(0); mcId < mcCount; mcId++ {
			go func(mcId int64) {
				defer g.GinkgoRecover()
				err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", mcTemplate, "-n", subD.namespace, "-p", "NAME="+mcPrefix+strconv.Itoa(int(mcId)), "ID="+strconv.Itoa(int(mcId)))
				o.Expect(err).NotTo(o.HaveOccurred())
			}(mcId)
		}
		errPoll := wait.Poll(5*time.Second, 100*time.Second, func() (bool, error) {
			cnt = getResouceCnt(oc, "mc", mcPrefix)
			if cnt == int(mcCount) {
				return true, nil
			}
			return false, nil
		})
		if errPoll != nil {
			e2e.Logf("The created machineconfig number is: %d", cnt)
		}
		o.Expect(errPoll).NotTo(o.HaveOccurred())

		g.By("Create scansetting and scansettingbinding... !!!\n")
		ss.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ss.name, ok, []string{"scansetting", "-n", ss.namespace, ss.name,
			"-o=jsonpath={.metadata.name}"}).check(oc)
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check whether ClusterOperator is healthy")
		clusterOperatorHealthcheck(oc, 1500)

		g.By("Check test results !!!\n")
		assertCompliancescanDone(oc, subD.namespace, "compliancesuite", ssb.name, "-o=jsonpath={.status.phase}", "-n", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, "NON-COMPLIANT", ok, []string{"compliancesuite",
			ssb.name, "-n", subD.namespace, "-o=jsonpath={.status.result}"}).check(oc)
		g.By("ocp-53667 Check the ocp4-pci-dss-modified-api-checks-pod will not in CrashLoopBackoff state with large scale of mc... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("CPaasrunOnly-Author:xiyuan-High-71325-compliance operator should pass DAST test", func() {
		architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		configFile := filepath.Join(buildPruningBaseDir, "rapidast/data_rapidastconfig_compliance_v1alpha1.yaml")
		policyFile := filepath.Join(buildPruningBaseDir, "rapidast/customscan.policy")

		_, err := rapidastScan(oc, oc.Namespace(), configFile, policyFile, "compliance.openshift.io_v1alpha1")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
})

var _ = g.Describe("[sig-isc] Security_and_Compliance The Compliance Operator on hypershift hosted cluster", func() {
	defer g.GinkgoRecover()

	var (
		oc                                     = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir                    string
		og                                     operatorGroupDescription
		ogCoTemplate                           string
		scansettingbindingTemplate             string
		sub                                    subscriptionDescription
		subCoHypershiftHostedTemplate          string
		tprofileCisHypershiftHostedTemplate    string
		tprofilePcidssHypershiftHostedTemplate string
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogCoTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		scansettingbindingTemplate = filepath.Join(buildPruningBaseDir, "scansettingbinding.yaml")
		subCoHypershiftHostedTemplate = filepath.Join(buildPruningBaseDir, "subscription_hypershift_env.yaml")
		tprofileCisHypershiftHostedTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-cis-hypershift-hosted.yaml")
		tprofilePcidssHypershiftHostedTemplate = filepath.Join(buildPruningBaseDir, "tailoredprofile-pci-dss-hypershift-hosted.yaml")
		og = operatorGroupDescription{
			name:      "openshift-compliance",
			namespace: "openshift-compliance",
			template:  ogCoTemplate,
		}
		sub = subscriptionDescription{
			subName:                "compliance-operator",
			namespace:              "openshift-compliance",
			channel:                "stable",
			ipApproval:             "Automatic",
			operatorPackage:        "compliance-operator",
			catalogSourceName:      "qe-app-registry",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			platform:               "HyperShift",
			template:               subCoHypershiftHostedTemplate,
			singleNamespace:        true,
		}

		g.By("Skip test when precondition not meet !!!")
		exutil.SkipNoOLMCore(oc)
		SkipNonHypershiftHostedClusters(oc)
		sub.skipMissingCatalogsources(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
		SkipMissingDefaultSC(oc)
		SkipMissingRhcosWorkers(oc)

		g.By("Install Compliance Operator and check it is sucessfully installed !!! ")
		createComplianceOperator(oc, sub, og)

		g.By("Check only worker role available !!! ")
		newCheck("expect", asAdmin, withoutNamespace, compare, "[\"worker\"]", ok, []string{"ss", "default", "-n",
			sub.namespace, "-o=jsonpath={.roles}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "[\"worker\"]", ok, []string{"ss", "default-auto-apply", "-n",
			sub.namespace, "-o=jsonpath={.roles}"}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-High-63796-Scan hosted cluster for ocp4-cis and ocp4-cis-node profiles on hypershift hosted cluster [Serial]", func() {
		var tprofileCis = "cis-compliance-hypershift" + getRandomString()
		var ssb = scanSettingBindingDescription{
			name:            "test-cis-" + getRandomString(),
			namespace:       sub.namespace,
			profilekind1:    "TailoredProfile",
			profilename1:    tprofileCis,
			profilename2:    "ocp4-cis-node",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", sub.namespace, ssb.name},
			objectTableRef{"tailoredprofile", sub.namespace, ssb.profilename1})

		g.By("Create tailoredprofile for ocp4-cis and check status !!!\n")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", sub.namespace, "-f", tprofileCisHypershiftHostedTemplate, "-p", "NAME="+tprofileCis,
			"NAMESPACE="+sub.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", "-n", sub.namespace, tprofileCis,
			"-o=jsonpath={.status.state}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", sub.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertCompliancescanDone(oc, sub.namespace, "compliancesuite", ssb.name, "-o=jsonpath={.status.phase}", "-n", sub.namespace)

		g.By("Check complianceSuite name and result.. !!!\n")
		sub.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")
		sub.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("Verify result for rules in compliancecheckresult.. !!!.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, compare, "FAIL", ok, []string{"compliancecheckresult", tprofileCis + "-configure-network-policies-namespaces",
			"-n", sub.namespace, "-o=jsonpath={.status}"}).check(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "kubeadmin", "-n", "kube-system").Output()
		if err != nil && strings.Contains(output, "NotFound") {
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult", tprofileCis + "-kubeadmin-removed",
				"-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		} else if err == nil && strings.Contains(output, "Opaque") {
			newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult", tprofileCis + "-kubeadmin-removed",
				"-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		} else {
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("ocp-63796 Scan hosted cluster for ocp4-cis and ocp4-cis-node profiles on hypershift hosted cluster... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-High-63795-Scan hosted cluster for ocp4-pci-dss and ocp4-pci-dss-node profiles on hypershift hosted cluster [Serial]", func() {
		var tprofilePci = "pci-dss-compliance-hypershift" + getRandomString()
		var ssb = scanSettingBindingDescription{
			name:            "test-pci-dss" + getRandomString(),
			namespace:       sub.namespace,
			profilekind1:    "TailoredProfile",
			profilename1:    tprofilePci,
			profilename2:    "ocp4-pci-dss-node",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		defer cleanupObjects(oc,
			objectTableRef{"scansettingbinding", sub.namespace, ssb.name},
			objectTableRef{"tailoredprofile", sub.namespace, ssb.profilename1})

		g.By("Create tailoredprofile for ocp4-pci-dss and check status !!!\n")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-n", sub.namespace, "-f", tprofilePcidssHypershiftHostedTemplate, "-p", "NAME="+tprofilePci,
			"NAMESPACE="+sub.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "READY", ok, []string{"tailoredprofile", "-n", sub.namespace, tprofilePci,
			"-o=jsonpath={.status.state}"}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", sub.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status !!!\n")
		assertKeywordsExists(oc, 300, "DONE", "compliancesuite", ssb.name, "-o=jsonpath={.status.phase}", "-n", sub.namespace)

		g.By("Check complianceSuite name and result.. !!!\n")
		sub.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")
		sub.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("Verify result for rules in compliancecheckresult.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, compare, "FAIL", ok, []string{"compliancecheckresult", tprofilePci + "-configure-network-policies-namespaces",
			"-n", sub.namespace, "-o=jsonpath={.status}"}).check(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "kubeadmin", "-n", "kube-system").Output()
		if err != nil && strings.Contains(output, "NotFound") {
			newCheck("expect", asAdmin, withoutNamespace, contain, "PASS", ok, []string{"compliancecheckresult", tprofilePci + "-kubeadmin-removed",
				"-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		} else if err == nil && strings.Contains(output, "Opaque") {
			newCheck("expect", asAdmin, withoutNamespace, contain, "FAIL", ok, []string{"compliancecheckresult", tprofilePci + "-kubeadmin-removed",
				"-n", ssb.namespace, "-o=jsonpath={.status}"}).check(oc)
		} else {
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("ocp-63795 Scan hosted cluster for ocp4-pci-dss and ocp4-pci-dss-node profiles on hypershift hosted cluster... !!!\n")
	})
})

var _ = g.Describe("[sig-isc] Security_and_Compliance Compliance_operator The Compliance Operator on ROSA hcp cluster", func() {
	defer g.GinkgoRecover()

	var (
		oc                            = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir           string
		og                            operatorGroupDescription
		ogCoTemplate                  string
		scansettingbindingTemplate    string
		sub                           subscriptionDescription
		subCoHypershiftHostedTemplate string
		subRosaHcpTemplate            string
		subwithPlatformEnv            subscriptionDescription
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogCoTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		scansettingbindingTemplate = filepath.Join(buildPruningBaseDir, "scansettingbinding.yaml")
		subRosaHcpTemplate = filepath.Join(buildPruningBaseDir, "subscription_rosa_hcp.yaml")
		subCoHypershiftHostedTemplate = filepath.Join(buildPruningBaseDir, "subscription_hypershift_env.yaml")

		og = operatorGroupDescription{
			name:      "openshift-compliance",
			namespace: "openshift-compliance",
			template:  ogCoTemplate,
		}
		sub = subscriptionDescription{
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
			template:               subRosaHcpTemplate,
			singleNamespace:        true,
		}
		subwithPlatformEnv = subscriptionDescription{
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
			platform:               "rosa",
			template:               subCoHypershiftHostedTemplate,
			singleNamespace:        true,
		}

		g.By("Skip test when precondition not meet !!!")
		exutil.SkipNoOLMCore(oc)
		SkipNonRosaHcpCluster(oc)
		sub.skipMissingCatalogsources(oc)
		architecture.SkipNonAmd64SingleArch(oc)
		SkipMissingDefaultSC(oc)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-High-74182-Install CO without PLATFORM setting in sub and Scan hosted cluster with ocp4-cis-node and ocp4-pci-dss-node profiles on a Rosa hcp cluster [Serial]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "test-cis-" + getRandomString(),
			namespace:       sub.namespace,
			profilekind1:    "Profile",
			profilename1:    "ocp4-pci-dss-node",
			profilename2:    "ocp4-cis-node",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		g.By("Install Compliance Operator and check it is sucessfully installed !!! ")
		defer cleanupObjects(oc, objectTableRef{"profilebundle", sub.namespace, "ocp4"},
			objectTableRef{"profilebundle", sub.namespace, "rhcos4"},
			objectTableRef{"ns", sub.namespace, sub.namespace})
		createComplianceOperator(oc, sub, og)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.spec.config.env}"}).check(oc)

		g.By("Check the pb and ss status !!! ")
		sub.getProfileBundleNameandStatus(oc, "ocp4", "VALID")
		sub.getProfileBundleNameandStatus(oc, "rhcos4", "VALID")
		newCheck("expect", asAdmin, withoutNamespace, compare, "[\"worker\"]", ok, []string{"ss", "default", "-n",
			sub.namespace, "-o=jsonpath={.roles}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"scansetting", "default-auto-apply", "-n", sub.namespace}).check(oc)

		g.By("Check only nodes profiles available.. !!!\n")
		allProfiles := []string{
			"ocp4-cis-node",
			"ocp4-cis-node-1-4",
			"ocp4-cis-node-1-5",
			"ocp4-high-node",
			"ocp4-high-node-rev-4",
			"ocp4-moderate-node",
			"ocp4-moderate-node-rev-4",
			"ocp4-nerc-cip-node",
			"ocp4-pci-dss-node",
			"ocp4-pci-dss-node-3-2",
			"ocp4-stig-node",
			"ocp4-stig-node-v1r1",
			"rhcos4-e8",
			"rhcos4-high",
			"rhcos4-high-rev-4",
			"rhcos4-moderate",
			"rhcos4-moderate-rev-4",
			"rhcos4-nerc-cip",
			"rhcos4-stig",
			"rhcos4-stig-v1r1"}
		for _, profile := range allProfiles {
			sub.getProfileName(oc, profile)
		}
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"profile", "ocp4-cis", "-n", ssb.namespace}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", sub.namespace, ssb.name})
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result !!!\n")
		assertCompliancescanDone(oc, sub.namespace, "compliancesuite", ssb.name, "-o=jsonpath={.status.phase}", "-n", ssb.namespace)
		sub.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")
		sub.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("ocp-74182 Install CO without PLATFORM setting in sub and Scan hosted cluster with ocp4-cis-node and ocp4-pci-dss-node profiles on a Rosa hcp cluster finished... !!!\n")
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-Medium-73945-Install CO with PLATFORM set to rosa and Scan hosted cluster with ocp4-cis-node and ocp4-pci-dss-node [Serial]", func() {
		var ssb = scanSettingBindingDescription{
			name:            "test-cis-" + getRandomString(),
			namespace:       subwithPlatformEnv.namespace,
			profilekind1:    "Profile",
			profilename1:    "ocp4-pci-dss-node",
			profilename2:    "ocp4-cis-node",
			scansettingname: "default",
			template:        scansettingbindingTemplate,
		}

		g.By("Install Compliance Operator and check it is sucessfully installed !!! ")
		defer cleanupObjects(oc, objectTableRef{"profilebundle", subwithPlatformEnv.namespace, "ocp4"},
			objectTableRef{"profilebundle", subwithPlatformEnv.namespace, "rhcos4"},
			objectTableRef{"ns", sub.namespace, subwithPlatformEnv.namespace})
		createComplianceOperator(oc, subwithPlatformEnv, og)
		newCheck("expect", asAdmin, withoutNamespace, contain, subwithPlatformEnv.platform, ok, []string{"sub", subwithPlatformEnv.subName, "-n", subwithPlatformEnv.namespace, "-o=jsonpath={.spec.config.env}"}).check(oc)

		g.By("Check the pb and ss status !!! ")
		subwithPlatformEnv.getProfileBundleNameandStatus(oc, "ocp4", "VALID")
		subwithPlatformEnv.getProfileBundleNameandStatus(oc, "rhcos4", "VALID")
		newCheck("expect", asAdmin, withoutNamespace, compare, "[\"worker\"]", ok, []string{"ss", "default", "-n",
			ssb.namespace, "-o=jsonpath={.roles}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"ss", "default-auto-apply", "-n", ssb.namespace}).check(oc)

		g.By("Check only nodes profiles available.. !!!\n")
		allProfiles := []string{
			"ocp4-cis-node",
			"ocp4-cis-node-1-4",
			"ocp4-cis-node-1-5",
			"ocp4-high-node",
			"ocp4-high-node-rev-4",
			"ocp4-moderate-node",
			"ocp4-moderate-node-rev-4",
			"ocp4-nerc-cip-node",
			"ocp4-pci-dss-node",
			"ocp4-pci-dss-node-3-2",
			"ocp4-stig-node",
			"ocp4-stig-node-v1r1",
			"rhcos4-e8",
			"rhcos4-high",
			"rhcos4-high-rev-4",
			"rhcos4-moderate",
			"rhcos4-moderate-rev-4",
			"rhcos4-nerc-cip",
			"rhcos4-stig",
			"rhcos4-stig-v1r1"}
		for _, profile := range allProfiles {
			subwithPlatformEnv.getProfileName(oc, profile)
		}
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"profile", "ocp4-cis", "-n", ssb.namespace}).check(oc)

		g.By("Create scansettingbinding !!!\n")
		defer cleanupObjects(oc, objectTableRef{"scansettingbinding", sub.namespace, ssb.name})
		ssb.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ssb.name, ok, []string{"scansettingbinding", "-n", ssb.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check ComplianceSuite status and result !!!\n")
		assertCompliancescanDone(oc, sub.namespace, "compliancesuite", ssb.name, "-o=jsonpath={.status.phase}", "-n", ssb.namespace)
		sub.complianceSuiteResult(oc, ssb.name, "NON-COMPLIANT")
		sub.getScanExitCodeFromConfigmapWithSuiteName(oc, ssb.name, "2")

		g.By("ocp-73945 Install CO with PLATFORM set to rosa and Scan hosted cluster with ocp4-cis-node and ocp4-pci-dss-node profiles on a Rosa hcp cluster finished... !!!\n")
	})
})
