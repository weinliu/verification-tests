package netobserv

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// get metrics from Prometheus
func getPromMetrics(oc *exutil.CLI, metricsUrl string) string {
	caCert := "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt"
	command := []string{"exec", "-n", "openshift-monitoring", "prometheus-k8s-0", "--", "curl", "-s", "-w 'HTTP response code: %{http_code}'", "-L", metricsUrl, "--cacert", caCert}
	var err error
	var output string
	attempts := 5
	for attempts > 0 {
		output, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().OutputToFile("metrics.txt")
		if err == nil {
			break
		}
		time.Sleep(3 * time.Second)
		attempts--
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).NotTo(o.BeEmpty(), "No Metrics found")
	return output
}

// Verify metrics by doing curl commands
func verifyFLPPromMetrics(oc *exutil.CLI, flpMetricsURL string) {
	metricsOutFile := getPromMetrics(oc, flpMetricsURL)

	// grep the HTTP Code
	metric1, _ := exec.Command("bash", "-c", "cat "+metricsOutFile+" | grep \"HTTP response code\"| awk -F ':' '{print $2}'").Output()
	httpCode := strings.TrimSpace(string(metric1))
	httpCode = strings.Trim(httpCode, "'")
	e2e.Logf("The http code is : %v", httpCode)
	o.Expect(httpCode).To(o.Equal("200"))

	// grep the number of flows processed
	metric2, _ := exec.Command("bash", "-c", "cat "+metricsOutFile+" | grep  -o \"ingest_flows_processed.*\" | tail -1 | awk '{print $2}'").Output()
	flowLogsProcessed := strings.TrimSpace(string(metric2))
	e2e.Logf("Number of flowslogs processed are : %v", flowLogsProcessed)
	o.Expect(flowLogsProcessed).NotTo(o.BeEmpty(), "Number of flowlogs processed is empty")
	flowsProcessedInt, err := strconv.ParseInt(flowLogsProcessed, 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(flowsProcessedInt).Should(o.BeNumerically(">", 0))

	// grep the number of loki records written
	metric3, _ := exec.Command("bash", "-c", "cat "+metricsOutFile+" | grep -o \"records_written.*\" | tail -1 | awk '{print $2}'").Output()
	lokiRecordsWritten := strings.TrimSpace(string(metric3))
	e2e.Logf("Number of loki records written are : %v", lokiRecordsWritten)
	o.Expect(lokiRecordsWritten).NotTo(o.BeEmpty(), "Number of loki records written is empty")

	lokiRecordsWrittenInt, err := strconv.ParseInt(lokiRecordsWritten, 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(lokiRecordsWrittenInt).Should(o.BeNumerically(">", 0))
}

func getMetricsScheme(oc *exutil.CLI, servicemonitor string) (string, error) {
	out, err := oc.AsAdmin().Run("get").Args("servicemonitor", servicemonitor, "-n", oc.Namespace(), "-o", "jsonpath='{.spec.endpoints[].scheme}'").Output()

	return out, err
}

func getMetricsServerName(oc *exutil.CLI, servicemonitor string) (string, error) {
	out, err := oc.AsAdmin().Run("get").Args("servicemonitor", servicemonitor, "-n", oc.Namespace(), "-o", "jsonpath='{.spec.endpoints[].tlsConfig.serverName}'").Output()

	return out, err
}
