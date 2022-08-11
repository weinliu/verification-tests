package clusterinfrastructure

import (
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("machine-proxy-cluster", exutil.KubeConfigPath())
	)

	// author: miyadav@redhat.com
	g.It("Author:miyadav-High-37384-Machine API components should honour cluster wide proxy settings", func() {
		g.By("Check if it's a proxy cluster")
		httpProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		httpsProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec.httpsProxy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(httpProxy) == 0 && len(httpsProxy) == 0 {
			g.Skip("Skip for non-proxy cluster!")
		}
		g.By("Check if machine-controller-pod is using cluster proxy")
		machineControllerPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-machine-api", "-l", "k8s-app=controller", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(machineControllerPod) == 0 {
			e2e.Failf("machine controller pod did not started , cluster might be unstable")
		} else {
			envMapi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", machineControllerPod, "-n", "openshift-machine-api", "-o=jsonpath={.spec.containers[0].env[0].name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(envMapi) == 0 {
				e2e.Failf("jsonpath needs to be reviewed")
			} else if strings.Compare(envMapi, "HTTP_PROXY") != 0 {
				g.By("machine-api does not uses cluster proxy")
				e2e.Failf("For more details refer - BZ 1896704")
			}
		}
	})

})
