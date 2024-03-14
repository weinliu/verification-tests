package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("vector-es-namespace", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Vector collector tests", func() {
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
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-49368-Low-46880-Vector Deploy Cluster Logging with Vector as collector using CLI and exclude Vector logs from collection[Serial]", func() {

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Make sure the Elasticsearch cluster is healthy")
			assertResourceStatus(oc, "clusterlogging", cl.name, cl.namespace, "{.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Check priorityClass in ds/collector")
			pri, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ds/collector", "-n", cl.namespace, "-ojsonpath={.spec.template.spec.priorityClassName}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(pri, "system-node-critical")).To(o.BeTrue(), "the priorityClass in ds/collector is: "+pri)

			g.By("Check app indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-000")

			g.By("Check infra indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "infra-000")

			g.By("Check for Vector logs in Elasticsearch")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.container_name\": \"collector\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "*", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Vector logs should not be collected")
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-49390-Vector Collecting Kubernetes events using event router[Serial][Slow]", func() {
			eventrouterTemplate := filepath.Join(loggingBaseDir, "eventrouter", "eventrouter.yaml")

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Make sure the Elasticsearch cluster is healthy")
			assertResourceStatus(oc, "clusterlogging", cl.name, cl.namespace, "{.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Deploy the Event Router")
			evt := resource{"deployment", "eventrouter", cl.namespace}
			defer deleteEventRouter(oc, cl.namespace)
			evt.createEventRouter(oc, "-f", eventrouterTemplate)

			g.By("Check event logs in the Event Router pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=eventrouter"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cl.namespace}
			pl.checkLogsFromRs(oc, "ADDED", "kube-eventrouter")
			pl.checkLogsFromRs(oc, "Update", "kube-eventrouter")

			g.By("Check for Event Router logs in Elasticsearch")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"component=eventrouter\"}}}"
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
				logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "infra", checkLog)
				if logs.Hits.Total > 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "No Event Router logs found when using vector as log collector.")
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-53311-Vector Configure Vector collector with CPU/Memory requests/limits[Serial]", func() {

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				waitForReady:  true,
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc, "COLLECTOR_LIMITS_MEMORY=750Mi", "COLLECTOR_LIMITS_CPU=150m", "COLLECTOR_REQUESTS_CPU=150m", "COLLECTOR_REQUESTS_MEMORY=750Mi")

			g.By("Check collector CPU/memory requests/limits")
			assertResourceStatus(oc, "daemonset", "collector", cl.namespace, "{.spec.template.spec.containers[].resources.limits.cpu}", "150m")
			assertResourceStatus(oc, "daemonset", "collector", cl.namespace, "{.spec.template.spec.containers[].resources.requests.cpu}", "150m")
			assertResourceStatus(oc, "daemonset", "collector", cl.namespace, "{.spec.template.spec.containers[].resources.limits.memory}", "750Mi")
			assertResourceStatus(oc, "daemonset", "collector", cl.namespace, "{.spec.template.spec.containers[].resources.requests.memory}", "750Mi")
		})

		g.It("CPaasrunOnly-Author:ikanse-High-53312-Vector Deploy Vector collector with nodeSelector[Serial]", func() {

			g.By("Set OCP node label to vector: test")
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("nodes", "--selector=kubernetes.io/os=linux", "vector=test", "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-collector-nodeselector.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc, "NODE_LABEL={\"vector\": \"deploy\"}")
			g.By("Check Collector daemonset has no running pods")
			esDeployNames := getDeploymentsNameByLabel(oc, cl.namespace, "cluster-name=elasticsearch")
			for _, name := range esDeployNames {
				WaitForDeploymentPodsToBeReady(oc, cl.namespace, name)
			}
			WaitForDeploymentPodsToBeReady(oc, cl.namespace, "kibana")
			var output string
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				daemonset, err := oc.AdminKubeClient().AppsV1().DaemonSets(cl.namespace).Get(context.Background(), "collector", metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						e2e.Logf("Waiting for availability of daemonset collector")
						return false, nil
					}
					return false, err
				}
				if daemonset.Status.NumberReady == 0 {
					return true, nil
				}
				output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("ds/collector", "-n", cl.namespace, "-ojsonpath={.status}").Output()
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Daemonset collector is not available with output:\n %s", output))

			g.By("Set OCP node label to vector: deploy")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("nodes", "--selector=kubernetes.io/os=linux", "vector=deploy", "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check Collector daemonset pods are running")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By("Set OCP node label to vector: deploy1")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("nodes", "--selector=kubernetes.io/os=linux", "vector=deploy1", "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check Collector daemonset has no running pods")
			esDeployNames = getDeploymentsNameByLabel(oc, cl.namespace, "cluster-name=elasticsearch")
			for _, name := range esDeployNames {
				WaitForDeploymentPodsToBeReady(oc, cl.namespace, name)
			}
			WaitForDeploymentPodsToBeReady(oc, cl.namespace, "kibana")
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				daemonset, err := oc.AdminKubeClient().AppsV1().DaemonSets(cl.namespace).Get(context.Background(), "collector", metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						e2e.Logf("Waiting for availability of daemonset collector")
						return false, nil
					}
					return false, err
				}
				if daemonset.Status.NumberReady == 0 {
					return true, nil
				}
				output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("ds/collector", "-n", cl.namespace, "-ojsonpath={.status}").Output()
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Daemonset collector is not available with output:\n %s", output))

			g.By("Patch Cluster Logging instance to use nodeSelector vector: deploy1")
			cl.update(oc, "", "{\"spec\":{\"collection\":{\"nodeSelector\":{\"vector\":\"deploy1\"}}}}", "--type=merge")

			g.By("Check Collector daemonset pods are running")
			WaitForECKPodsToBeReady(oc, cl.namespace)
		})

		g.It("CPaasrunOnly-Author:ikanse-High-53995-Vector Collect OVN audit logs [Serial]", func() {

			g.By("Check the network type for the test")
			networkType := checkNetworkType(oc)
			if !strings.Contains(networkType, "ovnkubernetes") {
				g.Skip("Skip for non-supported network type, type is not OVNKubernetes!!!")
			}

			g.By("Create clusterlogforwarder instance to forward OVN audit logs to default Elasticsearch instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			g.By("Deploy ClusterLogging instance.")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)
			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By("Check audit index in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "audit-00")

			g.By("Create a test project, enable OVN network log collection on it, add the OVN log app and network policies for the project")
			oc.SetupProject()
			ovnProj := oc.Namespace()
			ovn := resource{"deployment", "ovn-app", ovnProj}
			esTemplate := filepath.Join(loggingBaseDir, "generatelog", "42981.yaml")
			err = ovn.applyFromTemplate(oc, "-n", ovn.namespace, "-f", esTemplate, "-p", "NAMESPACE="+ovn.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForDeploymentPodsToBeReady(oc, ovnProj, ovn.name)

			g.By("Access the OVN app pod from another pod in the same project to generate OVN ACL messages")
			ovnPods, err := oc.AdminKubeClient().CoreV1().Pods(ovnProj).List(context.Background(), metav1.ListOptions{LabelSelector: "app=ovn-app"})
			o.Expect(err).NotTo(o.HaveOccurred())
			podIP := ovnPods.Items[0].Status.PodIP
			e2e.Logf("Pod IP is %s ", podIP)
			var ovnCurl string
			if strings.Contains(podIP, ":") {
				ovnCurl = "curl --globoff [" + podIP + "]:8080"
			} else {
				ovnCurl = "curl --globoff " + podIP + ":8080"
			}
			_, err = e2eoutput.RunHostCmdWithRetries(ovnProj, ovnPods.Items[1].Name, ovnCurl, 3*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check for the generated OVN audit logs on the OpenShift cluster nodes")
			nodeLogs, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", ovnProj, "node-logs", "-l", "beta.kubernetes.io/os=linux", "--path=/ovn/acl-audit-log.log").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(nodeLogs).Should(o.ContainSubstring(ovnProj), "The OVN logs doesn't contain logs from project %s", ovnProj)

			g.By("Check for the generated OVN audit logs in Elasticsearch")
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				cmd := "es_util --query=audit*/_search?format=JSON -d '{\"query\":{\"query_string\":{\"query\":\"verdict=allow AND severity=alert AND tcp,vlan_tci AND tcp_flags=ack\",\"default_field\":\"message\"}}}'"
				stdout, err := e2eoutput.RunHostCmdWithRetries(cl.namespace, esPods.Items[0].Name, cmd, 3*time.Second, 30*time.Second)
				if err != nil {
					return false, err
				}
				res := SearchResult{}
				json.Unmarshal([]byte(stdout), &res)
				if res.Hits.Total > 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "The ovn audit logs are not collected")

			g.By("Check audit logs for missing @timestamp field.")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"query\": {\"bool\":{\"must_not\":{\"exists\":{\"field\":\"@timestamp\"}}}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Audit logs are missing @timestamp field.")
		})

		g.It("CPaasrunOnly-Author:ikanse-High-53313-Vector collector deployed with tolerations[Serial][disruptive]", func() {

			g.By("Taint a worker node vector=deploy:NoSchedule so that no collector pod will be scheduled on it")
			taintNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", taintNode, "vector-", "--overwrite").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", taintNode, "vector=deploy:NoSchedule", "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By(fmt.Sprintf("Make sure that the collector pod is not scheduled on the %s", taintNode))
			chkCollector, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", cl.namespace, "pods", "--selector=component=collector", "--field-selector", "spec.nodeName="+taintNode+"", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(chkCollector).Should(o.BeEmpty())

			g.By("Add toleration for collector for the taint vector=deploy:NoSchedule")
			cl.update(oc, "", "{\"spec\":{\"collection\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"vector\",\"operator\":\"Equal\",\"value\":\"deploy\"}]}}}", "--type=merge")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By(fmt.Sprintf("Make sure that the collector pod is scheduled on the node %s after applying toleration for taint vector=deploy:NoSchedule", taintNode))
			chkCollector, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", cl.namespace, "pods", "--selector=component=collector", "--field-selector", "spec.nodeName="+taintNode+"", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(chkCollector).Should(o.ContainSubstring("collector"))
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-47063-Vector ServiceMonitor object for Vector is deployed along with Cluster Logging[Serial]", func() {

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			exutil.By("deploy logfilesmetricexporter")
			lfme := logFileMetricExporter{
				name:          "instance",
				namespace:     loggingNS,
				template:      filepath.Join(loggingBaseDir, "logfilemetricexporter", "lfme.yaml"),
				waitPodsReady: true,
			}
			defer lfme.delete(oc)
			lfme.create(oc)

			g.By("Make sure the Elasticsearch cluster is healthy")
			assertResourceStatus(oc, "elasticsearch", "elasticsearch", cl.namespace, "{.status.cluster.status}", "green")

			g.By("Check if the ServiceMonitor object for Vector is created.")
			resource{"servicemonitor", "collector", cl.namespace}.WaitForResourceToAppear(oc)

			g.By("Check the Vector metrics")
			bearerToken := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			output, err := e2eoutput.RunHostCmdWithRetries(cl.namespace, podList.Items[0].Name, "curl -k -H \"Authorization: Bearer "+bearerToken+"\" -H \"Content-type: application/json\" https://collector.openshift-logging.svc:24231/metrics", 10*time.Second, 20*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("vector_component_received_event_bytes_total"))

			g.By("Check the Log file metrics exporter metrics")
			output, err = e2eoutput.RunHostCmdWithRetries(cl.namespace, podList.Items[0].Name, "curl -k -H \"Authorization: Bearer "+bearerToken+"\" -H \"Content-type: application/json\" https://logfilesmetricexporter.openshift-logging.svc:2112/metrics", 10*time.Second, 20*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("log_logged_bytes_total"))

			g.By("Check Vector and Log File metrics exporter metrics from Prometheus")
			for _, metric := range []string{"log_logged_bytes_total", "vector_processed_bytes_total"} {
				checkMetric(oc, bearerToken, metric, 3)
			}
		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Critical-47060-All node logs have been sent to Elasticsearch[Serial]", func() {
			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				waitForReady:  true,
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check indices in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "infra")
			collectorPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())

			var nodeNames []string
			for _, pod := range collectorPods.Items {
				nodeNames = append(nodeNames, pod.Spec.NodeName)
			}

			g.By("check container logs")
			queryContainerLog := "_search?size=0 -d'{\"aggs\": {\"logging_aggregations\": {\"filter\": {\"exists\": {\"field\":\"kubernetes\"}},\"aggs\": {\"inner_aggregations\": {\"terms\": {\"field\" : \"hostname\"}}}}}}'"
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				aggRes, err := queryInES(cl.namespace, esPods.Items[0].Name, queryContainerLog)
				if err != nil {
					return false, err
				}
				containerBuckets := aggRes.Aggregations.LoggingAggregations.InnerAggregations.Buckets
				if aggRes.Hits.Total <= 0 || len(containerBuckets) != len(collectorPods.Items) {
					return false, nil
				}
				for _, node := range nodeNames {
					if !containSubstring(containerBuckets, node) {
						return false, nil
					}
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "Not all nodes' container logs have been sent to ES")

			g.By("check journal logs")
			queryJournalLog := "_search?size=0 -d'{\"aggs\": {\"logging_aggregations\": {\"filter\": {\"exists\": {\"field\":\"systemd\"}},\"aggs\": {\"inner_aggregations\": {\"terms\": {\"field\" : \"hostname\"}}}}}}'"
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				journalLog, err := queryInES(cl.namespace, esPods.Items[0].Name, queryJournalLog)
				if err != nil {
					return false, err
				}
				if journalLog.Hits.Total <= 0 {
					return false, nil
				}
				journalBuckets := journalLog.Aggregations.LoggingAggregations.InnerAggregations.Buckets
				if len(journalBuckets) == 0 {
					return false, nil
				}
				// In AWS, the hostname in journal logs is not the same as the node name
				if exutil.CheckPlatform(oc) == "aws" {
					for _, bu := range journalBuckets {
						if !containSubstring(nodeNames, bu.Key) {
							return false, nil
						}
					}
				} else {
					for _, node := range nodeNames {
						if !containSubstring(journalBuckets, node) {
							return false, nil
						}
					}
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "Not all nodes' journal logs have been sent to ES")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-47061-Vector: Container logs collection and metadata check.[Serial]", func() {
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

			g.By("deploy EFK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			cl := clusterlogging{
				name:             "instance",
				namespace:        loggingNS,
				collectorType:    "vector",
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

		g.It("CPaasrunOnly-Author:ikanse-High-55396-Vector alert rule CollectorNodeDown [Serial][Slow]", func() {
			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc, "COLLECTOR_LIMITS_MEMORY=750Gi", "COLLECTOR_LIMITS_CPU=150m", "COLLECTOR_REQUESTS_CPU=150m", "COLLECTOR_REQUESTS_MEMORY=750Gi")

			g.By("Check Collector daemonset has no running pods")
			esDeployNames := getDeploymentsNameByLabel(oc, cl.namespace, "cluster-name=elasticsearch")
			for _, name := range esDeployNames {
				WaitForDeploymentPodsToBeReady(oc, cl.namespace, name)
			}
			WaitForDeploymentPodsToBeReady(oc, cl.namespace, "kibana")
			checkResource(oc, true, true, "0", []string{"ds", "collector", "-n", cl.namespace, "-ojsonpath={.status.numberReady}"})

			g.By("Set the ClusterLoging instance managementState to Unmanaged to avoid prometheus overwrite")
			cl.update(oc, "", "{\"spec\": {\"managementState\": \"Unmanaged\"}}", "--type=merge")

			g.By("Patch the collector Prometheus Rule for alert CollectorNodeDown to set alert firing time to 2m")
			patch := `[{"op": "replace", "path": "/spec/groups/0/rules/0/for", "value":"2m"}]`
			er := oc.AsAdmin().WithoutNamespace().Run("patch").Args("prometheusrules", "collector", "--type=json", "-p", patch, "-n", cl.namespace).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Check the alert CollectorNodeDown is in state firing")
			checkAlert(oc, getSAToken(oc, "prometheus-k8s", "openshift-monitoring"), "CollectorNodeDown", "firing", 5)

		})

	})

	g.Context("Vector User-Managed-ES tests", func() {
		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			g.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-52129-Vector Send JSON logs from containers in the same pod to separate indices", func() {
			app := oc.Namespace()
			containerName := "log-52129-" + getRandomString()
			multiContainerJSONLog := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", multiContainerJSONLog, "-n", app, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "7",
				serverName: "external-es",
				httpSSL:    true,
				secretName: "json-log-52129",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)
			eesURL := "https://" + ees.serverName + "." + ees.namespace + ".svc:9200"

			g.By("create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                   "clf-52129",
				namespace:              esProj,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-logs.yaml"),
				secretName:             ees.secretName,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "STRUCTURED_CONTAINER=true", "URL="+eesURL, "ES_VERSION="+ees.version)

			g.By("check indices in externale ES")
			ees.waitForIndexAppear(oc, containerName+"-0")
			ees.waitForIndexAppear(oc, containerName+"-1")
			ees.waitForIndexAppear(oc, "app-"+app)

			queryContainerLog := func(container string) string {
				return "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.container_name\": \"" + container + "\"}}}"
			}

			// in index app-$containerName-0, only logs in container $containerName-0 are stored in it, and json logs are parsed
			log0 := ees.searchDocByQuery(oc, "app-"+containerName+"-0", queryContainerLog(containerName+"-0"))
			o.Expect(len(log0.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log0.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log01 := ees.searchDocByQuery(oc, "app-"+containerName+"-0", queryContainerLog(containerName+"-1"))
			o.Expect(len(log01.Hits.DataHits) == 0).To(o.BeTrue())
			log02 := ees.searchDocByQuery(oc, "app-"+containerName+"-0", queryContainerLog(containerName+"-2"))
			o.Expect(len(log02.Hits.DataHits) == 0).To(o.BeTrue())

			// in index app-$containerName-1, only logs in container $containerName-1 are stored in it, and json logs are parsed
			log1 := ees.searchDocByQuery(oc, "app-"+containerName+"-1", queryContainerLog(containerName+"-1"))
			o.Expect(len(log1.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log1.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log10 := ees.searchDocByQuery(oc, "app-"+containerName+"-1", queryContainerLog(containerName+"-0"))
			o.Expect(len(log10.Hits.DataHits) == 0).To(o.BeTrue())
			log12 := ees.searchDocByQuery(oc, "app-"+containerName+"-1", queryContainerLog(containerName+"-2"))
			o.Expect(len(log12.Hits.DataHits) == 0).To(o.BeTrue())

			// in index app-$app-project, only logs in container $containerName-2 are stored in it, and json logs are parsed
			log2 := ees.searchDocByQuery(oc, "app-"+app, queryContainerLog(containerName+"-2"))
			o.Expect(len(log2.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log20 := ees.searchDocByQuery(oc, "app-"+app, queryContainerLog(containerName+"-0"))
			o.Expect(len(log20.Hits.DataHits) == 0).To(o.BeTrue())
			log21 := ees.searchDocByQuery(oc, "app-"+app, queryContainerLog(containerName+"-1"))
			o.Expect(len(log21.Hits.DataHits) == 0).To(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-52131-Vector Logs from different projects are forwarded to the same index if the pods have same annotation", func() {
			containerName := "log-52131-" + getRandomString()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			app1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app1, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			app2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app2, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "7",
				serverName: "external-es",
				httpSSL:    true,
				secretName: "json-log-52131",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)
			eesURL := "https://" + ees.serverName + "." + ees.namespace + ".svc:9200"

			g.By("create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                   "clf-52131",
				namespace:              esProj,
				templateFile:           filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-logs.yaml"),
				secretName:             ees.secretName,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "STRUCTURED_CONTAINER=true", "URL="+eesURL, "ES_VERSION="+ees.version)

			g.By("check indices in externale ES")
			ees.waitForIndexAppear(oc, "app-"+containerName)

			g.By("check data in ES")
			for _, proj := range []string{app1, app2} {
				count, err := ees.getDocCount(oc, "app-"+containerName, "{\"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+proj+"\"}}}")
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(count > 0).To(o.BeTrue())
			}
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48591-Vector ClusterLogForwarder Label all messages with same tag", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:                      "clf-48591",
				namespace:                 esProj,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				serviceAccountName:        "test-clf-" + getRandomString(),
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=http://"+getRouteAddress(oc, ees.namespace, ees.serverName)+":80", "ES_VERSION="+ees.version)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

			g.By("Add pipeline labels to the ClusterLogForwarder instance")
			clf.update(oc, "", "{\"spec\":{\"pipelines\":[{\"inputRefs\":[\"infrastructure\",\"application\",\"audit\"],\"labels\":{\"logging-labels\":\"test-labels\"},\"name\":\"forward-to-external-es\",\"outputRefs\":[\"es-created-by-user\"]}]}}", "--type=merge")

			g.By("Wait for collector pods to pick new ClusterLogForwarder config changes")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("Check logs with pipeline label in external ES")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging-labels\": \"test-labels\"}}}"
			indexName := []string{"app", "infra", "audit"}
			for i := 0; i < len(indexName); i++ {
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
					logs := ees.searchDocByQuery(oc, indexName[i], checkLog)
					if logs.Hits.Total > 0 || len(logs.Hits.DataHits) > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No %s logs found with pipeline label in extranl ES", indexName[i]))
			}

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48593-Vector ClusterLogForwarder Label each message type differently and send all to the same output", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:                      "clf-48593",
				namespace:                 esProj,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "48593.yaml"),
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

			g.By("Add pipeline labels to the ClusterLogForwarder instance")
			clf.update(oc, "", "{\"spec\":{\"pipelines\":[{\"inputRefs\":[\"application\"],\"labels\":{\"logging\":\"app-logs\"},\"name\":\"forward-app-logs\",\"outputRefs\":[\"es-created-by-user\"]},{\"inputRefs\":[\"infrastructure\"],\"labels\":{\"logging\":\"infra-logs\"},\"name\":\"forward-infra-logs\",\"outputRefs\":[\"es-created-by-user\"]},{\"inputRefs\":[\"audit\"],\"labels\":{\"logging\":\"audit-logs\"},\"name\":\"forward-audit-logs\",\"outputRefs\":[\"es-created-by-user\"]}]}}", "--type=merge")

			g.By("Wait for collector pods to pick new ClusterLogForwarder config changes")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("Check logs with pipeline label in external ES")
			indexName := []string{"app", "infra", "audit"}
			for i := 0; i < len(indexName); i++ {
				checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging\": \"" + indexName[i] + "-logs\"}}}"
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
					logs := ees.searchDocByQuery(oc, indexName[i], checkLog)
					if logs.Hits.Total > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No %s logs found with pipeline label in extranl ES", indexName[i]))
			}

		})

		g.It("CPaasrunOnly-Author:ikanse-High-46882-Vector ClusterLogForwarder forward logs to non ClusterLogging managed Elasticsearch insecure forward", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				httpSSL:    false,
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder")
			clf := clusterlogforwarder{
				name:                      "clf-46882",
				namespace:                 esProj,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es.yaml"),
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-High-46920-Vector ClusterLogForwarder forward logs to non ClusterLogging managed Elasticsearch secure forward", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				httpSSL:    true,
				secretName: "ees-https",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:                      "clf-46920",
				namespace:                 esProj,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml"),
				secretName:                ees.secretName,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version, `TLS={"securityProfile": {"type": "Old"}}`)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-55200-Medium-47753-Vector Forward logs to external Elasticsearch with username password HTTP ES 6.x", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				userAuth:   true,
				username:   "user1",
				password:   getRandomString(),
				secretName: "ees-http",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:                      "clf-47753",
				namespace:                 esProj,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml"),
				secretName:                ees.secretName,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				waitForPodReady:           true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-55199-Medium-47755-Vector Forward logs to external Elasticsearch with username password HTTPS ES 7.x", func() {
			oc.SetupProject()
			clfNS := oc.Namespace()

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "7",
				serverName: "elasticsearch-server",
				httpSSL:    true,
				userAuth:   true,
				username:   "user1",
				password:   getRandomString(),
				secretName: "ees-47755",
				loggingNS:  clfNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:                      "clf-55199",
				namespace:                 clfNS,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml"),
				secretName:                ees.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
				enableMonitoring:          true,
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-55201-Medium-47758-Vector Forward logs to external Elasticsearch with username password mTLS ES 8.x", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "8",
				serverName: "elasticsearch-server",
				httpSSL:    true,
				clientAuth: true,
				userAuth:   true,
				username:   "user1",
				password:   getRandomString(),
				secretName: "ees-47758",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:                      "clf-47758",
				namespace:                 esProj,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml"),
				secretName:                ees.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-High-61450-Collector-External Elasticsearch output complies with the tlsSecurityProfile config.[Slow][Disruptive]", func() {

			g.By("Configure the global tlsSecurityProfile to use custom profile")
			ogTLS, er := oc.AsAdmin().WithoutNamespace().Run("get").Args("apiserver/cluster", "-o", "jsonpath={.spec.tlsSecurityProfile}").Output()
			o.Expect(er).NotTo(o.HaveOccurred())
			if ogTLS == "" {
				ogTLS = "null"
			}
			ogPatch := fmt.Sprintf(`[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": %s}]`, ogTLS)
			defer func() {
				oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", ogPatch).Execute()
				waitForOperatorsRunning(oc)
			}()
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256","TLS_AES_128_GCM_SHA256","TLS_AES_256_GCM_SHA384","TLS_CHACHA20_POLY1305_SHA256","ECDHE-ECDSA-AES256-GCM-SHA384","ECDHE-RSA-AES256-GCM-SHA384","ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","DHE-RSA-AES128-GCM-SHA256","DHE-RSA-AES256-GCM-SHA384","DHE-RSA-CHACHA20-POLY1305","ECDHE-ECDSA-AES128-SHA256","ECDHE-RSA-AES128-SHA256","ECDHE-ECDSA-AES128-SHA","ECDHE-RSA-AES128-SHA","ECDHE-ECDSA-AES256-SHA384","ECDHE-RSA-AES256-SHA384","ECDHE-ECDSA-AES256-SHA","ECDHE-RSA-AES256-SHA","DHE-RSA-AES128-SHA256","DHE-RSA-AES256-SHA256","AES128-GCM-SHA256","AES256-GCM-SHA384","AES128-SHA256","AES256-SHA256"],"minTLSVersion":"VersionTLS10"},"type":"Custom"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				httpSSL:    true,
				secretName: "ees-https",
				loggingNS:  esProj,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:                      "clf-61450",
				namespace:                 esProj,
				templateFile:              filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml"),
				secretName:                ees.secretName,
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version)

			g.By("The Elasticsearch sink in Vector config must use the Custom tlsSecurityProfile")
			searchString := `[sinks.output_es_created_by_user.tls]
min_tls_version = "VersionTLS10"
ciphersuites = "ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384,DHE-RSA-CHACHA20-POLY1305,ECDHE-ECDSA-AES128-SHA256,ECDHE-RSA-AES128-SHA256,ECDHE-ECDSA-AES128-SHA,ECDHE-RSA-AES128-SHA,ECDHE-ECDSA-AES256-SHA384,ECDHE-RSA-AES256-SHA384,ECDHE-ECDSA-AES256-SHA,ECDHE-RSA-AES256-SHA,DHE-RSA-AES128-SHA256,DHE-RSA-AES256-SHA256,AES128-GCM-SHA256,AES256-GCM-SHA384,AES128-SHA256,AES256-SHA256"
ca_file = "/var/run/ocp-collector/secrets/ees-https/ca-bundle.crt"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

			g.By("Set Old tlsSecurityProfile for the External ES output.")
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls", "value": {"securityProfile": {"type": "Old"}}}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("The Elasticsearch sink in Vector config must use the Old tlsSecurityProfile")
			searchString = `[sinks.output_es_created_by_user.tls]
min_tls_version = "VersionTLS10"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384,DHE-RSA-CHACHA20-POLY1305,ECDHE-ECDSA-AES128-SHA256,ECDHE-RSA-AES128-SHA256,ECDHE-ECDSA-AES128-SHA,ECDHE-RSA-AES128-SHA,ECDHE-ECDSA-AES256-SHA384,ECDHE-RSA-AES256-SHA384,ECDHE-ECDSA-AES256-SHA,ECDHE-RSA-AES256-SHA,DHE-RSA-AES128-SHA256,DHE-RSA-AES256-SHA256,AES128-GCM-SHA256,AES256-GCM-SHA384,AES128-SHA256,AES256-SHA256,AES128-SHA,AES256-SHA,DES-CBC3-SHA"
ca_file = "/var/run/ocp-collector/secrets/ees-https/ca-bundle.crt"`
			result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external Elasticsearch server.")

			g.By("Delete the Elasticsearch server pod to recollect logs")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-n", esProj, "-l", "app=elasticsearch-server").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForPodReadyWithLabel(oc, esProj, "app=elasticsearch-server")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

	})

	g.Context("Vector ClusterLogging-Managed-Elasticsearch tests", func() {
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
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-47998-Vector Forward logs to multiple log stores[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  loggingNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Create ClusterLogForwarder instance to forward logs to both default and external log store")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-es-and-default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "ES_VERSION="+ees.version)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Check logs in default ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-000")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "infra-000")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "audit-000")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Critical-51740-Vector Preserve k8s Common Labels[Serial]", func() {
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			labels := map[string]string{
				"app.kubernetes.io/name":       "test",
				"app.kubernetes.io/instance":   "functionaltest",
				"app.kubernetes.io/version":    "123",
				"app.kubernetes.io/component":  "thecomponent",
				"app.kubernetes.io/part-of":    "clusterlogging",
				"app.kubernetes.io/managed-by": "clusterloggingoperator",
				"app.kubernetes.io/created-by": "anoperator",
				"run":                          "test-51740",
				"test":                         "test-logging-51740",
			}
			processedLabels := map[string]string{
				"app_kubernetes_io_name":       "test",
				"app_kubernetes_io_instance":   "functionaltest",
				"app_kubernetes_io_version":    "123",
				"app_kubernetes_io_component":  "thecomponent",
				"app_kubernetes_io_part-of":    "clusterlogging",
				"app_kubernetes_io_managed-by": "clusterloggingoperator",
				"app_kubernetes_io_created-by": "anoperator",
				"run":                          "test-51740",
				"test":                         "test-logging-51740",
			}
			labelJSON, _ := json.Marshal(labels)
			labelStr := string(labelJSON)
			app := oc.Namespace()
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
				collectorType: "vector",
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
			o.Expect(len(flatLabelsInLoki) == 0).Should(o.BeTrue())
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-48724-Vector ClusterLogForwarder Message label with multiple log stores.[Serial][Slow]", func() {
			g.By("Deploy a pod to generate some logs")
			appNS := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", appNS).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create Loki project and deploy Loki Server")
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)

			g.By("Deploy zookeeper")
			kafka := kafka{
				namespace:      loggingNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "sasl-plaintext",
				pipelineSecret: "vector-kafka",
				collectorType:  "vector",
				loggingNS:      loggingNS,
			}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "http://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9092/clo-topic"

			g.By("Create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "48724.yaml"),
				secretName:   kafka.pipelineSecret,
			}
			defer clf.delete(oc)
			clf.create(oc, "KAFKA_URL="+kafkaEndpoint, "LOKI_URL=http://"+loki.name+"."+loki.namespace+".svc:3100")

			g.By("Deploy logging pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check data in ES")
			esPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", cl.namespace, "-l", "es-node-master=true", "-ojsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, logType := range []string{"audit", "infra", "app"} {
				waitForIndexAppear(cl.namespace, esPod, logType)
				logs := searchDocByQuery(cl.namespace, esPod, logType, "")
				o.Expect(logs.Hits.DataHits[0].Source.OpenShift.Labels["logtype"] == "all" && logs.Hits.DataHits[0].Source.OpenShift.Labels["logstore"] == "default").To(o.BeTrue(), "labels in CLF are not added to %s logs", logType)
			}

			g.By("Check app logs in kafka consumer pod")
			var logsInKafka []LogEntity
			consumerPods, err := oc.AdminKubeClient().CoreV1().Pods(kafka.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=kafka-consumer"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				logsInKafka, err = getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPods.Items[0].Name, appNS)
				if err != nil {
					return false, err
				}
				return len(logsInKafka) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, "can't find application logs in Kafka")
			o.Expect(logsInKafka[0].OpenShift.Labels["logtype"] == "app-logs" && logsInKafka[0].OpenShift.Labels["logstore"] == "kafka").To(o.BeTrue())

			g.By("check data in Loki")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			for _, logType := range []string{"infrastructure", "audit"} {
				err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
					res, err := lc.searchByKey("", "log_type", logType)
					if err != nil {
						e2e.Logf("\ngot err while querying %s logs: %v\n", logType, err)
						return false, err
					}
					if len(res.Data.Result) > 0 {
						e2e.Logf("%s logs found\n", logType)
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", logType))
			}
			auditLogsInLoki, _ := lc.searchByKey("", "log_type", "audit")
			auditLogs := extractLogEntities(auditLogsInLoki)
			o.Expect(auditLogs[0].OpenShift.Labels["logtype"] == "audit-and-infra-logs" && auditLogs[0].OpenShift.Labels["logstore"] == "loki").To(o.BeTrue())

			infraLogsInLoki, _ := lc.searchByKey("", "log_type", "infrastructure")
			infraLogs := extractLogEntities(infraLogsInLoki)
			o.Expect(infraLogs[0].OpenShift.Labels["logtype"] == "audit-and-infra-logs" && infraLogs[0].OpenShift.Labels["logstore"] == "loki").To(o.BeTrue())

		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-52128-Vector Send JSON logs from containers in the same pod to separate indices -- outputDefaults[Serial][Slow]", func() {
			app := oc.Namespace()
			containerName := "log-52128-" + getRandomString()
			multiContainerJSONLog := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", multiContainerJSONLog, "-n", app, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-output-default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "STRUCTURED_CONTAINER=true")

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check indices in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-0")
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-1")
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "app-"+app)

			queryContainerLog := func(container string) string {
				return "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.container_name\": \"" + container + "\"}}}"
			}

			// in index $containerName-0, only logs in container $containerName-0 are stored in it, and json logs are parsed
			log0 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-0", queryContainerLog(containerName+"-0"))
			o.Expect(len(log0.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log0.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log01 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-0", queryContainerLog(containerName+"-1"))
			o.Expect(len(log01.Hits.DataHits) == 0).To(o.BeTrue())
			log02 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-0", queryContainerLog(containerName+"-2"))
			o.Expect(len(log02.Hits.DataHits) == 0).To(o.BeTrue())

			// in index $containerName-1, only logs in container $containerName-1 are stored in it, and json logs are parsed
			log1 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-1", queryContainerLog(containerName+"-1"))
			o.Expect(len(log1.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log1.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log10 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-1", queryContainerLog(containerName+"-0"))
			o.Expect(len(log10.Hits.DataHits) == 0).To(o.BeTrue())
			log12 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+containerName+"-1", queryContainerLog(containerName+"-2"))
			o.Expect(len(log12.Hits.DataHits) == 0).To(o.BeTrue())

			// in index app-$app-project, only logs in container $containerName-2 are stored in it, and json logs are parsed
			log2 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+app, queryContainerLog(containerName+"-2"))
			o.Expect(len(log2.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log20 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+app, queryContainerLog(containerName+"-0"))
			o.Expect(len(log20.Hits.DataHits) == 0).To(o.BeTrue())
			log21 := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+app, queryContainerLog(containerName+"-1"))
			o.Expect(len(log21.Hits.DataHits) == 0).To(o.BeTrue())

		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-52130-Vector JSON logs from containers in the same pod are not sent to separate indices when enableStructuredContainerLogs is false[Serial]", func() {
			app := oc.Namespace()
			containerName := "log-52130-" + getRandomString()
			multiContainerJSONLog := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", multiContainerJSONLog, "-n", app, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-output-default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "STRUCTURED_CONTAINER=false")

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check indices in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "app-"+app)

			indices, err := getESIndices(cl.namespace, esPods.Items[0].Name)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, index := range indices {
				o.Expect(index.Index).NotTo(o.ContainSubstring(containerName))
			}

			// logs in container-0, container-1 and contianer-2 are stored in index app-$app-project, and json logs are parsed
			for _, container := range []string{containerName + "-0", containerName + "-1", containerName + "-2"} {
				log := searchDocByQuery(cl.namespace, esPods.Items[0].Name, "app-"+app, "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.container_name\": \""+container+"\"}}}")
				o.Expect(len(log.Hits.DataHits) > 0).To(o.BeTrue())
				o.Expect(log.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			}
		})

		g.It("CPaasrunOnly-Author:ikanse-High-50517-High-51846-Vector Structured index by kubernetes.labels and send unmatched JSON logs to fallback index outputDefaults[Serial][Slow]", func() {

			g.By("Create project1 for app logs and deploy the log generator app with test=centos-logtest-qa and run=centos-logtest-stage labels")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			jsonLogFileUnAnnoted := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template_unannoted.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-p", "LABELS={\"test\": \"centos-logtest-qa\"}", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-qa", "-p", "CONFIGMAP=logtest-config-qa", "-p", "CONTAINER=logging-centos-qa", "-f", jsonLogFileUnAnnoted).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-p", "LABELS={\"run\": \"centos-logtest-stage\"}", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-stage", "-p", "CONFIGMAP=logtest-config-stage", "-p", "CONTAINER=logging-centos-stage", "-f", jsonLogFileUnAnnoted).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create project2 for app logs and deploy the log generator app with test=centos-logtest-dev label")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-p", "LABELS={\"test\": \"centos-logtest-dev\"}", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-dev", "-p", "CONFIGMAP=logtest-config-dev", "-p", "CONTAINER=logging-centos-dev", "-f", jsonLogFileUnAnnoted).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-output-default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "STRUCTURED_CONTAINER=true", "TYPE_KEY=kubernetes.labels.test", "TYPE_NAME=qa-fallback-index")

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check the indices in ES for structured logs")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-centos-logtest-qa-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-centos-logtest-dev-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-qa-fallback-index-00")

			g.By("Check for logs with label test=centos-logtest-qa")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"test=centos-logtest-qa\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest-qa-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with test=centos-logtest-qa in logs not found in default ES index app-centos-logtest-qa-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest-dev-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-qa in logs are in app-centos-logtest-dev-00* index")
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-qa-fallback-index-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-qa in logs are in app-qa-fallback-index-00* index")

			g.By("Check for logs with label test=centos-logtest-dev")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"test=centos-logtest-dev\"}}}"
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest-dev-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with test=centos-logtest-dev in logs not found in default ES index app-centos-logtest-dev-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest-qa-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-dev in logs are in app-centos-logtest-qa-00* index")
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-qa-fallback-index-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-dev in logs are in app-qa-fallback-index-00* index")

			g.By("Unmatch logs with label run=centos-logtest-stage should be sent to app-qa-fallback index")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-stage\"}}}"
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-qa-fallback-index-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with run=centos-logtest-stage in logs not found in default ES app-qa-fallback-index-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest-qa-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with run=centos-logtest-stage in logs are in app-centos-logtest-qa-00* index")
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest-dev-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with run=centos-logtest-stage in logs are in app-centos-logtest-dev-99* index")
		})

		g.It("CPaasrunOnly-Author:ikanse-High-50510-Vector Structured index by kubernetes.container_name outputDefaults[Serial][Slow]", func() {

			g.By("Create project-qa for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			jsonLogFileUnAnnoted := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template_unannoted.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-p", "LABELS={\"test\": \"centos-logtest-qa\"}", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-qa", "-p", "CONFIGMAP=logtest-config-qa", "-p", "CONTAINER=logging-centos-qa", "-f", jsonLogFileUnAnnoted).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-output-default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "STRUCTURED_CONTAINER=false", "TYPE_KEY=kubernetes.container_name", "TYPE_NAME=qa-index-name")

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check the indices in ES for structured logs")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-logging-centos-qa-00")

			g.By("Check for logs with kubernetes.container_name=logging-centos-qa")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.container_name\": \"logging-centos-qa\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-logging-centos-qa-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Logs with container name logging-centos-qa not found in ES index app-logging-centos-qa-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-qa-index-name-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Logs with container name logging-centos-qa found in ES app-qa-index-name-00*")
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Logs with container name logging-centos-qa found in ES app-00")
		})

		g.It("CPaasrunOnly-Author:ikanse-High-51844-High-50511-Vector Structured index by kubernetes.namespace_name and default index outputDefaults[Serial][Slow]", func() {

			g.By("Create project-qa for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			jsonLogFileUnAnnoted := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template_unannoted.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-p", "LABELS={\"test\": \"centos-logtest-qa\"}", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-qa", "-p", "CONFIGMAP=logtest-config-qa", "-p", "CONTAINER=logging-centos-qa", "-f", jsonLogFileUnAnnoted).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-output-default-all-logs.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "PROJECT_NAME="+appProj, "STRUCTURED_CONTAINER=false", "TYPE_KEY=kubernetes.namespace_name", "TYPE_NAME=qa-index-name")

			g.By("Create ClusterLogging instance with Vector as collector")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "vector",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check the indices in ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-"+appProj+"-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "infra-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "audit-00")

			g.By("Check for logs with kubernetes.namespace_name " + appProj + "")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-"+appProj+"-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Logs with namespace name "+appProj+" not found in ES index app-"+appProj+"-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-qa-index-name-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Logs with namespace name "+appProj+" found in ES app-qa-index-name-00*")
			logs = searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Logs with namespace name "+appProj+" not found in ES index app-00*")
		})

	})

})
