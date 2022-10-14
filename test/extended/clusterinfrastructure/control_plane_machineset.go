package clusterinfrastructure

import (
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("control-plane-machineset", exutil.KubeConfigPath())
		iaasPlatform string
	)
	g.BeforeEach(func() {
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-53320-Owner reference could be added/removed to control plan machines [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		g.By("Check if controlplanemachineset exists")
		controlplanemachineset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(controlplanemachineset) == 0 {
			g.Skip("Skip for controlplanemachineset doesn't exist!")
		}

		g.By("Check ownerReferences is added to master machines")
		masterMachineList := exutil.ListMasterMachineNames(oc)
		for _, masterMachineName := range masterMachineList {
			ownerReferences, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, masterMachineName, "-o=jsonpath={.metadata.ownerReferences}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ownerReferences).ShouldNot(o.BeEmpty())
		}

		g.By("Delete controlplanemachineset")
		controlplanemachinesetJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=json").OutputToFile("controlplanemachineset.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", controlplanemachinesetJSON, "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check ownerReferences is removed from master machines")
		for _, masterMachineName := range masterMachineList {
			ownerReferences, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, masterMachineName, "-o=jsonpath={.metadata.ownerReferences}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ownerReferences).Should(o.BeEmpty())
		}
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-53081-Finalizer should be added to control plan machineset [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "vsphere")

		g.By("Check if controlplanemachineset exists")
		controlplanemachineset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(controlplanemachineset) == 0 {
			g.Skip("Skip for controlplanemachineset doesn't exist!")
		}

		g.By("Check finalizer is added to controlplanemachineset")
		finalizers, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.metadata.finalizers[0]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(finalizers).To(o.ContainSubstring("controlplanemachineset.machine.openshift.io"))

		g.By("Remove finalizer")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"metadata":{"finalizers":null}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Finalizer should be re-added to controlplanemachineset")
		finalizers, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.metadata.finalizers[0]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(finalizers).To(o.ContainSubstring("controlplanemachineset.machine.openshift.io"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-High-53610-Operator control-plane-machine-set should be in Available state and report version information", func() {
		state, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/control-plane-machine-set", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/control-plane-machine-set", "-o=jsonpath={.status.versions[0].version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrueFalseFalse"))
		o.Expect(version).To(o.ContainSubstring("4."))
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-Medium-53323-[CPMS] Implement update logic for RollingUpdate CPMS strategy [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")

		g.By("Check if controlplanemachineset exists")
		controlplanemachineset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(controlplanemachineset) == 0 {
			g.Skip("Skip for controlplanemachineset doesn't exist!")
		}

		g.By("Check if RollingUpdate is onging")
		readyReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.readyReplicas}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		currentReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.status.replicas}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		desiredReplicas, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.replicas}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !(desiredReplicas == currentReplicas && desiredReplicas == readyReplicas) {
			g.Skip("Skip for the previous RollingUpdate is onging!")
		}

		var changeInstanceType, backupInstanceType, getInstanceTypeJSON string
		var patchstrPrefix, patchstrSuffix string
		switch iaasPlatform {
		case "aws":
			changeInstanceType = "m5.xlarge"
			backupInstanceType = "m6i.xlarge"
			getInstanceTypeJSON = "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.instanceType}"
			patchstrPrefix = `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"instanceType":"`
			patchstrSuffix = `"}}}}}}}`
		case "azure":
			changeInstanceType = "Standard_D4s_v3"
			backupInstanceType = "Standard_D8s_v3"
			getInstanceTypeJSON = "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.vmSize}"
			patchstrPrefix = `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"vmSize":"`
			patchstrSuffix = `"}}}}}}}`
		default:
			e2e.Logf("The " + iaasPlatform + " Platform is not supported for now.")
		}

		g.By("Get current instanceType")
		currentInstanceType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getInstanceTypeJSON, "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("currentInstanceType:%s", currentInstanceType)
		if currentInstanceType == changeInstanceType {
			changeInstanceType = backupInstanceType
		}

		labelsAfter := "machine.openshift.io/instance-type=" + changeInstanceType + ",machine.openshift.io/cluster-api-machine-type=master"
		labelsBefore := "machine.openshift.io/instance-type=" + currentInstanceType + ",machine.openshift.io/cluster-api-machine-type=master"
		patchstrChange := patchstrPrefix + changeInstanceType + patchstrSuffix
		patchstrRecover := patchstrPrefix + currentInstanceType + patchstrSuffix

		g.By("Change instanceType to trigger RollingUpdate")
		defer e2e.Logf(oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output())
		defer WaitForRollingUpdateCompleted(oc, 1)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrRecover, "--type=merge", "-n", machineAPINamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrChange, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		/*
			The RollingUpdate will update all the master machines one by one,
			 here only check the first machine updated success, then consider the case passed to save time,
			 because all the machines update are the same, so I think it's ok to assumpt that.
		*/
		updatedMachineName := exutil.WaitForSpecificMachinesRunning(oc, 1, labelsAfter)[0]
		e2e.Logf("updatedMachineName:%s", updatedMachineName)
		suffix := updatedMachineName[len(updatedMachineName)-2:]
		e2e.Logf("suffix:%s", suffix)
		exutil.WaitForMachineDisappear(oc, suffix, labelsBefore)
	})
})
