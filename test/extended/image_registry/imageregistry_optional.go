package imageregistry

import (
	"encoding/base64"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("imageregistry-optional", exutil.KubeConfigPath())

	g.It("NonHyperShiftHOST-Author:xiuwang-High-66514-Disable Image Registry operator", func() {
		if checkOptionalOperatorInstalled(oc, "ImageRegistry") {
			g.Skip("Skip for the test when registry is installed")
		}

		g.By("All registry related resources are not installed")
		var object imageObject
		resources := map[string]string{
			"co/image-registry":           `clusteroperators.config.openshift.io "image-registry" not found`,
			"config.image/cluster":        `configs.imageregistry.operator.openshift.io "cluster" not found`,
			"ns/openshift-image-registry": `namespaces "openshift-image-registry" not found`,
			"imagepruner/cluster":         `imagepruners.imageregistry.operator.openshift.io "cluster" not found`,
		}
		for key := range resources {
			output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args(key).Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring(resources[key]))
		}
		g.By("Created imagestream only could be source policy")
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "66514:latest", "--import-mode=PreserveOriginal", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "66514", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		object.getManifestObject(oc, "imagestreamtag", "66514:latest", oc.Namespace())
		if len(object.architecture) == 1 && len(object.digest) == 1 {
			e2e.Failf("Only one arch image imported")
		}
		imageRepo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is", "66514", "-n", oc.Namespace(), "-ojsonpath={.status.dockerImageRepository}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(imageRepo).To(o.BeEmpty())

		g.By("Check additional ca could be merged by mco")
		trustCAName, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("image.config", "-o=jsonpath={..spec.additionalTrustedCA.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if trustCAName != "" {
			g.By("additionalTrustedCA is set")
			addTrustCA, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", trustCAName, "-o=jsonpath={.data}", "-n", "openshift-config").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			addTrustCA = strings.TrimSuffix(string(addTrustCA), "}")
			addTrustCA = strings.TrimPrefix(string(addTrustCA), "{")
			regCerts, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", "merged-trusted-image-registry-ca", "-o=jsonpath={.data}", "-n", "openshift-config-managed").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(regCerts, addTrustCA) {
				e2e.Failf("The additional ca can't be merged by mco")
			}
		}
	})

	g.It("NonHyperShiftHOST-Author:xiuwang-High-66516-Image registry pruner job should pass when cluster was installed without DeploymentConfig capability [Serial]", func() {
		if checkOptionalOperatorInstalled(oc, "DeploymentConfig") || !checkOptionalOperatorInstalled(oc, "ImageRegistry") {
			g.Skip("Skip for the test when DeploymentConfig is installed or ImageRegistry is not installed")
		}
		g.By("Set imagepruner cronjob started every 1 minutes")
		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err := waitCoBecomes(oc, "image-registry", 240, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":""}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("imagepruner/cluster", "-p", `{"spec":{"schedule":"* * * * *"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		imagePruneLog(oc, "", "the server could not find the requested resource (get deploymentconfigs.apps.openshift.io)")
	})

	g.It("NonHyperShiftHOST-Author:wewang-High-66667-Install Image Registry operator after cluster install successfully", func() {
		if !checkOptionalOperatorInstalled(oc, "Build") {
			g.Skip("Skip for the test due to Build not installed")
		}
		g.By("Check image registry operator installed optionally")
		if !checkOptionalOperatorInstalled(oc, "ImageRegistry") {
			g.Skip("Skip for the test due to image registry not installed")
		} else {
			baseCapSet, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].spec.capabilities.baselineCapabilitySet}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if baseCapSet != "None" {
				g.Skip("Skip for the test when registry is installed in default")
			}
		}

		g.By("Check serviceaccount and pull secret created after the project created")
		oc.SetupProject()
		secretName, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("sa/builder", "-o=jsonpath={.imagePullSecrets[0].name}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dockerCfg, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("secret/"+secretName, "-o=jsonpath={.data.\\.dockercfg}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		credsTXT, err := base64.StdEncoding.DecodeString(dockerCfg)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(credsTXT), "image-registry.openshift-image-registry.svc:5000")).To(o.BeTrue())

		g.By("Create a imagestream with pullthrough")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("registry.redhat.io/ubi8/httpd-24:latest", "httpd:latest", "--reference-policy=local", "--insecure", "--import-mode=PreserveOriginal", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, oc.Namespace(), "httpd", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("deployment", "pull-test", "--image", "image-registry.openshift-image-registry.svc:5000/"+oc.Namespace()+"/httpd:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=pull-test", 1)
	})
})
