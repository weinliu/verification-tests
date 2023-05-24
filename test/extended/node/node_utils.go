package node

import (
	"fmt"
	"math/rand"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

type podWkloadCpuNoAnotation struct {
	name        string
	namespace   string
	workloadcpu string
	template    string
}

type podWkloadCpuDescription struct {
	name        string
	namespace   string
	workloadcpu string
	template    string
}

type podNoWkloadCpuDescription struct {
	name      string
	namespace string
	template  string
}

type podHelloDescription struct {
	name      string
	namespace string
	template  string
}

type podModifyDescription struct {
	name          string
	namespace     string
	mountpath     string
	command       string
	args          string
	restartPolicy string
	user          string
	role          string
	level         string
	template      string
}

type podLivenessProbe struct {
	name                  string
	namespace             string
	overridelivenessgrace string
	terminationgrace      int
	failurethreshold      int
	periodseconds         int
	template              string
}

type kubeletCfgMaxpods struct {
	name       string
	labelkey   string
	labelvalue string
	maxpods    int
	template   string
}

type ctrcfgDescription struct {
	namespace  string
	pidlimit   int
	loglevel   string
	overlay    string
	logsizemax string
	command    string
	configFile string
	template   string
}

type objectTableRefcscope struct {
	kind string
	name string
}

type podTerminationDescription struct {
	name      string
	namespace string
	template  string
}

type podInitConDescription struct {
	name      string
	namespace string
	template  string
}

type podUserNSDescription struct {
	name      string
	namespace string
	template  string
}

type podSleepDescription struct {
	namespace string
	template  string
}

type kubeletConfigDescription struct {
	name       string
	labelkey   string
	labelvalue string
	template   string
}

type memHogDescription struct {
	name       string
	namespace  string
	labelkey   string
	labelvalue string
	template   string
}

type podTwoContainersDescription struct {
	name      string
	namespace string
	template  string
}

type ctrcfgOverlayDescription struct {
	name     string
	overlay  string
	template string
}

func (podNoWkloadCpu *podNoWkloadCpuDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podNoWkloadCpu.template, "-p", "NAME="+podNoWkloadCpu.name, "NAMESPACE="+podNoWkloadCpu.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podNoWkloadCpu *podNoWkloadCpuDescription) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podNoWkloadCpu.namespace, "pod", podNoWkloadCpu.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

type runtimeTimeoutDescription struct {
	name       string
	labelkey   string
	labelvalue string
	template   string
}

type upgradeMachineconfig1Description struct {
	name     string
	template string
}

type upgradeMachineconfig2Description struct {
	name     string
	template string
}

func (podWkloadCpu *podWkloadCpuDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podWkloadCpu.template, "-p", "NAME="+podWkloadCpu.name, "NAMESPACE="+podWkloadCpu.namespace, "WORKLOADCPU="+podWkloadCpu.workloadcpu)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podWkloadCpu *podWkloadCpuDescription) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podWkloadCpu.namespace, "pod", podWkloadCpu.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podWkloadCpuNoAnota *podWkloadCpuNoAnotation) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podWkloadCpuNoAnota.template, "-p", "NAME="+podWkloadCpuNoAnota.name, "NAMESPACE="+podWkloadCpuNoAnota.namespace, "WORKLOADCPU="+podWkloadCpuNoAnota.workloadcpu)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podWkloadCpuNoAnota *podWkloadCpuNoAnotation) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podWkloadCpuNoAnota.namespace, "pod", podWkloadCpuNoAnota.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podHello *podHelloDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podHello.template, "-p", "NAME="+podHello.name, "NAMESPACE="+podHello.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podHello *podHelloDescription) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podHello.namespace, "pod", podHello.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (ctrcfg *ctrcfgOverlayDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ctrcfg.template, "-p", "NAME="+ctrcfg.name, "OVERLAY="+ctrcfg.overlay)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podUserNS *podUserNSDescription) createPodUserNS(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podUserNS.template, "-p", "NAME="+podUserNS.name, "NAMESPACE="+podUserNS.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podUserNS *podUserNSDescription) deletePodUserNS(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podUserNS.namespace, "pod", podUserNS.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (kubeletConfig *kubeletConfigDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", kubeletConfig.template, "-p", "NAME="+kubeletConfig.name, "LABELKEY="+kubeletConfig.labelkey, "LABELVALUE="+kubeletConfig.labelvalue)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (memHog *memHogDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", memHog.template, "-p", "NAME="+memHog.name, "LABELKEY="+memHog.labelkey, "LABELVALUE="+memHog.labelvalue)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podSleep *podSleepDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podSleep.template, "-p", "NAMESPACE="+podSleep.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (runtimeTimeout *runtimeTimeoutDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", runtimeTimeout.template, "-p", "NAME="+runtimeTimeout.name, "LABELKEY="+runtimeTimeout.labelkey, "LABELVALUE="+runtimeTimeout.labelvalue)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (runtimeTimeout *runtimeTimeoutDescription) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("kubeletconfig", runtimeTimeout.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (upgradeMachineconfig1 *upgradeMachineconfig1Description) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", upgradeMachineconfig1.template, "-p", "NAME="+upgradeMachineconfig1.name)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (upgradeMachineconfig1 *upgradeMachineconfig1Description) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("kubeletconfig", upgradeMachineconfig1.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (upgradeMachineconfig2 *upgradeMachineconfig2Description) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", upgradeMachineconfig2.template, "-p", "NAME="+upgradeMachineconfig2.name)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (upgradeMachineconfig2 *upgradeMachineconfig2Description) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("kubeletconfig", upgradeMachineconfig2.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete Namespace with all resources
func (podSleep *podSleepDescription) deleteProject(oc *exutil.CLI) error {
	e2e.Logf("Deleting Project ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", podSleep.namespace).Execute()
}

func (podInitCon *podInitConDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podInitCon.template, "-p", "NAME="+podInitCon.name, "NAMESPACE="+podInitCon.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podInitCon *podInitConDescription) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podInitCon.namespace, "pod", podInitCon.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

func (kubeletcfg *kubeletCfgMaxpods) createKubeletConfigMaxpods(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", kubeletcfg.template, "-p", "NAME="+kubeletcfg.name, "LABELKEY="+kubeletcfg.labelkey, "LABELVALUE="+kubeletcfg.labelvalue, "MAXPODS="+strconv.Itoa(kubeletcfg.maxpods))
	if err != nil {
		e2e.Logf("the err of createKubeletConfigMaxpods:%v", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (kubeletcfg *kubeletCfgMaxpods) deleteKubeletConfigMaxpods(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("kubeletconfig", kubeletcfg.name).Execute()
	if err != nil {
		e2e.Logf("the err of deleteKubeletConfigMaxpods:%v", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pod *podLivenessProbe) createPodLivenessProbe(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "OVERRIDELIVENESSGRACE="+pod.overridelivenessgrace, "TERMINATIONGRACE="+strconv.Itoa(pod.terminationgrace), "FAILURETHRESHOLD="+strconv.Itoa(pod.failurethreshold), "PERIODSECONDS="+strconv.Itoa(pod.periodseconds))
	if err != nil {
		e2e.Logf("the err of createPodLivenessProbe:%v", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pod *podLivenessProbe) deletePodLivenessProbe(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", pod.namespace, "pod", pod.name).Execute()
	if err != nil {
		e2e.Logf("the err of deletePodLivenessProbe:%v", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podModify *podModifyDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podModify.template, "-p", "NAME="+podModify.name, "NAMESPACE="+podModify.namespace, "MOUNTPATH="+podModify.mountpath, "COMMAND="+podModify.command, "ARGS="+podModify.args, "POLICY="+podModify.restartPolicy, "USER="+podModify.user, "ROLE="+podModify.role, "LEVEL="+podModify.level)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podModify *podModifyDescription) delete(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podModify.namespace, "pod", podModify.name).Execute()
}

func (podTermination *podTerminationDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podTermination.template, "-p", "NAME="+podTermination.name, "NAMESPACE="+podTermination.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (podTermination *podTerminationDescription) delete(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podTermination.namespace, "pod", podTermination.name).Execute()
}

func createResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var jsonCfg string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "node-config.json")
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		jsonCfg = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("The resource is %s", jsonCfg)
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", jsonCfg).Execute()
}

func podStatusReason(oc *exutil.CLI) error {
	e2e.Logf("check if pod is available")
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[*].status.initContainerStatuses[*].state.waiting.reason}", "-n", oc.Namespace()).Output()
		e2e.Logf("the status of pod is %v", status)
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		if strings.Contains(status, "CrashLoopBackOff") {
			e2e.Logf(" Pod failed status reason is :%s", status)
			return true, nil
		}
		return false, nil
	})
}

func podStatusterminatedReason(oc *exutil.CLI) error {
	e2e.Logf("check if pod is available")
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[*].status.initContainerStatuses[*].state.terminated.reason}", "-n", oc.Namespace()).Output()
		e2e.Logf("the status of pod is %v", status)
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		if strings.Contains(status, "Error") {
			e2e.Logf(" Pod failed status reason is :%s", status)
			return true, nil
		}
		return false, nil
	})
}

