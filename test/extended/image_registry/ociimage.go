package imageregistry

import (
	"os"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("default-image-oci", exutil.KubeConfigPath())
		manifestType = "application/vnd.oci.image.manifest.v1+json"
	)
	g.BeforeEach(func() {
		if !checkOptionalOperatorInstalled(oc, "ImageRegistry") {
			g.Skip("Skip for the test due to image registry not installed")
		}
	})

	// author: wewang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:wewang-VMonly-ConnectedOnly-High-36291-OCI image is supported by API server and image registry", func() {
		if !checkOptionalOperatorInstalled(oc, "Build") {
			g.Skip("Skip for the test due to Build not installed")
		}
		var containerCLI = container.NewPodmanCLI()
		oc.SetupProject()
		g.By("Import an OCI image to internal registry")
		err := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("myimage", "--from", "quay.io/openshifttest/ociimage:multiarch", "--confirm", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Expose route of internal registry")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		imageInfo := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		_, err = containerCLI.Run("pull").Args(imageInfo, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Display oci image info")
		output, err := containerCLI.Run("inspect").Args(imageInfo).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, manifestType) {
			e2e.Failf("The internal registry image is not oci image")
		}
	})
})

var _ = g.Describe("[sig-imageregistry] Image_Registry Vmonly", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("default-image-oci-vm", exutil.KubeConfigPath())
	)
	// author: wewang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:wewang-ConnectedOnly-VMonly-High-37498-Push image with OCI format directly to the internal registry", func() {
		if !checkOptionalOperatorInstalled(oc, "Build") {
			g.Skip("Skip for the test due to Build not installed")
		}
		var podmanCLI = container.NewPodmanCLI()
		containerCLI := podmanCLI
		ociImage := "quay.io/openshifttest/ociimage:multiarch"

		g.By("Expose route of internal registry")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)
		oc.SetupProject()

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("podman tag an image")
		output, err := containerCLI.Run("pull").Args(ociImage, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}

		defer containerCLI.RemoveImage(ociImage)
		output, err = containerCLI.Run("tag").Args(ociImage, regRoute+"/"+oc.Namespace()+"/myimage:latest").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}

		g.By(" Push it with oci format")
		out := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		output, err = containerCLI.Run("push").Args("--format=oci", out, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}

		g.By("Check the manifest type")
		output, err = containerCLI.Run("inspect").Args(out).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}
		defer containerCLI.RemoveImage(out)
		o.Expect(output).To(o.ContainSubstring("application/vnd.oci.image.manifest.v1+json"))
	})

	// author: wewang@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:wewang-ConnectedOnly-VMonly-Critical-35998-OCI images layers configs can be pruned completely", func() {
		if !checkOptionalOperatorInstalled(oc, "Build") {
			g.Skip("Skip for the test due to Build not installed")
		}
		var podmanCLI = container.NewPodmanCLI()
		containerCLI := podmanCLI
		ociImage := "quay.io/openshifttest/ociimage:multiarch"

		g.By("Tag the image to internal registry")
		oc.SetupProject()
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("--import-mode=PreserveOriginal", "--source=docker", ociImage, "35998-image:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check if new imagestreamtag created")
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "35998-image", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Log into the default route")
		routeName := getRandomString()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("route", routeName, "-n", "openshift-image-registry").Execute()
		regRoute := exposeRouteFromSVC(oc, "reencrypt", "openshift-image-registry", routeName, "image-registry")
		waitRouteReady(regRoute)

		g.By("Save the external registry auth with the specific token")
		authFile, err := saveImageRegistryAuth(oc, "builder", regRoute, oc.Namespace())
		defer os.RemoveAll(authFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the file is %s", authFile)

		g.By("Pull internal image locally")
		imageInfo := regRoute + "/" + oc.Namespace() + "/35998-image:latest"
		output, err := containerCLI.Run("pull").Args(imageInfo, "--authfile="+authFile, "--tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}
		defer containerCLI.RemoveImage(imageInfo)

		g.By("Mark down the config/layer info of oci image")
		output, err = oc.WithoutNamespace().AsAdmin().Run("image").Args("info", ociImage, "--filter-by-os=linux/amd64").Output()

		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(output).To(o.ContainSubstring("application/vnd.oci.image.manifest.v1+json"))
		o.Expect(output).To(o.ContainSubstring("Layers"))
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", oc.Namespace(), "all", "--all").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Prune image")
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.WithoutNamespace().AsAdmin().Run("adm").Args("prune", "images", "--keep-tag-revisions=0", "--keep-younger-than=0m", "--token="+token).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Deleting layer link"))
		o.Expect(output).To(o.ContainSubstring("Deleting blob"))
		o.Expect(output).To(o.ContainSubstring("Deleting image"))
	})
})
