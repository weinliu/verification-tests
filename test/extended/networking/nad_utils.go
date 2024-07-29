package networking

import (
	"fmt"
	"net"
	"strconv"
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

type udnNetDefResource struct {
	nadname             string
	namespace           string
	nad_network_name    string
	topology            string
	subnet              string
	mtu                 int32
	net_attach_def_name string
	template            string
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

func (nad *udnNetDefResource) createUdnNad(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", nad.template, "-p", "NADNAME="+nad.nadname, "NAMESPACE="+nad.namespace, "NAD_NETWORK_NAME="+nad.nad_network_name, "TOPOLOGY="+nad.topology, "SUBNET="+nad.subnet, "MTU="+strconv.Itoa(int(nad.mtu)), "NET_ATTACH_DEF_NAME="+nad.net_attach_def_name)
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
