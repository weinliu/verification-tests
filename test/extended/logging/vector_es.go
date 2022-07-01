package logging

import (
	"fmt"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("vector-es-namespace", exutil.KubeConfigPath())
		eo             = "elasticsearch-operator"
		clo            = "cluster-logging-operator"
		cloPackageName = "cluster-logging"
		eoPackageName  = "elasticsearch-operator"
	)

	g.Context("Vector collector tests", func() {
		var (
			subTemplate       = exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
			SingleNamespaceOG = exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml")
			AllNamespaceOG    = exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml")
			loglabeltemplate  = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		)
		cloNS := "openshift-logging"
		eoNS := "openshift-operators-redhat"
		CLO := SubscriptionObjects{clo, cloNS, SingleNamespaceOG, subTemplate, cloPackageName, CatalogSourceObjects{}}
		EO := SubscriptionObjects{eo, eoNS, AllNamespaceOG, subTemplate, eoPackageName, CatalogSourceObjects{}}
		g.BeforeEach(func() {
			g.By("deploy CLO and EO")
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
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Make sure the Elasticsearch cluster is healthy")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Check Vector status")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "component=collector"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cloNS}
			pl.checkLogsFromRs(oc, "Healthcheck: Passed", "collector")
			pl.checkLogsFromRs(oc, "Vector has started", "collector")

			g.By("Check app indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-000")

			g.By("Check infra indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "infra-000")

			g.By("Check for Vector logs in Elasticsearch")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.container_name\": \"collector\"}}}"
			logs := searchDocByQuery(oc, cloNS, podList.Items[0].Name, "*", checkLog)
			o.Expect(logs.Hits.Total).Should(o.Equal(0), "Vector logs should not be collected")
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
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Make sure the Elasticsearch cluster is healthy")
			cl.assertResourceStatus(oc, "jsonpath={.status.logStore.elasticsearchStatus[0].cluster.status}", "green")

			g.By("Deploy the Event Router")
			evt := resource{"deployment", "eventrouter", cloNS}
			defer deleteEventRouter(oc, cloNS)
			evt.createEventRouter(oc, "-f", eventrouterTemplate)

			g.By("Check event logs in the Event Router pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "component=eventrouter"})
			o.Expect(err).NotTo(o.HaveOccurred())
			pl := resource{"pods", podList.Items[0].Name, cloNS}
			pl.checkLogsFromRs(oc, "ADDED", "kube-eventrouter")
			pl.checkLogsFromRs(oc, "Update", "kube-eventrouter")

			g.By("Check for Event Router logs in Elasticsearch")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"component=eventrouter\"}}}"
			err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
				logs := searchDocByQuery(oc, cloNS, podList.Items[0].Name, "infra", checkLog)
				if logs.Hits.Total > 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("No Event Router logs found when using %s as log collector.", "vector"))
		})

	})

	g.Context("Vector Elasticsearch tests", func() {
		var (
			subTemplate       = exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
			SingleNamespaceOG = exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml")
			AllNamespaceOG    = exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml")
			loglabeltemplate  = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		)
		cloNS := "openshift-logging"
		eoNS := "openshift-operators-redhat"
		CLO := SubscriptionObjects{clo, cloNS, SingleNamespaceOG, subTemplate, cloPackageName, CatalogSourceObjects{}}
		EO := SubscriptionObjects{eo, eoNS, AllNamespaceOG, subTemplate, eoPackageName, CatalogSourceObjects{}}
		g.BeforeEach(func() {
			g.By("deploy CLO and EO")
			CLO.SubscribeOperator(oc)
			EO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48591-Vector ClusterLogForwarder Label all messages with same tag[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{esProj, "6.8", "elasticsearch-server", false, false, false, "", "", "", cloNS}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "48591.yaml")
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
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-000")
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "infra-000")
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "audit-000")

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
					if logs.Hits.Total > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "No logs found with pipeline label in extranl ES")
			}

			g.By("Check logs with pipeline label in default ES")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging-labels\": \"test-labels\"}}}"
			for i := 0; i < len(indexName); i++ {
				err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
					logs := searchDocByQuery(oc, cloNS, podList.Items[0].Name, indexName[i], checkLog)
					if logs.Hits.Total > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "No logs found with pipeline label in default ES instance")
			}

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48593-Vector ClusterLogForwarder Label each message type differently and send all to the same output[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{esProj, "6.8", "elasticsearch-server", false, false, false, "", "", "", cloNS}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
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
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-000")
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "infra-000")
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "audit-000")

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
				exutil.AssertWaitPollNoErr(err, "No logs found with pipeline label in extranl ES")
			}

			g.By("Check logs with pipeline label in default ES")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			for i := 0; i < len(indexName); i++ {
				checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"openshift.labels.logging\": \"" + indexName[i] + "-logs\"}}}"
				err = wait.Poll(10*time.Second, 60*time.Second, func() (done bool, err error) {
					logs := searchDocByQuery(oc, cloNS, podList.Items[0].Name, indexName[i], checkLog)
					if logs.Hits.Total > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "No logs found with pipeline label in default ES instance")
			}

		})

		g.It("CPaasrunOnly-Author:ikanse-High-46882-Vector ClusterLogForwarder forward logs to non ClusterLogging managed Elasticsearch insecure forward[Serial][Slow]", func() {

			g.By("Create external Elasticsearch instance")
			esProj := oc.Namespace()
			ees := externalES{esProj, "6.8", "elasticsearch-server", false, false, false, "", "", "", cloNS}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "46882.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=http://"+ees.serverName+"."+esProj+".svc:9200")
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
			ees := externalES{esProj, "6.8", "elasticsearch-server", true, false, false, "", "", "ees-https", cloNS}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("Create project for app logs and deploy the log generator app")
			oc.SetupProject()
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Create ClusterLogForwarder instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "46920.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "ES_URL=https://"+ees.serverName+"."+esProj+".svc:9200", "-p", "ES_SECRET="+ees.secretName)
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
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-000")

			g.By("Check for project1 logs in default ES")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj1 + "\"}}}"
			logs := searchDocByQuery(oc, cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total).ShouldNot(o.Equal(0), "Project1 %s logs not found in default ES", appProj1)

			g.By("Check for project2 logs in default ES")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj2 + "\"}}}"
			logs = searchDocByQuery(oc, cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total).Should(o.Equal(0), "Projec2 %s logs should not be collected", appProj2)

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48786-Vector Forward logs from specified pods using label selector[Serial][Slow]", func() {

			g.By("Create project1 for app logs and deploy the log generator app with run=centos-logtest-qa label")
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
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-000")

			g.By("Check for logs with run=centos-logtest-qa labels")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-qa\"}}}"
			logs := searchDocByQuery(oc, cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total).ShouldNot(o.Equal(0), "Labels with run=centos-logtest-qa in logs not found in default ES")

			g.By("Check for logs with run=centos-logtest-dev labels")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-dev\"}}}"
			logs = searchDocByQuery(oc, cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total).Should(o.Equal(0), "Logs with label with run=centos-logtest-dev should not be collected")

		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-48787-Vector Forward logs from specified pods using label and namespace selectors[Serial][Slow]", func() {

			g.By("Create project1 for app logs and deploy the log generator app with run=centos-logtest-qa and run=centos-logtest-stage labels")
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
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-000")

			g.By("Check for logs with run=centos-logtest-qa in label")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-qa\"}}}"
			logs := searchDocByQuery(oc, cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total).ShouldNot(o.Equal(0), "Labels with run=centos-logtest-qa in logs not found in default ES")

			g.By("Check for logs with run=centos-logtest-stage in label")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-stage\"}}}"
			logs = searchDocByQuery(oc, cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total).Should(o.Equal(0), "Labels with run=centos-logtest-dev in logs should not be collected")

			g.By("Check for logs with run=centos-logtest-dev label")
			checkLog = "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.flat_labels\": \"run=centos-logtest-dev\"}}}"
			logs = searchDocByQuery(oc, cloNS, podList.Items[0].Name, "app", checkLog)
			o.Expect(logs.Hits.Total).Should(o.Equal(0), "Labels with run=centos-logtest-dev in logs should not be collected")

		})

	})

})
