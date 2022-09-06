package clusterinfrastructure

import (
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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
		if !(iaasPlatform == "aws" || iaasPlatform == "azure") {
			g.Skip("Skip this scenario because it is not supported on the " + iaasPlatform + " platform")
		}
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
		if !(iaasPlatform == "aws" || iaasPlatform == "azure") {
			g.Skip("Skip this scenario because it is not supported on the " + iaasPlatform + " platform")
		}

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
})
