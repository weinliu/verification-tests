package hypershift

import (
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-hypershift] Hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("hypershift-hosted", exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		if !exutil.IsHypershiftHostedCluster(oc) {
			g.Skip("not a hosted cluster, skip the test case")
		}
	})

	// author: heli@redhat.com
	g.It("NonPreRelease-PreChkUpgrade-PstChkUpgrade-Author:heli-Critical-66831-HCP to support mgmt on 4.13 non-OVNIC and hosted on 4.14 OVN-IC and mgmt to upgrade to 4.14", func() {
		version := doOcpReq(oc, OcpGet, true, "clusterversion", "version", `-ojsonpath={.status.desired.version}`)
		g.By(fmt.Sprintf("check hosted cluster version: %s", version))

		ovnLeaseHolder := doOcpReq(oc, OcpGet, true, "lease", "ovn-kubernetes-master", "-n", "openshift-ovn-kubernetes", `-ojsonpath={.spec.holderIdentity}`)
		g.By(fmt.Sprintf("check hosted cluster ovn lease holder: %s", ovnLeaseHolder))

		if strings.Contains(version, "4.13") {
			// currently we only check aws 4.13
			if strings.ToLower(exutil.CheckPlatform(oc)) == "aws" {
				o.Expect(ovnLeaseHolder).Should(o.ContainSubstring("compute.internal"))
			}
		}

		if strings.Contains(version, "4.14") {
			o.Expect(ovnLeaseHolder).Should(o.ContainSubstring("ovnkube-control-plane"))
		}

	})
})
