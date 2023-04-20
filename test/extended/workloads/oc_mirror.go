package workloads

import (
	"fmt"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("ocmirror", exutil.KubeConfigPath())
	)
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-Medium-46517-List operator content with different options", func() {
		dirname := "/tmp/case46517"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		dockerCreFile, homePath, err := locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			os.RemoveAll(dockerCreFile)
			_, err = os.Stat(homePath + "/.docker/config.json.back")
			if err == nil {
				copyFile(homePath+"/.docker/config.json.back", homePath+"/.docker/config.json")
			}
		}()

		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--version=4.11").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMessage := []string{
			"registry.redhat.io/redhat/redhat-operator-index:v4.11",
			"registry.redhat.io/redhat/certified-operator-index:v4.11",
			"registry.redhat.io/redhat/community-operator-index:v4.11",
			"registry.redhat.io/redhat/redhat-marketplace-index:v4.11",
		}
		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		out, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--version=4.11", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMessage = []string{
			"3scale-operator",
			"amq-online",
			"amq-streams",
			"amq7-interconnect-operator",
			"ansible-automation-platform-operator",
			"ansible-cloud-addons-operator",
			"apicast-operator",
			"businessautomation-operator",
			"cincinnati-operator",
			"cluster-logging",
			"compliance-operator",
			"container-security-operator",
			"costmanagement-metrics-operator",
			"cryostat-operator",
			"datagrid",
			"devworkspace-operator",
			"eap",
			"elasticsearch-operator",
			"external-dns-operator",
			"file-integrity-operator",
			"fuse-apicurito",
			"fuse-console",
			"fuse-online",
			"gatekeeper-operator-product",
			"jaeger-product",
			"jws-operator",
			"kiali-ossm",
			"kubevirt-hyperconverged",
			"mcg-operator",
			"mtc-operator",
			"mtv-operator",
			"node-healthcheck-operator",
			"node-maintenance-operator",
			"ocs-operator",
			"odf-csi-addons-operator",
			"odf-lvm-operator",
			"odf-multicluster-orchestrator",
			"odf-operator",
			"odr-cluster-operator",
			"odr-hub-operator",
			"openshift-cert-manager-operator",
			"openshift-gitops-operator",
			"openshift-pipelines-operator-rh",
			"openshift-secondary-scheduler-operator",
			"opentelemetry-product",
			"quay-bridge-operator",
			"quay-operator",
			"red-hat-camel-k",
			"redhat-oadp-operator",
			"rh-service-binding-operator",
			"rhacs-operator",
			"rhpam-kogito-operator",
			"rhsso-operator",
			"sandboxed-containers-operator",
			"serverless-operator",
			"service-registry-operator",
			"servicemeshoperator",
			"skupper-operator",
			"submariner",
			"web-terminal",
		}

		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11", "--package=cluster-logging", "--channel=stable").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11", "--package=cluster-logging").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

	})
	g.It("ConnectedOnly-Author:yinzhou-Medium-46818-Low-46523-check the User Agent for oc-mirror", func() {
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "catlog-loggings.yaml")

		dirname := "/tmp/case46523"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("/tmp/case46523/oc-mirror-workspace")
		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/case46523", "-v", "7", "--dry-run").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//check user-agent and dry-run should write mapping file
		checkMessage := []string{
			"User-Agent: oc-mirror",
			"Writing image mapping",
		}
		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		_, err = os.Stat("/tmp/case46523/oc-mirror-workspace/mapping.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-Medium-46770-Low-46520-Local backend support for oc-mirror", func() {
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "ocmirror-localbackend.yaml")

		dirname := "/tmp/46770test"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {

			out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/46770test", "--continue-on-error", "-v", "3").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !strings.Contains(out, "Using local backend at location") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but the oc-mirror still failed"))

		_, err = os.Stat("/tmp/46770test/publish/.metadata.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("describe", "/tmp/46770test/mirror_seq1_000000.tar").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-46506-High-46817-Mirror a single image works well [Serial]", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI)
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "config_singleimage.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("Mirror to registry")
		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out, "using stateless mode") {
			e2e.Failf("Can't see the stateless mode log")
		}
		g.By("Mirror to localhost")
		dirname := "/tmp/46506test"
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		out1, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/46506test").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out1, "using stateless mode") {
			e2e.Failf("Can't see the stateless mode log")
		}
		_, err = os.Stat("/tmp/46506test/mirror_seq1_000000.tar")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Mirror to registry from archive")
		defer os.RemoveAll("oc-mirror-workspace")
		out2, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "/tmp/46506test/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName+"/mirrorachive", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out2, "using stateless mode") {
			e2e.Failf("Can't see the stateless mode log")
		}
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-Low-51093-oc-mirror init", func() {
		g.By("Set podman registry config")
		dirname := "/tmp/case51093"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("init").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out, "local") {
			e2e.Failf("Can't find the storageconfig of local")
		}
		out1, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("init", "--registry", "localhost:5000/test:latest").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out1, "registry") {
			e2e.Failf("Can't find the storageconfig of registry")
		}
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("init", "--registry", "localhost:5000/test:latest", "--output", "json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-46769-Critical-46515-High-registry backend test [Serial]", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI)
		g.By("Set podman registry config")
		dirname := "/tmp/case46769"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorConfigS := filepath.Join(ocmirrorBaseDir, "registry_backend_operator_helm.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, operatorConfigS)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Mirroring selected operator and helm image")
		defer os.RemoveAll("oc-mirror-workspace")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", operatorConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--continue-on-error").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("NonHyperShiftHOST-Author:yinzhou-NonPreRelease-Medium-37372-High-40322-oc adm release extract pull from localregistry when given a localregistry image [Disruptive]", func() {
		var imageDigest string
		g.By("Set podman registry config")
		dirname := "/tmp/case37372"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ocpPlatformConfigS := filepath.Join(ocmirrorBaseDir, "registry_backend_ocp_latest.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, ocpPlatformConfigS)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeOcMirrorLog()
		g.By("Create the mapping file by oc-mirror dry-run command")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ocpPlatformConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Checkpoint for 40322, mirror with mapping")
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "-f", "oc-mirror-workspace/mapping.txt", "--max-per-registry", "1", "--skip-multiple-scopes=true", "--insecure").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check for the mirrored image and get the image digest")
		imageDigest = getDigestFromImageInfo(oc, serInfo.serviceName)

		g.By("Run oc-mirror to create ICSP file")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ocpPlatformConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Checkpoint for 37372")
		g.By("Remove the podman Cred")
		os.RemoveAll(dirname)
		g.By("Try to extract without icsp file, will failed")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc", "--to=oc-mirror-workspace/", serInfo.serviceName+"/openshift/release-images"+imageDigest, "--insecure").Execute()
		o.Expect(err).Should(o.HaveOccurred())
		g.By("Try to extract with icsp file, will extract from localregisty")
		imageContentSourcePolicy := findImageContentSourcePolicy()
		waitErr := wait.Poll(120*time.Second, 600*time.Second, func() (bool, error) {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc", "--to=oc-mirror-workspace/", "--icsp-file="+imageContentSourcePolicy, serInfo.serviceName+"/openshift/release-images"+"@"+imageDigest, "--insecure").Execute()
			if err != nil {
				e2e.Logf("mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the mirror still failed"))
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-NonPreRelease-Medium-46518-List ocp release content with different options", func() {
		g.By("Set podman registry config")
		dirname := "/tmp/case46518"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("List releases for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List release channels for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channels").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List available releases from channel candidate for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channel=candidate-4.11").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List available releases from channel candidate for ocp 4.11 and specify arch arm64")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channel=candidate-4.11", "--filter-by-archs=arm64").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-NonPreRelease-Medium-60594-ImageSetConfig containing OCI FBC and release platform and additionalImages works well with --use-oci-feature flag [Serial]", func() {
		err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("version").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry config")
		dirname := "/tmp/case60594"
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.12 oci://%s  --remove-signatures", dirname+"/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ociFullConfig := filepath.Join(ocmirrorBaseDir, "config-oci-all.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll(".oc-mirror.log")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFullConfig, "docker://"+serInfo.serviceName, "--use-oci-feature", "--dest-skip-tls", "--dry-run").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Longduration-Author:yinzhou-NonPreRelease-Medium-60597-Critical-60595-oc-mirror support for TargetCatalog field for operator[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60597"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.12 oci://%s  --remove-signatures", dirname+"/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads/config-60597")
		normalTargetConfig := filepath.Join(ocmirrorBaseDir, "config-60597-normal-target.yaml")
		ociTargetTagConfig := filepath.Join(ocmirrorBaseDir, "config-60597-oci-target-tag.yaml")
		normalConfig := filepath.Join(ocmirrorBaseDir, "config-60597-normal.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalTargetConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(serInfo.serviceName+"/abc/redhat-operator-index:v4.12", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociTargetTagConfig, "docker://"+serInfo.serviceName, "--use-oci-feature", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("/mno/redhat-operator-index:v5", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociTargetTagConfig, "docker://"+serInfo.serviceName+"/ocit", "--use-oci-feature", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("/ocit/mno/redhat-operator-index:v5", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(serInfo.serviceName+"/redhat/redhat-operator-index:v4.12", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalConfig, "docker://"+serInfo.serviceName+"/testname", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(serInfo.serviceName+"/testname/redhat/redhat-operator-index:v4.12", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		g.By("Checkpoint for 60595")
		ocmirrorDir := exutil.FixturePath("testdata", "workloads")
		ociFirstConfig := filepath.Join(ocmirrorDir, "config-oci-f.yaml")
		ociSecondConfig := filepath.Join(ocmirrorDir, "config-oci-s.yaml")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFirstConfig, "docker://"+serInfo.serviceName, "--use-oci-feature", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociSecondConfig, "docker://"+serInfo.serviceName, "--use-oci-feature", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", output); !matched {
			e2e.Failf("Can't find the prune log\n")
		}
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-NonPreRelease-Low-60603-oc-mirror negative test", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads/config-60603")
		configOCI := filepath.Join(buildPruningBaseDir, "config-oci.yaml")
		configNormal := filepath.Join(buildPruningBaseDir, "config-normal.yaml")

		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configOCI, "--use-oci-feature", "file://").Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("oci feature cannot be used when mirroring to local archive"))
		output, err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configOCI, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err1).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("configuration file is authorized only with flag --use-oci-feature"))
		output, err2 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configNormal, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--use-oci-feature").Output()
		o.Expect(err2).Should(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("please execute without the --use-oci-feature flag"))
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Longduration-Author:yinzhou-NonPreRelease-Medium-60607-oc mirror purne for mirror2disk and mirror2mirror with and without skip-pruning[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60607"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads/config-60603")
		configFirst := filepath.Join(buildPruningBaseDir, "config-normal-first.yaml")
		configSecond := filepath.Join(buildPruningBaseDir, "config-normal-second.yaml")
		configThird := filepath.Join(buildPruningBaseDir, "config-normal-third.yaml")

		fileList := []string{configFirst, configSecond, configThird}
		for _, file := range fileList {
			sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, file)
			_, err = exec.Command("bash", "-c", sedCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")

		defer os.RemoveAll("mirror_seq1_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configFirst, "file://").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the tag for mirrored image")
		checkCmd := fmt.Sprintf(`curl -k 'https://%s/v2/openshift4/ose-cluster-kube-descheduler-operator/tags/list'`, serInfo.serviceName)
		output, err := exec.Command("bash", "-c", checkCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("null"))
		defer os.RemoveAll("mirror_seq2_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configSecond, "file://").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		outputMirror, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq2_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", outputMirror); !matched {
			e2e.Failf("Can't find the prune log\n")
		}
		g.By("Check the tag again, should be null")
		outputNew, err := exec.Command("bash", "-c", checkCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputNew).To(o.ContainSubstring("null"))
		defer os.RemoveAll("mirror_seq3_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configThird, "file://").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		outputMirror, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq3_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--skip-pruning").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", outputMirror); matched {
			e2e.Failf("Should not find the prune log\n")
		}
	})
})
