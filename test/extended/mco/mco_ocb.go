package mco

import (
	"fmt"
	"strings"
	"sync"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO ocb", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("mco-ocb", exutil.KubeConfigPath())
	)

	g.JustBeforeEach(func() {
		preChecks(oc)
		// According to https://issues.redhat.com/browse/MCO-831, featureSet:TechPreviewNoUpgrade is required
		// xref: featureGate: OnClusterBuild
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test")
		}

		skipTestIfOCBIsEnabled(oc)
	})

	g.It("Author:sregidor-NonPreRelease-High-73494-[P1] OCB Wiring up Productionalized Build Controller. New 4.16 OCB API [Disruptive]", func() {
		var (
			infraMcpName = "infra"
			moscName     = "tc-73494-infra"
		)

		exutil.By("Create custom infra MCP")
		// We add no workers to the infra pool, it is not necessary
		infraMcp, err := CreateCustomMCP(oc.AsAdmin(), infraMcpName, 0)
		defer infraMcp.delete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating a new custom pool: %s", infraMcpName)
		logger.Infof("OK!\n")

		exutil.By("Configure OCB functionality for the new infra MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, infraMcpName, nil)
		defer mosc.CleanupAndDelete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		ValidateSuccessfulMOSC(mosc, nil)

		exutil.By("Remove the MachineOSConfig resource")
		o.Expect(mosc.CleanupAndDelete()).To(o.Succeed(), "Error cleaning up %s", mosc)
		logger.Infof("OK!\n")

		ValidateMOSCIsGarbageCollected(mosc, infraMcp)

		exutil.AssertAllPodsToBeReady(oc.AsAdmin(), MachineConfigNamespace)
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonPreRelease-Medium-73599-[P2] OCB Validate MachineOSConfig. New 41.6 OCB API [Disruptive]", func() {
		var (
			infraMcpName = "infra"
			moscName     = "tc-73599-infra"
			pushSpec     = fmt.Sprintf("%s/openshift-machine-config-operator/ocb-%s-image:latest", InternalRegistrySvcURL, infraMcpName)
			pullSecret   = NewSecret(oc.AsAdmin(), "openshift-config", "pull-secret")

			fakePullSecretName         = "fake-pull-secret"
			expectedWrongPullSecretMsg = fmt.Sprintf(`invalid MachineOSConfig %s: could not validate baseImagePullSecret "%s" for MachineOSConfig %s: secret %s from %s is not found. Did you use the right secret name?`,
				moscName, fakePullSecretName, moscName, fakePullSecretName, moscName)
			fakePushSecretName         = "fake-push-secret"
			expectedWrongPushSecretMsg = fmt.Sprintf(`invalid MachineOSConfig %s: could not validate renderedImagePushSecret "%s" for MachineOSConfig %s: secret %s from %s is not found. Did you use the right secret name?`,
				moscName, fakePushSecretName, moscName, fakePushSecretName, moscName)

			fakeBuilderType             = "FakeBuilderType"
			expectedWrongBuilderTypeMsg = fmt.Sprintf(`Unsupported value: "%s": supported values: "PodImageBuilder"`, fakeBuilderType)
		)

		exutil.By("Create custom infra MCP")
		// We add no workers to the infra pool, it is not necessary
		infraMcp, err := CreateCustomMCP(oc.AsAdmin(), infraMcpName, 0)
		defer infraMcp.delete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating a new custom pool: %s", infraMcpName)
		logger.Infof("OK!\n")

		exutil.By("Clone the pull-secret in MCO namespace")
		clonedSecret, err := CloneResource(pullSecret, "cloned-pull-secret-"+exutil.GetRandomString(), MachineConfigNamespace, nil)
		defer clonedSecret.Delete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error duplicating the cluster's pull-secret in MCO namespace")
		logger.Infof("OK!\n")

		// Check behaviour when wrong pullSecret
		checkMisconfiguredMOSC(oc.AsAdmin(), moscName, infraMcpName, clonedSecret.GetName(), fakePullSecretName, clonedSecret.GetName(), pushSpec, nil,
			expectedWrongPullSecretMsg,
			"Check that MOSC using wrong pull secret are failing as expected")

		// Check behaviour when wrong pushSecret
		checkMisconfiguredMOSC(oc.AsAdmin(), moscName, infraMcpName, clonedSecret.GetName(), clonedSecret.GetName(), fakePushSecretName, pushSpec, nil,
			expectedWrongPushSecretMsg,
			"Check that MOSC using wrong push secret are failing as expected")

		// Try to create a MOSC with a wrong pushSpec
		logger.Infof("Create a MachineOSConfig resource with a wrong builder type")

		err = NewMCOTemplate(oc, "generic-machine-os-config.yaml").Create("-p", "NAME="+moscName, "POOL="+infraMcpName, "PULLSECRET="+clonedSecret.GetName(),
			"PUSHSECRET="+clonedSecret.GetName(), "PUSHSPEC="+pushSpec, "IMAGEBUILDERTYPE="+fakeBuilderType)
		o.Expect(err).To(o.HaveOccurred(), "Expected oc command to fail, but it didn't")
		o.Expect(err).To(o.BeAssignableToTypeOf(&exutil.ExitError{}), "Unexpected error while creating the new MOSC")
		o.Expect(err.(*exutil.ExitError).StdErr).To(o.ContainSubstring(expectedWrongBuilderTypeMsg),
			"MSOC creation using wrong image type builder should be forbidden")

		logger.Infof("OK!")
	})

	g.It("Author:ptalgulk-ConnectedOnly-Longduration-NonPreRelease-Critical-74645-Panic Condition for Non-Matching MOSC Resources [Disruptive]", func() {
		var (
			infraMcpName = "infra"
			moscName     = "tc-74645"
			mcc          = NewController(oc.AsAdmin())
		)
		exutil.By("Create New Custom MCP")
		defer DeleteCustomMCP(oc.AsAdmin(), infraMcpName)
		infraMcp, err := CreateCustomMCP(oc.AsAdmin(), infraMcpName, 1)
		o.Expect(err).NotTo(o.HaveOccurred(), "Could not create a new custom MCP")
		node := infraMcp.GetNodesOrFail()[0]
		logger.Infof("%s", node.GetName())
		logger.Infof("OK!\n")

		exutil.By("Configure OCB functionality for the new infra MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, infraMcpName, nil)
		defer DisableOCL(mosc)
		// remove after this bug is fixed OCPBUGS-36810
		defer func() {
			logger.Infof("Configmaps should also be deleted ")
			cmList := NewConfigMapList(mosc.GetOC(), MachineConfigNamespace).GetAllOrFail()
			for _, cm := range cmList {
				if strings.Contains(cm.GetName(), "rendered-") {
					o.Expect(cm.Delete()).Should(o.Succeed(), "The ConfigMap related to MOSC has not been removed")
				}
			}
		}()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		exutil.By("Check that a new build has been triggered")
		o.Eventually(mosc.GetCurrentMachineOSBuild, "5m", "20s").Should(Exist(),
			"No build was created when OCB was enabled")
		mosb, err := mosc.GetCurrentMachineOSBuild()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error getting MOSB from MOSC")
		o.Eventually(mosb.GetJob, "5m", "20s").Should(Exist(),
			"No build pod was created when OCB was enabled")
		o.Eventually(mosb, "5m", "20s").Should(HaveConditionField("Building", "status", TrueString),
			"MachineOSBuild didn't report that the build has begun")
		logger.Infof("OK!\n")

		exutil.By("Delete the MCOS and check it is deleted")
		o.Expect(mosc.CleanupAndDelete()).To(o.Succeed(), "Error cleaning up %s", mosc)
		ValidateMOSCIsGarbageCollected(mosc, infraMcp)
		o.Expect(mosb).NotTo(Exist(), "Build is not deleted")
		o.Expect(mosc).NotTo(Exist(), "MOSC is not deleted")
		logger.Infof("OK!\n")

		exutil.By("Check MCC Logs for Panic is not produced")
		exutil.AssertAllPodsToBeReady(oc.AsAdmin(), MachineConfigNamespace)

		o.Expect(mcc.GetPreviousLogs()).NotTo(o.Or(o.ContainSubstring("panic"), o.ContainSubstring("Panic")), "Panic is seen in MCC pod after deleting OCB resources")
		o.Expect(mcc.GetLogs()).NotTo(o.Or(o.ContainSubstring("panic"), o.ContainSubstring("Panic")), "Panic is seen in MCC pod after deleting OCB resources")
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-ConnectedOnly-Longduration-NonPreRelease-Critical-73496-[P1] OCB use custom Containerfile. New 4.16 OCB API[Disruptive]", func() {
		var (
			mcp = GetCompactCompatiblePool(oc.AsAdmin())

			containerFileContent = `
	# Pull the centos base image and enable the EPEL repository.
        FROM quay.io/centos/centos:stream9 AS centos
        RUN dnf install -y epel-release

        # Pull an image containing the yq utility.
        FROM quay.io/multi-arch/yq:4.25.3 AS yq

        # Build the final OS image for this MachineConfigPool.
        FROM configs AS final

        # Copy the EPEL configs into the final image.
        COPY --from=yq /usr/bin/yq /usr/bin/yq
        COPY --from=centos /etc/yum.repos.d /etc/yum.repos.d
        COPY --from=centos /etc/pki/rpm-gpg/RPM-GPG-KEY-* /etc/pki/rpm-gpg/

        # Install cowsay and ripgrep from the EPEL repository into the final image,
        # along with a custom cow file.
        RUN sed -i 's/\$stream/9-stream/g' /etc/yum.repos.d/centos*.repo && \
            rpm-ostree install cowsay ripgrep
`

			checkers = []Checker{
				CommandOutputChecker{
					Command:  []string{"cowsay", "-t", "hello"},
					Matcher:  o.ContainSubstring("< hello >"),
					ErrorMsg: fmt.Sprintf("Cowsay is not working after installing the new image"),
					Desc:     fmt.Sprintf("Check that cowsay is installed and working"),
				},
			}
		)

		testContainerFile([]ContainerFile{{Content: containerFileContent}}, MachineConfigNamespace, mcp, checkers)
	})

	g.It("Author:sregidor-ConnectedOnly-Longduration-NonPreRelease-Medium-78001-[P2] The etc-pki-etitlement secret is created automatically for OCB Use custom Containerfile with rhel enablement [Disruptive]", func() {
		var (
			entitlementSecret    = NewSecret(oc.AsAdmin(), "openshift-config-managed", "etc-pki-entitlement")
			containerFileContent = `
        FROM configs AS final
    
        RUN rm -rf /etc/rhsm-host && \
          rpm-ostree install buildah && \
          ln -s /run/secrets/rhsm /etc/rhsm-host && \
          ostree container commit
`

			checkers = []Checker{
				CommandOutputChecker{
					Command:  []string{"rpm", "-q", "buildah"},
					Matcher:  o.ContainSubstring("buildah-"),
					ErrorMsg: fmt.Sprintf("Buildah package is not installed after the image was deployed"),
					Desc:     fmt.Sprintf("Check that buildah is installed"),
				},
			}

			mcp = GetCompactCompatiblePool(oc.AsAdmin())
		)

		if !entitlementSecret.Exists() {
			g.Skip(fmt.Sprintf("There is no entitlement secret available in this cluster %s. This test case cannot be executed", entitlementSecret))
		}

		testContainerFile([]ContainerFile{{Content: containerFileContent}}, MachineConfigNamespace, mcp, checkers)
	})

	g.It("Author:sregidor-ConnectedOnly-Longduration-NonPreRelease-High-73947-OCB use OutputImage CurrentImagePullSecret [Disruptive]", func() {
		var (
			mcp              = GetCompactCompatiblePool(oc.AsAdmin())
			tmpNamespaceName = "tc-73947-mco-ocl-images"
			checkers         = []Checker{
				CommandOutputChecker{
					Command:  []string{"rpm-ostree", "status"},
					Matcher:  o.ContainSubstring(fmt.Sprintf("%s/%s/ocb-%s-image", InternalRegistrySvcURL, tmpNamespaceName, mcp.GetName())),
					ErrorMsg: fmt.Sprintf("The nodes are not using the expected OCL image stored in the internal registry"),
					Desc:     fmt.Sprintf("Check that the nodes are using the righ OS image"),
				},
			}
		)

		testContainerFile([]ContainerFile{}, tmpNamespaceName, mcp, checkers)
	})

	g.It("Author:sregidor-ConnectedOnly-Longduration-NonPreRelease-High-72003-[P1] OCB Opting into on-cluster builds must respect maxUnavailable setting. Workers.[Disruptive]", func() {
		SkipIfSNO(oc.AsAdmin()) // This test makes no sense in SNO

		var (
			moscName    = "test-" + GetCurrentTestPolarionIDNumber()
			wMcp        = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			workerNodes = wMcp.GetSortedNodesOrFail()
		)

		exutil.By("Configure maxUnavailable if worker pool has more than 2 nodes")
		if len(workerNodes) > 2 {
			wMcp.SetMaxUnavailable(2)
			defer wMcp.RemoveMaxUnavailable()
		}

		maxUnavailable := exutil.OrFail[int](wMcp.GetMaxUnavailableInt())
		logger.Infof("Current maxUnavailable value %d", maxUnavailable)
		logger.Infof("OK!\n")

		exutil.By("Configure OCB functionality for the new worker MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, wMcp.GetName(), nil)
		defer DisableOCL(mosc)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		exutil.By("Configure OCB functionality for the new worker MCP")
		o.Eventually(wMcp.GetUpdatingStatus, "15m", "15s").Should(o.Equal("True"),
			"The worker MCP did not start updating")
		logger.Infof("OK!\n")

		exutil.By("Poll the nodes sorted by the order they are updated")
		updatedNodes := wMcp.GetSortedUpdatedNodes(maxUnavailable)
		for _, n := range updatedNodes {
			logger.Infof("updated node: %s created: %s zone: %s", n.GetName(), n.GetOrFail(`{.metadata.creationTimestamp}`), n.GetOrFail(`{.metadata.labels.topology\.kubernetes\.io/zone}`))
		}
		logger.Infof("OK!\n")

		exutil.By("Wait for the configuration to be applied in all nodes")
		wMcp.waitForComplete()
		logger.Infof("OK!\n")

		exutil.By("Check that nodes were updated in the right order")
		rightOrder := checkUpdatedLists(workerNodes, updatedNodes, maxUnavailable)
		o.Expect(rightOrder).To(o.BeTrue(), "Expected update order %s, but found order %s", workerNodes, updatedNodes)
		logger.Infof("OK!\n")

		exutil.By("Remove the MachineOSConfig resource")
		o.Expect(DisableOCL(mosc)).To(o.Succeed(), "Error cleaning up %s", mosc)
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-ConnectedOnly-Longduration-NonPreRelease-High-73497-[P2] OCB build images in many MCPs at the same time [Disruptive]", func() {
		SkipIfCompactOrSNO(oc.AsAdmin()) // This test makes no sense in SNO or compact

		var (
			customMCPNames = "infra"
			numCustomPools = 5
			moscList       = []*MachineOSConfig{}
			mcpList        = []*MachineConfigPool{}
			wg             sync.WaitGroup
		)

		exutil.By("Create custom MCPS")
		for i := 0; i < numCustomPools; i++ {
			infraMcpName := fmt.Sprintf("%s-%d", customMCPNames, i)
			infraMcp, err := CreateCustomMCP(oc.AsAdmin(), infraMcpName, 0)
			defer infraMcp.delete()
			o.Expect(err).NotTo(o.HaveOccurred(), "Error creating a new custom pool: %s", infraMcpName)
			mcpList = append(mcpList, infraMcp)

		}
		logger.Infof("OK!\n")

		exutil.By("Checking that all MOSCs were executed properly")
		for _, infraMcp := range mcpList {
			moscName := fmt.Sprintf("mosc-%s", infraMcp.GetName())

			mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, infraMcp.GetName(), nil)
			defer mosc.CleanupAndDelete()
			o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
			moscList = append(moscList, mosc)

			wg.Add(1)
			go func() {
				defer g.GinkgoRecover()
				defer wg.Done()

				ValidateSuccessfulMOSC(mosc, nil)
			}()
		}

		wg.Wait()
		logger.Infof("OK!\n")

		exutil.By("Removing all MOSC resources")
		for _, mosc := range moscList {
			o.Expect(mosc.CleanupAndDelete()).To(o.Succeed(), "Error cleaning up %s", mosc)
		}
		logger.Infof("OK!\n")

		exutil.By("Validate that all resources were garbage collected")
		for i := 0; i < numCustomPools; i++ {
			ValidateMOSCIsGarbageCollected(moscList[i], mcpList[i])
		}
		logger.Infof("OK!\n")

		exutil.AssertAllPodsToBeReady(oc.AsAdmin(), MachineConfigNamespace)
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-77498-OCB Trigger new build when renderedImagePushspec is updated [Disruptive]", func() {

		var (
			infraMcpName = "infra"
			moscName     = "tc-77498-infra"
		)

		exutil.By("Create custom infra MCP")
		// We add no workers to the infra pool, it is not necessary
		infraMcp, err := CreateCustomMCP(oc.AsAdmin(), infraMcpName, 0)
		defer infraMcp.delete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating a new custom pool: %s", infraMcpName)
		logger.Infof("OK!\n")

		exutil.By("Configure OCB functionality for the new infra MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, infraMcpName, nil)
		defer mosc.CleanupAndDelete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		ValidateSuccessfulMOSC(mosc, nil)

		exutil.By("Set a new rendered image pull spec")
		initialMOSB := exutil.OrFail[*MachineOSBuild](mosc.GetCurrentMachineOSBuild())
		initialRIPS := exutil.OrFail[string](mosc.GetRenderedImagePushspec())
		o.Expect(
			mosc.SetRenderedImagePushspec(strings.ReplaceAll(initialRIPS, "ocb-", "ocb77498-")),
		).NotTo(o.HaveOccurred(), "Error patching %s to set the new renderedImagePullSpec", mosc)
		logger.Infof("OK!\n")

		exutil.By("Check that a new build is triggered")
		checkNewBuildIsTriggered(mosc, initialMOSB)
		logger.Infof("OK!\n")

		exutil.By("Set the original rendered image pull spec")
		o.Expect(
			mosc.SetRenderedImagePushspec(initialRIPS),
		).NotTo(o.HaveOccurred(), "Error patching %s to set the new renderedImagePullSpec", mosc)
		logger.Infof("OK!\n")

		exutil.By("Check that the initial build is reused")
		var currentMOSB *MachineOSBuild
		o.Eventually(func() (string, error) {
			currentMOSB, err = mosc.GetCurrentMachineOSBuild()
			return currentMOSB.GetName(), err
		}, "5m", "20s").Should(o.Equal(initialMOSB.GetName()),
			"When the containerfiles were removed and initial MOSC configuration was restored, the initial MOSB was not used")

		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-77497-OCB Trigger new build when Containerfile is updated [Disruptive]", func() {

		var (
			infraMcpName     = "infra"
			moscName         = "tc-77497-infra"
			containerFile    = ContainerFile{Content: "RUN touch /etc/test-add-containerfile"}
			containerFileMod = ContainerFile{Content: "RUN touch /etc/test-modified-containerfile"}
		)

		exutil.By("Create custom infra MCP")
		// We add no workers to the infra pool, it is not necessary
		infraMcp, err := CreateCustomMCP(oc.AsAdmin(), infraMcpName, 0)
		defer infraMcp.delete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating a new custom pool: %s", infraMcpName)
		logger.Infof("OK!\n")

		exutil.By("Configure OCB functionality for the new infra MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, infraMcpName, nil)
		defer mosc.CleanupAndDelete()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		ValidateSuccessfulMOSC(mosc, nil)

		exutil.By("Add new container file")
		initialMOSB := exutil.OrFail[*MachineOSBuild](mosc.GetCurrentMachineOSBuild())

		o.Expect(
			mosc.SetContainerfiles([]ContainerFile{containerFile}),
		).NotTo(o.HaveOccurred(), "Error patching %s to add a container file", mosc)
		logger.Infof("OK!\n")

		exutil.By("Check that a new build is triggered when a containerfile is added")
		checkNewBuildIsTriggered(mosc, initialMOSB)
		logger.Infof("OK!\n")

		exutil.By("Modify the container file")
		currentMOSB := exutil.OrFail[*MachineOSBuild](mosc.GetCurrentMachineOSBuild())

		o.Expect(
			mosc.SetContainerfiles([]ContainerFile{containerFileMod}),
		).NotTo(o.HaveOccurred(), "Error patching %s to modify an existing container file", mosc)
		logger.Infof("OK!\n")

		exutil.By("Check that a new build is triggered when a containerfile is modified")
		checkNewBuildIsTriggered(mosc, currentMOSB)
		logger.Infof("OK!\n")

		exutil.By("Remove the container files")
		o.Expect(
			mosc.RemoveContainerfiles(),
		).NotTo(o.HaveOccurred(), "Error patching %s to remove the configured container files", mosc)
		logger.Infof("OK!\n")

		exutil.By("Check that the initial build is reused")
		o.Eventually(func() (string, error) {
			currentMOSB, err = mosc.GetCurrentMachineOSBuild()
			return currentMOSB.GetName(), err
		}, "5m", "20s").Should(o.Equal(initialMOSB.GetName()),
			"When the containerfiles were removed and initial MOSC configuration was restored, the initial MOSB was not used")

		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-77576-In OCB. Create a new MC while a build is running [Disruptive]", func() {

		var (
			mcp      = GetCompactCompatiblePool(oc.AsAdmin())
			node     = mcp.GetSortedNodesOrFail()[0]
			moscName = "test-" + mcp.GetName() + "-" + GetCurrentTestPolarionIDNumber()

			fileMode    = "0644" // decimal 420
			filePath    = "/etc/test-77576"
			fileContent = "test file"
			mcName      = "tc-77576-testfile"
			fileConfig  = getBase64EncodedFileConfig(filePath, fileContent, fileMode)
			mc          = NewMachineConfig(oc.AsAdmin(), mcName, mcp.GetName())
		)

		exutil.By("Configure OCB functionality for the new worker MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, mcp.GetName(), nil)
		defer DisableOCL(mosc)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		exutil.By("Check that a new build has been triggered and is building")
		var mosb *MachineOSBuild
		o.Eventually(func() (*MachineOSBuild, error) {
			var err error
			mosb, err = mosc.GetCurrentMachineOSBuild()
			return mosb, err
		}, "5m", "20s").Should(Exist(),
			"No build was created when OCB was enabled")
		o.Eventually(mosb.GetJob, "5m", "20s").Should(Exist(),
			"No build job was created when OCB was enabled")
		o.Eventually(mosb, "5m", "20s").Should(HaveConditionField("Building", "status", TrueString),
			"MachineOSBuild didn't report that the build has begun")
		logger.Infof("OK!\n")

		exutil.By("Create a MC to trigger a new build")
		defer mc.delete()
		err = mc.Create("-p", "NAME="+mcName, "-p", "POOL="+mcp.GetName(), "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())
		logger.Infof("OK!\n")

		exutil.By("Check that a new build is triggered and the old build is removed")
		checkNewBuildIsTriggered(mosc, mosb)
		o.Eventually(mosb, "2m", "20s").ShouldNot(Exist(), "The old MOSB %s was not deleted", mosb)
		logger.Infof("OK!\n")

		exutil.By("Wait for the configuration to be applied")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		exutil.By("Check that the MC was applied")
		rf := NewRemoteFile(node, filePath)
		o.Eventually(rf, "2m", "20s").Should(HaveContent(o.Equal(fileContent)),
			"%s doesn't have the right content", rf)
		logger.Infof("OK!\n")

		exutil.By("Remove the MachineOSConfig resource")
		o.Expect(DisableOCL(mosc)).To(o.Succeed(), "Error cleaning up %s", mosc)
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-77781-OCB Rebuild a successful build [Disruptive]", func() {

		var (
			mcp      = GetCompactCompatiblePool(oc.AsAdmin())
			moscName = "test-" + mcp.GetName() + "-" + GetCurrentTestPolarionIDNumber()
		)

		exutil.By("Configure OCB functionality for the new worker MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, mcp.GetName(), nil)
		defer DisableOCL(mosc)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		ValidateSuccessfulMOSC(mosc, nil)

		// rebuild the image and check that the image is properly applied in the nodes
		RebuildImageAndCheck(mosc)

		exutil.By("Remove the MachineOSConfig resource")
		o.Expect(DisableOCL(mosc)).To(o.Succeed(), "Error cleaning up %s", mosc)
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-77782-[P2] OCB Rebuild an interrupted build [Disruptive]", func() {

		var (
			mcp      = GetCompactCompatiblePool(oc.AsAdmin())
			moscName = "test-" + mcp.GetName() + "-" + GetCurrentTestPolarionIDNumber()
		)

		exutil.By("Configure OCB functionality for the new worker MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, mcp.GetName(), nil)
		defer DisableOCL(mosc)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		exutil.By("Wait until MOSB starts building")
		var mosb *MachineOSBuild
		var job *Job
		o.Eventually(func() (*MachineOSBuild, error) {
			var err error
			mosb, err = mosc.GetCurrentMachineOSBuild()
			return mosb, err
		}, "5m", "20s").Should(Exist(),
			"No build was created when OCB was enabled")
		o.Eventually(func() (*Job, error) {
			var err error
			job, err = mosb.GetJob()
			return job, err
		}, "5m", "20s").Should(Exist(),
			"No build job was created when OCB was enabled")
		o.Eventually(mosb, "5m", "20s").Should(HaveConditionField("Building", "status", TrueString),
			"MachineOSBuild didn't report that the build has begun")
		logger.Infof("OK!\n")

		exutil.By("Interrupt the build")
		o.Expect(job.Delete()).To(o.Succeed(),
			"Error deleting %s", job)
		o.Eventually(mosb, "5m", "20s").Should(HaveConditionField("Interrupted", "status", TrueString),
			"MachineOSBuild didn't report that the build has begun")
		logger.Infof("OK!\n")

		// TODO: what's the intended MCP status when a build is interrupted? We need to check this status here

		// rebuild the image and check that the image is properly applied in the nodes
		RebuildImageAndCheck(mosc)

		exutil.By("Remove the MachineOSConfig resource")
		o.Expect(DisableOCL(mosc)).To(o.Succeed(), "Error cleaning up %s", mosc)
		logger.Infof("OK!\n")
	})

	g.It("Author:ptalgulk-ConnectedOnly-Longduration-NonPreRelease-Medium-77977-Install extension after OCB is enabled [Disruptive]", func() {

		var (
			moscName = "test-" + GetCurrentTestPolarionIDNumber()
			mcp      = GetCompactCompatiblePool(oc.AsAdmin())
			node     = mcp.GetSortedNodesOrFail()[0]
			mcName   = "test-install-extenstion-" + GetCurrentTestPolarionIDNumber()
		)

		exutil.By("Configure OCB functionality for the new worker MCP")
		mosc, err := CreateMachineOSConfigUsingInternalRegistry(oc.AsAdmin(), MachineConfigNamespace, moscName, mcp.GetName(), nil)
		defer DisableOCL(mosc)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
		logger.Infof("OK!\n")

		ValidateSuccessfulMOSC(mosc, nil)

		exutil.By("Create a MC")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.SetParams(`EXTENSIONS=["usbguard"]`)
		defer mc.delete()
		mc.create()

		exutil.By("Wait for the configuration to be applied")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		exutil.By("Verify worker node includes usbguard extenstion")
		o.Expect(
			node.DebugNodeWithChroot("rpm", "-q", "usbguard"),
		).Should(o.ContainSubstring("usbguard-"), "usbguard has not been installed")
		logger.Infof("OK!\n")

		exutil.By("Delete a MC.")
		mc.delete()
		logger.Infof("OK!\n")

		exutil.By("Remove the MachineOSConfig resource")
		o.Expect(DisableOCL(mosc)).To(o.Succeed(), "Error cleaning up %s", mosc)
		ValidateMOSCIsGarbageCollected(mosc, mcp)
		logger.Infof("OK!\n")
	})
})

func testContainerFile(containerFiles []ContainerFile, imageNamespace string, mcp *MachineConfigPool, checkers []Checker) {
	var (
		oc       = mcp.GetOC().AsAdmin()
		moscName = "test-" + GetCurrentTestPolarionIDNumber()
		mosc     *MachineOSConfig
		err      error
	)
	exutil.By("Configure OCB functionality for the new infra MCP. Create MOSC")
	switch imageNamespace {
	case MachineConfigNamespace:
		mosc, err = CreateMachineOSConfigUsingInternalRegistry(oc, MachineConfigNamespace, moscName, mcp.GetName(), containerFiles)
	default:
		tmpNamespace := NewResource(oc.AsAdmin(), "ns", imageNamespace)
		if !tmpNamespace.Exists() {
			defer tmpNamespace.Delete()
			o.Expect(oc.AsAdmin().WithoutNamespace().Run("new-project").Args(tmpNamespace.GetName(), "--skip-config-write").Execute()).To(o.Succeed(), "Error creating a new project to store the OCL images")
		}
		mosc, err = CreateMachineOSConfigUsingInternalRegistry(oc, tmpNamespace.GetName(), moscName, mcp.GetName(), containerFiles)
	}
	defer DisableOCL(mosc)
	o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the MachineOSConfig resource")
	logger.Infof("OK!\n")

	verifyEntitlementSecretIsPresent(oc.AsAdmin(), mcp)

	ValidateSuccessfulMOSC(mosc, checkers)

	exutil.By("Remove the MachineOSConfig resource")
	o.Expect(DisableOCL(mosc)).To(o.Succeed(), "Error cleaning up %s", mosc)
	logger.Infof("OK!\n")

	ValidateMOSCIsGarbageCollected(mosc, mcp)

}

func skipTestIfOCBIsEnabled(oc *exutil.CLI) {
	moscl := NewMachineOSConfigList(oc)
	allMosc := moscl.GetAllOrFail()
	if len(allMosc) != 0 {
		moscl.PrintDebugCommand()
		g.Skip(fmt.Sprintf("To run this test case we need that OCB is not enabled in any pool. At least %s OBC is enabled in this cluster.", allMosc[0]))
	}
}

func checkMisconfiguredMOSC(oc *exutil.CLI, moscName, poolName, currentImagePullSecret, baseImagePullSecret, renderedImagePushSecret, pushSpec string, containerFile []ContainerFile,
	expectedMsg, stepMgs string) {
	var (
		machineConfigCO = NewResource(oc.AsAdmin(), "co", "machine-config")
	)

	exutil.By(stepMgs)
	defer logger.Infof("OK!\n")

	logger.Infof("Create a misconfiugred MOSC")
	mosc, err := CreateMachineOSConfig(oc, moscName, poolName, currentImagePullSecret, baseImagePullSecret, renderedImagePushSecret, pushSpec, containerFile)
	defer mosc.Delete()
	o.Expect(err).NotTo(o.HaveOccurred(), "Error creating MOSC with wrong pull secret")
	logger.Infof("OK!")

	logger.Infof("Expect machine-config CO to be degraded")
	o.Eventually(machineConfigCO, "5m", "20s").Should(BeDegraded(),
		"%s should be degraded when a MOSC is configured with a wrong pull secret", machineConfigCO)
	o.Eventually(machineConfigCO, "1m", "20s").Should(HaveConditionField("Degraded", "message", o.ContainSubstring(expectedMsg)),
		"%s should be degraded when a MOSC is configured with a wrong pull secret", machineConfigCO)
	logger.Infof("OK!")

	logger.Infof("Delete the offending MOSC")
	o.Expect(mosc.Delete()).To(o.Succeed(), "Error deleing the offendint MOSC %s", mosc)
	logger.Infof("OK!")

	logger.Infof("CHeck that machine-config CO is not degraded anymore")
	o.Eventually(machineConfigCO, "5m", "20s").ShouldNot(BeDegraded(),
		"%s should stop being degraded when the offending MOSC is deleted", machineConfigCO)

}

// ValidateMOSCIsGarbageCollected makes sure that all resources related to the provided MOSC have been removed
func ValidateMOSCIsGarbageCollected(mosc *MachineOSConfig, mcp *MachineConfigPool) {
	exutil.By("Check that the OCB resources are cleaned up")

	logger.Infof("Validating that MOSB resources were garbage collected")
	NewMachineOSBuildList(mosc.GetOC()).PrintDebugCommand() // for debugging purposes

	o.Eventually(mosc.GetMachineOSBuildList, "2m", "20s").Should(o.HaveLen(0), "MachineSOBuilds were not cleaned when %s was removed", mosc)

	logger.Infof("Validating that machine-os-builder pod was garbage collected")
	mOSBuilder := NewNamespacedResource(mosc.GetOC().AsAdmin(), "deployment", MachineConfigNamespace, "machine-os-builder")
	o.Eventually(mOSBuilder, "2m", "30s").ShouldNot(Exist(),
		"The machine-os-builder deployment was not removed when the infra pool was unlabeled")

	logger.Infof("Validating that configmaps were garbage collected")
	for _, cm := range NewConfigMapList(mosc.GetOC(), MachineConfigNamespace).GetAllOrFail() {
		o.Expect(cm.GetName()).NotTo(o.ContainSubstring("rendered-"+mcp.GetName()),
			"%s should have been garbage collected by OCB when the %s was deleted", cm, mosc)
	}
	logger.Infof("OK!")

	exutil.By("Verify the etc-pki-entitlement secret is removed")
	oc := mosc.GetOC()
	secretName := fmt.Sprintf("etc-pki-entitlement-%s", mcp.GetName())
	entitlementSecretInMco := NewSecret(oc.AsAdmin(), "openshift-machine-config-operator", secretName)
	o.Eventually(entitlementSecretInMco.Exists, "5m", "30s").Should(o.BeFalse(), "Error etc-pki-entitlement should not exist")
	logger.Infof("OK!\n")

}

// ValidateSuccessfulMOSC check that the provided MOSC is successfully applied
func ValidateSuccessfulMOSC(mosc *MachineOSConfig, checkers []Checker) {
	mcp, err := mosc.GetMachineConfigPool()
	o.Expect(err).NotTo(o.HaveOccurred(), "Cannot get the MCP for %s", mosc)

	exutil.By("Check that the deployment machine-os-builder is created")
	mOSBuilder := NewNamespacedResource(mosc.GetOC(), "deployment", MachineConfigNamespace, "machine-os-builder")

	o.Eventually(mOSBuilder, "5m", "30s").Should(Exist(),
		"The machine-os-builder deployment was not created when the OCB functionality was enabled in the infra pool")

	o.Expect(mOSBuilder.Get(`{.spec.template.spec.containers[?(@.name=="machine-os-builder")].command}`)).To(o.ContainSubstring("machine-os-builder"),
		"Error the machine-os-builder is not invoking the machine-os-builder binary")

	o.Eventually(mOSBuilder.Get, "3m", "30s").WithArguments(`{.spec.replicas}`).Should(o.Equal("1"),
		"The machine-os-builder deployment was created but the configured number of replicas is not the expected one")
	o.Eventually(mOSBuilder.Get, "2m", "30s").WithArguments(`{.status.availableReplicas}`).Should(o.Equal("1"),
		"The machine-os-builder deployment was created but the available number of replicas is not the expected one")

	exutil.AssertAllPodsToBeReady(mosc.GetOC(), MachineConfigNamespace)
	logger.Infof("OK!\n")

	exutil.By("Check that the  machine-os-builder is using leader election without failing")
	o.Expect(mOSBuilder.Logs()).To(o.And(
		o.ContainSubstring("attempting to acquire leader lease openshift-machine-config-operator/machine-os-builder"),
		o.ContainSubstring("successfully acquired lease openshift-machine-config-operator/machine-os-builder")),
		"The machine os builder pod is not using the leader election without failures")
	logger.Infof("OK!\n")

	exutil.By("Check that a new build has been triggered")
	o.Eventually(mosc.GetCurrentMachineOSBuild, "5m", "20s").Should(Exist(),
		"No build was created when OCB was enabled")
	mosb, err := mosc.GetCurrentMachineOSBuild()
	o.Expect(err).NotTo(o.HaveOccurred(), "Error getting MOSB from MOSC")
	o.Eventually(mosb.GetJob, "2m", "20s").Should(Exist(),
		"No build job was created when OCB was enabled")
	o.Eventually(mosb, "5m", "20s").Should(HaveConditionField("Building", "status", TrueString),
		"MachineOSBuild didn't report that the build has begun")
	logger.Infof("OK!\n")

	exutil.By("Check that a new build is successfully executed")
	o.Eventually(mosb, "20m", "20s").Should(HaveConditionField("Building", "status", FalseString), "Build was not finished")
	o.Eventually(mosb, "10m", "20s").Should(HaveConditionField("Succeeded", "status", TrueString), "Build didn't succeed")
	o.Eventually(mosb, "2m", "20s").Should(HaveConditionField("Interrupted", "status", FalseString), "Build was interrupted")
	o.Eventually(mosb, "2m", "20s").Should(HaveConditionField("Failed", "status", FalseString), "Build was failed")
	logger.Infof("Check that the build job was deleted")
	o.Eventually(mosb.GetJob, "2m", "20s").ShouldNot(Exist(), "Build job was not cleaned")
	logger.Infof("OK!\n")

	numNodes, err := mcp.getMachineCount()
	o.Expect(err).NotTo(o.HaveOccurred(), "Error getting MachineCount from %s", mcp)
	if numNodes > 0 {
		exutil.By("Wait for the new image to be applied")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		node := mcp.GetSortedNodesOrFail()[0]
		exutil.By("Check that the right image is deployed in the nodes")
		currentImagePullSpec, err := mosc.GetStatusCurrentImagePullSpec()
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error getting the current image pull spec in %s", mosc)
		o.Expect(node.GetCurrentBootOSImage()).To(o.Equal(currentImagePullSpec),
			"The image installed in node %s is not the expected one", mosc)
		logger.Infof("OK!\n")

		for _, checker := range checkers {
			checker.Check(node)
		}
	} else {
		logger.Infof("There is no node configured in %s. We don't wait for the configuration to be applied", mcp)
	}
}

// DisableOCL this function disables OCL.
func DisableOCL(mosc *MachineOSConfig) error {
	if !mosc.Exists() {
		logger.Infof("%s does not exist. No need to remove/disable it", mosc)
		return nil
	}

	mcp, err := mosc.GetMachineConfigPool()
	if err != nil {
		return err
	}

	currentOSImageSpec, err := mosc.GetStatusCurrentImagePullSpec()
	if err != nil {
		return err
	}

	err = mosc.CleanupAndDelete()
	if err != nil {
		return err
	}

	nodes, err := mcp.GetCoreOsNodes()
	if err != nil {
		return err
	}

	if len(nodes) > 0 {
		node := nodes[0]

		mcp.waitForComplete()

		o.Expect(node.GetCurrentBootOSImage()).NotTo(o.Equal(currentOSImageSpec),
			"OCL was disabled in %s but the OCL image is still used in %s", node)
	} else {
		logger.Infof("There is no coreos node configured in %s. We don't wait for the configuration to be applied and we don't execute any verification on the nodes", mcp)
	}

	return nil
}

// checkNewBuildIsTriggered executes the necessary validations to make sure that a new build was triggered and succeeded. Fails the test case if validaions fail
func checkNewBuildIsTriggered(mosc *MachineOSConfig, currentMOSB *MachineOSBuild) {
	var (
		newMOSB *MachineOSBuild
		err     error
	)
	logger.Infof("Current mosb: %s", currentMOSB)
	o.Eventually(func() (string, error) {
		newMOSB, err = mosc.GetCurrentMachineOSBuild()
		return newMOSB.GetName(), err
	}, "5m", "20s").ShouldNot(o.Equal(currentMOSB.GetName()),
		"A new MOSB should be created after the new rendered image pull spec is configured")

	logger.Infof("New mosb: %s", newMOSB)

	o.Eventually(newMOSB, "5m", "20s").Should(HaveConditionField("Building", "status", TrueString),
		"MachineOSBuild didn't report that the build has begun")

	o.Eventually(newMOSB, "20m", "20s").Should(HaveConditionField("Building", "status", FalseString), "Build was not finished")
	o.Eventually(newMOSB, "10m", "20s").Should(HaveConditionField("Succeeded", "status", TrueString), "Build didn't succeed")
	o.Eventually(newMOSB, "2m", "20s").Should(HaveConditionField("Interrupted", "status", FalseString), "Build was interrupted")
	o.Eventually(newMOSB, "2m", "20s").Should(HaveConditionField("Failed", "status", FalseString), "Build was failed")
}

func verifyEntitlementSecretIsPresent(oc *exutil.CLI, mcp *MachineConfigPool) {
	entitlementSecret := NewSecret(oc.AsAdmin(), "openshift-config-managed", "etc-pki-entitlement")
	secretName := fmt.Sprintf("etc-pki-entitlement-%s", mcp.GetName())
	entitlementSecretInMco := NewSecret(oc.AsAdmin(), "openshift-machine-config-operator", secretName)

	exutil.By("Verify the etc-pki-entitlement secret is present in openshift-config-managed namespace ")
	if entitlementSecret.Exists() {
		exutil.By("Verify the etc-pki-entitlement secret is present")
		logger.Infof("%s\n", entitlementSecretInMco)
		o.Eventually(entitlementSecretInMco.Exists, "5m", "30s").Should(o.BeTrue(), "Error etc-pki-entitlement should exist")
		logger.Infof("OK!\n")
	} else {
		logger.Infof("etc-pki-entitlement does not exist in openshift-config-managed namespace")
	}
}

// RebuildImageAndCheck rebuild the latest image of the MachineOSConfig resource and checks that it is properly built and applied
func RebuildImageAndCheck(mosc *MachineOSConfig) {
	exutil.By("Rebuild the current image")

	var (
		mcp                  = exutil.OrFail[*MachineConfigPool](mosc.GetMachineConfigPool())
		mosb                 = exutil.OrFail[*MachineOSBuild](mosc.GetCurrentMachineOSBuild())
		currentImagePullSpec = exutil.OrFail[string](mosc.GetStatusCurrentImagePullSpec())
	)

	o.Expect(mosc.Rebuild()).To(o.Succeed(),
		"Error patching %s to rebuild the current image", mosc)
	logger.Infof("OK!\n")

	exutil.By("Check that the existing MOSB is reused and it builds a new image")
	o.Eventually(mosb.GetJob, "2m", "20s").Should(Exist(), "Rebuild job was not created")
	o.Eventually(mosb, "20m", "20s").Should(HaveConditionField("Building", "status", FalseString), "Rebuild was not finished")
	o.Eventually(mosb, "10m", "20s").Should(HaveConditionField("Succeeded", "status", TrueString), "Rebuild didn't succeed")
	o.Eventually(mosb, "2m", "20s").Should(HaveConditionField("Interrupted", "status", FalseString), "Reuild was interrupted")
	o.Eventually(mosb, "2m", "20s").Should(HaveConditionField("Failed", "status", FalseString), "Reuild was failed")
	logger.Infof("Check that the rebuild job was deleted")
	o.Eventually(mosb.GetJob, "2m", "20s").ShouldNot(Exist(), "Build job was not cleaned")
	logger.Infof("OK!\n")

	exutil.By("Wait for the new image to be applied")
	nodes, err := mcp.GetCoreOsNodes()
	o.Expect(err).NotTo(o.HaveOccurred(), "Error getting coreos nodes from %s", mcp)
	if len(nodes) > 0 {
		node := nodes[0]

		mcp.waitForComplete()
		logger.Infof("OK!\n")

		exutil.By("Check that the new image is the one used in the nodes")
		newImagePullSpec := exutil.OrFail[string](mosc.GetStatusCurrentImagePullSpec())
		o.Expect(newImagePullSpec).NotTo(o.Equal(currentImagePullSpec),
			"The new image after the rebuild operation should be different fron the initial image")
		o.Expect(node.GetCurrentBootOSImage()).To(o.Equal(newImagePullSpec),
			"The new image is not being used in node %s", node)
		logger.Infof("OK!\n")
	} else {
		logger.Infof("There is no coreos node configured in %s. We don't wait for the configuration to be applied and we don't execute any verification on the nodes", mcp)
		logger.Infof("OK!\n")
	}
}
