package netobserv

import (
	"regexp"

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
			//o.MatchRegexp("DstAS.:[0-9]+"),
			//o.MatchRegexp("DstMac.:\"[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}."),
			//o.MatchRegexp("DstNet.:[0-9]+"),
			o.MatchRegexp("DstPort.:[0-9]+"),
			o.MatchRegexp("Etype.:[0-9]+"),
			o.MatchRegexp("FlowDirection.:[0-9]+"),
			o.MatchRegexp("ForwardingStatus.:[0-9]+"),
			o.MatchRegexp("FragmentId.:[0-9]+"),
			o.MatchRegexp("FragmentOffset.:[0-9]+"),
			o.MatchRegexp("HasMPLS.:(true|false)"),
			o.MatchRegexp("IPv6FlowLabel.:[0-9]+"),
			o.MatchRegexp("IcmpCode.:[0-9]+"),
			o.MatchRegexp("IcmpType.:[0-9]+"),
			o.MatchRegexp("InIf.:[0-9]+"),
			o.MatchRegexp("OutIf.:[0-9]+"),
			o.MatchRegexp("Packets.:[0-9]+"),
			o.MatchRegexp("Proto.:[0-9]+"),
			o.MatchRegexp("SamplerAddress.:[a-zA-Z=]*"),
			o.MatchRegexp("SequenceNum.:[0-9]+"),
			//o.MatchRegexp("SrcAddr.:[0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5]"),
			//o.MatchRegexp("SrcK8S_Name.:[0-9a-zA-Z-]*"),
			//o.MatchRegexp("SrcK8S_Namespace.:[0-9a-zA-Z-]*"),
			//o.MatchRegexp("SrcK8S_OwnerName.:[0-9a-zA-Z-]*"),
			//o.MatchRegexp("SrcK8S_OwnerType.:[a-zA-Z]*"),
			//o.MatchRegexp("SrcK8S_Type.:[a-zA-Z]*"),
			o.MatchRegexp("SrcMac.:\"[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}:[a-zA-Z0-9]{2}\""),
			//o.MatchRegexp("SrcSubnet.:[0-9]+\\//d+"),
			o.MatchRegexp("SrcPort.:[0-9]+"),
			o.MatchRegexp("TimeFlowEnd.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowEndMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowStart.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowStartMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeReceived.:[1-9][0-9]+"),
			o.MatchRegexp("Type.:[0-9]+")))
	}
}
