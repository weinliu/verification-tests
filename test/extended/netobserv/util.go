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
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type flowlogsPipelineDescription struct {
	serviceNs string
	name      string
	cmname    string
	template  string
}

func (flowlogsPipeline *flowlogsPipelineDescription) create(oc *exutil.CLI, ns string, flowlogsPipelineDeploymenTemplate string) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", flowlogsPipelineDeploymenTemplate, "-p", "NAMESPACE="+ns)
}

func waitPodReady(oc *exutil.CLI, ns string, label string) {
	podName := getFlowlogsPipelinePod(oc, ns, label)
	exutil.AssertPodToBeReady(oc, podName, ns)
}

func patchResourceAsAdmin(oc *exutil.CLI, ns, resource, rsname, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, rsname, "--type=json", "-p", patch, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getFlowlogsPipelineCollector(oc *exutil.CLI, resource string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("get flowCollector: %v", output)
	return output
}

// get name of flowlogsPipeline pod by label
func getFlowlogsPipelinePod(oc *exutil.CLI, ns string, name string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", "app="+name, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of podname:%v", podName)
	return podName
}

// Verify some key and deterministic fields and their values
func verifyFlowRecord(podLog string) {
	re := regexp.MustCompile(`{\"Bytes\":.*}`)
	//e2e.Logf("the logs of flowlogs-pipeline pods are: %v", podLog)
	flowRecords := re.FindAllString(podLog, -3)
	//e2e.Logf("The flowRecords %v\n\n\n", flowRecords)
	for i, flow := range flowRecords {
		e2e.Logf("The %d th flow record is: %v\n\n\n", i, flow)
		o.Expect(flow).Should(o.And(
			o.MatchRegexp("Bytes.:[0-9]+"),
			o.MatchRegexp("FlowDirection.:[0-9]+"),
			o.MatchRegexp("Interface.:[0-9a-zA-Z-]*"),
			o.MatchRegexp("Proto.:[0-9]+"),
			o.MatchRegexp("TimeFlowEndMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowStartMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeReceived.:[1-9][0-9]+")))
	}
}

// Verify metrics by doing curl commands
func verifyCurl(oc *exutil.CLI, podName string, ns string, curlDest string, CertPath string) {
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

func verifyTime(oc *exutil.CLI, namespace string, lokiStackName string, lokiStackNS string) {
	var s string
	bearerToken := getSAToken(oc, "flowlogs-pipeline", namespace)
	route := "https://" + getRouteAddress(oc, lokiStackNS, lokiStackName)
	lc := newLokiClient(route).withToken(bearerToken).retry(5)
	res, err := lc.searchLogsInLoki("infrastructure", "{app=\"netobserv-flowcollector\"}")
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(res.Data.Result) == 0 {
		exutil.AssertWaitPollNoErr(err, "infrastructure logs are not found")
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

	timeminus := now - logtime
	o.Expect(timeminus).Should(o.BeNumerically(">", 0))
	o.Expect(timeminus).Should(o.BeNumerically("<=", 300000000000))
}
