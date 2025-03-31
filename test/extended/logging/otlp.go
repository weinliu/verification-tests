package logging

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging NonPreRelease Otlp output testing", func() {
	defer g.GinkgoRecover()
	var (
		oc             = exutil.NewCLI("vector-otlp", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.BeforeEach(func() {
		loggingBaseDir = exutil.FixturePath("testdata", "logging")
		CLO := SubscriptionObjects{
			OperatorName:  "cluster-logging-operator",
			Namespace:     cloNS,
			PackageName:   "cluster-logging",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		exutil.By("deploy CLO")
		CLO.SubscribeOperator(oc)
		oc.SetupProject()
	})

	g.It("Author:qitang-CPaasrunOnly-Critical-68961-Forward logs to OTEL collector", func() {
		var (
			expectedCSV       string
			operatorInstalled bool
		)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "openshift-operators", "-ojsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		csvs := strings.Split(output, " ")
		for _, csv := range csvs {
			if strings.Contains(csv, "opentelemetry-operator.v") {
				expectedCSV = csv
				break
			}
		}
		if len(expectedCSV) > 0 {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "openshift-operators", expectedCSV, "-ojsonpath={.status.phase}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if output == "Succeeded" {
				operatorInstalled = true
			}
		}
		if !operatorInstalled {
			exutil.By("Deploy opentelemetry-operator")
			otelOperator := SubscriptionObjects{
				OperatorName:  "opentelemetry-operator",
				Namespace:     "openshift-opentelemetry-operator",
				PackageName:   "opentelemetry-product",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
				CatalogSource: CatalogSourceObjects{
					Channel: "stable",
				},
				OperatorPodLabel: "app.kubernetes.io/name=opentelemetry-operator",
			}
			defer otelOperator.uninstallOperator(oc)
			otelOperator.SubscribeOperator(oc)
		}
		exutil.By("Deploy OTEL collector")
		otelTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "otel", "otel-collector.yaml")
		otel := resource{
			kind:      "opentelemetrycollectors",
			name:      "otel",
			namespace: oc.Namespace(),
		}
		defer otel.clear(oc)
		err = otel.applyFromTemplate(oc, "-f", otelTemplate, "-n", otel.namespace, "-p", "NAMESPACE="+otel.namespace, "-p", "NAME="+otel.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodReadyWithLabel(oc, otel.namespace, "app.kubernetes.io/component=opentelemetry-collector")
		svc := "http://" + otel.name + "-collector." + otel.namespace + ".svc:4318"

		exutil.By("Deploy clusterlogforwarder")
		clf := clusterlogforwarder{
			name:                      "otlp-68961",
			namespace:                 oc.Namespace(),
			serviceAccountName:        "logcollector-68961",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "otlp.yaml"),
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
		}
		defer clf.delete(oc)
		clf.create(oc, "URL="+svc)
		//exclude logs from  project otel.namespace because the OTEL collector writes received logs to stdout
		patch := `[{"op": "add", "path": "/spec/inputs", "value": [{"name": "new-app", "type": "application", "application": {"excludes": [{"namespace":"` + otel.namespace + `"}]}}]},{"op": "replace", "path": "/spec/pipelines/0/inputRefs", "value": ["new-app", "infrastructure", "audit"]}]`
		clf.update(oc, "", patch, "--type=json")
		clf.waitForCollectorPodsReady(oc)

		exutil.By("check collector configurations")
		expectedConfigs := []string{
			"compression = \"gzip\"",
			`[sinks.output_otlp.batch]
max_bytes = 10000000`,
			`[sinks.output_otlp.buffer]
type = "disk"
when_full = "block"
max_size = 268435488`,
			`[sinks.output_otlp.request]
retry_initial_backoff_secs = 5
retry_max_duration_secs = 20`,
		}
		result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", expectedConfigs...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		exutil.By("check log data in OTEL collector")
		time.Sleep(1 * time.Minute)
		otelCollector, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", otel.namespace, "pod", "-l", "app.kubernetes.io/component=opentelemetry-collector", "-ojsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		logs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", otel.namespace, otelCollector, "--tail=60").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(logs, "LogRecord")).Should(o.BeTrue())

	})

	//author: qitang@redhat.com
	g.It("Author:qitang-CPaasrunOnly-ConnectedOnly-High-76728-Add stream info to data model OTEL LokiStack[Serial][Slow]", func() {
		s := getStorageType(oc)
		if len(s) == 0 {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		sc, _ := getStorageClassName(oc)
		if len(sc) == 0 {
			g.Skip("The cluster doesn't have a storage class for this test!")
		}
		LO := SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   "loki-operator",
			Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
			OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
		}
		exutil.By("deploy Loki Operator")
		LO.SubscribeOperator(oc)

		multilineLogs := []string{
			javaExc, complexJavaExc, nestedJavaExc,
			goExc, goOnGaeExc, goSignalExc, goHTTP,
			rubyExc, railsExc,
			clientJsExc, nodeJsExc, v8JsExc,
			csharpAsyncExc, csharpNestedExc, csharpExc,
			pythonExc,
			phpOnGaeExc, phpExc,
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
		}

		exutil.By("Deploying LokiStack")
		lokiStackTemplate := filepath.Join(loggingBaseDir, "lokistack", "lokistack-simple.yaml")
		ls := lokiStack{
			name:          "loki-76727",
			namespace:     loggingNS,
			tSize:         "1x.demo",
			storageType:   s,
			storageSecret: "storage-secret-76727",
			storageClass:  sc,
			bucketName:    "logging-loki-76727-" + getInfrastructureName(oc),
			template:      lokiStackTemplate,
		}
		defer ls.removeObjectStorage(oc)
		err := ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer ls.removeLokiStack(oc)
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
		e2e.Logf("LokiStack deployed")
		lokiGatewaySVC := "https://" + ls.name + "-gateway-http." + ls.namespace + ".svc:8080"

		exutil.By("create a CLF to test forward to lokistack")
		clf := clusterlogforwarder{
			name:                      "otlp-76727",
			namespace:                 loggingNS,
			serviceAccountName:        "logcollector-76727",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "otlp-lokistack.yaml"),
			secretName:                "lokistack-secret-76727",
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		clf.createServiceAccount(oc)
		defer removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "logging-collector-logs-writer")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer resource{"secret", clf.secretName, clf.namespace}.clear(oc)
		ls.createSecretFromGateway(oc, clf.secretName, clf.namespace, "")
		defer clf.delete(oc)
		clf.create(oc, "URL="+lokiGatewaySVC)

		exutil.By("create some pods to generate multiline errors")
		multilineLogFile := filepath.Join(loggingBaseDir, "generatelog", "multiline-error-log.yaml")
		ioStreams := []string{"stdout", "stderr"}
		for _, ioStream := range ioStreams {
			ns := "multiline-log-" + ioStream + "-76727"
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--wait=false").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "deploy/multiline-log", "cm/multiline-log").Execute()
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-n", ns, "-f", multilineLogFile, "-p", "OUT_STREAM="+ioStream, "-p", "RATE=60.00").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("checking app, infra and audit logs in loki")
		defer removeClusterRoleFromServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		err = addClusterRoleToServiceAccount(oc, oc.Namespace(), "default", "cluster-admin")
		o.Expect(err).NotTo(o.HaveOccurred())
		bearerToken := getSAToken(oc, "default", oc.Namespace())
		route := "https://" + getRouteAddress(oc, ls.namespace, ls.name)
		lc := newLokiClient(route).withToken(bearerToken).retry(5)
		for _, logType := range []string{"application", "infrastructure", "audit"} {
			lc.waitForLogsAppearByKey(logType, "log_type", logType)
		}

		for _, ioStream := range ioStreams {
			lc.waitForLogsAppearByProject("application", "multiline-log-"+ioStream+"-76727")
			dataInLoki, _ := lc.searchByNamespace("application", "multiline-log-"+ioStream+"-76727")
			for _, log := range dataInLoki.Data.Result {
				o.Expect(log.Stream.LogIOStream == ioStream).Should(o.BeTrue(), `iostream is wrong, expected: `+ioStream+`, got: `+log.Stream.LogIOStream)
				for _, value := range log.Values {
					message := convertInterfaceToArray(value)
					o.Expect(containSubstring(multilineLogs, message[1])).Should(o.BeTrue(), fmt.Sprintf("Parse multiline error failed, iostream: %s, message: \n%s", ioStream, message[1]))
				}
			}

		}
	})

	g.It("Author:qitang-CPaasrunOnly-Medium-75351-Tech preview annotation should be enabled when forwarding logs via Otlp", func() {
		exutil.By("Deploy collector pods")
		clf := clusterlogforwarder{
			name:                      "otlp-68961",
			namespace:                 oc.Namespace(),
			serviceAccountName:        "logcollector-68961",
			templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "otlp.yaml"),
			collectApplicationLogs:    true,
			collectAuditLogs:          true,
			collectInfrastructureLogs: true,
			waitForPodReady:           true,
		}
		defer clf.delete(oc)
		clf.create(oc, "URL=http://fake-otel-collector."+clf.namespace+".svc:4318")

		exutil.By("remove the tech-preview annotation from CLF")
		patch := `[{"op": "remove", "path": "/metadata/annotations"}]`
		clf.update(oc, "", patch, "--type=json")
		checkResource(oc, true, false, `output "otlp" requires a valid tech-preview annotation`, []string{"clusterlogforwarder.observability.openshift.io", clf.name, "-n", clf.namespace, "-ojsonpath={.status.outputConditions[*].message}"})

		exutil.By("Add back the annotations, then set the value to disabled")
		clf.update(oc, "", `{"metadata": {"annotations": {"observability.openshift.io/tech-preview-otlp-output": "enabled"}}}`, "--type=merge")
		checkResource(oc, false, false, `output "otlp" requires a valid tech-preview annotation`, []string{"clusterlogforwarder.observability.openshift.io", clf.name, "-n", clf.namespace, "-ojsonpath={.status.outputConditions[*].message}"})

		clf.update(oc, "", `{"metadata": {"annotations": {"observability.openshift.io/tech-preview-otlp-output": "disabled"}}}`, "--type=merge")
		checkResource(oc, true, false, `output "otlp" requires a valid tech-preview annotation`, []string{"clusterlogforwarder.observability.openshift.io", clf.name, "-n", clf.namespace, "-ojsonpath={.status.outputConditions[*].message}"})
	})
})
