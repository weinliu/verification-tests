package rosacli

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A Cluster Verification", func() {
	var (
		clusterID          string
		rosaClient         *rosacli.Client
		machinePoolService rosacli.MachinePoolService
		clusterConfig      *ClusterConfig
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		// clusterID = getClusterIDENVExisted()
		clusterID = getClusterID() // For Jean Chen
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		machinePoolService = rosaClient.MachinePool
		var err error
		clusterConfig, err = parseProfile(getClusterConfigFile())
		o.Expect(err).ToNot(o.HaveOccurred())
	})
	g.It("Author:xueli-Critical-66359-Create rosa cluster with volume size will work via rosacli [Serial]", func() {
		g.By("Classic cluster check")
		isHosted, err := isHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		if isHosted {
			g.Skip("This case is only working for classic right now")
		}

		alignDiskSize := func(diskSize string) string {
			aligned := strings.Join(strings.Split(diskSize, " "), "")
			return aligned
		}

		g.By("Set expected worker pool size")
		expectedDiskSize := clusterConfig.WorkerDiskSize
		if expectedDiskSize == "" {
			expectedDiskSize = "300GiB" // if no worker disk size set, it will use default value
		}

		g.By("Check the machinepool list")
		output, err := machinePoolService.ListMachinePool(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		mplist, err := machinePoolService.ReflectMachinePoolList(output)
		o.Expect(err).ToNot(o.HaveOccurred())

		workPool, err := mplist.Machinepool(defaultClassicWorkerPool)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(workPool).ToNot(o.BeNil(), "worker pool is not found for the cluster")
		o.Expect(alignDiskSize(workPool.DiskSize)).To(o.Equal(expectedDiskSize))

		g.By("Check the default worker pool description")
		output, err = machinePoolService.DescribeMachinePool(clusterID, defaultClassicWorkerPool)
		o.Expect(err).ToNot(o.HaveOccurred())
		mpD, err := machinePoolService.ReflectMachinePoolDescription(output)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(alignDiskSize(mpD.DiskSize)).To(o.Equal(expectedDiskSize))

	})

	g.It("Author:xueli-Critical-57056-Create ROSA cluster with default-mp-labels option will succeed [Serial]", func() {
		g.By("Classic cluster check")
		isHosted, err := isHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		if isHosted {
			g.Skip("This case is only working for classic right now")
		}

		g.By("Check the cluster config")
		mpLables := strings.Join(strings.Split(clusterConfig.DefaultMpLabels, ","), ", ")

		g.By("Check the machinepool list")
		output, err := machinePoolService.ListMachinePool(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		mplist, err := machinePoolService.ReflectMachinePoolList(output)
		o.Expect(err).ToNot(o.HaveOccurred())

		workPool, err := mplist.Machinepool(defaultClassicWorkerPool)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(workPool).ToNot(o.BeNil(), "worker pool is not found for the cluster")
		o.Expect(workPool.Lables).To(o.Equal(mpLables))

		g.By("Check the default worker pool description")
		output, err = machinePoolService.DescribeMachinePool(clusterID, defaultClassicWorkerPool)
		o.Expect(err).ToNot(o.HaveOccurred())

		mpD, err := machinePoolService.ReflectMachinePoolDescription(output)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(mpD.Lables).To(o.Equal(mpLables))

	})
})
