package operators

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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
			unhealthyPod              = bundleDeploymentDescription{
				bdName:       "68903-pod-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-podunhealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			unhealthyPodChild = []childResource{
				{kind: "namespace", ns: ""},
			}
			unhealthyApiservice = bundleDeploymentDescription{
				bdName:       "68903-apis-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-apisunhealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			unhealthyApiserviceChild = []childResource{
				{kind: "APIService", ns: ""},
			}
			unhealthyCRD = bundleDeploymentDescription{
				bdName:       "68903-crd-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-crdunhealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			unhealthyDS = bundleDeploymentDescription{
				bdName:       "68903-ds-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-dsunhealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			unhealthyDSChild = []childResource{
				{kind: "namespace", ns: ""},
			}
		)

		exutil.By("create unhealthy pod")
		defer unhealthyPod.deleteWithoutCheck(oc)
		unhealthyPod.createWithoutCheck(oc)
		unhealthyPod.assertHealthyWithConsistent(oc, "false")
		unhealthyPod.delete(oc, unhealthyPodChild)

		exutil.By("create unhealthy APIService")
		defer unhealthyApiservice.deleteWithoutCheck(oc)
		unhealthyApiservice.createWithoutCheck(oc)
		unhealthyApiservice.assertHealthyWithConsistent(oc, "false")
		unhealthyApiservice.delete(oc, unhealthyApiserviceChild)

		exutil.By("create unhealthy CRD")
		defer unhealthyCRD.deleteWithoutCheck(oc)
		unhealthyCRD.createWithoutCheck(oc)
		unhealthyCRD.assertHealthyWithConsistent(oc, "false")
		unhealthyCRD.deleteWithoutCheck(oc)

		exutil.By("create unhealthy DS")
		defer unhealthyDS.deleteWithoutCheck(oc)
		unhealthyDS.createWithoutCheck(oc)
		unhealthyDS.assertHealthyWithConsistent(oc, "false")
		unhealthyDS.delete(oc, unhealthyDSChild)

	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68936-BundleDeployment Health resource healthy and install fail", func() {

		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate    = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			basicBdRegistryImageTemplate = filepath.Join(baseDir, "basic-bd-registry-image.yaml")
			healthBd                     = bundleDeploymentDescription{
				bdName:       "68903-healthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-healthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			healthChild = []childResource{
				{kind: "CustomResourceDefinition", ns: ""},
				{kind: "pod", ns: "olmv1-68903-healthy"},
				{kind: "APIService", ns: ""},
				{kind: "namespace", ns: ""},
			}
			unhealthyDp = bundleDeploymentDescription{
				bdName:       "68903-deployment-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:registry-68903-deployunhealthy",
				activeBundle: "",
				template:     basicBdRegistryImageTemplate,
			}
			unhealthyDpChild = []childResource{
				{kind: "CustomResourceDefinition", ns: ""},
				{kind: "namespace", ns: ""},
			}
			unhealthyRC = bundleDeploymentDescription{
				bdName:       "68903-rc-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-rcunhealth",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			unhealthyRCChild = []childResource{
				{kind: "namespace", ns: ""},
			}
			unhealthyInstall = bundleDeploymentDescription{
				bdName:       "68903-install-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-installunhealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
		)

		exutil.By("create health bundledeployment")
		defer healthBd.deleteWithoutCheck(oc)
		healthBd.create(oc)
		healthBd.delete(oc, healthChild)

		exutil.By("create unhealthy deployment")
		defer unhealthyDp.deleteWithoutCheck(oc)
		unhealthyDp.createWithoutCheck(oc)
		unhealthyDp.assertHealthyWithConsistent(oc, "false")
		unhealthyDp.delete(oc, unhealthyDpChild)

		exutil.By("create unhealthy RC")
		defer unhealthyRC.deleteWithoutCheck(oc)
		unhealthyRC.createWithoutCheck(oc)
		unhealthyRC.assertHealthy(oc, "true") // here is possible issue
		unhealthyRC.delete(oc, unhealthyRCChild)

		exutil.By("install fails")
		defer unhealthyInstall.deleteWithoutCheck(oc)
		unhealthyInstall.createWithoutCheck(oc)
		unhealthyInstall.assertHealthyWithConsistent(oc, "false")
		unhealthyInstall.deleteWithoutCheck(oc)

	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68937-BundleDeployment Health resource unhealthy ss rs unspport", func() {

		var (
			baseDir                   = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			unhealthySS               = bundleDeploymentDescription{
				bdName:       "68903-ss-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-ssunhealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			unhealthySSChild = []childResource{
				{kind: "namespace", ns: ""},
			}
			unhealthyRS = bundleDeploymentDescription{
				bdName:       "68903-rs-unhealthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-rsunhealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			unhealthyRSChild = []childResource{
				{kind: "namespace", ns: ""},
			}

			healthUnspport = bundleDeploymentDescription{
				bdName:       "68903-unspport-healthy",
				address:      "quay.io/olmqe/olmv1bundle:plain-68903-unsupporthealthy",
				activeBundle: "",
				template:     basicBdPlainImageTemplate,
			}
			healthUnspportChild = []childResource{
				{kind: "namespace", ns: ""},
			}
		)

		exutil.By("create unhealthy SS")
		defer unhealthySS.deleteWithoutCheck(oc)
		unhealthySS.createWithoutCheck(oc)
		unhealthySS.assertHealthyWithConsistent(oc, "false")
		unhealthySS.delete(oc, unhealthySSChild)

		exutil.By("create unhealthy RS")
		defer unhealthyRS.deleteWithoutCheck(oc)
		unhealthyRS.createWithoutCheck(oc)
		unhealthyRS.assertHealthyWithConsistent(oc, "false")
		unhealthyRS.delete(oc, unhealthyRSChild)

		exutil.By("unsupport health")
		defer healthUnspport.deleteWithoutCheck(oc)
		healthUnspport.createWithoutCheck(oc)
		healthUnspport.assertHealthy(oc, "true")
		healthUnspport.delete(oc, healthUnspportChild)

	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-High-68821-OLMv1 Supports Version Ranges during Installation", func() {
		var (
			baseDir                               = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate                       = filepath.Join(baseDir, "catalog.yaml")
			operatorTemplate                      = filepath.Join(baseDir, "operator.yaml")
			operatorWithoutChannelTemplate        = filepath.Join(baseDir, "operatorWithoutChannel.yaml")
			operatorWithoutChannelVersionTemplate = filepath.Join(baseDir, "operatorWithoutChannelVersion.yaml")
			catalog                               = catalogDescription{
				name:     "catalog-68821",
				imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm68821",
				template: catalogTemplate,
			}
			operator = operatorDescription{
				name:        "operator-68821",
				packageName: "nginx68821",
				channel:     "candidate-v0.0",
				version:     ">=0.0.1",
				template:    operatorTemplate,
			}
		)
		exutil.By("create catalog")
		defer catalog.delete(oc)
		catalog.create(oc)

		exutil.By("create operator with channel candidate-v0.0, version >=0.0.1")
		defer operator.delete(oc)
		operator.create(oc)
		o.Expect(operator.resolvedBundleResource).To(o.ContainSubstring("v0.0.3"))
		operator.delete(oc)

		exutil.By("create operator with channel candidate-v1.0, version 1.0.x")
		operator.channel = "candidate-v1.0"
		operator.version = "1.0.x"
		operator.create(oc)
		o.Expect(operator.resolvedBundleResource).To(o.ContainSubstring("v1.0.2"))
		operator.delete(oc)

		exutil.By("create operator with channel empty, version >=0.0.1 !=1.1.0 <1.1.2")
		operator.channel = ""
		operator.version = ">=0.0.1 !=1.1.0 <1.1.2"
		operator.template = operatorWithoutChannelTemplate
		operator.create(oc)
		o.Expect(operator.resolvedBundleResource).To(o.ContainSubstring("v1.0.2"))
		operator.delete(oc)

		exutil.By("create operator with channel empty, version empty")
		operator.channel = ""
		operator.version = ""
		operator.template = operatorWithoutChannelVersionTemplate
		operator.create(oc)
		o.Expect(operator.resolvedBundleResource).To(o.ContainSubstring("v1.1.0"))
		operator.delete(oc)

		exutil.By("create operator with invalid version")
		operator.version = "!1.0.1"
		operator.template = operatorTemplate
		err := operator.createWithoutCheck(oc)
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-69196-OLMv1 Supports Version Ranges during operator upgrade", func() {
		var (
			baseDir          = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate  = filepath.Join(baseDir, "catalog.yaml")
			operatorTemplate = filepath.Join(baseDir, "operator.yaml")
			catalog          = catalogDescription{
				name:     "catalog-69196",
				imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69196",
				template: catalogTemplate,
			}
			operator = operatorDescription{
				name:        "operator-69196",
				packageName: "nginx69196",
				channel:     "candidate-v1.0",
				version:     "1.0.1",
				template:    operatorTemplate,
			}
		)
		exutil.By("create catalog")
		defer catalog.delete(oc)
		catalog.create(oc)

		exutil.By("create operator with channel candidate-v1.0, version 1.0.1")
		defer operator.delete(oc)
		operator.create(oc)
		o.Expect(operator.installedBundleResource).To(o.ContainSubstring("v1.0.1"))

		exutil.By("update version to be >=1.0.1")
		operator.patch(oc, `{"spec":{"version":">=1.0.1"}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundleResource, _ := getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o", "jsonpath={.status.resolvedBundleResource}")
			if !strings.Contains(resolvedBundleResource, "v1.0.2") {
				e2e.Logf("operator.resolvedBundleResource is %s, not v1.0.2, and try next", resolvedBundleResource)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "operator resolvedBundleResource is not v1.0.2")
		}

		exutil.By("update channel to be candidate-v1.1")
		operator.patch(oc, `{"spec":{"channel":"candidate-v1.1"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundleResource, _ := getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o", "jsonpath={.status.resolvedBundleResource}")
			if !strings.Contains(resolvedBundleResource, "v1.1.0") {
				e2e.Logf("operator.resolvedBundleResource is %s, not v1.1.0, and try next", resolvedBundleResource)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			getField(oc, false, asAdmin, withoutNamespace, "operator.operators.operatorframework.io", operator.name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "operator resolvedBundleResource is not v1.1.0")
		}
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
