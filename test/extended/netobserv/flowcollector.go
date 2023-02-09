package netobserv

import (
	"fmt"
	"strconv"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Flowcollector struct to handle Flowcollector resources
type Flowcollector struct {
	Namespace           string
	ProcessorKind       string
	DeploymentModel     string
	Template            string
	MetricServerTLSType string
	LokiURL             string
	LokiAuthToken       string
	LokiTLSEnable       bool
	LokiCertName        string
	KafkaAddress        string
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

// HostClusterRoleBinding struct to handle ClusterRoleBinding in Host mode
type HostClusterRoleBinding struct {
	Name               string
	Namespace          string
	ServiceAccountName string
	Template           string
}

// create flowcollector CRD for a given manifest file
func (flow *Flowcollector) createFlowcollector(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", flow.Template, "-p", "NAMESPACE=" + flow.Namespace}

	if flow.ProcessorKind != "" {
		parameters = append(parameters, "KIND="+flow.ProcessorKind)
	}

	if flow.MetricServerTLSType != "" {
		parameters = append(parameters, "METRIC_SERVER_TLS_TYPE="+flow.MetricServerTLSType)
	}

	if flow.LokiURL != "" {
		parameters = append(parameters, "LOKI_URL="+flow.LokiURL)
	}

	if flow.LokiAuthToken != "" {
		parameters = append(parameters, "LOKI_AUTH_TOKEN="+flow.LokiAuthToken)
	}

	if strconv.FormatBool(flow.LokiTLSEnable) != "" {
		parameters = append(parameters, "LOKI_TLS_ENABLE="+strconv.FormatBool(flow.LokiTLSEnable))
	}

	if flow.LokiCertName != "" {
		parameters = append(parameters, "LOKI_CERT_NAME="+flow.LokiCertName)
	}

	if flow.DeploymentModel != "" {
		parameters = append(parameters, "DEPLOYMENT_MODEL="+flow.DeploymentModel)
	}

	if flow.KafkaAddress != "" {
		parameters = append(parameters, "KAFKA_ADDRESS="+flow.KafkaAddress)
	}
	exutil.ApplyNsResourceFromTemplate(oc, flow.Namespace, parameters...)
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

// deploy HostClusterRoleBinding
func (crb *HostClusterRoleBinding) deployHostCRB(oc *exutil.CLI) {
	e2e.Logf("Deploy ClusterRoleBinding in Host mode")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", crb.Template, "-p", "NAMESPACE=" + crb.Namespace}

	if crb.Name != "" {
		parameters = append(parameters, "NAME="+crb.Name)
	}

	if crb.ServiceAccountName != "" {
		parameters = append(parameters, "SERVICE_ACCOUNT_NAME="+crb.ServiceAccountName)
	}

	exutil.ApplyNsResourceFromTemplate(oc, crb.Namespace, parameters...)
}
