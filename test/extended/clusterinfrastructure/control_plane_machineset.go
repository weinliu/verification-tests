package clusterinfrastructure

import (
	"math/rand"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
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

	g.It("NonHyperShiftHOST-Author:zhsun-High-56086-[CPMS] Controlplanemachineset should be created by default", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")

		g.By("CPMS should be created by default and state is Active")
		cpmsState, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=jsonpath={.spec.state}").Output()
		o.Expect(cpmsState).To(o.ContainSubstring("Active"))
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-53320-[CPMS] Owner reference could be added/removed to control plan machines [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)

		g.By("Check ownerReferences is added to master machines")
		masterMachineList := exutil.ListMasterMachineNames(oc)
		for _, masterMachineName := range masterMachineList {
			ownerReferences, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, masterMachineName, "-o=jsonpath={.metadata.ownerReferences}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ownerReferences).ShouldNot(o.BeEmpty())
		}

		g.By("Delete controlplanemachineset")
		defer printNodeInfo(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"state":"Active"}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check ownerReferences is removed from master machines")

		err = wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			cpmsState, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=jsonpath={.spec.state}").Output()
			if cpmsState == "Inactive" {
				for _, masterMachineName := range masterMachineList {
					ownerReferences, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, masterMachineName, "-o=jsonpath={.metadata.ownerReferences}", "-n", machineAPINamespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(ownerReferences).Should(o.BeEmpty())
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "controlplanemachineset is not re-created")
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-53081-[CPMS] Finalizer should be added to control plan machineset [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "vsphere")
		skipForCPMSNotExist(oc)

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
	g.It("NonHyperShiftHOST-Author:zhsun-High-53610-[CPMS] Operator control-plane-machine-set should be in Available state and report version information", func() {
		state, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/control-plane-machine-set", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
		version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/control-plane-machine-set", "-o=jsonpath={.status.versions[0].version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrueFalseFalse"))
		o.Expect(version).To(o.ContainSubstring("4."))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-53323-[CPMS] Implement update logic for RollingUpdate CPMS strategy [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

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
		defer printNodeInfo(oc)
		defer waitForCPMSUpdateCompleted(oc, 1)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrRecover, "--type=merge", "-n", machineAPINamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrChange, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		/*
			The RollingUpdate will update all the master machines one by one,
			 here only check the first machine updated success, then consider the case passed to save time,
			 because all the machines update are the same, so I think it's ok to assumpt that.
		*/
		updatedMachineName := exutil.WaitForMachinesRunningByLabel(oc, 1, labelsAfter)[0]
		e2e.Logf("updatedMachineName:%s", updatedMachineName)
		suffix := getMachineSuffix(oc, updatedMachineName)
		e2e.Logf("suffix:%s", suffix)
		exutil.WaitForMachineDisappearBySuffix(oc, suffix, labelsBefore)
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-55631-[CPMS] Implement update logic for RollingUpdate CPMS strategy - Delete a master machine [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

		g.By("Random pick a master machine")
		machineName := exutil.ListMasterMachineNames(oc)[rand.Int31n(int32(len(exutil.ListMasterMachineNames(oc))))]
		start := strings.LastIndex(machineName, "-")
		suffix := machineName[start:]
		availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerSpec.value.placement.availabilityZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		labels := "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"

		g.By("Delete the master machine to trigger RollingUpdate")
		defer printNodeInfo(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(mapiMachine, machineName, "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachineRunningBySuffix(oc, suffix, labels)
		exutil.WaitForMachineDisappearByName(oc, machineName)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-Medium-54005-[CPMS] Control plane machine set OnDelete update strategies - update instance type [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

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
		patchstrChange := patchstrPrefix + changeInstanceType + patchstrSuffix
		patchstrRecover := patchstrPrefix + currentInstanceType + patchstrSuffix

		g.By("Update strategy to OnDelete, change instanceType to trigger OnDelete update")
		defer printNodeInfo(oc)
		defer waitForCPMSUpdateCompleted(oc, 1)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrRecover, "--type=merge", "-n", machineAPINamespace).Execute()

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrChange, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete one master manually")
		toDeletedMachineName := exutil.ListMasterMachineNames(oc)[rand.Int31n(int32(len(exutil.ListMasterMachineNames(oc))))]
		exutil.DeleteMachine(oc, toDeletedMachineName)

		g.By("Check new master will be created and old master will be deleted")
		newCreatedMachineName := exutil.WaitForMachinesRunningByLabel(oc, 1, labelsAfter)[0]
		e2e.Logf("newCreatedMachineName:%s", newCreatedMachineName)
		exutil.WaitForMachineDisappearByName(oc, toDeletedMachineName)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-Medium-55724-[CPMS] Control plane machine set OnDelete update strategies - Delete/Add a failureDomain [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

		g.By("Check failureDomains")
		availabilityZones := getCPMSAvailabilityZones(oc)
		if len(availabilityZones) <= 1 {
			g.Skip("Skip for the failureDomains is no more than 1")
		}
		g.By("Update strategy to OnDelete")
		key, value, machineName := getZoneAndMachineFromCPMSZones(oc, availabilityZones)
		getAvailabilityZonesJSON := "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[*].placement.availabilityZone}"
		deleteFailureDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws["+strconv.Itoa(key)+"]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer printNodeInfo(oc)
		defer func() {
			availabilityZonesStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getAvailabilityZonesJSON, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(availabilityZonesStr, value) {
				oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
				waitForCPMSUpdateCompleted(oc, 1)
			}
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Pick the failureDomain which has only one master machine and delete the failureDomain")
		suffix := getMachineSuffix(oc, machineName)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"remove","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/`+strconv.Itoa(key)+`"}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete the master machine in the selected failureDomain")
		exutil.DeleteMachine(oc, machineName)

		g.By("Check new master will be created in other zones and old master will be deleted")
		labelsBefore := "machine.openshift.io/zone=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		labelsAfter := "machine.openshift.io/zone!=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		newMachineNameRolledWithFailureDomain := exutil.WaitForMachineRunningBySuffix(oc, suffix, labelsAfter)
		exutil.WaitForMachineDisappearBySuffix(oc, suffix, labelsBefore)

		g.By("Add the failureDomain back to check OnDelete strategy rebalance the machines")
		availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, newMachineNameRolledWithFailureDomain, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerSpec.value.placement.availabilityZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		labelsAfter = "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()

		g.By("Delete the new created master machine ")
		exutil.DeleteMachine(oc, newMachineNameRolledWithFailureDomain)

		g.By("Check new master will be created in new added zone and old master will be deleted")
		newMachineNameRolledBalancedFailureDomain := exutil.WaitForMachinesRunningByLabel(oc, 1, labelsBefore)[0]
		e2e.Logf("updatedMachineName:%s", newMachineNameRolledBalancedFailureDomain)
		suffix = getMachineSuffix(oc, newMachineNameRolledBalancedFailureDomain)
		exutil.WaitForMachineDisappearBySuffix(oc, suffix, labelsAfter)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-Medium-55725-[CPMS] Control plane machine set OnDelete update strategies - Delete a master machine [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

		g.By("Update strategy to OnDelete")
		defer printNodeInfo(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Random pick a master machine and delete manually to trigger OnDelete update")
		toDeletedMachineName := exutil.ListMasterMachineNames(oc)[rand.Int31n(int32(len(exutil.ListMasterMachineNames(oc))))]
		availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, toDeletedMachineName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerSpec.value.placement.availabilityZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		labels := "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"
		exutil.DeleteMachine(oc, toDeletedMachineName)

		g.By("Check new master will be created and old master will be deleted")
		suffix := getMachineSuffix(oc, toDeletedMachineName)
		exutil.WaitForMachineRunningBySuffix(oc, suffix, labels)
		exutil.WaitForMachineDisappearByName(oc, toDeletedMachineName)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-53328-[CPMS] It doesn't rearrange the availability zones if the order of the zones isn't matching in the CPMS and the Control Plane [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

		g.By("Check failureDomains")
		availabilityZones := getCPMSAvailabilityZones(oc)
		if len(availabilityZones) <= 1 {
			g.Skip("Skip for the failureDomains is no more than 1")
		}

		g.By("Update strategy to OnDelete so that it will not trigger update automaticly")
		defer printNodeInfo(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Change the failureDomain's order by deleting/adding failureDomain")
		changeFailureDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[1]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"remove","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/1"}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/0","value":`+changeFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Update strategy to RollingUpdate check if will rearrange the availability zones and no update for masters")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newAvailabilityZones := getCPMSAvailabilityZones(oc)
		o.Expect(strings.Join(newAvailabilityZones, "")).To(o.ContainSubstring(availabilityZones[1] + availabilityZones[0] + strings.Join(availabilityZones[2:], "")))
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-54895-[CPMS] CPMS generator controller will create a new CPMS if a CPMS is removed from cluster [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

		g.By("Delete controlplanemachineset")
		defer printNodeInfo(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"state":"Active"}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check a new controlplanemachineset will be created and state is Inactive ")
		err = wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			cpmsState, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=jsonpath={.spec.state}").Output()
			if cpmsState != "Inactive" {
				e2e.Logf("controlplanemachineset is not in Inactive state and waiting up to 2 seconds ...")
				return false, nil
			}
			e2e.Logf("controlplanemachineset is in Inactive state")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "controlplanemachineset is not in Inactive state")

		g.By("Check controlplanemachineset do not reconcile master machines if state is Inactive")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"instanceType":"invalid"}}}}}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"state":"Active"}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-52587-[CPMS] Webhook validations for CPMS resource [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)

		g.By("Update CPMS name")
		cpmsName, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"metadata":{"name":"invalid"}}`, "--type=merge", "-n", machineAPINamespace).Output()
		o.Expect(cpmsName).To(o.ContainSubstring("the name of the object (invalid) does not match the name on the URL (cluster)"))
		g.By("Update CPMS replicas")
		cpmsReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"replicas": 4}}`, "--type=merge", "-n", machineAPINamespace).Output()
		o.Expect(cpmsReplicas).To(o.ContainSubstring("replicas is immutable"))
		g.By("Update CPMS selector")
		cpmsSelector, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"selector":{"matchLabels":{"machine.openshift.io/cluster-api-cluster": null}}}}`, "--type=merge", "-n", machineAPINamespace).Output()
		o.Expect(cpmsSelector).To(o.ContainSubstring("selector is immutable"))
		g.By("Update CPMS labels")
		cpmsLabel, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"metadata":{"labels":{"machine.openshift.io/cluster-api-cluster": null, "machine.openshift.io/cluster-api-machine-role": "invalid", "machine.openshift.io/cluster-api-machine-type": "invalid"}}}}}}`, "--type=merge", "-n", machineAPINamespace).Output()
		o.Expect(cpmsLabel).To(o.ContainSubstring("label 'machine.openshift.io/cluster-api-cluster' is required"))
		o.Expect(cpmsLabel).To(o.ContainSubstring("label 'machine.openshift.io/cluster-api-machine-role' is required, and must have value 'master'"))
		o.Expect(cpmsLabel).To(o.ContainSubstring("label 'machine.openshift.io/cluster-api-machine-type' is required, and must have value 'master'"))
		g.By("Update CPMS state")
		cpmsState, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"state":"Inactive"}}`, "--type=merge", "-n", machineAPINamespace).Output()
		o.Expect(cpmsState).To(o.ContainSubstring("state cannot be changed once Active"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-55485-[CPMS] Implement update logic for RollingUpdate CPMS strategy with non-standard indexes - Delete/Add a failureDomain [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure")
		skipForCPMSNotExist(oc)
		skipForCPMSNotStable(oc)

		g.By("Test delete/add a failureDoamin to trigger RollingUpdate right with non-standard indexes")
		g.By("Get failureDomains")
		availabilityZones := getCPMSAvailabilityZones(oc)
		if len(availabilityZones) <= 1 {
			g.Skip("Skip for the failureDomains is no more than 1")
		}

		g.By("Pick the failureDomain which has only one master machine")
		key, value, machineName := getZoneAndMachineFromCPMSZones(oc, availabilityZones)
		deleteFailureDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws["+strconv.Itoa(key)+"]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer activeControlPlaneMachineSet(oc)
		deleteControlPlaneMachineSet(oc)
		suffix, newMasterMachineName := randomMasterMachineName(machineName)
		e2e.Logf("newMasterMachineName:%s", newMasterMachineName)
		replaceOneMasterMachine(oc, machineName, newMasterMachineName)
		waitForClusterStable(oc)
		activeControlPlaneMachineSet(oc)

		g.By("Delete the failureDomain to trigger RollingUpdate")
		getAvailabilityZonesJSON := "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[*].placement.availabilityZone}"
		labelsBefore := "machine.openshift.io/zone=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		labelsAfter := "machine.openshift.io/zone!=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		defer printNodeInfo(oc)
		defer func() {
			availabilityZonesStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getAvailabilityZonesJSON, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(availabilityZonesStr, value) {
				oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
				waitForCPMSUpdateCompleted(oc, 1)
			}
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"remove","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/`+strconv.Itoa(key)+`"}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newMachineNameRolledWithFailureDomain := exutil.WaitForMachineRunningBySuffix(oc, suffix, labelsAfter)
		exutil.WaitForMachineDisappearBySuffix(oc, suffix, labelsBefore)

		g.By("Add the failureDomain back to check RollingUpdate strategy rebalance the machines")
		availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, newMachineNameRolledWithFailureDomain, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerSpec.value.placement.availabilityZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		labelsAfter = "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/aws/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
		newMachineNameRolledBalancedFailureDomain := exutil.WaitForMachinesRunningByLabel(oc, 1, labelsBefore)[0]
		e2e.Logf("updatedMachineName:%s", newMachineNameRolledBalancedFailureDomain)
		suffix = getMachineSuffix(oc, newMachineNameRolledBalancedFailureDomain)
		exutil.WaitForMachineDisappearBySuffix(oc, suffix, labelsAfter)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})
})
