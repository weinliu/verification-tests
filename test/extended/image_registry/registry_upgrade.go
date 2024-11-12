package imageregistry

import (
	"context"
	"os"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("default-registry-upgrade", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		if !checkOptionalOperatorInstalled(oc, "ImageRegistry") {
			g.Skip("Skip for the test due to image registry not installed")
		}
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PreChkUpgrade-Author:wewang-High-26401-Upgrade cluster with insecureRegistries and blockedRegistries defined prepare [Disruptive]", func() {
		SkipDnsFailure(oc)
		var (
			ns = "26401-upgrade-ns"
			mc = machineConfig{
				name:     "",
				pool:     "worker",
				source:   "",
				path:     "",
				template: "",
			}
		)
		g.By("Create two registries")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		blockedRoute := setSecureRegistryWithoutAuth(oc, ns, "blockedreg", "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3", "5000")
		insecuredRoute := setSecureRegistryWithoutAuth(oc, ns, "insecuredreg", "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3", "5000")
		blockedImage := blockedRoute + "/" + ns + "/blockedimage:latest"
		insecuredImage := insecuredRoute + "/" + ns + "/insecuredimage:latest"

		g.By("Push image to insecured registries")
		checkDnsCO(oc)
		waitRouteReady(insecuredImage)
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", insecuredImage, "--insecure", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Without --insecure the imagestream will import fail")
		insecuredOut, _ := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("insecured-firstis:latest", "--from="+insecuredImage, "--confirm", "-n", ns).Output()
		o.Expect(insecuredOut).To(o.ContainSubstring("x509"))

		g.By("Add insecureRegistries and blockedRegistries to image.config")
		err = oc.AsAdmin().Run("patch").Args("images.config.openshift.io/cluster", "-p", `{"spec":{"registrySources":{"blockedRegistries": ["`+blockedRoute+`"],"insecureRegistries": ["`+insecuredRoute+`"]}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// image.conf.spec.registrySources will restart crio to apply change to every node
		// Need ensure master and worker update completed
		mc.waitForMCPComplete(oc)
		mc.pool = "master"
		mc.waitForMCPComplete(oc)

		g.By("Push images to two registries")
		checkDnsCO(oc)
		waitRouteReady(blockedImage)
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", blockedImage, "--insecure", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// worker restart, data is removed, need mirror again
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", insecuredImage, "--insecure", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Can't import image from blocked registry")
		blockedOut, _ := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("blocked-firstis:latest", "--from="+blockedImage, "--reference-policy=local", "--import-mode=PreserveOriginal", "--insecure", "--confirm", "-n", ns).Output()
		o.Expect(blockedOut).To(o.ContainSubstring(blockedRoute + " blocked"))

		g.By("Could import image from the insecured registry without --insecure")
		insecuredOut, insecuredErr := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("insecured-secondis:latest", "--from="+insecuredImage, "--import-mode=PreserveOriginal", "--confirm", "-n", ns).Output()
		o.Expect(insecuredErr).NotTo(o.HaveOccurred())
		o.Expect(insecuredOut).NotTo(o.ContainSubstring("x509"))
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:wewang-High-26401-Upgrade cluster with insecureRegistries and blockedRegistries defined after upgrade [Disruptive]", func() {
		g.By("Setup upgrade info")
		ns := "26401-upgrade-ns"
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("namespace", ns, "--ignore-not-found").Execute()
		blockedRoute, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "blockedreg", "-n", ns, "-o=jsonpath={.spec.host}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		insecuredRoute, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "insecuredreg", "-n", ns, "-o=jsonpath={.spec.host}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		blockedImage := blockedRoute + "/" + ns + "/blockedimage:latest"
		insecuredImage := insecuredRoute + "/" + ns + "/insecuredimage:latest"

		g.By("Push images to two registries")
		// After upgrade the reigstry pods restarted, the data should be lost
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", blockedImage, "--insecure", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", insecuredImage, "--insecure", "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Can't import image from blocked registry")
		blockedOut, _ := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("blocked-secondis:latest", "--from="+blockedImage, "--reference-policy=local", "--insecure", "--confirm", "--import-mode=PreserveOriginal", "-n", ns).Output()
		o.Expect(blockedOut).To(o.ContainSubstring(blockedRoute + " blocked"))

		g.By("Could import image from the insecured registry without --insecure")
		insecuredOut, insecuredErr := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("insecured-thirdis:latest", "--from="+insecuredImage, "--confirm", "--import-mode=PreserveOriginal", "-n", ns).Output()
		o.Expect(insecuredErr).NotTo(o.HaveOccurred())
		o.Expect(insecuredOut).NotTo(o.ContainSubstring("x509"))
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:wewang-High-41400- Users providing custom AWS tags are set with bucket creation after upgrade", func() {
		g.By("Check platforms")
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		g.By("Check the cluster is with resourceTags")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure.config.openshift.io", "-o=jsonpath={..status.platformStatus.aws}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "resourceTags") {
			g.Skip("Skip for no resourceTags")
		}
		g.By("Get bucket name")
		bucket, _ := oc.AsAdmin().Run("get").Args("config.image", "-o=jsonpath={..spec.storage.s3.bucket}").Output()
		o.Expect(bucket).NotTo(o.BeEmpty())

		g.By("Check the tags")
		aws := getAWSClient(oc)
		tag, err := awsGetBucketTagging(aws, bucket)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tag).To(o.ContainSubstring("customTag"))
		o.Expect(tag).To(o.ContainSubstring("installer-qe"))
	})

	// author: xiuwang@redhat.com
	g.It("NonPreRelease-PstChkUpgrade-Author:xiuwang-Medium-45346-Payload imagestream new tags should properly updated during cluster upgrade prepare", func() {
		g.By("Check payload imagestream")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("is", "-n", "openshift", "-l", "samples.operator.openshift.io/managed!=true", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		plimage := strings.Split(output, " ")
		for _, isname := range plimage {
			output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("is", isname, "-n", "openshift", "-o=jsonpath={.spec.tags[*].name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			tagname := strings.Split(output, " ")
			for _, tname := range tagname {
				e2e.Logf("tag is %s", tname)
				if tname == "" {
					e2e.Failf("The imagestream %s is broken after upgrade", isname)
				}
			}
		}
	})

	// author: xiuwang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PreChkUpgrade-Author:xiuwang-Critical-24345-Set proxy in Image-registry-operator before upgrade", func() {
		if !checkOptionalOperatorInstalled(oc, "Build") {
			g.Skip("Skip for the test due to Build not installed")
		}
		g.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, noProxy := saveGlobalProxy(oc)
		if !strings.Contains(httpProxy, "http") {
			g.Skip("Skip for non-proxy platform")
		}

		g.By("Check if registry operator degraded")
		registryDegrade := checkRegistryDegraded(oc)
		if registryDegrade {
			e2e.Failf("Image registry is degraded")
		}

		g.By("Set image-registry proxy setting")
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"proxy":{"http": "`+httpProxy+`","https":"`+httpsProxy+`","noProxy":"`+noProxy+`"}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			registryDegrade := checkRegistryDegraded(oc)
			if registryDegrade {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is degraded")
		oc.SetupProject()
		err = oc.AsAdmin().WithoutNamespace().Run("new-build").Args("-D", "FROM quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--to=prepare-24345", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "prepare-24345-1", nil, nil, nil)
		exutil.AssertWaitPollNoErr(err, "build is not complete")
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "prepare-24345", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		imagename := "image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/prepare-24345:latest"
		err = oc.AsAdmin().WithoutNamespace().Run("run").Args("prepare-24345", "--image", imagename, `--overrides={"spec":{"securityContext":{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}}}`, "-n", oc.Namespace(), "--command", "--", "/bin/sleep", "120").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		errWait := wait.Poll(30*time.Second, 6*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods(oc.Namespace()).List(context.Background(), metav1.ListOptions{LabelSelector: "run=prepare-24345"})
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					e2e.Logf("Continue to next round")
					return false, nil
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Pod can't be running")
	})

	// author: xiuwang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:xiuwang-Critical-24345-Set proxy in Image-registry-operator after upgrade", func() {
		g.By("Check if it's a proxy cluster")
		httpProxy, httpsProxy, noProxy := saveGlobalProxy(oc)
		if !strings.Contains(httpProxy, "http") {
			g.Skip("Skip for non-proxy platform")
		}

		g.By("Check if registry operator degraded")
		registryDegrade := checkRegistryDegraded(oc)
		if registryDegrade {
			e2e.Failf("Image registry is degraded")
		}

		g.By("Check image-registry proxy setting")
		Output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.proxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(Output, httpProxy) || !strings.Contains(Output, httpsProxy) || !strings.Contains(Output, noProxy) {
			e2e.Failf("http proxy is not same")
		}

		oc.SetupProject()
		checkRegistryFunctionFine(oc, "check-24345", oc.Namespace())
	})

	g.It("NonPreRelease-PreChkUpgrade-PstChkUpgrade-Author:xiuwang-Medium-22678-Check image-registry operator upgrade", func() {
		g.By("image-registry operator should be avaliable")
		expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err := waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-VMonly-PreChkUpgrade-Author:wewang-High-71533-Upgrade cluster with pull Internal Registry custom images successfully prepare[Disruptive]", func() {
		SkipDnsFailure(oc)
		var (
			podmanCLI       = container.NewPodmanCLI()
			ns              = "71533-upgrade-ns"
			expectedStatus1 = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		)
		containerCLI := podmanCLI
		oriImage := "quay.io/openshifttest/ociimage:multiarch"

		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("set default route of registry")
		defroute, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("route", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(defroute, "No resources found") {
			defer func() {
				restoreRouteExposeRegistry(oc)
				err := waitCoBecomes(oc, "image-registry", 60, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
				err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
				err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()

			createRouteExposeRegistry(oc)
			err = waitCoBecomes(oc, "image-registry", 60, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		regRoute := getRegistryDefaultRoute(oc)
		checkDnsCO(oc)
		waitRouteReady(regRoute)

		g.By("get authfile")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(authFile)

		g.By("podman tag an image")
		output, err := containerCLI.Run("pull").Args(oriImage, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}
		out := regRoute + "/" + ns + "/myimage:latest"
		output, err = containerCLI.Run("tag").Args(oriImage, out).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}

		g.By(" Push the image to internal registry")
		output, err = containerCLI.Run("push").Args(out, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(output).To(o.ContainSubstring("Writing manifest to image destination"))
		containerCLI.RemoveImage(out)

		g.By("Pull the image from internal registry")
		output, err = containerCLI.Run("pull").Args(out, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(output).To(o.ContainSubstring("Writing manifest to image destination"))

		g.By("Check the image is in local")
		defer containerCLI.RemoveImage(out)
		output, err = containerCLI.Run("images").Args("-n").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(strings.Replace(out, ":latest", "", -1)))
	})

	// author: wewang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-VMonly-PstChkUpgrade-Author:wewang-Critical-71533-Upgrade cluster with pull Internal Registry custom images successfully after upgrade[Disruptive]", func() {
		SkipDnsFailure(oc)
		var (
			podmanCLI       = container.NewPodmanCLI()
			ns              = "71533-upgrade-ns"
			expectedStatus1 = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		)
		_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), "71533-upgrade-ns", metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				g.Skip("The project 71533-upgrade-ns is not found, skip test")
			} else {
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("namespace", ns, "--ignore-not-found").Execute()
		containerCLI := podmanCLI

		g.By("set default route of registry")
		defroute, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("route", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(defroute, "No resources found") {
			defer func() {
				restoreRouteExposeRegistry(oc)
				err := waitCoBecomes(oc, "image-registry", 60, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
				err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
				err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus1)
				o.Expect(err).NotTo(o.HaveOccurred())
			}()

			createRouteExposeRegistry(oc)
			err = waitCoBecomes(oc, "image-registry", 60, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitCoBecomes(oc, "kube-apiserver", 600, expectedStatus1)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		regRoute := getRegistryDefaultRoute(oc)
		checkDnsCO(oc)
		waitRouteReady(regRoute)

		g.By("get authfile")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(authFile)

		g.By(" Pull existed images from internal registry")
		out := regRoute + "/" + ns + "/myimage:latest"
		output, err := containerCLI.Run("pull").Args(out, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}
		g.By("Check the image is in local")
		defer containerCLI.RemoveImage(out)
		output, err = containerCLI.Run("images").Args("-n").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(strings.Replace(out, ":latest", "", -1)))
	})
})
