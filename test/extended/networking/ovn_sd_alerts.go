package networking

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-alerts", exutil.KubeConfigPath())

	g.BeforeEach(func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51438-Upgrade NoRunningOvnControlPlane to critical severity and inclue runbook.", func() {
		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("NoRunningOvnControlPlane"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoRunningOvnControlPlane\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))

		// https://issues.redhat.com/browse/OCPBUGS-18340 is minor bug, not sure when it will be fixed, disable below steps and wait for fix
		/*
			alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoRunningOvnControlPlane\")].annotations.runbook_url}").Output()
			o.Expect(runbookErr).NotTo(o.HaveOccurred())
			e2e.Logf("The alertRunbook is %v", alertRunbook)
			o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/NoRunningOvnControlPlane.md"))
		*/
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51439-Upgrade NoOvnClusterManagerLeader to critical severity and inclue runbook.", func() {
		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("NoOvnClusterManagerLeader"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoOvnClusterManagerLeader\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))

		// https://issues.redhat.com/browse/OCPBUGS-18340 is minor bug, not sure when it will be fixed, disable below steps and wait for fix
		/*
			alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoOvnClusterManagerLeader\")].annotations.runbook_url}").Output()
			o.Expect(runbookErr).NotTo(o.HaveOccurred())
			e2e.Logf("The alertRunbook is %v", alertRunbook)
			o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/NoOvnClusterManagerLeader.md"))
		*/
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51722-Create runbook and link SOP for SouthboundStale alert", func() {
		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("SouthboundStale"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoOvnClusterManagerLeader\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"SouthboundStale\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/SouthboundStaleAlert.md"))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51724-Create runbook and link SOP for V4SubnetAllocationThresholdExceeded alert", func() {
		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("V4SubnetAllocationThresholdExceeded"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"V4SubnetAllocationThresholdExceeded\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"V4SubnetAllocationThresholdExceeded\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/V4SubnetAllocationThresholdExceeded.md"))
	})

	g.It("Author:weliang-Medium-51726-Create runbook and link SOP for NodeWithoutOVNKubeNodePodRunning alert", func() {
		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "networking-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("NodeWithoutOVNKubeNodePodRunning"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "networking-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NodeWithoutOVNKubeNodePodRunning\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "networking-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NodeWithoutOVNKubeNodePodRunning\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/NodeWithoutOVNKubeNodePodRunning.md"))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51723-bug 2094068 Create runbook and link SOP for NorthboundStale alert", func() {
		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("NorthboundStale"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NorthboundStale\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NorthboundStale\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/NorthboundStaleAlert.md"))
	})

	g.It("Author:qiowang-Medium-53999-OVN-K alerts for ovn controller disconnection", func() {
		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "networking-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("OVNKubernetesControllerDisconnectedSouthboundDatabase"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "networking-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="OVNKubernetesControllerDisconnectedSouthboundDatabase")].labels.severity}`).Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity of OVNKubernetesControllerDisconnectedSouthboundDatabase is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
	})

	g.It("Author:qiowang-Medium-60705-Verify alert OVNKubernetesNodeOVSOverflowKernel", func() {
		alertSeverity, alertExpr := getOVNAlertNetworkingRules(oc, "OVNKubernetesNodeOVSOverflowKernel")
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
		o.Expect(alertExpr).To(o.ContainSubstring("increase(ovs_vswitchd_dp_flows_lookup_lost[5m]) > 0"))
	})

	g.It("Author:qiowang-Medium-60706-Verify alert OVNKubernetesNodeOVSOverflowUserspace", func() {
		alertSeverity, alertExpr := getOVNAlertNetworkingRules(oc, "OVNKubernetesNodeOVSOverflowUserspace")
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
		o.Expect(alertExpr).To(o.ContainSubstring("increase(ovs_vswitchd_netlink_overflow[5m]) > 0"))
	})

	g.It("Author:qiowang-Medium-60709-Verify alert OVNKubernetesResourceRetryFailure", func() {
		alertSeverity, alertExpr := getOVNAlertNetworkingRules(oc, "OVNKubernetesResourceRetryFailure")
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
		o.Expect(alertExpr).To(o.ContainSubstring("increase(ovnkube_resource_retry_failures_total[10m]) > 0"))
	})
})
