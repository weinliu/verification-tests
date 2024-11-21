package networking

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

type udnPodResource struct {
	name      string
	namespace string
	label     string
	template  string
}

type udnPodResourceNode struct {
	name      string
	namespace string
	label     string
	nodename  string
	template  string
}

type udnPodSecNADResource struct {
	name       string
	namespace  string
	label      string
	annotation string
	template   string
}

type udnNetDefResource struct {
	nadname             string
	namespace           string
	nad_network_name    string
	topology            string
	subnet              string
	mtu                 int32
	net_attach_def_name string
	role                string
	template            string
}

type udnCRDResource struct {
	crdname    string
	namespace  string
	IPv4cidr   string
	IPv4prefix int32
	IPv6cidr   string
	IPv6prefix int32
	cidr       string
	prefix     int32
	mtu        int32
	role       string
	template   string
}

type udnPodWithProbeResource struct {
	name             string
	namespace        string
	label            string
	port             int
	failurethreshold int
	periodseconds    int
	template         string
}

func (pod *udnPodResource) createUdnPod(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABEL="+pod.label)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (pod *udnPodResourceNode) createUdnPodNode(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABEL="+pod.label, "NODENAME="+pod.nodename)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (pod *udnPodWithProbeResource) createUdnPodWithProbe(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABEL="+pod.label, "PORT="+strconv.Itoa(int(pod.port)), "FAILURETHRESHOLD="+strconv.Itoa(int(pod.failurethreshold)), "PERIODSECONDS="+strconv.Itoa(int(pod.periodseconds)))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (pod *udnPodSecNADResource) createUdnPodWithSecNAD(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABEL="+pod.label, "ANNOTATION="+pod.annotation)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (nad *udnNetDefResource) createUdnNad(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", nad.template, "-p", "NADNAME="+nad.nadname, "NAMESPACE="+nad.namespace, "NAD_NETWORK_NAME="+nad.nad_network_name, "TOPOLOGY="+nad.topology, "SUBNET="+nad.subnet, "MTU="+strconv.Itoa(int(nad.mtu)), "NET_ATTACH_DEF_NAME="+nad.net_attach_def_name, "ROLE="+nad.role)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", nad.nadname))
}

func (nad *udnNetDefResource) deleteUdnNetDef(oc *exutil.CLI) {
	removeResource(oc, false, true, "net-attach-def", nad.nadname, "-n", nad.namespace)
}

// getPodIPUDN returns IPv6 and IPv4 in vars in order on dual stack respectively and main IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
func getPodIPUDN(oc *exutil.CLI, namespace string, podName string, netName string) (string, string) {
	ipStack := checkIPStackType(oc)
	cmdIPv4 := "ip a sho " + netName + " | awk 'NR==3{print $2}' |grep -Eo '((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])'"
	cmdIPv6 := "ip -o -6 addr show dev " + netName + " | awk '$3 == \"inet6\" && $6 == \"global\" {print $4}' | cut -d'/' -f1"
	if ipStack == "ipv4single" {
		podIPv4, err := execCommandInSpecificPod(oc, namespace, podName, cmdIPv4)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The UDN pod %s IPv4 in namespace %s is %q", podName, namespace, podIPv4)
		return podIPv4, ""
	} else if ipStack == "ipv6single" {
		podIPv6, err := execCommandInSpecificPod(oc, namespace, podName, cmdIPv6)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The UDN pod %s IPv6 in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6, ""
	} else {
		podIPv4, err := execCommandInSpecificPod(oc, namespace, podName, cmdIPv4)
		o.Expect(err).NotTo(o.HaveOccurred())
		podIPv6, err := execCommandInSpecificPod(oc, namespace, podName, cmdIPv6)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The UDN pod's %s IPv6 and IPv4 IP in namespace %s is %q %q", podName, namespace, podIPv6, podIPv4)
		return podIPv6, podIPv4
	}
	return "", ""
}

