package rosacli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/logext"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service Node Pools testing", func() {
	defer g.GinkgoRecover()

	var (
		clusterID          string
		rosaClient         *rosacli.Client
		clusterService     rosacli.ClusterService
		machinePoolService rosacli.MachinePoolService
		versionService     rosacli.VersionService
	)

	const (
		defaultNodePoolReplicas = "2"
	)

	g.BeforeEach(func() {
		var err error

		g.By("Get the cluster")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		machinePoolService = rosaClient.MachinePool
		clusterService = rosaClient.Cluster
		versionService = rosaClient.Version

		g.By("Check hosted cluster")
		hosted, err := clusterService.IsHostedCPCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		if !hosted {
			g.Skip("Node pools are only supported on Hosted clusters")
		}
	})

	g.AfterEach(func() {
		g.By("Clean remaining resources")
		err := rosaClient.CleanResources(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.It("Author:tradisso-Critical-56782-rosacli Create/Edit/List/Delete node pool of the hosted cluster will succeed [Serial]", func() {
		nodePoolName := rosacli.GenerateRandomName("np-56782", 2)
		labels := "label1=value1,label2=value2"
		taints := "t1=v1:NoSchedule,l2=:NoSchedule"
		instanceType := "m5.2xlarge"

		g.By("Retrieve cluster initial information")
		cluster, err := clusterService.DescribeClusterAndReflect(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		cpVersion := cluster.OpenshiftVersion

		g.By("Create new nodepool")
		output, err := machinePoolService.CreateMachinePool(clusterID, nodePoolName,
			"--replicas", "0",
			"--instance-type", instanceType,
			"--labels", labels,
			"--taints", taints)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Machine pool '%s' created successfully on hosted cluster '%s'", nodePoolName, clusterID))

		g.By("Check created nodepool")
		npList, err := machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		np := npList.Nodepool(nodePoolName)
		o.Expect(np).ToNot(o.BeNil())
		o.Expect(np.AutoScaling).To(o.Equal("No"))
		o.Expect(np.Replicas).To(o.Equal("0/0"))
		o.Expect(np.InstanceType).To(o.Equal(instanceType))
		o.Expect(np.AvalaiblityZones).ToNot(o.BeNil())
		o.Expect(np.Subnet).ToNot(o.BeNil())
		o.Expect(np.Version).To(o.Equal(cpVersion))
		o.Expect(np.AutoRepair).To(o.Equal("Yes"))
		o.Expect(len(rosacli.ParseLabels(np.Labels))).To(o.Equal(len(rosacli.ParseLabels(labels))))
		o.Expect(rosacli.ParseLabels(np.Labels)).To(o.ContainElements(rosacli.ParseLabels(labels)))
		o.Expect(len(rosacli.ParseTaints(np.Taints))).To(o.Equal(len(rosacli.ParseTaints(taints))))
		o.Expect(rosacli.ParseTaints(np.Taints)).To(o.ContainElements(rosacli.ParseTaints(taints)))

		g.By("Edit nodepool")
		newLabels := "l3=v3"
		newTaints := "t3=value3:NoExecute"
		replicasNb := "3"
		output, err = machinePoolService.EditMachinePool(clusterID, nodePoolName,
			"--replicas", replicasNb,
			"--labels", newLabels,
			"--taints", newTaints)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Updated machine pool '%s' on hosted cluster '%s'", nodePoolName, clusterID))

		g.By("Check edited nodepool")
		npList, err = machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		np = npList.Nodepool(nodePoolName)
		o.Expect(np).ToNot(o.BeNil())
		o.Expect(np.Replicas).To(o.Equal(fmt.Sprintf("0/%s", replicasNb)))
		o.Expect(len(rosacli.ParseLabels(np.Labels))).To(o.Equal(len(rosacli.ParseLabels(newLabels))))
		o.Expect(rosacli.ParseLabels(np.Labels)).To(o.BeEquivalentTo(rosacli.ParseLabels(newLabels)))
		o.Expect(len(rosacli.ParseTaints(np.Taints))).To(o.Equal(len(rosacli.ParseTaints(newTaints))))
		o.Expect(rosacli.ParseTaints(np.Taints)).To(o.BeEquivalentTo(rosacli.ParseTaints(newTaints)))

		g.By("Check describe nodepool")
		npDesc, err := machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
		o.Expect(err).ToNot(o.HaveOccurred())

		o.Expect(npDesc).ToNot(o.BeNil())
		o.Expect(npDesc.AutoScaling).To(o.Equal("No"))
		o.Expect(npDesc.DesiredReplicas).To(o.Equal(replicasNb))
		o.Expect(npDesc.CurrentReplicas).To(o.Equal("0"))
		o.Expect(npDesc.InstanceType).To(o.Equal(instanceType))
		o.Expect(npDesc.AvalaiblityZones).ToNot(o.BeNil())
		o.Expect(npDesc.Subnet).ToNot(o.BeNil())
		o.Expect(npDesc.Version).To(o.Equal(cpVersion))
		o.Expect(npDesc.AutoRepair).To(o.Equal("Yes"))
		o.Expect(len(rosacli.ParseLabels(npDesc.Labels))).To(o.Equal(len(rosacli.ParseLabels(newLabels))))
		o.Expect(rosacli.ParseLabels(npDesc.Labels)).To(o.BeEquivalentTo(rosacli.ParseLabels(newLabels)))
		o.Expect(len(rosacli.ParseTaints(npDesc.Taints))).To(o.Equal(len(rosacli.ParseTaints(newTaints))))
		o.Expect(rosacli.ParseTaints(npDesc.Taints)).To(o.BeEquivalentTo(rosacli.ParseTaints(newTaints)))

		g.By("Wait for nodepool replicas available")
		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 20*time.Minute, false, func(context.Context) (bool, error) {
			npDesc, err := machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
			if err != nil {
				return false, err
			}
			return npDesc.CurrentReplicas == replicasNb, nil
		})
		exutil.AssertWaitPollNoErr(err, "Replicas are not ready after 600")

		g.By("Delete nodepool")
		output, err = machinePoolService.DeleteMachinePool(clusterID, nodePoolName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Successfully deleted machine pool '%s' from hosted cluster '%s'", nodePoolName, clusterID))

		g.By("Nodepool does not appear anymore")
		npList, err = machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(npList.Nodepool(nodePoolName)).To(o.BeNil())

		if len(npList.NodePools) == 1 {
			g.By("Try to delete remaining nodepool")
			output, err = machinePoolService.DeleteMachinePool(clusterID, npList.NodePools[0].ID)
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Failed to delete machine pool '%s' on hosted cluster '%s': The last node pool can not be deleted from a cluster.", npList.NodePools[0].ID, clusterID))
		}
	})

	g.It("Author:tradisso-Critical-60202-rosacli Create node pool for the hosted cluster with subnets via rosacli is succeed [Serial]", func() {
		var subnets []string
		nodePoolName := rosacli.GenerateRandomName("np-60202", 2)
		replicasNumber := 3
		maxReplicasNumber := 6

		g.By("Retrieve cluster nodes information")
		CD, err := clusterService.DescribeClusterAndReflect(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		initialNodesNumber, err := rosacli.RetrieveDesiredComputeNodes(CD)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("List nodepools")
		npList, err := machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		for _, np := range npList.NodePools {
			o.Expect(np.ID).ToNot(o.BeNil())
			if strings.HasPrefix(np.ID, rosacli.DefaultHostedWorkerPool) {
				o.Expect(np.AutoScaling).ToNot(o.BeNil())
				o.Expect(np.Subnet).ToNot(o.BeNil())
				o.Expect(np.AutoRepair).ToNot(o.BeNil())
			}

			if !slices.Contains(subnets, np.Subnet) {
				subnets = append(subnets, np.Subnet)
			}
		}

		g.By("Create new nodepool with defined subnet")
		output, err := machinePoolService.CreateMachinePool(clusterID, nodePoolName,
			"--replicas", strconv.Itoa(replicasNumber),
			"--subnet", subnets[0])
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Machine pool '%s' created successfully on hosted cluster '%s'", nodePoolName, clusterID))

		npList, err = machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		np := npList.Nodepool(nodePoolName)
		o.Expect(np).ToNot(o.BeNil())
		o.Expect(np.AutoScaling).To(o.Equal("No"))
		o.Expect(np.Replicas).To(o.Equal("0/3"))
		o.Expect(np.AvalaiblityZones).ToNot(o.BeNil())
		o.Expect(np.Subnet).To(o.Equal(subnets[0]))
		o.Expect(np.AutoRepair).To(o.Equal("Yes"))

		g.By("Check cluster nodes information with new replicas")
		CD, err = clusterService.DescribeClusterAndReflect(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		newNodesNumber, err := rosacli.RetrieveDesiredComputeNodes(CD)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(newNodesNumber).To(o.Equal(initialNodesNumber + replicasNumber))

		g.By("Add autoscaling to nodepool")
		output, err = machinePoolService.EditMachinePool(clusterID, nodePoolName,
			"--enable-autoscaling",
			"--min-replicas", strconv.Itoa(replicasNumber),
			"--max-replicas", strconv.Itoa(maxReplicasNumber),
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Updated machine pool '%s' on hosted cluster '%s'", nodePoolName, clusterID))
		npList, err = machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		np = npList.Nodepool(nodePoolName)
		o.Expect(np).ToNot(o.BeNil())
		o.Expect(np.AutoScaling).To(o.Equal("Yes"))

		// Change autorepair
		output, err = machinePoolService.EditMachinePool(clusterID, nodePoolName,
			"--autorepair=false",

			// Temporary fix until https://issues.redhat.com/browse/OCM-5186 is corrected
			"--enable-autoscaling",
			"--min-replicas", strconv.Itoa(replicasNumber),
			"--max-replicas", strconv.Itoa(maxReplicasNumber),
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Updated machine pool '%s' on hosted cluster '%s'", nodePoolName, clusterID))
		npList, err = machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		np = npList.Nodepool(nodePoolName)
		o.Expect(np).ToNot(o.BeNil())
		o.Expect(np.AutoRepair).To(o.Equal("No"))

		g.By("Delete nodepool")
		output, err = machinePoolService.DeleteMachinePool(clusterID, nodePoolName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(rosaClient.Parser.TextData.Input(output).Parse().Tip()).Should(o.ContainSubstring("Successfully deleted machine pool '%s' from hosted cluster '%s'", nodePoolName, clusterID))

		g.By("Check cluster nodes information after deletion")
		CD, err = clusterService.DescribeClusterAndReflect(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		newNodesNumber, err = rosacli.RetrieveDesiredComputeNodes(CD)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(newNodesNumber).To(o.Equal(initialNodesNumber))

		g.By("Create new nodepool with replicas 0")
		replicas0NPName := rosacli.GenerateRandomName(nodePoolName, 2)
		_, err = machinePoolService.CreateMachinePool(clusterID, replicas0NPName,
			"--replicas", strconv.Itoa(0),
			"--subnet", subnets[0])
		o.Expect(err).ToNot(o.HaveOccurred())
		npList, err = machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		np = npList.Nodepool(replicas0NPName)
		o.Expect(np).ToNot(o.BeNil())
		o.Expect(np.Replicas).To(o.Equal("0/0"))

		g.By("Create new nodepool with min replicas 0")
		minReplicas0NPName := rosacli.GenerateRandomName(nodePoolName, 2)
		_, err = machinePoolService.CreateMachinePool(clusterID, minReplicas0NPName,
			"--enable-autoscaling",
			"--min-replicas", strconv.Itoa(0),
			"--max-replicas", strconv.Itoa(3),
			"--subnet", subnets[0],
		)
		o.Expect(err).To(o.HaveOccurred())
	})

	g.It("Author:tradisso-Critical-63178-rosacli Create Nodepool with tuning config [Serial]", func() {
		tuningConfigService := rosaClient.TuningConfig
		nodePoolName := rosacli.GenerateRandomName("np-63178", 2)
		tuningConfig1Name := rosacli.GenerateRandomName("tuned01", 2)
		tuningConfig2Name := rosacli.GenerateRandomName("tuned02", 2)
		tuningConfig3Name := rosacli.GenerateRandomName("tuned03", 2)
		allTuningConfigNames := []string{tuningConfig1Name, tuningConfig2Name, tuningConfig3Name}

		tuningConfigPayload := `
		{
			"profile": [
			  {
				"data": "[main]\nsummary=Custom OpenShift profile\ninclude=openshift-node\n\n[sysctl]\nvm.dirty_ratio=\"25\"\n",
				"name": "%s-profile"
			  }
			],
			"recommend": [
			  {
				"priority": 10,
				"profile": "%s-profile"
			  }
			]
		 }
		`

		g.By("Prepare tuning configs")
		_, err := tuningConfigService.CreateTuningConfig(clusterID, tuningConfig1Name, fmt.Sprintf(tuningConfigPayload, tuningConfig1Name, tuningConfig1Name))
		o.Expect(err).ToNot(o.HaveOccurred())
		_, err = tuningConfigService.CreateTuningConfig(clusterID, tuningConfig2Name, fmt.Sprintf(tuningConfigPayload, tuningConfig2Name, tuningConfig2Name))
		o.Expect(err).ToNot(o.HaveOccurred())
		_, err = tuningConfigService.CreateTuningConfig(clusterID, tuningConfig3Name, fmt.Sprintf(tuningConfigPayload, tuningConfig3Name, tuningConfig3Name))
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Create nodepool with tuning configs")
		_, err = machinePoolService.CreateMachinePool(clusterID, nodePoolName,
			"--replicas", "3",
			"--tuning-configs", strings.Join(allTuningConfigNames, ","),
		)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Describe nodepool")
		np, err := machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(len(rosacli.ParseTuningConfigs(np.TuningConfigs))).To(o.Equal(3))
		o.Expect(rosacli.ParseTuningConfigs(np.TuningConfigs)).To(o.ContainElements(allTuningConfigNames))

		g.By("Update nodepool with only one tuning config")
		_, err = machinePoolService.EditMachinePool(clusterID, nodePoolName,
			"--tuning-configs", tuningConfig1Name,
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		np, err = machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(len(rosacli.ParseTuningConfigs(np.TuningConfigs))).To(o.Equal(1))
		o.Expect(rosacli.ParseTuningConfigs(np.TuningConfigs)).To(o.ContainElements([]string{tuningConfig1Name}))

		g.By("Update nodepool with no tuning config")
		_, err = machinePoolService.EditMachinePool(clusterID, nodePoolName,
			"--tuning-configs", "",
		)
		o.Expect(err).ToNot(o.HaveOccurred())
		np, err = machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(len(rosacli.ParseTuningConfigs(np.TuningConfigs))).To(o.Equal(0))
	})

	g.It("Author:tradisso-High-61138-rosacli Support 'version' parameter on nodepools in hosted clusters [Serial]", func() {
		nodePoolName := rosacli.GenerateRandomName("np-61138", 2)

		g.By("Get previous version")
		clusterVersionInfo, err := clusterService.GetClusterVersion(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		clusterVersion := clusterVersionInfo.RawID
		clusterChannelGroup := clusterVersionInfo.ChannelGroup
		versionList, err := versionService.ListAndReflectVersions(clusterChannelGroup, true)
		o.Expect(err).ToNot(o.HaveOccurred())

		previousVersionsList, err := versionList.FilterVersionsLowerThan(clusterVersion)
		o.Expect(err).ToNot(o.HaveOccurred())
		if previousVersionsList.Len() <= 1 {
			g.Skip("Skipping as no previous version is available for testing")
		}
		previousVersionsList.Sort(true)
		previousVersion := previousVersionsList.OpenShiftVersions[0].Version

		g.By("Check create nodepool version help parameter")
		help, err := machinePoolService.RetrieveHelpForCreate()
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(help.String()).To(o.ContainSubstring("--version"))

		g.By("Check version is displayed in list")
		nps, err := machinePoolService.ListAndReflectNodePools(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		for _, np := range nps.NodePools {
			o.Expect(np.Version).To(o.Not(o.BeEmpty()))
		}

		g.By("Create NP with previous version")
		_, err = machinePoolService.CreateMachinePool(clusterID, nodePoolName,
			"--replicas", defaultNodePoolReplicas,
			"--version", previousVersion,
		)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check NodePool was correctly created")
		np, err := machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
		o.Expect(err).ToNot(o.HaveOccurred())
		o.Expect(np.Version).To(o.Equal(previousVersion))

		g.By("Wait for NodePool replicas to be available")
		err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 20*time.Minute, false, func(context.Context) (bool, error) {
			npDesc, err := machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
			if err != nil {
				return false, err
			}
			return npDesc.CurrentReplicas == defaultNodePoolReplicas, nil
		})
		exutil.AssertWaitPollNoErr(err, "Replicas are not ready after 600")

		nodePoolVersion, err := versionList.FindNearestBackwardMinorVersion(clusterVersion, 1, true)
		o.Expect(err).ToNot(o.HaveOccurred())
		if nodePoolVersion != nil {
			g.By("Create NodePool with version minor - 1")
			nodePoolName = rosacli.GenerateRandomName("np-61138-m1", 2)
			_, err = machinePoolService.CreateMachinePool(clusterID,
				nodePoolName,
				"--replicas", defaultNodePoolReplicas,
				"--version", nodePoolVersion.Version,
			)
			o.Expect(err).ToNot(o.HaveOccurred())
			np, err = machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(np.Version).To(o.Equal(nodePoolVersion.Version))
		}

		nodePoolVersion, err = versionList.FindNearestBackwardMinorVersion(clusterVersion, 2, true)
		o.Expect(err).ToNot(o.HaveOccurred())
		if nodePoolVersion != nil {
			g.By("Create NodePool with version minor - 2")
			nodePoolName = rosacli.GenerateRandomName("np-61138-m1", 2)
			_, err = machinePoolService.CreateMachinePool(clusterID,
				nodePoolName,
				"--replicas", defaultNodePoolReplicas,
				"--version", nodePoolVersion.Version,
			)
			o.Expect(err).ToNot(o.HaveOccurred())
			np, err = machinePoolService.DescribeAndReflectNodePool(clusterID, nodePoolName)
			o.Expect(err).ToNot(o.HaveOccurred())
			o.Expect(np.Version).To(o.Equal(nodePoolVersion.Version))
		}

		nodePoolVersion, err = versionList.FindNearestBackwardMinorVersion(clusterVersion, 3, true)
		o.Expect(err).ToNot(o.HaveOccurred())
		if nodePoolVersion != nil {
			g.By("Create NodePool with version minor - 3 should fail")
			_, err = machinePoolService.CreateMachinePool(clusterID,
				rosacli.GenerateRandomName("np-61138-m3", 2),
				"--replicas", defaultNodePoolReplicas,
				"--version", nodePoolVersion.Version,
			)
			o.Expect(err).To(o.HaveOccurred())
		}
	})

	g.It("Author:tradisso-Medium-61139-rosacli Validation of Version parameter should work well for nodepool creation/editing	 [Serial]", func() {
		testVersionFailFunc := func(flags ...string) {
			logext.Infof("Creating nodepool with flags %v", flags)
			output, err := machinePoolService.CreateMachinePool(clusterID, rosacli.GenerateRandomName("np-61139", 2), flags...)
			o.Expect(err).To(o.HaveOccurred())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring(`ERR: Expected a valid OpenShift version: A valid version number must be specified`))
			textData = rosaClient.Parser.TextData.Input(output).Parse().Output()
			o.Expect(textData).Should(o.ContainSubstring(`Valid versions:`))
		}

		g.By("Get cluster version")
		clusterVersionInfo, err := clusterService.GetClusterVersion(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		clusterVersion := clusterVersionInfo.RawID
		clusterChannelGroup := clusterVersionInfo.ChannelGroup
		clusterSemVer, err := semver.NewVersion(clusterVersion)
		o.Expect(err).ToNot(o.HaveOccurred())
		clusterVersionList, err := versionService.ListAndReflectVersions(clusterChannelGroup, true)
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Create a nodepool with version greater than cluster's version should fail")
		testVersion := fmt.Sprintf("%d.%d.%d", clusterSemVer.Major()+100, clusterSemVer.Minor()+100, clusterSemVer.Patch()+100)
		testVersionFailFunc("--replicas",
			defaultNodePoolReplicas,
			"--version",
			testVersion)

		if clusterChannelGroup != rosacli.VersionChannelGroupNightly {
			versionList, err := versionService.ListAndReflectVersions(rosacli.VersionChannelGroupNightly, true)
			o.Expect(err).ToNot(o.HaveOccurred())
			lowerVersionsList, err := versionList.FilterVersionsLowerThan(clusterVersion)
			o.Expect(err).ToNot(o.HaveOccurred())
			if lowerVersionsList.Len() > 0 {
				g.By("Create a nodepool with version from incompatible channel group should fail")
				lowerVersionsList.Sort(true)
				testVersion := lowerVersionsList.OpenShiftVersions[0].Version
				testVersionFailFunc("--replicas",
					defaultNodePoolReplicas,
					"--version",
					testVersion)
			}
		}

		g.By("Create a nodepool with major different from cluster's version should fail")
		testVersion = fmt.Sprintf("%d.%d.%d", clusterSemVer.Major()-1, clusterSemVer.Minor(), clusterSemVer.Patch())
		testVersionFailFunc("--replicas",
			defaultNodePoolReplicas,
			"--version",
			testVersion)

		foundVersion, err := clusterVersionList.FindNearestBackwardMinorVersion(clusterVersion, 3, false)
		o.Expect(err).ToNot(o.HaveOccurred())
		if foundVersion != nil {
			g.By("Create a nodepool with minor lower than cluster's 'minor - 3' should fail")
			testVersion = foundVersion.Version
			testVersionFailFunc("--replicas",
				defaultNodePoolReplicas,
				"--version",
				testVersion)
		}

		g.By("Create a nodepool with non existing version should fail")
		testVersion = "24512.5632.85"
		testVersionFailFunc("--replicas",
			defaultNodePoolReplicas,
			"--version",
			testVersion)

		lowerVersionsList, err := clusterVersionList.FilterVersionsLowerThan(clusterVersion)
		o.Expect(err).ToNot(o.HaveOccurred())
		if lowerVersionsList.Len() > 0 {
			g.By("Edit nodepool version should fail")
			nodePoolName := rosacli.GenerateRandomName("np-61139", 2)
			lowerVersionsList.Sort(true)
			testVersion := lowerVersionsList.OpenShiftVersions[0].Version
			_, err := machinePoolService.CreateMachinePool(clusterID, nodePoolName,
				"--replicas",
				defaultNodePoolReplicas,
				"--version",
				testVersion)
			o.Expect(err).ToNot(o.HaveOccurred())

			output, err := machinePoolService.EditMachinePool(clusterID, nodePoolName, "--version", clusterVersion)
			o.Expect(err).To(o.HaveOccurred())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring(`ERR: Editing versions is not supported, for upgrades please use 'rosa upgrade machinepool'`))
		}
	})
})
