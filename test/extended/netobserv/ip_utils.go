package netobserv

import (
	"strings"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

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

func getPodIPv4(oc *exutil.CLI, namespace string, podName string) string {
	podIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s IP in namespace %s is %q", podName, namespace, podIPv4)
	return podIPv4
}

func getPodIPv6(oc *exutil.CLI, namespace string, podName string, ipStack string) string {
	if ipStack == "ipv6single" {
		podIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6
	} else if ipStack == "dualstack" {
		podIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[1].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6
	}
	return ""
}

func getServiceIPv4(oc *exutil.CLI, namespace string, serviceName string) string {
	serviceIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, serviceName, "-o=jsonpath={.spec.clusterIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IP in namespace %s is %q", serviceName, namespace, serviceIPv4)
	return serviceIPv4
}
