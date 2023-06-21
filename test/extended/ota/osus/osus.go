package osus

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-updates] OTA osus should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("osus", exutil.KubeConfigPath())

	//author: jiajliu@redhat.com
	g.It("Author:jiajliu-High-35869-install/uninstall osus operator from OperatorHub through CLI", func() {

		testDataDir := exutil.FixturePath("testdata", "ota/osus")
		ogTemp := filepath.Join(testDataDir, "operatorgroup.yaml")
		subTemp := filepath.Join(testDataDir, "subscription.yaml")

		oc.SetupProject()

		og := operatorGroup{
			name:      "osus-og",
			namespace: oc.Namespace(),
			template:  ogTemp,
		}

		sub := subscription{
			name:            "osus-sub",
			namespace:       oc.Namespace(),
			channel:         "v1",
			approval:        "Automatic",
			operatorName:    "cincinnati-operator",
			sourceName:      "qe-app-registry",
			sourceNamespace: "openshift-marketplace",
			template:        subTemp,
		}

		g.By("Create OperatorGroup...")
		og.create(oc)

		g.By("Create Subscription...")
		sub.create(oc)

		g.By("Check updateservice operator installed successully!")
		e2e.Logf("Waiting for osus operator pod creating...")
		err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector=name=updateservice-operator", "-n", oc.Namespace()).Output()
			if err != nil || strings.Contains(output, "No resources found") {
				e2e.Logf("error: %v; output: %w", err, output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod with name=updateservice-operator is not found")

		e2e.Logf("Waiting for osus operator pod running...")
		err = wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
			status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector=name=updateservice-operator", "-n", oc.Namespace(), "-o=jsonpath={.items[0].status.phase}").Output()
			if err != nil || strings.Compare(status, "Running") != 0 {
				e2e.Logf("error: %v; status: %w", err, status)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod with name=updateservice-operator is not Running")

		g.By("Delete OperatorGroup...")
		og.delete(oc)

		g.By("Delete Subscription...")
		sub.delete(oc)

		g.By("Delete CSV...")
		installedCSV, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", sub.namespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(installedCSV).NotTo(o.BeEmpty())
		removeResource(oc, "-n", sub.namespace, "csv", installedCSV)

		g.By("Check updateservice operator uninstalled successully!")
		err = wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("all", "-n", oc.Namespace()).Output()
			if err != nil || !strings.Contains(output, "No resources found") {
				e2e.Logf("error: %v; output: %w", err, output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "updateservice operator is not uninstalled")
	})

})

var _ = g.Describe("[sig-updates] OTA osus instance should", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("osusinstace", exutil.KubeConfigPath())
		testDataDir string
		ogTemp      string
		subTemp     string
		operatorPod string
	)

	g.BeforeEach(func() {
		testDataDir = exutil.FixturePath("testdata", "ota/osus")
		ogTemp = filepath.Join(testDataDir, "operatorgroup.yaml")
		subTemp = filepath.Join(testDataDir, "subscription.yaml")
		operatorPod = "name=updateservice-operator"

		g.By("Install OSUS operator")

		oc.SetupProject()

		og := operatorGroup{
			name:      "osus-og",
			namespace: oc.Namespace(),
			template:  ogTemp,
		}

		sub := subscription{
			name:            "osus-sub",
			namespace:       oc.Namespace(),
			channel:         "v1",
			approval:        "Automatic",
			operatorName:    "cincinnati-operator",
			sourceName:      "qe-app-registry",
			sourceNamespace: "openshift-marketplace",
			template:        subTemp,
		}
		e2e.Logf("osus project is %s", oc.Namespace())

		og.create(oc)
		sub.create(oc)
		waitForPodReady(oc, operatorPod)
	})

	//author: yanyang@redhat.com
	g.It("NonHyperShiftHOST-DisconnectedOnly-Author:yanyang-High-62641-install/uninstall updateservice instance using oc-mirror [Disruptive][Serial]", func() {
		g.By("Mirror OCP release and graph data image by oc-mirror")
		registry, err := exutil.GetMirrorRegistry(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)

		dirname := "/tmp/case62641"
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		outdir, err := ocmirror(oc, registry+"/oc-mirror", dirname)
		e2e.Logf("oc mirror output dir is %s", outdir)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		certFile := dirname + "/cert"
		err = exutil.GetUserCAToFile(oc, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc)
		err = trustCert(oc, registry, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Install OSUS instance")
		defer uninstallOSUSApp(oc)
		err = installOSUSAppOCMirror(oc, outdir)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify OSUS instance works")
		err = verifyOSUS(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	})
})
