package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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
	var (
		oc             = exutil.NewCLI("vector-kafka", exutil.KubeConfigPath())
		loggingBaseDir string
	)

	g.Context("Log Forward to Kafka via Vector as Collector", func() {
		g.BeforeEach(func() {
			loggingBaseDir = exutil.FixturePath("testdata", "logging")
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     cloNS,
				PackageName:   "cluster-logging",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "allnamespace-og.yaml"),
			}
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-49369-Vector Forward logs to kafka topic via Mutual Chained certificates[Serial][Slow]", func() {
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
				pipelineSecret: "kafka-vector",
				collectorType:  "vector",
				loggingNS:      loggingNS,
			}
			g.By("Deploy zookeeper")
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9093/clo-topic"

			g.By("Create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                   "clf-49369",
				namespace:              loggingNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "clf-kafka-no-auth.yaml"),
				secretName:             kafka.pipelineSecret,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL="+kafkaEndpoint)

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-52420-Vector Forward logs to kafka using SASL plaintext[Serial][Slow]", func() {
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

			g.By("Create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                   "clf-52420",
				namespace:              loggingNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "clf-kafka-with-auth.yaml"),
				secretName:             kafka.pipelineSecret,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL="+kafkaEndpoint)

			// Remove tls configuration from CLF as it is not required for this case
			patch := `[{"op": "remove", "path": "/spec/outputs/0/tls"}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		g.It("CPaasrunOnly-WRS-Author:ikanse-Critical-52496-Vector Forward logs to kafka using SASL SSL[Serial][Slow]", func() {
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
				pipelineSecret: "vector-kafka",
				collectorType:  "vector",
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
				name:                   "clf-52496",
				namespace:              loggingNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "clf-kafka-with-auth.yaml"),
				secretName:             kafka.pipelineSecret,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL="+kafkaEndpoint, "TLS_SECRET_NAME="+clf.secretName)

			// Validate tuning configuration under vector.toml
			checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", `compression = "zstd"`, "[sinks.output_kafka_app.batch]", `max_bytes = 10000000`, `[sinks.output_kafka_app.buffer]`)

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-47036-Vector Forward logs to different AMQ Kafka topics[Slow]", func() {
			nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodes.Items[0].Status.NodeInfo.Architecture == "arm64" {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			g.By("create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("subscribe AMQ kafka")
			oc.SetupProject()
			amqNS := oc.Namespace()
			catsrc := CatalogSourceObjects{"stable", "redhat-operators", "openshift-marketplace"}
			amq := SubscriptionObjects{
				OperatorName:  "amq-streams-cluster-operator",
				Namespace:     amqNS,
				PackageName:   "amq-streams",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
				CatalogSource: catsrc,
			}

			kafkaClusterName := "kafka-cluster"
			//defer amq.uninstallOperator(oc)
			amq.SubscribeOperator(oc)
			if isFipsEnabled(oc) {
				//disable FIPS_MODE due to "java.io.IOException: getPBEAlgorithmParameters failed: PBEWithHmacSHA256AndAES_256 AlgorithmParameters not available"
				err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub/"+amq.PackageName, "-n", amq.Namespace, "-p", "{\"spec\": {\"config\": {\"env\": [{\"name\": \"FIPS_MODE\", \"value\": \"disabled\"}]}}}", "--type=merge").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			// before creating kafka, check the existence of crd kafkas.kafka.strimzi.io
			checkResource(oc, true, true, "kafka.strimzi.io", []string{"crd", "kafkas.kafka.strimzi.io", "-ojsonpath={.spec.group}"})
			kafka := resource{"kafka", kafkaClusterName, amqNS}
			kafkaTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "kafka-cluster-no-auth.yaml")
			defer kafka.clear(oc)
			kafka.applyFromTemplate(oc, "-n", kafka.namespace, "-f", kafkaTemplate, "-p", "NAME="+kafka.name, "NAMESPACE="+kafka.namespace, "VERSION=3.7.0", "MESSAGE_VERSION=3.7.0")
			o.Expect(err).NotTo(o.HaveOccurred())
			// create topics
			topicNames := []string{"topic-logging-app", "topic-logging-infra", "topic-logging-audit"}
			for _, topicName := range topicNames {
				topicTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "kafka-topic.yaml")
				topic := resource{"Kafkatopic", topicName, amqNS}
				defer topic.clear(oc)
				err = topic.applyFromTemplate(oc, "-n", topic.namespace, "-f", topicTemplate, "-p", "NAME="+topic.name, "CLUSTER_NAME="+kafka.name, "NAMESPACE="+topic.namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			// wait for kafka cluster to be ready
			waitForPodReadyWithLabel(oc, kafka.namespace, "app.kubernetes.io/instance="+kafka.name)
			//create consumer pod
			for _, topicName := range topicNames {
				consumerTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "topic-consumer.yaml")
				consumer := resource{"job", topicName + "-consumer", amqNS}
				defer consumer.clear(oc)
				err = consumer.applyFromTemplate(oc, "-n", consumer.namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.name, "NAMESPACE="+consumer.namespace, "KAFKA_TOPIC="+topicName, "CLUSTER_NAME="+kafkaClusterName)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			for _, topicName := range topicNames {
				waitForPodReadyWithLabel(oc, amqNS, "job-name="+topicName+"-consumer")
			}

			oc.SetupProject()
			clfNS := oc.Namespace()

			g.By("forward logs to Kafkas")
			clf := clusterlogforwarder{
				name:                      "clf-47036",
				namespace:                 clfNS,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "clf_kafka_multi_topics.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}
			defer clf.delete(oc)

			bootstrapSVC := kafkaClusterName + "-kafka-bootstrap." + amqNS + ".svc"
			//since the data in kafka will be write to comsumer pods' stdout, and to avoid collecting these logs, here only collect app logs from appProj
			clf.create(oc, "BOOTSTRAP_SVC="+bootstrapSVC, "APP_PROJECT="+appProj, "APP_TOPIC="+topicNames[0], "INFRA_TOPIC="+topicNames[1], "AUDIT_TOPIC="+topicNames[2])

			g.By("check data in kafka")
			//application logs
			consumerPods, err := oc.AdminKubeClient().CoreV1().Pods(amqNS).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=topic-logging-app-consumer"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				logs, err := getDataFromKafkaConsumerPod(oc, amqNS, consumerPods.Items[0].Name)
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
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can't find app logs in %s/topic-logging-app-consumer", amqNS))
			e2e.Logf("find app logs \n")

			//infrastructure logs
			infraConsumerPods, err := oc.AdminKubeClient().CoreV1().Pods(amqNS).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=topic-logging-infra-consumer"})
			o.Expect(err).NotTo(o.HaveOccurred())
			infraLogs, err := getDataFromKafkaConsumerPod(oc, amqNS, infraConsumerPods.Items[0].Name)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(infraLogs) > 0).Should(o.BeTrue())
			o.Expect(infraLogs[0].LogType == "infrastructure").Should(o.BeTrue())
			e2e.Logf("find infra logs \n")

			//audit logs
			auditConsumerPods, err := oc.AdminKubeClient().CoreV1().Pods(amqNS).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=topic-logging-audit-consumer"})
			o.Expect(err).NotTo(o.HaveOccurred())
			auditLogs, err := getDataFromKafkaConsumerPod(oc, amqNS, auditConsumerPods.Items[0].Name)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(auditLogs) > 0).Should(o.BeTrue())
			o.Expect(auditLogs[0].LogType == "audit").Should(o.BeTrue())
			e2e.Logf("find audit logs \n")
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-48141-Vector Forward logs to different Kafka brokers.[Slow]", func() {
			nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodes.Items[0].Status.NodeInfo.Architecture == "arm64" {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			g.By("create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")
			// avoid hitting https://issues.redhat.com/browse/LOG-3025, set replicas to 3
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile, "-p", "REPLICAS=3").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("subscribe AMQ kafka into 2 different namespaces")
			// to avoid collecting kafka logs, deploy kafka in project openshift-*
			amqNs1 := "openshift-amq-1-" + getRandomString()
			amqNs2 := "openshift-amq-2-" + getRandomString()
			catsrc := CatalogSourceObjects{"stable", "redhat-operators", "openshift-marketplace"}
			amq1 := SubscriptionObjects{
				OperatorName:  "amq-streams-cluster-operator",
				Namespace:     amqNs1,
				PackageName:   "amq-streams",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
				CatalogSource: catsrc,
			}
			amq2 := SubscriptionObjects{
				OperatorName:  "amq-streams-cluster-operator",
				Namespace:     amqNs2,
				PackageName:   "amq-streams",
				Subscription:  filepath.Join(loggingBaseDir, "subscription", "sub-template.yaml"),
				OperatorGroup: filepath.Join(loggingBaseDir, "subscription", "singlenamespace-og.yaml"),
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
				defer kafka.clear(oc)
				kafka.applyFromTemplate(oc, "-n", kafka.namespace, "-f", kafkaTemplate, "-p", "NAME="+kafka.name, "NAMESPACE="+kafka.namespace, "VERSION=3.7.0", "MESSAGE_VERSION=3.7.0")
				o.Expect(err).NotTo(o.HaveOccurred())
				// create topics
				topicTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "kafka-topic.yaml")
				topic := resource{"Kafkatopic", topicName, amq.Namespace}
				defer topic.clear(oc)
				err = topic.applyFromTemplate(oc, "-n", topic.namespace, "-f", topicTemplate, "-p", "NAME="+topic.name, "CLUSTER_NAME="+kafka.name, "NAMESPACE="+topic.namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				// wait for kafka cluster to be ready
				waitForPodReadyWithLabel(oc, kafka.namespace, "app.kubernetes.io/instance="+kafka.name)
			}
			//deploy consumer pod
			for _, ns := range []string{amqNs1, amqNs2} {
				consumerTemplate := filepath.Join(loggingBaseDir, "external-log-stores", "kafka", "amqstreams", "topic-consumer.yaml")
				consumer := resource{"job", topicName + "-consumer", ns}
				defer consumer.clear(oc)
				err = consumer.applyFromTemplate(oc, "-n", consumer.namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.name, "NAMESPACE="+consumer.namespace, "KAFKA_TOPIC="+topicName, "CLUSTER_NAME="+kafkaClusterName)
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("forward logs to Kafkas")
			clf := clusterlogforwarder{
				name:                      "clf-48141",
				namespace:                 loggingNS,
				templateFile:              filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "kafka-multi-brokers.yaml"),
				waitForPodReady:           true,
				collectApplicationLogs:    true,
				collectAuditLogs:          true,
				collectInfrastructureLogs: true,
				serviceAccountName:        "clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			brokers, _ := json.Marshal([]string{"tls://" + kafkaClusterName + "-kafka-bootstrap." + amqNs1 + ".svc:9092", "tls://" + kafkaClusterName + "-kafka-bootstrap." + amqNs2 + ".svc:9092"})
			clf.create(oc, "TOPIC="+topicName, "BROKERS="+string(brokers))

			g.By("check data in kafka")
			for _, consumer := range []resource{{"job", topicName + "-consumer", amqNs1}, {"job", topicName + "-consumer", amqNs2}} {
				waitForPodReadyWithLabel(oc, consumer.namespace, "job-name="+consumer.name)
				consumerPods, err := oc.AdminKubeClient().CoreV1().Pods(consumer.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=" + consumer.name})
				o.Expect(err).NotTo(o.HaveOccurred())
				err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
					logs, err := getDataFromKafkaConsumerPod(oc, consumer.namespace, consumerPods.Items[0].Name)
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

		g.It("CPaasrunOnly-Author:ikanse-High-61549-Collector-External Kafka output complies with the tlsSecurityProfile configuration.[Slow][Disruptive]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := filepath.Join(loggingBaseDir, "generatelog", "container_json_log_template.json")

			g.By("Deploy the log generator app")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

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
			patch := `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value": {"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256"],"minTLSVersion":"VersionTLS12"},"type":"Custom"}}]`
			er = oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", patch).Execute()
			o.Expect(er).NotTo(o.HaveOccurred())

			g.By("Make sure that all the Cluster Operators are in healthy state before progressing.")
			waitForOperatorsRunning(oc)

			g.By("Deploy zookeeper")
			kafka := kafka{
				namespace:      loggingNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "sasl-ssl",
				pipelineSecret: "vector-kafka",
				collectorType:  "vector",
				loggingNS:      loggingNS,
			}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9093/clo-topic"

			g.By("Create clusterlogforwarder")
			clf := clusterlogforwarder{
				name:                   "clf-61549",
				namespace:              loggingNS,
				templateFile:           filepath.Join(loggingBaseDir, "observability.openshift.io_clusterlogforwarder", "clf-kafka-with-auth.yaml"),
				secretName:             kafka.pipelineSecret,
				waitForPodReady:        true,
				collectApplicationLogs: true,
				serviceAccountName:     "test-clf-" + getRandomString(),
			}
			defer clf.delete(oc)
			clf.create(oc, "URL="+kafkaEndpoint, "TLS_SECRET_NAME="+clf.secretName)

			g.By("The Kafka sink in Vector config must use the Custom tlsSecurityProfile")
			searchString := `[sinks.output_kafka_app.tls]
enabled = true
min_tls_version = "VersionTLS12"
ciphersuites = "ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES128-GCM-SHA256"
key_file = "/var/run/ocp-collector/secrets/vector-kafka/tls.key"
crt_file = "/var/run/ocp-collector/secrets/vector-kafka/tls.crt"
ca_file = "/var/run/ocp-collector/secrets/vector-kafka/ca-bundle.crt"`
			result, err := checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))

			g.By("Set Old tlsSecurityProfile for the External Kafka output.")
			patch = `[{"op": "add", "path": "/spec/outputs/0/tls/securityProfile", "value": {"type": "Old"}}]`
			clf.update(oc, "", patch, "--type=json")
			WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)

			g.By("Deploy the log generator app")
			oc.SetupProject()
			appProj1 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", appProj1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("The Kafka sink in Vector config must use the Old tlsSecurityProfile")
			searchString = `[sinks.output_kafka_app.tls]
enabled = true
min_tls_version = "VersionTLS10"
ciphersuites = "TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305_SHA256,ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-RSA-AES128-GCM-SHA256,ECDHE-ECDSA-AES256-GCM-SHA384,ECDHE-RSA-AES256-GCM-SHA384,ECDHE-ECDSA-CHACHA20-POLY1305,ECDHE-RSA-CHACHA20-POLY1305,DHE-RSA-AES128-GCM-SHA256,DHE-RSA-AES256-GCM-SHA384,DHE-RSA-CHACHA20-POLY1305,ECDHE-ECDSA-AES128-SHA256,ECDHE-RSA-AES128-SHA256,ECDHE-ECDSA-AES128-SHA,ECDHE-RSA-AES128-SHA,ECDHE-ECDSA-AES256-SHA384,ECDHE-RSA-AES256-SHA384,ECDHE-ECDSA-AES256-SHA,ECDHE-RSA-AES256-SHA,DHE-RSA-AES128-SHA256,DHE-RSA-AES256-SHA256,AES128-GCM-SHA256,AES256-GCM-SHA384,AES128-SHA256,AES256-SHA256,AES128-SHA,AES256-SHA,DES-CBC3-SHA"
key_file = "/var/run/ocp-collector/secrets/vector-kafka/tls.key"
crt_file = "/var/run/ocp-collector/secrets/vector-kafka/tls.crt"
ca_file = "/var/run/ocp-collector/secrets/vector-kafka/ca-bundle.crt"`
			result, err = checkCollectorConfiguration(oc, clf.namespace, clf.name+"-config", searchString)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.BeTrue(), "the configuration %s is not in vector.toml", searchString)

			g.By("Check for errors in collector pod logs.")
			e2e.Logf("Wait for a minute before the collector logs are generated.")
			time.Sleep(60 * time.Second)
			collectorLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", clf.namespace, "--selector=component=collector").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(collectorLogs, "Error trying to connect")).ShouldNot(o.BeTrue(), "Unable to connect to the external Kafka server.")

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj1)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})
	})
})
