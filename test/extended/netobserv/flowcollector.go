package netobserv

import (
	"fmt"
	filePath "path/filepath"
	"strconv"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Flowcollector struct to handle Flowcollector resources
type Flowcollector struct {
	Namespace                 string
	ProcessorKind             string
	LogType                   string
	DeploymentModel           string
	LokiURL                   string
	LokiAuthToken             string
	LokiTLSEnable             bool
	LokiTLSCertName           string
	LokiStatusTLSEnable       bool
	LokiStatusURL             string
	LokiStatusTLSCertName     string
	LokiStatusTLSUserCertName string
	KafkaAddress              string
	KafkaTLSEnable            bool
	KafkaClusterName          string
	KafkaTopic                string
	KafkaUser                 string
	MetricServerTLSType       string
	EbpfCacheActiveTimeout    string
	Template                  string
}

// Metrics struct to handle Metrics resources
type Metrics struct {
	Namespace string
	Template  string
	Scheme    string
}

// MonitoringConfig struct to handle MonitoringConfig resources
type MonitoringConfig struct {
	Name               string
	Namespace          string
	EnableUserWorkload bool
	Template           string
}

// ForwardClusterRoleBinding struct to handle ClusterRoleBinding in Forward mode
type ForwardClusterRoleBinding struct {
	Name               string
	Namespace          string
	ServiceAccountName string
	Template           string
}

type Flowlog struct {
	Packets          int
	SrcPort          int
	DstMac           string
	TimeReceived     int
	IcmpType         int
	DstK8S_Name      string
	DstPort          int
	DstK8S_HostIP    string
	Bytes            int
	SrcK8S_Type      string
	DstK8S_HostName  string
	Proto            int
	DstAddr          string
	Interface        string
	SrcAddr          string
	TimeFlowEndMs    int
	DstK8S_OwnerType string
	Flags            int
	Etype            int
	DstK8S_Type      string
	IfDirection      int
	SrcMac           string
	SrcK8S_OwnerType string
	SrcK8S_Name      string
	Duplicate        bool
	TimeFlowStartMs  int
	AgentIP          string
	IcmpCode         int
}

type FlowRecord struct {
	Timestamp int64
	Flowlog   Flowlog
}

// create flowcollector CRD for a given manifest file
func (flow *Flowcollector) createFlowcollector(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", flow.Template, "-p", "NAMESPACE=" + flow.Namespace}

	flpSA := "flowlogs-pipeline"
	if flow.DeploymentModel == "KAFKA" {
		parameters = append(parameters, "DEPLOYMENT_MODEL="+flow.DeploymentModel)
		flpSA = "flowlogs-pipeline-transformer"
	}

	if flow.ProcessorKind != "" {
		parameters = append(parameters, "KIND="+flow.ProcessorKind)
	}

	if flow.MetricServerTLSType != "" {
		parameters = append(parameters, "METRIC_SERVER_TLS_TYPE="+flow.MetricServerTLSType)
	}

	if flow.LokiURL != "" {
		parameters = append(parameters, "LOKI_URL="+flow.LokiURL)
	}

	if strconv.FormatBool(flow.LokiTLSEnable) != "" {
		parameters = append(parameters, "LOKI_TLS_ENABLE="+strconv.FormatBool(flow.LokiTLSEnable))
	}

	if flow.LokiTLSCertName != "" {
		parameters = append(parameters, "LOKI_TLS_CERT_NAME="+flow.LokiTLSCertName)
	}

	if flow.LokiStatusURL != "" {
		parameters = append(parameters, "LOKI_STATUS_URL="+flow.LokiStatusURL)
	}

	if strconv.FormatBool(flow.LokiStatusTLSEnable) != "" {
		parameters = append(parameters, "LOKI_STATUS_TLS_ENABLE="+strconv.FormatBool(flow.LokiStatusTLSEnable))
	}

	if flow.LokiStatusTLSCertName != "" {
		parameters = append(parameters, "LOKI_STATUS_TLS_USER_CERT_NAME="+flow.LokiStatusTLSCertName)
	}

	if flow.LokiStatusTLSUserCertName != "" {
		parameters = append(parameters, "LOKI_STATUS_TLS_USER_CERT_NAME="+flow.LokiStatusTLSUserCertName)
	}

	if flow.LogType != "" {
		parameters = append(parameters, "LOG_TYPE="+flow.LogType)
	}

	if flow.KafkaAddress != "" {
		parameters = append(parameters, "KAFKA_ADDRESS="+flow.KafkaAddress)
	}

	if strconv.FormatBool(flow.KafkaTLSEnable) != "" {
		parameters = append(parameters, "KAFKA_TLS_ENABLE="+strconv.FormatBool(flow.KafkaTLSEnable))
	}

	if flow.KafkaClusterName != "" {
		parameters = append(parameters, "KAFKA_CLUSTER_NAME="+flow.KafkaClusterName)
	}

	if flow.KafkaTopic != "" {
		parameters = append(parameters, "KAFKA_TOPIC="+flow.KafkaTopic)
	}

	if flow.KafkaUser != "" {
		parameters = append(parameters, "KAFKA_USER="+flow.KafkaUser)
	}

	if flow.EbpfCacheActiveTimeout != "" {
		parameters = append(parameters, "EBPF_CACHEACTIVETIMEOUT="+flow.EbpfCacheActiveTimeout)
	}

	exutil.ApplyNsResourceFromTemplate(oc, flow.Namespace, parameters...)

	// deploy Forward CRB
	baseDir := exutil.FixturePath("testdata", "netobserv")
	forwardCRBPath := filePath.Join(baseDir, "clusterRoleBinding-FORWARD.yaml")
	forwardCRB := ForwardClusterRoleBinding{
		Namespace:          oc.Namespace(),
		Template:           forwardCRBPath,
		ServiceAccountName: flpSA,
	}
	forwardCRB.deployForwardCRB(oc)
}

// delete flowcollector CRD from a cluster
func (flow *Flowcollector) deleteFlowcollector(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("flowcollector", "cluster").Execute()
}

// create metrics for a given manifest file
func (metric *Metrics) createMetrics(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", metric.Template, "-p", "NAMESPACE=" + metric.Namespace}

	if metric.Scheme != "" {
		parameters = append(parameters, "PROTOCOL="+metric.Scheme)
	}

	exutil.ApplyNsResourceFromTemplate(oc, metric.Namespace, parameters...)
}

// create configMap
func (cm *MonitoringConfig) createConfigMap(oc *exutil.CLI) {
	e2e.Logf("Create configmap: cluster-monitoring-config")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", cm.Template, "-p", "ENABLEUSERWORKLOAD=" + fmt.Sprintf("%v", cm.EnableUserWorkload)}

	if cm.Name != "" {
		parameters = append(parameters, "NAME="+cm.Name)
	}

	exutil.ApplyNsResourceFromTemplate(oc, cm.Namespace, parameters...)
}

// delete configMap
func (cm *MonitoringConfig) deleteConfigMap(oc *exutil.CLI) error {
	e2e.Logf("Delete configmap: cluster-monitoring-config")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "cluster-monitoring-config", "-n", "openshift-monitoring").Execute()
}

// deploy ForwardClusterRoleBinding
func (crb *ForwardClusterRoleBinding) deployForwardCRB(oc *exutil.CLI) {
	e2e.Logf("Deploy ClusterRoleBinding in Forward mode")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", crb.Template, "-p", "NAMESPACE=" + crb.Namespace}

	if crb.Name != "" {
		parameters = append(parameters, "NAME="+crb.Name)
	}

	if crb.ServiceAccountName != "" {
		parameters = append(parameters, "SERVICE_ACCOUNT_NAME="+crb.ServiceAccountName)
	}

	exutil.ApplyNsResourceFromTemplate(oc, crb.Namespace, parameters...)
}
