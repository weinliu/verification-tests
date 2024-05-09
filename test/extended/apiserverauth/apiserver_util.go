package apiserverauth

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/openshift-tests-private/test/extended/util"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// fixturePathCache to store fixture path mapping, key: dir name under testdata, value: fixture path
var fixturePathCache = make(map[string]string)

type admissionWebhook struct {
	name             string
	webhookname      string
	servicenamespace string
	servicename      string
	namespace        string
	apigroups        string
	apiversions      string
	operations       string
	resources        string
	version          string
	pluralname       string
	singularname     string
	kind             string
	shortname        string
	template         string
}

type service struct {
	name      string
	clusterip string
	namespace string
	template  string
}

const (
	asAdmin                   = true
	withoutNamespace          = true
	contain                   = false
	ok                        = true
	defaultRegistryServiceURL = "image-registry.openshift-image-registry.svc:5000"
)

type User struct {
	Username string
	Password string
}

// createAdmissionWebhookFromTemplate : Used for creating different admission hooks from pre-existing template.
func (admissionHook *admissionWebhook) createAdmissionWebhookFromTemplate(oc *exutil.CLI) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", admissionHook.template, "-p", "NAME="+admissionHook.name, "WEBHOOKNAME="+admissionHook.webhookname,
		"SERVICENAMESPACE="+admissionHook.servicenamespace, "SERVICENAME="+admissionHook.servicename, "NAMESPACE="+admissionHook.namespace, "APIGROUPS="+admissionHook.apigroups, "APIVERSIONS="+admissionHook.apiversions,
		"OPERATIONS="+admissionHook.operations, "RESOURCES="+admissionHook.resources, "KIND="+admissionHook.kind, "SHORTNAME="+admissionHook.shortname,
		"SINGULARNAME="+admissionHook.singularname, "PLURALNAME="+admissionHook.pluralname, "VERSION="+admissionHook.version)
}

func (service *service) createServiceFromTemplate(oc *exutil.CLI) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", service.template, "-p", "NAME="+service.name, "CLUSTERIP="+service.clusterip, "NAMESPACE="+service.namespace)
}

func compareAPIServerWebhookConditions(oc *exutil.CLI, conditionReason interface{}, conditionStatus string, conditionTypes []string) {
	for _, webHookErrorConditionType := range conditionTypes {
		// increase wait time for prow ci failures
		err := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			webhookError, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", `jsonpath='{.status.conditions[?(@.type=="`+webHookErrorConditionType+`")]}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			//Inline conditional statement for evaluating 1) reason and status together,2) only status.
			webhookConditionStatus := gjson.Get(webhookError, `status`).String()
			if containsAnyWebHookReason(webhookError, conditionReason) && webhookConditionStatus == conditionStatus {
				e2e.Logf("kube-apiserver admission webhook errors as \n %s ::: %s ::: %s ::: %s", conditionStatus, webhookError, webHookErrorConditionType, conditionReason)
				o.Expect(webhookError).Should(o.MatchRegexp(`"type":"%s"`, webHookErrorConditionType), "Mismatch in 'type' of admission errors reported")
				o.Expect(webhookError).Should(o.MatchRegexp(`"status":"%s"`, conditionStatus), "Mismatch in 'status' of admission errors reported")
				return true, nil
			}
			// Adding logging for more debug
			e2e.Logf("Retrying for expected kube-apiserver admission webhook error ::: %s ::: %s ::: %s ::: %s", conditionStatus, webhookError, webHookErrorConditionType, conditionReason)
			return false, nil
		})

		if err != nil {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ValidatingWebhookConfiguration").Output()
			e2e.Logf("#### Debug #### List all ValidatingWebhookConfiguration when the case runs into failures:%s\n", output)
			exutil.AssertWaitPollNoErr(err, "Test Fail: Expected Kube-apiserver admissionwebhook errors not present.")
		}

	}
}

// GetEncryptionPrefix :
func GetEncryptionPrefix(oc *exutil.CLI, key string) (string, error) {
	var etcdPodName string

	encryptionType, err1 := oc.WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	if encryptionType != "aesabc" && encryptionType != "aesgcm" {
		e2e.Logf("The etcd is not encrypted on!")
	}
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
		podName, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", "openshift-etcd", "-l=etcd", "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			e2e.Logf("Fail to get etcd pod, error: %s. Trying again", err)
			return false, nil
		}
		etcdPodName = podName
		return true, nil
	})
	if err != nil {
		return "", err
	}
	var encryptionPrefix string
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(cxt context.Context) (bool, error) {
		prefix, err := oc.WithoutNamespace().Run("rsh").Args("-n", "openshift-etcd", "-c", "etcd", etcdPodName, "bash", "-c", `etcdctl get `+key+` --prefix -w fields | grep -e "Value" | grep -o k8s:enc:`+encryptionType+`:v1:[^:]*: | head -n 1`).Output()
		if err != nil {
			e2e.Logf("Fail to rsh into etcd pod, error: %s. Trying again", err)
			return false, nil
		}
		encryptionPrefix = prefix
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return encryptionPrefix, nil
}

// GetEncryptionKeyNumber :
func GetEncryptionKeyNumber(oc *exutil.CLI, patten string) (int, error) {
	secretNames, err := oc.WithoutNamespace().Run("get").Args("secrets", "-n", "openshift-config-managed", `-o=jsonpath={.items[*].metadata.name}`, "--sort-by=metadata.creationTimestamp").Output()
	if err != nil {
		e2e.Logf("Fail to get secret, error: %s", err)
		return 0, nil
	}
	rePattern := regexp.MustCompile(patten)
	locs := rePattern.FindAllStringIndex(secretNames, -1)
	i, j := locs[len(locs)-1][0], locs[len(locs)-1][1]
	maxSecretName := secretNames[i:j]
	strSlice := strings.Split(maxSecretName, "-")
	var number int
	number, err = strconv.Atoi(strSlice[len(strSlice)-1])
	if err != nil {
		e2e.Logf("Fail to get secret, error: %s", err)
		return 0, nil
	}
	return number, nil
}

