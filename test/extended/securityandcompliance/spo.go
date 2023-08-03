package securityandcompliance

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

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
		profileBindingTemplate        string
		saRoleRolebindingTemplate     string
		selinuxProfileCustomTemplate  string
		workloadDaeTemplate           string
		workloadDeployTemplate        string
		pathWebhookBinding            string
		pathWebhookRecording          string
		podWithLabelsTemplate         string
		podWithOneLabelTemplate       string
		workloadRepTemplate           string
		workloadCronjobTemplate       string
		workloadPodTemplate           string
		workloadJobTemplate           string

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
		selinuxProfileCustomTemplate = filepath.Join(buildPruningBaseDir, "/spo/selinux-profile-with-custom-policy-template.yaml")
		pathWebhookBinding = filepath.Join(buildPruningBaseDir, "/spo/patch-webhook-binding.yaml")
		pathWebhookRecording = filepath.Join(buildPruningBaseDir, "/spo/patch-webhook-recording.yaml")
		podWithSelinuxProfileTemplate = filepath.Join(buildPruningBaseDir, "/spo/pod-with-selinux-profile.yaml")
		podWithLabelsTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-pod-with-labels.yaml")
		podWithOneLabelTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-pod-with-one-label.yaml")
		profileRecordingTemplate = filepath.Join(buildPruningBaseDir, "/spo/profile-recording.yaml")
		profileBindingTemplate = filepath.Join(buildPruningBaseDir, "/spo/profile-binding.yaml")
		saRoleRolebindingTemplate = filepath.Join(buildPruningBaseDir, "/spo/sa-previleged-role-rolebinding.yaml")
		workloadDaeTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-daemonset.yaml")
		workloadDeployTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-deployment.yaml")
		workloadRepTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-replicaset.yaml")
		workloadCronjobTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-cronjob.yaml")
		workloadPodTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-pod-with-one-label.yaml")
		workloadJobTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-job.yaml")

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
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
		SkipClustersWithRhelNodes(oc)

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
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

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
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

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

	// author: xiyuan@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-High-61609-High-61599-check enable memory optimization in spod could work for seccompprofiles/selinuxprofiles recording for pod [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingSec = profileRecordingDescription{
				name:       "spo-recording-sec-" + getRandomString(),
				namespace:  ns1,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "my-app",
				template:   profileRecordingTemplate,
			}
			podWithoutLabelSec = workloadDescription{
				name:         "pod-without-label" + getRandomString(),
				namespace:    ns1,
				workloadKind: "Pod",
				labelKey:     profileRecordingSec.labelKey,
				labelValue:   profileRecordingSec.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
			podWithLabelsSec = workloadDescription{
				name:         "pod-with-label" + getRandomString(),
				namespace:    ns1,
				workloadKind: "Pod",
				labelKey:     profileRecordingSec.labelKey,
				labelValue:   profileRecordingSec.labelValue,
				labelKey2:    "spo.x-k8s.io/enable-recording",
				labelValue2:  "true",
				imageName:    "nginx",
				image:        "quay.io/security-profiles-operator/test-nginx-unprivileged:1.21",
				template:     podWithLabelsTemplate,
			}
			profileRecordingSel = profileRecordingDescription{
				name:       "spo-recording-sel-" + getRandomString(),
				namespace:  ns2,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "my-app",
				template:   profileRecordingTemplate,
			}
			podWithoutLabelSel = workloadDescription{
				name:         "pod-without-label" + getRandomString(),
				namespace:    ns2,
				workloadKind: "Pod",
				labelKey:     profileRecordingSec.labelKey,
				labelValue:   profileRecordingSec.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
			podWithLabelsSel = workloadDescription{
				name:         "pod-with-label" + getRandomString(),
				namespace:    ns2,
				workloadKind: "Pod",
				labelKey:     profileRecordingSel.labelKey,
				labelValue:   profileRecordingSel.labelValue,
				labelKey2:    "spo.x-k8s.io/enable-recording",
				labelValue2:  "true",
				imageName:    "nginx",
				image:        "quay.io/security-profiles-operator/test-nginx-unprivileged:1.21",
				template:     podWithLabelsTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		defer func() {
			patch := fmt.Sprintf("{\"spec\":{\"enableMemoryOptimization\":false}}")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
			// trigger spod daemonset restart manually before bug https://issues.redhat.com/browse/OCPBUGS-14063 fixed
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("daemonsets", "spod", "-n", subD.namespace, "--ignore-not-found").Execute()
			checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		}()
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true,\"enableMemoryOptimization\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns1)
		exutil.SetNamespacePrivileged(oc, ns2)

		g.By("Create profilerecording !!!")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("profilerecording", profileRecordingSec.name, "-n", ns1, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("profilerecording", profileRecordingSel.name, "-n", ns2, "--ignore-not-found").Execute()
		profileRecordingSec.create(oc)
		profileRecordingSel.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingSec.name, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingSel.name, "-n", ns2}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns1, podWithoutLabelSec.name},
			objectTableRef{"pod", ns1, podWithLabelsSec.name},
			objectTableRef{"pod", ns2, podWithoutLabelSel.name},
			objectTableRef{"pod", ns2, podWithLabelsSel.name})
		podWithoutLabelSec.create(oc)
		podWithLabelsSec.create(oc)
		podWithoutLabelSel.create(oc)
		podWithLabelsSel.create(oc)
		exutil.AssertPodToBeReady(oc, podWithoutLabelSec.name, ns1)
		exutil.AssertPodToBeReady(oc, podWithLabelsSec.name, ns1)
		exutil.AssertPodToBeReady(oc, podWithoutLabelSel.name, ns2)
		exutil.AssertPodToBeReady(oc, podWithLabelsSel.name, ns2)

		g.By("Check seccompprofile/selinuxprofile recorded !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SeccompProfile", ns1, "--all"})
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SelinuxProfile", ns2, "--all"})
		//sleep 30s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(30 * time.Second)
		cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns1, podWithoutLabelSec.name},
			objectTableRef{"pod", ns1, podWithLabelsSec.name},
			objectTableRef{"pod", ns2, podWithoutLabelSel.name},
			objectTableRef{"pod", ns2, podWithLabelsSel.name})
		checkPrfolieNumbers(oc, "SeccompProfile", ns1, 1)
		checkPrfolieNumbers(oc, "SelinuxProfile", ns2, 1)
		checkPrfolieStatus(oc, "SelinuxProfile", ns1, "Installed")
		checkPrfolieStatus(oc, "SelinuxProfile", ns2, "Installed")

		g.By("Check seccompprofile/selinuxprofile name is expected !!!")
		spShouldNotExists := profileRecordingSec.name + "-" + podWithoutLabelSel.imageName
		spShouldExists := profileRecordingSec.name + "-" + podWithLabelsSec.imageName
		selinuxProfileShouldNotExists := profileRecordingSel.name + "-" + podWithoutLabelSel.imageName
		selinuxProfileShouldExists := profileRecordingSel.name + "-" + podWithLabelsSel.imageName
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"SeccompProfile", spShouldNotExists, "-n", ns1}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, spShouldExists, ok, []string{"SeccompProfile", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"selinuxprofile", selinuxProfileShouldNotExists,
			"-n", ns2}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, selinuxProfileShouldExists, ok, []string{"selinuxprofiles", "-n", ns2,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-High-51391-Log based selinuxprofile recording could make use of webhookOptions object in webhooks [Slow][Disruptive]", func() {
		ns1 := "do-record-" + getRandomString()
		ns2 := "dont-record1-" + getRandomString()
		ns3 := "dont-record2-" + getRandomString()

		var (
			profileRecordingNs1 = profileRecordingDescription{
				name:       "spo-recording-sel-" + getRandomString(),
				namespace:  ns1,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "my-app",
				template:   profileRecordingTemplate,
			}
			podNs1 = workloadDescription{
				name:         "my-pod" + getRandomString(),
				namespace:    ns1,
				workloadKind: "Pod",
				labelKey:     profileRecordingNs1.labelKey,
				labelValue:   profileRecordingNs1.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
			profileRecordingNs2 = profileRecordingDescription{
				name:       "spo-recording-sel-" + getRandomString(),
				namespace:  ns2,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "my-app",
				template:   profileRecordingTemplate,
			}
			podNs2 = workloadDescription{
				name:         "my-pod" + getRandomString(),
				namespace:    ns2,
				workloadKind: "Pod",
				labelKey:     profileRecordingNs2.labelKey,
				labelValue:   profileRecordingNs2.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
			profileRecordingNs3 = profileRecordingDescription{
				name:       "spo-recording-sel-" + getRandomString(),
				namespace:  ns3,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "my-test",
				template:   profileRecordingTemplate,
			}
			podNs3 = workloadDescription{
				name:         "my-pod" + getRandomString(),
				namespace:    ns3,
				workloadKind: "Pod",
				labelKey:     profileRecordingNs3.labelKey,
				labelValue:   profileRecordingNs3.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Update webhookOptions.. !!!")
		defer func() {
			patchRecover := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/webhookOptions\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "--type", "json", "--patch", patchRecover, "-n", subD.namespace)
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("daemonsets", "spod", "-n", subD.namespace, "--ignore-not-found").Execute()
			newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/enable-recording", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
				subD.namespace, "-o=jsonpath={.webhooks[1].namespaceSelector.matchExpressions[0].key}"}).check(oc)
		}()
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "--patch-file", pathWebhookRecording)
		newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/record-here", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
			subD.namespace, "-o=jsonpath={.webhooks[1].namespaceSelector.matchExpressions[0].key}"}).check(oc)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2},
			objectTableRef{"ns", ns3, ns3})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		//kubectl label --overwrite pods foo status=unhealthy
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/record-here=true", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns3).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns3, "-n", ns3, "spo.x-k8s.io/record-here=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns1)
		exutil.SetNamespacePrivileged(oc, ns2)
		exutil.SetNamespacePrivileged(oc, ns3)

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", profileRecordingNs1.name, ns1},
			objectTableRef{"profilerecording", profileRecordingNs2.name, ns2},
			objectTableRef{"profilerecording", profileRecordingNs3.name, ns3})
		profileRecordingNs1.create(oc)
		profileRecordingNs2.create(oc)
		profileRecordingNs3.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileRecordingNs1.name, ok, []string{"profilerecording", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileRecordingNs2.name, ok, []string{"profilerecording", "-n", ns2,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileRecordingNs3.name, ok, []string{"profilerecording", "-n", ns3,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns1, podNs1.name},
			objectTableRef{"pod", ns2, podNs2.name},
			objectTableRef{"pod", ns3, podNs3.name})
		podNs1.create(oc)
		podNs2.create(oc)
		podNs3.create(oc)
		exutil.AssertPodToBeReady(oc, podNs1.name, ns1)
		exutil.AssertPodToBeReady(oc, podNs2.name, ns2)
		exutil.AssertPodToBeReady(oc, podNs3.name, ns3)

		g.By("Check seccompprofile/selinuxprofile recorded !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SelinuxProfile", ns1, "--all"})
		//sleep 30s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(30 * time.Second)
		cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns1, podNs1.name},
			objectTableRef{"pod", ns2, podNs1.name},
			objectTableRef{"pod", ns3, podNs1.name})
		checkPrfolieNumbers(oc, "selinuxprofile", ns1, 1)
		checkPrfolieStatus(oc, "selinuxprofile", ns1, "Installed")
		selinuxProfileShouldExists := profileRecordingNs1.name + "-" + podNs1.imageName
		newCheck("expect", asAdmin, withoutNamespace, contain, selinuxProfileShouldExists, ok, []string{"selinuxprofile", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"selinuxprofile", "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"selinuxprofile", "-n", ns3}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-High-51392-Log based secompprofile recording could make use of webhookOptions object in webhooks [Slow][Disruptive]", func() {
		ns1 := "do-record-" + getRandomString()
		ns2 := "dont-record1-" + getRandomString()
		ns3 := "dont-record2-" + getRandomString()

		var (
			profileRecordingNs1 = profileRecordingDescription{
				name:       "spo-recording-sec-" + getRandomString(),
				namespace:  ns1,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "my-app",
				template:   profileRecordingTemplate,
			}
			podNs1 = workloadDescription{
				name:         "my-pod" + getRandomString(),
				namespace:    ns1,
				workloadKind: "Pod",
				labelKey:     profileRecordingNs1.labelKey,
				labelValue:   profileRecordingNs1.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
			profileRecordingNs2 = profileRecordingDescription{
				name:       "spo-recording-sec-" + getRandomString(),
				namespace:  ns2,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "my-app",
				template:   profileRecordingTemplate,
			}
			podNs2 = workloadDescription{
				name:         "my-pod" + getRandomString(),
				namespace:    ns2,
				workloadKind: "Pod",
				labelKey:     profileRecordingNs2.labelKey,
				labelValue:   profileRecordingNs2.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
			profileRecordingNs3 = profileRecordingDescription{
				name:       "spo-recording-sec-" + getRandomString(),
				namespace:  ns3,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "my-test",
				template:   profileRecordingTemplate,
			}
			podNs3 = workloadDescription{
				name:         "my-pod" + getRandomString(),
				namespace:    ns3,
				workloadKind: "Pod",
				labelKey:     profileRecordingNs3.labelKey,
				labelValue:   profileRecordingNs3.labelValue,
				imageName:    "redis",
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				template:     podWithOneLabelTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Update webhookOptions.. !!!")
		defer func() {
			patchRecover := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/webhookOptions\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "--type", "json", "--patch", patchRecover, "-n", subD.namespace)
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("daemonsets", "spod", "-n", subD.namespace, "--ignore-not-found").Execute()
			newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/enable-recording", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
				subD.namespace, "-o=jsonpath={.webhooks[1].namespaceSelector.matchExpressions[0].key}"}).check(oc)
		}()
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "--patch-file", pathWebhookRecording)
		newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/record-here", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
			subD.namespace, "-o=jsonpath={.webhooks[1].namespaceSelector.matchExpressions[0].key}"}).check(oc)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2},
			objectTableRef{"ns", ns3, ns3})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		//kubectl label --overwrite pods foo status=unhealthy
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/record-here=true", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns3).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns3, "-n", ns3, "spo.x-k8s.io/record-here=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns1)
		exutil.SetNamespacePrivileged(oc, ns2)
		exutil.SetNamespacePrivileged(oc, ns3)

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", profileRecordingNs1.name, ns1},
			objectTableRef{"profilerecording", profileRecordingNs2.name, ns2},
			objectTableRef{"profilerecording", profileRecordingNs3.name, ns3})
		profileRecordingNs1.create(oc)
		profileRecordingNs2.create(oc)
		profileRecordingNs3.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileRecordingNs1.name, ok, []string{"profilerecording", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileRecordingNs2.name, ok, []string{"profilerecording", "-n", ns2,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileRecordingNs3.name, ok, []string{"profilerecording", "-n", ns3,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns1, podNs1.name},
			objectTableRef{"pod", ns2, podNs2.name},
			objectTableRef{"pod", ns3, podNs3.name})
		podNs1.create(oc)
		podNs2.create(oc)
		podNs3.create(oc)
		exutil.AssertPodToBeReady(oc, podNs1.name, ns1)
		exutil.AssertPodToBeReady(oc, podNs2.name, ns2)
		exutil.AssertPodToBeReady(oc, podNs3.name, ns3)

		g.By("Check seccompprofile/selinuxprofile recorded !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"seccompprofile", ns1, "--all"})
		//sleep 30s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(30 * time.Second)
		cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns1, podNs1.name},
			objectTableRef{"pod", ns2, podNs1.name},
			objectTableRef{"pod", ns3, podNs1.name})
		checkPrfolieNumbers(oc, "seccompprofile", ns1, 1)
		checkPrfolieStatus(oc, "seccompprofile", ns1, "Installed")
		selinuxProfileShouldExists := profileRecordingNs1.name + "-" + podNs1.imageName
		newCheck("expect", asAdmin, withoutNamespace, contain, selinuxProfileShouldExists, ok, []string{"seccompprofile", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"seccompprofile", "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"seccompprofile", "-n", ns3}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-High-51405-Check profilebinding could make use of webhookOptions object in webhooks for seccompprofile [Disruptive]", func() {
		ns1 := "do-binding-" + getRandomString()
		ns2 := "dont-binding-" + getRandomString()
		podBusybox := filepath.Join(buildPruningBaseDir, "spo/pod-busybox.yaml")
		podBusyboxNs1 := "pod-busybox-ns1"
		podBusyboxNs2 := "pod-busybox-ns2"

		var (
			seccompNs1 = seccompProfile{
				name:      "sleep-sh-pod-" + getRandomString(),
				namespace: ns1,
				template:  secProfileTemplate,
			}
			profileBindingNs1 = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   ns1,
				kind:        "SeccompProfile",
				profilename: seccompNs1.name,
				image:       "quay.io/openshifttest/busybox:latest",
				template:    profileBindingTemplate,
			}
			seccompNs2 = seccompProfile{
				name:      "sleep-sh-pod-" + getRandomString(),
				namespace: ns2,
				template:  secProfileTemplate,
			}
			profileBindingNs2 = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   ns2,
				kind:        "SeccompProfile",
				profilename: seccompNs2.name,
				image:       "quay.io/openshifttest/busybox:latest",
				template:    profileBindingTemplate,
			}
		)

		g.By("Update webhookOptions.. !!!")
		defer func() {
			patchRecover := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/webhookOptions\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "--type", "json", "--patch", patchRecover, "-n", subD.namespace)
			newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/enable-binding", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
				subD.namespace, "-o=jsonpath={.webhooks[0].namespaceSelector.matchExpressions[0].key}"}).check(oc)
		}()
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "--patch-file", pathWebhookBinding)
		newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/bind-here", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
			subD.namespace, "-o=jsonpath={.webhooks[0].namespaceSelector.matchExpressions[0].key}"}).check(oc)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/bind-here=true", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, ns1)
		exutil.SetNamespacePrivileged(oc, ns2)

		g.By("Create seccompprofiles and check status !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"seccompprofile", seccompNs1.name, ns1},
			objectTableRef{"seccompprofile", seccompNs1.name, ns2})
		seccompNs1.create(oc)
		seccompNs2.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccompNs1.name, "-n", ns1, "-o=jsonpath={.status.status}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccompNs2.name, "-n", ns2, "-o=jsonpath={.status.status}"})

		g.By("Create profilebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilebinding", profileBindingNs1.name, ns1},
			objectTableRef{"profilebinding", profileBindingNs2.name, ns2})
		profileBindingNs1.create(oc)
		profileBindingNs2.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingNs1.name, ok, []string{"profilebinding", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingNs2.name, ok, []string{"profilebinding", "-n", ns2,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podBusyboxNs1, "-n", ns1, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podBusyboxNs2, "-n", ns2, "--ignore-not-found").Execute()
		}()
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podBusybox, "-p", "NAME="+podBusyboxNs1, "NAMESPACE="+ns1)
		o.Expect(err1).NotTo(o.HaveOccurred())
		err2 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podBusybox, "-p", "NAME="+podBusyboxNs2, "NAMESPACE="+ns2)
		o.Expect(err2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", podBusyboxNs1, "-n", ns1, "-o=jsonpath={.status.phase}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", podBusyboxNs2, "-n", ns2, "-o=jsonpath={.status.phase}"})

		g.By("Check whether mkdir operator allowed for pods !!!")
		result1, _ := oc.AsAdmin().Run("exec").Args(podBusyboxNs1, "-n", ns1, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result1, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result1)
		}
		result2, _ := oc.AsAdmin().Run("exec").Args(podBusyboxNs2, "-n", ns2, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result2, "/tmpo/foo") && !strings.Contains(result2, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result2)
		}
	})

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-High-51408-Check profilebinding could make use of webhookOptions object in webhooks for selinuxprofile [Disruptive]", func() {
		ns1 := "do-binding-" + getRandomString()
		ns2 := "dont-binding-" + getRandomString()
		errorLoggerSelTemplate := filepath.Join(buildPruningBaseDir, "spo/selinux-profile-errorlogger.yaml")
		errorLoggerPodTemplate := filepath.Join(buildPruningBaseDir, "spo/pod-errorlogger.yaml")
		podErrorloggerNs1 := "pod-errorlogger-ns1"
		podErrorloggerNs2 := "pod-errorlogger-ns2"

		var (
			selinuxNs1 = selinuxProfile{
				name:      "error-logger-" + getRandomString(),
				namespace: ns1,
				template:  errorLoggerSelTemplate,
			}
			profileBindingNs1 = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   ns1,
				kind:        "SelinuxProfile",
				profilename: selinuxNs1.name,
				image:       "quay.io/openshifttest/busybox",
				template:    profileBindingTemplate,
			}
			selinuxNs2 = seccompProfile{
				name:      "sleep-sh-pod-" + getRandomString(),
				namespace: ns2,
				template:  errorLoggerSelTemplate,
			}
			profileBindingNs2 = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   ns2,
				kind:        "SelinuxProfile",
				profilename: selinuxNs2.name,
				image:       "quay.io/openshifttest/busybox",
				template:    profileBindingTemplate,
			}
		)

		g.By("Update webhookOptions.. !!!")
		defer func() {
			patchRecover := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/webhookOptions\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "--type", "json", "--patch", patchRecover, "-n", subD.namespace)
			newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/enable-binding", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
				subD.namespace, "-o=jsonpath={.webhooks[0].namespaceSelector.matchExpressions[0].key}"}).check(oc)
		}()
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "--patch-file", pathWebhookBinding)
		newCheck("expect", asAdmin, withoutNamespace, contain, "spo.x-k8s.io/bind-here", ok, []string{"mutatingwebhookconfiguration", "spo-mutating-webhook-configuration", "-n",
			subD.namespace, "-o=jsonpath={.webhooks[0].namespaceSelector.matchExpressions[0].key}"}).check(oc)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/bind-here=true", "--overwrite=true")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns2, "-n", ns1, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns1)
		exutil.SetNamespacePrivileged(oc, ns2)

		g.By("Create selinuxprofile and check status !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"selinuxprofile", selinuxNs1.name, ns1},
			objectTableRef{"selinuxprofile", selinuxNs2.name, ns2})
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxNs1.template, "-p", "NAME="+selinuxNs1.name, "NAMESPACE="+selinuxNs1.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxNs2.template, "-p", "NAME="+selinuxNs2.name, "NAMESPACE="+selinuxNs2.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxNs1.name, "-o=jsonpath={.status.status}", "-n", ns1)
		usageNs1 := selinuxNs1.name + "_" + selinuxNs1.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usageNs1, ok, []string{"selinuxprofiles", selinuxNs1.name, "-n", selinuxNs1.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxNs2.name, "-o=jsonpath={.status.status}", "-n", ns2)
		usageNs2 := selinuxNs2.name + "_" + selinuxNs2.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usageNs2, ok, []string{"selinuxprofiles", selinuxNs2.name, "-n", selinuxNs2.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)

		g.By("Create profilebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilebinding", profileBindingNs1.name, ns1},
			objectTableRef{"profilebinding", profileBindingNs2.name, ns2})
		profileBindingNs1.create(oc)
		profileBindingNs2.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingNs1.name, ok, []string{"profilebinding", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingNs2.name, ok, []string{"profilebinding", "-n", ns2,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podErrorloggerNs1, "-n", ns1, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podErrorloggerNs2, "-n", ns2, "--ignore-not-found").Execute()
		}()
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodTemplate, "-p", "NAME="+podErrorloggerNs1, "NAMESPACE="+ns1)
		o.Expect(err1).NotTo(o.HaveOccurred())
		err2 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodTemplate, "-p", "NAME="+podErrorloggerNs2, "NAMESPACE="+ns2)
		o.Expect(err2).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, podErrorloggerNs1, ns1)
		exutil.AssertPodToBeReady(oc, podErrorloggerNs2, ns2)

		g.By("Check whether the profilebinding take effect !!!")
		result1, _ := oc.AsAdmin().Run("logs").Args("pod/"+podErrorloggerNs1, "-n", ns1, "-c", "errorlogger").Output()
		o.Expect(result1).Should(o.BeEmpty())
		result2, _ := oc.AsAdmin().Run("logs").Args("pod/"+podErrorloggerNs2, "-n", ns2, "-c", "errorlogger").Output()
		o.Expect(result2).Should(o.ContainSubstring("Permission denied"))
	})

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-Medium-61581-Verify a custom container-selinux policy templates could be used [Serial]", func() {
		ns := "net-container-policy-" + getRandomString()
		selinuxProfileName := "net-container-policy"

		g.By("Create selinuxprofile !!!")
		defer deleteNamespace(oc, ns)
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("selinuxprofile", selinuxProfileName, "-n", ns, "--ignore-not-found").Execute()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxProfileCustomTemplate, "-p", "NAME="+selinuxProfileName, "NAMESPACE="+ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Error", "selinuxprofiles", selinuxProfileName, "-o=jsonpath={.status.status}", "-n", ns)

		g.By("Patch the allowedSystemProfiles and recreate the selinux profile !!!\n")
		defer func() {
			g.By("Recover the default allowedSystemProfiles.. !!!\n")
			patchRecover := fmt.Sprintf("{\"spec\":{\"selinuxOptions\":{\"allowedSystemProfiles\":[\"container\"]}}}")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patchRecover)
			newCheck("expect", asAdmin, withoutNamespace, contain, "[\"container\"]", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.selinuxOptions.allowedSystemProfiles}"}).check(oc)
			checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		}()
		//patch  oc -n openshift-security-profiles patch spod spod --type=merge -p '{"spec":{"selinuxOptions":{"allowedSystemProfiles":["container","net_container"]}}}'
		patch := fmt.Sprintf("{\"spec\":{\"selinuxOptions\":{\"allowedSystemProfiles\":[\"container\",\"net_container\"]}}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)

		g.By("Recreate the selinux profile !!!\n")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("selinuxprofile", selinuxProfileName, "-n", ns, "--ignore-not-found").Execute()
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"selinuxprofile", "-n", ns}).check(oc)
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxProfileCustomTemplate, "-p", "NAME="+selinuxProfileName, "NAMESPACE="+ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxProfileName, "-o=jsonpath={.status.status}", "-n", ns)
		usage := selinuxProfileName + "_" + ns + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usage, ok, []string{"selinuxprofiles", selinuxProfileName, "-n",
			ns, "-o=jsonpath={.status.usage}"}).check(oc)
		fileName := selinuxProfileName + "_" + ns + ".cil"
		assertKeywordsExistsInSelinuxFile(oc, "blockinherit net_container", "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "cat", "/etc/selinux.d/"+fileName)
		assertKeywordsExistsInSelinuxFile(oc, selinuxProfileName+"_"+ns, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "semodule", "-l")
	})

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-Medium-61579-Set a custom priority class name for spod daemon pod [Serial]", func() {
		var (
			priorityClassTemplate = filepath.Join(buildPruningBaseDir, "priorityclass.yaml")
			prioritym             = priorityClassDescription{
				name:         "my-priority-class" + getRandomString(),
				namespace:    subD.namespace,
				prirotyValue: 99,
				template:     priorityClassTemplate,
			}
		)

		g.By("Check the default piorityClassName.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "system-node-critical", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.priorityClassName}"}).check(oc)
		assertParameterValueForBulkPods(oc, "system-node-critical", "pod", "-l", "name=spod", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, strconv.Itoa(2000001000), "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")

		g.By("Create priorityclass and patch spod!!!\n")
		patchRecover := fmt.Sprintf("{\"spec\":{\"priorityClassName\":\"system-node-critical\"}}")
		defer func() {
			g.By("Recover the default config of priorityclass.. !!!\n")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patchRecover)
			cleanupObjects(oc, objectTableRef{"priorityclass", prioritym.name, subD.namespace})
			newCheck("expect", asAdmin, withoutNamespace, contain, "system-node-critical", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.priorityClassName}"}).check(oc)
			checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		}()
		prioritym.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, prioritym.name, ok, []string{"priorityclass", "-n", subD.namespace,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		patch := fmt.Sprintf("{\"spec\":{\"priorityClassName\":\"") + prioritym.name + fmt.Sprintf("\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)

		g.By("Check priorityclass in use!!!\n")
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		newCheck("expect", asAdmin, withoutNamespace, contain, prioritym.name, ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.priorityClassName}"}).check(oc)
		assertParameterValueForBulkPods(oc, prioritym.name, "pod", "-l", "name=spod", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, strconv.Itoa(prioritym.prirotyValue), "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")
	})

	// author: xiyuan@redhat.com
	g.It("ConnectedOnly-NonPreRelease-Author:xiyuan-Medium-61580-Set a non-exist priority class name for spod daemon pod [Serial]", func() {
		var priorityClassNotExist = "priority-not-exist-" + getRandomString()

		g.By("Check the default piorityClassName.. !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, "system-node-critical", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.priorityClassName}"}).check(oc)
		assertParameterValueForBulkPods(oc, "system-node-critical", "pod", "-l", "name=spod", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priorityClassName}")
		assertParameterValueForBulkPods(oc, strconv.Itoa(2000001000), "pod", "-l", "workload=scanner", "-n", subD.namespace, "-o=jsonpath={.items[*].spec.priority}")

		g.By("Patch a not exist priorityclass to spod!!!\n")
		defer func() {
			g.By("Recover the default config of priorityclass.. !!!\n")
			patchRecover := fmt.Sprintf("{\"spec\":{\"priorityClassName\":\"system-node-critical\"}}")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patchRecover)
			newCheck("expect", asAdmin, withoutNamespace, contain, "system-node-critical", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.priorityClassName}"}).check(oc)
			checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		}()
		patch := fmt.Sprintf("{\"spec\":{\"priorityClassName\":\"") + priorityClassNotExist + fmt.Sprintf("\"}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)

		g.By("Check and priorityclass in use and event !!!\n")
		newCheck("expect", asAdmin, withoutNamespace, contain, priorityClassNotExist, ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.priorityClassName}"}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"pod", "-l", "name=spod", "-n", subD.namespace}).check(oc)
		message := "no PriorityClass with name " + priorityClassNotExist + " was found"
		assertEventMessageRegexpMatch(oc, message, "event", "-n", subD.namespace, "--field-selector", "involvedObject.name=spod,reason=FailedCreate", "-o=jsonpath={.items[*].message}")
	})

	// author: bgudi@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ConnectedOnly-NonPreRelease-Author:bgudi-Medium-50222-check Log enricher based seccompprofile recording working as expected for job [Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		var (
			profileRecordingSeccom = profileRecordingDescription{
				name:       "spo-recording1",
				namespace:  ns1,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "hello-openshift",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingSeccom = saRoleRoleBindingDescription{
				saName:          "spo-record-sa1",
				namespace:       ns1,
				roleName:        "spo-record1" + getRandomString(),
				roleBindingName: "spo-record1" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			jobHello = workloadDescription{
				name:         "hello-job",
				namespace:    ns1,
				workloadKind: "Job",
				saName:       saRoleRoleBindingSeccom.saName,
				labelKey:     profileRecordingSeccom.labelKey,
				labelValue:   profileRecordingSeccom.labelValue,
				template:     workloadJobTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"ns", ns1, ns1})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilerecording", ns1, profileRecordingSeccom.name})
		profileRecordingSeccom.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingSeccom.name, "-n", ns1}).check(oc)

		g.By("Create sa, role, rolebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"sa", ns1, saRoleRoleBindingSeccom.saName},
			objectTableRef{"role", ns1, saRoleRoleBindingSeccom.roleName},
			objectTableRef{"rolebinding", ns1, saRoleRoleBindingSeccom.roleBindingName})
		saRoleRoleBindingSeccom.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingSeccom.saName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingSeccom.roleName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingSeccom.roleBindingName, "-n", ns1}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"job", ns1, jobHello.name})
		jobHello.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, jobHello.name, ok, []string{"job", jobHello.name, "-n", ns1, "-o=jsonpath={.metadata.name}"}).check(oc)

		g.By("Check for pod is ready")
		podName, _ := oc.AsAdmin().Run("get").Args("pods", "-n", ns1, "-o=jsonpath={.items..metadata.name}").Output()
		exutil.AssertPodToBeReady(oc, podName, ns1)

		g.By("Check seccompprofile and selinuxprofile generated !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"seccompprofile", ns1, "--all"})
		//sleep 60s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(60 * time.Second)
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("job", jobHello.name, "-n", ns1, "--ignore-not-found").Execute()
		checkPrfolieNumbers(oc, "seccompprofile", ns1, 2)
		checkPrfolieStatus(oc, "seccompprofile", ns1, "Installed")
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

	// author: jkuriako@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ConnectedOnly-NonPreRelease-Author:jkuriako-Medium-50264-Medium-50155-check Log enricher based selinuxrecording/seccompprofiles working as expected for replicaset [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingRep = profileRecordingDescription{
				name:       "spo-recording1",
				namespace:  ns1,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "hello-replicaset",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingRep = saRoleRoleBindingDescription{
				saName:          "spo-record-sa1",
				namespace:       ns1,
				roleName:        "spo-record1" + getRandomString(),
				roleBindingName: "spo-record1" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			replicasetHelloSelinux = workloadDescription{
				name:         "hello-replicaset",
				namespace:    ns1,
				workloadKind: "ReplicaSet",
				replicas:     3,
				saName:       saRoleRoleBindingRep.saName,
				labelKey:     profileRecordingRep.labelKey,
				labelValue:   profileRecordingRep.labelValue,
				template:     workloadRepTemplate,
			}
			profileRecordingRepSec = profileRecordingDescription{
				name:       "spo-recording2",
				namespace:  ns2,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "hello-replicaset-sec",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingRepSec = saRoleRoleBindingDescription{
				saName:          "spo-record-sa2",
				namespace:       ns2,
				roleName:        "spo-record2" + getRandomString(),
				roleBindingName: "spo-record2" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			replicasetHelloSecc = workloadDescription{
				name:         "hello-replicaset-sec",
				namespace:    ns2,
				workloadKind: "ReplicaSet",
				replicas:     3,
				saName:       saRoleRoleBindingRepSec.saName,
				labelKey:     profileRecordingRepSec.labelKey,
				labelValue:   profileRecordingRepSec.labelValue,
				template:     workloadRepTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

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
			objectTableRef{"profilerecording", ns1, profileRecordingRep.name},
			objectTableRef{"profilerecording", ns2, profileRecordingRepSec.name})
		profileRecordingRep.create(oc)
		profileRecordingRepSec.create(oc)

		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingRep.name, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingRepSec.name, "-n", ns2}).check(oc)

		g.By("Create sa, role, rolebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"sa", ns1, saRoleRoleBindingRep.saName},
			objectTableRef{"sa", ns2, saRoleRoleBindingRepSec.saName},
			objectTableRef{"role", ns1, saRoleRoleBindingRep.roleName},
			objectTableRef{"role", ns2, saRoleRoleBindingRepSec.roleName},
			objectTableRef{"rolebinding", ns1, saRoleRoleBindingRep.roleBindingName},
			objectTableRef{"rolebinding", ns2, saRoleRoleBindingRepSec.roleBindingName})
		saRoleRoleBindingRep.create(oc)
		saRoleRoleBindingRepSec.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingRep.saName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingRepSec.saName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingRep.roleName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingRepSec.roleName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingRep.roleBindingName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingRepSec.roleBindingName, "-n", ns2}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"replicaset", ns1, replicasetHelloSelinux.name},
			objectTableRef{"replicaset", ns2, replicasetHelloSecc.name})
		replicasetHelloSelinux.create(oc)
		replicasetHelloSecc.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"replicaset", replicasetHelloSelinux.name, "-n", ns1, "-o=jsonpath={.status.replicas}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"replicaset", replicasetHelloSecc.name, "-n", ns2, "-o=jsonpath={.status.replicas}"}).check(oc)

		g.By("Check SelinuxProfile and SeccompProfile is generated !!!")
		// Sleep 30 secondes so the log enricher will have enough time to catch necessary logs.
		time.Sleep(30 * time.Second)
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SelinuxProfile", ns1, "--all"})
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SeccompProfile", ns2, "--all"})
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("replicaset", replicasetHelloSelinux.name, "-n", ns1, "--ignore-not-found").Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("replicaset", replicasetHelloSecc.name, "-n", ns2, "--ignore-not-found").Execute()
		checkPrfolieNumbers(oc, "SelinuxProfile", ns1, replicasetHelloSelinux.replicas)
		checkPrfolieNumbers(oc, "SeccompProfile", ns2, replicasetHelloSecc.replicas)
		checkPrfolieStatus(oc, "SelinuxProfile", ns1, "Installed")
		checkPrfolieStatus(oc, "SeccompProfile", ns2, "Installed")
	})

	// author: bgudi@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ConnectedOnly-NonPreRelease-Author:bgudi-Medium-50259-Medium-50244-check Log enricher based selinuxprofile/seccompprofile recording and metrics working as expected for cronjob [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingSeccom = profileRecordingDescription{
				name:       "spo-recording1",
				namespace:  ns1,
				kind:       "SeccompProfile",
				labelKey:   "app",
				labelValue: "hello-openshift",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingSeccom = saRoleRoleBindingDescription{
				saName:          "spo-record-sa1",
				namespace:       ns1,
				roleName:        "spo-record1" + getRandomString(),
				roleBindingName: "spo-record1" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			cronjobSeccomHello = workloadDescription{
				name:         "hello-cronjob",
				namespace:    ns1,
				workloadKind: "CronJob",
				saName:       saRoleRoleBindingSeccom.saName,
				labelKey:     profileRecordingSeccom.labelKey,
				labelValue:   profileRecordingSeccom.labelValue,
				template:     workloadCronjobTemplate,
			}
			profileRecordingSelinux = profileRecordingDescription{
				name:       "spo-recording2",
				namespace:  ns2,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "hello-openshift",
				template:   profileRecordingTemplate,
			}
			saRoleRoleBindingSelinux = saRoleRoleBindingDescription{
				saName:          "spo-record-sa2",
				namespace:       ns2,
				roleName:        "spo-record2" + getRandomString(),
				roleBindingName: "spo-record2" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			cronjobSelinuxHello = workloadDescription{
				name:         "hello-cronjob",
				namespace:    ns2,
				workloadKind: "CronJob",
				saName:       saRoleRoleBindingSelinux.saName,
				labelKey:     profileRecordingSelinux.labelKey,
				labelValue:   profileRecordingSelinux.labelValue,
				template:     workloadCronjobTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

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
			objectTableRef{"profilerecording", ns1, profileRecordingSeccom.name},
			objectTableRef{"profilerecording", ns2, profileRecordingSelinux.name})
		profileRecordingSeccom.create(oc)
		profileRecordingSelinux.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingSeccom.name, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingSelinux.name, "-n", ns2}).check(oc)

		g.By("Create sa, role, rolebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"sa", ns1, saRoleRoleBindingSeccom.saName},
			objectTableRef{"role", ns1, saRoleRoleBindingSeccom.roleName},
			objectTableRef{"rolebinding", ns1, saRoleRoleBindingSeccom.roleBindingName},
			objectTableRef{"sa", ns2, saRoleRoleBindingSelinux.saName},
			objectTableRef{"role", ns2, saRoleRoleBindingSelinux.roleName},
			objectTableRef{"rolebinding", ns2, saRoleRoleBindingSelinux.roleBindingName})
		saRoleRoleBindingSeccom.create(oc)
		saRoleRoleBindingSelinux.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingSeccom.saName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingSeccom.roleName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingSeccom.roleBindingName, "-n", ns1}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingSelinux.saName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingSelinux.roleName, "-n", ns2}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingSelinux.roleBindingName, "-n", ns2}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"cronjob", ns1, cronjobSeccomHello.name},
			objectTableRef{"cronjob", ns2, cronjobSelinuxHello.name})
		cronjobSeccomHello.create(oc)
		cronjobSelinuxHello.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, cronjobSeccomHello.name, ok, []string{"cronjob", cronjobSeccomHello.name, "-n", ns1, "-o=jsonpath={.metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, cronjobSelinuxHello.name, ok, []string{"cronjob", cronjobSelinuxHello.name, "-n", ns2, "-o=jsonpath={.metadata.name}"}).check(oc)

		g.By("Get jobs.batch details")
		newCheck("expect", asAdmin, withoutNamespace, contain, cronjobSeccomHello.name, ok, []string{"jobs.batch", "-n", ns1}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, cronjobSelinuxHello.name, ok, []string{"jobs.batch", "-n", ns2}).check(oc)

		g.By("Get pods details")
		newCheck("expect", asAdmin, withoutNamespace, contain, cronjobSeccomHello.name, ok, []string{"pods", "-n", ns1}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, cronjobSelinuxHello.name, ok, []string{"pods", "-n", ns2}).check(oc)

		g.By("Check seccompprofile and selinuxprofile generated !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"seccompprofile", ns1, "--all"})
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"selinuxprofile", ns2, "--all"})
		//sleep 60s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(60 * time.Second)
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("cronjob", cronjobSeccomHello.name, "-n", ns1, "--ignore-not-found").Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("cronjob", cronjobSelinuxHello.name, "-n", ns2, "--ignore-not-found").Execute()
		checkPrfolieNumbers(oc, "seccompprofile", ns1, 2)
		checkPrfolieStatus(oc, "seccompprofile", ns1, "Installed")
		checkPrfolieNumbers(oc, "selinuxprofile", ns2, 2)
		checkPrfolieStatus(oc, "selinuxprofile", ns2, "Installed")
	})

	// author: jkuriako@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ConnectedOnly-NonPreRelease-Author:jkuriako-Critical-50254-check Log enricher based selinuxprofiles recording and metrics working as expected for pod [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		var (
			profileRecordingPod = profileRecordingDescription{
				name:       "test-recording",
				namespace:  ns1,
				kind:       "SelinuxProfile",
				labelKey:   "app",
				labelValue: "my-app",
				template:   profileRecordingTemplate,
			}

			podHello = workloadDescription{
				name:         "my-pod" + getRandomString(),
				namespace:    ns1,
				workloadKind: "Pod",
				labelKey:     profileRecordingPod.labelKey,
				labelValue:   profileRecordingPod.labelValue,
				image:        "quay.io/security-profiles-operator/redis:6.2.1",
				imageName:    "redis",
				template:     workloadPodTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"ns", ns1, ns1})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilerecording", ns1, profileRecordingPod.name})
		profileRecordingPod.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingPod.name, "-n", ns1}).check(oc)

		g.By("Create pod and check pod status !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", ns1, podHello.name})
		podHello.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, podHello.name, ok, []string{"pod", podHello.name, "-n", ns1, "-o=jsonpath={.metadata.name}"}).check(oc)
		exutil.AssertPodToBeReady(oc, podHello.name, ns1)

		g.By("Check SelinuxProfile generated !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SelinuxProfile", ns1, "--all"})
		//sleep 60s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(60 * time.Second)
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podHello.name, "-n", ns1, "--ignore-not-found").Execute()
		checkPrfolieStatus(oc, "SelinuxProfile", ns1, "Installed")
		checkPrfolieNumbers(oc, "SelinuxProfile", ns1, 1)
	})
})
