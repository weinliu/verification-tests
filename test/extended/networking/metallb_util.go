package networking

import (
	"context"
	"fmt"
	"net"
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
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

type subscriptionResource struct {
	name             string
	namespace        string
	operatorName     string
	channel          string
	catalog          string
	catalogNamespace string
	template         string
}
type namespaceResource struct {
	name     string
	template string
}
type operatorGroupResource struct {
	name             string
	namespace        string
	targetNamespaces string
	template         string
}

type metalLBCRResource struct {
	name                  string
	namespace             string
	nodeSelectorKey       string
	nodeSelectorVal       string
	controllerSelectorKey string
	controllerSelectorVal string
	template              string
}
type loadBalancerServiceResource struct {
	name                          string
	namespace                     string
	protocol                      string
	annotationKey                 string
	annotationValue               string
	labelKey                      string
	labelValue                    string
	externaltrafficpolicy         string
	allocateLoadBalancerNodePorts bool
	template                      string
}

type ipAddressPoolResource struct {
	name                      string
	namespace                 string
	label1                    string
	value1                    string
	priority                  int
	avoidBuggyIPs             bool
	autoAssign                bool
	addresses                 []string
	namespaces                []string
	serviceLabelKey           string
	serviceLabelValue         string
	serviceSelectorKey        string
	serviceSelectorOperator   string
	serviceSelectorValue      []string
	namespaceLabelKey         string
	namespaceLabelValue       string
	namespaceSelectorKey      string
	namespaceSelectorOperator string
	namespaceSelectorValue    []string
	template                  string
}
type l2AdvertisementResource struct {
	name                  string
	namespace             string
	interfaces            []string
	ipAddressPools        []string
	nodeSelectorsKey      string
	nodeSelectorsOperator string
	nodeSelectorValues    []string
	template              string
}
type bgpPeerResource struct {
	name          string
	namespace     string
	bfdProfile    string
	holdTime      string
	password      string
	keepAliveTime string
	myASN         int
	peerASN       int
	peerAddress   string
	template      string
}

type bgpAdvertisementResource struct {
	name                  string
	namespace             string
	communities           []string
	aggregationLength     int
	aggregationLengthV6   int
	ipAddressPools        []string
	nodeSelectorsKey      string
	nodeSelectorsOperator string
	nodeSelectorValues    []string
	peer                  []string
	template              string
}
type bfdProfileResource struct {
	name                 string
	namespace            string
	detectMultiplier     int
	echoMode             bool
	echoReceiveInterval  int
	echoTransmitInterval int
	minimumTtl           int
	passiveMode          bool
	receiveInterval      int
	transmitInterval     int
	template             string
}
type routerConfigMapResource struct {
	name         string
	namespace    string
	bgpd_enabled string
	bfdd_enabled string
	routerIP     string
	node1IP      string
	node2IP      string
	node3IP      string
	node4IP      string
	node5IP      string
	password     string
	bfdProfile   string
	template     string
}

type routerNADResource struct {
	name          string
	namespace     string
	interfaceName string
	template      string
}

type routerPodResource struct {
	name           string
	namespace      string
	configMapName  string
	NADName        string
	routerIP       string
	masterNodeName string
	template       string
}

var (
	snooze                 time.Duration = 720
	bgpRouterIP                          = "192.168.111.60/24"
	bgpRouterConfigMapName               = "router-master1-config"
	bgpRouterPodName                     = "router-master1"
	bgpRouterNamespace                   = "router-system"
	bgpRouterNADName                     = "external1"
)

func operatorInstall(oc *exutil.CLI, sub subscriptionResource, ns namespaceResource, og operatorGroupResource) (status bool) {
	//Installing Operator
	g.By(" (1) INSTALLING Operator in the namespace")

	//Applying the config of necessary yaml files from templates to create metallb operator
	g.By("(1.1) Applying namespace template")
	err0 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ns.template, "-p", "NAME="+ns.name)
	if err0 != nil {
		e2e.Logf("Error creating namespace %v", err0)
	}

	g.By("(1.2)  Applying operatorgroup yaml")
	err0 = applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace, "TARGETNAMESPACES="+og.targetNamespaces)
	if err0 != nil {
		e2e.Logf("Error creating operator group %v", err0)
	}

	g.By("(1.3) Creating subscription yaml from template")
	// no need to check for an existing subscription
	err0 = applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "OPERATORNAME="+sub.operatorName, "SUBSCRIPTIONNAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"CATALOGSOURCE="+sub.catalog, "CATALOGSOURCENAMESPACE="+sub.catalogNamespace)
	if err0 != nil {
		e2e.Logf("Error creating subscription %v", err0)
	}

	//confirming operator install
	g.By("(1.4) Verify the operator finished subscribing")
	errCheck := wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		subState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(subState, "AtLatestKnown") == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Subscription %s in namespace %v does not have expected status", sub.name, sub.namespace))

	g.By("(1.5) Get csvName")
	csvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(csvName).NotTo(o.BeEmpty())
	errCheck = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
		csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", sub.namespace, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(csvState, "Succeeded") == 0 {
			e2e.Logf("CSV check complete!!!")
			return true, nil

		}
		return false, nil

	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("CSV %v in %v namespace does not have expected status", csvName, sub.namespace))

	return true
}

