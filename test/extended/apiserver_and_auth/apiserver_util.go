package apiserver_and_auth
import (
	"time"
	"regexp"
	"strconv"
	"strings"
	"errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"os/exec"
	"fmt"
	"reflect"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

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
		prefix, err := oc.WithoutNamespace().Run("rsh").Args("-n", "openshift-etcd", "-c", "etcd", etcdPodName, "bash", "-c", `etcdctl get ` + key + ` --prefix -w fields | grep -e "Value" | grep -o k8s:enc:aescbc:v1:[^:]*: | head -n 1`).Output()
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

func GetEncryptionKeyNumber(oc *exutil.CLI, patten string) (int, error) {
	secretNames, err := oc.WithoutNamespace().Run("get").Args("secrets", "-n", "openshift-config-managed", `-o=jsonpath={.items[*].metadata.name}`, "--sort-by=metadata.creationTimestamp").Output()
	if err != nil {
		e2e.Logf("Fail to get secret, error: %s", err)
		return 0, nil
	}
	rePattern := regexp.MustCompile(patten)
	locs := rePattern.FindAllStringIndex(secretNames, -1)
	i, j := locs[len(locs) - 1][0], locs[len(locs) - 1][1]
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

func WaitEncryptionKeyMigration (oc *exutil.CLI, secret string) (bool, error) {
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

		if matchedStr := rePattern.FindString(output); matchedStr == "" {
			e2e.Logf("Not yet see migrated-resources. Trying again")
			return false, nil
		} else {
			e2e.Logf("Saw all migrated-resources:\n%s", matchedStr)
			return true, nil
		}
	})
	if err != nil {
		return false, err
	} else {
		return true, nil
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
func verify_ciphers(oc *exutil.CLI, expectedCipher string, operator string) error {
	return wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		switch operator {
			case "openshift-authentication":
				e2e.Logf("Get the cipers for openshift-authentication:")
				getadminoutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-authentication", "v4-0-config-system-cliconfig", "-o=jsonpath='{.data.v4-0-config-system-cliconfig}'").Output()
				if err == nil {
					// Use jqCMD to call jq because .servingInfo part JSON comming in string format
					jqCMD := fmt.Sprintf(`echo %s | jq -cr '.servingInfo | "\(.cipherSuites) \(.minTLSVersion)"'|tr -d '\n'`, getadminoutput)
					output,err := exec.Command("bash", "-c", jqCMD).Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					gottenCipher := string(output)
					e2e.Logf("Comparing the ciphers: %s with %s", expectedCipher, gottenCipher)
					if expectedCipher == gottenCipher {
						e2e.Logf("Ciphers are matched: %s", gottenCipher )
						return true, nil
					} else {
						e2e.Logf("Ciphers are not matched: %s", gottenCipher)
						return false, nil
					}
				}
				return false, nil

			case "openshiftapiservers.operator" , "kubeapiservers.operator":
				e2e.Logf("Get the cipers for %s:", operator)
				getadminoutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(operator, "cluster", "-o=jsonpath={.spec.observedConfig.servingInfo['cipherSuites', 'minTLSVersion']}").Output()
				if err == nil {
					e2e.Logf("Comparing the ciphers: %s with %s", expectedCipher, getadminoutput)
					if expectedCipher == getadminoutput {
						e2e.Logf("Ciphers are matched: %s", getadminoutput)
						return true, nil
					} else {
						e2e.Logf("Ciphers are not matched: %s", getadminoutput)
						return false, nil
					}
				}
				return false, nil

			default:
				e2e.Logf("Operators parameters not correct..")
			}
		return false, nil
	})
}

func restore_cluster_ocp_41899(oc *exutil.CLI) {
	e2e.Logf("Checking openshift-controller-manager operator should be Available")
	expected_status := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	err := waitCoBecomes(oc, "openshift-controller-manager", 300, expected_status)
	exutil.AssertWaitPollNoErr(err, "openshift-controller-manager operator is not becomes available")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "openshift-config").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(output, "client-ca-custom") {
		configmap_err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "client-ca-custom", "-n", "openshift-config").Execute()
		o.Expect(configmap_err).NotTo(o.HaveOccurred())
		e2e.Logf("Cluster configmap reset to default values")
	} else {
		e2e.Logf("Cluster configmap not changed from default values")
	}
}

func check_cluster_load(oc *exutil.CLI, node_type, dirname string) (int, int) {
	tmp_path, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "nodes", "-l", "node-role.kubernetes.io/"+node_type, "--no-headers").OutputToFile(dirname)
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf(`cat %v | awk '{print $3}'|awk -F '%%' '{ sum += $1 } END { print(sum / NR) }'|cut -d "." -f1`, tmp_path)
	cpu_avg, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd = fmt.Sprintf(`cat %v | awk '{print $5}'|awk -F'%%' '{ sum += $1 } END { print(sum / NR) }'|cut -d "." -f1`, tmp_path)
	mem_avg, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	re, _ := regexp.Compile(`[^\w]`)
	cpu_avgs := string(cpu_avg)
	mem_avgs := string(mem_avg)
	cpu_avgs = re.ReplaceAllString(cpu_avgs, "")
	mem_avgs = re.ReplaceAllString(mem_avgs, "")
	cpu_avg_val, _ := strconv.Atoi(cpu_avgs)
	mem_avg_val, _ := strconv.Atoi(mem_avgs)
	return cpu_avg_val, mem_avg_val
}

func check_resources(oc *exutil.CLI, dirname string) map[string]string {
	res_used_det := make(map[string]string)
	res_used := []string{"secrets", "deployments", "namespaces", "pods"}
	for _, key := range res_used {
		tmp_path, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(key, "-A", "--no-headers").OutputToFile(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`cat %v | wc -l | awk '{print $1}'`, tmp_path)
		output, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		res_used_det[key] = string(output)
	}
	return res_used_det
}
