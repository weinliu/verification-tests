package workloads

import (
	"encoding/json"
	"fmt"
	o "github.com/onsi/gomega"
	"io/ioutil"
	"regexp"

	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type podNodeSelector struct {
	name       string
	namespace  string
	labelKey   string
	labelValue string
	nodeKey    string
	nodeValue  string
	template   string
}

type podSinglePts struct {
	name       string
	namespace  string
	labelKey   string
	labelValue string
	ptsKeyName string
	ptsPolicy  string
	skewNum    int
	template   string
}

type podSinglePtsNodeSelector struct {
	name       string
	namespace  string
	labelKey   string
	labelValue string
	ptsKeyName string
	ptsPolicy  string
	skewNum    int
	nodeKey    string
	nodeValue  string
	template   string
}

type deploySinglePts struct {
	dName      string
	namespace  string
	replicaNum int
	labelKey   string
	labelValue string
	ptsKeyName string
	ptsPolicy  string
	skewNum    int
	template   string
}

type deployNodeSelector struct {
	dName      string
	namespace  string
	replicaNum int
	labelKey   string
	labelValue string
	nodeKey    string
	nodeValue  string
	template   string
}

type podAffinityRequiredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	affinityMethod string
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podAffinityPreferredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	affinityMethod string
	weigthNum      int
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podNodeAffinityRequiredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	ptsKey2Name    string
	ptsPolicy2     string
	skewNum2       int
	affinityMethod string
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podSingleNodeAffinityRequiredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	affinityMethod string
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podTolerate struct {
	namespace      string
	keyName        string
	operatorPolicy string
	valueName      string
	effectPolicy   string
	tolerateTime   int
	template       string
}

// ControlplaneInfo ...
type ControlplaneInfo struct {
	HolderIdentity       string `json:"holderIdentity"`
	LeaseDurationSeconds int    `json:"leaseDurationSeconds"`
	AcquireTime          string `json:"acquireTime"`
	RenewTime            string `json:"renewTime"`
	LeaderTransitions    int    `json:"leaderTransitions"`
}

type serviceInfo struct {
	serviceIP   string
	namespace   string
	servicePort string
	serviceURL  string
	serviceName string
}

type registry struct {
	dockerImage string
	namespace   string
}

type podMirror struct {
	name            string
	namespace       string
	cliImageID      string
	imagePullSecret string
	imageSource     string
	imageTo         string
	imageToRelease  string
	template        string
}

type debugPodUsingDefinition struct {
	name       string
	namespace  string
	cliImageID string
	template   string
}

type priorityClassDefinition struct {
	name          string
	priorityValue int
	template      string
}

type priorityPod struct {
	dName      string
	namespace  string
	replicaSum int
	template   string
}

type cronJobCreationTZ struct {
	cName     string
	namespace string
	schedule  string
	timeZone  string
	template  string
}

func (pod *podNodeSelector) createPodNodeSelector(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"NODEKEY="+pod.nodeKey, "NODEVALUE="+pod.nodeValue, "LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSinglePts) createPodSinglePts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSinglePtsNodeSelector) createPodSinglePtsNodeSelector(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum),
			"NODEKEY="+pod.nodeKey, "NODEVALUE="+pod.nodeValue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (deploy *deploySinglePts) createDeploySinglePts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deploy.template, "-p", "DNAME="+deploy.dName, "NAMESPACE="+deploy.namespace,
			"REPLICASNUM="+strconv.Itoa(deploy.replicaNum), "LABELKEY="+deploy.labelKey, "LABELVALUE="+deploy.labelValue, "PTSKEYNAME="+deploy.ptsKeyName,
			"PTSPOLICY="+deploy.ptsPolicy, "SKEWNUM="+strconv.Itoa(deploy.skewNum))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("deploy %s with %s is not created successfully", deploy.dName, deploy.labelKey))
}

