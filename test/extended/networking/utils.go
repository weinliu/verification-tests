package networking

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	ci "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfrastructure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type pingPodResource struct {
	name      string
	namespace string
	template  string
}

type pingPodResourceNode struct {
	name      string
	namespace string
	nodename  string
	template  string
}

type egressIPResource1 struct {
	name          string
	template      string
	egressIP1     string
	egressIP2     string
	nsLabelKey    string
	nsLabelValue  string
	podLabelKey   string
	podLabelValue string
}

type egressFirewall1 struct {
	name      string
	namespace string
	template  string
}

type egressFirewall2 struct {
	name      string
	namespace string
	ruletype  string
	cidr      string
	template  string
}

type ipBlock_ingress_dual struct {
	name      string
	namespace string
	cidr_ipv4 string
	cidr_ipv6 string
	template  string
}

type ipBlock_ingress_single struct {
	name      string
	namespace string
	cidr      string
	template  string
}

type genericServiceResource struct {
	servicename             string
	namespace               string
	protocol                string
	selector                string
	service_type            string
	ip_family_policy        string
	internal_traffic_policy string
	template                string
}

func (pod *pingPodResource) createPingPod(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (pod *pingPodResourceNode) createPingPodNode(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "NODENAME="+pod.nodename)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.Run("process").Args(parameters...).OutputToFile(getRandomString() + "ping-pod.json")
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

func (egressIP *egressIPResource1) createEgressIPObject1(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressIP.template, "-p", "NAME="+egressIP.name, "EGRESSIP1="+egressIP.egressIP1, "EGRESSIP2="+egressIP.egressIP2)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressIP %v", egressIP.name))
}

func (egressIP *egressIPResource1) deleteEgressIPObject1(oc *exutil.CLI) {
	removeResource(oc, true, true, "egressip", egressIP.name)
}

func (egressIP *egressIPResource1) createEgressIPObject2(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressIP.template, "-p", "NAME="+egressIP.name, "EGRESSIP1="+egressIP.egressIP1, "NSLABELKEY="+egressIP.nsLabelKey, "NSLABELVALUE="+egressIP.nsLabelValue, "PODLABELKEY="+egressIP.podLabelKey, "PODLABELVALUE="+egressIP.podLabelValue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressIP %v", egressIP.name))
}

func (egressFirewall *egressFirewall1) createEgressFWObject1(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressFirewall.template, "-p", "NAME="+egressFirewall.name, "NAMESPACE="+egressFirewall.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressFW %v", egressFirewall.name))
}

func (egressFirewall *egressFirewall1) deleteEgressFWObject1(oc *exutil.CLI) {
	removeResource(oc, true, true, "egressfirewall", egressFirewall.name, "-n", egressFirewall.namespace)
}

func (egressFirewall *egressFirewall2) createEgressFW2Object(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressFirewall.template, "-p", "NAME="+egressFirewall.name, "NAMESPACE="+egressFirewall.namespace, "RULETYPE="+egressFirewall.ruletype, "CIDR="+egressFirewall.cidr)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressFW2 %v", egressFirewall.name))
}

func (ipBlock_ingress_policy *ipBlock_ingress_dual) createipBlockIngressObjectDual(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_ingress_policy.template, "-p", "NAME="+ipBlock_ingress_policy.name, "NAMESPACE="+ipBlock_ingress_policy.namespace, "CIDR_IPv6="+ipBlock_ingress_policy.cidr_ipv6, "CIDR_IPv4="+ipBlock_ingress_policy.cidr_ipv4)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_ingress_policy.name))
}

func (ipBlock_ingress_policy *ipBlock_ingress_single) createipBlockIngressObjectSingle(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_ingress_policy.template, "-p", "NAME="+ipBlock_ingress_policy.name, "NAMESPACE="+ipBlock_ingress_policy.namespace, "CIDR="+ipBlock_ingress_policy.cidr)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_ingress_policy.name))
}

