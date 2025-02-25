package operators

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	olmv1util "github.com/openshift/openshift-tests-private/test/extended/operators/olmv1util"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM v1 opeco should", func() {
	// Hypershift will be supported from 4.19, so add NonHyperShiftHOST Per cases now.
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("olmv1-opeco"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		exutil.SkipNoOLMv1Core(oc)
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-VMonly-High-69758-Catalogd Polling remote registries for update to images content", func() {
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			quayCLI                = container.NewQuayCLI()
			imagev1                = "quay.io/olmqe/olmtest-operator-index:nginxolm69758v1"
			imagev2                = "quay.io/olmqe/olmtest-operator-index:nginxolm69758v2"

			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69758",
				Imageref: "quay.io/olmqe/olmtest-operator-index:test69758",
				Template: clustercatalogTemplate,
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
		indexImageDigest, err := quayCLI.GetImageDigest(strings.Replace(clustercatalog.Imageref, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(indexImageDigest).NotTo(o.BeEmpty())
		if indexImageDigest != manifestDigestv1 {
			//tag v1 to testrun image
			tagResult, tagErr := quayCLI.ChangeTag(strings.Replace(clustercatalog.Imageref, "quay.io/", "", 1), manifestDigestv1)
			if !tagResult {
				e2e.Logf("Error: %v", tagErr)
				e2e.Failf("Change tag failed on quay.io")
			}
			e2e.Logf("Successful init tag v1")
		}

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Add image pollInterval time")
		err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"pollIntervalMinutes": 1}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		pollInterval, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.spec.source.image.pollIntervalMinutes}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(pollInterval)).To(o.ContainSubstring("1"))
		clustercatalog.WaitCatalogStatus(oc, "true", "Serving", 0)

		exutil.By("Collect the initial image status information")
		resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		v1bundlesDataOut, err := clustercatalog.UnmarshalContent(oc, "bundle")
		o.Expect(err).NotTo(o.HaveOccurred())
		v1bundlesImage := olmv1util.GetBundlesImageTag(v1bundlesDataOut.Bundles)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Update the image and check for changes")
		//tag v2 to testrun image
		tagResult, tagErr := quayCLI.ChangeTag(strings.Replace(clustercatalog.Imageref, "quay.io/", "", 1), manifestDigestv2)
		if !tagResult {
			e2e.Logf("Error: %v", tagErr)
			e2e.Failf("Change tag failed on quay.io")
		}
		e2e.Logf("Successful tag v2")

		errWait := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 90*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedRef2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			if resolvedRef == resolvedRef2 {
				e2e.Logf("resolvedRef:%v,resolvedRef2:%v", resolvedRef, resolvedRef2)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Error resolvedRef are same")

		exutil.By("check the index content changes")
		v2bundlesDataOut, err := clustercatalog.UnmarshalContent(oc, "bundle")
		o.Expect(err).NotTo(o.HaveOccurred())
		v2bundlesImage := olmv1util.GetBundlesImageTag(v2bundlesDataOut.Bundles)
		o.Expect(err).NotTo(o.HaveOccurred())

		if reflect.DeepEqual(v1bundlesImage, v2bundlesImage) {
			e2e.Logf("v1bundlesImage%v, v2bundlesImage%v", v1bundlesImage, v2bundlesImage)
			e2e.Failf("Failed, The index content no changes")
		}
		e2e.Logf("v1bundlesImage%v, v2bundlesImage%v", v1bundlesImage, v2bundlesImage)

		exutil.By("Update use the digest image and check it")
		output, err := oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index@`+manifestDigestv1+`"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("cannot specify pollIntervalMinutes while using digest-based"))

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-69123-Catalogd clustercatalog offer the operator content through http server", func() {
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			clustercatalog         = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69123",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69123",
				Template: clustercatalogTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("get the index content through http service on cluster")
		unmarshalContent, err := clustercatalog.UnmarshalContent(oc, "all")
		o.Expect(err).NotTo(o.HaveOccurred())

		allPackageName := olmv1util.ListPackagesName(unmarshalContent.Packages)
		o.Expect(allPackageName[0]).To(o.ContainSubstring("nginx69123"))

		channelData := olmv1util.GetChannelByPakcage(unmarshalContent.Channels, "nginx69123")
		o.Expect(channelData[0].Name).To(o.ContainSubstring("candidate-v0.0"))

		bundlesName := olmv1util.GetBundlesNameByPakcage(unmarshalContent.Bundles, "nginx69123")
		o.Expect(bundlesName[0]).To(o.ContainSubstring("nginx69123.v0.0.1"))

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-DEPRECATED-ConnectedOnly-NonHyperShiftHOST-High-69124-check the clustercatalog source type before created", func() {
		var (
			baseDir             = exutil.FixturePath("testdata", "olm", "v1")
			catalogPollTemplate = filepath.Join(baseDir, "clustercatalog-secret.yaml")
			clustercatalog      = olmv1util.ClusterCatalogDescription{
				Name:                "clustercatalog-69124",
				Imageref:            "quay.io/olmqe/olmtest-operator-index:nginxolm69124",
				PollIntervalMinutes: "1",
				Template:            catalogPollTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Check image pollInterval time")
		errMsg, err := oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"pollIntervalMinutes":"1mm"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(errMsg, "Invalid value: \"1mm\": spec.source.image.pollIntervalMinutes in body")).To(o.BeTrue())

		exutil.By("Check type value")
		errMsg, err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"type":"redhat"}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(errMsg, "Unsupported value: \"redhat\": supported values: \"Image\"")).To(o.BeTrue())

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-69242-Catalogd deprecated package/bundlemetadata/catalogmetadata from clustercatalog CR", func() {
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			clustercatalog         = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69242",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69242",
				Template: clustercatalogTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

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

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-69069-Replace pod-based image unpacker with an image registry client", func() {
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			clustercatalog         = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69069",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69069",
				Template: clustercatalogTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		initresolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Update the index image with different tag , but the same digestID")
		err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index:nginxolm69069v1"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the image is updated without wait but the resolvedSource is still the same and won't unpack again")
		jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, "Serving")
		statusOutput, err := olmv1util.GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o", jsonpath)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(statusOutput, "True") {
			e2e.Failf("status is %v, not Serving", statusOutput)
		}
		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			img, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
			if err != nil {
				return false, err
			}
			if strings.Contains(img, initresolvedRef) {
				return true, nil
			}
			e2e.Logf("diff image1: %v, but expect same", img)
			return false, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o=jsonpath-as-json={.status}")
		}
		exutil.AssertWaitPollNoErr(errWait, "disgest is not same, but should be same")

		exutil.By("Update the index image with different tag and digestID")
		err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index:nginxolm69069v2"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		errWait = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 90*time.Second, false, func(ctx context.Context) (bool, error) {
			img, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
			if err != nil {
				return false, err
			}
			if strings.Contains(img, initresolvedRef) {
				e2e.Logf("same image, but expect not same")
				return false, nil
			}
			e2e.Logf("image2: %v", img)
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o=jsonpath-as-json={.status}")
		}
		exutil.AssertWaitPollNoErr(errWait, "digest is same, but should be not same")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-69869-Catalogd Add metrics to the Storage implementation", func() {
		exutil.SkipOnProxyCluster(oc)
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			clustercatalog         = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69869",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69869",
				Template: clustercatalogTemplate,
			}
			metricsMsg string
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Get http content")
		packageDataOut, err := clustercatalog.UnmarshalContent(oc, "package")
		o.Expect(err).NotTo(o.HaveOccurred())
		packageName := olmv1util.ListPackagesName(packageDataOut.Packages)
		o.Expect(packageName[0]).To(o.ContainSubstring("nginx69869"))

		exutil.By("Get token and clusterIP")
		promeEp, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("service", "-n", "openshift-catalogd", "catalogd-service", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(promeEp).NotTo(o.BeEmpty())

		metricsToken, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metricsToken).NotTo(o.BeEmpty())

		clustercatalogPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=catalog-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clustercatalogPodname).NotTo(o.BeEmpty())

		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			queryContent := "https://" + promeEp + ":7443/metrics"
			metricsMsg, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", clustercatalogPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", metricsToken), queryContent).Output()
			e2e.Logf("err:%v", err)
			if strings.Contains(metricsMsg, "catalogd_http_request_duration_seconds_bucket{code=\"200\"") {
				e2e.Logf("found catalogd_http_request_duration_seconds_bucket{code=\"200\"")
				return true, nil
			}
			return false, nil
		})
		if errWait != nil {
			e2e.Logf("metricsMsg:%v", metricsMsg)
			exutil.AssertWaitPollNoErr(errWait, "catalogd_http_request_duration_seconds_bucket{code=\"200\" not found.")
		}

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-VMonly-DEPRECATED-ConnectedOnly-NonHyperShiftHOST-High-70817-catalogd support setting a pull secret", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-secret.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-70817"
			sa                           = "sa70817"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:                "clustercatalog-70817-quay",
				Imageref:            "quay.io/olmqe/olmtest-operator-index-private:nginxolm70817",
				PullSecret:          "fake-secret-70817",
				PollIntervalMinutes: "1",
				Template:            clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-70817",
				InstallNamespace: ns,
				PackageName:      "nginx70817",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("1) Create secret")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-catalogd", "secret", "secret-70817-quay").Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-catalogd", "secret", "generic", "secret-70817-quay", "--from-file=.dockerconfigjson=/home/cloud-user/.docker/config.json", "--type=kubernetes.io/dockerconfigjson").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2) Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.CreateWithoutCheck(oc)
		clustercatalog.WaitCatalogStatus(oc, "false", "Serving", 30)
		conditions, _ := olmv1util.GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o", "jsonpath={.status.conditions}")
		o.Expect(conditions).To(o.ContainSubstring("error fetching"))
		o.Expect(conditions).To(o.ContainSubstring("401 Unauthorized"))

		exutil.By("3) Patch the clustercatalog")
		patchResource(oc, asAdmin, withoutNamespace, "clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"pullSecret":"secret-70817-quay"}}}}`, "--type=merge")
		clustercatalog.WaitCatalogStatus(oc, "true", "Serving", 0)

		exutil.By("4) install clusterextension")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-69202-Catalogd clustercatalog offer the operator content through http server off cluster", func() {
		exutil.SkipOnProxyCluster(oc)
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			clustercatalog         = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-69202",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69202",
				Template: clustercatalogTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("get the index content through http service off cluster")
		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 100*time.Second, false, func(ctx context.Context) (bool, error) {
			checkOutput, err := exec.Command("bash", "-c", "curl -k "+clustercatalog.ContentURL).Output()
			if err != nil {
				e2e.Logf("failed to execute the curl: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("nginx69202", string(checkOutput)); matched {
				e2e.Logf("Check the content off cluster success\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Cannot get the result")
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-73219-Fetch deprecation data from the catalogd http server", func() {
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			clustercatalog         = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-73219",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm73219",
				Template: clustercatalogTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("get the deprecation content through http service on cluster")
		unmarshalContent, err := clustercatalog.UnmarshalContent(oc, "deprecations")
		o.Expect(err).NotTo(o.HaveOccurred())

		deprecatedChannel := olmv1util.GetDeprecatedChannelNameByPakcage(unmarshalContent.Deprecations, "nginx73219")
		o.Expect(deprecatedChannel[0]).To(o.ContainSubstring("candidate-v0.0"))

		deprecatedBundle := olmv1util.GetDeprecatedBundlesNameByPakcage(unmarshalContent.Deprecations, "nginx73219")
		o.Expect(deprecatedBundle[0]).To(o.ContainSubstring("nginx73219.v0.0.1"))

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-73289-Check the deprecation conditions and messages", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-73289"
			sa                           = "sa73289"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-73289",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm73289",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-73289",
				InstallNamespace: ns,
				PackageName:      "nginx73289v1",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v1.0, version 1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		// Test BundleDeprecated
		exutil.By("Check BundleDeprecated status")
		clusterextension.WaitClusterExtensionCondition(oc, "Deprecated", "True", 0)
		clusterextension.WaitClusterExtensionCondition(oc, "BundleDeprecated", "True", 0)

		exutil.By("Check BundleDeprecated message info")
		message := clusterextension.GetClusterExtensionMessage(oc, "BundleDeprecated")
		if !strings.Contains(message, "nginx73289v1.v1.0.1 is deprecated. Uninstall and install v1.0.3 for support.") {
			e2e.Failf("Info does not meet expectations, message :%v", message)
		}

		exutil.By("update version to be >=1.0.2")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"version": ">=1.0.2"}}}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			installedBundle, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.install.bundle.name}")
			if !strings.Contains(installedBundle, "v1.0.3") {
				e2e.Logf("clusterextension.InstalledBundle is %s, not v1.0.3, and try next", installedBundle)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension resolvedBundle is not v1.0.3")
		}

		exutil.By("Check if BundleDeprecated status and messages still exist")
		clusterextension.WaitClusterExtensionCondition(oc, "Deprecated", "False", 0)
		clusterextension.WaitClusterExtensionCondition(oc, "BundleDeprecated", "False", 0)
		message = clusterextension.GetClusterExtensionMessage(oc, "BundleDeprecated")
		if strings.Contains(message, "nginx73289v1.v1.0.1 is deprecated. Uninstall and install v1.0.3 for support.") {
			e2e.Failf("BundleDeprecated message still exists :%v", message)
		}
		clusterextension.Delete(oc)
		exutil.By("BundleDeprecated test done")

		// Test ChannelDeprecated
		exutil.By("update channel to candidate-v3.0")
		clusterextension.PackageName = "nginx73289v2"
		clusterextension.Channel = "candidate-v3.0"
		clusterextension.Version = ">=1.0.0"
		clusterextension.Template = clusterextensionTemplate

		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v3.0.1"))

		exutil.By("Check ChannelDeprecated status and message")
		clusterextension.WaitClusterExtensionCondition(oc, "Deprecated", "True", 0)
		clusterextension.WaitClusterExtensionCondition(oc, "ChannelDeprecated", "True", 0)
		message = clusterextension.GetClusterExtensionMessage(oc, "ChannelDeprecated")
		if !strings.Contains(message, "The 'candidate-v3.0' channel is no longer supported. Please switch to the 'candidate-v3.1' channel.") {
			e2e.Failf("Info does not meet expectations, message :%v", message)
		}

		exutil.By("update channel to candidate-v3.1")
		clusterextension.Patch(oc, `{"spec":{"source":{"catalog":{"channels": ["candidate-v3.1"]}}}}`)

		exutil.By("Check if ChannelDeprecated status and messages still exist")
		clusterextension.WaitClusterExtensionCondition(oc, "Deprecated", "False", 0)
		clusterextension.WaitClusterExtensionCondition(oc, "ChannelDeprecated", "False", 0)
		message = clusterextension.GetClusterExtensionMessage(oc, "ChannelDeprecated")
		if strings.Contains(message, "The 'candidate-v3.0' channel is no longer supported. Please switch to the 'candidate-v3.1' channel.") {
			e2e.Failf("ChannelDeprecated message still exists :%v", message)
		}
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "reason", "Succeeded", 3, 150, 0)
		clusterextension.WaitClusterExtensionCondition(oc, "Installed", "True", 0)
		clusterextension.Delete(oc)
		exutil.By("ChannelDeprecated test done")

		// Test PackageDeprecated
		exutil.By("update Package to 73289v3")
		clusterextension.PackageName = "nginx73289v3"
		clusterextension.Channel = "candidate-v1.0"
		clusterextension.Version = ">=1.0.0"
		clusterextension.Template = clusterextensionTemplate
		clusterextension.Create(oc)

		exutil.By("Check PackageDeprecated status and message")
		clusterextension.WaitClusterExtensionCondition(oc, "Deprecated", "True", 0)
		clusterextension.WaitClusterExtensionCondition(oc, "PackageDeprecated", "True", 0)
		message = clusterextension.GetClusterExtensionMessage(oc, "PackageDeprecated")
		if !strings.Contains(message, "The nginx73289v3 package is end of life. Please use the another package for support.") {
			e2e.Failf("Info does not meet expectations, message :%v", message)
		}
		exutil.By("PackageDeprecated test done")

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-74948-catalog offer the operator content through https server", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-74948"
			sa                           = "sa74948"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-74948",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm74948",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-74948",
				InstallNamespace: ns,
				PackageName:      "nginx74948",
				Channel:          "candidate-v1.0",
				Version:          "1.0.3",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Examine the service to confirm that the annotations are present")
		describe, err := oc.WithoutNamespace().AsAdmin().Run("describe").Args("service", "catalogd-service", "-n", "openshift-catalogd").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(describe).To(o.ContainSubstring("service.beta.openshift.io/serving-cert-secret-name: catalogserver-cert"))

		exutil.By("Ensure that the service CA bundle has been injected")
		crt, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configmap", "openshift-service-ca.crt", "-n", "openshift-catalogd", "-o", "jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(crt).To(o.ContainSubstring("{\"service.beta.openshift.io/inject-cabundle\":\"true\"}"))

		exutil.By("Check secret data tls.crt tls.key")
		secretData, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("secret", "catalogserver-cert", "-n", "openshift-catalogd", "-o", "jsonpath={.data}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(secretData, "tls.crt") || !strings.Contains(secretData, "tls.key") {
			e2e.Failf("secret data not found")
		}

		exutil.By("Get the index content through https service on cluster")
		unmarshalContent, err := clustercatalog.UnmarshalContent(oc, "all")
		o.Expect(err).NotTo(o.HaveOccurred())

		allPackageName := olmv1util.ListPackagesName(unmarshalContent.Packages)
		o.Expect(allPackageName[0]).To(o.ContainSubstring("nginx74948"))

		exutil.By("Create clusterextension to verify operator-controller has been started, appropriately loaded the CA certs")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.3"))

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-OSD_CCS-High-74978-CRD upgrade will be prevented if the Scope is switched between Namespaced and Cluster", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-74978"
			sa                           = "sa74978"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-74978",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm74978",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-74978",
				InstallNamespace: ns,
				PackageName:      "nginx74978",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension v1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("Update the version to 1.0.2, check changed from Namespaced to Cluster")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`CustomResourceDefinition nginxolm74978s.cache.example.com failed upgrade safety validation. "NoScopeChange" validation failed: scope changed from "Namespaced" to "Cluster"`, 10, 60, 0)

		clusterextension.Delete(oc)

		exutil.By("Create clusterextension v1.0.2")
		clusterextension.Version = "1.0.2"
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.2"))

		exutil.By("Update the version to 1.0.3, check changed from Cluster to Namespaced")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.3","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.2"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`CustomResourceDefinition nginxolm74978s.cache.example.com failed upgrade safety validation. "NoScopeChange" validation failed: scope changed from "Cluster" to "Namespaced"`, 10, 60, 0)

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-75218-Disabling the CRD Upgrade Safety preflight checks", func() {
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-75218"
			sa                           = "sa75218"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-75218",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm75218",
				Template: clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-75218",
				InstallNamespace: ns,
				PackageName:      "nginx75218",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension v1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("update the version to 1.0.2, report messages and upgrade safety fail")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`scope changed from "Namespaced" to "Cluster"`, 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`.spec.field1 may not be removed`, 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`calculating schema diff for CRD version "v1alpha1"`, 10, 60, 0)

		exutil.By("disabled crd upgrade safety check, it will not affect spec.scope: Invalid value: Cluster")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}, "install":{"preflight":{"crdUpgradeSafety":{"enforcement":"None"}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		var message string
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 18*time.Second, false, func(ctx context.Context) (bool, error) {
			message = clusterextension.GetClusterExtensionMessage(oc, "Progressing")
			if !strings.Contains(message, `CustomResourceDefinition.apiextensions.k8s.io "nginxolm75218s.cache.example.com" is invalid: spec.scope: Invalid value: "Cluster": field is immutable`) {
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o=jsonpath-as-json={.status}")
		}
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("Unexpected results message: %v", message))

		exutil.By("disabled crd upgrade safety check An existing stored version of the CRD is removed")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.3","upgradeConstraintPolicy":"SelfCertified"}}, "install":{"preflight":{"crdUpgradeSafety":{"enforcement":"None"}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`must have exactly one version marked as storage version, status.storedVersions[0]: Invalid value: "v1alpha1": must appear in spec.versions`, 10, 60, 0)

		exutil.By("disabled crd upgrade safety successfully")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.5","upgradeConstraintPolicy":"SelfCertified"}}, "install":{"preflight":{"crdUpgradeSafety":{"enforcement":"None"}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "reason", "Succeeded", 3, 150, 0)
		clusterextension.WaitClusterExtensionCondition(oc, "Installed", "True", 0)
		clusterextension.GetBundleResource(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.5"))

		clusterextension.CheckClusterExtensionCondition(oc, "Installed", "message",
			"Installed bundle quay.io/openshifttest/nginxolm-operator-bundle:v1.0.5-nginxolm75218 successfully", 10, 60, 0)

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-75122-CRD upgrade check Removing an existing stored version and add a new CRD with no modifications to existing versions", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "75122"
			labelValue                   = caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension-withselectorlabel.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-75122"
			sa                           = "sa75122"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:       "clustercatalog-75122",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginxolm75122",
				LabelValue: labelValue,
				Template:   clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-75122",
				InstallNamespace: ns,
				PackageName:      "nginx75122",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension v1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("upgrade will be prevented if An existing stored version of the CRD is removed")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`failed: stored version "v1alpha1" removed for resolved bundle "nginx75122.v1.0.2" with version "1.0.2"`, 10, 60, 0)

		exutil.By("upgrade will be allowed if A new version of the CRD is added with no modifications to existing versions")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.3","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "reason", "Succeeded", 3, 150, 0)
		clusterextension.WaitClusterExtensionCondition(oc, "Installed", "True", 0)
		clusterextension.GetBundleResource(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.3"))

		clusterextension.CheckClusterExtensionCondition(oc, "Installed", "message",
			"Installed bundle quay.io/openshifttest/nginxolm-operator-bundle:v1.0.3-nginxolm75122 successfully", 10, 60, 0)

		exutil.By("upgrade will be prevented if An existing served version of the CRD is removed")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.6","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterextension.CheckClusterExtensionCondition(oc, "Installed", "message",
			"Installed bundle quay.io/openshifttest/nginxolm-operator-bundle:v1.0.6-nginxolm75122 successfully", 10, 60, 0)

	})

	// author: xzha@redhat.com
	// Cover test case: OCP-75123 and OCP-75217
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-75123-High-75217-CRD upgrade checks for changes in required field and field type", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "75123"
			labelValue                   = caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension-withselectorlabel.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-75123"
			sa                           = "sa75123"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:       "clustercatalog-75123",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginxolm75123",
				LabelValue: labelValue,
				Template:   clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-75123",
				InstallNamespace: ns,
				PackageName:      "nginx75123",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension v1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("upgrade will be prevented if A new required field is added to an existing version of the CRD")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		// Cover test case: OCP-75217 - [olmv1] Override the unsafe upgrades with the warning message
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`new required fields [requiredfield2] added for resolved bundle`, 10, 60, 0)

		exutil.By("upgrade will be prevented if An existing field is removed from an existing version of the CRD")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.3","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`CustomResourceDefinition nginxolm75123s.cache.example.com failed upgrade safety validation. "NoExistingFieldRemoved" validation failed: crd/nginxolm75123s.cache.example.com version/v1alpha1 field/^.spec.field may not be removed`, 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`CustomResourceDefinition nginxolm75123s.cache.example.com failed upgrade safety validation. "ChangeValidator" validation failed: calculating schema diff for CRD version "v1alpha1"`, 10, 60, 0)

		exutil.By("upgrade will be prevented if An existing field type is changed in an existing version of the CRD")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.6","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`field "^.spec.field": type changed from "integer" to "string" for resolved bundle`, 10, 60, 0)

		exutil.By("upgrade will be allowed if An existing required field is changed to optional in an existing version")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.8","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "reason", "Succeeded", 3, 150, 0)
		clusterextension.WaitClusterExtensionCondition(oc, "Installed", "True", 0)
		clusterextension.GetBundleResource(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.8"))

		clusterextension.CheckClusterExtensionCondition(oc, "Installed", "message",
			"Installed bundle quay.io/openshifttest/nginxolm-operator-bundle:v1.0.8-nginxolm75123 successfully", 10, 60, 0)

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-75124-CRD upgrade checks for changes in default values", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "75124"
			labelValue                   = caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension-withselectorlabel.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-75124"
			sa                           = "sa75124"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:       "clustercatalog-75124",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginxolm75124",
				LabelValue: labelValue,
				Template:   clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-75124",
				InstallNamespace: ns,
				PackageName:      "nginx75124",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension v1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("upgrade will be prevented if A new default value is added to a field that did not previously have a default value")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`default value "\"default-string-jitli\"" added when there was no default previously for resolved bundle`, 10, 60, 0)

		exutil.By("upgrade will be prevented if The default value of a field is changed")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.3","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`failed: version "v1alpha1", field "^.spec.defaultenum": default value changed from "\"value1\"" to "\"value3\"" for resolved bundle`, 10, 60, 0)

		exutil.By("upgrade will be prevented if An existing default value of a field is removed")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.6","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`field "^.spec.defaultint": default value "9" removed for resolved bundle`, 10, 60, 0)

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-75515-CRD upgrade checks for changes in enumeration values", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "75515"
			labelValue                   = caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension-withselectorlabel.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-75515"
			sa                           = "sa75515"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:       "clustercatalog-75515",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginxolm75515",
				LabelValue: labelValue,
				Template:   clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-75515",
				InstallNamespace: ns,
				PackageName:      "nginx75515",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension v1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("upgrade will be prevented if New enum restrictions are added to an existing field which did not previously have enum restrictions")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`field "^.spec.unenumfield": enum constraints ["value1" "value2" "value3"] added when there were no restrictions previously for resolved bundle`, 10, 60, 0)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension v1.0.3")
		clusterextension.Version = "1.0.3"
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.3"))

		exutil.By("upgrade will be prevented if Existing enum values from an existing field are removed")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.5","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message", "validation failed", 10, 60, 0)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.3"))

		exutil.By("upgrade will be allowed if Adding new enum values to the list of allowed enum values in a field")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.6","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterextension.WaitClusterExtensionVersion(oc, "v1.0.6")
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-High-75516-CRD upgrade checks for the field maximum minimum changes", func() {
		exutil.SkipForSNOCluster(oc)
		var (
			caseID                       = "75516"
			labelValue                   = caseID
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-withlabel.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension-withselectorlabel.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-admin.yaml")
			ns                           = "ns-75516"
			sa                           = "sa75516"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:       "clustercatalog-75516",
				Imageref:   "quay.io/openshifttest/nginxolm-operator-index:nginxolm75516",
				LabelValue: labelValue,
				Template:   clustercatalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:             "clusterextension-75516",
				InstallNamespace: ns,
				PackageName:      "nginx75516",
				Channel:          "candidate-v1.0",
				Version:          "1.0.1",
				SaName:           sa,
				LabelValue:       labelValue,
				Template:         clusterextensionTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Create namespace")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmv1util.Appearance(oc, exutil.Appear, "ns", ns)).To(o.BeTrue())

		exutil.By("Create SA for clusterextension")
		defer saCrb.Delete(oc)
		saCrb.Create(oc)

		exutil.By("Create clusterextension v1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		exutil.By("upgrade will be prevented if The minimum value of an existing field is increased in an existing version and The maximum value of an existing field is decreased in an existing version")
		exutil.By("Check minimum & maximum")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.2","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"maximum: constraint decreased from 100 to 80", 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"minimum: constraint increased from 10 to 20", 10, 60, 0)

		exutil.By("Check minLength & maxLength")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.3","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"maxLength: constraint decreased from 50 to 30", 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"minLength: constraint increased from 3 to 9", 10, 60, 0)

		exutil.By("Check minProperties & maxProperties")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.4","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"maxProperties: constraint decreased from 5 to 4", 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"minProperties: constraint increased from 2 to 3", 10, 60, 0)

		exutil.By("Check minItems & maxItems")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.5","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"maxItems: constraint decreased from 10 to 9", 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			"minItems: constraint increased from 2 to 3", 10, 60, 0)

		exutil.By("upgrade will be prevented if Minimum or maximum field constraints are added to a field that did not previously have constraints")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.6","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.1"))

		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`maximum: constraint 100 added when there were no restrictions previously`, 10, 60, 0)
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "message",
			`minimum: constraint 10 added when there were no restrictions previously`, 10, 60, 0)

		exutil.By("upgrade will be Allowed if The minimum value of an existing field is decreased in an existing version & The maximum value of an existing field is increased in an existing version")
		err = oc.AsAdmin().Run("patch").Args("clusterextension", clusterextension.Name, "-p", `{"spec":{"source":{"catalog":{"version":"1.0.7","upgradeConstraintPolicy":"SelfCertified"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterextension.CheckClusterExtensionCondition(oc, "Progressing", "reason", "Succeeded", 3, 150, 0)
		clusterextension.WaitClusterExtensionCondition(oc, "Installed", "True", 0)
		clusterextension.GetBundleResource(oc)
		o.Expect(clusterextension.InstalledBundle).To(o.ContainSubstring("v1.0.7"))

		clusterextension.CheckClusterExtensionCondition(oc, "Installed", "message",
			"Installed bundle quay.io/openshifttest/nginxolm-operator-bundle:v1.0.7-nginxolm75516 successfully", 10, 60, 0)

	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-NonHyperShiftHOST-Critical-75441-Catalogd supports compression and jsonlines format", func() {
		var (
			baseDir                = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate = filepath.Join(baseDir, "clustercatalog.yaml")
			clustercatalog         = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-75441",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm75441",
				Template: clustercatalogTemplate,
			}
			clustercatalog1 = olmv1util.ClusterCatalogDescription{
				Name:     "clustercatalog-75441v2",
				Imageref: "quay.io/openshifttest/nginxolm-operator-index:nginxolm75441v2",
				Template: clustercatalogTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)
		defer clustercatalog1.Delete(oc)
		clustercatalog1.Create(oc)

		exutil.By("Get the gzip response")
		url1 := clustercatalog.ContentURL

		exutil.By("Check the url response of clustercatalog-75441")
		getCmd := fmt.Sprintf("curl -ki %s -H \"Accept-Encoding: gzip\" --output -", url1)
		stringMessage, err := exec.Command("bash", "-c", getCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(strings.ToLower(string(stringMessage)), "content-encoding: gzip") {
			e2e.Logf(string(stringMessage))
			e2e.Failf("string Content-Encoding: gzip not in the output")
		}
		if !strings.Contains(strings.ToLower(string(stringMessage)), "content-type: application/jsonl") {
			e2e.Logf(string(stringMessage))
			e2e.Failf("string Content-Type: application/jsonl not in the output")
		}
		exutil.By("Check the url response of clustercatalog-75441v2")
		url2 := clustercatalog1.ContentURL
		getCmd2 := fmt.Sprintf("curl -ki %s -H \"Accept-Encoding: gzip\"", url2)
		stringMessage2, err := exec.Command("bash", "-c", getCmd2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.ToLower(string(stringMessage2))).NotTo(o.ContainSubstring("content-encoding: gzip"))
		o.Expect(strings.ToLower(string(stringMessage2))).To(o.ContainSubstring("content-type: application/jsonl"))

	})
})