func podStatus(oc *exutil.CLI) error {
	e2e.Logf("check if pod is available")
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[*].status.phase}", "-n", oc.Namespace()).Output()
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		if strings.Contains(status, "Running") && !strings.Contains(status, "Pending") {
			e2e.Logf("Pod status is : %s", status)
			return true, nil
		}
		return false, nil
	})
}

func podEvent(oc *exutil.CLI, timeout int, keyword string) error {
	return wait.Poll(10*time.Second, time.Duration(timeout)*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", oc.Namespace()).Output()
		if err != nil {
			e2e.Logf("Can't get events from test project, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString(keyword, output); matched {
			e2e.Logf(keyword)
			return true, nil
		}
		return false, nil
	})
}

func kubeletNotPromptDupErr(oc *exutil.CLI, keyword string, name string) error {
	return wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		re := regexp.MustCompile(keyword)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeletconfig", name, "-o=jsonpath={.status.conditions[*]}").Output()
		if err != nil {
			e2e.Logf("Can't get kubeletconfig status, error: %s. Trying again", err)
			return false, nil
		}
		found := re.FindAllString(output, -1)
		if lenStr := len(found); lenStr > 1 {
			e2e.Logf("[%s] appear %d times.", keyword, lenStr)
			return false, nil
		} else if lenStr == 1 {
			e2e.Logf("[%s] appear %d times.\nkubeletconfig not prompt duplicate error message", keyword, lenStr)
			return true, nil
		} else {
			e2e.Logf("error: kubelet not prompt [%s]", keyword)
			return false, nil
		}
	})
}

