package monitoring

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	"reflect"

	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type monitoringStackSecretDescription struct {
	name      string
	namespace string
	template  string
}
type monitoringStackDescription struct {
	name       string
	clusterID  string
	namespace  string
	secretName string
	tokenURL   string
	url        string
	region     string
	template   string
}

const (
	subName    = "observability-operator"
	ogName     = "observability-operator-og"
	namespace  = "openshift-observability-operator"
	monSvcName = "hypershift-monitoring-stack-prometheus"
)

var (
	csvName string
	targets = []string{"catalog-operator", "cluster-version-operator", "etcd", "kube-apiserver", "kube-controller-manager", "monitor-multus-admission-controller", "monitor-ovn-master-metrics", "node-tuning-operator", "olm-operator", "openshift-apiserver", "openshift-controller-manager", "openshift-route-controller-manager"}
)

func checkSubscription(oc *exutil.CLI) (out string, err error) {
	exutil.By("Check the state of Operator")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", subName, "-n", namespace, "-o=jsonpath={.status.state}").Output()
		if strings.Contains(out, "NotFound") || strings.Contains(out, "No resources") || err != nil {
			return false, err
		}
		if strings.Compare(out, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Subscription %v doesnot contain the correct status in namespace %v", subName, namespace))

	exutil.By("Get ClusterServiceVersion name")
	csvName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", subName, "-n", namespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	exutil.By("Check that ClusterServiceVersion " + csvName + " is finished")
	errCheck = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterserviceversions", csvName, "-n", namespace, "-o=jsonpath={.status.phase}{.status.reason}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(out, "SucceededInstallSucceeded") == 0 {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("ClusterServiceVersion %v is not successfully finished in namespace %v with error: %v", csvName, namespace, err))
	out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", subName, "-n", namespace, "--no-headers").Output()
	return out, err
}

func createOperator(oc *exutil.CLI, ogTemplate string, subTemplate string, nsTemplate string) {
	exutil.By("Create Namespace")
	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", nsTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	exutil.By("Create Operator Group")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", ogTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	exutil.By("Create subscription")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	out, err := checkSubscription(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Output: %v", out)
}
func createObservabilityOperator(oc *exutil.CLI, oboBaseDir string) {
	ogTemplate := filepath.Join(oboBaseDir, "operator-group.yaml")
	subTemplate := filepath.Join(oboBaseDir, "subscription.yaml")
	nsTemplate := filepath.Join(oboBaseDir, "namespace.yaml")
	exutil.By("Install Observability Operator")
	createOperator(oc, ogTemplate, subTemplate, nsTemplate)
	exutil.By("create servicemonitor")
	smTemplate := filepath.Join(oboBaseDir, "obo-service-monitor.yaml")
	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", smTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	cooVersion, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-o=jsonpath={.items[?(@.spec.displayName==\"Cluster Observability Operator\")].metadata.name}", "-n", "openshift-monitoring").Output()
	waitErr := oc.AsAdmin().WithoutNamespace().Run("wait").Args("csv/"+cooVersion, "--for=jsonpath={.status.phase}=Succeeded", "--timeout=5m", "-n", "openshift-monitoring").Execute()
	if waitErr != nil {
		g.Skip("COO is not ready or been installed")
	}

}
func getClusterDetails(oc *exutil.CLI) (clusterID string, region string) {
	cluserID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversions", "version", "-o=jsonpath={.spec.clusterID}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus..region}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return cluserID, region
}
func createMonitoringStack(oc *exutil.CLI, msD monitoringStackDescription, secD monitoringStackSecretDescription) {
	exutil.By("Creating Monitoring Stack")
	createStack(oc, msD, secD, "rosa_mc", "")
}
func createStack(oc *exutil.CLI, msD monitoringStackDescription, secD monitoringStackSecretDescription, stack, oboBaseDir string) {
	stack = strings.ToLower(stack)
	if stack == "rosa_mc" {
		exutil.By("Creating Secret")
		secFile, err := oc.AsAdmin().Run("process").Args("-f", secD.template, "-p", "NAME="+secD.name, "NAMESPACE="+secD.namespace).OutputToFile(getRandomString() + "ms-secret.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", secFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Install Monitoring Stack")
		msFile, err := oc.AsAdmin().Run("process").Args("-f", msD.template, "-p", "CLUSTERID="+msD.clusterID, "REGION="+msD.region, "NAME="+msD.name, "NAMESPACE="+msD.namespace, "SECRETNAME="+msD.secretName, "TOKENURL="+msD.tokenURL, "URL="+msD.url).OutputToFile(getRandomString() + "ms.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", msFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	if stack == "monitor_example_app" {
		exutil.By("Install Monitoring Stack")
		var msTemplate string
		if exutil.IsSNOCluster(oc) {
			msTemplate = filepath.Join(oboBaseDir, "example-app-monitoring-stack-sno.yaml")
		} else {
			msTemplate = filepath.Join(oboBaseDir, "example-app-monitoring-stack.yaml")
		}
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", msTemplate).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	exutil.By("Check MonitoringStack status")
	checkMonitoringStack(oc, msD, stack)
	exutil.By("Check MonitoringStack Prometheus pods status")
	checkMonitoringStackPods(oc, stack)
}
func checkMonitoringStack(oc *exutil.CLI, msD monitoringStackDescription, stack string) {
	var name string
	stack = strings.ToLower(stack)
	if stack == "rosa_mc" {
		name = msD.name
	}
	if stack == "monitor_example_app" {
		name = "example-app-monitoring-stack"
	}
	exutil.By("Check the state of MonitoringStack")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("monitoringstack", name, "-n", namespace, "-o=jsonpath={.status.conditions[*].reason}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(out, "MonitoringStackAvailable") {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Monitoring Stack %v doesnot contain the correct status in namespace %v", name, namespace))
}
func checkMonitoringStackPods(oc *exutil.CLI, stack string) {
	exutil.By("Check " + namespace + " namespace monitoringstack pods liveliness")
	var name string
	if stack == "rosa_mc" {
		name = "hypershift-monitoring-stack"
	}
	if stack == "monitor_example_app" {
		name = "example-app-monitoring-stack"
	}
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", "prometheus="+name, "-o=jsonpath={.items[*].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if exutil.IsSNOCluster(oc) {
			if strings.Compare(out, "Running") == 0 {
				return true, nil
			}
		} else {
			if strings.Compare(out, "Running Running") == 0 {
				return true, nil
			}
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v namespace monitoringstack pods are not in healthy state", namespace))
}
func checkOperatorPods(oc *exutil.CLI) {
	exutil.By("Check " + namespace + " namespace pods liveliness")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-o", "jsonpath={.items[*].status.phase}").Output()
		if strings.Compare(out, "Running Running Running Running") == 0 {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v namespace does not contain pods", namespace))
}
func checkRemoteWriteConfig(oc *exutil.CLI, msD monitoringStackDescription) {
	var (
		actual              interface{}
		expected            interface{}
		remoteWriteExpected = fmt.Sprintf(`[
			{
			  "oauth2": {
				"clientId": {
				  "secret": {
					"key": "client-id",
					"name": "%v"
				  }
				},
				"clientSecret": {
				  "key": "client-secret",
				  "name": "%v"
				},
				"tokenUrl": "%v"
			  },
			  "url": "%v"
			}
		  ]`, msD.secretName, msD.secretName, msD.tokenURL, msD.url)
	)

	exutil.By("Check remote write config")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("monitoringstack", msD.name, "-n", msD.namespace, "-o=jsonpath={.spec.prometheusConfig.remoteWrite}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		actual = gjson.Parse(out).Value()
		expected = gjson.Parse(remoteWriteExpected).Value()
		if reflect.DeepEqual(actual, expected) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Remote write config is not correct in monitoringstack %v in %v namespace", msD.name, msD.namespace))
}
func checkMonitoringStackDetails(oc *exutil.CLI, msD monitoringStackDescription, stack string) {
	var name string
	stack = strings.ToLower(stack)
	if stack == "rosa_mc" {
		name = msD.name
		exutil.By("Get clusterID and region")
		errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("monitoringstack", msD.name, "-n", msD.namespace, "-o=jsonpath={.spec.prometheusConfig.externalLabels.hypershift_cluster_id}{.spec.prometheusConfig.externalLabels.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Compare(out, msD.clusterID+msD.region) == 0 {
				return true, nil
			}
			return false, err
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("ClusterID and region did not match. Expected: %v %v", msD.clusterID, msD.region))
	}
	if stack == "custom" {
		name = "hypershift-monitoring-stack"
	}
	exutil.By("Check status of MonitoringStack")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("monitoringstack", name, "-n", namespace, "-o=jsonpath={.status.conditions[*].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(out, "False") {
			return false, err
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("MonitoringStack %v reports invalid status in namespace %v", name, namespace))
}
func deleteMonitoringStack(oc *exutil.CLI, msD monitoringStackDescription, secD monitoringStackSecretDescription, stack string) {
	stack = strings.ToLower(stack)
	if stack == "rosa_mc" {
		exutil.By("Removing MonitoringStack " + msD.name)
		errStack := oc.AsAdmin().WithoutNamespace().Run("delete").Args("monitoringstack", msD.name, "-n", msD.namespace).Execute()
		exutil.By("Removing MonitoringStack Secret " + secD.name)
		errSecret := oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", secD.name, "-n", secD.namespace).Execute()
		o.Expect(errStack).NotTo(o.HaveOccurred())
		o.Expect(errSecret).NotTo(o.HaveOccurred())
	}
	if stack == "monitor_example_app" {
		exutil.By("Removing MonitoringStack hypershift-monitoring-stack")
		errStack := oc.AsAdmin().WithoutNamespace().Run("delete").Args("monitoringstack", "example-app-monitoring-stack", "-n", "openshift-observability-operator").Execute()
		o.Expect(errStack).NotTo(o.HaveOccurred())
	}
}
func deleteOperator(oc *exutil.CLI) {
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", subName, "-n", namespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	exutil.By("Removing servicemoitor")
	errSm := oc.AsAdmin().WithoutNamespace().Run("delete").Args("servicemonitors.monitoring.coreos.com", "observability-operator", "-n", namespace).Execute()
	exutil.By("Removing ClusterServiceVersion " + csvName)
	errCsv := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterserviceversions", csvName, "-n", namespace).Execute()
	exutil.By("Removing Subscription " + subName)
	errSub := oc.AsAdmin().WithoutNamespace().Run("delete").Args("subscription", subName, "-n", namespace).Execute()
	exutil.By("Removing OperatorGroup " + ogName)
	errOg := oc.AsAdmin().WithoutNamespace().Run("delete").Args("operatorgroup", ogName, "-n", namespace).Execute()
	exutil.By("Removing Namespace " + namespace)
	errNs := oc.AsAdmin().WithoutNamespace().Run("delete").Args("namespace", namespace, "--force").Execute()
	crds, err := oc.AsAdmin().WithoutNamespace().Run("api-resources").Args("--api-group=monitoring.rhobs", "-o", "name").Output()
	if err != nil {
		e2e.Logf("err %v, crds %v", err, crds)
	} else {
		crda := append([]string{"crd"}, strings.Split(crds, "\n")...)
		errCRD := oc.AsAdmin().WithoutNamespace().Run("delete").Args(crda...).Execute()
		o.Expect(errCRD).NotTo(o.HaveOccurred())
	}
	o.Expect(errSm).NotTo(o.HaveOccurred())
	o.Expect(errCsv).NotTo(o.HaveOccurred())
	o.Expect(errSub).NotTo(o.HaveOccurred())
	o.Expect(errOg).NotTo(o.HaveOccurred())
	o.Expect(errNs).NotTo(o.HaveOccurred())
}
func checkRuleExists(oc *exutil.CLI, token, routeName, namespace, ruleName string) bool {
	var rules []gjson.Result
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		path, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", routeName, "-n", namespace, "-o=jsonpath={.spec.path}").Output()
		if err != nil {
			return false, nil
		}
		host, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", routeName, "-n", namespace, "-o=jsonpath={.spec.host}").Output()
		if err != nil {
			return false, nil
		}
		ruleCmd := fmt.Sprintf("curl -G -s -k -H\"Authorization: Bearer %s\" https://%s%s/v1/rules", token, host, path)
		out, err := exec.Command("bash", "-c", ruleCmd).Output()
		if err != nil {
			return false, nil
		}
		rules = gjson.ParseBytes(out).Get("data.groups.#.file").Array()
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, "Rules are not loaded")
	for _, rule := range rules {
		if strings.Contains(rule.String(), ruleName) {
			return true
		}
	}
	return false
}
func checkConfigMapExists(oc *exutil.CLI, namespace, configmapName, checkStr string) bool {
	searchOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", configmapName, "-n", namespace, "-o=jsonpath={.data.config\\.yaml}").Output()
	if err != nil {
		return false
	}
	if strings.Contains(searchOutput, checkStr) {
		return true
	}
	return false
}
func createConfig(oc *exutil.CLI, namespace, cmName, config string) {
	if !checkConfigMapExists(oc, namespace, cmName, "enableUserWorkload: true") {
		e2e.Logf("Create configmap: user-workload-monitoring-config")
		output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", config).Output()
		if err != nil {
			if strings.Contains(output, "AlreadyExists") {
				err = nil
			}
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}
func checkOperatorMonitoring(oc *exutil.CLI, oboBaseDir string) {
	exutil.By("Check if UWM exists")
	uwMonitoringConfig := filepath.Join(oboBaseDir, "user-workload-monitoring-cm.yaml")
	createConfig(oc, "openshift-monitoring", "cluster-monitoring-config", uwMonitoringConfig)
	exutil.By("Get SA token")
	token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
	exutil.By("Check prometheus rules")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheusrule", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(out, "alertmanager-rules") && strings.Contains(out, "prometheus-operator-rules") && strings.Contains(out, "prometheus-rules") && strings.Contains(out, "observability-operator-rules") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Prometheus rules are not created in %v namespace", namespace))
	exutil.By("Check Observability Operator Alertmanager Rules")
	errCheck = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		IsAlertManagerRule := checkRuleExists(oc, token, "thanos-querier", "openshift-monitoring", "openshift-observability-operator-observability-operator-alertmanager-rules")
		exutil.By("Check Observability Operator Prometheus Operator Rules")
		IsPrometheusOperatorRule := checkRuleExists(oc, token, "thanos-querier", "openshift-monitoring", "openshift-observability-operator-observability-operator-prometheus-operator-rules")
		exutil.By("Check Observability Operator Prometheus Rules")
		IsPrometheusRule := checkRuleExists(oc, token, "thanos-querier", "openshift-monitoring", "openshift-observability-operator-observability-operator-prometheus-rules")
		exutil.By("Check Observability Operator Rules")
		IsOperatorRule := checkRuleExists(oc, token, "thanos-querier", "openshift-monitoring", "openshift-observability-operator-observability-operator-rules")
		if IsAlertManagerRule && IsPrometheusOperatorRule && IsPrometheusRule && IsOperatorRule {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, "Observability operator rules are not loaded")
	exutil.By("Check Observability Operator metrics")
	checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query={__name__=~"controller_runtime_reconcile.*",job="observability-operator",namespace="openshift-observability-operator"}'`, token, "openshift-observability-operator", uwmLoadTime)
}
func checkLabel(oc *exutil.CLI) {
	var labelName = "network.openshift.io/policy-group=monitoring"
	exutil.By("Check if the label" + labelName + "exists in the namespace" + namespace)
	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("namespace", namespace, "-o=jsonpath={.metadata.labels}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(strings.Contains(out, "monitoring")).To(o.BeTrue())
}
func checkPodHealth(oc *exutil.CLI) {
	var (
		actualLiveness  interface{}
		actualReadiness interface{}
		outputLiveness  = `{
			"failureThreshold": 3,
			"httpGet": {
			  "path": "/healthz",
			  "port": 8081,
			  "scheme": "HTTP"
			},
			"periodSeconds": 10,
			"successThreshold": 1,
			"timeoutSeconds": 1
		  }`
		outputReadiness = `{
			"failureThreshold": 3,
			"httpGet": {
			  "path": "/healthz",
			  "port": 8081,
			  "scheme": "HTTP"
			},
			"periodSeconds": 10,
			"successThreshold": 1,
			"timeoutSeconds": 1
		  }`
		expectedLiveness  = gjson.Parse(outputLiveness).Value()
		expectedReadiness = gjson.Parse(outputReadiness).Value()
	)

	exutil.By("Check remote write config")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		exutil.By("Get the observability operator pod")
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-l", "app.kubernetes.io/name=observability-operator", "-oname").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Get the liveliness for " + podName)
		livenessOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(podName, "-n", namespace, "-o=jsonpath={.spec.containers[].livenessProbe}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		readinessOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(podName, "-n", namespace, "-o=jsonpath={.spec.containers[].readinessProbe}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Live: %v", livenessOut)
		e2e.Logf("Ready: %v", readinessOut)
		actualLiveness = gjson.Parse(livenessOut).Value()
		actualReadiness = gjson.Parse(readinessOut).Value()
		if reflect.DeepEqual(actualLiveness, expectedLiveness) && reflect.DeepEqual(actualReadiness, expectedReadiness) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, "liveness/readiness probe not implemented correctly in observability operator pod")
}
func checkHCPTargets(oc *exutil.CLI) {
	exutil.By("Get SA token")
	token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
	exutil.By("Check whether the scrape targets are present")
	for _, target := range targets {
		checkMetric(oc, fmt.Sprintf(`http://%s.%s.svc.cluster.local:9090/api/v1/query --data-urlencode 'query=prometheus_sd_discovered_targets{config=~".*%s.*"}' `, monSvcName, namespace, target), token, target, platformLoadTime)
	}
}
func checkExampleAppTarget(oc *exutil.CLI) {
	exutil.By("Get SA token")
	token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
	exutil.By("Check whether the scrape targets are present")
	checkMetric(oc, fmt.Sprintf(`http://%s.%s.svc.cluster.local:9090/api/v1/query --data-urlencode 'query=prometheus_sd_discovered_targets{config=~".*%s.*"}' `, "example-app-monitoring-stack-prometheus", namespace, "prometheus-example-monitor"), token, "prometheus-example-monitor", uwmLoadTime)
}
func checkIfMetricValueExists(oc *exutil.CLI, token, url string, timeout time.Duration) {
	var (
		res string
		err error
	)
	getCmd := "curl -G -k -s -H \"Authorization:Bearer " + token + "\" " + url
	err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		res, err = exutil.RemoteShPod(oc, "openshift-monitoring", "prometheus-k8s-0", "sh", "-c", getCmd)
		val := gjson.Parse(res).Get("data.result.#.value").Array()
		if err != nil || len(val) == 0 {
			return false, nil
		}
		return true, err
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The metric %s does not contain any value", res))
}
func checkMetricValue(oc *exutil.CLI, clusterType string) {
	exutil.By("Get SA token")
	token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
	exutil.By("Check the metrics exists and contain value")
	if clusterType == "rosa_mc" {
		checkIfMetricValueExists(oc, token, fmt.Sprintf(`http://%s.%s.svc.cluster.local:9090/api/v1/query --data-urlencode 'query=topk(1,cluster_version{type="cluster"})' `, monSvcName, namespace), platformLoadTime)
	} else {
		checkIfMetricValueExists(oc, token, fmt.Sprintf(`http://%s.%s.svc.cluster.local:9090/api/v1/query --data-urlencode 'query=version' `, "example-app-monitoring-stack-prometheus", namespace), platformLoadTime)
	}
}
func createCustomMonitoringStack(oc *exutil.CLI, oboBaseDir string) {
	exutil.By("Create Clustom Monitoring Stack")
	createStack(oc, monitoringStackDescription{}, monitoringStackSecretDescription{}, "monitor_example_app", oboBaseDir)
}
func checkExampleAppStatus(oc *exutil.CLI, ns string) {
	exutil.By("Check the status of Example App")
	errCheck := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 180*time.Second, false, func(ctx context.Context) (bool, error) {
		exutil.By("Get the pod name")
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", "app=prometheus-example-app", "-oname").Output()
		if err != nil {
			return false, nil
		}
		exutil.By("Check the status of pod")
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(podName, "-n", ns, "-o=jsonpath={.status.phase}").Output()
		if err != nil {
			return false, nil
		}
		exutil.By("Check service is present")
		svcName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns, "-l", "app=prometheus-example-app", "-oname").Output()
		if err != nil {
			return false, nil
		}
		e2e.Logf("Service: %v", svcName)
		exutil.By("Check service monitor is present")
		svMonName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("servicemonitor.monitoring.rhobs", "-n", ns, "-l", "k8s-app=prometheus-example-monitor", "-oname").Output()
		if err != nil {
			return false, nil
		}
		e2e.Logf("Service Monitor: %v", svMonName)
		if status != "Running" {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Example app status is not healthy in %s namespace", ns))
}

func createExampleApp(oc *exutil.CLI, oboBaseDir, ns string) {
	appTemplate := filepath.Join(oboBaseDir, "example-app.yaml")
	exutil.By("Install Example App")
	createResourceFromYaml(oc, ns, appTemplate)
	checkExampleAppStatus(oc, ns)
}

func rapidastScan(oc *exutil.CLI, ns, configFile string, scanPolicyFile string, apiGroupName string) (bool, error) {
	//update the token and create a new config file
	content, err := os.ReadFile(configFile)
	if err != nil {
		return false, err
	}
	defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", ns)).Execute()
	oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", ns)).Execute()
	token := getSAToken(oc, "default", ns)
	originConfig := string(content)
	targetConfig := strings.Replace(originConfig, "Bearer sha256~xxxxxxxx", "Bearer "+token, -1)
	newConfigFile := "/tmp/coodast" + getRandomString()
	f, err := os.Create(newConfigFile)
	defer f.Close()
	defer exec.Command("rm", newConfigFile).Output()
	if err != nil {
		return false, err
	}
	f.WriteString(targetConfig)

	//Create configmap
	err = oc.WithoutNamespace().Run("create").Args("-n", ns, "configmap", "rapidast-configmap", "--from-file=rapidastconfig.yaml="+newConfigFile, "--from-file=customscan.policy="+scanPolicyFile).Execute()
	if err != nil {
		return false, err
	}

	//Create job
	oboBaseDir := exutil.FixturePath("testdata", "monitoring", "observabilityoperator")
	jobTemplate := filepath.Join(oboBaseDir, "rapidast-coo-job.yaml")
	err = oc.WithoutNamespace().Run("create").Args("-n", ns, "-f", jobTemplate).Execute()
	if err != nil {
		return false, err
	}
	//Waiting up to 10 minutes until pod Failed or Success
	err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, true, func(context.Context) (done bool, err error) {
		jobStatus, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", "job-name=rapidast-coo-job", "-ojsonpath={.items[0].status.phase}").Output()
		e2e.Logf(" rapidast Job status %s ", jobStatus)
		if err1 != nil {
			return false, nil
		}
		if jobStatus == "Pending" || jobStatus == "Running" {
			return false, nil
		}
		if jobStatus == "Failed" {
			return true, fmt.Errorf("rapidast-coo-job status failed")
		}
		return jobStatus == "Succeeded", nil
	})
	//return if the pod status is not Succeeded
	if err != nil {
		return false, err
	}
	// Get the rapidast pod name
	jobPods, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=rapidast-coo-job"})
	if err != nil {
		return false, err
	}
	podLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, jobPods.Items[0].Name).Output()
	//return if failed to get logs
	if err != nil {
		return false, err
	}
	//remove token from pod logs
	podLogsNoToken := strings.Replace(podLogs, token, "xxxxxxxx", -1)
	podLogsNoBearer := strings.Replace(podLogsNoToken, "Bearer ", "bbbbbb ", -1)

	// Copy DAST Report into $ARTIFACT_DIR
	artifactAvaiable := true
	artifactdirPath := os.Getenv("ARTIFACT_DIR")
	if artifactdirPath == "" {
		artifactAvaiable = false
	}
	info, err := os.Stat(artifactdirPath)
	if err != nil {
		e2e.Logf("%s doesn't exist", artifactdirPath)
		artifactAvaiable = false
	} else if !info.IsDir() {
		e2e.Logf("%s isn't a directory", artifactdirPath)
		artifactAvaiable = false
	}

	if artifactAvaiable {
		rapidastResultsSubDir := artifactdirPath + "/rapiddastresultsCOO"
		err = os.MkdirAll(rapidastResultsSubDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		artifactFile := rapidastResultsSubDir + "/" + apiGroupName + "_rapidast.result"
		e2e.Logf("Write report into %s", artifactFile)
		f1, err := os.Create(artifactFile)
		defer f1.Close()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = f1.WriteString(podLogsNoBearer)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		// print pod logs if artifactdirPath is not writable
		e2e.Logf("#oc logs -n %s %s \n %s", jobPods.Items[0].Name, ns, podLogsNoBearer)
	}

	//return false, if high risk is reported
	podLogA := strings.Split(podLogs, "\n")
	riskHigh := 0
	riskMedium := 0
	re1 := regexp.MustCompile(`"riskdesc": .*High`)
	re2 := regexp.MustCompile(`"riskdesc": .*Medium`)
	for _, item := range podLogA {
		if re1.MatchString(item) {
			riskHigh++
		}
		if re2.MatchString(item) {
			riskMedium++
		}
	}
	e2e.Logf("rapidast result: riskHigh=%v riskMedium=%v", riskHigh, riskMedium)

	if riskHigh > 0 {
		return false, fmt.Errorf("high risk alert, please check the scan result report")
	}
	return true, nil
}

func ifMonitoringStackCRDExists(oc *exutil.CLI) bool {
	var monitoringstackCRD = "monitoringstacks.monitoring.rhobs"
	exutil.By("Check if the monitoringstack crd exists")
	_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", monitoringstackCRD).Output()
	if err != nil {
		e2e.Logf("%s doesn't exist", monitoringstackCRD)
		return false
	}
	e2e.Logf("%s exist", monitoringstackCRD)
	return true
}
