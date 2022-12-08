package hypershift

import (
	"strings"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

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

func (h *hostedCluster) getHostedClusterInfrastructureTopology() (string, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--kubeconfig="+h.hostedClustersKubeconfigFile, "infrastructure", "cluster", `-o=jsonpath={.status.infrastructureTopology}`).Output()
	if er != nil {
		e2e.Logf(" get infrastructure/cluster status error: %v", er)
		return "", er
	}
	return value, nil
}

func (h *hostedCluster) pollGetHostedClusterInfrastructureTopology() func() string {
	return func() string {
		value, _ := h.getHostedClusterInfrastructureTopology()
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

func (h *hostedCluster) hostedClustersReady() (bool, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", "-n", h.namespace, "--ignore-not-found", h.name, `-ojsonpath='{.status.conditions[?(@.type=="Available")].status}'`).Output()
	if er != nil {
		e2e.Logf("error occurred: %v, try next round", er)
		return false, er
	}
	if strings.Contains(value, "True") {
		return true, nil
	}
	return false, nil
}

func (h *hostedCluster) pollHostedClustersReady() func() bool {
	return func() bool {
		value, _ := h.hostedClustersReady()
		return value
	}
}

func (h *hostedCluster) getHostedClustersHACPWorkloadNames(workloadType string) ([]string, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args(workloadType, "-n", h.namespace+"-"+h.name, `-ojsonpath={.items[?(@.spec.replicas>1)].metadata.name}`).Output()
	if er != nil {
		e2e.Logf("get HA HostedClusters Workload Names, error occurred: %v", er)
		return nil, er
	}
	return strings.Split(value, " "), nil
}

func (h *hostedCluster) isCPPodOnlyRunningOnOneNode(nodeName string) (bool, error) {
	value, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", h.namespace+"-"+h.name, `-ojsonpath={.items[?(@.spec.nodeName!="`+nodeName+`")].metadata.name}`).Output()
	if err != nil {
		e2e.Logf("check HostedClusters CP PodOnly One Node, error occurred: %v", err)
		return false, err
	}
	if len(value) == 0 {
		return true, nil
	}
	e2e.Logf("not on %s node pod name:%s", nodeName, value)
	if len(strings.Split(value, " ")) == 1 && strings.Contains(value, "ovnkube") {
		return true, nil
	}
	return false, nil
}

func (h *hostedCluster) pollIsCPPodOnlyRunningOnOneNode(nodeName string) func() bool {
	return func() bool {
		value, _ := h.isCPPodOnlyRunningOnOneNode(nodeName)
		return value
	}
}

func (h *hostedCluster) getAzureDiskSizeGBByNodePool(nodePool string) string {
	return doOcpReq(h.oc, OcpGet, false, "nodepools", "-n", h.namespace, nodePool, `-ojsonpath={.spec.platform.azure.diskSizeGB}`)
}

func (h *hostedCluster) pollGetNodePoolReplicas() func() string {
	return func() string {
		value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("nodepools", "-n", h.namespace, `-ojsonpath={.items[*].status.replicas}`).Output()
		if er != nil {
			return ""
		}
		return value
	}
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

// checkHCConditions check conditions and exit test if not available.
func (h *hostedCluster) checkHCConditions() bool {
	res, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("hostedcluster", h.name, "-n", h.namespace,
		`-ojsonpath={range .status.conditions[*]}{@.type}{" "}{@.status}{" "}{end}`).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	return checkSubstringWithNoExit(res,
		[]string{"ValidHostedControlPlaneConfiguration True", "ClusterVersionSucceeding True",
			"Degraded False", "EtcdAvailable True", "KubeAPIServerAvailable True", "InfrastructureReady True",
			"Available True", "ValidConfiguration True", "SupportedHostedCluster True",
			"ValidHostedControlPlaneConfiguration True", "IgnitionEndpointAvailable True", "ReconciliationActive True",
			"ValidReleaseImage True", "ValidOIDCConfiguration True", "ReconciliationSucceeded True"})
}