// WaitEncryptionKeyMigration :
func WaitEncryptionKeyMigration(oc *exutil.CLI, secret string) (bool, error) {
	var pattern string
	var waitTime time.Duration
	if strings.Contains(secret, "openshift-apiserver") {
		pattern = `migrated-resources: .*route.openshift.io.*routes`
		waitTime = 15 * time.Minute
	} else if strings.Contains(secret, "openshift-kube-apiserver") {
		pattern = `migrated-resources: .*configmaps.*secrets.*`
		waitTime = 30 * time.Minute // see below explanation
	} else {
		return false, errors.New("Unknown key " + secret)
	}

	rePattern := regexp.MustCompile(pattern)
	// In observation, the waiting time in max can take 25 mins if it is kube-apiserver,
	// and 12 mins if it is openshift-apiserver, so the Poll parameters are long.
	err := wait.PollUntilContextTimeout(context.Background(), 1*time.Minute, waitTime, false, func(cxt context.Context) (bool, error) {
		output, err := oc.WithoutNamespace().Run("get").Args("secrets", secret, "-n", "openshift-config-managed", "-o=yaml").Output()
		if err != nil {
			e2e.Logf("Fail to get the encryption key secret %s, error: %s. Trying again", secret, err)
			return false, nil
		}
		matchedStr := rePattern.FindString(output)
		if matchedStr == "" {
			e2e.Logf("Not yet see migrated-resources. Trying again")
			return false, nil
		}
		e2e.Logf("Saw all migrated-resources:\n%s", matchedStr)
		return true, nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// CheckIfResourceAvailable :
func CheckIfResourceAvailable(oc *exutil.CLI, resource string, resourceNames []string, namespace ...string) {
	args := append([]string{resource}, resourceNames...)
	if len(namespace) == 1 {
		args = append(args, "-n", namespace[0]) // HACK: implement no namespace input
	}
	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, resourceName := range resourceNames {
		o.Expect(out).Should(o.ContainSubstring(resourceName))
	}
}

func waitCoBecomes(oc *exutil.CLI, coName string, waitTime int, expectedStatus map[string]string) error {
	errCo := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		gottenStatus := getCoStatus(oc, coName, expectedStatus)
		eq := reflect.DeepEqual(expectedStatus, gottenStatus)
		if eq {
			eq := reflect.DeepEqual(expectedStatus, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
			if eq {
				// For True False False, we want to wait some bit more time and double check, to ensure it is stably healthy
				time.Sleep(100 * time.Second)
				gottenStatus := getCoStatus(oc, coName, expectedStatus)
				eq := reflect.DeepEqual(expectedStatus, gottenStatus)
				if eq {
					e2e.Logf("Given operator %s becomes available/non-progressing/non-degraded", coName)
					return true, nil
				}
			} else {
				e2e.Logf("Given operator %s becomes %s", coName, gottenStatus)
				return true, nil
			}
		}
		return false, nil
	})
	if errCo != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errCo
}

func getCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) map[string]string {
	newStatusToCompare := make(map[string]string)
	for key := range statusToCompare {
		args := fmt.Sprintf(`-o=jsonpath={.status.conditions[?(.type == '%s')].status}`, key)
		status, _ := getResource(oc, asAdmin, withoutNamespace, "co", coName, args)
		newStatusToCompare[key] = status
	}
	return newStatusToCompare
}

// Check ciphers for authentication operator cliconfig, openshiftapiservers.operator.openshift.io and kubeapiservers.operator.openshift.io:
func verifyCiphers(oc *exutil.CLI, expectedCipher string, operator string) error {
	return wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
		switch operator {
		case "openshift-authentication":
			e2e.Logf("Get the ciphers for openshift-authentication:")
			getadminoutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-authentication", "v4-0-config-system-cliconfig", "-o=jsonpath='{.data.v4-0-config-system-cliconfig}'").Output()
			if err == nil {
				// Use jqCMD to call jq because .servingInfo part JSON comming in string format
				jqCMD := fmt.Sprintf(`echo %s | jq -cr '.servingInfo | "\(.cipherSuites) \(.minTLSVersion)"'|tr -d '\n'`, getadminoutput)
				output, err := exec.Command("bash", "-c", jqCMD).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				gottenCipher := string(output)
				e2e.Logf("Comparing the ciphers: %s with %s", expectedCipher, gottenCipher)
				if expectedCipher == gottenCipher {
					e2e.Logf("Ciphers are matched: %s", gottenCipher)
					return true, nil
				}
				e2e.Logf("Ciphers are not matched: %s", gottenCipher)
				return false, nil
			}
			return false, nil

		case "openshiftapiservers.operator", "kubeapiservers.operator":
			e2e.Logf("Get the ciphers for %s:", operator)
			getadminoutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(operator, "cluster", "-o=jsonpath={.spec.observedConfig.servingInfo['cipherSuites', 'minTLSVersion']}").Output()
			if err == nil {
				e2e.Logf("Comparing the ciphers: %s with %s", expectedCipher, getadminoutput)
				if expectedCipher == getadminoutput {
					e2e.Logf("Ciphers are matched: %s", getadminoutput)
					return true, nil
				}
				e2e.Logf("Ciphers are not matched: %s", getadminoutput)
				return false, nil
			}
			return false, nil

		default:
			e2e.Logf("Operators parameters not correct..")
		}
		return false, nil
	})
}

func restoreClusterOcp41899(oc *exutil.CLI) {
	e2e.Logf("Checking openshift-controller-manager operator should be Available")
	expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	err := waitCoBecomes(oc, "openshift-controller-manager", 500, expectedStatus)
	exutil.AssertWaitPollNoErr(err, "openshift-controller-manager operator is not becomes available")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "openshift-config").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(output, "client-ca-custom") {
		configmapErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "client-ca-custom", "-n", "openshift-config").Execute()
		o.Expect(configmapErr).NotTo(o.HaveOccurred())
		e2e.Logf("Cluster configmap reset to default values")
	} else {
		e2e.Logf("Cluster configmap not changed from default values")
	}
}

func checkClusterLoad(oc *exutil.CLI, nodeType, dirname string) (int, int) {
	var tmpPath string
	var errAdm error
	errAdmNode := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
		tmpPath, errAdm = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "nodes", "-l", "node-role.kubernetes.io/"+nodeType, "--no-headers").OutputToFile(dirname)
		if errAdm != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errAdmNode, fmt.Sprintf("Not able to run adm top command :: %v", errAdm))
	cmd := fmt.Sprintf(`cat %v | grep -v 'protocol-buffers' | awk '{print $3}'|awk -F '%%' '{ sum += $1 } END { print(sum / NR) }'|cut -d "." -f1`, tmpPath)
	cpuAvg, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd = fmt.Sprintf(`cat %v | grep -v 'protocol-buffers' | awk '{print $5}'|awk -F'%%' '{ sum += $1 } END { print(sum / NR) }'|cut -d "." -f1`, tmpPath)
	memAvg, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	re, _ := regexp.Compile(`[^\w]`)
	cpuAvgs := string(cpuAvg)
	memAvgs := string(memAvg)
	cpuAvgs = re.ReplaceAllString(cpuAvgs, "")
	memAvgs = re.ReplaceAllString(memAvgs, "")
	cpuAvgVal, _ := strconv.Atoi(cpuAvgs)
	memAvgVal, _ := strconv.Atoi(memAvgs)
	return cpuAvgVal, memAvgVal
}

func checkResources(oc *exutil.CLI, dirname string) map[string]string {
	resUsedDet := make(map[string]string)
	resUsed := []string{"secrets", "deployments", "namespaces", "pods"}
	for _, key := range resUsed {
		tmpPath, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(key, "-A", "--no-headers").OutputToFile(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`cat %v | wc -l | awk '{print $1}'`, tmpPath)
		output, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		resUsedDet[key] = string(output)
	}
	return resUsedDet
}

func getTestDataFilePath(filename string) string {
	// returns the file path of the testdata files with respect to apiserverauth subteam.
	apiDirName := "apiserverauth"
	apiBaseDir := ""
	if apiBaseDir = fixturePathCache[apiDirName]; len(apiBaseDir) == 0 {
		e2e.Logf("apiserver fixture dir is not initialized, start to create")
		apiBaseDir = exutil.FixturePath("testdata", apiDirName)
		fixturePathCache[apiDirName] = apiBaseDir
		e2e.Logf("apiserver fixture dir is initialized: %s", apiBaseDir)
	} else {
		apiBaseDir = fixturePathCache[apiDirName]
		e2e.Logf("apiserver fixture dir found in cache: %s", apiBaseDir)
	}
	return filepath.Join(apiBaseDir, filename)
}

func checkCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) {
	// Check ,compare and assert the current cluster operator status against the expected status given.
	currentCoStatus := getCoStatus(oc, coName, statusToCompare)
	o.Expect(reflect.DeepEqual(currentCoStatus, statusToCompare)).To(o.Equal(true), "Wrong %s CO status reported, actual status : %s", coName, currentCoStatus)
}

func getNodePortRange(oc *exutil.CLI) (int, int) {
	// Follow the steps in https://docs.openshift.com/container-platform/4.11/networking/configuring-node-port-service-range.html
	output, err := oc.AsAdmin().Run("get").Args("configmaps", "-n", "openshift-kube-apiserver", "config", `-o=jsonpath="{.data['config\.yaml']}"`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	rgx := regexp.MustCompile(`"service-node-port-range":\["([0-9]*)-([0-9]*)"\]`)
	rs := rgx.FindSubmatch([]byte(output))
	o.Expect(rs).To(o.HaveLen(3))

	leftBound, err := strconv.Atoi(string(rs[1]))
	o.Expect(err).NotTo(o.HaveOccurred())
	rightBound, err := strconv.Atoi(string(rs[2]))
	o.Expect(err).NotTo(o.HaveOccurred())
	return leftBound, rightBound
}

// Get a random number of int32 type [m,n], n > m
func getRandomNum(m int32, n int32) int32 {
	rand.Seed(time.Now().UnixNano())
	return rand.Int31n(n-m+1) + m
}

func countResource(oc *exutil.CLI, resource string, namespace string) (int, error) {
	output, err := oc.Run("get").Args(resource, "-n", namespace, "-o", "jsonpath='{.items[*].metadata.name}'").Output()
	output = strings.Trim(strings.Trim(output, " "), "'")
	if output == "" {
		return 0, err
	}
	resources := strings.Split(output, " ")
	return len(resources), err
}

// GetAlertsByName get all the alerts
func GetAlertsByName(oc *exutil.CLI, alertName string) (string, error) {
	mon, monErr := exutil.NewPrometheusMonitor(oc.AsAdmin())
	if monErr != nil {
		return "", monErr
	}
	allAlerts, allAlertErr := mon.GetAlerts()
	if allAlertErr != nil {
		return "", allAlertErr
	}
	return allAlerts, nil
}

func isSNOCluster(oc *exutil.CLI) bool {
	//Only 1 master, 1 worker node and with the same hostname.
	masterNodes, _ := exutil.GetClusterNodesBy(oc, "master")
	workerNodes, _ := exutil.GetClusterNodesBy(oc, "worker")
	if len(masterNodes) == 1 && len(workerNodes) == 1 && masterNodes[0] == workerNodes[0] {
		return true
	}
	return false
}

// LoadCPUMemWorkload load cpu and memory workload
func LoadCPUMemWorkload(oc *exutil.CLI, workLoadtime int) {
	var (
		workerCPUtopstr    string
		workerCPUtopint    int
		workerMEMtopstr    string
		workerMEMtopint    int
		n                  int
		m                  int
		r                  int
		dn                 int
		cpuMetric          = 800
		memMetric          = 700
		reserveCPUP        = 50
		reserveMemP        = 50
		snoPodCapacity     = 250
		reservePodCapacity = 120
	)

	workerCPUtopall := []int{}
	workerMEMtopall := []int{}

	randomStr := exutil.GetRandomString()
	dirname := fmt.Sprintf("/tmp/-load-cpu-mem_%s/", randomStr)
	defer os.RemoveAll(dirname)
	os.MkdirAll(dirname, 0755)

	workerNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/master", "--no-headers").OutputToFile("load-cpu-mem_" + randomStr + "-log")
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf(`cat %v |head -1 | awk '{print $1}'`, workerNode)
	cmdOut, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	worker1 := strings.Replace(string(cmdOut), "\n", "", 1)
	// Check if there is an node.metrics on node
	err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodemetrics", worker1).Execute()
	var workerTop string
	if err == nil {
		workerTop, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", worker1, "--no-headers=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	cpuUsageCmd := fmt.Sprintf(`echo "%v" | awk '{print $2}'`, workerTop)
	cpuUsage, err := exec.Command("bash", "-c", cpuUsageCmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cpu1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cpuUsage), "")
	cpu, _ := strconv.Atoi(cpu1)
	cpuUsageCmdP := fmt.Sprintf(`echo "%v" | awk '{print $3}'`, workerTop)
	cpuUsageP, err := exec.Command("bash", "-c", cpuUsageCmdP).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cpuP1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cpuUsageP), "")
	cpuP, _ := strconv.Atoi(cpuP1)
	totalCPU := int(float64(cpu) / (float64(cpuP) / 100))
	cmd = fmt.Sprintf(`cat %v | awk '{print $1}'`, workerNode)
	workerCPU1, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	workerCPU := strings.Fields(string(workerCPU1))
	workerNodeCount := len(workerCPU)
	o.Expect(err).NotTo(o.HaveOccurred())

	for i := 0; i < len(workerCPU); i++ {
		// Check if there is node.metrics on node
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodemetrics", workerCPU[i]).Execute()
		var workerCPUtop string
		if err == nil {
			workerCPUtop, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", workerCPU[i], "--no-headers=true").OutputToFile("load-cpu-mem_" + randomStr + "-log")
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		workerCPUtopcmd := fmt.Sprintf(`cat %v | awk '{print $3}'`, workerCPUtop)
		workerCPUUsage, err := exec.Command("bash", "-c", workerCPUtopcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		workerCPUtopstr = regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(workerCPUUsage), "")
		workerCPUtopint, _ = strconv.Atoi(workerCPUtopstr)
		workerCPUtopall = append(workerCPUtopall, workerCPUtopint)
	}
	for j := 1; j < len(workerCPU); j++ {
		if workerCPUtopall[0] < workerCPUtopall[j] {
			workerCPUtopall[0] = workerCPUtopall[j]
		}
	}
	cpuMax := workerCPUtopall[0]
	availableCPU := int(float64(totalCPU) * (100 - float64(reserveCPUP) - float64(cpuMax)) / 100)
	e2e.Logf("----> Cluster has total CPU, Reserved CPU percentage, Max CPU of node :%v,%v,%v", totalCPU, reserveCPUP, cpuMax)
	n = int(availableCPU / int(cpuMetric))
	if n <= 0 {
		e2e.Logf("No more CPU resource is available, no load will be added!")
	} else {
		if workerNodeCount == 1 {
			dn = 1
			r = 2
		} else {
			dn = 2
			if n > workerNodeCount {
				r = 3
			} else {
				r = workerNodeCount
			}
		}
		// Get the available pods of worker nodes, based on this, the upper limit for a namespace is calculated
		cmd1 := fmt.Sprintf(`oc describe node/%s | grep 'Non-terminated Pods' | grep -oP "[0-9]+"`, worker1)
		cmdOut1, err := exec.Command("bash", "-c", cmd1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		usedPods, err := strconv.Atoi(regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cmdOut1), ""))
		o.Expect(err).NotTo(o.HaveOccurred())
		availablePods := snoPodCapacity - usedPods - reservePodCapacity
		if workerNodeCount > 1 {
			availablePods = availablePods * workerNodeCount
		}
		nsMax := int(availablePods / dn / r)
		if nsMax > 0 {
			if n > nsMax {
				n = nsMax
			}
		} else {
			n = 1
			r = 1
			dn = 1
		}
		e2e.Logf("Start CPU load ...")
		cpuloadCmd := fmt.Sprintf(`clusterbuster --basename=cpuload --workload=cpusoaker --namespaces=%v --processes=1 --deployments=%v --node-selector=node-role.kubernetes.io/master --tolerate=node-role.kubernetes.io/master:Equal:NoSchedule --workloadruntime=7200 --report=none > %v &`, n, dn, dirname+"clusterbuster-cpu-log")
		e2e.Logf("%v", cpuloadCmd)
		cmd := exec.Command("bash", "-c", cpuloadCmd)
		cmdErr := cmd.Start()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		// Wait for 3 mins(this time is based on many tests), when the load starts, it will reach a peak within a few minutes, then falls back.
		time.Sleep(180 * time.Second)
		e2e.Logf("----> Created cpuload related pods: %v", n*r*dn)
	}

	memUsageCmd := fmt.Sprintf(`echo "%v" | awk '{print $4}'`, workerTop)
	memUsage, err := exec.Command("bash", "-c", memUsageCmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	mem1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(memUsage), "")
	mem, _ := strconv.Atoi(mem1)
	memUsageCmdP := fmt.Sprintf(`echo "%v" | awk '{print $5}'`, workerTop)
	memUsageP, err := exec.Command("bash", "-c", memUsageCmdP).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	memP1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(memUsageP), "")
	memP, _ := strconv.Atoi(memP1)
	totalMem := int(float64(mem) / (float64(memP) / 100))

	for i := 0; i < len(workerCPU); i++ {
		// Check if there is node.metrics on node
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodemetrics", workerCPU[i]).Execute()
		var workerMEMtop string
		if err == nil {
			workerMEMtop, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", workerCPU[i], "--no-headers=true").OutputToFile("load-cpu-mem_" + randomStr + "-log")
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		workerMEMtopcmd := fmt.Sprintf(`cat %v | awk '{print $5}'`, workerMEMtop)
		workerMEMUsage, err := exec.Command("bash", "-c", workerMEMtopcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		workerMEMtopstr = regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(workerMEMUsage), "")
		workerMEMtopint, _ = strconv.Atoi(workerMEMtopstr)
		workerMEMtopall = append(workerMEMtopall, workerMEMtopint)
	}
	for j := 1; j < len(workerCPU); j++ {
		if workerMEMtopall[0] < workerMEMtopall[j] {
			workerMEMtopall[0] = workerMEMtopall[j]
		}
	}
	memMax := workerMEMtopall[0]
	availableMem := int(float64(totalMem) * (100 - float64(reserveMemP) - float64(memMax)) / 100)
	m = int(availableMem / int(memMetric))
	e2e.Logf("----> Cluster has total Mem, Reserved Mem percentage, Max memory of node :%v,%v,%v", totalMem, reserveMemP, memMax)
	if m <= 0 {
		e2e.Logf("No more memory resource is available, no load will be added!")
	} else {
		if workerNodeCount == 1 {
			dn = 1
			r = 2
		} else {
			r = workerNodeCount
			if m > workerNodeCount {
				dn = m
			} else {
				dn = workerNodeCount
			}
		}
		// Get the available pods of worker nodes, based on this, the upper limit for a namespace is calculated
		cmd1 := fmt.Sprintf(`oc describe node/%v | grep 'Non-terminated Pods' | grep -oP "[0-9]+"`, worker1)
		cmdOut1, err := exec.Command("bash", "-c", cmd1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		usedPods, err := strconv.Atoi(regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cmdOut1), ""))
		o.Expect(err).NotTo(o.HaveOccurred())
		availablePods := snoPodCapacity - usedPods - reservePodCapacity
		if workerNodeCount > 1 {
			availablePods = availablePods * workerNodeCount
			// Reduce the number pods in which workers create memory loads concurrently, avoid kubelet crash
			if availablePods > 200 {
				availablePods = int(availablePods / 2)
			}
		}
		nsMax := int(availablePods / dn / r)
		if nsMax > 0 {
			if m > nsMax {
				m = nsMax
			}
		} else {
			m = 1
			r = 1
			dn = 1
		}
		e2e.Logf("Start Memory load ...")
		memloadCmd := fmt.Sprintf(`clusterbuster --basename=memload --workload=memory --namespaces=%v --processes=1 --deployments=%v --node-selector=node-role.kubernetes.io/master --tolerate=node-role.kubernetes.io/master:Equal:NoSchedule --workloadruntime=7200 --report=none> %v &`, m, dn, dirname+"clusterbuster-mem-log")
		e2e.Logf("%v", memloadCmd)
		cmd := exec.Command("bash", "-c", memloadCmd)
		cmdErr := cmd.Start()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		// Wait for 5 mins, ensure that all load pods are strated up.
		time.Sleep(300 * time.Second)
		e2e.Logf("----> Created memload related pods: %v", m*r*dn)
	}
	// If load are landed, will do some checking with logs
	if n > 0 || m > 0 {
		keywords := "body: net/http: request canceled (Client.Timeout|panic"
		bustercmd := fmt.Sprintf(`cat %v | grep -iE '%s' || true`, dirname+"clusterbuster*", keywords)
		busterLogs, err := exec.Command("bash", "-c", bustercmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(busterLogs) > 0 {
			e2e.Logf("%s", busterLogs)
			e2e.Logf("Found some panic or timeout errors, if errors are  potential bug then file a bug.")
		} else {
			e2e.Logf("No errors found in clusterbuster logs")
		}
	} else {
		e2e.Logf("No more CPU and memory resource, no any load is added.")
	}
}

