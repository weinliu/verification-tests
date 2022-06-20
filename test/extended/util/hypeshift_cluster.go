package util

import (
	"fmt"
	"os/exec"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
)

// ValidHypershiftAndGetGuestKubeConf check if it is hypershift env and get kubeconf of the guest cluster
// the first return is guest cluster name
// the second return is the file of kubeconfig of the guest cluster
// if it is not hypershift env, it will skip test.
func ValidHypershiftAndGetGuestKubeConf(oc *CLI) (string, string) {
	hypershiftOperator, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"pods", "-n", "hypershift", "-ojsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(hypershiftOperator) <= 0 {
		g.Skip("there is no hypeshift operagtor on host cluster, skip test run")
	}

	clusterNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", "clusters", "hostedclusters", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(clusterNames) <= 0 {
		g.Skip("there is no guest cluster, skip test run")
	}

	hypersfhitPodStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", "hypershift", "pod", "-o=jsonpath={.items[0].status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(hypersfhitPodStatus).To(o.ContainSubstring("Running"))

	//get first guest cluster to run test
	guestClusterName := strings.Split(clusterNames, " ")[0]

	guestClusterKubeconfigFile := "/tmp/guestcluster-kubeconfig-" + guestClusterName + "-" + getRandomString()
	_, err = exec.Command("bash", "-c", fmt.Sprintf("hypershift create kubeconfig --name %s > %s",
		guestClusterName, guestClusterKubeconfigFile)).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return guestClusterName, guestClusterKubeconfigFile

}
