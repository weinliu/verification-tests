package networking

import (
	"fmt"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-multus", exutil.KubeConfigPath())

	// author: weliang@redhat.com
	g.It("Author:weliang-Medium-46387-[BZ 1896533] network operator degraded due to additionalNetwork in non-existent namespace. [Disruptive]", func() {
		var (
			patchSResource = "networks.operator.openshift.io/cluster"
			patchInfo      = fmt.Sprintf("{\"spec\":{\"additionalNetworks\": [{\"name\": \"secondary\",\"namespace\":\"ocp-46387\",\"simpleMacvlanConfig\": {\"ipamConfig\": {\"staticIPAMConfig\": {\"addresses\": [{\"address\": \"10.1.1.0/24\"}] },\"type\": \"static\"}},\"type\": \"SimpleMacvlan\"}]}}")
		)

		g.By("create new namespace")
		namespace := fmt.Sprintf("ocp-46387")
		err := oc.Run("new-project").Args(namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()

		g.By("Configure network-attach-definition through network operator")
		patchResourceAsAdmin(oc, patchSResource, patchInfo)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()

		//Testing will exit when network operator is in abnormal state during 60 seconding of checking operator.
		g.By("Check NetworkOperatorStatus")
		checkNetworkOperatorState(oc, 10, 60)

		g.By("Delete the namespace")
		nsErr := oc.AsAdmin().Run("delete").Args("project", namespace, "--ignore-not-found").Execute()
		o.Expect(nsErr).NotTo(o.HaveOccurred())

		//Testing will exit when network operator is in abnormal state during 60 seconding of checking operator.
		g.By("Check NetworkOperatorStatus after deleting namespace")
		checkNetworkOperatorState(oc, 10, 60)
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-High-59440-Whereabouts ip-reconciler should be opt-in and not required. [Disruptive]", func() {
		//Bug: https://bugzilla.redhat.com/show_bug.cgi?id=2101918
		var (
			patchSResource = "networks.operator.openshift.io/cluster"
			patchInfo      = fmt.Sprintf(`{"spec":{ "additionalNetworks": [{"name": "whereabouts-shim", "namespace": "default","rawCNIConfig":"{\"cniVersion\":\"0.3.0\",\"type\":\"bridge\",\"name\":\"cnitest0\",\"ipam\": {\"type\":\"whereabouts\",\"subnet\":\"192.0.2.0/24\"}}","type":"Raw"}]}}`)
			stringInfo1    = "No resources found in openshift-multus namespace"
			stringInfo2    = "ip-reconciler"
		)

		g.By("Check the cronjobs in the openshift-multus namespace")
		cronjobLog1 := getMultusCronJob(oc)
		o.Expect(cronjobLog1).To(o.ContainSubstring(stringInfo1))

		g.By("Add additionalNetworks through network operator")
		patchResourceAsAdmin(oc, patchSResource, patchInfo)
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()

		g.By("Check the cronjobs in the openshift-multus namespace")
		o.Eventually(func() string {
			cronjobLog2 := getMultusCronJob(oc)
			return cronjobLog2
		}, "90s", "10s").Should(o.ContainSubstring(stringInfo2), fmt.Sprintf("Failed to get correct multus cronjobs"))

		g.By("Delete additionalNetworks through network operator")
		oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", `[{"op": "remove", "path": "/spec/additionalNetworks"}]`, "--type=json").Execute()

		g.By("Check the cronjobs in the openshift-multus namespace")
		o.Eventually(func() string {
			cronjobLog3 := getMultusCronJob(oc)
			return cronjobLog3
		}, "90s", "10s").Should(o.ContainSubstring(stringInfo1), fmt.Sprintf("Failed to get correct multus cronjobs"))
	})
})