// CurlPod2PodPass checks connectivity across udn pods regardless of network addressing type on cluster
func CurlPod2PodPassUDN(oc *exutil.CLI, namespaceSrc string, podNameSrc string, namespaceDst string, podNameDst string) {
	// getPodIPUDN will returns IPv6 and IPv4 in vars in order on dual stack respectively and main IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
	podIP1, podIP2 := getPodIPUDN(oc, namespaceDst, podNameDst, "ovn-udn1")
	if podIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// CurlPod2PodFailUDN ensures no connectivity from a udn pod to pod regardless of network addressing type on cluster
func CurlPod2PodFailUDN(oc *exutil.CLI, namespaceSrc string, podNameSrc string, namespaceDst string, podNameDst string) {
	// getPodIPUDN will returns IPv6 and IPv4 in vars in order on dual stack respectively and main IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
	podIP1, podIP2 := getPodIPUDN(oc, namespaceDst, podNameDst, "ovn-udn1")
	if podIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	}
}

func CurlUDNPod2PodPassMultiNetwork(oc *exutil.CLI, namespaceSrc string, namespaceDst string, podNameSrc string, netNameInterface string, podNameDst string, netNameDst string) {
	podIP1, podIP2 := getPodIPUDN(oc, namespaceDst, podNameDst, netNameDst)
	if podIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+netNameInterface+" --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+netNameInterface+" --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+netNameInterface+" --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func CurlUDNPod2PodFailMultiNetwork(oc *exutil.CLI, namespaceSrc string, namespaceDst string, podNameSrc string, netNameInterface string, podNameDst string, netNameDst string) {
	podIP1, podIP2 := getPodIPUDN(oc, namespaceDst, podNameDst, netNameDst)
	if podIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+netNameInterface+" --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+netNameInterface+" --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --interface "+netNameInterface+" --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	}
}

func (udncrd *udnCRDResource) createUdnCRDSingleStack(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 20*time.Second, false, func(ctx context.Context) (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", udncrd.template, "-p", "CRDNAME="+udncrd.crdname, "NAMESPACE="+udncrd.namespace, "CIDR="+udncrd.cidr, "PREFIX="+strconv.Itoa(int(udncrd.prefix)), "MTU="+strconv.Itoa(int(udncrd.mtu)), "ROLE="+udncrd.role)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create udn CRD %s due to %v", udncrd.crdname, err))
}

func (udncrd *udnCRDResource) createUdnCRDDualStack(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 20*time.Second, false, func(ctx context.Context) (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", udncrd.template, "-p", "CRDNAME="+udncrd.crdname, "NAMESPACE="+udncrd.namespace, "IPv4CIDR="+udncrd.IPv4cidr, "IPv4PREFIX="+strconv.Itoa(int(udncrd.IPv4prefix)), "IPv6CIDR="+udncrd.IPv6cidr, "IPv6PREFIX="+strconv.Itoa(int(udncrd.IPv6prefix)), "MTU="+strconv.Itoa(int(udncrd.mtu)), "ROLE="+udncrd.role)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create udn CRD %s due to %v", udncrd.crdname, err))
}

func (udncrd *udnCRDResource) deleteUdnCRDDef(oc *exutil.CLI) {
	removeResource(oc, true, true, "UserDefinedNetwork", udncrd.crdname, "-n", udncrd.namespace)
}

func waitUDNCRDApplied(oc *exutil.CLI, ns, crdName string) error {
	checkErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
		output, efErr := oc.AsAdmin().WithoutNamespace().Run("wait").Args("UserDefinedNetwork/"+crdName, "-n", ns, "--for", "condition=NetworkAllocationSucceeded=True").Output()
		if efErr != nil {
			e2e.Logf("Failed to get UDN %v, error: %s. Trying again", crdName, efErr)
			return false, nil
		}
		if !strings.Contains(output, fmt.Sprintf("userdefinednetwork.k8s.ovn.org/%s condition met", crdName)) {
			e2e.Logf("UDN CRD was not applied yet, trying again. \n %s", output)
			return false, nil
		}
		return true, nil
	})
	return checkErr
}

