package hypernto

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-node] PSAP should", func() {
	defer g.GinkgoRecover()

	var (
		oc                             = exutil.NewCLI("hypernto-test", exutil.KubeConfigPath())
		ntoNamespace                   = "openshift-cluster-node-tuning-operator"
		tunedWithSameProfileName       string
		tunedWithDiffProfileName       string
		tunedWithInvalidProfileName    string
		tunedWithNodeLevelProfileName  string
		tunedWithKernelBootProfileName string
		isNTO                          bool
		isNTO2                         bool
		guestClusterName               string
		guestClusterNS                 string
		guestClusterKube               string
		hostedClusterNS                string
		guestClusterName2              string
		guestClusterNS2                string
		guestClusterKube2              string
		hostedClusterNS2               string
		iaasPlatform                   string
		firstNodePoolName              string
		secondNodePoolName             string
	)

	g.BeforeEach(func() {
		//First Hosted Cluster
		guestClusterName, guestClusterKube, hostedClusterNS = exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		e2e.Logf("%s, %s, %s", guestClusterName, guestClusterKube, hostedClusterNS)

		guestClusterNS = hostedClusterNS + "-" + guestClusterName
		e2e.Logf("HostedClusterControlPlaneNS: %v", guestClusterNS)
		// ensure NTO operator is installed
		isNTO = isHyperNTOPodInstalled(oc, guestClusterNS)

		oc.SetGuestKubeconf(guestClusterKube)
		tunedWithSameProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "tuned-with-sameprofilename.yaml")
		tunedWithDiffProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "tuned-with-diffprofilename.yaml")
		tunedWithInvalidProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "nto-basic-tuning-sysctl-invalid.yaml")
		tunedWithNodeLevelProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "nto-basic-tuning-sysctl-nodelevel.yaml")
		tunedWithKernelBootProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "nto-basic-tuning-kernel-boot.yaml")

		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		e2e.Logf("Cloud provider is: %v", iaasPlatform)
	})

	g.It("HyperShiftMGMT-Author:liqcui-Medium-53875-NTO Support profile that have the same name with tuned on hypershift [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithSameProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=20")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-pidmax"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-pidmax in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in hosted cluster, this worker node will be labeled with hc-nodepool-pidmax=")
		workerNodeName, err := exutil.GetFirstLinuxWorkerNodeInHostedCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-pidmax\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-pidmax created in hosted cluster nodepool")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-pidmax is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-pidmax"))

		g.By("Get the tuned pod name that running on labeled node with hc-nodepool-pidmax=")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Label the worker nodes with hc-nodepool-pidmax=")
		defer oc.AsAdmin().AsGuestKubeconf().Run("label").Args("node", workerNodeName, "hc-nodepool-pidmax-").Execute()

		err = oc.AsAdmin().AsGuestKubeconf().Run("label").Args("node", workerNodeName, "hc-nodepool-pidmax=").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the tuned profile applied to labeled worker nodes with hc-nodepool-pidmax=")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "hc-nodepool-pidmax")

		g.By("Assert recommended profile (hc-nodepool-pidmax) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(hc-nodepool-pidmax\) matches current configuration`)

		g.By("Check if the setting of sysctl kernel.pid_max applied to labeled worker nodes, expected value is 868686")
		compareSpecifiedValueByNameOnLabelNodeWithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max", "868686")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)

		g.By("Check if the custom tuned profile removed from labeled worker nodes, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")

		pidMaxValue := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max")
		o.Expect(pidMaxValue).NotTo(o.BeEmpty())
		o.Expect(pidMaxValue).NotTo(o.ContainSubstring("868686"))
	})

	g.It("HyperShiftMGMT-Author:liqcui-Medium-53876-NTO Operand logs errors when applying profile with invalid settings in HyperShift. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//Delete configmap in hostedClusterNS namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-invalid", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-invalid in management cluster")
		exutil.ApplyOperatorResourceByYaml(oc, hostedClusterNS, tunedWithInvalidProfileName)

		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-invalid"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-invalid in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in hosted cluster, this worker node will be labeled with hc-nodepool-invalid=")
		workerNodeName, err := exutil.GetFirstLinuxWorkerNodeInHostedCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-invalid\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-invalid created in hosted cluster nodepool")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-invalid is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-invalid"))

		g.By("Get the tuned pod name that running on labeled node with hc-nodepool-invalid=")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Label the worker nodes with hc-nodepool-invalid=")
		defer oc.AsAdmin().AsGuestKubeconf().Run("label").Args("node", workerNodeName, "hc-nodepool-invalid-").Execute()

		err = oc.AsAdmin().AsGuestKubeconf().Run("label").Args("node", workerNodeName, "hc-nodepool-invalid=").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the tuned profile applied to labeled worker nodes with hc-nodepool-invalid=")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "hc-nodepool-invalid")

		g.By("Assert recommended profile (hc-nodepool-invalid) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(hc-nodepool-invalid\) matches current configuration|static tuning from profile 'hc-nodepool-invalid' applied`)

		g.By("Assert Failed to read sysctl parameter in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "20", 300, `ERROR    tuned.plugins.plugin_sysctl: Failed to read sysctl parameter`)

		expectedDegradedStatus, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io", workerNodeName, `-ojsonpath='{.status.conditions[?(@.type=="Degraded")].status}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(expectedDegradedStatus).NotTo(o.BeEmpty())
		o.Expect(expectedDegradedStatus).To(o.ContainSubstring("True"))

		g.By("Check if the setting of sysctl kernel.pid_max applied to labeled worker nodes, expected value is 868686")
		compareSpecifiedValueByNameOnLabelNodeWithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max", "868686")

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to labeled worker nodes, expected value is 56")
		compareSpecifiedValueByNameOnLabelNodeWithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max and vm.dirty_ratio rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-invalid", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)

		g.By("Check if the custom tuned profile removed from labeled worker nodes, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")

		pidMaxValue := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max")
		o.Expect(pidMaxValue).NotTo(o.BeEmpty())
		o.Expect(pidMaxValue).NotTo(o.ContainSubstring("868686"))

		vmDirtyRatioValue := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "vm.dirty_ratio")
		o.Expect(vmDirtyRatioValue).NotTo(o.BeEmpty())
		o.Expect(vmDirtyRatioValue).NotTo(o.ContainSubstring("56"))
	})

	g.It("HyperShiftMGMT-Author:liqcui-Medium-53877-NTO support tuning sysctl that applied to all nodes of nodepool-level settings in hypershift. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-vmdratio in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=20", "INCLUDE=openshift-node")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-vmdratio in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		workerNodeName, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, nodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-vmdratio created in hosted cluster nodepool")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-vmdratio is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		g.By("Get the tuned pod name that running on first node of nodepool")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Check if the tuned profile applied to all worker node in specifed nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(hc-nodepool-vmdratio)\ matches current configuration|static tuning from profile 'hc-nodepool-vmdratio' applied`)
		g.By("Check if the setting of sysctl vm.dirty_ratio applied to labeled worker nodes, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max and vm.dirty_ratio rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)

		g.By("Check if the custom tuned profile removed from worker nodes of nodepool, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "openshift-node")

		g.By("The value of vm.dirty_ratio on specified nodepool should not equal to 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")
	})

	g.It("HyperShiftMGMT-Author:liqcui-Medium-53886-NTO support tuning sysctl with different name that applied to one labeled node of nodepool in hypershift. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax-cm", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithDiffProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=20")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-pidmax-cm"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-pidmax in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in hosted cluster, this worker node will be labeled with hc-nodepool-pidmax=")
		workerNodeName, err := exutil.GetFirstLinuxWorkerNodeInHostedCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-pidmax-cm\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-pidmax created in hosted cluster nodepool")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-pidmax is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-pidmax-tuned"))

		g.By("Get the tuned pod name that running on labeled node with hc-nodepool-pidmax=")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Label the worker nodes with hc-nodepool-pidmax=")
		defer oc.AsAdmin().AsGuestKubeconf().Run("label").Args("node", workerNodeName, "hc-nodepool-pidmax-").Execute()

		err = oc.AsAdmin().AsGuestKubeconf().Run("label").Args("node", workerNodeName, "hc-nodepool-pidmax=").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the tuned profile applied to labeled worker nodes with hc-nodepool-pidmax=")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "hc-nodepool-pidmax-profile")

		g.By("Assert recommended profile (hc-nodepool-pidmax-profile) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(hc-nodepool-pidmax-profile\) matches current configuration`)

		g.By("Check if the setting of sysctl kernel.pid_max applied to labeled worker nodes, expected value is 868686")
		compareSpecifiedValueByNameOnLabelNodeWithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max", "868686")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax-cm", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)

		g.By("Check if the custom tuned profile removed from labeled worker nodes, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")

		pidMaxValue := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max")
		o.Expect(pidMaxValue).NotTo(o.BeEmpty())
		o.Expect(pidMaxValue).NotTo(o.ContainSubstring("868686"))
	})

	g.It("Longduration-NonPreRelease-HyperShiftMGMT-Author:liqcui-Medium-54522-NTO Applying tuning which requires kernel boot parameters. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--ignore-not-found").Execute()
			isMatch := exutil.CheckAllNodepoolReadyByHostedClusterName(oc, "hugepages-nodepool", hostedClusterNS, 300)
			o.Expect(isMatch).To(o.Equal(true))
		}()

		//Create custom node pool yaml file
		g.By("Create custom node pool in hosted cluster")
		exutil.CreateCustomNodePoolInHypershift(oc, "aws", guestClusterName, "hugepages-nodepool", "1", "m5.xlarge", hostedClusterNS)

		g.By("Check if custom node pool is ready in hosted cluster")
		exutil.AssertIfNodePoolIsReadyByName(oc, "hugepages-nodepool", 360, hostedClusterNS)

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hugepages", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap tuned-hugepages in management cluster")
		exutil.ApplyOperatorResourceByYaml(oc, hostedClusterNS, tunedWithKernelBootProfileName)
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hugepages"))

		g.By("Pick one worker node in custom node pool of hosted cluster")
		workerNodeName, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, "hugepages-nodepool")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hugepages-nodepool", "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"tuned-hugepages\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap tuned-hugepages-nodepool created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "hugepages-nodepool")
		o.Expect(configMaps).To(o.ContainSubstring("hugepages-nodepool"))

		g.By("Check if the configmap applied to tuned-hugepages-nodepool in management cluster")
		exutil.AssertIfNodePoolUpdatingConfigByName(oc, "hugepages-nodepool", 360, hostedClusterNS)

		g.By("Check if the tuned hugepages-xxxxxx is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hugepages"))

		g.By("Get the tuned pod name that running on custom node pool worker node")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Check if the tuned profile applied to custom node pool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node-hugepages")

		g.By("Assert hugepagesz match in /proc/cmdline on the worker node in custom node pool")
		assertIfMatchKenelBootOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "hugepagesz", true)

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hugepages", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hugepages", "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the worker node is ready after reboot due to removing kernel boot settings")
		AssertIfNodeIsReadyByNodeNameInHostedCluster(oc, workerNodeName, 360)

		g.By("Check if the removed configmap applied to tuned-hugepages-nodepool in management cluster")
		exutil.AssertIfNodePoolUpdatingConfigByName(oc, "hugepages-nodepool", 360, hostedClusterNS)

		g.By("Check if the custom tuned profile removed from labeled worker nodes, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")

		g.By("Assert hugepagesz match in /proc/cmdline on the worker node in custom node pool")
		assertIfMatchKenelBootOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "hugepagesz", false)
	})

	g.It("Longduration-NonPreRelease-HyperShiftMGMT-Author:liqcui-Medium-56609-NTO Scale out node pool which applied tuning with required kernel boot. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--ignore-not-found").Execute()
			isMatch := exutil.CheckAllNodepoolReadyByHostedClusterName(oc, "hugepages-nodepool", hostedClusterNS, 300)
			o.Expect(isMatch).To(o.Equal(true))
		}()

		//Create custom node pool yaml file
		g.By("Create custom node pool in hosted cluster")
		exutil.CreateCustomNodePoolInHypershift(oc, "aws", guestClusterName, "hugepages-nodepool", "1", "m5.xlarge", hostedClusterNS)

		g.By("Check if custom node pool is ready in hosted cluster")
		exutil.AssertIfNodePoolIsReadyByName(oc, "hugepages-nodepool", 720, hostedClusterNS)

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hugepages", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap tuned-hugepages in management cluster")
		exutil.ApplyOperatorResourceByYaml(oc, hostedClusterNS, tunedWithKernelBootProfileName)
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hugepages"))

		g.By("Pick one worker node in custom node pool of hosted cluster")
		workerNodeName, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, "hugepages-nodepool")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hugepages-nodepool", "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"tuned-hugepages\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap tuned-hugepages-nodepool created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "hugepages-nodepool")
		o.Expect(configMaps).To(o.ContainSubstring("hugepages-nodepool"))

		g.By("Check if the configmap applied to tuned-hugepages-nodepool in management cluster")
		exutil.AssertIfNodePoolUpdatingConfigByName(oc, "hugepages-nodepool", 360, hostedClusterNS)

		g.By("Check if the tuned hugepages-xxxxxx is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hugepages"))

		g.By("Get the tuned pod name that running on custom node pool worker node")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Check if the tuned profile applied to custom node pool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node-hugepages")

		g.By("Assert hugepagesz match in /proc/cmdline on the worker node in custom node pool")
		assertIfMatchKenelBootOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "hugepagesz", true)

		g.By("Scale out a new worker node in custom nodepool hugepages-nodepool")
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--replicas=2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if updating config applied to custom node pool in hosted cluster")
		exutil.AssertIfNodePoolUpdatingConfigByName(oc, "hugepages-nodepool", 720, hostedClusterNS)

		g.By("Check if the custom tuned profile openshift-node-hugepages applied to all nodes of custom nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "openshift-node-hugepages")

		g.By("Assert hugepagesz match in /proc/cmdline on all nodes include the second new worker node in custom node pool")
		assertIfMatchKenelBootOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "hugepagesz", true)

	})
	g.It("Longduration-NonPreRelease-HyperShiftMGMT-Author:liqcui-Medium-55360-NTO does not generate MachineConfigs with bootcmdline from manual change to Profile status.bootcmdline. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--ignore-not-found").Execute()
			isMatch := exutil.CheckAllNodepoolReadyByHostedClusterName(oc, "hugepages-nodepool", hostedClusterNS, 300)
			o.Expect(isMatch).To(o.Equal(true))
		}()

		//Create custom node pool yaml file
		g.By("Create custom node pool in hosted cluster")
		exutil.CreateCustomNodePoolInHypershift(oc, "aws", guestClusterName, "hugepages-nodepool", "1", "m5.xlarge", hostedClusterNS)

		g.By("Check if custom node pool is ready in hosted cluster")
		exutil.AssertIfNodePoolIsReadyByName(oc, "hugepages-nodepool", 720, hostedClusterNS)

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hugepages", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap tuned-hugepages in management cluster")
		exutil.ApplyOperatorResourceByYaml(oc, hostedClusterNS, tunedWithKernelBootProfileName)
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hugepages"))

		g.By("Pick one worker node in custom node pool of hosted cluster")

		workerNodeName, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, "hugepages-nodepool")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		g.By("Get operator pod name in hosted cluster controlplane namespaceh")
		ntoOperatorPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", guestClusterNS, "-lname=cluster-node-tuning-operator", "-ojsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ntoOperatorPodName).NotTo(o.BeEmpty())

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hugepages-nodepool", "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", "hugepages-nodepool", "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"tuned-hugepages\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap tuned-hugepages-nodepool created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "hugepages-nodepool")
		o.Expect(configMaps).NotTo(o.BeEmpty())
		o.Expect(configMaps).To(o.ContainSubstring("hugepages-nodepool"))

		g.By("Check if the configmap applied to tuned-hugepages-nodepool in management cluster")
		exutil.AssertIfNodePoolUpdatingConfigByName(oc, "hugepages-nodepool", 360, hostedClusterNS)

		g.By("Check if the tuned hugepages-xxxxxx is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hugepages"))

		g.By("Get the tuned pod name that running on custom node pool worker node")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Check if the tuned profile applied to custom node pool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node-hugepages")

		g.By("Assert hugepagesz match in /proc/cmdline on the worker node in custom node pool")
		assertIfMatchKenelBootOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "hugepagesz", true)

		g.By("Manually change the hugepage value in the worker node of custom nodepool hugepages-nodepool in hosted clusters")
		err = oc.AsAdmin().AsGuestKubeconf().Run("patch").Args("-n", ntoNamespace, "profile/"+workerNodeName, "--type", "merge", "-p", `{"status":{"bootcmdline": "hugepagesz=2M hugepages=10"}}`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the value of profile change in the worker node of custom nodepool hugepages-nodepool in hosted clusters, the expected value is still hugepagesz=2M hugepages=50")
		bootCMDLinestdOut, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", ntoNamespace, "profile/"+workerNodeName, "-ojsonpath='{.status.bootcmdline}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("status.bootcmdline is: %v", bootCMDLinestdOut)
		o.Expect(bootCMDLinestdOut).NotTo(o.ContainSubstring("hugepagesz=2M hugepages=50"))
		//The field of bootcmdline has been deprecated

		g.By("Check if custom node pool is ready in hosted cluster")
		exutil.AssertIfNodePoolIsReadyByName(oc, "hugepages-nodepool", 360, hostedClusterNS)

		g.By("Check if the custom tuned profile openshift-node-hugepages applied to all nodes of custom nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "openshift-node-hugepages")

		g.By("Assert hugepagesz match in /proc/cmdline on all nodes include the second new worker node in custom node pool")
		assertIfMatchKenelBootOnNodePoolLevelInHostedCluster(oc, ntoNamespace, "hugepages-nodepool", "hugepagesz=2M hugepages=50", true)
	})

	g.It("Longduration-NonPreRelease-HyperShiftMGMT-Author:liqcui-Medium-55359-NTO applies one configmap that is referenced in two nodepools in the same hosted cluster. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		firstNodePoolName = "hc-custom-nodepool"

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-vmdratio in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=20", "INCLUDE=openshift-node")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--ignore-not-found").Execute()
			isMatch := exutil.CheckAllNodepoolReadyByHostedClusterName(oc, firstNodePoolName, hostedClusterNS, 300)
			o.Expect(isMatch).To(o.Equal(true))
		}()

		//Create custom node pool yaml file
		g.By("Create custom node pool as the first nodepool in hosted cluster")
		exutil.CreateCustomNodePoolInHypershift(oc, "aws", guestClusterName, firstNodePoolName, "1", "m5.xlarge", hostedClusterNS)

		g.By("Check if custom node pool is ready in hosted cluster")
		exutil.AssertIfNodePoolIsReadyByName(oc, firstNodePoolName, 720, hostedClusterNS)

		//Apply tuned profile to hosted clusters
		g.By("Ge the default nodepool in hosted cluster as secondary nodepool")
		secondNodePoolName = getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(secondNodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in first custom node pool of hosted cluster")
		workerNodeNameInFirstNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, firstNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInFirstNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in first nodepool: %v", workerNodeNameInFirstNodepool)

		g.By("Pick one worker node in second node pool of hosted cluster")
		workerNodeNameInSecondtNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, secondNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInSecondtNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in second nodepool: %v", workerNodeNameInSecondtNodepool)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "openshift-node")
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInSecondtNodepool, "openshift-node")

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hc-nodepool-vmdratio", "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+secondNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()

		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Apply the tuned profile in first nodepool {firstNodePoolName}
		g.By("Apply the tuned profile in first nodepool {firstNodePoolName} in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap tuned-{firstNodePoolName} created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "tuned-"+firstNodePoolName)
		o.Expect(configMaps).NotTo(o.BeEmpty())
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + firstNodePoolName))

		g.By("Check if the tuned hc-nodepool-vmdratio-xxxxxx is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		g.By("Get the tuned pod name that running on first custom nodepool worker node")
		tunedPodNameInFirstNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInFirstNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInFirstNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInFirstNodePool)

		g.By("Check if the tuned profile applied to first custom nodepool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "hc-nodepool-vmdratio")

		g.By("Check if the tuned profile applied to all worker node in the first nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(hc-nodepool-vmdratio\) matches current configuration|static tuning from profile 'hc-nodepool-vmdratio' applied`)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in the first custom nodepool, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the second nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Apply the tuned profile in second nodepool
		g.By("Apply the tuned profile in second nodepool  in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get the tuned pod name that running on second nodepool worker node")
		tunedPodNameInSecondNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInSecondtNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInSecondNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInSecondNodePool)

		g.By("Check if the tuned profile applied to second nodepool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInSecondtNodepool, "hc-nodepool-vmdratio")

		g.By("Check if the tuned profile applied to all worker node in the first nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(hc-nodepool-vmdratio\) matches current configuration|static tuning from profile 'hc-nodepool-vmdratio' applied`)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in the second nodepool, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from the first nodepool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the first nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in the second nodepool, no impact with removing vm.dirty_ratio setting in first nodepool")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from the second nodepool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the first nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the second nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Clean up all left resrouce/settings
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+firstNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+secondNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)

		g.By("Check if the custom tuned profile removed from worker nodes of nodepool, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "openshift-node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "openshift-node")
	})

	g.It("Longduration-NonPreRelease-HyperShiftMGMT-Author:liqcui-Medium-53885-NTO applies different configmaps that reference to into two node pool in the same hosted clusters. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		firstNodePoolName = "hc-custom-nodepool"

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-vmdratio and hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=20", "INCLUDE=openshift-node")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=20", "INCLUDE=openshift-node")

		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-vmdratio"))
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-pidmax"))

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--ignore-not-found").Execute()
			isMatch := exutil.CheckAllNodepoolReadyByHostedClusterName(oc, firstNodePoolName, hostedClusterNS, 300)
			o.Expect(isMatch).To(o.Equal(true))
		}()

		//Create custom node pool yaml file
		g.By("Create custom node pool as the first nodepool in hosted cluster")
		exutil.CreateCustomNodePoolInHypershift(oc, "aws", guestClusterName, firstNodePoolName, "1", "m5.xlarge", hostedClusterNS)

		g.By("Check if custom node pool is ready in hosted cluster")
		exutil.AssertIfNodePoolIsReadyByName(oc, firstNodePoolName, 720, hostedClusterNS)

		//Apply tuned profile to hosted clusters
		g.By("Ge the default nodepool in hosted cluster as secondary nodepool")
		secondNodePoolName = getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(secondNodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in first custom node pool of hosted cluster")
		workerNodeNameInFirstNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, firstNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInFirstNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in first nodepool: %v", workerNodeNameInFirstNodepool)

		g.By("Pick one worker node in second node pool of hosted cluster")
		workerNodeNameInSecondtNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, secondNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInSecondtNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in second nodepool: %v", workerNodeNameInSecondtNodepool)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "openshift-node")
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInSecondtNodepool, "openshift-node")

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hc-nodepool-vmdratio", "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hc-nodepool-pidmax", "-n", guestClusterNS, "--ignore-not-found").Execute()

		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Apply the tuned profile in first nodepool {firstNodePoolName}
		g.By("Apply the tuned profile in first nodepool {firstNodePoolName} in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Apply the tuned profile in second nodepool {secondNodePoolName}
		g.By("Apply the tuned profile in second nodepool {secondNodePoolName} in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-pidmax\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap tuned-{firstNodePoolName} created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "tuned-"+firstNodePoolName)
		o.Expect(configMaps).NotTo(o.BeEmpty())
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + firstNodePoolName))

		g.By("Check if the configmap tuned-{secondNodePoolName} created in corresponding hosted ns in management cluster")
		configMaps = getTuningConfigMapNameWithRetry(oc, guestClusterNS, "tuned-"+secondNodePoolName)
		o.Expect(configMaps).NotTo(o.BeEmpty())
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + secondNodePoolName))

		g.By("Check if the tuned hc-nodepool-vmdratio-xxxxxx and hc-nodepool-pidmax-xxxxxx is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-vmdratio"))
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-pidmax"))

		g.By("Get the tuned pod name that running on first custom nodepool worker node")
		tunedPodNameInFirstNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInFirstNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInFirstNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInFirstNodePool)

		g.By("Get the tuned pod name that running on second nodepool worker node")
		tunedPodNameInSecondNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInSecondtNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInSecondNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInSecondNodePool)

		g.By("Check if the tuned profile applied to first custom nodepool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "hc-nodepool-vmdratio")

		g.By("Check if the tuned profile applied to second nodepool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInSecondtNodepool, "hc-nodepool-pidmax")

		g.By("Check if the tuned profile applied to all worker node in the first nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "hc-nodepool-vmdratio")

		g.By("Check if the tuned profile applied to all worker node in the second nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "hc-nodepool-pidmax")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(hc-nodepool-vmdratio\) matches current configuration`)

		g.By("Assert recommended profile (hc-nodepool-pidmax) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(hc-nodepool-pidmax\) matches current configuration`)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in the first custom nodepool, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl kernel.pid_max applied to worker nodes in the second custom nodepool, expected value is 868686")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "kernel.pid_max", "868686")

		//Compare the sysctl kernel.pid_max not equal to 868686 in first nodepool
		g.By("Check if the setting of sysctl  kernel.pid_max shouldn't applied to worker nodes in the first nodepool, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "kernel.pid_max", "868686")

		//Compare the sysctl vm.dirty_ratio not equal to 56 in second nodepool
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the second nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from the first nodepool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the tuned profile still applied to all worker node in the second nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "hc-nodepool-pidmax")

		//Compare the sysctl vm.dirty_ratio not equal to 56 in first nodepool
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the first nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl kernel.pid_max still applied to worker nodes in the second nodepool, no impact with removing vm.dirty_ratio setting in first nodepool")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "kernel.pid_max", "868686")

		g.By("Remove the custom tuned profile from the second nodepool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Compare the sysctl vm.dirty_ratio not equal to 56 in first nodepool
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the first nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Compare the sysctl kernel.pid_max not equal to 868686 in second nodepool
		g.By("Check if the setting of sysctl kernel.pid_max shouldn't applied to worker nodes in the second nodepool, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "kernel.pid_max", "868686")

		//Clean up all left resrouce/settings
		g.By("Remove configmap from management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+firstNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+secondNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)

		g.By("Check if the custom tuned profile removed from worker nodes of nodepool, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "openshift-node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "openshift-node")
	})

	g.It("HyperShiftMGMT-Author:liqcui-Medium-54546-NTO applies two Tuneds from two configmap referenced in one nodepool of a hosted cluster on hypershift.[Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-vmdratio and hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=20", "INCLUDE=openshift-node")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=20", "INCLUDE=openshift-node")

		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-vmdratio"))
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-pidmax"))

		//Apply tuned profile to hosted clusters
		g.By("Get the default nodepool name in hosted cluster")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in first custom node pool of hosted cluster")
		workerNodeNameInNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, nodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in first nodepool: %v", workerNodeNameInNodepool)

		//Delete configmap in hosted cluster namespace and disable tuningConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInNodepool, "openshift-node")

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hc-nodepool-vmdratio", "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-hc-nodepool-pidmax", "-n", guestClusterNS, "--ignore-not-found").Execute()

		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Apply the tuned profile in nodepool in hostedcluster
		g.By("Apply the tuned profile in default nodepool in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"},{\"name\": \"hc-nodepool-pidmax\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap tuned-{nodePoolName} created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "tuned-"+nodePoolName)
		o.Expect(configMaps).NotTo(o.BeEmpty())
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-vmdratio-xxxxxx and hc-nodepool-pidmax-xxxxxx is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-vmdratio"))
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-pidmax"))

		g.By("Get the tuned pod name that running on default nodepool worker node")
		tunedPodNameInNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInNodePool)

		g.By("Check if the tuned profile applied to nodepool worker nodes, the second profile hc-nodepool-pidmax take effective by default, the first one won't take effective")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInNodepool, "hc-nodepool-pidmax")

		g.By("Check if the tuned profile applied to all worker node in the second nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "hc-nodepool-pidmax")

		g.By("Assert recommended profile (hc-nodepool-pidmax) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInNodePool, "12", 300, `recommended profile \(hc-nodepool-pidmax\) matches current configuration`)

		g.By("Check if the setting of sysctl kernel.pid_max applied to worker nodes in the default nodepool, expected value is 868686")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "kernel.pid_max", "868686")

		//Compare the sysctl vm.dirty_ratio not equal to 56 in default nodepool
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Chnagge the hc-nodepool-vmdratio with a higher priority in management cluster, the lower number of priority with higher priority")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=18", "INCLUDE=openshift-node")

		g.By("Check if the tuned profile hc-nodepool-vmdratio applied to all worker node in the nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInNodePool, "12", 300, `recommended profile \(hc-nodepool-vmdratio\) matches current configuration`)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in the nodepool, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Compare the sysctl kernel.pid_max not equal to 868686 in first nodepool
		g.By("Check if the setting of sysctl  kernel.pid_max shouldn't applied to worker nodes in the nodepool, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "kernel.pid_max", "868686")

		g.By("Chnagge custom profile include setting with <openshift-node,hc-nodepool-vmdratio> and set priority to 16 in management cluster, both custom profile take effective")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=16", "INCLUDE=openshift-node,hc-nodepool-vmdratio")

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in the nodepool, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl kernel.pid_max applied to worker nodes in the default nodepool, expected value is 868686")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "kernel.pid_max", "868686")

		g.By("Chnagge the value of kernel.pid_max of custom profile hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=888888", "PRIORITY=16", "INCLUDE=openshift-node,hc-nodepool-vmdratio")

		g.By("Check if the setting of sysctl kernel.pid_max applied to worker nodes in the default nodepool, expected value is 888888")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "kernel.pid_max", "888888")

		g.By("Remove the custom tuned profile from the first nodepool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Compare the sysctl vm.dirty_ratio not equal to 56 in first nodepool
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Compare the sysctl kernel.pid_max not equal to 868686 in second nodepool
		g.By("Check if the setting of sysctl kernel.pid_max shouldn't applied to worker nodes in the nodepool, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "kernel.pid_max", "868686")

		//Clean up all left resrouce/settings
		g.By("Remove configmap from management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)

		g.By("Check if the custom tuned profile removed from worker nodes of nodepool, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "openshift-node")
	})

	g.It("NonPreRelease-Longduration-HyperShiftMGMT-Author:liqcui-Medium-53880-NTO apply one configmap that reference to two separated hosted clusters on hypershift. [Disruptive]", func() {

		//Second Hosted Cluster
		guestClusterName2, guestClusterKube2, hostedClusterNS2 = exutil.ValidHypershiftAndGetGuestKubeConf4SecondHostedCluster(oc)
		e2e.Logf("%s, %s, %s", guestClusterName2, guestClusterKube2, hostedClusterNS2)

		guestClusterNS2 = hostedClusterNS2 + "-" + guestClusterName2
		e2e.Logf("HostedClusterControlPlaneNS: %v", guestClusterNS2)
		// ensure NTO operator is installed
		isNTO2 = isHyperNTOPodInstalled(oc, guestClusterNS2)

		// test requires NTO to be installed
		if !isNTO || !isNTO2 {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-vmdratio in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=20", "INCLUDE=openshift-node")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		g.By("Ge the default nodepool in hosted cluster as first nodepool")
		firstNodePoolName = getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(firstNodePoolName).NotTo(o.BeEmpty())

		g.By("Ge the default nodepool in hosted cluster as second nodepool")
		secondNodePoolName = getNodePoolNamebyHostedClusterName(oc, guestClusterName2, hostedClusterNS2)
		o.Expect(secondNodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in default node pool of first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)
		workerNodeNameInFirstNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, firstNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInFirstNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in nodepool in first hosted cluster: %v", workerNodeNameInFirstNodepool)

		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "openshift-node")
		defer oc.SetGuestKubeconf(guestClusterKube)

		oc.SetGuestKubeconf(guestClusterKube2)
		g.By("Pick one worker node in default node pool of second hosted cluster")
		workerNodeNameInSecondNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, secondNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInSecondNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in nodepool in second hosted cluster: %v", workerNodeNameInSecondNodepool)

		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc.SetGuestKubeconf(guestClusterKube2), ntoNamespace, workerNodeNameInSecondNodepool, "openshift-node")
		defer oc.SetGuestKubeconf(guestClusterKube2)

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+firstNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+secondNodePoolName, "-n", guestClusterNS2, "--ignore-not-found").Execute()

		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS2, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Apply the tuned profile in first nodepool {firstNodePoolName}
		g.By("Apply the tuned profile in default nodepool {firstNodePoolName} in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetGuestKubeconf(guestClusterKube)
		g.By("Check if the configmap tuned-{firstNodePoolName} created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "tuned-"+firstNodePoolName)
		o.Expect(configMaps).NotTo(o.BeEmpty())
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + firstNodePoolName))

		g.By("Check if the tuned hc-nodepool-vmdratio-xxxxxx is created in hosted cluster nodepool")
		AssertIfTunedIsReadyByNameInHostedCluster(oc, "hc-nodepool-vmdratio", ntoNamespace)

		g.By("Get the tuned pod name that running on first custom nodepool worker node")
		tunedPodNameInFirstNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInFirstNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInFirstNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInFirstNodePool)

		g.By("Check if the tuned profile applied to first custom nodepool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "hc-nodepool-vmdratio")

		g.By("Check if the tuned profile applied to all worker node in the first nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(hc-nodepool-vmdratio\) matches current configuration|static tuning from profile 'hc-nodepool-vmdratio' applied`)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in first hosted cluster, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)

		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the second nodepool, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Apply the tuned profile in second nodepool
		g.By("Apply the tuned profile in second nodepool of second hosted cluster in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS2, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the tuned hc-nodepool-vmdratio-xxxxxx is created in hosted cluster nodepool")
		AssertIfTunedIsReadyByNameInHostedCluster(oc, "hc-nodepool-vmdratio", ntoNamespace)

		g.By("Get the tuned pod name that running on first custom nodepool worker node")
		tunedPodNameInSecondNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInSecondNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInSecondNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInSecondNodePool)

		g.By("Check if the tuned profile applied to second nodepool worker nodes in second hosted cluster")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInSecondNodepool, "hc-nodepool-vmdratio")

		g.By("Check if the tuned profile applied to all worker node in in second hosted cluster.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(hc-nodepool-vmdratio\) matches current configuration|static tuning from profile 'hc-nodepool-vmdratio' applied`)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes of default nodepool in second hosted cluster, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from the nodepool in first hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set first kubeconfig to access first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)
		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the first hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in the second hosted cluster, no impact with removing vm.dirty_ratio setting in first hosted cluster")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from the nodepool in second hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set first kubeconfig to access first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the first hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)
		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't applied to worker nodes in the second hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		//Clean up all left resrouce/settings
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+firstNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+secondNodePoolName, "-n", guestClusterNS2, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set first kubeconfig to access first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)
		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log in first hosted")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)
		g.By("Check if the custom tuned profile removed from worker nodes of nodepool in first hosted cluster, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "openshift-node")

		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)
		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log in second hosted")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)
		g.By("Check if the custom tuned profile removed from worker nodes of nodepool in second hosted cluster, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "openshift-node")
	})

	g.It("NonPreRelease-Longduration-HyperShiftMGMT-Author:liqcui-Medium-53883-NTO can apply different tunings to two separated hosted clusters on hypershift. [Disruptive]", func() {

		//Second Hosted Cluster
		guestClusterName2, guestClusterKube2, hostedClusterNS2 = exutil.ValidHypershiftAndGetGuestKubeConf4SecondHostedCluster(oc)
		e2e.Logf("%s, %s, %s", guestClusterName2, guestClusterKube2, hostedClusterNS2)

		guestClusterNS2 = hostedClusterNS2 + "-" + guestClusterName2
		e2e.Logf("HostedClusterControlPlaneNS: %v", guestClusterNS2)
		// ensure NTO operator is installed
		isNTO2 = isHyperNTOPodInstalled(oc, guestClusterNS2)

		// test requires NTO to be installed
		if !isNTO || !isNTO2 {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform != "aws" {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS2, "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-vmdratio in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=20", "INCLUDE=openshift-node")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		g.By("Create configmap hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, hostedClusterNS, "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=20", "INCLUDE=openshift-node")
		configmapsInMgmtClusters, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-pidmax"))

		g.By("Ge the default nodepool in hosted cluster as first nodepool")
		firstNodePoolName = getNodePoolNamebyHostedClusterName(oc, guestClusterName, hostedClusterNS)
		o.Expect(firstNodePoolName).NotTo(o.BeEmpty())

		g.By("Ge the default nodepool in hosted cluster as second nodepool")
		secondNodePoolName = getNodePoolNamebyHostedClusterName(oc, guestClusterName2, hostedClusterNS2)
		o.Expect(secondNodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in default node pool of first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)
		workerNodeNameInFirstNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, firstNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInFirstNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in nodepool in first hosted cluster: %v", workerNodeNameInFirstNodepool)

		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "openshift-node")
		defer oc.SetGuestKubeconf(guestClusterKube)

		oc.SetGuestKubeconf(guestClusterKube2)
		g.By("Pick one worker node in default node pool of second hosted cluster")
		workerNodeNameInSecondNodepool, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, secondNodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeNameInSecondNodepool).NotTo(o.BeEmpty())
		e2e.Logf("Worker node in nodepool in second hosted cluster: %v", workerNodeNameInSecondNodepool)

		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInSecondNodepool, "openshift-node")
		defer oc.SetGuestKubeconf(guestClusterKube2)

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+firstNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+secondNodePoolName, "-n", guestClusterNS2, "--ignore-not-found").Execute()

		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS2, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()

		//Apply the tuned profile in first nodepool {firstNodePoolName}
		g.By("Apply the tuned profile in default nodepool {firstNodePoolName} in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetGuestKubeconf(guestClusterKube)
		g.By("Check if the configmap tuned-{firstNodePoolName} created in corresponding hosted ns in management cluster")
		configMaps := getTuningConfigMapNameWithRetry(oc, guestClusterNS, "tuned-"+firstNodePoolName)
		o.Expect(configMaps).NotTo(o.BeEmpty())
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + firstNodePoolName))

		g.By("Check if the tuned hc-nodepool-vmdratio-xxxxxx is created in hosted cluster nodepool")
		AssertIfTunedIsReadyByNameInHostedCluster(oc, "hc-nodepool-vmdratio", ntoNamespace)

		g.By("Get the tuned pod name that running on first custom nodepool worker node")
		tunedPodNameInFirstNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInFirstNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInFirstNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInFirstNodePool)

		g.By("Check if the tuned profile applied to first custom nodepool worker nodes")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInFirstNodepool, "hc-nodepool-vmdratio")

		g.By("Check if the tuned profile applied to all worker node in the first nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert recommended profile (hc-nodepool-vmdratio) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(hc-nodepool-vmdratio\) matches current configuration|static tuning from profile 'hc-nodepool-vmdratio' applied`)

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to worker nodes in first hosted cluster, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl kernel.pid_max applies default settings on worker nodes in the first hosted cluster, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "kernel.pid_max", "868686")

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)

		g.By("Check if the setting of sysctl vm.dirty_ratio applies default settings on worker nodes in the second hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl kernel.pid_max applies default settings on worker nodes in the second hosted cluster, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "kernel.pid_max", "868686")

		//Apply the tuned profile in second nodepool
		g.By("Apply the tuned profile in second nodepool of second hosted cluster in management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS2, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[{\"name\": \"hc-nodepool-pidmax\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the tuned hc-nodepool-pidmax-xxxxxx is created in hosted cluster nodepool")
		AssertIfTunedIsReadyByNameInHostedCluster(oc, "hc-nodepool-pidmax", ntoNamespace)

		g.By("Get the tuned pod name that running on first custom nodepool worker node")
		tunedPodNameInSecondNodePool, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeNameInSecondNodepool)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodNameInSecondNodePool).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodNameInSecondNodePool)

		g.By("Check if the tuned profile applied to second nodepool worker nodes in second hosted cluster")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeNameInSecondNodepool, "hc-nodepool-pidmax")

		g.By("Check if the tuned profile applied to all worker node in in second hosted cluster.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "hc-nodepool-pidmax")

		g.By("Assert recommended profile (hc-nodepool-pidmax) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(hc-nodepool-pidmax\) matches current configuration|static tuning from profile 'hc-nodepool-pidmax' applied`)

		g.By("Check if the setting of sysctl kernel.pid_max applies on worker nodes in second hosted cluster, expected value is 868686")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "kernel.pid_max", "868686")

		g.By("Check if the setting of sysctl vm.dirty_ratio applies default settings on worker nodes in the second hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Set first kubeconfig to access first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)

		g.By("Check if the setting of sysctl kernel.pid_max applies default settings on worker nodes in the first hosted cluster, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "kernel.pid_max", "868686")

		g.By("Remove the custom tuned profile from the nodepool in first hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", firstNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Compare the sysctl vm.dirty_ratio not equal to 56
		g.By("Check if the setting of sysctl vm.dirty_ratio shouldn't apply to worker nodes in the first hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)

		g.By("Check if the setting of sysctl kernel.pid_max still apply to worker nodes in the second hosted cluster, no impact with removing vm.dirty_ratio setting in first hosted cluster")
		compareSpecifiedValueByNameOnNodePoolLevelWithRetryInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "kernel.pid_max", "868686")

		g.By("Remove the custom tuned profile from the nodepool in second hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", secondNodePoolName, "-n", hostedClusterNS, "--type", "merge", "-p", "{\"spec\":{\"tuningConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set first kubeconfig to access first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)

		g.By("All settings of vm.dirty_ratio and kernel.pid_max rollback to default settings ...")

		g.By("Check if the setting of sysctl vm.dirty_ratio applies default settings on worker nodes in the first hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl kernel.pid_max applies default settings on worker nodes in the first hosted cluster, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "sysctl", "kernel.pid_max", "868686")

		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)

		g.By("Check if the setting of sysctl vm.dirty_ratio applies default settings on worker nodes in the second hosted cluster, expected value is default value, not equal 56")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Check if the setting of sysctl  kernel.pid_max applies default settings on worker nodes in the second hosted cluster, expected value is default value, not equal 868686")
		assertMisMatchTunedSystemSettingsByParamNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "sysctl", "kernel.pid_max", "868686")
		//Clean up all left resrouce/settings
		g.By("Remove configmap from management cluster")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", hostedClusterNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", hostedClusterNS2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+firstNodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+secondNodePoolName, "-n", guestClusterNS2, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set first kubeconfig to access first hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube)
		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log in first hosted")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInFirstNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)
		g.By("Check if the custom tuned profile removed from worker nodes of nodepool in first hosted cluster, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, firstNodePoolName, "openshift-node")

		g.By("Set second kubeconfig to access second hosted cluster")
		oc.SetGuestKubeconf(guestClusterKube2)
		g.By("Assert recommended profile (openshift-node) matches current configuration in tuned pod log in second hosted")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodNameInSecondNodePool, "12", 300, `recommended profile \(openshift-node\) matches current configuration`)
		g.By("Check if the custom tuned profile removed from worker nodes of nodepool in second hosted cluster, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, secondNodePoolName, "openshift-node")
	})

})
