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
	Sampling                          string
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

type Flowlog struct {
	// Source
	SrcPort          int
	SrcK8S_Type      string
	SrcK8S_Name      string
	SrcK8S_HostName  string
	SrcK8S_OwnerType string
	SrcAddr          string
	SrcMac           string
	// Destination
	DstPort          int
	DstK8S_Type      string
	DstK8S_Name      string
	DstK8S_HostName  string
	DstK8S_OwnerType string
	DstAddr          string
	DstMac           string
	DstK8S_HostIP    string
	// Protocol
	Proto    int
	IcmpCode int
	IcmpType int
	Dscp     int
	Flags    []string
	// Time
	TimeReceived    int
	TimeFlowEndMs   int
	TimeFlowStartMs int
	// Interface
	IfDirection  int
	IfDirections []int
	Interfaces   []string
	Etype        int
	// Others
	Packets         int
	Bytes           int
	Duplicate       bool
	AgentIP         string
	HashId          string `json:"_HashId,omitempty"`
	IsFirst         bool   `json:"_IsFirst,omitempty"`
	RecordType      string `json:"_RecordType,omitempty"`
	NumFlowLogs     int    `json:"numFlowLogs,omitempty"`
	K8S_ClusterName string `json:"K8S_ClusterName,omitempty"`
	// Zone
	SrcK8S_Zone string `json:"SrcK8S_Zone,omitempty"`
	DstK8S_Zone string `json:"DstK8S_Zone,omitempty"`
	// DNS
	DnsLatencyMs         int    `json:"DnsLatencyMs,omitempty"`
	DnsFlagsResponseCode string `json:"DnsFlagsResponseCode,omitempty"`
	// Packet Drop
	PktDropBytes           int    `json:"PktDropBytes,omitempty"`
	PktDropPackets         int    `json:"PktDropPackets,omitempty"`
	PktDropLatestState     string `json:"PktDropLatestState,omitempty"`
	PktDropLatestDropCause string `json:"PktDropLatestDropCause,omitempty"`
	// RTT
	TimeFlowRttNs int `json:"TimeFlowRttNs,omitempty"`
	// Packet Translation
	XlatDstAddr          string `json:"XlatDstAddr,omitempty"`
	XlatDstK8S_Name      string `json:"XlatDstK8S_Name,omitempty"`
	XlatDstK8S_Namespace string `json:"XlatDstK8S_Namespace,omitempty"`
	XlatDstK8S_Type      string `json:"XlatDstK8S_Type,omitempty"`
	XlatDstPort          int    `json:"XlatDstPort,omitempty"`
	XlatSrcAddr          string `json:"XlatSrcAddr,omitempty"`
	XlatSrcK8S_Name      string `json:"XlatSrcK8S_Name,omitempty"`
	XlatSrcK8S_Namespace string `json:"XlatSrcK8S_Namespace,omitempty"`
	ZoneId               int    `json:"ZoneId,omitempty"`
	// Network Events
	NetworkEvents []NetworkEvent `json:"NetworkEvents,omitempty"`
	// Secondary Network
	SrcK8S_NetworkName string `json:"SrcK8S_NetworkName,omitempty"`
	DstK8S_NetworkName string `json:"DstK8S_NetworkName,omitempty"`
	// UDN
	Udns []string `json:"Udns,omitempty"`
}

type NetworkEvent struct {
	Action    string `json:"Action,omitempty"`
	Type      string `json:"Type,omitempty"`
	Name      string `json:"Name,omitempty"`
	Namespace string `json:"Namespace,omitempty"`
	Direction string `json:"Direction,omitempty"`
	Feature   string `json:"Feature,omitempty"`
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
