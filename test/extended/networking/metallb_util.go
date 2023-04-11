package networking

import (
	"fmt"
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
	name      string
	namespace string
	template  string
}

type addressPoolResource struct {
	name      string
	namespace string
	protocol  string
	addresses []string
	template  string
}

type loadBalancerServiceResource struct {
	name                          string
	namespace                     string
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
	priority                  uint
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

var (
	snooze time.Duration = 720
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

	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", metallbcr.template, "-p", "NAME="+metallbcr.name, "NAMESPACE="+metallbcr.namespace)
	e2e.Logf("Error creating MetalLB CR %v", err)

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

func createAddressPoolCR(oc *exutil.CLI, addresspool addressPoolResource, addressPoolTemplate string) (status bool) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", addresspool.template, "-p", "NAME="+addresspool.name, "NAMESPACE="+addresspool.namespace, "PROTOCOL="+addresspool.protocol, "ADDRESS1="+addresspool.addresses[0], "ADDRESS2="+addresspool.addresses[1])
	if err != nil {
		e2e.Logf("Error creating addresspool %v", err)
		return false
	}
	return true

}

func createLoadBalancerService(oc *exutil.CLI, loadBalancerSvc loadBalancerServiceResource, loadBalancerServiceTemplate string) (status bool) {
	var msg string
	svcFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", loadBalancerSvc.template, "-p", "NAME="+loadBalancerSvc.name, "NAMESPACE="+loadBalancerSvc.namespace, "LABELKEY1="+loadBalancerSvc.labelKey, "LABELVALUE1="+loadBalancerSvc.labelValue,
		"EXTERNALTRAFFICPOLICY="+loadBalancerSvc.externaltrafficpolicy, "NODEPORTALLOCATION="+strconv.FormatBool(loadBalancerSvc.allocateLoadBalancerNodePorts)).OutputToFile(getRandomString() + "svc.json")
	g.By("Creating service file")
	if err != nil {
		e2e.Logf("Error creating LoadBalancerService %v with %v", err, svcFile)
		return false
	}

	g.By("Applying deployment file " + svcFile)
	msg, err = oc.AsAdmin().Run("apply").Args("-f", svcFile, "-n", loadBalancerSvc.namespace).Output()
	if err != nil {
		e2e.Logf("Could not apply svcFile %v %v", msg, err)
		return false
	}

	return true
}

func checkLoadBalancerSvcStatus(oc *exutil.CLI, namespace string, svcName string) error {

	return wait.Poll(20*time.Second, 120*time.Second, func() (bool, error) {
		e2e.Logf("Checking status of service %s", svcName)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.status.loadBalancer.ingress[0].ip}").Output()
		if err != nil {
			e2e.Logf("MetalLB failed to get service status, error:%s. Trying again", err)
			return false, nil
		}
		if strings.Contains(output, "Pending") {
			e2e.Logf("MetalLB failed to assign address to service, error:%s. Trying again", err)
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

func validateService(oc *exutil.CLI, nodeName string, svcExternalIP string) bool {
	e2e.Logf("Validating LoadBalancer service with IP %s", svcExternalIP)
	stdout, err := exutil.DebugNode(oc, nodeName, "curl", svcExternalIP, "--connect-timeout", "30")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(stdout).Should(o.ContainSubstring("Hello OpenShift!"))
	if err != nil {
		e2e.Logf("Error %s", err)
		return false
	}
	return true

}

func deleteMetalLBCR(oc *exutil.CLI, rs metalLBCRResource) {
	e2e.Logf("delete %s %s in namespace %s", "metallb", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("metallb", rs.name, "-n", rs.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func deleteAddressPool(oc *exutil.CLI, rs addressPoolResource) {
	e2e.Logf("delete %s %s in namespace %s", "addresspool", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("addresspool", rs.name, "-n", rs.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func obtainMACAddressForIP(oc *exutil.CLI, nodeName string, svcExternalIP string, arpReuests int) string {
	defInterface, intErr := getDefaultInterface(oc)
	o.Expect(intErr).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf("arping -I %s %s -c %d", defInterface, svcExternalIP, arpReuests)
	//https://issues.redhat.com/browse/OCPBUGS-10321 DebugNodeWithOptionsAndChroot replaced
	output, arpErr := exutil.DebugNodeWithOptions(oc, nodeName, []string{"-q"}, "bin/sh", "-c", cmd)
	o.Expect(arpErr).NotTo(o.HaveOccurred())
	e2e.Logf("ARP request response %s", output)
	re := regexp.MustCompile(`([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})`)
	var macAddress string
	if re.MatchString(output) {
		submatchall := re.FindAllString(output, -1)
		macAddress = submatchall[0]
	}
	return macAddress
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
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com") {
		return true

	}
	return false

}

func createIPAddressPoolCR(oc *exutil.CLI, ipAddresspool ipAddressPoolResource, addressPoolTemplate string) (status bool) {
	err := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipAddresspool.template, "-p", "NAME="+ipAddresspool.name, "NAMESPACE="+ipAddresspool.namespace, "PRIORITY="+strconv.Itoa(int(ipAddresspool.priority)), "ADDRESS1="+ipAddresspool.addresses[0], "ADDRESS2="+ipAddresspool.addresses[1],
		"NAMESPACE1="+ipAddresspool.namespaces[0], "NAMESPACE2="+ipAddresspool.namespaces[1],
		"MLSERVICEKEY1="+ipAddresspool.serviceLabelKey, "MLSERVICEVALUE1="+ipAddresspool.serviceLabelValue, "MESERVICEKEY1="+ipAddresspool.serviceSelectorKey, "MESERVICEOPERATOR1="+ipAddresspool.serviceSelectorOperator, "MESERVICEKEY1VALUE1="+ipAddresspool.serviceSelectorValue[0],
		"MLNAMESPACEKEY1="+ipAddresspool.serviceLabelKey, "MLNAMESPACEVALUE1="+ipAddresspool.serviceLabelValue, "MENAMESPACEKEY1="+ipAddresspool.namespaceSelectorKey, "MENAMESPACEOPERATOR1="+ipAddresspool.namespaceSelectorOperator, "MENAMESPACEKEY1VALUE1="+ipAddresspool.namespaceSelectorValue[0])
	if err != nil {
		e2e.Logf("Error creating ip addresspool %v", err)
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
		"IPADDRESSPOOL1="+l2advertisement.ipAddressPools[0], "INTERFACE1="+l2advertisement.interfaces[0], "INTERFACES2="+l2advertisement.interfaces[1], "INTERFACES3="+l2advertisement.interfaces[2],
		"WORKER1="+l2advertisement.nodeSelectorValues[0], "WORKER2="+l2advertisement.nodeSelectorValues[1])
	if err != nil {
		e2e.Logf("Error creating l2advertisement %v", err)
		return false
	}
	return true

}

func deleteL2Advertisement(oc *exutil.CLI, rs l2AdvertisementResource) {
	e2e.Logf("delete %s %s in namespace %s", "l2advertisement", rs.name, rs.namespace)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("l2advertisement", rs.name, "-n", rs.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getLoadBalancerSvcNodePort(oc *exutil.CLI, namespace string, svcName string) string {
	nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ports[0].nodePort}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return nodePort
}
