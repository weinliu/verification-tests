package netobserv

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type flowlogsPipelineDescription struct {
	serviceNs string
	name      string
	cmname    string
	template  string
}

type TestClientServerTemplate struct {
	ServerNS   string
	ClientNS   string
	ObjectSize string
	Template   string
}

// returns ture/false if flowcollector API exists.
func isFlowCollectorAPIExists(oc *exutil.CLI) (bool, error) {
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "-o", "jsonpath='{.items[*].spec.names.kind}'").Output()

	if err != nil {
		return false, err
	}
	return strings.Contains(stdout, "FlowCollector"), nil
}

func (flowlogsPipeline *flowlogsPipelineDescription) create(oc *exutil.CLI, ns, flowlogsPipelineDeploymenTemplate string) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", flowlogsPipelineDeploymenTemplate, "-p", "NAMESPACE="+ns)
}

func getFlowlogsPipelineCollector(oc *exutil.CLI, resource string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("get flowCollector: %v", output)
	return output
}

// get name of flowlogsPipeline pod by label
func getFlowlogsPipelinePod(oc *exutil.CLI, ns, name string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", "app="+name, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of podname:%v", podName)
	return podName
}

func getFlowRecords(lokiValues [][]string) ([]FlowRecord, error) {
	flowRecords := []FlowRecord{}
	for _, values := range lokiValues {
		timestamp, _ := strconv.ParseInt(values[0], 10, 64)
		var flowlog Flowlog
		err := json.Unmarshal([]byte(values[1]), &flowlog)
		if err != nil {
			return []FlowRecord{}, err
		}
		flowRecord := FlowRecord{
			Timestamp: timestamp,
			Flowlog:   flowlog,
		}
		flowRecords = append(flowRecords, flowRecord)
	}
	return flowRecords, nil
}

// Verify flow records from logs
func verifyFlowRecordFromLogs(podLog string) {
	re := regexp.MustCompile("{\"AgentIP\":.*")
	//e2e.Logf("the logs of flowlogs-pipeline pods are: %v", podLog)
	flowRecords := re.FindAllString(podLog, -3)
	//e2e.Logf("The flowRecords %v\n\n\n", flowRecords)
	// regex for ip
	//numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	//regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock
	for _, flow := range flowRecords {
		//e2e.Logf("The %d th flow record is: %v\n\n\n", i, flow)
		o.Expect(flow).Should(o.And(
			//o.MatchRegexp(fmt.Sprintf("AgentIP.:%s", regexPattern)),
			o.MatchRegexp("Bytes.:[0-9]+"),
			o.MatchRegexp("Duplicate.:(true|false)"),
			o.MatchRegexp("TimeFlowEndMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowStartMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeReceived.:[1-9][0-9]+")))
	}
}

// Verify some key and deterministic fields and their values
func (flowlog *Flowlog) verifyFlowRecord() {
	o.Expect(flowlog.AgentIP).To(o.Equal(flowlog.DstK8S_HostIP))
	o.Expect(flowlog.Bytes).Should(o.BeNumerically(">", 0))
	var testDuplicate bool
	o.Expect(flowlog.Duplicate).To(o.BeAssignableToTypeOf(testDuplicate))
	now := time.Now()
	compareTime := now.Add(time.Duration(-2) * time.Hour)
	compareTimeMs := compareTime.UnixMilli()
	o.Expect(flowlog.TimeFlowEndMs).Should(o.BeNumerically(">", compareTimeMs))
	o.Expect(flowlog.TimeFlowStartMs).Should(o.BeNumerically(">", compareTimeMs))
	o.Expect(flowlog.TimeReceived).Should(o.BeNumerically(">", compareTime.Unix()))
}

