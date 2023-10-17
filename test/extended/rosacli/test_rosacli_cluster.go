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
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		clusterService = rosaClient.Cluster
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
})