// CopyToFile copy a given file into a temp folder with given file name
func CopyToFile(fromPath string, toFilename string) string {
	// check if source file is regular file
	srcFileStat, err := os.Stat(fromPath)
	if err != nil {
		e2e.Failf("get source file %s stat failed: %v", fromPath, err)
	}
	if !srcFileStat.Mode().IsRegular() {
		e2e.Failf("source file %s is not a regular file", fromPath)
	}

	// open source file
	source, err := os.Open(fromPath)
	if err != nil {
		e2e.Failf("open source file %s failed: %v", fromPath, err)
	}
	defer source.Close()

	// open dest file
	saveTo := filepath.Join(e2e.TestContext.OutputDir, toFilename)
	dest, err := os.Create(saveTo)
	if err != nil {
		e2e.Failf("open destination file %s failed: %v", saveTo, err)
	}
	defer dest.Close()

	// copy from source to dest
	_, err = io.Copy(dest, source)
	if err != nil {
		e2e.Failf("copy file from %s to %s failed: %v", fromPath, saveTo, err)
	}
	return saveTo
}

func ExecCommandOnPod(oc *exutil.CLI, podname string, namespace string, command string) string {
	var podOutput string
	var execpodErr error
	errExec := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
		podOutput, execpodErr = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, podname, "--", "/bin/sh", "-c", command).Output()
		podOutput = strings.TrimSpace(podOutput)
		if execpodErr != nil {
			return false, nil
		} else if podOutput != "" {
			return true, nil
		} else {
			return false, nil
		}
	})
	exutil.AssertWaitPollNoErr(errExec, fmt.Sprintf("Not able to run command on pod %v :: %v :: %v :: %v", podname, command, podOutput, execpodErr))
	return podOutput
}