func volStatus(oc *exutil.CLI) error {
	e2e.Logf("check content of volume")
	return wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("init-volume", "-c", "hello-pod", "cat", "/init-test/volume-test", "-n", oc.Namespace()).Output()
		e2e.Logf("The content of the vol is %v", status)
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		if strings.Contains(status, "This is OCP volume test") {
			e2e.Logf(" Init containers with volume work fine \n")
			return true, nil
		}
		return false, nil
	})
}

// ContainerSccStatus get scc status of container
func ContainerSccStatus(oc *exutil.CLI) error {
	return wait.Poll(1*time.Second, 1*time.Second, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "hello-pod", "-o=jsonpath={.spec.securityContext.seLinuxOptions.*}", "-n", oc.Namespace()).Output()
		e2e.Logf("The Container SCC Content is %v", status)
		if err != nil {
			e2e.Failf("the result of ReadFile:%v", err)
			return false, nil
		}
		if strings.Contains(status, "unconfined_u unconfined_r s0:c25,c968") {
			e2e.Logf("SeLinuxOptions in pod applied to container Sucessfully \n")
			return true, nil
		}
		return false, nil
	})
}

func (ctrcfg *ctrcfgDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ctrcfg.template, "-p", "LOGLEVEL="+ctrcfg.loglevel, "OVERLAY="+ctrcfg.overlay, "LOGSIZEMAX="+ctrcfg.logsizemax)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (ctrcfg *ctrcfgDescription) checkCtrcfgParameters(oc *exutil.CLI) error {
	return wait.Poll(10*time.Minute, 11*time.Minute, func() (bool, error) {
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		for _, v := range node {
			nodeStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", fmt.Sprintf("%s", v), "-o=jsonpath={.status.conditions[3].type}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("\nNode %s Status is %s\n", v, nodeStatus)

			if nodeStatus == "Ready" {
				criostatus, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args(`node/`+fmt.Sprintf("%s", v), "--", "chroot", "/host", "crio", "config").OutputToFile("crio.conf")
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf(`\nCRI-O PARAMETER ON THE WORKER NODE :` + fmt.Sprintf("%s", v))
				e2e.Logf("\ncrio config file path is  %v", criostatus)

				wait.Poll(2*time.Second, 1*time.Minute, func() (bool, error) {
					result, err1 := exec.Command("bash", "-c", "cat "+criostatus+" | egrep 'pids_limit|log_level'").Output()
					if err != nil {
						e2e.Failf("the result of ReadFile:%v", err1)
						return false, nil
					}
					e2e.Logf("\nCtrcfg Parameters is %s", result)
					if strings.Contains(string(result), "debug") && strings.Contains(string(result), "2048") {
						e2e.Logf("\nCtrcfg parameter pod limit and log_level configured successfully")
						return true, nil
					}
					return false, nil
				})
			} else {
				e2e.Logf("\n NODES ARE NOT READY\n ")
			}
		}
		return true, nil
	})
}

func (podTermination *podTerminationDescription) getTerminationGrace(oc *exutil.CLI) error {
	e2e.Logf("check terminationGracePeriodSeconds period")
	return wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
		nodename, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].spec.nodeName}", "-n", podTermination.namespace).Output()
		e2e.Logf("The nodename is %v", nodename)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeReadyBool, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", fmt.Sprintf("%s", nodename), "-o=jsonpath={.status.conditions[?(@.reason=='KubeletReady')].status}").Output()
		e2e.Logf("The Node Ready status is %v", nodeReadyBool)
		o.Expect(err).NotTo(o.HaveOccurred())
		containerID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].status.containerStatuses[0].containerID}", "-n", podTermination.namespace).Output()
		e2e.Logf("The containerID is %v", containerID)
		o.Expect(err).NotTo(o.HaveOccurred())
		if nodeReadyBool == "True" {
			terminationGrace, err := exutil.DebugNodeWithChroot(oc, nodename, "systemctl", "show", containerID)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(string(terminationGrace), "TimeoutStopUSec=1min 30s") {
				e2e.Logf("\nTERMINATION GRACE PERIOD IS SET CORRECTLY")
				return true, nil
			}
			e2e.Logf("\ntermination grace is NOT Updated")
			return false, nil
		}
		return false, nil
	})
}

