package clusterinfrastructure

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure MAPI", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("mapi-operator", exutil.KubeConfigPath())
	)
	// author: miyadav@redhat.com
	g.It("Author:miyadav-NonHyperShiftHOST-Medium-46078-Signal when mao no-op in the clusterOperator status conditions", func() {
		g.By("watch the message from machine-api(mapi) clusteroperator ")
		if clusterinfra.CheckPlatform(oc) == clusterinfra.None {
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "machine-api", "-o=jsonpath={.status.conditions}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(out).To(o.ContainSubstring("Cluster Machine API Operator is in NoOp mode"))
		} else {
			e2e.Logf("Only baremetal platform supported for the test")
			g.Skip("We have to skip the test")
		}
	})
})
