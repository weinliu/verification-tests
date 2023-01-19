package monitoring

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	"reflect"

	"github.com/tidwall/gjson"
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
	subName   = "observability-operator"
	namespace = "openshift-observability-operator"
)

func checkSubscription(oc *exutil.CLI) (out string, err error) {
	var csvName string
	g.By("Check the state of Operator")
	errCheck := wait.PollImmediate(3*time.Second, 30*time.Second, func() (bool, error) {
		out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", subName, "-n", namespace, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(out, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Subscription %v doesnot contain the correct status in namespace %v", subName, namespace))

	g.By("Get ClusterServiceVersion name")
	csvName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", subName, "-n", namespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Check that ClusterServiceVersion " + csvName + " is finished")
	errCheck = wait.PollImmediate(3*time.Second, 30*time.Second, func() (bool, error) {
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

func createOperator(oc *exutil.CLI, csTemplate string, ogTemplate string, subTemplate string, nsTemplate string) {
	g.By("Create Namespace")
	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", nsTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	g.By("Create Catalog Source")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", csTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	g.By("Create Operator Group")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", ogTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	g.By("Create subscription")
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subTemplate).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	out, err := checkSubscription(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Output: %v", out)

}
func createObservabilityOperator(oc *exutil.CLI, baseDir string) {
	csTemplate := filepath.Join(baseDir, "catalog-src.yaml")
	ogTemplate := filepath.Join(baseDir, "operator-group.yaml")
	subTemplate := filepath.Join(baseDir, "subscription.yaml")
	nsTemplate := filepath.Join(baseDir, "namespace.yaml")
	g.By("Install Observability Operator")
	createOperator(oc, csTemplate, ogTemplate, subTemplate, nsTemplate)
}
func getClusterDetails(oc *exutil.CLI) (clusterID string, region string) {
	cluserID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversions", "version", "-o=jsonpath={.spec.clusterID}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus..region}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return cluserID, region
}
func createMonitoringStack(oc *exutil.CLI, msD monitoringStackDescription, secD monitoringStackSecretDescription) {
	g.By("Creating Monitoring Stack")
	createStack(oc, msD, secD)

}
func createStack(oc *exutil.CLI, msD monitoringStackDescription, secD monitoringStackSecretDescription) {
	g.By("Creating Secret")
	secFile, err := oc.AsAdmin().Run("process").Args("-f", secD.template, "-p", "NAME="+secD.name, "NAMESPACE="+secD.namespace).OutputToFile(getRandomString() + "ms-secret.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", secFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Install Monitoring Stack")
	msFile, err := oc.AsAdmin().Run("process").Args("-f", msD.template, "-p", "CLUSTERID="+msD.clusterID, "REGION="+msD.region, "NAME="+msD.name, "NAMESPACE="+msD.namespace, "SECRETNAME="+msD.secretName, "TOKENURL="+msD.tokenURL, "URL="+msD.url).OutputToFile(getRandomString() + "ms.json")
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", msFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	out, err := checkMonitoringStack(oc, msD)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Output: %v", out)
}
func checkMonitoringStack(oc *exutil.CLI, msD monitoringStackDescription) (out string, err error) {
	g.By("Check the state of MonitoringStack")
	errCheck := wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("monitoringstack", msD.name, "-n", msD.namespace, "-o=jsonpath={.status.conditions[*].reason}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(out, "MonitoringStackAvailable") {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Monitoring Stack %v doesnot contain the correct status in namespace %v", msD.name, msD.namespace))
	out, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", msD.namespace, "-l", "app.kubernetes.io/part-of=hypershift-monitoring-stack").Output()
	return out, err
}
func checkOperatorPods(oc *exutil.CLI, msD monitoringStackDescription) {
	g.By("Check " + msD.namespace + " namespace pods liveliness")
	errCheck := wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", msD.namespace, "-l", "app.kubernetes.io/part-of=observability-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(out, "obo-prometheus-operator-") {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("%v namespace does not contain pods", msD.namespace))

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

	g.By("Check remote write config")
	errCheck := wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
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
func checkMonitoringStackDetails(oc *exutil.CLI, msD monitoringStackDescription) {
	g.By("Get clusterID and region")
	errCheck := wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("monitoringstack", msD.name, "-n", msD.namespace, "-o=jsonpath={.spec.prometheusConfig.externalLabels.hypershift_cluster_id}{.spec.prometheusConfig.externalLabels.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(out, msD.clusterID+msD.region) == 0 {
			return true, nil
		}
		return false, err
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("ClusterID and region did not match. Expected: %v %v", msD.clusterID, msD.region))
	g.By("Check status of MonitoringStack")
	errCheck = wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("monitoringstack", msD.name, "-n", msD.namespace, "-o=jsonpath={.status.conditions[*].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(out, "False") {
			return false, err
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("MonitoringStack %v reports invalid status in namespace %v", msD.name, msD.namespace))
}
func cleanResources(oc *exutil.CLI, msD monitoringStackDescription, secD monitoringStackSecretDescription) {
	g.By("Removing MonitoringStack")
	errStack := oc.AsAdmin().WithoutNamespace().Run("delete").Args("monitoringstack", msD.name, "-n", msD.namespace).Execute()
	g.By("Removing MonitoringStack Secret")
	errSecret := oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", secD.name, "-n", secD.namespace).Execute()
	o.Expect(errStack).NotTo(o.HaveOccurred())
	o.Expect(errSecret).NotTo(o.HaveOccurred())

}
