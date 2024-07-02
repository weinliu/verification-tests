package clusterinfrastructure

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure MAPI", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())
		iaasPlatform clusterinfra.PlatformType
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = clusterinfra.CheckPlatform(oc)
	})
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Critical-29147-Check that all the baremetalhosts are up and running", func() {
		g.By("Check if baremetal cluster")
		if !(iaasPlatform == clusterinfra.BareMetal) {
			e2e.Logf("Cluster is: %s", iaasPlatform.String())
			g.Skip("For Non-baremetal cluster , this is not supported!")
		}
		g.By("Check if baremetal hosts are up and running")
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "--all-namespaces", "-o=jsonpath={.items[*].status.poweredOn}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(status, "false") {
			g.By("Issue with bmh provisioning please review")
			e2e.Failf("baremetal hosts not provisioned properly")
		}
	})
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Critical-32198-Verify all master bmh are 'externally provisioned'", func() {
		g.By("Check if baremetal cluster")
		if !(iaasPlatform == clusterinfra.BareMetal) {
			e2e.Logf("Cluster is: %s", iaasPlatform.String())
			g.Skip("For Non-baremetal cluster , this is not supported!")
		}
		g.By("Verify all master bmh are 'externally provisioned'")
		bmhNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, bmhMastersWorkers := range strings.Fields(bmhNames) {
			if strings.Contains(bmhMastersWorkers, "master") {
				bmhMasters := bmhMastersWorkers
				g.By("Check if master bmh is externally provisioned")
				state, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", bmhMasters, "-n", machineAPINamespace, "-o=jsonpath={.spec.externallyProvisioned}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if state != "true" {
					e2e.Failf("baremetal master not provisioned externally")
				}
			}
		}
	})
})
