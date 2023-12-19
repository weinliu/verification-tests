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

var _ = g.Describe("[sig-operators] OLM v1 should", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("olmv1-"+getRandomString(), exutil.KubeConfigPath())
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

	// var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// For now, for 4.15, OLM removes the Package and CatalogMetadata resources,
	// details: https://github.com/operator-framework/catalogd/pull/149 and https://github.com/operator-framework/catalogd/pull/169
	// // author: jiazha@redhat.com
	// g.It("NonHyperShiftHOST-ConnectedOnly-Author:jiazha-High-68407-operator version pinning and pivoting based on OLMv1", func() {
	// 	// By now, OLMv1 is TP, need to check if the featuregate is enabled
	// 	featureSet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("featuregate", "cluster", "-o=jsonpath={.spec.featureSet}").Output()
	// 	if err != nil {
	// 		e2e.Failf("Fail to get the featureSet: %s, error:%v", featureSet, err)
	// 	}
	// 	// skip it if featureSet is empty
	// 	if featureSet == "" {
	// 		g.Skip("featureSet is empty, skip it")
	// 	}
	// 	// The FeatureGate "cluster" is invalid: spec.featureSet: Forbidden: once enabled, custom feature gates may not be disabled
	// 	if featureSet != "" && featureSet != "TechPreviewNoUpgrade" {
	// 		g.Skip(fmt.Sprintf("featureSet is not TechPreviewNoUpgrade, but %s", featureSet))
	// 	}

	// 	exutil.By("1, check the catalog")
	// 	olmBaseDir := exutil.FixturePath("testdata", "olm")
	// 	redhatOperators, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalog", "redhat-operators").Output()
	// 	if err != nil {
	// 		if strings.Contains(redhatOperators, "not found") {
	// 			// create it
	// 			exutil.By("1-1, create the catalog")
	// 			catalogTemplate := filepath.Join(olmBaseDir, "catalog.yaml")
	// 			ocpVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.desired.version}").Output()
	// 			if err != nil {
	// 				e2e.Failf("Failed to get the OCP version: %s", err)
	// 			}
	// 			re, _ := regexp.Compile(`\d\.\d{2}`)
	// 			ocpVersion = re.FindString(ocpVersion)
	// 			indexImage := fmt.Sprintf("registry.redhat.io/redhat/redhat-operator-index:v%s", ocpVersion)
	// 			//ToDo: this redhat-operators catalog is a precondition for the following test,
	// 			// and to save the creating/deleting costs, we're considering to add this action into a Prow/Jenkins CI step.
	// 			// for now, don't remove it after this case finished.
	// 			CreateCatalog(oc, "redhat-operators", indexImage, catalogTemplate)
	// 		}
	// 	}

	// 	exutil.By("2, install an operator, for example, quay-operator")
	// 	err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
	// 		quayPackage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("package", "redhat-operators-quay-operator").Output()
	// 		if err != nil || strings.Contains(quayPackage, "not found") {
	// 			return false, nil
	// 		}
	// 		return true, nil
	// 	})
	// 	exutil.AssertWaitPollNoErr(err, "failed to get package redhat-operators-quay-operator!")

	// 	operatorTemplate := filepath.Join(olmBaseDir, "operator.yaml")
	// 	err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", operatorTemplate, "-p", "NAME=quay-example", "PACKAGE=quay-operator", "CHANNEL=stable-3.8", "VERSION=3.8.12")
	// 	if err != nil {
	// 		e2e.Failf("Failed to create operator quay-example: %s", err)
	// 	}
	// 	defer func() {
	// 		exutil.By("4, remove quay-example operator")
	// 		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("operator.operators.operatorframework.io", "quay-example").Output()
	// 		if err != nil {
	// 			e2e.Failf("Fail to delete quay-example operator, error:%v", err)
	// 		}
	// 	}()
	// 	err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
	// 		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "quay-operator-system", "deploy", "quay-operator.v3.8.12", "-o=jsonpath={.status.readyReplicas}").Output()
	// 		if err != nil && !strings.Contains(status, "not found") {
	// 			e2e.Failf("! fail to check quay-operator.v3.8.12: %s", err)
	// 		}
	// 		if status != "1" {
	// 			return false, nil
	// 		}
	// 		return true, nil
	// 	})
	// 	exutil.AssertWaitPollNoErr(err, "failed to install quay-operator.v3.8.12 operator!")

	// 	exutil.By("3, upgrade quay-operator v3.8.12 to v3.9.1")
	// 	_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("operator.operators.operatorframework.io", "quay-example", "-p", "{\"spec\": {\"version\": \"3.9.1\", \"channel\": \"stable-3.9\"}}", "--type=merge").Output()
	// 	if err != nil {
	// 		e2e.Failf("Fail to upgrade quay-operator v3.8.12 to v3.9.1, error:%v", err)
	// 	}
	// 	err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
	// 		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "quay-operator-system", "deploy", "quay-operator.v3.9.1", "-o=jsonpath={.status.readyReplicas}").Output()
	// 		if err != nil && !strings.Contains(status, "not found") {
	// 			e2e.Failf("! fail to check quay-operator.v3.9.1: %s", err)
	// 		}
	// 		if status != "1" {
	// 			return false, nil
	// 		}
	// 		return true, nil
	// 	})
	// 	exutil.AssertWaitPollNoErr(err, "failed to upgrade quay-operator v3.8.12 to v3.9.1!")
	// })

})