func createMetalLBCR(oc *exutil.CLI, metallbcr metalLBCRResource, metalLBCRTemplate string) (status bool) {
	g.By("Creating MetalLB CR from template")

	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", metallbcr.template, "-p", "NAME="+metallbcr.name, "NAMESPACE="+metallbcr.namespace,
		"NODESELECTORKEY="+metallbcr.nodeSelectorKey, "NODESELECTORVAL="+metallbcr.nodeSelectorVal,
		"CONTROLLERSELECTORKEY="+metallbcr.controllerSelectorKey, "CONTROLLERSELECTORVAL="+metallbcr.controllerSelectorVal)
	if err != nil {
		e2e.Logf("Error creating MetalLB CR %v", err)
		return false
	}
	err = waitForPodWithLabelReady(oc, metallbcr.namespace, "component=speaker")
	exutil.AssertWaitPollNoErr(err, "The pods with label component=speaker are not ready")
	if err != nil {
		e2e.Logf("Speaker Pods did not transition to ready state %v", err)
		return false
	}
	err = waitForPodWithLabelReady(oc, metallbcr.namespace, "component=controller")
	exutil.AssertWaitPollNoErr(err, "The pod with label component=controller is not ready")
	if err != nil {
		e2e.Logf("Controller pod did not transition to ready state %v", err)
		return false
	}
	e2e.Logf("Controller and speaker pods created successfully")
	return true

}

func validateAllWorkerNodeMCR(oc *exutil.CLI, namespace string) bool {
	podList, err := exutil.GetAllPodsWithLabel(oc, namespace, "component=speaker")

	if err != nil {
		e2e.Logf("Unable to get list of speaker pods %s", err)
		return false
	}
	nodeList, err := exutil.GetClusterNodesBy(oc, "worker")
	if len(podList) != len(nodeList) {
		e2e.Logf("Speaker pods not scheduled on all worker nodes")
	}
	if err != nil {
		e2e.Logf("Unable to get nodes to determine if node is worker node  %s", err)
		return false
	}
	// Iterate over the speaker pods to validate they are scheduled on node that is worker node
	for _, pod := range podList {
		nodeName, _ := exutil.GetPodNodeName(oc, namespace, pod)
		e2e.Logf("Pod %s, node name %s", pod, nodeName)
		if isWorkerNode(oc, nodeName, nodeList) == false {
			return false
		}

	}
	return true

}

func isWorkerNode(oc *exutil.CLI, nodeName string, nodeList []string) bool {
	for i := 0; i <= (len(nodeList) - 1); i++ {
		if nodeList[i] == nodeName {
			return true
		}
	}
	return false

}

