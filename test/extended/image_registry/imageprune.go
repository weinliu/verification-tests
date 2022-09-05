package imageregistry

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()

	var (
		oc                 = exutil.NewCLI("default-image-prune", exutil.KubeConfigPath())
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
		token, err := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
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
			if !foundValue {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Don't find the value")
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Low-43717-Add necessary priority class to pruner", func() {
		g.By("Check priority class of pruner")
		out := getResource(oc, asAdmin, withoutNamespace, "cronjob.batch", "-n", "openshift-image-registry", "-o=jsonpath={.items[0].spec.jobTemplate.spec.template.spec.priorityClassName}")
		o.Expect(out).To(o.ContainSubstring(priorityClassName))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-35292-LogLevel setting for the pruner [Serial]", func() {
		g.By("Set imagepruner cronjob started every 1 minutes")
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":"*/1 * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":""}}`, "--type=merge").Execute()

		g.By("Check log when imagerpruner loglevel is Normal")
		imagePruneLog(oc, normalInfo, "DEBUGTEST")

		g.By("Check log when imagerpruner loglevel is Debug")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"Debug"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"Normal"}}`, "--type=merge").Execute()
		imagePruneLog(oc, debugInfo, "DEBUGTEST")

		g.By("Check log when imagerpruner loglevel is Trace")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"Trace"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		imagePruneLog(oc, traceInfo, "DEBUGTEST")

		g.By("Check log when imagerpruner loglevel is TraceAll")
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"logLevel":"TraceAll"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		imagePruneLog(oc, traceAllInfo, "DEBUGTEST")

	})
	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-44113-Image pruner should use custom tolerations [Serial]", func() {
		g.By("Set tolerations for imagepruner cluster")
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"tolerations":[{"effect":"NoSchedule","key":"key","operator":"Equal","value":"value"}]}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"tolerations":null}}`, "--type=merge").Execute()
		g.By("Check image pruner cron job uses these tolerations")
		out := getResource(oc, asAdmin, withoutNamespace, "cronjob/image-pruner", "-n", "openshift-image-registry", "-o=jsonpath={.spec.jobTemplate.spec.template.spec.tolerations}")
		o.Expect(out).Should(o.Equal(tolerationsInfo))
	})

	//Author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:xiuwang-Medium-44107-Image pruner should skip images that has already been deleted [Serial]", func() {
		g.By("Setup imagepruner")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"keepTagRevisions":3,"keepYoungerThanDuration":null,"schedule":""}}`, "--type=merge").Execute()
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"keepTagRevisions":0,"keepYoungerThanDuration":"0s","schedule": "* * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Image pruner should tolerate concurrent deletion of image objects")
		oc.SetupProject()
		for i := 0; i < 3; i++ {
			bcName := getRandomString()
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("registry.redhat.io/rhel8/httpd-24:latest~https://github.com/openshift/httpd-ex.git", fmt.Sprintf("--name=%s", bcName), "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), fmt.Sprintf("%s-1", bcName), nil, nil, nil)
			if err != nil {
				exutil.DumpBuildLogs(bcName, oc)
			}
			exutil.AssertWaitPollNoErr(err, "build is not complete")

			g.By("Delete imagestreamtag when the pruner is processing")
			err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), bcName, "latest")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("istag", fmt.Sprintf("%s:latest", bcName), "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			imagePruneLog(oc, "", fmt.Sprintf("%s", bcName))
		}

		g.By("Check if imagepruner degraded image registry")
		out := getResource(oc, asAdmin, withoutNamespace, "imagepruner/cluster", "-o=jsonpath={.status.conditions}")
		o.Expect(out).To(o.ContainSubstring(`"reason":"Complete"`))
	})

	// author: xiuwang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Medium-33708-Verify spec.ignoreInvalidImageReference with invalid image reference [Serial]", func() {
		var (
			imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
			podFile              = filepath.Join(imageRegistryBaseDir, "single-pod.yaml")
			podsrc               = podSource{
				name:      "pod-pull-with-invalid-image",
				namespace: "",
				image:     "quay.io/openshifttest/hello-openshift:1.2.0@",
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
		imagePruneLog(oc, `"quay.io/openshifttest/hello-openshift:1.2.0@": invalid reference format - skipping`, "DEBUGTEST")

		// Add retry check when imagepruner job failed https://bugzilla.redhat.com/show_bug.cgi?id=1990125
		g.By("Check if imagepruner retry after failed")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"ignoreInvalidImageReferences":true}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"ignoreInvalidImageReferences":false}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		imagePruneLog(oc, `attempt #1 has failed (exit code 1), going to make another attempt`, "DEBUGTEST")
	})

	//Author: xiuwang@redhat.com
	g.It("Author:xiuwang-Medium-15126-Registry hard prune procedure works well [Serial]", func() {
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

	//author: xiuwang@redhat.com
	g.It("Author:xiuwang-Medium-52705-Medium-11623-Could delete recently created images when --prune-over-size-limit is used", func() {
		var (
			imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
			limitFile            = filepath.Join(imageRegistryBaseDir, "project-limitRange-image.yaml")
			limitsrc             = limitSource{
				name:      "52705-image-limit-range",
				namespace: "",
				size:      "1Mi",
				template:  limitFile,
			}
		)
		g.By("Create 2 imagestream, one image is large than 1M, anther one is small than 1M")
		oc.SetupProject()
		err := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("busybox:smaller", "--from", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "busybox", "smaller")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3", "registry:bigger", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "registry", "bigger")

		g.By("Create project limit for image")
		limitsrc.namespace = oc.Namespace()
		limitsrc.create(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("policy").Args("remove-role-from-user", "system:image-pruner", "-z", "default", "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("policy").Args("add-role-to-user", "system:image-pruner", "-z", "default", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(oc, regRoute)

		token, err := getSAToken(oc, "default", oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		g.By("Could prune images with --prune-over-size-limit")
		pruneout, pruneerr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--prune-over-size-limit", "--token="+token, "--registry-url="+regRoute, "-n", oc.Namespace()).Output()
		o.Expect(pruneerr).NotTo(o.HaveOccurred())
		o.Expect(pruneout).To(o.ContainSubstring("Deleting 1 items from image stream " + oc.Namespace() + "/registry"))
		o.Expect(pruneout).NotTo(o.ContainSubstring(oc.Namespace() + "/busybox"))

		g.By("11623-Can not prune image by conflicted condition flags")
		conflictinfo1, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--prune-over-size-limit", "--token="+token, "--registry-url="+regRoute, "-n", oc.Namespace(), "--keep-younger-than=1m", "--confirm").Output()
		o.Expect(conflictinfo1).To(o.ContainSubstring("error: --prune-over-size-limit cannot be specified with --keep-tag-revisions nor --keep-younger-than"))
		conflictinfo2, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--prune-over-size-limit", "--token="+token, "--registry-url="+regRoute, "-n", oc.Namespace(), "--keep-tag-revisions=1", "--confirm").Output()
		o.Expect(conflictinfo2).To(o.ContainSubstring("error: --prune-over-size-limit cannot be specified with --keep-tag-revisions nor --keep-younger-than"))

	})

	//author: wewang@redhat.com
	g.It("Author:wewang-Hign-27576-ImageRegistry CronJob is added to automate image prune [Disruptive]", func() {
		g.By("Check imagepruner fields")
		output, checkErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("imagepruner/cluster", "-o", "yaml").Output()
		o.Expect(checkErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("failedJobsHistoryLimit: 3"))
		o.Expect(output).To(o.ContainSubstring("ignoreInvalidImageReferences: true"))
		o.Expect(output).To(o.ContainSubstring("keepTagRevisions: 3"))
		o.Expect(output).To(o.ContainSubstring(`schedule: ""`))
		o.Expect(output).To(o.ContainSubstring("successfulJobsHistoryLimit: 3"))
		o.Expect(output).To(o.ContainSubstring("suspend: false"))
		g.By("Update imagepruner fields")
		defer oc.AsAdmin().Run("delete").Args("imagepruner/cluster").Execute()
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"failedJobsHistoryLimit": 1,"keepTagRevisions": 1,"ignoreInvalidImageReferences": false, "schedule": "* * * * *", "successfulJobsHistoryLimit": 1}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, checkErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("imagepruner/cluster", "-o", "yaml").Output()
		o.Expect(checkErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("failedJobsHistoryLimit: 1"))
		o.Expect(output).To(o.ContainSubstring("ignoreInvalidImageReferences: false"))
		o.Expect(output).To(o.ContainSubstring("keepTagRevisions: 1"))
		o.Expect(output).To(o.ContainSubstring(`schedule: '* * * * *'`))
		o.Expect(output).To(o.ContainSubstring("successfulJobsHistoryLimit: 1"))

		g.By("Check imagepruner pod is running")
		errWait := wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-image-registry").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(output, "image-pruner") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "no image-pruner created")
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-High-27577-Explain and check the custom resource definition for the prune", func() {
		g.By("Check the custom resource definition for the prune")
		result, explainErr := oc.WithoutNamespace().AsAdmin().Run("explain").Args("imagepruners", "--api-version=imageregistry.operator.openshift.io/v1").Output()
		o.Expect(explainErr).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("ImagePruner is the configuration object for an image registry pruner"))
		o.Expect(result).To(o.ContainSubstring("ImagePrunerSpec defines the specs for the running image pruner"))
		o.Expect(result).To(o.ContainSubstring("ImagePrunerStatus reports image pruner operational status"))
	})

	// author: wewang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:wewang-Medium-17167-Image soft prune via 'prune-registry' option with invalid argument", func() {
		result, _ := oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--keep_tag_revisions=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--confirm=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--keep_younger_than=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--prune_over_size_limit=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--prune_registry=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
	})
})
