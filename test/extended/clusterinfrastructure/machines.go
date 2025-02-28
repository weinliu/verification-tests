package clusterinfrastructure

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure MAPI", func() {
	defer g.GinkgoRecover()
	var (
		oc                 = exutil.NewCLI("machine-api-operator", exutil.KubeConfigPath())
		infrastructureName string
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		infrastructureName = clusterinfra.GetInfrastructureName(oc)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-45772-MachineSet selector is immutable", func() {
		g.By("Create a new machineset")
		clusterinfra.SkipConditionally(oc)
		machinesetName := infrastructureName + "-45772"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with empty clusterID")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"selector":{"matchLabels":{"machine.openshift.io/cluster-api-cluster": null}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("selector is immutable"))
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-45377-Enable accelerated network via MachineSets on azure [Disruptive]", func() {
		g.By("Create a new machineset with acceleratedNetworking: true")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		machinesetName := infrastructureName + "-45377"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		arch, err := clusterinfra.GetArchitectureFromMachineSet(oc, machinesetName)
		o.Expect(err).NotTo(o.HaveOccurred())
		var vmSize string
		switch arch {
		case architecture.AMD64:
			vmSize = "Standard_D2s_v3"
		case architecture.ARM64:
			vmSize = "Standard_D8ps_v5"
		default:
			g.Skip("This case doesn't support other architectures than arm64, amd64")
		}
		g.By("Update machineset with acceleratedNetworking: true")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n",
			machineAPINamespace, "-p",
			fmt.Sprintf(`{"spec":{"replicas":1,"template":{"spec":{"providerSpec":`+
				`{"value":{"acceleratedNetworking":true,"vmSize":"%s"}}}}}}`, vmSize),
			"--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//test when set acceleratedNetworking: true, machine running needs nearly 9 minutes. so change the method timeout as 10 minutes.
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with acceleratedNetworking: true")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.acceleratedNetworking}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).To(o.ContainSubstring("true"))
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-46967-Implement Ephemeral OS Disks - OS cache placement on azure [Disruptive]", func() {
		g.By("Create a new machineset with Ephemeral OS Disks - OS cache placement")
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		skipTestIfSpotWorkers(oc)
		machinesetName := infrastructureName + "-46967"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		arch, err := clusterinfra.GetArchitectureFromMachineSet(oc, machinesetName)
		o.Expect(err).NotTo(o.HaveOccurred())
		var vmSize string
		switch arch {
		case architecture.AMD64:
			vmSize = "Standard_D2s_v3"
		case architecture.ARM64:
			vmSize = "Standard_D2plds_v5"
		default:
			g.Skip("This case doesn't support other architectures than arm64, amd64")
		}
		g.By("Update machineset with Ephemeral OS Disks - OS cache placement")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n",
			machineAPINamespace, "-p",
			fmt.Sprintf(`{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"vmSize":"%s",`+
				`"osDisk":{"diskSizeGB":30,"cachingType":"ReadOnly","diskSettings":{"ephemeralStorageLocation":"Local"},`+
				`"managedDisk":{"storageAccountType":""}}}}}}}}`, vmSize), "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with Ephemeral OS Disks - OS cache placement")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.osDisk.diskSettings.ephemeralStorageLocation}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).To(o.ContainSubstring("Local"))
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-46303-Availability sets could be created when needed for azure [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		defaultWorkerMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
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
		machinesetName := infrastructureName + "-46303"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
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

		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with availabilitySet")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.availabilitySet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("availability set name is: %s", out)
		o.Expect(out == availabilitySetName).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-47177-Medium-47201-[MDH] Machine Deletion Hooks appropriately block lifecycle phases [Disruptive]", func() {
		g.By("Create a new machineset with lifecycle hook")
		clusterinfra.SkipConditionally(oc)
		machinesetName := infrastructureName + "-47177-47201"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with lifecycle hook")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"lifecycleHooks":{"preDrain":[{"name":"drain1","owner":"drain-controller1"}],"preTerminate":[{"name":"terminate2","owner":"terminate-controller2"}]}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

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
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("outDrain:%s", outDrain)
		o.Expect(strings.Contains(outDrain, "\"message\":\"Drain operation currently blocked by: [{Name:drain1 Owner:drain-controller1}]\"") && strings.Contains(outDrain, "\"reason\":\"HookPresent\"") && strings.Contains(outDrain, "\"status\":\"False\"") && strings.Contains(outDrain, "\"type\":\"Drainable\"")).To(o.BeTrue())

		outTerminate, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.conditions[2]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("outTerminate:%s", outTerminate)
		o.Expect(strings.Contains(outTerminate, "\"message\":\"Terminate operation currently blocked by: [{Name:terminate2 Owner:terminate-controller2}]\"") && strings.Contains(outTerminate, "\"reason\":\"HookPresent\"") && strings.Contains(outTerminate, "\"status\":\"False\"") && strings.Contains(outTerminate, "\"type\":\"Terminable\"")).To(o.BeTrue())

		g.By("Update machine without lifecycle hook")
		machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachine, machineName, "-n", "openshift-machine-api", "-p", `[{"op": "remove", "path": "/spec/lifecycleHooks/preDrain"},{"op": "remove", "path": "/spec/lifecycleHooks/preTerminate"}]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-47230-[MDH] Negative lifecycle hook validation [Disruptive]", func() {
		g.By("Create a new machineset")
		clusterinfra.SkipConditionally(oc)
		machinesetName := infrastructureName + "-47230"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
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
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-44977-Machine with GPU is supported on gcp [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)
		architecture.SkipArchitectures(oc, architecture.ARM64)

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-44977"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		arch, err := clusterinfra.GetArchitectureFromMachineSet(oc, machinesetName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if arch != architecture.AMD64 {
			g.Skip("The selected machine set's arch is not amd64, skip this case!")
		}
		//check supported zone for gpu
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if !strings.Contains(zone, "us-central1-") {
			g.Skip("not valid zone for GPU machines")
		}
		g.By("Update machineset with GPU")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{ "gpus": [ { "count": 1,"type": "nvidia-tesla-p100" }],"machineType":"n1-standard-1", "zone":"us-central1-c", "onHostMaintenance":"Terminate","restartPolicy":"Always"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with GPU")
		gpuType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.gpus[0].type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		onHostMaintenance, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.onHostMaintenance}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		restartPolicy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.restartPolicy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("gpuType:%s, onHostMaintenance:%s, restartPolicy:%s", gpuType, onHostMaintenance, restartPolicy)
		o.Expect(strings.Contains(gpuType, "nvidia-tesla-p100") && strings.Contains(onHostMaintenance, "Terminate") && strings.Contains(restartPolicy, "Always")).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-48363-Machine providerID should be consistent with node providerID", func() {
		g.By("Check machine providerID and node providerID are consistent")
		clusterinfra.SkipConditionally(oc)
		machineList := clusterinfra.ListAllMachineNames(oc)
		for _, machineName := range machineList {
			nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineName)
			if nodeName == "" {
				continue
			}
			machineProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineName, "-o=jsonpath={.spec.providerID}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeProviderID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.providerID}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(machineProviderID).Should(o.Equal(nodeProviderID))
		}
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-High-35513-Windows machine should successfully provision for aws [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		architecture.SkipNonAmd64SingleArch(oc)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-35513"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var amiID string
		switch region {
		case "us-east-1", "us-iso-east-1":
			amiID = "ami-0e09e139aca053387"
		case "us-east-2":
			amiID = "ami-0f4f40c1e7ef56be6"
		default:
			e2e.Logf("Not support region for the case for now.")
			g.Skip("Not support region for the case for now.")
		}
		g.By("Update machineset with windows ami")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"metadata":{"labels":{"machine.openshift.io/os-id": "Windows"}},"spec":{"providerSpec":{"value":{"ami":{"id":"`+amiID+`"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.WaitForMachineProvisioned(oc, machinesetName)
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-48012-Change AWS EBS GP3 IOPS in MachineSet should take affect on aws [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		clusterinfra.SkipForAwsOutpostCluster(oc)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-48012"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with gp3 iops 5000")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"blockDevices":[{"ebs":{"volumeType":"gp3","iops":5000}}]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check on aws instance with gp3 iops 5000")
		instanceID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-o=jsonpath={.items[0].status.providerStatus.instanceId}", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterinfra.GetAwsCredentialFromCluster(oc)

		volumeInfo, err := clusterinfra.GetAwsVolumeInfoAttachedToInstanceID(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("volumeInfo:%s", volumeInfo)
		o.Expect(strings.Contains(volumeInfo, "\"Iops\":5000") && strings.Contains(volumeInfo, "\"VolumeType\":\"gp3\"")).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-High-33040-Required configuration should be added to the ProviderSpec to enable spot instances - azure [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovtexas" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}

		g.By("Create a spot instance on azure")
		clusterinfra.SkipConditionally(oc)
		machinesetName := infrastructureName + "-33040"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"spotVMOptions":{}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine and node were labelled as an `interruptible-instance`")
		machine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(machine).NotTo(o.BeEmpty())
		node, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-48594-AWS EFA network interfaces should be supported via machine api [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		architecture.SkipNonAmd64SingleArch(oc)
		clusterinfra.SkipForAwsOutpostCluster(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" && region != "us-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-48594"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with networkInterfaceType: EFA")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"networkInterfaceType":"EFA","instanceType":"m5dn.24xlarge"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with networkInterfaceType: EFA")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.networkInterfaceType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).Should(o.Equal("EFA"))
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-48595-Negative validation for AWS NetworkInterfaceType [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		architecture.SkipNonAmd64SingleArch(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" && region != "us-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-48595"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with networkInterfaceType: invalid")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"networkInterfaceType":"invalid","instanceType":"m5dn.24xlarge"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value")).To(o.BeTrue())

		g.By("Update machineset with not supported instance types")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"networkInterfaceType":"EFA","instanceType":"m6i.xlarge"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.errorMessage}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(strings.Contains(out, "not supported")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-49827-Ensure pd-balanced disk is supported on GCP via machine api [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-49827"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with invalid disk type")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `[{"op":"replace","path":"/spec/template/spec/providerSpec/value/disks/0/type","value":"invalid"}]`, "--type=json").Output()
		o.Expect(strings.Contains(out, "Unsupported value")).To(o.BeTrue())

		g.By("Update machineset with pd-balanced disk type")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `[{"op":"replace","path":"/spec/replicas","value": 1},{"op":"replace","path":"/spec/template/spec/providerSpec/value/disks/0/type","value":"pd-balanced"}]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with pd-balanced disk type")
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.disks[0].type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).Should(o.Equal("pd-balanced"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-50731-Enable IMDSv2 on existing worker machines via machine set [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-50731"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with imds required")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"Required"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.metadataServiceOptions.authentication}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("out:%s", out)
		o.Expect(out).Should(o.ContainSubstring("Required"))

		g.By("Update machineset with imds optional")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"Optional"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		machineName := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		oc.AsAdmin().WithoutNamespace().Run("delete").Args(mapiMachine, machineName, "-n", machineAPINamespace).Execute()
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[*].spec.providerSpec.value.metadataServiceOptions.authentication}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("Optional"))

		g.By("Update machine with invalid authentication ")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"invalid"}}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value: \"invalid\": Allowed values are either 'Optional' or 'Required'")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-37915-Creating machines using KMS keys from AWS [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		clusterinfra.SkipForAwsOutpostCluster(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		kmsClient := exutil.NewKMSClient(region)
		key, err := kmsClient.CreateKey(infrastructureName + " key 37915")
		if err != nil {
			g.Skip("Create key failed, skip the cases!!")
		}
		defer func() {
			err := kmsClient.DeleteKey(key)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-37915"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"blockDevices": [{"ebs":{"encrypted":true,"iops":0,"kmsKey":{"arn":"`+key+`"},"volumeSize":120,"volumeType":"gp2"}}]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with KMS keys")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.blockDevices[0].ebs.kmsKey.arn}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("arn:aws:kms"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-52471-Enable configuration of boot diagnostics when creating VMs on azure [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)

		g.By("Create a machineset configuring boot diagnostics with Azure managed storage accounts")
		machinesetName := infrastructureName + "-52471-1"
		ms1 := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms1.DeleteMachineSet(oc)
		ms1.CreateMachineSet(oc)

		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"AzureManaged"}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.diagnostics.boot.storageAccountType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("AzureManaged"))

		g.By("Create machineset configuring boot diagnostics with Customer managed storage accounts")
		machinesetName = infrastructureName + "-52471-2"
		ms2 := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms2.DeleteMachineSet(oc)
		ms2.CreateMachineSet(oc)

		storageAccount, _, err1 := exutil.GetAzureStorageAccountFromCluster(oc)
		o.Expect(err1).NotTo(o.HaveOccurred())
		cloudName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		storageAccountURISuffix := ".blob.core.windows.net/"
		if strings.ToLower(cloudName) == "azureusgovernmentcloud" {
			storageAccountURISuffix = ".blob.core.usgovcloudapi.net/"
		}
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"CustomerManaged","customerManaged":{"storageAccountURI":"https://`+storageAccount+storageAccountURISuffix+`"}}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.diagnostics.boot.storageAccountType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("CustomerManaged"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-52473-Webhook validations for azure boot diagnostics [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)

		g.By("Create a machineset")
		machinesetName := infrastructureName + "-52473-1"
		ms1 := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
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
		clusterinfra.WaitForMachineFailed(oc, machinesetName)

		g.By("Update machineset with invalid storage account")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":2,"template":{"spec":{"providerSpec":{"value":{"diagnostics":{"boot":{"storageAccountType":"CustomerManaged","customerManaged":{"storageAccountURI":"https://invalid.blob.core.windows.net"}}}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)
	})

	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Low-36489-Machineset creation when publicIP:true in disconnected or normal (stratergy private or public) azure,aws,gcp enviroment [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure, clusterinfra.AWS, clusterinfra.GCP)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-36489"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		iaasPlatform := clusterinfra.CheckPlatform(oc)

		publicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
		if err != nil {
			g.Fail("Issue with dns setup")
		}
		g.By("Update machineset with publicIP: true")
		switch iaasPlatform {
		case clusterinfra.AWS:
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"publicIP": true}}}}}}`, "--type=merge").Output()
			if publicZone == "" && iaasPlatform == clusterinfra.Azure {
				o.Expect(msg).To(o.ContainSubstring("publicIP is not allowed in Azure disconnected installation with publish strategy as internal"))
			} else {
				o.Expect(err).NotTo(o.HaveOccurred())
				//to scale up machineset with publicIP: true
				err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas": 1}}`, "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
			}
		case clusterinfra.Azure:
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"publicIP": true}}}}}}`, "--type=merge").Output()
			if publicZone == "" && iaasPlatform == clusterinfra.Azure {
				o.Expect(msg).To(o.ContainSubstring("publicIP is not allowed in Azure disconnected installation with publish strategy as internal"))
			} else {
				o.Expect(err).NotTo(o.HaveOccurred())
				//to scale up machineset with publicIP: true
				//OutboundRules for VMs with public IpConfigurations with capi installation cannot provision publicIp(Limitation Azure)
				err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas": 1}}`, "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				machineName := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]

				g.By("Check machineset with publicIP: true is not allowed for Azure")
				status, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args(mapiMachine, machineName, "-n", machineAPINamespace).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(strings.Contains(status, "NicWithPublicIpCannotReferencePoolWithOutboundRule"))
			}
		case clusterinfra.GCP:
			network, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.networkInterfaces[0].network}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetwork, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.networkInterfaces[0].subnetwork}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			patchString := fmt.Sprintf(`{"spec":{"template":{"spec":{"providerSpec":{"value":{"networkInterfaces":[{"network":"%s","subnetwork":"%s","publicIP": true}]}}}},"replicas":1}}`, network, subnetwork)
			_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", patchString, "--type=merge").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
		}
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-51013-machine api should issue client cert when AWS DNS suffix missing [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)

		g.By("Create a new dhcpOptions")
		var newDhcpOptionsID, currentDhcpOptionsID string
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		newDhcpOptionsID, err := awsClient.CreateDhcpOptions()
		if err != nil {
			g.Skip("The credential is insufficient to perform create dhcpOptions operation, skip the cases!!")
		}
		defer awsClient.DeleteDhcpOptions(newDhcpOptionsID)

		g.By("Associate the VPC with the new dhcpOptionsId")
		machineName := clusterinfra.ListMasterMachineNames(oc)[0]
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
		machinesetName := infrastructureName + "-51013"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0])
		e2e.Logf("nodeName:%s", nodeName)
		o.Expect(strings.HasPrefix(nodeName, "ip-")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-59718-[Nutanix] Support bootType categories and project fields of NutanixMachineProviderConfig [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Nutanix)
		// skip zones other than Development-LTS
		zones, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-o=jsonpath={.items[*].metadata.labels.machine\\.openshift\\.io\\/zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(zones, "Development-LTS") {
			g.Skip(fmt.Sprintf("this case can be only run in Development-LTS zone, but is's %s", zones))
		}
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-59718"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset adding these new fields")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"bootType":"Legacy","categories":[{"key":"AppType","value":"Kubernetes"},{"key":"Environment","value":"Testing"}],"project":{"type":"name","name":"qe-project"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with these new fields")
		bootType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.bootType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		categories, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.categories}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		projectName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.project.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("bootType:%s, categories:%s, projectName:%s", bootType, categories, projectName)
		o.Expect(strings.Contains(bootType, "Legacy") && strings.Contains(categories, "Kubernetes") && strings.Contains(categories, "Testing") && strings.Contains(projectName, "qe-project")).To(o.BeTrue())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-59760-Create confidential compute VMs on GCP [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)
		//We should enable this case when Google provide this support for their ARM Machines
		//https://issues.redhat.com/browse/OCPQE-22305
		architecture.SkipArchitectures(oc, architecture.ARM64)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-59760"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		arch, err := clusterinfra.GetArchitectureFromMachineSet(oc, machinesetName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if arch != architecture.AMD64 {
			g.Skip("The selected machine set's arch is not amd64, skip this case!")
		}

		g.By("Update machineset with confidential compute options")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"`+clusterinfra.GetInstanceTypeValuesByProviderAndArch(clusterinfra.GCP, arch)[1]+`","onHostMaintenance":"Terminate","confidentialCompute":"Enabled"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with confidentialCompute enabled")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.confidentialCompute}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("Enabled"))

		g.By("Validate onHostMaintenance should  be set to terminate in case confidential compute is enabled")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"`+clusterinfra.GetInstanceTypeValuesByProviderAndArch(clusterinfra.GCP, arch)[1]+`","onHostMaintenance":"invalid","confidentialCompute":"Enabled"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value: \"invalid\": onHostMaintenance must be either Migrate or Terminate")).To(o.BeTrue())

		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"`+clusterinfra.GetInstanceTypeValuesByProviderAndArch(clusterinfra.GCP, arch)[1]+`","onHostMaintenance":"Migrate","confidentialCompute":"Enabled"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value: \"Migrate\": ConfidentialCompute require OnHostMaintenance to be set to Terminate, the current value is: Migrate")).To(o.BeTrue())

		g.By("Validate the instance type support confidential computing")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"`+clusterinfra.GetInstanceTypeValuesByProviderAndArch(clusterinfra.GCP, arch)[0]+`","onHostMaintenance":"Terminate","confidentialCompute":"Enabled"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value: \""+clusterinfra.GetInstanceTypeValuesByProviderAndArch(clusterinfra.GCP, arch)[0]+"\": ConfidentialCompute require machine type in the following series: n2d,c2d")).To(o.BeTrue())
	})

	//author miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-57438-Add support to Shielded VMs on GCP [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)
		//We should enable this case when the bug fixed
		//https://issues.redhat.com/browse/OCPBUGS-17904
		architecture.SkipArchitectures(oc, architecture.ARM64)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-57438"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		arch, err := clusterinfra.GetArchitectureFromMachineSet(oc, machinesetName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if arch != architecture.AMD64 {
			g.Skip("The selected machine set's arch is not amd64, skip this case!")
		}

		g.By("Update machineset with shieldedInstanceConfig compute options Enabled")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"shieldedInstanceConfig": {"secureBoot": "Enabled","integrityMonitoring": "Enabled","virtualizedTrustedPlatformModule": "Enabled"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with shieldedInstanceConfig options enabled")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.shieldedInstanceConfig}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("{\"integrityMonitoring\":\"Enabled\",\"secureBoot\":\"Enabled\",\"virtualizedTrustedPlatformModule\":\"Enabled\"}"))

		g.By("Validate the webhooks warns with invalid values of shieldedVM config")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"shieldedInstanceConfig": {"secureBoot": "nabled","integrityMonitoring": "Enabled","virtualizedTrustedPlatformModule": "Enabled"}}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "secureBoot must be either Enabled or Disabled")).To(o.BeTrue())
	})

	//author miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Longduration-NonPreRelease-High-48464-Dedicated tenancy should be exposed on aws providerspec [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		clusterinfra.SkipForAwsOutpostCluster(oc)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-48464"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset to have dedicated tenancy ")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placement": {"tenancy": "dedicated"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine available with dedicated tenancy ")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.placement.tenancy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("dedicated"))

		g.By("Validate the webhooks warns with invalid values of tenancy config")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placement": {"tenancy": "invalid"}}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid providerSpec.tenancy, the only allowed options are: default, dedicated, host")).To(o.BeTrue())
	})

	//author miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Longduration-NonPreRelease-High-39639-host-based disk encryption at VM on azure platform	[Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-39639"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset to have encryption at host enabled ")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"securityProfile": {"encryptionAtHost": true}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine available with encrytption enabled ")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.securityProfile}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("{\"encryptionAtHost\":true"))
	})

	//author huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-32269-Implement validation/defaulting for AWS [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		mapiBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "mapi")
		defaultMachinesetAwsTemplate := filepath.Join(mapiBaseDir, "default-machineset-aws.yaml")

		clusterID := clusterinfra.GetInfrastructureName(oc)
		masterArchtype := architecture.GetControlPlaneArch(oc)
		randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
		msArchtype, err := clusterinfra.GetArchitectureFromMachineSet(oc, randomMachinesetName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if masterArchtype != msArchtype {
			g.Skip("The selected machine set's arch is not the same with the master machine's arch, skip this case!")
		}
		amiID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.ami.id}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.placement.availabilityZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sgName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.securityGroups[0].filters[0].values[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		subnet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.filters[0].values[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if subnet == "" {
			subnet, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			defaultMachinesetAwsTemplate = filepath.Join(mapiBaseDir, "default-machineset-aws-id.yaml")
		}
		iamInstanceProfileID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.iamInstanceProfile.id}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defaultMachinesetAws := defaultMachinesetAwsDescription{
			name:                 infrastructureName + "-32269-default",
			clustername:          clusterID,
			template:             defaultMachinesetAwsTemplate,
			amiID:                amiID,
			availabilityZone:     availabilityZone,
			sgName:               sgName,
			subnet:               subnet,
			namespace:            machineAPINamespace,
			iamInstanceProfileID: iamInstanceProfileID,
		}
		defer clusterinfra.WaitForMachinesDisapper(oc, defaultMachinesetAws.name)
		defer defaultMachinesetAws.deleteDefaultMachineSetOnAws(oc)
		defaultMachinesetAws.createDefaultMachineSetOnAws(oc)
		instanceTypeMachine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+defaultMachinesetAws.name, "-o=jsonpath={.items[0].spec.providerSpec.value.instanceType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		switch arch := architecture.ClusterArchitecture(oc); arch {
		case architecture.AMD64:
			o.Expect(instanceTypeMachine).Should(o.Equal("m5.large"))
		case architecture.ARM64:
			o.Expect(instanceTypeMachine).Should(o.Equal("m6g.large"))
		default:
			e2e.Logf("ignoring the validation of the instanceType for cluster architecture %s", arch.String())
		}
	})

	//author huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-37497-ClusterInfrastructure Dedicated Spot Instances could be created [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		architecture.SkipNonAmd64SingleArch(oc)
		clusterinfra.SkipForAwsOutpostCluster(oc)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-37497"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset to Dedicated Spot Instances")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"spotMarketOptions":{},"instanceType":"c4.8xlarge","placement": {"tenancy": "dedicated"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-64909-AWS Placement group support for MAPI [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" && region != "us-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		_, err = awsClient.GetPlacementGroupByName("pgcluster")
		if err != nil {
			g.Skip("There is no this placement group for testing, skip the cases!!")
		}

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-64909"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		availabilityZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.placement.availabilityZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if availabilityZone != "us-east-2b" && availabilityZone != "us-east-1b" {
			g.Skip("Restricted to b availabilityZone testing because cluster placement group cannot span zones. But it's " + availabilityZone)
		}
		g.By("Update machineset with Placement group")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placementGroupName":"pgcluster"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with Placement group")
		placementGroupName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.placementGroupName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("placementGroupName:%s", placementGroupName)
		o.Expect(placementGroupName).Should(o.Equal("pgcluster"))
	})
	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-LEVEL0-Critical-25436-Scale up/scale down the cluster by changing the replicas of the machineSet [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.VSphere, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud, clusterinfra.Nutanix, clusterinfra.OpenStack, clusterinfra.Ovirt)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-25436g"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Scale up machineset")
		clusterinfra.ScaleMachineSet(oc, machinesetName, 1)

		g.By("Scale down machineset")
		clusterinfra.ScaleMachineSet(oc, machinesetName, 0)
	})

	// author: dtobolik@redhat.com
	g.It("Author:dtobolik-NonHyperShiftHOST-NonPreRelease-Medium-66866-AWS machineset support for multiple AWS security groups [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)

		g.By("Create aws security group")
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		randomMachineName := clusterinfra.ListWorkerMachineNames(oc)[0]
		randomInstanceID, err := awsClient.GetAwsInstanceID(randomMachineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(randomInstanceID)
		o.Expect(err).NotTo(o.HaveOccurred())

		sgName := "ocp-66866-sg"
		sgID, err := awsClient.CreateSecurityGroup(sgName, vpcID, "ocp-66866 testing security group")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer awsClient.DeleteSecurityGroup(sgID)
		err = awsClient.CreateTag(sgID, "Name", sgName)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new machineset")
		machineSetName := infrastructureName + "-66866"
		machineSet := clusterinfra.MachineSetDescription{Name: machineSetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machineSetName)
		defer machineSet.DeleteMachineSet(oc)
		machineSet.CreateMachineSet(oc)

		g.By("Add security group to machineset")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machineSetName, "-n", "openshift-machine-api", "-p", `[{"op":"replace","path":"/spec/replicas","value":1},{"op":"add","path":"/spec/template/spec/providerSpec/value/securityGroups/-","value":{"filters":[{"name":"tag:Name","values":["`+sgName+`"]}]}}]`, "--type=json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machineSetName)

		g.By("Check security group is attached")
		machineName := clusterinfra.GetMachineNamesFromMachineSet(oc, machineSetName)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		securityGroups, err := awsClient.GetInstanceSecurityGroupIDs(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(securityGroups).Should(o.ContainElement(sgID))
	})

	//author zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-33058-Implement defaulting machineset values for azure [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		credType, err := oc.AsAdmin().Run("get").Args("cloudcredentials.operator.openshift.io/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(credType, "Manual") {
			g.Skip("Skip test on azure sts cluster")
		}
		mapiBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "mapi")
		defaultMachinesetAzureTemplate := filepath.Join(mapiBaseDir, "default-machineset-azure.yaml")

		clusterID := clusterinfra.GetInfrastructureName(oc)
		randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
		location, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		vnet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.vnet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		subnet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		networkResourceGroup, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.networkResourceGroup}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defaultMachinesetAzure := defaultMachinesetAzureDescription{
			name:                 infrastructureName + "-33058-default",
			clustername:          clusterID,
			template:             defaultMachinesetAzureTemplate,
			location:             location,
			vnet:                 vnet,
			subnet:               subnet,
			namespace:            machineAPINamespace,
			networkResourceGroup: networkResourceGroup,
		}
		defer clusterinfra.WaitForMachinesDisapper(oc, defaultMachinesetAzure.name)
		defaultMachinesetAzure.createDefaultMachineSetOnAzure(oc)
		defer defaultMachinesetAzure.deleteDefaultMachineSetOnAzure(oc)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-46966-Validation webhook check for gpus on GCP [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.GCP)
		skipTestIfSpotWorkers(oc)
		architecture.SkipArchitectures(oc, architecture.ARM64)

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-46966"
		ms := clusterinfra.MachineSetDescription{machinesetName, 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(zone, "us-central1-") {
			g.Skip("not valid zone for GPU machines")
		}

		g.By("1.Update machineset with A100 GPUs (A family) and set gpus")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"machineType":"a2-highgpu-1g","gpus": [ { "count": 1,"type": "nvidia-tesla-p100" }]}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "A2 machine types have already attached gpus, additional gpus cannot be specified")).To(o.BeTrue())

		g.By("2.Update machineset with nvidia-tesla-A100 Type")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"machineType":"n1-standard-1","gpus": [ { "count": 1,"type": "nvidia-tesla-a100" }]}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "nvidia-tesla-a100 gpus, are only attached to the A2 machine types")).To(o.BeTrue())

		g.By("3.Update machineset with other machine type families and set gpus")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"e2-medium","gpus": [ { "count": 1,"type": "nvidia-tesla-p100" }]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)
		machineName := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		out, _ = oc.AsAdmin().WithoutNamespace().Run("describe").Args(mapiMachine, machineName, "-n", machineAPINamespace).Output()
		o.Expect(out).Should(o.ContainSubstring("MachineType e2-medium does not support accelerators. Only A2, A3 and N1 machine type families support guest accelerators"))
		clusterinfra.ScaleMachineSet(oc, machinesetName, 0)

		g.By("4.Update machineset with A100 GPUs (A2 family) nvidia-tesla-a100, onHostMaintenance is set to Migrate")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"machineType":"a2-highgpu-1g","onHostMaintenance":"Migrate"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Forbidden: When GPUs are specified or using machineType with pre-attached GPUs(A2 machine family), onHostMaintenance must be set to Terminate")).To(o.BeTrue())

		g.By("5.Update machineset with A100 GPUs (A2 family) nvidia-tesla-a100, restartPolicy with an invalid value")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"machineType":"a2-highgpu-1g","restartPolicy": "Invalid","onHostMaintenance": "Terminate"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value: \"Invalid\": restartPolicy must be either Never or Always")).To(o.BeTrue())

		g.By("6.Update machineset with A100 GPUs (A2 family) nvidia-tesla-a100, onHostMaintenance with an invalid value")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"machineType":"a2-highgpu-1g","restartPolicy": "Always","onHostMaintenance": "Invalid"}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Invalid value: \"Invalid\": onHostMaintenance must be either Migrate or Terminate")).To(o.BeTrue())

		g.By("7.Update machineset with other GPU types,  count with an invalid value")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"n1-standard-1","restartPolicy": "Always","onHostMaintenance": "Terminate","gpus": [ { "count": -1,"type": "nvidia-tesla-p100" }]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)
		machineName = clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		out, _ = oc.AsAdmin().WithoutNamespace().Run("describe").Args(mapiMachine, machineName, "-n", machineAPINamespace).Output()
		o.Expect(strings.Contains(out, "Only A2, A3 and N1 machine type families support guest accelerators") || strings.Contains(out, "AcceleratorType nvidia-tesla-p100 not available in the zone")).To(o.BeTrue())
		clusterinfra.ScaleMachineSet(oc, machinesetName, 0)

		g.By("8.Update machineset with other GPU types,  type with an empty value")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"template":{"spec":{"providerSpec":{"value":{"machineType":"n1-standard-1","restartPolicy": "Always","onHostMaintenance": "Terminate","gpus": [ { "count": 1,"type": "" }]}}}}}}`, "--type=merge").Output()
		o.Expect(strings.Contains(out, "Required value: Type is required")).To(o.BeTrue())

		g.By("9.Update machineset with other GPU types,  type with an invalid value")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"machineType":"n1-standard-1","restartPolicy": "Always","onHostMaintenance": "Terminate","gpus": [ { "count": 1,"type": "invalid" }]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)
		machineName = clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		out, _ = oc.AsAdmin().WithoutNamespace().Run("describe").Args(mapiMachine, machineName, "-n", machineAPINamespace).Output()
		o.Expect(strings.Contains(out, "Only A2, A3 and N1 machine type families support guest accelerators") || strings.Contains(out, "AcceleratorType invalid not available in the zone")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-High-30379-New machine can join cluster when VPC has custom DHCP option set [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)

		g.By("Create a new dhcpOptions")
		var newDhcpOptionsID, currentDhcpOptionsID string
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		newDhcpOptionsID, err := awsClient.CreateDhcpOptionsWithDomainName("example30379.com")
		if err != nil {
			g.Skip("The credential is insufficient to perform create dhcpOptions operation, skip the cases!!")
		}
		defer func() {
			err := awsClient.DeleteDhcpOptions(newDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Associate the VPC with the new dhcpOptionsId")
		machineName := clusterinfra.ListMasterMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		currentDhcpOptionsID, err = awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			err := awsClient.AssociateDhcpOptions(vpcID, currentDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = awsClient.AssociateDhcpOptions(vpcID, newDhcpOptionsID)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-30379"
		ms := clusterinfra.MachineSetDescription{machinesetName, 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		machineNameOfMachineSet := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineNameOfMachineSet)
		readyStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readyStatus).Should(o.Equal("True"))
		internalDNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineNameOfMachineSet, "-o=jsonpath={.status.addresses[?(@.type==\"InternalDNS\")].address}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(internalDNS, "example30379.com")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-73762-New machine can join cluster when VPC has custom DHCP option set containing multiple domain names [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)

		g.By("Create a new dhcpOptions")
		var newDhcpOptionsID, currentDhcpOptionsID string
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		newDhcpOptionsID, err := awsClient.CreateDhcpOptionsWithDomainName("EXAMple73762A.com. example73762b.com. eXaMpLe73762C.COM")
		if err != nil {
			g.Skip("The credential is insufficient to perform create dhcpOptions operation, skip the cases!!")
		}
		defer func() {
			err := awsClient.DeleteDhcpOptions(newDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Associate the VPC with the new dhcpOptionsId")
		machineName := clusterinfra.ListMasterMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		currentDhcpOptionsID, err = awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			err := awsClient.AssociateDhcpOptions(vpcID, currentDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = awsClient.AssociateDhcpOptions(vpcID, newDhcpOptionsID)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-73762"
		ms := clusterinfra.MachineSetDescription{machinesetName, 1}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		machineNameOfMachineSet := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineNameOfMachineSet)
		readyStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readyStatus).Should(o.Equal("True"))
		internalDNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineNameOfMachineSet, "-o=jsonpath={.status.addresses[?(@.type==\"InternalDNS\")].address}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(internalDNS, "EXAMple73762A.com") && strings.Contains(internalDNS, "example73762b.com") && strings.Contains(internalDNS, "eXaMpLe73762C.COM")).To(o.BeTrue())
	})

	//author zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-High-73851-Node shouldn't have uninitialized taint [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-73851"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset taint")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"taints":[{"key":"node.kubernetes.io/unreachable","effect":"NoExecute"},{"key":"anything","effect":"NoSchedule"},{"key":"node-role.kubernetes.io/infra","effect":"NoExecute"},{"key":"node.kubernetes.io/not-ready","effect":"NoExecute"}]}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check no uninitialized taint in node")
		machineName := clusterinfra.GetLatestMachineFromMachineSet(oc, machinesetName)
		o.Expect(machineName).NotTo(o.BeEmpty())
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineName)
		o.Eventually(func() bool {
			readyStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
			return err == nil && o.Expect(readyStatus).Should(o.Equal("True"))
		}).WithTimeout(5 * time.Minute).WithPolling(30 * time.Second).Should(o.BeTrue())

		taints, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.spec.taints}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(taints).ShouldNot(o.ContainSubstring("uninitialized"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-73668-Create machineset with Reserved Capacity [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		skipTestIfSpotWorkers(oc)
		azureCloudName, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(azureErr).NotTo(o.HaveOccurred())
		if azureCloudName == "AzureStackCloud" || azureCloudName == "AzureUSGovernmentCloud" {
			g.Skip("Skip for ASH and azure Gov due to no zone for ash, and for USGov it's hard to getclient with baseURI!")
		}

		exutil.By("Create a machineset")
		machinesetName := infrastructureName + "-73668"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if zone == "" {
			g.Skip("Zone doesn't exist, capacity reservation group cannot be set on a virtual machine which is part of an availability set!")
		}
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		machineType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.vmSize}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create capacityReservationGroup and capacityReservation")
		resourceGroup, err := exutil.GetAzureCredentialFromCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		capacityReservationGroupName := "capacityReservationGroup73668"
		capacityReservationName := "capacityReservation73668"
		azClientSet := exutil.NewAzureClientSetWithRootCreds(oc)
		capacityReservationGroup, err := azClientSet.CreateCapacityReservationGroup(context.Background(), capacityReservationGroupName, resourceGroup, region, zone)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(capacityReservationGroup).NotTo(o.BeEmpty())
		err = azClientSet.CreateCapacityReservation(context.Background(), capacityReservationGroupName, capacityReservationName, region, resourceGroup, machineType, zone)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			ms.DeleteMachineSet(oc)
			clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
			azClientSet.DeleteCapacityReservation(context.Background(), capacityReservationGroupName, capacityReservationName, resourceGroup)
			azClientSet.DeleteCapacityReservationGroup(context.Background(), capacityReservationGroupName, resourceGroup)
		}()

		exutil.By("Patch machineset with valid capacityReservationGroupID")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"capacityReservationGroupID": "`+capacityReservationGroup+`"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		exutil.By("Check machine with capacityReservationGroupID")
		capacityReservationGroupID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.capacityReservationGroupID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(capacityReservationGroupID).Should(o.ContainSubstring("capacityReservationGroups"))

		exutil.By("Patch machineset with empty capacityReservationGroupID and set replicas to 2")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":2,"template":{"spec":{"providerSpec":{"value":{ "capacityReservationGroupID": ""}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 2, machinesetName)

		exutil.By("Check machine without capacityReservationGroupID")
		machine := clusterinfra.GetLatestMachineFromMachineSet(oc, machinesetName)
		capacityReservationGroupID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machine, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerSpec.value.capacityReservationGroupID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(capacityReservationGroupID).To(o.BeEmpty())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Medium-73669-Webhook validation for Reserved Capacity [Disruptive]", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		skipTestIfSpotWorkers(oc)
		azureCloudName, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(azureErr).NotTo(o.HaveOccurred())
		if azureCloudName == "AzureStackCloud" || azureCloudName == "AzureUSGovernmentCloud" {
			g.Skip("Skip for ASH and azure Gov due to no zone for ash, and for USGov it's hard to getclient with baseURI!")
		}

		exutil.By("Create a machineset ")
		machinesetName := infrastructureName + "-73669"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if zone == "" {
			g.Skip("Zone doesn't exist, capacity reservation group cannot be set on a virtual machine which is part of an availability set!")
		}

		exutil.By("Patch machineset that the value of capacityReservationGroupID does not start with /")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{ "capacityReservationGroupID": "subscriptions/53b8f551-f0fc-4bea-8cba-6d1fefd54c8a/resourceGroups/ZHSUN-AZ9-DVD88-RG/providers/Microsoft.Compute/capacityReservationGroups/zhsun-capacity"}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("must start with '/'"))

		exutil.By("Patch machineset with invalid subscriptions")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{ "capacityReservationGroupID": "/subscrip/53b8f551-f0fc-4bea-8cba-6d1fefd54c8a/resourceGroups/ZHSUN-AZ9-DVD88-RG/providers/Microsoft.Compute/capacityReservationGroups/zhsun-capacity"}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("capacityReservationGroupID: Invalid value"))

		exutil.By("Patch machineset with invalid resourceGroups")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{ "capacityReservationGroupID": "/subscriptions/53b8f551-f0fc-4bea-8cba-6d1fefd54c8a/resource/ZHSUN-AZ9-DVD88-RG/providers/Microsoft.Compute/capacityReservationGroups/zhsun-capacity"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)

		exutil.By("Patch machineset with invalid capacityReservationGroups")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":2,"template":{"spec":{"providerSpec":{"value":{ "capacityReservationGroupID": "/subscriptions/53b8f551-f0fc-4bea-8cba-6d1fefd54c8a/resourceGroups/ZHSUN-AZ9-DVD88-RG/providers/Microsoft.Compute/capacityReservation/zhsun-capacity"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-74603-[MAPI] Support AWS Placement Group Partition Number [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" && region != "us-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		_, err = awsClient.GetPlacementGroupByName("pgpartition3")
		if err != nil {
			g.Skip("There is no this placement group for testing, skip the cases!!")
		}

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-74603"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		exutil.By("Patch machineset only with valid partition placementGroupName")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placementGroupName":"pgpartition3"}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		exutil.By("Check machine with placementGroupName and without placementGroupPartition ")
		placementGroupName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.placementGroupName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(placementGroupName).Should(o.Equal("pgpartition3"))
		placementGroupPartition, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.placementGroupPartition}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(placementGroupPartition).To(o.BeEmpty())

		exutil.By("Patch machineset with valid partition placementGroupName and placementGroupPartition")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":2,"template":{"spec":{"providerSpec":{"value":{"placementGroupName":"pgpartition3", "placementGroupPartition":2}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 2, machinesetName)

		exutil.By("Check machine with placementGroupName and placementGroupPartition")
		machine := clusterinfra.GetLatestMachineFromMachineSet(oc, machinesetName)
		placementGroupName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machine, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerSpec.value.placementGroupName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(placementGroupName).Should(o.Equal("pgpartition3"))
		placementGroupPartition, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machine, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerSpec.value.placementGroupPartition}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(placementGroupPartition).Should(o.Equal("2"))
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-75037-[MAPI] Webhook validation for AWS Placement Group Partition Number [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region != "us-east-2" && region != "us-east-1" {
			g.Skip("Not support region " + region + " for the case for now.")
		}
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		_, err = awsClient.GetPlacementGroupByName("pgpartition3")
		if err != nil {
			g.Skip("There is no this placement group for testing, skip the cases!!")
		}

		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-75037"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		exutil.By("Update machineset with invalid Placement group partition nubmer")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placementGroupName":"pgpartition3", "placementGroupPartition":0}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("placementGroupPartition: Invalid value: 0: providerSpec.placementGroupPartition must be between 1 and 7"))

		exutil.By("Update machineset with placementGroupPartition but without placementGroupName")
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placementGroupPartition":2}}}}}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("placementGroupPartition: Invalid value: 2: providerSpec.placementGroupPartition is set but providerSpec.placementGroupName is empty"))

		exutil.By("Patch machineset with valid placementGroupPartition but cluster placementGroupName")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placementGroupName":"pgcluster", "placementGroupPartition":2}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)

		exutil.By("Patch machineset with invalid placementGroupPartition of the partition placementGroupName")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"placementGroupName":"pgpartition3", "placementGroupPartition":4}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachineFailed(oc, machinesetName)
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-24721-Add support for machine tags [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		exutil.By("Create a machineset")
		machinesetName := infrastructureName + "-24721"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		exutil.By("Update machineset with tags")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"tags":{"key24721a":"value24721a","key24721b":"value24721b"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		exutil.By("Check machine with tags")
		tags, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.tags}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("tags:%s", tags)
		o.Expect(tags).Should(o.ContainSubstring("key24721b"))
	})

	//author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-52602-Drain operation should be asynchronous from the other machine operations [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.GCP, clusterinfra.Azure, clusterinfra.IBMCloud, clusterinfra.Nutanix, clusterinfra.VSphere, clusterinfra.OpenStack)

		exutil.By("Create a new machineset")
		machinesetName := infrastructureName + "-52602"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		exutil.By("Scale machineset to 5")
		clusterinfra.ScaleMachineSet(oc, machinesetName, 5)

		exutil.By("Create PDB")
		miscDir := exutil.FixturePath("testdata", "clusterinfrastructure", "misc")
		pdbTemplate := filepath.Join(miscDir, "pdb.yaml")
		workloadTemplate := filepath.Join(miscDir, "workload-with-label.yaml")
		pdb := PodDisruptionBudget{name: "pdb-52602", namespace: machineAPINamespace, template: pdbTemplate, label: "label-52602"}
		workLoad := workLoadDescription{name: "workload-52602", namespace: machineAPINamespace, template: workloadTemplate, label: "label-52602"}
		defer pdb.deletePDB(oc)
		pdb.createPDB(oc)
		defer workLoad.deleteWorkLoad(oc)
		workLoad.createWorkLoad(oc)

		exutil.By("Delete machines")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "--wait=false").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check machines can quickly be created without waiting for the other Nodes to drain.")
		o.Eventually(func() bool {
			machineNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[*].metadata.name}", "-n", machineAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			machines := strings.Fields(machineNames)
			if len(machines) == 10 {
				return true
			}
			return false
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-76367-[MAPI] Allow creating Nutanix worker VMs with GPUs [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Nutanix)
		// skip zones other than Development-GPU
		zones, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-o=jsonpath={.items[*].metadata.labels.machine\\.openshift\\.io\\/zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(zones, "Development-GPU") {
			g.Skip(fmt.Sprintf("this case can be only run in Development-GPU zone, but is's %s", zones))
		}
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-76367"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with gpus")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"gpus":[{"type":"Name","name":"Tesla T4 compute"}]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with gpus")
		gpus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.gpus}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("gpus:%s", gpus)
		o.Expect(strings.Contains(gpus, "Tesla T4 compute")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-Medium-76366-[MAPI] Allow creating Nutanix VMs with multiple disks [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Nutanix)
		g.By("Create a new machineset")
		machinesetName := infrastructureName + "-76366"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		g.By("Update machineset with data disks")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"dataDisks":[{"deviceProperties":{"deviceType":"Disk","adapterType":"SCSI","deviceIndex":1},"diskSize":"1Gi","storageConfig":{"diskMode":"Standard"}}]}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine with data disks")
		dataDisks, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.dataDisks}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("dataDisks:%s", dataDisks)
		o.Expect(strings.Contains(dataDisks, "SCSI")).To(o.BeTrue())
	})
})
