package workloads

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("ocmirrorv2", exutil.KubeConfigPath())
	)
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Medium-72973-support mirror multi-arch additional images for v2 [Serial]", func() {
		dirname := "/tmp/case72973"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72973.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72973")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72973")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start mirroring of additionalImages to disk")
		waitErr := wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk for additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk for additionalImages still failed")

		exutil.By("Start mirroring of additionalImages to registry")
		waitErr = wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/multiarch", "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror of additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror for additionalImages still failed")

		// Validate if multi arch additionalImages have been mirrored
		exutil.By("Validate if multi arch additionalImages have been mirrored")
		additionalImageList := []string{"/multiarch/ubi8/ubi:latest", "/multiarch/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27", "/multiarch/openshifttest/scratch@sha256:b045c6ba28db13704c5cbf51aff3935dbed9a692d508603cc80591d89ab26308"}
		for _, image := range additionalImageList {
			ref, err := docker.ParseReference("//" + serInfo.serviceName + image)
			o.Expect(err).NotTo(o.HaveOccurred())
			sys := &types.SystemContext{
				AuthFilePath:                dirname + "/.dockerconfigjson",
				OCIInsecureSkipTLSVerify:    true,
				DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
			}
			ctx := context.Background()
			src, err := ref.NewImageSource(ctx, sys)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func(src types.ImageSource) {
				err := src.Close()
				o.Expect(err).NotTo(o.HaveOccurred())
			}(src)
			rawManifest, _, err := src.GetManifest(ctx, nil)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(image, "scratch") {
				o.Expect(manifest.MIMETypeIsMultiImage(manifest.GuessMIMEType(rawManifest))).To(o.BeFalse())
			} else {
				o.Expect(manifest.MIMETypeIsMultiImage(manifest.GuessMIMEType(rawManifest))).To(o.BeTrue())
			}
		}

	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Critical-73359-Validate mirror2mirror for operator for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case73359"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73359")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73359")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73359.yaml")

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-redhatcatalog73359-v4-14", "openshift-marketplace", "Running")

		exutil.By("Install the operator from the new catalogsource")
		localstorageSub, localstorageOG := getOperatorInfo(oc, "local-storage-operator", "openshift-local-storage", "registry.redhat.io/redhat/redhat-operator-index:v4.14", "cs-redhatcatalog73359-v4-14")
		defer removeOperatorFromCustomCS(oc, localstorageSub, localstorageOG, "openshift-local-storage")
		installOperatorFromCustomCS(oc, localstorageSub, localstorageOG, "openshift-local-storage", "local-storage-operator")
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-73452-Validate mirror2mirror for OCI operator  and addition image for v2 [Serial]", func() {
		exutil.By("Set registry config")
		dirname := "/tmp/case73452"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73452")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73452")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73452.yaml")

		exutil.By("Skopeo oci to localhost")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 oci://%s  --remove-signatures", dirname+"/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		exutil.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		exutil.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		exutil.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-ocicatalog73452-v14", "openshift-marketplace", "Running")

		exutil.By("Install the operator from the new catalogsource")
		rhssoSub, rhssoOG := getOperatorInfo(oc, "rhsso-operator", "openshift-rhsso-operator", "registry.redhat.io/redhat/redhat-operator-index:v4.15", "cs-ocicatalog73452-v14")
		defer removeOperatorFromCustomCS(oc, rhssoSub, rhssoOG, "openshift-rhsso-operator")
		installOperatorFromCustomCS(oc, rhssoSub, rhssoOG, "openshift-rhsso-operator", "rhsso-operator")
	})
})