func (service *genericServiceResource) createServiceFromParams(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", service.template, "-p", "SERVICENAME="+service.servicename, "NAMESPACE="+service.namespace, "PROTOCOL="+service.protocol, "SELECTOR="+service.selector, "SERVICE_TYPE="+service.service_type, "IP_FAMILY_POLICY="+service.ip_family_policy, "INTERNAL_TRAFFIC_POLICY="+service.internal_traffic_policy)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", service.servicename))
}

func (egressFirewall *egressFirewall2) deleteEgressFW2Object(oc *exutil.CLI) {
	removeResource(oc, true, true, "egressfirewall", egressFirewall.name, "-n", egressFirewall.namespace)
}

func (pingPod *pingPodResource) deletePingPod(oc *exutil.CLI) {
	removeResource(oc, false, true, "pod", pingPod.name, "-n", pingPod.namespace)
}

func (pingPodNode *pingPodResourceNode) deletePingPodNode(oc *exutil.CLI) {
	removeResource(oc, false, true, "pod", pingPodNode.name, "-n", pingPodNode.namespace)
}

func removeResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) {
	output, err := doAction(oc, "delete", asAdmin, withoutNamespace, parameters...)
	if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
		e2e.Logf("the resource is deleted already")
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())

	err = wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
			e2e.Logf("the resource is delete successfully")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to delete resource %v", parameters))
}

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

