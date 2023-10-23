package rosacli

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A oidc provider test", func() {
	defer g.GinkgoRecover()

	var (
		clusterID string
	)

	g.BeforeEach(func() {
		g.By("Get the cluster id")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

	})

	g.It("Author:yuwan-High-43046-Validation will work when user create oidc-provider to cluster [Serial]", func() {
		g.By("Check if cluster is sts cluster")
		StsCluster, err := isSTSCluster(clusterID)
		o.Expect(err).To(o.BeNil())

		g.By("Check if cluster is using reusable oidc config")
		UsingReusableOIDCConfig, err := isUsingReusableOIDCConfig(clusterID)
		o.Expect(err).To(o.BeNil())

		notExistedClusterID := "notexistedclusterid111"
		rosaClient := rosacli.NewClient()
		ocmResourceService := rosaClient.OCMResource

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
				o.Expect(strings.Contains(textData, "OIDC provider already exists")).Should(o.BeTrue())
			} else {
				o.Expect(strings.Contains(textData, "is ready and does not need additional configuration")).Should(o.BeTrue())
			}
		case false:
			g.By("Create oidc-provider on classic non-sts cluster")
			output, err := ocmResourceService.CreateOIDCProvider(
				"--mode", "auto",
				"-c", clusterID,
				"-y")
			o.Expect(err).NotTo(o.BeNil())
			textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
			o.Expect(strings.Contains(textData, "is not an STS cluster")).Should(o.BeTrue())
		}
		g.By("Create oidc-provider on not-existed cluster")
		output, err := ocmResourceService.CreateOIDCProvider(
			"--mode", "auto",
			"-c", notExistedClusterID,
			"-y")
		o.Expect(err).NotTo(o.BeNil())
		textData := rosaClient.Parser.TextData.Input(output).Parse().Tip()
		o.Expect(strings.Contains(textData, "There is no cluster with identifier or name")).Should(o.BeTrue())
	})
})
