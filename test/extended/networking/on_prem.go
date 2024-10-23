package networking

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-networking] SDN on-prem", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("networking-cno", exutil.KubeConfigPath())
	)

	//author: zzhao@redhat.com
	g.It("Author:zzhao-Medium-77042-Add annotation in the on-prem namespace static pods for workload partitioning", func() {
		// Skip this case for un-supported platform

		g.By("Check platforms")
		platformtype := exutil.CheckPlatform(oc)
		nsForPlatforms := map[string]string{
			"baremetal": "openshift-kni-infra",
			"vsphere":   "openshift-vsphere-infra",
			"nutanix":   "openshift-nutanix-infra",
		}
		ns := nsForPlatforms[platformtype]
		if ns == "" {
			g.Skip("Skip for non-supported platform")
		}
		appLabel := strings.Replace(ns, "openshift-", "", -1)
		lbappLable := appLabel + "-api-lb"
		dnsappLable := appLabel + "-coredns"
		kaappLabel := appLabel + "-vrrp"

		allLabels := []string{lbappLable, dnsappLable, kaappLabel}

		exutil.By("check all pods annotation")
		for _, label := range allLabels {
			podNames, error := oc.WithoutNamespace().AsAdmin().Run("get").Args("po", "-n", ns, "-l=app="+label, `-ojsonpath={.items[?(@.status.phase=="Running")].metadata.name}`).Output()
			o.Expect(error).NotTo(o.HaveOccurred())
			o.Expect(podNames).NotTo(o.BeEmpty())
			podName := strings.Fields(podNames)
			// Check if workload partioning annotation is added
			podAnnotation, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("po", "-n", ns, podName[0], `-ojsonpath={.metadata.annotations}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(podAnnotation).To(o.ContainSubstring(`"target.workload.openshift.io/management":"{\"effect\": \"PreferredDuringScheduling\"}"`))
		}
	})
})
