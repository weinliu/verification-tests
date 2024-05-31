package storage

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	prometheusQueryURL  string = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query="
	prometheusNamespace string = "openshift-monitoring"
	prometheusK8s       string = "prometheus-k8s"
)

// Define a monitor object
type monitor struct {
	token    string
	ocClient *exutil.CLI
}

// Init a monitor
func newMonitor(oc *exutil.CLI) *monitor {
	var mo monitor
	mo.ocClient = oc
	mo.token = getSAToken(oc)
	return &mo
}

// Get a specified metric's value from prometheus
func (mo *monitor) getSpecifiedMetricValue(metricName string, valueJSONPath string) (metricValue string, err error) {
	getCmd := "curl -k -s -H \"" + fmt.Sprintf("Authorization: Bearer %v", mo.token) + "\" " + prometheusQueryURL + metricName
	// Retry to avoid some network connection system issue
	var responseContent string
	wait.Poll(5*time.Second, 15*time.Second, func() (bool, error) {
		responseContent, err = execCommandInSpecificPod(mo.ocClient, prometheusNamespace, "statefulsets/"+prometheusK8s, getCmd)
		if err != nil {
			e2e.Logf(`Get metric: %q *failed with* :"%v".`, metricName, err)
			return false, err
		}
		return true, nil
	})

	return gjson.Get(responseContent, valueJSONPath).String(), err
}

// Waiting for a specified metric's value update to expected
func (mo *monitor) waitSpecifiedMetricValueAsExpected(metricName string, valueJSONPath string, expectedValue string) {
	err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		realValue, err := mo.getSpecifiedMetricValue(metricName, valueJSONPath)
		if err != nil {
			e2e.Logf("Can't get %v metrics, error: %s. Trying again", metricName, err)
			return false, nil
		}
		if realValue == expectedValue {
			e2e.Logf("The metric: %s's {%s} value become to expected \"%s\"", metricName, valueJSONPath, expectedValue)
			return true, nil
		}
		e2e.Logf("The metric: %s's {%s} current value is \"%s\"", metricName, valueJSONPath, realValue)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for metric: metric: %s's {%s} value become to expected timeout", metricName, valueJSONPath))
}

// GetSAToken get a token assigned to prometheus-k8s from openshift-monitoring namespace
func getSAToken(oc *exutil.CLI) string {
	e2e.Logf("Create a token for prometheus-k8s sa from openshift-monitoring namespace...")
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", prometheusK8s, "-n", prometheusNamespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(token).NotTo(o.BeEmpty())
	return token
}

// Check the alert raised (pending or firing)
func checkAlertRaised(oc *exutil.CLI, alertName string) {
	o.Eventually(func() bool {
		return isSpecifiedAlertRaised(oc, alertName)
	}, 720*time.Second, 30*time.Second).Should(o.BeTrue())
}

// function to check the alert node name
func checkAlertNodeNameMatchDesc(oc *exutil.CLI, alertName string, nodeName string) bool {
	alertData := getAlertContent(oc, alertName)
	description := gjson.Get(alertData, "annotations.description").String()
	nodeNameAlert := strings.SplitAfter(description, "spec.nodeName=")
	nodeNameAlert = strings.Split(nodeNameAlert[1], " --all-namespaces")
	if nodeNameAlert[0] == nodeName {
		e2e.Logf("Node name for alert %v is %v", alertName, nodeName)
		return true
	}
	return false
}

// function to get the alert content for specified alert
func getAlertContent(oc *exutil.CLI, alertName string) string {
	checkAlertRaised(oc, alertName)

	var alertData string
	token := getSAToken(oc)
	url, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", prometheusK8s, "-n", prometheusNamespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	alertCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/v1/alerts", token, url)
	result, err := exec.Command("bash", "-c", alertCMD).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	alertNamesList := gjson.Get(string(result), "data.alerts.#.labels.alertname").String()
	alertNamesList = strings.ReplaceAll(alertNamesList, "[", "")
	alertNamesList = strings.ReplaceAll(alertNamesList, "]", "")
	alertNamesList = strings.ReplaceAll(alertNamesList, "\"", "")
	for i, value := range strings.Split(alertNamesList, ",") {
		if value == alertName {
			alertData = gjson.Get(string(result), "data.alerts."+strconv.Itoa(i)).String()
			break
		}
	}
	return alertData
}

