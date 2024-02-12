package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service machinepool", func() {
	defer g.GinkgoRecover()
	var (
		clusterID          string
		rosaClient         *rosacli.Client
		machinePoolService rosacli.MachinePoolService
		isHosted           bool
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		machinePoolService = rosaClient.MachinePool

		var err error
		isHosted, err = rosaClient.Cluster.IsHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.AfterEach(func() {
		g.By("Clean remaining resources")
		err := rosaClient.CleanResources(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.It("Author:xueli-Critical-66872-Customer can create machinepool with volume size set via rosacli [Serial]", func() {

		if isHosted {
			g.Skip("This test case only work for classic cluster")
			// Below step will be enabled after OCM-3417 fixed
			// o.Expect(err).ToNot(o.HaveOccurred())
			// o.Expect(output.String()).Should(o.ContainSubstring(""))
			return
		}
		mpID := "mp-66359"
		expectedDiskSize := "186 GiB" // it is 200GB
		machineType := "r5.xlarge"

		g.By("Create a machinepool with the disk size")
		_, err := machinePoolService.CreateMachinePool(clusterID, mpID,
			"--replicas", "0",
			"--disk-size", "200GB",
			"--instance-type", machineType,
		)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the machinepool list")
		output, err := machinePoolService.ListMachinePool(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		mplist, err := machinePoolService.ReflectMachinePoolList(output)
		o.Expect(err).ToNot(o.HaveOccurred())

		mp := mplist.Machinepool(mpID)
		o.Expect(mp).ToNot(o.BeNil(), "machine pool is not found for the cluster")
		o.Expect(mp.DiskSize).To(o.Equal(expectedDiskSize))
		o.Expect(mp.InstanceType).To(o.Equal(machineType))

		g.By("Check the default worker pool description")
		output, err = machinePoolService.DescribeMachinePool(clusterID, mpID)
		o.Expect(err).ToNot(o.HaveOccurred())
		mpD, err := machinePoolService.ReflectMachinePoolDescription(output)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(mpD.DiskSize).To(o.Equal(expectedDiskSize))

		g.By("Create another machinepool with volume size 0.5TiB")
		mpID = "mp-66359-2"
		expectedDiskSize = "512 GiB" // it is 0.5TiB
		machineType = "m5.2xlarge"
		_, err = machinePoolService.CreateMachinePool(clusterID, mpID,
			"--replicas", "0",
			"--disk-size", "0.5TiB",
			"--instance-type", machineType,
		)

		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the machinepool list")
		output, err = machinePoolService.ListMachinePool(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		mplist, err = machinePoolService.ReflectMachinePoolList(output)
		o.Expect(err).ToNot(o.HaveOccurred())

		mp = mplist.Machinepool(mpID)
		o.Expect(mp).ToNot(o.BeNil(), "machine pool is not found for the cluster")
		o.Expect(mp.DiskSize).To(o.Equal(expectedDiskSize))
		o.Expect(mp.DiskSize).To(o.Equal(expectedDiskSize))
		o.Expect(mp.InstanceType).To(o.Equal(machineType))

	})

	g.It("Author:mgahagan-High-43251-rosacli user can create spot machinepool by the rosacli command [Serial]", func() {
		if isHosted {
			g.Skip("This test case only work for classic cluster")
			return
		}
		g.By("Create a spot machinepool on the cluster")
		machinePoolName := "spotmp"
		output, err := machinePoolService.CreateMachinePool(clusterID, machinePoolName, "--spot-max-price", "10.2", "--use-spot-instances",
			"--replicas", "0")
		o.Expect(err).ToNot(o.HaveOccurred())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Machine pool '%s' created successfully on cluster '%s'", machinePoolName, clusterID))

		g.By("Create another machinepool without spot instances")
		machinePoolName = "nospotmp"
		output, err = machinePoolService.CreateMachinePool(clusterID, machinePoolName, "--replicas", "0")
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Machine pool '%s' created successfully on cluster '%s'", machinePoolName, clusterID))

		g.By("Create another machinepool with use-spot-instances but no spot-max-price set")
		machinePoolName = "nopricemp"
		output, err = machinePoolService.CreateMachinePool(clusterID, machinePoolName, "--use-spot-instances", "--replicas", "0")
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Machine pool '%s' created successfully on cluster '%s'", machinePoolName, clusterID))

		g.By("Confirm list of machinepools contains all created machinepools with SpotInstance field set appropriately")
		output, err = machinePoolService.ListMachinePool(clusterID)
		o.Expect(err).To(o.BeNil())
		mpTab, err := machinePoolService.ReflectMachinePoolList(output)
		o.Expect(err).To(o.BeNil())
		for _, mp := range mpTab.MachinePools {
			switch mp.ID {
			case "spotmp":
				o.Expect(mp.SpotInstances).To(o.Equal("Yes (max $10.2)"))
			case "nospotmp":
				o.Expect(mp.SpotInstances).To(o.Equal("No"))
			case "nopricemp":
				o.Expect(mp.SpotInstances).To(o.Equal("Yes (on-demand)"))
			default:
				continue
			}
		}

	})
})
