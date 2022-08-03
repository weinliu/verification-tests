package workloads

import (
	"os"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("ocmirror", exutil.KubeConfigPath())
	)
	g.It("Author:yinzhou-Medium-46517-List operator content with different options", func() {
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
			"integration-operator",
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
			"poison-pill-manager",
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
	g.It("Author:yinzhou-Medium-46818-Low-46523-check the User Agent for oc-mirror", func() {
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
	g.It("ConnectedOnly-Author:yinzhou-Medium-46770-Low-46520-Local backend support for oc-mirror", func() {
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "ocmirror-localbackend.yaml")

		dirname := "/tmp/46770test"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/46770test", "-v", "3").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out, "Using local backend at location") {
			e2e.Failf("Do not expect the backend setting")
		}
		_, err = os.Stat("/tmp/46770test/publish/.metadata.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("describe", "/tmp/46770test/mirror_seq1_000000.tar").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

})