// clusterHealthcheck do cluster health check like pod, node and operators
func clusterHealthcheck(oc *exutil.CLI, dirname string) error {
	err := clusterNodesHealthcheck(oc, 600, dirname)
	if err != nil {
		return fmt.Errorf("Cluster nodes health check failed. Abnormality found in nodes.")
	}
	err = clusterOperatorHealthcheck(oc, 1500, dirname)
	if err != nil {
		return fmt.Errorf("Cluster operators health check failed. Abnormality found in cluster operators.")
	}
	err = clusterPodsHealthcheck(oc, 600, dirname)
	if err != nil {
		return fmt.Errorf("Cluster pods health check failed. Abnormality found in pods.")
	}
	return nil
}

// clusterOperatorHealthcheck check abnormal operators
func clusterOperatorHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal operators")
	errCo := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		coLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "--no-headers").OutputToFile(dirname)
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -v '.True.*False.*False' || true`, coLogFile)
			coLogs, err := exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(coLogs) > 0 {
				return false, nil
			}
		} else {
			return false, nil
		}
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("No abnormality found in cluster operators...")
		return true, nil
	})
	if errCo != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errCo
}

// clusterPodsHealthcheck check abnormal pods.
func clusterPodsHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal pods")
	var podLogs []byte
	errPod := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		podLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-A").OutputToFile(dirname)
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -ivE 'Running|Completed|namespace|installer' || true`, podLogFile)
			podLogs, err = exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(podLogs) > 0 {
				return false, nil
			}
		} else {
			return false, nil
		}
		e2e.Logf("No abnormality found in pods...")
		return true, nil
	})
	if errPod != nil {
		e2e.Logf("%s", podLogs)
	}
	return errPod
}

// clusterNodesHealthcheck check abnormal nodes
func clusterNodesHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	errNode := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
		if err == nil {
			if strings.Contains(output, "NotReady") || strings.Contains(output, "SchedulingDisabled") {
				return false, nil
			}
		} else {
			return false, nil
		}
		e2e.Logf("Nodes are normal...")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		return true, nil
	})
	if errNode != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errNode
}

