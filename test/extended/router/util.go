package router

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ingressControllerDescription struct {
	name        string
	namespace   string
	defaultCert string
	domain      string
	shard       string
	replicas    int
	template    string
}

type ingctrlHostPortDescription struct {
	name        string
	namespace   string
	defaultCert string
	domain      string
	httpport    int
	httpsport   int
	statsport   int
	replicas    int
	template    string
}

type ipfailoverDescription struct {
	name        string
	namespace   string
	image       string
	vip         string
	HAInterface string
	template    string
}

type routeDescription struct {
	name      string
	namespace string
	domain    string
	subDomain string
	template  string
}

type ingressDescription struct {
	name        string
	namespace   string
	domain      string
	serviceName string
	template    string
}

type webServerRcDescription struct {
	podLabelName      string
	secSvcLabelName   string
	unsecSvcLabelName string
	template          string
	namespace         string
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

func getFixedLengthRandomString(length int) string {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	letterBytes := []byte(chars)
	result := make([]byte, length)
	for i := range result {
		result[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(result)
}

func getBaseDomain(oc *exutil.CLI) string {
	var basedomain string

	basedomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.config/cluster", "-o=jsonpath={.spec.baseDomain}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the base domain of the cluster: %v", basedomain)
	return basedomain
}

// to exact available worker node count and details
func exactNodeDetails(oc *exutil.CLI) (int, string) {
	workerNodeDetails, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeCount := int(strings.Count(workerNodeDetails, "Ready")) - (int(strings.Count(workerNodeDetails, "SchedulingDisabled")) + int(strings.Count(workerNodeDetails, "NotReady")))
	e2e.Logf("Worker node details are: %v", workerNodeDetails)
	e2e.Logf("Available worker node count is: %v", nodeCount)
	return nodeCount, workerNodeDetails
}

func (ingctrl *ingressControllerDescription) create(oc *exutil.CLI) {
	availableWorkerNode, _ := exactNodeDetails(oc)
	if availableWorkerNode < 1 {
		g.Skip("Skipping as there is no enough worker nodes")
	}
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ingctrl.template, "-p", "NAME="+ingctrl.name, "NAMESPACE="+ingctrl.namespace, "DOMAIN="+ingctrl.domain, "SHARD="+ingctrl.shard)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (ingctrl *ingressControllerDescription) delete(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("--ignore-not-found", "-n", ingctrl.namespace, "ingresscontroller", ingctrl.name).Execute()
}

// Function to create hostnetwork type ingresscontroller with custom http/https/stat ports
func (ingctrl *ingctrlHostPortDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ingctrl.template, "-p", "NAME="+ingctrl.name, "NAMESPACE="+ingctrl.namespace, "DOMAIN="+ingctrl.domain, "HTTPPORT="+strconv.Itoa(ingctrl.httpport), "HTTPSPORT="+strconv.Itoa(ingctrl.httpsport), "STATSPORT="+strconv.Itoa(ingctrl.statsport))
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Function to delete hostnetwork type ingresscontroller
func (ingctrl *ingctrlHostPortDescription) delete(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("--ignore-not-found", "-n", ingctrl.namespace, "ingresscontroller", ingctrl.name).Execute()
}

// create route object from file.
func (rut *routeDescription) create(oc *exutil.CLI) {
	err := createResourceToNsFromTemplate(oc, rut.namespace, "--ignore-unknown-parameters=true", "-f", rut.template, "-p", "SUBDOMAIN_NAME="+rut.subDomain, "NAMESPACE="+rut.namespace, "DOMAIN="+rut.domain)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// create ingress object from file.
func (ing *ingressDescription) create(oc *exutil.CLI) {
	err := createResourceToNsFromTemplate(oc, ing.namespace, "--ignore-unknown-parameters=true", "-f", ing.template, "-p", "NAME="+ing.name, "NAMESPACE="+ing.namespace, "DOMAIN="+ing.domain, "SERVICE_NAME="+ing.serviceName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (websrvrc *webServerRcDescription) create(oc *exutil.CLI) {
	err := createResourceToNsFromTemplate(oc, websrvrc.namespace, "--ignore-unknown-parameters=true", "-f", websrvrc.template, "-p", "PodLabelName="+websrvrc.podLabelName, "SecSvcLabelName="+websrvrc.secSvcLabelName, "UnsecSvcLabelName="+websrvrc.unsecSvcLabelName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (websrvrc *webServerRcDescription) delete(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", websrvrc.namespace, "--ignore-not-found", "ReplicationController", websrvrc.podLabelName).Execute()
}

// parse the yaml file to json.
func parseToJSON(oc *exutil.CLI, parameters []string) string {
	var jsonCfg string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "-temp-resource.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		jsonCfg = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))
	e2e.Logf("the file of resource is %s", jsonCfg)
	return jsonCfg
}

func createResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	jsonCfg := parseToJSON(oc, parameters)
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", jsonCfg).Execute()
}

func createResourceToNsFromTemplate(oc *exutil.CLI, ns string, parameters ...string) error {
	jsonCfg := parseToJSON(oc, parameters)
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", ns, "-f", jsonCfg).Execute()
}

func waitForCustomIngressControllerAvailable(oc *exutil.CLI, icname string) error {
	e2e.Logf("check ingresscontroller if available")
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingresscontroller", icname, "--namespace=openshift-ingress-operator", "-ojsonpath={.status.conditions[?(@.type==\"Available\")].status}").Output()
		e2e.Logf("the status of ingresscontroller is %v", status)
		if err != nil || status == "" {
			e2e.Logf("failed to get ingresscontroller %s: %v, retrying...", icname, err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("ingresscontroller %s conditions not available, retrying...", icname)
			return false, nil
		}
		return true, nil
	})
}

func getOnePodNameByLabel(oc *exutil.CLI, ns, label string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", label, "-o=jsonpath={.items[0].metadata.name}", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the one pod with label %v is %v", label, podName)
	return podName
}

// getNewRouterPod immediatly after/during deployment rolling update, don't care the previous pod status
func getNewRouterPod(oc *exutil.CLI, icName string) string {
	ns := "openshift-ingress"
	deployName := "deployment/router-" + icName
	re := regexp.MustCompile(`NewReplicaSet:\s+router-.+-([a-z0-9]+)\s+`)

	output, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args(deployName, "-n", ns).Output()
	rsLabel := "pod-template-hash=" + re.FindStringSubmatch(output)[1]
	e2e.Logf("the new ReplicaSet labels is %s", rsLabel)
	err := waitForPodWithLabelReady(oc, ns, rsLabel)
	exutil.AssertWaitPollNoErr(err, "the new router pod failed to be ready within allowed time!")
	return getOnePodNameByLabel(oc, ns, rsLabel)
}

func ensureRouterDeployGenerationIs(oc *exutil.CLI, icName, expectGeneration string) {
	ns := "openshift-ingress"
	deployName := "deployment/router-" + icName
	actualGeneration := "0"

	waitErr := wait.PollImmediate(3*time.Second, 30*time.Second, func() (bool, error) {
		actualGeneration, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(deployName, "-n", ns, "-o=jsonpath={.metadata.generation}").Output()
		e2e.Logf("Get the deployment generation is: %v", actualGeneration)
		if actualGeneration == expectGeneration {
			e2e.Logf("The router deployment generation is updated to %v", actualGeneration)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached and the expected deployment generation is %v but got %v", expectGeneration, actualGeneration))
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

func waitForPodWithLabelAppear(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Output()
		e2e.Logf("the pod list is %v", podList)
		// add check for OCPQE-17360: pod list is "No resources found in xxx namespace"
		podFlag := 1
		if strings.Contains(podList, "No resources found") {
			podFlag = 0
		}
		if err != nil || len(podList) < 1 || podFlag == 0 {
			e2e.Logf("failed to get pod: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
}

// wait for the named resource is disappeared, e.g. used while router deployment rolled out
func waitForResourceToDisappear(oc *exutil.CLI, ns, rsname string) error {
	return wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(rsname, "-n", ns).Output()
		e2e.Logf("check resource %v and got: %v", rsname, status)
		primary := false
		if err != nil {
			if strings.Contains(status, "NotFound") {
				e2e.Logf("the resource is disappeared!")
				primary = true
			} else {
				e2e.Logf("failed to get the resource: %v, retrying...", err)
			}
		} else {
			e2e.Logf("the resource is still there, retrying...")
		}
		return primary, nil
	})
}

// For normal user to create resources in the specified namespace from the file (not template)
func createResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.WithoutNamespace().Run("create").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// For admin user to create/delete resources in the specified namespace from the file (not template)
// oper, should be create or delete
func operateResourceFromFile(oc *exutil.CLI, oper, ns, file string) {
	err := oc.AsAdmin().WithoutNamespace().Run(oper).Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// For normal user to patch a resource in the specified namespace
func patchResourceAsUser(oc *exutil.CLI, ns, resource, patch string) {
	err := oc.WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type=merge", "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// For Admin to patch a resource in the specified namespace
func patchResourceAsAdmin(oc *exutil.CLI, ns, resource, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type=merge", "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// To patch global resources as Admin. Can used for patching resources such as ingresses or CVO
func patchGlobalResourceAsAdmin(oc *exutil.CLI, resource, patch string) {
	patchOut, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "--patch="+patch, "--type=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The output from the patch is:- %q ", patchOut)
}

// For Admin to patch a resource in the specified namespace, and then return the output after the patching operation
func patchResourceAsAdminAndGetLog(oc *exutil.CLI, ns, resource, patch string) (string, error) {
	outPut, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type=merge", "-n", ns).Output()
	return outPut, err
}

func createARoute(oc *exutil.CLI, ns, routeType, routeName, serviceName, routeHost string, extraParas []string) {
	if routeType == "http" {
		cmd := []string{"-n", ns, "service", serviceName, "--name=" + routeName, "--hostname=" + routeHost}
		cmd = append(cmd, extraParas...)
		_, err := oc.Run("expose").Args(cmd...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		cmd := []string{"-n", ns, "route", routeType, routeName, "--service=" + serviceName, "--hostname=" + routeHost}
		cmd = append(cmd, extraParas...)
		_, err := oc.WithoutNamespace().Run("create").Args(cmd...).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func exposeRoute(oc *exutil.CLI, ns, resource string) {
	err := oc.Run("expose").Args(resource, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func setAnnotation(oc *exutil.CLI, ns, resource, annotation string) {
	err := oc.Run("annotate").Args("-n", ns, resource, annotation, "--overwrite").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// this function will read the annotation from the given resource
func getAnnotation(oc *exutil.CLI, ns, resource, resourceName string) string {
	findAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		resource, resourceName, "-n", ns, "-o=jsonpath={.metadata.annotations}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return findAnnotation
}

func setEnvVariable(oc *exutil.CLI, ns, resource, envstring string) {
	err := oc.WithoutNamespace().Run("set").Args("env", "-n", ns, resource, envstring).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	time.Sleep(10 * time.Second)
}

// Generic function to collect resource values with jsonpath option
func fetchJSONPathValue(oc *exutil.CLI, ns, resource, searchline string) string {
	searchLine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, resource, "-o=jsonpath={"+searchline+"}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the searchline has result:%v", searchLine)
	return searchLine
}

// getNodeNameByPod gets the pod located node's name
func getNodeNameByPod(oc *exutil.CLI, namespace string, podName string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The nodename for pod %s in namespace %s is %s", podName, namespace, nodeName)
	return nodeName
}

// Collect pod describe command details:
func describePodResource(oc *exutil.CLI, podName, namespace string) string {
	podDescribe, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", podName, "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podDescribe
}

// for collecting a single pod name for general use.
// usage example: podname := getRouterPod(oc, "default/labelname")
// note: it might get wrong pod which will be terminated during deployment rolling update
func getRouterPod(oc *exutil.CLI, icname string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller="+icname, "-o=jsonpath={.items[0].metadata.name}", "-n", "openshift-ingress").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of podname:%v", podName)
	return podName
}

// For collecting env details with grep from router pod [usage example: readRouterPodEnv(oc, podname, "search string")] .
// NOTE: This requires getRouterPod function to collect the podname variable first!
func readRouterPodEnv(oc *exutil.CLI, routername, envname string) string {
	ns := "openshift-ingress"
	output := readPodEnv(oc, routername, ns, envname)
	return output
}

// For collecting env details with grep [usage example: readPodEnv(oc, namespace, podname, "search string")]
func readPodEnv(oc *exutil.CLI, routername, ns string, envname string) string {
	cmd := fmt.Sprintf("/usr/bin/env | grep %s", envname)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, routername, "--", "bash", "-c", cmd).Output()
	if err != nil {
		output = "NotFound"
	}
	e2e.Logf("the matched Env are: %v", output)
	return output
}

// to check the route data in haproxy.config
// grepOptions can specify the lines of the context, e.g. "-A20" or "-C10"
// searchString2 is the config to be checked, since it might exists in multiple routes so use
// searchString1 to locate the specified route config
// after configuring the route the searchString2 need some time to be updated in haproxy.config so wait.Poll is required
func readHaproxyConfig(oc *exutil.CLI, routerPodName, searchString1, grepOption, searchString2 string) string {
	e2e.Logf("Polling and search haproxy config file")
	cmd1 := fmt.Sprintf("grep \"%s\" haproxy.config %s | grep \"%s\"", searchString1, grepOption, searchString2)
	cmd2 := fmt.Sprintf("grep \"%s\" haproxy.config %s", searchString1, grepOption)
	waitErr := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		_, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerPodName, "--", "bash", "-c", cmd1).Output()
		if err != nil {
			e2e.Logf("string not found, wait and try again...")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but config not found"))
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerPodName, "--", "bash", "-c", cmd2).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of haproxy.config that matching \"%s\" is: %v", searchString1, output)
	return output
}

// used to get block content of haproxy.conf, for example, get one route's whole backend's configuration specified by searchString(for exmpale: "be_edge_http:" + project1 + ":r1-edg")
func getBlockConfig(oc *exutil.CLI, routerPodName, searchString string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerPodName, "--", "bash", "-c", "cat haproxy.config").Output()
	o.Expect(err).NotTo(o.HaveOccurred(), "get the content of haproxy.config failed")
	result := ""
	flag := 0
	startIndex := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, searchString) {
			result = result + line + "\n"
			flag = 1
			startIndex = len(line) - len(strings.TrimLeft(line, " "))
		} else if flag == 1 {
			lineLen := len(line)
			if lineLen == 0 {
				result = result + "\n"
			} else {
				currentIndex := len(line) - len(strings.TrimLeft(line, " "))
				if currentIndex > startIndex {
					result = result + line + "\n"
				} else {
					flag = 2
				}
			}

		} else if flag == 2 {
			break
		}
	}
	return result
}

func regexpGet(target, searchString string, index int) string {
	result := ""
	if len(regexp.MustCompile(searchString).FindStringSubmatch(target)) > 0 {
		result = regexp.MustCompile(searchString).FindStringSubmatch(target)[index]
	}
	return result
}

// this function is used to get haproxy's version
func getHAProxyVersion(oc *exutil.CLI) string {
	var proxyVersion = "notFound"
	routerpod := getRouterPod(oc, "default")
	haproxyOutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "haproxy -v | grep version").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	haproxyRe := regexp.MustCompile("([0-9\\.]+)-([0-9a-z]+)")
	haproxyInfo := haproxyRe.FindStringSubmatch(haproxyOutput)
	if len(haproxyInfo) > 0 {
		proxyVersion = haproxyInfo[0]
	}
	return proxyVersion
}

func getHAProxyRPMVersion(oc *exutil.CLI) string {
	routerpod := getRouterPod(oc, "default")
	haproxyOutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "rpm -qa | grep haproxy").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return haproxyOutput
}

func getImagePullSpecFromPayload(oc *exutil.CLI, image string) string {
	var pullspec string
	baseDir := exutil.FixturePath("testdata", "router")
	indexTmpPath := filepath.Join(baseDir, getRandomString())
	dockerconfigjsonpath := filepath.Join(indexTmpPath, ".dockerconfigjson")
	defer exec.Command("rm", "-rf", indexTmpPath).Output()
	err := os.MkdirAll(indexTmpPath, 0755)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--confirm", "--to="+indexTmpPath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	pullspec, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--image-for="+image, "-a", dockerconfigjsonpath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the pull spec of image %v is: %v", image, pullspec)
	return pullspec
}

func (ipf *ipfailoverDescription) create(oc *exutil.CLI, ns string) {
	// create ServiceAccount and add it to related SCC
	_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", "ipfailover", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "ipfailover").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	// create the ipfailover deployment
	err = createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ipf.template, "-p", "NAME="+ipf.name, "NAMESPACE="+ipf.namespace, "IMAGE="+ipf.image, "HAINTERFACE="+ipf.HAInterface)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func ensureLogsContainString(oc *exutil.CLI, ns, label, match string) {
	waitErr := wait.Poll(3*time.Second, 90*time.Second, func() (bool, error) {
		log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, "-l", label).Output()
		// for debugging only
		// e2e.Logf("the logs of labeled pods are: %v", log)
		if err != nil || log == "" {
			e2e.Logf("failed to get logs: %v, retrying...", err)
			return false, nil
		}
		if !strings.Contains(log, match) {
			e2e.Logf("cannot find the matched string in the logs, retrying...")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but cannot find the string in the logs."))
}

// This function will identify the master and backup pod of the ipfailover pods
func ensureIpfailoverMasterBackup(oc *exutil.CLI, ns string, podList []string) (string, string) {
	var masterPod, backupPod string
	// The sleep is given for the election process to finish
	time.Sleep(10 * time.Second)
	podLogs1, err1 := exutil.GetSpecificPodLogs(oc, ns, "", podList[0], "Entering")
	o.Expect(err1).NotTo(o.HaveOccurred())
	logList1 := strings.Split((strings.TrimSpace(podLogs1)), "\n")
	e2e.Logf("The first pod log's last line is:- %v", logList1[len(logList1)-1])
	podLogs2, err2 := exutil.GetSpecificPodLogs(oc, ns, "", podList[1], "Entering")
	o.Expect(err2).NotTo(o.HaveOccurred())
	logList2 := strings.Split((strings.TrimSpace(podLogs2)), "\n")
	e2e.Logf("The second pod log's last line is:- %v", logList2[len(logList2)-1])

	switch {
	// Checking whether the first pod is failover state master and second pod backup
	case strings.Contains(logList1[len(logList1)-1], "Entering MASTER STATE"):
		o.Expect(logList2[len(logList2)-1]).To(o.ContainSubstring("Entering BACKUP STATE"))
		masterPod = podList[0]
		backupPod = podList[1]
	// Checking whether the second pod is failover state master and first pod backup
	case strings.Contains(logList1[len(logList1)-1], "Entering BACKUP STATE"):
		o.Expect(logList2[len(logList2)-1]).To(o.ContainSubstring("Entering MASTER STATE"))
		masterPod = podList[1]
		backupPod = podList[0]
	default:
		e2e.Failf("The pod is niether MASTER nor BACKUP and hence IPfailover didn't happened")
	}
	e2e.Logf("The Master pod is %v and Backup pod is %v", masterPod, backupPod)
	return masterPod, backupPod
}

// For collecting information from router pod [usage example: readRouterPodData(oc, podname, executeCmd, "search string")] .
// NOTE: This requires getRouterPod function to collect the podname variable first!
func readRouterPodData(oc *exutil.CLI, routername, executeCmd string, searchString string) string {
	output := readPodData(oc, routername, "openshift-ingress", executeCmd, searchString)
	return output
}

func createConfigMapFromFile(oc *exutil.CLI, ns, name, cmFile string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", name, "--from-file="+cmFile, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func deleteConfigMap(oc *exutil.CLI, ns, name string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", name, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// check if a configmap is created in specific namespace [usage: checkConfigMap(oc, namesapce, configmapName)]
func checkConfigMap(oc *exutil.CLI, ns, configmapName string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		searchOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", ns).Output()
		if err != nil {
			e2e.Logf("failed to get configmap: %v", err)
			return false, nil
		}
		if o.Expect(searchOutput).To(o.ContainSubstring(configmapName)) {
			e2e.Logf("configmap %v found", configmapName)
			return true, nil
		}
		return false, nil
	})
}

// To Collect ingresscontroller domain name
func getIngressctlDomain(oc *exutil.CLI, icname string) string {
	var ingressctldomain string
	ingressctldomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingresscontroller", icname, "--namespace=openshift-ingress-operator", "-o=jsonpath={.spec.domain}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the domain for the ingresscontroller is : %v", ingressctldomain)
	return ingressctldomain
}

// Function to deploy Edge route with default ceritifcates
func exposeRouteEdge(oc *exutil.CLI, ns, route, service, hostname string) {
	_, err := oc.WithoutNamespace().Run("create").Args("-n", ns, "route", "edge", route, "--service="+service, "--hostname="+hostname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// this function helps to get the ipv4 address of the given pod
func getPodv4Address(oc *exutil.CLI, podName, namespace string) string {
	podIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.status.podIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("IP of the %s pod in namespace %s is %q ", podName, namespace, podIPv4)
	return podIPv4
}

// this function will replace the octate of the ipaddress with the given value
func replaceIPOctet(ipaddress string, octet int, octetValue string) string {
	ipList := strings.Split(ipaddress, ".")
	ipList[octet] = octetValue
	vip := strings.Join(ipList, ".")
	e2e.Logf("The modified ipaddress is %s ", vip)
	return vip
}

// this function is to obtain the pod name based on the particular label
func getPodName(oc *exutil.CLI, namespace string, label string) []string {
	var podName []string
	podNameAll, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pod", "-l", label, "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podName = strings.Split(podNameAll, " ")
	e2e.Logf("The pod(s) are  %v ", podName)
	return podName
}

func getDNSPodName(oc *exutil.CLI) string {
	ns := "openshift-dns"
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pods", "-l", "dns.operator.openshift.io/daemonset-dns=default", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The DNS pod name is: %v", podName)
	return podName
}

// to read the Corefile content in DNS pod
// searchString is to locate the specified section since Corefile might has multiple zones
// that containing same config strings
// grepOptions can specify the lines of the context, e.g. "-A20" or "-C10"
func readDNSCorefile(oc *exutil.CLI, dnsPodName, searchString, grepOption string) string {
	ns := "openshift-dns"
	cmd := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s", searchString, grepOption)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of Corefile that matching \"%s\" is: %v", searchString, output)
	return output
}

// coredns introduced reload plugin to update the Corefile without receating dns-default pod
// similar to readHaproxyConfig(), use wait.Poll to wait the searchString2 to be updated.
// searchString1 can locate the specified zone section since Corefile might has multiple zones
// grepOptions can specify the lines of the context, e.g. "-A20" or "-C10"
// searchString2 is the config to be checked, it might exist in multiple zones so searchString1 is required
func pollReadDnsCorefile(oc *exutil.CLI, dnsPodName, searchString1, grepOption, searchString2 string) string {
	e2e.Logf("Polling and search dns Corefile")
	ns := "openshift-dns"
	cmd1 := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s | grep \"%s\"", searchString1, grepOption, searchString2)
	cmd2 := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s", searchString1, grepOption)

	waitErr := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		// trigger an immediately refresh configmap by updating pod's annotations
		hackAnnotatePod(oc, ns, dnsPodName)
		_, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", cmd1).Output()
		if err != nil {
			e2e.Logf("string not found, wait and try again...")
			return false, nil
		}
		return true, nil
	})
	// print all dns pods and one Corefile for debugging (normally the content is less than 20 lines)
	if waitErr != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", "dns.operator.openshift.io/daemonset-dns=default").Output()
		e2e.Logf("All current dns pods are:\n%v", output)
		output, _ = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", "cat /etc/coredns/Corefile").Output()
		e2e.Logf("The existing Corefile is: %v", output)
	}
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but Corefile is not updated"))
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", cmd2).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of Corefile that matching \"%s\" is: %v", searchString1, output)
	return output
}

// to trigger the configmap refresh immediately
// see https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#mounted-configmaps-are-updated-automatically
func hackAnnotatePod(oc *exutil.CLI, ns, podName string) {
	hackAnnotation := "ne-testing-hack=" + getRandomString()
	oc.AsAdmin().WithoutNamespace().Run("annotate").Args("pod", podName, "-n", ns, hackAnnotation, "--overwrite").Execute()
}

// this function get all cluster's operators
func getClusterOperators(oc *exutil.CLI) []string {
	outputOps, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperators", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	opList := strings.Split(outputOps, " ")
	return opList
}

// wait for "Progressing" is True
func ensureClusterOperatorProgress(oc *exutil.CLI, coName string) {
	e2e.Logf("waiting for CO %v to start rolling update......", coName)
	jsonPath := "-o=jsonpath={.status.conditions[?(@.type==\"Progressing\")].status}"
	waitErr := wait.PollImmediate(6*time.Second, 180*time.Second, func() (bool, error) {
		status, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/"+coName, jsonPath).Output()
		primary := false
		if strings.Compare(status, "True") == 0 {
			e2e.Logf("Progressing status is True.")
			primary = true
		} else {
			e2e.Logf("Progressing status is not True, wait and try again...")
		}
		return primary, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf(
		"reached max time allowed but CO %v didn't goto Progressing status.", coName))
}

// wait for the cluster operator back to normal status ("True False False")
// wait until get the specified number of successive normal status, which is defined by healthyThreshold and totalWaitTime
// healthyThreshold: max rounds for checking an CO,  int type,and no less than 1
// totalWaitTime: total checking time, time.Durationshould type, and no less than 1
func ensureClusterOperatorNormal(oc *exutil.CLI, coName string, healthyThreshold int, totalWaitTime time.Duration) {
	count := 0
	printCount := 0
	jsonPath := "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"

	e2e.Logf("waiting for CO %v back to normal status......", coName)
	waitErr := wait.Poll(5*time.Second, totalWaitTime*time.Second, func() (bool, error) {
		status, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/"+coName, jsonPath).Output()
		primary := false
		printCount++
		if strings.Compare(status, "TrueFalseFalse") == 0 {
			count++
			if count == healthyThreshold {
				e2e.Logf("got %v successive good status (%v), the CO is stable!", count, status)
				primary = true
			} else {
				e2e.Logf("got %v successive good status (%v), try again...", count, status)
			}
		} else {
			count = 0
			if printCount%10 == 1 {
				e2e.Logf("CO status is still abnormal (%v), wait and try again...", status)
			}
		}
		return primary, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but CO %v is still abnoraml.", coName))
}

// this function ensure all cluster's operators become normal
func ensureAllClusterOperatorsNormal(oc *exutil.CLI, waitTime time.Duration) {
	opList := getClusterOperators(oc)
	for _, operator := range opList {
		ensureClusterOperatorNormal(oc, operator, 1, waitTime)
	}
}

// this function pick up those cluster operators in bad status
func checkAllClusterOperatorsStatus(oc *exutil.CLI) []string {
	badOpList := []string{}
	opList := getClusterOperators(oc)
	jsonPath := "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"
	for _, operator := range opList {
		searchLine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator", operator, jsonPath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(searchLine, "TrueFalseFalse") {
			badOpList = append(badOpList, operator)
		}
	}
	return badOpList
}

// to ensure DNS rolling upgrade is done after updating the global resource "dns.operator/default"
// 1st, co/dns go to Progressing status
// 2nd, co/dns is back to normal and stable
func ensureDNSRollingUpdateDone(oc *exutil.CLI) {
	ensureClusterOperatorNormal(oc, "dns", 5, 300)
}

// this function is to get all linux nodes on which coredns pods can land, for windows nodes, there won't be any coredns pods on them
func getAllLinuxNodes(oc *exutil.CLI) string {
	allLinuxNodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "kubernetes.io/os=linux", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return allLinuxNodes
}

// to speed up the dns/coredns testing, just force only one dns-default pod in the cluster during the test
// find random linux node and add label "ne-dns-testing=true" to it, then patch spec.nodePlacement.nodeSelector
// please use func deleteDnsOperatorToRestore() for clear up.
func forceOnlyOneDnsPodExist(oc *exutil.CLI) string {
	ns := "openshift-dns"
	dnsPodLabel := "dns.operator.openshift.io/daemonset-dns=default"
	dnsNodeSelector := "[{\"op\":\"replace\", \"path\":\"/spec/nodePlacement/nodeSelector\", \"value\":{\"ne-dns-testing\":\"true\"}}]"
	// ensure no node with the label "ne-dns-testing=true"
	oc.AsAdmin().WithoutNamespace().Run("label").Args("node", "-l", "ne-dns-testing=true", "ne-dns-testing-").Execute()
	podList := getAllDNSPodsNames(oc)
	if len(podList) == 1 {
		e2e.Logf("Found only one dns-default pod and it looks like SNO cluster. Continue the test...")
	} else {
		dnsPodName := getRandomDNSPodName(podList)
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", dnsPodName, "-o=jsonpath={.spec.nodeName}", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Find random dns pod '%s' and its node '%s' which will be used for the following testing", dnsPodName, nodeName)
		// add special label "ne-dns-testing=true" to the node and force only one dns pod running on it
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nodeName, "ne-dns-testing=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchGlobalResourceAsAdmin(oc, "dnses.operator.openshift.io/default", dnsNodeSelector)
		err1 := waitForResourceToDisappear(oc, ns, "pod/"+dnsPodName)
		if err1 != nil {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", dnsPodLabel).Output()
			e2e.Logf("All current dns pods are:\n%v", output)
		}
		exutil.AssertWaitPollNoErr(err1, fmt.Sprintf("max time reached but pod %s is not terminated", dnsPodName))
		err2 := waitForPodWithLabelReady(oc, ns, dnsPodLabel)
		if err2 != nil {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", dnsPodLabel).Output()
			e2e.Logf("All current dns pods are:\n%v", output)
		}
		exutil.AssertWaitPollNoErr(err2, fmt.Sprintf("max time reached but no dns pod ready"))
	}
	return getDNSPodName(oc)
}

func deleteDnsOperatorToRestore(oc *exutil.CLI) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("dnses.operator.openshift.io/default").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	ensureClusterOperatorNormal(oc, "dns", 2, 120)
	// remove special label "ne-dns-testing=true" from the node
	oc.AsAdmin().WithoutNamespace().Run("label").Args("node", "-l", "ne-dns-testing=true", "ne-dns-testing-").Execute()
}

func waitAllDNSPodsAppear(oc *exutil.CLI) {
	for _, nodeName := range strings.Split(getAllLinuxNodes(oc), " ") {
		waitErr := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
			landedNodes, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-dns", "pods", "-l", "dns.operator.openshift.io/daemonset-dns=default", "-o=jsonpath={.items[*].spec.nodeName}").Output()
			if strings.Contains(landedNodes, nodeName) {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the desired dns pod on node "+nodeName+"does not appear"))
	}
	for _, podName := range getAllDNSPodsNames(oc) {
		waitForOutput(oc, "openshift-dns", "pod/"+podName, ".status.phase", "Running")
	}
}

// this function is to get all dns pods' names, the return is the string slice of all dns pods' names, together with an error
func getAllDNSPodsNames(oc *exutil.CLI) []string {
	podList := []string{}
	outputPods, err := oc.AsAdmin().Run("get").Args("pods", "-n", "openshift-dns").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podsRe := regexp.MustCompile("dns-default-[a-z0-9]+")
	pods := podsRe.FindAllStringSubmatch(outputPods, -1)
	if len(pods) > 0 {
		for i := 0; i < len(pods); i++ {
			podList = append(podList, pods[i][0])
		}
	} else {
		o.Expect(errors.New("Can't find a dns pod")).NotTo(o.HaveOccurred())
	}
	return podList
}

// this function is to select a dns pod randomly
func getRandomDNSPodName(podList []string) string {
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	index := seed.Intn(len(podList))
	return podList[index]
}

// this function is to check whether the given resource pod's are deleted or not
func waitForRangeOfResourceToDisappear(oc *exutil.CLI, resource string, podList []string) {
	for _, podName := range podList {
		err := waitForResourceToDisappear(oc, resource, "pod/"+podName)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s pod %s is NOT deleted", resource, podName))
	}
}

// this function is to wait for the expStr appearing in the corefile of the coredns under all dns pods
func keepSearchInAllDNSPods(oc *exutil.CLI, podList []string, expStr string) {
	cmd := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile", expStr)
	o.Expect(podList).NotTo(o.BeEmpty())
	for _, podName := range podList {
		count := 0
		waitErr := wait.Poll(15*time.Second, 360*time.Second, func() (bool, error) {
			output, _ := oc.AsAdmin().Run("exec").Args("-n", "openshift-dns", podName, "-c", "dns", "--", "bash", "-c", cmd).Output()
			count++
			primary := false
			if strings.Contains(output, expStr) {
				e2e.Logf("find " + expStr + " in the Corefile of pod " + podName)
				primary = true
			} else {
				// reduce the logs
				if count%2 == 1 {
					e2e.Logf("can't find " + expStr + " in the Corefile of pod " + podName + ", wait and try again...")
				}
			}
			return primary, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "can't find "+expStr+" in the Corefile of pod "+podName)
	}
}

// this function is to get desired logs from all dns pods
func searchLogFromDNSPods(oc *exutil.CLI, podList []string, searchStr string) string {
	o.Expect(podList).NotTo(o.BeEmpty())
	for _, podName := range podList {
		output, _ := oc.AsAdmin().Run("logs").Args(podName, "-c", "dns", "-n", "openshift-dns").Output()
		outputList := strings.Split(output, "\n")
		for _, line := range outputList {
			if strings.Contains(line, searchStr) {
				return line
			}
		}
	}
	return "none"
}

// this function is to wait the dns logs appearing by using searchLogFromDNSPods function repeatly
func waitDNSLogsAppear(oc *exutil.CLI, podList []string, searchStr string) string {
	result := "none"
	err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		result = searchLogFromDNSPods(oc, podList, searchStr)
		primary := false
		if result != "none" {
			primary = true
		}
		return primary, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("expected string \"%s\" is not found in the dns logs", searchStr))
	return result
}

func waitRouterLogsAppear(oc *exutil.CLI, routerpod, searchStr string) string {
	result := ""
	err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(routerpod, "-c", "logs", "-n", "openshift-ingress").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		primary := false
		outputList := strings.Split(output, "\n")
		for _, line := range outputList {
			if strings.Contains(line, searchStr) {
				primary = true
				result = line
				e2e.Logf("the searchline has result:%v", line)
				break
			}
		}
		return primary, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("expected string \"%s\" is not found in the router pod's logs", searchStr))
	return result
}

// this function to get one dns pod's Corefile info related to the modified time, it looks like {{"dns-default-0001", "2021-12-30 18.011111 Modified"}}
func getOneCorefileStat(oc *exutil.CLI, dnspodname string) [][]string {
	attrList := [][]string{}
	cmd := "stat /etc/coredns/..data/Corefile | grep Modify"
	output, err := oc.AsAdmin().Run("exec").Args("-n", "openshift-dns", dnspodname, "-c", "dns", "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return append(attrList, []string{dnspodname, output})
}

// replace the coredns image that specified by co/dns, currently only for replacement of coreDNS-pod.yaml
func replaceCoreDnsImage(oc *exutil.CLI, file string) {
	coreDnsImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/dns", "-o=jsonpath={.status.versions[?(.name == \"coredns\")].version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	result, err := exec.Command("bash", "-c", fmt.Sprintf(`grep "image: " %s`, file)).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of grep command is: %s", result)
	if strings.Contains(string(result), coreDnsImage) {
		e2e.Logf("the image has been updated, no action and continue")
	} else {
		// use "|" as delimiter here since the image looks like
		// "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:xxxxx"
		sedCmd := fmt.Sprintf(`sed -i'' -e 's|replaced-at-runtime|%s|g' %s`, coreDnsImage, file)
		_, err := exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// this fucntion will return the master pod who has the virtual ip
func getVipOwnerPod(oc *exutil.CLI, ns string, podname []string, vip string) string {
	cmd := fmt.Sprintf("ip address |grep %s", vip)
	var primaryNode string
	for i := 0; i < len(podname); i++ {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, podname[i], "--", "bash", "-c", cmd).Output()
		if len(podname) == 1 && output == "command terminated with exit code 1" {
			e2e.Failf("The given pod is not master")
		}
		if output == "command terminated with exit code 1" {
			e2e.Logf("This Pod %v does not have the VIP", podname[i])
		} else if o.Expect(output).To(o.ContainSubstring(vip)) {
			e2e.Logf("The pod owning the VIP is %v", podname[i])
			primaryNode = podname[i]
			break
		} else {
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	}
	return primaryNode
}

// this function will remove the given element from the slice
func slicingElement(element string, podList []string) []string {
	var newPodList []string
	for index, pod := range podList {
		if pod == element {
			newPodList = append(podList[:index], podList[index+1:]...)
			break
		}
	}
	e2e.Logf("The remaining pod/s in the list is %v", newPodList)
	return newPodList
}

// this function checks whether given pod becomes primary
func waitForPreemptPod(oc *exutil.CLI, ns string, pod string, vip string) {
	cmd := fmt.Sprintf("ip address |grep %s", vip)
	waitErr := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, pod, "--", "bash", "-c", cmd).Output()
		primary := false
		if o.Expect(output).To(o.ContainSubstring(vip)) {
			e2e.Logf("The new pod %v preempt to become Primary", pod)
			primary = true
		} else {
			e2e.Logf("pod failed to become Primary yet, retrying...", output)
		}
		return primary, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached, pod failed to become Primary"))
}

// this function will search the specific data from the given pod
func readPodData(oc *exutil.CLI, podname string, ns string, executeCmd string, searchString string) string {
	cmd := fmt.Sprintf("%s | grep \"%s\"", executeCmd, searchString)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, podname, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the matching part is: %s", output)
	return output
}

// this function is a wrapper for polling `readPodData` function
func pollReadPodData(oc *exutil.CLI, ns, routername, executeCmd, searchString string) string {
	cmd := fmt.Sprintf("%s | grep \"%s\"", executeCmd, searchString)
	var output string
	var err error
	waitErr := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		output, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, routername, "--", "bash", "-c", cmd).Output()
		if err != nil {
			e2e.Logf("failed to get search string: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	e2e.Logf("the matching part is: %s", output)
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but cannot find the search string."))
	return output
}

// this function create external dns operator
func createExternalDNSOperator(oc *exutil.CLI) {
	buildPruningBaseDir := exutil.FixturePath("testdata", "router", "extdns")
	operatorGroup := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	subscription := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	nsOperator := filepath.Join(buildPruningBaseDir, "ns-external-dns-operator.yaml")
	operatorNamespace := "external-dns-operator"

	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", nsOperator).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroup).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	// Deciding subscription need to be taken from which catalog
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
	if strings.Contains(output, "NotFound") {
		e2e.Logf("Warning: catalogsource/qe-app-registry is not installed, using redhat-operators instead")
		sedCmd := fmt.Sprintf(`sed -i'' -e 's/qe-app-registry/redhat-operators/g' %s`, subscription)
		_, err := exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subscription).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	// checking subscription status
	errCheck := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		subState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "external-dns-operator", "-n", operatorNamespace, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(subState, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription external-dns-operator is not correct status"))

	// checking csv status
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "external-dns-operator", "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
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

func deleteNamespace(oc *exutil.CLI, ns string) {
	err := oc.AdminKubeClient().CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		_, err = oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Namespace %s is not deleted in 3 minutes", ns))
}

// Get OIDC from STS cluster
func getOidc(oc *exutil.CLI) string {
	oidc, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("authentication.config", "cluster", "-o=jsonpath={.spec.serviceAccountIssuer}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	oidc = strings.TrimPrefix(oidc, "https://")
	e2e.Logf("The OIDC of STS cluster is: %v\n", oidc)
	return oidc
}

// this function create aws-load-balancer-operator
func createAWSLoadBalancerOperator(oc *exutil.CLI) {
	buildPruningBaseDir := exutil.FixturePath("testdata", "router", "awslb")
	operatorGroup := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	subscription := filepath.Join(buildPruningBaseDir, "subscription-src-qe.yaml")
	namespaceFile := filepath.Join(buildPruningBaseDir, "namespace.yaml")
	ns := "aws-load-balancer-operator"
	deployName := "deployment/aws-load-balancer-operator-controller-manager"

	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", namespaceFile).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	if exutil.IsSTSCluster(oc) {
		e2e.Logf("This is STS cluster, create ALB operator and controller secrets via AWS SDK")
		prepareAllForStsCluster(oc)
	}

	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroup).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	// if qe-app-registry is not installed then replace the source to redhat-operators
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
	if strings.Contains(output, "NotFound") {
		e2e.Logf("Warning: catalogsource/qe-app-registry is not installed, using redhat-operators instead")
		sedCmd := fmt.Sprintf(`sed -i'' -e 's/qe-app-registry/redhat-operators/g' %s`, subscription)
		_, err := exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subscription).Output()
	e2e.Logf("err %v, msg %v", err, msg)

	if exutil.IsSTSCluster(oc) {
		patchAlboSubscriptionWithRoleArn(oc, ns)
	}

	// checking subscription status
	errCheck := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		subState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "aws-load-balancer-operator", "-n", ns, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(subState, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("subscription aws-load-balancer-operator is not correct status"))

	// checking csv status
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "aws-load-balancer-operator", "-n", ns, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	errCheck = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", ns, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(csvState, "Succeeded") == 0 {
			e2e.Logf("CSV check complete!!!")
			return true, nil
		}
		return false, nil
	})
	// output log of deployment/aws-load-balancer-operator-controller-manager for debugging
	if errCheck != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args(deployName, "-n", ns, "--tail=10").Output()
		e2e.Logf("The logs of albo deployment: %v", output)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status", csvName))
}

func patchAlboSubscriptionWithRoleArn(oc *exutil.CLI, ns string) {
	e2e.Logf("patching the ALBO subcripton with Role ARN on STS cluster")
	jsonPatch := fmt.Sprintf("[{\"op\":\"add\",\"path\":\"/spec/config\",\"value\":{\"env\":[{\"name\":\"ROLEARN\",\"value\":%s}]}}]", os.Getenv("ALBO_ROLE_ARN"))
	_, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ns, "sub/aws-load-balancer-operator", "-p", jsonPatch, "--type=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func patchAlbControllerWithRoleArn(oc *exutil.CLI, ns string) {
	e2e.Logf("patching the ALB Controller with Role ARN on STS cluster")
	jsonPatch := fmt.Sprintf("[{\"op\":\"add\",\"path\":\"/spec/credentialsRequestConfig\",\"value\":{\"stsIAMRoleARN\":%s}}]", os.Getenv("ALBC_ROLE_ARN"))
	_, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", ns, "awsloadbalancercontroller/cluster", "-p", jsonPatch, "--type=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// get AWS outposts subnet so we can add annonation to ingress
func getOutpostSubnetId(oc *exutil.CLI) string {
	machineSet := exutil.GetOneOutpostMachineSet(oc)
	subnetId, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", machineSet, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.id}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the outpost subnet is %v", subnetId)
	return subnetId
}

// this function check if the load balancer provisioned
func waitForLoadBalancerProvision(oc *exutil.CLI, ns string, ingressName string) {
	waitErr := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "ingress", ingressName, "-o=jsonpath={.status.loadBalancer.ingress}").Output()
		if output != "" && strings.Contains(output, "k8s-") {
			e2e.Logf("The load balancer is provisoned: %v", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the Load Balancer is not provisioned"))
}

// curl command with poll
func waitForCurl(oc *exutil.CLI, podName, baseDomain string, routestring string, searchWord string, controllerIP string) {
	e2e.Logf("Polling for curl command")
	var output string
	var err error
	waitErr := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		if controllerIP != "" {
			route := routestring + baseDomain + ":80"
			toDst := routestring + baseDomain + ":80:" + controllerIP
			output, err = oc.Run("exec").Args(podName, "--", "curl", "-v", "http://"+route, "--resolve", toDst, "--connect-timeout", "10").Output()
		} else {
			curlCmd2 := routestring + baseDomain
			output, err = oc.Run("exec").Args(podName, "--", "curl", "-v", "http://"+curlCmd2, "--connect-timeout", "10").Output()
		}
		if err != nil {
			e2e.Logf("curl is not yet resolving, retrying...")
			return false, nil
		}
		if !strings.Contains(output, searchWord) {
			e2e.Logf("retrying...cannot find the searchWord '%s' in the output:- %v ", searchWord, output)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the route is not reachable"))
}

// used to send the nslookup command until the desired dns logs appear
func nslookupsAndWaitForDNSlog(oc *exutil.CLI, podName, searchLog string, dnsPodList []string, nslookupCmdPara ...string) string {
	e2e.Logf("Polling for executing nslookupCmd and waiting the dns logs appear")
	output := ""
	cmd := append([]string{podName, "--", "nslookup"}, nslookupCmdPara...)
	waitErr := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		oc.Run("exec").Args(cmd...).Execute()
		output = searchLogFromDNSPods(oc, dnsPodList, searchLog)
		primary := false
		if len(output) > 1 && output != "none" {
			primary = true
		}
		return primary, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached,but expected string \"%s\" is not found in the dns logs", searchLog))
	return output
}

// this function will get the route detail
func getRoutes(oc *exutil.CLI, ns string) string {
	output, err := oc.AsAdmin().Run("get").Args("route", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("oc get route: %v", output)
	return output
}

// Function to deploy passthough route with default ceritifcates
func exposeRoutePassth(oc *exutil.CLI, ns, route, service, hostname string) {
	_, err := oc.WithoutNamespace().Run("create").Args("-n", ns, "route", "passthrough", route, "--service="+service, "--hostname="+hostname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Function to deploy Reencrypt route  with service serving certificate:
// https://docs.openshift.com/container-platform/4.10/security/certificates/service-serving-certificate.html
// To be only with web-server-signed-rc.yaml pod template.
func exposeRouteReen(oc *exutil.CLI, ns, route, service, hostname string) {
	_, err := oc.WithoutNamespace().Run("create").Args("-n", ns, "route", "reencrypt", route, "--service="+service, "--hostname="+hostname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// this function will get the ingress detail
func getIngress(oc *exutil.CLI, ns string) string {
	output, err := oc.Run("get").Args("ingress", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("oc get ingress: %v", output)
	return output
}

// this function will help to create Opaque secret using cert file and its key_name
func createGenericSecret(oc *exutil.CLI, ns, name, keyName, certFile string) {
	cmd := fmt.Sprintf(`--from-file=%s=%v`, keyName, certFile)
	_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args(
		"secret", "generic", name, cmd, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// this function is to obtain the resource name like ingress's,route's name
func getResourceName(oc *exutil.CLI, namespace, resourceName string) []string {
	var resourceList []string
	resourceNames, err := oc.AsAdmin().Run("get").Args("-n", namespace, resourceName,
		"-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	resourceList = strings.Split(resourceNames, " ")
	e2e.Logf("The resource '%s' names are  %v ", resourceName, resourceList)
	return resourceList
}

// this function is used to check whether proxy is configured or not
func checkProxy(oc *exutil.CLI) bool {
	httpProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status.httpProxy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	httpsProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status.httpsProxy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if httpProxy != "" || httpsProxy != "" {
		return true
	}
	return false
}

// this function will advertise unicast peers for Nutanix
func unicastIPFailover(oc *exutil.CLI, ns, failoverName string) {
	platformtype := exutil.CheckPlatform(oc)

	if platformtype == "nutanix" || platformtype == "none" {
		getPodName(oc, oc.Namespace(), "ipfailover=hello-openshift")
		workerIPAddress, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-ojsonpath={.items[*].status.addresses[0].address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		modifiedIPList := strings.Split(workerIPAddress, " ")
		if len(modifiedIPList) < 2 {
			e2e.Failf("There is not enough IP addresses to add as unicast peer")
		}
		ipList := strings.Join(modifiedIPList, ",")
		cmd := fmt.Sprintf("OPENSHIFT_HA_UNICAST_PEERS=%v", ipList)
		setEnvVariable(oc, ns, "deploy/"+failoverName, "OPENSHIFT_HA_USE_UNICAST=true")
		setEnvVariable(oc, ns, "deploy/"+failoverName, cmd)
	}
}

// this function is to obtain the route details based on namespaces
func getNamespaceRouteDetails(oc *exutil.CLI, namespace, resourceName, jsonSearchString, matchString string, noMatchIfPresent bool) {
	e2e.Logf("polling for route details")
	waitErr := wait.Poll(5*time.Second, 150*time.Second, func() (bool, error) {
		resourceNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "-n", namespace, resourceName,
			"-ojsonpath={"+jsonSearchString+"}").Output()
		if err != nil {
			e2e.Logf("there is some execution error and it is  %v, retrying...", err)
			return false, nil
		}
		if noMatchIfPresent == true {
			if strings.Contains(resourceNames, matchString) {
				e2e.Logf("the matched string is still in the logs, retrying...")
				return false, nil
			}
		} else {
			if !strings.Contains(resourceNames, matchString) {
				e2e.Logf("cannot find the matched string in the logs, retrying...")
				return false, nil
			}
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the route details are not reachable"))
}

// this function is to make sure a command such as curl the route successfully, for the route isn't reachable occasionally
func repeatCmd(oc *exutil.CLI, cmd []string, expectOutput string, repeatTimes int) string {
	result := "failed"
	for i := 0; i < repeatTimes; i++ {
		output, _ := oc.Run("exec").Args(cmd...).Output()
		if strings.Contains(output, expectOutput) {
			result = "passed"
			break
		}
	}
	return result
}

func adminRepeatCmd(oc *exutil.CLI, cmd []string, expectOutput string, duration time.Duration) {
	waitErr := wait.Poll(5*time.Second, duration*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("exec").Args(cmd...).Output()
		if err != nil {
			e2e.Logf("failed to execute cmd %v successfully, retrying...", cmd)
			return false, nil
		}
		if strings.Contains(output, expectOutput) {
			return true, nil
		} else {
			return false, nil
		}

	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but can't execute the cmd successfully"))
}

// this function will collect all dns pods which resides in master node and its respective machine names
func getAllDNSAndMasterNodes(oc *exutil.CLI) ([]string, []string) {
	masterNodeList := []string{}
	dnsPodList := []string{}
	podNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "dns.operator.openshift.io/daemonset-dns=default", "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-dns").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podList := strings.Split(podNames, " ")
	e2e.Logf("The podname are %v", podList)
	masterRe := regexp.MustCompile("[a-z0-9-]+-master-+[a-z0-9-]+")
	if len(podList) > 0 {
		for i := 0; i < len(podList); i++ {
			nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", podList[i], "-o=jsonpath={.spec.nodeName}", "-n", "openshift-dns").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The dns pod '%s' is residing in '%s' node", podList[i], nodeName)
			machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", nodeName, "-o=jsonpath={.metadata.annotations.machine\\.openshift\\.io/machine}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			pod := masterRe.FindAllStringSubmatch(machineName, -1)
			if pod != nil {
				dnsPodList = append(dnsPodList, podList[i])
				masterNodeList = append(masterNodeList, pod[0][0])
			}
		}
	}
	e2e.Logf("The dns pods which are residing in master nodes are :%s", dnsPodList)
	e2e.Logf("The machine name's of master nodes where dns pods residing are :%s", masterNodeList)
	return dnsPodList, masterNodeList
}

// this function is to check whether given string is present or not in a list
func checkGivenStringPresentOrNot(shouldContain bool, iterateObject []string, searchString string) {
	if shouldContain {
		o.Expect(iterateObject).To(o.ContainElement(o.ContainSubstring(searchString)))
	} else {
		o.Expect(iterateObject).NotTo(o.ContainElement(o.ContainSubstring(searchString)))
	}
}

// this function check output of fetch command is polled
func waitForOutput(oc *exutil.CLI, ns, resourceName, searchString, value string) {
	waitErr := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		sourceRange := fetchJSONPathValue(oc, ns, resourceName, searchString)
		if strings.Contains(sourceRange, value) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the desired searchString does not appear"))
}

// this function keep checking util the searching for the regular expression matches
func waitForRegexpOutput(oc *exutil.CLI, ns, resourceName, searchString, regExpress string) string {
	result := "NotMatch"
	wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		sourceRange := fetchJSONPathValue(oc, ns, resourceName, searchString)
		searchRe := regexp.MustCompile(regExpress)
		searchInfo := searchRe.FindStringSubmatch(sourceRange)
		if len(searchInfo) > 0 {
			result = searchInfo[0]
			return true, nil
		}
		return false, nil
	})
	return result
}

// this function check the polled output of config map
func waitForConfigMapOutput(oc *exutil.CLI, ns, resourceName, searchString string) string {
	var output string
	var err error
	waitErr := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, resourceName, "-ojsonpath={"+searchString+"}").Output()
		if err != nil {
			e2e.Logf("failed to get search string: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but cannot find the search string."))
	return output
}

// this function search the polled output using label
func searchStringUsingLabel(oc *exutil.CLI, resource, label, searchString string) string {
	var output string
	var err error
	waitErr := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, "-l", label, "-ojsonpath={"+searchString+"}").Output()
		if err != nil || output == "" {
			e2e.Logf("failed to get output: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but cannot find the search string."))
	return output
}

// this function will search in the polled and described resource details
func searchInDescribeResource(oc *exutil.CLI, resource, resourceName, match string) string {
	var output string
	var err error
	waitErr := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args(resource, resourceName).Output()
		if err != nil || output == "" {
			e2e.Logf("failed to get describe output: %v, retrying...", err)
			return false, nil
		}
		if !strings.Contains(output, match) {
			e2e.Logf("cannot find the matched string in the output, retrying...")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but cannot find the search string."))
	return output
}

// this function is to add taint to resource
func addTaint(oc *exutil.CLI, resource, resourceName, taint string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", resource, resourceName, taint).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring(resource + "/" + resourceName + " tainted"))
}

// this function is to remove the configured taint
func deleteTaint(oc *exutil.CLI, resource, resourceName, taint string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", resource, resourceName, taint).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring(resource + "/" + resourceName + " untainted"))
}

func waitCoBecomes(oc *exutil.CLI, coName string, waitTime int, expectedStatus map[string]string) error {
	return wait.Poll(10*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		gottenStatus := getCoStatus(oc, coName, expectedStatus)
		eq := reflect.DeepEqual(expectedStatus, gottenStatus)
		if eq {
			e2e.Logf("Given operator %s becomes %s", coName, gottenStatus)
			return true, nil
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

// this function will check the status of dns record in ingress operator
func checkDnsRecordStatusOfIngressOperator(oc *exutil.CLI, dnsRecordsName, statusToSearch, stringToCheck string) []string {
	jsonPath := fmt.Sprintf(`.status.zones[*].conditions[*].%s`, statusToSearch)
	status := fetchJSONPathValue(oc, "openshift-ingress-operator", "dnsrecords/"+dnsRecordsName, jsonPath)
	statusList := strings.Split(status, " ")
	for _, line := range statusList {
		o.Expect(stringToCheck).To(o.ContainSubstring(line))
	}
	return statusList
}

// this function is to check whether the DNS Zone details are present in ingresss operator records
func checkDnsRecordsInIngressOperator(oc *exutil.CLI, recordName, privateZoneId, publicZoneId string) {
	// Collecting zone details from ingress operator
	Zones := fetchJSONPathValue(oc, "openshift-ingress-operator", "dnsrecords/"+recordName, ".status.zones[*].dnsZone")
	// check the private and public zone detail are matching
	o.Expect(Zones).To(o.ContainSubstring(privateZoneId))
	if publicZoneId != "" {
		o.Expect(Zones).To(o.ContainSubstring(publicZoneId))
	}
}

func checkIPStackType(oc *exutil.CLI) string {
	svcNetwork, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.serviceNetwork}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Count(svcNetwork, ":") >= 2 && strings.Count(svcNetwork, ".") >= 2 {
		return "dualstack"
	} else if strings.Count(svcNetwork, ":") >= 2 {
		return "ipv6single"
	} else if strings.Count(svcNetwork, ".") >= 2 {
		return "ipv4single"
	}
	return ""
}

// based on the orignal yaml file, this function is used to add some extra parameters behind the specified parameter, the return is the new file name
func addExtraParametersToYamlFile(originalFile, flagPara, AddedContent string) string {
	filePath, _ := filepath.Split(originalFile)
	newFile := filePath + getRandomString()
	originalFileContent, err := os.ReadFile(originalFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	newFileContent := ""
	for _, line := range strings.Split(string(originalFileContent), "\n") {
		newFileContent = newFileContent + line + "\n"
		if strings.Contains(line, flagPara) {
			newFileContent = newFileContent + AddedContent
		}
	}
	os.WriteFile(newFile, []byte(newFileContent), 0644)
	return newFile
}

// this function returns IPv6 and IPv4 on dual stack and main IP in case of single stack (v4 or v6)
func getPodIP(oc *exutil.CLI, namespace string, podName string) []string {
	ipStack := checkIPStackType(oc)
	var podIp []string
	if (ipStack == "ipv6single") || (ipStack == "ipv4single") {
		podIp1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIp1)
		podIp = append(podIp, podIp1)
	} else if ipStack == "dualstack" {
		podIp1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod's %s 1st IP in namespace %s is %q", podName, namespace, podIp1)
		podIp2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[1].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod's %s 2nd IP in namespace %s is %q", podName, namespace, podIp2)
		podIp = append(podIp, podIp1, podIp2)
	}
	return podIp
}
