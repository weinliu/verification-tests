package storage

import (
	"strings"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type hostedCluster struct {
	oc                           *exutil.CLI
	namespace                    string
	name                         string
	hostedClustersKubeconfigFile string
}

func newHostedCluster(oc *exutil.CLI, namespace string, name string) *hostedCluster {
	return &hostedCluster{oc: oc, namespace: namespace, name: name}
}

func (h *hostedCluster) getHostedClusterKubeconfigFile() string {
	return h.hostedClustersKubeconfigFile
}

func (h *hostedCluster) setHostedClusterKubeconfigFile(kubeconfig string) {
	h.hostedClustersKubeconfigFile = kubeconfig
}

// function to get the hosted cluster resoure tags, currently only support for aws hosted clusters
func getHostedClusterResourceTags(oc *exutil.CLI, hostedClusterNS string, guestClusterName string) string {
	resourceTags, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("hostedclusters/"+guestClusterName, "-n", hostedClusterNS, "-o=jsonpath={.spec.platform.aws.resourceTags}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The Hostedcluster %q resource tags are %v\n", guestClusterName, resourceTags)
	return resourceTags
}

// function to wait for node pools in ready status
func (h *hostedCluster) pollCheckAllNodepoolReady() func() bool {
	return func() bool {
		return h.checkAllNodepoolReady()
	}
}

func (h *hostedCluster) checkAllNodepoolReady() bool {
	nodePoolsInfo, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", "-ojson", "--namespace", h.namespace).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	nodePoolsAllMachinesReadyConditions := gjson.Get(nodePoolsInfo, `items.#(spec.clusterName=="`+h.name+`")#.status.conditions.#(type=="AllMachinesReady").status`).String()
	nodePoolsAllNodesHealthyConditions := gjson.Get(nodePoolsInfo, `items.#(spec.clusterName=="`+h.name+`")#.status.conditions.#(type=="AllNodesHealthy").status`).String()
	nodePoolsReadyConditions := gjson.Get(nodePoolsInfo, `items.#(spec.clusterName=="`+h.name+`")#.status.conditions.#(type=="Ready").status`).String()

	if strings.Contains(nodePoolsAllMachinesReadyConditions, "False") || strings.Contains(nodePoolsAllNodesHealthyConditions, "False") || strings.Contains(nodePoolsReadyConditions, "False") {
		e2e.Logf("Nodepools are not in ready status, AllMachinesReady: %s, AllNodesHealthy: %s, Ready: %s\n", nodePoolsAllMachinesReadyConditions, nodePoolsAllNodesHealthyConditions, nodePoolsReadyConditions)
		return false
	}
	e2e.Logf("Hosted cluster %q nodepools are in ready status", h.name)
	return true
}
