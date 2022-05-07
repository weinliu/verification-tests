package image_registry

import (
	"fmt"
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("default-image-prune", exutil.KubeConfigPath())
		logInfo            = "Only API objects will be removed.  No modifications to the image registry will be made"
		warnInfo           = "batch/v1beta1 CronJob is deprecated in v1.21+, unavailable in v1.25+; use batch/v1 CronJob"
		monitoringns       = "openshift-monitoring"
		promPod            = "prometheus-k8s-0"
		queryImagePruner   = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=image_registry_operator_image_pruner_install_status"
		queryImageRegistry = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=image_registry_operator_storage_reconfigured_total"
		priorityClassName  = "system-cluster-critical"
		normalInfo         = "Creating image pruner with keepYoungerThan"
		debugInfo          = "Examining ImageStream"
		traceInfo          = "keeping because it is used by imagestreams"
		traceAllInfo       = "Content-Type: application/json"
		tolerationsInfo    = `[{"effect":"NoSchedule","key":"key","operator":"Equal","value":"value"}]`
	)
	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-35906-Only API objects will be removed in image pruner pod when image registry is set to Removed [Disruptive]", func() {
		g.By("Check platforms")
		//When registry configured using pvc, the following removed registry operation will remove pvc too.
		//This is not suitable for the defer recoverage. Only run this case on cloud storage.
		if checkRegistryUsingFSVolume(oc) {
			g.Skip("Skip for fs volume")
		}

		g.By("Set image registry cluster Removed")
		err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Set image registry cluster Managed")
			oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Execute()
			recoverRegistryDefaultPods(oc)
		}()
		checkRegistrypodsRemoved(oc)

		g.By("Set imagepruner cronjob started every 2 minutes")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":"*/2 * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":""}}`, "--type=merge").Execute()
		podsOfImagePrune := []corev1.Pod{}
		foundImagePruneLog := false
		err = wait.Poll(25*time.Second, 3*time.Minute, func() (bool, error) {
			podsOfImagePrune = ListPodStartingWith("image-pruner", oc, "openshift-image-registry")
			if len(podsOfImagePrune) == 0 {
				e2e.Logf("Go to next round")
				return false, nil
			}
			foundImagePruneLog = DePodLogs(podsOfImagePrune, oc, logInfo)
			if foundImagePruneLog != true {
				e2e.Logf("Go to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Error retrieving logs")

		g.By("Check the log of image pruner and expected info about:Only API objects will be removed")
		foundWarnPruneLog := true
		foundWarnPruneLog = DePodLogs(podsOfImagePrune, oc, warnInfo)
		o.Expect(!foundWarnPruneLog).To(o.BeTrue())
	})

	// author: wewang@redhat.com
	g.It("ConnectedOnly-Author:wewang-High-27613-registry operator can publish metrics reporting the status of image-pruner [Disruptive]", func() {
		g.By("granting the cluster-admin role to user")
		oc.SetupProject()
		_, err := oc.AsAdmin().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", oc.Username()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("adm").Args("policy", "remove-cluster-role-from-user", "cluster-admin", oc.Username()).Execute()
		_, err = oc.AsAdmin().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-monitoring-view", oc.Username()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("adm").Args("policy", "remove-cluster-role-from-user", "cluster-monitoring-view", oc.Username()).Execute()
		g.By("Get prometheus token")
		token, err := oc.AsAdmin().WithoutNamespace().Run("sa").Args("-n", "openshift-monitoring", "get-token", "prometheus-k8s").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Prometheus query results report image pruner installed")
		foundValue := metricReportStatus(queryImagePruner, monitoringns, promPod, token, 2)
		o.Expect(foundValue).To(o.BeTrue())
		g.By("Prometheus query results report image registry operator not reconfiged")
		foundValue = metricReportStatus(queryImageRegistry, monitoringns, promPod, token, 0)
		o.Expect(foundValue).To(o.BeTrue())

		g.By("Set imagepruner suspend")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"suspend":true}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"suspend":false}}`, "--type=merge").Execute()
		g.By("Prometheus query results report image registry operator not reconfiged")
		foundValue = metricReportStatus(queryImageRegistry, monitoringns, promPod, token, 0)
		o.Expect(foundValue).To(o.BeTrue())
		g.By("Prometheus query results report image pruner not installed")
		err = wait.PollImmediate(30*time.Second, 1*time.Minute, func() (bool, error) {
			foundValue = metricReportStatus(queryImagePruner, monitoringns, promPod, token, 1)
			if foundValue != true {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the value")
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Low-43717-Add necessary priority class to pruner", func() {
		g.By("Check priority class of pruner")
		out := getResource(oc, asAdmin, withoutNamespace, "cronjob.batch", "-n", "openshift-image-registry", "-o=jsonpath={.items[0].spec.jobTemplate.spec.template.spec.priorityClassName}")
		o.Expect(out).To(o.ContainSubstring(priorityClassName))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-35292-LogLevel setting for the pruner", func() {
		g.By("Set imagepruner cronjob started every 1 minutes")
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":"*/1 * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":""}}`, "--type=merge").Execute()

		g.By("Check log when imagerpruner loglevel is Normal")
		foundPruneLog := false
		err = wait.PollImmediate(30*time.Second, 90*time.Second, func() (bool, error) {
			foundPruneLog = imagePruneLog(oc, normalInfo)
			if foundPruneLog != true {
				e2e.Logf("Go to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the normalInfo value")

		g.By("Check log when imagerpruner loglevel is Debug")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"Debug"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"Normal"}}`, "--type=merge").Execute()
		foundPruneLog = false
		err = wait.PollImmediate(30*time.Second, 90*time.Second, func() (bool, error) {
			foundPruneLog = imagePruneLog(oc, debugInfo)
			if foundPruneLog != true {
				e2e.Logf("Go to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the debugInfo value")

		g.By("Check log when imagerpruner loglevel is Trace")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"Trace"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		foundPruneLog = false
		err = wait.PollImmediate(30*time.Second, 90*time.Second, func() (bool, error) {
			foundPruneLog = imagePruneLog(oc, traceInfo)
			if foundPruneLog != true {
				e2e.Logf("Go to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the traceInfo value")

		g.By("Check log when imagerpruner loglevel is TraceAll")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"TraceAll"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		foundPruneLog = false
		err = wait.PollImmediate(30*time.Second, 90*time.Second, func() (bool, error) {
			foundPruneLog = imagePruneLog(oc, traceAllInfo)
			if foundPruneLog != true {
				e2e.Logf("Go to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the traceAllInfo value")
	})
	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-44113-Image pruner should use custom tolerations", func() {
		g.By("Set tolerations for imagepruner cluster")
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"tolerations":[{"effect":"NoSchedule","key":"key","operator":"Equal","value":"value"}]}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"tolerations":null}}`, "--type=merge").Execute()
		g.By("Check image pruner cron job uses these tolerations")
		out := getResource(oc, asAdmin, withoutNamespace, "cronjob/image-pruner", "-n", "openshift-image-registry", "-o=jsonpath={.spec.jobTemplate.spec.template.spec.tolerations}")
		o.Expect(out).Should(o.Equal(tolerationsInfo))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-High-27588-ManagementState setting in Image registry operator config can influence image prune [Disruptive]", func() {
		//When registry configured using pvc, the following removed registry operation will remove pvc too.
		//This is not suitable for the defer recoverage. Only run this case on cloud storage.
		if checkRegistryUsingFSVolume(oc) {
			g.Skip("Skip for fs volume")
		}

		g.By("In default image registry cluster Managed and prune-registry flag is true")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.managementState}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("Managed"))
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob.batch/image-pruner", "-n", "openshift-image-registry", "-o=jsonpath={.spec.jobTemplate.spec.template.spec.containers[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("--prune-registry=true"))

		g.By("Set image registry cluster Removed")
		defer func() {
			g.By("Set image registry cluster Managed")
			err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			recoverRegistryDefaultPods(oc)
		}()
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check image-registry pods are removed")
		checkRegistrypodsRemoved(oc)

		g.By("Check prune-registry flag is false")
		time.Sleep(5 * time.Second)
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob.batch/image-pruner", "-n", "openshift-image-registry", "-o=jsonpath={.spec.jobTemplate.spec.template.spec.containers[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("--prune-registry=false"))

		g.By("Make update in the pruning custom resource")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":""}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":"*/1 * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podsOfImagePrune := []corev1.Pod{}
		foundImagePruneLog := false
		err = wait.PollImmediate(30*time.Second, 2*time.Minute, func() (bool, error) {
			podsOfImagePrune = ListPodStartingWith("image-pruner", oc, "openshift-image-registry")
			if len(podsOfImagePrune) == 0 {
				e2e.Logf("Go to next round")
				return false, nil
			}
			foundImagePruneLog = DePodLogs(podsOfImagePrune, oc, logInfo)
			if foundImagePruneLog != true {
				e2e.Logf("Go to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the expect infor")
	})

	//Author: xiuwang@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:xiuwang-Medium-44107-Image pruner should skip images that has already been deleted [Serial][Slow]", func() {
		g.By("Setup imagepruner")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"keepTagRevisions":3,"keepYoungerThanDuration":null,"schedule":""}}`, "--type=merge").Execute()
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"keepTagRevisions":0,"keepYoungerThanDuration":"0s","schedule": "* * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Image pruner should tolerate concurrent deletion of image objects")
		oc.SetupProject()
		for i := 0; i < 6; i++ {
			bcName := getRandomString()
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("registry.redhat.io/rhel8/httpd-24:latest~https://github.com/openshift/httpd-ex.git", fmt.Sprintf("--name=%s", bcName), "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), fmt.Sprintf("%s-1", bcName), nil, nil, nil)
			if err != nil {
				exutil.DumpBuildLogs(bcName, oc)
			}
			exutil.AssertWaitPollNoErr(err, "build is not complete")

			g.By("Delete imagestreamtag when the pruner is processing")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("istag", fmt.Sprintf("%s:latest", bcName), "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			foundPruneLog := true
			foundPruneLog = imagePruneLog(oc, fmt.Sprintf("%s", bcName))
			o.Expect(foundPruneLog).To(o.BeFalse())
		}

		g.By("Check if imagepruner degraded image registry")
		out := getResource(oc, asAdmin, withoutNamespace, "imagepruner/cluster", "-o=jsonpath={.status.conditions}")
		o.Expect(out).To(o.ContainSubstring(`"reason":"Complete"`))
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Medium-33708-Verify spec.ignoreInvalidImageReference with invalid image reference [Serial]", func() {
		var (
			imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
			podFile              = filepath.Join(imageRegistryBaseDir, "single-pod.yaml")
			podsrc               = podSource{
				name:      "pod-pull-with-invalid-image",
				namespace: "",
				image:     "quay.io/openshifttest/hello-openshift:multiarch@",
				template:  podFile,
			}
		)

		g.By("Setup imagepruner running every minute")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":""}}`, "--type=merge").Execute()
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule": "* * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create pod with invalid image")
		oc.SetupProject()
		podsrc.namespace = oc.Namespace()
		podsrc.create(oc)
		foundPruneLog := false
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			foundPruneLog = imagePruneLog(oc, `"quay.io/openshifttest/hello-openshift:multiarch@": invalid reference format - skipping`)
			if foundPruneLog != true {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the value")

		// Add retry check when imagepruner job failed https://bugzilla.redhat.com/show_bug.cgi?id=1990125
		g.By("Check if imagepruner retry after failed")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"ignoreInvalidImageReferences":true}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"ignoreInvalidImageReferences":false}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		foundPruneLog = false
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			foundPruneLog = imagePruneLog(oc, `attempt #1 has failed (exit code 1), going to make another attempt`)
			if foundPruneLog != true {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Imagepruner doesn't retry")
	})

	//Author: xiuwang@redhat.com
	g.It("NonPreRelease-Author:xiuwang-Medium-15126-Registry hard prune procedure works well [Serial]", func() {
		if !checkRegistryUsingFSVolume(oc) {
			g.Skip("Skip for cloud storage")
		}
		g.By("Push uniqe images to internal registry")
		oc.SetupProject()
		err := oc.Run("new-build").Args("-D", "FROM quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--to=image-15126").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "image-15126-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs("image-15126", oc)
		}
		exutil.AssertWaitPollNoErr(err, "build is not complete")
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "image-15126", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		manifest := saveImageMetadataName(oc, oc.Namespace()+"/image-15126")
		if len(manifest) == 0 {
			e2e.Failf("Expect image not existing")
		}

		g.By("Delete image from etcd manually")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("image", manifest).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Add system:image-pruner role to system:serviceaccount:openshift-image-registry:registry")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check invaild image source can be pruned")
		output, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/usr/bin/dockerregistry", "-prune=check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Would delete manifest link: %s/image-15126`, oc.Namespace()))
		output, err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/usr/bin/dockerregistry", "-prune=delete").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Deleting manifest link: %s/image-15126`, oc.Namespace()))
	})
})