func createLoadBalancerService(oc *exutil.CLI, loadBalancerSvc loadBalancerServiceResource, loadBalancerServiceTemplate string) (status bool) {
	var msg, svcFile string
	var err error
	if strings.Contains(loadBalancerServiceTemplate, "annotated") {
		e2e.Logf("Template %s", loadBalancerServiceTemplate)
		svcFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", loadBalancerSvc.template, "-p", "NAME="+loadBalancerSvc.name, "NAMESPACE="+loadBalancerSvc.namespace,
			"PROTOCOL="+loadBalancerSvc.protocol,
			"LABELKEY1="+loadBalancerSvc.labelKey, "LABELVALUE1="+loadBalancerSvc.labelValue,
			"ANNOTATIONKEY="+loadBalancerSvc.annotationKey, "ANNOTATIONVALUE="+loadBalancerSvc.annotationValue,
			"EXTERNALTRAFFICPOLICY="+loadBalancerSvc.externaltrafficpolicy, "NODEPORTALLOCATION="+strconv.FormatBool(loadBalancerSvc.allocateLoadBalancerNodePorts)).OutputToFile(getRandomString() + "svc.json")
	} else {
		svcFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", loadBalancerSvc.template, "-p", "NAME="+loadBalancerSvc.name, "NAMESPACE="+loadBalancerSvc.namespace,
			"PROTOCOL="+loadBalancerSvc.protocol,
			"LABELKEY1="+loadBalancerSvc.labelKey, "LABELVALUE1="+loadBalancerSvc.labelValue,
			"EXTERNALTRAFFICPOLICY="+loadBalancerSvc.externaltrafficpolicy, "NODEPORTALLOCATION="+strconv.FormatBool(loadBalancerSvc.allocateLoadBalancerNodePorts)).OutputToFile(getRandomString() + "svc.json")
	}
	g.By("Creating service file")
	if err != nil {
		e2e.Logf("Error creating LoadBalancerService %v with %v", err, svcFile)
		return false
	}

	g.By("Applying service file " + svcFile)
	msg, err = oc.AsAdmin().Run("apply").Args("-f", svcFile, "-n", loadBalancerSvc.namespace).Output()
	if err != nil {
		e2e.Logf("Could not apply svcFile %v %v", msg, err)
		return false
	}

	return true
}

// statusCheckTime is interval and timeout in seconds e.g. 10 and 30
func checkLoadBalancerSvcStatus(oc *exutil.CLI, namespace string, svcName string, statusCheckTime ...time.Duration) error {
	interval := 10 * time.Second
	timeout := 120 * time.Second
	if len(statusCheckTime) > 0 {
		e2e.Logf("Interval %s, Timeout %s", statusCheckTime[0], statusCheckTime[1])
		interval = statusCheckTime[0]
		timeout = statusCheckTime[1]
	}
	return wait.Poll(interval, timeout, func() (bool, error) {
		e2e.Logf("Checking status of service %s", svcName)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.status.loadBalancer.ingress[0].ip}").Output()
		if err != nil {
			e2e.Logf("Failed to get service status, error:%s. Trying again", err)
			return false, nil
		}
		if strings.Contains(output, "<pending>") || output == "" {
			e2e.Logf("Failed to assign address to service, error:%s. Trying again", err)
			return false, nil
		}
		return true, nil

	})

}

func getLoadBalancerSvcIP(oc *exutil.CLI, namespace string, svcName string) string {
	svcIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.status.loadBalancer.ingress[0].ip}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("LoadBalancer service %s's, IP is :%s", svcName, svcIP)
	return svcIP
}

func validateService(oc *exutil.CLI, curlHost string, svcExternalIP string) bool {
	e2e.Logf("Validating service with IP %s", svcExternalIP)
	curlHostIP := net.ParseIP(curlHost)
	var curlOutput string
	var curlErr error
	connectTimeout := "5"
	if curlHostIP.To4() != nil {
		//From test runner with proxy
		var cmdOutput []byte
		svcChkCmd := fmt.Sprintf("curl -H 'Cache-Control: no-cache' -x 'http://%s:8888' %s --connect-timeout %s", curlHost, svcExternalIP, connectTimeout)
		cmdOutput, curlErr = exec.Command("bash", "-c", svcChkCmd).Output()
		curlOutput = string(cmdOutput)
	} else {
		curlOutput, curlErr = exutil.DebugNode(oc, curlHost, "curl", svcExternalIP, "--connect-timeout", connectTimeout)
	}

	if strings.Contains(curlOutput, "Hello OpenShift!") {
		return true
	}
	if curlErr != nil {
		e2e.Logf("Error %s", curlErr)
		return false
	}
	e2e.Logf("Output of curl %s", curlOutput)
	return false

}

