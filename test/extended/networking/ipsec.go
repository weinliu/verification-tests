package networking

import (
	"context"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-ipsec", exutil.KubeConfigPath())

	// author: rbrattai@redhat.com
	g.It("Author:rbrattai-High-66652-Verify IPsec encapsulation is enabled for NAT-T", func() {
		// Epic https://issues.redhat.com/browse/SDN-2629

		platform := checkPlatform(oc)
		if !strings.Contains(platform, "ibmcloud") {
			g.Skip("Skip for un-expected platform,not IBMCloud!")
		}

		ns := "openshift-ovn-kubernetes"
		exutil.By("Checking ipsec_encapsulation in ovnkube-node pods")

		podList, podListErr := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=ovnkube-node",
		})
		o.Expect(podListErr).NotTo(o.HaveOccurred())

		for _, pod := range podList.Items {
			cmd := "ovn-nbctl --no-leader-only get NB_Global . options"
			e2e.Logf("The command is: %v", cmd)
			command1 := []string{"-n", ns, "-c", "nbdb", pod.Name, "--", "bash", "-c", cmd}
			out, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(command1...).Output()
			if err != nil {
				e2e.Logf("Execute command failed with  err:%v  and output is %v.", err, out)
			}
			o.Expect(err).NotTo(o.HaveOccurred())

			o.Expect(out).To(o.ContainSubstring(`ipsec_encapsulation="true"`))
		}

	})
})
