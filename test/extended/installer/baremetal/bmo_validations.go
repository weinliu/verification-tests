package baremetal

import (
	"fmt"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-baremetal] INSTALLER IPI on BareMetal", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("cluster-baremetal-operator", exutil.KubeConfigPath())
		iaasPlatform string
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		if !(iaasPlatform == "baremetal") {
			e2e.Logf("Cluster is: %s", iaasPlatform)
			g.Skip("For Non-baremetal cluster , this is not supported!")
		}
	})
	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-66490-Allow modification of BMC address after installation [Disruptive]", func() {
		g.By("Running oc patch bmh -n openshift-machine-api master-00")
		bmhName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		bmcAddressOrig, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[2].spec.bmc.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchConfig := `[{"op": "replace", "path": "/spec/bmc/address", "value":"redfish-virtualmedia://10.1.234.25/redfish/v1/Systems/System.Embedded.1"}]`
		out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("denied the request: BMC address can not be changed if the BMH is not in the Registering state, or if the BMH is not detached"))

		g.By("Detach the BareMetal host")
		patch := `{"metadata":{"annotations":{"baremetalhost.metal3.io/detached": ""}}}`
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=merge", "-p", patch).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Modify BMC address of BareMetal host")
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			g.By("Revert changes")
			patchConfig = fmt.Sprintf(`[{"op": "replace", "path": "/spec/bmc/address", "value": "%s"}]`, bmcAddressOrig)
			_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			patchConfig = `[{"op": "remove", "path": "/metadata/annotations/baremetalhost.metal3.io~1detached"}]`
			_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		bmcAddress, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[2].spec.bmc.address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(bmcAddress).To(o.ContainSubstring("redfish-virtualmedia://10.1.234.25/redfish/v1/Systems/System.Embedded.1"))

	})
	// author: jhajyahy@redhat.com
	g.It("Author:jhajyahy-Medium-66491-bootMACAddress can't be changed once set [Disruptive]", func() {
		g.By("Running oc patch bmh -n openshift-machine-api master-00")
		bmhName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("baremetalhosts", "-n", machineAPINamespace, "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchConfig := `[{"op": "replace", "path": "/spec/bootMACAddress", "value":"f4:02:70:b8:d8:ff"}]`
		out, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", machineAPINamespace, bmhName, "--type=json", "-p", patchConfig).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("bootMACAddress can not be changed once it is set"))

	})
})
