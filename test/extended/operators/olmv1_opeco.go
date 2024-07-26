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
		exutil.SkipOnProxyCluster(oc)
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
		err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"pollInterval":"20s"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		pollInterval, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.spec.source.image.pollInterval}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(pollInterval)).To(o.ContainSubstring("20s"))
		clustercatalog.WaitCatalogStatus(oc, "Unpacked", 0)

		exutil.By("Collect the initial image status information")
		lastPollAttempt, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.lastPollAttempt}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
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
			lastPollAttempt2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.lastPollAttempt}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			resolvedRef2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
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
		o.Expect(string(output)).To(o.ContainSubstring("cannot specify PollInterval while using digest-based image"))

	})

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-High-69123-Catalogd clustercatalog offer the operator content through http server", func() {
		exutil.SkipOnProxyCluster(oc)
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

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-High-69124-check the clustercatalog source type before created", func() {
		exutil.SkipOnProxyCluster(oc)
		var (
			baseDir             = exutil.FixturePath("testdata", "olm", "v1")
			catalogPollTemplate = filepath.Join(baseDir, "clustercatalog-secret.yaml")
			clustercatalog      = olmv1util.ClusterCatalogDescription{
				Name:         "clustercatalog-69124",
				Imageref:     "quay.io/olmqe/olmtest-operator-index:nginxolm69124",
				PollInterval: "1m",
				Template:     catalogPollTemplate,
			}
		)
		exutil.By("Create clustercatalog")
		defer clustercatalog.Delete(oc)
		clustercatalog.Create(oc)

		exutil.By("Check image pollInterval time")
		errMsg, err := oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"pollInterval":"1mm"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(errMsg, "Invalid value: \"1mm\": spec.source.image.pollInterval in body")).To(o.BeTrue())

		exutil.By("Check type value")
		errMsg, err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"type":"redhat"}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(errMsg, "Unsupported value: \"redhat\": supported values: \"image\"")).To(o.BeTrue())

	})

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-High-69242-Catalogd deprecated package/bundlemetadata/catalogmetadata from clustercatalog CR", func() {
		exutil.SkipOnProxyCluster(oc)
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

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69069-Replace pod-based image unpacker with an image registry client", func() {
		exutil.SkipOnProxyCluster(oc)
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

		initresolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Update the index image with different tag , but the same digestID")
		err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index:nginxolm69069v1"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the image is updated without wait but the resolvedSource is still the same and won't unpack again")
		statusOutput, err := olmv1util.GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o", "jsonpath={.status.phase}")
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(statusOutput, "Unpacked") {
			e2e.Failf("status is %v, not Unpacked", statusOutput)
		}
		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			img, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if img != "quay.io/olmqe/olmtest-operator-index:nginxolm69069v1" {
				e2e.Logf("image: %v", img)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Error image wrong or resolvedRef are same")
		v1resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if initresolvedRef != v1resolvedRef {
			e2e.Failf("initresolvedRef:%v,v1resolvedRef:%v", initresolvedRef, v1resolvedRef)
		}

		exutil.By("Update the index image with different tag and digestID")
		err = oc.AsAdmin().Run("patch").Args("clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index:nginxolm69069v2"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		errWait = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 90*time.Second, false, func(ctx context.Context) (bool, error) {
			img, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			v2resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercatalog", clustercatalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if initresolvedRef == v2resolvedRef || img != "quay.io/olmqe/olmtest-operator-index:nginxolm69069v2" {
				e2e.Logf("image: %v,v2resolvedRef: %v", img, v2resolvedRef)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Error image wrong or resolvedRef are same")

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69869-Catalogd Add metrics to the Storage implementation", func() {
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
		promeEp, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("service", "-n", "openshift-catalogd", "catalogd-controller-manager-metrics-service", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(promeEp).NotTo(o.BeEmpty())

		metricsToken, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metricsToken).NotTo(o.BeEmpty())

		clustercatalogPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=catalog-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clustercatalogPodname).NotTo(o.BeEmpty())

		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			queryContent := "https://" + promeEp + ":8443/metrics"
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
	g.It("VMonly-ConnectedOnly-Author:xzha-High-70817-catalogd support setting a pull secret", func() {
		exutil.SkipOnProxyCluster(oc)
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog-secret.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-clusterrolebinding.yaml")
			ns                           = "ns-70817"
			sa                           = "sa70817"
			saCrb                        = olmv1util.SaCLusterRolebindingDescription{
				Name:      sa,
				Namespace: ns,
				Template:  saClusterRoleBindingTemplate,
			}
			clustercatalog = olmv1util.ClusterCatalogDescription{
				Name:         "clustercatalog-70817-quay",
				Imageref:     "quay.io/olmqe/olmtest-operator-index-private:nginxolm70817",
				PullSecret:   "fake-secret-70817",
				PollInterval: "1m",
				Template:     clustercatalogTemplate,
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
		clustercatalog.WaitCatalogStatus(oc, "Failing", 30)
		conditions, _ := olmv1util.GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o", "jsonpath={.status.conditions}")
		o.Expect(conditions).To(o.ContainSubstring("error fetching image"))
		o.Expect(conditions).To(o.ContainSubstring("401 Unauthorized"))

		exutil.By("3) Patch the clustercatalog")
		patchResource(oc, asAdmin, withoutNamespace, "clustercatalog", clustercatalog.Name, "-p", `{"spec":{"source":{"image":{"pullSecret":"secret-70817-quay"}}}}`, "--type=merge")
		clustercatalog.WaitCatalogStatus(oc, "Unpacked", 0)

		exutil.By("4) install clusterextension")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("v1.0.1"))
	})

	// author: jfan@redhat.com
	g.It("Author:jfan-VMonly-ConnectedOnly-High-69202-Catalogd clustercatalog offer the operator content through http server off cluster", func() {
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

		exutil.By("port-forward the catalogd-catalogserver")
		cmd1, _, _, err := oc.AsAdmin().WithoutNamespace().Run("port-forward").Args("svc/catalogd-catalogserver", "6920:80", "-n", "openshift-catalogd").Background()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer cmd1.Process.Kill()

		exutil.By("get the index content through http service off cluster")
		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 100*time.Second, false, func(ctx context.Context) (bool, error) {
			checkOutput, err := exec.Command("bash", "-c", "curl http://127.0.0.1:6920/catalogs/clustercatalog-69202/all.json").Output()
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
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("Cannot get the port-forward result"))
	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-73219-Fetch deprecation data from the catalogd http server", func() {
		exutil.SkipOnProxyCluster(oc)
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

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-73289-Check the deprecation conditions and messages", func() {
		exutil.SkipOnProxyCluster(oc)
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-clusterrolebinding.yaml")
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
		clusterextension.Patch(oc, `{"spec":{"version":">=1.0.2"}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundle, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.resolvedBundle}")
			if !strings.Contains(resolvedBundle, "v1.0.3") {
				e2e.Logf("clusterextension.resolvedBundle is %s, not v1.0.3, and try next", resolvedBundle)
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
		o.Expect(clusterextension.ResolvedBundle).To(o.ContainSubstring("v3.0.1"))

		exutil.By("Check ChannelDeprecated status and message")
		clusterextension.WaitClusterExtensionCondition(oc, "Deprecated", "True", 0)
		clusterextension.WaitClusterExtensionCondition(oc, "ChannelDeprecated", "True", 0)
		message = clusterextension.GetClusterExtensionMessage(oc, "ChannelDeprecated")
		if !strings.Contains(message, "The 'candidate-v3.0' channel is no longer supported. Please switch to the 'candidate-v3.1' channel.") {
			e2e.Failf("Info does not meet expectations, message :%v", message)
		}

		exutil.By("update channel to candidate-v3.1")
		clusterextension.Patch(oc, `{"spec":{"channel":"candidate-v3.1"}}`)

		exutil.By("Check if ChannelDeprecated status and messages still exist")
		clusterextension.WaitClusterExtensionCondition(oc, "Deprecated", "False", 0)
		clusterextension.WaitClusterExtensionCondition(oc, "ChannelDeprecated", "False", 0)
		message = clusterextension.GetClusterExtensionMessage(oc, "ChannelDeprecated")
		if strings.Contains(message, "The 'candidate-v3.0' channel is no longer supported. Please switch to the 'candidate-v3.1' channel.") {
			e2e.Failf("ChannelDeprecated message still exists :%v", message)
		}
		clusterextension.WaitClusterExtensionCondition(oc, "Resolved", "True", 0)
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

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-High-74948-catalog offer the operator content through https server", func() {
		exutil.SkipOnProxyCluster(oc)
		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			clustercatalogTemplate       = filepath.Join(baseDir, "clustercatalog.yaml")
			clusterextensionTemplate     = filepath.Join(baseDir, "clusterextension.yaml")
			saClusterRoleBindingTemplate = filepath.Join(baseDir, "sa-clusterrolebinding.yaml")
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
		describe, err := oc.WithoutNamespace().AsAdmin().Run("describe").Args("service", "catalogd-catalogserver", "-n", "openshift-catalogd").Output()
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

})
