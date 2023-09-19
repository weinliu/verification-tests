package workloads

import (
	"fmt"
	"os"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-apps] Workloads", func() {
	defer g.GinkgoRecover()
	const (
		hostport = "8443"
	)
	var (
		oc             = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
		labels         string
		ns             string
		kubeconfigFile string
	)
	g.It("HyperShiftMGMT-Author:wewang-Medium-29780-Controller metrics reported from openshift-controller-manager [Flaky]", func() {
		guestClusterName, guestClusterKubeconfig, hostedClusterName := exutil.ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc)
		if guestClusterKubeconfig != "" {
			e2e.Logf("GuestClusterkubeconfig is %s", guestClusterKubeconfig)
			oc.SetGuestKubeconf(guestClusterKubeconfig)
			ns = hostedClusterName + "-" + guestClusterName
			labels = "hypershift.openshift.io/control-plane-component=openshift-controller-manager"
		} else {
			kubeconfigFile = os.Getenv("KUBECONFIG")
			oc.SetGuestKubeconf(kubeconfigFile)
			ns = "openshift-controller-manager"
			labels = "app=openshift-controller-manager-a"
		}
		g.By("check controller metrics")
		token, err := oc.AsAdmin().AsGuestKubeconf().WithoutNamespace().Run("create").Args("token", "-n", "openshift-monitoring", "prometheus-k8s").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		foundMetrics := false
		if err != nil {
			e2e.Logf("Error listing pods: %v", err)
		}

		g.By("Retreive podlist based on ns & labels")
		podList, err := exutil.GetAllPodsWithLabel(oc, ns, labels)
		o.Expect(err).NotTo(o.HaveOccurred())

		for _, p := range podList {
			foundAvaliableDc := false
			foundFailDc := false
			foundCancelDc := false
			//https://access.redhat.com/solutions/5356961
			//cURL to NO_PROXY CIDR addresses are not working as expected, it's not openshift issue.
			//Change to use 127.0.0.1, so it could work on proxy cluster.
			results, err := getBearerTokenURLViaPod(ns, p, fmt.Sprintf("https://%s:%s/metrics", "127.0.0.1", hostport), token)

			o.Expect(err).NotTo(o.HaveOccurred())
			foundAvaliableDc = strings.Contains(string(results), "openshift_apps_deploymentconfigs_complete_rollouts_total{phase=\"available\"}")
			foundFailDc = strings.Contains(string(results), "openshift_apps_deploymentconfigs_complete_rollouts_total{phase=\"cancelled\"}")
			foundCancelDc = strings.Contains(string(results), "openshift_apps_deploymentconfigs_complete_rollouts_total{phase=\"failed\"}")
			if foundAvaliableDc && foundFailDc && foundCancelDc {
				foundMetrics = true
				break
			}
		}
		o.Expect(foundMetrics).To(o.BeTrue())
	})
})

func getBearerTokenURLViaPod(ns string, execPodName string, url string, bearer string) (string, error) {
	cmd := fmt.Sprintf("curl -s -k -H 'Authorization: Bearer %s' %q", bearer, url)
	output, err := e2eoutput.RunHostCmd(ns, execPodName, cmd)
	if err != nil {
		return "", fmt.Errorf("host command failed: %v\n%s", err, output)
	}
	return output, nil
}