func deleteMetalLBCR(oc *exutil.CLI, rs metalLBCRResource) {
	e2e.Logf("delete %s %s in namespace %s", "metallb", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("metallb", rs.name, "-n", rs.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func obtainMACAddressForIP(oc *exutil.CLI, nodeName string, svcExternalIP string, arpReuests int) (string, bool) {
	defInterface, intErr := getDefaultInterface(oc)
	o.Expect(intErr).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf("arping -I %s %s -c %d", defInterface, svcExternalIP, arpReuests)
	//https://issues.redhat.com/browse/OCPBUGS-10321 DebugNodeWithOptionsAndChroot replaced
	output, arpErr := exutil.DebugNodeWithOptions(oc, nodeName, []string{"-q"}, "bin/sh", "-c", cmd)
	//CI run the command returns non-zero exit code from debug container
	if arpErr != nil {
		return "", false
	}
	e2e.Logf("ARP request response %s", output)
	re := regexp.MustCompile(`([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})`)
	var macAddress string
	if re.MatchString(output) {
		submatchall := re.FindAllString(output, -1)
		macAddress = submatchall[0]
		return macAddress, true
	} else {
		return "", false
	}
}

func getNodeAnnouncingL2Service(oc *exutil.CLI, svcName string, namespace string) string {
	fieldSelectorArgs := fmt.Sprintf("reason=nodeAssigned,involvedObject.kind=Service,involvedObject.name=%s", svcName)
	var nodeName string
	errCheck := wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
		var allEvents []string
		var svcEvents string
		svcEvents, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", namespace, "--field-selector", fieldSelectorArgs).Output()
		if err != nil {
			return false, nil
		}
		if !strings.Contains(svcEvents, "No resources found") {
			for _, index := range strings.Split(svcEvents, "\n") {
				if strings.Contains(index, "announcing from node") {
					e2e.Logf("Processing event service %s", index)
					re := regexp.MustCompile(`"([^\"]+)"`)
					event := re.FindString(index)
					allEvents = append(allEvents, event)
				}
			}
			nodeName = strings.Trim(allEvents[len(allEvents)-1], "\"")
			return true, nil
		}
		return false, nil

	})
	o.Expect(nodeName).NotTo(o.BeEmpty())
	o.Expect(errCheck).NotTo(o.HaveOccurred())
	return nodeName
}

func isPlatformSuitable(oc *exutil.CLI) bool {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
	if err != nil || !(strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
		g.Skip("This case will only run on rdu1/rdu2 cluster , skip for other envrionment!!!")
	}
	return true

}

func createIPAddressPoolCR(oc *exutil.CLI, ipAddresspool ipAddressPoolResource, addressPoolTemplate string) (status bool) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipAddresspool.template, "-p", "NAME="+ipAddresspool.name, "NAMESPACE="+ipAddresspool.namespace, "PRIORITY="+strconv.Itoa(int(ipAddresspool.priority)),
		"AUTOASSIGN="+strconv.FormatBool(ipAddresspool.autoAssign), "AVOIDBUGGYIPS="+strconv.FormatBool(ipAddresspool.avoidBuggyIPs),
		"ADDRESS1="+ipAddresspool.addresses[0], "ADDRESS2="+ipAddresspool.addresses[1], "NAMESPACE1="+ipAddresspool.namespaces[0], "NAMESPACE2="+ipAddresspool.namespaces[1],
		"MLSERVICEKEY1="+ipAddresspool.serviceLabelKey, "MLSERVICEVALUE1="+ipAddresspool.serviceLabelValue, "MESERVICEKEY1="+ipAddresspool.serviceSelectorKey, "MESERVICEOPERATOR1="+ipAddresspool.serviceSelectorOperator, "MESERVICEKEY1VALUE1="+ipAddresspool.serviceSelectorValue[0],
		"MLNAMESPACEKEY1="+ipAddresspool.serviceLabelKey, "MLNAMESPACEVALUE1="+ipAddresspool.serviceLabelValue, "MENAMESPACEKEY1="+ipAddresspool.namespaceSelectorKey, "MENAMESPACEOPERATOR1="+ipAddresspool.namespaceSelectorOperator, "MENAMESPACEKEY1VALUE1="+ipAddresspool.namespaceSelectorValue[0])
	if err != nil {
		e2e.Logf("Error creating IP Addresspool %v", err)
		return false
	}
	return true

}
func deleteIPAddressPool(oc *exutil.CLI, rs ipAddressPoolResource) {
	e2e.Logf("delete %s %s in namespace %s", "ipaddresspool", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ipaddresspool", rs.name, "-n", rs.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createL2AdvertisementCR(oc *exutil.CLI, l2advertisement l2AdvertisementResource, l2AdvertisementTemplate string) (status bool) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", l2advertisement.template, "-p", "NAME="+l2advertisement.name, "NAMESPACE="+l2advertisement.namespace,
		"IPADDRESSPOOL1="+l2advertisement.ipAddressPools[0], "INTERFACE1="+l2advertisement.interfaces[0], "INTERFACE2="+l2advertisement.interfaces[1], "INTERFACE3="+l2advertisement.interfaces[2],
		"WORKER1="+l2advertisement.nodeSelectorValues[0], "WORKER2="+l2advertisement.nodeSelectorValues[1])
	if err != nil {
		e2e.Logf("Error creating l2advertisement %v", err)
		return false
	}
	return true

}