func applyResourceFromTemplateByAdmin(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "resource.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.WithoutNamespace().AsAdmin().Run("apply").Args("-f", configFile).Execute()
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

func getPodStatus(oc *exutil.CLI, namespace string, podName string) (string, error) {
	podStatus, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status in namespace %s is %q", podName, namespace, podStatus)
	return podStatus, err
}

func checkPodReady(oc *exutil.CLI, namespace string, podName string) (bool, error) {
	podOutPut, err := getPodStatus(oc, namespace, podName)
	status := []string{"Running", "Ready", "Complete"}
	return contains(status, podOutPut), err
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func waitPodReady(oc *exutil.CLI, namespace string, podName string) {
	err := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		status, err1 := checkPodReady(oc, namespace, podName)
		if err1 != nil {
			e2e.Logf("the err:%v, wait for pod %v to become ready.", err1, podName)
			return status, err1
		}
		if !status {
			return status, nil
		}
		return status, nil
	})

	if err != nil {
		podDescribe := describePod(oc, namespace, podName)
		e2e.Logf("oc describe pod %v.", podName)
		e2e.Logf(podDescribe)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %v is not ready", podName))
}

func describePod(oc *exutil.CLI, namespace string, podName string) string {
	podDescribe, err := oc.WithoutNamespace().Run("describe").Args("pod", "-n", namespace, podName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status is %q", podName, podDescribe)
	return podDescribe
}

func execCommandInSpecificPod(oc *exutil.CLI, namespace string, podName string, command string) (string, error) {
	e2e.Logf("The command is: %v", command)
	command1 := []string{"-n", namespace, podName, "--", "bash", "-c", command}
	msg, err := oc.WithoutNamespace().Run("exec").Args(command1...).Output()
	if err != nil {
		e2e.Logf("Execute command failed with  err:%v  and output is %v.", err, msg)
		return msg, err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

func execCommandInNetworkingPod(oc *exutil.CLI, command string) (string, error) {
	networkType := checkNetworkType(oc)
	var cmd []string
	if strings.Contains(networkType, "ovn") {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-ovn-kubernetes", "-l", "app=ovnkube-node", "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			e2e.Logf("Cannot get onv-kubernetes pods, errors: %v", err)
			return "", err
		}
		cmd = []string{"-n", "openshift-ovn-kubernetes", "-c", "ovnkube-node", podName, "--", "/bin/sh", "-c", command}
	} else if strings.Contains(networkType, "sdn") {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-sdn", "-l", "app=sdn", "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			e2e.Logf("Cannot get openshift-sdn pods, errors: %v", err)
			return "", err
		}
		cmd = []string{"-n", "openshift-sdn", "-c", "sdn", podName, "--", "/bin/sh", "-c", command}
	}

	msg, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(cmd...).Output()
	if err != nil {
		e2e.Logf("Execute command failed with  err:%v .", err)
		return "", err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

func getDefaultInterface(oc *exutil.CLI) (string, error) {
	getDefaultInterfaceCmd := "/usr/sbin/ip -4 route show default"
	int1, err := execCommandInNetworkingPod(oc, getDefaultInterfaceCmd)
	if err != nil {
		e2e.Logf("Cannot get default interface, errors: %v", err)
		return "", err
	}
	defInterface := strings.Split(int1, " ")[4]
	e2e.Logf("Get the default inteface: %s", defInterface)
	return defInterface, nil
}

func getDefaultSubnet(oc *exutil.CLI) (string, error) {
	int1, _ := getDefaultInterface(oc)
	getDefaultSubnetCmd := "/usr/sbin/ip -4 -brief a show " + int1
	subnet1, err := execCommandInNetworkingPod(oc, getDefaultSubnetCmd)
	if err != nil {
		e2e.Logf("Cannot get default subnet, errors: %v", err)
		return "", err
	}
	defSubnet := strings.Fields(subnet1)[2]
	e2e.Logf("Get the default subnet: %s", defSubnet)
	return defSubnet, nil
}

func Hosts(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ips = append(ips, ip.String())
	}
	// remove network address and broadcast address
	return ips[1 : len(ips)-1], nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func findUnUsedIPs(oc *exutil.CLI, cidr string, number int) []string {
	ipRange, _ := Hosts(cidr)
	var ipUnused = []string{}
	//shuffle the ips slice
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(ipRange), func(i, j int) { ipRange[i], ipRange[j] = ipRange[j], ipRange[i] })
	for _, ip := range ipRange {
		if len(ipUnused) < number {
			pingCmd := "ping -c4 -t1 " + ip
			_, err := execCommandInNetworkingPod(oc, pingCmd)
			if err != nil {
				e2e.Logf("%s is not used!\n", ip)
				ipUnused = append(ipUnused, ip)
			}
		} else {
			break
		}

	}
	return ipUnused
}

func ipEchoServer() string {
	return "172.31.249.80:9095"
}

func checkPlatform(oc *exutil.CLI) string {
	output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
	return strings.ToLower(output)
}

func checkNetworkType(oc *exutil.CLI) string {
	output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.type}").Output()
	return strings.ToLower(output)
}

func getDefaultIPv6Subnet(oc *exutil.CLI) (string, error) {
	int1, _ := getDefaultInterface(oc)
	getDefaultSubnetCmd := "/usr/sbin/ip -6 -brief a show " + int1
	subnet1, err := execCommandInNetworkingPod(oc, getDefaultSubnetCmd)
	if err != nil {
		e2e.Logf("Cannot get default ipv6 subnet, errors: %v", err)
		return "", err
	}
	defSubnet := strings.Fields(subnet1)[2]
	e2e.Logf("Get the default ipv6 subnet: %s", defSubnet)
	return defSubnet, nil
}

func findUnUsedIPv6(oc *exutil.CLI, cidr string, number int) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	number += 2
	var ips []string
	var i = 0
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		//Not use the first two IPv6 addresses , such as 2620:52:0:4e::  , 2620:52:0:4e::1
		if i == 0 || i == 1 {
			i++
			continue
		}
		//Start to detect the IPv6 adress is used or not
		pingCmd := "ping -c4 -t1 -6 " + ip.String()
		_, err := execCommandInNetworkingPod(oc, pingCmd)
		if err != nil && i < number {
			e2e.Logf("%s is not used!\n", ip)
			ips = append(ips, ip.String())
		} else if i >= number {
			break
		}
		i++
	}

	return ips, nil
}

func ipv6EchoServer(isIPv6 bool) string {
	if isIPv6 {
		return "[2620:52:0:4974:def4:1ff:fee7:8144]:8085"
	} else {
		return "10.73.116.56:8085"
	}
}

