package securityandcompliance

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"path/filepath"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-isc] Security_and_Compliance The Security Profiles Operator", func() {
	defer g.GinkgoRecover()

	var (
		oc                            = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir           string
		ogSpoTemplate                 string
		subSpoTemplate                string
		secProfileTemplate            string
		secProfileStackTemplate       string
		podWithProfileTemplate        string
		selinuxProfileNginxTemplate   string
		podWithSelinuxProfileTemplate string
		profileRecordingTemplate      string
		saRoleRolebindingTemplate     string
		workloadDaeTemplate           string
		workloadDeployTemplate        string

		ogD                 operatorGroupDescription
		subD                subscriptionDescription
		seccompP, seccompP1 seccompProfile
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		ogSpoTemplate = filepath.Join(buildPruningBaseDir, "operator-group-all-namespaces.yaml")
		subSpoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		secProfileTemplate = filepath.Join(buildPruningBaseDir, "seccompprofile.yaml")
		secProfileStackTemplate = filepath.Join(buildPruningBaseDir, "seccompprofilestack.yaml")
		podWithProfileTemplate = filepath.Join(buildPruningBaseDir, "pod-with-seccompprofile.yaml")
		selinuxProfileNginxTemplate = filepath.Join(buildPruningBaseDir, "/spo/selinux-profile-nginx.yaml")
		podWithSelinuxProfileTemplate = filepath.Join(buildPruningBaseDir, "/spo/pod-with-selinux-profile.yaml")
		profileRecordingTemplate = filepath.Join(buildPruningBaseDir, "/spo/profile-recording.yaml")
		saRoleRolebindingTemplate = filepath.Join(buildPruningBaseDir, "/spo/sa-previleged-role-rolebinding.yaml")
		workloadDaeTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-daemonset.yaml")
		workloadDeployTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-deployment.yaml")

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
		exutil.SkipARM64(oc)

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
	g.It("StagerunBoth-Author:xiyuan-High-49885-Check SeccompProfile stack working as expected", func() {
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
		exutil.SetNamespacePrivileged(oc, testPod1.namespace)
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", testPod1.template, "-p", "NAME="+testPod1.name, "NAMESPACE="+testPod1.namespace, "BASEPROFILENAME="+testPod1.localhostProfile)
		o.Expect(err1).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, testPod2.namespace)
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

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-StagerunBoth-Author:xiyuan-High-56704-Create a SelinuxProfile and apply it to pod", func() {
		ns := "nginx-deploy" + getRandomString()
		selinuxProfileName := "nginx-secure"

		g.By("Create selinuxprofile !!!")
		defer deleteNamespace(oc, ns)
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("selinuxprofile", selinuxProfileName, "-n", ns, "--ignore-not-found").Execute()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxProfileNginxTemplate, "-p", "NAME="+selinuxProfileName, "NAMESPACE="+ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxProfileName, "-o=jsonpath={.status.status}", "-n", ns)
		usage := selinuxProfileName + "_" + ns + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usage, ok, []string{"selinuxprofiles", selinuxProfileName, "-n",
			ns, "-o=jsonpath={.status.usage}"}).check(oc)
		fileName := selinuxProfileName + "_" + ns + ".cil"
		assertKeywordsExistsInSelinuxFile(oc, usage, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "cat", "/etc/selinux.d/"+fileName)
		assertKeywordsExistsInSelinuxFile(oc, selinuxProfileName+"_"+ns, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "semodule", "-l")

		g.By("Create pod and apply above selinuxprofile to the pod !!!")
		lableNamespace(oc, "namespace", ns, "-n", ns, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", selinuxProfileName, "-n", ns, "--ignore-not-found").Execute()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podWithSelinuxProfileTemplate, "-p", "NAME="+selinuxProfileName, "NAMESPACE="+ns, "USAGE="+usage)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check pod status and seLinuxOptions type of the pod !!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", selinuxProfileName, "-n", ns, "-o=jsonpath={.status.phase}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, usage, ok, []string{"pod", selinuxProfileName, "-n", ns, "-o=jsonpath={.spec.containers[0].securityContext.seLinuxOptions.type}"})
	})

	// author: xiyuan@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-Medium-50242-High-50174-check Log enricher based seccompprofile recording and metrics working as expected for daemonset/deployment [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingDae = profileRecordingDescription{
				name:       "spo-recording1",
				namespace:  ns1,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "hello-daemonset",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingDae = saRoleRoleBindingDescription{
				saName:          "spo-record-sa1",
				namespace:       ns1,
				roleName:        "spo-record1" + getRandomString(),
				roleBindingName: "spo-record1" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			daemonsetHello = workloadDescription{
				name:         "hello-daemonset",
				namespace:    ns1,
				workloadKind: "DaemonSet",
				saName:       saRoleRoleBindingDae.saName,
				labelKey:     profileRecordingDae.labelKey,
				labelValue:   profileRecordingDae.labelValue,
				template:     workloadDaeTemplate,
			}
			profileRecordingDep = profileRecordingDescription{
				name:       "spo-recording2",
				namespace:  ns2,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "hello-openshift",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingDep = saRoleRoleBindingDescription{
				saName:          "spo-record-sa2",
				namespace:       ns2,
				roleName:        "spo-record2" + getRandomString(),
				roleBindingName: "spo-record2" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			deployHello = workloadDescription{
				name:         "hello-deployment",
				namespace:    ns2,
				workloadKind: "Deployment",
				saName:       saRoleRoleBindingDep.saName,
				labelKey:     profileRecordingDep.labelKey,
				labelValue:   profileRecordingDep.labelValue,
				template:     workloadDeployTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		nodeCount := getNodeCount(oc)
		checkReadyPodCountOfDaemonset(oc, "spod", subD.namespace, nodeCount)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", ns1, profileRecordingDae.name},
			objectTableRef{"profilerecording", ns2, profileRecordingDep.name})
		profileRecordingDae.create(oc)
		profileRecordingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingDae.name, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingDep.name, "-n", ns2}).check(oc)

		g.By("Create sa, role, rolebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"sa", ns1, saRoleRoleBindingDae.saName},
			objectTableRef{"sa", ns2, saRoleRoleBindingDep.saName},
			objectTableRef{"role", ns1, saRoleRoleBindingDae.roleName},
			objectTableRef{"role", ns2, saRoleRoleBindingDep.roleName},
			objectTableRef{"rolebinding", ns1, saRoleRoleBindingDae.roleBindingName},
			objectTableRef{"rolebinding", ns2, saRoleRoleBindingDep.roleBindingName})
		saRoleRoleBindingDae.create(oc)
		saRoleRoleBindingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingDae.saName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingDep.saName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingDae.roleName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingDep.roleName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingDae.roleBindingName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingDep.roleBindingName, "-n", ns2}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"daemonset", ns1, daemonsetHello.name},
			objectTableRef{"deploy", ns2, deployHello.name})
		daemonsetHello.create(oc)
		deployHello.create(oc)
		linuxWorkerCount := getLinuxWorkerCount(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, strconv.Itoa(linuxWorkerCount), ok, []string{"daemonset", daemonsetHello.name, "-n", ns1, "-o=jsonpath={.status.numberReady}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"deploy", deployHello.name, "-n", ns2, "-o=jsonpath={.status.availableReplicas}"}).check(oc)

		g.By("Check seccompprofile generated !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"seccompprofile", ns1, "--all"})
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"seccompprofile", ns2, "--all"})
		//sleep 120s so the seccompprofiles of the worklod could be recorded
		time.Sleep(120 * time.Second)
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("daemonset", daemonsetHello.name, "-n", ns1, "--ignore-not-found").Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("deploy", deployHello.name, "-n", ns2, "--ignore-not-found").Execute()
		checkPrfolieStatus(oc, "seccompprofile", ns1, "Installed")
		checkPrfolieStatus(oc, "seccompprofile", ns2, "Installed")
		checkPrfolieNumbers(oc, "seccompprofile", ns1, linuxWorkerCount*2)
		checkPrfolieNumbers(oc, "seccompprofile", ns2, 6)
	})
	// author: xiyuan@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-Medium-50262-High-50263-check Log enricher based selinuxprofile recording and metrics working as expected for daemonset/deployment [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingDae = profileRecordingDescription{
				name:       "spo-recording1",
				namespace:  ns1,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "hello-daemonset",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingDae = saRoleRoleBindingDescription{
				saName:          "spo-record-sa1",
				namespace:       ns1,
				roleName:        "spo-record1" + getRandomString(),
				roleBindingName: "spo-record1" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			daemonsetHello = workloadDescription{
				name:         "hello-daemonset",
				namespace:    ns1,
				workloadKind: "DaemonSet",
				saName:       saRoleRoleBindingDae.saName,
				labelKey:     profileRecordingDae.labelKey,
				labelValue:   profileRecordingDae.labelValue,
				template:     workloadDaeTemplate,
			}
			profileRecordingDep = profileRecordingDescription{
				name:       "spo-recording2",
				namespace:  ns2,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "hello-openshift",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingDep = saRoleRoleBindingDescription{
				saName:          "spo-record-sa2",
				namespace:       ns2,
				roleName:        "spo-record2" + getRandomString(),
				roleBindingName: "spo-record2" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			deployHello = workloadDescription{
				name:         "hello-deployment",
				namespace:    ns2,
				workloadKind: "Deployment",
				saName:       saRoleRoleBindingDep.saName,
				labelKey:     profileRecordingDep.labelKey,
				labelValue:   profileRecordingDep.labelValue,
				template:     workloadDeployTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		nodeCount := getNodeCount(oc)
		checkReadyPodCountOfDaemonset(oc, "spod", subD.namespace, nodeCount)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", ns1, profileRecordingDae.name},
			objectTableRef{"profilerecording", ns2, profileRecordingDep.name})
		profileRecordingDae.create(oc)
		profileRecordingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingDae.name, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingDep.name, "-n", ns2}).check(oc)

		g.By("Create sa, role, rolebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"sa", ns1, saRoleRoleBindingDae.saName},
			objectTableRef{"sa", ns2, saRoleRoleBindingDep.saName},
			objectTableRef{"role", ns1, saRoleRoleBindingDae.roleName},
			objectTableRef{"role", ns2, saRoleRoleBindingDep.roleName},
			objectTableRef{"rolebinding", ns1, saRoleRoleBindingDae.roleBindingName},
			objectTableRef{"rolebinding", ns2, saRoleRoleBindingDep.roleBindingName})
		saRoleRoleBindingDae.create(oc)
		saRoleRoleBindingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingDae.saName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingDep.saName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingDae.roleName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingDep.roleName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingDae.roleBindingName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingDep.roleBindingName, "-n", ns2}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"daemonset", ns1, daemonsetHello.name},
			objectTableRef{"deploy", ns2, deployHello.name})
		daemonsetHello.create(oc)
		deployHello.create(oc)
		linuxWorkerCount := getLinuxWorkerCount(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, strconv.Itoa(linuxWorkerCount), ok, []string{"daemonset", daemonsetHello.name, "-n", ns1, "-o=jsonpath={.status.numberReady}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"deploy", deployHello.name, "-n", ns2, "-o=jsonpath={.status.availableReplicas}"}).check(oc)

		g.By("Check seccompprofile generated !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SelinuxProfile", ns1, "--all"})
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SelinuxProfile", ns2, "--all"})
		//sleep 60s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(60 * time.Second)
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("daemonset", daemonsetHello.name, "-n", ns1, "--ignore-not-found").Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("deploy", deployHello.name, "-n", ns2, "--ignore-not-found").Execute()
		checkPrfolieStatus(oc, "SelinuxProfile", ns1, "Installed")
		checkPrfolieStatus(oc, "SelinuxProfile", ns2, "Installed")
		checkPrfolieNumbers(oc, "SelinuxProfile", ns1, linuxWorkerCount*2)
		checkPrfolieNumbers(oc, "SelinuxProfile", ns2, 6)
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