func (udncrd *udnCRDResource) createLayer2DualStackUDNCRD(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 20*time.Second, false, func(ctx context.Context) (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", udncrd.template, "-p", "CRDNAME="+udncrd.crdname, "NAMESPACE="+udncrd.namespace, "IPv4CIDR="+udncrd.IPv4cidr, "IPv6CIDR="+udncrd.IPv6cidr, "MTU="+strconv.Itoa(int(udncrd.mtu)), "ROLE="+udncrd.role)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create udn CRD %s due to %v", udncrd.crdname, err))
}

func (udncrd *udnCRDResource) createLayer2SingleStackUDNCRD(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 20*time.Second, false, func(ctx context.Context) (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", udncrd.template, "-p", "CRDNAME="+udncrd.crdname, "NAMESPACE="+udncrd.namespace, "CIDR="+udncrd.cidr, "MTU="+strconv.Itoa(int(udncrd.mtu)), "ROLE="+udncrd.role)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create udn CRD %s due to %v", udncrd.crdname, err))
}

func checkPodCIDRsOverlap(oc *exutil.CLI, namespace string, ipStack string, Pods []string, netName string) bool {
	var subnetsIPv4 []*net.IPNet
	var subnetsIPv6 []*net.IPNet
	var subnets []*net.IPNet
	cmdIPv4 := "ip a sho " + netName + " | awk 'NR==3{print $2}'"
	cmdIPv6 := "ip -o -6 addr show dev " + netName + " | awk '$3 == \"inet6\" && $6 == \"global\" {print $4}'"
	for _, pod := range Pods {
		if ipStack == "dualstack" {
			podIPv4, ipv4Err := execCommandInSpecificPod(oc, namespace, pod, cmdIPv4)
			o.Expect(ipv4Err).NotTo(o.HaveOccurred())
			podIPv6, ipv6Err := execCommandInSpecificPod(oc, namespace, pod, cmdIPv6)
			o.Expect(ipv6Err).NotTo(o.HaveOccurred())
			_, subnetIPv4, err := net.ParseCIDR(strings.TrimSpace(podIPv4))
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetsIPv4 = append(subnetsIPv4, subnetIPv4)
			_, subnetIPv6, err := net.ParseCIDR(strings.TrimSpace(podIPv6))
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetsIPv6 = append(subnetsIPv6, subnetIPv6)
		} else {
			if ipStack == "ipv6single" {
				podIPv6, ipv6Err := execCommandInSpecificPod(oc, namespace, pod, cmdIPv6)
				o.Expect(ipv6Err).NotTo(o.HaveOccurred())
				_, subnet, err := net.ParseCIDR(strings.TrimSpace(podIPv6))
				o.Expect(err).NotTo(o.HaveOccurred())
				subnets = append(subnets, subnet)
			} else {
				podIPv4, ipv4Err := execCommandInSpecificPod(oc, namespace, pod, cmdIPv4)
				o.Expect(ipv4Err).NotTo(o.HaveOccurred())
				_, subnet, err := net.ParseCIDR(strings.TrimSpace(podIPv4))
				o.Expect(err).NotTo(o.HaveOccurred())
				subnets = append(subnets, subnet)
			}
		}
	}
	if ipStack == "dualstack" {
		return subnetsIPv4[0].Contains(subnetsIPv4[1].IP) || subnetsIPv4[1].Contains(subnetsIPv4[0].IP) ||
			subnetsIPv6[0].Contains(subnetsIPv6[1].IP) || subnetsIPv6[1].Contains(subnetsIPv6[0].IP)
	} else {
		return subnets[0].Contains(subnets[1].IP) || subnets[1].Contains(subnets[0].IP)
	}
}