func deleteL2Advertisement(oc *exutil.CLI, rs l2AdvertisementResource) {
	e2e.Logf("delete %s %s in namespace %s", "l2advertisement", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("l2advertisement", rs.name, "-n", rs.namespace, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getLoadBalancerSvcNodePort(oc *exutil.CLI, namespace string, svcName string) string {
	nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ports[0].nodePort}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return nodePort
}

func createConfigMap(oc *exutil.CLI, testDataDir string, namespace string, cmdArgs ...string) (status bool) {
	var bgpPassword string
	var bfdEnabled string
	var bfdProfile string

	//parse cmd arguments
	if len(cmdArgs) > 1 {
		e2e.Logf("BGP Password %s, BFD Status %s, BFD Profile %s", cmdArgs[0], cmdArgs[1], cmdArgs[2])
		bgpPassword = cmdArgs[0]
		bfdEnabled = cmdArgs[1]
		bfdProfile = cmdArgs[2]
	} else if len(cmdArgs) == 1 {
		bgpPassword = cmdArgs[0]
	}

	nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(len(nodeList.Items) >= 5).NotTo(o.BeFalse())

	var nodeIPs []string
	var nodeIP string

	for _, node := range nodeList.Items {
		nodeIP = getNodeIPv4(oc, namespace, node.Name)
		nodeIPs = append(nodeIPs, nodeIP)
	}

	frrMasterSingleStackConfigMapTemplate := filepath.Join(testDataDir, "frr-master-singlestack-configmap-template.yaml")
	frrMasterSingleStackConfigMap := routerConfigMapResource{
		name:         bgpRouterConfigMapName,
		namespace:    namespace,
		bgpd_enabled: "yes",
		bfdd_enabled: bfdEnabled,
		routerIP:     "192.168.111.60",
		node1IP:      nodeIPs[0],
		node2IP:      nodeIPs[1],
		node3IP:      nodeIPs[2],
		node4IP:      nodeIPs[3],
		node5IP:      nodeIPs[4],
		password:     bgpPassword,
		bfdProfile:   bfdProfile,
		template:     frrMasterSingleStackConfigMapTemplate,
	}

	errTemplate := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", frrMasterSingleStackConfigMap.template, "-p", "NAME="+frrMasterSingleStackConfigMap.name, "NAMESPACE="+frrMasterSingleStackConfigMap.namespace,
		"BGPD_ENABLED="+frrMasterSingleStackConfigMap.bgpd_enabled, "BFDD_ENABLED="+frrMasterSingleStackConfigMap.bfdd_enabled, "ROUTER_IP="+frrMasterSingleStackConfigMap.routerIP, "NODE1_IP="+frrMasterSingleStackConfigMap.node1IP,
		"NODE2_IP="+frrMasterSingleStackConfigMap.node2IP, "NODE3_IP="+frrMasterSingleStackConfigMap.node3IP, "NODE4_IP="+frrMasterSingleStackConfigMap.node4IP,
		"NODE5_IP="+frrMasterSingleStackConfigMap.node5IP, "BFD_PROFILE="+frrMasterSingleStackConfigMap.bfdProfile, "PASSWORD="+frrMasterSingleStackConfigMap.password)
	if errTemplate != nil {
		e2e.Logf("Error creating config map %v", errTemplate)
		return false
	}

	return true

}

func createNAD(oc *exutil.CLI, testDataDir string, namespace string) (status bool) {
	defInterface, intErr := getDefaultInterface(oc)
	o.Expect(intErr).NotTo(o.HaveOccurred())
	frrMasterSingleStackNADTemplate := filepath.Join(testDataDir, "frr-master-singlestack-nad-template.yaml")
	frrMasterSingleStackNAD := routerNADResource{
		name:          bgpRouterNADName,
		namespace:     namespace,
		interfaceName: defInterface,
		template:      frrMasterSingleStackNADTemplate,
	}
	errTemplate := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", frrMasterSingleStackNAD.template, "-p", "NAME="+frrMasterSingleStackNAD.name, "INTERFACE="+frrMasterSingleStackNAD.interfaceName, "NAMESPACE="+frrMasterSingleStackNAD.namespace)
	if errTemplate != nil {
		e2e.Logf("Error creating network attachment definition %v", errTemplate)
		return false
	}

	return true
}

func createRouterPod(oc *exutil.CLI, testDataDir string, namespace string) (status bool) {
	frrMasterSingleStackRouterPodTemplate := filepath.Join(testDataDir, "frr-master-singlestack-router-pod-template.yaml")
	NADName, errNAD := oc.AsAdmin().WithoutNamespace().Run("get").Args("network-attachment-definitions", "-n", namespace, "--no-headers", "-o=custom-columns=NAME:.metadata.name").Output()
	o.Expect(errNAD).NotTo(o.HaveOccurred())
	masterNode, errMaster := exutil.GetFirstMasterNode(oc)
	o.Expect(errMaster).NotTo(o.HaveOccurred())

	frrMasterSingleStackRouterPod := routerPodResource{
		name:           bgpRouterPodName,
		namespace:      namespace,
		configMapName:  bgpRouterConfigMapName,
		NADName:        NADName,
		routerIP:       bgpRouterIP,
		masterNodeName: masterNode,
		template:       frrMasterSingleStackRouterPodTemplate,
	}
	errTemplate := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", frrMasterSingleStackRouterPod.template, "-p", "NAME="+frrMasterSingleStackRouterPod.name, "NAMESPACE="+frrMasterSingleStackRouterPod.namespace,
		"CONFIG_MAP_NAME="+frrMasterSingleStackRouterPod.configMapName, "ROUTER_IP="+frrMasterSingleStackRouterPod.routerIP, "MASTER_NODENAME="+frrMasterSingleStackRouterPod.masterNodeName, "NAD_NAME="+frrMasterSingleStackRouterPod.NADName)
	if errTemplate != nil {
		e2e.Logf("Error creating router pod %v", errTemplate)
		return false
	}
	err := waitForPodWithLabelReady(oc, namespace, "name=router-pod")
	o.Expect(err).NotTo(o.HaveOccurred())

	return true
}

