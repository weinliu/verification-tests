package mco

import (
	"fmt"
	"os"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

var _ = g.Describe("[sig-mco] MCO Upgrade", func() {
	defer g.GinkgoRecover()

	var (
		// init cli object, temp namespace contains prefix mco.
		// tip: don't put this in BeforeEach/JustBeforeEach, you will get error
		// "You may only call AfterEach from within a Describe, Context or When"
		oc = exutil.NewCLI("mco-upgrade", exutil.KubeConfigPath())
		// temp dir to store all test files, and it will be recycled when test is finished
		tmpdir string
		wMcp   *MachineConfigPool
	)

	g.JustBeforeEach(func() {
		tmpdir = createTmpDir()
		preChecks(oc)
		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
	})

	g.JustAfterEach(func() {
		os.RemoveAll(tmpdir)
		logger.Infof("test dir %s is cleaned up", tmpdir)
	})

	g.It("NonHyperShiftHOST-Author:rioliu-PstChkUpgrade-NonPreRelease-High-45550-upgrade cluster is failed on RHEL node", func() {

		skipTestIfOsIsNotRhelOs(oc)

		g.By("iterate all rhel nodes to check the machine config related annotations")

		allRhelNodes := NewNodeList(oc).GetAllRhelWokerNodesOrFail()
		for _, node := range allRhelNodes {
			state := node.GetAnnotationOrFail(NodeAnnotationState)
			reason := node.GetAnnotationOrFail(NodeAnnotationReason)
			logger.Infof("checking node %s ...", node.GetName())
			o.Expect(state).Should(o.Equal("Done"), fmt.Sprintf("annotation [%s] value is not expected: %s", NodeAnnotationState, state))
			o.Expect(reason).ShouldNot(o.ContainSubstring(`Failed to find /dev/disk/by-label/root`),
				fmt.Sprintf("annotation [%s] value has unexpected error message", NodeAnnotationReason))
		}

	})

	g.It("NonHyperShiftHOST-Author:rioliu-PstChkUpgrade-NonPreRelease-High-55748-Upgrade failed with Transaction in progress", func() {

		g.By("check machine config daemon log to verify no error `Transaction in progress` found")

		allNodes, getNodesErr := NewNodeList(oc).GetAllLinux()
		o.Expect(getNodesErr).NotTo(o.HaveOccurred(), "Get all linux nodes error")
		for _, node := range allNodes {
			logger.Infof("checking mcd log on %s", node.GetName())
			errLog, getLogErr := node.GetMCDaemonLogs("'Transaction in progress: (null)'")
			o.Expect(getLogErr).Should(o.HaveOccurred(), "Unexpected error found in MCD log")
			o.Expect(errLog).Should(o.BeEmpty(), "Transaction in progress error found, it is unexpected")
			logger.Infof("no error found")
		}
	})

	g.It("NonHyperShiftHOST-Author:rioliu-PstChkUpgrade-NonPreRelease-High-59427-ssh keys can be migrated to new dir when node is upgraded from RHCOS8 to RHCOS9", func() {

		var (
			oldAuthorizedKeyPath = "/home/core/.ssh/authorized_key"
			newAuthorizedKeyPath = "/home/core/.ssh/authorized_keys.d/ignition"
		)

		allCoreOsNodes := NewNodeList(oc).GetAllCoreOsNodesOrFail()
		for _, node := range allCoreOsNodes {
			g.By(fmt.Sprintf("check authorized key dir and file on %s", node.GetName()))
			o.Eventually(func(gm o.Gomega) {
				output, err := node.DebugNodeWithChroot("stat", oldAuthorizedKeyPath)
				gm.Expect(err).Should(o.HaveOccurred(), "old authorized key file still exists")
				gm.Expect(output).Should(o.ContainSubstring("No such file or directory"))
			}, "3m", "20s",
			).Should(o.Succeed(),
				"The old authorized key file still exists")

			output, err := node.DebugNodeWithChroot("stat", newAuthorizedKeyPath)
			o.Expect(err).ShouldNot(o.HaveOccurred(), "new authorized key file not found")
			o.Expect(output).Should(o.ContainSubstring("File: " + newAuthorizedKeyPath))
		}

	})

	g.It("NonHyperShiftHOST-Author:sregidor-PreChkUpgrade-NonPreRelease-High-62154-Don't render new MC until base MCs update [Disruptive]", func() {
		var (
			kcName     = "mco-tc-62154-kubeletconfig"
			kcTemplate = generateTemplateAbsolutePath("change-maxpods-kubelet-config.yaml")
			crName     = "mco-tc-62154-crconfig"
			crTemplate = generateTemplateAbsolutePath("generic-container-runtime-config.yaml")

			crConfig = `{"pidsLimit": 2048}`
		)

		if len(wMcp.GetNodesOrFail()) == 0 {
			g.Skip("Worker pool has 0 nodes configured.")
		}

		g.By("create kubelet config to add 500 max pods")
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		kc.create()

		g.By("create ContainerRuntimeConfig")
		cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
		cr.create("-p", "CRCONFIG="+crConfig)

		g.By("wait for worker pool to be ready")
		wMcp.waitForComplete()

	})

	g.It("NonHyperShiftHOST-Author:sregidor-PstChkUpgrade-NonPreRelease-High-62154-Don't render new MC until base MCs update  [Disruptive]", func() {

		var (
			kcName     = "mco-tc-62154-kubeletconfig"
			kcTemplate = generateTemplateAbsolutePath("change-maxpods-kubelet-config.yaml")
			crName     = "mco-tc-62154-crconfig"
			crTemplate = generateTemplateAbsolutePath("generic-container-runtime-config.yaml")

			mcKubeletConfigName                = "99-worker-generated-kubelet"
			mcContainerRuntimeConfigConfigName = "99-worker-generated-containerruntime"
		)

		// Skip if worker pool has no nodes
		if len(wMcp.GetNodesOrFail()) == 0 {
			g.Skip("Worker pool has 0 nodes configured.")
		}

		// Skip if the precheck part of the test was not executed
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		if !kc.Exists() {
			g.Skip(fmt.Sprintf(`The PreChkUpgrade part of the test should have created a KubeletConfig resource "%s". This resource does not exist in the cluster. Maybe we are upgrading from an old branch like 4.5?`, kc.GetName()))
		}
		defer wMcp.waitForComplete()
		defer kc.Delete()

		cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
		if !cr.Exists() {
			g.Skip(fmt.Sprintf(`The PreChkUpgrade part of the test should have created a ContainerRuntimConfig resource "%s". This resource does not exist in the cluster. Maybe we are upgrading from an old branch like 4.5?`, cr.GetName()))
		}
		defer cr.Delete()

		logger.Infof("Jira issure: https://issues.redhat.com/browse/OCPBUGS-6018")
		logger.Infof("PR: https://github.com/openshift/machine-config-operator/pull/3501")

		g.By("check that the MC in the worker pool has the right kubelet configuration")
		worker := wMcp.GetNodesOrFail()[0]
		config := NewRemoteFile(worker, "/etc/kubernetes/kubelet.conf")
		o.Expect(config.Fetch()).To(o.Succeed(),
			"Could not get the current kubelet config in node %s", worker.GetName())

		o.Expect(config.GetTextContent()).To(o.ContainSubstring(`"maxPods": 500`),
			"The kubelet configuration is not the expected one.")

		g.By("check controller versions")
		rmc, err := wMcp.GetConfiguredMachineConfig()
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Cannot get the MC configured for worker pool")

		logger.Infof("Get controller version in rendered MC %s", rmc.GetName())
		rmcCV := rmc.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/generated-by-controller-version}`)
		logger.Infof("rendered MC controller version %s", rmcCV)

		kblmc := NewMachineConfig(oc.AsAdmin(), mcKubeletConfigName, MachineConfigPoolWorker)
		logger.Infof("Get controller version in KubeletConfig generated MC %s", rmc.GetName())
		kblmcCV := kblmc.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/generated-by-controller-version}`)
		logger.Infof("KubeletConfig generated MC controller version %s", kblmcCV)

		crcmc := NewMachineConfig(oc.AsAdmin(), mcContainerRuntimeConfigConfigName, MachineConfigPoolWorker)
		logger.Infof("Get controller version in ContainerRuntimeConfig generated MC %s", rmc.GetName())
		crcmcCV := crcmc.GetOrFail(`{.metadata.annotations.machineconfiguration\.openshift\.io/generated-by-controller-version}`)
		logger.Infof("ContainerRuntimeConfig generated MC controller version %s", crcmcCV)

		o.Expect(kblmcCV).To(o.Equal(rmcCV),
			"KubeletConfig generated MC and worker pool rendered MC should have the same Controller Version annotation")
		o.Expect(crcmcCV).To(o.Equal(rmcCV),
			"ContainerRuntimeConfig generated MC and worker pool rendered MC should have the same Controller Version annotation")

	})

	g.It("NonHyperShiftHOST-Author:sregidor-PstChkUpgrade-NonPreRelease-Critical-64781-MCO should be compliant with CIS benchmark rule", func() {
		exutil.By("Verify that machine-config-opeartor pod is not using the default SA")

		o.Expect(
			oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", MachineConfigNamespace, "-l", "k8s-app=machine-config-operator",
				"-o", `jsonpath={.items[0].spec.serviceAccountName}`).Output(),
		).NotTo(o.Equal("default"),
			"machine-config-operator pod is using the 'default' serviceAccountName and it should not")
		logger.Infof("OK!\n")

		exutil.By("Verify that there is no clusterrolebinding for the default ServiceAccount")

		defaultSAClusterRoleBinding := NewResource(oc.AsAdmin(), "clusterrolebinding", "default-account-openshift-machine-config-operator")
		o.Expect(defaultSAClusterRoleBinding).NotTo(Exist(),
			"The old clusterrolebinding for the 'default' service account exists and it should not exist")
		logger.Infof("OK!\n")
	})
})
