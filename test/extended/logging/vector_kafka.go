package logging

import (
	"context"
	"encoding/json"
	"fmt"
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
	var oc = exutil.NewCLI("vector-kafka-namespace", exutil.KubeConfigPath())

	g.Context("Log Forward to Kafka via Vector as Collector", func() {
		g.BeforeEach(func() {
			CLO := SubscriptionObjects{
				OperatorName:  "cluster-logging-operator",
				Namespace:     "openshift-logging",
				PackageName:   "cluster-logging",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
			}
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-49369-Vector Forward logs to kafka topic via Mutual Chained certificates[Serial][Slow]", func() {
			cloNS := "openshift-logging"
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			kafka := kafka{
				namespace:      cloNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "plaintext-ssl",
				pipelineSecret: "kafka-vector",
				collectorType:  "vector",
				loggingNS:      cloNS,
			}
			g.By("Deploy zookeeper")
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9093/clo-topic"

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRET_NAME="+kafka.pipelineSecret, "URL="+kafkaEndpoint, "NAMESPACE="+clf.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-52420-Vector Forward logs to kafka using SASL plaintext[Serial][Slow]", func() {
			cloNS := "openshift-logging"
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy zookeeper")
			kafka := kafka{
				namespace:      cloNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "sasl-plaintext",
				pipelineSecret: "vector-kafka",
				collectorType:  "vector",
				loggingNS:      cloNS,
			}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "http://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9092/clo-topic"

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRET_NAME="+kafka.pipelineSecret, "URL="+kafkaEndpoint, "NAMESPACE="+clf.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-52496-Vector Forward logs to kafka using SASL SSL[Serial][Slow]", func() {
			cloNS := "openshift-logging"
			g.By("Create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy zookeeper")
			kafka := kafka{
				namespace:      cloNS,
				kafkasvcName:   "kafka",
				zoosvcName:     "zookeeper",
				authtype:       "sasl-ssl",
				pipelineSecret: "vector-kafka",
				collectorType:  "vector",
				loggingNS:      cloNS,
			}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9093/clo-topic"

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "SECRET_NAME="+kafka.pipelineSecret, "URL="+kafkaEndpoint, "NAMESPACE="+clf.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", kafka.namespace, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				appLogs, err := getDataFromKafkaByNamespace(oc, kafka.namespace, consumerPodPodName, appProj)
				if err != nil {
					return false, err
				}
				return len(appLogs) > 0, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-Medium-47036-Vector Forward logs to different AMQ Kafka topics[Serial][Slow]", func() {
			nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodes.Items[0].Status.NodeInfo.Architecture == "arm64" {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			g.By("create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
				CatalogSource: catsrc,
			}

			kafkaClusterName := "kafka-cluster"
			//defer amq.uninstallOperator(oc)
			amq.SubscribeOperator(oc)
			// before creating kafka, check the existence of crd kafkas.kafka.strimzi.io
			checkResource(oc, true, true, "kafka.strimzi.io", []string{"crd", "kafkas.kafka.strimzi.io", "-ojsonpath={.spec.group}"})
			kafka := resource{"kafka", kafkaClusterName, amqNS}
			kafkaTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka", "amqstreams", "kafka-cluster-no-auth.yaml")
			defer kafka.clear(oc)
			kafka.applyFromTemplate(oc, "-n", kafka.namespace, "-f", kafkaTemplate, "-p", "NAME="+kafka.name, "NAMESPACE="+kafka.namespace, "VERSION=3.2.3", "MESSAGE_VERSION=3.2.3")
			o.Expect(err).NotTo(o.HaveOccurred())
			// create topics
			topicNames := []string{"topic-logging-app", "topic-logging-infra", "topic-logging-audit"}
			for _, topicName := range topicNames {
				topicTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka", "amqstreams", "kafka-topic.yaml")
				topic := resource{"Kafkatopic", topicName, amqNS}
				defer topic.clear(oc)
				err = topic.applyFromTemplate(oc, "-n", topic.namespace, "-f", topicTemplate, "-p", "NAME="+topic.name, "CLUSTER_NAME="+kafka.name, "NAMESPACE="+topic.namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			// wait for kafka cluster to be ready
			waitForPodReadyWithLabel(oc, kafka.namespace, "app.kubernetes.io/instance="+kafka.name)
			//create consumer pod
			for _, topicName := range topicNames {
				consumerTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka", "amqstreams", "topic-consumer.yaml")
				consumer := resource{"job", topicName + "-consumer", amqNS}
				defer consumer.clear(oc)
				err = consumer.applyFromTemplate(oc, "-n", consumer.namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.name, "NAMESPACE="+consumer.namespace, "KAFKA_TOPIC="+topicName, "CLUSTER_NAME="+kafkaClusterName)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			for _, topicName := range topicNames {
				waitForPodReadyWithLabel(oc, amqNS, "job-name="+topicName+"-consumer")
			}

			g.By("forward logs to Kafkas")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka_multi_topics.yaml")
			clf := resource{"clusterlogforwarder", "instance", "openshift-logging"}
			defer clf.clear(oc)
			bootstrapSVC := kafkaClusterName + "-kafka-bootstrap." + amqNS + ".svc"
			//since the data in kafka will be write to comsumer pods' stdout, and to avoid collecting these logs, here only collect app logs from appProj
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "BOOTSTRAP_SVC="+bootstrapSVC, "APP_PROJECT="+appProj)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", "openshift-logging"}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("check data in kafka")
			//application logs
			consumerPods, err := oc.AdminKubeClient().CoreV1().Pods(amqNS).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=topic-logging-app-consumer"})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
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
		g.It("CPaasrunOnly-Author:qitang-Medium-48141-Vector Forward logs to different Kafka brokers.[Serial][Slow]", func() {
			nodes, err := oc.AdminKubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.io/os=linux"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodes.Items[0].Status.NodeInfo.Architecture == "arm64" {
				g.Skip("Current platform not supported/resources not available for this test!")
			}

			g.By("create log producer")
			appProj := oc.Namespace()
			jsonLogFile := exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
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
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
				CatalogSource: catsrc,
			}
			amq2 := SubscriptionObjects{
				OperatorName:  "amq-streams-cluster-operator",
				Namespace:     amqNs2,
				PackageName:   "amq-streams",
				Subscription:  exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml"),
				OperatorGroup: exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml"),
				CatalogSource: catsrc,
			}
			topicName := "topic-logging-app"
			kafkaClusterName := "kafka-cluster"
			for _, amq := range []SubscriptionObjects{amq1, amq2} {
				defer deleteNamespace(oc, amq.Namespace)
				//defer amq.uninstallOperator(oc)
				amq.SubscribeOperator(oc)
				// before creating kafka, check the existence of crd kafkas.kafka.strimzi.io
				checkResource(oc, true, true, "kafka.strimzi.io", []string{"crd", "kafkas.kafka.strimzi.io", "-ojsonpath={.spec.group}"})
				kafka := resource{"kafka", kafkaClusterName, amq.Namespace}
				kafkaTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka", "amqstreams", "kafka-cluster-no-auth.yaml")
				//defer kafka.clear(oc)
				kafka.applyFromTemplate(oc, "-n", kafka.namespace, "-f", kafkaTemplate, "-p", "NAME="+kafka.name, "NAMESPACE="+kafka.namespace, "VERSION=3.2.3", "MESSAGE_VERSION=3.2.3")
				o.Expect(err).NotTo(o.HaveOccurred())
				// create topics
				topicTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka", "amqstreams", "kafka-topic.yaml")
				topic := resource{"Kafkatopic", topicName, amq.Namespace}
				//defer topic.clear(oc)
				err = topic.applyFromTemplate(oc, "-n", topic.namespace, "-f", topicTemplate, "-p", "NAME="+topic.name, "CLUSTER_NAME="+kafka.name, "NAMESPACE="+topic.namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				// wait for kafka cluster to be ready
				waitForPodReadyWithLabel(oc, kafka.namespace, "app.kubernetes.io/instance="+kafka.name)
			}
			//deploy consumer pod
			for _, ns := range []string{amqNs1, amqNs2} {
				consumerTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka", "amqstreams", "topic-consumer.yaml")
				consumer := resource{"job", topicName + "-consumer", ns}
				//defer consumer.clear(oc)
				err = consumer.applyFromTemplate(oc, "-n", consumer.namespace, "-f", consumerTemplate, "-p", "NAME="+consumer.name, "NAMESPACE="+consumer.namespace, "KAFKA_TOPIC="+topicName, "CLUSTER_NAME="+kafkaClusterName)
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("forward logs to Kafkas")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka_multi_brokers.yaml")
			clf := resource{"clusterlogforwarder", "instance", "openshift-logging"}
			defer clf.clear(oc)
			brokers, _ := json.Marshal([]string{"tls://" + kafkaClusterName + "-kafka-bootstrap." + amqNs1 + ".svc:9092", "tls://" + kafkaClusterName + "-kafka-bootstrap." + amqNs2 + ".svc:9092"})
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "TOPIC="+topicName, "BROKERS="+string(brokers))
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", "openshift-logging"}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "COLLECTOR=vector")
			WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")

			g.By("check data in kafka")
			for _, consumer := range []resource{{"job", topicName + "-consumer", amqNs1}, {"job", topicName + "-consumer", amqNs2}} {
				waitForPodReadyWithLabel(oc, consumer.namespace, "job-name="+consumer.name)
				consumerPods, err := oc.AdminKubeClient().CoreV1().Pods(consumer.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=" + consumer.name})
				o.Expect(err).NotTo(o.HaveOccurred())
				err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
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
	})
})