func (pod *podAffinityRequiredPts) createPodAffinityRequiredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum),
			"AFFINITYMETHOD="+pod.affinityMethod, "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podAffinityPreferredPts) createPodAffinityPreferredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum),
			"AFFINITYMETHOD="+pod.affinityMethod, "WEIGHTNUM="+strconv.Itoa(pod.weigthNum), "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSinglePts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podNodeSelector) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podSinglePtsNodeSelector) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podAffinityRequiredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podAffinityPreferredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "workload-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func applyResourceFromTemplate48681(oc *exutil.CLI, parameters ...string) (string, error) {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "workload-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return configFile, oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func describePod(oc *exutil.CLI, namespace string, podName string) string {
	podDescribe, err := oc.WithoutNamespace().Run("describe").Args("pod", "-n", namespace, podName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status is %q", podName, podDescribe)
	return podDescribe
}

func getPodStatus(oc *exutil.CLI, namespace string, podName string) string {
	podStatus, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status is %q", podName, podStatus)
	return podStatus
}

func getPodNodeListByLabel(oc *exutil.CLI, namespace string, labelKey string) []string {
	output, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", labelKey, "-o=jsonpath={.items[*].spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeNameList := strings.Fields(output)
	return nodeNameList
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

func (pod *podNodeAffinityRequiredPts) createpodNodeAffinityRequiredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum), "PTSKEY2NAME="+pod.ptsKey2Name, "PTSPOLICY2="+pod.ptsPolicy2, "SKEWNUM2="+strconv.Itoa(pod.skewNum2), "AFFINITYMETHOD="+pod.affinityMethod, "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podNodeAffinityRequiredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podSingleNodeAffinityRequiredPts) createpodSingleNodeAffinityRequiredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum), "AFFINITYMETHOD="+pod.affinityMethod, "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSingleNodeAffinityRequiredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podTolerate) createPodTolerate(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAMESPACE="+pod.namespace, "KEYNAME="+pod.keyName,
			"OPERATORPOLICY="+pod.operatorPolicy, "VALUENAME="+pod.valueName, "EFFECTPOLICY="+pod.effectPolicy, "TOLERATETIME="+strconv.Itoa(pod.tolerateTime))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s is not created successfully", pod.keyName))
}

func getPodNodeName(oc *exutil.CLI, namespace string, podName string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", podName, nodeName)
	return nodeName
}

func createLdapService(oc *exutil.CLI, namespace string, podName string, initGroup string) {
	err := oc.AsAdmin().WithoutNamespace().Run("run").Args(podName, "--image", "quay.io/openshifttest/ldap@sha256:2700c5252cc72e12b845fe97ed659b8178db5ac72e13116b617de431c7826600", "-n", namespace).Execute()
	if err != nil {
		oc.Run("delete").Args("pod/ldapserver", "-n", namespace).Execute()
		e2e.Failf("failed to run the ldap pod")
	}
	checkPodStatus(oc, "run="+podName, namespace, "Running")
	err = oc.Run("cp").Args("-n", namespace, initGroup, podName+":/tmp/").Execute()
	if err != nil {
		oc.Run("delete").Args("pod/ldapserver", "-n", oc.Namespace()).Execute()
		e2e.Failf("failed to copy the init group to ldap server")
	}
	err = oc.Run("exec").Args(podName, "-n", namespace, "--", "ldapadd", "-x", "-h", "[::1]", "-p", "389", "-D", "cn=Manager,dc=example,dc=com", "-w", "admin", "-f", "/tmp/init.ldif").Execute()
	if err != nil {
		oc.Run("delete").Args("pod/ldapserver", "-n", namespace).Execute()
		e2e.Failf("failed to config the ldap server ")
	}

}

