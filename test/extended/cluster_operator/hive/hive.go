package hive

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

//
// Hive test case suite for platform independent and all other platforms
//

var _ = g.Describe("[sig-hive] Cluster_Operator hive should", func() {
	defer g.GinkgoRecover()

	var (
		oc          = exutil.NewCLI("hive-"+getRandomString(), exutil.KubeConfigPath())
		ns          hiveNameSpace
		og          operatorGroup
		sub         subscription
		hc          hiveconfig
		testDataDir string
	)
	g.BeforeEach(func() {
		//Install Hive operator if not
		testDataDir = exutil.FixturePath("testdata", "cluster_operator/hive")
		installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)

		//Enable hive Metric
		exportMetric(oc, enable)

	})

	//author: lwan@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:lwan-Critical-29670-install/uninstall hive operator from OperatorHub", func() {
		g.By("Check Subscription...")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "AllCatalogSourcesHealthy", ok, DefaultTimeout, []string{"sub", sub.name, "-n",
			sub.namespace, "-o=jsonpath={.status.conditions[0].reason}"}).check(oc)

		g.By("Check Hive Operator pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-operator", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check Hive Operator pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		g.By("Hive Operator sucessfully installed !!! ")

		g.By("Check hive-clustersync pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check hive-clustersync pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		g.By("Check hive-controllers pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check hive-controllers pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		g.By("Check hiveadmission pods are created !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission",
			"-n", sub.namespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		g.By("Check hiveadmission pods are in running state !!!")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission", "-n",
			sub.namespace, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		g.By("Hive controllers,clustersync and hiveadmission sucessfully installed !!! ")
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44914"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:lwan-Medium-44914-View Hive Metrics with OpenShift Cluster Monitoring [Serial]", func() {
		g.By("Verify hive metrics can get from prometheus...")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		e2e.Logf("Check hive metrics can be query from promethues")
		query := []string{"hive_clustersync_first_success_duration_seconds_count"}
		checkMetricExist(oc, ok, token, PrometheusURL, query)

		g.By("Disabled exportedMetric in HiveConfig, Check hive metrics disappear from prometheus...")
		defer exportMetric(oc, enable)
		exportMetric(oc, disable)
		e2e.Logf("Check hive metrics can't be query from promethues after exportedMetric disabled in HiveConfig")
		checkMetricExist(oc, nok, token, PrometheusURL, query)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41932"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("NonHyperShiftHOST-NonPreRelease-ConnectedOnly-Author:lwan-Medium-41932-Add metric for hive-operator[Serial]", func() {
		g.By("Create PodMonitor for HiveConfig...")
		podMonitorYaml := filepath.Join(testDataDir, "hive-operator-podmonitor.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", podMonitorYaml, "--ignore-not-found").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", podMonitorYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check hive-operator metrics can be query from promethues")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query1 := "hive_operator_reconcile_seconds_sum"
		query2 := "hive_operator_reconcile_seconds_count"
		query3 := "hive_operator_reconcile_seconds_bucket"
		query4 := "hive_hiveconfig_conditions"
		query := []string{query1, query2, query3, query4}
		checkMetricExist(oc, ok, token, PrometheusURL, query)

		g.By("Check HiveConfig status from Metric...")
		expectedType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.status.conditions[0].type}").Output()
		expectedReason, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.status.conditions[0].reason}").Output()
		checkHiveConfigMetric(oc, "condition", expectedType, token, PrometheusURL, query4)
		checkHiveConfigMetric(oc, "reason", expectedReason, token, PrometheusURL, query4)
	})

})
