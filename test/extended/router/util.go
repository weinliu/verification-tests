package router

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
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
		if err != nil {
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
}

// Generic function to collect resource values with jsonpath option
func fetchJSONPathValue(oc *exutil.CLI, ns, resource, searchline string) string {
	searchLine, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, resource, "-o=jsonpath={"+searchline+"}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the searchline has result:%v", searchLine)
	return searchLine
}

// For collecting nodename on which the haproxy pod resides:
func getRouterNodeName(oc *exutil.CLI, icname string) string {
	podName := getRouterPod(oc, icname)
	e2e.Logf("The podname   is :%v", podName)
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", podName, "-o=jsonpath={.spec.nodeName}", "-n", "openshift-ingress").Output()
	e2e.Logf("The router residing  node  is :%s", nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
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
func getRouterPod(oc *exutil.CLI, icname string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller="+icname, "-o=jsonpath={.items[0].metadata.name}", "-n", "openshift-ingress").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of podname:%v", podName)
	return podName
}

// For collecting env details with grep from router pod [usage example: readDeploymentData(oc, podname, "search string")] .
// NOTE: This requires getRouterPod function to collect the podname variable first!
func readRouterPodEnv(oc *exutil.CLI, routername, envname string) string {
	ns := "openshift-ingress"
	output := readPodEnv(oc, routername, ns, envname)
	return output
}

// For collecting env details with grep [usage example: readDeploymentData(oc, namespace, podname, "search string")]
func readPodEnv(oc *exutil.CLI, routername, ns string, envname string) string {
	cmd := fmt.Sprintf("/usr/bin/env | grep %s", envname)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, routername, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
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

func ensureIpfailoverEnterMaster(oc *exutil.CLI, ns, label string) {
	ensureLogsContainString(oc, ns, label, "Entering MASTER STATE")
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

// this function will describe the given pod details
func describePod(oc *exutil.CLI, podName, namespace string) string {
	podDescribe, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-n", podName, namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podDescribe
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
func readDNSCorefile(oc *exutil.CLI, DNSPodName, searchString, grepOption string) string {
	ns := "openshift-dns"
	cmd := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s", searchString, grepOption)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, DNSPodName, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of Corefile that matching \"%s\" is: %v", searchString, output)
	return output
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
// wait until get 5 successive normal status to ensure it is stable
func ensureClusterOperatorNormal(oc *exutil.CLI, coName string) {
	jsonPath := "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"

	e2e.Logf("waiting for CO %v back to normal status......", coName)
	var count = 0
	waitErr := wait.Poll(6*time.Second, 300*time.Second, func() (bool, error) {
		status, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/"+coName, jsonPath).Output()
		primary := false
		if strings.Compare(status, "TrueFalseFalse") == 0 {
			count++
			if count == 5 {
				e2e.Logf("got %v successive good status (%v), the CO is stable!", count, status)
				primary = true
			} else {
				e2e.Logf("got %v successive good status (%v), try again...", count, status)
			}
		} else {
			count = 0
			e2e.Logf("CO status is still abnormal (%v), wait and try again...", status)
		}
		return primary, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but CO %v is still abnoraml.", coName))
}

// to ensure DNS rolling upgrade is done after updating the global resource "dns.operator/default"
// 1st, co/dns go to Progressing status
// 2nd, co/dns is back to normal and stable
func ensureDNSRollingUpdateDone(oc *exutil.CLI) {
	ensureClusterOperatorProgress(oc, "dns")
	ensureClusterOperatorNormal(oc, "dns")
}

// patch the dns.operator/default with the original value
func restoreDNSOperatorDefault(oc *exutil.CLI) {
	// the json value might be different in different version
	jsonPatch := "[{\"op\":\"replace\", \"path\":\"/spec\", \"value\":{\"cache\":{\"negativeTTL\":\"0s\",\"positiveTTL\":\"0s\"},\"logLevel\":\"Normal\",\"nodePlacement\":{},\"operatorLogLevel\":\"Normal\",\"upstreamResolvers\":{\"policy\":\"Sequential\",\"transportConfig\":{},\"upstreams\":[{\"port\":53,\"type\":\"SystemResolvConf\"}]}}}]"
	e2e.Logf("restore(patch) dns.operator/default with original settings.")
	output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("dns.operator/default", "-p", jsonPatch, "--type=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	// patched but got "no change" that means no DNS rolling update, shouldn't goto Progressing
	if strings.Contains(output, "no change") {
		e2e.Logf("skip the Progressing check step.")
	} else {
		delAllDNSPodsNoWait(oc)
		ensureClusterOperatorProgress(oc, "dns")
	}
	ensureClusterOperatorNormal(oc, "dns")
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

// this function is to delete all dns pods
func delAllDNSPods(oc *exutil.CLI) {
	podList := getAllDNSPodsNames(oc)
	o.Expect(podList).NotTo(o.BeEmpty())
	oc.AsAdmin().Run("delete").Args("pods", "-l", "dns.operator.openshift.io/daemonset-dns=default", "-n", "openshift-dns").Execute()
	waitForRangeOfResourceToDisappear(oc, "openshift-dns", podList)
}

// this function is to delete all dns pods without wait
func delAllDNSPodsNoWait(oc *exutil.CLI) {
	oc.AsAdmin().Run("delete").Args("pods", "-l", "dns.operator.openshift.io/daemonset-dns=default", "-n", "openshift-dns", "--wait=false").Execute()
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

// this function to get one dns pod's Corefile info related to the modified time, it looks like {{"dns-default-0001", "2021-12-30 18.011111 Modified"}}
func getOneCorefileStat(oc *exutil.CLI, dnspodname string) [][]string {
	attrList := [][]string{}
	cmd := "stat /etc/coredns/..data/Corefile | grep Modify"
	output, err := oc.AsAdmin().Run("exec").Args("-n", "openshift-dns", dnspodname, "-c", "dns", "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return append(attrList, []string{dnspodname, output})
}

// this function is to make sure all Corefiles(or one Corefile) of the dns pods are updated
// the value of parameter attrList should be from the getOneCorefileStat or getAllCorefilesStat function, it is related to the time before patching something to the dns operator
func waitAllCorefilesUpdated(oc *exutil.CLI, attrList [][]string) [][]string {
	cmd := "stat /etc/coredns/..data/Corefile | grep Modify"
	updatedAttrList := [][]string{}
	for _, dnspod := range attrList {
		dnspodname := dnspod[0]
		dnspodattr := dnspod[1]
		count := 0
		waitErr := wait.Poll(3*time.Second, 180*time.Second, func() (bool, error) {
			output, _ := oc.AsAdmin().Run("exec").Args("-n", "openshift-dns", dnspodname, "-c", "dns", "--", "bash", "-c", cmd).Output()
			count++
			primary := false
			if dnspodattr != output {
				e2e.Logf(dnspodname + " Corefile is updated")
				updatedAttrList = append(updatedAttrList, []string{dnspodname, output})
				primary = true
			} else {
				// reduce the logs
				if count%10 == 1 {
					e2e.Logf(dnspodname + " Corefile isn't updated , wait and try again...")
				}
			}
			return primary, nil
		})
		if waitErr != nil {
			updatedAttrList = append(updatedAttrList, []string{dnspodname, dnspodattr})
		}
		exutil.AssertWaitPollNoErr(waitErr, dnspodname+" Corefile isn't updated")
	}
	return updatedAttrList
}

// this function is to wait for Corefile(s) is updated
func waitCorefileUpdated(oc *exutil.CLI, attrList [][]string) [][]string {
	updatedAttrList := waitAllCorefilesUpdated(oc, attrList)
	return updatedAttrList
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

// this function create aws-load-balancer-operator
func createAWSLoadBalancerOperator(oc *exutil.CLI) {
	buildPruningBaseDir := exutil.FixturePath("testdata", "router", "awslb")
	credentials := filepath.Join(buildPruningBaseDir, "credentialsrequest.yaml")
	operatorGroup := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
	subscription := filepath.Join(buildPruningBaseDir, "subscription.yaml")
	namespaceFile := filepath.Join(buildPruningBaseDir, "namespace.yaml")
	ns := "aws-load-balancer-operator"

	msg, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", namespaceFile).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", credentials).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", operatorGroup).Output()
	e2e.Logf("err %v, msg %v", err, msg)
	msg, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", subscription).Output()
	e2e.Logf("err %v, msg %v", err, msg)

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
	errCheck = wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", ns, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(csvState, "Succeeded") == 0 {
			e2e.Logf("CSV check complete!!!")
			return true, nil
		}
		return false, nil
	})
	// output entire status of CSV for debugging
	if errCheck != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", ns, "-o=jsonpath={.status}").Output()
		e2e.Logf("The detailed output of CSV is: %v", output)
	}
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("csv %v is not correct status", csvName))
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
			output, err = oc.Run("exec").Args(podName, "--", "curl", "-v", "http://"+route, "--resolve", toDst).Output()
		} else {
			curlCmd2 := routestring + baseDomain
			output, err = oc.Run("exec").Args(podName, "--", "curl", "-v", "http://"+curlCmd2).Output()
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

// this function will get the route detail
func getRoutes(oc *exutil.CLI, ns string) string {
	output, err := oc.Run("get").Args("route", "-n", ns).Output()
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

	if platformtype == "nutanix" {
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
	waitErr := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		resourceNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "-n", namespace, resourceName,
			"-ojsonpath={"+jsonSearchString+"}").Output()
		if err != nil || resourceNames == "" {
			e2e.Logf("failed to get logs: %v, retrying...", err)
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
	output := "none"
	for i := 0; i < repeatTimes; i++ {
		output, _ = oc.Run("exec").Args(cmd...).Output()
		if strings.Contains(output, expectOutput) {
			result = "passed"
			break
		}
	}
	return result
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
