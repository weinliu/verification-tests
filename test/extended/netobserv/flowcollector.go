package netobserv

import (
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
	LokiEnable                bool
	LokiURL                   string
	LokiAuthToken             string
	LokiTLSEnable             bool
	LokiTLSCertName           string
	LokiStatusTLSEnable       bool
	LokiStatusURL             string
	LokiStatusTLSCertName     string
	LokiStatusTLSUserCertName string
	LokiNamespace             string
	KafkaAddress              string
	KafkaTLSEnable            bool
	KafkaClusterName          string
	KafkaTopic                string
	KafkaUser                 string
	KafkaNamespace            string
	MetricServerTLSType       string
	EbpfCacheActiveTimeout    string
	EbpfPrivileged            bool
	PacketDropEnable          bool
	DNSTrackingEnable         bool
	PluginEnable              bool
	Template                  string
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

	if !flow.LokiEnable {
		parameters = append(parameters, "LOKI_ENABLE="+strconv.FormatBool(flow.LokiEnable))
	}

	if flow.LokiEnable && flow.LokiURL != "" {
		parameters = append(parameters, "LOKI_URL="+flow.LokiURL)
	}

	if flow.LokiEnable && flow.LokiTLSEnable {
		parameters = append(parameters, "LOKI_TLS_ENABLE="+strconv.FormatBool(flow.LokiTLSEnable))
	}

	if flow.LokiEnable && flow.LokiTLSCertName != "" {
		parameters = append(parameters, "LOKI_TLS_CERT_NAME="+flow.LokiTLSCertName)
	}

	if flow.LokiEnable && flow.LokiStatusURL != "" {
		parameters = append(parameters, "LOKI_STATUS_URL="+flow.LokiStatusURL)
	}

	if flow.LokiEnable && flow.LokiStatusTLSEnable {
		parameters = append(parameters, "LOKI_STATUS_TLS_ENABLE="+strconv.FormatBool(flow.LokiStatusTLSEnable))
	}

	if flow.LokiEnable && flow.LokiStatusTLSCertName != "" {
		parameters = append(parameters, "LOKI_STATUS_TLS_USER_CERT_NAME="+flow.LokiStatusTLSCertName)
	}

	if flow.LokiEnable && flow.LokiStatusTLSUserCertName != "" {
		parameters = append(parameters, "LOKI_STATUS_TLS_USER_CERT_NAME="+flow.LokiStatusTLSUserCertName)
	}

	if flow.LokiEnable && flow.LokiNamespace != "" {
		parameters = append(parameters, "LOKI_NAMESPACE="+flow.LokiNamespace)
	}

	if flow.LogType != "" {
		parameters = append(parameters, "LOG_TYPE="+flow.LogType)
	}

	if flow.KafkaAddress != "" {
		parameters = append(parameters, "KAFKA_ADDRESS="+flow.KafkaAddress)
	}

	if flow.KafkaTLSEnable {
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

	if flow.KafkaNamespace != "" {
		parameters = append(parameters, "KAFKA_NAMESPACE="+flow.KafkaNamespace)
	}

	if flow.EbpfCacheActiveTimeout != "" {
		parameters = append(parameters, "EBPF_CACHEACTIVETIMEOUT="+flow.EbpfCacheActiveTimeout)
	}

	if !flow.PluginEnable {
		parameters = append(parameters, "PLUGIN_ENABLE="+strconv.FormatBool(flow.PluginEnable))
	}

	if flow.EbpfPrivileged {
		parameters = append(parameters, "EBPF_PRIVILEGED="+strconv.FormatBool(flow.EbpfPrivileged))
	}

	if flow.PacketDropEnable {
		parameters = append(parameters, "PACKET_DROP_ENABLE="+strconv.FormatBool(flow.PacketDropEnable))
	}

	if flow.DNSTrackingEnable {
		parameters = append(parameters, "DNS_TRACKING_ENABLE="+strconv.FormatBool(flow.DNSTrackingEnable))
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
	if flow.LokiEnable && flow.PluginEnable {
		forwardCRB.deployForwardCRB(oc)
	}
}

// delete flowcollector CRD from a cluster
func (flow *Flowcollector) deleteFlowcollector(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("flowcollector", "cluster").Execute()
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
