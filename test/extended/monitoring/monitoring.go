package monitoring

import (
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
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

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-49172-Enable validating webhook for AlertmanagerConfig customer resource", func() {
		var (
			err                       error
			output                    string
			namespace                 string
			invalidAlertmanagerConfig = filepath.Join(monitoringBaseDir, "invalid-alertmanagerconfig.yaml")
			validAlertmanagerConfig   = filepath.Join(monitoringBaseDir, "valid-alertmanagerconfig.yaml")
		)

		g.By("Get prometheus-operator-admission-webhook deployment")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "prometheus-operator-admission-webhook", "-n", "openshift-monitoring").Execute()
		if err != nil {
			e2e.Logf("Unable to get deployment prometheus-operator-admission-webhook.")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		namespace = oc.Namespace()

		g.By("Create invalid AlertmanagerConfig, should throw out error")
		output, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", invalidAlertmanagerConfig, "-n", namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("The AlertmanagerConfig \"invalid-test-config\" is invalid"))

		g.By("Create valid AlertmanagerConfig, should not have error")
		output, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", validAlertmanagerConfig, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("valid-test-config created"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-42800-Allow configuration of the log level for Alertmanager in the CMO configmap", func() {
		g.By("Check alertmanager container logs")
		exutil.WaitAndGetSpecificPodLogs(oc, "openshift-monitoring", "alertmanager", "alertmanager-main-0", "level=debug")
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-43748-Ensure label namespace exists on all alerts", func() {
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check alerts, should have label namespace exists on all alerts")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="Watchdog"}'`, token, `"namespace":"openshift-monitoring"`, 2*platformLoadTime)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-47307-Add external label of origin to platform alerts", func() {
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check alerts, could see the `openshift_io_alert_source` field for in-cluster alerts")
		checkMetric(oc, "https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts", token, `"openshift_io_alert_source":"platform"`, 2*platformLoadTime)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-45163-Show labels for pods/nodes/namespaces/PV/PVC/PDB in metrics", func() {
		var (
			ns          string
			helloPodPvc = filepath.Join(monitoringBaseDir, "helloPodPvc.yaml")
		)
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("Check labels for pod")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_labels{pod="alertmanager-main-0"}'`, token, `"label_statefulset_kubernetes_io_pod_name"`, uwmLoadTime)

		g.By("Check labels for node")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_node_labels'`, token, `"label_kubernetes_io_hostname"`, uwmLoadTime)

		g.By("Check labels for namespace")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_namespace_labels{namespace="openshift-monitoring"}'`, token, `"label_kubernetes_io_metadata_name"`, uwmLoadTime)

		g.By("Check labels for PDB")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_poddisruptionbudget_labels{poddisruptionbudget="thanos-querier-pdb"}'`, token, `"label_app_kubernetes_io_name"`, uwmLoadTime)

		g.By("create project ns then attach pv/pvc")
		oc.SetupProject()
		ns = oc.Namespace()
		createResourceFromYaml(oc, ns, helloPodPvc)

		g.By("Check labels for PV/PVC") //make sure pv/pvcs have been attached before checkMetric
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_persistentvolume_labels'`, token, `"persistentvolume"`, 2*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_persistentvolumeclaim_labels'`, token, `"persistentvolumeclaim"`, uwmLoadTime)
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-45271-Allow OpenShift users to configure audit logs for prometheus-adapter", func() {
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		podList, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=prometheus-adapter")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("prometheus-adapter Pods: %v", podList)

		g.By("check the audit logs")
		for _, pod := range podList {
			exutil.AssertPodToBeReady(oc, pod, "openshift-monitoring")
			output, err := exutil.RemoteShContainer(oc, "openshift-monitoring", pod, "prometheus-adapter", "cat", "/var/log/adapter/audit.log")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, `"level":"Request"`)).To(o.BeTrue(), "level Request is not in audit.log")
		}
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-48432-Allow OpenShift users to configure request logging for Thanos Querier query endpoint", func() {
		var (
			thanosQuerierPodName string
		)
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("make sure thanos-querier pods are ready")
		time.Sleep(60 * time.Second) //thanos-querier pod name will changed when cm modified, need time to create pods
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")

		g.By("query with thanos-querier svc")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=cluster_version'`, token, `"cluster_version"`, 3*uwmLoadTime)
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=cluster_version'`, token, `"cluster-version-operator"`, 2*uwmLoadTime)

		g.By("check from thanos-querier logs")
		thanosQuerierPodName, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-monitoring", "-l", "app.kubernetes.io/instance=thanos-querier", "-ojsonpath={.items[].metadata.name}").Output()
		exutil.WaitAndGetSpecificPodLogs(oc, "openshift-monitoring", "thanos-query", thanosQuerierPodName, "query=cluster_version")
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Low-43038-Should not have error for loading OpenAPI spec for v1beta1.metrics.k8s.io", func() {
		var (
			searchString string
			result       string
		)
		searchString = "loading OpenAPI spec for \"v1beta1.metrics.k8s.io\" failed with:"
		podList, err := exutil.GetAllPodsWithLabel(oc, "openshift-kube-apiserver", "app=openshift-kube-apiserver")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("kube-apiserver Pods: %v", podList)

		g.By("check the kube-apiserver logs, should not have error for v1beta1.metrics.k8s.io")
		for _, pod := range podList {
			exutil.AssertPodToBeReady(oc, pod, "openshift-kube-apiserver")
			result, _ = exutil.GetSpecificPodLogs(oc, "openshift-kube-apiserver", "kube-apiserver", pod, searchString)
			e2e.Logf("output result in logs: %v", result)
			o.Expect(len(result) == 0).To(o.BeTrue(), "found the error logs which is unexpected")
		}
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Low-55670-Prometheus should not collecting error messages for completed pods", func() {
		var output string
		g.By("check pod conditioning in openshift-kube-scheduler, all pods should be ready")
		exutil.AssertAllPodsToBeReady(oc, "openshift-kube-scheduler")

		g.By("get prometheus-adapter pod names")
		prometheusAdapterPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=prometheus-adapter")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check prometheus-adapter pod logs")
		for _, pod := range prometheusAdapterPodNames {
			output, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args(pod, "-n", "openshift-monitoring").Output()
			if strings.Contains(output, "unable to fetch CPU metrics for pod") {
				e2e.Failf("found unexpected logs: unable to fetch CPU metrics for pod")
			}
		}
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-55767-Missing metrics in kube-state-metrics", func() {
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check kube-state-metrics metrics, the following metrics should be visible")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/label/__name__/values`, token, `"kube_pod_container_status_terminated_reason"`, uwmLoadTime)
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/label/__name__/values`, token, `"kube_pod_init_container_status_terminated_reason"`, uwmLoadTime)
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/label/__name__/values`, token, `"kube_pod_status_scheduled_time"`, uwmLoadTime)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-High-56168-PreChkUpgrade-NonPreRelease-Prometheus never sees endpoint propagation of a deleted pod", func() {
		var (
			ns          = "56168-upgrade-ns"
			exampleApp  = filepath.Join(monitoringBaseDir, "example-app.yaml")
			roleBinding = filepath.Join(monitoringBaseDir, "sa-prometheus-k8s-access.yaml")
		)
		g.By("Create example app")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", ns).Execute()
		createResourceFromYaml(oc, ns, exampleApp)
		exutil.AssertAllPodsToBeReady(oc, ns)

		g.By("add role and role binding for example app")
		createResourceFromYaml(oc, ns, roleBinding)

		g.By("label namespace")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", ns, "openshift.io/cluster-monitoring=true").Execute()

		g.By("check target is up")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, "up", 2*uwmLoadTime)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-High-56168-PstChkUpgrade-NonPreRelease-Prometheus never sees endpoint propagation of a deleted pod", func() {
		g.By("get the ns name in PreChkUpgrade")
		ns := "56168-upgrade-ns"

		g.By("delete related resource at the end of case")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns).Execute()

		g.By("delete example app deployment")
		deleteApp, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("deploy", "prometheus-example-app", "-n", ns).Output()
		o.Expect(deleteApp).To(o.ContainSubstring(`"prometheus-example-app" deleted`))

		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check metric up==0, return null")
		checkMetric(oc, "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query= up == 0'", token, `"result":[]`, 2*uwmLoadTime)

		g.By("check no alert 'TargetDown'")
		checkAlertNotExist(oc, "TargetDown", token)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-57254-oc adm top node/pod output should not give negative numbers", func() {
		g.By("check on node")
		checkNode, err := exec.Command("bash", "-c", `oc adm top node | awk '{print $2,$3,$4,$5}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkNode).NotTo(o.ContainSubstring("-"))

		g.By("check on pod under specific namespace")
		checkNs, err := exec.Command("bash", "-c", `oc -n openshift-monitoring adm top pod | awk '{print $2,$3}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkNs).NotTo(o.ContainSubstring("-"))
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-55696-add telemeter alert TelemeterClientFailures", func() {
		g.By("check TelemeterClientFailures alert is added")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheusrules", "telemetry", "-ojsonpath={.spec.groups}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("TelemeterClientFailures"))
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
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "\"result\":[]", 2*uwmLoadTime)
				g.By("check alerts")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{namespace=\""+ns+"\"}'", token, "\"result\":[]", uwmLoadTime)

				g.By("label project being monitored")
				labelNameSpace(oc, ns, "openshift.io/user-monitoring=true")

				g.By("check metrics")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "prometheus-example-app", 2*uwmLoadTime)

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

			// author: tagao@redhat.com
			g.It("Author:tagao-Medium-50241-Prometheus (uwm) externalLabels not showing always in alerts", func() {
				var (
					exampleAppRule = filepath.Join(monitoringBaseDir, "in-cluster_query_alert_rule.yaml")
				)
				g.By("Create alert rule with expression about data provided by in-cluster prometheus")
				createResourceFromYaml(oc, ns, exampleAppRule)

				g.By("Get token of SA prometheus-k8s")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

				g.By("Check labelmy is in the alert")
				checkMetric(oc, "https://alertmanager-main.openshift-monitoring.svc:9094/api/v1/alerts", token, "labelmy", 2*uwmLoadTime)
			})

			// author: tagao@redhat.com
			g.It("Author:tagao-Medium-42825-Expose EnforcedTargetLimit in the CMO configuration for UWM", func() {
				g.By("check user metrics")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "prometheus-example-app", 2*uwmLoadTime)

				g.By("scale deployment replicas to 2")
				oc.WithoutNamespace().Run("scale").Args("deployment", "prometheus-example-app", "--replicas=2", "-n", ns).Execute()

				g.By("check user metrics again, the user metrics can't be found from thanos-querier")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "\"result\":[]", 2*uwmLoadTime)
			})

			// author: tagao@redhat.com
			g.It("Author:tagao-Medium-49189-Enforce label scrape limits for UWM [Serial]", func() {
				var (
					invalidUWM = filepath.Join(monitoringBaseDir, "invalid-uwm.yaml")
				)
				g.By("delete uwm-config/cm-config at the end of a serial case")
				defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
				defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

				g.By("Get token of SA prometheus-k8s")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

				g.By("query metrics from thanos-querier")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version'", token, "prometheus-example-app", uwmLoadTime)

				g.By("trigger label_limit exceed")
				createResourceFromYaml(oc, "openshift-user-workload-monitoring", invalidUWM)

				g.By("check in thanos-querier /targets api, it should complains the label_limit exceeded")
				checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, `label_limit exceeded`, 2*uwmLoadTime)

				g.By("trigger label_name_length_limit exceed")
				err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "user-workload-monitoring-config", "-p", `{"data": {"config.yaml": "prometheus:\n enforcedLabelLimit: 8\n enforcedLabelNameLengthLimit: 1\n enforcedLabelValueLengthLimit: 1\n"}}`, "--type=merge", "-n", "openshift-user-workload-monitoring").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				g.By("check in thanos-querier /targets api, it should complains the label_name_length_limit exceeded")
				checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, `label_name_length_limit exceeded`, 2*uwmLoadTime)

				g.By("trigger label_value_length_limit exceed")
				err2 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "user-workload-monitoring-config", "-p", `{"data": {"config.yaml": "prometheus:\n enforcedLabelLimit: 8\n enforcedLabelNameLengthLimit: 8\n enforcedLabelValueLengthLimit: 1\n"}}`, "--type=merge", "-n", "openshift-user-workload-monitoring").Execute()
				o.Expect(err2).NotTo(o.HaveOccurred())

				g.By("check in thanos-querier /targets api, it should complains the label_value_length_limit exceeded")
				checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, `label_value_length_limit exceeded`, 2*uwmLoadTime)

				g.By("relax restrictions")
				err3 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "user-workload-monitoring-config", "-p", `{"data": {"config.yaml": "prometheus:\n enforcedLabelLimit: 10\n enforcedLabelNameLengthLimit: 10\n enforcedLabelValueLengthLimit: 50\n"}}`, "--type=merge", "-n", "openshift-user-workload-monitoring").Execute()
				o.Expect(err3).NotTo(o.HaveOccurred())

				g.By("able to see the metrics")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version'", token, "prometheus-example-app", 2*uwmLoadTime)
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

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-46301-Allow OpenShift users to configure query log file for Prometheus", func() {
			g.By("make sure all pods in openshift-monitoring/openshift-user-workload-monitoring are ready")
			exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
			exutil.AssertAllPodsToBeReady(oc, "openshift-user-workload-monitoring")

			g.By("check query log file for prometheus in openshift-monitoring")
			oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "http://localhost:9090/api/v1/query?query=prometheus_build_info").Execute()
			output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "bash", "-c", "cat /tmp/promethues_query.log | grep prometheus_build_info").Output()
			o.Expect(output).To(o.ContainSubstring("prometheus_build_info"))

			g.By("check query log file for prometheus in openshift-user-workload-monitoring")
			oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-user-workload-monitoring", "-c", "prometheus", "prometheus-user-workload-0", "--", "curl", "http://localhost:9090/api/v1/query?query=up").Execute()
			output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-user-workload-monitoring", "-c", "prometheus", "prometheus-user-workload-0", "--", "bash", "-c", "cat /tmp/uwm_query.log | grep up").Output()
			o.Expect(output2).To(o.ContainSubstring("up"))
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-50008-Expose sigv4 settings for remote write in the CMO configuration [Serial]", func() {
			var (
				sigv4ClusterCM = filepath.Join(monitoringBaseDir, "sigv4-cluster-monitoring-cm.yaml")
				sigv4UwmCM     = filepath.Join(monitoringBaseDir, "sigv4-uwm-monitoring-cm.yaml")
				sigv4Secret    = filepath.Join(monitoringBaseDir, "sigv4-secret.yaml")
				sigv4SecretUWM = filepath.Join(monitoringBaseDir, "sigv4-secret-uwm.yaml")
			)
			g.By("delete secret/cm at the end of case")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "sigv4-credentials-uwm", "-n", "openshift-user-workload-monitoring").Execute()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "sigv4-credentials", "-n", "openshift-monitoring").Execute()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("Create sigv4 secret under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", sigv4Secret)

			g.By("Configure remote write sigv4 and enable user workload monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", sigv4ClusterCM)

			g.By("Check sig4 config under openshift-monitoring")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "sigv4:")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "region: us-central1")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "access_key: basic_user")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "secret_key: basic_pass")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "profile: SomeProfile")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "role_arn: SomeRoleArn")

			g.By("Create sigv4 secret under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", sigv4SecretUWM)

			g.By("Configure remote write sigv4 setting for user workload monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", sigv4UwmCM)

			g.By("Check sig4 config under openshift-user-workload-monitoring")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "sigv4:")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "region: us-east2")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "access_key: basic_user_uwm")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "secret_key: basic_pass_uwm")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "profile: umw_Profile")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "role_arn: umw_RoleArn")
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-49694-Expose OAuth2 settings for remote write in the CMO configuration [Serial]", func() {
			var (
				oauth2ClusterCM = filepath.Join(monitoringBaseDir, "oauth2-cluster-monitoring-cm.yaml")
				oauth2UwmCM     = filepath.Join(monitoringBaseDir, "oauth2-uwm-monitoring-cm.yaml")
				oauth2Secret    = filepath.Join(monitoringBaseDir, "oauth2-secret.yaml")
				oauth2SecretUWM = filepath.Join(monitoringBaseDir, "oauth2-secret-uwm.yaml")
			)
			g.By("delete secret/cm at the end of case")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "oauth2-credentials", "-n", "openshift-user-workload-monitoring").Execute()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "oauth2-credentials", "-n", "openshift-monitoring").Execute()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("Create oauth2 secret under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", oauth2Secret)

			g.By("Configure remote write oauth2 and enable user workload monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", oauth2ClusterCM)

			g.By("Check oauth2 config under openshift-monitoring")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://test.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "remote_timeout: 30s")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "client_id: basic_user")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "client_secret: basic_pass")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "token_url: https://example.com/oauth2/token")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "scope1")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "scope2")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "param1: value1")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "param2: value2")

			g.By("Create oauth2 secret under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", oauth2SecretUWM)

			g.By("Configure remote write oauth2 setting for user workload monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", oauth2UwmCM)

			g.By("Check oauth2 config under openshift-user-workload-monitoring")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://test.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "remote_timeout: 30s")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "client_id: basic_user")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "client_secret: basic_pass")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "token_url: https://example.com/oauth2/token")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "scope3")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "scope4")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "param3: value3")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "param4: value4")
		})

		//author: tagao@redhat.com
		g.It("Author:tagao-Medium-47519-Platform prometheus operator should reconcile AlertmanagerConfig resources from user namespaces [Serial]", func() {
			var (
				enableAltmgrConfig = filepath.Join(monitoringBaseDir, "enableUserAlertmanagerConfig.yaml")
				wechatConfig       = filepath.Join(monitoringBaseDir, "exampleAlertConfigAndSecret.yaml")
			)
			g.By("delete uwm-config/cm-config at the end of a serial case")
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("enable alert manager config")
			createResourceFromYaml(oc, "openshift-monitoring", enableAltmgrConfig)
			exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")

			g.By("check the initial alertmanager configuration")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "alertname = Watchdog", true)

			g.By("create&check alertmanagerconfig under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", wechatConfig)
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("alertmanagerconfig/config-example", "secret/wechat-config", "-n", "openshift-monitoring").Output()
			o.Expect(output).To(o.ContainSubstring("config-example"))
			o.Expect(output).To(o.ContainSubstring("wechat-config"))

			g.By("check if the new created AlertmanagerConfig is reconciled in the Alertmanager configuration (should not)")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", false)

			g.By("delete the alertmanagerconfig/secret created under openshift-monitoring")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("alertmanagerconfig/config-example", "secret/wechat-config", "-n", "openshift-monitoring").Execute()

			g.By("create one new project, label the namespace and create the same AlertmanagerConfig")
			oc.SetupProject()
			ns := oc.Namespace()
			oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", ns, "openshift.io/user-monitoring=false").Execute()

			g.By("create&check alertmanagerconfig under the namespace")
			createResourceFromYaml(oc, ns, wechatConfig)
			output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("alertmanagerconfig/config-example", "secret/wechat-config", "-n", ns).Output()
			o.Expect(output2).To(o.ContainSubstring("config-example"))
			o.Expect(output2).To(o.ContainSubstring("wechat-config"))

			g.By("check if the new created AlertmanagerConfig is reconciled in the Alertmanager configuration (should not)")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", false)

			g.By("update the label to true")
			oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", ns, "openshift.io/user-monitoring=true", "--overwrite").Execute()

			g.By("check if the new created AlertmanagerConfig is reconciled in the Alertmanager configuration")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", true)

			g.By("set enableUserAlertmanagerConfig to false")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "cluster-monitoring-config", "-p", `{"data": {"config.yaml": "alertmanagerMain:\n enableUserAlertmanagerConfig: false\n"}}`, "--type=merge", "-n", "openshift-monitoring").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("the AlertmanagerConfig from user project is removed")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", false)
		})

		g.It("Author:tagao-Medium-49404-Medium-49176-Expose Authorization settings for remote write in the CMO configuration, Add the relabel config to all user-supplied remote_write configurations [Serial]", func() {
			var (
				authClusterCM = filepath.Join(monitoringBaseDir, "auth-cluster-monitoring-cm.yaml")
				authUwmCM     = filepath.Join(monitoringBaseDir, "auth-uwm-monitoring-cm.yaml")
				authSecret    = filepath.Join(monitoringBaseDir, "auth-secret.yaml")
				authSecretUWM = filepath.Join(monitoringBaseDir, "auth-secret-uwm.yaml")
			)
			g.By("delete secret/cm at the end of case")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "rw-auth", "-n", "openshift-user-workload-monitoring").Execute()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "rw-auth", "-n", "openshift-monitoring").Execute()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("Create auth secret under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", authSecret)

			g.By("Configure remote write auth and enable user workload monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", authClusterCM)

			g.By("Check auth config under openshift-monitoring")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://remote-write.endpoint")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "target_label: __tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://basicAuth.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "username: basic_user")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "password: basic_pass")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "__tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "target_label: cluster_id")

			g.By("Create auth secret under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", authSecretUWM)

			g.By("Configure remote write auth setting for user workload monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", authUwmCM)

			g.By("Check auth config under openshift-user-workload-monitoring")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://remote-write.endpoint")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "target_label: __tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://basicAuth.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "username: basic_user")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "password: basic_pass")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://bearerTokenFile.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "__tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "target_label: cluster_id_1")
		})

	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Low-30088-User can not deploy ThanosRuler CRs in user namespaces [Serial]", func() {
		var (
			ns                string
			output            string
			deployThanosRuler = filepath.Join(monitoringBaseDir, "deployThanosRuler.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("deploy ThanosRuler under namespace as a common user (non-admin)")
		oc.SetupProject()
		ns = oc.Namespace()
		output, _ = oc.Run("apply").Args("-n", ns, "-f", deployThanosRuler).Output()
		o.Expect(output).To(o.ContainSubstring("Error from server (Forbidden):"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-NonPreRelease-Medium-49191-Enforce body_size_limit [Serial]", func() {
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("set `enforcedBodySizeLimit` to 0, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "0", "0")

		g.By("set `enforcedBodySizeLimit` to a invalid value, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "20MiBPS", "")

		g.By("set `enforcedBodySizeLimit` to 1MB to trigger PrometheusScrapeBodySizeLimitHit alert, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "1MB", "1MB")

		g.By("check PrometheusScrapeBodySizeLimitHit alert is triggered")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusScrapeBodySizeLimitHit"}'`, token, "PrometheusScrapeBodySizeLimitHit", 5*uwmLoadTime)

		g.By("set `enforcedBodySizeLimit` to 40MB, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "40MB", "40MB")

		g.By("check from alert, should not have enforcedBodySizeLimit")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusScrapeBodySizeLimitHit"}'`, token, `"result":[]`, 5*uwmLoadTime)

		g.By("set `enforcedBodySizeLimit` to automatic, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "automatic", "body_size_limit")

		g.By("check from alert, should not have enforcedBodySizeLimit")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusScrapeBodySizeLimitHit"}'`, token, `"result":[]`, 5*uwmLoadTime)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-60485-check On/Off switch of netdev Collector in Node Exporter [Serial]", func() {
		var (
			disableNetdev = filepath.Join(monitoringBaseDir, "disableNetdev.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check netdev Collector is enabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.netdev"))

		g.By("check netdev metrics in prometheus k8s pod")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="netdev"}'`, token, `"collector":"netdev"`, uwmLoadTime)

		g.By("disable netdev in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", disableNetdev)

		g.By("check netdev metrics in prometheus k8s pod again, should not have related metrics")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="netdev"}'`, token, `"result":[]`, 3*uwmLoadTime)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-59521-check On/Off switch of cpufreq Collector in Node Exporter [Serial]", func() {
		var (
			enableCpufreq = filepath.Join(monitoringBaseDir, "enableCpufreq.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check cpufreq Collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.cpufreq"))

		g.By("check cpufreq metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="cpufreq"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable cpufreq in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", enableCpufreq)

		g.By("check cpufreq metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="cpufreq"}'`, token, `"collector":"cpufreq"`, 3*uwmLoadTime)

		g.By("check cpufreq in daemonset")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output2).To(o.ContainSubstring("--collector.cpufreq"))
	})

	// author: hongyli@redhat.com
	g.It("Author:hongyli-Critical-44032-Restore cluster monitoring stack default configuration [Serial]", func() {
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)
		g.By("Delete config map user-workload--monitoring-config")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		g.By("Delete config map cluster-monitoring-config")
	})
})