func checkIpStackType(oc *exutil.CLI) string {
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

func installSctpModule(oc *exutil.CLI, configFile string) {
	status, _ := oc.AsAdmin().Run("get").Args("machineconfigs").Output()
	if !strings.Contains(status, "load-sctp-module") {
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func checkSctpModule(oc *exutil.CLI, nodeName string) {
	err := wait.Poll(30*time.Second, 15*time.Minute, func() (bool, error) {
		// Check nodes status to make sure all nodes are up after rebooting caused by load-sctp-module
		nodes_status, err := oc.AsAdmin().Run("get").Args("node").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("oc_get_nodes: %v", nodes_status)
		status, _ := oc.AsAdmin().Run("debug").Args("node/"+nodeName, "--", "cat", "/sys/module/sctp/initstate").Output()
		if strings.Contains(status, "live") {
			e2e.Logf("stcp module is installed in the %s", nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "stcp module is installed in the nodes")
}

func getPodIPv4(oc *exutil.CLI, namespace string, podName string) string {
	podIPv4, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv4)
	return podIPv4
}

func getPodIPv6(oc *exutil.CLI, namespace string, podName string, ipStack string) string {
	if ipStack == "ipv6single" {
		podIPv6, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6
	} else if ipStack == "dualstack" {
		podIPv6, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[1].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6
	}
	return ""
}

// For normal user to create resources in the specified namespace from the file (not template)
func createResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.WithoutNamespace().Run("create").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func waitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(15*time.Second, 10*time.Minute, func() (bool, error) {
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

func getSvcIPv4(oc *exutil.CLI, namespace string, svcName string) string {
	svcIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv4 in namespace %s is %q", svcName, namespace, svcIPv4)
	return svcIPv4
}

func getSvcIPv6(oc *exutil.CLI, namespace string, svcName string) string {
	svcIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv6 in namespace %s is %q", svcName, namespace, svcIPv6)
	return svcIPv6
}

func getSvcIPdualstack(oc *exutil.CLI, namespace string, svcName string) (string, string) {
	svcIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv4 in namespace %s is %q", svcName, namespace, svcIPv4)
	svcIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[1]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv6 in namespace %s is %q", svcName, namespace, svcIPv6)
	return svcIPv4, svcIPv6
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

func sshRunCmd(host string, user string, cmd string) error {
	privateKey := os.Getenv("SSH_CLOUD_PRIV_KEY")
	if privateKey == "" {
		privateKey = "../internal/config/keys/openshift-qe.pem"
	}
	sshClient := exutil.SshClient{User: user, Host: host, Port: 22, PrivateKey: privateKey}
	return sshClient.Run(cmd)
}

// For Admin to patch a resource in the specified namespace
func patchResourceAsAdmin(oc *exutil.CLI, resource, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type=merge").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

//Testing will exit when network operator is in abnormal state during 60 seconding of checking operator.
func checkNetworkOperatorDEGRADEDState(oc *exutil.CLI) {
	errCheck := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "network").Output()
		if err != nil {
			e2e.Logf("Fail to get clusteroperator network, error:%s. Trying again", err)
			return false, nil
		}
		matched, _ := regexp.MatchString("True.*False.*False", output)
		e2e.Logf("Network operator state is:%s", output)
		o.Expect(matched).To(o.BeTrue())
		return false, nil
	})
	o.Expect(errCheck.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
}

func getNodeIPv4(oc *exutil.CLI, namespace string, nodeName string) string {
	nodeipv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", oc.Namespace(), "node", nodeName, "-o=jsonpath={.status.addresses[0].address}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil {
		e2e.Logf("Cannot get node default interface ipv4 address, errors: %v", err)
	}
	e2e.Logf("The IPv4 of node's default interface is %q", nodeipv4)
	return nodeipv4
}

func getLeaderInfo(oc *exutil.CLI, namespace string, cmName string, networkType string) string {
	if networkType == "ovnkubernetes" {
		output1, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-n", namespace, "-o=jsonpath={.metadata.annotations.control-plane\\.alpha\\.kubernetes\\.io/leader}").OutputToFile("oc_describe_nodes.txt")
		o.Expect(err1).NotTo(o.HaveOccurred())
		output2, err2 := exec.Command("bash", "-c", "cat "+output1+" |  jq -r .holderIdentity").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		leaderNodeName := strings.Trim(strings.TrimSpace(string(output2)), "\"")
		e2e.Logf("The leader node name is %s", leaderNodeName)
		leaderNodeIP := getNodeIPv4(oc, namespace, leaderNodeName)
		e2e.Logf("The leader node's IP is: %v", leaderNodeIP)
		return leaderNodeIP
	} else {
		output1, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-n", namespace, "-o=jsonpath={.metadata.annotations.control-plane\\.alpha\\.kubernetes\\.io/leader}").OutputToFile("oc_describe_nodes.txt")
		o.Expect(err1).NotTo(o.HaveOccurred())
		output2, err2 := exec.Command("bash", "-c", "cat "+output1+" |  jq -r .holderIdentity").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		leaderNodeName := strings.Trim(strings.TrimSpace(string(output2)), "\"")
		e2e.Logf("The leader node name is %s", leaderNodeName)
		oc_get_pods, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-sdn", "pod", "-l app=sdn", "-o=wide").OutputToFile("ocgetpods.txt")
		o.Expect(err3).NotTo(o.HaveOccurred())
		raw_grep_output, err3 := exec.Command("bash", "-c", "cat "+oc_get_pods+" | grep "+leaderNodeName+" | awk '{print $1}'").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		leaderPodName := strings.TrimSpace(string(raw_grep_output))
		e2e.Logf("The leader Pod's name: %v", leaderPodName)
		return leaderPodName
	}
}

func checkSDNMetrics(oc *exutil.CLI, url string, metrics string) {
	var metrics_output []byte
	var metrics_log []byte
	olmToken, err := oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	metrics_err := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), fmt.Sprintf("%s", url)).OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metrics_log, _ = exec.Command("bash", "-c", "cat "+output+" ").Output()
		metrics_string := string(metrics_log)
		if strings.Contains(metrics_string, "ovnkube_master_pod") {
			metrics_output, _ = exec.Command("bash", "-c", "cat "+output+" | grep "+metrics+" | awk 'NR==1{print $2}'").Output()
		} else {
			metrics_output, _ = exec.Command("bash", "-c", "cat "+output+" | grep "+metrics+" | awk 'NR==3{print $2}'").Output()
		}
		metrics_value := strings.TrimSpace(string(metrics_output))
		if metrics_value != "" {
			e2e.Logf("The output of the metrics for %s is : %v", metrics, metrics_value)
		} else {
			e2e.Logf("Can't get metrics for %s:", metrics)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metrics_err, fmt.Sprintf("Fail to get metric and the error is:%s", metrics_err))
}

func getEgressCIDRs(oc *exutil.CLI, node string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node, "-o=jsonpath={.egressCIDRs}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("egressCIDR for hostsubnet node %v is: %v", node, output)
	return output
}

// get egressIP from a node
// When they are multiple egressIPs on the node, egressIp list is in format of ["10.0.247.116","10.0.156.51"]
// as an example from the output of command "oc get hostsubnet <node> -o=jsonpath={.egressIPs}"
// convert the iplist into an array of ip addresses
func getEgressIPonSDNHost(oc *exutil.CLI, node string, expectedNum int) ([]string, error) {
	var ip = []string{}
	iplist, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node, "-o=jsonpath={.egressIPs}").Output()
	if iplist != "" {
		ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
	}
	if iplist == "" || len(ip) < expectedNum || err != nil {
		err = wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
			iplist, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node, "-o=jsonpath={.egressIPs}").Output()
			if iplist != "" {
				ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
			}
			if len(ip) < expectedNum || err != nil {
				e2e.Logf("only got %d egressIP, or have err:%v, and try next round", len(ip), err)
				return false, nil
			}
			if iplist != "" && len(ip) == expectedNum {
				e2e.Logf("Found egressIP list for node %v is: %v", node, iplist)
				return true, nil
			}
			return false, nil
		})
		e2e.Logf("Only got %d egressIP, or have err:%v", len(ip), err)
		return ip, err
	}
	return ip, nil
}

func getPodName(oc *exutil.CLI, namespace string, label string) []string {
	var podName []string
	podNameAll, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pod", "-l", label, "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podName = strings.Split(podNameAll, " ")
	e2e.Logf("The pod(s) are  %v ", podName)
	return podName
}

// starting from first node, compare its subnet with subnet of subsequent nodes in the list
// until two nodes with same subnet found, otherwise, return false to indicate that no two nodes with same subnet found
func findTwoNodesWithSameSubnet(oc *exutil.CLI, nodeList *v1.NodeList) (bool, [2]string) {
	var nodes [2]string
	for i := 0; i < (len(nodeList.Items) - 1); i++ {
		for j := i + 1; j < len(nodeList.Items); j++ {
			firstSub := getIfaddrFromNode(nodeList.Items[i].Name, oc)
			secondSub := getIfaddrFromNode(nodeList.Items[j].Name, oc)
			if firstSub == secondSub {
				e2e.Logf("Found nodes with same subnet.")
				nodes[0] = nodeList.Items[i].Name
				nodes[1] = nodeList.Items[j].Name
				return true, nodes
			}
		}
	}
	return false, nodes
}

func getSDNMetrics(oc *exutil.CLI, podName string) string {
	var metrics_log string
	metrics_err := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-sdn", fmt.Sprintf("%s", podName), "--", "curl", "localhost:29100/metrics").OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metrics_log = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metrics_err, fmt.Sprintf("Fail to get metric and the error is:%s", metrics_err))
	return metrics_log
}

func getOVNMetrics(oc *exutil.CLI, url string) string {
	var metrics_log string
	olmToken, err := oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	metrics_err := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), fmt.Sprintf("%s", url)).OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metrics_log = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metrics_err, fmt.Sprintf("Fail to get metric and the error is:%s", metrics_err))
	return metrics_log
}