func (podInitCon *podInitConDescription) containerExit(oc *exutil.CLI) error {
	return wait.Poll(2*time.Second, 2*time.Minute, func() (bool, error) {
		initConStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].status.initContainerStatuses[0].state.terminated.reason}", "-n", podInitCon.namespace).Output()
		e2e.Logf("The initContainer status is %v", initConStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(string(initConStatus), "Completed") {
			e2e.Logf("The initContainer exit normally")
			return true, nil
		}
		e2e.Logf("The initContainer not exit!")
		return false, nil
	})
}

func (podInitCon *podInitConDescription) deleteInitContainer(oc *exutil.CLI) (string, error) {
	nodename, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].spec.nodeName}", "-n", podInitCon.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	containerID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].status.initContainerStatuses[0].containerID}", "-n", podInitCon.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The containerID is %v", containerID)
	initContainerID := string(containerID)[8:]
	e2e.Logf("The initContainerID is %s", initContainerID)
	return exutil.DebugNodeWithChroot(oc, fmt.Sprintf("%s", nodename), "crictl", "rm", initContainerID)
}

func (podInitCon *podInitConDescription) initContainerNotRestart(oc *exutil.CLI) error {
	return wait.Poll(3*time.Minute, 6*time.Minute, func() (bool, error) {
		re := regexp.MustCompile("running")
		podname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", podInitCon.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(string(podname), "-n", podInitCon.namespace, "--", "cat", "/mnt/data/test").Output()
		e2e.Logf("The /mnt/data/test: %s", output)
		o.Expect(err).NotTo(o.HaveOccurred())
		found := re.FindAllString(output, -1)
		if lenStr := len(found); lenStr > 1 {
			e2e.Logf("initContainer restart %d times.", (lenStr - 1))
			return false, nil
		} else if lenStr == 1 {
			e2e.Logf("initContainer not restart")
			return true, nil
		}
		return false, nil
	})
}

func checkNodeStatus(oc *exutil.CLI, workerNodeName string) error {
	return wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
		nodeStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", workerNodeName, "-o=jsonpath={.status.conditions[3].type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Status is %s\n", nodeStatus)
		if nodeStatus == "Ready" {
			e2e.Logf("\n WORKER NODE IS READY\n ")
		} else {
			e2e.Logf("\n WORKERNODE IS NOT READY\n ")
			return false, nil
		}
		return true, nil
	})
}

func getSingleWorkerNode(oc *exutil.CLI) string {
	workerNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nWorker Node Name is %v", workerNodeName)
	return workerNodeName
}

func getSingleMasterNode(oc *exutil.CLI) string {
	masterNodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/master=", "-o=jsonpath={.items[1].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nMaster Node Name is %v", masterNodeName)
	return masterNodeName
}

func getPodNodeName(oc *exutil.CLI, namespace string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].spec.nodeName}", "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Pod Node Name is %v \n", nodeName)
	return nodeName
}

func getPodNetNs(oc *exutil.CLI, hostname string) (string, error) {
	NetNsStr, err := exutil.DebugNodeWithChroot(oc, hostname, "/bin/bash", "-c", "journalctl -u crio --since=\"5 minutes ago\" | grep pod-56266 | grep NetNS")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("NetNs string : %v", NetNsStr)
	keyword := "NetNS:[^\\s]*"
	re := regexp.MustCompile(keyword)
	found := re.FindAllString(NetNsStr, -1)
	if len(found) == 0 {
		e2e.Logf("can not find NetNS for pod")
		return "", fmt.Errorf("can not find NetNS for pod")
	}
	e2e.Logf("found : %v \n", found[0])
	NetNs := strings.Split(found[0], ":")
	e2e.Logf("NetNs : %v \n", NetNs[1])
	return NetNs[1], nil
}

func addLabelToNode(oc *exutil.CLI, label string, workerNodeName string, resource string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args(resource, workerNodeName, label, "--overwrite").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nLabel Added")
}

func removeLabelFromNode(oc *exutil.CLI, label string, workerNodeName string, resource string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args(resource, workerNodeName, label).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nLabel Removed")
}

func rebootNode(oc *exutil.CLI, workerNodeName string) error {
	return wait.Poll(1*time.Second, 1*time.Second, func() (bool, error) {
		e2e.Logf("\nRebooting....")
		_, err1 := oc.AsAdmin().WithoutNamespace().Run("debug").Args(`node/`+workerNodeName, "--", "chroot", "/host", "reboot").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		return true, nil
	})
}

func masterNodeLog(oc *exutil.CLI, masterNode string) error {
	return wait.Poll(1*time.Second, 1*time.Second, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args(`node/`+masterNode, "--", "chroot", "/host", "journalctl", "-u", "crio").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(status, "layer not known") {
			e2e.Logf("\nTest successfully executed")
		} else {
			e2e.Logf("\nTest fail executed, and try next")
			return false, nil
		}
		return true, nil
	})
}

func getmcpStatus(oc *exutil.CLI, nodeName string) error {
	return wait.Poll(60*time.Second, 15*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeName, "-ojsonpath={.status.conditions[?(@.type=='Updating')].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nCurrent mcp UPDATING Status is %s\n", status)
		if strings.Contains(status, "False") {
			e2e.Logf("\nmcp updated successfully ")
		} else {
			e2e.Logf("\nmcp is still in UPDATING state")
			return false, nil
		}
		return true, nil
	})
}

