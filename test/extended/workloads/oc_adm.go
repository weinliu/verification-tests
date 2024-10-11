package workloads

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cli] Workloads oc adm command works well", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
	)

	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-10618-Prune old builds by admin command [Serial]", func() {
		if checkOpenshiftSamples(oc) {
			g.Skip("Can't find the cluster operator openshift-samples, skip it.")
		}
		// Skip the case if cluster is C2S/SC2S disconnected as external network cannot be accessed
		if strings.HasPrefix(getClusterRegion(oc), "us-iso") {
			g.Skip("Skipped: AWS C2S/SC2S disconnected clusters are not satisfied for this test case")
		}

		// Skip the test if baselinecaps is set to v4.13 or v4.14
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}
		if !isBaselineCapsSet(oc, "ImageRegistry") || !isEnabledCapability(oc, "Build") {
			g.Skip("Can't find the cluster operator imageregistry or build, skip it.")
		}
		if !checkMustgatherImagestreamTag(oc) {
			g.Skip("Skipping the test as can't find the imagestreamtag for must-gather")
		}

		g.By("create new namespace")
		oc.SetupProject()
		ns10618 := oc.Namespace()

		g.By("create the build")
		err := oc.WithoutNamespace().Run("new-build").Args("-D", "FROM must-gather", "-n", ns10618).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		for i := 0; i < 4; i++ {
			err := oc.Run("start-build").Args("bc/must-gather", "-n", ns10618).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			time.Sleep(30 * time.Second)
		}
		out, err := oc.AsAdmin().Run("adm").Args("prune", "builds", "-h").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "Prune old completed and failed builds")).To(o.BeTrue())

		for j := 1; j < 6; j++ {
			checkBuildStatus(oc, "must-gather-"+strconv.Itoa(j), ns10618, "Complete")
		}

		keepCompletedRsNum := 2
		expectedPrunebuildcmdDryRun := fmt.Sprintf("oc adm prune builds --keep-complete=%v --keep-younger-than=1s --keep-failed=1  |grep %s |awk '{print $2}'", keepCompletedRsNum, ns10618)
		pruneBuildCMD := fmt.Sprintf("oc adm prune builds --keep-complete=%v --keep-younger-than=1s --keep-failed=1 --confirm  |grep %s|awk '{print $2}'", keepCompletedRsNum, ns10618)

		g.By("Get the expected prune build list from dry run")
		buildbeforedryrun, err := oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Complete\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildListPre := strings.Fields(buildbeforedryrun)
		e2e.Logf("the remain build list is %v", buildListPre)
		expectedPruneRsName := getPruneResourceName(expectedPrunebuildcmdDryRun)

		g.By("Get the pruned build list")
		buildbeforeprune, err := oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Complete\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildListPre2 := strings.Fields(buildbeforeprune)
		e2e.Logf("the remain build list is %v", buildListPre2)
		prunedBuildName := getPruneResourceName(pruneBuildCMD)
		if comparePrunedRS(expectedPruneRsName, prunedBuildName) {
			e2e.Logf("Checked the pruned resources is expected")
		} else {
			e2e.Failf("Pruned the wrong build")
		}

		g.By("Get the remain build and completed should <=2")
		out, err = oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Complete\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildNameList := strings.Fields(out)
		e2e.Logf("the remain build list is %v", buildNameList)
		e2e.Logf("the remain build list len is %v", len(buildNameList))
		o.Expect(len(buildNameList) < 3).To(o.BeTrue())

		g.By("Get the remain build and failed should <=1")
		out, err = oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Failed\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		failedBuildNameList := strings.Fields(out)
		o.Expect(len(failedBuildNameList) < 2).To(o.BeTrue())

		err = oc.Run("delete").Args("bc", "must-gather", "-n", ns10618, "--cascade=orphan").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("adm").Args("prune", "builds", "--keep-younger-than=1s", "--keep-complete=2", "--keep-failed=1", "--confirm", "--orphans").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err = oc.Run("get").Args("build", "-n", ns10618).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "No resources found")).To(o.BeTrue())
	})

	g.It("ARO-Author:yinzhou-Medium-62956-oc adm node-logs works for nodes logs api", func() {
		windowNodeList, err := exutil.GetAllNodesbyOSType(oc, "windows")
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(windowNodeList) < 1 {
			e2e.Logf("No windows nodes support to test output")
			_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--unit=kubelet", "-o", "short", "--tail", "10").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			e2e.Logf("With windows nodes not support to test output")
			_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--unit=kubelet", "--tail", "10").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--unit=kubelet", "-g", "crio", "--tail", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--since=-5m", "--until=-1m").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--tail", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		now := time.Now()
		sinceT := now.Add(time.Minute * -2).Format("2006-01-02 15:04:05")
		untilT := now.Add(time.Minute * -10).Format("2006-01-02 15:04:05")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--since", sinceT).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--until", untilT).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-11112-Prune old deploymentconfig by admin command", func() {
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.11") || isBaselineCapsSet(oc, "v4.14") || isBaselineCapsSet(oc, "v4.15") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns11112 := oc.Namespace()

		err := oc.WithoutNamespace().Run("create").Args("deploymentconfig", "mydc11112", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", ns11112).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertSpecifiedPodStatus(oc, "mydc11112-1-deploy", ns11112, "Succeeded")

		g.By("Trigger more deployment and wait for succeed")
		for i := 0; i < 3; i++ {
			err := oc.Run("rollout").Args("latest", "dc/mydc11112", "-n", ns11112).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			assertSpecifiedPodStatus(oc, "mydc11112-"+strconv.Itoa(i+2)+"-deploy", ns11112, "Succeeded")
		}

		g.By("Add pre hook to make sure new deployment failed")
		err = oc.Run("set").Args("deployment-hook", "dc/mydc11112", "--pre", "-c=default-container", "--failure-policy=abort", "--", "/bin/false").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < 2; i++ {
			err := oc.Run("rollout").Args("latest", "dc/mydc11112", "-n", ns11112).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			assertSpecifiedPodStatus(oc, "mydc11112-"+strconv.Itoa(i+5)+"-deploy", ns11112, "Failed")
		}

		g.By("Dry run prune the DC and wait for pruned DC is expected")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			output, warning, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "deployments", "--keep-complete=2", "--keep-failed=1", "--keep-younger-than=1m").Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The warning %v", warning)
			if strings.Contains(output, "mydc11112-1") && strings.Contains(output, "mydc11112-5") {
				e2e.Logf("Found the expected prune output %v", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "timeout wait for prune deploymentconfig dry run")
		rcOutput, err := oc.Run("get").Args("rc", "-n", ns11112).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(rcOutput, "mydc11112-1")).To(o.BeTrue())
		o.Expect(strings.Contains(rcOutput, "mydc11112-5")).To(o.BeTrue())

		g.By("Prune the DC and check the result is only prune the first completed and first failed DC")
		output, _, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "deployments", "--keep-complete=2", "--keep-failed=1", "--keep-younger-than=1m", "--confirm").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "mydc11112-1")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "mydc11112-5")).To(o.BeTrue())

		err = oc.Run("delete").Args("dc/mydc11112", "-n", ns11112, "--cascade=orphan").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Prune the DC with orphans and make sure all the non-running DC are all pruned")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "deployments", "--keep-younger-than=1m", "--confirm", "--orphans").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForResourceDisappear(oc, "rc", "mydc11112-2", ns11112)
		waitForResourceDisappear(oc, "rc", "mydc11112-3", ns11112)
		waitForResourceDisappear(oc, "rc", "mydc11112-6", ns11112)
		rcOutput, err = oc.Run("get").Args("rc", "-n", ns11112).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(rcOutput, "mydc11112-4")).To(o.BeTrue())
	})
	g.It("ROSA-OSD_CCS-ARO-NonPreRelease-Author:yinzhou-Medium-68242-oc adm release mirror works fine with multi-arch image to image stream", func() {
		extractTmpDirName := "/tmp/case68242"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		g.By("Create new namespace")
		oc.SetupProject()
		ns68242 := oc.Namespace()

		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		payloadImage := getLatestPayload("https://multi.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable-multi/latest")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "mirror", "-a", extractTmpDirName+"/.dockerconfigjson", "--from="+payloadImage, "--to-image-stream=release", "--keep-manifest-list=true", "-n", ns68242).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		imageMediaType, err := oc.Run("get").Args("istag", "release:installer", "-n", ns68242, "-o=jsonpath={.image.dockerImageManifestMediaType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The output %v", imageMediaType)
		o.Expect(strings.Contains(imageMediaType, "application/vnd.docker.distribution.manifest.list.v2+json")).To(o.BeTrue())
	})

	//yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-High-72307-High-72293-Should ignore certificate-authority checking when use insecure for oc image commands", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}

		exutil.By("Create temp dir and get pull secret")
		extractTmpDirName := "/tmp/case72307"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)

		err = getRouteCAToFile(oc, extractTmpDirName)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get desired image from ocp cluster")
		pullSpec, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pullSpec).NotTo(o.BeEmpty())

		exutil.By("Specify --insecure for `oc adm release info` command  without certificate-authority")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Specify --insecure for `oc adm release info` command  with certificate-authority at the same time")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--insecure", "--certificate-authority", extractTmpDirName+"/tls.crt").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Specify `oc adm release info` command with certificate-authority")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--certificate-authority", "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem").NotShowInfo().Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Specify --insecure for `oc image info` command  without certificate-authority")
		_, oupErr, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--insecure").Outputs()
		if err != nil && strings.Contains(oupErr, "the image is a manifest list and contains multiple images") {
			_, filterOsErr := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--insecure", "--filter-by-os", "linux/amd64").Output()
			o.Expect(filterOsErr).NotTo(o.HaveOccurred())
		} else if strings.Contains(oupErr, "certificate signed by unknown authority") {
			e2e.Failf("Hit certificate signed error %v", err)
		}

		exutil.By("Specify --insecure for `oc image info` command  with certificate-authority at the same time")
		_, oupErr, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--insecure", "--certificate-authority", extractTmpDirName+"/tls.crt").Outputs()
		if err != nil && strings.Contains(oupErr, "the image is a manifest list and contains multiple images") {
			_, filterErr := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--insecure", "--certificate-authority", extractTmpDirName+"/tls.crt", "--filter-by-os", "linux/amd64").Output()
			o.Expect(filterErr).NotTo(o.HaveOccurred())
		} else if strings.Contains(oupErr, "certificate signed by unknown authority") {
			e2e.Failf("Hit certificate signed error %v", err)
		}

		exutil.By("Specify `oc image info` command  with certificate-authority")
		_, oupErr, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--certificate-authority", "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem").NotShowInfo().Outputs()
		if err != nil && strings.Contains(oupErr, "the image is a manifest list and contains multiple images") {
			_, filterErr := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--certificate-authority", "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", "--filter-by-os", "linux/amd64").NotShowInfo().Output()
			o.Expect(filterErr).NotTo(o.HaveOccurred())
		} else if strings.Contains(oupErr, "certificate signed by unknown authority") {
			e2e.Failf("Hit certificate signed error %v", err)
		}
	})

	//yinzhou@redhat.com
	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-ConnectedOnly-Low-11111-Buildconfig should support providing cpu and memory usage", func() {
		if !isEnabledCapability(oc, "ImageRegistry") {
			g.Skip("Skip for the test due to image registry not installed")

		}

		if !checkImageRegistryPodNum(oc) {
			g.Skip("Skip for the test due to image registry not running well as expected")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns11111 := oc.Namespace()

		fileBaseDir := exutil.FixturePath("testdata", "workloads/ocp11111")
		quotaFile := filepath.Join(fileBaseDir, "quota.yaml")
		limitFile := filepath.Join(fileBaseDir, "limits.yaml")
		appFile := filepath.Join(fileBaseDir, "application-template-with-resources.json")
		g.By("Create the quota and limit")
		err := oc.AsAdmin().Run("create").Args("-f", quotaFile, "-n", ns11111).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("create").Args("-f", limitFile, "-n", ns11111).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the template and app")
		err = oc.Run("create").Args("-f", appFile, "-n", ns11111).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("new-app").Args("--template=ruby-helloworld-sample-with-resources", "--import-mode=PreserveOriginal", "-n", ns11111).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertPodOutput(oc, "openshift.io/build.name=ruby-sample-build-1", ns11111, "Running")
		g.By("Check the build should has cpu and memory setting")
		output, err := oc.Run("get").Args("pod", "-l", "openshift.io/build.name=ruby-sample-build-1", "-n", ns11111, "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "cpu: 120m")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "memory: 256Mi")).To(o.BeTrue())
		g.By("Patch buildconfig to use higher cpu and memory setting")
		patchCpu := `[{"op": "replace", "path": "/spec/resources/limits/cpu", "value":"1020m"}]`
		err = oc.Run("patch").Args("bc", "ruby-sample-build", "-n", ns11111, "--type=json", "-p", patchCpu).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchMemory := `[{"op": "replace", "path": "/spec/resources/limits/memory", "value":"760Mi"}]`
		err = oc.Run("patch").Args("bc", "ruby-sample-build", "-n", ns11111, "--type=json", "-p", patchMemory).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("delete").Args("build", "--all", "-n", ns11111).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Make sure when exceed limit should failed to create pod")
		err = oc.Run("start-build").Args("ruby-sample-build", "-n", ns11111).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			buildOut, err := oc.Run("describe").Args("build", "-n", ns11111).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(buildOut, "Failed creating build pod") && strings.Contains(buildOut, "maximum cpu usage per Pod") {
				e2e.Logf("Found the expected information about the limit")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "timeout wait for prune deploymentconfig dry run")
	})

	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-Medium-76271-make sure when must-gather failed and use oc adm inspect should have log lines with a timestamp", func() {
		output, outErr, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--image=quay.io/test/must-gather:latest").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(output, "OUT 202")).To(o.BeTrue())
		o.Expect(strings.Contains(outErr, "Error running must-gather collection")).To(o.BeTrue())
		o.Expect(strings.Contains(outErr, "Falling back to `oc adm inspect clusteroperators.v1.config.openshift.io` to collect basic cluster information")).To(o.BeTrue())
	})

	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-ConnectedOnly-High-76524-Make sure could extract oc based on s390x", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}

		exutil.By("Create temp dir and get pull secret")
		extractTmpDirName := "/tmp/case76524"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)

		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get desired image from ocp cluster")
		pullSpec, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pullSpec).NotTo(o.BeEmpty())

		exutil.By("Use `oc adm release extract` command with certificate-authority")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--registry-config", extractTmpDirName+"/.dockerconfigjson", pullSpec, "--certificate-authority", "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", "--command-os", "linux/s390x", "--command", "oc.rhel9", "--to="+extractTmpDirName+"/").NotShowInfo().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocheckcmd := "file /tmp/case76524/oc"
		ocfileOutput, err := exec.Command("bash", "-c", ocheckcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(ocfileOutput), "IBM S/390")).To(o.BeTrue())
	})
})
