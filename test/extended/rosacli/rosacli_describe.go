package rosacli

import (
	"strconv"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A Decribe resources", func() {
	var clusterID string

	g.BeforeEach(func() {
		g.By("Get the cluster")
		clusterID = getClusterIDENVExisted()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")
	})

	g.It("Author:yuwan-Medium-34102-rosacli testing: Check the description of the cluster", func() {
		g.By("Describe cluster in text format")
		var rosaClient = NewClient()
		clusterService := rosaClient.Cluster
		output, err := clusterService.describeCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		CD := clusterService.reflectClusterDescription(output)

		g.By("Describe cluster in json format")
		rosaClient.Runner.format = "json"
		jsonOutput, err := clusterService.describeCluster(clusterID)
		o.Expect(err).To(o.BeNil())
		rosaClient.Runner.CloseFormat()
		jsonData := rosaClient.Parser.jsonData.Input(jsonOutput).Parse()

		g.By("Compare the text result with the json result")
		o.Expect(CD.ID).To(o.Equal(jsonData.digString("id")))
		o.Expect(CD.ExternalID).To(o.Equal(jsonData.digString("external_id")))
		o.Expect(CD.ChannelGroup).To(o.Equal(jsonData.digString("version", "channel_group")))
		o.Expect(CD.DNS).To(o.Equal(jsonData.digString("name") + "." + jsonData.digString("dns", "base_domain")))
		o.Expect(CD.AWSAccount).NotTo(o.BeEmpty())
		o.Expect(CD.APIURL).To(o.Equal(jsonData.digString("api", "url")))
		o.Expect(CD.ConsoleURL).To(o.Equal(jsonData.digString("console", "url")))
		o.Expect(CD.Region).To(o.Equal(jsonData.digString("region", "id")))
		o.Expect(CD.MultiAZ).To(o.Equal(strconv.FormatBool(jsonData.digBool("multi_az"))))
		o.Expect(CD.State).To(o.Equal(jsonData.digString("status", "state")))
		o.Expect(CD.Created).NotTo(o.BeEmpty())
		o.Expect(CD.DetailsPage).NotTo(o.BeEmpty())

		if jsonData.digBool("aws", "private_link") {
			o.Expect(CD.Private).To(o.Equal("Yes"))
		} else {
			o.Expect(CD.Private).To(o.Equal("No"))
		}

		if jsonData.digBool("hypershift", "enabled") {
			//todo
		} else {
			if jsonData.digBool("multi_az") {
				//todo
			} else {
				o.Expect(CD.Nodes[0]["Control plane"]).To(o.Equal(strconv.FormatFloat(jsonData.digFloat("nodes", "master"), 'f', -1, 64)))
				o.Expect(CD.Nodes[1]["Infra"]).To(o.Equal(strconv.FormatFloat(jsonData.digFloat("nodes", "infra"), 'f', -1, 64)))
				o.Expect(CD.Nodes[2]["Compute"]).To(o.Equal(strconv.FormatFloat(jsonData.digFloat("nodes", "compute"), 'f', -1, 64)))
			}
		}

		o.Expect(CD.Network[1]["Service CIDR"]).To(o.Equal(jsonData.digString("network", "service_cidr")))
		o.Expect(CD.Network[2]["Machine CIDR"]).To(o.Equal(jsonData.digString("network", "machine_cidr")))
		o.Expect(CD.Network[3]["Pod CIDR"]).To(o.Equal(jsonData.digString("network", "pod_cidr")))
		o.Expect(CD.Network[4]["Host Prefix"]).Should(o.ContainSubstring(strconv.FormatFloat(jsonData.digFloat("network", "host_prefix"), 'f', -1, 64)))
		o.Expect(CD.InfraID).To(o.Equal(jsonData.digString("infra_id")))
	})
})