func getWorkerNodeDescribe(oc *exutil.CLI, workerNodeName string) error {
	return wait.Poll(3*time.Second, 1*time.Minute, func() (bool, error) {
		nodeStatus, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("node", workerNodeName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(nodeStatus, "EvictionThresholdMet") {
			e2e.Logf("\n WORKER NODE MET EVICTION THRESHOLD\n ")
		} else {
			e2e.Logf("\n WORKER NODE DO NOT HAVE MEMORY PRESSURE\n ")
			return false, nil
		}
		return true, nil
	})
}

func checkOverlaySize(oc *exutil.CLI, overlaySize string) error {
	return wait.Poll(3*time.Second, 1*time.Minute, func() (bool, error) {
		workerNode := getSingleWorkerNode(oc)
		overlayString, err := exutil.DebugNodeWithChroot(oc, workerNode, "/bin/bash", "-c", "head -n 7 /etc/containers/storage.conf | grep size")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("overlaySize string : %v", overlayString)
		if strings.Contains(string(overlayString), overlaySize) {
			e2e.Logf("overlay size check successfully")
		} else {
			e2e.Logf("overlay size check failed")
			return false, nil
		}
		return true, nil
	})
}

func checkPodOverlaySize(oc *exutil.CLI, overlaySize string) error {
	return wait.Poll(1*time.Second, 3*time.Second, func() (bool, error) {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		overlayString, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", oc.Namespace(), podName, "/bin/bash", "-c", "df -h | grep overlay").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("overlayString is : %v", overlayString)
		overlaySizeStr := strings.Fields(string(overlayString))
		e2e.Logf("overlaySize : %s", overlaySizeStr[1])
		overlaySizeInt := strings.Split(string(overlaySizeStr[1]), ".")[0] + "G"
		e2e.Logf("overlaySizeInt : %s", overlaySizeInt)
		if overlaySizeInt == overlaySize {
			e2e.Logf("pod overlay size is correct")
		} else {
			e2e.Logf("pod overlay size is not correct !!!")
			return false, nil
		}
		return true, nil
	})
}

func checkNetNs(oc *exutil.CLI, hostname string, netNsPath string) error {
	return wait.Poll(1*time.Second, 3*time.Second, func() (bool, error) {
		result, _ := exutil.DebugNodeWithChroot(oc, hostname, "ls", "-l", netNsPath)
		e2e.Logf("the check result: %v", result)
		if strings.Contains(string(result), "No such file or directory") {
			e2e.Logf("the NetNS file is cleaned successfully")
		} else {
			e2e.Logf("the NetNS file still exist")
			return false, nil
		}
		return true, nil
	})
}

// this function check if crontab events include error like : MountVolume.SetUp failed for volume "serviceca" : object "openshift-image-registry"/"serviceca" not registered
func checkEventsForErr(oc *exutil.CLI) error {
	return wait.Poll(1*time.Second, 3*time.Second, func() (bool, error) {
		// get all cronjob's namespace from:
		// NAMESPACE                              NAME               SCHEDULE       SUSPEND   ACTIVE   LAST SCHEDULE   AGE
		// openshift-image-registry               image-pruner       0 0 * * *      False     0        <none>          4h36m
		// openshift-operator-lifecycle-manager   collect-profiles   */15 * * * *   False     0        9m11s           4h40m
		allcronjobs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "--all-namespaces", "-o=jsonpath={range .items[*]}{@.metadata.namespace}{\"\\n\"}{end}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the cronjobs namespaces are: %v", allcronjobs)
		for _, s := range strings.Fields(allcronjobs) {
			events, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", s).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			keyword := "MountVolume.SetUp failed for volume.*object.*not registered"
			re := regexp.MustCompile(keyword)
			found := re.FindAllString(events, -1)
			if len(found) > 0 {
				e2e.Logf("The events of ns [%v] hit the error: %v", s, found[0])
				return false, nil
			}
		}
		e2e.Logf("all the crontab events don't hit the error: MountVolume.SetUp failed for volume ... not registered")
		return true, nil
	})
}

func cleanupObjectsClusterScope(oc *exutil.CLI, objs ...objectTableRefcscope) error {
	return wait.Poll(1*time.Second, 1*time.Second, func() (bool, error) {
		for _, v := range objs {
			e2e.Logf("\n Start to remove: %v", v)
			status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(v.kind, v.name).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(status, "Error") {
				e2e.Logf("Error getting resources... Seems resources objects are already deleted. \n")
				return true, nil
			}
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, v.name).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		return true, nil
	})
}

func (podTwoContainers *podTwoContainersDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podTwoContainers.template, "-p", "NAME="+podTwoContainers.name, "NAMESPACE="+podTwoContainers.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}
func (podTwoContainers *podTwoContainersDescription) delete(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", podTwoContainers.namespace, "pod", podTwoContainers.name).Execute()
}

