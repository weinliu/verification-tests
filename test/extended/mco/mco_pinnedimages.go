package mco

import (
	"fmt"
	"math"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO Pinnedimages", func() {
	defer g.GinkgoRecover()

	var (
		oc   = exutil.NewCLI("mco-pinnedimages", exutil.KubeConfigPath())
		wMcp *MachineConfigPool
		mMcp *MachineConfigPool
		// Compact compatible MCP. If the node is compact/SNO this variable will be the master pool, else it will be the worker pool
		mcp *MachineConfigPool
	)

	g.JustBeforeEach(func() {
		// The pinnedimageset feature is currently only supported in techpreview
		skipIfNoTechPreview(oc.AsAdmin())

		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
		mcp = GetCompactCompatiblePool(oc.AsAdmin())
		logger.Infof("%s %s %s", wMcp, mMcp, mcp)

		preChecks(oc)
	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-73631-Pinned images garbage collection [Disruptive]", func() {
		var (
			waitForPinned       = time.Minute * 5
			pinnedImageSetName  = "tc-73631-pinned-images-garbage-collector"
			gcKubeletConfig     = `{"imageMinimumGCAge": "2m", "imageGCHighThresholdPercent": 2, "imageGCLowThresholdPercent": 1}`
			kcTemplate          = generateTemplateAbsolutePath("generic-kubelet-config.yaml")
			kcName              = "tc-73631-pinned-garbage-collector"
			node                = mcp.GetNodesOrFail()[0]
			pinnedImage         = NewRemoteImage(node, "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f")
			manuallyPulledImage = NewRemoteImage(node, "quay.io/openshifttest/alpine@sha256:dc1536cbff0ba235d4219462aeccd4caceab9def96ae8064257d049166890083")
		)

		exutil.By("Remove the test images")
		_ = pinnedImage.Rmi()
		_ = manuallyPulledImage.Rmi()
		logger.Infof("OK!\n")

		exutil.By("Configure kubelet to start garbage collection")
		logger.Infof("Create worker KubeletConfig")
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		defer mcp.waitForComplete()
		defer kc.Delete()
		kc.create("KUBELETCONFIG="+gcKubeletConfig, "POOL="+mcp.GetName())

		exutil.By("Wait for configurations to be applied in worker pool")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		logger.Infof("Pin image")
		pis, err := CreateGenericPinnedImageSet(oc.AsAdmin(), pinnedImageSetName, mcp.GetName(), []string{pinnedImage.ImageName})
		defer pis.DeleteAndWait(waitForPinned)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating pinnedimageset %s", pis)
		logger.Infof("OK!\n")

		exutil.By("Wait for all images to be pinned")
		o.Expect(mcp.waitForPinComplete(waitForPinned)).To(o.Succeed(), "Pinned image operation is not completed in %s", mcp)
		logger.Infof("OK!\n")

		exutil.By("Manually pull image")
		o.Expect(manuallyPulledImage.Pull()).To(o.Succeed(),
			"Error pulling %s", manuallyPulledImage)
		logger.Infof("Check that the manually pulled image is not pinned")
		o.Expect(manuallyPulledImage.IsPinned()).To(o.BeFalse(),
			"Error, %s is pinned, but it should not", manuallyPulledImage)
		logger.Infof("OK!\n")

		exutil.By("Check that the manually pulled image is garbage collected")
		o.Eventually(manuallyPulledImage, "20m", "20s").ShouldNot(Exist(),
			"Error, %s has not been garbage collected", manuallyPulledImage)
		logger.Infof("OK!\n")

		exutil.By("Check that the pinned image is still pinned after garbage collection")
		o.Expect(pinnedImage.IsPinned()).To(o.BeTrue(),
			"Error, after the garbage collection happened %s is not pinned anymore", pinnedImage)
		logger.Infof("OK!\n")

		exutil.By("Reboot node")
		o.Expect(node.Reboot()).To(o.Succeed(),
			"Error rebooting node %s", node.GetName())
		logger.Infof("OK!\n")

		exutil.By("Check that the pinned image is still pinned after reboot")
		o.Expect(pinnedImage.IsPinned()).To(o.BeTrue(),
			"Error, after the garbage collection happened %s is not pinned anymore", pinnedImage)
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-73635-Pod can use pinned images while no access to the registry [Disruptive]", func() {
		var (
			waitForPinned      = time.Minute * 5
			pinnedImageSetName = "tc-73635-pinned-images-no-registry"
			// We pin the current release's tools image
			pinnedImage = getCurrentReleaseInfoImageSpecOrFail(oc.AsAdmin(), "tools")
			allNodes    = mcp.GetNodesOrFail()
			pullSecret  = GetPullSecret(oc.AsAdmin())

			deploymentName      = "tc-73635-test"
			deploymentNamespace = oc.Namespace()
			deployment          = NewNamespacedResource(oc, "deployment", deploymentNamespace, deploymentName)
			scaledReplicas      = 5
			nodeList            = NewNamespacedResourceList(oc, "pod", deploymentNamespace)
		)
		defer nodeList.PrintDebugCommand() // for debugging purpose in case of failed deployment

		exutil.By("Remove the image from all nodes in the pool")
		for _, node := range allNodes {
			if node.HasTaintEffectOrFail("NoExecute") {
				logger.Infof("Node %s is tainted with 'NoExecute'. Validation skipped.", node.GetName())
				continue
			}
			// We ignore errors, since the image can be present or not in the nodes
			_ = NewRemoteImage(node, pinnedImage).Rmi()
		}
		logger.Infof("OK!\n")

		exutil.By("Create pinnedimageset")
		pis, err := CreateGenericPinnedImageSet(oc.AsAdmin(), pinnedImageSetName, mcp.GetName(), []string{pinnedImage})
		defer pis.DeleteAndWait(waitForPinned)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating pinnedimageset %s", pis)
		logger.Infof("OK!\n")

		exutil.By("Wait for all images to be pinned")
		o.Expect(mcp.waitForPinComplete(waitForPinned)).To(o.Succeed(), "Pinned image operation is not completed in %s", mcp)
		logger.Infof("OK!\n")

		exutil.By("Check that the image was pinned in all nodes in the pool")
		for _, node := range allNodes {
			if node.HasTaintEffectOrFail("NoExecute") {
				logger.Infof("Node %s is tainted with 'NoExecute'. Validation skipped.", node.GetName())
				continue
			}
			ri := NewRemoteImage(node, pinnedImage)
			logger.Infof("Checking %s", ri)
			o.Expect(ri.IsPinned()).To(o.BeTrue(),
				"%s is not pinned, but it should. %s")
		}
		logger.Infof("OK!\n")

		exutil.By("Capture the current pull-secret value")
		// We don't use the pullSecret resource directly, instead we use auxiliary functions that will
		// extract and restore the secret's values using a file. Like that we can recover the value of the pull-secret
		// if our execution goes wrong, without printing it in the logs (for security reasons).
		secretFile, err := getPullSecret(oc)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error getting the pull-secret")
		logger.Debugf("Pull-secret content stored in file %s", secretFile)
		defer func() {
			logger.Infof("Restoring initial pull-secret value")
			output, err := setDataForPullSecret(oc, secretFile)
			if err != nil {
				logger.Errorf("Error restoring the pull-secret's value. Error: %s\nOutput: %s", err, output)
			}
			wMcp.waitForComplete()
			mMcp.waitForComplete()
		}()
		logger.Infof("OK!\n")

		exutil.By("Set an empty pull-secret")
		o.Expect(pullSecret.SetDataValue(".dockerconfigjson", "{}")).To(o.Succeed(),
			"Error setting an empty pull-secret value")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		exutil.By("Check that the image is pinned")
		for _, node := range allNodes {
			if node.HasTaintEffectOrFail("NoExecute") {
				logger.Infof("Node %s is tainted with 'NoExecute'. Validation skipped.", node.GetName())
				continue
			}
			logger.Infof("Checking node %s", node.GetName())
			ri := NewRemoteImage(node, pinnedImage)
			o.Expect(ri.IsPinned()).To(o.BeTrue(),
				"%s is not pinned, but it should. %s")
		}
		logger.Infof("OK!\n")

		exutil.By("Create test deployment")
		defer deployment.Delete()
		o.Expect(
			NewMCOTemplate(oc.AsAdmin(), "create-deployment.yaml").Create("-p", "NAME="+deploymentName, "IMAGE="+pinnedImage, "NAMESPACE="+deploymentNamespace),
		).To(o.Succeed(),
			"Error creating the deployment")
		o.Eventually(deployment, "6m", "15s").Should(BeAvailable(),
			"Resource is NOT available:\n/%s", deployment.PrettyString())
		o.Eventually(deployment.Get, "6m", "15s").WithArguments(`{.status.readyReplicas}`).Should(o.Equal(deployment.GetOrFail(`{.spec.replicas}`)),
			"Resource is NOT stable, still creating replicas:\n/%s", deployment.PrettyString())
		logger.Infof("OK!\n")

		exutil.By("Scale app")
		o.Expect(
			deployment.Patch("merge", fmt.Sprintf(`{"spec":{"replicas":%d}}`, scaledReplicas)),
		).To(o.Succeed(),
			"Error scaling %s", deployment)
		o.Eventually(deployment, "6m", "15s").Should(BeAvailable(),
			"Resource is NOT available:\n/%s", deployment.PrettyString())
		o.Eventually(deployment.Get, "6m", "15s").WithArguments(`{.status.readyReplicas}`).Should(o.Equal(deployment.GetOrFail(`{.spec.replicas}`)),
			"Resource is NOT stable, still creating replicas:\n/%s", deployment.PrettyString())
		logger.Infof("OK!\n")

		exutil.By("Reboot nodes")
		o.Expect(mcp.Reboot()).To(o.Succeed(), "Error rebooting pool %s", mcp)
		o.Expect(mcp.WaitForRebooted()).To(o.Succeed(), "Nodes in pool %s were not rebooted", mcp)
		logger.Infof("OK!\n")

		exutil.By("Check that the applicaion is OK after the reboot")
		o.Eventually(deployment, "6m", "15s").Should(BeAvailable(),
			"Resource is NOT available:\n/%s", deployment.PrettyString())
		o.Eventually(deployment.Get, "6m", "15s").WithArguments(`{.status.readyReplicas}`).Should(o.Equal(deployment.GetOrFail(`{.spec.replicas}`)),
			"Resource is NOT stable, still creating replicas:\n/%s", deployment.PrettyString())
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-73630-Pin release images [Disruptive]", func() {
		var (
			waitForPinned            = time.Minute * 10
			pinnedImageSetName       = "tc-73630-pinned-imageset-release"
			pinnedImages             = getReleaseInfoPullspecOrFail(oc.AsAdmin())
			node                     = mcp.GetNodesOrFail()[0]
			minGigasAvailableInNodes = 40
		)

		skipIfDiskSpaceLessThanBytes(node, "/var/lib/containers/storage/", int64(float64(minGigasAvailableInNodes)*(math.Pow(1024, 3))))

		exutil.By("Create pinnedimageset to pin all pullSpec images")
		pis, err := CreateGenericPinnedImageSet(oc.AsAdmin(), pinnedImageSetName, mcp.GetName(), pinnedImages)
		defer pis.DeleteAndWait(waitForPinned)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating pinnedimageset %s", pis)
		logger.Infof("OK!\n")

		exutil.By("Wait for all images to be pinned")
		o.Expect(mcp.waitForPinComplete(waitForPinned)).To(o.Succeed(), "Pinned image operation is not completed in %s", mcp)
		logger.Infof("OK!\n")

		exutil.By("Check that all images were pinned")
		for _, image := range pinnedImages {
			ri := NewRemoteImage(node, image)
			o.Expect(ri.IsPinned()).To(o.BeTrue(),
				"%s is not pinned, but it should. %s")
		}
		logger.Infof("OK!\n")

	})
})

