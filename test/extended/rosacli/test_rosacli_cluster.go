package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A Edit cluster", func() {
	defer g.GinkgoRecover()

	var (
		clusterID      string
		rosaClient     *rosacli.Client
		clusterService rosacli.ClusterService
		clusterConfig  *ClusterConfig
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		clusterService = rosaClient.Cluster

		g.By("Load the original cluster config")
		var err error
		clusterConfig, err = parseProfile(getClusterConfigFile())
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.It("Author:yuwan-High-38850-Restrict master API endpoint to direct, private connectivity or not [Serial]", func() {
		g.By("Check the cluster is not private cluster")
		private, err := isPrivateCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		if private {
			g.Skip("This case needs to test on private cluster as the prerequirement,it was not fullfilled, skip the case!!")
		}
		g.By("Edit cluster to private to true")
		out, err := clusterService.EditCluster(
			clusterID,
			"--private",
			"-y",
		)
		o.Expect(err).To(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(out).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("You are choosing to make your cluster API private. You will not be able to access your cluster"))
		o.Expect(textData).Should(o.ContainSubstring("Updated cluster '%s'", clusterID))

		defer func() {
			g.By("Edit cluster to private back to false")
			out, err = clusterService.EditCluster(
				clusterID,
				"--private=false",
				"-y",
			)
			o.Expect(err).To(o.BeNil())
			textData = rosaClient.Parser.TextData.Input(out).Parse().Tip()
			o.Expect(textData).Should(o.ContainSubstring("Updated cluster '%s'", clusterID))

			g.By("Describe cluster to check Private is true")
			output, err := clusterService.DescribeCluster(clusterID)
			o.Expect(err).To(o.BeNil())
			CD := clusterService.ReflectClusterDescription(output)
			o.Expect(CD.Private).To(o.Equal("No"))
		}()

		g.By("Describe cluster to check Private is true")
		output, err := clusterService.DescribeCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		CD := clusterService.ReflectClusterDescription(output)
		o.Expect(CD.Private).To(o.Equal("Yes"))

	})

	// OCM-5231 caused the description parser issue
	g.It("Author:xueli-High-45159-User can disable workload monitoring on/off via rosa-cli [Serial]", func() {
		g.By("Check the cluster UWM is in expected status")
		output, err := clusterService.DescribeCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		clusterDetail := clusterService.ReflectClusterDescription(output)
		expectedUWMValue := "Enabled"
		if clusterConfig.DisableWorkloadMonitoring {
			expectedUWMValue = "Disabled"
		}
		o.Expect(clusterDetail.UserWorkloadMonitoring).To(o.Equal(expectedUWMValue))

		g.By("Disable the UWM")
		expectedUWMValue = "Disabled"
		_, err = clusterService.EditCluster(clusterID,
			"--disable-workload-monitoring",
			"-y")
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the disable result for cluster description")
		output, err = clusterService.DescribeCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		clusterDetail = clusterService.ReflectClusterDescription(output)
		o.Expect(clusterDetail.UserWorkloadMonitoring).To(o.Equal(expectedUWMValue))

		g.By("Enable the UWM again")
		expectedUWMValue = "Enabled"
		_, err = clusterService.EditCluster(clusterID,
			"--disable-workload-monitoring=false",
			"-y")
		o.Expect(err).ToNot(o.HaveOccurred())

		g.By("Check the disable result for cluster description")
		output, err = clusterService.DescribeCluster(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())

		clusterDetail = clusterService.ReflectClusterDescription(output)
		o.Expect(clusterDetail.UserWorkloadMonitoring).To(o.Equal(expectedUWMValue))
	})

	g.It("Author:mgahagan-Medium-38787-Validation for deletion of upgrade policy of rosa cluster via rosa-cli [Serial]", func() {
		g.By("Validate that deletion of upgrade policy for rosa cluster will work via rosacli")
		output, err := clusterService.DeleteUpgrade("")
		o.Expect(err).To(o.HaveOccurred())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring(`required flag(s) "cluster" not set`))

		g.By("Delete an non-existant upgrade when cluster has no scheduled policy")
		output, err = clusterService.DeleteUpgrade("-c", clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring(`There are no scheduled upgrades on cluster '%s'`, clusterID))

		g.By("Delete with unknown flag --interactive")
		output, err = clusterService.DeleteUpgrade("-c", clusterID, "--interactive")
		o.Expect(err).To(o.HaveOccurred())
		textData = rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).Should(o.ContainSubstring("Error: unknown flag: --interactive"))

	})
})
