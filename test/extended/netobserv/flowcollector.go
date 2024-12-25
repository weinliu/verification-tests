package netobserv

import (
	"context"
	"fmt"
	"reflect"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Flowcollector struct to handle Flowcollector resources
type Flowcollector struct {
	Namespace                         string
	ProcessorKind                     string
	MultiClusterDeployment            string
	AddZone                           string
	LogType                           string
	DeploymentModel                   string
	LokiEnable                        string
	LokiMode                          string
	LokiURL                           string
	LokiTLSCertName                   string
	LokiStatusTLSEnable               string
	LokiStatusURL                     string
	LokiStatusTLSCertName             string
	LokiStatusTLSUserCertName         string
	LokiNamespace                     string
	MonolithicLokiURL                 string
	KafkaAddress                      string
	KafkaTLSEnable                    string
	KafkaClusterName                  string
	KafkaTopic                        string
	KafkaUser                         string
	KafkaNamespace                    string
	FLPMetricServerTLSType            string
	EBPFMetricServerTLSType           string
	EBPFCacheActiveTimeout            string
	EBPFPrivileged                    string
	EBPFMetrics                       string
	EBPFeatures                       []string
	CacheMaxFlows                     string
	PacketDropEnable                  string
	DNStrackingEnable                 string
	PluginEnable                      string
	NetworkPolicyEnable               string
	NetworkPolicyAdditionalNamespaces []string
	Exporters                         []string
	SecondayNetworks                  []string
	Template                          string
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
	Dscp             int
	SrcPort          int
	DstMac           string
	TimeReceived     int
	IcmpType         int
	DstK8S_Name      string
	DstPort          int
	DstK8S_HostIP    string
	Bytes            int
	SrcK8S_Type      string
	SrcK8S_HostName  string
	DstK8S_HostName  string
	Proto            int
	DstAddr          string
	IfDirections     []int
	Interfaces       []string
	SrcAddr          string
	TimeFlowEndMs    int
	DstK8S_OwnerType string
	Flags            []string
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
	HashId           string `json:"_HashId,omitempty"`
	IsFirst          bool   `json:"_IsFirst,omitempty"`
	RecordType       string `json:"_RecordType,omitempty"`
	NumFlowLogs      int    `json:"numFlowLogs,omitempty"`
	K8S_ClusterName  string `json:"K8S_ClusterName,omitempty"`
	SrcK8S_Zone      string `json:"SrcK8S_Zone,omitempty"`
	DstK8S_Zone      string `json:"DstK8S_Zone,omitempty"`
}

type FlowRecord struct {
	Timestamp int64
	Flowlog   Flowlog
}

type Lokilabels struct {
	App              string
	SrcK8S_Namespace string
	DstK8S_Namespace string
	RecordType       string
	FlowDirection    string
	SrcK8S_OwnerName string
	DstK8S_OwnerName string
	K8S_ClusterName  string
	SrcK8S_Type      string
	DstK8S_Type      string
	Interfaces       string
}

// create flowcollector CRD for a given manifest file
func (flow Flowcollector) CreateFlowcollector(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", flow.Template, "-p"}

	flowCollector := reflect.ValueOf(&flow).Elem()

	for i := 0; i < flowCollector.NumField(); i++ {
		if flowCollector.Field(i).Interface() != "" {
			if flowCollector.Type().Field(i).Name != "Template" {
				parameters = append(parameters, fmt.Sprintf("%s=%s", flowCollector.Type().Field(i).Name, flowCollector.Field(i).Interface()))
			}
		}
	}

	exutil.ApplyNsResourceFromTemplate(oc, flow.Namespace, parameters...)
	flow.WaitForFlowcollectorReady(oc)
}

// delete flowcollector CRD from a cluster
func (flow *Flowcollector) DeleteFlowcollector(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("flowcollector", "cluster").Execute()
}

func (flow *Flowcollector) WaitForFlowcollectorReady(oc *exutil.CLI) {
	// check FLP status
	if flow.DeploymentModel == "Kafka" {
		waitUntilDeploymentReady(oc, "flowlogs-pipeline-transformer", flow.Namespace)
	} else {
		waitUntilDaemonSetReady(oc, "flowlogs-pipeline", flow.Namespace)
	}
	// check ebpf-agent status
	waitUntilDaemonSetReady(oc, "netobserv-ebpf-agent", flow.Namespace+"-privileged")

	// check plugin status
	if flow.PluginEnable != "false" {
		waitUntilDeploymentReady(oc, "netobserv-plugin", flow.Namespace)
	}

	exutil.AssertAllPodsToBeReady(oc, flow.Namespace)
	exutil.AssertAllPodsToBeReady(oc, flow.Namespace+"-privileged")
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(context.Context) (done bool, err error) {

		status, err := oc.AsAdmin().Run("get").Args("flowcollector", "-o", "jsonpath='{.items[*].status.conditions[0].reason}'").Output()

		if err != nil {
			return false, err
		}
		if status == "'Ready'" {
			return true, nil
		}
		msg, err := oc.AsAdmin().Run("get").Args("flowcollector", "-o", "jsonpath='{.items[*].status.conditions[0].message}'").Output()
		e2e.Logf("flowcollector status is %s due to %s", status, msg)
		if err != nil {
			return false, err
		}

		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Flowcollector did not become Ready")
}
