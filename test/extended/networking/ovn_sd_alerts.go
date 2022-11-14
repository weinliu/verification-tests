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

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51438-Upgrade NoRunningOvnMaster to critical severity and inclue runbook.", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("NoRunningOvnMaster"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoRunningOvnMaster\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoRunningOvnMaster\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/NoRunningOvnMaster.md"))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51439-Upgrade NoOvnMasterLeader to critical severity and inclue runbook.", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("NoOvnMasterLeader"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoOvnMasterLeader\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoOvnMasterLeader\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/NoOvnMasterLeader.md"))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51722-Create runbook and link SOP for SouthboundStale alert", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("SouthboundStale"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NoOvnMasterLeader\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"SouthboundStale\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/SouthboundStaleAlert.md"))
	})

	g.It("NonHyperShiftHOST-Author:weliang-Medium-51724-Create runbook and link SOP for V4SubnetAllocationThresholdExceeded alert", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

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
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

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
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("NorthboundStale"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NorthboundStale\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))

		alertRunbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\"NorthboundStale\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertRunbook is %v", alertRunbook)
		o.Expect(alertRunbook).To(o.ContainSubstring("https://github.com/openshift/runbooks/blob/master/alerts/cluster-network-operator/NorthboundStaleAlert.md"))
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-55903-OVN-K alerts for ovn db leader", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		dbLeaderAlertsList := []string{
			"OVNKubernetesNorthboundDatabaseLeaderError",
			"OVNKubernetesSouthboundDatabaseLeaderError",
			"OVNKubernetesNorthboundDatabaseMultipleLeadersError",
			"OVNKubernetesSouthboundDatabaseMultipleLeadersError",
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		for _, dbLeaderAlerts := range dbLeaderAlertsList {
			o.Expect(alertName).To(o.ContainSubstring(dbLeaderAlerts))

			alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="`+dbLeaderAlerts+`")].labels.severity}`).Output()
			o.Expect(severityErr).NotTo(o.HaveOccurred())
			e2e.Logf("alertSeverity of "+dbLeaderAlerts+" is %v", alertSeverity)
			o.Expect(alertSeverity).To(o.ContainSubstring("critical"))
		}
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-55909-OVN-K alerts for ovn db cluster ID error", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		dbLeaderAlertsList := []string{
			"OVNKubernetesNorthboundDatabaseClusterIDError",
			"OVNKubernetesSouthboundDatabaseClusterIDError",
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		for _, dbLeaderAlerts := range dbLeaderAlertsList {
			o.Expect(alertName).To(o.ContainSubstring(dbLeaderAlerts))

			alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="`+dbLeaderAlerts+`")].labels.severity}`).Output()
			o.Expect(severityErr).NotTo(o.HaveOccurred())
			e2e.Logf("alertSeverity of "+dbLeaderAlerts+" is %v", alertSeverity)
			o.Expect(alertSeverity).To(o.ContainSubstring("critical"))
		}
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-53822-OVN-K alerts for ovn db term lag", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		dbTermLagAlertsList := []string{
			"OVNKubernetesNorthboundDatabaseTermLag",
			"OVNKubernetesSouthboundDatabaseTermLag",
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		for _, dbTermLagAlerts := range dbTermLagAlertsList {
			o.Expect(alertName).To(o.ContainSubstring(dbTermLagAlerts))

			alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="`+dbTermLagAlerts+`")].labels.severity}`).Output()
			o.Expect(severityErr).NotTo(o.HaveOccurred())
			e2e.Logf("alertSeverity of "+dbTermLagAlerts+" is %v", alertSeverity)
			o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
		}
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-53859-OVN-K alerts for ovn db cluster member error", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		dbMemeberAlertsList := []string{
			"OVNKubernetesNorthboundDatabaseClusterMemberError",
			"OVNKubernetesSouthboundDatabaseClusterMemberError",
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		for _, dbMemberAlerts := range dbMemeberAlertsList {
			o.Expect(alertName).To(o.ContainSubstring(dbMemberAlerts))

			alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="`+dbMemberAlerts+`")].labels.severity}`).Output()
			o.Expect(severityErr).NotTo(o.HaveOccurred())
			e2e.Logf("alertSeverity of "+dbMemberAlerts+" is %v", alertSeverity)
			o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
		}
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-53902-OVN-K alerts for ovn db connection error", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		dbConnAlertsList := []string{
			"OVNKubernetesNorthboundDatabaseOutboundConnectionError",
			"OVNKubernetesSouthboundDatabaseOutboundConnectionError",
			"OVNKubernetesNorthboundDatabaseOutboundConnectionMissing",
			"OVNKubernetesSouthboundDatabaseOutboundConnectionMissing",
			"OVNKubernetesNorthboundDatabaseInboundConnectionError",
			"OVNKubernetesSouthboundDatabaseInboundConnectionError",
			"OVNKubernetesNorthboundDatabaseInboundConnectionMissing",
			"OVNKubernetesSouthboundDatabaseInboundConnectionMissing",
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		for _, dbConnAlerts := range dbConnAlertsList {
			o.Expect(alertName).To(o.ContainSubstring(dbConnAlerts))

			alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="`+dbConnAlerts+`")].labels.severity}`).Output()
			o.Expect(severityErr).NotTo(o.HaveOccurred())
			e2e.Logf("alertSeverity of "+dbConnAlerts+" is %v", alertSeverity)
			o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
		}
	})

	g.It("NonHyperShiftHOST-Author:qiowang-Medium-53926-OVN-K alerts for ovn northd inactivity", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("OVNKubernetesNorthdInactive"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "master-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="OVNKubernetesNorthdInactive")].labels.severity}`).Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity of OVNKubernetesNorthdInactive is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("critical"))
	})

	g.It("Author:qiowang-Medium-53999-OVN-K alerts for ovn controller disconnection", func() {
		networkType := exutil.CheckNetworkType(oc)
		if !strings.Contains(networkType, "ovn") {
			g.Skip("Skip testing on non-ovn cluster!!!")
		}

		alertName, NameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "networking-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
		o.Expect(NameErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alertName is %v", alertName)
		o.Expect(alertName).To(o.ContainSubstring("OVNKubernetesControllerDisconnectedSouthboundDatabase"))

		alertSeverity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", "openshift-ovn-kubernetes", "networking-rules", `-o=jsonpath={.spec.groups[*].rules[?(@.alert=="OVNKubernetesControllerDisconnectedSouthboundDatabase")].labels.severity}`).Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("alertSeverity of OVNKubernetesControllerDisconnectedSouthboundDatabase is %v", alertSeverity)
		o.Expect(alertSeverity).To(o.ContainSubstring("warning"))
	})
})
