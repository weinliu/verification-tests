package rosacli

import (
	"strconv"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service Decribe resources", func() {
	defer g.GinkgoRecover()

	var (
		clusterID      string
		rosaClient     *rosacli.Client
		clusterService rosacli.ClusterService
	)

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		clusterService = rosaClient.Cluster
	})

	g.AfterEach(func() {
		g.By("Clean remaining resources")
		err := rosaClient.CleanResources(clusterID)
		o.Expect(err).ToNot(o.HaveOccurred())
	})

	g.It("Author:yuwan-Medium-34102-rosacli testing: Check the description of the cluster [Serial]", func() {
		g.By("Describe cluster in text format")
		output, err := clusterService.DescribeCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		CD, err := clusterService.ReflectClusterDescription(output)
		o.Expect(err).To(o.BeNil())

		g.By("Describe cluster in json format")
		rosaClient.Runner.JsonFormat()
		jsonOutput, err := clusterService.DescribeCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		rosaClient.Runner.UnsetFormat()
		jsonData := rosaClient.Parser.JsonData.Input(jsonOutput).Parse()

		g.By("Compare the text result with the json result")
		o.Expect(CD.ID).To(o.Equal(jsonData.DigString("id")))
		o.Expect(CD.ExternalID).To(o.Equal(jsonData.DigString("external_id")))
		o.Expect(CD.ChannelGroup).To(o.Equal(jsonData.DigString("version", "channel_group")))
		o.Expect(CD.DNS).To(o.Equal(jsonData.DigString("name") + "." + jsonData.DigString("dns", "base_domain")))
		o.Expect(CD.AWSAccount).NotTo(o.BeEmpty())
		o.Expect(CD.APIURL).To(o.Equal(jsonData.DigString("api", "url")))
		o.Expect(CD.ConsoleURL).To(o.Equal(jsonData.DigString("console", "url")))
		o.Expect(CD.Region).To(o.Equal(jsonData.DigString("region", "id")))
		o.Expect(CD.Ec2MetadataHttpTokens).To(o.Equal(jsonData.DigString("ec2_metadata_http_tokens")))

		o.Expect(CD.State).To(o.Equal(jsonData.DigString("status", "state")))
		o.Expect(CD.Created).NotTo(o.BeEmpty())
		o.Expect(CD.DetailsPage).NotTo(o.BeEmpty())

		if jsonData.DigBool("aws", "private_link") {
			o.Expect(CD.Private).To(o.Equal("Yes"))
		} else {
			o.Expect(CD.Private).To(o.Equal("No"))
		}

		if jsonData.DigBool("hypershift", "enabled") {
			//todo
		} else {
			if jsonData.DigBool("multi_az") {
				o.Expect(CD.MultiAZ).To(o.Equal(strconv.FormatBool(jsonData.DigBool("multi_az"))))
			} else {
				o.Expect(CD.Nodes[0]["Control plane"]).To(o.Equal(int(jsonData.DigFloat("nodes", "master"))))
				o.Expect(CD.Nodes[1]["Infra"]).To(o.Equal(int(jsonData.DigFloat("nodes", "infra"))))
				o.Expect(CD.Nodes[2]["Compute"]).To(o.Equal(int(jsonData.DigFloat("nodes", "compute"))))
			}
		}

		o.Expect(CD.Network[1]["Service CIDR"]).To(o.Equal(jsonData.DigString("network", "service_cidr")))
		o.Expect(CD.Network[2]["Machine CIDR"]).To(o.Equal(jsonData.DigString("network", "machine_cidr")))
		o.Expect(CD.Network[3]["Pod CIDR"]).To(o.Equal(jsonData.DigString("network", "pod_cidr")))
		o.Expect(CD.Network[4]["Host Prefix"]).Should(o.ContainSubstring(strconv.FormatFloat(jsonData.DigFloat("network", "host_prefix"), 'f', -1, 64)))
		o.Expect(CD.InfraID).To(o.Equal(jsonData.DigString("infra_id")))
	})
})
