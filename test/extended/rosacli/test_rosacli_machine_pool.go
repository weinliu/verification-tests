package rosacli

import (
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A machinepool", func() {
	var (
		clusterID          string
		rosaClient         *rosacli.Client
		machinePoolService rosacli.MachinePoolService
		isHosted           bool
		mpClean            func(string, string) = func(clusterID string, mpID string) {
			g.By("Check if the machinepool existing for the cluster")
			output, err := machinePoolService.DescribeMachinePool(clusterID, mpID)
			if err != nil && strings.Contains(output.String(), "not found") {
				return
			}
			g.By("Delete the machinepool")
			_, err = machinePoolService.DeleteMachinePool(clusterID, mpID)
			o.Expect(err).ToNot(o.HaveOccurred())

			g.By("Detect the machinepool again")
			_, err = machinePoolService.DescribeMachinePool(clusterID, mpID)
			o.Expect(err).To(o.HaveOccurred(), fmt.Sprintf("Machinepool %s cannot be deleted from cluster %s", mpID, clusterID))

		}
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		machinePoolService = rosaClient.MachinePool

		var err error
		isHosted, err = isHostedCPCluster(clusterID)
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
		_, err := machinePoolService.CreateMachinePool(clusterID,
			"--name", mpID,
			"--replicas", "0",
			"--disk-size", "200GB",
			"--instance-type", machineType,
		)

		o.Expect(err).ToNot(o.HaveOccurred())
		defer mpClean(clusterID, mpID)

		g.By("Check the machinepool list")
		output, err := machinePoolService.ListMachinePool(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		mplist, err := machinePoolService.ReflectMachinePoolList(output)
		o.Expect(err).ToNot(o.HaveOccurred())

		mp, err := mplist.Machinepool(mpID)
		o.Expect(err).ToNot(o.HaveOccurred())
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
		_, err = machinePoolService.CreateMachinePool(clusterID,
			"--name", mpID,
			"--replicas", "0",
			"--disk-size", "0.5TiB",
			"--instance-type", machineType,
		)

		o.Expect(err).ToNot(o.HaveOccurred())
		defer mpClean(clusterID, mpID)

		g.By("Check the machinepool list")
		output, err = machinePoolService.ListMachinePool(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		mplist, err = machinePoolService.ReflectMachinePoolList(output)
		o.Expect(err).ToNot(o.HaveOccurred())

		mp, err = mplist.Machinepool(mpID)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(mp).ToNot(o.BeNil(), "machine pool is not found for the cluster")
		o.Expect(mp.DiskSize).To(o.Equal(expectedDiskSize))
		o.Expect(mp.DiskSize).To(o.Equal(expectedDiskSize))
		o.Expect(mp.InstanceType).To(o.Equal(machineType))

	})
})
