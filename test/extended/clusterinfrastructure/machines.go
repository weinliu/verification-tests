package clusterinfrastructure

import (
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
		oc = exutil.NewCLI("machine-api-operator", exutil.KubeConfigPath())
	)

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-45772-MachineSet selector is immutable", func() {
		g.By("Create a new machineset")
		exutil.SkipConditionally(oc)
		ms := exutil.MachineSetDescription{"machineset-45772", 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with empty clusterID")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, "machineset-45772", "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"selector":{"matchLabels":{"machine.openshift.io/cluster-api-cluster": null}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("selector is immutable"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-45377-Enable accelerated network via MachineSets on Azure [Disruptive]", func() {
		g.By("Create a new machineset with acceleratedNetworking: true")
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		machinesetName := "machineset-45377"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with acceleratedNetworking: true")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"acceleratedNetworking":true,"vmSize":"STANDARD_D4S_V3"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//test when set acceleratedNetworking: true, machine running needs nearly 9 minutes. so change the method timeout as 10 minutes.
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with acceleratedNetworking: true")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.acceleratedNetworking}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).To(o.ContainSubstring("true"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-46967-Implement Ephemeral OS Disks - OS cache placement on Azure [Disruptive]", func() {
		g.By("Create a new machineset with Ephemeral OS Disks - OS cache placement")
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		machinesetName := "machineset-46967"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with Ephemeral OS Disks - OS cache placement")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"vmSize":"Standard_D4s_v3","osDisk":{"diskSizeGB":30,"cachingType":"ReadOnly","diskSettings":{"ephemeralStorageLocation":"Local"},"managedDisk":{"storageAccountType":""}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with Ephemeral OS Disks - OS cache placement")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.osDisk.diskSettings.ephemeralStorageLocation}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).To(o.ContainSubstring("Local"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-46303-Availability sets could be created when needed for Azure [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		defaultWorkerMachinesetName := exutil.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, defaultWorkerMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "northcentralus" && region != "westus" {
			/*
				This case only supports on a region which doesn't have zones.
				These two regions cover most of the templates in flexy-templates and they don't have zones,
				so restricting the test is only applicable in these two regions.
			*/
			g.Skip("Skip this test scenario because the test is only applicable in \"northcentralus\" or \"westus\" region")
		}

		g.By("Create a new machineset")
		machinesetName := "machineset-46303"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with availabilitySet already created for the default worker machineset")
		/*
			If the availability set is not created for the default worker machineset,
			machine status will be failed and error message shows "Availability Set cannot be found".
			Therefore, if machine created successfully with the availability set,
			then it can prove that the availability set has been created when the default worker machineset is created.
		*/
		availabilitySetName := defaultWorkerMachinesetName + "-as"
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"availabilitySet":"`+availabilitySetName+`"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with availabilitySet")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.availabilitySet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("availability set name is: %s", out)
		o.Expect(out == availabilitySetName).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-47177-Medium-47201-[MDH] Machine Deletion Hooks appropriately block lifecycle phases [Disruptive]", func() {
		g.By("Create a new machineset with lifecycle hook")
		exutil.SkipConditionally(oc)
		machinesetName := "machineset-47177-47201"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with lifecycle hook")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"lifecycleHooks":{"preDrain":[{"name":"drain1","owner":"drain-controller1"}],"preTerminate":[{"name":"terminate2","owner":"terminate-controller2"}]}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Delete newly created machine by scaling " + machinesetName + " to 0")
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas=0", "-n", "openshift-machine-api", mapiMachineset, machinesetName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for machine to go into Deleting phase")
		err = wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.phase}").Output()
			if output != "Deleting" {
				e2e.Logf("machine is not in Deleting phase and waiting up to 2 seconds ...")
				return false, nil
			}
			e2e.Logf("machine is in Deleting phase")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Check machine phase failed")

		g.By("Check machine stuck in Deleting phase because of lifecycle hook")
		outDrain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.conditions[0]}").Output()
		e2e.Logf("outDrain:%s", outDrain)
		o.Expect(strings.Contains(outDrain, "\"message\":\"Drain operation currently blocked by: [{Name:drain1 Owner:drain-controller1}]\"") && strings.Contains(outDrain, "\"reason\":\"HookPresent\"") && strings.Contains(outDrain, "\"status\":\"False\"") && strings.Contains(outDrain, "\"type\":\"Drainable\"")).To(o.BeTrue())

		outTerminate, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.conditions[2]}").Output()
		e2e.Logf("outTerminate:%s", outTerminate)
		o.Expect(strings.Contains(outTerminate, "\"message\":\"Terminate operation currently blocked by: [{Name:terminate2 Owner:terminate-controller2}]\"") && strings.Contains(outTerminate, "\"reason\":\"HookPresent\"") && strings.Contains(outTerminate, "\"status\":\"False\"") && strings.Contains(outTerminate, "\"type\":\"Terminable\"")).To(o.BeTrue())

		g.By("Update machine without lifecycle hook")
		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachine, machineName, "-n", "openshift-machine-api", "-p", `[{"op": "remove", "path": "/spec/lifecycleHooks/preDrain"},{"op": "remove", "path": "/spec/lifecycleHooks/preTerminate"}]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-47230-[MDH] Negative lifecycle hook validation [Disruptive]", func() {
		g.By("Create a new machineset")
		exutil.SkipConditionally(oc)
		machinesetName := "machineset-47230"
		ms := exutil.MachineSetDescription{machinesetName, 1}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		checkItems := []struct {
			patchstr string
			errormsg string
		}{
			{
				patchstr: `{"spec":{"lifecycleHooks":{"preTerminate":[{"name":"","owner":"drain-controller1"}]}}}`,
				errormsg: "name in body should be at least 3 chars long",
			},
			{
				patchstr: `{"spec":{"lifecycleHooks":{"preDrain":[{"name":"drain1","owner":""}]}}}`,
				errormsg: "owner in body should be at least 3 chars long",
			},
			{
				patchstr: `{"spec":{"lifecycleHooks":{"preDrain":[{"name":"drain1","owner":"drain-controller1"},{"name":"drain1","owner":"drain-controller2"}]}}}`,
				errormsg: "Duplicate value: map[string]interface {}{\"name\":\"drain1\"}",
			},
		}

		for i, checkItem := range checkItems {
			g.By("Update machine with invalid lifecycle hook")
			out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachine, machineName, "-n", "openshift-machine-api", "-p", checkItem.patchstr, "--type=merge").Output()
			e2e.Logf("out"+strconv.Itoa(i)+":%s", out)
			o.Expect(strings.Contains(out, checkItem.errormsg)).To(o.BeTrue())
		}
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-44977-Machine with GPU is supported on gcp [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "gcp")
		g.By("Create a new machineset")
		machinesetName := "machineset-44977"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with GPU")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"a2-highgpu-1g","onHostMaintenance":"Terminate","restartPolicy":"Always"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with GPU")
		machineType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.machineType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		onHostMaintenance, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.onHostMaintenance}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartPolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.restartPolicy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("machineType:%s, onHostMaintenance:%s, restartPolicy:%s", machineType, onHostMaintenance, restartPolicy)
		o.Expect(strings.Contains(machineType, "a2-highgpu-1g") && strings.Contains(onHostMaintenance, "Terminate") && strings.Contains(restartPolicy, "Always")).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-48363-Machine providerID should be consistent with node providerID", func() {
		g.By("Check machine providerID and node providerID are consistent")
		exutil.SkipConditionally(oc)
		machineList := exutil.ListAllMachineNames(oc)
		for _, machineName := range machineList {
			nodeName := exutil.GetNodeNameFromMachine(oc, machineName)
			machineProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineName, "-o=jsonpath={.spec.providerID}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.providerID}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(machineProviderID).Should(o.Equal(nodeProviderID))
		}
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-High-35513-Windows machine should successfully provision for aws [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		g.By("Create a new machineset")
		machinesetName := "machineset-35513"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var amiID string
		switch region {
		case "us-east-1", "us-iso-east-1":
			amiID = "ami-0e09e139aca053387"
		case "us-east-2":
			amiID = "ami-0d939f6eeb364cbc6"
		case "cn-north-1":
			amiID = "ami-07a0c9b547ce24896"
		case "us-gov-west-1":
			amiID = "ami-040d6045c342dceaa"
		default:
			e2e.Logf("Not support region for the case for now.")
			g.Skip("Not support region for the case for now.")
		}
		g.By("Update machineset with windows ami")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"metadata":{"labels":{"machine.openshift.io/os-id": "Windows"}},"spec":{"providerSpec":{"value":{"ami":{"id":"`+amiID+`"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.WaitForMachineProvisioned(oc, machinesetName)
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-48012-Change AWS EBS GP3 IOPS in MachineSet should take affect on aws [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		g.By("Create a new machineset")
		machinesetName := "machineset-48012"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with gp3 iops 5000")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"blockDevices":[{"ebs":{"volumeType":"gp3","iops":5000}}]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check on aws instance with gp3 iops 5000")
		instanceID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-o=jsonpath={.items[0].status.providerStatus.instanceId}", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.GetAwsCredentialFromCluster(oc)

		volumeInfo, err := exutil.GetAwsVolumeInfoAttachedToInstanceID(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("volumeInfo:%s", volumeInfo)
		o.Expect(strings.Contains(volumeInfo, "\"Iops\":5000") && strings.Contains(volumeInfo, "\"VolumeType\":\"gp3\"")).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-High-33040-Required configuration should be added to the ProviderSpec to enable spot instances - Azure [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		randomMachinesetName := exutil.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovvirginia" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}

		g.By("Create a spot instance on azure")
		exutil.SkipConditionally(oc)
		ms := exutil.MachineSetDescription{"machineset-33040", 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, "machineset-33040", "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"spotVMOptions":{}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, "machineset-33040")

		g.By("Check machine and node were labelled as an `interruptible-instance`")
		machine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(machine).NotTo(o.BeEmpty())
		node, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-48594-AWS EFA network interfaces should be supported via machine api [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "us-iso-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		g.By("Create a new machineset")
		machinesetName := "machineset-48594"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with networkInterfaceType: EFA")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"networkInterfaceType":"EFA","instanceType":"m5dn.24xlarge"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with networkInterfaceType: EFA")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.networkInterfaceType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).Should(o.Equal("EFA"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-48595-Negative validation for AWS NetworkInterfaceType [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "us-iso-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		g.By("Create a new machineset")
		machinesetName := "machineset-48595"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with networkInterfaceType: invalid")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"networkInterfaceType":"invalid","instanceType":"m5dn.24xlarge"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value")).To(o.BeTrue())

		g.By("Update machineset with not supported instance types")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"networkInterfaceType":"EFA","instanceType":"m6i.xlarge"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachineFailed(oc, machinesetName)
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.errorMessage}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(strings.Contains(out, "not supported")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-49827-Ensure pd-balanced disk is supported on GCP via machine api [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "gcp")
		g.By("Create a new machineset")
		machinesetName := "machineset-49827"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with invalid disk type")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `[{"op":"replace","path":"/spec/template/spec/providerSpec/value/disks/0/type","value":"invalid"}]`, "--type=json").Output()
		o.Expect(strings.Contains(out, "Unsupported value")).To(o.BeTrue())

		g.By("Update machineset with pd-balanced disk type")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `[{"op":"replace","path":"/spec/replicas","value": 1},{"op":"replace","path":"/spec/template/spec/providerSpec/value/disks/0/type","value":"pd-balanced"}]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with pd-balanced disk type")
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.disks[0].type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).Should(o.Equal("pd-balanced"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Author:zhsun-Medium-50731-Enable IMDSv2 on existing worker machines via machine set [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		g.By("Create a new machineset")
		machinesetName := "machineset-50731"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with imds required")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"Required"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.metadataServiceOptions.authentication}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).Should(o.ContainSubstring("Required"))

		g.By("Update machineset with imds optional")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"Optional"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		machineName := exutil.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		oc.AsAdmin().WithoutNamespace().Run("delete").Args(mapiMachine, machineName, "-n", machineAPINamespace).Execute()
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[*].spec.providerSpec.value.metadataServiceOptions.authentication}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("Optional"))

		g.By("Update machine with invalid authentication ")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"invalid"}}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value: \"invalid\": Allowed values are either 'Optional' or 'Required'")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-37915-Creating machines using KMS keys from AWS [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" {
			g.Skip("Region is " + region + ", skip this test scenario because this case is only supported on us-east-2 region")
		}

		g.By("Create a new machineset")
		machinesetName := "machineset-37915"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with KMS keys")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"blockDevices": [{"ebs":{"encrypted":true,"iops":0,"kmsKey":{"arn":"arn:aws:kms:us-east-2:301721915996:key/c228ef83-df2c-4151-84c4-d9f39f39a972"},"volumeSize":120,"volumeType":"gp2"}}]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with KMS keys")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.blockDevices[0].ebs.kmsKey.arn}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("arn:aws:kms:us-east-2"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-Medium-52471-Enable configuration of boot diagnostics when creating VMs on Azure [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")

		g.By("Create a machineset configuring boot diagnostics with Azure managed storage accounts")
		machinesetName := "machineset-52471-1"
		ms1 := exutil.MachineSetDescription{machinesetName, 0}
		defer ms1.DeleteMachineSet(oc)
		ms1.CreateMachineSet(oc)

		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"AzureManaged"}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.diagnostics.boot.storageAccountType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("AzureManaged"))

		g.By("Create machineset configuring boot diagnostics with Customer managed storage accounts")
		machinesetName = "machineset-52471-2"
		ms2 := exutil.MachineSetDescription{machinesetName, 0}
		defer ms2.DeleteMachineSet(oc)
		ms2.CreateMachineSet(oc)

		resourceGroupName, rgerr := exutil.GetAzureCredentialFromCluster(oc)
		o.Expect(rgerr).NotTo(o.HaveOccurred())
		session, sessErr := exutil.NewAzureSessionFromEnv()
		o.Expect(sessErr).NotTo(o.HaveOccurred())
		storageAccount, instanceErr := exutil.GetAzureStorageAccount(session, resourceGroupName)
		o.Expect(instanceErr).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"CustomerManaged","customerManaged":{"storageAccountURI":"https://`+storageAccount+`.blob.core.windows.net/"}}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.diagnostics.boot.storageAccountType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("CustomerManaged"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-Medium-52473-Webhook validations for Azure boot diagnostics [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")

		g.By("Create a machineset")
		machinesetName := "machineset-52473-1"
		ms1 := exutil.MachineSetDescription{machinesetName, 0}
		defer ms1.DeleteMachineSet(oc)
		ms1.CreateMachineSet(oc)

		g.By("Update machineset with invalid storage account type")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"AzureManaged-invalid"}}}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("storageAccountType must be one of: AzureManaged, CustomerManaged"))

		g.By("Update machineset with Customer Managed boot diagnostics, with a missing storage account URI")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"CustomerManaged"}}}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("customerManaged configuration must be provided"))

		g.By("Update machineset Azure managed storage accounts")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"AzureManaged","customerManaged":{"storageAccountURI":"https://clusterqa2ob.blob.core.windows.net"}}}}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("customerManaged may not be set when type is AzureManaged"))

		g.By("Update machineset with invalid storageAccountURI")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"CustomerManaged","customerManaged":{"storageAccountURI":"https://clusterqa2ob.blob.core.windows.net.invalid"}}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachineFailed(oc, machinesetName)

		g.By("Update machineset with invalid storage account")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"CustomerManaged","customerManaged":{"storageAccountURI":"https://invalid.blob.core.windows.net"}}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachineFailed(oc, machinesetName)
	})

	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-DisconnectedOnly-Author:miyadav-Low-36489-Machineset creation when publicIP:true in disconnected(stratergy private or public) Azure enviroment [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		g.By("Create a new machineset")
		machinesetName := "machineset-36489"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		publicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
		if err != nil {
			g.Fail("Issue with dns setup")
		}
		g.By("Update machineset with publicIP: true")
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"publicIP": true}}}}}}`, "--type=merge").Output()
		if publicZone == "" {
			o.Expect(msg).To(o.ContainSubstring("publicIP is not allowed in Azure disconnected installation with publish strategy as internal"))
		} else {
			o.Expect(err).NotTo(o.HaveOccurred())
			//to scale up machineset with publicIP: true
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas": 1}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			exutil.WaitForMachinesRunning(oc, 1, machinesetName)
		}
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huliu-Medium-51013-machine api should issue client cert when AWS DNS suffix missing [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")

		g.By("Create a new dhcpOptions")
		var newDhcpOptionsID, currentDhcpOptionsID string
		exutil.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		newDhcpOptionsID, err := awsClient.CreateDhcpOptions()
		if err != nil {
			g.Skip("The credential is insufficient to perform create dhcpOptions operation, skip the cases!!")
		}
		defer awsClient.DeleteDhcpOptions(newDhcpOptionsID)

		g.By("Associate the VPC with the new dhcpOptionsId")
		machineName := exutil.ListWorkerMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		currentDhcpOptionsID, err = awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer awsClient.AssociateDhcpOptions(vpcID, currentDhcpOptionsID)
		err = awsClient.AssociateDhcpOptions(vpcID, newDhcpOptionsID)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new machineset")
		machinesetName := "machineset-51013"
		ms := exutil.MachineSetDescription{machinesetName, 1}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
	})
})
