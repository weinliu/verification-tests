package clusterinfrastructure

import (
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"time"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cluster-api-operator", exutil.KubeConfigPath())
		iaasPlatform string
	)

	g.BeforeEach(func() {
		iaasPlatform = exutil.CheckPlatform(oc)
	})

	// author: zhsun@redhat.com
	g.It("Longduration-NonPreRelease-Author:zhsun-High-51061-Enable cluster API with feature gate [Disruptive]", func() {
		g.By("Check if cluster api on this platform is supported")
		if !(iaasPlatform == "aws" || iaasPlatform == "azure" || iaasPlatform == "gcp" || iaasPlatform == "vsphere") {
			g.Skip("Skip for cluster api on this platform is not supported or don't need to enable!")
		}
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
	g.It("NonPreRelease-Author:zhsun-medium-51088-Providers can be recreated by operator [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		if !(iaasPlatform == "aws" || iaasPlatform == "azure" || iaasPlatform == "gcp" || iaasPlatform == "vsphere") {
			g.Skip("Skip for cluster api on this platform is not supported or don't need to enable!")
		}
		g.By("Check if cluster api is deployed")
		capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(capi) == 0 {
			g.Skip("Skip for cluster api is not deployed!")
		}

		g.By("Delete coreprovider and infrastructureprovider")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("coreprovider", "cluster-api", "-n", clusterAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("infrastructureprovider", "--all", "-n", clusterAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check coreprovider and infrastructureprovider will be recreated by cluster-api-operator")
		err = wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
			coreprovider, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("coreprovider", "-n", clusterAPINamespace).Output()
			infrastructureProvider, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("InfrastructureProvider", "-n", clusterAPINamespace).Output()
			if len(coreprovider) == 0 || len(infrastructureProvider) == 0 {
				e2e.Logf("Providers are not recreated by cluster-api-operator")
				return false, nil
			}
			e2e.Logf("Providers are recreated by cluster-api-operator")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Providers are not recreated by cluster-api-operator in 5m")
	})
})
