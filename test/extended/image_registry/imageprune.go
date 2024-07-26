package imageregistry

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()

	var (
		oc                   = exutil.NewCLI("default-image-prune", exutil.KubeConfigPath())
		queryImagePruner     = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=image_registry_operator_image_pruner_install_status"
		queryImageRegistry   = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=image_registry_operator_storage_reconfigured_total"
		priorityClassName    = "system-cluster-critical"
		normalInfo           = "Creating image pruner with keepYoungerThan"
		debugInfo            = "Examining ImageStream"
		traceInfo            = "keeping because it is used by imagestream"
		traceAllInfo         = "Content-Type: application/json"
		tolerationsInfo      = `[{"effect":"NoSchedule","key":"key","operator":"Equal","value":"value"}]`
		imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
	)
	g.BeforeEach(func() {
		if !checkOptionalOperatorInstalled(oc, "ImageRegistry") {
			g.Skip("Skip for the test due to image registry not installed")
		}
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:wewang-High-27613-registry operator can publish metrics reporting the status of image-pruner [Disruptive]", func() {
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
		o.Expect(doPrometheusQuery(oc, token, queryImagePruner)).To(o.Equal(2))

		g.By("Prometheus query results report image registry operator not reconfiged")
		o.Expect(doPrometheusQuery(oc, token, queryImageRegistry) >= 0).Should(o.BeTrue())

		g.By("Set imagepruner suspend")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"suspend":false}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"suspend":true}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Prometheus query results report image registry operator not reconfiged")
		o.Expect(doPrometheusQuery(oc, token, queryImageRegistry) >= 0).Should(o.BeTrue())

		g.By("Prometheus query results report image pruner not installed")
		o.Expect(doPrometheusQuery(oc, token, queryImagePruner)).To(o.Equal(1))
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
		// TODO: remove this skip when the builds v1 API will support producing manifest list images
		architecture.SkipArchitectures(oc, architecture.MULTI)
		if !checkImagePruners(oc) {
			g.Skip("This cluster does't contain imagepruners, skip the test.")
		}
		g.By("Setup imagepruner")
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"keepTagRevisions":3,"keepYoungerThanDuration":null,"schedule":""}}`, "--type=merge").Execute()
		err := oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"keepTagRevisions":0,"keepYoungerThanDuration":"0s","schedule": "* * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Image pruner should tolerate concurrent deletion of image objects")
		oc.SetupProject()
		for i := 0; i < 3; i++ {
			bcName := getRandomString()
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("registry.redhat.io/rhel8/httpd-24:latest~https://github.com/openshift/httpd-ex.git", fmt.Sprintf("--name=%s", bcName), "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), fmt.Sprintf("%s-1", bcName), nil, nil, nil)
			if err != nil {
				exutil.DumpBuildLogs(bcName, oc)
			}
			exutil.AssertWaitPollNoErr(err, "build is not complete")

			g.By("Delete imagestreamtag when the pruner is processing")
			err = waitForAnImageStreamTag(oc, oc.Namespace(), bcName, "latest")
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
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "image-15126", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		manifest := saveImageMetadataName(oc, oc.Namespace()+"/image-15126")
		if manifest == "" {
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
				size:      "3Mi",
				template:  limitFile,
			}
		)
		g.By("Create 2 imagestream, one image is large than 1M, anther one is small than 1M")
		oc.SetupProject()
		err := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("busybox:smaller", "--from", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "busybox", "smaller")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3", "registry:bigger", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForAnImageStreamTag(oc, oc.Namespace(), "registry", "bigger")

		g.By("Create project limit for image")
		limitsrc.namespace = oc.Namespace()
		limitsrc.create(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", "-z", "default", "-n", oc.Namespace()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", "-z", "default", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

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
	g.It("Author:wewang-High-27576-ImageRegistry CronJob is added to automate image prune [Disruptive]", func() {
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
		errWait := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
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
		result, _ := oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--keep-tag-revisions=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--confirm=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--keep-younger-than=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--prune-over-size-limit=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
		result, _ = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--prune-registry=abc").Output()
		o.Expect(result).To(o.ContainSubstring("invalid argument"))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-32329-keepYoungerThanDuration can be defined for image-pruner [Disruptive]", func() {
		g.By(" Define keepYoungerThan in imagepruner")
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("imagepruner/cluster", "-p", `{"spec": {"keepYoungerThan": null}}`, "--type=merge").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("imagepruner/cluster", "-p", `{"spec": {"keepYoungerThan": 60}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("cronjob/image-pruner", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("keep-younger-than=60ns"))

		g.By(" Define keepYoungerThanDuration in imagepruner")
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("imagepruner/cluster", "-p", `{"spec": {"keepYoungerThanDuration": null}}`, "--type=merge").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("imagepruner/cluster", "-p", `{"spec": {"keepYoungerThanDuration": "90s"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("cronjob/image-pruner", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("keep-younger-than=1m30s"))
	})

	//author: wewang@redhat.com
	g.It("ConnectedOnly-Author:wewang-High-16495-High-19196-No prune layer of a valid Image due to minimum aging and prune images when DC reference to invalid image [Disruptive]", func() {
		// TODO: remove this skip when the builds v1 API will support producing manifest list images
		architecture.SkipArchitectures(oc, architecture.MULTI)
		// Check if openshift-sample operator installed
		sampleOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/openshift-samples").Output()
		if err != nil && strings.Contains(sampleOut, `openshift-samples" not found`) {
			g.Skip("Skip test for openshift-samples which managed templates and imagestream are not installed")
		}
		sampleOut, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.samples/cluster", "-o=jsonpath={.spec.managementState}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if sampleOut == "Removed" {
			g.Skip("Skip test for openshift-samples which is removed")
		}

		g.By("Check if it's a https_proxy cluster")
		output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec}").Output()
		if strings.Contains(output, "httpProxy") && strings.Contains(output, "user-ca-bundle") {
			g.Skip("Skip for non https_proxy platform")
		}

		g.By("Get server host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		refRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		checkDnsCO(oc)
		waitRouteReady(refRoute)

		g.By("Add system:image-pruner role to user")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", oc.Username()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Start a new build")
		createAppError := oc.WithoutNamespace().AsAdmin().Run("new-build").Args("openshift/ruby:3.1-ubi8~https://github.com/sclorg/ruby-ex.git", "-n", oc.Namespace()).Execute()
		o.Expect(createAppError).NotTo(o.HaveOccurred())

		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "ruby-ex-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs("ruby-ex", oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		firOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is/ruby-ex", "-n", oc.Namespace(), "-o=jsonpath={.status.tags[0].items[0].image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(3 * time.Minute)

		g.By("Create another build")
		createAppError = oc.WithoutNamespace().AsAdmin().Run("new-build").Args("quay.io/openshifttest/ruby-27:1.2.0~https://github.com/sclorg/ruby-ex.git", "--name=test-16495", "-n", oc.Namespace()).Execute()
		o.Expect(createAppError).NotTo(o.HaveOccurred())
		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "test-16495-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs("test-16495", oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForAnImageStreamTag(oc, oc.Namespace(), "test-16495", "latest")
		secOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is/test-16495", "-n", oc.Namespace(), "-o=jsonpath={.status.tags[0].items[0].image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Prune images")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--keep-younger-than=2m", "--token="+token, "--registry-url="+refRoute, "--confirm", "--loglevel=4").Output()
		o.Expect(out).To(o.ContainSubstring("Deleting blob " + firOut))
		o.Expect(out).To(o.ContainSubstring(secOut + ": keeping because of --keep-younger-than"))

		g.By("OCP-19196 is as below:")
		g.By("Create the app")
		err = oc.WithoutNamespace().AsAdmin().Run("new-app").Args("registry.redhat.io/ubi8/ruby-30:latest~https://github.com/sclorg/ruby-ex.git", "--as-deployment-config", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "ruby-ex-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs("ruby-ex", oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("deployment complete")
		err = exutil.WaitForDeploymentConfig(oc.KubeClient(), oc.AppsClient().AppsV1(), oc.Namespace(), "ruby-ex", 1, true, oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Edit dc, set invalid image as the value in 'image' field")
		err = oc.AsAdmin().Run("patch").Args("dc/ruby-ex", "-p", `{"spec":{"triggers":[{"type":"ConfigChange"}]}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchInfo := fmt.Sprintf("{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"image\":\"%v/%v/ruby-ex@sha256:nonono\",\"name\":\"ruby-ex\"}]}}}}", refRoute, oc.Namespace())
		err = oc.AsAdmin().Run("patch").Args("dc/ruby-ex", "-p", patchInfo, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Prune images without '--ignore-invalid-refs' options")
		pruneout, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--keep-tag-revisions=1", "--keep-younger-than=0", "--token="+token, "--registry-url="+refRoute, "--confirm", "--loglevel=5").SilentOutput()
		o.Expect(strings.Contains(pruneout, "invalid image reference")).To(o.BeTrue())
		if strings.Contains(pruneout, "invalid reference format - skipping") {
			e2e.Failf("Won't skip prune pod with invalid image without --ignore-invalid-refs")
		}

		g.By("Prune images with '--ignore-invalid-refs' options")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--keep-tag-revisions=1", "--keep-younger-than=0", "--token="+token, "--registry-url="+refRoute, "--ignore-invalid-refs", "--confirm", "--loglevel=4").SilentOutput()
		o.Expect(strings.Contains(out, "invalid reference format - skipping")).To(o.BeTrue())
	})

	//author: wewang@redhat.com
	g.It("ConnectedOnly-Author:wewang-Medium-54964-Hard pruning the registry should not lead to unexpected blob deletion [Disruptive]", func() {
		// When registry configured pvc or emptryDir, the replicas is 1 and with recreate pod policy.
		// This is not suitable for the defer recoverage. Only run this case on cloud storage.
		platforms := map[string]bool{
			"aws":          true,
			"azure":        true,
			"gcp":          true,
			"alibabacloud": true,
			"ibmcloud":     true,
		}
		if !platforms[exutil.CheckPlatform(oc)] {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Config image registry to emptydir")
		defer recoverRegistryStorageConfig(oc)
		defer recoverRegistryDefaultReplicas(oc)
		configureRegistryStorageToEmptyDir(oc)
		g.By("Create a imagestream using oci image")
		err := oc.AsAdmin().Run("tag").Args("quay.io/openshifttest/ociimage@sha256:d58e3e003ddec723dd14f72164beaa609d24c5e5e366579e23bc8b34b9a58324", "oci:latest", "--reference-policy=local", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForAnImageStreamTag(oc, oc.Namespace(), "oci", "latest")

		g.By("Create a pod to pull this image")
		err = oc.Run("set").Args("image-lookup", "oci", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		expectInfo := `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/` + oc.Namespace()
		createSimpleRunPod(oc, "oci", expectInfo)
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items..metadata.name}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Add the system:image-pruner role")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Log into registry pod")
		output, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/usr/bin/dockerregistry", "-prune=check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Would delete 0 blobs"))

		g.By("Delete pod and imagestream, then hard prune the registry")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod/"+podName, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod/"+podName, "-n", oc.Namespace()).Output()
		o.Expect(out).To(o.ContainSubstring("pods \"" + podName + "\" not found"))
		g.By("Log into registry pod")
		output, err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/usr/bin/dockerregistry", "-prune=check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Would delete 0 blobs"))

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("is/oci", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("is/oci", "-n", oc.Namespace()).Output()
		o.Expect(out).To(o.ContainSubstring("imagestreams.image.openshift.io \"oci\" not found"))
		g.By("Log into registry pod")
		output, err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/usr/bin/dockerregistry", "-prune=check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(oc.Namespace() + "/oci is not found, will remove the whole repository"))
	})

	g.It("Author:wewang-High-54905-Critical-54904-Critical-54051-Critical-54050-Critical-54171-Could import manifest lists via ImageStreamImport and sub-manifests did not be pruned when prune image [Serial]", func() {
		g.By("Create ImageStreamImport with docker multiarch image")
		var (
			isImportFile = filepath.Join(imageRegistryBaseDir, "imagestream-import-oci.yaml")
			isimportsrc  = isImportSource{
				namespace: "",
				name:      "",
				image:     "",
				policy:    "Source",
				mode:      "",
				template:  isImportFile,
			}
		)

		isarr := [3]string{"ociapp", "dockerapp", "simpleapp"}
		imagearr := [3]string{"quay.io/openshifttest/ociimage@sha256:d58e3e003ddec723dd14f72164beaa609d24c5e5e366579e23bc8b34b9a58324", "quay.io/openshifttest/registry-toomany-request@sha256:56b816ca086d714680235d0ee96320bc9b1375a8abd037839d17a8759961e842", "quay.io/openshifttest/ociimage-singlearch@sha256:93b3159f0a3a3b8f6ce46888adffb19d55779fd4038cbfece92650040acc034b"}
		shortimage := [3]string{"ociimage@", "registry-toomany-request@", "ociimage-singlearch"}
		num := [3]int{7, 5, 1}
		g.By("Get server host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		refRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		checkDnsCO(oc)
		waitRouteReady(refRoute)

		g.By("Prune the images")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", oc.Username()).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		isimportsrc.namespace = oc.Namespace()
		for i := 0; i < 2; i++ {
			isimportsrc.mode = "PreserveOriginal"
			isimportsrc.name = isarr[i]
			isimportsrc.image = imagearr[i]
			isimportsrc.create(oc)
			pruneImage(oc, isarr[i], shortimage[i], refRoute, token, num[i])
			isimportsrc.mode = "Legacy"
			isimportsrc.create(oc)
			importOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is/"+isarr[i], "-o=jsonpath={.spec.tags[0].importPolicy.importMode}", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(importOut).To(o.ContainSubstring("Legacy"))
			err = wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
				generationOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is/"+isarr[i], "-o=jsonpath={.status.tags[0].items[0].generation}", "-n", oc.Namespace()).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if generationOut != "2" {
					e2e.Logf("Continue to next round")
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "generation is not 2")
		}

		g.By("Create imagestream with ManifestList importMode for simple manifest for ocp-54171")
		isimportsrc.mode = "PreserveOriginal"
		isimportsrc.name = isarr[2]
		isimportsrc.image = imagearr[2]
		isimportsrc.create(oc)
		g.By("Check image object and no manifest list")
		isOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is/"+isarr[2], "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(isOut).To(o.ContainSubstring(isarr[2]))
		imageOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("images", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageCount := strings.Count(imageOut, shortimage[2])
		o.Expect(imageCount).To(o.Equal(num[2]))
	})

	g.It("Author:wewang-High-56210-High-56209-High-56366-image manifest list blobs can be deleted after hard prune [Serial]", func() {
		g.By("Create ImageStreamImport with docker multiarch image")
		var (
			isImportFile = filepath.Join(imageRegistryBaseDir, "imagestream-import-oci.yaml")
			isimportsrc  = isImportSource{
				namespace: "",
				name:      "",
				image:     "",
				policy:    "Local",
				mode:      "",
				template:  isImportFile,
			}
		)
		isarr := [2]string{"ociapp", "dockerapp"}
		imagearr := [2]string{"quay.io/openshifttest/ociimage@sha256:d58e3e003ddec723dd14f72164beaa609d24c5e5e366579e23bc8b34b9a58324", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f"}
		amdid := [2]string{"sha256:97923994fdc1c968eed6bdcb64be8e70d5356b88cfab0481cb6b73a4849361b7", "0415f56ccc05526f2af5a7ae8654baec97d4a614f24736e8eef41a4591f08019"}
		armid := [2]string{"sha256:bd0be70569d8b18321d7d3648d51925e22865df760c5379b69762f302cacd30d", "sha256:bf920ca7f146b802e1c9a8aab1fba3a3fe601c56b075ecef90834c13b90bb5bb"}
		isimportsrc.namespace = oc.Namespace()
		isimportsrc.mode = "PreserveOriginal"

		g.By("Add the system:image-pruner role")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get cluster architecture")
		// TODO[aleskandro,wewang]: this case needs a fix to be fully multi-arch compatible.
		// The arch-specific SHAs are hardcoded and that's not needed.
		// Moreover, we might rely on the pod's node architecture by default (not only for multi-arch clusters)
		curArchitecture := architecture.SkipArchitectures(oc, architecture.PPC64LE, architecture.S390X)

		g.By("Could pull a sub-manifest of manifest list via pullthrough and manifest list blobs can delete via hard prune")
		for i := 0; i < 2; i++ {
			shaarr := strings.Split(imagearr[i], "@")
			blobarr := strings.Split(imagearr[i], ":")
			isimportsrc.name = isarr[i]
			isimportsrc.image = imagearr[i]
			isimportsrc.create(oc)
			imagename := "image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/" + isarr[i] + ":latest"
			err = oc.AsAdmin().WithoutNamespace().Run("run").Args(isarr[i], "--image", imagename, `--overrides={"spec":{"securityContext":{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}}}`, "-n", oc.Namespace(), "--command", "--", "/bin/sleep", "300").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Check the pod is running")
			checkPodsRunningWithLabel(oc, oc.Namespace(), "run="+isarr[i], 1)

			g.By("Get architecture from node of pod for multi-arch cluster")
			if curArchitecture == architecture.MULTI {
				podnode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod/"+isarr[i], "-o=jsonpath={.spec.nodeName}", "-n", oc.Namespace()).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				podnodeArchitecture, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("node/"+podnode, "-o=jsonpath={.items[*].status.nodeInfo.architecture}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				curArchitecture = architecture.FromString(podnodeArchitecture)
			}

			g.By("Check the pod's image and image id")
			out, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod/"+isarr[i], "-n", oc.Namespace()).Output()
			switch curArchitecture {
			case architecture.AMD64:
				o.Expect(out).To(o.ContainSubstring(isarr[i] + ":latest"))
				o.Expect(out).To(o.ContainSubstring(amdid[i]))
			case architecture.ARM64:
				o.Expect(out).To(o.ContainSubstring(isarr[i] + ":latest"))
				o.Expect(out).To(o.ContainSubstring(armid[i]))
			default:
				e2e.Logf("only amd64 and arm64 are currently supported")
			}

			g.By("Delete pod and imagestream, then hard prune the registry")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod/"+isarr[i], "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			out, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod/"+isarr[i], "-n", oc.Namespace()).Output()
			o.Expect(out).To(o.ContainSubstring("not found"))
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("is/"+isarr[i], "-n", oc.Namespace()).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			out, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("is/"+isarr[i], "-n", oc.Namespace()).Output()
			o.Expect(out).To(o.ContainSubstring("not found"))
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("image/" + shaarr[1]).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			out, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("image/" + shaarr[1]).Output()
			o.Expect(out).To(o.ContainSubstring("not found"))

			g.By("Check manifest list deleted")
			output, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/usr/bin/dockerregistry", "-prune=delete").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("Deleting blob:"))
			o.Expect(output).To(o.ContainSubstring(blobarr[1]))
		}

	})

	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-Medium-11332-Admin can understand/manage image use and prune unreferenced image", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}

		g.By("Create imagestream")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", oc.Username()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/deployment-example@sha256:9d29ff0fdbbec33bb4eebb0dbe0d0f3860a856987e5481bb0fc39f3aba086184", "deployment-example:latest", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "deployment-example", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "images", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(string(output), oc.Namespace()+"/deployment-example (latest)") {
			e2e.Failf("Failed to get image")
		}
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output2, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "images", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(string(output2), oc.Namespace()+"/deployment-example (latest)") {
			e2e.Failf("The project info should be pruned")
		}
	})

	g.It("ROSA-OSD_CCS-ARO-Author:xiuwang-LEVEL0-Critical-12400-Prune images by command 'oc adm prune images' [Serial]", func() {
		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}
		g.By("Create imagestream")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", oc.Username()).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "soft-prune:latest", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "soft-prune", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get external registry host")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		checkDnsCO(oc)
		waitRouteReady(regRoute)

		manifestList := getManifestList(oc, "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", `""`)
		o.Expect(manifestList).NotTo(o.BeEmpty())
		e2e.Logf("print the manifest", manifestList)

		g.By("Could prune images with oc adm prune images")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("all", "--all", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		pruneout, pruneerr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--keep-younger-than=0", "--token="+token, "--registry-url="+regRoute, "--confirm").Output()
		o.Expect(pruneerr).NotTo(o.HaveOccurred())
		if !strings.Contains(pruneout, "Deleting blob "+manifestList) || !strings.Contains(pruneout, "Deleting image "+manifestList) {
			e2e.Failf("Failed to prune image")
		}
	})
})