func checkIPsec(oc *exutil.CLI) string {
	output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.ovnKubernetesConfig.ipsecConfig}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return output
}

func getAssignedEIPInEIPObject(oc *exutil.CLI, egressIPObject string) []map[string]string {
	var egressIPs string
	egressip_err := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		egressIPStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip", egressIPObject, "-ojsonpath={.status.items}").Output()
		if err != nil {
			e2e.Logf("Wait to get EgressIP object applied,try next round. %v", err)
			return false, nil
		}
		if egressIPStatus == "" {
			e2e.Logf("Wait to get EgressIP object applied,try next round. %v", err)
			return false, nil
		}
		egressIPs = egressIPStatus
		e2e.Logf("egressIPStatus: %v", egressIPs)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(egressip_err, fmt.Sprintf("Failed to apply egressIPs:%s", egressip_err))

	var egressIPJsonMap []map[string]string
	json.Unmarshal([]byte(egressIPs), &egressIPJsonMap)
	return egressIPJsonMap
}

func rebootNode(oc *exutil.CLI, nodeName string) {
	e2e.Logf("\nRebooting node %s....", nodeName)
	_, err1 := exutil.DebugNodeWithChroot(oc, nodeName, "shutdown", "-r", "+1")
	o.Expect(err1).NotTo(o.HaveOccurred())
}

