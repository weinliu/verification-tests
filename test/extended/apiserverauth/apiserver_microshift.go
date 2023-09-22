package apiserverauth

import (
	"fmt"
	"os"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-api-machinery] API_Server on Microshift", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("default")
	var tmpdir string

	g.JustBeforeEach(func() {
		tmpdir = "/tmp/-OCP-microshift-apiseerver-cases-" + exutil.GetRandomString() + "/"
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.JustAfterEach(func() {
		os.RemoveAll(tmpdir)
		logger.Infof("test dir %s is cleaned up", tmpdir)
	})

	// author: rgangwar@redhat.com
	g.It("MicroShiftOnly-Longduration-NonPreRelease-Author:rgangwar-Medium-63298-[Apiserver] manifest directory scanning [Disruptive][Slow]", func() {
		var (
			e2eTestNamespace = "microshift-ocp63298"
			etcConfigYaml    = "/etc/microshift/config.yaml"
			etcConfigYamlbak = "/etc/microshift/config.yaml.bak"
			tmpManifestPath  = "/etc/microshift/manifests.d/my-app/base /etc/microshift/manifests.d/my-app/dev /etc/microshift/manifests.d/my-app/dev/patches/"
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "hello-openshift-dev-app-ocp63298", "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "busybox-base-app-ocp63298", "--ignore-not-found").Execute()
		}()

		defer func() {
			etcConfigCMD := fmt.Sprintf(`configfile=%v;
			configfilebak=%v;
			if [ -f $configfilebak ]; then
				cp $configfilebak $configfile; 
				rm -f $configfilebak;
			else
				rm -f $configfile;
			fi`, etcConfigYaml, etcConfigYamlbak)
			_, mchgConfigErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNodes[0], []string{"--quiet=true", "--to-namespace=" + e2eTestNamespace}, "bash", "-c", etcConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			restartMicroshift(oc, masterNodes[0])
		}()

		defer func() {
			_, mchgConfigErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNodes[0], []string{"--quiet=true", "--to-namespace=" + e2eTestNamespace}, "bash", "-c", "sudo rm -rf "+tmpManifestPath)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("3. Take backup of config file")
		etcConfig := fmt.Sprintf(`configfile=%v;
		configfilebak=%v;
		if [ -f $configfile ]; then 
			cp $configfile $configfilebak;
		fi`, etcConfigYaml, etcConfigYamlbak)
		_, mchgConfigErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNodes[0], []string{"--quiet=true", "--to-namespace=" + e2eTestNamespace}, "bash", "-c", etcConfig)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		exutil.By("4. Create tmp manifest path on node")
		_, dirErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNodes[0], []string{"--quiet=true", "--to-namespace=" + e2eTestNamespace}, "bash", "-c", "sudo mkdir -p "+tmpManifestPath)
		o.Expect(dirErr).NotTo(o.HaveOccurred())

		//  Setting glob path values to multiple values should load manifests from all of them.
		exutil.By("4.1 Set glob path values to the manifest option in config")
		etcConfig = fmt.Sprintf(`
manifests:
  kustomizePaths:
  - /etc/microshift/manifests.d/my-app/*/
  - /etc/microshift/manifests.d/my-app/*/patches`)
		changeMicroshiftConfig(oc, etcConfig, masterNodes[0], e2eTestNamespace, etcConfigYaml)

		newSrcFiles := map[string][]string{
			"busybox.yaml": {
				"microshift-busybox-deployment.yaml",
				"/etc/microshift/manifests.d/my-app/base/",
				"NAMESPACEVAR",
				"base-app-ocp63298",
			},
			"kustomization.yaml": {
				"microshift-busybox-kustomization.yaml",
				"/etc/microshift/manifests.d/my-app/base/",
				"NAMESPACEVAR",
				"base-app-ocp63298",
			},
			"hello-openshift.yaml": {
				"microshift-hello-openshift.yaml",
				"/etc/microshift/manifests.d/my-app/dev/patches/",
				"NAMESPACEVAR",
				"dev-app-ocp63298",
			},
			"kustomization": {
				"microshift-hello-openshift-kustomization.yaml",
				"/etc/microshift/manifests.d/my-app/dev/patches/",
				"NAMESPACEVAR",
				"dev-app-ocp63298",
			},
		}
		exutil.By("4.2 Create kustomization and deployemnt files")
		addKustomizationToMicroshift(oc, masterNodes[0], e2eTestNamespace, newSrcFiles)
		restartMicroshift(oc, masterNodes[0])

		exutil.By("4.3 Check pods after microshift restart")
		podsOutput := getPodsList(oc, "hello-openshift-dev-app-ocp63298")
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Test case :: Failed :: Pods are not created, manifests are not loaded from defined location")
		podsOutput = getPodsList(oc, "busybox-base-app-ocp63298")
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Test case :: Failed :: Pods are not created, manifests are not loaded from defined location")
		e2e.Logf("Test case :: Passed :: Pods are created, manifests are loaded from defined location :: %s", podsOutput[0])
	})
})
