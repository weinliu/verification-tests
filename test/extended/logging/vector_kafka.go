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

	var (
		oc             = exutil.NewCLI("vector-kafka-namespace", exutil.KubeConfigPath())
		clo            = "cluster-logging-operator"
		cloPackageName = "cluster-logging"
	)

	g.Context("Log Forward to Kafka via Vector as Collector", func() {
		var (
			subTemplate       = exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
			SingleNamespaceOG = exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml")
			jsonLogFile       = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
		)
		cloNS := "openshift-logging"
		CLO := SubscriptionObjects{clo, cloNS, SingleNamespaceOG, subTemplate, cloPackageName, CatalogSourceObjects{}}
		g.BeforeEach(func() {
			g.By("deploy CLO")
			CLO.SubscribeOperator(oc)
			oc.SetupProject()
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-49369-Vector Forward logs to kafka topic via Mutual Chained certificates[Serial][Slow]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			kafka := kafka{cloNS, "kafka", "zookeeper", "plaintext-ssl"}
			g.By("Deploy zookeeper")
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", cloNS, "-p", "SECRET_NAME=kafka-fluentd", "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", cloNS, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				consumerPodLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(consumerPodPodName, "-n", cloNS).Output()
				if err != nil {
					return false, err
				}
				if strings.Contains(consumerPodLogs, appProj) {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", cloNS, consumerPodPodName))
		})

		g.It("CPaasrunOnly-Author:ikanse-Medium-52420-Vector Forward logs to kafka using SASL plaintext[Serial][Slow]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy zookeeper")
			kafka := kafka{cloNS, "kafka", "zookeeper", "sasl-plaintext"}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", cloNS, "-p", "URL=tls://kafka."+cloNS+".svc.cluster.local:9092/clo-topic", "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", cloNS, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				consumerPodLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(consumerPodPodName, "-n", cloNS).Output()
				if err != nil {
					return false, err
				}
				if strings.Contains(consumerPodLogs, appProj) {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", cloNS, consumerPodPodName))
		})

		g.It("CPaasrunOnly-Author:ikanse-Critical-52496-Vector Forward logs to kafka using SASL SSL[Serial][Slow]", func() {
			g.By("Create log producer")
			appProj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", appProj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy zookeeper")
			kafka := kafka{cloNS, "kafka", "zookeeper", "sasl-ssl"}
			defer kafka.removeZookeeper(oc)
			kafka.deployZookeeper(oc)
			g.By("Deploy kafka")
			defer kafka.removeKafka(oc)
			kafka.deployKafka(oc)

			g.By("Create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "clf_kafka.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", cloNS, "-f", clfTemplate)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Deploy collector pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "collector_only.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "COLLECTOR=vector", "-p", "NAMESPACE="+cl.namespace)
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")

			g.By("Check app logs in kafka consumer pod")
			consumerPodPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", cloNS, "-l", "component=kafka-consumer", "-o", "name").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.Poll(10*time.Second, 180*time.Second, func() (done bool, err error) {
				consumerPodLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(consumerPodPodName, "-n", cloNS).Output()
				if err != nil {
					return false, err
				}
				if strings.Contains(consumerPodLogs, appProj) {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("App logs are not found in %s/%s", cloNS, consumerPodPodName))
		})
	})
})