func (podUserNS *podUserNSDescription) crioWorkloadConfigExist(oc *exutil.CLI) error {
	return wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodename := nodeList.Items[0].Name
		workloadString, _ := exutil.DebugNodeWithChroot(oc, nodename, "cat", "/etc/crio/crio.conf.d/00-default")
		//not handle err as a workaround of issue: debug container needs more time to start in 4.13&4.14
		//o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(string(workloadString), "crio.runtime.workloads.openshift-builder") && strings.Contains(string(workloadString), "io.kubernetes.cri-o.userns-mode") && strings.Contains(string(workloadString), "io.kubernetes.cri-o.Devices") {
			e2e.Logf("the crio workload exist in /etc/crio/crio.conf.d/00-default")
		} else {
			e2e.Logf("the crio workload not exist in /etc/crio/crio.conf.d/00-default")
			return false, nil
		}
		return true, nil
	})
}

func (podUserNS *podUserNSDescription) userContainersExistForNS(oc *exutil.CLI) error {
	return wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodename := nodeList.Items[0].Name
		xContainers, _ := exutil.DebugNodeWithChroot(oc, nodename, "bash", "-c", "cat /etc/subuid /etc/subgid")
		//not handle err as a workaround of issue: debug container needs more time to start in 4.13&4.14
		//o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Count(xContainers, "containers") == 2 {
			e2e.Logf("the user containers exist in /etc/subuid and /etc/subgid")
		} else {
			e2e.Logf("the user containers not exist in /etc/subuid and /etc/subgid")
			return false, nil
		}
		return true, nil
	})
}

func (podUserNS *podUserNSDescription) podRunInUserNS(oc *exutil.CLI) error {
	return wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", podUserNS.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		idString, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", podUserNS.namespace, podName, "id").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(string(idString), "uid=0(root) gid=0(root) groups=0(root)") {
			e2e.Logf("the user id in pod is root")
			podUserNSstr, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", podUserNS.namespace, podName, "lsns", "-o", "NS", "-t", "user").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("string(podUserNS) is : %s", string(podUserNSstr))
			podNS := strings.Fields(string(podUserNSstr))
			e2e.Logf("pod user namespace : %s", podNS[1])

			nodename, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].spec.nodeName}", "-n", podUserNS.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeUserNS, _ := exutil.DebugNodeWithChroot(oc, string(nodename), "/bin/bash", "-c", "lsns -t user | grep /usr/lib/systemd/systemd")
			//not handle err as a workaround of issue: debug container needs more time to start in 4.13&4.14
			//o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("host user ns string : %v", nodeUserNS)
			nodeNSstr := strings.Split(string(nodeUserNS), "\n")
			nodeNS := strings.Fields(nodeNSstr[0])
			e2e.Logf("host user namespace : %s", nodeNS[0])
			if nodeNS[0] == podNS[1] {
				e2e.Logf("pod run in the same user namespace with host")
				return false, nil
			}
			e2e.Logf("pod run in different user namespace with host")
			return true, nil
		}
		e2e.Logf("the user id in pod is not root")
		return false, nil
	})
}

func crioConfigExist(oc *exutil.CLI, crioConfig []string, configPath string) error {
	return wait.Poll(1*time.Second, 3*time.Second, func() (bool, error) {
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodename := nodeList.Items[0].Name
		crioString, err := exutil.DebugNodeWithChroot(oc, nodename, "cat", configPath)
		e2e.Logf("the %s is: \n%v", configPath, crioString)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, config := range crioConfig {
			if !strings.Contains(string(crioString), config) {
				e2e.Logf("the config: %s not exist in %s", config, configPath)
				return false, nil
			}
		}
		e2e.Logf("all the config exist in %s", configPath)
		return true, nil
	})
}

func checkMachineConfigPoolStatus(oc *exutil.CLI, nodeSelector string) error {
	return wait.Poll(10*time.Second, 15*time.Minute, func() (bool, error) {
		mCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.machineCount}").Output()
		unmCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.unavailableMachineCount}").Output()
		dmCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.degradedMachineCount}").Output()
		rmCount, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", nodeSelector, "-n", oc.Namespace(), "-o=jsonpath={.status.readyMachineCount}").Output()
		e2e.Logf("MachineCount:%v unavailableMachineCount:%v degradedMachineCount:%v ReadyMachineCount:%v", mCount, unmCount, dmCount, rmCount)
		if strings.Compare(mCount, rmCount) == 0 && strings.Compare(unmCount, dmCount) == 0 {
			return true, nil
		}
		return false, nil
	})
}

// this func check the pod's cpu setting override the host default
func overrideWkloadCpu(oc *exutil.CLI, cpuset string, cpushare string, namespace string) error {
	return wait.Poll(1*time.Second, 3*time.Second, func() (bool, error) {
		podname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cpuSet, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(string(podname), "-n", namespace, "--", "cat", "/sys/fs/cgroup/cpuset/cpuset.cpus").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cpuset is : %s", cpuSet)
		cpuShare, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(string(podname), "-n", namespace, "--", "cat", "/sys/fs/cgroup/cpu/cpu.shares").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cpushare is : %s", cpuShare)
		if cpuset == "" {
			//if cpuset == "", the pod will keep default value as the /sys/fs/cgroup/cpuset/cpuset.cpus on node
			nodename, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].spec.nodeName}", "-n", namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The nodename is %v", nodename)
			cpusetDeft, err := exutil.DebugNodeWithChroot(oc, nodename, "cat", "/sys/fs/cgroup/cpuset/cpuset.cpus")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The cpuset of host is : %s", cpusetDeft)
			if strings.Contains(cpusetDeft, cpuSet) && cpuShare == cpushare {
				e2e.Logf("the pod only override cpushares but not cpuset")
				return true, nil
			}
		} else if cpuSet == cpuset && cpuShare == cpushare {
			e2e.Logf("the pod override the default workload setting")
			return true, nil
		}
		return false, nil
	})
}

