package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service oidc provider test", func() {
	defer g.GinkgoRecover()

	var (
		clusterID          string
		rosaClient         *rosacli.Client
		clusterService     rosacli.ClusterService
		ocmResourceService rosacli.OCMResourceService
	)

	g.BeforeEach(func() {
		g.By("Get the cluster id")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		clusterService = rosaClient.Cluster
		ocmResourceService = rosaClient.OCMResource
	})

	g.AfterEach(func() {
		g.By("Clean remaining resources")
		err := rosaClient.CleanResources(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.It("Author:yuwan-High-43046-Validation will work when user create oidc-provider to cluster [Serial]", func() {
		g.By("Check if cluster is sts cluster")
		StsCluster, err := clusterService.IsSTSCluster(clusterID)
		o.Expect(err).To(o.BeNil())

		g.By("Check if cluster is using reusable oidc config")
		UsingReusableOIDCConfig, err := clusterService.IsUsingReusableOIDCConfig(clusterID)
		o.Expect(err).To(o.BeNil())

		notExistedClusterID := "notexistedclusterid111"

		switch StsCluster {
		case true:
			g.By("Create oidc-provider on sts cluster which status is not pending")
			output, err := ocmResourceService.CreateOIDCProvider(
				"--mode", "auto",
				"-c", clusterID,
				"-y")
			o.Expect(err).To(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			if UsingReusableOIDCConfig {
				o.Expect(textData).To(o.ContainSubstring("OIDC provider already exists"))
			} else {
				o.Expect(textData).To(o.ContainSubstring("is ready and does not need additional configuration"))
			}
		case false:
			g.By("Create oidc-provider on classic non-sts cluster")
			output, err := ocmResourceService.CreateOIDCProvider(
				"--mode", "auto",
				"-c", clusterID,
				"-y")
			o.Expect(err).NotTo(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(textData).To(o.ContainSubstring("is not an STS cluster"))
		}
		g.By("Create oidc-provider on not-existed cluster")
		output, err := ocmResourceService.CreateOIDCProvider(
			"--mode", "auto",
			"-c", notExistedClusterID,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(textData).To(o.ContainSubstring("There is no cluster with identifier or name"))
	})
})
