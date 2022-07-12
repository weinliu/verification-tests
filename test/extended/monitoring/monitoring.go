package monitoring

import (
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"path/filepath"
)

var _ = g.Describe("[sig-monitoring] Cluster_Observability parallel monitoring", func() {

	defer g.GinkgoRecover()

	var (
		oc                = exutil.NewCLI("monitor-"+getRandomString(), exutil.KubeConfigPath())
		monitoringCM      monitoringConfig
		monitoringBaseDir string
	)

	g.BeforeEach(func() {
		monitoringBaseDir = exutil.FixturePath("testdata", "monitoring")
		monitoringCMTemplate := filepath.Join(monitoringBaseDir, "cluster-monitoring-cm.yaml")
		// enable user workload monitoring and load other configurations from cluster-monitoring-config configmap
		monitoringCM = monitoringConfig{
			name:               "cluster-monitoring-config",
			namespace:          "openshift-monitoring",
			enableUserWorkload: true,
			template:           monitoringCMTemplate,
		}
		monitoringCM.create(oc)
	})

	// author: hongyli@redhat.com
	g.It("Author:hongyli-High-49073-Retention size settings for platform", func() {
		checkRetention(oc, "openshift-monitoring", "prometheus-k8s", "storage.tsdb.retention.size=10GiB", platformLoadTime)
		checkRetention(oc, "openshift-monitoring", "prometheus-k8s", "storage.tsdb.retention.time=45d", 20)
	})

	// author: hongyli@redhat.com
	g.It("Author:hongyli-High-49514-federate service endpoint and route of platform Prometheus", func() {
		var err error
		g.By("Bind cluster-monitoring-view cluster role to current user")
		clusterRoleBindingName := "clusterMonitoringViewFederate"
		defer deleteClusterRoleBinding(oc, clusterRoleBindingName)
		clusterRoleBinding, err := bindClusterRoleToUser(oc, "cluster-monitoring-view", oc.Username(), clusterRoleBindingName)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Created: %v %v", "ClusterRoleBinding", clusterRoleBinding.Name)

		g.By("Get token of current user")
		token := oc.UserConfig().BearerToken
		g.By("check federate endpoint service")
		checkMetric(oc, "https://prometheus-k8s.openshift-monitoring.svc:9091/federate --data-urlencode 'match[]=cluster_version'", token, "cluster_version{endpoint", platformLoadTime)

		g.By("check federate route")
		checkRoute(oc, "openshift-monitoring", "prometheus-k8s-federate", token, "match[]=cluster_version", "cluster_version{endpoint", platformLoadTime)

	})

	g.Context("user workload monitoring", func() {
		var (
			uwmMonitoringConfig string
		)
		g.BeforeEach(func() {
			monitoringBaseDir = exutil.FixturePath("testdata", "monitoring")
			uwmMonitoringConfig = filepath.Join(monitoringBaseDir, "uwm-monitoring-cm.yaml")
			createUWMConfig(oc, uwmMonitoringConfig)
		})

		g.When("Need example app", func() {
			var (
				ns         string
				exampleApp string
			)
			g.BeforeEach(func() {
				exampleApp = filepath.Join(monitoringBaseDir, "example-app.yaml")
				//create project
				oc.SetupProject()
				ns = oc.Namespace()
				//create example app and alert rule under the project
				g.By("Create example app!")
				createResourceFromYaml(oc, ns, exampleApp)
				exutil.AssertAllPodsToBeReady(oc, ns)
			})

			// author: hongyli@redhat.com
			g.It("Author:hongyli-Critical-43341-Exclude namespaces from user workload monitoring based on label", func() {
				var (
					exampleAppRule = filepath.Join(monitoringBaseDir, "example-alert-rule.yaml")
				)

				g.By("label project not being monitored")
				labelNameSpace(oc, ns, "openshift.io/user-monitoring=false")

				//create example app and alert rule under the project
				g.By("Create example alert rule!")
				createResourceFromYaml(oc, ns, exampleAppRule)

				g.By("Get token of SA prometheus-k8s")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

				g.By("check metrics")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "\"result\":[]", uwmLoadTime)
				g.By("check alerts")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{namespace=\""+ns+"\"}'", token, "\"result\":[]", uwmLoadTime)

				g.By("label project being monitored")
				labelNameSpace(oc, ns, "openshift.io/user-monitoring=true")

				g.By("check metrics")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "prometheus-example-app", uwmLoadTime)

				g.By("check alerts")
				checkMetric(oc, "https://thanos-ruler.openshift-user-workload-monitoring.svc:9091/api/v1/alerts", token, "TestAlert", uwmLoadTime)
			})

			// author: hongyli@redhat.com
			g.It("Author:hongyli-High-50024-High-49515-Check federate route and service of user workload Prometheus", func() {
				var err error
				g.By("Bind cluster-monitoring-view RBAC to default service account")
				uwmFederateRBACViewName := "uwm-federate-rbac-" + ns
				defer deleteBindMonitoringViewRoleToDefaultSA(oc, uwmFederateRBACViewName)
				clusterRoleBinding, err := bindMonitoringViewRoleToDefaultSA(oc, ns, uwmFederateRBACViewName)
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("Created: %v %v", "ClusterRoleBinding", clusterRoleBinding.Name)
				g.By("Get token of default service account")
				token := getSAToken(oc, "default", ns)

				g.By("check uwm federate endpoint service")
				checkMetric(oc, "https://prometheus-user-workload.openshift-user-workload-monitoring.svc:9092/federate --data-urlencode 'match[]=version'", token, "prometheus-example-app", uwmLoadTime)

				g.By("check uwm federate route")
				checkRoute(oc, "openshift-user-workload-monitoring", "federate", token, "match[]=version", "prometheus-example-app", 20)

			})
		})

		// author: hongyli@redhat.com
		g.It("Author:hongyli-High-49745-High-50519-Retention for UWM Prometheus and thanos ruler", func() {
			g.By("Check retention size of prometheus user workload")
			checkRetention(oc, "openshift-user-workload-monitoring", "prometheus-user-workload", "storage.tsdb.retention.size=5GiB", uwmLoadTime)
			g.By("Check retention of prometheus user workload")
			checkRetention(oc, "openshift-user-workload-monitoring", "prometheus-user-workload", "storage.tsdb.retention.time=15d", 20)
			g.By("Check retention of thanos ruler")
			checkRetention(oc, "openshift-user-workload-monitoring", "thanos-ruler-user-workload", "retention=15d", uwmLoadTime)
		})

		// author: juzhao@redhat.com
		g.It("Author:juzhao-Medium-42956-Should not have PrometheusNotIngestingSamples alert if enabled user workload monitoring only", func() {
			g.By("Get token of SA prometheus-k8s")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

			g.By("check alerts, Should not have PrometheusNotIngestingSamples alert fired")
			checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusNotIngestingSamples"}'`, token, `"result":[]`, uwmLoadTime)
		})

	})

	// author: hongyli@redhat.com
	g.It("Author:hongyli-Critical-44032-Restore cluster monitoring stack default configuration [Serial]", func() {
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)
		g.By("Delete config map user-workload--monitoring-config")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		g.By("Delete config map cluster-monitoring-config")
	})
})