// Get flows from Loki logs
func (testTemplate *TestClientServerTemplate) getLokiFlowLogs(oc *exutil.CLI, token, namespace, lokiStackName string) ([]FlowRecord, error) {
	route := "https://" + getRouteAddress(oc, namespace, lokiStackName)
	lc := newLokiClient(route).withToken(token).retry(5)
	lokiQuery := fmt.Sprintf("{app=\"netobserv-flowcollector\", DstK8S_Namespace=\"%s\", SrcK8S_Namespace=\"%s\", FlowDirection=\"0\"}", testTemplate.ClientNS, testTemplate.ServerNS)
	tenantID := "network"

	var res *lokiQueryResponse
	err := wait.Poll(30*time.Second, 300*time.Second, func() (done bool, err error) {
		var qErr error
		res, qErr = lc.searchLogsInLoki(tenantID, lokiQuery)
		if qErr != nil {
			e2e.Logf("\ngot error %v when getting %s logs for query: %s\n", qErr, tenantID, lokiQuery)
			return false, qErr
		}
		return len(res.Data.Result) > 0, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s logs are not found", tenantID))
	flowRecords := []FlowRecord{}

	for _, result := range res.Data.Result {
		if result.Stream.DstK8S_Namespace == testTemplate.ClientNS && result.Stream.SrcK8S_Namespace == testTemplate.ServerNS && result.Stream.SrcK8S_OwnerName == "nginx-service" {
			flowRecords, err = getFlowRecords(result.Values)
		}
	}
	return flowRecords, err
}

// Verify metrics by doing curl commands
func verifyCurl(oc *exutil.CLI, podName, ns, curlDest, CertPath string) {
	command := []string{"exec", "-n", ns, podName, "--", "curl", "-s", "-v", "-L", curlDest, "--cacert", CertPath}
	output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().OutputToFile("metrics.txt")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).NotTo(o.BeEmpty(), "No Metrics found")

	// grep the HTTPS Code
	metric1, _ := exec.Command("bash", "-c", "cat "+output+" | grep -o \"HTTP/2.*\"| tail -1 | awk '{print $2}'").Output()
	httpCode := strings.TrimSpace(string(metric1))
	e2e.Logf("The http code is : %v", httpCode)
	o.Expect(httpCode).NotTo(o.BeEmpty(), "HTTP Code not found")

	// grep the number of flows processed
	metric2, _ := exec.Command("bash", "-c", "cat "+output+" | grep  -o \"ingest_flows_processed.*\" | tail -1 | awk '{print $2}'").Output()
	flowLogsProcessed := strings.TrimSpace(string(metric2))
	e2e.Logf("The number of flowslogs processed are : %v", flowLogsProcessed)
	o.Expect(flowLogsProcessed).NotTo(o.BeEmpty(), "The number of flowlogs processed is empty")

	// grep the number of loki records written
	metric3, _ := exec.Command("bash", "-c", "cat "+output+" | grep -o \"records_written.*\" | tail -1 | awk '{print $2}'").Output()
	lokiRecordsWritten := strings.TrimSpace(string(metric3))
	e2e.Logf("The number of loki records written are : %v", lokiRecordsWritten)
	o.Expect(lokiRecordsWritten).NotTo(o.BeEmpty(), "The number of loki records written is empty")

	flowsProcessedInt, err := strconv.ParseInt(flowLogsProcessed, 10, 64)
	if err == nil {
		e2e.Logf("%d of type %T", flowsProcessedInt, flowsProcessedInt)
	}

	lokiRecordsWrittenInt, err := strconv.ParseInt(lokiRecordsWritten, 10, 64)
	if err == nil {
		e2e.Logf("%d of type %T", lokiRecordsWrittenInt, lokiRecordsWrittenInt)
	}

	// verify all the metrics
	o.Expect(httpCode).To(o.Equal("200"))
	o.Expect(flowsProcessedInt).Should(o.BeNumerically(">", 0))
	o.Expect(lokiRecordsWrittenInt).Should(o.BeNumerically(">", 0))
}

// Verify loki records and if it was written in the last 5 minutes
func verifyLokilogsTime(oc *exutil.CLI, lokiStackNS, flowNS, lokiStackName, serviceAccountName string) error {
	bearerToken := getSAToken(oc, serviceAccountName, flowNS)
	route := "https://" + getRouteAddress(oc, lokiStackNS, lokiStackName)
	lc := newLokiClient(route).withToken(bearerToken).retry(5)
	res, err := lc.searchLogsInLoki("network", "{app=\"netobserv-flowcollector\", FlowDirection=\"0\"}")

	if err != nil {
		return err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(res.Data.Result) == 0 {
		exutil.AssertWaitPollNoErr(err, "network logs are not found")
	}
	flowRecords := []FlowRecord{}

	for _, result := range res.Data.Result {
		flowRecords, err = getFlowRecords(result.Values)
		if err != nil {
			return err
		}
	}

	for _, r := range flowRecords {
		now := time.Now().UnixNano()
		// check if the record is written in the last 5 mins
		timeminus := now - r.Timestamp
		o.Expect(timeminus).Should(o.BeNumerically(">", 0))
		o.Expect(timeminus).Should(o.BeNumerically("<=", 120000000000))
		r.Flowlog.verifyFlowRecord()
	}
	return nil
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

func (testTemplate *TestClientServerTemplate) createTestClientServer(oc *exutil.CLI) error {
	configFile := exutil.ProcessTemplate(oc, "--ignore-unknown-parameters=true", "-f", testTemplate.Template, "-p", "SERVERNS="+testTemplate.ServerNS, "-p", "CLIENTNS="+testTemplate.ClientNS, "-p", "OBJECT_SIZE="+testTemplate.ObjectSize)

	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
	if err != nil {
		return err
	}
	return nil
}
