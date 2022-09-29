package mco

import (
	"regexp"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	logger "github.com/openshift/openshift-tests-private/test/extended/mco/logext"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-mco] MCO Layering", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("mco-layering", exutil.KubeConfigPath())

	g.JustBeforeEach(func() {
		g.By("MCO Preconditions Checks")
		preChecks(oc)
		logger.Infof("End Of MCO Preconditions")
	})

	g.It("Author:sregidor-ConnectedOnly-VMonly-Longduration-NonPreRelease-Critical-54085-Update osImage changing /etc /usr and /var [Disruptive]", func() {
		dockerFileCommands := `
RUN mkdir /etc/tc_54085 && chmod 3770 /etc/tc_54085 && ostree container commit

RUN echo 'Test case 54085 test file' > /etc/tc54085.txt && chmod 5400 /etc/tc54085.txt && ostree container commit

RUN echo 'echo "Hello world"' > /usr/bin/tc54085_helloworld && chmod 5770 /usr/bin/tc54085_helloworld && ostree container commit

RUN cd /etc/yum.repos.d/ && curl -LO https://pkgs.tailscale.com/stable/fedora/tailscale.repo && \
    rpm-ostree install tailscale && rpm-ostree cleanup -m && \
    systemctl enable tailscaled && \
    ostree container commit
`
		// Extract secret
		g.By("Capture the current ostree deployment")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		initialDeployment, err := workerNode.GetBootedOsTreeDeployment()
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the booted ostree deployment")
		logger.Infof("OK\n")

		g.By("Extract pull-secret")
		pullSecret := GetPullSecret(oc.AsAdmin())
		secretExtractDir, err := pullSecret.Extract()
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error extracting pull-secret")
		logger.Infof("Pull secret has been extracted to: %s\n", secretExtractDir)

		// Build new osImage
		g.By("Get base image for layering")
		baseImage, err := getLayeringBaseImage(oc.AsAdmin(), secretExtractDir)
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the base image to build new osImages")
		logger.Infof("Base image: %s\n", baseImage)

		g.By("Build a new layered image using the Dockerfile")
		layeringImageRepo := getLayeringTestImageRepository()

		dockerFile := "FROM " + baseImage + "\n" + dockerFileCommands
		logger.Infof(" Using Dockerfile:\n%s", dockerFile)

		buildDir, err := prepareDockerfileDirectory(dockerFile)
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error creating the build directory with the Dockerfile")

		berr := buildPushImage("arm64", buildDir, layeringImageRepo, secretExtractDir)
		o.Expect(berr).NotTo(o.HaveOccurred(),
			"Error building and pushing the image %s", layeringImageRepo)
		logger.Infof("Image pushed to: %s\n", layeringImageRepo)

		// Create MC and wait for MCP
		g.By("Create a MC to deploy the new osImage")
		layeringMcName := "layering-mc"
		genericMcTemplate := "generic-machine-config-template.yml"
		layeringMC := MachineConfig{name: layeringMcName, Template: *NewMCOTemplate(oc, genericMcTemplate),
			pool: MachineConfigPoolWorker, parameters: []string{"OS_IMAGE=" + layeringImageRepo}}

		defer layeringMC.delete(oc)
		layeringMC.create(oc.AsAdmin())

		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()
		logger.Infof("The new osImage was deployed successfully\n")

		// Check rpm-ostree status
		g.By("Check that the rpm-ostree status is reporting the right booted image")

		status, err := workerNode.GetRpmOstreeStatus(false)
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the rpm-ostree status value in node %s", workerNode.GetName())
		logger.Infof("Current rpm-ostree status:\n%s\n", status)

		deployment, err := workerNode.GetBootedOsTreeDeployment()
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the rpm-ostree status value in node %s", workerNode.GetName())

		containerRef, jerr := JSON(deployment).GetSafe("container-image-reference")
		o.Expect(jerr).NotTo(o.HaveOccurred(),
			"We cant get 'container-image-reference' from the deployment status. Wrong rpm-ostree status!")
		o.Expect(containerRef.ToString()).To(o.Equal("ostree-unverified-registry:"+layeringImageRepo),
			"container reference in the status is not the exepeced one")
		logger.Infof("OK!\n")

		// Check image content
		g.By("Load remote resources to verify that the osImage content has been deployed properly")

		tc54085Dir := NewRemoteFile(workerNode, "/etc/tc_54085")
		tc54085File := NewRemoteFile(workerNode, "/etc/tc54085.txt")
		binHelloWorld := NewRemoteFile(workerNode, "/usr/bin/tc54085_helloworld")

		o.Expect(tc54085Dir.Fetch()).ShouldNot(o.HaveOccurred(),
			"Error getting information about file %s in node %s", tc54085Dir.GetName(), workerNode.GetName())
		o.Expect(tc54085File.Fetch()).ShouldNot(o.HaveOccurred(),
			"Error getting information about file %s in node %s", tc54085File.GetName(), workerNode.GetName())
		o.Expect(binHelloWorld.Fetch()).ShouldNot(o.HaveOccurred(),
			"Error getting information about file %s in node %s", binHelloWorld.GetName(), workerNode.GetName())
		logger.Infof("OK!\n")

		g.By("Check that the directory in /etc exists and has the right permissions")
		o.Expect(tc54085Dir.IsDirectory()).To(o.BeTrue(),
			"Error, %s in node %s is not a directory", tc54085Dir.GetName(), workerNode.GetName())
		o.Expect(tc54085Dir.GetNpermissions()).To(o.Equal("3770"),
			"Error, permissions of %s in node %s are not the expected ones", tc54085Dir.GetName(), workerNode.GetName())
		logger.Infof("OK!\n")

		g.By("Check that the file in /etc exists and has the right permissions")
		o.Expect(tc54085File.GetNpermissions()).To(o.Equal("5400"),
			"Error, permissions of %s in node %s are not the expected ones", tc54085File.GetName(), workerNode.GetName())
		o.Expect(tc54085File.GetTextContent()).To(o.Equal("Test case 54085 test file\n"),
			"Error, content of %s in node %s are not the expected one", tc54085File.GetName(), workerNode.GetName())

		g.By("Check that the file in /usr/bin exists, has the right permissions and can be executed")
		o.Expect(binHelloWorld.GetNpermissions()).To(o.Equal("5770"),
			"Error, permissions of %s in node %s are not the expected ones", tc54085File.GetName(), workerNode.GetName())

		output, herr := workerNode.DebugNodeWithChroot("/usr/bin/tc54085_helloworld")
		o.Expect(herr).NotTo(o.HaveOccurred(),
			"Error executing 'hello world' executable file /usr/bin/tc54085_helloworld")
		o.Expect(output).To(o.ContainSubstring("Hello world"),
			"Error, 'Hellow world' executable file's output was not the expected one")
		logger.Infof("OK!\n")

		g.By("Check that the tailscale rpm has been deployed")
		tailscaledRpm, rpmErr := workerNode.DebugNodeWithChroot("rpm", "-q", "tailscale")
		o.Expect(rpmErr).NotTo(o.HaveOccurred(),
			"Error, getting the installed rpms in node %s.  'tailscale' rpm is not installed.", workerNode.GetName())
		o.Expect(tailscaledRpm).To(o.ContainSubstring("tailscale-"),
			"Error, 'tailscale' rpm is not installed in node %s", workerNode.GetName())
		logger.Infof("OK!\n")

		g.By("Check that the tailscaled.service unit is loaded, active and enabled")
		tailscaledStatus, unitErr := workerNode.GetUnitStatus("tailscaled.service")
		o.Expect(unitErr).NotTo(o.HaveOccurred(),
			"Error getting the status of the 'tailscaled.service' unit in node %s", workerNode.GetName())
		o.Expect(tailscaledStatus).Should(
			o.And(
				o.ContainSubstring("tailscaled.service"),
				o.ContainSubstring("Active: active"), // is active
				o.ContainSubstring("Loaded: loaded"), // is loaded
				o.ContainSubstring("; enabled;")),    // is enabled
			"tailscaled.service unit should be loaded, active and enabled and it is not")
		logger.Infof("OK!\n")

		// Delete the MC and wait for MCP
		g.By("Delete the MC so that original osImage is restored")
		layeringMC.delete(oc)
		mcp.waitForComplete()
		logger.Infof("MC was successfully deleted\n")

		// Check the rpm-ostree status after the MC deletion
		g.By("Check that the original ostree deployment was restored")
		deployment, derr := workerNode.GetBootedOsTreeDeployment()
		o.Expect(derr).NotTo(o.HaveOccurred(),
			"Error getting the rpm-ostree status value in node %s", workerNode.GetName())
		o.Expect(deployment).To(o.MatchJSON(initialDeployment),
			"Error! the initial deployment was not properly restored after deleting the MachineConfig")
		logger.Infof("OK!\n")

		// Check the image content after the MC deletion
		g.By("Check that the directory in /etc does not exist anymore")
		o.Expect(tc54085Dir.Fetch()).Should(o.HaveOccurred(),
			"Error, file %s should not exist in node %s, but it exists", tc54085Dir.GetName(), workerNode.GetName())
		logger.Infof("OK!\n")

		g.By("Check that the file in /etc does not exist anymore")
		o.Expect(tc54085File.Fetch()).Should(o.HaveOccurred(),
			"Error, file %s should not exist in node %s, but it exists", tc54085File.GetName(), workerNode.GetName())
		logger.Infof("OK!\n")

		g.By("Check that the file in /usr/bin does not exist anymore")
		o.Expect(binHelloWorld.Fetch()).Should(o.HaveOccurred(),
			"Error, file %s should not exist in node %s, but it exists", binHelloWorld.GetName(), workerNode.GetName())
		logger.Infof("OK!\n")

		g.By("Check that the tailscale rpm is not installed anymore")
		tailscaledRpm, rpmErr = workerNode.DebugNodeWithChroot("rpm", "-q", "tailscale")
		o.Expect(rpmErr).To(o.HaveOccurred(),
			"Error,  'tailscale' rpm should not be installed in node %s, but it is installed.\n Output %s", workerNode.GetName(), tailscaledRpm)
		logger.Infof("OK!\n")

		g.By("Check that the tailscaled.service is not present anymore")
		tailscaledStatus, unitErr = workerNode.GetUnitStatus("tailscaled.service")
		o.Expect(unitErr).To(o.HaveOccurred(),
			"Error,  'tailscaled.service'  unit should not be available in node %s, but it is.\n Output %s", workerNode.GetName(), tailscaledStatus)
		logger.Infof("OK!\n")

	})
	g.It("Author:sregidor-ConnectedOnly-NonPreRelease-Medium-54052-Not bootable layered osImage provided[Disruptive]", func() {
		var (
			nonBootableImage = "quay.io/openshifttest/hello-openshift:1.2.0"
			layeringMcName   = "not-bootable-image-tc54052"

			expectedNDStatus  = "True"
			expectedNDMessage = ".*failed to update OS to " + regexp.QuoteMeta(nonBootableImage+" : error running ostree refs --repo") + ".*"
			expectedNDReason  = "1 nodes are reporting degraded status on sync"
		)

		checkInvalidOsImagesDegradedStatus(oc.AsAdmin(), nonBootableImage, layeringMcName, expectedNDStatus, expectedNDMessage, expectedNDReason)
	})

	g.It("Author:sregidor-NonPreRelease-Medium-54054-Not pullable layered osImage provided[Disruptive]", func() {
		var (
			nonPullableImage = "quay.io/openshifttest/tc54054fakeimage:latest"
			layeringMcName   = "not-pullable-image-tc54054"

			expectedNDStatus  = "True"
			expectedNDMessage = regexp.QuoteMeta("Error checking type of update image: failed to run command podman (6 tries): [timed out waiting for the condition, running podman pull") +
				".*" + regexp.QuoteMeta(nonPullableImage) + ".*"
			expectedNDReason = "1 nodes are reporting degraded status on sync"
		)

		checkInvalidOsImagesDegradedStatus(oc.AsAdmin(), nonPullableImage, layeringMcName, expectedNDStatus, expectedNDMessage, expectedNDReason)
	})
})

