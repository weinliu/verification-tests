package util

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// ValidHypershiftAndGetGuestKubeConf check if it is hypershift env and get kubeconf of the guest cluster
// the first return is guest cluster name
// the second return is the file of kubeconfig of the guest cluster
// if it is not hypershift env, it will skip test.
func ValidHypershiftAndGetGuestKubeConf(oc *CLI) (string, string) {
	operatorNS := GetHyperShiftOperatorNameSpace(oc)
	if len(operatorNS) <= 0 {
		g.Skip("there is no hypershift operator on host cluster, skip test run")
	}

	hostedclusterNS := GetHyperShiftHostedClusterNameSpace(oc)
	if len(hostedclusterNS) <= 0 {
		g.Skip("there is no hosted cluster NS in mgmt cluster, skip test run")
	}

	clusterNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", hostedclusterNS, "hostedclusters", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(clusterNames) <= 0 {
		g.Skip("there is no hosted cluster, skip test run")
	}

	hypersfhitPodStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", operatorNS, "pod", "-o=jsonpath={.items[0].status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(hypersfhitPodStatus).To(o.ContainSubstring("Running"))

	//get first hosted cluster to run test
	guestClusterName := strings.Split(clusterNames, " ")[0]

	var guestClusterKubeconfigFile string
	if os.Getenv("GUEST_KUBECONFIG") != "" {
		guestClusterKubeconfigFile = os.Getenv("GUEST_KUBECONFIG")
		e2e.Logf(fmt.Sprintf("use a known guest cluster kubeconfig: %v", guestClusterKubeconfigFile))
	} else {
		guestClusterKubeconfigFile = "/tmp/guestcluster-kubeconfig-" + guestClusterName + "-" + getRandomString()
		_, err = exec.Command("bash", "-c", fmt.Sprintf("hypershift create kubeconfig --name %s > %s",
			guestClusterName, guestClusterKubeconfigFile)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(fmt.Sprintf("create a new guest cluster kubeconfig: %v", guestClusterKubeconfigFile))
	}
	return guestClusterName, guestClusterKubeconfigFile
}

// ValidHypershiftAndGetGuestKubeConfWithNoSkip check if it is hypershift env and get kubeconf of the guest cluster
// the first return is guest cluster name
// the second return is the file of kubeconfig of the guest cluster
// if it is not hypershift env, it will not skip the testcase and return null string.
func ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc *CLI) (string, string) {
	operatorNS := GetHyperShiftOperatorNameSpace(oc)
	if len(operatorNS) <= 0 {
		return "", ""
	}

	hostedclusterNS := GetHyperShiftHostedClusterNameSpace(oc)
	if len(hostedclusterNS) <= 0 {
		return "", ""
	}

	clusterNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", hostedclusterNS, "hostedclusters", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(clusterNames) <= 0 {
		return "", ""
	}

	hypersfhitPodStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", operatorNS, "pod", "-o=jsonpath={.items[0].status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(hypersfhitPodStatus).To(o.ContainSubstring("Running"))

	//get first hosted cluster to run test
	guestClusterName := strings.Split(clusterNames, " ")[0]

	var guestClusterKubeconfigFile string
	if os.Getenv("GUEST_KUBECONFIG") != "" {
		guestClusterKubeconfigFile = os.Getenv("GUEST_KUBECONFIG")
		e2e.Logf(fmt.Sprintf("use a known guest cluster kubeconfig: %v", guestClusterKubeconfigFile))
	} else {
		guestClusterKubeconfigFile = "/tmp/guestcluster-kubeconfig-" + guestClusterName + "-" + getRandomString()
		_, err = exec.Command("bash", "-c", fmt.Sprintf("hypershift create kubeconfig --name %s > %s",
			guestClusterName, guestClusterKubeconfigFile)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(fmt.Sprintf("create a new guest cluster kubeconfig: %v", guestClusterKubeconfigFile))
	}
	return guestClusterName, guestClusterKubeconfigFile
}

// GetHyperShiftOperatorNameSpace get hypershift operator namespace
// if not exist, it will return empty string.
func GetHyperShiftOperatorNameSpace(oc *CLI) string {
	args := []string{
		"pods", "-A",
		"-l", "hypershift.openshift.io/operator-component=operator",
		"-l", "app=operator",
		"--ignore-not-found",
		"-ojsonpath={.items[0].metadata.namespace}",
	}
	namespace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return namespace
}

// GetHyperShiftHostedClusterNameSpace get hypershift hostedcluster namespace
// if not exist, it will return empty string. If more than one exists, it will return the first one.
func GetHyperShiftHostedClusterNameSpace(oc *CLI) string {
	namespace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"hostedcluster", "-A", "--ignore-not-found", "-ojsonpath={.items[0].metadata.namespace}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return namespace
}
