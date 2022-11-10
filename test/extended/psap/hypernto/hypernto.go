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
		oc                            = exutil.NewCLI("hypernto-test", exutil.KubeConfigPath())
		ntoNamespace                  = "openshift-cluster-node-tuning-operator"
		tunedWithSameProfileName      string
		tunedWithDiffProfileName      string
		tunedWithInvalidProfileName   string
		tunedWithNodeLevelProfileName string
		isNTO                         bool
		guestClusterName              string
		guestClusterNS                string
		guestClusterKube              string
	)

	g.BeforeEach(func() {
		guestClusterName, guestClusterKube = exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		e2e.Logf("%s, %s", guestClusterName, guestClusterKube)
		oc.SetGuestKubeconf(guestClusterKube)
		tunedWithSameProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "tuned-with-sameprofilename.yaml")
		tunedWithDiffProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "tuned-with-diffprofilename.yaml")
		tunedWithInvalidProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "nto-basic-tuning-sysctl-invalid.yaml")
		tunedWithNodeLevelProfileName = exutil.FixturePath("testdata", "psap", "hypernto", "nto-basic-tuning-sysctl-nodelevel.yaml")
		// ensure NTO operator is installed
		guestClusterNS = "clusters-" + guestClusterName
		isNTO = isHyperNTOPodInstalled(oc, guestClusterNS)
	})

	g.It("HyperShiftMGMT-Author:liqcui-Medium-53875-NTO Support profile that have the same name with tuned on hypershift [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", "clusters", "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, "clusters", "--ignore-unknown-parameters=true", "-f", tunedWithSameProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=20")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "clusters").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-pidmax"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-pidmax in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in hosted cluster, this worker node will be labeled with hc-nodepool-pidmax=")
		workerNodeName, err := exutil.GetFirstLinuxWorkerNodeInHostedCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tunedConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[{\"name\": \"hc-nodepool-pidmax\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-pidmax created in hosted cluster nodepool")
		configMaps := getTunedConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-pidmax is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-pidmax"))

		g.By("Check if the tuned rendered contain hc-nodepool-pidmax")
		renderCheck, err := getTunedRenderInHostedCluster(oc, ntoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(renderCheck).NotTo(o.BeEmpty())
		o.Expect(renderCheck).To(o.ContainSubstring("hc-nodepool-pidmax"))

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

		g.By("Assert active and recommended profile (hc-nodepool-pidmax) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "2", 300, `active and recommended profile \(hc-nodepool-pidmax\) match`)

		g.By("Check if the setting of sysctl kernel.pid_max applied to labeled worker nodes, expected value is 868686")
		compareSpecifiedValueByNameOnLabelNodewithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max", "868686")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax", "-n", "clusters").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert active and recommended profile (openshift-node) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "3", 300, `active and recommended profile \(openshift-node\) match`)

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

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-invalid", "-n", "clusters", "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-invalid in management cluster")
		exutil.ApplyOperatorResourceByYaml(oc, "", tunedWithInvalidProfileName)

		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "clusters").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-invalid"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-invalid in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in hosted cluster, this worker node will be labeled with hc-nodepool-invalid=")
		workerNodeName, err := exutil.GetFirstLinuxWorkerNodeInHostedCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tunedConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[{\"name\": \"hc-nodepool-invalid\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-invalid created in hosted cluster nodepool")
		configMaps := getTunedConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-invalid is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-invalid"))

		g.By("Check if the tuned rendered contain hc-nodepool-invalid")
		renderCheck, err := getTunedRenderInHostedCluster(oc, ntoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(renderCheck).NotTo(o.BeEmpty())
		o.Expect(renderCheck).To(o.ContainSubstring("hc-nodepool-invalid"))

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

		g.By("Assert active and recommended profile (hc-nodepool-invalid) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "2", 300, `active and recommended profile \(hc-nodepool-invalid\) match|static tuning from profile 'hc-nodepool-invalid' applied`)

		g.By("Assert active and recommended profile (hc-nodepool-invalid) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "20", 300, `ERROR    tuned.plugins.plugin_sysctl: Failed to read sysctl parameter`)

		expectedDegradedStatus, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", ntoNamespace, "profile", workerNodeName, `-ojsonpath='{.status.conditions[?(@.type=="Degraded")].status}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(expectedDegradedStatus).NotTo(o.BeEmpty())
		o.Expect(expectedDegradedStatus).To(o.ContainSubstring("True"))

		g.By("Check if the setting of sysctl kernel.pid_max applied to labeled worker nodes, expected value is 868686")
		compareSpecifiedValueByNameOnLabelNodewithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max", "868686")

		g.By("Check if the setting of sysctl vm.dirty_ratio applied to labeled worker nodes, expected value is 56")
		compareSpecifiedValueByNameOnLabelNodewithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[{\"name\": \"\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max and vm.dirty_ratio rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-invalid", "-n", "clusters").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert active and recommended profile (openshift-node) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "3", 300, `active and recommended profile \(openshift-node\) match`)

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
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", "clusters", "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-vmdratio in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, "clusters", "--ignore-unknown-parameters=true", "-f", tunedWithNodeLevelProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-vmdratio", "SYSCTLPARM=vm.dirty_ratio", "SYSCTLVALUE=56", "PRIORITY=20")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "clusters").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-vmdratio in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		workerNodeName, err := exutil.GetFirstWorkerNodeByNodePoolNameInHostedCluster(oc, nodePoolName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())

		//Delete configmap in hosted cluster namespace and disable tunedConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[{\"name\": \"hc-nodepool-vmdratio\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-vmdratio created in hosted cluster nodepool")
		configMaps := getTunedConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-vmdratio is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		g.By("Check if the tuned rendered contain hc-nodepool-vmdratio")
		renderCheck, err := getTunedRenderInHostedCluster(oc, ntoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(renderCheck).NotTo(o.BeEmpty())
		o.Expect(renderCheck).To(o.ContainSubstring("hc-nodepool-vmdratio"))

		g.By("Get the tuned pod name that running on first node of nodepool")
		tunedPodName, err := exutil.GetPodNameInHostedCluster(oc, ntoNamespace, "", workerNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		g.By("Check if the tuned profile applied to all worker node in specifed nodepool.")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "hc-nodepool-vmdratio")

		g.By("Assert active and recommended profile (hc-nodepool-vmdratio) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "2", 300, `active and recommended profile \(hc-nodepool-vmdratio\) match|static tuning from profile 'hc-nodepool-vmdratio' applied`)
		g.By("Check if the setting of sysctl vm.dirty_ratio applied to labeled worker nodes, expected value is 56")
		compareSpecifiedValueByNameOnNodePoolLevelwithRetryInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max and vm.dirty_ratio rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-vmdratio", "-n", "clusters").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert active and recommended profile (openshift-node) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "3", 300, `active and recommended profile \(openshift-node\) match`)

		g.By("Check if the custom tuned profile removed from worker nodes of nodepool, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "openshift-node")

		g.By("The value of vm.dirty_ratio on specified nodepool should not equal to 56")
		assertMisMatchTunedSystemSettingsByParmNameOnNodePoolLevelInHostedCluster(oc, ntoNamespace, nodePoolName, "sysctl", "vm.dirty_ratio", "56")
	})

	g.It("HyperShiftMGMT-Author:liqcui-Medium-53886-NTO support tuning sysctl with different name that applied to one labeled node of nodepool in hypershift. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//Delete configmap in clusters namespace
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax-cm", "-n", "clusters", "--ignore-not-found").Execute()

		//Create configmap, it will create custom tuned profile based on this configmap
		g.By("Create configmap hc-nodepool-pidmax in management cluster")
		exutil.ApplyNsResourceFromTemplate(oc, "clusters", "--ignore-unknown-parameters=true", "-f", tunedWithDiffProfileName, "-p", "TUNEDPROFILENAME=hc-nodepool-pidmax", "SYSCTLPARM=kernel.pid_max", "SYSCTLVALUE=868686", "PRIORITY=20")
		configmapsInMgmtClusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "clusters").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configmapsInMgmtClusters).NotTo(o.BeEmpty())
		o.Expect(configmapsInMgmtClusters).To(o.ContainSubstring("hc-nodepool-pidmax-cm"))

		//Apply tuned profile to hosted clusters
		g.By("Apply tunedCconfig hc-nodepool-pidmax in hosted cluster nodepool")
		nodePoolName := getNodePoolNamebyHostedClusterName(oc, guestClusterName)
		o.Expect(nodePoolName).NotTo(o.BeEmpty())

		g.By("Pick one worker node in hosted cluster, this worker node will be labeled with hc-nodepool-pidmax=")
		workerNodeName, err := exutil.GetFirstLinuxWorkerNodeInHostedCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		e2e.Logf("Worker Node: %v", workerNodeName)

		//Delete configmap in hosted cluster namespace and disable tunedConfig
		defer assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[]}}").Execute()

		//Enable tuned in hosted clusters
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[{\"name\": \"hc-nodepool-pidmax-cm\"}]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if the configmap hc-nodepool-pidmax created in hosted cluster nodepool")
		configMaps := getTunedConfigMapNameWithRetry(oc, guestClusterNS, nodePoolName)
		o.Expect(configMaps).To(o.ContainSubstring("tuned-" + nodePoolName))

		g.By("Check if the tuned hc-nodepool-pidmax is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.ContainSubstring("hc-nodepool-pidmax-tuned"))

		g.By("Check if the tuned rendered contain hc-nodepool-pidmax")
		renderCheck, err := getTunedRenderInHostedCluster(oc, ntoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(renderCheck).NotTo(o.BeEmpty())
		o.Expect(renderCheck).To(o.ContainSubstring("hc-nodepool-pidmax-profile"))

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

		g.By("Assert active and recommended profile (hc-nodepool-pidmax-profile) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "2", 300, `active and recommended profile \(hc-nodepool-pidmax-profile\) match`)

		g.By("Check if the setting of sysctl kernel.pid_max applied to labeled worker nodes, expected value is 868686")
		compareSpecifiedValueByNameOnLabelNodewithRetryInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max", "868686")

		g.By("Remove the custom tuned profile from node pool in hosted cluster ...")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("nodepool", nodePoolName, "-n", "clusters", "--type", "merge", "-p", "{\"spec\":{\"tunedConfig\":[]}}").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Remove custom tuned profile to check if kernel.pid_max rollback to origin value
		g.By("Remove configmap from management cluster")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "hc-nodepool-pidmax-cm", "-n", "clusters").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "tuned-"+nodePoolName, "-n", guestClusterNS, "--ignore-not-found").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Assert active and recommended profile (openshift-node) match in tuned pod log")
		assertNTOPodLogsLastLinesInHostedCluster(oc, ntoNamespace, tunedPodName, "3", 300, `active and recommended profile \(openshift-node\) match`)

		g.By("Check if the custom tuned profile removed from labeled worker nodes, default openshift-node applied to worker node")
		assertIfTunedProfileAppliedOnSpecifiedNodeInHostedCluster(oc, ntoNamespace, workerNodeName, "openshift-node")

		pidMaxValue := getTunedSystemSetValueByParamNameInHostedCluster(oc, ntoNamespace, workerNodeName, "sysctl", "kernel.pid_max")
		o.Expect(pidMaxValue).NotTo(o.BeEmpty())
		o.Expect(pidMaxValue).NotTo(o.ContainSubstring("868686"))
	})
})
