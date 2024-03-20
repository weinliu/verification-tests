package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("logfwd-namespace", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Log Forward with namespace selector in the CLF", func() {

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

		g.It("CPaasrunOnly-Author:kbharti-High-41598-forward logs only from specific pods via a label selector inside the Log Forwarding API[Serial]", func() {
			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_non_json_log_template.json")
			)
			// Dev label - create a project and pod in the project to generate some logs
			g.By("create application for logs with dev label")
			appProjDev := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProjDev, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest-dev").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// QA label - create a project and pod in the project to generate some logs
			g.By("create application for logs with qa label")
			oc.SetupProject()
			appProjQa := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProjQa, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest-qa").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//Create ClusterLogForwarder instance
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41598.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc)

			//Create ClusterLogging instance
			g.By("deploy ECK pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
				waitForReady:  true,
			}
			defer cl.delete(oc)
			cl.create(oc)

			//check app index in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-000")

			//Waiting for the app index to be populated
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, appProjQa, "app-000")

			// check data in ES for QA namespace
			g.By("check logs in ES pod for QA namespace in CLF")
			count1, err := getDocCountByQuery(cl.namespace, podList.Items[0].Name, "app", "{\"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+appProjQa+"\"}}}")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(count1 > 0).Should(o.BeTrue())

			//check that no data exists for the other Dev namespace - Negative test
			g.By("check logs in ES pod for Dev namespace in CLF")
			count2, _ := getDocCountByQuery(cl.namespace, podList.Items[0].Name, "app-0000", "{\"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+appProjDev+"\"}}}")
			o.Expect(count2 == 0).Should(o.BeTrue())

		})

		g.It("CPaasrunOnly-Author:kbharti-High-41599-Forward Logs from specified pods combining namespaces and label selectors[Serial]", func() {
			var (
				loglabeltemplate = filepath.Join(loggingBaseDir, "generatelog", "container_non_json_log_template.json")
			)

			g.By("create application for logs with dev1 label")
			appProjDev := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProjDev, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest-dev-1", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-dev1", "-p", "CONFIGMAP=logtest-config-dev1").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create application for logs with dev2 label")
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProjDev, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest-dev-2", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-dev2", "-p", "CONFIGMAP=logtest-config-dev2").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create application for logs with qa1 label")
			oc.SetupProject()
			appProjQa := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProjQa, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest-qa-1", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-qa1", "-p", "CONFIGMAP=logtest-config-qa1").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create application for logs with qa2 label")
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProjQa, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest-qa-2", "-p", "REPLICATIONCONTROLLER=logging-centos-logtest-qa2", "-p", "CONFIGMAP=logtest-config-qa2").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//Create ClusterLogForwarder instance
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41599.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "APP_NAMESPACE_QA="+appProjQa, "APP_NAMESPACE_DEV="+appProjDev)

			//Create ClusterLogging instance
			g.By("deploy ECK pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				logStoreType:  "elasticsearch",
				esNodeCount:   1,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "cl-template.yaml"),
				waitForReady:  true,
			}
			defer cl.delete(oc)
			cl.create(oc)

			//check app index in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app-00")

			//Waiting for the app index to be populated
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, appProjQa, "app-00")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, appProjDev, "app-00")

			g.By("check doc count in ES pod for QA1 namespace in CLF")
			logCount, _ := getDocCountByQuery(cl.namespace, podList.Items[0].Name, "app-00", "{\"query\": {\"terms\": {\"kubernetes.flat_labels\": [\"run=centos-logtest-qa-1\"]}}}")
			o.Expect(logCount == 0).ShouldNot(o.BeTrue())

			g.By("check doc count in ES pod for QA2 namespace in CLF")
			logCount, _ = getDocCountByQuery(cl.namespace, podList.Items[0].Name, "app-00", "{\"query\": {\"terms\": {\"kubernetes.flat_labels\": [\"run=centos-logtest-qa-2\"]}}}")
			o.Expect(logCount == 0).Should(o.BeTrue())

			g.By("check doc count in ES pod for DEV1 namespace in CLF")
			logCount, _ = getDocCountByQuery(cl.namespace, podList.Items[0].Name, "app-00", "{\"query\": {\"terms\": {\"kubernetes.flat_labels\": [\"run=centos-logtest-dev-1\"]}}}")
			o.Expect(logCount == 0).ShouldNot(o.BeTrue())

			g.By("check doc count in ES pod for DEV2 namespace in CLF")
			logCount, _ = getDocCountByQuery(cl.namespace, podList.Items[0].Name, "app-00", "{\"query\": {\"terms\": {\"kubernetes.flat_labels\": [\"run=centos-logtest-dev-2\"]}}}")
			o.Expect(logCount == 0).Should(o.BeTrue())

		})

	})

	g.Context("test forward logs to external log stores", func() {

		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			g.By("deploy CLO and EO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     loggingNS,
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

		g.It("CPaasrunOnly-Author:ikanse-High-42981-Collect OVN audit logs [Serial]", func() {
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
			source := []string{"audit"}
			inputs, _ := json.Marshal(source)
			clf.create(oc, "INPUTS="+string(inputs))

			g.By("Deploy ECK pods")
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
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
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
			exutil.AssertWaitPollNoErr(err, fmt.Sprint("The ovn audit logs are not collected", ""))
		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-45256-[fluentd]Forward logs to log store for multiline log assembly[Serial][Slow]", func() {
			multilineLogTypes := map[string][]string{
				"java":   {javaExc, complexJavaExc, nestedJavaExc},
				"go":     {goExc, goOnGaeExc, goSignalExc, goHTTP},
				"ruby":   {rubyExc, railsExc},
				"js":     {clientJsExc, nodeJsExc, v8JsExc},
				"csharp": {csharpAsyncExc, csharpNestedExc, csharpExc},
				"python": {pythonExc},
				"php":    {phpOnGaeExc, phpExc},
				"dart": {
					dartAbstractClassErr,
					dartArgumentErr,
					dartAssertionErr,
					dartAsyncErr,
					dartConcurrentModificationErr,
					dartDivideByZeroErr,
					dartErr,
					dartTypeErr,
					dartExc,
					dartUnsupportedErr,
					dartUnimplementedErr,
					dartOOMErr,
					dartRangeErr,
					dartReadStaticErr,
					dartStackOverflowErr,
					dartFallthroughErr,
					dartFormatErr,
					dartFormatWithCodeErr,
					dartNoMethodErr,
					dartNoMethodGlobalErr,
				},
			}

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
			g.By("create CLF and enable detectMultilineErrors")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "forward_to_default.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "DETECT_MULTILINE_ERRORS=true")

			g.By("waiting for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)
			defer resource{"route", "elasticsearch", cl.namespace}.clear(oc)
			exposeESService(oc, cl.namespace)

			g.By("create some pods to generate multiline error")
			multilineLogFile := filepath.Join(loggingBaseDir, "generatelog", "multiline-error-log.yaml")
			for k := range multilineLogTypes {
				ns := "multiline-log-" + k + "-45256"
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--wait=false").Execute()
				err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "deploy/multiline-log", "cm/multiline-log").Execute()
				err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", ns, "-f", multilineLogFile, "-p", "LOG_TYPE="+k, "-p", "RATE=60.00").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("check data in ES")
			esPods, err := getPodNames(oc, cl.namespace, "es-node-master=true")
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, esPods[0], "app-00")

			esRoute := "https://" + getRouteAddress(oc, cl.namespace, "elasticsearch")
			_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", oc.Namespace())).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			token := getSAToken(oc, "default", oc.Namespace())
			for k, v := range multilineLogTypes {
				g.By("check " + k + " logs\n")
				waitForProjectLogsAppear(cl.namespace, esPods[0], "multiline-log-"+k+"-45256", "app-00")
				for _, log := range v {
					var messages []string
					err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(context.Context) (done bool, err error) {
						indices, err := getIndexNamesViaRoute(esRoute, token, "app")
						o.Expect(err).NotTo(o.HaveOccurred())
						resp, err := queryInESViaRoute(esRoute, token, indices, "_search", "{\"size\": "+strconv.Itoa(len(v)*2+7)+", \"sort\": [{\"@timestamp\": {\"order\": \"desc\"}}], \"query\": {\"regexp\": {\"kubernetes.namespace_name\": \"multiline-log-"+k+"-45256\"}}}", "post")
						o.Expect(err).NotTo(o.HaveOccurred())
						var multilineLog SearchResult
						json.Unmarshal([]byte(resp), &multilineLog)
						for _, msg := range multilineLog.Hits.DataHits {
							messages = append(messages, msg.Source.Message)
						}
						if len(messages) == 0 {
							return false, nil
						}
						if !containSubstring(messages, log) {
							e2e.Logf("can't find log\n%s, try next round", log)
							return false, nil
						}
						return true, nil
					})
					if err != nil {
						e2e.Logf("\n\nlogs in ES:\n\n")
						for _, m := range messages {
							e2e.Logf(m)
						}
						e2e.Failf("%s logs are not parsed", k)
					}
				}
				e2e.Logf("\nfound %s logs\n", k)
			}
		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-49040-High-49041-Forward logs to multiple log stores for multiline log assembly[Serial][Slow]", func() {
			multilineLogTypes := map[string][]string{
				"java":   {javaExc, complexJavaExc, nestedJavaExc},
				"go":     {goExc, goOnGaeExc, goSignalExc, goHTTP},
				"ruby":   {rubyExc, railsExc},
				"js":     {clientJsExc, nodeJsExc, v8JsExc},
				"python": {pythonExc},
				"php":    {phpOnGaeExc, phpExc},
			}

			g.By("Create Loki project and deploy Loki Server")
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)

			logsToLoki, _ := json.Marshal([]string{"multiline-log-java-49040", "multiline-log-go-49040", "multiline-log-ruby-49040"})
			logsToEs, _ := json.Marshal([]string{"multiline-log-python-49040", "multiline-log-js-49040", "multiline-log-php-49040"})

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
			cl.create(oc)
			g.By("create CLF and enable detectMultilineErrors")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "49040.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "NAMESPACES_LOKI="+string(logsToLoki), "NAMESPACES_ES="+string(logsToEs))
			g.By("wait for the EFK pods to be ready...")
			WaitForECKPodsToBeReady(oc, cl.namespace)
			defer resource{"route", "elasticsearch", cl.namespace}.clear(oc)
			exposeESService(oc, cl.namespace)

			g.By("create some pods to generate multiline error")
			multilineLogFile := filepath.Join(loggingBaseDir, "generatelog", "multiline-error-log.yaml")
			for k := range multilineLogTypes {
				ns := "multiline-log-" + k + "-49040"
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--wait=false").Execute()
				err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "deploy/multiline-log", "cm/multiline-log").Execute()
				err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", ns, "-f", multilineLogFile, "-p", "LOG_TYPE="+k, "-p", "RATE=60.00").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("check data in ES")
			esPods, err := getPodNames(oc, cl.namespace, "es-node-master=true")
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, esPods[0], "app-00")
			esRoute := "https://" + getRouteAddress(oc, cl.namespace, "elasticsearch")
			_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", oc.Namespace())).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			token := getSAToken(oc, "default", oc.Namespace())
			for _, l := range []string{"python", "js", "php"} {
				g.By("check " + l + " logs\n")
				waitForProjectLogsAppear(cl.namespace, esPods[0], "multiline-log-"+l+"-49040", "app-00")
				for _, log := range multilineLogTypes[l] {
					var messages []string
					err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(context.Context) (done bool, err error) {
						indices, err := getIndexNamesViaRoute(esRoute, token, "app")
						o.Expect(err).NotTo(o.HaveOccurred())
						resp, err := queryInESViaRoute(esRoute, token, indices, "_search", "{\"size\": "+strconv.Itoa(len(multilineLogTypes[l])*2+7)+", \"sort\": [{\"@timestamp\": {\"order\": \"desc\"}}], \"query\": {\"regexp\": {\"kubernetes.namespace_name\": \"multiline-log-"+l+"-49040\"}}}", "post")
						o.Expect(err).NotTo(o.HaveOccurred())
						var multilineLog SearchResult
						json.Unmarshal([]byte(resp), &multilineLog)
						for _, msg := range multilineLog.Hits.DataHits {
							messages = append(messages, msg.Source.Message)
						}
						if len(messages) == 0 {
							return false, nil
						}
						if !containSubstring(messages, log) {
							e2e.Logf("can't find log\n%s, try next round", log)
							return false, nil
						}
						return true, nil
					})
					if err != nil {
						e2e.Logf("\n\nlogs in ES:\n\n")
						for _, m := range messages {
							e2e.Logf(m)
						}
						e2e.Failf("%s logs are not parsed", l)
					}
				}
				e2e.Logf("\nfound %s logs in ES\n", l)
			}

			g.By("check data in loki")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			for _, t := range []string{"java", "go", "ruby"} {
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
					appLogs, err := lc.searchByNamespace("", "multiline-log-"+t+"-49040")
					if err != nil {
						return false, err
					}
					if appLogs.Status == "success" && len(appLogs.Data.Result) > 0 {
						return true, nil
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, "can't find "+t+" logs")
				for _, log := range multilineLogTypes[t] {
					var messages []string
					err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(context.Context) (done bool, err error) {
						dataInLoki, _ := lc.queryRange("", "{kubernetes_namespace_name=\"multiline-log-"+t+"-49040\"}", len(multilineLogTypes[t])*2, time.Now().Add(time.Duration(-2)*time.Hour), time.Now(), false)
						lokiLog := extractLogEntities(dataInLoki)
						var messages []string
						for _, log := range lokiLog {
							messages = append(messages, log.Message)
						}
						if len(messages) == 0 {
							return false, nil
						}
						if !containSubstring(messages, log) {
							e2e.Logf("can't find log\n%s, try next round", log)
							return false, nil
						}
						return true, nil
					})
					if err != nil {
						e2e.Logf("\n\nlogs in Loki:\n\n")
						for _, m := range messages {
							e2e.Logf(m)
						}
						e2e.Failf("%s logs are not parsed", t)
					}
				}
				e2e.Logf("\nfound %s logs in Loki\n", t)
			}
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-41134-Forward Log under different namespaces to different external Elasticsearch[Serial][Slow]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			appProj3 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj3, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy 2 external ES servers")
			oc.SetupProject()
			esProj1 := oc.Namespace()
			ees1 := externalES{
				namespace:  esProj1,
				version:    "6",
				serverName: "elasticsearch-server-1",
				loggingNS:  loggingNS,
			}
			defer ees1.remove(oc)
			ees1.deploy(oc)

			oc.SetupProject()
			esProj2 := oc.Namespace()
			ees2 := externalES{
				namespace:  esProj2,
				version:    "7",
				serverName: "elasticsearch-server-2",
				loggingNS:  loggingNS,
			}
			defer ees2.remove(oc)
			ees2.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41134.yaml"),
			}
			defer clf.delete(oc)
			qa := []string{appProj1, appProj2}
			qaProjects, _ := json.Marshal(qa)
			dev := []string{appProj1, appProj3}
			devProjects, _ := json.Marshal(dev)
			clf.create(oc, "QA_NS="+string(qaProjects), "DEV_NS="+string(devProjects), "URL_QA=http://"+ees1.serverName+"."+esProj1+".svc:9200", "URL_DEV=http://"+ees2.serverName+"."+esProj2+".svc:9200")

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

			g.By("check logs in external ES")
			ees1.waitForIndexAppear(oc, "app")
			for _, proj := range qa {
				ees1.waitForProjectLogsAppear(oc, proj, "app")
			}
			count1, _ := ees1.getDocCount(oc, "app", "{\"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+appProj3+"\"}}}")
			o.Expect(count1 == 0).Should(o.BeTrue())

			ees2.waitForIndexAppear(oc, "app")
			for _, proj := range dev {
				ees2.waitForProjectLogsAppear(oc, proj, "app")
			}
			count2, _ := ees2.getDocCount(oc, "app", "{\"query\": {\"match_phrase\": {\"kubernetes.namespace_name\": \""+appProj2+"\"}}}")
			o.Expect(count2 == 0).Should(o.BeTrue())

		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41240-BZ1905615 The application logs can be sent to the default ES when part of projects logs are sent to external aggregator[Serial][Slow]", func() {
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{serverName: "rsyslog", namespace: syslogProj, tls: false, loggingNS: loggingNS}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "41240.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "PROJ_NS="+appProj1, "URL=udp://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:514")

			g.By("deploy collector pods")
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

			g.By("check logs in internal ES")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cl.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "app")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "infra")
			waitForIndexAppear(cl.namespace, podList.Items[0].Name, "audit")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, appProj1, "app")
			waitForProjectLogsAppear(cl.namespace, podList.Items[0].Name, appProj2, "app")

			g.By("check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-45419-ClusterLogForwarder Forward logs to remote syslog with tls[Serial][Slow]", func() {
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{serverName: "rsyslog", namespace: syslogProj, tls: true, loggingNS: loggingNS, secretName: "rsyslog-45419"}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog-with-secret.yaml"),
				secretName:   rsyslog.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514")

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

			g.By("check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-55014-[LOG-2843]Forward logs to remote-syslog - mtls with private key passphrase[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy rsyslog server")
			oc.SetupProject()
			syslogProj := oc.Namespace()
			rsyslog := rsyslog{serverName: "rsyslog", namespace: syslogProj, tls: true, loggingNS: loggingNS, clientKeyPassphrase: "test-rsyslog-55014", secretName: "rsyslog-55014"}
			defer rsyslog.remove(oc)
			rsyslog.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-rsyslog-with-secret.yaml"),
				secretName:   rsyslog.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tls://"+rsyslog.serverName+"."+rsyslog.namespace+".svc:6514")

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

			g.By("check logs in rsyslog server")
			rsyslog.checkData(oc, true, "app-container.log")
			rsyslog.checkData(oc, true, "infra-container.log")
			rsyslog.checkData(oc, true, "audit.log")
			rsyslog.checkData(oc, true, "infra.log")

			g.By("check fluent.conf")
			fluentConf, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", cl.namespace, "cm/collector-config", `-ojsonpath='{.data.fluent\.conf}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(fluentConf).Should(o.ContainSubstring("client_cert_key_password"))
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-43250-Forward logs to fluentd enable mTLS with shared_key and tls_client_private_key_passphrase[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentd := fluentdServer{
				serverName:                 "fluentdtest",
				namespace:                  fluentdProj,
				serverAuth:                 true,
				clientAuth:                 true,
				clientPrivateKeyPassphrase: "testOCP43250",
				secretName:                 "fluentd-43250",
				loggingNS:                  loggingNS,
				inPluginType:               "forward",
			}
			defer fluentd.remove(oc)
			fluentd.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-fluentdforward.yaml"),
				secretName:   fluentd.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tls://"+fluentd.serverName+"."+fluentd.namespace+".svc:24224")

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

			g.By("check logs in fluentd server")
			fluentd.checkData(oc, true, "app.log")
			fluentd.checkData(oc, true, "infra-container.log")
			fluentd.checkData(oc, true, "audit.log")
			fluentd.checkData(oc, true, "infra.log")
		})
	})

	g.Context("Log Forward to user-managed loki", func() {

		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			g.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     loggingNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
		})

		//Author: kbharti@redhat.com
		g.It("CPaasrunOnly-Author:kbharti-High-43745-Forward to Loki using default value via http[Serial]", func() {
			//create a project and app to generate some logs
			g.By("create project for app logs")
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_non_json_log_template.json")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Create Loki project and deploy Loki Server
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)

			//Create ClusterLogForwarder
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100")

			//Create ClusterLogging instance
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

			g.By("Searching for Application Logs in Loki")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByNamespace("", appProj)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query is a success")

			g.By("Searching for Audit Logs in Loki")
			auditLogs, err := lc.searchLogsInLoki("", "{log_type=\"audit\"}")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(auditLogs.Status).Should(o.Equal("success"))
			o.Expect(auditLogs.Data.Result[0].Stream.LogType).Should(o.Equal("audit"))
			o.Expect(auditLogs.Data.Stats.Summary.BytesProcessedPerSecond).ShouldNot(o.BeZero())
			e2e.Logf("Audit Logs Query is a success")

			g.By("Searching for Infra Logs in Loki")
			infraLogs, err := lc.searchLogsInLoki("", "{log_type=\"infrastructure\"}")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(infraLogs.Status).Should(o.Equal("success"))
			o.Expect(infraLogs.Data.Result[0].Stream.LogType).Should(o.Equal("infrastructure"))
			o.Expect(infraLogs.Data.Stats.Summary.BytesProcessedPerSecond).ShouldNot(o.BeZero())
			e2e.Logf("Infra Logs Query is a success")
		})

		//Author: kbharti@redhat.com
		g.It("CPaasrunOnly-Author:kbharti-High-43746-Forward to Loki using loki.tenantkey[Serial]", func() {
			//create a project and app to generate some logs
			g.By("create project for app logs")
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_non_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Create Loki project and deploy Loki Server
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)
			tenantKey := "kubernetes_pod_name"

			//Create ClusterLogForwarder
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-set-tenantkey.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "TENANTKEY=kubernetes.pod_name")

			//Create ClusterLogging instance
			g.By("deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd", waitForReady: true,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Searching for Application Logs in Loki using tenantKey")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByKey("", tenantKey, appPodName.Items[0].Name)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query using tenantKey is a success")
		})

		g.It("CPaasrunOnly-Author:kbharti-High-43771-Forward to Loki using correct loki.tenantKey.kubernetes.namespace_name via http[Serial]", func() {
			//create a project and app to generate some logs
			g.By("create project for app logs")
			appProj := oc.Namespace()
			loglabeltemplate := filepath.Join(loggingBaseDir, "generatelog", "container_non_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", loglabeltemplate, "-p", "LABELS=centos-logtest").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Create Loki project and deploy Loki Server
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			defer loki.remove(oc)
			loki.deployLoki(oc)
			tenantKey := "kubernetes_namespace_name"

			//Create ClusterLogForwarder
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-set-tenantkey.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "TENANTKEY=kubernetes.namespace_name")

			//Create ClusterLogging instance
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

			g.By("Searching for Application Logs in Loki using tenantKey")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			appPodName, err := oc.AdminKubeClient().CoreV1().Pods(appProj).List(context.Background(), metav1.ListOptions{LabelSelector: "run=centos-logtest"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByKey("", tenantKey, appProj)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && appLogs.Data.Result[0].Stream.LogType == "application" && appLogs.Data.Result[0].Stream.KubernetesPodName == appPodName.Items[0].Name {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("Application Logs Query using namespace as tenantKey is a success")
		})

		g.It("CPaasrunOnly-Author:kbharti-Low-43770-Forward to Loki using loki.labelKeys which does not exist[Serial]", func() {
			//This case covers OCP-45697 and OCP-43770
			//create a project and app to generate some logs
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			g.By("create project1 for app logs")
			appProj1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile, "-p", "LABELS={\"negative\": \"centos-logtest\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create project2 for app logs")
			oc.SetupProject()
			appProj2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj2, "-f", jsonLogFile, "-p", "LABELS={\"positive\": \"centos-logtest\"}").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			// Create Loki project and deploy Loki Server
			oc.SetupProject()
			lokiNS := oc.Namespace()
			loki := externalLoki{"loki-server", lokiNS}
			loki.deployLoki(oc)
			labelKeys := "kubernetes_labels_positive"
			podLabel := "centos-logtest"

			//Create ClusterLogForwarder
			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-external-loki-set-labelkey.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+loki.name+"."+lokiNS+".svc:3100", "LABELKEY=kubernetes.labels.positive")

			//Create ClusterLogging instance
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

			//Positive Scenario - Matching labelKeys
			g.By("Searching for Application Logs in Loki using LabelKey - Postive match")
			route := "http://" + getRouteAddress(oc, loki.namespace, loki.name)
			lc := newLokiClient(route)
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := lc.searchByKey("", labelKeys, podLabel)
				if err != nil {
					return false, err
				}
				if appLogs.Status == "success" && appLogs.Data.Stats.Summary.BytesProcessedPerSecond != 0 && len(appLogs.Data.Result) > 0 && appLogs.Data.Stats.Ingester.TotalLinesSent != 0 {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "failed searching for application logs in Loki")
			e2e.Logf("App logs found with matching LabelKey: " + labelKeys + " and pod Label: " + podLabel)

			// Negative Scenario - No labelKeys are matching
			g.By("Searching for Application Logs in Loki using LabelKey - Negative match")
			labelKeys = "kubernetes_labels_negative"
			appLogs, err := lc.searchByKey("", labelKeys, podLabel)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(appLogs.Status).Should(o.Equal("success"))
			o.Expect(appLogs.Data.Result).Should(o.BeEmpty())
			o.Expect(appLogs.Data.Stats.Summary.BytesProcessedPerSecond).Should(o.BeZero())
			o.Expect(appLogs.Data.Stats.Store.TotalChunksDownloaded).Should((o.BeZero()))
			e2e.Logf("No App logs found with matching LabelKey: " + labelKeys + " and pod Label: " + podLabel)
		})

	})

	g.Context("Log Forward to Cloudwatch", func() {
		var cw cloudwatchSpec
		g.BeforeEach(func() {
			platform := exutil.CheckPlatform(oc)
			if platform != "aws" {
				g.Skip("Skip for non-supported platform, the support platform is AWS!!!")
			}
			_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "aws-creds", metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				g.Skip("Can not find secret/aws-creds. Maybe that is an aws STS cluster.")
			}
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			g.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     loggingNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
			g.By("init Cloudwatch test spec")
			cw.init(oc)
		})

		g.AfterEach(func() {
			cw.deleteGroups()
		})

		g.It("CPaasrunOnly-Author:anli-High-43839-Fluentd logs to Cloudwatch group by namespaceName and groupPrefix [Serial]", func() {
			cw.setGroupPrefix("logging-43839-" + getInfrastructureName(oc))
			cw.setGroupType("namespaceName")
			// Disable audit, so the test be more stable
			cw.setLogTypes("infrastructure", "application")

			g.By("create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch.yaml"),
				secretName:   cw.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType)

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

			g.By("check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		g.It("CPaasrunOnly-Author:anli-High-43840-Forward logs to Cloudwatch group by namespaceUUID and groupPrefix [Serial]", func() {
			cw.setGroupPrefix("logging-43840-" + getInfrastructureName(oc))
			cw.setGroupType("namespaceUUID")
			// Disable audit, so the test be more stable
			cw.setLogTypes("infrastructure", "application")

			g.By("create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			uuid, err := oc.WithoutNamespace().Run("get").Args("project", appProj, "-ojsonpath={.metadata.uid}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cw.selNamespacesUUID = []string{uuid}

			g.By("create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch.yaml"),
				secretName:   cw.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType)

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

			g.By("check logs in Cloudwatch")
			o.Expect(cw.logsFound()).To(o.BeTrue())
		})

		//author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-47052-[fluentd]CLF API change for Opt-in to multiline error detection (Forward to CloudWatch)[Serial]", func() {
			cw.setGroupPrefix("logging-47052-" + getInfrastructureName(oc))
			cw.setGroupType("logType")
			cw.setLogTypes("infrastructure", "audit", "application")

			g.By("create clusterlogforwarder/instance")
			defer resource{"secret", cw.secretName, cw.secretNamespace}.clear(oc)
			cw.createClfSecret(oc)

			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-cloudwatch.yaml"),
				secretName:   cw.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "REGION="+cw.awsRegion, "PREFIX="+cw.groupPrefix, "GROUPTYPE="+cw.groupType, "DETECT_MULTILINE_ERRORS=true")

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

			multilineLogTypes := map[string][]string{
				"java": {javaExc, complexJavaExc, nestedJavaExc},
			}

			g.By("create a pod to generate multiline error")
			ns := oc.Namespace()
			multilineLogFile := filepath.Join(loggingBaseDir, "generatelog", "multiline-error-log.yaml")
			err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", ns, "-f", multilineLogFile, "-p", "LOG_TYPE=java", "-p", "RATE=120").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForDeploymentPodsToBeReady(oc, ns, "multiline-log")
			cw.selAppNamespaces = []string{ns}

			g.By("check logs in Cloudwatch")
			logGroupName := cw.groupPrefix + ".application"
			o.Expect(cw.logsFound()).To(o.BeTrue())

			filteredLogs, err := cw.getLogRecordsFromCloudwatchByNamespace(30, logGroupName, ns)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(filteredLogs) > 0).Should(o.BeTrue(), "couldn't filter logs by namespace")
			var filteredMessages []string
			for _, f := range filteredLogs {
				filteredMessages = append(filteredMessages, f.Message)
			}
			for _, msg := range multilineLogTypes["java"] {
				o.Expect(containSubstring(filteredMessages, msg)).Should(o.BeTrue(), "%s log is not found", msg)
			}
		})

	})

	g.Context("Log Forward to Kafka", func() {

		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			g.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     loggingNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-41726-Forward logs to different kafka brokers[Serial][Slow]", func() {
			nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodes.Items[0].Status.NodeInfo.Architecture == "arm64" {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			var (
				subTemplate       = filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml")
				SingleNamespaceOG = filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml")
				jsonLogFile       = filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			)
			g.By("create log producer")
			appProj := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("subscribe AMQ kafka into 2 different namespaces")
			// to avoid collecting kafka logs, deploy kafka in project openshift-*
			amqNs1 := "openshift-amq-1" + getRandomString()
			amqNs2 := "openshift-amq-2" + getRandomString()
			catsrc := CatalogSourceObjects{"stable", "redhat-operators", "openshift-marketplace"}
			amq1 := SubscriptionObjects{
				OperatorName:  "amq-streams-cluster-operator",
				Namespace:     amqNs1,
				OperatorGroup: SingleNamespaceOG,
				Subscription:  subTemplate,
				PackageName:   "amq-streams",
				CatalogSource: catsrc,
			}
			amq2 := SubscriptionObjects{
				OperatorName:  "amq-streams-cluster-operator",
				Namespace:     amqNs2,
				OperatorGroup: SingleNamespaceOG,
				Subscription:  subTemplate,
				PackageName:   "amq-streams",
				CatalogSource: catsrc,
			}
			topicName := "topic-logging-app"
			kafkaClusterName := "kafka-cluster"
			fips := isFipsEnabled(oc)
			for _, amq := range []SubscriptionObjects{amq1, amq2} {
				defer deleteNamespace(oc, amq.Namespace)
				//defer amq.uninstallOperator(oc)
				amq.SubscribeOperator(oc)
				if fips {
					//disable FIPS_MODE due to "java.io.IOException: getPBEAlgorithmParameters failed: PBEWithHmacSHA256AndAES_256 AlgorithmParameters not available"
					err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub/"+amq.PackageName, "-n", amq.Namespace, "-p", "{\"spec\": {\"config\": {\"env\": [{\"name\": \"FIPS_MODE\", \"value\": \"disabled\"}]}}}", "--type=merge").Execute()
					o.Expect(err).NotTo(o.HaveOccurred())
				}
				// before creating kafka, check the existence of crd kafkas.kafka.strimzi.io
				checkResource(oc, true, true, "kafka.strimzi.io", []string{"crd", "kafkas.kafka.strimzi.io", "-ojsonpath={.spec.group}"})
				kafka := resource{"kafka", kafkaClusterName, amq.Namespace}
				kafkaTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "kafka-cluster-no-auth.yaml")
				//defer kafka.clear(oc)
				kafka.applyFromTemplate(oc, "-n", kafka.namespace, "-f", kafkaTemplate, "-p", "NAME="+kafka.name, "NAMESPACE="+kafka.namespace, "VERSION=3.4.0", "MESSAGE_VERSION=3.4.0")
				o.Expect(err).NotTo(o.HaveOccurred())
				// create topics
				topicTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "kafka-topic.yaml")
				topic := resource{"Kafkatopic", topicName, amq.Namespace}
				//defer topic.clear(oc)
				err = topic.applyFromTemplate(oc, "-n", topic.namespace, "-f", topicTemplate, "-p", "NAME="+topic.name, "CLUSTER_NAME="+kafka.name, "NAMESPACE="+topic.namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				// wait for kafka cluster to be ready
				waitForPodReadyWithLabel(oc, kafka.namespace, "app.kubernetes.io/instance="+kafka.name)
			}
			g.By("forward logs to Kafkas")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_kafka_multi_brokers.yaml"),
			}
			defer clf.delete(oc)
			brokers, _ := json.Marshal([]string{"tls://" + kafkaClusterName + "-kafka-bootstrap." + amqNs1 + ".svc:9092", "tls://" + kafkaClusterName + "-kafka-bootstrap." + amqNs2 + ".svc:9092"})
			clf.create(oc, "TOPIC="+topicName, "BROKERS="+string(brokers))

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

			//create consumer pod
			for _, ns := range []string{amqNs1, amqNs2} {
				consumerTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "topic-consumer.yaml")
				consumer := resource{"job", topicName + "-consumer", ns}
				//defer consumer.clear(oc)
				err = consumer.applyFromTemplate(oc, "-n", consumer.namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.name, "NAMESPACE="+consumer.namespace, "KAFKA_TOPIC="+topicName, "CLUSTER_NAME="+kafkaClusterName)
				o.Expect(err).NotTo(o.HaveOccurred())
				waitForPodReadyWithLabel(oc, consumer.namespace, "job-name="+consumer.name)
			}

			g.By("check data in kafka")
			for _, consumer := range []resource{{"job", topicName + "-consumer", amqNs1}, {"job", topicName + "-consumer", amqNs2}} {
				consumerPods, err := oc.AdminKubeClient().CoreV1().Pods(consumer.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=" + consumer.name})
				o.Expect(err).NotTo(o.HaveOccurred())
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
					logs, err := getDataFromKafkaByLogType(oc, consumer.namespace, consumerPods.Items[0].Name, "application")
					if err != nil {
						return false, err
					}
					for _, log := range logs {
						if log.Kubernetes.NamespaceName == appProj {
							return true, nil
						}
					}
					return false, nil
				})
				exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", consumer.namespace, consumer.name))
			}
		})

		// author gkarager@redhat.com
		g.It("CPaasrunOnly-Author:gkarager-Medium-45368-Forward logs to kafka using sasl-plaintext[Serial][Slow]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy zookeeper")
			kafka := kafka{
				namespace:      loggingNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "sasl-plaintext",
				pipelineSecret: "kafka-fluentd",
				collectorType:  "fluentd",
				loggingNS:      loggingNS,
			}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9092/clo-topic"

			g.By("Create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_kafka.yaml"),
				secretName:   kafka.pipelineSecret,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL="+kafkaEndpoint)

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				logs, err := getDataFromKafkaByLogType(oc, kafka.namespace, consumerPodPodName, "application")
				if err != nil {
					return false, err
				}
				for _, log := range logs {
					if log.Kubernetes.NamespaceName == appProj {
						return true, nil
					}
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		// author gkarager@redhat.com
		g.It("CPaasrunOnly-Author:gkarager-Medium-41771-Forward logs to kafka using sasl-ssl[Serial][Slow]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy zookeeper")
			kafka := kafka{
				namespace:      loggingNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "sasl-ssl",
				pipelineSecret: "kafka-fluentd",
				collectorType:  "fluentd",
				loggingNS:      loggingNS,
			}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9093/clo-topic"

			g.By("Create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_kafka.yaml"),
				secretName:   kafka.pipelineSecret,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL="+kafkaEndpoint)

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				logs, err := getDataFromKafkaByLogType(oc, kafka.namespace, consumerPodPodName, "application")
				if err != nil {
					return false, err
				}
				for _, log := range logs {
					if log.Kubernetes.NamespaceName == appProj {
						return true, nil
					}
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		// author gkarager@redhat.com
		g.It("CPaasrunOnly-Author:gkarager-Medium-32333-Forward logs to kafka topic via Mutual Chained certificates[Serial][Slow]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			kafka := kafka{
				namespace:      loggingNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "plaintext-ssl",
				pipelineSecret: "kafka-fluentd",
				collectorType:  "fluentd",
				loggingNS:      loggingNS,
			}
			g.By("Deploy zookeeper")
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9093/clo-topic"

			g.By("Create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf_kafka.yaml"),
				secretName:   kafka.pipelineSecret,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL="+kafkaEndpoint)

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				logs, err := getDataFromKafkaByLogType(oc, kafka.namespace, consumerPodPodName, "application")
				if err != nil {
					return false, err
				}
				for _, log := range logs {
					if log.Kubernetes.NamespaceName == appProj {
						return true, nil
					}
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})
	})

	g.Context("Log Forward to Logstash", func() {

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
		g.It("CPaasrunOnly-Author:qitang-Medium-48844-Fluentd forward logs to logstash[Serial]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy Logstash")
			oc.SetupProject()
			logstash := logstash{
				name:      "logstash",
				namespace: oc.Namespace(),
			}
			logstash.deploy(oc)
			defer logstash.remove(oc)

			g.By("Create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-fluentdforward-no-secret.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=tcp://"+logstash.name+"."+logstash.namespace+".svc:24114")

			g.By("Deploy collector pods")
			cl := clusterlogging{
				name:          "instance",
				namespace:     loggingNS,
				collectorType: "fluentd",
				waitForReady:  true,
				templateFile:  filepath.Join(loggingBaseDir, "clusterlogging", "collector_only.yaml"),
			}
			defer cl.delete(oc)
			cl.create(oc)

			logstash.checkData(oc, true, "infra.log")
			logstash.checkData(oc, true, "audit.log")
			logstash.checkData(oc, true, "app.log")
		})
	})

	g.Context("fluentd forward logs to external store over http", func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		g.BeforeEach(func() {
			g.By("deploy CLO")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			CLO.SubscribeOperator(oc)
		})
		g.It("CPaasrunOnly-Author:anli-Medium-60934-fluentd Forward logs to fluentd over http - https[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentdS := fluentdServer{
				serverName:   "fluentdtest",
				namespace:    fluentdProj,
				serverAuth:   true,
				clientAuth:   false,
				secretName:   "to-fluentd-60934",
				loggingNS:    loggingNS,
				inPluginType: "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-forward-all-over-https-template.yaml"),
				secretName:   fluentdS.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224")

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

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:anli-Medium-60925-fluentd Forward logs to fluentd over http - http[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentdS := fluentdServer{
				serverName:   "fluentdtest",
				namespace:    fluentdProj,
				serverAuth:   false,
				clientAuth:   false,
				loggingNS:    loggingNS,
				inPluginType: "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-forward-all-over-http-template.yaml"),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=http://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224")

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

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})

		g.It("CPaasrunOnly-Author:anli-Medium-60935-fluentd Forward logs to fluentd over http - TLSSkipVerify[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			fluentdS := fluentdServer{
				serverName:   "fluentdtest",
				namespace:    fluentdProj,
				serverAuth:   true,
				clientAuth:   false,
				secretName:   "to-fluentd-60935",
				loggingNS:    loggingNS,
				inPluginType: "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			//Create a fake secret from root ca which is used for TLSSkipVerify
			fakeSecret := resource{"secret", "fake-bundle-60935", loggingNS}
			defer fakeSecret.clear(oc)
			dirname := "/tmp/60936-keys"
			defer os.RemoveAll(dirname)
			err = os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("cm/kube-root-ca.crt", "-n", loggingNS, "--confirm", "--to="+dirname).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", fakeSecret.name, "-n", fakeSecret.namespace, "--from-file=ca-bundle.crt="+dirname+"/ca.crt").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-forward-all-over-https-skipverify-template.yaml"),
				secretName:   fakeSecret.name,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224")

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

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})
		// author anli@redhat.com
		g.It("CPaasrunOnly-Author:anli-Medium-60937-fluentd forward logs to fluentdserver over http - mtls[Serial]", func() {
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy fluentd server")
			oc.SetupProject()
			fluentdProj := oc.Namespace()
			keyPassphase := getRandomString()
			fluentdS := fluentdServer{
				serverName:                 "fluentdtest",
				namespace:                  fluentdProj,
				serverAuth:                 true,
				clientAuth:                 true,
				clientPrivateKeyPassphrase: keyPassphase,
				secretName:                 "to-fluentd-60937",
				loggingNS:                  loggingNS,
				inPluginType:               "http",
			}
			defer fluentdS.remove(oc)
			fluentdS.deploy(oc)

			g.By("create clusterlogforwarder/instance")
			clf := clusterlogforwarder{
				name:         "instance",
				namespace:    loggingNS,
				templateFile: filepath.Join(loggingBaseDir, "clusterlogforwarder", "clf-forward-all-over-https-template.yaml"),
				secretName:   fluentdS.secretName,
			}
			defer clf.delete(oc)
			clf.create(oc, "URL=https://"+fluentdS.serverName+"."+fluentdS.namespace+".svc:24224")

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

			g.By("check logs in fluentd server")
			fluentdS.checkData(oc, true, "app.log")
			fluentdS.checkData(oc, true, "audit.log")
			fluentdS.checkData(oc, true, "infra.log")
		})
	})

})
