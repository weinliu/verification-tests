package operators

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	olmv1util "github.com/openshift/openshift-tests-private/test/extended/util/olmv1"
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