// getReleaseInfoPullspecOrFail returns a list of strings containing the names of the pullspec images
func getReleaseInfoPullspecOrFail(oc *exutil.CLI) []string {
	mMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
	master := mMcp.GetNodesOrFail()[0]

	remoteAdminKubeConfig := fmt.Sprintf("/root/remoteKubeConfig-%s", exutil.GetRandomString())
	adminKubeConfig := exutil.KubeConfigPath()

	defer master.RemoveFile(remoteAdminKubeConfig)
	o.Expect(master.CopyFromLocal(adminKubeConfig, remoteAdminKubeConfig)).To(o.Succeed(),
		"Error copying kubeconfig file to master node")

	stdout, _, err := master.DebugNodeWithChrootStd("oc", "adm", "release", "info", "-o", "pullspec", "--registry-config", "/var/lib/kubelet/config.json", "--kubeconfig", remoteAdminKubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred(), "Error getting image release pull specs")
	return strings.Split(stdout, "\n")
}

// skipIfDiskSpaceLessThanBytes skip test case if there is less than minNumBytes space available in the given path
func skipIfDiskSpaceLessThanBytes(node Node, path string, minNumBytes int64) {
	diskUsage, err := node.GetFileSystemSpaceUsage(path)

	o.Expect(err).NotTo(o.HaveOccurred(),
		"Cannot get the disk usage in node %s", node.GetName())

	if minNumBytes > diskUsage.Avail {
		g.Skip(fmt.Sprintf("Available diskspace in %s is %d bytes, which is less than the required %d bytes",
			node.GetName(), diskUsage.Avail, minNumBytes))
	}

	logger.Infof("Required disk space %d bytes, available disk space %d", minNumBytes, diskUsage.Avail)

}

func getCurrentReleaseInfoImageSpecOrFail(oc *exutil.CLI, imageName string) string {
	mMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
	master := mMcp.GetNodesOrFail()[0]

	remoteAdminKubeConfig := fmt.Sprintf("/root/remoteKubeConfig-%s", exutil.GetRandomString())
	adminKubeConfig := exutil.KubeConfigPath()

	defer master.RemoveFile(remoteAdminKubeConfig)
	o.Expect(master.CopyFromLocal(adminKubeConfig, remoteAdminKubeConfig)).To(o.Succeed(),
		"Error copying kubeconfig file to master node")

	stdout, _, err := master.DebugNodeWithChrootStd("oc", "adm", "release", "info", "--image-for", imageName, "--registry-config", "/var/lib/kubelet/config.json", "--kubeconfig", remoteAdminKubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred(), "Error getting image release pull specs")
	return stdout
}
