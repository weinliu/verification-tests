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
		g.By("Check if controlplanemachineset is created")
		controlplanemachineset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(controlplanemachineset) == 0 {
			g.Skip("Skip for controlplanemachineset is not deployed!")
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
})