// apiserverReadinessProbe use for microshift to check apiserver readiness
func apiserverReadinessProbe(tokenValue string, apiserverName string) string {
	timeoutDuration := 3 * time.Second
	var bodyString string
	url := fmt.Sprintf(`%s/apis`, apiserverName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		e2e.Failf("error creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenValue)
	req.Header.Set("X-OpenShift-Internal-If-Not-Ready", "reject")

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeoutDuration,
	}
	errCurl := wait.PollImmediate(1*time.Second, 300*time.Second, func() (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			e2e.Logf("Error while making curl request :: %v", err)
			return false, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode == 429 {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			bodyString = string(bodyBytes)
			return strings.Contains(bodyString, "The apiserver hasn't been fully initialized yet, please try again later"), nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCurl, fmt.Sprintf("error waiting for API server readiness: %v", errCurl))
	return bodyString
}

// Get one available service IP, retry 3 times
func getServiceIP(oc *exutil.CLI, clusterIP string) net.IP {
	var serviceIP net.IP
	err := wait.PollUntilContextTimeout(context.Background(), 1*time.Second, 3*time.Second, false, func(cxt context.Context) (bool, error) {
		randomServiceIP := net.ParseIP(clusterIP).To4()
		if randomServiceIP != nil {
			randomServiceIP[3] += byte(rand.Intn(254 - 1))
		} else {
			randomServiceIP = net.ParseIP(clusterIP).To16()
			randomServiceIP[len(randomServiceIP)-1] = byte(rand.Intn(254 - 1))
		}
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-A", `-o=jsonpath={.items[*].spec.clusterIP}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(randomServiceIP.String(), output); matched {
			e2e.Logf("IP %v has been used!", randomServiceIP)
			return false, nil
		}
		serviceIP = randomServiceIP
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Failed to get one available service IP!")
	return serviceIP
}

// the method is to do something with oc.
func doAction(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, parameters ...string) (string, error) {
	if asAdmin && withoutNamespace {
		return oc.AsAdmin().WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if asAdmin && !withoutNamespace {
		return oc.AsAdmin().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && withoutNamespace {
		return oc.WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && !withoutNamespace {
		return oc.Run(action).Args(parameters...).Output()
	}
	return "", nil
}

// Get something existing resource
func getResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) (string, error) {
	return doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
}

// Get something resource to be ready
func getResourceToBeReady(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) string {
	var result string
	var err error
	errPoll := wait.PollUntilContextTimeout(context.Background(), 6*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
		result, err = doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil || len(result) == 0 {
			e2e.Logf("Unable to retrieve the expected resource, retrying...")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errPoll, fmt.Sprintf("Failed to retrieve %v", parameters))
	e2e.Logf("The resource returned:\n%v", result)
	return result
}

func getGlobalProxy(oc *exutil.CLI) (string, string, string) {
	httpProxy, err := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpProxy}")
	o.Expect(err).NotTo(o.HaveOccurred())
	httpsProxy, err := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpsProxy}")
	o.Expect(err).NotTo(o.HaveOccurred())
	noProxy, err := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.noProxy}")
	o.Expect(err).NotTo(o.HaveOccurred())
	return httpProxy, httpsProxy, noProxy
}

// Get the pods List by label
func getPodsListByLabel(oc *exutil.CLI, namespace string, selectorLabel string) []string {
	podsOp := getResourceToBeReady(oc, asAdmin, withoutNamespace, "pod", "-n", namespace, "-l", selectorLabel, "-o=jsonpath={.items[*].metadata.name}")
	o.Expect(podsOp).NotTo(o.BeEmpty())
	return strings.Split(podsOp, " ")
}

func checkApiserversAuditPolicies(oc *exutil.CLI, auditPolicyName string) {
	e2e.Logf("Checking the current " + auditPolicyName + " audit policy of cluster")
	defaultProfile := getResourceToBeReady(oc, asAdmin, withoutNamespace, "apiserver/cluster", `-o=jsonpath={.spec.audit.profile}`)
	o.Expect(defaultProfile).Should(o.ContainSubstring(auditPolicyName), "current audit policy of cluster is not default :: "+defaultProfile)

	e2e.Logf("Checking the audit config file of kube-apiserver currently in use.")
	podsList := getPodsListByLabel(oc.AsAdmin(), "openshift-kube-apiserver", "app=openshift-kube-apiserver")
	execKasOuptut := ExecCommandOnPod(oc, podsList[0], "openshift-kube-apiserver", "ls /etc/kubernetes/static-pod-resources/configmaps/kube-apiserver-audit-policies/")
	re := regexp.MustCompile(`policy.yaml`)
	matches := re.FindAllString(execKasOuptut, -1)
	if len(matches) == 0 {
		e2e.Failf("Audit config file of kube-apiserver is wrong :: %s", execKasOuptut)
	}
	e2e.Logf("Audit config file of kube-apiserver :: %s", execKasOuptut)

	e2e.Logf("Checking the audit config file of openshif-apiserver currently in use.")
	podsList = getPodsListByLabel(oc.AsAdmin(), "openshift-apiserver", "app=openshift-apiserver-a")
	execOasOuptut := ExecCommandOnPod(oc, podsList[0], "openshift-apiserver", "cat /var/run/configmaps/config/config.yaml")
	re = regexp.MustCompile(`/var/run/configmaps/audit/policy.yaml`)
	matches = re.FindAllString(execOasOuptut, -1)
	if len(matches) == 0 {
		e2e.Failf("Audit config file of openshift-apiserver is wrong :: %s", execOasOuptut)
	}
	e2e.Logf("Audit config file of openshift-apiserver :: %v", matches)

	e2e.Logf("Checking the audit config file of openshif-oauth-apiserver currently in use.")
	podsList = getPodsListByLabel(oc.AsAdmin(), "openshift-oauth-apiserver", "app=openshift-oauth-apiserver")
	execAuthOuptut := ExecCommandOnPod(oc, podsList[0], "openshift-oauth-apiserver", "ls /var/run/configmaps/audit/")
	re = regexp.MustCompile(`policy.yaml`)
	matches = re.FindAllString(execAuthOuptut, -1)
	if len(matches) == 0 {
		e2e.Failf("Audit config file of openshift-oauth-apiserver is wrong :: %s", execAuthOuptut)
	}
	e2e.Logf("Audit config file of openshift-oauth-apiserver :: %v", execAuthOuptut)
}

func checkAuditLogs(oc *exutil.CLI, script string, masterNode string, namespace string) (string, int) {
	g.By(fmt.Sprintf("Get audit log file from %s", masterNode))
	masterNodeLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", script)
	o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())
	errCount := len(strings.TrimSpace(masterNodeLogs))
	return masterNodeLogs, errCount
}

func setAuditProfile(oc *exutil.CLI, patchNamespace string, patch string) string {
	expectedProgCoStatus := map[string]string{"Progressing": "True"}
	expectedCoStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	coOps := []string{"authentication", "openshift-apiserver"}
	patchOutput, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchNamespace, "--type=json", "-p", patch).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(patchOutput, "patched") {
		e2e.Logf("Checking KAS, OAS, Auththentication operators should be in Progressing and Available after audit profile change")
		g.By("Checking kube-apiserver operator should be in Progressing in 100 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 100, expectedProgCoStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedCoStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")
		// Using 60s because KAS takes long time, when KAS finished rotation, OAS and Auth should have already finished.
		for _, ops := range coOps {
			e2e.Logf("Checking %s should be Available in 60 seconds", ops)
			err = waitCoBecomes(oc, ops, 60, expectedCoStatus)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%v operator is not becomes available in 60 seconds", ops))
		}
		e2e.Logf("Post audit profile set. KAS, OAS and Auth operator are available after rollout")
		return patchOutput
	}
	return patchOutput
}

func getNewUser(oc *exutil.CLI, count int) ([]User, string, string) {
	usersDirPath := "/tmp/" + exutil.GetRandomString()
	usersHTpassFile := usersDirPath + "/htpasswd"
	err := os.MkdirAll(usersDirPath, 0o755)
	o.Expect(err).NotTo(o.HaveOccurred())

	htPassSecret, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("oauth/cluster", "-o", "jsonpath={.spec.identityProviders[0].htpasswd.fileData.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if htPassSecret == "" {
		htPassSecret = "htpass-secret"
		os.Create(usersHTpassFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-config", "secret", "generic", htPassSecret, "--from-file", "htpasswd="+usersHTpassFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("--type=json", "-p", `[{"op": "add", "path": "/spec/identityProviders", "value": [{"htpasswd": {"fileData": {"name": "htpass-secret"}}, "mappingMethod": "claim", "name": "htpasswd", "type": "HTPasswd"}]}]`, "oauth/cluster").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", "openshift-config", "secret/"+htPassSecret, "--to", usersDirPath, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	users := make([]User, count)

	for i := 0; i < count; i++ {
		// Generate new username and password
		users[i].Username = fmt.Sprintf("testuser-%v-%v", i, exutil.GetRandomString())
		users[i].Password = exutil.GetRandomString()

		// Add new user to htpasswd file in the temp directory
		cmd := fmt.Sprintf("htpasswd -b %v %v %v", usersHTpassFile, users[i].Username, users[i].Password)
		err := exec.Command("bash", "-c", cmd).Run()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	// Update htpass-secret with the modified htpasswd file
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("-n", "openshift-config", "data", "secret/"+htPassSecret, "--from-file", "htpasswd="+usersHTpassFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Checking authentication operator should be in Progressing in 180 seconds")
	err = waitCoBecomes(oc, "authentication", 180, map[string]string{"Progressing": "True"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not start progressing in 180 seconds")
	e2e.Logf("Checking authentication operator should be Available in 600 seconds")
	err = waitCoBecomes(oc, "authentication", 600, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 600 seconds")

	return users, usersHTpassFile, htPassSecret
}

func userCleanup(oc *exutil.CLI, users []User, usersHTpassFile string, htPassSecret string) {
	defer os.RemoveAll(usersHTpassFile)
	for _, user := range users {
		// Add new user to htpasswd file in the temp directory
		cmd := fmt.Sprintf("htpasswd -D %v %v", usersHTpassFile, user.Username)
		err := exec.Command("bash", "-c", cmd).Run()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	// Update htpass-secret with the modified htpasswd file
	err := oc.AsAdmin().WithoutNamespace().Run("set").Args("-n", "openshift-config", "data", "secret/"+htPassSecret, "--from-file", "htpasswd="+usersHTpassFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Checking authentication operator should be in Progressing in 180 seconds")
	err = waitCoBecomes(oc, "authentication", 180, map[string]string{"Progressing": "True"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not start progressing in 180 seconds")
	e2e.Logf("Checking authentication operator should be Available in 600 seconds")
	err = waitCoBecomes(oc, "authentication", 600, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 600 seconds")
}

func isConnectedInternet(oc *exutil.CLI) bool {
	masterNode, masterErr := exutil.GetFirstMasterNode(oc)
	o.Expect(masterErr).NotTo(o.HaveOccurred())

	cmd := `timeout 9 curl -k https://github.com/openshift/ruby-hello-world/ > /dev/null;[ $? -eq 0 ] && echo "connected"`
	output, _ := exutil.DebugNodeWithChroot(oc, masterNode, "bash", "-c", cmd)
	if matched, _ := regexp.MatchString("connected", output); !matched {
		// Failed to access to the internet in the cluster.
		return false
	}
	return true
}

func restartMicroshift(oc *exutil.CLI, nodename string) {
	_, restartErr := runSSHCommand(nodename, "redhat", "sudo systemctl restart microshift")
	o.Expect(restartErr).NotTo(o.HaveOccurred())
	mstatusErr := wait.PollUntilContextTimeout(context.Background(), 6*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
		output, err := runSSHCommand(nodename, "redhat", "sudo systemctl is-active microshift")
		if err == nil && strings.TrimSpace(output) == "active" {
			e2e.Logf("microshift status is: %v ", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(mstatusErr, fmt.Sprintf("Failed to restart Microshift: %v", mstatusErr))
}

func replacePatternInfile(oc *exutil.CLI, microshiftFilePathYaml string, oldPattern string, newPattern string) {
	content, err := ioutil.ReadFile(microshiftFilePathYaml)
	o.Expect(err).NotTo(o.HaveOccurred())

	re := regexp.MustCompile(oldPattern)
	newContent := re.ReplaceAll(content, []byte(newPattern))

	err = ioutil.WriteFile(microshiftFilePathYaml, newContent, 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Get the pods List by label
func getPodsList(oc *exutil.CLI, namespace string) []string {
	podsOp := getResourceToBeReady(oc, asAdmin, withoutNamespace, "pod", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}")
	podNames := strings.Split(strings.TrimSpace(podsOp), " ")
	e2e.Logf("Namespace %s pods are: %s", namespace, string(podsOp))
	return podNames
}

func changeMicroshiftConfig(oc *exutil.CLI, configStr string, nodeName string, namespace string, configPath string) {
	etcConfigCMD := fmt.Sprintf(`'
configfile=%v
cat > $configfile << EOF
%v
EOF'`, configPath, configStr)
	_, mchgConfigErr := runSSHCommand(nodeName, "redhat", "sudo bash -c", etcConfigCMD)
	o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
}

func addKustomizationToMicroshift(oc *exutil.CLI, nodeName string, namespace string, kustomizationFiles map[string][]string) {
	for key, file := range kustomizationFiles {
		tmpFileName := getTestDataFilePath(file[0])
		replacePatternInfile(oc, tmpFileName, file[2], file[3])
		fileOutput, err := exec.Command("bash", "-c", fmt.Sprintf(`cat %s`, tmpFileName)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		destFile := file[1] + strings.Split(key, ".")[0] + ".yaml"
		fileCmd := fmt.Sprintf(`'cat > %s << EOF
%s
EOF'`, destFile, string(fileOutput))
		_, mchgConfigErr := runSSHCommand(nodeName, "redhat", "sudo bash -c", fileCmd)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
	}
}

// Check ciphers of configmap of kube-apiservers, openshift-apiservers and oauth-openshift-apiservers are using.
func verifyHypershiftCiphers(oc *exutil.CLI, expectedCipher string, ns string) error {
	var (
		cipherStr string
		randomStr = exutil.GetRandomString()
		tmpDir    = fmt.Sprintf("/tmp/-api-%s/", randomStr)
	)

	defer os.RemoveAll(tmpDir)
	os.MkdirAll(tmpDir, 0755)

	for _, item := range []string{"kube-apiserver", "openshift-apiserver", "oauth-openshift"} {
		e2e.Logf("#### Checking the ciphers of  %s:", item)
		if item == "kube-apiserver" {
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", ns, "kas-config", `-o=jsonpath='{.data.config\.json}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Use jq command line to extrack .servingInfo part JSON comming in string format
			jqCmd := fmt.Sprintf(`echo %s | jq -cr '.servingInfo | "\(.cipherSuites) \(.minTLSVersion)"'|tr -d '\n'`, out)
			outJQ, err := exec.Command("bash", "-c", jqCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cipherStr = string(outJQ)
		} else {
			jsonOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", ns, item, `-ojson`).OutputToFile("api-" + randomStr + "." + item)
			o.Expect(err).NotTo(o.HaveOccurred())
			jqCmd := fmt.Sprintf(`cat %v | jq -r '.data."config.yaml"'`, jsonOut)
			yamlConfig, err := exec.Command("bash", "-c", jqCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			jsonConfig, errJson := util.Yaml2Json(string(yamlConfig))
			o.Expect(errJson).NotTo(o.HaveOccurred())

			jsonFile := tmpDir + item + "config.json"
			f, err := os.Create(jsonFile)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer f.Close()
			w := bufio.NewWriter(f)
			_, err = fmt.Fprintf(w, "%s", jsonConfig)
			w.Flush()
			o.Expect(err).NotTo(o.HaveOccurred())

			jqCmd1 := fmt.Sprintf(`jq -cr '.servingInfo | "\(.cipherSuites) \(.minTLSVersion)"' %s |tr -d '\n'`, jsonFile)
			jsonOut1, err := exec.Command("bash", "-c", jqCmd1).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cipherStr = string(jsonOut1)
		}
		e2e.Logf("#### Checking if the ciphers has been changed as the expected: %s", expectedCipher)
		if expectedCipher != cipherStr {
			e2e.Logf("#### Ciphers of %s are: %s", item, cipherStr)
			return fmt.Errorf("Ciphers not matched")
		}
		e2e.Logf("#### Ciphers are matched.")
	}
	return nil
}

// Waiting for apiservers restart
func waitApiserverRestartOfHypershift(oc *exutil.CLI, appLabel string, ns string, waitTime int) error {
	re, err := regexp.Compile(`(0/[0-9]|Pending|Terminating|Init)`)
	o.Expect(err).NotTo(o.HaveOccurred())
	errKas := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		out, _ := getResource(oc, asAdmin, withoutNamespace, "pods", "-l", "app="+appLabel, "--no-headers", "-n", ns)
		if matched := re.MatchString(out); matched {
			e2e.Logf("#### %s was restarting ...", appLabel)
			return false, nil
		}
		// Recheck status of pods and to do further confirm , avoid false restarts
		for i := 1; i <= 3; i++ {
			time.Sleep(10 * time.Second)
			out, _ = getResource(oc, asAdmin, withoutNamespace, "pods", "-l", "app="+appLabel, "--no-headers", "-n", ns)
			if matchedAgain := re.MatchString(out); matchedAgain {
				e2e.Logf("#### %s was restarting ...", appLabel)
				return false, nil
			}
		}
		e2e.Logf("#### %s have been restarted!", appLabel)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errKas, "Failed to complete the restart within the expected time, please check the cluster status!")
	return errKas
}

func containsAnyWebHookReason(webhookError string, conditionReasons interface{}) bool {
	switch reasons := conditionReasons.(type) {
	case string:
		return strings.Contains(webhookError, reasons)
	case []string:
		for _, reason := range reasons {
			if strings.Contains(webhookError, reason) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func clientCurl(tokenValue string, url string) string {
	timeoutDuration := 3 * time.Second
	var bodyString string

	proxyURL := getProxyURL()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		e2e.Failf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenValue)
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeoutDuration,
	}

	errCurl := wait.PollImmediate(10*time.Second, 300*time.Second, func() (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			bodyString = string(bodyBytes)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCurl, fmt.Sprintf("error waiting for curl request output: %v", errCurl))
	return bodyString
}

// parse base domain from dns config. format is like $clustername.$basedomain
func getBaseDomain(oc *exutil.CLI) string {
	str, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns/cluster", `-ojsonpath={.spec.baseDomain}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return str
}

// Return  the API server FQDN. format is like api.$clustername.$basedomain
func getApiServerFQDN(oc *exutil.CLI) string {
	return fmt.Sprintf("api.%s", getBaseDomain(oc))
}

// isTechPreviewNoUpgrade checks if a cluster is a TechPreviewNoUpgrade cluster
func isTechPreviewNoUpgrade(oc *exutil.CLI) bool {
	featureGate, err := oc.AdminConfigClient().ConfigV1().FeatureGates().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		e2e.Failf("could not retrieve feature-gate: %v", err)
	}

	return featureGate.Spec.FeatureSet == configv1.TechPreviewNoUpgrade
}

// IsIPv4 check if the string is an IPv4 address.
func isIPv4(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ".")
}

// IsIPv6 check if the string is an IPv6 address.
func isIPv6(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ":")
}

// Copy one public image to the internel image registry of OCP cluster
func copyImageToInternelRegistry(oc *exutil.CLI, namespace string, source string, dest string) (string, error) {
	var (
		podName string
		appName = "skopeo"
		err     error
	)

	podName, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", "name="+appName, "-o", `jsonpath={.items[*].metadata.name}`).Output()
	// If the skopeo pod doesn't exist, create it
	if len(podName) == 0 {
		template := getTestDataFilePath("skopeo-deployment.json")
		err = oc.Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podName = getPodsListByLabel(oc.AsAdmin(), namespace, "name="+appName)[0]
		exutil.AssertPodToBeReady(oc, podName, namespace)
	} else {
		output, err := oc.AsAdmin().Run("get").Args("pod", podName, "-n", namespace, "-o", "jsonpath='{.status.conditions[?(@.type==\"Ready\")].status}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("True"), appName+" pod is not ready!")
	}

	token, err := getSAToken(oc, "builder", namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(token).NotTo(o.BeEmpty())

	command := []string{podName, "-n", namespace, "--", appName, "--insecure-policy", "--src-tls-verify=false", "--dest-tls-verify=false", "copy", "--dcreds", "dnm:" + token, source, dest}
	results, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(command...).Output()
	return results, err
}

// Check if BaselineCapabilities have been set
func isBaselineCapsSet(oc *exutil.CLI) bool {
	baselineCapabilitySet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.spec.capabilities.baselineCapabilitySet}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("baselineCapabilitySet parameters: %v\n", baselineCapabilitySet)
	return len(baselineCapabilitySet) != 0
}

// Check if component is listed in clusterversion.status.capabilities.enabledCapabilities
func isEnabledCapability(oc *exutil.CLI, component string) bool {
	enabledCapabilities, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].status.capabilities.enabledCapabilities}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Cluster enabled capability parameters: %v\n", enabledCapabilities)
	return strings.Contains(enabledCapabilities, component)
}

func checkURLEndpointAccess(oc *exutil.CLI, hostIP, nodePort, podName, portCommand, status string) {
	var url string
	var curlOutput string
	var curlErr error

	if isIPv6(hostIP) {
		url = fmt.Sprintf("[%s]:%s", hostIP, nodePort)
	} else {
		url = fmt.Sprintf("%s:%s", hostIP, nodePort)
	}

	// Construct the full command with the specified command and URL
	var fullCommand string
	if portCommand == "https" {
		fullCommand = fmt.Sprintf("curl -k https://%s", url)
	} else {
		fullCommand = fmt.Sprintf("curl %s", url)
	}

	e2e.Logf("Command: %v", fullCommand)
	e2e.Logf("Checking if the specified URL endpoint %s  is accessible", url)

	err := wait.PollUntilContextTimeout(context.Background(), 2*time.Second, 6*time.Second, false, func(cxt context.Context) (bool, error) {
		curlOutput, curlErr = oc.Run("exec").Args(podName, "-i", "--", "sh", "-c", fullCommand).Output()
		if curlErr != nil {
			return false, nil
		}
		return true, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Unable to access %s", url))
	o.Expect(curlOutput).To(o.ContainSubstring(status))
}

type CertificateDetails struct {
	CurlResponse   string
	Subject        string
	Issuer         string
	NotBefore      string
	NotAfter       string
	SubjectAltName []string
	SerialNumber   string
}

func urlHealthCheck(fqdnName string, certPath string, returnValues []string) (*CertificateDetails, error) {
	caCert, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("Error reading CA certificate: %v", err)
	}

	// Create a CertPool and add the CA certificate
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Create a custom transport with the CA certificate
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caCertPool,
		},
	}

	client := &http.Client{
		Transport: transport,
	}

	url := fmt.Sprintf("https://%s:6443/healthz", fqdnName)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Error performing HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response body: %v", err)
	}

	// Create a CertificateDetails struct to store the details
	certDetails := &CertificateDetails{}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		for _, value := range returnValues {
			switch value {
			case "CurlResponse":
				certDetails.CurlResponse = string(body)
			case "Subject":
				certDetails.Subject = cert.Subject.String()
			case "Issuer":
				certDetails.Issuer = cert.Issuer.String()
			case "NotBefore":
				certDetails.NotBefore = cert.NotBefore.Format(time.RFC3339)
			case "NotAfter":
				certDetails.NotAfter = cert.NotAfter.Format(time.RFC3339)
			case "SubjectAltName":
				certDetails.SubjectAltName = cert.DNSNames
			case "SerialNumber":
				certDetails.SerialNumber = cert.SerialNumber.String()
			}
		}
	}
	return certDetails, nil
}

func runSSHCommand(server, user string, commands ...string) (string, error) {
	// Combine commands into a single string
	fullCommand := strings.Join(commands, " ")
	sshkey, err := exutil.GetPrivateKey()
	o.Expect(err).NotTo(o.HaveOccurred())

	sshClient := exutil.SshClient{User: user, Host: server, Port: 22, PrivateKey: sshkey}
	return sshClient.RunOutput(fullCommand)
}

func getProxyURL() *url.URL {
	// Prefer https_proxy, fallback to http_proxy
	proxyURLString := os.Getenv("https_proxy")
	if proxyURLString == "" {
		proxyURLString = os.Getenv("http_proxy")
	}
	if proxyURLString == "" {
		return nil
	}
	proxyURL, err := url.Parse(proxyURLString)
	if err != nil {
		e2e.Failf("error parsing proxy URL: %v", err)
	}
	return proxyURL
}

func getMicroshiftHostname(oc *exutil.CLI) string {
	microShiftURL, err := oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "-ojsonpath={.clusters[0].cluster.server}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	fqdnName, err := url.Parse(microShiftURL)
	o.Expect(err).NotTo(o.HaveOccurred())
	return fqdnName.Hostname()
}

func applyLabel(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) {
	_, err := doAction(oc, "label", asAdmin, withoutNamespace, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred(), "Adding label to the namespace failed")
}

// Function to get audit event logs for user login.
func checkUserAuditLog(oc *exutil.CLI, logGroup string, user string, pass string) (string, int) {
	var (
		eventLogs  string
		eventCount = 0
		n          int
		now        = time.Now().UTC().Unix()
	)

	errUser := oc.AsAdmin().WithoutNamespace().Run("login").Args("-u", user, "-p", pass).NotShowInfo().Execute()
	o.Expect(errUser).NotTo(o.HaveOccurred())

	script := fmt.Sprintf(`rm -if /tmp/audit-test-*.json;
	for logpath in kube-apiserver oauth-apiserver openshift-apiserver;do
	  grep -h "%s" /var/log/${logpath}/audit*.log | jq -c 'select (.requestReceivedTimestamp | .[0:19] + "Z" | fromdateiso8601 > %v)' >> /tmp/audit-test-$logpath.json;
	done;
	cat /tmp/audit-test-*.json`, logGroup, now)
	contextErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", "admin").Execute()
	o.Expect(contextErr).NotTo(o.HaveOccurred())

	e2e.Logf("Get all master nodes.")
	masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
	o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
	o.Expect(masterNodes).NotTo(o.BeEmpty())
	for _, masterNode := range masterNodes {
		eventLogs, n = checkAuditLogs(oc, script, masterNode, "openshift-kube-apiserver")
		e2e.Logf("event logs count:%v", n)
		eventCount += n
	}
	return eventLogs, eventCount
}
