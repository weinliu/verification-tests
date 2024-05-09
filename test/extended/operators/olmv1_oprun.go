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
		oc = exutil.NewCLIWithoutNamespace("default")
		// we need to check if it is TP in BeforeEach before every case. if we use exutil.NewCLI("olmv1-oprun-"+getRandomString(), exutil.KubeConfigPath())
		// it will create temp project, but it will fail sometime on SNO cluster because of system issue.
		// so, we use exutil.NewCLIWithoutNamespace("default") not to create temp project to get oc client to check if it is TP.
		// if it need temp project, could use oc.SetupProject() in g.It to create it firstly.
	)

	g.BeforeEach(func() {
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("OLMv1 is supported in TP only currently, so skip it")
		}
	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68903-BundleDeployment Health resource unhealthy pod api crd ds", func() {
		// oc.SetupProject() // it is example if the case need temp project. here it does not need it, so comment it.

		var (
			baseDir                   = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			unhealthyPod              = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-pod-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-podunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyPodChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyApiservice = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-apis-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-apisunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyApiserviceChild = []olmv1util.ChildResource{
				{Kind: "APIService", Ns: ""},
			}
			unhealthyCRD = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-crd-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-crdunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyDS = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-ds-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-dsunhealthy",
				Template: basicBdPlainImageTemplate,
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
				BdName:   "68903-healthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-healthy",
				Template: basicBdPlainImageTemplate,
			}
			healthChild = []olmv1util.ChildResource{
				{Kind: "CustomResourceDefinition", Ns: ""},
				{Kind: "pod", Ns: "olmv1-68903-healthy"},
				{Kind: "APIService", Ns: ""},
				{Kind: "namespace", Ns: ""},
			}
			unhealthyDp = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-deployment-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:registry-68903-deployunhealthy",
				Template: basicBdRegistryImageTemplate,
			}
			unhealthyDpChild = []olmv1util.ChildResource{
				{Kind: "CustomResourceDefinition", Ns: ""},
				{Kind: "namespace", Ns: ""},
			}
			unhealthyRC = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-rc-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-rcunhealth",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyRCChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyInstall = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-install-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-installunhealthy",
				Template: basicBdPlainImageTemplate,
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
				BdName:   "68903-ss-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-ssunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthySSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyRS = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-rs-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-rsunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyRSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}

			healthUnspport = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-unspport-healthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-unsupporthealthy",
				Template: basicBdPlainImageTemplate,
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
			baseDir                                       = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate                               = filepath.Join(baseDir, "catalog.yaml")
			clusterextensionTemplate                      = filepath.Join(baseDir, "clusterextension.yaml")
			clusterextensionWithoutChannelTemplate        = filepath.Join(baseDir, "clusterextensionWithoutChannel.yaml")
			clusterextensionWithoutChannelVersionTemplate = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			catalog                                       = olmv1util.CatalogDescription{
				Name:     "catalog-68821",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm68821",
				Template: catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-68821",
				PackageName: "nginx68821",
				Channel:     "candidate-v0.0",
				Version:     ">=0.0.1",
				Template:    clusterextensionTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v0.0, version >=0.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("v0.0.3"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel candidate-v1.0, version 1.0.x")
		clusterextension.Channel = "candidate-v1.0"
		clusterextension.Version = "1.0.x"
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("v1.0.2"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel empty, version >=0.0.1 !=1.1.0 <1.1.2")
		clusterextension.Channel = ""
		clusterextension.Version = ">=0.0.1 !=1.1.0 <1.1.2"
		clusterextension.Template = clusterextensionWithoutChannelTemplate
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("v1.0.2"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel empty, version empty")
		clusterextension.Channel = ""
		clusterextension.Version = ""
		clusterextension.Template = clusterextensionWithoutChannelVersionTemplate
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("v1.1.0"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with invalid version")
		clusterextension.Version = "!1.0.1"
		clusterextension.Template = clusterextensionTemplate
		err := clusterextension.CreateWithoutCheck(oc)
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-69196-OLMv1 Supports Version Ranges during clusterextension upgrade", func() {
		var (
			baseDir                  = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate          = filepath.Join(baseDir, "catalog.yaml")
			clusterextensionTemplate = filepath.Join(baseDir, "clusterextension.yaml")
			catalog                  = olmv1util.CatalogDescription{
				Name:     "catalog-69196",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69196",
				Template: catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-69196",
				PackageName: "nginx69196",
				Channel:     "candidate-v1.0",
				Version:     "1.0.1",
				Template:    clusterextensionTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v1.0, version 1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("update version to be >=1.0.1")
		clusterextension.Patch(oc, `{"spec":{"version":">=1.0.1"}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundle, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.resolvedBundle}")
			if !strings.Contains(resolvedBundle, "v1.0.2") {
				e2e.Logf("clusterextension.resolvedBundle is %s, not v1.0.2, and try next", resolvedBundle)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension resolvedBundle is not v1.0.2")
		}

		exutil.By("update channel to be candidate-v1.1")
		clusterextension.Patch(oc, `{"spec":{"channel":"candidate-v1.1"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundle, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.resolvedBundle}")
			if !strings.Contains(resolvedBundle, "v1.1.0") {
				e2e.Logf("clusterextension.resolvedBundle is %s, not v1.1.0, and try next", resolvedBundle)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextensiono", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension resolvedBundle is not v1.1.0")
		}
	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-High-69193-OLMv1 major version zero", func() {
		var (
			baseDir                  = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate          = filepath.Join(baseDir, "catalog.yaml")
			clusterextensionTemplate = filepath.Join(baseDir, "clusterextension.yaml")
			catalog                  = olmv1util.CatalogDescription{
				Name:     "catalog-69193",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm69193",
				Template: catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-69193",
				PackageName: "nginx69193",
				Channel:     "candidate-v0.0",
				Version:     "0.0.1",
				Template:    clusterextensionTemplate,
			}
		)
		exutil.By("1) Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("2) Install version 0.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("0.0.1"))

		exutil.By("3) Attempt to update to version 0.0.2 with Enforce policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"version":"0.0.2"}}`)
		/*
			errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
				message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
				if !strings.Contains(message, "constraints not satisfiable") {
					e2e.Logf("status is %s", message)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.0.2 should not be installed")

			exutil.By("4) change UpgradeConstraintPolicy to be Ignore, that should work")
			clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"Ignore"}}`)*/
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "0.0.2") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.0.2 is not installed")

		clusterextension.Delete(oc)
		err := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 120*time.Second, false, func(ctx context.Context) (bool, error) {
			catsrcStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "nginx69193-system").Output()
			if strings.Contains(catsrcStatus, "NotFound") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "ns nginx69193-system is not deleted")

		exutil.By("5) Install version 0.1.0 with Enforce policy, that should work")
		clusterextension.Channel = "candidate-v0.1"
		clusterextension.Version = "0.1.0"
		clusterextension.UpgradeConstraintPolicy = "Enforce"
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("0.1.0"))

		exutil.By("6) Attempt to update to version 0.2.0 with Enforce policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"version":"0.2.0","channel":"candidate-v0.2"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "constraints not satisfiable") {
				e2e.Logf("status is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.2.0 should not be installed")

		exutil.By("7) Install version 0.2.0 with Ignore policy, that should work")
		clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"Ignore"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "0.2.0") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.2.0 is not installed")

		exutil.By("8) Install version 0.2.2 with Enforce policy, that should work")
		clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"Enforce"}}`)
		clusterextension.Patch(oc, `{"spec":{"version":"0.2.2"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "0.2.2") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx69193 0.2.2 is not installed")

	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-High-70719-OLMv1 Upgrade non-zero major version	", func() {
		var (
			baseDir                  = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate          = filepath.Join(baseDir, "catalog.yaml")
			clusterextensionTemplate = filepath.Join(baseDir, "clusterextension.yaml")
			catalog                  = olmv1util.CatalogDescription{
				Name:     "catalog-70719",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm70719",
				Template: catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-70719",
				PackageName: "nginx70719",
				Channel:     "candidate-v0",
				Version:     "0.2.2",
				Template:    clusterextensionTemplate,
			}
		)
		exutil.By("1) Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("2) Install version 0.2.2")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("0.2.2"))

		exutil.By("3) Attempt to update to version 1.0.0 with Enforce policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"channel":"candidate-v1", "version":"1.0.0"}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "constraints not satisfiable") {
				e2e.Logf("status is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.0.0 should not be installed")

		exutil.By("4) change UpgradeConstraintPolicy to be Ignore, that should work")
		clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"Ignore"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "1.0.0") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.0.0 is not installed")

		exutil.By("5) change UpgradeConstraintPolicy to be Enforce, attempt to update to version 1.0.1, that should work")
		clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"Enforce"}}`)
		clusterextension.Patch(oc, `{"spec":{"version":"1.0.1"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "1.0.1") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.0.1 is not installed")

		exutil.By("6) attempt to update to version 1.2.1, that should work")
		clusterextension.Patch(oc, `{"spec":{"version":"1.2.1"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "1.2.1") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 1.2.1 is not installed")

		exutil.By("7) Attempt to update to version 2.0.0 with Enforce policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"version":"2.0.0"}}`)
		/*
			errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
				message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
				if !strings.Contains(message, "installed package nginx70719 requires at least one of") {
					e2e.Logf("status is %s", message)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(errWait, "nginx70719 2.0.0 should not be installed")

			exutil.By("8) Install version 2.0.0 with Ignore policy, that should work")
			clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"Ignore"}}`)*/
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "2.0.0") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70719 2.0.0 is not installed")

	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-High-70723-OLMv1 downgrade version", func() {
		var (
			baseDir                  = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate          = filepath.Join(baseDir, "catalog.yaml")
			clusterextensionTemplate = filepath.Join(baseDir, "clusterextension.yaml")
			catalog                  = olmv1util.CatalogDescription{
				Name:     "catalog-70723",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm70723",
				Template: catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-70723",
				PackageName: "nginx70723",
				Channel:     "candidate-v2",
				Version:     "2.2.1",
				Template:    clusterextensionTemplate,
			}
		)
		exutil.By("1) Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("2) Install version 2.2.1")
		clusterextension.Create(oc)
		defer clusterextension.Delete(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("2.2.1"))

		exutil.By("3) Attempt to downgrade to version 2.0.0 with Enforce policy, that should fail")
		clusterextension.Patch(oc, `{"spec":{"version":"2.0.0"}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			message, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.conditions[*].message}")
			if !strings.Contains(message, "constraints not satisfiable") {
				e2e.Logf("message is %s", message)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70723 2.0.0 should not be installed")

		exutil.By("4) change UpgradeConstraintPolicy to be Ignore, that should work")
		clusterextension.Patch(oc, `{"spec":{"upgradeConstraintPolicy":"Ignore"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			clusterextension.GetBundleResource(oc)
			if !strings.Contains(clusterextension.ResolvedBundle, "2.0.0") {
				e2e.Logf("ResolvedBundle is %s", clusterextension.ResolvedBundle)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "nginx70723 2.0.0 is not installed")
	})

})