func checkNodeStatus(oc *exutil.CLI, nodeName string, expectedStatus string) {
	var expectedStatus1 string
	if expectedStatus == "Ready" {
		expectedStatus1 = "True"
	} else if expectedStatus == "NotReady" {
		expectedStatus1 = "Unknown"
	} else {
		err1 := fmt.Errorf(" TBD supported node status.")
		o.Expect(err1).NotTo(o.HaveOccurred())
	}
	err := wait.Poll(10*time.Second, 6*time.Minute, func() (bool, error) {
		statusOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", nodeName, "-ojsonpath={.status.conditions[-1].status}").Output()
		if err != nil {
			e2e.Logf("\nGet node status with error : %v", err)
			return false, nil
		}
		e2e.Logf("Node %s kubelet status is %s", nodeName, statusOutput)
		if statusOutput != expectedStatus1 {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Node %s is not in expected status %s", nodeName, expectedStatus))
}

func updateEgressIPObject(oc *exutil.CLI, egressIPObjectName string, egressIP string) {
	patchResourceAsAdmin(oc, "egressip/"+egressIPObjectName, "{\"spec\":{\"egressIPs\":[\""+egressIP+"\"]}}")
	egressip_err := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressip", egressIPObjectName, "-o=jsonpath={.status.items[*]}").Output()
		if err != nil {
			e2e.Logf("Wait to get EgressIP object applied,try next round. %v", err)
			return false, nil
		}
		if !strings.Contains(output, egressIP) {
			e2e.Logf("Wait for new IP applied,try next round.")
			e2e.Logf(output)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(egressip_err, fmt.Sprintf("Failed to apply new egressIPs:%s", egressip_err))
}

func getTwoNodesSameSubnet(oc *exutil.CLI, nodeList *v1.NodeList) (bool, []string) {
	var egressNodes []string
	if len(nodeList.Items) < 2 {
		e2e.Logf("Not enough nodes available for the test, skip the case!!")
		return false, nil
	}
	switch ci.CheckPlatform(oc) {
	case "aws":
		e2e.Logf("find the two nodes that have same subnet")
		check, nodes := findTwoNodesWithSameSubnet(oc, nodeList)
		if check {
			egressNodes = nodes[:2]
		} else {
			e2e.Logf("No more than 2 worker nodes in same subnet, skip the test!!!")
			return false, nil
		}
	case "gcp":
		e2e.Logf("since GCP worker nodes all have same subnet, just pick first two nodes as egress nodes")
		egressNodes = append(egressNodes, nodeList.Items[0].Name)
		egressNodes = append(egressNodes, nodeList.Items[1].Name)
	default:
		e2e.Logf("Not supported platform yet!")
		return false, nil
	}

	return true, egressNodes
}

//getSvcIP returns IPv6 and IPv4 in vars in order on dual stack respectively and main Svc IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
func getSvcIP(oc *exutil.CLI, namespace string, svcName string) (string, string) {
	ipStack := checkIpStackType(oc)
	ipFamilyType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ipFamilyPolicy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if (ipStack == "ipv6single") || (ipStack == "ipv4single") {
		svcIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIP)
		return svcIP, ""
	} else if (ipStack == "dualstack" && ipFamilyType == "PreferDualStack") || (ipStack == "dualstack" && ipFamilyType == "RequireDualStack") {
		ipFamilyPrecedence, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ipFamilies[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		//if IPv4 is listed first in ipFamilies then clustrIPs allocation will take order as Ipv4 first and then Ipv6 else reverse
		svcIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIPv4)
		svcIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[1]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIPv6)
		if ipFamilyPrecedence == "IPv4" {
			e2e.Logf("The ipFamilyPrecedence is Ipv4, Ipv6")
			return svcIPv6, svcIPv4
		} else {
			e2e.Logf("The ipFamilyPrecedence is Ipv6, Ipv4")
			svcIPv4, svcIPv6 = svcIPv6, svcIPv4
			return svcIPv6, svcIPv4
		}
	} else {
		//Its a Dual Stack Cluster with SingleStack ipFamilyPolicy
		svcIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIP)
		return svcIP, ""
	}
}

//getPodIP returns IPv6 and IPv4 in vars in order on dual stack respectively and main IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
func getPodIP(oc *exutil.CLI, namespace string, podName string) (string, string) {
	ipStack := checkIpStackType(oc)
	if (ipStack == "ipv6single") || (ipStack == "ipv4single") {
		podIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIP)
		return podIP, ""
	} else if ipStack == "dualstack" {
		podIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[1].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IPv6 in namespace %s is %q", podName, namespace, podIPv6)
		podIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IPv4 in namespace %s is %q", podName, namespace, podIPv4)
		return podIPv6, podIPv4
	}
	return "", ""
}

