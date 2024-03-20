package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logging-es", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Cluster Logging Instance tests", func() {

		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			g.By("deploy CLO and EO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			eoSource := CatalogSourceObjects{
				Channel: "stable",
			}
			EO := SubscriptionObjects{
				OperatorName:  "elasticsearch-operator",
				Namespace:     eoNS,
				PackageName:   "elasticsearch-operator",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
				CatalogSource: eoSource,
			}
			CLO.SubscribeOperator(oc)
			EO.SubscribeOperator(oc)
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-40168-oc adm must-gather can collect logging data [Slow][Disruptive]", func() {

			g.By("Create external Elasticsearch instance")
			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  loggingNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create CLF")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200")

			g.By("Deploy ClusterLogging with collector only instance")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check must-gather can collect cluster logging data")
			chkMustGather(oc, cl.namespace, "collector")

			g.By("Update CLF to forward logs to default ES and external ES")
			clf.update(oc, filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-and-default.yaml"), "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200")

			g.By("Update ClusterLogging to use Elasticsearch as default log store")
			cl.update(oc, filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"))
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By("Check must-gather can collect cluster logging data")
			chkMustGather(oc, cl.namespace, "elasticsearch")
		})

		// author ikanse@redhat.com
		g.It("CPaasrunOnly-Author:ikanse-Medium-46423-fluentd total_limit_size is not set beyond available space[Serial]", func() {

			g.Skip("Known issue in Cluster Logging 5.5.z https://issues.redhat.com/browse/LOG-2790")

			g.By("Create Cluster Logging instance with totalLimitSize which is more than the available space")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			cl := clusterlogging{
				name:             "instance",
				namespace:        loggingNS,
				collectorType:    "fluentd",
				logStoreType:     "elasticsearch",
				storageClassName: sc,
				esNodeCount:      1,
				waitForReady:     true,
				templateFile:     filepath.Join(loggingBaseDir, "clusterlogging", "cl-fluentd-buffer.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc, "REDUNDANCY_POLICY=ZeroRedundancy", "TOTAL_LIMIT_SIZE=1000G")

			g.By("Check Fluentd pod logs when Fluentd buffer totalLimitSize is set more than available space")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cl.namespace}
			pl.checkLogsFromRs(oc, "exceeds maximum available size", "collector")

			g.By("Set totalLimitSize to 3 GB")
			cl.update(oc, "", "{\"spec\":{\"collection\":{\"fluentd\":{\"buffer\":{\"totalLimitSize\":\"3G\"}}}}}", "--type=merge")

			g.By("Wait for 30 seconds for the config to be effective")
			time.Sleep(30 * time.Second)

			g.By("Delete collector pods for the Fluentd config changes to be picked up")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-n", cl.namespace, "-l", "component=collector").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By("Check Fluentd pod logs for Fluentd buffer totalLimitSize set to avilable space")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl = resource{"pods", podList.Items[0].Name, cl.namespace}
			// 3 x 1024 x 1024 x 1024 https://github.com/openshift/cluster-logging-operator/blob/c34520fd1a42151453b2d0a41e7e0cb14dce0d83/pkg/components/fluentd/run_script.go#L80
			pl.checkLogsFromRs(oc, "3221225472", "collector")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-49212-Logging should work as usual when secrets deleted or regenerated[Serial]", func() {
			g.By("deploy ECK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			cl := clusterlogging{
				name:             "instance",
				namespace:        loggingNS,
				collectorType:    "fluentd",
				logStoreType:     "elasticsearch",
				storageClassName: sc,
				esNodeCount:      3,
				waitForReady:     true,
				templateFile:     filepath.Join(loggingBaseDir, "clusterlogging", "cl-storage-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc, "REDUNDANCY_POLICY=SingleRedundancy")

			elasticsearch := resource{"secret", "elasticsearch", cl.namespace}
			collector := resource{"secret", "collector", cl.namespace}
			signingES := resource{"secret", "signing-elasticsearch", cl.namespace}
			g.By("remove secrets elasticsearch and collector, then check if theses secrets can be recreated or not")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "-n", cl.namespace, elasticsearch.name, collector.name).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			collector.WaitForResourceToAppear(oc)
			elasticsearch.WaitForResourceToAppear(oc)
			WaitForECKPodsToBeReady(oc, cl.namespace)
			esSVC := "https://elasticsearch." + cl.namespace + ".svc:9200"

			g.By("test connections between collector/kibana and ES")
			collectorPODs, _ := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			output, err := e2eoutput.RunHostCmdWithRetries(cl.namespace, collectorPODs.Items[0].Name, "curl --cacert /var/run/ocp-collector/secrets/collector/ca-bundle.crt --cert /var/run/ocp-collector/secrets/collector/tls.crt --key /var/run/ocp-collector/secrets/collector/tls.key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))
			kibanaPods, _ := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=kibana"})
			output, err = e2eoutput.RunHostCmdWithRetries(cl.namespace, kibanaPods.Items[0].Name, "curl -s --cacert /etc/kibana/keys/ca --cert /etc/kibana/keys/cert --key /etc/kibana/keys/key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))

			g.By("remove secret/signing-elasticsearch, then wait for the logging pods to be recreated")
			esPods, _ := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=elasticsearch"})
			signingES.clear(oc)
			signingES.WaitForResourceToAppear(oc)
			//should recreate ES and Kibana pods
			resource{"pod", esPods.Items[0].Name, cl.namespace}.WaitUntilResourceIsGone(oc)
			resource{"pod", kibanaPods.Items[0].Name, cl.namespace}.WaitUntilResourceIsGone(oc)
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By("test if kibana and collector pods can connect to ES again")
			collectorPODs, _ = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			output, err = e2eoutput.RunHostCmdWithRetries(cl.namespace, collectorPODs.Items[0].Name, "curl --cacert /var/run/ocp-collector/secrets/collector/ca-bundle.crt --cert /var/run/ocp-collector/secrets/collector/tls.crt --key /var/run/ocp-collector/secrets/collector/tls.key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))
			kibanaPods, _ = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=kibana"})
			output, err = e2eoutput.RunHostCmdWithRetries(cl.namespace, kibanaPods.Items[0].Name, "curl -s --cacert /etc/kibana/keys/ca --cert /etc/kibana/keys/cert --key /etc/kibana/keys/key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-53997-Medium-54664-Fluentd: Container logs collection and metadata check.[Serial]", func() {
			app := oc.Namespace()
			// to test fix for LOG-3463, add labels to the app project
			_, err := exutil.AddLabelsToSpecificResource(oc, "ns/"+app, "", "app=logging-apps", "app.kubernetes.io/instance=logging-apps-test", "app.test=test")
			o.Expect(err).NotTo(o.HaveOccurred())
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_non_json_log_template.json")
			err = oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			clusterID, err := getClusterID(oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create clusterlogforwarder instance to forward all logs to default Elasticsearch")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("deploy ECK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			cl := clusterlogging{
				name:             "instance",
				namespace:        loggingNS,
				collectorType:    "fluentd",
				logStoreType:     "elasticsearch",
				storageClassName: sc,
				waitForReady:     true,
				esNodeCount:      1,
				templateFile:     filepath.Join(loggingBaseDir, "clusterlogging", "cl-storage-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check logs in ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app, "app-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "infra-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "audit-00")

			appLogs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-00", "{\"query\": {\"regexp\": {\"kubernetes.namespace_name\": \""+app+"\"}}}")
			log := appLogs.Hits.DataHits[0].Source
			o.Expect(log.Message == "ㄅㄉˇˋㄓˊ˙ㄚㄞㄢㄦㄆ 中国 883.317µs ā á ǎ à ō ó ▅ ▆ ▇ █ 々").Should(o.BeTrue())
			o.Expect(log.OpenShift.ClusterID == clusterID).Should(o.BeTrue())
			o.Expect(log.OpenShift.Sequence > 0).Should(o.BeTrue())
			o.Expect(log.Kubernetes.NamespaceLabels["app_kubernetes_io_instance"] == "logging-apps-test").Should(o.BeTrue())
			o.Expect(log.Kubernetes.NamespaceLabels["app_test"] == "test").Should(o.BeTrue())
			infraLogs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "infra-00", "")
			o.Expect(infraLogs.Hits.DataHits[0].Source.OpenShift.ClusterID == clusterID).Should(o.BeTrue())
			auditLogs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "audit-00", "")
			o.Expect(auditLogs.Hits.DataHits[0].Source.OpenShift.ClusterID == clusterID).Should(o.BeTrue())

			for _, logType := range []string{"app", "infra", "audit"} {
				for _, field := range []string{"@timestamp", "openshift.cluster_id", "openshift.sequence"} {
					count, err := getDocCountByQuery(cl.namespace, podList.Items[0].Name, logType, "{\"query\": {\"bool\": {\"must_not\": {\"exists\": {\"field\": \""+field+"\"}}}}}")
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(count == 0).Should(o.BeTrue())
				}
			}
		})
	})
})
var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Fluentd should", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("logging-fluentd-"+getRandomString(), exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		g.By("deploy CLO and EO")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		eoSource := CatalogSourceObjects{
			Channel: "stable",
		}
		EO := SubscriptionObjects{
			OperatorName:  "elasticsearch-operator",
			Namespace:     eoNS,
			PackageName:   "elasticsearch-operator",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			CatalogSource: eoSource,
		}
		CLO.SubscribeOperator(oc)
		EO.SubscribeOperator(oc)
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-43177-expose the metrics needed to understand the volume of logs being collected.", func() {
		// create clusterlogging instance
		g.By("deploy ECK pods")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		instance := filepath.Join(loggingBaseDir, "clusterlogging", "cl-storage-template.yaml")
		cl := resource{"clusterlogging", "instance", loggingNS}
		cl.applyFromTemplate(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc)
		g.By("waiting for the ECK pods to be ready...")
		WaitForECKPodsToBeReady(oc, cl.namespace)

		exutil.By("deploy logfilesmetricexporter")
		lfme := logFileMetricExporter{
			name:          "instance",
			namespace:     loggingNS,
			template:      filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml"),
			waitPodsReady: true,
		}
		defer lfme.delete(oc)
		lfme.create(oc)

		podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForIndexAppear(cl.namespace, podList.Items[0].Name, "infra")

		g.By("check metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		for _, metric := range []string{"log_logged_bytes_total", "log_collected_bytes_total"} {
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				result, err := queryPrometheus(oc, token, "/api/v1/query", metric, "GET")
				if err != nil {
					return false, err
				}
				if len(result.Data.Result) > 0 {
					value, _ := strconv.Atoi(result.Data.Result[0].Value[1].(string))
					return (value > 0) && (len(result.Data.Result) > 0), nil
				}
				return false, nil

			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can't find metric %s", metric))
		}
	})

	//author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-High-53133-Fluentd Preserve k8s Common Labels[Serial]", func() {
		labels := map[string]string{
			"app.kubernetes.io/name":       "test",
			"app.kubernetes.io/instance":   "functionaltest",
			"app.kubernetes.io/version":    "123",
			"app.kubernetes.io/component":  "thecomponent",
			"app.kubernetes.io/part-of":    "clusterlogging",
			"app.kubernetes.io/managed-by": "clusterloggingoperator",
			"app.kubernetes.io/created-by": "anoperator",
			"run":                          "test-53133",
			"test":                         "test-logging-53133",
		}
		processedLabels := map[string]string{
			"app_kubernetes_io_name":       "test",
			"app_kubernetes_io_instance":   "functionaltest",
			"app_kubernetes_io_version":    "123",
			"app_kubernetes_io_component":  "thecomponent",
			"app_kubernetes_io_part-of":    "clusterlogging",
			"app_kubernetes_io_managed-by": "clusterloggingoperator",
			"app_kubernetes_io_created-by": "anoperator",
			"run":                          "test-53133",
			"test":                         "test-logging-53133",
		}
		labelJSON, _ := json.Marshal(labels)
		labelStr := string(labelJSON)
		oc.SetupProject()
		app := oc.Namespace()
		loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
		err := oc.WithoutNamespace().Run("new-app").Args("-f", loglabeltemplate, "-n", app, "-p", "LABELS="+labelStr).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//For this case, we need to cover ES and non-ES, and we need to check the log entity in log store,
		//to make the functions simple, here use external loki as the non-ES log store
		g.By("Create Loki project and deploy Loki Server")
		oc.SetupProject()
		lokiNS := oc.Namespace()
		loki := externalLoki{"loki-server", lokiNS}
		defer loki.remove(oc)
		loki.deployLoki(oc)

		g.By("Create ClusterLogForwarder instance")
		clf := clusterlogforwarder{
			name:         "instance",
			namespace:    loggingNS,
			templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki.yaml"),
		}
		defer clf.delete(oc)
		inputs, _ := json.Marshal([]string{"application"})
		outputs, _ := json.Marshal([]string{"loki-server", "default"})
		clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "INPUTREFS="+string(inputs), "OUTPUTREFS="+string(outputs))

		// create clusterlogging instance
		cl := clusterlogging{
			name:          "instance",
			namespace:     loggingNS,
			collectorType: "fluentd",
			logStoreType:  "elasticsearch",
			esNodeCount:   1,
			waitForReady:  true,
			templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc)

		g.By("check data in ES")
		esPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "app")
		waitForProjectLogsAppear(cl.namespace, esPods.Items[0].Name, app, "app")
		dataInES := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app", "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+app+"\"}}}")
		k8sLabelsInES := dataInES.Hits.DataHits[0].Source.Kubernetes.Lables
		o.Expect(len(k8sLabelsInES) > 0).Should(o.BeTrue())
		o.Expect(k8sLabelsInES["run"] == "").Should(o.BeTrue())
		o.Expect(k8sLabelsInES["test"] == "").Should(o.BeTrue())

		flatLabelsInES := dataInES.Hits.DataHits[0].Source.Kubernetes.FlatLabels
		// convert array to map and compare it with labels
		flatLabelsMap := make(map[string]string)
		for _, flatLabel := range flatLabelsInES {
			res := strings.Split(flatLabel, "=")
			flatLabelsMap[res[0]] = res[1]
		}
		o.Expect(reflect.DeepEqual(processedLabels, flatLabelsMap)).Should(o.BeTrue())

		g.By("check data in Loki")
		route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
		lc := newLokiClient(route)
		err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
			appLogs, err := lc.searchByNamespace("", app)
			if err != nil {
				return false, err
			}
			if appLogs.Status == "success" && len(appLogs.Data.Result) > 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "can't find app logs")
		dataInLoki, _ := lc.searchByNamespace("", app)
		lokiLog := extractLogEntities(dataInLoki)
		k8sLabelsInLoki := lokiLog[0].Kubernetes.Lables
		o.Expect(reflect.DeepEqual(processedLabels, k8sLabelsInLoki)).Should(o.BeTrue())
		flatLabelsInLoki := lokiLog[0].Kubernetes.FlatLabels
		o.Expect(reflect.DeepEqual(flatLabelsInES, flatLabelsInLoki)).Should(o.BeTrue())
	})

	//author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Low-49210-Fluentd alert rule: FluentdQueueLengthIncreasing[Serial][Slow]", func() {
		oc.SetupProject()
		app := oc.Namespace()
		logTemplate := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
		err := oc.WithoutNamespace().Run("new-app").Args("-f", logTemplate, "-n", app).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		// create clusterlogging instance
		g.By("deploy clusterlogging and make ES pod in CrashLoopBackOff status")
		cl := clusterlogging{
			name:             "instance",
			namespace:        loggingNS,
			collectorType:    "fluentd",
			logStoreType:     "elasticsearch",
			storageClassName: sc,
			templateFile:     filepath.Join(loggingBaseDir, "clusterlogging", "cl-storage-template.yaml"),
		}
		defer cl.delete(oc)
		cl.create(oc, "ES_REQUEST_MEMORY=600Mi")
		g.By("wait for collector pods to be ready")
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
		resource{"servicemonitor", "collector", cl.namespace}.WaitForResourceToAppear(oc)
		resource{"prometheusrule", "collector", cl.namespace}.WaitForResourceToAppear(oc)

		g.By("set clusterlogging to managementStatus to Unmanaged")
		cl.update(oc, "", "{\"spec\": {\"managementState\": \"Unmanaged\"}}", "--type=merge")

		g.By("update alert rule FluentdQueueLengthIncreasing to make it easier to appear for automation testing")
		patch := `[{"op": "replace", "path": "/spec/groups/0/rules/1/for", "value":"2m"}]`
		er := oc.AsAdmin().WithoutNamespace().Run("patch").Args("prometheusrules", "collector", "--type=json", "-p", patch, "-n", cl.namespace).Execute()
		o.Expect(er).NotTo(o.HaveOccurred())
		patch = `[{"op": "replace", "path": "/spec/groups/0/rules/1/expr", "value":"sum by (pod,plugin_id) ( 0 * (deriv(fluentd_output_status_emit_records[1m] offset 1m)))  + on(pod,plugin_id)  ( deriv(fluentd_output_status_buffer_queue_length[1m]) > 0 and delta(fluentd_output_status_buffer_queue_length[1m]) > 1 )"}]`
		er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("prometheusrules", "collector", "--type=json", "-p", patch, "-n", cl.namespace).Execute()
		o.Expect(er).NotTo(o.HaveOccurred())

		g.By("Check the alert CollectorNodeDown is in state firing")
		checkAlert(oc, getSAToken(oc, "prometheus-k8s", "openshift-monitoring"), "FluentdQueueLengthIncreasing", "pending|firing", 10)

	})

})
