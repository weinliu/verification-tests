package osus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	arch "github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-updates] OTA osus should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("osus", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		exutil.SkipMissingQECatalogsource(oc)
		arch.SkipNonAmd64SingleArch(oc)
	})

	//author: jiajliu@redhat.com
	g.It("Author:jiajliu-High-35869-install/uninstall osus operator from OperatorHub through CLI [Serial]", func() {

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

		exutil.By("Create OperatorGroup...")
		og.create(oc)

		exutil.By("Create Subscription...")
		sub.create(oc)

		exutil.By("Check updateservice operator installed successully!")
		e2e.Logf("Waiting for osus operator pod creating...")
		err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector=name=updateservice-operator", "-n", oc.Namespace()).Output()
			if err != nil || strings.Contains(output, "No resources found") {
				e2e.Logf("error: %v; output: %s", err, output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod with name=updateservice-operator is not found")

		e2e.Logf("Waiting for osus operator pod running...")
		err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector=name=updateservice-operator", "-n", oc.Namespace(), "-o=jsonpath={.items[0].status.phase}").Output()
			if err != nil || strings.Compare(status, "Running") != 0 {
				e2e.Logf("error: %v; status: %s", err, status)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod with name=updateservice-operator is not Running")

		exutil.By("Delete OperatorGroup...")
		og.delete(oc)

		exutil.By("Delete Subscription...")
		sub.delete(oc)

		exutil.By("Delete CSV...")
		installedCSV, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", sub.namespace, "-o=jsonpath={.items[?(@.spec.displayName==\"OpenShift Update Service\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(installedCSV).NotTo(o.BeEmpty())
		removeResource(oc, "-n", sub.namespace, "csv", installedCSV)

		exutil.By("Check updateservice operator uninstalled successully!")
		err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("all", "-n", oc.Namespace()).Output()
			if err != nil || !strings.Contains(output, "No resources found") {
				e2e.Logf("error: %v; output: %s", err, output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "updateservice operator is not uninstalled")
	})

	//author: jiajliu@redhat.com
	g.It("NonPreRelease-Longduration-DisconnectedOnly-Author:jiajliu-High-44958-z version upgrade OSUS operator and operand for disconnected cluster [Disruptive]", func() {
		updatePath := map[string]string{
			"srcver": "5.0.2",
			"tgtver": "5.0.3",
		}
		tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))
		defer os.RemoveAll(tempDataDir)
		err := os.MkdirAll(tempDataDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		oc.SetupProject()

		exutil.By("Install osus operator with srcver")
		installOSUSOperator(oc, updatePath["srcver"], "Manual")
		preOPName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector=name=updateservice-operator", "-o=jsonpath={.items[*].metadata.name}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		csvInPrePod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", preOPName, "-o=jsonpath={.spec.containers[].env[?(@.name=='OPERATOR_CONDITION_NAME')].value}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(csvInPrePod).To(o.ContainSubstring(updatePath["srcver"]), "Unexpected operator version installed: %s.", csvInPrePod)

		exutil.By("Install OSUS instance")
		e2e.Logf("Mirror OCP release and graph data image by oc-mirror...")
		registry, err := exutil.GetMirrorRegistry(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		credDir, err := locatePodmanCred(oc, tempDataDir)
		defer os.RemoveAll(credDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		outdir, err := ocmirror(oc, registry+"/oc-mirror", tempDataDir, "")
		e2e.Logf("oc mirror output dir is %s", outdir)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Configure the Registry Certificate as trusted for cincinnati...")
		certFile := tempDataDir + "/cert"
		err = exutil.GetUserCAToFile(oc, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA)
		err = trustCert(oc, registry, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Create updateservice...")
		defer uninstallOSUSApp(oc)
		err = installOSUSAppOCMirror(oc, outdir)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = verifyOSUS(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("z-version upgrade against operator and operand")
		err = upgradeOSUS(oc, "update-service-oc-mirror", updatePath["tgtver"])
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	//author: jiajliu@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:jiajliu-High-69204-y version upgrade OSUS operator and operand for connected cluster", func() {
		updatePath := map[string]string{
			"srcver": "4.9.1",
			"tgtver": "5.0.1",
		}
		oc.SetupProject()
		skipUnsupportedOCPVer(oc, updatePath["srcver"])
		exutil.By("Install osus operator with srcver")
		installOSUSOperator(oc, updatePath["srcver"], "Manual")
		preOPName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector=name=updateservice-operator", "-o=jsonpath={.items[*].metadata.name}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		csvInPrePod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", preOPName, "-o=jsonpath={.spec.containers[].env[?(@.name=='OPERATOR_CONDITION_NAME')].value}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(csvInPrePod).To(o.ContainSubstring(updatePath["srcver"]), "Unexpected operator version installed: %s.", csvInPrePod)

		exutil.By("Install OSUS instance")
		usTemp := exutil.FixturePath("testdata", "ota", "osus", "updateservice.yaml")
		us := updateService{
			name:      "us69204",
			namespace: oc.Namespace(),
			template:  usTemp,
			graphdata: "quay.io/openshift-qe-optional-operators/graph-data:latest",
			releases:  "quay.io/openshifttest/ocp-release",
			replicas:  1,
		}
		defer uninstallOSUSApp(oc)
		err = installOSUSAppOC(oc, us)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify OSUS instance works")
		err = verifyOSUS(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Y-version upgrade against operator and operand")
		err = upgradeOSUS(oc, us.name, updatePath["tgtver"])
		o.Expect(err).NotTo(o.HaveOccurred())
	})

})

var _ = g.Describe("[sig-updates] OTA osus instance should", func() {
	defer g.GinkgoRecover()

	oc := exutil.NewCLI("osusinstace", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		exutil.SkipMissingQECatalogsource(oc)
		arch.SkipNonAmd64SingleArch(oc)
		oc.SetupProject()
		installOSUSOperator(oc, "", "Automatic")
	})

	//author: jianl@redhat.com
	g.It("NonPreRelease-Longduration-DisconnectedOnly-Author:jianl-High-62641-install/uninstall updateservice instance using oc-mirror [Disruptive]", func() {
		exutil.By("Mirror OCP release and graph data image by oc-mirror")
		registry, err := exutil.GetMirrorRegistry(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)

		dirname := "/tmp/case62641"
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		credDir, err := locatePodmanCred(oc, dirname)
		defer os.RemoveAll(credDir)
		o.Expect(err).NotTo(o.HaveOccurred())

		outdir, err := ocmirror(oc, registry+"/oc-mirror", dirname, "")
		e2e.Logf("oc mirror output dir is %s", outdir)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		certFile := dirname + "/cert"
		err = exutil.GetUserCAToFile(oc, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA)
		err = trustCert(oc, registry, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Install OSUS instance")
		defer uninstallOSUSApp(oc)
		err = installOSUSAppOCMirror(oc, outdir)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify OSUS instance works")
		err = verifyOSUS(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	//author: jianl@redhat.com
	g.It("DisconnectedOnly-VMonly-Author:jianl-High-35944-install/uninstall updateservice instance and build graph image as non root [Disruptive]", func() {
		exutil.By("Check if it's a AWS/GCP/Azure cluster")
		exutil.SkipIfPlatformTypeNot(oc, "gcp, aws, azure")

		dirname := "/tmp/case35944"
		registry, err := exutil.GetMirrorRegistry(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)

		defer os.RemoveAll(dirname)
		err = exutil.GetPullSec(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Build and push graph data image by podman as non root user")
		graphdataTag := registry + "/ota-35944/graph-data:latest"
		err = buildPushGraphImage(oc, graphdataTag, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Mirror OCP images using oc adm release mirror")
		err = mirror(oc, registry, "quay.io/openshift-release-dev/ocp-release:4.13.0-x86_64", dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Configure the Registry Certificate as trusted for cincinnati")
		certFile := dirname + "/cert"
		err = exutil.GetUserCAToFile(oc, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA)
		err = trustCert(oc, registry, certFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Install OSUS instance")
		usTemp := exutil.FixturePath("testdata", "ota", "osus", "updateservice.yaml")
		us := updateService{
			name:      "update-service-35944",
			namespace: oc.Namespace(),
			template:  usTemp,
			graphdata: graphdataTag,
			releases:  registry + "/ocp-release",
			replicas:  2,
		}
		defer uninstallOSUSApp(oc)
		err = installOSUSAppOC(oc, us)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify OSUS instance works")
		err = verifyOSUS(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	//author: jianl@redhat.com
	g.It("ConnectedOnly-Author:jianl-High-52596-High-59687-install/uninstall updateservice instance on a connected/http/https proxy cluster", func() {
		dirname := "/tmp/" + oc.Namespace() + "-osus"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Install OSUS instance")
		//We need to build and push the latest graph-data if there is new feature to the container
		usTemp := exutil.FixturePath("testdata", "ota", "osus", "updateservice.yaml")
		us := updateService{
			name:      "update-service-52596",
			namespace: oc.Namespace(),
			template:  usTemp,
			graphdata: "quay.io/openshift-qe-optional-operators/graph-data:latest",
			releases:  "quay.io/openshift-qe-optional-operators/osus-ocp-release",
			replicas:  2,
		}
		defer uninstallOSUSApp(oc)
		err = installOSUSAppOC(oc, us)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Verify OSUS instance works")
		err = verifyOSUS(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-ConnectedOnly-Author:jiajliu-High-48621-Updateservice pod should be re-deployed when update graphDataImage of updateservice", func() {
		exutil.By("Install OSUS instance with graph-data:1.0")
		usTemp := exutil.FixturePath("testdata", "ota", "osus", "updateservice.yaml")
		us := updateService{
			name:      "us48621",
			namespace: oc.Namespace(),
			template:  usTemp,
			graphdata: "quay.io/openshift-qe-optional-operators/graph-data:1.0",
			releases:  "quay.io/openshift-qe-optional-operators/osus-ocp-release",
			replicas:  1,
		}
		defer uninstallOSUSApp(oc)
		err := installOSUSAppOC(oc, us)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Waiting for osus instance pod rolling to expected replicas...")
		err = wait.Poll(1*time.Minute, 10*time.Minute, func() (bool, error) {
			runningPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
			if err != nil || len(strings.Fields(runningPodName)) != us.replicas {
				e2e.Logf("error: %v; running pod: %s", err, runningPodName)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod is not rolling to expected replicas")

		runningPodNamePre, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		graphDataImagePre, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", runningPodNamePre, "-n", us.namespace, "-o=jsonpath={.spec.initContainers[].image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(graphDataImagePre).To(o.ContainSubstring("1.0"))

		exutil.By("Update OSUS instance with graph-data:1.1")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", us.namespace, "updateservice/"+us.name, "-p", `{"spec":{"graphDataImage":"quay.io/openshift-qe-optional-operators/graph-data:1.1"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Waiting for osus instance pod rolling...")
		err = wait.Poll(1*time.Minute, 10*time.Minute, func() (bool, error) {
			runningPodNamePost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
			if err != nil || len(strings.Fields(runningPodNamePost)) != us.replicas || strings.Contains(runningPodNamePost, runningPodNamePre) {
				e2e.Logf("error: %v; running pod after update image: %s; while running pod before update image: %s", err, runningPodNamePost, runningPodNamePre)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod is not rolling successfully after update image")
		runningPodNamePost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		graphDataImagePost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", runningPodNamePost, "-n", us.namespace, "-o=jsonpath={.spec.initContainers[].image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(graphDataImagePost).To(o.ContainSubstring("1.1"))
	})

	//author: jiajliu@redhat.com
	g.It("Longduration-NonPreRelease-ConnectedOnly-VMonly-Author:jiajliu-High-52586-Updateservice pod should pull the latest graphDataImage instead of existed old one [Disruptive]", func() {
		tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))
		defer os.RemoveAll(tempDataDir)
		err := os.MkdirAll(tempDataDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.GetPullSec(oc, tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		authFile := tempDataDir + "/.dockerconfigjson"

		podmanCLI := container.NewPodmanCLI()
		graphdataRepo := "quay.io/openshift-qe-optional-operators/graph-data"
		graphdataOld := graphdataRepo + ":1.0"
		graphdataNew := graphdataRepo + ":1.1"

		usTemp := exutil.FixturePath("testdata", "ota", "osus", "updateservice.yaml")
		us := updateService{
			name:      "us52586",
			namespace: oc.Namespace(),
			template:  usTemp,
			graphdata: graphdataRepo + ":latest",
			releases:  "quay.io/openshift-qe-optional-operators/osus-ocp-release",
			replicas:  1,
		}
		exutil.By("Tag image graph-data:1.0 with latest and push the image")
		output, err := podmanCLI.Run("pull").Args(graphdataOld, "--tls-verify=false", "--authfile", authFile).Output()
		defer podmanCLI.RemoveImage(graphdataOld)
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to pull image: %s", output)

		output, err = podmanCLI.Run("tag").Args(graphdataOld, us.graphdata).Output()
		defer podmanCLI.RemoveImage(us.graphdata)
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to tag image: %s", output)

		output, err = podmanCLI.Run("push").Args(us.graphdata, "--tls-verify=false", "--authfile", authFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to push image: %s", output)

		exutil.By("Install OSUS instance with graph-data:latest")
		defer uninstallOSUSApp(oc)
		err = installOSUSAppOC(oc, us)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Waiting for osus instance pod rolling to expected replicas...")
		err = wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
			runningPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
			if err != nil || len(strings.Fields(runningPodName)) != us.replicas {
				e2e.Logf("error: %v; running pod: %s", err, runningPodName)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod is not rolling to expected replicas")
		runningPodNamePre, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Check imagePullPolicy...")
		graphDataImagePolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", runningPodNamePre, "-n", us.namespace, "-o=jsonpath={.spec.initContainers[].imagePullPolicy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(graphDataImagePolicy).To(o.Equal("Always"), "Unexpected imagePullPolicy: %v", graphDataImagePolicy)

		nodeNamePre, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", us.namespace, runningPodNamePre, "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		graphDataImageIDPre, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", runningPodNamePre, "-n", us.namespace, "-o=jsonpath={.status.initContainerStatuses[].imageID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Cordon worker nodes without osus instance pod scheduled")
		nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			for _, node := range strings.Fields(nodes) {
				if node == nodeNamePre {
					continue
				}
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", node).Execute()
				err = wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
					nodeReady, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
					if err != nil || nodeReady != "True" {
						e2e.Logf("error: %v; node %s status: %s", err, node, nodeReady)
						return false, nil
					}
					return true, nil
				})
				exutil.AssertWaitPollNoErr(err, "fail to uncordon node!")
			}
		}()

		for _, node := range strings.Fields(nodes) {
			if node == nodeNamePre {
				continue
			}
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", node).Execute()
			o.Expect(err).NotTo(o.HaveOccurred(), "fail to cordon node %s: %v", node, err)
		}

		exutil.By("Tag image graph-data:1.1 with latest and push the image")
		output, err = podmanCLI.Run("pull").Args(graphdataNew, "--tls-verify=false", "--authfile", authFile).Output()
		defer podmanCLI.RemoveImage(graphdataNew)
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to pull image: %s", output)

		output, err = podmanCLI.Run("tag").Args(graphdataNew, us.graphdata).Output()
		defer podmanCLI.RemoveImage(us.graphdata)
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to tag image: %s", output)

		output, err = podmanCLI.Run("push").Args(us.graphdata, "--tls-verify=false", "--authfile", authFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "fail to push image: %s", output)

		e2e.Logf("Waiting for osus instance pod rolling...")
		err = wait.Poll(30*time.Second, 600*time.Second, func() (bool, error) {
			runningPodNamePost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
			if err != nil || strings.Contains(runningPodNamePost, runningPodNamePre) {
				e2e.Logf("error: %v; running pod after update image: %s; while running pod before retag image: %s", err, runningPodNamePost, runningPodNamePre)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod is not rolling successfully after retag image")

		runningPodNamePost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector", "app="+us.name, "-n", us.namespace, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Check osus instance pod is not rescheduled...")
		nodeNamePost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", us.namespace, runningPodNamePost, "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeNamePost).To(o.Equal(nodeNamePre), "osus instance pod rescheduled from node %v to node %s unexpectedly", nodeNamePre, nodeNamePost)

		e2e.Logf("Check osus instance pod image updated...")
		graphDataImageIDPost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", runningPodNamePost, "-n", us.namespace, "-o=jsonpath={.status.initContainerStatuses[].imageID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(graphDataImageIDPost).NotTo(o.Equal(graphDataImageIDPre), "fail to update osus instance pod image")
	})

	//author: jiajliu@redhat.com
	g.It("NonPreRelease-Longduration-DisconnectedOnly-Author:jiajliu-High-69648-[disconnect]deploy osus against graph-data with signatures created by oc-mirror [Disruptive]", func() {
		tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))
		defer os.RemoveAll(tempDataDir)
		o.Expect(os.MkdirAll(tempDataDir, 0755)).NotTo(o.HaveOccurred())
		imagesetcfg1 := exutil.FixturePath("testdata", "ota", "osus", "ocp-69648", "imageset-cfg1.yaml")
		imagesetcfg2 := exutil.FixturePath("testdata", "ota", "osus", "ocp-69648", "imageset-cfg2.yaml")
		usname := "update-service-oc-mirror"

		exutil.By("Mirror one OCP release and graph data image with imageset-cfg1.yaml...")
		registry, err := exutil.GetMirrorRegistry(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		credDir, err := locatePodmanCred(oc, tempDataDir)
		defer os.RemoveAll(credDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Mirror release payload with oc-mirror...")
		outdir, err := ocmirror(oc, registry+"/oc-mirror", tempDataDir, imagesetcfg1)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Configure the Registry Certificate as trusted for cincinnati...")
		certFile := tempDataDir + "/cert"
		o.Expect(exutil.GetUserCAToFile(oc, certFile)).NotTo(o.HaveOccurred())
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA)
		o.Expect(trustCert(oc, registry, certFile)).NotTo(o.HaveOccurred())

		exutil.By("Create updateservice and verify it works...")
		defer uninstallOSUSApp(oc)
		o.Expect(installOSUSAppOCMirror(oc, outdir)).NotTo(o.HaveOccurred())
		o.Expect(verifyOSUS(oc)).NotTo(o.HaveOccurred())
		runningPodNamePre, _ := oc.AsAdmin().Run("get").Args("pods", "--selector", "app="+usname, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
		e2e.Logf("Running app pods: %s", runningPodNamePre)
		o.Expect(runningPodNamePre).NotTo(o.BeEmpty())
		runningPodPre := strings.Fields(runningPodNamePre)
		e2e.Logf("Running app pods list: %s", runningPodPre)
		digests, err := checkMetadata(oc, runningPodPre[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check only one signature file found...")
		o.Expect(len(digests)).To(o.Equal(1), "expected one signature file, actual signature files: %s", digests)

		exutil.By("Re-mirror two OCP release and graph data image with imageset-cfg2.yaml...")
		outdir, err = ocmirror(oc, registry+"/oc-mirror", tempDataDir, imagesetcfg2)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check updateservice re-deployed and works well...")
		o.Expect(oc.AsAdmin().Run("apply").Args("-f", outdir+"/updateService.yaml").Execute()).NotTo(o.HaveOccurred())
		runningPodMid, err := verifyAppRolling(oc, usname, runningPodPre)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(verifyOSUS(oc)).NotTo(o.HaveOccurred())
		digests, err = checkMetadata(oc, runningPodMid[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check two signature files found...")
		o.Expect(len(digests)).To(o.Equal(2), "expected two signature files, actual signature files: %s", digests)
	})
})
