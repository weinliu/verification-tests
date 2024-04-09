package clusterinfrastructure

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cluster-infrastructure-upgrade", exutil.KubeConfigPath())
		iaasPlatform string
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-PreChkUpgrade-Author:zhsun-Medium-41804-[Upgrade]Spot/preemptible instances should not block upgrade - Azure [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		randomMachinesetName := exutil.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovvirginia" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}

		g.By("Create a spot instance on azure")
		ms := exutil.MachineSetDescription{"machineset-41804", 0}
		ms.CreateMachineSet(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, "machineset-41804", "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"spotVMOptions":{}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, "machineset-41804")

		g.By("Check machine and node were labelled `interruptible-instance`")
		machine, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(machine).NotTo(o.BeEmpty())
		node, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-PstChkUpgrade-Author:zhsun-Medium-41804-[Upgrade]Spot/preemptible instances should not block upgrade - Azure [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		randomMachinesetName := exutil.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovvirginia" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}
		ms := exutil.MachineSetDescription{"machineset-41804", 0}
		defer exutil.WaitForMachinesDisapper(oc, "machineset-41804")
		defer ms.DeleteMachineSet(oc)

		g.By("Check machine and node were still be labelled `interruptible-instance`")
		machine, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(machine).NotTo(o.BeEmpty())
		node, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:zhsun-Medium-61086-[Upgrade] Enable IMDSv2 on existing worker machines via machine set [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		historyVersions := getClusterHistoryVersions(oc)
		if strings.Contains(historyVersions, "4.6") {
			g.Skip("Skipping this case due to IMDSv2 is only supported on AWS clusters that were created with version 4.7 or later")
		}

		g.By("Create a new machineset")
		machinesetName := "machineset-61086"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer exutil.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with imds required")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"Required"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.metadataServiceOptions.authentication}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("Required"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:huliu-Medium-62265-[Upgrade] Ensure controlplanemachineset is generated automatically after upgrade", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "nutanix", "openstack")
		cpmsOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
		e2e.Logf("cpmsOut:%s", cpmsOut)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:zhsun-LEVEL0-Critical-22612-[Upgrade] Cluster could scale up/down after upgrade [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "vsphere", "ibmcloud", "alibabacloud", "nutanix", "openstack")
		g.By("Create a new machineset")
		machinesetName := "machineset-22612"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer exutil.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Scale up machineset")
		exutil.ScaleMachineSet(oc, machinesetName, 1)

		g.By("Scale down machineset")
		exutil.ScaleMachineSet(oc, machinesetName, 0)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:zhsun-LEVEL0-Critical-70626-[Upgrade] Service of type LoadBalancer can be created successful after upgrade [Disruptive][Slow]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "ibmcloud", "alibabacloud")
		if strings.Contains(iaasPlatform, "aws") && strings.HasPrefix(getClusterRegion(oc), "us-iso") {
			g.Skip("Skipped: There is no public subnet on AWS C2S/SC2S disconnected clusters!")
		}
		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		loadBalancer := filepath.Join(ccmBaseDir, "svc-loadbalancer.yaml")
		loadBalancerService := loadBalancerServiceDescription{
			template:  loadBalancer,
			name:      "svc-loadbalancer",
			namespace: "default",
		}
		g.By("Create loadBalancerService")
		defer loadBalancerService.deleteLoadBalancerService(oc)
		loadBalancerService.createLoadBalancerService(oc)

		g.By("Check External-IP assigned")
		getLBSvcIP(oc, loadBalancerService)
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-PreChkUpgrade-Author:huliu-High-72031-[Upgrade] Instances with custom DHCP option set should not block upgrade - AWS [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")

		g.By("Create a new dhcpOptions")
		var newDhcpOptionsID, currentDhcpOptionsID string
		exutil.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		newDhcpOptionsID, err := awsClient.CreateDhcpOptionsWithDomainName("example72031.com")
		if err != nil {
			g.Skip("The credential is insufficient to perform create dhcpOptions operation, skip the cases!!")
		}

		g.By("Associate the VPC with the new dhcpOptionsId")
		machineName := exutil.ListMasterMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		currentDhcpOptionsID, err = awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = awsClient.AssociateDhcpOptions(vpcID, newDhcpOptionsID)
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new machineset")
		machinesetName := "machineset-72031"
		ms := exutil.MachineSetDescription{machinesetName, 1}
		ms.CreateMachineSet(oc)
		//Add a specicacl tag for the original dhcp so that we can find it in PstChkUpgrade case
		err = awsClient.CreateTag(currentDhcpOptionsID, "specialName", clusterID+"previousdhcp72031")
		o.Expect(err).NotTo(o.HaveOccurred())

		machineNameOfMachineSet := exutil.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		nodeName := exutil.GetNodeNameFromMachine(oc, machineNameOfMachineSet)
		readyStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readyStatus).Should(o.Equal("True"))
		internalDNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineNameOfMachineSet, "-o=jsonpath={.status.addresses[?(@.type==\"InternalDNS\")].address}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(internalDNS, "example72031.com")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-PstChkUpgrade-huliu-High-72031-[Upgrade] Instances with custom DHCP option set should not block upgrade - AWs [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		machinesetName := "machineset-72031"
		machineset, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace).Output()
		if strings.Contains(machineset, "not found") {
			g.Skip("The machineset machineset-72031 is not created before upgrade, skip this case!")
		}
		exutil.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()

		machineName := exutil.ListMasterMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		newDhcpOptionsID, err := awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		previousDhcpOptionsID, err := awsClient.GetDhcpOptionsIDFromTag("specialName", clusterID+"previousdhcp72031")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			err := awsClient.DeleteDhcpOptions(newDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		defer func() {
			err := awsClient.AssociateDhcpOptions(vpcID, previousDhcpOptionsID[0])
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		defer func() {
			err := awsClient.DeleteTag(previousDhcpOptionsID[0], "specialName", clusterID+"previousdhcp72031")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer exutil.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)

		g.By("Check machine is still Running and node is still Ready")
		phase, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(phase).Should(o.Equal("Running"))
		nodeName := exutil.GetNodeNameFromMachine(oc, exutil.GetMachineNamesFromMachineSet(oc, machinesetName)[0])
		readyStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readyStatus).Should(o.Equal("True"))
	})
})