// oc: the CLI
// image: the layered image that will be configured in the MC
// layeringMcName: the name of the MC
// expectedNDStatus: expected value for the status in the MCP NodeDegraded condition
// expectedNDMessage: expected value for the message in the MCP NodeDegraded condition
// expectedNDReason: expected value for the reason in the MCP NodeDegraded condition
func checkInvalidOsImagesDegradedStatus(oc *exutil.CLI, image, layeringMcName, expectedNDStatus, expectedNDMessage, expectedNDReason string) {
	var (
		genericMcTemplate = "generic-machine-config-template.yml"
		mcp               = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
	)
	// Create MC and wait for MCP
	g.By("Create a MC to deploy the new osImage that is non-bootable")
	layeringMC := MachineConfig{name: layeringMcName, Template: *NewMCOTemplate(oc, genericMcTemplate), skipWaitForMcp: true,
		pool: mcp.GetName(), parameters: []string{"OS_IMAGE=" + image}}

	// Defer the error recovery logic
	defer func() {
		logger.Infof("Start image checker helper defer block")
		if layeringMC.exists(oc.AsAdmin()) {
			logger.Infof("Deleting MC")
			layeringMC.deleteNoWait(oc)

			logger.Infof("Recovering degraded nodes")
			// At this point we dont know exactly the state of the cluster, so we make sure that
			// all nodes (and not only the degraded ones) have the right desiredConfig annotation
			degradedNodes, _ := NewNodeList(oc.AsAdmin()).GetAll()
			for _, degradedNode := range degradedNodes {
				logger.Infof("Recovering from errors in node: %s", degradedNode)
				_ = degradedNode.RestoreDesiredConfig()
			}

			derr := mcp.WaitForNotDegradedStatus()
			if derr != nil {
				logger.Infof("Could not recover from the degraded status: %s", derr)
			}

			uerr := mcp.WaitForUpdatedStatus()
			if uerr != nil {
				logger.Infof("Could not recover from the degraded status: %s", uerr)
			}
		} else {
			logger.Infof("The machine config was deleted during the test execution. Nothing to clean.")
		}

		logger.Infof("End image checker helper defer block")
	}()

	layeringMC.create(oc.AsAdmin())
	logger.Infof("OK!\n")

	// Verify that MCP is degraded
	g.By("Check that the worker MCP becomes degraded")
	logger.Infof("Wait for degraded status")
	o.Eventually(mcp.pollDegradedStatus(), "25m", "30s").Should(o.Equal("True"),
		"The 'worker' MCP should become degraded when a non-bootable osImage is configure, but it didn't.")

	logger.Infof("Wait for degraded machinecount")
	o.Eventually(mcp.pollDegradedMachineCount(), "5m", "30s").Should(o.Equal("1"),
		"The 'worker' MCP should report '1' degraded machine count, but it doesn't.")

	logger.Infof("OK!\n")

	// Verify that degraded messages are ok
	g.By("Check that the error is reported properly in the MCP")
	nodeDegradedCondition := mcp.GetConditionByType("NodeDegraded")

	nodeDegradedConditionJSON := JSON(nodeDegradedCondition)

	nodeDegradedStatus := nodeDegradedConditionJSON.Get("status").ToString()
	nodeDegradedMessage := nodeDegradedConditionJSON.Get("message").ToString()
	nodeDegradedReason := nodeDegradedConditionJSON.Get("reason").ToString()

	o.ExpectWithOffset(1, nodeDegradedStatus).Should(o.Equal(expectedNDStatus),
		"'worker' MCP should report degraded status in the NodeDegraded condition: %s", nodeDegradedCondition)
	o.ExpectWithOffset(1, nodeDegradedMessage).Should(o.MatchRegexp(expectedNDMessage),
		"'worker' MCP is not reporting the expected message in the NodeDegraded condition: %s", nodeDegradedCondition)
	o.ExpectWithOffset(1, nodeDegradedReason).Should(o.MatchRegexp(expectedNDReason),
		"'worker' MCP is not reporting the expected reason in the NodeDegraded condition: %s", nodeDegradedCondition)
	logger.Infof("OK!\n")

	// Recover from error
	g.By("Restore the original status and recover from the error")
	logger.Infof("Deleting MC")
	layeringMC.deleteNoWait(oc)

	logger.Infof("Restoring annotations in degraded nodes")
	degradedNodes, err := NewNodeList(oc.AsAdmin()).GetAllDegraded()
	o.ExpectWithOffset(1, err).ShouldNot(o.HaveOccurred(),
		"Where was an error trying to get the list of the Degraded nodes")
	for _, degradedNode := range degradedNodes {
		logger.Infof("Recovering from errors in node: %s", degradedNode)
		err := degradedNode.RestoreDesiredConfig()
		o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(),
			"Error restoring the desiredConfig annotation in node %s", degradedNode.GetName())
	}

	o.Eventually(mcp.pollDegradedStatus(), "10m", "30s").Should(o.Equal("False"),
		"The 'worker' MCP should not be degraded once the wrong MC is deleted and the node annotations are restored. But it is degraded.")

	mcp.waitForComplete()

	logger.Infof("OK!\n")

}
