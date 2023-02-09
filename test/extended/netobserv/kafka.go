package netobserv

import (
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Kafka struct to handle default Kafka installation
type Kafka struct {
	Name         string
	Namespace    string
	Template     string
	StorageClass string
}

// KafkaMetrics struct to handle kafka metrics config deployment
type KafkaMetrics struct {
	Namespace string
	Template  string
}

// KafkaTopic struct handles creation of kafka topic
type KafkaTopic struct {
	Namespace string
	TopicName string
	Name      string
	Template  string
}

// deploys default Kafka
func (kafka *Kafka) deployKafka(oc *exutil.CLI) {
	e2e.Logf("Deploy Default KAFKA")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", kafka.Template, "-p", "NAMESPACE=" + kafka.Namespace}

	if kafka.Name != "" {
		parameters = append(parameters, "NAME="+kafka.Name)
	}

	if kafka.StorageClass != "" {
		parameters = append(parameters, "STORAGE_CLASS="+kafka.StorageClass)
	}

	exutil.ApplyNsResourceFromTemplate(oc, kafka.Namespace, parameters...)
}

// deploys Kafka Metrics
func (kafkaMetrics *KafkaMetrics) deployKafkaMetrics(oc *exutil.CLI) {
	e2e.Logf("Deploy KAFKA metrics")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", kafkaMetrics.Template, "-p", "NAMESPACE=" + kafkaMetrics.Namespace}

	exutil.ApplyNsResourceFromTemplate(oc, kafkaMetrics.Namespace, parameters...)
}

// creates a Kafka topic
func (kafkaTopic *KafkaTopic) deployKafkaTopic(oc *exutil.CLI) {
	e2e.Logf("Create KAFKA topic")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", kafkaTopic.Template, "-p", "NAMESPACE=" + kafkaTopic.Namespace}

	if kafkaTopic.Name != "" {
		parameters = append(parameters, "NAME="+kafkaTopic.Name)
	}

	if kafkaTopic.TopicName != "" {
		parameters = append(parameters, "TOPIC="+kafkaTopic.TopicName)
	}

	exutil.ApplyNsResourceFromTemplate(oc, kafkaTopic.Namespace, parameters...)
}

// deletes kafkaTopic
func (kafkaTopic *KafkaTopic) deleteKafkaTopic(oc *exutil.CLI) {
	e2e.Logf("Deleting KAFKA topic")
	command := []string{"kafkatopic", kafkaTopic.TopicName, "-n", kafkaTopic.Namespace}
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(command...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// deletes kafka
func (kafka *Kafka) deleteKafka(oc *exutil.CLI) {
	e2e.Logf("Deleting KAFKA")
	command2 := []string{"kafka", kafka.Name, "-n", kafka.Namespace}
	_, err2 := oc.AsAdmin().WithoutNamespace().Run("delete").Args(command2...).Output()
	o.Expect(err2).NotTo(o.HaveOccurred())
}

// Poll to wait for kafka to be ready
func waitForKafkaReady(oc *exutil.CLI, kafkaName string, kafkaNS string) {
	err := wait.Poll(3*time.Second, 180*time.Second, func() (done bool, err error) {
		command := []string{"kafka", kafkaName, "-n", kafkaNS, `-o=jsonpath={.status.conditions[*].type}`}
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(command...).Output()
		if err != nil {
			e2e.Logf("kafka status ready error: %v", err)
			return false, err
		}
		if output == "Ready" || output == "Warning Ready" {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource kafka/%s did not appear", kafkaName))
}

// Poll to wait for kafka Topic to be ready
func waitForKafkaTopicReady(oc *exutil.CLI, kafkaTopicName string, kafkaTopicNS string) {
	err := wait.Poll(3*time.Second, 180*time.Second, func() (done bool, err error) {
		command := []string{"kafkaTopic", kafkaTopicName, "-n", kafkaTopicNS, `-o=jsonpath='{.status.conditions[*].type}'`}
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(command...).Output()
		if err != nil {
			e2e.Logf("kafka Topic status ready error: %v", err)
			return false, err
		}
		status := strings.Replace(output, "'", "", 2)
		e2e.Logf("Waiting for kafka status %s", status)
		if status == "Ready" {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource kafkaTopic/%s did not appear", kafkaTopicName))
}
