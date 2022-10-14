package netobserv

import (
	"os/exec"
	"regexp"
	"strings"

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
	flowRecords := re.FindAllString(podLog, -1)
	e2e.Logf("The flowRecords %v\n\n\n", flowRecords)
	for i, flow := range flowRecords {
		e2e.Logf("The %d th flow record is: %v\n\n\n", i, flow)
		o.Expect(flow).Should(o.And(
			o.MatchRegexp("Bytes.:[0-9]+"),
			o.MatchRegexp("DstMac.:\"[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}."),
			o.MatchRegexp("DstPort.:[0-9]+"),
			o.MatchRegexp("Etype.:[0-9]+"),
			o.MatchRegexp("FlowDirection.:[0-9]+"),
			o.MatchRegexp("Interface.:[0-9a-zA-Z-]*"),
			o.MatchRegexp("Packets.:[0-9]+"),
			o.MatchRegexp("Proto.:[0-9]+"),
			o.MatchRegexp("SrcMac.:\"[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}\""),
			o.MatchRegexp("SrcPort.:[0-9]+"),
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
	metric1, _ := exec.Command("bash", "-c", "cat "+output+" | grep \"HTTP/2\" | awk 'NR==4{print $3}'").Output()
	httpCode := strings.TrimSpace(string(metric1))
	o.Expect(httpCode).NotTo(o.BeEmpty(), "HTTP Code not found")
	e2e.Logf("The http code is : %v", httpCode)

	// grep the number of flows processed
	metric2, _ := exec.Command("bash", "-c", "cat "+output+" | grep ingest_flows_processed | awk 'NR==3{print $2}'").Output()
	flowLogsProcessed := strings.TrimSpace(string(metric2))
	o.Expect(flowLogsProcessed).NotTo(o.BeEmpty(), "The number of flowlogs processed is empty")
	e2e.Logf("The number of flowslogs processed are : %v", flowLogsProcessed)

	// grep the number of loki records written
	metric3, _ := exec.Command("bash", "-c", "cat "+output+" | grep records_written | awk 'NR==3{print $2}'").Output()
	lokiRecordsWritten := strings.TrimSpace(string(metric3))
	o.Expect(lokiRecordsWritten).NotTo(o.BeEmpty(), "The number of loki records written is empty")
	e2e.Logf("The number of loki records written are : %v", lokiRecordsWritten)

	// verify all the metrics
	o.Expect(httpCode).To(o.Equal("200"))
	o.Expect(flowLogsProcessed).NotTo(o.Equal("0"))
	o.Expect(lokiRecordsWritten).NotTo(o.Equal("0"))
}
