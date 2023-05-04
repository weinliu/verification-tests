package netobserv

import (
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

// Verify some key and deterministic fields and their values
func verifyFlowRecord(podLog string) {
	re := regexp.MustCompile("{\"AgentIP\":.*")
	//e2e.Logf("the logs of flowlogs-pipeline pods are: %v", podLog)
	flowRecords := re.FindAllString(podLog, -3)
	//e2e.Logf("The flowRecords %v\n\n\n", flowRecords)
	// regex for ip
	//numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	//regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock
	for i, flow := range flowRecords {
		e2e.Logf("The %d th flow record is: %v\n\n\n", i, flow)
		o.Expect(flow).Should(o.And(
			//o.MatchRegexp(fmt.Sprintf("AgentIP.:%s", regexPattern)),
			o.MatchRegexp("Bytes.:[0-9]+"),
			o.MatchRegexp("Duplicate.:(true|false)"),
			o.MatchRegexp("TimeFlowEndMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowStartMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeReceived.:[1-9][0-9]+")))
	}
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
func verifyTime(oc *exutil.CLI, namespace, lokiStackName, serviceAccountName, lokiStackNS string) {
	var s string
	bearerToken := getSAToken(oc, serviceAccountName, namespace)
	route := "https://" + getRouteAddress(oc, lokiStackNS, lokiStackName)
	lc := newLokiClient(route).withToken(bearerToken).retry(5)
	res, err := lc.searchLogsInLoki("network", "{app=\"netobserv-flowcollector\"}")
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(res.Data.Result) == 0 {
		exutil.AssertWaitPollNoErr(err, "network logs are not found")
	}

	for _, r := range res.Data.Result {
		e2e.Logf("\nlog is : %v\n", r.Values[0])
		s = fmt.Sprint(r.Values[0])
	}
	l := strings.Split(s, " ")

	ltime := strings.Replace(l[0], "[", "", 1)

	logtime, err := strconv.ParseInt(ltime, 10, 64)
	if err == nil {
		e2e.Logf("%d of type %T", logtime, logtime)
	}

	now := time.Now().UnixNano()

	// check if the record is written in the last 5 mins
	timeminus := now - logtime
	o.Expect(timeminus).Should(o.BeNumerically(">", 0))
	o.Expect(timeminus).Should(o.BeNumerically("<=", 300000000000))
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
