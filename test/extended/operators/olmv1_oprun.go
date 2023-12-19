package operators

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	olmv1util "github.com/openshift/openshift-tests-private/test/extended/util/olmv1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM v1 oprun should", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("olmv1-oprun-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("OLMv1 is supported in TP only currently, so skip it")
		}
	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68903-BundleDeployment Health resource unhealthy pod api crd ds", func() {

		var (
			baseDir                   = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			unhealthyPod              = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-pod-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-podunhealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			unhealthyPodChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyApiservice = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-apis-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-apisunhealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			unhealthyApiserviceChild = []olmv1util.ChildResource{
				{Kind: "APIService", Ns: ""},
			}
			unhealthyCRD = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-crd-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-crdunhealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			unhealthyDS = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-ds-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-dsunhealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			unhealthyDSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
		)

		exutil.By("Create unhealthy pod")
		defer unhealthyPod.DeleteWithoutCheck(oc)
		unhealthyPod.CreateWithoutCheck(oc)
		unhealthyPod.AssertHealthyWithConsistent(oc, "false")
		unhealthyPod.Delete(oc, unhealthyPodChild)

		exutil.By("Create unhealthy APIService")
		defer unhealthyApiservice.DeleteWithoutCheck(oc)
		unhealthyApiservice.CreateWithoutCheck(oc)
		unhealthyApiservice.AssertHealthyWithConsistent(oc, "false")
		unhealthyApiservice.Delete(oc, unhealthyApiserviceChild)

		exutil.By("Create unhealthy CRD")
		defer unhealthyCRD.DeleteWithoutCheck(oc)
		unhealthyCRD.CreateWithoutCheck(oc)
		unhealthyCRD.AssertHealthyWithConsistent(oc, "false")
		unhealthyCRD.DeleteWithoutCheck(oc)

		exutil.By("Create unhealthy DS")
		defer unhealthyDS.DeleteWithoutCheck(oc)
		unhealthyDS.CreateWithoutCheck(oc)
		unhealthyDS.AssertHealthyWithConsistent(oc, "false")
		unhealthyDS.Delete(oc, unhealthyDSChild)

	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68936-BundleDeployment Health resource healthy and install fail", func() {

		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate    = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			basicBdRegistryImageTemplate = filepath.Join(baseDir, "basic-bd-registry-image.yaml")
			healthBd                     = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-healthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-healthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			healthChild = []olmv1util.ChildResource{
				{Kind: "CustomResourceDefinition", Ns: ""},
				{Kind: "pod", Ns: "olmv1-68903-healthy"},
				{Kind: "APIService", Ns: ""},
				{Kind: "namespace", Ns: ""},
			}
			unhealthyDp = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-deployment-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:registry-68903-deployunhealthy",
				ActiveBundle: "",
				Template:     basicBdRegistryImageTemplate,
			}
			unhealthyDpChild = []olmv1util.ChildResource{
				{Kind: "CustomResourceDefinition", Ns: ""},
				{Kind: "namespace", Ns: ""},
			}
			unhealthyRC = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-rc-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-rcunhealth",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			unhealthyRCChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyInstall = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-install-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-installunhealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
		)

		exutil.By("Create health bundledeployment")
		defer healthBd.DeleteWithoutCheck(oc)
		healthBd.Create(oc)
		healthBd.Delete(oc, healthChild)

		exutil.By("Create unhealthy deployment")
		defer unhealthyDp.DeleteWithoutCheck(oc)
		unhealthyDp.CreateWithoutCheck(oc)
		unhealthyDp.AssertHealthyWithConsistent(oc, "false")
		unhealthyDp.Delete(oc, unhealthyDpChild)

		exutil.By("Create unhealthy RC")
		defer unhealthyRC.DeleteWithoutCheck(oc)
		unhealthyRC.CreateWithoutCheck(oc)
		unhealthyRC.AssertHealthy(oc, "true") // here is possible issue
		unhealthyRC.Delete(oc, unhealthyRCChild)

		exutil.By("install fails")
		defer unhealthyInstall.DeleteWithoutCheck(oc)
		unhealthyInstall.CreateWithoutCheck(oc)
		unhealthyInstall.AssertHealthyWithConsistent(oc, "false")
		unhealthyInstall.DeleteWithoutCheck(oc)

	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68937-BundleDeployment Health resource unhealthy ss rs unspport", func() {

		var (
			baseDir                   = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			unhealthySS               = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-ss-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-ssunhealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			unhealthySSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyRS = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-rs-unhealthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-rsunhealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			unhealthyRSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}

			healthUnspport = olmv1util.BundleDeploymentDescription{
				BdName:       "68903-unspport-healthy",
				Address:      "quay.io/olmqe/olmv1bundle:plain-68903-unsupporthealthy",
				ActiveBundle: "",
				Template:     basicBdPlainImageTemplate,
			}
			healthUnspportChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
		)

		exutil.By("Create unhealthy SS")
		defer unhealthySS.DeleteWithoutCheck(oc)
		unhealthySS.CreateWithoutCheck(oc)
		unhealthySS.AssertHealthyWithConsistent(oc, "false")
		unhealthySS.Delete(oc, unhealthySSChild)

		exutil.By("Create unhealthy RS")
		defer unhealthyRS.DeleteWithoutCheck(oc)
		unhealthyRS.CreateWithoutCheck(oc)
		unhealthyRS.AssertHealthyWithConsistent(oc, "false")
		unhealthyRS.Delete(oc, unhealthyRSChild)

		exutil.By("unsupport health")
		defer healthUnspport.DeleteWithoutCheck(oc)
		healthUnspport.CreateWithoutCheck(oc)
		healthUnspport.AssertHealthy(oc, "true")
		healthUnspport.Delete(oc, healthUnspportChild)

	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-High-68821-OLMv1 Supports Version Ranges during Installation", func() {
		var (
			baseDir                               = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate                       = filepath.Join(baseDir, "catalog.yaml")
			operatorTemplate                      = filepath.Join(baseDir, "operator.yaml")
			operatorWithoutChannelTemplate        = filepath.Join(baseDir, "operatorWithoutChannel.yaml")
			operatorWithoutChannelVersionTemplate = filepath.Join(baseDir, "operatorWithoutChannelVersion.yaml")
			catalog                               = olmv1util.CatalogDescription{
				Name:     "catalog-68821",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm68821",
				Template: catalogTemplate,
			}
			operator = olmv1util.OperatorDescription{
				Name:        "operator-68821",
				PackageName: "nginx68821",
				Channel:     "candidate-v0.0",
				Version:     ">=0.0.1",
				Template:    operatorTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Create operator with channel candidate-v0.0, version >=0.0.1")
		defer operator.Delete(oc)
		operator.Create(oc)
		o.Expect(operator.ResolvedBundleResource).To(o.ContainSubstring("v0.0.3"))
		operator.Delete(oc)

		exutil.By("Create operator with channel candidate-v1.0, version 1.0.x")
		operator.Channel = "candidate-v1.0"
		operator.Version = "1.0.x"
		operator.Create(oc)
		o.Expect(operator.ResolvedBundleResource).To(o.ContainSubstring("v1.0.2"))
		operator.Delete(oc)

		exutil.By("Create operator with channel empty, version >=0.0.1 !=1.1.0 <1.1.2")
		operator.Channel = ""
		operator.Version = ">=0.0.1 !=1.1.0 <1.1.2"
		operator.Template = operatorWithoutChannelTemplate
		operator.Create(oc)
		o.Expect(operator.ResolvedBundleResource).To(o.ContainSubstring("v1.0.2"))
		operator.Delete(oc)

		exutil.By("Create operator with channel empty, version empty")
		operator.Channel = ""
		operator.Version = ""
		operator.Template = operatorWithoutChannelVersionTemplate
		operator.Create(oc)
		o.Expect(operator.ResolvedBundleResource).To(o.ContainSubstring("v1.1.0"))
		operator.Delete(oc)

		exutil.By("Create operator with invalid version")
		operator.Version = "!1.0.1"
		operator.Template = operatorTemplate
		err := operator.CreateWithoutCheck(oc)
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-69196-OLMv1 Supports Version Ranges during operator upgrade", func() {
		var (
			baseDir          = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate  = filepath.Join(baseDir, "catalog.yaml")
			operatorTemplate = filepath.Join(baseDir, "operator.yaml")
			catalog          = olmv1util.CatalogDescription{
				Name:     "catalog-69196",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69196",
				Template: catalogTemplate,
			}
			operator = olmv1util.OperatorDescription{
				Name:        "operator-69196",
				PackageName: "nginx69196",
				Channel:     "candidate-v1.0",
				Version:     "1.0.1",
				Template:    operatorTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Create operator with channel candidate-v1.0, version 1.0.1")
		defer operator.Delete(oc)
		operator.Create(oc)
		o.Expect(operator.InstalledBundleResource).To(o.ContainSubstring("v1.0.1"))

		exutil.By("update version to be >=1.0.1")
		operator.Patch(oc, `{"spec":{"version":">=1.0.1"}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundleResource, _ := olmv1util.GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o", "jsonpath={.status.resolvedBundleResource}")
			if !strings.Contains(resolvedBundleResource, "v1.0.2") {
				e2e.Logf("operator.resolvedBundleResource is %s, not v1.0.2, and try next", resolvedBundleResource)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "operator resolvedBundleResource is not v1.0.2")
		}

		exutil.By("update channel to be candidate-v1.1")
		operator.Patch(oc, `{"spec":{"channel":"candidate-v1.1"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundleResource, _ := olmv1util.GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o", "jsonpath={.status.resolvedBundleResource}")
			if !strings.Contains(resolvedBundleResource, "v1.1.0") {
				e2e.Logf("operator.resolvedBundleResource is %s, not v1.1.0, and try next", resolvedBundleResource)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "operator.operators.operatorframework.io", operator.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "operator resolvedBundleResource is not v1.1.0")
		}
	})

})
