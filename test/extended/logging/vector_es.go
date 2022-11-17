package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("vector-es-namespace", exutil.KubeConfigPath())

	g.Context("Vector collector tests", func() {
		cloNS := "openshift-logging"
		g.BeforeEach(func() {
			g.By("deploy CLO and EO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			EO := SubscriptionObjects{
				OperatorName:  "elasticsearch-operator",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "elasticsearch-operator",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
			EO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-49368-Low-46880-Vector Deploy Cluster Logging with Vector as collector using CLI and exclude Vector logs from collection[Serial]", func() {

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Make sure the Elasticsearch cluster is healthy")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Check Vector status")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cloNS}
			pl.checkLogsFromRs(oc, "Healthcheck: Passed", "collector")
			pl.checkLogsFromRs(oc, "Vector has started", "collector")

			g.By("Check app indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-000")

			g.By("Check infra indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "infra-000")

			g.By("Check for Vector logs in Elasticsearch")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.container_name\": \"collector\"}}}"
			logs := searchDocByQuery(cloNS, podList.Items[0].Name, "*", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Vector logs should not be collected")
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-49390-Vector Collecting Kubernetes events using event router[Serial][Slow]", func() {

			eventrouterTemplate := exutil.FixturePath("testdata", "logging", "eventrouter", "eventrouter.yaml")

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Create project for app logs and deploy the log generator app")
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Make sure the Elasticsearch cluster is healthy")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Deploy the Event Router")
			evt := resource{"deployment", "eventrouter", cloNS}
			defer deleteEventRouter(oc, cloNS)
			evt.createEventRouter(oc, "-f", eventrouterTemplate)

			g.By("Check event logs in the Event Router pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=eventrouter"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cloNS}
			pl.checkLogsFromRs(oc, "ADDED", "kube-eventrouter")
			pl.checkLogsFromRs(oc, "Update", "kube-eventrouter")

			g.By("Check for Event Router logs in Elasticsearch")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"component=eventrouter\"}}}"
			err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
				logs := searchDocByQuery(cloNS, podList.Items[0].Name, "infra", checkLog)
				if logs.Hits.Total > 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "No Event Router logs found when using vector as log collector.")
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-53311-Vector Configure Vector collector with CPU/Memory requests/limits[Serial]", func() {

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "COLLECTOR_LIMITS_MEMORY=750Mi", "-p", "COLLECTOR_LIMITS_CPU=150m", "-p", "COLLECTOR_REQUESTS_CPU=150m", "-p", "COLLECTOR_REQUESTS_MEMORY=750Mi", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check collector CPU/memory requests/limits")
			col := resource{"daemonset", "collector", cloNS}
			col.assertResourceStatus(oc, "jsonpath={.spec.template.spec.containers[].resources.limits.cpu}", "150m")
			col.assertResourceStatus(oc, "jsonpath={.spec.template.spec.containers[].resources.requests.cpu}", "150m")
			col.assertResourceStatus(oc, "jsonpath={.spec.template.spec.containers[].resources.limits.memory}", "750Mi")
			col.assertResourceStatus(oc, "jsonpath={.spec.template.spec.containers[].resources.requests.memory}", "750Mi")
		})

		g.It("CPaasrunOnly-Author:ikanse-High-53312-Vector Deploy Vector collector with nodeSelector[Serial]", func() {

			g.By("Set OCP node label to vector: test")
			err := oc.AsAdmin().WithoutNamespace().Run("label").Args("nodes", "--selector=kubernetes.io/os=linux", "vector=test", "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-collector-nodeselector.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NODE_LABEL={\"vector\": \"deploy\"}", "-p", "NAMESPACE="+cl.namespace)

			g.By("Check Collector daemonset has no running pods")
			esDeployNames := GetDeploymentsNameByLabel(oc, cloNS, "cluster-name=elasticsearch")
			for _, name := range esDeployNames {
				WaitForDeploymentPodsToBeReady(oc, cloNS, name)
			}
			WaitForDeploymentPodsToBeReady(oc, cloNS, "kibana")
			var output string
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				daemonset, err := oc.AdminKubeClient().AppsV1().DaemonSets(cloNS).Get(context.Background(), "collector", metav1.GetOptions{})
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
				output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("ds/collector", "-n", cloNS, "-ojsonpath={.status}").Output()
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Daemonset collector is not availabile with output:\n %s", output))

			g.By("Set OCP node label to vector: deploy")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("nodes", "--selector=kubernetes.io/os=linux", "vector=deploy", "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check Collector daemonset pods are running")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Set OCP node label to vector: deploy1")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("nodes", "--selector=kubernetes.io/os=linux", "vector=deploy1", "--overwrite").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check Collector daemonset has no running pods")
			esDeployNames = GetDeploymentsNameByLabel(oc, cloNS, "cluster-name=elasticsearch")
			for _, name := range esDeployNames {
				WaitForDeploymentPodsToBeReady(oc, cloNS, name)
			}
			WaitForDeploymentPodsToBeReady(oc, cloNS, "kibana")
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				daemonset, err := oc.AdminKubeClient().AppsV1().DaemonSets(cloNS).Get(context.Background(), "collector", metav1.GetOptions{})
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
				output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("ds/collector", "-n", cloNS, "-ojsonpath={.status}").Output()
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Daemonset collector is not availabile with output:\n %s", output))

			g.By("Patch Cluster Logging instance to use nodeSelector vector: deploy1")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterlogging", "instance", "--type=merge", "-p", "{\"spec\":{\"collection\":{\"nodeSelector\":{\"vector\":\"deploy1\"}}}}", "-n", cloNS).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check Collector daemonset pods are running")
			WaitForECKPodsToBeReady(oc, cloNS)
		})

		g.It("CPaasrunOnly-Author:ikanse-High-53995-Vector Collect OVN audit logs [Serial]", func() {

			g.By("Check the network type for the test")
			networkType := checkNetworkType(oc)
			if !strings.Contains(networkType, "ovnkubernetes") {
				g.Skip("Skip for non-supported network type, type is not OVNKubernetes!!!")
			}

			g.By("Create clusterlogforwarder instance to forward OVN audit logs to default Elasticsearch instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "forward_to_default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err := clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy ClusterLogging instance.")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check audit index in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, esPods.Items[0].Name, "audit-00")

			g.By("Create a test project, enable OVN network log collection on it, add the OVN log app and network policies for the project")
			oc.SetupProject()
			ovnProj := oc.Namespace()
			ovn := resource{"deployment", "ovn-app", ovnProj}
			esTemplate := exutil.FixturePath("testdata", "logging", "generatelog", "42981.yaml")
			err = ovn.applyFromTemplate(oc, "-n", ovn.namespace, "-f", esTemplate, "-p", "NAMESPACE="+ovn.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForDeploymentPodsToBeReady(oc, ovnProj, ovn.name)

			g.By("Access the OVN app pod from another pod in the same project to generate OVN ACL messages")
			ovnPods, err := oc.AdminKubeClient().CoreV1().Pods(ovnProj).List(context.Background(), metav1.ListOptions{LabelSelector: "app=ovn-app"})
			o.Expect(err).NotTo(o.HaveOccurred())
			podIP := ovnPods.Items[0].Status.PodIP
			e2e.Logf("Pod IP is %s ", podIP)
			ovnCurl := "curl " + podIP + ":8080"
			_, err = e2e.RunHostCmdWithRetries(ovnProj, ovnPods.Items[1].Name, ovnCurl, 3*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check for the generated OVN audit logs on the OpenShift cluster nodes")
			nodeLogs, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", ovnProj, "node-logs", "-l", "beta.kubernetes.io/os=linux", "--path=/ovn/acl-audit-log.log").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(nodeLogs).Should(o.ContainSubstring(ovnProj), "The OVN logs doesn't contain logs from project %s", ovnProj)

			g.By("Check for the generated OVN audit logs in Elasticsearch")
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
				cmd := "es_util --query=audit*/_search?format=JSON -d '{\"query\":{\"query_string\":{\"query\":\"verdict=allow AND severity=alert AND tcp,vlan_tci AND tcp_flags=ack\",\"default_field\":\"message\"}}}'"
				stdout, err := e2e.RunHostCmdWithRetries(cloNS, esPods.Items[0].Name, cmd, 3*time.Second, 30*time.Second)
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
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"query\": {\"bool\":{\"must_not\":{\"exists\":{\"field\":\"@timestamp\"}}}}}"
			logs := searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
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
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By(fmt.Sprintf("Make sure that the collector pod is not scheduled on the %s", taintNode))
			chkCollector, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", cl.namespace, "pods", "--selector=component=collector", "--field-selector", "spec.nodeName="+taintNode+"", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(chkCollector).Should(o.BeEmpty())

			g.By("Add toleration for collector for the taint vector=deploy:NoSchedule")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterlogging/instance", "-n", cl.namespace, "-p", "{\"spec\":{\"collection\":{\"tolerations\":[{\"effect\":\"NoSchedule\",\"key\":\"vector\",\"operator\":\"Equal\",\"value\":\"deploy\"}]}}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By(fmt.Sprintf("Make sure that the collector pod is scheduled on the node %s after applying toleration for taint vector=deploy:NoSchedule", taintNode))
			chkCollector, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", cl.namespace, "pods", "--selector=component=collector", "--field-selector", "spec.nodeName="+taintNode+"", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(chkCollector).Should(o.ContainSubstring("collector"))
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-47063-Vector ServiceMonitor object for Vector is deployed along with Cluster Logging[Serial]", func() {

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By("Make sure the Elasticsearch cluster is healthy")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Check Vector status")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cl.namespace}
			pl.checkLogsFromRs(oc, "Healthcheck: Passed", "collector")
			pl.checkLogsFromRs(oc, "Vector has started", "collector")

			g.By("Check if the ServiceMonitor object for Vector is created.")
			resource{"servicemonitor", "collector", cl.namespace}.WaitForResourceToAppear(oc)

			g.By("Check the Vector metrics")
			bearerToken := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			output, err := e2e.RunHostCmdWithRetries(cl.namespace, podList.Items[0].Name, "curl -k -H \"Authorization: Bearer "+bearerToken+"\" -H \"Content-type: application/json\" https://collector:24231/metrics", 10*time.Second, 20*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("vector_component_received_event_bytes_total"))

			g.By("Check the Log file metrics exporter metrics")
			output, err = e2e.RunHostCmdWithRetries(cl.namespace, podList.Items[0].Name, "curl -k -H \"Authorization: Bearer "+bearerToken+"\" -H \"Content-type: application/json\" https://collector:2112/metrics", 10*time.Second, 20*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("log_logged_bytes_total"))

			g.By("Check Vector and Log File metrics exporter metrics from Prometheus")
			for _, metric := range []string{"log_logged_bytes_total", "vector_processed_bytes_total"} {
				err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
					result, err := queryPrometheus(oc, "", "/api/v1/query?", metric, "GET")
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
		g.It("CPaasrunOnly-Author:qitang-Critical-47060-All node logs have been sent to Elasticsearch[Serial]", func() {
			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", "openshift-logging"}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

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
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
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
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				journalLog, err := queryInES(cl.namespace, esPods.Items[0].Name, queryJournalLog)
				if err != nil {
					return false, err
				}
				if journalLog.Hits.Total <= 0 {
					return false, nil
				}
				journalBuckets := journalLog.Aggregations.LoggingAggregations.InnerAggregations.Buckets
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

	})

	g.Context("Vector Elasticsearch tests", func() {
		cloNS := "openshift-logging"
		g.BeforeEach(func() {
			g.By("deploy CLO and EO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			EO := SubscriptionObjects{
				OperatorName:  "elasticsearch-operator",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "elasticsearch-operator",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
			EO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48591-Vector ClusterLogForwarder Label all messages with same tag[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-exteranl-es-and-default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

			g.By("Check logs in default ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-000")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "infra-000")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "audit-000")

			g.By("Add pipeline labels to the ClusterLogForwarder instance")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterlogforwarders/instance", "-n", cloNS, "-p", "{\"spec\":{\"pipelines\":[{\"inputRefs\":[\"infrastructure\",\"application\",\"audit\"],\"labels\":{\"logging-labels\":\"test-labels\"},\"name\":\"forward-to-external-es\",\"outputRefs\":[\"es-created-by-user\",\"default\"]}]}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for collector pods to pick new ClusterLogForwarder config changes")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check logs with pipeline label in external ES")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging-labels\": \"test-labels\"}}}"
			indexName := []string{"app", "infra", "audit"}
			for i := 0; i < len(indexName); i++ {
				err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
					logs := ees.searchDocByQuery(oc, indexName[i], checkLog)
					if logs.Hits.Total > 0 || len(logs.Hits.DataHits) > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No %s logs found with pipeline label in extranl ES", indexName[i]))
			}

			g.By("Check logs with pipeline label in default ES")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging-labels\": \"test-labels\"}}}"
			for i := 0; i < len(indexName); i++ {
				err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
					logs := searchDocByQuery(cloNS, podList.Items[0].Name, indexName[i], checkLog)
					if logs.Hits.Total > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No %s logs found with pipeline label in default ES instance", indexName[i]))
			}

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48593-Vector ClusterLogForwarder Label each message type differently and send all to the same output[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "48593.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

			g.By("Check logs in default ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-000")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "infra-000")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "audit-000")

			g.By("Add pipeline labels to the ClusterLogForwarder instance")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterlogforwarders/instance", "-n", cloNS, "-p", "{\"spec\":{\"pipelines\":[{\"inputRefs\":[\"application\"],\"labels\":{\"logging\":\"app-logs\"},\"name\":\"forward-app-logs\",\"outputRefs\":[\"es-created-by-user\",\"default\"]},{\"inputRefs\":[\"infrastructure\"],\"labels\":{\"logging\":\"infra-logs\"},\"name\":\"forward-infra-logs\",\"outputRefs\":[\"es-created-by-user\",\"default\"]},{\"inputRefs\":[\"audit\"],\"labels\":{\"logging\":\"audit-logs\"},\"name\":\"forward-audit-logs\",\"outputRefs\":[\"es-created-by-user\",\"default\"]}]}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for collector pods to pick new ClusterLogForwarder config changes")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check logs with pipeline label in external ES")
			indexName := []string{"app", "infra", "audit"}
			for i := 0; i < len(indexName); i++ {
				checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging\": \"" + indexName[i] + "-logs\"}}}"
				err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
					logs := ees.searchDocByQuery(oc, indexName[i], checkLog)
					if logs.Hits.Total > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No %s logs found with pipeline label in extranl ES", indexName[i]))
			}

			g.By("Check logs with pipeline label in default ES")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			for i := 0; i < len(indexName); i++ {
				checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging\": \"" + indexName[i] + "-logs\"}}}"
				err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
					logs := searchDocByQuery(cloNS, podList.Items[0].Name, indexName[i], checkLog)
					if logs.Hits.Total > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No %s logs found with pipeline label in default ES instance", indexName[i]))
			}

		})

		g.It("CPaasrunOnly-Author:ikanse-High-46882-Vector ClusterLogForwarder forward logs to non ClusterLogging managed Elasticsearch insecure forward[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				httpSSL:    false,
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-es.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-High-46920-Vector ClusterLogForwarder forward logs to non ClusterLogging managed Elasticsearch secure forward[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				httpSSL:    true,
				secretName: "ees-https",
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_SECRET="+ees.secretName, "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-High-48768-Vector ClusterLogForwarder Collect app logs from a predefined namespace[Serial][Slow]", func() {

			g.By("Create project1 for app logs and deploy the log generator app")
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create project2 for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "48768.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "APP_PROJECT="+appProj1)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check the app index in default ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-000")

			g.By("Check for project1 logs in default ES")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj1 + "\"}}}"
			logs := searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Project1 %s logs not found in default ES", appProj1)

			g.By("Check for project2 logs in default ES")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj2 + "\"}}}"
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Projec2 %s logs should not be collected", appProj2)

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48786-Vector Forward logs from specified pods using label selector[Serial][Slow]", func() {

			g.By("Create project1 for app logs and deploy the log generator app with run=centos-logtest-qa label")
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-p", "LABELS={\"run\": \"centos-logtest-qa\"}", "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create project2 for app logs and deploy the log generator app with run=centos-logtest-dev label")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-p", "LABELS={\"run\": \"centos-logtest-dev\"}", "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "48786.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check the app index in default ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-000")

			g.By("Check for logs with run=centos-logtest-qa labels")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-qa\"}}}"
			logs := searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with run=centos-logtest-qa in logs not found in default ES")

			g.By("Check for logs with run=centos-logtest-dev labels")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-dev\"}}}"
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Logs with label with run=centos-logtest-dev should not be collected")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48787-Vector Forward logs from specified pods using label and namespace selectors[Serial][Slow]", func() {

			g.By("Create project1 for app logs and deploy the log generator app with run=centos-logtest-qa and run=centos-logtest-stage labels")
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-p", "LABELS={\"run\": \"centos-logtest-qa\"}", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-qa", "-p", "CONFIGMAP=logtest-config-qa", "-p", "CONTAINER=logging-centos-qa", "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-p", "LABELS={\"run\": \"centos-logtest-stage\"}", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-stage", "-p", "CONFIGMAP=logtest-config-stage", "-p", "CONTAINER=logging-centos-stage", "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create project2 for app logs and deploy the log generator app with run=centos-logtest-dev label")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-p", "LABELS={\"run\": \"centos-logtest-dev\"}", "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "48787.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-p", "APP_NAMESPACE="+appProj1+"", "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check the app index in default ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-000")

			g.By("Check for logs with run=centos-logtest-qa in label")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-qa\"}}}"
			logs := searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with run=centos-logtest-qa in logs not found in default ES")

			g.By("Check for logs with run=centos-logtest-stage in label")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-stage\"}}}"
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with run=centos-logtest-dev in logs should not be collected")

			g.By("Check for logs with run=centos-logtest-dev label")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-dev\"}}}"
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with run=centos-logtest-dev in logs should not be collected")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-55200-Medium-47753-Vector Forward logs to external Elasticsearch with username password HTTP ES 6.x[Serial][Slow]", func() {

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
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_SECRET="+ees.secretName, "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-55199-Medium-47755-Vector Forward logs to external Elasticsearch with username password HTTPS ES 7.x[Serial][Slow]", func() {

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
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_SECRET="+ees.secretName, "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-55201-Medium-47758-Vector Forward logs to external Elasticsearch with username password mTLS ES 8.x[Serial][Slow]", func() {

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
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-es-pipelinesecret.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_SECRET="+ees.secretName, "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-47998-Vector Forward logs to multiple log stores[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance to forward logs to both default and external log store")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-exteranl-es-and-default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check logs in default ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-000")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "infra-000")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "audit-000")

			g.By("Check logs in external ES")
			ees.waitForIndexAppear(oc, "app")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")

		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Critical-51740-Vector Preserve k8s Common Labels[Serial]", func() {
			loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-external-loki.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			inputs, _ := json.Marshal([]string{"application"})
			outputs, _ := json.Marshal([]string{"loki-server", "default"})
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "-p", "INPUTREFS="+string(inputs), "-p", "OUTPUTREFS="+string(outputs))
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			g.By("waiting for the ECK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

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
			o.Expect(reflect.DeepEqual(labels, flatLabelsMap)).Should(o.BeTrue())

			g.By("check data in Loki")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			err = wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
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
			o.Expect(reflect.DeepEqual(labels, k8sLabelsInLoki)).Should(o.BeTrue())
			flatLabelsInLoki := lokiLog[0].Kubernetes.FlatLabels
			o.Expect(len(flatLabelsInLoki) == 0).Should(o.BeTrue())
		})

	})

	g.Context("JSON log tests", func() {
		cloNS := "openshift-logging"
		g.BeforeEach(func() {
			g.By("deploy CLO and EO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			EO := SubscriptionObjects{
				OperatorName:  "elasticsearch-operator",
				Namespace:     "openshift-operators-redhat",
				PackageName:   "elasticsearch-operator",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
			EO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-52128-Vector Send JSON logs from containers in the same pod to separate indices -- outputDefaults[Serial][Slow]", func() {
			app := oc.Namespace()
			containerName := "log-52128-" + getRandomString()
			multiContainerJSONLog := exutil.FixturePath("testdata", "logging", "generatelog", "multi_container_json_log_template.yaml")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", multiContainerJSONLog, "-n", app, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "structured-container-output-default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "STRUCTURED_CONTAINER=true")
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			g.By("waiting for the ECK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("check indices in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, esPods.Items[0].Name, "app-"+containerName+"-0")
			waitForIndexAppear(cloNS, esPods.Items[0].Name, "app-"+containerName+"-1")
			waitForIndexAppear(cloNS, esPods.Items[0].Name, "app-"+app)

			queryContainerLog := func(container string) string {
				return "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.container_name\": \"" + container + "\"}}}"
			}

			// in index $containerName-0, only logs in container $containerName-0 are stored in it, and json logs are parsed
			log0 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+containerName+"-0", queryContainerLog(containerName+"-0"))
			o.Expect(len(log0.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log0.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log01 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+containerName+"-0", queryContainerLog(containerName+"-1"))
			o.Expect(len(log01.Hits.DataHits) == 0).To(o.BeTrue())
			log02 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+containerName+"-0", queryContainerLog(containerName+"-2"))
			o.Expect(len(log02.Hits.DataHits) == 0).To(o.BeTrue())

			// in index $containerName-1, only logs in container $containerName-1 are stored in it, and json logs are parsed
			log1 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+containerName+"-1", queryContainerLog(containerName+"-1"))
			o.Expect(len(log1.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log1.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log10 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+containerName+"-1", queryContainerLog(containerName+"-0"))
			o.Expect(len(log10.Hits.DataHits) == 0).To(o.BeTrue())
			log12 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+containerName+"-1", queryContainerLog(containerName+"-2"))
			o.Expect(len(log12.Hits.DataHits) == 0).To(o.BeTrue())

			// in index app-$app-project, only logs in container $containerName-2 are stored in it, and json logs are parsed
			log2 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+app, queryContainerLog(containerName+"-2"))
			o.Expect(len(log2.Hits.DataHits) > 0).To(o.BeTrue())
			o.Expect(log2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			log20 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+app, queryContainerLog(containerName+"-0"))
			o.Expect(len(log20.Hits.DataHits) == 0).To(o.BeTrue())
			log21 := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+app, queryContainerLog(containerName+"-1"))
			o.Expect(len(log21.Hits.DataHits) == 0).To(o.BeTrue())

		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-52129-Vector Send JSON logs from containers in the same pod to separate indices[Serial]", func() {
			app := oc.Namespace()
			containerName := "log-52129-" + getRandomString()
			multiContainerJSONLog := exutil.FixturePath("testdata", "logging", "generatelog", "multi_container_json_log_template.yaml")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", multiContainerJSONLog, "-n", app, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "external-es",
				httpSSL:    true,
				secretName: "json-log-52129",
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)
			eesURL := "https://" + ees.serverName + "." + ees.namespace + ".svc:9200"

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "structured-container-logs.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "STRUCTURED_CONTAINER=true", "URL="+eesURL, "SECRET="+ees.secretName, "-p", "ES_VERSION="+ees.version)
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

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
		g.It("CPaasrunOnly-Author:qitang-Medium-52130-Vector JSON logs from containers in the same pod are not sent to separate indices when enableStructuredContainerLogs is false[Serial]", func() {
			app := oc.Namespace()
			containerName := "log-52130-" + getRandomString()
			multiContainerJSONLog := exutil.FixturePath("testdata", "logging", "generatelog", "multi_container_json_log_template.yaml")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", multiContainerJSONLog, "-n", app, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "structured-container-output-default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "STRUCTURED_CONTAINER=false")
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			g.By("waiting for the ECK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("check indices in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, esPods.Items[0].Name, "app-"+app)

			indices, err := getESIndices(cloNS, esPods.Items[0].Name)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, index := range indices {
				o.Expect(index.Index).NotTo(o.ContainSubstring(containerName))
			}

			// logs in container-0, container-1 and contianer-2 are stored in index app-$app-project, and json logs are parsed
			for _, container := range []string{containerName + "-0", containerName + "-1", containerName + "-2"} {
				log := searchDocByQuery(cloNS, esPods.Items[0].Name, "app-"+app, "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.container_name\": \""+container+"\"}}}")
				o.Expect(len(log.Hits.DataHits) > 0).To(o.BeTrue())
				o.Expect(log.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			}
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-52131-Vector Logs from different projects are forwarded to the same index if the pods have same annotation[Serial]", func() {
			containerName := "log-52131-" + getRandomString()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			app1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app1, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			app2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app2, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "structured-container-output-default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "STRUCTURED_CONTAINER=true")
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=vector")
			g.By("waiting for the ECK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("check indices in ES pod")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, esPods.Items[0].Name, "app-"+containerName)

			g.By("check data in ES")
			for _, proj := range []string{app1, app2} {
				count, err := getDocCountByQuery(cloNS, esPods.Items[0].Name, "app-"+containerName, "{\"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+proj+"\"}}}")
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(count > 0).To(o.BeTrue())
			}
		})

		g.It("CPaasrunOnly-Author:ikanse-High-50517-High-51846-Vector Structured index by kubernetes.labels and send unmatched JSON logs to fallback index outputDefaults[Serial][Slow]", func() {

			g.By("Create project1 for app logs and deploy the log generator app with test=centos-logtest-qa and run=centos-logtest-stage labels")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			jsonLogFileUnAnnoted := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template_unannoted.json")
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
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "structured-container-output-default.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "STRUCTURED_CONTAINER=true", "-p", "TYPE_KEY=kubernetes.labels.test", "-p", "TYPE_NAME=qa-fallback-index")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogging instance with Vector as collector")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			g.By("Waiting for the Logging pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check the indices in ES for structured logs")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-centos-logtest-qa-00")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-centos-logtest-dev-00")
			waitForIndexAppear(cloNS, podList.Items[0].Name, "app-qa-fallback-index-00")

			g.By("Check for logs with label test=centos-logtest-qa")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"test=centos-logtest-qa\"}}}"
			logs := searchDocByQuery(cloNS, podList.Items[0].Name, "app-centos-logtest-qa-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with test=centos-logtest-qa in logs not found in default ES index app-centos-logtest-qa-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-centos-logtest-dev-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-qa in logs are in app-centos-logtest-dev-00* index")
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-qa-fallback-index-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-qa in logs are in app-qa-fallback-index-00* index")

			g.By("Check for logs with label test=centos-logtest-dev")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"test=centos-logtest-dev\"}}}"
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-centos-logtest-dev-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with test=centos-logtest-dev in logs not found in default ES index app-centos-logtest-dev-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-centos-logtest-qa-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-dev in logs are in app-centos-logtest-qa-00* index")
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-qa-fallback-index-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with test=centos-logtest-dev in logs are in app-qa-fallback-index-00* index")

			g.By("Unmatch logs with label run=centos-logtest-stage should be sent to app-qa-fallback index")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-stage\"}}}"
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-qa-fallback-index-00", checkLog)
			o.Expect(logs.Hits.Total == 0).ShouldNot(o.BeTrue(), "Labels with run=centos-logtest-stage in logs not found in default ES app-qa-fallback-index-00*")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-centos-logtest-qa-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with run=centos-logtest-stage in logs are in app-centos-logtest-qa-00* index")
			logs = searchDocByQuery(cloNS, podList.Items[0].Name, "app-centos-logtest-dev-00", checkLog)
			o.Expect(logs.Hits.Total == 0).Should(o.BeTrue(), "Labels with run=centos-logtest-stage in logs are in app-centos-logtest-dev-99* index")
		})

	})

})
