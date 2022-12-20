package apiserverauth

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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

func compareAPIServerWebhookConditions(oc *exutil.CLI, conditionReason string, conditionStatus string, conditionTypes []string) {
	for _, webHookErrorConditionType := range conditionTypes {
		err := wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
			webhookError, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", `jsonpath='{.status.conditions[?(@.type=="`+webHookErrorConditionType+`")]}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(webhookError).Should(o.MatchRegexp(`"type":"%s"`, webHookErrorConditionType), "Mismatch in 'type' of admission errors reported")
			//Inline conditional statement for evaluating 1) reason and status together,2) only status.
			if conditionReason != "" && strings.Contains(webhookError, conditionReason) {
				e2e.Logf("kube-apiserver admission webhook errors as \n %s ", string(webhookError))
				o.Expect(webhookError).Should(o.MatchRegexp(`"status":"%s"`, conditionStatus), "Mismatch in 'status' of admission errors reported")
				o.Expect(webhookError).Should(o.MatchRegexp(`"reason":"%s"`, conditionReason), "Mismatch in 'reason' of admission errors reported")
				return true, nil
			} else if conditionReason == "" && strings.Contains(webhookError, conditionStatus) {
				o.Expect(webhookError).Should(o.MatchRegexp(`"status":"%s"`, conditionStatus), "Mismatch in 'status' of admission errors reported")
				e2e.Logf("kube-apiserver admission webhook errors as \n %s ", string(webhookError))
				return true, nil
			}
			e2e.Logf("Retrying for expected kube-apiserver admission webhook error")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Test Fail: Expected Kube-apiserver admissionwebhook errors not present.")
	}
}

// GetEncryptionPrefix :
func GetEncryptionPrefix(oc *exutil.CLI, key string) (string, error) {
	var etcdPodName string
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
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
	err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		prefix, err := oc.WithoutNamespace().Run("rsh").Args("-n", "openshift-etcd", "-c", "etcd", etcdPodName, "bash", "-c", `etcdctl get `+key+` --prefix -w fields | grep -e "Value" | grep -o k8s:enc:aescbc:v1:[^:]*: | head -n 1`).Output()
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
		pattern = `migrated-resources: .*oauthaccesstokens.*oauthauthorizetokens.*routes`
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
	err := wait.Poll(1*time.Minute, waitTime, func() (bool, error) {
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
	return wait.Poll(5*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
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
}

func getCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) map[string]string {
	newStatusToCompare := make(map[string]string)
	for key := range statusToCompare {
		args := fmt.Sprintf(`-o=jsonpath={.status.conditions[?(.type == '%s')].status}`, key)
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", args, coName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newStatusToCompare[key] = status
	}
	return newStatusToCompare
}

// Check ciphers for authentication operator cliconfig, openshiftapiservers.operator.openshift.io and kubeapiservers.operator.openshift.io:
func verifyCiphers(oc *exutil.CLI, expectedCipher string, operator string) error {
	return wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		switch operator {
		case "openshift-authentication":
			e2e.Logf("Get the cipers for openshift-authentication:")
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
			e2e.Logf("Get the cipers for %s:", operator)
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
	err := waitCoBecomes(oc, "openshift-controller-manager", 300, expectedStatus)
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
	tmpPath, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "nodes", "-l", "node-role.kubernetes.io/"+nodeType, "--no-headers").OutputToFile(dirname)
	o.Expect(err).NotTo(o.HaveOccurred())
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

func isTargetPortAvailable(oc *exutil.CLI, port int) bool {
	masterNodes, err := exutil.GetClusterNodesBy(oc, "master")
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, masterNode := range masterNodes {
		cmd := fmt.Sprintf("netstat -tulpn | grep LISTEN | { grep :%d || true; }", port)
		checkPortResult, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=openshift-kube-apiserver"}, "bash", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		checkPortResult = strings.Trim(strings.Trim(checkPortResult, "\n"), " ")
		if checkPortResult != "" {
			return false
		}
	}
	return true
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
func LoadCPUMemWorkload(oc *exutil.CLI) {
	var (
		workerCPUtopstr    string
		workerCPUtopint    int
		workerMEMtopstr    string
		workerMEMtopint    int
		n                  int
		m                  int
		c                  int
		r                  int
		dn                 int
		s                  int
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

	workerNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker", "--no-headers").OutputToFile("load-cpu-mem_" + randomStr + "-log")
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf(`cat %v |head -1 | awk '{print $1}'`, workerNode)
	cmdOut, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	worker1 := strings.Replace(string(cmdOut), "\n", "", 1)
	workerTop, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", worker1, "--no-headers=true").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
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
		workerCPUtop, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", workerCPU[i], "--no-headers=true").OutputToFile("load-cpu-mem_" + randomStr + "-log")
		o.Expect(err).NotTo(o.HaveOccurred())
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
		p := workerNodeCount
		if workerNodeCount == 1 {
			dn = 1
			r = 2
			c = 3
		} else {
			dn = 2
			c = 3
			if n > workerNodeCount {
				r = 3
			} else {
				r = workerNodeCount
			}
		}
		s = int(500 / n / dn)
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
			c = 3
			s = 10
		}
		e2e.Logf("Start CPU load ...")
		cpuloadCmd := fmt.Sprintf(`clusterbuster -N %v -B cpuload -P server -b 5 -r %v -p %v -d %v -c %v -s %v -W -m 1000 -D .2 -M 1 -t 36000 -x -v > %v`, n, r, p, dn, c, s, dirname+"clusterbuster-cpu-log")
		e2e.Logf("%v", cpuloadCmd)
		_, err = exec.Command("bash", "-c", cpuloadCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
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
		workerMEMtop, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", workerCPU[i], "--no-headers=true").OutputToFile("load-cpu-mem_" + randomStr + "-log")
		o.Expect(err).NotTo(o.HaveOccurred())
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
		p := workerNodeCount
		if workerNodeCount == 1 {
			dn = 1
			r = 2
			c = 6
		} else {
			r = workerNodeCount
			if m > workerNodeCount {
				dn = m
			} else {
				dn = workerNodeCount
			}
			c = 3
		}
		s = int(500 / m / dn)
		// Get the available pods of worker nodes, based on this, the upper limit for a namespace is calculated
		cmd1 := fmt.Sprintf(`oc describe node/%v | grep 'Non-terminated Pods' | grep -oP "[0-9]+"`, worker1)
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
			if m > nsMax {
				m = nsMax
			}
		} else {
			m = 1
			r = 1
			dn = 1
			c = 3
			s = 10
		}
		e2e.Logf("Start Memory load ...")
		memloadCmd := fmt.Sprintf(`clusterbuster -N %v -B memload -P server -r %v -p %v -d %v -c %v -s %v -W -x -v > %v`, m, r, p, dn, c, s, dirname+"clusterbuster-mem-log")
		e2e.Logf("%v", memloadCmd)
		_, err = exec.Command("bash", "-c", memloadCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
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