func getSyncGroup(oc *exutil.CLI, syncConfig string) string {
	var groupFile string
	err := wait.Poll(5*time.Second, 200*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("adm").Args("groups", "sync", "--sync-config="+syncConfig).OutputToFile(getRandomString() + "workload-group.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		groupFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "adm groups sync fails")
	if strings.Compare(groupFile, "") == 0 {
		e2e.Failf("Failed to get group infomation!")
	}
	return groupFile
}

func getLeaderKCM(oc *exutil.CLI) string {
	var leaderKCM string
	e2e.Logf("Get the control-plane from configmap")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap/kube-controller-manager", "-n", "kube-system", "-o=jsonpath={.metadata.annotations.control-plane\\.alpha\\.kubernetes\\.io/leader}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Print the output: %v ", output)
	contronplanInfo := &ControlplaneInfo{}
	e2e.Logf("convert to json file ")
	if err = json.Unmarshal([]byte(output), &contronplanInfo); err != nil {
		e2e.Failf("unable to decode with error: %v", err)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	leaderIP := strings.Split(contronplanInfo.HolderIdentity, "_")[0]

	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/master=", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	masterList := strings.Fields(out)
	for _, masterNode := range masterList {
		if matched, _ := regexp.MatchString(leaderIP, masterNode); matched {
			e2e.Logf("Find the leader of KCM :%s\n", masterNode)
			leaderKCM = masterNode
			break
		}
	}
	return leaderKCM
}

func removeDuplicateElement(elements []string) []string {
	result := make([]string, 0, len(elements))
	temp := map[string]struct{}{}
	for _, item := range elements {
		if _, ok := temp[item]; !ok { //if can't find the item，ok=false，!ok is true，then append item。
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

func (registry *registry) createregistry(oc *exutil.CLI) serviceInfo {
	defer func() {
		oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "registry", "-n", registry.namespace, "-oyaml").Execute()
		oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "deployment=registry", "-n", registry.namespace, "-oyaml").Execute()
	}()
	err := oc.AsAdmin().Run("new-app").Args("--image", registry.dockerImage, "-n", registry.namespace).Execute()
	if err != nil {
		e2e.Failf("Failed to create the registry server")
	}
	err = oc.AsAdmin().Run("set").Args("probe", "deploy/registry", "--readiness", "--liveness", "--get-url="+"http://:5000/v2", "-n", registry.namespace).Execute()
	if err != nil {
		e2e.Failf("Failed to config the registry")
	}
	if ok := waitForAvailableRsRunning(oc, "deployment", "registry", registry.namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		e2e.Failf("private registry pod is not running even afer waiting for about 3 minutes")
	}

	e2e.Logf("Get the service info of the registry")
	regSvcIP, err := oc.AsAdmin().Run("get").Args("svc", "registry", "-n", registry.namespace, "-o=jsonpath={.spec.clusterIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().Run("create").Args("route", "edge", "my-route", "--service=registry", "-n", registry.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	regSvcPort, err := oc.AsAdmin().Run("get").Args("svc", "registry", "-n", registry.namespace, "-o=jsonpath={.spec.ports[0].port}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	regRoute, err := oc.AsAdmin().Run("get").Args("route", "my-route", "-n", registry.namespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Check the route of registry available")
	ingressOpratorPod, err := oc.AsAdmin().Run("get").Args("pod", "-l", "name=ingress-operator", "-n", "openshift-ingress-operator", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	waitErr := wait.Poll(5*time.Second, 90*time.Second, func() (bool, error) {
		err := oc.AsAdmin().Run("exec").Args("pod/"+ingressOpratorPod, "-n", "openshift-ingress-operator", "--", "curl", "-v", "https://"+regRoute, "-I", "-k").Execute()
		if err != nil {
			e2e.Logf("route is not yet resolving, retrying...")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the route is not reachable"))

	regSvcURL := regSvcIP + ":" + regSvcPort
	svc := serviceInfo{
		serviceIP:   regSvcIP,
		namespace:   registry.namespace,
		servicePort: regSvcPort,
		serviceURL:  regSvcURL,
		serviceName: regRoute,
	}
	return svc

}

func (registry *registry) deleteregistry(oc *exutil.CLI) {
	_ = oc.Run("delete").Args("svc", "registry", "-n", registry.namespace).Execute()
	_ = oc.Run("delete").Args("deploy", "registry", "-n", registry.namespace).Execute()
	_ = oc.Run("delete").Args("is", "registry", "-n", registry.namespace).Execute()
}

func (pod *podMirror) createPodMirror(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "CLIIMAGEID="+pod.cliImageID, "IMAGEPULLSECRET="+pod.imagePullSecret, "IMAGESOURCE="+pod.imageSource, "IMAGETO="+pod.imageTo, "IMAGETORELEASE="+pod.imageToRelease)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.cliImageID))
}

func createPullSecret(oc *exutil.CLI, namespace string) {
	err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to=/tmp", "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", "my-secret", "--from-file="+"/tmp/.dockerconfigjson", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getCliImage(oc *exutil.CLI) string {
	cliImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("imagestreams", "cli", "-n", "openshift", "-o=jsonpath={.spec.tags[0].from.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return cliImage
}

func getScanNodesLabels(oc *exutil.CLI, nodeList []string, expected string) []string {
	var machedLabelsNodeNames []string
	for _, nodeName := range nodeList {
		nodeLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(expected, nodeLabels); matched {
			machedLabelsNodeNames = append(machedLabelsNodeNames, nodeName)
		}
	}
	return machedLabelsNodeNames
}

func checkMustgatherPodNode(oc *exutil.CLI) {
	var nodeNameList []string
	e2e.Logf("Get the node list of the must-gather pods running on")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "app=must-gather", "-A", "-o=jsonpath={.items[*].spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeNameList = strings.Fields(output)
		if nodeNameList == nil {
			e2e.Logf("Can't find must-gather pod now, and try next round")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("must-gather pod is not created successfully"))
	e2e.Logf("must-gather scheduled on: %v", nodeNameList)

	e2e.Logf("make sure all the nodes in nodeNameList are not windows node")
	expectedNodeLabels := getScanNodesLabels(oc, nodeNameList, "windows")
	if expectedNodeLabels == nil {
		e2e.Logf("must-gather scheduled as expected, no windows node found in the cluster")
	} else {
		e2e.Failf("Scheduled the must-gather pod to windows node: %v", expectedNodeLabels)
	}
}

func (pod *debugPodUsingDefinition) createDebugPodUsingDefinition(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		outputFile, err1 := applyResourceFromTemplate48681(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "CLIIMAGEID="+pod.cliImageID)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		e2e.Logf("Waiting for pod running")
		err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
			phase, err := oc.AsAdmin().Run("get").Args("pods", pod.name, "--template", "{{.status.phase}}", "-n", pod.namespace).Output()
			if err != nil {
				return false, nil
			}
			if phase != "Running" {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod has not been started successfully"))

		debugPod, err := oc.Run("debug").Args("-f", outputFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if match, _ := regexp.MatchString("Starting pod/pod48681-debug", debugPod); !match {
			e2e.Failf("Image debug container is being started instead of debug pod using the pod definition yaml file")
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.cliImageID))
}

func createDeployment(oc *exutil.CLI, namespace string, deployname string) {
	err := oc.Run("create").Args("-n", namespace, "deployment", deployname, "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--replicas=20").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func triggerSucceedDeployment(oc *exutil.CLI, namespace string, deployname string, num int, expectedPods int) {
	var generation string
	var getGenerationerr error
	err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
		generation, getGenerationerr = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", deployname, "-n", namespace, "-o=jsonpath={.status.observedGeneration}").Output()
		if getGenerationerr != nil {
			e2e.Logf("Err Occurred, try again: %v", getGenerationerr)
			return false, nil
		}
		if generation == "" {
			e2e.Logf("Can't get generation, try again: %v", generation)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to get  generation "))

	generationNum, err := strconv.Atoi(generation)
	o.Expect(err).NotTo(o.HaveOccurred())
	for i := 0; i < num; i++ {
		generationNum++
		err := oc.Run("set").Args("-n", namespace, "env", "deployment", deployname, "paramtest=test"+strconv.Itoa(i)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, currentRsName := getCurrentRs(oc, namespace, "app="+deployname, generationNum)
		err = wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
			availablePodNum, errGet := oc.Run("get").Args("-n", namespace, "rs", currentRsName, "-o=jsonpath='{.status.availableReplicas}'").Output()
			if errGet != nil {
				e2e.Logf("Err Occurred: %v", errGet)
				return false, errGet
			}
			availableNum, _ := strconv.Atoi(strings.ReplaceAll(availablePodNum, "'", ""))
			if availableNum != expectedPods {
				e2e.Logf("new triggered apps not deploy successfully, wait more times")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("failed to deploy %v", deployname))

	}
}
func triggerFailedDeployment(oc *exutil.CLI, namespace string, deployname string) {
	patchYaml := `[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "quay.io/openshifttest/hello-openshift:nonexist"}]`
	err := oc.Run("patch").Args("-n", namespace, "deployment", deployname, "--type=json", "-p", patchYaml).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getShouldPruneRSFromPrune(oc *exutil.CLI, pruneRsNumCMD string, pruneRsCMD string, prunedNum int) []string {
	e2e.Logf("Get pruned rs name by dry-run")
	e2e.Logf("pruneRsNumCMD %v:", pruneRsNumCMD)
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		pruneRsNum, err := exec.Command("bash", "-c", pruneRsNumCMD).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pruneNum, err := strconv.Atoi(strings.ReplaceAll(string(pruneRsNum), "\n", ""))
		o.Expect(err).NotTo(o.HaveOccurred())
		if pruneNum != prunedNum {
			e2e.Logf("pruneNum is not equal %v: ", prunedNum)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check pruned RS failed"))

	e2e.Logf("pruneRsCMD %v:", pruneRsCMD)
	pruneRsName, err := exec.Command("bash", "-c", pruneRsCMD).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	pruneRsList := strings.Fields(strings.ReplaceAll(string(pruneRsName), "\n", " "))
	sort.Strings(pruneRsList)
	e2e.Logf("pruneRsList %v:", pruneRsList)
	return pruneRsList
}

func getCompeletedRsInfo(oc *exutil.CLI, namespace string, deployname string) (completedRsList []string, completedRsNum int) {
	out, err := oc.Run("get").Args("-n", namespace, "rs", "--sort-by={.metadata.creationTimestamp}", "-o=jsonpath='{.items[?(@.spec.replicas == 0)].metadata.name}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("string out %v:", out)
	totalCompletedRsList := strings.Fields(strings.ReplaceAll(out, "'", ""))
	totalCompletedRsListNum := len(totalCompletedRsList)
	return totalCompletedRsList, totalCompletedRsListNum
}

func getShouldPruneRSFromCreateTime(totalCompletedRsList []string, totalCompletedRsListNum int, keepNum int) []string {
	rsList := totalCompletedRsList[0:(totalCompletedRsListNum - keepNum)]
	sort.Strings(rsList)
	e2e.Logf("rsList %v:", rsList)
	return rsList

}

func comparePrunedRS(rsList []string, pruneRsList []string) bool {
	e2e.Logf("Check pruned rs whether right")
	if !reflect.DeepEqual(rsList, pruneRsList) {
		return false
	}
	return true
}

func checkRunningRsList(oc *exutil.CLI, namespace string, deployname string) []string {
	e2e.Logf("Get all the running RSs")
	out, err := oc.Run("get").Args("-n", namespace, "rs", "-o=jsonpath='{.items[?(@.spec.replicas > 0)].metadata.name}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	runningRsList := strings.Fields(strings.ReplaceAll(out, "'", ""))
	sort.Strings(runningRsList)
	e2e.Logf("runningRsList %v:", runningRsList)
	return runningRsList
}

func pruneCompletedRs(oc *exutil.CLI, parameters ...string) {
	e2e.Logf("Delete all the completed RSs")
	err := oc.AsAdmin().WithoutNamespace().Run("adm").Args(parameters...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getRemainingRs(oc *exutil.CLI, namespace string, deployname string) []string {
	e2e.Logf("Get all the remaining RSs")
	remainRs, err := oc.WithoutNamespace().Run("get").Args("rs", "-l", "app="+deployname, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	remainRsList := strings.Fields(string(remainRs))
	sort.Strings(remainRsList)
	e2e.Logf("remainRsList %v:", remainRsList)
	return remainRsList
}

func getCurrentRs(oc *exutil.CLI, projectName string, labels string, generationNum int) (string, string) {
	var podTHash, rsName string
	e2e.Logf("Print the deploy current generation %v", generationNum)
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		generationC, err1 := oc.Run("get").Args("deploy", "-n", projectName, "-l", labels, "-o=jsonpath={.items[*].status.observedGeneration}").Output()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		if matched, _ := regexp.MatchString(strconv.Itoa(generationNum), generationC); !matched {
			e2e.Logf("the generation is not expected, and try next round")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "The deploy generation is failed to update")
	err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		rsName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("rs", "-n", projectName, "-l", labels, fmt.Sprintf(`-o=jsonpath={.items[?(@.metadata.annotations.deployment\.kubernetes\.io/revision=='%s')].metadata.name}`, strconv.Itoa(generationNum))).Output()
		e2e.Logf("Print the deploy current rs is %v", rsName)
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Failed to get the current rs for deploy")
	podTHash, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("rs", rsName, "-n", projectName, "-o=jsonpath={.spec.selector.matchLabels.pod-template-hash}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podTHash, rsName
}

func copyFile(source string, dest string) {
	bytesRead, err := ioutil.ReadFile(source)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = ioutil.WriteFile(dest, bytesRead, 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func locatePodmanCred(oc *exutil.CLI, dst string) error {
	err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dst, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	key := "XDG_RUNTIME_DIR"
	currentRuntime, ex := os.LookupEnv(key)
	if !ex {
		err = os.MkdirAll("/tmp/configocmirror/containers", 0700)
		o.Expect(err).NotTo(o.HaveOccurred())
		os.Setenv(key, "/tmp/configocmirror")
		copyFile(dst+"/"+".dockerconfigjson", "/tmp/configocmirror/containers/auth.json")
		return nil
	}
	_, err = os.Stat(currentRuntime + "containers/auth.json")
	if os.IsNotExist(err) {
		err1 := os.MkdirAll(currentRuntime+"containers", 0700)
		o.Expect(err1).NotTo(o.HaveOccurred())
		copyFile(dst+"/"+".dockerconfigjson", "/tmp/configocmirror/containers/auth.json")
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}

func checkPodStatus(oc *exutil.CLI, podLabel string, namespace string, expected string) {
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", podLabel, "-o=jsonpath={.items[*].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the result of pod:%v", output)
		if strings.Contains(output, expected) && (!(strings.Contains(strings.ToLower(output), "error"))) && (!(strings.Contains(strings.ToLower(output), "crashLoopbackOff"))) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the state of pod with %s is not expected %s", podLabel, expected))
}

func locateDockerCred(oc *exutil.CLI, dst string) (string, string, error) {
	err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dst, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	homePath := os.Getenv("HOME")
	dockerCreFile := homePath + "/.docker/config.json"
	_, err = os.Stat(homePath + "/.docker/config.json")
	if os.IsNotExist(err) {
		err1 := os.MkdirAll(homePath+"/.docker", 0700)
		o.Expect(err1).NotTo(o.HaveOccurred())
		copyFile(dst+"/"+".dockerconfigjson", homePath+"/.docker/config.json")
		return dockerCreFile, homePath, nil
	}
	if err != nil {
		return "", "", err
	}
	copyFile(homePath+"/.docker/config.json", homePath+"/.docker/config.json.back")
	copyFile(dst+"/"+".dockerconfigjson", homePath+"/.docker/config.json")
	return dockerCreFile, homePath, nil

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

func (pc *priorityClassDefinition) createPriorityClass(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pc.template, "-p", "NAME="+pc.name, "PRIORITYVALUE="+strconv.Itoa(pc.priorityValue))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("priorityClass %s has not been created successfully", pc.name))
}

func (pc *priorityClassDefinition) deletePriorityClass(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("priorityclass", pc.name).Execute()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("priorityclass %s is not deleted successfully", pc.name))
}

func checkNetworkType(oc *exutil.CLI) string {
	output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.type}").Output()
	return strings.ToLower(output)
}

func checkDockerCred() bool {
	homePath := os.Getenv("HOME")
	_, err := os.Stat(homePath + "/.docker/config.json")
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func checkPodmanCred() bool {
	currentRuntime := os.Getenv("XDG_RUNTIME_DIR")
	_, err := os.Stat(currentRuntime + "containers/auth.json")
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func getPullSecret(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/pull-secret", "-n", "openshift-config", `--template={{index .data ".dockerconfigjson" | base64decode}}`).OutputToFile("auth.dockerconfigjson")
}

func getHostFromRoute(oc *exutil.CLI, routeName string, routeNamespace string) string {
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", routeName, "-n", routeNamespace, "-o", "jsonpath='{.spec.host}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	return stdout
}
func createEdgeRoute(oc *exutil.CLI, serviceName string, namespace string, routeName string) {
	err := oc.Run("create").Args("route", "edge", routeName, "--service", serviceName, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createDir(dirname string) {
	err := os.MkdirAll(dirname, 0755)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createSpecialRegistry(oc *exutil.CLI, namespace string, ssldir string, dockerConfig string) string {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("deploy", "mydauth", "-n", namespace, "--image=quay.io/openshifttest/registry-auth-server:1.2.0", "--port=5001").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("deploy", "mydauth", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("route", "passthrough", "r1", "--service=mydauth", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	hostD, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "r1", "-n", namespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	caSubj := "/C=GB/CN=foo  -addext \"subjectAltName = DNS:" + hostD + "\""
	opensslCmd := fmt.Sprintf(`openssl req -x509 -nodes -days 3650 -newkey rsa:2048 -keyout  %s/server.key  -out  %s/server.pem -subj %s`, ssldir, ssldir, caSubj)
	e2e.Logf("opensslcmd is :%v", opensslCmd)
	_, err = exec.Command("bash", "-c", opensslCmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "dockerauthssl", "--from-file="+ssldir, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("volume", "deploy", "mydauth", "--add", "--name=v2", "--type=secret", "--secret-name=dockerauthssl", "--mount-path=/ssl", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "dockerautoconfig", "--from-file="+dockerConfig, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("volume", "deploy", "mydauth", "--add", "--name=v1", "--type=secret", "--secret-name=dockerautoconfig", "--mount-path=/config", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Check the docker_auth pod should running")
	if ok := waitForAvailableRsRunning(oc, "deployment", "mydauth", namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		e2e.Failf("docker_auth pod is not running even afer waiting for about 3 minutes")
	}

	registryAuthToken := "https://" + hostD + "/auth"
	registryPara := fmt.Sprintf(`REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY=/tmp/registry REGISTRY_AUTH=token REGISTRY_AUTH_TOKEN_REALM=%s REGISTRY_AUTH_TOKEN_SERVICE="Docker registry" REGISTRY_AUTH_TOKEN_ISSUER="Acme auth server" REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE=/ssl/server.pem `, registryAuthToken)
	err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--name=myregistry", fmt.Sprintf("%s", registryPara), "-n", namespace, "--image=quay.io/openshifttest/registry:1.2.0").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("volume", "deploy", "myregistry", "--add", "--name=v2", "--type=secret", "--secret-name=dockerauthssl", "--mount-path=/ssl", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("route", "edge", "r2", "--service=myregistry", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	registryHost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "r2", "-n", namespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Check the registry pod should running")
	if ok := waitForAvailableRsRunning(oc, "deployment", "myregistry", namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		e2e.Failf("private registry pod is not running even afer waiting for about 3 minutes")
	}
	return registryHost
}

func checkNodeUncordoned(oc *exutil.CLI, workerNodeName string) error {
	return wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
		schedulableStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", workerNodeName, "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Schedulable Status is %s\n", schedulableStatus)
		if !strings.Contains(schedulableStatus, "unschedulable") {
			e2e.Logf("\n WORKER NODE IS READY\n ")
		} else {
			e2e.Logf("\n WORKERNODE IS NOT READY\n ")
			return false, nil
		}
		return true, nil
	})
}

func (prio *priorityPod) createPodWithPriorityParam(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", prio.template, "-p", "NAMESPACE="+prio.namespace, "DNAME="+prio.dName,
			"REPLICASNUM="+strconv.Itoa(prio.replicaSum))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with priority has not been created successfully", prio.dName))
}
func nonAdminApplyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.Run("process").Args(parameters...).OutputToFile(getRandomString() + "workload-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func getDigestFromImageInfo(oc *exutil.CLI, registryRoute string) string {
	path := "/tmp/mirroredimageinfo.yaml"
	defer os.Remove(path)
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		imageInfo, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", registryRoute+"/openshift/release-images", "--insecure").Output()
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		e2e.Logf("the imageinfo is :%v", imageInfo)
		err1 := ioutil.WriteFile(path, []byte(imageInfo), 0o644)
		o.Expect(err1).NotTo(o.HaveOccurred())
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("failed to get the mirrored image info"))
	imageDigest, err := exec.Command("bash", "-c", "cat "+path+"|grep Digest | awk -F' ' '{print $2}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the imagedeigest is :%v, ", string(imageDigest))
	return strings.ReplaceAll(string(imageDigest), "\n", "")
}

func findImageContentSourcePolicy() string {
	imageContentSourcePolicyFile, err := exec.Command("bash", "-c", "find . -name 'imageContentSourcePolicy.yaml'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.ReplaceAll(string(imageContentSourcePolicyFile), "\n", "")
}

func removeOcMirrorLog() {
	os.RemoveAll("oc-mirror-workspace")
	os.RemoveAll(".oc-mirror.log")
}

func (cj *cronJobCreationTZ) createCronJobWithTimeZone(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cj.template, "-p", "CNAME="+cj.cName, "NAMESPACE="+cj.namespace,
			"SCHEDULE="+cj.schedule, "TIMEZONE="+cj.timeZone)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cronjob with %s is not created successfully", cj.cName))
}

func getTimeFromTimezone(oc *exutil.CLI) (string, string) {
	var schedule, timeZoneName = "None", "None"
	t := time.Now()
	zoneName, _ := t.Zone()
	if zoneName == "IST" {
		timeZoneName = "Asia/Calcutta"
		ist, err := time.LoadLocation("Asia/Calcutta")
		if err != nil {
			e2e.Failf("Error is %v", err)
		}
		e2e.Logf("location:", ist, "Time:", t.In(ist))
		localTimeInHours := t.In(ist).Hour()
		localTimeInMinutes := t.In(ist).Minute()
		if localTimeInHours == 23 && localTimeInMinutes == 59 {
			schedule = "01 0 * * *"
		} else if localTimeInMinutes == 59 {
			localTimeInHours = localTimeInHours + 1
			schedule = "02 " + strconv.Itoa(localTimeInHours) + " " + "* * *"
		} else {
			localTimeInMinutes = localTimeInMinutes + 2
			if localTimeInMinutes == 60 {
				localTimeInHours = localTimeInHours + 1
				schedule = "00 " + strconv.Itoa(localTimeInHours) + " " + "* * *"
			} else {
				schedule = strconv.Itoa(localTimeInMinutes) + " " + strconv.Itoa(localTimeInHours) + " " + "* * *"
			}
		}
	} else if zoneName == "UTC" {
		timeZoneName = "America/New_York"
		utc, err := time.LoadLocation("America/New_York")
		if err != nil {
			e2e.Failf("Error is: ", err.Error())
		}
		e2e.Logf("location:", utc, "Time:", t.In(utc))
		localTimeInHours := t.In(utc).Hour()
		localTimeInMinutes := t.In(utc).Minute()
		if localTimeInHours == 23 && localTimeInMinutes == 59 {
			schedule = "01 0 * * *"
		} else if localTimeInMinutes == 59 {
			localTimeInHours = localTimeInHours + 1
			schedule = "02 " + strconv.Itoa(localTimeInHours) + " " + "* * *"
		} else {
			localTimeInMinutes = localTimeInMinutes + 2
			if localTimeInMinutes == 60 {
				localTimeInHours = localTimeInHours + 1
				schedule = "00 " + strconv.Itoa(localTimeInHours) + " " + "* * *"
			} else {
				schedule = strconv.Itoa(localTimeInMinutes) + " " + strconv.Itoa(localTimeInHours) + " " + "* * *"
			}
		}
	} else {
		e2e.Failf("Given zone name is %s", zoneName)
	}
	return schedule, timeZoneName
}