// this func check the pod's cpu setting keep the same as host default
func defaultWkloadCpu(oc *exutil.CLI, cpuset string, cpushare string, namespace string) error {
	return wait.Poll(1*time.Second, 3*time.Second, func() (bool, error) {
		podname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cpuSet, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(string(podname), "-n", namespace, "--", "cat", "/sys/fs/cgroup/cpuset/cpuset.cpus").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cpuset of pod is : %s", cpuSet)
		cpuShare, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(string(podname), "-n", namespace, "--", "cat", "/sys/fs/cgroup/cpu/cpu.shares").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cpushare of pod is : %s", cpuShare)

		nodename, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].spec.nodeName}", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The nodename is %v", nodename)
		cpusetDeft, err := exutil.DebugNodeWithChroot(oc, nodename, "cat", "/sys/fs/cgroup/cpuset/cpuset.cpus")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cpuset of host is : %s", cpusetDeft)
		cpushareDeft, err := exutil.DebugNodeWithChroot(oc, nodename, "cat", "/sys/fs/cgroup/cpu/cpu.shares")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The cpushare of host is : %s", cpushareDeft)

		//cpushares 2 is a default from https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/kuberuntime/kuberuntime_sandbox_linux_test.go#L118,so we can ignore host value
		if (cpuShare == "2") && strings.Contains(cpusetDeft, cpuSet) {
			if string(cpuSet) != cpuset || string(cpuShare) != cpushare {
				e2e.Logf("the pod keep the default workload setting")
				return true, nil
			}
			e2e.Logf("the pod specified value is the same as default, invalid test!")
			return false, nil
		}
		return false, nil
	})
}

// this function create CMA(Keda) operator
func createKedaOperator(oc *exutil.CLI) {
	buildPruningBaseDir := exutil.FixturePath("testdata", "node")
	operatorGroup := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	subscription := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	nsOperator := filepath.Join(buildPruningBaseDir, "ns-keda-operator.yaml")
	operatorNamespace := "openshift-keda"

	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", nsOperator).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroup).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subscription).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	// checking subscription status
	errCheck := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		subState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "openshift-custom-metrics-autoscaler-operator", "-n", operatorNamespace, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(subState, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription openshift-custom-metrics-autoscaler-operator is not correct status"))

	// checking csv status
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "openshift-custom-metrics-autoscaler-operator", "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	errCheck = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", operatorNamespace, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(csvState, "Succeeded") == 0 {
			e2e.Logf("CSV check complete!!!")
			return true, nil
		}
		return false, nil

	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status", csvName))
}
func waitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.conditions[?(@.type==\"Ready\")].status}").Output()
		e2e.Logf("the Ready status of pod is %v", status)
		if err != nil || status == "" {
			e2e.Logf("failed to get pod status: %v, retrying...", err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("the pod Ready status not met; wanted True but got %v, retrying...", status)
			return false, nil
		}
		return true, nil
	})
}

// this function is for check kubelet_log_level
func assertKubeletLogLevel(oc *exutil.CLI) {
	var kubeservice string
	var kublet string
	var err error
	waitErr := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
		nodeName, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		nodes := strings.Fields(nodeName)

		for _, node := range nodes {
			nodeStatus, statusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node, "-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}").Output()
			o.Expect(statusErr).NotTo(o.HaveOccurred())
			e2e.Logf("\nNode %s Status is %s\n", node, nodeStatus)

			if nodeStatus == "True" {
				kubeservice, err = exutil.DebugNodeWithChroot(oc, node, "/bin/bash", "-c", "systemctl show kubelet.service | grep KUBELET_LOG_LEVEL")
				o.Expect(err).NotTo(o.HaveOccurred())
				kublet, err = exutil.DebugNodeWithChroot(oc, node, "/bin/bash", "-c", "ps aux | grep kubelet")
				o.Expect(err).NotTo(o.HaveOccurred())

				if strings.Contains(string(kubeservice), "KUBELET_LOG_LEVEL") && strings.Contains(string(kublet), "--v=2") {
					e2e.Logf(" KUBELET_LOG_LEVEL is 2. \n")
					return true, nil
				} else {
					e2e.Logf(" KUBELET_LOG_LEVEL is not 2. \n")
					return false, nil
				}
			} else {
				e2e.Logf("\n NODES ARE NOT READY\n ")
			}
		}
		return false, nil
	})
	if waitErr != nil {
		e2e.Logf("Kubelet Log level is:\n %v\n", kubeservice)
		e2e.Logf("Running Proccess of kubelet are:\n %v\n", kublet)
	}
	exutil.AssertWaitPollNoErr(waitErr, "KUBELET_LOG_LEVEL is not expected")
}

