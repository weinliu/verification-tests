package logging

import (
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
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
				consumerPodLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(consumerPodPodName, "-n", kafka.namespace).Output()
				if err != nil {
					return false, err
				}
				if strings.Contains(consumerPodLogs, appProj) {
					return true, nil
				}
				return false, nil
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
			kafkaEndpoint := "tls://" + kafka.kafkasvcName + "." + kafka.namespace + ".svc.cluster.local:9092/clo-topic"

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
				consumerPodLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(consumerPodPodName, "-n", kafka.namespace).Output()
				if err != nil {
					return false, err
				}
				if strings.Contains(consumerPodLogs, appProj) {
					return true, nil
				}
				return false, nil
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
				consumerPodLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(consumerPodPodName, "-n", kafka.namespace).Output()
				if err != nil {
					return false, err
				}
				if strings.Contains(consumerPodLogs, appProj) {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", kafka.namespace, consumerPodPodName))
		})
	})
})
