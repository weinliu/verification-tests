package securityandcompliance

import (
	"fmt"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance Security_Profiles_Operator The Security_Profiles_Operator", func() {
	defer g.GinkgoRecover()

	var (
		oc                            = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir           string
		errorLoggerSelTemplate        string
		errorLoggerPodTemplate        string
		ogSpoTemplate                 string
		podWithSelinuxProfileTemplate string
		profileBindingTemplate        string
		subSpoTemplate                string
		selinuxProfileNginxTemplate   string

		ogD  operatorGroupDescription
		subD subscriptionDescription
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		errorLoggerSelTemplate = filepath.Join(buildPruningBaseDir, "spo/selinux-profile-errorlogger.yaml")
		errorLoggerPodTemplate = filepath.Join(buildPruningBaseDir, "spo/pod-errorlogger.yaml")
		ogSpoTemplate = filepath.Join(buildPruningBaseDir, "operator-group-all-namespaces.yaml")
		podWithSelinuxProfileTemplate = filepath.Join(buildPruningBaseDir, "/spo/pod-with-selinux-profile.yaml")
		profileBindingTemplate = filepath.Join(buildPruningBaseDir, "/spo/profile-binding.yaml")
		subSpoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		selinuxProfileNginxTemplate = filepath.Join(buildPruningBaseDir, "/spo/selinux-profile-nginx.yaml")

		ogD = operatorGroupDescription{
			name:      "security-profiles-operator",
			namespace: "security-profiles-operator",
			template:  ogSpoTemplate,
		}

		subD = subscriptionDescription{
			subName:                "security-profiles-operator-sub",
			namespace:              "security-profiles-operator",
			channel:                "release-alpha-rhel-8",
			ipApproval:             "Automatic",
			operatorPackage:        "security-profiles-operator",
			catalogSourceName:      "redhat-operators",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			template:               subSpoTemplate,
			singleNamespace:        true,
		}

		exutil.SkipNoOLMCore(oc)
		subD.skipMissingCatalogsources(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
		architecture.SkipNonAmd64SingleArch(oc)
		SkipClustersWithRhelNodes(oc)

		// Uninstall security profiles operator first
		cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"seccompprofiles", subD.namespace, "--all"},
			objectTableRef{"selinuxprofiles", subD.namespace, "--all"},
			objectTableRef{"rawselinuxprofiles", subD.namespace, "--all"},
			objectTableRef{"mutatingwebhookconfiguration", subD.namespace, "spo-mutating-webhook-configuration"})
		deleteNamespace(oc, subD.namespace)

		createSecurityProfileOperator(oc, subD, ogD)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-NonHyperShiftHOST-ConnectedOnly-ARO-OSD_CCS-Critical-50958-Check selinxprofiles working as expected before/after upgrade [Serial][Slow]", func() {
		var (
			selinuxTest        = "do-binding-" + getRandomString()
			selinuxselinuxTest = selinuxProfile{
				name:      "error-logger-" + getRandomString(),
				namespace: selinuxTest,
				template:  errorLoggerSelTemplate,
			}
			podErrorlogger            = "errorlogger-" + getRandomString()
			profileBindingselinuxTest = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   selinuxTest,
				kind:        "SelinuxProfile",
				profilename: selinuxselinuxTest.name,
				image:       "quay.io/openshifttest/busybox",
				template:    profileBindingTemplate,
			}
		)

		g.By("Get installed version and check whether upgradable !!!\n")
		csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/security-profiles-operator.security-profiles-operator=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oldVersion := strings.ReplaceAll(csvName, "security-profiles-operator.v", "")
		oldVersion = strings.Trim(oldVersion, "'")
		upgradable, err := checkUpgradable(oc, "qe-app-registry", "release-alpha-rhel-8", "security-profiles-operator", oldVersion, "security-profiles-operator.v")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The result of upgradable is: %v", upgradable)
		if !upgradable {
			g.Skip("Skip as no new version detected!")
		}

		defer func() {
			cleanupObjectsIgnoreNotFound(oc, objectTableRef{"seccompprofiles", subD.namespace, "--all"},
				objectTableRef{"selinuxprofiles", subD.namespace, "--all"},
				objectTableRef{"rawselinuxprofiles", subD.namespace, "--all"},
				objectTableRef{"mutatingwebhookconfiguration", subD.namespace, "spo-mutating-webhook-configuration"})
			deleteNamespace(oc, subD.namespace)
		}()

		defer func() {
			g.By("delete resources in profilebinding test namespace !!!")
			cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", selinuxTest, podErrorlogger},
				objectTableRef{"selinuxprofiles", selinuxTest, selinuxselinuxTest.name},
				objectTableRef{"profilebinding", selinuxTest, profileBindingselinuxTest.name})
			deleteNamespace(oc, selinuxTest)
		}()

		g.By("Create namespace and add labels !!!")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", selinuxTest).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", selinuxTest, "-n", selinuxTest, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, selinuxTest)

		g.By("Created a pod!!!")
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodTemplate, "-p", "NAME="+podErrorlogger, "NAMESPACE="+selinuxTest)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, podErrorlogger, selinuxTest)
		newCheck("expect", asAdmin, withoutNamespace, contain, `{"capabilities":{"drop":["MKNOD"]}}`, ok, []string{"pod", podErrorlogger, "-n", selinuxTest,
			"-o=jsonpath={.spec.initContainers[0].securityContext}"}).check(oc)
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+podErrorlogger, "-c", "errorlogger", "-n", selinuxTest).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, `/var/log/test.log: Permission denied`)).To(o.BeTrue())

		g.By("Create selinuxprofile and check status !!!")
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxselinuxTest.template, "-p", "NAME="+selinuxselinuxTest.name, "NAMESPACE="+selinuxselinuxTest.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxselinuxTest.name, "-o=jsonpath={.status.status}", "-n", selinuxTest)
		usageselinuxTest := selinuxselinuxTest.name + "_" + selinuxselinuxTest.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usageselinuxTest, ok, []string{"selinuxprofiles", selinuxselinuxTest.name, "-n", selinuxselinuxTest.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)
		selinuxTestfileName := selinuxselinuxTest.name + "_" + selinuxselinuxTest.namespace + ".cil"
		assertKeywordsExistsInSelinuxFile(oc, selinuxselinuxTest.name+"_"+selinuxselinuxTest.namespace, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "cat", "/etc/selinux.d/"+selinuxTestfileName)
		assertKeywordsExistsInSelinuxFile(oc, selinuxselinuxTest.name+"_"+selinuxselinuxTest.namespace, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "semodule", "-l")

		g.By("Create profilebinding !!!")
		profileBindingselinuxTest.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingselinuxTest.name, ok, []string{"profilebinding", "-n", selinuxTest,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Check whether the profilebinding take effect !!!")
		cleanupObjects(oc, objectTableRef{"pod", selinuxTest, podErrorlogger})
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodTemplate, "-p", "NAME="+podErrorlogger, "NAMESPACE="+selinuxTest)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, podErrorlogger, selinuxTest)
		result1, _ := oc.AsAdmin().Run("logs").Args("pod/"+podErrorlogger, "-n", selinuxTest, "-c", "errorlogger").Output()
		o.Expect(result1).Should(o.BeEmpty())

		g.By("Operator upgrade..!!!\n")
		patchSub := fmt.Sprintf("{\"spec\":{\"source\":\"qe-app-registry\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "sub", subD.subName, "--type", "merge", "-p", patchSub, "-n", subD.namespace)
		// Sleep 10 sesonds so that the operator upgrade will be triggered
		time.Sleep(10 * time.Second)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Succeeded", ok, []string{"csv", "-n", subD.namespace,
			"-ojsonpath={.items[0].status.phase}"}).check(oc)
		newCsvName, errGet := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", subD.namespace, "-l", "operators.coreos.com/security-profiles-operator.security-profiles-operator=",
			"-o=jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(errGet).NotTo(o.HaveOccurred())
		newVersion := strings.ReplaceAll(newCsvName, "security-profiles-operator.v", "")
		o.Expect(newVersion).ShouldNot(o.Equal(oldVersion))
		checkReadyPodCountOfDeployment(oc, "security-profiles-operator", subD.namespace, 3)
		checkReadyPodCountOfDeployment(oc, "security-profiles-operator-webhook", subD.namespace, 3)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Check selinuxprofile status after operator upgrade!!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"selinuxprofiles", selinuxselinuxTest.name, "-o=jsonpath={.status.status}", "-n", selinuxTest}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, usageselinuxTest, ok, []string{"selinuxprofiles", selinuxselinuxTest.name, "-n", selinuxselinuxTest.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)
		assertKeywordsExistsInSelinuxFile(oc, selinuxselinuxTest.name+"_"+selinuxselinuxTest.namespace, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "cat", "/etc/selinux.d/"+selinuxTestfileName)
		assertKeywordsExistsInSelinuxFile(oc, selinuxselinuxTest.name+"_"+selinuxselinuxTest.namespace, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "semodule", "-l")
		result1, _ = oc.AsAdmin().Run("logs").Args("pod/"+podErrorlogger, "-n", selinuxTest, "-c", "errorlogger").Output()
		o.Expect(result1).Should(o.BeEmpty())

		g.By("Create one more selinuxprofile after operator upgrade!!!")
		niginxNs := "nginx-deploy" + getRandomString()
		selinuxNginx := "nginx-secure"
		defer deleteNamespace(oc, niginxNs)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", niginxNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("selinuxprofile", selinuxNginx, "-n", niginxNs, "--ignore-not-found").Execute()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxProfileNginxTemplate, "-p", "NAME="+selinuxNginx, "NAMESPACE="+niginxNs)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxNginx, "-o=jsonpath={.status.status}", "-n", niginxNs)
		usage := selinuxNginx + "_" + niginxNs + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usage, ok, []string{"selinuxprofiles", selinuxNginx, "-n",
			niginxNs, "-o=jsonpath={.status.usage}"}).check(oc)
		fileName := selinuxNginx + "_" + niginxNs + ".cil"
		assertKeywordsExistsInSelinuxFile(oc, selinuxNginx+"_"+niginxNs, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "cat", "/etc/selinux.d/"+fileName)
		assertKeywordsExistsInSelinuxFile(oc, selinuxNginx+"_"+niginxNs, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "semodule", "-l")

		g.By("Create pod and apply above selinuxprofile to the pod !!!")
		exutil.SetNamespacePrivileged(oc, niginxNs)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", selinuxNginx, "-n", niginxNs, "--ignore-not-found").Execute()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podWithSelinuxProfileTemplate, "-p", "NAME="+selinuxNginx, "NAMESPACE="+niginxNs, "USAGE="+usage)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", selinuxNginx, "-n", niginxNs, "-o=jsonpath={.status.phase}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, usage, ok, []string{"pod", selinuxNginx, "-n", niginxNs, "-o=jsonpath={.spec.containers[0].securityContext.seLinuxOptioniginxNs.type}"})
	})
})