// this function create VPA(Vertical Pod Autoscaler) operator
func createVpaOperator(oc *exutil.CLI) {
	buildPruningBaseDir := exutil.FixturePath("testdata", "node")
	operatorGroup := filepath.Join(buildPruningBaseDir, "vpa-operatorgroup.yaml")
	subscription := filepath.Join(buildPruningBaseDir, "vpa-subscription.yaml")
	nsOperator := filepath.Join(buildPruningBaseDir, "ns-vpa-operator.yaml")
	operatorNamespace := "openshift-vertical-pod-autoscaler"

	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", nsOperator).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroup).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subscription).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	// checking subscription status
	errCheck := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		subState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "vertical-pod-autoscaler", "-n", operatorNamespace, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(subState, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription vertical-pod-autoscaler is not correct status"))

	// checking csv status
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "vertical-pod-autoscaler", "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	errCheck = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", operatorNamespace, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(csvState, "Succeeded") == 0 {
			e2e.Logf("CSV check complete!!!")
			return true, nil
		}
		return false, nil

	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status", csvName))
}

// this function is for  updating the runtimeRequestTimeout parameter using KubeletConfig CR

func runTimeTimeout(oc *exutil.CLI) {
	var kubeletConf string
	var err error
	nodeName, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(nodeErr).NotTo(o.HaveOccurred())
	e2e.Logf("\nNode Names are %v", nodeName)
	nodes := strings.Fields(nodeName)

	waitErr := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {

		for _, node := range nodes {
			nodeStatus, statusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node, "-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}").Output()
			o.Expect(statusErr).NotTo(o.HaveOccurred())
			e2e.Logf("\nNode %s Status is %s\n", node, nodeStatus)

			if nodeStatus == "True" {
				kubeletConf, err = exutil.DebugNodeWithChroot(oc, node, "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf | grep runtimeRequestTimeout")
				o.Expect(err).NotTo(o.HaveOccurred())

				if strings.Contains(string(kubeletConf), "runtimeRequestTimeout") && strings.Contains(string(kubeletConf), ":") && strings.Contains(string(kubeletConf), "3m0s") {
					e2e.Logf(" RunTime Request Timeout is 3 minutes. \n")
					return true, nil
				} else {
					e2e.Logf("Runtime Request Timeout is not 3 minutes. \n")
					return false, nil
				}
			} else {
				e2e.Logf("\n NODES ARE NOT READY\n ")
			}
		}
		return false, nil
	})
	if waitErr != nil {
		e2e.Logf("RunTime Request Timeout is:\n %v\n", kubeletConf)

	}
	exutil.AssertWaitPollNoErr(waitErr, "Runtime Request Timeout is not expected")
}

func checkConmonForAllNode(oc *exutil.CLI) {
	var configStr string
	waitErr := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
		nodeName, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		nodes := strings.Fields(nodeName)

		for _, node := range nodes {
			nodeStatus, statusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node, "-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}").Output()
			o.Expect(statusErr).NotTo(o.HaveOccurred())
			e2e.Logf("\nNode [%s] Status is %s\n", node, nodeStatus)

			if nodeStatus == "True" {
				configStr, err := exutil.DebugNodeWithChroot(oc, node, "/bin/bash", "-c", "crio config | grep 'conmon = \"\"'")
				o.Expect(err).NotTo(o.HaveOccurred())

				if strings.Contains(string(configStr), "conmon = \"\"") {
					e2e.Logf(" conmon check pass. \n")
				} else {
					e2e.Logf(" conmon check failed. \n")
					return false, nil
				}
			} else {
				e2e.Logf("\n NODES ARE NOT READY\n ")
				return false, nil
			}
		}
		return true, nil
	})

	if waitErr != nil {
		e2e.Logf("conmon string is:\n %v\n", configStr)
	}
	exutil.AssertWaitPollNoErr(waitErr, "the conmon is not as expected!")
}

func checkUpgradeMachineConfig(oc *exutil.CLI) {

	var machineconfig string
	waitErr := wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
		upgradestatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "machine-config").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\n Upgrade status is %s\n", upgradestatus)
		machineconfig, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("mc").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())

		if strings.Contains(string(machineconfig), "99-worker-generated-kubelet-1") {
			re := regexp.MustCompile("99-worker-generated-kubelet")
			found := re.FindAllString(machineconfig, -1)
			lenstr := len(found)
			if lenstr == 2 {
				e2e.Logf("\n Upgrade happened successfully")
				return true, nil
			} else {
				e2e.Logf("\nError")
				return false, nil
			}
		} else {
			e2e.Logf(" Upgarde has failed \n")
			return false, nil
		}

		return true, nil
	})
	if waitErr != nil {
		e2e.Logf("machine config is %s\n", machineconfig)
	}
	exutil.AssertWaitPollNoErr(waitErr, "the machine config is not as expected.")
}