func CurlPod2PodPass(oc *exutil.CLI, namespaceSrc string, podNameSrc string, namespaceDst string, podNameDst string) {
	podIP1, podIP2 := getPodIP(oc, namespaceDst, podNameDst)
	if podIP2 != "" {
		_, err := e2e.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = e2e.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		_, err := e2e.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func CurlPod2PodFail(oc *exutil.CLI, namespaceSrc string, podNameSrc string, namespaceDst string, podNameDst string) {
	podIP1, podIP2 := getPodIP(oc, namespaceDst, podNameDst)
	if podIP2 != "" {
		_, err := e2e.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		_, err = e2e.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	} else {
		_, err := e2e.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	}
}

func CurlNode2PodPass(oc *exutil.CLI, nodeName string, namespace string, podName string) {
	//getPodIP returns IPv6 and IPv4 in order on dual stack in PodIP1 and PodIP2 respectively and main IP in case of single stack (v4 or v6) in PodIP1, and nil in PodIP2
	podIP1, podIP2 := getPodIP(oc, namespace, podName)
	if podIP2 != "" {
		podv6_url := net.JoinHostPort(podIP1, "8080")
		podv4_url := net.JoinHostPort(podIP2, "8080")
		_, err := exutil.DebugNode(oc, nodeName, "curl", podv4_url, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = exutil.DebugNode(oc, nodeName, "curl", podv6_url, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		pod_url := net.JoinHostPort(podIP1, "8080")
		_, err := exutil.DebugNode(oc, nodeName, "curl", pod_url, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func CurlNode2SvcPass(oc *exutil.CLI, nodeName string, namespace string, svcName string) {
	svcIP1, svcIP2 := getSvcIP(oc, namespace, svcName)
	if svcIP2 != "" {
		svcv6_url := net.JoinHostPort(svcIP1, "27017")
		svcv4_url := net.JoinHostPort(svcIP2, "27017")
		_, err := exutil.DebugNode(oc, nodeName, "curl", svcv4_url, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = exutil.DebugNode(oc, nodeName, "curl", svcv6_url, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		svc_url := net.JoinHostPort(svcIP1, "27017")
		_, err := exutil.DebugNode(oc, nodeName, "curl", svc_url, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func CurlPod2SvcPass(oc *exutil.CLI, namespace string, podNameSrc string, svcName string) {
	svcIP1, svcIP2 := getSvcIP(oc, namespace, svcName)
	if svcIP2 != "" {
		_, err := e2e.RunHostCmd(namespace, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = e2e.RunHostCmd(namespace, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP2, "27017"))
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		_, err := e2e.RunHostCmd(namespace, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func CurlPod2SvcFail(oc *exutil.CLI, namespace string, podNameSrc string, svcName string) {
	svcIP1, svcIP2 := getSvcIP(oc, namespace, svcName)
	if svcIP2 != "" {
		_, err := e2e.RunHostCmd(namespace, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).To(o.HaveOccurred())
		_, err = e2e.RunHostCmd(namespace, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP2, "27017"))
		o.Expect(err).To(o.HaveOccurred())
	} else {
		_, err := e2e.RunHostCmd(namespace, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).To(o.HaveOccurred())
	}
}

func checkProxy(oc *exutil.CLI) bool {
	httpProxy, err := doAction(oc, "get", true, true, "proxy", "cluster", "-o=jsonpath={.status.httpProxy}")
	o.Expect(err).NotTo(o.HaveOccurred())
	httpsProxy, err := doAction(oc, "get", true, true, "proxy", "cluster", "-o=jsonpath={.status.httpsProxy}")
	o.Expect(err).NotTo(o.HaveOccurred())
	if httpProxy != "" || httpsProxy != "" {
		return true
	}
	return false
}

// find out which egress node has the egressIP
func SDNHostwEgressIP(oc *exutil.CLI, node []string, egressip string) string {
	var ip []string
	var foundHost string
	for i := 0; i < len(node); i++ {
		iplist, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node[i], "-o=jsonpath={.egressIPs}").Output()
		if iplist != "" {
			ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
		}
		if iplist == "" || err != nil {
			err = wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
				iplist, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node[i], "-o=jsonpath={.egressIPs}").Output()
				if iplist != "" {
					e2e.Logf("Found egressIP list for node %v is: %v", node, iplist)
					ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
					return true, nil
				}
				if err != nil {
					e2e.Logf("only got %d egressIP, or have err:%v, and try next round", len(ip), err)
					return false, nil
				}
				return false, nil
			})
		}
		if isValueInList(egressip, ip) {
			foundHost = node[i]
			break
		}
	}
	return foundHost
}

func isValueInList(value string, list []string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
