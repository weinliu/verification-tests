package clusterinfrastructure

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure Upgrade", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cluster-infrastructure-upgrade", exutil.KubeConfigPath())
		iaasPlatform clusterinfra.PlatformType
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = clusterinfra.CheckPlatform(oc)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-PreChkUpgrade-Medium-41804-Spot/preemptible instances should not block upgrade - azure [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovvirginia" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}

		g.By("Create a spot instance on azure")
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-41804"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		ms.CreateMachineSet(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"spotVMOptions":{}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterinfra.WaitForMachinesRunning(oc, 1, machinesetName)

		g.By("Check machine and node were labelled `interruptible-instance`")
		machine, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(machine).NotTo(o.BeEmpty())
		node, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-Longduration-NonPreRelease-PstChkUpgrade-Medium-41804-Spot/preemptible instances should not block upgrade - azure [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.Azure)
		randomMachinesetName := clusterinfra.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovvirginia" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-41804"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)

		g.By("Check machine and node were still be labelled `interruptible-instance`")
		machine, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(machine).NotTo(o.BeEmpty())
		node, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Medium-61086-Enable IMDSv2 on existing worker machines via machine set [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		historyVersions := getClusterHistoryVersions(oc)
		if strings.Contains(historyVersions, "4.6") {
			g.Skip("Skipping this case due to IMDSv2 is only supported on AWS clusters that were created with version 4.7 or later")
		}

		g.By("Create a new machineset")
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-61086"
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
		o.Expect(out).Should(o.ContainSubstring("Required"))
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Medium-62265-Ensure controlplanemachineset is generated automatically after upgrade", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.Nutanix, clusterinfra.OpenStack)
		cpmsOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
		e2e.Logf("cpmsOut:%s", cpmsOut)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-LEVEL0-Critical-22612-Cluster could scale up/down after upgrade [Disruptive][Slow]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.VSphere, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud, clusterinfra.Nutanix, clusterinfra.OpenStack)
		g.By("Create a new machineset")
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-22612"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Scale up machineset")
		clusterinfra.ScaleMachineSet(oc, machinesetName, 1)

		g.By("Scale down machineset")
		clusterinfra.ScaleMachineSet(oc, machinesetName, 0)
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-NonPreRelease-PstChkUpgrade-LEVEL0-Critical-70626-Service of type LoadBalancer can be created successful after upgrade", func() {
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud)
		if iaasPlatform == clusterinfra.AWS && strings.HasPrefix(getClusterRegion(oc), "us-iso") {
			g.Skip("Skipped: There is no public subnet on AWS C2S/SC2S disconnected clusters!")
		}
		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		loadBalancer := filepath.Join(ccmBaseDir, "svc-loadbalancer.yaml")
		loadBalancerService := loadBalancerServiceDescription{
			template:  loadBalancer,
			name:      "svc-loadbalancer-70626",
			namespace: oc.Namespace(),
		}
		g.By("Create loadBalancerService")
		defer loadBalancerService.deleteLoadBalancerService(oc)
		loadBalancerService.createLoadBalancerService(oc)

		g.By("Check External-IP assigned")
		getLBSvcIP(oc, loadBalancerService)
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-PreChkUpgrade-High-72031-Instances with custom DHCP option set should not block upgrade - AWS [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)

		g.By("Create a new dhcpOptions")
		var newDhcpOptionsID, currentDhcpOptionsID string
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()
		newDhcpOptionsID, err := awsClient.CreateDhcpOptionsWithDomainName("example72031.com")
		if err != nil {
			g.Skip("The credential is insufficient to perform create dhcpOptions operation, skip the cases!!")
		}

		g.By("Associate the VPC with the new dhcpOptionsId")
		machineName := clusterinfra.ListMasterMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		currentDhcpOptionsID, err = awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = awsClient.AssociateDhcpOptions(vpcID, newDhcpOptionsID)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new machineset")
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-72031"
		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 1}
		ms.CreateMachineSet(oc)
		//Add a specicacl tag for the original dhcp so that we can find it in PstChkUpgrade case
		err = awsClient.CreateTag(currentDhcpOptionsID, infrastructureName, "previousdhcp72031")
		o.Expect(err).NotTo(o.HaveOccurred())

		machineNameOfMachineSet := clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0]
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineNameOfMachineSet)
		readyStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readyStatus).Should(o.Equal("True"))
		internalDNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, machineNameOfMachineSet, "-o=jsonpath={.status.addresses[?(@.type==\"InternalDNS\")].address}", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(internalDNS, "example72031.com")).To(o.BeTrue())
	})

	// author: huliu@redhat.com
	g.It("Author:huliu-NonHyperShiftHOST-Longduration-NonPreRelease-PstChkUpgrade-High-72031-Instances with custom DHCP option set should not block upgrade - AWs [Disruptive]", func() {
		clusterinfra.SkipConditionally(oc)
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS)
		infrastructureName := clusterinfra.GetInfrastructureName(oc)
		machinesetName := infrastructureName + "-72031"
		machineset, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace).Output()
		if strings.Contains(machineset, "not found") {
			g.Skip("The machineset " + machinesetName + " is not created before upgrade, skip this case!")
		}
		clusterinfra.GetAwsCredentialFromCluster(oc)
		awsClient := exutil.InitAwsSession()

		machineName := clusterinfra.ListMasterMachineNames(oc)[0]
		instanceID, err := awsClient.GetAwsInstanceID(machineName)
		o.Expect(err).NotTo(o.HaveOccurred())
		vpcID, err := awsClient.GetAwsInstanceVPCId(instanceID)
		o.Expect(err).NotTo(o.HaveOccurred())
		newDhcpOptionsID, err := awsClient.GetDhcpOptionsIDOfVpc(vpcID)
		o.Expect(err).NotTo(o.HaveOccurred())
		previousDhcpOptionsID, err := awsClient.GetDhcpOptionsIDFromTag(infrastructureName, "previousdhcp72031")
		e2e.Logf("previousDhcpOptionsID:" + strings.Join(previousDhcpOptionsID, "*"))
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func(dhcpOptionsID []string) {
			if len(dhcpOptionsID) > 0 {
				e2e.Logf("previousDhcpOptionsID[0]:" + dhcpOptionsID[0])
			} else {
				e2e.Fail("there is no previousDhcpOptionsID")
			}
			err := awsClient.DeleteTag(dhcpOptionsID[0], infrastructureName, "previousdhcp72031")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = awsClient.AssociateDhcpOptions(vpcID, dhcpOptionsID[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			err = awsClient.DeleteDhcpOptions(newDhcpOptionsID)
			o.Expect(err).NotTo(o.HaveOccurred())
		}(previousDhcpOptionsID)

		ms := clusterinfra.MachineSetDescription{Name: machinesetName, Replicas: 0}
		defer clusterinfra.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)

		g.By("Check machine is still Running and node is still Ready")
		phase, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(phase).Should(o.Equal("Running"))
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, clusterinfra.GetMachineNamesFromMachineSet(oc, machinesetName)[0])
		readyStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.status.conditions[?(@.type==\"Ready\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(readyStatus).Should(o.Equal("True"))
	})
})