func setUpExternalFRRRouter(oc *exutil.CLI, bgpRouterNamespace string, cmdArgs ...string) (status bool) {
	testDataDir := exutil.FixturePath("testdata", "networking/metallb")
	g.By(" Create namespace")
	oc.CreateSpecifiedNamespaceAsAdmin(bgpRouterNamespace)
	exutil.SetNamespacePrivileged(oc, bgpRouterNamespace)

	g.By(" Create config map")
	o.Expect(createConfigMap(oc, testDataDir, bgpRouterNamespace, cmdArgs...)).To(o.BeTrue())

	g.By(" Create network attachment defiition")
	o.Expect(createNAD(oc, testDataDir, bgpRouterNamespace)).To(o.BeTrue())

	g.By(" Create FRR router pod on master")
	o.Expect(createRouterPod(oc, testDataDir, bgpRouterNamespace)).To(o.BeTrue())

	return true
}

func checkBGPSessions(oc *exutil.CLI, bgpRouterNamespace string) (status bool) {

	cmd := []string{"-n", bgpRouterNamespace, bgpRouterPodName, "--", "vtysh", "-c", "show bgp summary"}
	errCheck := wait.Poll(60*time.Second, 120*time.Second, func() (bool, error) {
		e2e.Logf("Checking status of BGP session")
		bgpSummaryOutput, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(cmd...).Output()
		o.Expect(bgpSummaryOutput).NotTo(o.BeEmpty())
		if err != nil {
			return false, nil
		}
		if strings.Contains(bgpSummaryOutput, "Active") {
			e2e.Logf("Failed to establish BGP session between router and speakers, Trying again")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, "Establishing BGP session between router and speakers timed out")
	e2e.Logf("BGP session established")
	return true

}

func createBGPPeerCR(oc *exutil.CLI, bgppeer bgpPeerResource) (status bool) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", bgppeer.template, "-p", "NAME="+bgppeer.name, "NAMESPACE="+bgppeer.namespace,
		"PASSWORD="+bgppeer.password, "KEEPALIVETIME="+bgppeer.keepAliveTime,
		"HOLDTIME="+bgppeer.holdTime, "MY_ASN="+strconv.Itoa(int(bgppeer.myASN)), "PEER_ASN="+strconv.Itoa(int(bgppeer.peerASN)), "PEER_IPADDRESS="+bgppeer.peerAddress)
	if err != nil {
		e2e.Logf("Error creating BGP Peer %v", err)
		return false
	}
	return true

}

