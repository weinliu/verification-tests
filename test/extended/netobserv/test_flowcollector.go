package netobserv

import (
	"fmt"
	"math"
	"os/exec"
	filePath "path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-netobserv] Network_Observability", func() {

	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("netobserv", exutil.KubeConfigPath())
		// NetObserv Operator variables
		netobservNS   = "openshift-netobserv-operator"
		NOPackageName = "netobserv-operator"
		catsrc        = resource{"catsrc", "qe-app-registry", "openshift-marketplace"}
		NOSource      = CatalogSourceObjects{"v1.0.x", catsrc.name, catsrc.namespace}

		// Template directories
		baseDir         = exutil.FixturePath("testdata", "netobserv")
		lokiDir         = exutil.FixturePath("testdata", "netobserv", "loki")
		kafkaDir        = exutil.FixturePath("testdata", "netobserv", "kafka")
		subscriptionDir = exutil.FixturePath("testdata", "netobserv", "subscription")
		flowFixturePath = filePath.Join(baseDir, "flowcollector_v1beta1_template.yaml")

		// Namespace object
		OperatorNS = OperatorNamespace{
			Name:                netobservNS,
			NamespaceTemplate:   filePath.Join(subscriptionDir, "namespace.yaml"),
			RoleTemplate:        filePath.Join(subscriptionDir, "role.yaml"),
			RoleBindingTemplate: filePath.Join(subscriptionDir, "roleBinding.yaml"),
		}
		NO = SubscriptionObjects{
			OperatorName:  "netobserv-operator",
			Namespace:     netobservNS,
			PackageName:   NOPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: NOSource,
		}
		// Loki Operator variables
		lokiNS          = "openshift-operators-redhat"
		lokiPackageName = "loki-operator"
		ls              lokiStack
		priorExists     = false
		lokiSource      = CatalogSourceObjects{"stable", catsrc.name, catsrc.namespace}
		LO              = SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     lokiNS,
			PackageName:   lokiPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: lokiSource,
		}
	)

	g.BeforeEach(func() {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
		if strings.Contains(output, "NotFound") {
			g.Skip("Skip since catalogsource/qe-app-registry is not installed")
		}

		g.By(fmt.Sprintf("Subscribe operators to %s channel", NOSource.Channel))

		// Check if Network Observability Operator is already present
		NOexisting := checkOperatorStatus(oc, netobservNS, NOPackageName)

		// Create operatorNS and deploy operator if not present
		if !NOexisting {
			OperatorNS.deployOperatorNamespace(oc)
			NO.SubscribeOperator(oc)
			// Check if NO operator is deployed
			waitForPodReadyWithLabel(oc, netobservNS, "app="+NO.OperatorName)
			NOStatus := checkOperatorStatus(oc, netobservNS, NOPackageName)
			o.Expect((NOStatus)).To(o.BeTrue())

			// check if flowcollector API exists
			flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
			o.Expect((flowcollectorAPIExists)).To(o.BeTrue())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
			g.Skip("Current platform does not have enough resources available for this test!")
		}

		g.By("Deploy loki operator")
		namespace := oc.Namespace()
		priorExists = checkOperatorStatus(oc, lokiNS, lokiPackageName)

		// Don't delete if Loki Operator existed already before NetObserv
		// If Loki Operator was installed by NetObserv tests,
		// it will install and uninstall after each spec/test.
		if !priorExists {
			LO.SubscribeOperator(oc)
			waitForPodReadyWithLabel(oc, lokiNS, "name="+LO.OperatorName)
		}

		g.By("Deploy lokiStack")
		// get storageClass Name
		sc, err := getStorageClassName(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		lokiTenant := "openshift-network"

		lokiStackTemplate := filePath.Join(lokiDir, "lokistack-simple.yaml")
		objectStorageType := getStorageType(oc)

		ls = lokiStack{
			Name:          "lokistack",
			Namespace:     namespace,
			TSize:         "1x.extra-small",
			StorageType:   objectStorageType,
			StorageSecret: "objectstore-secret",
			StorageClass:  sc,
			BucketName:    "netobserv-loki-" + getInfrastructureName(oc),
			Tenant:        lokiTenant,
			Template:      lokiStackTemplate,
		}

		err = ls.prepareResourcesForLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = ls.deployLokiStack(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		ls.waitForLokiStackToBeReady(oc)
	})

	g.AfterEach(func() {
		ls.removeObjectStorage(oc)
		ls.removeLokiStack(oc)
		if !priorExists {
			LO.uninstallOperator(oc)
		}
	})

	g.It("Author:aramesha-High-54043-verify metric server on TLS [Serial]", func() {
		namespace := oc.Namespace()
		g.By("Deploy FlowCollector")
		// loki URL
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, ls.Namespace)

		flow := Flowcollector{
			Namespace:           namespace,
			Template:            flowFixturePath,
			MetricServerTLSType: "AUTO",
			LokiURL:             lokiURL,
			LokiAuthToken:       "HOST",
			LokiTLSEnable:       true,
			LokiTLSCertName:     fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Deploy metrics")
		// metrics Template path
		promMetricsFixturePath := filePath.Join(baseDir, "monitoring.yaml")

		metric := Metrics{
			Namespace: namespace,
			Template:  promMetricsFixturePath,
			Scheme:    "https",
		}

		metric.createMetrics(oc)

		g.By("Ensure FLP pods eBPF pods and lokistack are ready")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		g.By("Verify metrics by running curl on FLP pod")
		curlDest := fmt.Sprintf("https://flowlogs-pipeline-prom.%s.svc:9102/metrics", namespace)

		podName := getFlowlogsPipelinePod(oc, namespace, "flowlogs-pipeline")
		// certificate path
		flowlogsPodCertPath := "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
		verifyCurl(oc, podName, namespace, curlDest, flowlogsPodCertPath)

		g.By("Verify metrics by running curl on prometheus pod")
		// certificate path
		promCertPath := "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt"
		verifyCurl(oc, "prometheus-k8s-0", "openshift-monitoring", curlDest, promCertPath)
	})

	g.It("Author:aramesha-High-50504-Verify flowlogs-pipeline metrics and health [Serial]", func() {
		namespace := oc.Namespace()
		g.By("Deploy FlowCollector")
		// loki URL
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, namespace)

		flow := Flowcollector{
			Namespace:       namespace,
			Template:        flowFixturePath,
			LokiURL:         lokiURL,
			LokiAuthToken:   "HOST",
			LokiTLSEnable:   true,
			LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Ensure FLP pods, eBPF pods and lokistack are ready")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		// get all flowlogs pipeline pods
		FLPpods, err := exutil.GetAllPodsWithLabel(oc, namespace, "app=flowlogs-pipeline")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Ensure metrics are reported")
		// Metrics URL
		curlMetrics := fmt.Sprintf("http://flowlogs-pipeline-prom.%s.svc:9102/metrics", namespace)

		for _, pod := range FLPpods {
			command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", "-v", "-L", curlMetrics}
			output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().OutputToFile("metrics.txt")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).NotTo(o.BeEmpty(), "No Metrics found")

			metric, _ := exec.Command("bash", "-c", "cat "+output+" | grep -o \"HTTP/1.1.*\"| tail -1 | awk '{print $2}'").Output()
			httpCode := strings.TrimSpace(string(metric))
			o.Expect(httpCode).NotTo(o.BeEmpty(), "HTTP Code not found")
			e2e.Logf("The http code is : %v", httpCode)
			o.Expect(httpCode).To(o.Equal("200"))
		}

		g.By("Ensure liveliness/readiness of FLP pods")
		// Liveliness URL
		curlLive := "http://localhost:8080/live"

		for _, pod := range FLPpods {
			command := []string{"exec", "-n", namespace, pod, "--", "curl", "-s", curlLive}
			output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.Equal("{}"))
		}
	})

	g.It("Author:aramesha-High-54840-Use console-plugin authorize API with HOST authToken [Serial]", func() {
		namespace := oc.Namespace()
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, namespace)

		g.By("Deploy flowcollector with loki in Host and TLS enabled")
		flow := Flowcollector{
			Namespace:           namespace,
			Template:            flowFixturePath,
			MetricServerTLSType: "AUTO",
			LokiURL:             lokiURL,
			LokiAuthToken:       "HOST",
			LokiTLSEnable:       true,
			LokiTLSCertName:     fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
		}
		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		// Service Account name
		g.By("Ensure flows are observed and all pods are running")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// verify logs
		sa_name := "flowlogs-pipeline"
		err := verifyTime(oc, namespace, ls.Name, sa_name, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonPreRelease-Author:aramesha-High-53597-Verify network flows are captured with Kafka [Serial]", func() {
		namespace := oc.Namespace()
		g.By("Subscribe to AMQ operator")
		catsrc := CatalogSourceObjects{"stable", "redhat-operators", "openshift-marketplace"}
		amq := SubscriptionObjects{
			OperatorName:  "amq-streams-cluster-operator",
			Namespace:     namespace,
			PackageName:   "amq-streams",
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "singlenamespace-og.yaml"),
			CatalogSource: catsrc,
		}

		defer amq.uninstallOperator(oc)
		amq.SubscribeOperator(oc)
		// before creating kafka, check the existence of crd kafkas.kafka.strimzi.io
		checkResource(oc, true, true, "kafka.strimzi.io", []string{"crd", "kafkas.kafka.strimzi.io", "-ojsonpath={.spec.group}"})

		g.By("Deploy KAFKA")
		// Kafka metrics config Template path
		kafkaMetricsPath := filePath.Join(kafkaDir, "kafka-metrics-config.yaml")

		kafkaMetrics := KafkaMetrics{
			Namespace: namespace,
			Template:  kafkaMetricsPath,
		}

		// Kafka Template path
		kafkaPath := filePath.Join(kafkaDir, "kafka-default.yaml")

		kafka := Kafka{
			Name:         "kafka-cluster",
			Namespace:    namespace,
			Template:     kafkaPath,
			StorageClass: ls.StorageClass,
		}

		// Kafka Topic path
		kafkaTopicPath := filePath.Join(kafkaDir, "kafka-topic.yaml")

		kafkaTopic := KafkaTopic{
			TopicName: "network-flows",
			Name:      kafka.Name,
			Namespace: namespace,
			Template:  kafkaTopicPath,
		}

		defer kafkaTopic.deleteKafkaTopic(oc)
		defer kafka.deleteKafka(oc)
		kafkaMetrics.deployKafkaMetrics(oc)
		kafka.deployKafka(oc)
		kafkaTopic.deployKafkaTopic(oc)

		g.By("Check if Kafka and Kafka topic are ready")
		// Wait for Kafka and KafkaTopic to be ready
		waitForKafkaReady(oc, kafka.Name, kafka.Namespace)
		waitForKafkaTopicReady(oc, kafkaTopic.TopicName, kafkaTopic.Namespace)

		g.By("Deploy FlowCollector")
		// loki URL
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, ls.Namespace)

		flow := Flowcollector{
			Namespace:           namespace,
			DeploymentModel:     "KAFKA",
			Template:            flowFixturePath,
			MetricServerTLSType: "AUTO",
			LokiURL:             lokiURL,
			LokiAuthToken:       "HOST",
			LokiTLSEnable:       true,
			LokiTLSCertName:     fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
			KafkaAddress:        fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s", namespace),
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Ensure flows are observed and all pods are running")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		// verify logs
		err := verifyTime(oc, namespace, ls.Name, "netobserv-plugin", ls.Namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonPreRelease-Author:memodi-High-49107-High-45304-High-54929-High-54840-Verify flow correctness with FORWARD authToken [Disruptive][Slow]", func() {
		namespace := oc.Namespace()
		g.By("Deploying test server and client pods")
		template := filePath.Join(baseDir, "test-client-server_template.yaml")
		testTemplate := TestClientServerTemplate{
			ServerNS:   "test-server",
			ClientNS:   "test-client",
			ObjectSize: "100K",
			Template:   template,
		}
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ClientNS)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testTemplate.ServerNS)
		err := testTemplate.createTestClientServer(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Deploy FlowCollector")
		// loki URL
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, namespace)

		flow := Flowcollector{
			Namespace:       namespace,
			DeploymentModel: "DIRECT",
			Template:        flowFixturePath,
			LokiURL:         lokiURL,
			LokiAuthToken:   "FORWARD",
			LokiTLSEnable:   true,
			LokiTLSCertName: fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		g.By("Ensure flows are observed and all pods are running")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")
		g.By("Wait for 2 mins before logs gets collected and written to loki")
		time.Sleep(120 * time.Second)

		g.By("Provision new user and give admin permission")
		users, usersHTpassFile, htPassSecret := getNewUser(oc, 1)
		defer userCleanup(oc, users, usersHTpassFile, htPassSecret)
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", users[0].Username).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		origContxt, contxtErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		e2e.Logf("orginal context is %v", origContxt)
		origWho, whoErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("").Output()
		o.Expect(whoErr).NotTo(o.HaveOccurred())
		e2e.Logf("original whoami is %v", origWho)
		defer func() {
			useContxtErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", origContxt).Execute()
			o.Expect(useContxtErr).NotTo(o.HaveOccurred())
			origContxt, contxtErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("current-context").Output()
			o.Expect(contxtErr).NotTo(o.HaveOccurred())
			e2e.Logf("defer context is %v", origContxt)
			origWho, whoErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("").Output()
			o.Expect(whoErr).NotTo(o.HaveOccurred())
			e2e.Logf("defer whoami is %v", origWho)
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("login").Args("-u", users[0].Username, "-p", users[0].Password).NotShowInfo().Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		currentContext, contxtErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("current-context").Output()
		o.Expect(contxtErr).NotTo(o.HaveOccurred())
		e2e.Logf("testuser context is %v", currentContext)
		currentWho, whoErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("").Output()
		o.Expect(whoErr).NotTo(o.HaveOccurred())
		e2e.Logf("testuser whoami is %v", currentWho)

		token, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check logs in loki")
		route := "https://" + getRouteAddress(oc, ls.Namespace, ls.Name)
		lc := newLokiClient(route).withToken(token).retry(5)
		lokiQuery := fmt.Sprintf("{app=\"netobserv-flowcollector\", DstK8S_Namespace=\"%s\", SrcK8S_Namespace=\"%s\", FlowDirection=\"0\"}", testTemplate.ClientNS, testTemplate.ServerNS)
		tenantID := "network"

		err = wait.Poll(30*time.Second, 300*time.Second, func() (done bool, err error) {
			res, err := lc.searchLogsInLoki(tenantID, lokiQuery)
			if err != nil {
				e2e.Logf("\ngot err %v when getting %s logs for query: %s\n", err, tenantID, lokiQuery)
				return false, err
			}
			if len(res.Data.Result) > 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", tenantID))
		res, err := lc.searchLogsInLoki("network", lokiQuery)
		o.Expect(err).NotTo(o.HaveOccurred())
		flowRecords := []FlowRecord{}

		for _, result := range res.Data.Result {
			if result.Stream.DstK8S_Namespace == testTemplate.ClientNS && result.Stream.SrcK8S_Namespace == testTemplate.ServerNS && result.Stream.SrcK8S_OwnerName == "nginx-service" {
				flowRecords, err = getFlowRecords(result.Values)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}
		var multiplier int = 0
		switch unit := testTemplate.ObjectSize[len(testTemplate.ObjectSize)-1:]; unit {
		case "K":
			multiplier = 1024
		case "M":
			multiplier = 1024 * 1024
		case "G":
			multiplier = 1024 * 1024 * 1024
		default:
			panic("invalid object size unit")
		}
		nObject, _ := strconv.Atoi(testTemplate.ObjectSize[0 : len(testTemplate.ObjectSize)-1])
		// minBytes is the size of the object fetched
		minBytes := nObject * multiplier
		// maxBytes is the minBytes +2% tolerance
		maxBytes := int(float64(minBytes) + (float64(minBytes) * 0.02))
		var errFlows float64 = 0
		nflows := float64(len(flowRecords))

		for _, r := range flowRecords {
			// occurs very rarely but sometimes >= comparison can be flaky
			// when eBPF-agent evicts packets sooner,
			// currently it configured to be 15seconds.
			if r.Flowlog.Bytes <= minBytes {
				errFlows += 1
			}
			if r.Flowlog.Bytes >= maxBytes {
				errFlows += 1
			}
			r.Flowlog.verifyFlowRecord()
		}
		// allow only 10% of flows to have Bytes violating minBytes and maxBytes.
		tolerance := math.Ceil(nflows * 0.10)
		o.Expect(errFlows).Should(o.BeNumerically("<=", tolerance))
	})

	g.It("NonPreRelease-Author:aramesha-High-57397-Verify network-flows export with Kafka [Serial]", func() {
		namespace := oc.Namespace()
		g.By("Subscribe to AMQ operator")
		catsrc := CatalogSourceObjects{"stable", "redhat-operators", "openshift-marketplace"}
		amq := SubscriptionObjects{
			OperatorName:  "amq-streams-cluster-operator",
			Namespace:     namespace,
			PackageName:   "amq-streams",
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "singlenamespace-og.yaml"),
			CatalogSource: catsrc,
		}

		defer amq.uninstallOperator(oc)
		amq.SubscribeOperator(oc)
		// before creating kafka, check the existence of crd kafkas.kafka.strimzi.io
		checkResource(oc, true, true, "kafka.strimzi.io", []string{"crd", "kafkas.kafka.strimzi.io", "-ojsonpath={.spec.group}"})

		g.By("Deploy KAFKA")
		// Kafka metrics config Template path
		kafkaMetricsPath := filePath.Join(kafkaDir, "kafka-metrics-config.yaml")

		kafkaMetrics := KafkaMetrics{
			Namespace: namespace,
			Template:  kafkaMetricsPath,
		}

		// Kafka Template path
		kafkaPath := filePath.Join(kafkaDir, "kafka-default.yaml")

		kafka := Kafka{
			Name:         "kafka-cluster",
			Namespace:    namespace,
			Template:     kafkaPath,
			StorageClass: ls.StorageClass,
		}

		// Kafka Topic path
		kafkaTopicPath := filePath.Join(kafkaDir, "kafka-topic.yaml")

		// deploy kafka topic
		kafkaTopic1 := KafkaTopic{
			TopicName: "network-flows",
			Name:      kafka.Name,
			Namespace: namespace,
			Template:  kafkaTopicPath,
		}

		// deploy kafka topic for export
		kafkaTopic2 := KafkaTopic{
			TopicName: "network-flows-export",
			Name:      kafka.Name,
			Namespace: namespace,
			Template:  kafkaTopicPath,
		}

		defer kafkaTopic1.deleteKafkaTopic(oc)
		defer kafkaTopic2.deleteKafkaTopic(oc)
		defer kafka.deleteKafka(oc)
		kafkaMetrics.deployKafkaMetrics(oc)
		kafka.deployKafka(oc)
		kafkaTopic1.deployKafkaTopic(oc)
		kafkaTopic1.deployKafkaTopic(oc)
		kafkaTopic2.deployKafkaTopic(oc)

		g.By("Check if Kafka and Kafka topic are ready")
		// Wait for Kafka and KafkaTopic to be ready
		waitForKafkaReady(oc, kafka.Name, kafka.Namespace)
		waitForKafkaTopicReady(oc, kafkaTopic1.TopicName, kafkaTopic1.Namespace)
		waitForKafkaTopicReady(oc, kafkaTopic2.TopicName, kafkaTopic2.Namespace)

		g.By("Deploy FlowCollector")
		// loki URL
		lokiURL := fmt.Sprintf("https://%s-gateway-http.%s.svc.cluster.local:8080/api/logs/v1/network/", ls.Name, namespace)

		flow := Flowcollector{
			Namespace:           namespace,
			DeploymentModel:     "KAFKA",
			Template:            flowFixturePath,
			MetricServerTLSType: "AUTO",
			LokiURL:             lokiURL,
			LokiAuthToken:       "HOST",
			LokiTLSEnable:       true,
			LokiTLSCertName:     fmt.Sprintf("%s-gateway-ca-bundle", ls.Name),
			KafkaAddress:        fmt.Sprintf("kafka-cluster-kafka-bootstrap.%s", namespace),
		}

		defer flow.deleteFlowcollector(oc)
		flow.createFlowcollector(oc)

		// Patch flowcollector exporters value
		patchValue := fmt.Sprintf(`[{"kafka":{"address": "` + flow.KafkaAddress + `", "topic": "network-flows-export"},"type": "KAFKA"}]`)
		oc.AsAdmin().WithoutNamespace().Run("patch").Args("flowcollector", "cluster", "-p", `[{"op": "replace", "path": "/spec/exporters", "value": `+patchValue+`}]`, "--type=json").Output()

		g.By("Ensure flows are observed and all pods are running")
		exutil.AssertAllPodsToBeReady(oc, namespace)
		// ensure eBPF pods are ready
		exutil.AssertAllPodsToBeReady(oc, namespace+"-privileged")

		g.By("Verify loki and KAFKA logs")
		verifyTime(oc, namespace, ls.Name, "flowlogs-pipeline-transformer", ls.Namespace)

		consumerTemplate := filePath.Join(kafkaDir, "topic-consumer.yaml")
		consumer := resource{"job", kafkaTopic2.TopicName + "-consumer", namespace}
		defer consumer.clear(oc)
		err := consumer.applyFromTemplate(oc, "-n", consumer.namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.name, "NAMESPACE="+consumer.namespace, "KAFKA_TOPIC="+kafkaTopic2.TopicName, "CLUSTER_NAME="+kafka.Name)
		o.Expect(err).NotTo(o.HaveOccurred())

		waitForPodReadyWithLabel(oc, namespace, "job-name="+kafkaTopic2.TopicName+"-consumer")

		consumerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", "job-name=network-flows-export-consumer", "-o=jsonpath={.items[0].metadata.name}").Output()

		o.Expect(err).NotTo(o.HaveOccurred())
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, namespace, "", consumerPodName, `'{"AgentIP":'`)
		exutil.AssertWaitPollNoErr(err, "Did not get log for the pod with job-name=network-flows-export-consumer label")
		verifyFlowRecordFromLogs(podLogs)
	})
})
