package securityandcompliance

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance The Security Profiles Operator", func() {
	defer g.GinkgoRecover()

	var (
		oc                      = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir     string
		ogSpoTemplate           string
		subSpoTemplate          string
		secProfileTemplate      string
		secProfileStackTemplate string
		podWithProfileTemplate  string

		ogD                 operatorGroupDescription
		subD                subscriptionDescription
		seccompP, seccompP1 seccompProfile
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogSpoTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subSpoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		secProfileTemplate = filepath.Join(buildPruningBaseDir, "seccompprofile.yaml")
		secProfileStackTemplate = filepath.Join(buildPruningBaseDir, "seccompprofilestack.yaml")
		podWithProfileTemplate = filepath.Join(buildPruningBaseDir, "pod-with-seccompprofile.yaml")

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
			catalogSourceName:      "qe-app-registry",
			catalogSourceNamespace: "openshift-marketplace",
			startingCSV:            "",
			currentCSV:             "",
			installedCSV:           "",
			template:               subSpoTemplate,
			singleNamespace:        true,
		}

		SkipMissingCatalogsource(oc)
		SkipARM64AndHetegenous(oc)

		createSecurityProfileOperator(oc, subD, ogD)
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-High-50078-Create two seccompprofiles with the same name in different namespaces", func() {
		seccompP = seccompProfile{
			name:      "sleep-sh-pod",
			namespace: "spo-ns-1",
			template:  secProfileTemplate,
		}
		seccompP1 = seccompProfile{
			name:      "sleep-sh-pod",
			namespace: "spo-ns-2",
			template:  secProfileTemplate,
		}

		g.By("Create seccompprofiles in different namespaces !!!")
		seccomps := [2]seccompProfile{seccompP, seccompP1}
		for _, seccompP := range seccomps {
			defer deleteNamespace(oc, seccompP.namespace)
			err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompP.namespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("seccompprofiles", seccompP.name, "-n", seccompP.namespace, "--ignore-not-found").Execute()
			seccompP.create(oc)
			newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", "--selector=spo.x-k8s.io/profile-id=SeccompProfile-sleep-sh-pod", "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
		}
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-High-49885-Check SeccompProfile stack working as expected", func() {
		ns := "spo-" + getRandomString()
		seccompP = seccompProfile{
			name:      "sleep-sh-pod",
			namespace: ns,
			template:  secProfileTemplate,
		}
		seccompProfilestack := seccompProfile{
			name:            "sleep-sh-pod-stack",
			namespace:       ns,
			baseprofilename: seccompP.name,
			template:        secProfileStackTemplate,
		}
		testPod1 := podWithProfile{
			name:             "pod-with-base-profile",
			namespace:        ns,
			localhostProfile: "",
			template:         podWithProfileTemplate,
		}
		testPod2 := podWithProfile{
			name:             "pod-with-stack-profile",
			namespace:        ns,
			localhostProfile: "",
			template:         podWithProfileTemplate,
		}

		g.By("Create base seccompprofile !!!")
		defer deleteNamespace(oc, seccompP.namespace)
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompP.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("seccompprofiles", seccompP.name, "-n", seccompP.namespace, "--ignore-not-found").Execute()
		seccompP.create(oc)

		g.By("Check base seccompprofile was installed correctly !!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", "--selector=spo.x-k8s.io/profile-id=SeccompProfile-" + seccompP.name, "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
		dir := "/var/lib/kubelet/seccomp/operator/" + seccompP.namespace + "/"
		secProfileName := seccompP.name + ".json"
		filePath := dir + secProfileName
		assertKeywordsExistsInFile(oc, "mkdir", filePath, false)

		g.By("Create seccompprofile stack !!!")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("seccompprofiles", seccompProfilestack.name, "-n", seccompProfilestack.namespace, "--ignore-not-found").Execute()
		seccompProfilestack.create(oc)

		g.By("Check base seccompprofile was installed correctly !!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", "--selector=spo.x-k8s.io/profile-id=SeccompProfile-" + seccompProfilestack.name, "-n", seccompProfilestack.namespace, "-o=jsonpath={.status.status}"})
		dir2 := "/var/lib/kubelet/seccomp/operator/" + seccompProfilestack.namespace + "/"
		secProfileStackName := seccompProfilestack.name + ".json"
		stackFilePath := dir2 + secProfileStackName
		assertKeywordsExistsInFile(oc, "mkdir", stackFilePath, true)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		testPod1.localhostProfile = "operator/" + ns + "/" + seccompP.name + ".json"
		testPod2.localhostProfile = "operator/" + ns + "/" + seccompProfilestack.name + ".json"
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", testPod1.name, "-n", ns, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", testPod2.name, "-n", ns, "--ignore-not-found").Execute()
		}()
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", testPod1.template, "-p", "NAME="+testPod1.name, "NAMESPACE="+testPod1.namespace, "BASEPROFILENAME="+testPod1.localhostProfile)
		o.Expect(err1).NotTo(o.HaveOccurred())
		err2 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", testPod2.template, "-p", "NAME="+testPod2.name, "NAMESPACE="+testPod2.namespace, "BASEPROFILENAME="+testPod2.localhostProfile)
		o.Expect(err2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", testPod1.name, "-n", testPod1.namespace, "-o=jsonpath={.status.phase}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", testPod2.name, "-n", testPod2.namespace, "-o=jsonpath={.status.phase}"})

		g.By("Check whether mkdir operator allowed in pod !!!")
		result1, _ := oc.AsAdmin().Run("exec").Args(testPod1.name, "-n", ns, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result1, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result1)
		}
		result2, _ := oc.AsAdmin().Run("exec").Args(testPod2.name, "-n", ns, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result2, "/tmpo/foo") && !strings.Contains(result2, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result2)
		}
	})

	// author: minmli@redhat.com
	g.It("Author:minmli-High-50397-check security profiles operator could be deleted successfully [Serial]", func() {
		defer func() {
			g.By("delete Security Profile Operator !!!")
			deleteNamespace(oc, subD.namespace)
			g.By("the Security Profile Operator is deleted successfully !!!")
		}()

		seccompP = seccompProfile{
			name:      "sleep-sh-pod",
			namespace: "spo",
			template:  secProfileTemplate,
		}

		g.By("Create a SeccompProfile in spo namespace !!!")
		defer deleteNamespace(oc, seccompP.namespace)
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompP.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("seccompprofiles", "--all", "-n", seccompP.namespace, "--ignore-not-found").Execute()
		seccompP.create(oc)

		g.By("Check the SeccompProfile is created sucessfully !!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", "--selector=spo.x-k8s.io/profile-id=SeccompProfile-sleep-sh-pod", "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n", subD.namespace, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("seccompprofiles", "--all", "-n", subD.namespace, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("selinuxprofiles", "--all", "-n", subD.namespace, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("rawselinuxprofiles", "--all", "-n", subD.namespace, "--ignore-not-found").Execute()
		g.By("delete seccompprofiles, selinuxprofiles and rawselinuxprofiles !!!")
	})
})
