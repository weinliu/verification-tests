package securityandcompliance

import (
	"fmt"
	"net"
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

var _ = g.Describe("[sig-isc] Security_and_Compliance Security_Profiles_Operator The Security_Profiles_Operator", func() {
	defer g.GinkgoRecover()

	var (
		oc                                        = exutil.NewCLI("compliance-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir                       string
		errorLoggerSelEnforcingTemplate           string
		machineConfigPoolYAML                     string
		mcSeccompNostatTemplate                   string
		ogSpoTemplate                             string
		subSpoTemplate                            string
		spSleepWithMkdirTemplate                  string
		secProfileTemplate                        string
		secProfileStackTemplate                   string
		podWithProfileTemplate                    string
		selinuxProfileNginxTemplate               string
		podBusybox                                string
		podBusyboxSaTemplate                      string
		podWithSelinuxProfileTemplate             string
		errorLoggerPodWithSecuritycontextTemplate string
		profileRecordingTemplate                  string
		profileBindingWildcardTemplate            string
		profileBindingTemplate                    string
		saRoleRolebindingTemplate                 string
		sccSelinuxprofileTemplate                 string
		selinuxProfileCustomTemplate              string
		workloadDaeTemplate                       string
		workloadDeployTemplate                    string
		workloadDeployHelloTemplate               string
		pathWebhookAllowedSyscalls                string
		pathWebhookBinding                        string
		pathWebhookRecording                      string
		podLegacySeccompTemplate                  string
		podWithLabelsTemplate                     string
		podWithOneLabelTemplate                   string
		workloadRepTemplate                       string
		workloadCronjobTemplate                   string
		workloadPodTemplate                       string
		workloadJobTemplate                       string

		ogD                 operatorGroupDescription
		subD                subscriptionDescription
		seccompP, seccompP1 seccompProfile
	)

	g.BeforeEach(func() {
		buildPruningBaseDir = exutil.FixturePath("testdata", "securityandcompliance")
		errorLoggerSelEnforcingTemplate = filepath.Join(buildPruningBaseDir, "spo/selinux-profile-errorlogger-enforcing.yaml")
		errorLoggerPodWithSecuritycontextTemplate = filepath.Join(buildPruningBaseDir, "spo/pod-errorlogger-with-securityContext.yaml")
		machineConfigPoolYAML = filepath.Join(buildPruningBaseDir, "machineConfigPool.yaml")
		mcSeccompNostatTemplate = filepath.Join(buildPruningBaseDir, "spo/machineconfig-nostat.yaml")
		ogSpoTemplate = filepath.Join(buildPruningBaseDir, "operator-group-all-namespaces.yaml")
		subSpoTemplate = filepath.Join(buildPruningBaseDir, "subscription.yaml")
		spSleepWithMkdirTemplate = filepath.Join(buildPruningBaseDir, "/spo/seccomp-profile-sleep-with-mkdir.yaml")
		secProfileTemplate = filepath.Join(buildPruningBaseDir, "seccompprofile.yaml")
		secProfileStackTemplate = filepath.Join(buildPruningBaseDir, "seccompprofilestack.yaml")
		podWithProfileTemplate = filepath.Join(buildPruningBaseDir, "pod-with-seccompprofile.yaml")
		selinuxProfileNginxTemplate = filepath.Join(buildPruningBaseDir, "/spo/selinux-profile-nginx.yaml")
		selinuxProfileCustomTemplate = filepath.Join(buildPruningBaseDir, "/spo/selinux-profile-with-custom-policy-template.yaml")
		pathWebhookAllowedSyscalls = filepath.Join(buildPruningBaseDir, "/spo/patch-webhook-allowed-syscalls.yaml")
		pathWebhookBinding = filepath.Join(buildPruningBaseDir, "/spo/patch-webhook-binding.yaml")
		pathWebhookRecording = filepath.Join(buildPruningBaseDir, "/spo/patch-webhook-recording.yaml")
		podBusybox = filepath.Join(buildPruningBaseDir, "spo/pod-busybox.yaml")
		podBusyboxSaTemplate = filepath.Join(buildPruningBaseDir, "spo/pod-busybox-sa.yaml")
		podLegacySeccompTemplate = filepath.Join(buildPruningBaseDir, "/spo/pod-legacy-seccomp.yaml")
		podWithSelinuxProfileTemplate = filepath.Join(buildPruningBaseDir, "/spo/pod-with-selinux-profile.yaml")
		podWithLabelsTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-pod-with-labels.yaml")
		podWithOneLabelTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-pod-with-one-label.yaml")
		profileRecordingTemplate = filepath.Join(buildPruningBaseDir, "/spo/profile-recording.yaml")
		profileBindingTemplate = filepath.Join(buildPruningBaseDir, "/spo/profile-binding.yaml")
		profileBindingWildcardTemplate = filepath.Join(buildPruningBaseDir, "/spo/profile-binding-wildcard.yaml")
		saRoleRolebindingTemplate = filepath.Join(buildPruningBaseDir, "/spo/sa-previleged-role-rolebinding.yaml")
		sccSelinuxprofileTemplate = filepath.Join(buildPruningBaseDir, "/spo/scc-selinuxprofile.yaml")
		workloadDaeTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-daemonset.yaml")
		workloadDeployTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-deployment.yaml")
		workloadDeployHelloTemplate = filepath.Join(buildPruningBaseDir, "/spo/workload-deployment-hello-openshift.yaml")
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

		exutil.SkipNoOLMCore(oc)
		subD.skipMissingCatalogsources(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64, architecture.MULTI)
		architecture.SkipNonAmd64SingleArch(oc)
		SkipClustersWithRhelNodes(oc)

		createSecurityProfileOperator(oc, subD, ogD)
	})

	// author: xiyuan@redhat.com
	g.It("ROSA-ARO-OSD_CCS-Author:xiyuan-High-50078-Create two seccompprofiles with the same name in different namespaces", func() {
		seccompP = seccompProfile{
			name:      "sleep-sh-pod",
			namespace: "spo-ns-1-" + getRandomString(),
			template:  secProfileTemplate,
		}
		seccompP1 = seccompProfile{
			name:      "sleep-sh-pod",
			namespace: "spo-ns-2-" + getRandomString(),
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
	g.It("ROSA-ARO-OSD_CCS-StagerunBoth-ConnectedOnly-Author:xiyuan-High-49885-Check SeccompProfile stack working as expected", func() {
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
		assertKeywordsExistsInFile(oc, seccompP.namespace, "mkdir", filePath, false)

		g.By("Create seccompprofile stack !!!")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("seccompprofiles", seccompProfilestack.name, "-n", seccompProfilestack.namespace, "--ignore-not-found").Execute()
		seccompProfilestack.create(oc)

		g.By("Check base seccompprofile was installed correctly !!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", "--selector=spo.x-k8s.io/profile-id=SeccompProfile-" + seccompProfilestack.name, "-n", seccompProfilestack.namespace, "-o=jsonpath={.status.status}"})
		dir2 := "/var/lib/kubelet/seccomp/operator/" + seccompProfilestack.namespace + "/"
		secProfileStackName := seccompProfilestack.name + ".json"
		stackFilePath := dir2 + secProfileStackName
		assertKeywordsExistsInFile(oc, seccompProfilestack.namespace, "mkdir", stackFilePath, true)

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
	g.It("Author:xiyuan-LEVEL0-ROSA-ARO-OSD_CCS-ConnectedOnly-StagerunBoth-High-56704-Create a SelinuxProfile and apply it to pod", func() {
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
	g.It("Author:xiyuan-ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Medium-50242-High-50174-check Log enricher based seccompprofile recording and metrics working as expected for daemonset/deployment [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingDae = profileRecordingDescription{
				name:          "spo-recording1",
				namespace:     ns1,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-daemonset",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording2",
				namespace:     ns2,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-openshift",
				template:      profileRecordingTemplate,
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
	g.It("Author:xiyuan-ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Medium-50262-High-50263-check Log enricher based selinuxprofile recording and metrics working as expected for daemonset/deployment [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingDae = profileRecordingDescription{
				name:          "spo-recording1",
				namespace:     ns1,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-daemonset",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording2",
				namespace:     ns2,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-openshift",
				template:      profileRecordingTemplate,
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-High-61609-High-61599-check enable memory optimization in spod could work for seccompprofiles/selinuxprofiles recording for pod [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingSec = profileRecordingDescription{
				name:          "spo-recording-sec-" + getRandomString(),
				namespace:     ns1,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording-sel-" + getRandomString(),
				namespace:     ns2,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-High-51391-Log based selinuxprofile recording could make use of webhookOptions object in webhooks [Slow][Disruptive]", func() {
		ns1 := "do-record-" + getRandomString()
		ns2 := "dont-record1-" + getRandomString()
		ns3 := "dont-record2-" + getRandomString()

		var (
			profileRecordingNs1 = profileRecordingDescription{
				name:          "spo-recording-sel-" + getRandomString(),
				namespace:     ns1,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording-sel-" + getRandomString(),
				namespace:     ns2,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording-sel-" + getRandomString(),
				namespace:     ns3,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-test",
				template:      profileRecordingTemplate,
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
			objectTableRef{"profilerecording", ns1, profileRecordingNs1.name},
			objectTableRef{"profilerecording", ns2, profileRecordingNs2.name},
			objectTableRef{"profilerecording", ns3, profileRecordingNs3.name})
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-High-51392-Log based secompprofile recording could make use of webhookOptions object in webhooks [Slow][Disruptive]", func() {
		ns1 := "do-record-" + getRandomString()
		ns2 := "dont-record1-" + getRandomString()
		ns3 := "dont-record2-" + getRandomString()

		var (
			profileRecordingNs1 = profileRecordingDescription{
				name:          "spo-recording-sec-" + getRandomString(),
				namespace:     ns1,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording-sec-" + getRandomString(),
				namespace:     ns2,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording-sec-" + getRandomString(),
				namespace:     ns3,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-test",
				template:      profileRecordingTemplate,
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
			objectTableRef{"profilerecording", ns1, profileRecordingNs1.name},
			objectTableRef{"profilerecording", ns2, profileRecordingNs2.name},
			objectTableRef{"profilerecording", ns3, profileRecordingNs3.name})
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-High-51405-Check profilebinding could make use of webhookOptions object in webhooks for seccompprofile [Disruptive]", func() {
		ns1 := "do-binding-" + getRandomString()
		ns2 := "dont-binding-" + getRandomString()
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
			objectTableRef{"seccompprofile", ns1, seccompNs1.name},
			objectTableRef{"seccompprofile", ns2, seccompNs1.name})
		seccompNs1.create(oc)
		seccompNs2.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccompNs1.name, "-n", ns1, "-o=jsonpath={.status.status}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccompNs2.name, "-n", ns2, "-o=jsonpath={.status.status}"})

		g.By("Create profilebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilebinding", ns1, profileBindingNs1.name},
			objectTableRef{"profilebinding", ns2, profileBindingNs2.name})
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
	g.It("Author:xiyuan-ROSA-ARO-OSD_CCS-ConnectedOnly-High-75255-Check profilebinding could use wildcard attribute to bind all new pods with a default seccompprofile in a given namespace", func() {
		ns := "test-spo-" + getRandomString()
		pod1 := "test-pod-1-" + getRandomString()
		pod2 := "test-pod-2-" + getRandomString()

		var (
			seccomp = seccompProfile{
				name:      "sleep-sh-pod-" + getRandomString(),
				namespace: ns,
				template:  secProfileTemplate,
			}
			profileBinding = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   ns,
				kind:        "SeccompProfile",
				profilename: seccomp.name,
				image:       "quay.io/openshifttest/busybox:latest",
				template:    profileBindingWildcardTemplate,
			}
		)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"ns", ns, ns})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns, "-n", ns, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns)

		g.By("Create seccompprofiles and check status !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"seccompprofile", ns, seccomp.name})
		seccomp.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccomp.name, "-n", ns, "-o=jsonpath={.status.status}"})

		g.By("Created a pod!!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", ns, pod1})
		errPod1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podBusybox, "-p", "NAME="+pod1, "NAMESPACE="+ns)
		o.Expect(errPod1).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", pod1, "-n", ns, "-o=jsonpath={.status.phase}"})

		g.By("Create profilebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilebinding", ns, profileBinding.name})
		profileBinding.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBinding.name, ok, []string{"profilebinding", "-n", ns,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", ns, pod2})
		errPod2 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podBusybox, "-p", "NAME="+pod2, "NAMESPACE="+ns)
		o.Expect(errPod2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", pod2, "-n", ns, "-o=jsonpath={.status.phase}"})

		g.By("Check the securityContext and whether mkdir operator allowed for pods !!!")
		localhostProfile := "operator/" + ns + "/" + seccomp.name + ".json"
		newCheck("expect", asAdmin, withoutNamespace, compare, localhostProfile, ok, []string{"pod", pod1, "-n", ns, "-o=jsonpath={.spec.securityContext}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "operator/"+ns+"/"+seccomp.name+".json", ok, []string{"pod", pod2, "-n", ns, "-o=jsonpath={.spec.securityContext.seccompProfile}"})
		result1, _ := oc.AsAdmin().Run("exec").Args(pod1, "-n", ns, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result1, "/tmpo/foo") && !strings.Contains(result1, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result1)
		}
		result2, _ := oc.AsAdmin().Run("exec").Args(pod2, "-n", ns, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result2, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result2)
		}
	})

	// author: xiyuan@redhat.com
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-High-50077-Check profilebinding working as expected for selinuxprofile", func() {
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
		)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"ns", ns1, ns1},
			objectTableRef{"ns", ns2, ns2})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns1, "-n", ns1, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns1)

		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns2, "-n", ns2, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns2)

		g.By("Created a pods with  !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", ns1, podErrorloggerNs1})
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodTemplate, "-p", "NAME="+podErrorloggerNs1, "NAMESPACE="+ns1)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, podErrorloggerNs1, ns1)
		newCheck("expect", asAdmin, withoutNamespace, contain, `{"capabilities":{"drop":["MKNOD"]}}`, ok, []string{"pod", podErrorloggerNs1, "-n", ns1,
			"-o=jsonpath={.spec.initContainers[0].securityContext}"}).check(oc)
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+podErrorloggerNs1, "-c", "errorlogger", "-n", ns1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, `/var/log/test.log: Permission denied`)).To(o.BeTrue())

		g.By("Create selinuxprofile and check status !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"selinuxprofile", ns1, selinuxNs1.name})
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxNs1.template, "-p", "NAME="+selinuxNs1.name, "NAMESPACE="+selinuxNs1.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxNs1.name, "-o=jsonpath={.status.status}", "-n", ns1)
		usageNs1 := selinuxNs1.name + "_" + selinuxNs1.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usageNs1, ok, []string{"selinuxprofiles", selinuxNs1.name, "-n", selinuxNs1.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)

		g.By("Create profilebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilebinding", ns1, profileBindingNs1.name})
		profileBindingNs1.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingNs1.name, ok, []string{"profilebinding", "-n", ns1,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns1, podErrorloggerNs1},
			objectTableRef{"pod", ns2, podErrorloggerNs2})
		cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", ns1, podErrorloggerNs1})
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
		o.Expect(strings.Contains(result2, "Permission denied")).To(o.BeTrue())
	})

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-ROSA-ARO-OSD_CCS-ConnectedOnly-High-75305-Check profilebinding working as expected for selinuxprofile", func() {
		ns := "do-binding" + getRandomString()
		errorLoggerSelTemplate := filepath.Join(buildPruningBaseDir, "spo/selinux-profile-errorlogger.yaml")
		errorLoggerPodTemplate := filepath.Join(buildPruningBaseDir, "spo/pod-errorlogger.yaml")
		podBefore := "pod-before-" + getRandomString()
		podAfter := "pod-after-" + getRandomString()
		saName := "test-profilebinding-" + getRandomString()
		sccName := "test-profilebinding-" + getRandomString()

		var (
			selinuxprofile = selinuxProfile{
				name:      "error-logger-" + getRandomString(),
				namespace: ns,
				template:  errorLoggerSelTemplate,
			}
			profilebinding = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   ns,
				kind:        "SelinuxProfile",
				profilename: selinuxprofile.name,
				template:    profileBindingWildcardTemplate,
			}
		)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"ns", ns, ns})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns, "-n", ns, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns)

		g.By("Created a pod !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", ns, podBefore})
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodTemplate, "-p", "NAME="+podBefore, "NAMESPACE="+ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, podBefore, ns)
		newCheck("expect", asAdmin, withoutNamespace, contain, "MKNOD", ok, []string{"pod", podBefore, "-n", ns,
			"-o=jsonpath={.spec.initContainers[0].securityContext.capabilities.drop}"}).check(oc)
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+podBefore, "-c", "errorlogger", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, `/var/log/test.log: Permission denied`)).To(o.BeTrue())

		g.By("Create selinuxprofile and check status !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"selinuxprofile", ns, selinuxprofile.name})
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxprofile.template, "-p", "NAME="+selinuxprofile.name, "NAMESPACE="+selinuxprofile.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxprofile.name, "-o=jsonpath={.status.status}", "-n", ns)
		usageNs1 := selinuxprofile.name + "_" + selinuxprofile.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usageNs1, ok, []string{"selinuxprofiles", selinuxprofile.name, "-n", selinuxprofile.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)

		g.By("Create profilebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilebinding", ns, profilebinding.name})
		profilebinding.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profilebinding.name, ok, []string{"profilebinding", "-n", ns,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"sa", ns, saName})
		_, errCreate := oc.AsAdmin().WithoutNamespace().Run("create").Args("sa", saName, "-n", ns).Output()
		o.Expect(errCreate).NotTo(o.HaveOccurred())
		//sccSelinuxprofileTemplate
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"scc", ns, sccName})
		user := "system:serviceaccount:" + ns + ":" + saName
		errApply := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sccSelinuxprofileTemplate, "-n", ns, "-p", "NAME="+sccName, "USER="+user)
		o.Expect(errApply).NotTo(o.HaveOccurred())
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"pod", ns, podAfter})
		//podBusyboxSaTemplate
		errPodAfter := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podBusyboxSaTemplate, "-p", "NAME="+podAfter, "NAMESPACE="+ns, "SERVICEACCOUNTNAME="+saName)
		o.Expect(errPodAfter).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, podAfter, ns)

		g.By("Check whether the profilebinding take effect !!!")
		seLinuxOptionsType := selinuxprofile.name + "_" + selinuxprofile.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, seLinuxOptionsType, nok, []string{"pod", podBefore, "-n", ns, "-o=jsonpath={.spec.securityContext}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, seLinuxOptionsType, ok, []string{"pod", podAfter, "-n", ns, "-o=jsonpath={.spec.securityContext.seLinuxOptions.type}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, ns+"/"+podAfter, ok, []string{"profilebinding", profilebinding.name, "-n", ns, "-o=jsonpath={.status.activeWorkloads}"}).check(oc)
	})

	// author: xiyuan@redhat.com
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Author:xiyuan-High-51408-Check profilebinding could make use of webhookOptions object in webhooks for selinuxprofile [Disruptive]", func() {
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
			objectTableRef{"selinuxprofile", ns1, selinuxNs1.name},
			objectTableRef{"selinuxprofile", ns2, selinuxNs2.name})
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
			objectTableRef{"profilebinding", ns1, profileBindingNs1.name},
			objectTableRef{"profilebinding", ns2, profileBindingNs2.name})
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-High-56130-Check the permissive boolean works for a selinuxprofile [Slow]", func() {
		ns := "permissive-test-" + getRandomString()
		podEnforing := "pod-errorlogger-enforcing"
		podPermissive := "pod-errorlogger-permissive"
		var (
			selinuxProfileEnforcing = selinuxProfile{
				name:       "error-logger-enforcing",
				namespace:  ns,
				permissive: false,
				template:   errorLoggerSelEnforcingTemplate,
			}
			selinuxProfilePermissive = selinuxProfile{
				name:       "error-logger-permissive",
				namespace:  ns,
				permissive: true,
				template:   errorLoggerSelEnforcingTemplate,
			}
		)

		g.By("Create namespace !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"ns", ns, ns})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns, "-n", ns, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		exutil.SetNamespacePrivileged(oc, ns)

		g.By("Create selinuxprofile and check status !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"selinuxprofile", ns, selinuxProfileEnforcing.name},
			objectTableRef{"selinuxprofile", ns, selinuxProfilePermissive.name})
		selinuxProfileEnforcing.create(oc)
		selinuxProfilePermissive.create(oc)
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxProfileEnforcing.name, "-o=jsonpath={.status.status}", "-n", ns)
		usageEnforcing := selinuxProfileEnforcing.name + "_" + selinuxProfileEnforcing.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usageEnforcing, ok, []string{"selinuxprofiles", selinuxProfileEnforcing.name, "-n", selinuxProfileEnforcing.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxProfilePermissive.name, "-o=jsonpath={.status.status}", "-n", ns)
		ususagePermissive := selinuxProfilePermissive.name + "_" + selinuxProfilePermissive.namespace + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, ususagePermissive, ok, []string{"selinuxprofiles", selinuxProfilePermissive.name, "-n", selinuxProfilePermissive.namespace,
			"-o=jsonpath={.status.usage}"}).check(oc)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns, podEnforing},
			objectTableRef{"pod", ns, podPermissive})
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodWithSecuritycontextTemplate, "-p", "NAME="+podEnforing, "NAMESPACE="+ns, "TYPE="+usageEnforcing)
		o.Expect(err1).NotTo(o.HaveOccurred())
		err2 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", errorLoggerPodWithSecuritycontextTemplate, "-p", "NAME="+podPermissive, "NAMESPACE="+ns, "TYPE="+ususagePermissive)
		o.Expect(err2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "Error", ok, []string{"pod", podEnforing, "-n", ns,
			"-o=jsonpath={.status.containerStatuses[?(@.name==\"errorlogger\")].state.terminated.reason}"}).check(oc)
		exutil.AssertPodToBeReady(oc, podPermissive, ns)

		g.By("Check whether the profilebinding take effect !!!")
		result1, _ := oc.AsAdmin().Run("logs").Args("pod/"+podEnforing, "-n", ns, "-c", "errorlogger").Output()
		o.Expect(strings.Contains(result1, "Permission denied")).To(o.BeTrue())
		result2, _ := oc.AsAdmin().Run("logs").Args("pod/"+podPermissive, "-n", ns, "-c", "errorlogger").Output()
		o.Expect(strings.Contains(result2, "Permission denied")).To(o.BeFalse())
	})

	// author: xiyuan@redhat.com
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-Medium-61581-Verify a custom container-selinux policy templates could be used [Serial]", func() {
		ns := "net-container-policy-" + getRandomString()
		selinuxProfileName := "net-container-policy"

		g.By("Create selinuxprofile !!!")
		defer deleteNamespace(oc, ns)
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("selinuxprofile", selinuxProfileName, "-n", ns, "--ignore-not-found").Execute()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxProfileCustomTemplate, "-p", "NAME="+selinuxProfileName, "NAMESPACE="+ns, "INHERITKIND="+"System",
			"INHERITNAME="+"net_container")
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
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxProfileCustomTemplate, "-p", "NAME="+selinuxProfileName, "NAMESPACE="+ns, "INHERITKIND="+"System",
			"INHERITNAME="+"net_container")
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Author:xiyuan-High-62986-Verify selinuxprofiles could inherit the custom selinuxprofiles from the same namespace", func() {
		ns := "nginx-deploy" + getRandomString()
		selinuxProfileName := "nginx-secure"
		selinuxProfileInheritName := "nginx-secure-inherit"

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

		g.By("Create another selinuxprofile based on the first selinuxprofile !!!\n")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("selinuxprofile", selinuxProfileInheritName, "-n", ns, "--ignore-not-found").Execute()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", selinuxProfileCustomTemplate, "-p", "NAME="+selinuxProfileInheritName, "NAMESPACE="+ns, "INHERITKIND=SelinuxProfile",
			"INHERITNAME="+selinuxProfileName)
		o.Expect(err).NotTo(o.HaveOccurred())
		assertKeywordsExists(oc, 300, "Installed", "selinuxprofiles", selinuxProfileInheritName, "-o=jsonpath={.status.status}", "-n", ns)
		usageInherit := selinuxProfileInheritName + "_" + ns + ".process"
		newCheck("expect", asAdmin, withoutNamespace, contain, usageInherit, ok, []string{"selinuxprofiles", selinuxProfileInheritName, "-n",
			ns, "-o=jsonpath={.status.usage}"}).check(oc)
		fileNameInherit := selinuxProfileInheritName + "_" + ns + ".cil"
		assertKeywordsExistsInSelinuxFile(oc, "sock_file", "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "cat", "/etc/selinux.d/"+fileNameInherit)
		assertKeywordsExistsInSelinuxFile(oc, selinuxProfileInheritName+"_"+ns, "-n", subD.namespace, "-c", "selinuxd", "ds/spod", "semodule", "-l")
	})

	// author: xiyuan@redhat.com
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-Medium-61579-Set a custom priority class name for spod daemon pod [Serial]", func() {
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:xiyuan-Medium-61580-Set a non-exist priority class name for spod daemon pod [Serial]", func() {
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
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:bgudi-Medium-50222-check Log enricher based seccompprofile recording working as expected for job [Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		var (
			profileRecordingSeccom = profileRecordingDescription{
				name:          "spo-recording1",
				namespace:     ns1,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-openshift",
				template:      profileRecordingTemplate,
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
	g.It("Author:minmli-ROSA-ARO-OSD_CCS-High-50397-check security profiles operator could be deleted successfully [Serial]", func() {
		ns := "test-50397-" + getRandomString()
		defer func() {
			g.By("delete Security Profile Operator !!!")
			cleanupObjectsIgnoreNotFound(oc,
				objectTableRef{"seccompprofiles", subD.namespace, "--all"},
				objectTableRef{"selinuxprofiles", subD.namespace, "--all"},
				objectTableRef{"rawselinuxprofiles", subD.namespace, "--all"},
				objectTableRef{"mutatingwebhookconfiguration", subD.namespace, "spo-mutating-webhook-configuration"})
			deleteNamespace(oc, subD.namespace)
			g.By("the Security Profile Operator is deleted successfully !!!")
		}()

		seccompP = seccompProfile{
			name:      "sleep-sh-pod",
			namespace: ns,
			template:  secProfileTemplate,
		}

		g.By("Create a SeccompProfile in spo namespace !!!")
		defer deleteNamespace(oc, seccompP.namespace)
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompP.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("seccompprofiles", "--all", "-n", seccompP.namespace, "--ignore-not-found", "--timeout=20s").Execute()
		seccompP.create(oc)

		g.By("Check the SeccompProfile is created sucessfully !!!")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", "--selector=spo.x-k8s.io/profile-id=SeccompProfile-sleep-sh-pod", "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
	})

	// author: jkuriako@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Author:jkuriako-Medium-50264-Medium-50155-check Log enricher based selinuxrecording/seccompprofiles working as expected for replicaset [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingRep = profileRecordingDescription{
				name:          "spo-recording1",
				namespace:     ns1,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-replicaset",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording2",
				namespace:     ns2,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-replicaset-sec",
				template:      profileRecordingTemplate,
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

	// author: xiyuan@redhat.com
	g.It("Author:xiyuan-ConnectedOnly-Medium-69598-check the http version for the metrics, webhook and profilefing service endpoints [Serial]", func() {
		g.By("Check http version for metric serive")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		url := fmt.Sprintf("https://metrics.%v/metrics-spod", subD.namespace)
		keywords := `HTTP/1.1 200 OK`
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-i", "-ks", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), keywords)).To(o.BeTrue())

		g.By("Check http version for profilebinding webhook-service")
		url = fmt.Sprintf("https://webhook-service.%v.svc/mutate-v1-pod-binding", subD.namespace)
		output, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-i", "-ks", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring(keywords))

		g.By("Check http version for profilerecording webhook-service")
		url = fmt.Sprintf("https://webhook-service.%v.svc/mutate-v1-pod-recording", subD.namespace)
		output, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-i", "-ks", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), keywords)).To(o.BeTrue())

		g.By("check the http version for profiling endpoints")
		patch := fmt.Sprintf("{\"spec\":{\"enableProfiling\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		podName, err := oc.AsAdmin().Run("get").Args("pods", "-n", subD.namespace, "-l", "name=spod", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podIP, err := oc.AsAdmin().Run("get").Args("pod", "-n", subD.namespace, podName, "-o=jsonpath={.status.podIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podPort, err := oc.AsAdmin().Run("get").Args("pod", "-n", subD.namespace, podName, "-o=jsonpath={.spec.containers[?(@.name==\"security-profiles-operator\")].env[?(@.name==\"SPO_PROFILING_PORT\")].value}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ip := net.ParseIP(podIP)
		if ip.To4() == nil {
			podIP = "[" + podIP + "]"
		}
		url = fmt.Sprintf("http://%v:%v/debug/pprof/heap", podIP, podPort)
		output, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-i", "-ks", "-H", fmt.Sprintf("Authorization: Bearer %v", token), url).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), keywords)).To(o.BeTrue())
	})

	// author: bgudi@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("Author:bgudi-ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Medium-50259-Medium-50244-check Log enricher based selinuxprofile/seccompprofile recording and metrics working as expected for cronjob [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		ns2 := "mytest" + getRandomString()
		var (
			profileRecordingSeccom = profileRecordingDescription{
				name:          "spo-recording1",
				namespace:     ns1,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-openshift",
				template:      profileRecordingTemplate,
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
				name:          "spo-recording2",
				namespace:     ns2,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "hello-openshift",
				template:      profileRecordingTemplate,
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
	g.It("Author:jkuriako-ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Longduration-Critical-50254-check Log enricher based selinuxprofiles recording and metrics working as expected for pod [Slow][Disruptive]", func() {
		ns1 := "mytest" + getRandomString()
		var (
			profileRecordingPod = profileRecordingDescription{
				name:          "test-recording",
				namespace:     ns1,
				kind:          "SelinuxProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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

	// author: xiyuan@redhat.com
	g.It("ROSA-ARO-OSD_CCS-NonPreRelease-Longduration-Author:xiyuan-Medium-61577-Customise the spo daemon resource requirements [Serial]", func() {
		g.By("Check the default resource requirements for spod !!!\n")
		assertEventMessageRegexpMatch(oc, "ephemeral-storage.*200Mi.*memory.*128Mi", "pod", "-l", "name=spod", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="security-profiles-operator")].resources.limits}`)
		assertEventMessageRegexpMatch(oc, "cpu.*100m.*ephemeral-storage.*50Mi.*memory.*64Mi", "pod", "-l", "name=spod", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="security-profiles-operator")].resources.requests}`)

		g.By("Patch the daemonResourceRequirements !!!\n")
		defer func() {
			g.By("Recover the default daemonResourceRequirements.. !!!\n")
			patchRecover := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/daemonResourceRequirements\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "--type", "json", "--patch", patchRecover, "-n", subD.namespace)
			checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
			assertEventMessageRegexpMatch(oc, "ephemeral-storage.*200Mi.*memory.*128Mi", "pod", "-l", "name=spod", "-n", subD.namespace,
				`-o=jsonpath={.items[*].spec.containers[?(@.name=="security-profiles-operator")].resources.limits}`)
			assertEventMessageRegexpMatch(oc, "cpu.*100m.*ephemeral-storage.*50Mi.*memory.*64Mi", "pod", "-l", "name=spod", "-n", subD.namespace,
				`-o=jsonpath={.items[*].spec.containers[?(@.name=="security-profiles-operator")].resources.requests}`)
		}()
		patchDaeResourceReq := fmt.Sprintf("{\"spec\":{\"daemonResourceRequirements\":{\"requests\":{\"memory\":\"256Mi\",\"cpu\":\"250m\"},\"limits\":{\"memory\":\"512Mi\",\"cpu\":\"500m\"}}}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "--type", "merge", "-p", patchDaeResourceReq, "-n", subD.namespace)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		assertEventMessageRegexpMatch(oc, "cpu.*500m.*memory.*512Mi", "pod", "-l", "name=spod", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="security-profiles-operator")].resources.limits}`)
		assertEventMessageRegexpMatch(oc, "cpu.*250m.*memory.*256Mi", "pod", "-l", "name=spod", "-n", subD.namespace,
			`-o=jsonpath={.items[*].spec.containers[?(@.name=="security-profiles-operator")].resources.requests}`)
	})

	// author: xiyuan@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Author:xiyuan-High-56018-Verify the mergeStrategy wroks for log enricher based selinuxprofile recording for deployment [Slow][Disruptive]", func() {
		ns := "merge-strategy-" + getRandomString()
		var (
			profileRecordingDep = profileRecordingDescription{
				name:          "spo-recording",
				namespace:     ns,
				kind:          "SelinuxProfile",
				mergestrategy: "containers",
				labelKey:      "app",
				labelValue:    "hello-openshift",
				template:      profileRecordingTemplate,
			}
			saRoleRoleBindingDep = saRoleRoleBindingDescription{
				saName:          "spo-record-sa",
				namespace:       ns,
				roleName:        "spo-record" + getRandomString(),
				roleBindingName: "spo-record" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			deployHello = workloadDescription{
				name:         "hello-deployment",
				namespace:    ns,
				workloadKind: "Deployment",
				saName:       saRoleRoleBindingDep.saName,
				labelKey:     profileRecordingDep.labelKey,
				labelValue:   profileRecordingDep.labelValue,
				template:     workloadDeployTemplate,
			}
			deployHelloOpenshift = workloadDescription{
				name:         "hello-openshift",
				namespace:    ns,
				workloadKind: "Deployment",
				saName:       saRoleRoleBindingDep.saName,
				labelKey:     profileRecordingDep.labelKey,
				labelValue:   profileRecordingDep.labelValue,
				template:     workloadDeployHelloTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"ns", ns, ns})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns, "-n", ns, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns, "-n", ns, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilerecording", ns, profileRecordingDep.name})
		profileRecordingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingDep.name, "-n", ns}).check(oc)

		g.By("Create sa, role, rolebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"sa", ns, saRoleRoleBindingDep.saName},
			objectTableRef{"role", ns, saRoleRoleBindingDep.roleName},
			objectTableRef{"rolebinding", ns, saRoleRoleBindingDep.roleBindingName})
		saRoleRoleBindingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingDep.saName, "-n", ns}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingDep.roleName, "-n", ns}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingDep.roleBindingName, "-n", ns}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", ns, profileRecordingDep.name},
			objectTableRef{"deploy", ns, deployHello.name},
			objectTableRef{"selinuxprofiles", ns, "--all"})
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"deploy", ns, deployHello.name})
		deployHello.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"deploy", deployHello.name, "-n", ns, "-o=jsonpath={.status.availableReplicas}"}).check(oc)
		assertParameterValueForBulkPods(oc, "Running", "pod", "-l", profileRecordingDep.labelKey+"="+profileRecordingDep.labelValue, "-n", ns, "-o=jsonpath={.items[*].status.phase}")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"deploy", ns, deployHello.name})

		g.By("Check selinuxprofile generated !!!")
		//sleep 60s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(60 * time.Second)
		cleanupObjectsIgnoreNotFound(oc, objectTableRef{"deploy", ns, deployHello.name})
		checkPrfolieNumbers(oc, "selinuxprofiles", ns, 6)
		checkPrfolieStatus(oc, "selinuxprofiles", ns, "Partial")

		g.By("Create a second deployement !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", ns, profileRecordingDep.name},
			objectTableRef{"deploy", ns, deployHelloOpenshift.name},
			objectTableRef{"selinuxprofiles", ns, "--all"})
		deployHelloOpenshift.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"deploy", deployHelloOpenshift.name, "-n", ns, "-o=jsonpath={.status.availableReplicas}"}).check(oc)
		assertParameterValueForBulkPods(oc, "Running", "pod", "-l", profileRecordingDep.labelKey+"="+profileRecordingDep.labelValue, "-n", ns, "-o=jsonpath={.items[*].status.phase}")

		g.By("Check selinuxprofiles generated !!!")
		cleanupObjectsIgnoreNotFound(oc, objectTableRef{"deploy", ns, deployHelloOpenshift.name})
		assertParameterValueForBulkPods(oc, "Partial", "sp", "-n", ns, "-o=jsonpath={.items[*].status.status}")
		checkPrfolieNumbers(oc, "selinuxprofiles", ns, 9)
		//sleep 60s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(60 * time.Second)
		cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilerecording", ns, profileRecordingDep.name})
		checkPrfolieNumbers(oc, "selinuxprofiles", ns, 3)
		spLists, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("selinuxprofiles", "-n", ns, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(spLists, profileRecordingDep.name+"-nginx")).To(o.BeTrue())
		o.Expect(strings.Contains(spLists, profileRecordingDep.name+"-redis")).To(o.BeTrue())
		o.Expect(strings.Contains(spLists, profileRecordingDep.name+"-openshift")).To(o.BeTrue())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"selinuxprofiles", profileRecordingDep.name + "-nginx", "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"selinuxprofiles", profileRecordingDep.name + "-redis", "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"selinuxprofiles", profileRecordingDep.name + "-openshift", "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
	})

	g.It("ROSA-ARO-OSD_CCS-NonPreRelease-Author:xiyuan-Critical-51413-Verify restricting the allowed syscalls in seccomp profiles should work [Serial]", func() {
		nsAllow := "do-allow-" + getRandomString()
		nsDontAllow := "dont-allow-" + getRandomString()
		seccompAllow := seccompProfile{
			name:      "do-allow",
			namespace: nsAllow,
			template:  spSleepWithMkdirTemplate,
		}
		seccompDontAllow := seccompProfile{
			name:      "dont-allow",
			namespace: nsDontAllow,
			template:  spSleepWithMkdirTemplate,
		}

		g.By("Patch the spod !!!\n")
		defer func() {
			g.By("Recover the default setting.. !!!\n")
			patchRecover := fmt.Sprintf("[{\"op\": \"remove\", \"path\": \"/spec/allowedSyscalls\"}]")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "--type", "json", "--patch", patchRecover, "-n", subD.namespace)
			checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
			newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.allowedSyscalls[*]}"}).check(oc)
		}()
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "--patch-file", pathWebhookAllowedSyscalls)
		syscalls, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.allowedSyscalls[*]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		o.Expect(strings.Contains(syscalls, "brk")).To(o.BeTrue())
		o.Expect(strings.Contains(syscalls, "mkdir")).To(o.BeFalse())

		g.By("Create seccompprofiles in different namespaces !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"seccompprofile", seccompAllow.namespace, seccompAllow.name},
			objectTableRef{"seccompprofile", seccompDontAllow.namespace, seccompDontAllow.name},
			objectTableRef{"ns", seccompAllow.namespace, seccompAllow.namespace},
			objectTableRef{"ns", seccompDontAllow.namespace, seccompDontAllow.namespace})
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompAllow.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompDontAllow.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = applyResourceFromTemplateWithoutKeyword(oc, "mkdir", "--ignore-unknown-parameters=true", "-n", seccompAllow.namespace, "-f", seccompAllow.template, "-p", "NAME="+seccompAllow.name, "NAMESPACE="+seccompAllow.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		seccompDontAllow.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccompAllow.name, "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
		commonMessage := `syscall not allowed: mkdir`
		assertEventMessageRegexpMatch(oc, commonMessage, "event", "-n", seccompDontAllow.namespace, "--field-selector", "reason=ProfileNotAllowed", "-o=jsonpath={.items[*].message}")
	})

	g.It("ROSA-ARO-OSD_CCS-NonPreRelease-Author:xiyuan-High-49877-Create a SeccompProfile and apply it to the pod", func() {
		ns := "spo-test-" + getRandomString()
		spWithoutMkidr := seccompProfile{
			name:      "sp-not-allowed",
			namespace: ns,
			template:  spSleepWithMkdirTemplate,
		}
		spWithMkidr := seccompProfile{
			name:      "sp-allowed",
			namespace: ns,
			template:  spSleepWithMkdirTemplate,
		}
		testPodWithoutMkdir := podWithProfile{
			name:             "pod-not-allowed",
			namespace:        ns,
			localhostProfile: "",
			template:         podWithProfileTemplate,
		}
		testPodWithMkdir := podWithProfile{
			name:             "pod-allowed",
			namespace:        ns,
			localhostProfile: "",
			template:         podWithProfileTemplate,
		}

		g.By("Create seccompprofiles in different namespaces !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", ns, testPodWithoutMkdir.name},
			objectTableRef{"seccompprofile", ns, spWithoutMkidr.name},
			objectTableRef{"pod", ns, testPodWithMkdir.name},
			objectTableRef{"seccompprofile", ns, spWithMkidr.name},
			objectTableRef{"ns", ns, ns})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, ns)

		g.By("Created seccompprofiles !!!")
		err = applyResourceFromTemplateWithoutKeyword(oc, "mkdir", "--ignore-unknown-parameters=true", "-n", spWithoutMkidr.namespace, "-f", spWithoutMkidr.template, "-p", "NAME="+spWithoutMkidr.name, "NAMESPACE="+spWithoutMkidr.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		spWithMkidr.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", spWithoutMkidr.name, "-n", ns, "-o=jsonpath={.status.status}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", spWithMkidr.name, "-n", ns, "-o=jsonpath={.status.status}"})

		g.By("Created pods with the seccompprofiles as securityContext !!!")
		testPodWithoutMkdir.localhostProfile = "operator/" + ns + "/" + spWithoutMkidr.name + ".json"
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", testPodWithoutMkdir.template, "-p", "NAME="+testPodWithoutMkdir.name, "NAMESPACE="+testPodWithoutMkdir.namespace, "BASEPROFILENAME="+testPodWithoutMkdir.localhostProfile)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", testPodWithoutMkdir.name, "-n", testPodWithoutMkdir.namespace, "-o=jsonpath={.status.phase}"})
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", testPodWithMkdir.template, "-p", "NAME="+testPodWithMkdir.name, "NAMESPACE="+testPodWithMkdir.namespace, "BASEPROFILENAME="+testPodWithMkdir.localhostProfile)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", testPodWithMkdir.name, "-n", testPodWithMkdir.namespace, "-o=jsonpath={.status.phase}"})

		g.By("Check the result !!!")
		result, _ := oc.AsAdmin().Run("exec").Args(testPodWithoutMkdir.name, "-n", ns, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result)
		}
		result, _ = oc.AsAdmin().Run("exec").Args(testPodWithMkdir.name, "-n", ns, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result, "/tmpo/foo") && !strings.Contains(result, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result)
		}
	})

	g.It("ROSA-ARO-OSD_CCS-NonPreRelease-Author:xiyuan-Critical-49870-Check profilebinding working as expected for seccompprofile", func() {
		nsAllow := "do-allow-" + getRandomString()
		nsDontAllow := "dont-allow-" + getRandomString()
		podBusyboxAllow := "pod-busybox-allow"
		podBusyboxDontAllow := "pod-busybox-dont-allow"
		var (
			seccompAllow = seccompProfile{
				name:      "do-allow",
				namespace: nsAllow,
				template:  spSleepWithMkdirTemplate,
			}
			seccompDontAllow = seccompProfile{
				name:      "dont-allow",
				namespace: nsDontAllow,
				template:  spSleepWithMkdirTemplate,
			}
			profileBindingAllow = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   nsAllow,
				kind:        "SeccompProfile",
				profilename: seccompAllow.name,
				image:       "quay.io/openshifttest/busybox:latest",
				template:    profileBindingTemplate,
			}
			profileBindingDontAllow = profileBindingDescription{
				name:        "spo-binding-sec-" + getRandomString(),
				namespace:   nsDontAllow,
				kind:        "SeccompProfile",
				profilename: seccompDontAllow.name,
				image:       "quay.io/openshifttest/busybox:latest",
				template:    profileBindingTemplate,
			}
		)

		g.By("Create seccompprofiles in different namespaces !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"seccompprofile", seccompAllow.namespace, seccompAllow.name},
			objectTableRef{"seccompprofile", seccompDontAllow.namespace, seccompDontAllow.name},
			objectTableRef{"ns", seccompAllow.namespace, seccompAllow.namespace},
			objectTableRef{"ns", seccompDontAllow.namespace, seccompDontAllow.namespace})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompAllow.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", seccompDontAllow.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, nsAllow)
		exutil.SetNamespacePrivileged(oc, nsDontAllow)
		lableNamespace(oc, "namespace", nsAllow, "-n", nsAllow, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		lableNamespace(oc, "namespace", nsAllow, "-n", nsAllow, "spo.x-k8s.io/enable-binding=true", "--overwrite=true")
		err = applyResourceFromTemplateWithoutKeyword(oc, "mkdir", "--ignore-unknown-parameters=true", "-n", seccompAllow.namespace, "-f", seccompAllow.template, "-p", "NAME="+seccompAllow.name, "NAMESPACE="+seccompAllow.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		seccompDontAllow.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccompAllow.name, "-n", seccompP.namespace, "-o=jsonpath={.status.status}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installed", ok, []string{"seccompprofile", seccompDontAllow.name, "-n", seccompDontAllow.namespace, "-o=jsonpath={.status.status}"})

		g.By("Create profilebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilebinding", nsAllow, profileBindingAllow.name},
			objectTableRef{"profilebinding", nsDontAllow, profileBindingDontAllow.name})
		profileBindingAllow.create(oc)
		profileBindingDontAllow.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingAllow.name, ok, []string{"profilebinding", "-n", nsAllow,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, profileBindingDontAllow.name, ok, []string{"profilebinding", "-n", nsDontAllow,
			"-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		g.By("Created pods with seccompprofiles and check pods status !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"pod", nsAllow, podBusyboxAllow},
			objectTableRef{"pod", nsDontAllow, podBusyboxDontAllow})
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podBusybox, "-p", "NAME="+podBusyboxAllow, "NAMESPACE="+nsAllow)
		o.Expect(err1).NotTo(o.HaveOccurred())
		err2 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podBusybox, "-p", "NAME="+podBusyboxDontAllow, "NAMESPACE="+nsDontAllow)
		o.Expect(err2).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", podBusyboxAllow, "-n", nsAllow, "-o=jsonpath={.status.phase}"})
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", podBusyboxDontAllow, "-n", nsDontAllow, "-o=jsonpath={.status.phase}"})

		g.By("Check whether mkdir operator allowed for pods !!!")
		result1, _ := oc.AsAdmin().Run("exec").Args(podBusyboxAllow, "-n", nsAllow, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result1, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result1)
		}
		result2, _ := oc.AsAdmin().Run("exec").Args(podBusyboxDontAllow, "-n", nsDontAllow, "--", "sh", "-c", "mkdir /tmp/foo", "&&", "ls", "-d", "/tmp/foo").Output()
		if strings.Contains(result2, "/tmpo/foo") && !strings.Contains(result2, "Operation not permittedd") {
			e2e.Logf("%s is expected result", result2)
		}
	})

	g.It("ROSA-ARO-OSD_CCS-NonPreRelease-Author:xiyuan-Critical-56443-Security Profiles Operator should not crash with non-operator seccomp profiles [Slow][Disruptive]", func() {
		if exutil.IsSNOCluster(oc) || exutil.Is3MasterNoDedicatedWorkerNode(oc) {
			g.Skip("Skipped: Skip test for SNO/Compact clusters")
		}

		mcSeccomp := "customer-seccomp-" + getRandomString()
		podSeccomp := "pod-seccomp" + getRandomString()
		mcCustomRole := "wrscan"

		g.By("Label one rhcos worker node as wrscan.. !!!\n")
		workerNodeName := getOneRhcosWorkerNodeName(oc)
		setLabelToOneWorkerNode(oc, workerNodeName)

		defer func() {
			g.By("Remove custom mcp.. !!!\n")
			cleanupObjects(oc, objectTableRef{"mcp", subD.namespace, mcCustomRole})
			checkMachineConfigPoolStatus(oc, "worker")
			checkNodeStatus(oc)
		}()
		defer func() {
			g.By("Remove lables for the worker nodes !!!\n")
			removeLabelFromWorkerNode(oc, workerNodeName)
			checkMachineConfigPoolStatus(oc, "worker")
			newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", mcCustomRole, "-n", subD.namespace, "-o=jsonpath={.status.machineCount}"}).check(oc)
		}()

		g.By("Create wrscan machineconfigpool.. !!!\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", subD.namespace, "-f", machineConfigPoolYAML).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMachineConfigPoolStatus(oc, mcCustomRole)
		g.By("Create a legacy seccompprofile by machineconfig !!!")
		defer func() {
			g.By("Remove machineconfig.. !!!\n")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("mc", mcSeccomp, "-n", subD.namespace, "--ignore-not-found").Execute()
			newCheck("expect", asAdmin, withoutNamespace, compare, "1", ok, []string{"machineconfigpool", mcCustomRole, "-n", subD.namespace, "-o=jsonpath={.status.readyMachineCount}"}).check(oc)
			checkMachineConfigPoolStatus(oc, "worker")
		}()
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", mcSeccompNostatTemplate, "-n", subD.namespace, "NAME="+mcSeccomp, "MCROLE="+mcCustomRole)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "0", ok, []string{"machineconfigpool", mcCustomRole, "-n", subD.namespace, "-o=jsonpath={.status.readyMachineCount}"}).check(oc)
		checkMachineConfigPoolStatus(oc, mcCustomRole)

		g.By("Get the legacy seccompprofile name and create pod with it !!!")
		defer func() {
			g.By("Remove pod.. !!!\n")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podSeccomp, "-n", subD.namespace, "--ignore-not-found").Execute()
		}()
		path, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mc", mcSeccomp, "-n", subD.namespace, "-o=jsonpath={.spec.config.storage.files[0].path}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		arr := strings.Split(path, "/")
		seccomppath := "localhost/" + arr[len(arr)-1]
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podLegacySeccompTemplate, "NAME="+podSeccomp, "NAMESPACE="+subD.namespace, "SECCOMPPATH="+seccomppath,
			"NODEName="+workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{"pod", podSeccomp, "-n", subD.namespace, "-o=jsonpath={.status.phase}"})

		g.By("Check all pods should not crash !!!")
		checkReadyPodCountOfDeployment(oc, "security-profiles-operator-webhook", subD.namespace, 3)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
		checkReadyPodCountOfDeployment(oc, "security-profiles-operator", subD.namespace, 3)
	})

	// author: xiyuan@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Author:xiyuan-High-56012-Verify the mergeStrategy wroks for log enricher based seccompprofiles recording for deployment [Slow][Disruptive]", func() {
		ns := "merge-strategy-" + getRandomString()
		var (
			profileRecordingDep = profileRecordingDescription{
				name:          "spo-recording",
				namespace:     ns,
				kind:          "SeccompProfile",
				mergestrategy: "containers",
				labelKey:      "app",
				labelValue:    "hello-openshift",
				template:      profileRecordingTemplate,
			}
			saRoleRoleBindingDep = saRoleRoleBindingDescription{
				saName:          "spo-record-sa",
				namespace:       ns,
				roleName:        "spo-record" + getRandomString(),
				roleBindingName: "spo-record" + getRandomString(),
				template:        saRoleRolebindingTemplate,
			}
			deployHello = workloadDescription{
				name:         "hello-deployment",
				namespace:    ns,
				workloadKind: "Deployment",
				saName:       saRoleRoleBindingDep.saName,
				labelKey:     profileRecordingDep.labelKey,
				labelValue:   profileRecordingDep.labelValue,
				template:     workloadDeployTemplate,
			}
			deployHelloOpenshift = workloadDescription{
				name:         "hello-openshift",
				namespace:    ns,
				workloadKind: "Deployment",
				saName:       saRoleRoleBindingDep.saName,
				labelKey:     profileRecordingDep.labelKey,
				labelValue:   profileRecordingDep.labelValue,
				template:     workloadDeployHelloTemplate,
			}
		)

		g.By("Enable LogEnricher.. !!!\n")
		patch := fmt.Sprintf("{\"spec\":{\"enableLogEnricher\":true}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Create namespace and add labels !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"ns", ns, ns})
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		lableNamespace(oc, "namespace", ns, "-n", ns, "spo.x-k8s.io/enable-recording=true", "--overwrite=true")
		lableNamespace(oc, "namespace", ns, "-n", ns, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite=true")

		g.By("Create profilerecording !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilerecording", ns, profileRecordingDep.name})
		profileRecordingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"profilerecording", profileRecordingDep.name, "-n", ns}).check(oc)

		g.By("Create sa, role, rolebinding !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"sa", ns, saRoleRoleBindingDep.saName},
			objectTableRef{"role", ns, saRoleRoleBindingDep.roleName},
			objectTableRef{"rolebinding", ns, saRoleRoleBindingDep.roleBindingName})
		saRoleRoleBindingDep.create(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"sa", saRoleRoleBindingDep.saName, "-n", ns}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"role", saRoleRoleBindingDep.roleName, "-n", ns}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"rolebinding", saRoleRoleBindingDep.roleBindingName, "-n", ns}).check(oc)

		g.By("Create workload !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", ns, profileRecordingDep.name},
			objectTableRef{"deploy", ns, deployHello.name},
			objectTableRef{"sp", ns, "--all"})
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"deploy", ns, deployHello.name})
		deployHello.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"deploy", deployHello.name, "-n", ns, "-o=jsonpath={.status.availableReplicas}"}).check(oc)
		assertParameterValueForBulkPods(oc, "Running", "pod", "-l", profileRecordingDep.labelKey+"="+profileRecordingDep.labelValue, "-n", ns, "-o=jsonpath={.items[*].status.phase}")

		g.By("Check seccompprofile generated !!!")
		pod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", profileRecordingDep.labelKey+"="+profileRecordingDep.labelValue, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < 3; i++ {
			cmd := fmt.Sprintf("mknod /tmp/foo%d p", i)
			_, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("pods/"+pod, "-c", "nginx", "-n", ns, "--", "bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		// sleep 5s before deleting the workload
		time.Sleep(5 * time.Second)
		cleanupObjectsIgnoreNotFound(oc, objectTableRef{"deploy", ns, deployHello.name})
		checkPrfolieNumbers(oc, "seccompprofile", ns, 6)
		checkPrfolieStatus(oc, "seccompprofile", ns, "Partial")
		split_pod := strings.Split(pod, "-")
		suffix := split_pod[len(split_pod)-1]
		spLists, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sp", "-n", ns, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		spName := profileRecordingDep.name + "-nginx-" + suffix
		o.Expect(strings.Contains(spLists, spName)).To(o.BeTrue())
		for _, sp := range strings.Fields(spLists) {
			syscalls, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sp", sp, "-n", ns, "-o=jsonpath={.spec.syscalls[?(@.action==\"SCMP_ACT_ALLOW\")].names}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Compare(sp, spName) == 0 {
				o.Expect(strings.Contains(syscalls, "mknod")).To(o.BeTrue())
			} else {
				o.Expect(strings.Contains(syscalls, "mknod")).To(o.BeFalse())
			}
		}

		g.By("Create a second deployement !!!")
		defer cleanupObjectsIgnoreNotFound(oc,
			objectTableRef{"profilerecording", ns, profileRecordingDep.name},
			objectTableRef{"deploy", ns, deployHelloOpenshift.name},
			objectTableRef{"sp", ns, "--all"})
		deployHelloOpenshift.create(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "3", ok, []string{"deploy", deployHelloOpenshift.name, "-n", ns, "-o=jsonpath={.status.availableReplicas}"}).check(oc)
		assertParameterValueForBulkPods(oc, "Running", "pod", "-l", profileRecordingDep.labelKey+"="+profileRecordingDep.labelValue, "-n", ns, "-o=jsonpath={.items[*].status.phase}")

		g.By("Check seccompprofile generated !!!")
		cleanupObjectsIgnoreNotFound(oc, objectTableRef{"deploy", ns, deployHelloOpenshift.name})
		assertParameterValueForBulkPods(oc, "Partial", "sp", "-n", ns, "-o=jsonpath={.items[*].status.status}")
		checkPrfolieNumbers(oc, "seccompprofile", ns, 9)
		cleanupObjectsIgnoreNotFound(oc, objectTableRef{"profilerecording", ns, profileRecordingDep.name})
		checkPrfolieStatus(oc, "seccompprofile", ns, "Installed")
		checkPrfolieNumbers(oc, "seccompprofile", ns, 3)
		spLists, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sp", "-n", ns, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(spLists, profileRecordingDep.name+"-nginx")).To(o.BeTrue())
		o.Expect(strings.Contains(spLists, profileRecordingDep.name+"-redis")).To(o.BeTrue())
		o.Expect(strings.Contains(spLists, profileRecordingDep.name+"-openshift")).To(o.BeTrue())
		syscalls, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sp", profileRecordingDep.name+"-nginx", "-n", ns, "-o=jsonpath={.spec.syscalls[?(@.action==\"SCMP_ACT_ALLOW\")].names}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(syscalls, "mknod")).To(o.BeTrue())
	})

	// author: bgudi@redhat.com
	g.It("ROSA-ARO-OSD_CCS-Author:bgudi-Low-49887-Set log verbosity for security profiles operator [Serial]", func() {
		defer func() {
			g.By("Cleanup.. !!!\n")
			patch := fmt.Sprintf("{\"spec\":{\"verbosity\":0}}")
			patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
			newCheck("expect", asAdmin, withoutNamespace, contain, "0", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.verbosity}"}).check(oc)
			checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)
			msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("ds/spod", "security-profiles-operator", "-n", subD.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(msg, `Set logging verbosity to 0`)).To(o.BeTrue())
		}()

		g.By("Check ds logs and verify log verbosity is 0 before patch")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("ds/spod", "security-profiles-operator", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, `Set logging verbosity to 0`)).To(o.BeTrue())

		g.By("set log verbosity for securityprofilesoperator daemon\n")
		patch := fmt.Sprintf("{\"spec\":{\"verbosity\":1}}")
		patchResource(oc, asAdmin, withoutNamespace, "spod", "spod", "-n", subD.namespace, "--type", "merge", "-p", patch)
		newCheck("expect", asAdmin, withoutNamespace, contain, "1", ok, []string{"spod", "spod", "-n", subD.namespace, "-o=jsonpath={.spec.verbosity}"}).check(oc)

		g.By("Check all spo pods are reinited")
		checkPodsStautsOfDaemonset(oc, "spod", subD.namespace)

		g.By("Check ds logs and verify log verbosity is set to 1")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("ds/spod", "security-profiles-operator", "-n", subD.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, `Set logging verbosity to 1`)).To(o.BeTrue())
	})

	// author: xiyuan@redhat.com
	g.It("ROSA-ARO-OSD_CCS-CPaasrunOnly-Author:xiyuan-High-71797-security profiles operator should pass DAST test", func() {
		configFile := filepath.Join(buildPruningBaseDir, "rapidast/data_rapidastconfig_security-profiles-operator_v1beta1.yaml")
		policyFile := filepath.Join(buildPruningBaseDir, "rapidast/customscan.policy")
		_, err := rapidastScan(oc, oc.Namespace(), configFile, policyFile, "security-profiles-operator_v1beta1")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: xiyuan@redhat.com
	// The Disruptive label could be removed once the bug https://issues.redhat.com/browse/OCPBUGS-4126 resolved
	g.It("Author:xiyuan-ROSA-ARO-OSD_CCS-ConnectedOnly-NonPreRelease-Critical-50013-check Log enricher based seccompprofiles recording and metrics working as expected for pod [Slow][Disruptive]", func() {
		// skip test if telemetry not found
		skipNotelemetryFound(oc)

		ns1 := "mytest" + getRandomString()
		var (
			profileRecordingPod = profileRecordingDescription{
				name:          "test-recording",
				namespace:     ns1,
				kind:          "SeccompProfile",
				mergestrategy: "none",
				labelKey:      "app",
				labelValue:    "my-app",
				template:      profileRecordingTemplate,
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

		g.By("Check SeccompProfile generated !!!")
		defer cleanupObjectsIgnoreNotFound(oc, objectTableRef{"SeccompProfile", ns1, "--all"})
		//sleep 60s so the selinuxprofiles of the worklod could be recorded
		time.Sleep(60 * time.Second)
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", podHello.name, "-n", ns1, "--ignore-not-found").Execute()
		checkPrfolieStatus(oc, "SeccompProfile", ns1, "Installed")
		checkPrfolieNumbers(oc, "SeccompProfile", ns1, 1)

		g.By("Check metrics !!!\n")
		metricsSeccompProfileAuditTotal := fmt.Sprintf("security_profiles_operator_seccomp_profile_audit_total{container=\"redis\",executable=\"/usr/local/bin/redis-server\",namespace=\"%s\"", ns1)
		metricsSeccompProfileTotal := fmt.Sprintf("security_profiles_operator_seccomp_profile_total{operation=\"update\"}")
		metricStr := []string{metricsSeccompProfileAuditTotal, metricsSeccompProfileTotal}
		url := fmt.Sprintf("https://metrics." + subD.namespace + ".svc/metrics-spod")
		checkMetric(oc, metricStr, url)
	})
})
