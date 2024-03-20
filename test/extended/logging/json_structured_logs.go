package logging

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("logging-json-log", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("JSON structured logs -- outputDefaults testing", func() {

		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			//deploy CLO and EO
			//CLO is deployed to `openshift-logging` namespace by default
			//and EO is deployed to `openshift-operators-redhat` namespace
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

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41847-High-41848-structured index by kubernetes.labels.test/openshift.labels.team [Serial][Slow]", func() {
			// create a project, then create a pod in the project to generate some json logs
			g.By("create some json logs")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "42475.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT="+appProj, "STRUCTURED_TYPE_KEY=kubernetes.labels.test")

			// create clusterlogging instance
			g.By("deploy EFK pods")
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

			// check data in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-centos-logtest")

			//check if the JSON logs are parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			//update clusterlogforwarder instance
			e2e.Logf("start testing OCP-41848")
			g.By("change clusterlogforwarder/instance")
			clf.update(oc, clf.templateFile, "DATA_PROJECT="+appProj, "STRUCTURED_TYPE_KEY=openshift.labels.team")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
			// check data in ES
			g.By("check indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-qa-openshift-label")
			//check if the JSON logs are parsed
			checkLog2 := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj + "\"}}}"
			logs2 := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-qa-openshift-label", checkLog2)
			o.Expect(logs2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-42475-High-42386-structured index by kubernetes.container_name/kubernetes.namespace_name [Serial][Slow]", func() {
			// create a project, then create a pod in the project to generate some json logs
			g.By("create some json logs")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "42475.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT="+appProj, "STRUCTURED_TYPE_KEY=kubernetes.container_name")

			// create clusterlogging instance
			g.By("deploy EFK pods")
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

			// check data in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-logging-centos-logtest")
			//check if the JSON logs are parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-logging-centos-logtest", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			e2e.Logf("start testing OCP-42386")
			g.By("updating clusterlogforwarder")
			clf.update(oc, clf.templateFile, "DATA_PROJECT="+appProj, "STRUCTURED_TYPE_KEY=kubernetes.namespace_name")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
			// check data in ES
			g.By("check indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-"+appProj)
			//check if the JSON logs are parsed
			checkLog2 := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj + "\"}}}"
			logs2 := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-"+appProj, checkLog2)
			o.Expect(logs2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-42363-the logs are sent to both structured and default index[Serial]", func() {
			//create 2 projects and generate json logs in each project
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("create some json logs")
			appProj1 := oc.Namespace()
			oc.SetupProject()
			appProj2 := oc.Namespace()

			// create clusterlogging instance
			g.By("deploy EFK pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc, "APP_LOG_MAX_AGE=10m")
			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "42363.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT="+appProj1, "STRUCTURED_TYPE_KEY=kubernetes.namespace_name")
			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// check indices name in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, indexName := range []string{"app-" + appProj1, "app-00", "infra-00", "audit-00"} {
				waitForIndexAppear(cl.namespace, podList.Items[0].Name, indexName)
			}

			// check log in ES
			// logs in proj1 should be stored in index "app-${appProj1}" and json logs should be parsed
			// logs in proj2,proj1 should be stored in index "app-000xxx", no json structured logs
			checkLog1 := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj1 + "\"}}}"
			logs1 := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-"+appProj1, checkLog1)
			o.Expect(logs1.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			for _, proj := range []string{appProj1, appProj2} {
				waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, proj, "app-00")
				checkLog2 := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + proj + "\"}}}"
				logs2 := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-00", checkLog2)
				o.Expect(logs2.Hits.DataHits[0].Source.Structured.Message).Should(o.BeEmpty())
			}

			// check if the retention policy works with the new indices
			// set managementState to Unmanaged in es/elasticsearch
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("es/elasticsearch", "-n", cl.namespace, "-p", "{\"spec\": {\"managementState\": \"Unmanaged\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			indices1, _ := getESIndicesByName(cl.namespace, podList.Items[0].Name, "app-"+appProj1)
			indexNames1 := make([]string, 0, len(indices1))
			for _, index := range indices1 {
				indexNames1 = append(indexNames1, index.Index)
			}
			e2e.Logf("indexNames1: %v\n\n", indexNames1)
			// change the schedule of cj/elasticsearch-im-xxx, make it run in every 2 minute
			for _, cj := range []string{"elasticsearch-im-app", "elasticsearch-im-infra", "elasticsearch-im-audit"} {
				err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cronjob/"+cj, "-n", cl.namespace, "-p", "{\"spec\": {\"schedule\": \"*/2 * * * *\"}}").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			// remove all the jobs
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("job", "-n", cl.namespace, "--all").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIMJobsToComplete(oc, cl.namespace, 180*time.Second)
			indices2, _ := getESIndicesByName(cl.namespace, podList.Items[0].Name, "app-"+appProj1)
			indexNames2 := make([]string, 0, len(indices2))
			for _, index := range indices2 {
				indexNames2 = append(indexNames2, index.Index)
			}
			e2e.Logf("indexNames2: %v\n\n", indexNames2)
			o.Expect(indexNames1).NotTo(o.Equal(indexNames2))
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-42419-Fall into app-00* index if message is not json[Serial]", func() {
			// create a project, then create a pod in the project to generate some non-json logs
			g.By("create some non-json logs")
			appProj := oc.Namespace()
			nonJSONLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_non_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", nonJSONLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "42475.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT="+appProj, "STRUCTURED_TYPE_KEY=kubernetes.namespace_name")

			// create clusterlogging instance
			g.By("deploy EFK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			cl := clusterlogging{
				name:             "instance",
				namespace:        loggingNS,
				collectorType:    "fluentd",
				logStoreType:     "elasticsearch",
				esNodeCount:      1,
				storageClassName: sc,
				waitForReady:     true,
				templateFile:     filepath.Join(loggingBaseDir, "clusterlogging", "cl-storage-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check logs in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-00")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, appProj, "app-00")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + appProj + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-00", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.BeEmpty())
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41742-Mix the structured index, non-structured and the default input type[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			app1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			app2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app2).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			app3 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app3).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41742.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT_1="+app1, "DATA_PROJECT_2="+app2)

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				waitForReady:  true,
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-centos-logtest")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-00")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "infra")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "audit")

			//check if the JSON logs are parsed
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app1, "app-centos-logtest")
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app1 + "\"}}}"
			logs1 := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest", checkLog)
			o.Expect(logs1.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app1, "app-00")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app2, "app-00")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app3, "app-00")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-50258-Send JSON logs from containers in the same pod to separate indices -- outputDefaults[Serial]", func() {
			app := oc.Namespace()
			multiContainerJSONLog := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
			containerName := "log-50258-" + getRandomString()
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
				collectorType: "fluentd",
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
		g.It("CPaasrunOnly-Author:qitang-Medium-50279-JSON logs from containers in the same pod are not sent to separate indices when enableStructuredContainerLogs is false[Serial]", func() {
			app := oc.Namespace()
			multiContainerJSONLog := filepath.Join(loggingBaseDir, "generatelog", "multi_container_json_log_template.yaml")
			containerName := "log-50279-" + getRandomString()
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
				collectorType: "fluentd",
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

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-50357-Logs from different projects are forwarded to the same index if the pods have same annotation[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			containerName := "log-50357-" + getRandomString()
			app1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app1, "-p", "CONTAINER="+containerName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			app2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app2, "-p", "CONTAINER="+containerName).Execute()
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
				collectorType: "fluentd",
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
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "app-"+containerName)

			g.By("check data in ES")
			waitForProjectLogsAppear(cl.namespace, esPods.Items[0].Name, app1, "app-"+containerName)
			waitForProjectLogsAppear(cl.namespace, esPods.Items[0].Name, app2, "app-"+containerName)
		})
	})

	g.Context("JSON structured logs -- outputs testing", func() {

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

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41300-dynamically index by openshift.labels[Serial]", func() {
			app := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41300.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT="+app, "STRUCTURED_TYPE_KEY=openshift.labels.team")
			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-qa-openshift-label")

			//check if the JSON logs are parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-qa-openshift-label", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41729-structured index by indexName(Fall in indexName when index key is not available)[Serial]", func() {
			app := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41729.yaml"),
			}
			defer clf.delete(oc)
			projects, _ := json.Marshal([]string{app})
			clf.create(oc, "DATA_PROJECTS="+string(projects), "STRUCTURED_TYPE_KEY=openshift.labels.team", "STRUCTURED_TYPE_NAME=ocp-41729")

			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-ocp-41729")

			//check if the JSON logs are parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-ocp-41729", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41730-High-41732-structured index by kubernetes.namespace_name or kubernetes.labels[Serial]", func() {
			app := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41729.yaml"),
			}
			defer clf.delete(oc)
			projects, _ := json.Marshal([]string{app})
			clf.create(oc, "DATA_PROJECTS="+string(projects), "STRUCTURED_TYPE_KEY=kubernetes.namespace_name")

			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-"+app)
			//check if the JSON logs are parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-"+app, checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			g.By("update CLF to test OCP-41732")
			clf.update(oc, clf.templateFile, "DATA_PROJECTS="+string(projects), "STRUCTURED_TYPE_KEY=kubernetes.labels.test")
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForECKPodsToBeReady(oc, cl.namespace)
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-centos-logtest")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app, "app-centos-logtest")
			//check if the JSON logs are parsed
			checkLog2 := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app + "\"}}}"
			logs2 := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest", checkLog2)
			o.Expect(logs2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-41785-No dynamically index when no type specified in output[Serial]", func() {
			app := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41785.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT="+app)

			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-000")

			//check if the JSON logs are parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-000", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.BeEmpty())
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41787-High-41788-The logs are sent to default app or structuredTypeName index when the label doesn't match the structuredIndexKey[Serial]", func() {
			app := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41788.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DATA_PROJECT="+app, "STRUCTURED_TYPE_KEY=kubernetes.labels.none")

			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-00")

			//check if the JSON logs are not parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-00", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.BeEmpty())

			g.By("update clusterlogforwarder/instance to test OCP-41787")
			newclfTemplate := filepath.Join(loggingBaseDir, "clusterlogforwarder", "41729.yaml")
			projects, _ := json.Marshal([]string{app})
			clf.update(oc, newclfTemplate, "DATA_PROJECTS="+string(projects), "STRUCTURED_TYPE_KEY=kubernetes.labels.none", "STRUCTURED_TYPE_NAME=test-41787")
			WaitForECKPodsToBeReady(oc, cl.namespace)
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-test-41787")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app, "app-test-41787")
			//check if the JSON logs are parsed
			checkLog2 := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app + "\"}}}"
			logs2 := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-test-41787", checkLog2)
			o.Expect(logs2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41790-The unmatched pod logs fall into index structuredTypeName[Serial]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			app1 := oc.Namespace()
			oc.SetupProject()
			app2 := oc.Namespace()

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41729.yaml"),
			}
			defer clf.delete(oc)
			projects, _ := json.Marshal([]string{app1, app2})
			clf.create(oc, "DATA_PROJECTS="+string(projects), "STRUCTURED_TYPE_KEY=kubernetes.labels.test", "STRUCTURED_TYPE_NAME=ocp-41790")

			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)

			err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app2, "-p", "LABELS={\"test-logging\": \"OCP-41790\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-centos-logtest")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-ocp-41790")

			//check if the JSON logs are parsed
			checkLog := "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app1 + "\"}}}"
			logs := searchDocByQuery(cl.namespace, podList.Items[0].Name, "app-centos-logtest", checkLog)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, app2, "app-ocp-41790")
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-41302-structuredTypeKey for external ES which doesn't enabled ingress plugin[Serial]", func() {
			app := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", jsonLogFile, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "7",
				serverName: "elasticsearch-server",
				httpSSL:    true,
				clientAuth: true,
				secretName: "external-es-41302",
				loggingNS:  loggingNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41729.yaml"),
				secretName:   ees.secretName,
			}
			defer clf.delete(oc)
			projects, _ := json.Marshal([]string{app})
			eesURL := "https://" + getRouteAddress(oc, ees.namespace, ees.serverName) + ":443"
			clf.create(oc, "DATA_PROJECTS="+string(projects), "STRUCTURED_TYPE_KEY=kubernetes.namespace_name", "URL="+eesURL)

			g.By("deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("check indices in external ES pod")
			ees.waitForIndexAppear(oc, "app-"+app+"-write")

			//check if the JSON logs are parsed
			logs := ees.searchDocByQuery(oc, "app-"+app, "{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+app+"\"}}}")
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-50276-Send JSON logs from containers in the same pod to separate indices[Serial]", func() {
			app := oc.Namespace()
			containerName := "log-50276-" + getRandomString()
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
				secretName: "json-log-50276",
				loggingNS:  loggingNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)
			eesURL := "https://" + ees.serverName + "." + ees.namespace + ".svc:9200"

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "structured-container-logs.yaml"),
				secretName:   ees.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "STRUCTURED_CONTAINER=true", "URL="+eesURL)

			// create clusterlogging instance
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

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
		g.It("CPaasrunOnly-Author:qitang-Medium-47834-Medium-60489-Forward logs to both external ES 8.x and outputDefault using json and enable structure[Serial]", func() {
			app := oc.Namespace()
			logTemplate := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-f", logTemplate, "-n", app).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			esProj := oc.Namespace()
			ees := externalES{
				namespace:  esProj,
				version:    "8",
				serverName: "external-es",
				httpSSL:    true,
				clientAuth: true,
				secretName: "structured-log-47834",
				loggingNS:  loggingNS,
			}
			defer ees.remove(oc)
			ees.deploy(oc)
			eesURL := "https://" + ees.serverName + "." + ees.namespace + ".svc:9200"

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "47834.yaml"),
				secretName:   ees.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "ES_URL="+eesURL, "ES_VERSION="+ees.version)

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

			g.By("check logs")
			esPods, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "app-"+app)
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "infra")
			waitForIndexAppear(cl.namespace, esPods.Items[0].Name, "audit")

			ees.waitForIndexAppear(oc, "app-write")
			ees.waitForIndexAppear(oc, "infra")
			ees.waitForIndexAppear(oc, "audit")
		})

	})

})