// Check the alert is not there (pending or firing) for about 720sec
func checkSpecifiedAlertNotRaisedConsistently(oc *exutil.CLI, alertName string) {
	o.Consistently(func() bool {
		return isSpecifiedAlertRaised(oc, alertName)
	}, 720*time.Second, 30*time.Second).ShouldNot(o.BeTrue(), "alert state is firing or pending")
}

// Check the alert is there (pending or firing)
func isSpecifiedAlertRaised(oc *exutil.CLI, alertName string) bool {
	token := getSAToken(oc)
	url, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", prometheusK8s, "-n", prometheusNamespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	alertCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/v1/alerts", token, url)
	result, err := exec.Command("bash", "-c", alertCMD).Output()
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Error retrieving prometheus alert: %s", alertName))
	if strings.Contains(gjson.Get(string(result), "data.alerts.#.labels.alertname").String(), alertName) {
		e2e.Logf("Alert %s found with the status firing or pending", alertName)
		return true
	}
	e2e.Logf("Alert %s is not found with the status firing or pending", alertName)
	return false
}

// Get metric with metric name
func getStorageMetrics(oc *exutil.CLI, metricName string) string {
	token := getSAToken(oc)
	output, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-s", "-H", fmt.Sprintf("Authorization: Bearer %v", token), prometheusQueryURL+metricName).Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	debugLogf("The metric outout is:\n %s", output)
	return output
}

// Check if metric contains specified content
func checkStorageMetricsContent(oc *exutil.CLI, metricName string, content string) {
	token := getSAToken(oc)
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", prometheusNamespace, "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), prometheusQueryURL+metricName).Outputs()
		if err != nil {
			e2e.Logf("Can't get %v metrics, error: %s. Trying again", metricName, err)
			return false, nil
		}
		if matched, _ := regexp.MatchString(content, output); matched {
			e2e.Logf("Check the %s in %s metric succeed \n", content, metricName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get %s in %s metric via prometheus", content, metricName))
}

// checkInvalidvSphereStorageClassMetric checks the vsphere specified invalid storageclass should report vsphere_cluster_check_errors metrics, only used for vsphere test clusters
func (mo *monitor) checkInvalidvSphereStorageClassMetric(oc *exutil.CLI, storageClassName string) {
	vsphereProblemDetector := newDeployment(setDeploymentName("vsphere-problem-detector-operator"), setDeploymentNamespace("openshift-cluster-storage-operator"), setDeploymentApplabel("name=vsphere-problem-detector-operator"))
	defer vsphereProblemDetector.waitReady(oc.AsAdmin())
	vsphereProblemDetector.hardRestart(oc.AsAdmin())
	mo.waitSpecifiedMetricValueAsExpected("vsphere_cluster_check_errors", "data.result.#(metric.check=CheckStorageClasses).value.1", "1")

	o.Expect(oc.WithoutNamespace().AsAdmin().Run("delete").Args("sc", storageClassName).Execute()).ShouldNot(o.HaveOccurred())
	mo.waitSpecifiedMetricValueAsExpected("vsphere_cluster_check_errors", "data.result.#(metric.check=CheckStorageClasses).value.1", "0")
}

// function to return the volume counts for the specified provisioner values
func (mo *monitor) getProvisionedVolumesMetric(oc *exutil.CLI, provisioner string) map[string]int64 {
	metricOri := make(map[string]int64, 2)
	metricOri["Filesystem"] = 0
	metricOri["Block"] = 0

	result, vm0err := mo.getSpecifiedMetricValue("cluster:kube_persistentvolume_plugin_type_counts:sum", "data.result")
	o.Expect(vm0err).NotTo(o.HaveOccurred())
	for i := 0; i < strings.Count(result, "plugin_name"); i++ {
		j := strconv.Itoa(i)
		valueOri0 := gjson.GetMany(result, j+".metric.plugin_name", j+".metric.volume_mode", j+".value.1")
		if strings.Contains(valueOri0[0].String(), provisioner) {
			if valueOri0[1].String() == "Filesystem" {
				metricOri["Filesystem"] = valueOri0[2].Int()
			}
			if valueOri0[1].String() == "Block" {
				metricOri["Block"] = valueOri0[2].Int()
			}
		}
	}
	e2e.Logf(`Currently volumes counts metric for provisioner %v is:  "%v"`, provisioner, metricOri)
	return metricOri
}
