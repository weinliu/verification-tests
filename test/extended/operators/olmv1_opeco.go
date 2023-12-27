package operators

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	olmv1util "github.com/openshift/openshift-tests-private/test/extended/util/olmv1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM v1 opeco should", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("olmv1-opeco"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("OLMv1 is supported in TP only currently, so skip it")
		}
	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-VMonly-Author:jitli-High-69758-Catalogd Polling remote registries for update to images content", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			quayCLI         = container.NewQuayCLI()
			imagev1         = "quay.io/olmqe/olmtest-operator-index:nginxolm69758v1"
			imagev2         = "quay.io/olmqe/olmtest-operator-index:nginxolm69758v2"

			catalog = olmv1util.CatalogDescription{
				Name:     "catalog-69758",
				Imageref: "quay.io/olmqe/olmtest-operator-index:test69758",
				Template: catalogTemplate,
			}
		)

		exutil.By("Get v1 v2 digestID")
		manifestDigestv1, err := quayCLI.GetImageDigest(strings.Replace(imagev1, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(manifestDigestv1).NotTo(o.BeEmpty())
		manifestDigestv2, err := quayCLI.GetImageDigest(strings.Replace(imagev2, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(manifestDigestv2).NotTo(o.BeEmpty())

		exutil.By("Check default digestID is v1")
		indexImageDigest, err := quayCLI.GetImageDigest(strings.Replace(catalog.Imageref, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(indexImageDigest).NotTo(o.BeEmpty())
		if indexImageDigest != manifestDigestv1 {
			//tag v1 to testrun image
			tagResult, tagErr := quayCLI.ChangeTag(strings.Replace(catalog.Imageref, "quay.io/", "", 1), manifestDigestv1)
			if !tagResult {
				e2e.Logf("Error: %v", tagErr)
				e2e.Failf("Change tag failed on quay.io")
			}
			e2e.Logf("Successful init tag v1")
		}

		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Add image pollInterval time")
		err = oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"pollInterval":"20s"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		pollInterval, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.spec.source.image.pollInterval}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(pollInterval)).To(o.ContainSubstring("20s"))
		catalog.WaitCatalogStatus(oc, "Unpacked", 0)

		exutil.By("Collect the initial image status information")
		lastPollAttempt, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.lastPollAttempt}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		v1bundlesDataOut, err := catalog.UnmarshalContent(oc, "bundle")
		o.Expect(err).NotTo(o.HaveOccurred())
		v1bundlesImage := olmv1util.GetBundlesImageTag(v1bundlesDataOut.Bundles)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Update the image and check for changes")
		//tag v2 to testrun image
		tagResult, tagErr := quayCLI.ChangeTag(strings.Replace(catalog.Imageref, "quay.io/", "", 1), manifestDigestv2)
		if !tagResult {
			e2e.Logf("Error: %v", tagErr)
			e2e.Failf("Change tag failed on quay.io")
		}
		e2e.Logf("Successful tag v2")

		errWait := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 90*time.Second, false, func(ctx context.Context) (bool, error) {
			lastPollAttempt2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.lastPollAttempt}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			resolvedRef2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			if lastPollAttempt == lastPollAttempt2 || resolvedRef == resolvedRef2 {
				e2e.Logf("lastPollAttempt:%v,lastPollAttempt2:%v", lastPollAttempt, lastPollAttempt2)
				e2e.Logf("resolvedRef:%v,resolvedRef2:%v", resolvedRef, resolvedRef2)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Error lastPollAttempt or resolvedRef are same")

		exutil.By("check the index content changes")
		v2bundlesDataOut, err := catalog.UnmarshalContent(oc, "bundle")
		o.Expect(err).NotTo(o.HaveOccurred())
		v2bundlesImage := olmv1util.GetBundlesImageTag(v2bundlesDataOut.Bundles)
		o.Expect(err).NotTo(o.HaveOccurred())

		if reflect.DeepEqual(v1bundlesImage, v2bundlesImage) {
			e2e.Logf("v1bundlesImage%v, v2bundlesImage%v", v1bundlesImage, v2bundlesImage)
			e2e.Failf("Failed, The index content no changes")
		}
		e2e.Logf("v1bundlesImage%v, v2bundlesImage%v", v1bundlesImage, v2bundlesImage)

		exutil.By("Update use the digest image and check it")
		output, err := oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index@`+manifestDigestv1+`"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("cannot specify PollInterval while using digest-based image"))

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69123-Catalogd catalog offer the operator content through http server", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			catalog         = olmv1util.CatalogDescription{
				Name:     "catalog-69123",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69123",
				Template: catalogTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("get the index content through http service on cluster")
		curlOutput := catalog.GetContent(oc)
		o.Expect(strings.Contains(string(curlOutput), "\"name\":\"nginx69123\"")).To(o.BeTrue())

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69242-Catalogd deprecated package/bundlemetadata/catalogmetadata from catalog CR", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			catalog         = olmv1util.CatalogDescription{
				Name:     "catalog-69242",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69242",
				Template: catalogTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("get the old related crd package/bundlemetadata/bundledeployment")
		packageOutput, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("package").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(packageOutput)).To(o.ContainSubstring("error: the server doesn't have a resource type \"package\""))

		bundlemetadata, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("bundlemetadata").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(bundlemetadata)).To(o.ContainSubstring("error: the server doesn't have a resource type \"bundlemetadata\""))

		catalogmetadata, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalogmetadata").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(catalogmetadata)).To(o.ContainSubstring("error: the server doesn't have a resource type \"catalogmetadata\""))

	})

})
