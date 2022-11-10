package clusterinfrastructure

import (
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("cluster-api-operator", exutil.KubeConfigPath())
	)

	// author: zhsun@redhat.com
	g.It("Longduration-NonPreRelease-Author:zhsun-High-51061-Enable cluster API with feature gate [Disruptive]", func() {
		g.By("Check if cluster api on this platform is supported")
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "vsphere")
		g.By("Check if cluster api is deployed, if no, enable it")
		capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(capi) == 0 {
			g.By("Enable cluster api with feature gate")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate/cluster", "-p", `{"spec":{"featureSet": "TechPreviewNoUpgrade"}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Check cluster is still healthy")
			waitForClusterHealthy(oc)
		}

		g.By("Check if cluster api is deployed")
		// Need to give more time in case cluster is private , we have seen it takes time , even after cluster becomes healthy , this happens only when publicZone is not present
		publicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
		if err != nil {
			g.Fail("Issue with dns setup")
		}
		g.By("if publicZone is {id:qe} or any other value it implies not a private set up, so no need to wait")
		if publicZone == "" {
			time.Sleep(360)
		}

		capi, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(capi).To(o.ContainSubstring("cluster-capi-operator"))

		g.By("Check if machine approver is deployed")
		approver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", machineApproverNamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(approver).To(o.ContainSubstring("machine-approver-capi"))

		g.By("Check if providers are deployed ")
		providers, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("provider", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(providers).To(o.ContainSubstring("cluster-api infrastructure"))

		g.By("Check user data secret is copied from openshift-machine-api namespace to openshift-cluster-api")
		secret, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secret).To(o.ContainSubstring("worker-user-data"))
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-medium-51088-[CAPI] Prevent users from deleting providers [Disruptive]", func() {
		g.By("Check if cluster api on this platform is supported")
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "vsphere")
		g.By("Check if cluster api is deployed")
		capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(capi) == 0 {
			g.Skip("Skip for cluster api is not deployed!")
		}

		g.By("Delete coreprovider and infrastructureprovider")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("coreprovider", "cluster-api", "-n", clusterAPINamespace).Execute()
		o.Expect(err).To(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("infrastructureprovider", "--all", "-n", clusterAPINamespace).Execute()
		o.Expect(err).To(o.HaveOccurred())

	})
})
