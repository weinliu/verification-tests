package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

var _ = g.Describe("[sig-rosacli] Cluster_Management_Service region testing", func() {
	defer g.GinkgoRecover()

	var (
		clusterID          string
		rosaClient         *rosacli.Client
		ocmResourceService rosacli.OCMResourceService
	)

	g.BeforeEach(func() {

		g.By("Get the cluster")
		clusterID = rosacli.GetClusterID()
		o.Expect(clusterID).ToNot(o.Equal(""), "ClusterID is required. Please export CLUSTER_ID")

		g.By("Init the client")
		rosaClient = rosacli.NewClient()
		ocmResourceService = rosaClient.OCMResource
	})

	g.It("Author:yuwan-High-55729-rosacli List regions via rosacli command [Serial]", func() {

		g.By("List region")
		usersTabNonH, _, err := ocmResourceService.ListRegion()
		o.Expect(err).To(o.BeNil())
		o.Expect(len(usersTabNonH)).NotTo(o.Equal(0))

		g.By("List region --hosted-cp")
		usersTabH, _, err := ocmResourceService.ListRegion("--hosted-cp")
		o.Expect(err).To(o.BeNil())
		o.Expect(len(usersTabH)).NotTo(o.Equal(0))

		g.By("Check out of 'rosa list region --hosted-cp' are supported for hosted-cp clusters")
		for _, r := range usersTabH {
			o.Expect(r.MultiAZSupported).To(o.Equal("true"))
		}
	})
})
