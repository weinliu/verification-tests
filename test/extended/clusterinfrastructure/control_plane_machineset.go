package clusterinfrastructure

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	changeInstanceTypeCon             = "changeInstanceType"
	backupInstanceTypeCon             = "backupInstanceType"
	getInstanceTypeJSONCon            = "getInstanceTypeJSON"
	patchInstanceTypePrefixCon        = "patchInstanceTypePrefix"
	patchInstanceTypeSuffixCon        = "patchInstanceTypeSuffix"
	getMachineAvailabilityZoneJSONCon = "getMachineAvailabilityZoneJSON"
	getCPMSAvailabilityZonesJSONCon   = "getCPMSAvailabilityZonesJSON"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc                         = exutil.NewCLI("control-plane-machineset", exutil.KubeConfigPath())
		iaasPlatform               clusterinfra.PlatformType
		changeToBackupInstanceType = map[clusterinfra.PlatformType]map[architecture.Architecture]map[string]string{
			clusterinfra.AWS: {architecture.AMD64: {changeInstanceTypeCon: "m5.xlarge", backupInstanceTypeCon: "m6i.xlarge"},
				architecture.ARM64: {changeInstanceTypeCon: "m6gd.xlarge", backupInstanceTypeCon: "m6g.xlarge"}},
			clusterinfra.Azure: {architecture.AMD64: {changeInstanceTypeCon: "Standard_D4s_v3", backupInstanceTypeCon: "Standard_D8s_v3"},
				architecture.ARM64: {changeInstanceTypeCon: "Standard_D4ps_v5", backupInstanceTypeCon: "Standard_D8ps_v5"}},
			clusterinfra.GCP: {architecture.AMD64: {changeInstanceTypeCon: "e2-standard-4", backupInstanceTypeCon: "n2-standard-4"},
				architecture.ARM64: {changeInstanceTypeCon: "t2a-standard-8", backupInstanceTypeCon: "t2a-standard-4"}},
		}
		getInstanceTypeJsonByCloud = map[clusterinfra.PlatformType]map[string]string{
			clusterinfra.AWS: {getInstanceTypeJSONCon: "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.instanceType}",
				patchInstanceTypePrefixCon: `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"instanceType":"`,
				patchInstanceTypeSuffixCon: `"}}}}}}}`},
			clusterinfra.Azure: {getInstanceTypeJSONCon: "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.vmSize}",
				patchInstanceTypePrefixCon: `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"vmSize":"`,
				patchInstanceTypeSuffixCon: `"}}}}}}}`},
			clusterinfra.GCP: {getInstanceTypeJSONCon: "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.machineType}",
				patchInstanceTypePrefixCon: `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"machineType":"`,
				patchInstanceTypeSuffixCon: `"}}}}}}}`},
		}
		getAvailabilityZoneJSONByCloud = map[clusterinfra.PlatformType]map[string]string{
			clusterinfra.AWS: {getMachineAvailabilityZoneJSONCon: "-o=jsonpath={.spec.providerSpec.value.placement.availabilityZone}",
				getCPMSAvailabilityZonesJSONCon: "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[*].placement.availabilityZone}"},
			clusterinfra.Azure: {getMachineAvailabilityZoneJSONCon: "-o=jsonpath={.spec.providerSpec.value.zone}",
				getCPMSAvailabilityZonesJSONCon: "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.azure[*].zone}"},
			clusterinfra.GCP: {getMachineAvailabilityZoneJSONCon: "-o=jsonpath={.spec.providerSpec.value.zone}",
				getCPMSAvailabilityZonesJSONCon: "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.gcp[*].zone}"},
		}
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = clusterinfra.CheckPlatform(oc)
	})

	g.It("NonHyperShiftHOST-Author:zhsun-High-56086-[CPMS] Controlplanemachineset should be created by default", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.Azure, clusterinfra.Nutanix, clusterinfra.VSphere)

		g.By("CPMS should be created by default and state is Active")
		cpmsState, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=jsonpath={.spec.state}").Output()
		o.Expect(cpmsState).To(o.ContainSubstring("Active"))
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-53320-[CPMS] Owner reference could be added/removed to control plan machines [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.Nutanix, clusterinfra.VSphere)

		g.By("Check ownerReferences is added to master machines")
		masterMachineList := clusterinfra.ListMasterMachineNames(oc)
		for _, masterMachineName := range masterMachineList {
			ownerReferences, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, masterMachineName, "-o=jsonpath={.metadata.ownerReferences}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ownerReferences).ShouldNot(o.BeEmpty())
		}

		g.By("Delete controlplanemachineset")
		defer printNodeInfo(oc)
		defer activeControlPlaneMachineSet(oc)
		deleteControlPlaneMachineSet(oc)

		g.By("Check ownerReferences is removed from master machines")
		err := wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			cpmsState, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace, "-o=jsonpath={.spec.state}").Output()
			if cpmsState == "Inactive" {
				for _, masterMachineName := range masterMachineList {
					ownerReferences, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, masterMachineName, "-o=jsonpath={.metadata.ownerReferences}", "-n", machineAPINamespace).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(ownerReferences).Should(o.BeEmpty())
				}
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "controlplanemachineset is not re-created")
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-53081-[CPMS] Finalizer should be added to control plan machineset [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.Nutanix, clusterinfra.VSphere)
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
		o.Expect(err).NotTo(o.HaveOccurred())
		version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/control-plane-machine-set", "-o=jsonpath={.status.versions[0].version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.ContainSubstring("TrueFalseFalse"))
		o.Expect(version).To(o.ContainSubstring("4."))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-53323-[CPMS] Implement update logic for RollingUpdate CPMS strategy update instance type [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		controlPlaneArch := architecture.GetControlPlaneArch(oc)
		changeInstanceType := changeToBackupInstanceType[iaasPlatform][controlPlaneArch][changeInstanceTypeCon]
		backupInstanceType := changeToBackupInstanceType[iaasPlatform][controlPlaneArch][backupInstanceTypeCon]
		if iaasPlatform == clusterinfra.GCP && controlPlaneArch == architecture.AMD64 {
			confidentialCompute, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.confidentialCompute}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if confidentialCompute == "Enabled" {
				changeInstanceType = "c2d-standard-4"
				backupInstanceType = "n2d-standard-4"
			}
		}

		g.By("Get current instanceType")
		currentInstanceType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getInstanceTypeJsonByCloud[iaasPlatform][getInstanceTypeJSONCon], "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("currentInstanceType:%s", currentInstanceType)
		if currentInstanceType == changeInstanceType {
			changeInstanceType = backupInstanceType
		}

		labelsAfter := "machine.openshift.io/instance-type=" + changeInstanceType + ",machine.openshift.io/cluster-api-machine-type=master"
		labelsBefore := "machine.openshift.io/instance-type=" + currentInstanceType + ",machine.openshift.io/cluster-api-machine-type=master"
		patchstrChange := getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypePrefixCon] + changeInstanceType + getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypeSuffixCon]
		patchstrRecover := getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypePrefixCon] + currentInstanceType + getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypeSuffixCon]

		g.By("Change instanceType to trigger RollingUpdate")
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer waitForCPMSUpdateCompleted(oc, 1)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrRecover, "--type=merge", "-n", machineAPINamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrChange, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		/*
			The RollingUpdate will update all the master machines one by one,
			 here only check the first machine updated success, then consider the case passed to save time,
			 because all the machines update are the same, so I think it's ok to assumpt that.
		*/
		updatedMachineName := clusterinfra.WaitForMachinesRunningByLabel(oc, 1, labelsAfter)[0]
		e2e.Logf("updatedMachineName:%s", updatedMachineName)
		suffix := getMachineSuffix(oc, updatedMachineName)
		e2e.Logf("suffix:%s", suffix)
		clusterinfra.WaitForMachineDisappearBySuffix(oc, suffix, labelsBefore)
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-53323-[CPMS] Implement update logic for RollingUpdate CPMS strategy update some field [Disruptive]", func() {
		//For the providers which don't have instance type, we will update some other field to trigger update
		//For nutanix, we choose vcpusPerSocket
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Nutanix)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		var changeFieldValue, backupFieldValue, getFieldValueJSON string
		var patchstrPrefix, patchstrSuffix string
		changeFieldValue = "2"
		backupFieldValue = "1"
		getFieldValueJSON = "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.vcpusPerSocket}"
		patchstrPrefix = `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"vcpusPerSocket":`
		patchstrSuffix = `}}}}}}}`

		g.By("Get current field value")
		currentFieldValue, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getFieldValueJSON, "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("currentFieldValue:%s", currentFieldValue)
		if currentFieldValue == changeFieldValue {
			changeFieldValue = backupFieldValue
		}

		getMachineFieldValueJSON := "-o=jsonpath={.spec.providerSpec.value.vcpusPerSocket}"
		patchstrChange := patchstrPrefix + changeFieldValue + patchstrSuffix
		patchstrRecover := patchstrPrefix + currentFieldValue + patchstrSuffix

		g.By("Change field value to trigger RollingUpdate")
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer waitForCPMSUpdateCompleted(oc, 1)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrRecover, "--type=merge", "-n", machineAPINamespace).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrChange, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		labelMaster := "machine.openshift.io/cluster-api-machine-type=master"
		updatedMachineName := clusterinfra.WaitForMachineRunningByField(oc, getMachineFieldValueJSON, changeFieldValue, labelMaster)
		e2e.Logf("updatedMachineName:%s", updatedMachineName)
		suffix := getMachineSuffix(oc, updatedMachineName)
		e2e.Logf("suffix:%s", suffix)
		clusterinfra.WaitForMachineDisappearBySuffixAndField(oc, suffix, getMachineFieldValueJSON, currentFieldValue, labelMaster)
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-55631-[CPMS] Implement update logic for RollingUpdate CPMS strategy - Delete a master machine [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.Nutanix, clusterinfra.VSphere)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		g.By("Random pick a master machine")
		machineName := clusterinfra.ListMasterMachineNames(oc)[rand.Int31n(int32(len(clusterinfra.ListMasterMachineNames(oc))))]
		suffix := getMachineSuffix(oc, machineName)
		var getMachineAvailabilityZoneJSON string
		labels := "machine.openshift.io/cluster-api-machine-type=master"
		if iaasPlatform == clusterinfra.AWS || iaasPlatform == clusterinfra.Azure || iaasPlatform == clusterinfra.GCP {
			getMachineAvailabilityZoneJSON = getAvailabilityZoneJSONByCloud[iaasPlatform][getMachineAvailabilityZoneJSONCon]
			availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineName, "-n", "openshift-machine-api", getMachineAvailabilityZoneJSON).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if availabilityZone != "" {
				labels = "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"
			}
		}
		g.By("Delete the master machine to trigger RollingUpdate")
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(mapiMachine, machineName, "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineRunningBySuffix(oc, suffix, labels)
		clusterinfra.WaitForMachineDisappearByName(oc, machineName)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-Medium-54005-[CPMS] Control plane machine set OnDelete update strategies - update instance type [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		controlPlaneArch := architecture.GetControlPlaneArch(oc)
		changeInstanceType := changeToBackupInstanceType[iaasPlatform][controlPlaneArch][changeInstanceTypeCon]
		backupInstanceType := changeToBackupInstanceType[iaasPlatform][controlPlaneArch][backupInstanceTypeCon]
		if iaasPlatform == clusterinfra.GCP && controlPlaneArch == architecture.AMD64 {
			confidentialCompute, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.confidentialCompute}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if confidentialCompute == "Enabled" {
				changeInstanceType = "c2d-standard-4"
				backupInstanceType = "n2d-standard-4"
			}
		}

		g.By("Get current instanceType")
		currentInstanceType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getInstanceTypeJsonByCloud[iaasPlatform][getInstanceTypeJSONCon], "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("currentInstanceType:%s", currentInstanceType)
		if currentInstanceType == changeInstanceType {
			changeInstanceType = backupInstanceType
		}

		labelsAfter := "machine.openshift.io/instance-type=" + changeInstanceType + ",machine.openshift.io/cluster-api-machine-type=master"
		patchstrChange := getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypePrefixCon] + changeInstanceType + getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypeSuffixCon]
		patchstrRecover := getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypePrefixCon] + currentInstanceType + getInstanceTypeJsonByCloud[iaasPlatform][patchInstanceTypeSuffixCon]

		g.By("Update strategy to OnDelete, change instanceType to trigger OnDelete update")
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer waitForCPMSUpdateCompleted(oc, 1)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrRecover, "--type=merge", "-n", machineAPINamespace).Execute()
		defer waitForClusterStable(oc)

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrChange, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete one master manually")
		toDeletedMachineName := clusterinfra.ListMasterMachineNames(oc)[rand.Int31n(int32(len(clusterinfra.ListMasterMachineNames(oc))))]
		clusterinfra.DeleteMachine(oc, toDeletedMachineName)

		g.By("Check new master will be created and old master will be deleted")
		newCreatedMachineName := clusterinfra.WaitForMachinesRunningByLabel(oc, 1, labelsAfter)[0]
		e2e.Logf("newCreatedMachineName:%s", newCreatedMachineName)
		clusterinfra.WaitForMachineDisappearByName(oc, toDeletedMachineName)
		waitForClusterStable(oc)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-54005-[CPMS] Control plane machine set OnDelete update strategies - update some field [Disruptive]", func() {
		//For the providers which don't have instance type, we will update some other field to trigger update
		//For nutanix, we choose vcpusPerSocket
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Nutanix)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		var changeFieldValue, backupFieldValue, getFieldValueJSON string
		var patchstrPrefix, patchstrSuffix string
		changeFieldValue = "2"
		backupFieldValue = "1"
		getFieldValueJSON = "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.vcpusPerSocket}"
		patchstrPrefix = `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"vcpusPerSocket":`
		patchstrSuffix = `}}}}}}}`

		g.By("Get current field value")
		currentFieldValue, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getFieldValueJSON, "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("currentFieldValue:%s", currentFieldValue)
		if currentFieldValue == changeFieldValue {
			changeFieldValue = backupFieldValue
		}
		getMachineFieldValueJSON := "-o=jsonpath={.spec.providerSpec.value.vcpusPerSocket}"
		patchstrChange := patchstrPrefix + changeFieldValue + patchstrSuffix
		patchstrRecover := patchstrPrefix + currentFieldValue + patchstrSuffix

		g.By("Update strategy to OnDelete, change field value to trigger OnDelete update")
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer waitForCPMSUpdateCompleted(oc, 1)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrRecover, "--type=merge", "-n", machineAPINamespace).Execute()
		defer waitForClusterStable(oc)

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", patchstrChange, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete one master manually")
		toDeletedMachineName := clusterinfra.ListMasterMachineNames(oc)[rand.Int31n(int32(len(clusterinfra.ListMasterMachineNames(oc))))]
		clusterinfra.DeleteMachine(oc, toDeletedMachineName)

		g.By("Check new master will be created and old master will be deleted")
		labelMaster := "machine.openshift.io/cluster-api-machine-type=master"
		newCreatedMachineName := clusterinfra.WaitForMachineRunningByField(oc, getMachineFieldValueJSON, changeFieldValue, labelMaster)
		e2e.Logf("newCreatedMachineName:%s", newCreatedMachineName)
		clusterinfra.WaitForMachineDisappearByName(oc, toDeletedMachineName)
		waitForClusterStable(oc)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-Medium-55724-[CPMS] Control plane machine set OnDelete update strategies - Delete/Add a failureDomain [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		g.By("Check failureDomains")
		availabilityZones := getCPMSAvailabilityZones(oc, iaasPlatform)
		if len(availabilityZones) <= 1 {
			g.Skip("Skip for the failureDomains is no more than 1")
		}
		g.By("Update strategy to OnDelete")
		key, value, machineName := getZoneAndMachineFromCPMSZones(oc, availabilityZones)
		getMachineAvailabilityZoneJSON := getAvailabilityZoneJSONByCloud[iaasPlatform][getMachineAvailabilityZoneJSONCon]
		getCPMSAvailabilityZonesJSON := getAvailabilityZoneJSONByCloud[iaasPlatform][getCPMSAvailabilityZonesJSONCon]
		deleteFailureDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains."+iaasPlatform.String()+"["+strconv.Itoa(key)+"]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer func() {
			availabilityZonesStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getCPMSAvailabilityZonesJSON, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(availabilityZonesStr, value) {
				oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
				waitForCPMSUpdateCompleted(oc, 1)
			}
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		defer waitForClusterStable(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Pick the failureDomain which has only one master machine and delete the failureDomain")
		suffix := getMachineSuffix(oc, machineName)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"remove","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/`+strconv.Itoa(key)+`"}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete the master machine in the selected failureDomain")
		clusterinfra.DeleteMachine(oc, machineName)

		g.By("Check new master will be created in other zones and old master will be deleted")
		labelsBefore := "machine.openshift.io/zone=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		labelsAfter := "machine.openshift.io/zone!=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		newMachineNameRolledWithFailureDomain := clusterinfra.WaitForMachineRunningBySuffix(oc, suffix, labelsAfter)
		clusterinfra.WaitForMachineDisappearBySuffix(oc, suffix, labelsBefore)
		waitForClusterStable(oc)

		g.By("Check if it will rebalance the machines")
		availabilityZones = getCPMSAvailabilityZones(oc, iaasPlatform)
		if len(availabilityZones) >= 3 {
			e2e.Logf("availabilityZones>=3 means the three master machines are in different zones now, it will not rebalance when adding new zone")
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
			o.Expect(checkIfCPMSCoIsStable(oc)).To(o.BeTrue())
		} else {
			g.By("Add the failureDomain back to check OnDelete strategy rebalance the machines")
			availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, newMachineNameRolledWithFailureDomain, "-n", "openshift-machine-api", getMachineAvailabilityZoneJSON).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			labelsAfter = "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()

			g.By("Delete the new created master machine ")
			clusterinfra.DeleteMachine(oc, newMachineNameRolledWithFailureDomain)

			g.By("Check new master will be created in new added zone and old master will be deleted")
			newMachineNameRolledBalancedFailureDomain := clusterinfra.WaitForMachinesRunningByLabel(oc, 1, labelsBefore)[0]
			e2e.Logf("updatedMachineName:%s", newMachineNameRolledBalancedFailureDomain)
			suffix = getMachineSuffix(oc, newMachineNameRolledBalancedFailureDomain)
			clusterinfra.WaitForMachineDisappearBySuffix(oc, suffix, labelsAfter)
			waitForClusterStable(oc)
		}
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-Medium-55725-[CPMS] Control plane machine set OnDelete update strategies - Delete a master machine [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.Nutanix, clusterinfra.VSphere)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		g.By("Update strategy to OnDelete")
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		defer waitForClusterStable(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Random pick a master machine and delete manually to trigger OnDelete update")
		toDeletedMachineName := clusterinfra.ListMasterMachineNames(oc)[rand.Int31n(int32(len(clusterinfra.ListMasterMachineNames(oc))))]
		var getMachineAvailabilityZoneJSON string
		labels := "machine.openshift.io/cluster-api-machine-type=master"
		if iaasPlatform == clusterinfra.AWS || iaasPlatform == clusterinfra.Azure || iaasPlatform == clusterinfra.GCP {
			getMachineAvailabilityZoneJSON = getAvailabilityZoneJSONByCloud[iaasPlatform][getMachineAvailabilityZoneJSONCon]
			availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, toDeletedMachineName, "-n", "openshift-machine-api", getMachineAvailabilityZoneJSON).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if availabilityZone != "" {
				labels = "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"
			}
		}
		clusterinfra.DeleteMachine(oc, toDeletedMachineName)

		g.By("Check new master will be created and old master will be deleted")
		suffix := getMachineSuffix(oc, toDeletedMachineName)
		clusterinfra.WaitForMachineRunningBySuffix(oc, suffix, labels)
		clusterinfra.WaitForMachineDisappearByName(oc, toDeletedMachineName)
		waitForClusterStable(oc)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-53328-[CPMS] It doesn't rearrange the availability zones if the order of the zones isn't matching in the CPMS and the Control Plane [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP)
		skipForCPMSNotStable(oc)
		g.By("Check failureDomains")
		availabilityZones := getCPMSAvailabilityZones(oc, iaasPlatform)
		if len(availabilityZones) <= 1 {
			g.Skip("Skip for the failureDomains is no more than 1")
		}

		g.By("Update strategy to OnDelete so that it will not trigger update automaticly")
		defer printNodeInfo(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"OnDelete"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Change the failureDomain's order by deleting/adding failureDomain")
		changeFailureDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains."+iaasPlatform.String()+"[1]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"remove","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/1"}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/0","value":`+changeFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Update strategy to RollingUpdate check if will rearrange the availability zones and no update for masters")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"strategy":{"type":"RollingUpdate"}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newAvailabilityZones := getCPMSAvailabilityZones(oc, iaasPlatform)
		o.Expect(strings.Join(newAvailabilityZones, "")).To(o.ContainSubstring(availabilityZones[1] + availabilityZones[0] + strings.Join(availabilityZones[2:], "")))
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-54895-[CPMS] CPMS generator controller will create a new CPMS if a CPMS is removed from cluster [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.Nutanix, clusterinfra.VSphere)
		skipForCPMSNotStable(oc)
		g.By("Delete controlplanemachineset")
		defer printNodeInfo(oc)
		defer activeControlPlaneMachineSet(oc)
		deleteControlPlaneMachineSet(oc)

		g.By("Check a new controlplanemachineset will be created and state is Inactive ")
		err := wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
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
		var fieldName string
		var fieldValue = "invalid"
		switch iaasPlatform {
		case clusterinfra.AWS:
			fieldName = "instanceType"
		case clusterinfra.Azure:
			fieldName = "vmSize"
		case clusterinfra.GCP:
			fieldName = "machineType"
			confidentialCompute, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.confidentialCompute}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if confidentialCompute == "Enabled" {
				fieldValue = "c2d-standard-4"
			}
		case clusterinfra.Nutanix:
			fieldName = "bootType"
			fieldValue = "Legacy"
		case clusterinfra.VSphere:
			fieldName = "diskGiB"
			fieldValue = strconv.Itoa(140)
		default:
			e2e.Logf("The " + iaasPlatform.String() + " Platform is not supported for now.")
		}
		if iaasPlatform == clusterinfra.VSphere {
			// Construct JSON payload with the appropriate type handling for fieldValue
			jsonPayload := fmt.Sprintf(`{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"%s":%v}}}}}}}`, fieldName, fieldValue)
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", jsonPayload, "--type=merge", "-n", machineAPINamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"template":{"machines_v1beta1_machine_openshift_io":{"spec":{"providerSpec":{"value":{"`+fieldName+`":"`+fieldValue+`"}}}}}}}`, "--type=merge", "-n", machineAPINamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		activeControlPlaneMachineSet(oc)
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-52587-[CPMS] Webhook validations for CPMS resource [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.Nutanix, clusterinfra.VSphere)
		g.By("Update CPMS name")
		cpmsName, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"metadata":{"name":"invalid"}}`, "--type=merge", "-n", machineAPINamespace).Output()
		o.Expect(cpmsName).To(o.ContainSubstring("the name of the object (invalid) does not match the name on the URL (cluster)"))
		g.By("Update CPMS replicas")
		cpmsReplicas, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `{"spec":{"replicas": 4}}`, "--type=merge", "-n", machineAPINamespace).Output()
		o.Expect(cpmsReplicas).To(o.ContainSubstring("Unsupported value"))
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
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-55485-[CPMS] Implement update logic for RollingUpdate CPMS strategy - Delete/Add a failureDomain [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP)
		skipForCPMSNotStable(oc)
		skipForClusterNotStable(oc)
		g.By("Check failureDomains")
		availabilityZones := getCPMSAvailabilityZones(oc, iaasPlatform)
		if len(availabilityZones) <= 1 {
			g.Skip("Skip for the failureDomains is no more than 1")
		}

		g.By("Pick the failureDomain which has only one master machine")
		availabilityZones = getCPMSAvailabilityZones(oc, iaasPlatform)
		key, value, machineName := getZoneAndMachineFromCPMSZones(oc, availabilityZones)
		suffix := getMachineSuffix(oc, machineName)
		getMachineAvailabilityZoneJSON := getAvailabilityZoneJSONByCloud[iaasPlatform][getMachineAvailabilityZoneJSONCon]
		getCPMSAvailabilityZonesJSON := getAvailabilityZoneJSONByCloud[iaasPlatform][getCPMSAvailabilityZonesJSONCon]
		deleteFailureDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.failureDomains."+iaasPlatform.String()+"["+strconv.Itoa(key)+"]}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete the failureDomain to trigger RollingUpdate")
		labelsBefore := "machine.openshift.io/zone=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		labelsAfter := "machine.openshift.io/zone!=" + value + ",machine.openshift.io/cluster-api-machine-type=master"
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer func() {
			availabilityZonesStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", getCPMSAvailabilityZonesJSON, "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(availabilityZonesStr, value) {
				oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
				waitForCPMSUpdateCompleted(oc, 1)
			}
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"remove","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/`+strconv.Itoa(key)+`"}]`, "--type=json", "-n", machineAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newMachineNameRolledWithFailureDomain := clusterinfra.WaitForMachineRunningBySuffix(oc, suffix, labelsAfter)
		clusterinfra.WaitForMachineDisappearBySuffix(oc, suffix, labelsBefore)
		waitForClusterStable(oc)

		g.By("Check if it will rebalance the machines")
		availabilityZones = getCPMSAvailabilityZones(oc, iaasPlatform)
		if len(availabilityZones) >= 3 {
			e2e.Logf("availabilityZones>=3 means the three master machines are in different zones now, it will not rebalance when adding new zone")
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
			o.Expect(checkIfCPMSCoIsStable(oc)).To(o.BeTrue())
		} else {
			g.By("Add the failureDomain back to check RollingUpdate strategy rebalance the machines")
			availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, newMachineNameRolledWithFailureDomain, "-n", "openshift-machine-api", getMachineAvailabilityZoneJSON).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			labelsAfter = "machine.openshift.io/zone=" + availabilityZone + ",machine.openshift.io/cluster-api-machine-type=master"
			oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/failureDomains/`+iaasPlatform.String()+`/0","value":`+deleteFailureDomain+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
			newMachineNameRolledBalancedFailureDomain := clusterinfra.WaitForMachinesRunningByLabel(oc, 1, labelsBefore)[0]
			e2e.Logf("updatedMachineName:%s", newMachineNameRolledBalancedFailureDomain)
			suffix = getMachineSuffix(oc, newMachineNameRolledBalancedFailureDomain)
			clusterinfra.WaitForMachineDisappearBySuffix(oc, suffix, labelsAfter)
			waitForClusterStable(oc)
		}
		o.Expect(checkIfCPMSIsStable(oc)).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-70442-[CPMS] A warning should be shown when removing the target pools from cpms [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)

		publicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if publicZone == "" {
			g.Skip("Because on private clusters we don't use target pools so skip this case for private clusters!!")
		}

		g.By("Remove targetpool")
		targetPool, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-o=jsonpath={.spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.targetPools}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer printNodeInfo(oc)
		defer waitMasterNodeReady(oc)
		defer waitForClusterStable(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"add","path":"/spec/template/machines_v1beta1_machine_openshift_io/spec/providerSpec/value/targetPools","value":`+targetPool+`}]`, "--type=json", "-n", machineAPINamespace).Execute()
		out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("controlplanemachineset/cluster", "-p", `[{"op":"remove","path":"/spec/template/machines_v1beta1_machine_openshift_io/spec/providerSpec/value/targetPools"}]`, "--type=json", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("Warning: spec.template.machines_v1beta1_machine_openshift_io.spec.providerSpec.value.targetPools: TargetPools field is not set on ControlPlaneMachineSet"))
	})
})
