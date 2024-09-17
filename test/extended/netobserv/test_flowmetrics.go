package netobserv

import (
	"fmt"
	filePath "path/filepath"
	"regexp"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-netobserv] Network_Observability", func() {

	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("netobserv", exutil.KubeConfigPath())
		// NetObserv Operator variables
		netobservNS   = "openshift-netobserv-operator"
		NOPackageName = "netobserv-operator"
		catsrc        = Resource{"catsrc", "qe-app-registry", "openshift-marketplace"}
		NOSource      = CatalogSourceObjects{"stable", catsrc.Name, catsrc.Namespace}

		// Template directories
		baseDir         = exutil.FixturePath("testdata", "netobserv")
		subscriptionDir = exutil.FixturePath("testdata", "netobserv", "subscription")
		flowFixturePath = filePath.Join(baseDir, "flowcollector_v1beta2_template.yaml")
		flowmetricsPath = filePath.Join(baseDir, "flowmetrics_v1alpha1_template.yaml")

		// Operator namespace object
		OperatorNS = OperatorNamespace{
			Name:              netobservNS,
			NamespaceTemplate: filePath.Join(subscriptionDir, "namespace.yaml"),
		}
		NO = SubscriptionObjects{
			OperatorName:  "netobserv-operator",
			Namespace:     netobservNS,
			PackageName:   NOPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: &NOSource,
		}
		flow Flowcollector
	)

	g.BeforeEach(func() {
		// check if qe-app-registry catSrc is present
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if strings.Contains(output, "NotFound") {
			g.Skip("QE catalogsource not found, skipping test")
		}
		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))
		// check if Network Observability Operator is already present
		NOexisting := CheckOperatorStatus(oc, NO.Namespace, NO.PackageName)

		// create operatorNS and deploy operator if not present
		if !NOexisting {
			OperatorNS.DeployOperatorNamespace(oc)
			NO.SubscribeOperator(oc)
			// check if NO operator is deployed
			WaitForPodReadyWithLabel(oc, NO.Namespace, "app="+NO.OperatorName)
			NOStatus := CheckOperatorStatus(oc, NO.Namespace, NO.PackageName)
			o.Expect((NOStatus)).To(o.BeTrue())

			// check if flowcollector API exists
			flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
			o.Expect((flowcollectorAPIExists)).To(o.BeTrue())
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		// Create flowcollector in beforeEach
		flow = Flowcollector{
			Namespace:   oc.Namespace(),
			EBPFeatures: []string{"\"FlowRTT\""},
			LokiMode:    "Monolithic",
			LokiEnable:  "false",
			Template:    flowFixturePath,
		}
		flow.CreateFlowcollector(oc)
	})
	g.AfterEach(func() {
		flow.DeleteFlowcollector(oc)
	})

	g.It("Author:memodi-High-73539-Create custom metrics and charts [Serial]", func() {
		namespace := oc.Namespace()
		customMetrics := CustomMetrics{
			Namespace: namespace,
			Template:  flowmetricsPath,
		}
		mainDashversion, err := getResourceVersion(oc, "cm", "netobserv-main", "openshift-config-managed")
		o.Expect(err).NotTo(o.HaveOccurred())

		curv, err := getResourceVersion(oc, "cm", "flowlogs-pipeline-config-dynamic", namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		customMetrics.createCustomMetrics(oc)
		waitForResourceGenerationUpdate(oc, "cm", "flowlogs-pipeline-config-dynamic", "resourceVersion", curv, namespace)
		customMetricsConfig := customMetrics.getCustomMetricConfigs()
		var allUniqueDash = make(map[string]bool)
		var uniqueDashboards []string
		for _, cmc := range customMetricsConfig {
			for _, dashboard := range cmc.DashboardNames {
				if _, ok := allUniqueDash[dashboard]; !ok {
					allUniqueDash[dashboard] = true
					uniqueDashboards = append(uniqueDashboards, dashboard)
				}
			}
			// verify custom metrics queries
			for _, query := range cmc.Queries {
				metricsQuery := strings.Replace(query, "$METRIC", "netobserv_"+cmc.MetricName, 1)
				metricVal := pollMetrics(oc, metricsQuery)
				e2e.Logf("metricsQuery %f for query %s", metricVal, metricsQuery)
			}
		}
		// verify dashboard exists
		for _, uniqDash := range uniqueDashboards {
			dashName := strings.ToLower(regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(uniqDash, "-"))
			if dashName == "main" {
				waitForResourceGenerationUpdate(oc, "cm", "netobserv-"+dashName, "resourceVersion", mainDashversion, "openshift-config-managed")
			}
			checkResourceExists(oc, "cm", "netobserv-"+dashName, "openshift-config-managed")
		}
	})
})
