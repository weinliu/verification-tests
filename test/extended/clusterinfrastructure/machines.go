package clusterinfrastructure

import (
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfrastructure"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("machine-api-operator", exutil.KubeConfigPath())
	)
	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-45772-MachineSet selector is immutable", func() {
		g.By("Create a new machineset")
		clusterinfra.SkipConditionally(oc)
		ms := clusterinfra.MachineSetDescription{"machineset-45772", 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with empty clusterID")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machineset/machineset-45772", "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"selector":{"matchLabels":{"machine.openshift.io/cluster-api-cluster": null}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("selector is immutable"))
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-Medium-45377-Enable accelerated network via MachineSets on Azure [Serial]", func() {
		g.By("Create a new machineset with acceleratedNetworking: true")
		if clusterinfra.CheckPlatform(oc) == "azure" {
			machinesetName := "machineset-45377"
			ms := clusterinfra.MachineSetDescription{machinesetName, 0}
			defer ms.DeleteMachineSet(oc)
			ms.CreateMachineSet(oc)
			g.By("Update machineset with acceleratedNetworking: true")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machineset/"+machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"acceleratedNetworking":true,"vmSize":"Standard_D4s_v3"}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//test when set acceleratedNetworking: true, machine running needs nearly 9 minutes. so change the method timeout as 10 minutes.
			clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

			g.By("Check machine with acceleratedNetworking: true")
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.acceleratedNetworking}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("out:%s", out)
			o.Expect(out).To(o.ContainSubstring("true"))
		}
		e2e.Logf("Only azure platform supported for the test")
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-Medium-46967-Implement Ephemeral OS Disks - OS cache placement on Azure [Disruptive]", func() {
		g.By("Create a new machineset with Ephemeral OS Disks - OS cache placement")
		if clusterinfra.CheckPlatform(oc) == "azure" {
			machinesetName := "machineset-46967"
			ms := clusterinfra.MachineSetDescription{machinesetName, 0}
			defer ms.DeleteMachineSet(oc)
			ms.CreateMachineSet(oc)
			g.By("Update machineset with Ephemeral OS Disks - OS cache placement")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machineset/"+machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"vmSize":"Standard_D4s_v3","osDisk":{"diskSizeGB":30,"cachingType":"ReadOnly","diskSettings":{"ephemeralStorageLocation":"Local"},"managedDisk":{"storageAccountType":""}}}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

			g.By("Check machine with Ephemeral OS Disks - OS cache placement")
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.osDisk.diskSettings.ephemeralStorageLocation}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("out:%s", out)
			o.Expect(out).To(o.ContainSubstring("Local"))
		}
		e2e.Logf("Only azure platform supported for the test")
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-Medium-46303-Availability sets could be created when needed for Azure [Disruptive]", func() {
		if clusterinfra.CheckPlatform(oc) == "azure" {
			defaultWorkerMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
			region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset/"+defaultWorkerMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			infrastructureName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.infrastructureName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			availabilitySetName := infrastructureName + "_" + defaultWorkerMachinesetName + "-as"
			if region == "northcentralus" || region == "westus" {
				/*
					This case only supports on a region which doesn't have zones.
					These two regions cover most of the templates in flexy-templates and they don't have zones,
					so restricting the test is only applicable in these two regions.
				*/
				g.By("Create a new machineset")
				machinesetName := "machineset-46303"
				ms := clusterinfra.MachineSetDescription{machinesetName, 0}
				defer ms.DeleteMachineSet(oc)
				ms.CreateMachineSet(oc)

				g.By("Update machineset with availabilitySet already created for the default worker machineset")
				/*
				 If the available set is not created for the default worker machineset,
				 the machine will create failed and error message shows "Availability Set cannot be found".
				 Therefore, if machine created successfully with the available set,
				 then it can prove that the available set has been created when the default worker machineset is created.
				*/
				err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machineset/"+machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"availabilitySet":"`+availabilitySetName+`"}}}}}}`, "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

				g.By("Check machine with availabilitySet")
				out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.availabilitySet}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("out:%s", out)
				o.Expect(out == availabilitySetName).To(o.BeTrue())
			}
			e2e.Logf("The test is only applicable in \"northcentralus\" or \"westus\" region")
		}
		e2e.Logf("Only azure platform supported for the test")
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-Medium-47177-Medium-47201-[MDH] Machine Deletion Hooks appropriately block lifecycle phases [Disruptive]", func() {
		g.By("Create a new machineset with lifecycle hook")
		machinesetName := "machineset-47177-47201"
		ms := clusterinfra.MachineSetDescription{machinesetName, 0}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with lifecycle hook")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machineset/"+machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"lifecycleHooks":{"preDrain":[{"name":"drain1","owner":"drain-controller1"}],"preTerminate":[{"name":"terminate2","owner":"terminate-controller2"}]}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Delete newly created machine by scaling " + machinesetName + " to 0")
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas=0", "-n", "openshift-machine-api", "machineset", machinesetName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for machine to go into Deleting phase")
		err = wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.phase}").Output()
			if output != "Deleting" {
				e2e.Logf("machine is not in Deleting phase and waiting up to 2 seconds ...")
				return false, nil
			}
			e2e.Logf("machine is in Deleting phase")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Check machine phase failed")

		g.By("Check machine stuck in Deleting phase because of lifecycle hook")
		outDrain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.conditions[0]}").Output()
		e2e.Logf("outDrain:%s", outDrain)
		o.Expect(strings.Contains(outDrain, "\"message\":\"Drain operation currently blocked by: [{Name:drain1 Owner:drain-controller1}]\"") && strings.Contains(outDrain, "\"reason\":\"HookPresent\"") && strings.Contains(outDrain, "\"status\":\"False\"") && strings.Contains(outDrain, "\"type\":\"Drainable\"")).To(o.BeTrue())

		outTerminate, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.conditions[2]}").Output()
		e2e.Logf("outTerminate:%s", outTerminate)
		o.Expect(strings.Contains(outTerminate, "\"message\":\"Terminate operation currently blocked by: [{Name:terminate2 Owner:terminate-controller2}]\"") && strings.Contains(outTerminate, "\"reason\":\"HookPresent\"") && strings.Contains(outTerminate, "\"status\":\"False\"") && strings.Contains(outTerminate, "\"type\":\"Terminable\"")).To(o.BeTrue())

		g.By("Update machine without lifecycle hook")
		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("machine/"+machineName, "-n", "openshift-machine-api", "-p", `[{"op": "remove", "path": "/spec/lifecycleHooks/preDrain"},{"op": "remove", "path": "/spec/lifecycleHooks/preTerminate"}]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-Medium-47230-[MDH] Negative lifecycle hook validation [Disruptive]", func() {
		g.By("Create a new machineset")
		machinesetName := "machineset-47230"
		ms := clusterinfra.MachineSetDescription{machinesetName, 1}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		checkItems := []struct {
			patchstr string
			errormsg string
		}{
			{
				patchstr: `{"spec":{"lifecycleHooks":{"preTerminate":[{"name":"","owner":"drain-controller1"}]}}}`,
				errormsg: "spec.lifecycleHooks.preTerminate.name: Invalid value: \"\": spec.lifecycleHooks.preTerminate.name in body should be at least 3 chars long",
			},
			{
				patchstr: `{"spec":{"lifecycleHooks":{"preDrain":[{"name":"drain1","owner":""}]}}}`,
				errormsg: "spec.lifecycleHooks.preDrain.owner: Invalid value: \"\": spec.lifecycleHooks.preDrain.owner in body should be at least 3 chars long",
			},
			{
				patchstr: `{"spec":{"lifecycleHooks":{"preDrain":[{"name":"drain1","owner":"drain-controller1"},{"name":"drain1","owner":"drain-controller2"}]}}}`,
				errormsg: "spec.lifecycleHooks.preDrain[1]: Duplicate value: map[string]interface {}{\"name\":\"drain1\"}",
			},
		}

		for i, checkItem := range checkItems {
			g.By("Update machine with invalid lifecycle hook")
			out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machine/"+machineName, "-n", "openshift-machine-api", "-p", checkItem.patchstr, "--type=merge").Output()
			e2e.Logf("out"+strconv.Itoa(i)+":%s", out)
			o.Expect(strings.Contains(out, checkItem.errormsg)).To(o.BeTrue())
		}
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-Medium-44977-Machine with GPU is supported on gcp [Disruptive]", func() {
		if clusterinfra.CheckPlatform(oc) == "gcp" {
			g.By("Create a new machineset")
			machinesetName := "machineset-44977"
			ms := clusterinfra.MachineSetDescription{machinesetName, 0}
			defer ms.DeleteMachineSet(oc)
			ms.CreateMachineSet(oc)
			g.By("Update machineset with GPU")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machineset/"+machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"a2-highgpu-1g","onHostMaintenance":"Terminate","restartPolicy":"Always"}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

			g.By("Check machine with GPU")
			machineType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.machineType}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			onHostMaintenance, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.onHostMaintenance}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			restartPolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.restartPolicy}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("machineType:%s, onHostMaintenance:%s, restartPolicy:%s", machineType, onHostMaintenance, restartPolicy)
			o.Expect(strings.Contains(machineType, "a2-highgpu-1g") && strings.Contains(onHostMaintenance, "Terminate") && strings.Contains(restartPolicy, "Always")).To(o.BeTrue())
		}
		e2e.Logf("Only gcp platform supported for the test")
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-48363-Machine providerID should be consistent with node providerID", func() {
		g.By("Check machine providerID and node providerID are consistent")
		machineList := clusterinfra.ListAllMachineNames(oc)
		for _, machineName := range machineList {
			nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineName)
			machineProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", machineName, "-o=jsonpath={.spec.providerID}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.providerID}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(machineProviderID).Should(o.Equal(nodeProviderID))
		}
	})

	// author: huliu@redhat.com
	g.It("Longduration-NonPreRelease-Author:huliu-High-35513-Windows machine should successfully provision for aws [Disruptive]", func() {
		if clusterinfra.CheckPlatform(oc) == "aws" {
			g.By("Create a new machineset")
			machinesetName := "machineset-35513"
			ms := clusterinfra.MachineSetDescription{machinesetName, 0}
			defer ms.DeleteMachineSet(oc)
			ms.CreateMachineSet(oc)
			region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			var amiId string
			switch region {
			case "us-east-1", "us-iso-east-1":
				amiId = "ami-0d9cdd823beb0f50b"
			case "us-east-2":
				amiId = "ami-0e05cb5a56f9043da"
			case "cn-north-1":
				amiId = "ami-07a0c9b547ce24896"
			case "us-gov-west-1":
				amiId = "ami-0fc1f8653c0f1c371"
			default:
				e2e.Logf("Not support region for the case for now.")
				g.Skip("Not support region for the case for now.")
			}
			g.By("Update machineset with windows ami")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("machineset/"+machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"metadata":{"labels":{"machine.openshift.io/os-id": "Windows"}},"spec":{"providerSpec":{"value":{"ami":{"id":"`+amiId+`"}}}}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			clusterinfra.WaitForMachineProvisioned(oc, machinesetName)
		}
		e2e.Logf("Only aws platform supported for the test")
	})
})
