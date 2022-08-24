package securityandcompliance

import (
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance The Security Profiles Operator", func() {
	defer g.GinkgoRecover()

	var (
		oc                  = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir string
		ogSpoTemplate       string
		subSpoTemplate      string
		secProfileTemplate  string

		ogD                 operatorGroupDescription
		subD                subscriptionDescription
		seccompP, seccompP1 seccompProfile
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogSpoTemplate = filepath.Join(buildPruningBaseDir, "operator-group.yaml")
		subSpoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		secProfileTemplate = filepath.Join(buildPruningBaseDir, "seccompprofile.yaml")

		ogD = operatorGroupDescription{
			name:      "security-profiles-operator",
			namespace: "security-profiles-operator",
			template:  ogSpoTemplate,
		}

		subD = subscriptionDescription{
			subName:                "security-profiles-operator-sub",
			namespace:              "security-profiles-operator",
			channel:                "release-0.4",
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

	// author: minmli@redhat.com
	g.It("Author:minmli-High-50397-Create two seccompprofiles with the same name in different namespaces", func() {
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
