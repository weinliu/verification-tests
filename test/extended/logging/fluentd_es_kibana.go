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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("logging-es", exutil.KubeConfigPath())

	g.Context("Cluster Logging Instance tests", func() {
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
		})

		// author ikanse@redhat.com
		g.It("CPaasrunOnly-Author:ikanse-Medium-36368-Elasticsearch nodes can scale down[Serial][Slow]", func() {
			// create clusterlogging instance with elasticsearch node count set to 3
			g.By("deploy EFK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc, "-p", "ES_NODE_COUNT=3", "-p", "REDUNDANCY_POLICY=SingleRedundancy")

			e2e.Logf("Start testing OCP-36368-Elasticsearch nodes can scale down")
			//Wait for EFK pods to be ready
			g.By("Waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check the elasticsearch node count")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.numNodes}", "3")

			g.By("Check the elasticsearch cluster health")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Set elasticsearch node count to 2")
			er := oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterlogging/instance", "-n", "openshift-logging", "-p", "{\"spec\": {\"logStore\": {\"elasticsearch\": {\"nodeCount\":2}}}}", "--type=merge").Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Check the elasticsearch node count")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.numNodes}", "2")

			g.By("Check the elasticsearch cluster health")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")
		})

		// author ikanse@redhat.com
		g.It("CPaasrunOnly-Author:ikanse-Medium-43065-Drop log messages after explicit time[Serial][Slow]", func() {

			g.By(" Create a Cluster Logging instance with Fluentd buffer retryTimeout set to 1 minute.")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "43065.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc, "-p", "ES_NODE_COUNT=1", "-p", "REDUNDANCY_POLICY=ZeroRedundancy", "-p", "FLUENTD_BUFFER_RETRYTIMEOUT=1m")

			g.By("Waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Make sure the Elasticsearch cluster is healthy")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")
			prePodList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, prePodList.Items[0].Name, "infra-00")

			g.By("Set the Elasticsearch operator instance managementState to Unmanaged.")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("es/elasticsearch", "-n", cloNS, "-p", "{\"spec\": {\"managementState\": \"Unmanaged\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Scale down the Elasticsearch deployment to 0.")
			deployList := GetDeploymentsNameByLabel(oc, cloNS, "component=elasticsearch")
			for _, name := range deployList {
				err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", name, "--replicas=0", "-n", cloNS).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			WaitUntilPodsAreGone(oc, cloNS, "component=elasticsearch")

			g.By("Create an instance of the logtest app")
			oc.SetupProject()
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			cerr := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(cerr).NotTo(o.HaveOccurred())
			waitForPodReadyWithLabel(oc, appProj, "run=centos-logtest")

			g.By("Make sure the logtest app has generated logs")
			appPodList, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", appPodList.Items[0].Name, appProj}
			pl.assertResourceStatus(oc, "jsonpath={.status.phase}", "Running")
			pl.checkLogsFromRs(oc, "foobar", "logging-centos-logtest")

			g.By("Delete the logtest app namespace")
			deleteNamespace(oc, appProj)

			g.By("Wait for 3 minutes for logtest app logs to be discarded")
			time.Sleep(180 * time.Second)

			g.By("Scale back the elasticsearch deployment to 1 replica")
			for _, name := range deployList {
				err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment", name, "--replicas=1", "-n", cloNS).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				WaitForDeploymentPodsToBeReady(oc, cloNS, name)
			}
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Get the log count for logtest app namespace")
			postPodList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cloNS, postPodList.Items[0].Name, "infra-00")
			LogCount, err := getDocCountByQuery(cloNS, postPodList.Items[0].Name, "app", "{\"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+appProj+"\"}}}")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("Logcount for the logtest app in %s project is %d", appProj, LogCount)

			g.By("Check if the logtest application logs are discarded")
			o.Expect(LogCount == 0).To(o.BeTrue(), "The log count for the %s namespace should be 0", appProj)
		})

		// author ikanse@redhat.com
		g.It("CPaasrunOnly-Author:ikanse-High-42674-Elasticsearch log4j2 properties file and configuration test[Serial][Slow]", func() {
			// create clusterlogging instance
			g.By("deploy EFK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc, "-p", "ES_NODE_COUNT=1", "-p", "REDUNDANCY_POLICY=ZeroRedundancy")

			g.By("Waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check if the log4j2 properties: file is mounted inside the elasticsearch pod.")
			prePodList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			statFile := "stat /usr/share/java/elasticsearch/config/log4j2.properties"
			_, err = e2e.RunHostCmd(cloNS, prePodList.Items[0].Name, statFile)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check if log4j2 properties: file is loaded by elasticsearch pod")
			el := resource{"pods", prePodList.Items[0].Name, cloNS}
			el.checkLogsFromRs(oc, "-Dlog4j2.configurationFile=/usr/share/java/elasticsearch/config/log4j2.properties", "elasticsearch")

			g.By("Set the Elasticsearch operator instance managementState to Unmanaged.")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("es/elasticsearch", "-n", cloNS, "-p", "{\"spec\": {\"managementState\": \"Unmanaged\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Change elasticsearch configmap to apply log4j2.properties file with log level set to debug")
			esCMTemplate := exutil.FixturePath("testdata", "logging", "elasticsearch", "42674.yaml")
			ecm := resource{"configmaps", "elasticsearch", cloNS}
			err = ecm.applyFromTemplate(oc, "-n", ecm.namespace, "-f", esCMTemplate, "-p", "LOG_LEVEL=debug")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Delete Elasticsearch pods to pick the new configmap changes to the log4j2.properties file")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-n", cloNS, "-l", "component=elasticsearch").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for EFK to be ready")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check the elasticsearch pod logs and confirm the logging level have changed.")
			postPodList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			elp := resource{"pods", postPodList.Items[0].Name, cloNS}
			elp.checkLogsFromRs(oc, "[DEBUG]", "elasticsearch")
		})

		// author: ikanse@redhat.com
		g.It("CPaasrunOnly-Author:ikanse-Medium-40168-oc adm must-gather can collect logging data [Slow][Disruptive]", func() {
			g.By("Deploy Logging with Fluentd only instance")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace)

			g.By("Check must-gather can collect cluster logging data")
			chkMustGather(oc, cloNS)

			g.By("Create external Elasticsearch instance")
			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "6",
				serverName: "elasticsearch-server",
				loggingNS:  cloNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create CLF")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf-exteranl-es-and-default.yaml")
			err := clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy EFK pods")
			instance = exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl = resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace)

			g.By("Check must-gather can collect cluster logging data")
			chkMustGather(oc, cloNS)
		})

		// author ikanse@redhat.com
		g.It("CPaasrunOnly-Author:ikanse-Medium-46423-fluentd total_limit_size is not set beyond available space[Serial]", func() {

			g.Skip("Known issue in Cluster Logging 5.5.z https://issues.redhat.com/browse/LOG-2790")

			g.By("Create Cluster Logging instance with totalLimitSize which is more than the available space")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "46423.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc, "-p", "ES_NODE_COUNT=1", "-p", "REDUNDANCY_POLICY=ZeroRedundancy", "-p", "TOTAL_LIMIT_SIZE=1000G")

			g.By("Waiting for the ECK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check Fluentd pod logs when Fluentd buffer totalLimitSize is set more than available space")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cloNS}
			pl.checkLogsFromRs(oc, "exceeds maximum available size", "collector")

			g.By("Set totalLimitSize to 3 GB")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterlogging/instance", "-n", "openshift-logging", "-p", "{\"spec\":{\"collection\":{\"fluentd\":{\"buffer\":{\"totalLimitSize\":\"3G\"}}}}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Wait for 30 seconds for the config to be effective")
			time.Sleep(30 * time.Second)

			g.By("Delete collector pods for the Fluentd config changes to be picked up")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", "-n", cloNS, "-l", "component=collector").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("Check Fluentd pod logs for Fluentd buffer totalLimitSize set to avilable space")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl = resource{"pods", podList.Items[0].Name, cloNS}
			// 3 x 1024 x 1024 x 1024 https://github.com/openshift/cluster-logging-operator/blob/c34520fd1a42151453b2d0a41e7e0cb14dce0d83/pkg/components/fluentd/run_script.go#L80
			pl.checkLogsFromRs(oc, "3221225472", "collector")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-49212-Logging should work as usual when secrets deleted or regenerated[Serial]", func() {
			g.By("deploy ECK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc, "-p", "ES_NODE_COUNT=3", "-p", "REDUNDANCY_POLICY=SingleRedundancy")
			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cloNS)

			elasticsearch := resource{"secret", "elasticsearch", cloNS}
			collector := resource{"secret", "collector", cloNS}
			signingES := resource{"secret", "signing-elasticsearch", cloNS}
			g.By("remove secrets elasticsearch and collector, then check if theses secrets can be recreated or not")
			elasticsearch.clear(oc)
			collector.clear(oc)
			elasticsearch.WaitForResourceToAppear(oc)
			collector.WaitForResourceToAppear(oc)
			WaitForECKPodsToBeReady(oc, cloNS)
			esSVC := "https://elasticsearch." + cloNS + ".svc:9200"

			g.By("test connections between collector/kibana and ES")
			collectorPODs, _ := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			output, err := e2e.RunHostCmdWithRetries(cloNS, collectorPODs.Items[0].Name, "curl --cacert /var/run/ocp-collector/secrets/collector/ca-bundle.crt --cert /var/run/ocp-collector/secrets/collector/tls.crt --key /var/run/ocp-collector/secrets/collector/tls.key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))
			kibanaPods, _ := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=kibana"})
			output, err = e2e.RunHostCmdWithRetries(cloNS, kibanaPods.Items[0].Name, "curl -s --cacert /etc/kibana/keys/ca --cert /etc/kibana/keys/cert --key /etc/kibana/keys/key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))

			g.By("remove secret/signing-elasticsearch, then wait for the logging pods to be recreated")
			esPods, _ := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=elasticsearch"})
			signingES.clear(oc)
			signingES.WaitForResourceToAppear(oc)
			//should recreate ES and Kibana pods
			resource{"pod", esPods.Items[0].Name, cloNS}.WaitUntilResourceIsGone(oc)
			resource{"pod", kibanaPods.Items[0].Name, cloNS}.WaitUntilResourceIsGone(oc)
			WaitForECKPodsToBeReady(oc, cloNS)

			g.By("test if kibana and collector pods can connect to ES again")
			collectorPODs, _ = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
			output, err = e2e.RunHostCmdWithRetries(cloNS, collectorPODs.Items[0].Name, "curl --cacert /var/run/ocp-collector/secrets/collector/ca-bundle.crt --cert /var/run/ocp-collector/secrets/collector/tls.crt --key /var/run/ocp-collector/secrets/collector/tls.key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))
			kibanaPods, _ = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=kibana"})
			output, err = e2e.RunHostCmdWithRetries(cloNS, kibanaPods.Items[0].Name, "curl -s --cacert /etc/kibana/keys/ca --cert /etc/kibana/keys/cert --key /etc/kibana/keys/key "+esSVC, 5*time.Second, 30*time.Second)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("You Know, for Search"))
		})
	})
})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Elasticsearch should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("logging-es-"+getRandomString(), exutil.KubeConfigPath())

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
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-43444-Expose Index Level Metrics es_index_namespaces_total and es_index_document_count", func() {
		// create clusterlogging instance
		g.By("deploy EFK pods")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
		cl := resource{"clusterlogging", "instance", cloNS}
		cl.applyFromTemplate(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc)
		g.By("waiting for the EFK pods to be ready...")
		WaitForECKPodsToBeReady(oc, cloNS)

		g.By("check logs in ES pod")
		podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForIndexAppear(cloNS, podList.Items[0].Name, "infra-00")

		g.By("check ES metric es_index_namespaces_total")
		err = wait.Poll(5*time.Second, 120*time.Second, func() (done bool, err error) {
			metricData1, err := queryPrometheus(oc, "", "/api/v1/query?", "es_index_namespaces_total", "GET")
			if err != nil {
				return false, err
			}
			if len(metricData1.Data.Result) == 0 {
				return false, nil
			}
			namespaceCount, _ := strconv.Atoi(metricData1.Data.Result[0].Value[1].(string))
			e2e.Logf("\nthe namespace count is: %d", namespaceCount)
			if namespaceCount > 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "The value of metric es_index_namespaces_total isn't more than 0")

		g.By("check ES metric es_index_document_count")
		metricData2, err := queryPrometheus(oc, "", "/api/v1/query?", "es_index_document_count", "GET")
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, content := range metricData2.Data.Result {
			metricValue, _ := strconv.Atoi(content.Value[1].(string))
			o.Expect(metricValue > 0).Should(o.BeTrue())
		}
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Low-43081-remove JKS certificates", func() {
		// create clusterlogging instance
		g.By("deploy EFK pods")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
		cl := resource{"clusterlogging", "instance", cloNS}
		cl.applyFromTemplate(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc)
		g.By("waiting for the EFK pods to be ready...")
		WaitForECKPodsToBeReady(oc, cloNS)

		g.By("check certificates in ES")
		podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := "ls /etc/elasticsearch/secret/"
		stdout, err := e2e.RunHostCmdWithRetries(cloNS, podList.Items[0].Name, cmd, 3*time.Second, 30*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stdout).ShouldNot(o.ContainSubstring("admin.jks"))
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-42943-remove template org.ovirt.viaq-collectd.template.json", func() {
		// create clusterlogging instance
		g.By("deploy EFK pods")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
		cl := resource{"clusterlogging", "instance", cloNS}
		cl.applyFromTemplate(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc)
		g.By("waiting for the EFK pods to be ready...")
		WaitForECKPodsToBeReady(oc, cloNS)

		g.By("check templates in ES")
		podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := "ls /usr/share/elasticsearch/index_templates/"
		stdout, err := e2e.RunHostCmdWithRetries(cloNS, podList.Items[0].Name, cmd, 3*time.Second, 30*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stdout).ShouldNot(o.ContainSubstring("org.ovirt.viaq-collectd.template.json"))
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-43259-Access to the ES root url from a project pod on Openshift", func() {
		// create clusterlogging instance
		g.By("deploy EFK pods")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
		cl := resource{"clusterlogging", "instance", cloNS}
		cl.applyFromTemplate(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc)
		g.By("waiting for the EFK pods to be ready...")
		WaitForECKPodsToBeReady(oc, cloNS)

		g.By("deploy a pod and try to connect to ES")
		oc.SetupProject()
		appProj := oc.Namespace()
		jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		err = oc.Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		token, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		podList, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := "curl -tlsv1.2 --insecure -H \"Authorization: Bearer " + token + "\" https://elasticsearch.openshift-logging.svc:9200"
		stdout, err := e2e.RunHostCmdWithRetries(appProj, podList.Items[0].Name, cmd, 5*time.Second, 60*time.Second)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stdout).Should(o.ContainSubstring("You Know, for Search"))
	})

	g.It("CPaasrunOnly-Author:qitang-Medium-49099-Elasticsearch should be upgraded successfully when the tolerations enabled[Serial][Slow]", func() {
		// create clusterlogging instance
		g.By("deploy EFK pods")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
		cl := resource{"clusterlogging", "instance", cloNS}
		defer cl.deleteClusterLogging(oc)
		tolerations := "[{\"effect\": \"NoSchedule\", \"operator\": \"Exists\"}]"
		cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc, "-p", "TOLERATIONS="+tolerations, "-p", "ES_NODE_COUNT=3", "-p", "REDUNDANCY_POLICY=SingleRedundancy")
		g.By("waiting for the EFK pods to be ready...")
		WaitForECKPodsToBeReady(oc, cloNS)

		g.By("update log store configurations to make ES pods do rolling-upgrade")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cl/instance", "-n", cloNS, "-p", "{\"spec\": {\"logStore\": {\"elasticsearch\": {\"resources\": {\"requests\": {\"memory\": \"3Gi\"}}}}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkResource(oc, true, true, "3Gi", []string{"elasticsearches.logging.openshift.io", "elasticsearch", "-n", cloNS, "-ojsonpath={.spec.nodeSpec.resources.requests.memory}"})

		g.By("wait for ES pods complete rolling upgrade, the ES cluster health should be green")
		// make sure the upgrade starts
		checkResource(oc, false, true, "green", []string{"elasticsearches.logging.openshift.io", "elasticsearch", "-n", cloNS, "-ojsonpath={.status.cluster.status}"})
		//rolling upgrade, the es health status will be green temporarily, so here compare the ready pods with the pod names in es/elasticsearch
		err = wait.Poll(3*time.Second, 300*time.Second, func() (done bool, err error) {
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			if err != nil {
				return false, err
			}
			esMasterReadyPods, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("elasticsearches.logging.openshift.io", "elasticsearch", "-n", cloNS, "-ojsonpath={.status.pods.master.ready}").Output()
			if err != nil {
				return false, err
			}
			match := true
			for _, pod := range esPods.Items {
				if !strings.Contains(esMasterReadyPods, pod.Name) {
					match = false
				}
			}
			return match, nil
		})
		exutil.AssertWaitPollNoErr(err, "The ES pods are not updated")
		// make sure ES cluster health is green in the end
		checkResource(oc, true, true, "green", []string{"elasticsearches.logging.openshift.io", "elasticsearch", "-n", cloNS, "-ojsonpath={.status.cluster.status}"})
		checkResource(oc, false, false, "preparationComplete", []string{"elasticsearches.logging.openshift.io", "elasticsearch", "-n", cloNS, "-ojsonpath={.status.nodes[*].upgradeStatus.upgradePhase}"})
	})

})

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Fluentd should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("logging-fluentd-"+getRandomString(), exutil.KubeConfigPath())
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
	})

	// author qitang@redhat.com
	g.It("CPaasrunOnly-Author:qitang-Medium-43177-expose the metrics needed to understand the volume of logs being collected.", func() {
		// create clusterlogging instance
		g.By("deploy EFK pods")
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
		cl := resource{"clusterlogging", "instance", cloNS}
		cl.applyFromTemplate(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc)
		g.By("waiting for the EFK pods to be ready...")
		WaitForECKPodsToBeReady(oc, cloNS)
		podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForIndexAppear(cloNS, podList.Items[0].Name, "infra")

		g.By("check metrics")
		for _, metric := range []string{"log_logged_bytes_total", "log_collected_bytes_total"} {
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
	g.It("CPaasrunOnly-Author:qitang-High-53133-Fluentd Preserve k8s Common Labels[Serial]", func() {
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
		oc.SetupProject()
		app := oc.Namespace()
		loglabeltemplate := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
		clf := resource{"clusterlogforwarder", "instance", "openshift-logging"}
		defer clf.clear(oc)
		inputs, _ := json.Marshal([]string{"application"})
		outputs, _ := json.Marshal([]string{"loki-server", "default"})
		err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "-p", "INPUTREFS="+string(inputs), "-p", "OUTPUTREFS="+string(outputs))
		o.Expect(err).NotTo(o.HaveOccurred())

		// create clusterlogging instance
		instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
		cl := resource{"clusterlogging", "instance", "openshift-logging"}
		defer cl.deleteClusterLogging(oc)
		cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "COLLECTOR=fluentd")
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
		o.Expect(reflect.DeepEqual(flatLabelsInES, flatLabelsInLoki)).Should(o.BeTrue())
	})

})