func deleteBGPPeer(oc *exutil.CLI, rs bgpPeerResource) {
	e2e.Logf("Delete %s %s in namespace %s", "bgppeer", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("bgppeer", rs.name, "-n", rs.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createBGPAdvertisementCR(oc *exutil.CLI, bgpAdvertisement bgpAdvertisementResource) (status bool) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", bgpAdvertisement.template, "-p", "NAME="+bgpAdvertisement.name, "NAMESPACE="+bgpAdvertisement.namespace,
		"AGGREGATIONLENGTH="+strconv.Itoa(int(bgpAdvertisement.aggregationLength)), "AGGREGATIONLENGTHV6="+strconv.Itoa(int(bgpAdvertisement.aggregationLengthV6)),
		"IPADDRESSPOOL1="+bgpAdvertisement.ipAddressPools[0], "COMMUNITIES="+bgpAdvertisement.communities[0],
		"NODESLECTORKEY1="+bgpAdvertisement.nodeSelectorsKey, "NODESELECTOROPERATOR1="+bgpAdvertisement.nodeSelectorsOperator,
		"WORKER1="+bgpAdvertisement.nodeSelectorValues[0], "WORKER2="+bgpAdvertisement.nodeSelectorValues[1],
		"BGPPEER1="+bgpAdvertisement.peer[0])
	if err != nil {
		e2e.Logf("Error creating BGP advertisement %v", err)
		return false
	}
	return true

}
func deleteBGPAdvertisement(oc *exutil.CLI, rs bgpAdvertisementResource) {
	e2e.Logf("Delete %s %s in namespace %s", "bgpadvertisement", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("bgpadvertisement", rs.name, "-n", rs.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}
func checkServiceEvents(oc *exutil.CLI, svcName string, namespace string, reason string) (bool, string) {
	fieldSelectorArgs := fmt.Sprintf("reason=%s,involvedObject.kind=Service,involvedObject.name=%s", reason, svcName)
	result := false
	message := ""
	errCheck := wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
		var svcEvents string
		svcEvents, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", namespace, "--field-selector", fieldSelectorArgs).Output()
		if err != nil {
			return false, nil
		}
		if !strings.Contains(svcEvents, "No resources found") {
			for _, index := range strings.Split(svcEvents, "\n") {
				if strings.Contains(index, reason) {
					e2e.Logf("Processing event %s for service", index)
					if reason == "AllocationFailed" {
						messageString := strings.Split(index, ":")
						message = messageString[1]
					}
					result = true
				}
			}
			return true, nil
		}
		return false, nil

	})
	if errCheck != nil {
		return result, ""
	}
	return result, message
}

func checkLogLevelPod(oc *exutil.CLI, component string, opNamespace string, level string) (bool, string) {
	var podLogLevelOutput string
	var err error
	if component == "controller" {
		podLogLevelOutput, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("pod", "-n", opNamespace, "-l", "component=controller", "-ojson").Output()
		if err != nil {
			e2e.Logf("Failed to get pod details due to %v", err)
			return false, "Get request to get controller pod failed"
		}
	} else {
		speakerPodList, err := exutil.GetAllPodsWithLabel(oc, opNamespace, "component=speaker")
		if err != nil {
			e2e.Logf("Failed to get pod %v", err)
			return false, "Get request to get speaker pod failed"
		}
		if len(speakerPodList) == 0 {
			return false, "Speaker pod list is empty"

		}
		podLogLevelOutput, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("pod", speakerPodList[0], "-n", opNamespace, "-ojson").Output()
		if err != nil {
			e2e.Logf("Failed to get details of pod %s due to %v", speakerPodList[0], err)
			return false, "Get request to get log level of speaker pod failed"
		}
	}
	if podLogLevelOutput == "" {
		return false, fmt.Sprintf("Failed to get log level of %s pod", component)
	}
	if strings.Contains(podLogLevelOutput, "--log-level="+level) {
		return true, ""
	}
	return false, fmt.Sprintf("The log level %s not set for %s pod", level, component)

}

func checkPrometheusMetrics(oc *exutil.CLI, interval time.Duration, timeout time.Duration, pollImmediate bool, metrics string, matchExpected bool) (bool, error) {
	prometheusURL := "localhost:9090/api/v1/query?query=" + metrics
	var metricsOutput string
	var err error
	metricsErr := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, pollImmediate, func(ctx context.Context) (bool, error) {
		metricsOutput, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "--", "curl", prometheusURL).Output()
		if err != nil {
			e2e.Logf("Could not get metrics %s status and trying again, the error is:%v", metrics, err)
			return false, nil
		}
		if matchExpected && !strings.Contains(metricsOutput, metrics) {
			return false, nil
		}
		if !matchExpected && strings.Contains(metricsOutput, metrics) {
			return false, nil
		}
		e2e.Logf("Metrics output %s", metricsOutput)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metricsErr, fmt.Sprintf("Failed to get metric status due to %v", metricsErr))
	return true, nil
}
func createBFDProfileCR(oc *exutil.CLI, bfdProfile bfdProfileResource) (status bool) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", bfdProfile.template, "-p", "NAME="+bfdProfile.name, "NAMESPACE="+bfdProfile.namespace,
		"DETECTMULTIPLIER="+strconv.Itoa(int(bfdProfile.detectMultiplier)), "ECHOMODE="+strconv.FormatBool(bfdProfile.echoMode),
		"ECHORECEIVEINTERVAL="+strconv.Itoa(int(bfdProfile.echoReceiveInterval)), "ECHOTRANSMITINTERVAL="+strconv.Itoa(int(bfdProfile.transmitInterval)),
		"MINIMUMTTL="+strconv.Itoa(int(bfdProfile.minimumTtl)), "PASSIVEMODE="+strconv.FormatBool(bfdProfile.passiveMode),
		"RECEIVEINTERVAL="+strconv.Itoa(int(bfdProfile.receiveInterval)), "TRANSMITINTERVAL="+strconv.Itoa(int(bfdProfile.transmitInterval)))
	if err != nil {
		e2e.Logf(fmt.Sprintf("Error creating BGP advertisement %v", err))
		return false
	}
	return true

}
