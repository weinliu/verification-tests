package netobserv

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Flowcollector struct to handle Flowcollector resources
type Flowcollector struct {
	Namespace           string
	ProcessorKind       string
	Template            string
	MetricServerTLSType string
	LokiURL             string
	LokiAuthToken       string
	LokiTLSEnable       bool
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

// LokiPersistentVolumeClaim struct to handle Loki PVC resources
type LokiPersistentVolumeClaim struct {
	Namespace string
	Template  string
}

// LokiStorage struct to handle LokiStorage resources
type LokiStorage struct {
	Namespace string
	Template  string
}

// ForwardClusterRoleBinding struct to handle ClusterRoleBinding in Forward mode
type ForwardClusterRoleBinding struct {
	Namespace string
	Template  string
}

// HostClusterRoleBinding struct to handle ClusterRoleBinding in Host mode
type HostClusterRoleBinding struct {
	Namespace string
	Template  string
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

// deploy LokiPVC
func (loki *LokiPersistentVolumeClaim) deployLokiPVC(oc *exutil.CLI) {
	e2e.Logf("Deploy Loki PVC")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", loki.Template, "-p", "NAMESPACE=" + loki.Namespace}
	exutil.ApplyNsResourceFromTemplate(oc, loki.Namespace, parameters...)
}

// deploy LokiStorage
func (loki *LokiStorage) deployLokiStorage(oc *exutil.CLI) {
	e2e.Logf("Deploy Loki storage")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", loki.Template, "-p", "NAMESPACE=" + loki.Namespace}
	exutil.ApplyNsResourceFromTemplate(oc, loki.Namespace, parameters...)
}

// delete LokiStorage
func (loki *LokiStorage) deleteLokiStorage(oc *exutil.CLI) {
	e2e.Logf("Delete Loki PVC")
	command1 := []string{"delete", "pod", "loki", "-n", loki.Namespace}
	_, err1 := oc.AsAdmin().WithoutNamespace().Run(command1...).Args().Output()

	command2 := []string{"delete", "configmap", "loki-config", "-n", loki.Namespace}
	_, err2 := oc.AsAdmin().WithoutNamespace().Run(command2...).Args().Output()

	command3 := []string{"delete", "service", "loki", "-n", loki.Namespace}
	_, err3 := oc.AsAdmin().WithoutNamespace().Run(command3...).Args().Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	o.Expect(err2).NotTo(o.HaveOccurred())
	o.Expect(err3).NotTo(o.HaveOccurred())
}

// deploy ForwardClusterRoleBinding
func (crb *ForwardClusterRoleBinding) deployForwardCRB(oc *exutil.CLI) {
	e2e.Logf("Deploy ClusterRoleBinding in Forward mode")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", crb.Template, "-p", "NAMESPACE=" + crb.Namespace}
	exutil.ApplyNsResourceFromTemplate(oc, crb.Namespace, parameters...)
}

// deploy HostClusterRoleBinding
func (crb *HostClusterRoleBinding) deployHostCRB(oc *exutil.CLI) {
	e2e.Logf("Deploy ClusterRoleBinding in Host mode")
	parameters := []string{"--ignore-unknown-parameters=true", "-f", crb.Template, "-p", "NAMESPACE=" + crb.Namespace}
	exutil.ApplyNsResourceFromTemplate(oc, crb.Namespace, parameters...)
}

// get flow collector port
func getCollectorPort(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("flowcollector", "cluster", "-n", oc.Namespace()).Template("{{.spec.processor.port}}").Output()
}

// returns service IP or error for flowlogsPipeline deployment
func getFlowlogsPipelineServiceIP(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("svc", "flowlogs-pipeline", "-n", oc.Namespace()).Template("{{.spec.clusterIP}}").Output()
}

// returns true/false if flow collection is enabled on cluster
func checkFlowcollectionEnabled(oc *exutil.CLI) string {
	collectorName, err, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("flowcollector").Template("{{range .items}}{{.metadata.name}}{{end}}").Outputs()

	if err != "" {
		return ""
	}

	return collectorName

}

// polls to check ovs-flows-config is created or deleted given shouldExist is true or false
func waitCnoConfigMapUpdate(oc *exutil.CLI, shouldExist bool) {
	err := wait.Poll(20*time.Second, 10*time.Minute, func() (bool, error) {

		// check whether ovs-flows-config config map exists in openshift-network-operator NS
		_, stderr, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "ovs-flows-config", "-n", "openshift-network-operator").Outputs()

		if stderr == "" && shouldExist {
			return true, nil
		}

		if stderr != "" && !shouldExist {
			return true, nil
		}
		return false, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf(" ovs-flows-config ConfigMap is not updated"))
}

// returns target configured in ovs-flows-config config map
func getOVSFlowsConfigTarget(oc *exutil.CLI, flowlogsPipelineDeployedAs string) (string, error) {

	var template string
	if flowlogsPipelineDeployedAs == "Deployment" {
		template = "{{.data.sharedTarget}}"
	}

	if flowlogsPipelineDeployedAs == "DaemonSet" {
		template = "{{.data.nodePort}}"
	}

	stdout, stderr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "ovs-flows-config", "-n", "openshift-network-operator").Template(template).Outputs()

	if stderr != "" || err != nil {
		e2e.Logf("Fetching ovs-flows-config configmap return err %s", stderr)
		return stdout, err
	}
	return stdout, err
}

// returns target port configured in eBPF pods
func getEBPFlowsConfigPort(oc *exutil.CLI) ([]string, error) {
	jsonpath := "{.items[*].spec.containers[*].env[?(@.name==\"FLOWS_TARGET_PORT\")].value}"
	var ports []string
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace()+"-privileged", "-o", "jsonpath="+jsonpath).Output()
	if err != nil {
		return ports, err
	}
	ports = strings.Split(stdout, " ")
	return ports, nil
}

// get flow collector IPs configured in OVS
func getOVSCollectorIP(oc *exutil.CLI) ([]string, error) {
	jsonpath := "{.items[*].spec.containers[*].env[?(@.name==\"IPFIX_COLLECTORS\")].value}"

	var collectors []string
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-ovn-kubernetes", "-o", "jsonpath="+jsonpath).Output()

	if err != nil {
		return collectors, err
	}
	collectors = strings.Split(stdout, " ")

	return collectors, nil
}

// returns target IP configured in eBPF pods
func getEBPFCollectorIP(oc *exutil.CLI, flowlogsPipelineDeployedAs string) ([]string, error) {
	var collectors []string
	var jsonpath string
	if flowlogsPipelineDeployedAs == "Deployment" {
		jsonpath = "{.items[*].spec.containers[*].env[?(@.name==\"FLOWS_TARGET_HOST\")].value}"
	}

	if flowlogsPipelineDeployedAs == "DaemonSet" {
		jsonpath = "{.items[*].spec.containers[*].env[?(@.name==\"FLOWS_TARGET_HOST\")].valueFrom.fieldRef.fieldPath}"
	}

	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace()+"-privileged", "-o", "jsonpath="+jsonpath).Output()
	if err != nil {
		return collectors, err
	}
	collectors = strings.Split(stdout, " ")
	return collectors, nil
}

// returns ture/false if flowcollector API exists.
func isFlowCollectorAPIExists(oc *exutil.CLI) (bool, error) {
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "-o", "jsonpath='{.items[*].spec.names.kind}'").Output()

	if err != nil {
		return false, err
	}
	return strings.Contains(stdout, "FlowCollector"), nil
}
