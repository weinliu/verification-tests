package hypershift

import (
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"strings"
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

func (h *hostedCluster) getHostedClusterReadyNodeCount() (int, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--kubeconfig="+h.hostedClustersKubeconfigFile, "node", `-ojsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}'`).Output()
	if er != nil {
		e2e.Logf(" get node status ready error: %v", er)
		return 0, er
	}
	return strings.Count(value, "True"), nil
}

func (h *hostedCluster) pollGetHostedClusterReadyNodeCount() func() int {
	return func() int {
		value, _ := h.getHostedClusterReadyNodeCount()
		return value
	}
}

func (h *hostedCluster) getInfraID() (string, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", "-n", h.namespace, h.name, `-ojsonpath={.spec.infraID}`).Output()
	if er != nil {
		e2e.Logf("get InfraID, error occurred: %v", er)
		return "", er
	}
	return value, nil
}

func (h *hostedCluster) getClustersDeletionTimestamp() (string, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("clusters", "-n", h.namespace+"-"+h.name, "--ignore-not-found", `-ojsonpath={.items[].metadata.deletionTimestamp}`).Output()
	if er != nil {
		e2e.Logf("get ClusterDeletionTimestamp, error occurred: %v", er)
		return "", er
	}
	return value, nil
}

func getHostedClusters(oc *exutil.CLI, namespace string) (string, error) {
	value, er := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	if er != nil {
		e2e.Logf("get HostedClusters, error occurred: %v", er)
		return "", er
	}
	return value, nil
}

func pollGetHostedClusters(oc *exutil.CLI, namespace string) func() string {
	return func() string {
		value, _ := getHostedClusters(oc, namespace)
		return value
	}
}
