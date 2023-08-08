package rosacli

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[sig-rosacli] Service_Development_A region testing", func() {
	var (
		rosaClient         = NewClient()
		ocmResourceService = rosaClient.OCMResource
	)

	g.It("Author:yuwan-High-55729-rosacli List regions via rosacli command [Serial]", func() {

		g.By("List region")
		out, err := ocmResourceService.listRegion()
		o.Expect(err).To(o.BeNil())
		usersTabNonH, err := ocmResourceService.reflectRegionList(out)
		o.Expect(err).To(o.BeNil())
		o.Expect(len(usersTabNonH)).NotTo(o.Equal(0))

		g.By("List region --hosted-cp")
		out, err = ocmResourceService.listRegion("--hosted-cp")
		o.Expect(err).To(o.BeNil())
		usersTabH, err := ocmResourceService.reflectRegionList(out)
		o.Expect(err).To(o.BeNil())
		o.Expect(len(usersTabH)).NotTo(o.Equal(0))

		g.By("Check out of 'rosa list region --hosted-cp' are supported for hosted-cp clusters")
		for _, r := range usersTabH {
			o.Expect(r.MultiAZSupported).To(o.Equal("true"))
		}
	})
})
