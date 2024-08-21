package hypershift

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	clientv3 "go.etcd.io/etcd/client/v3"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/util/retry"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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

// getHostedClusterReadyNodeCount get ready nodes count
// name: npName name, if empty, get all ready nodes' count
func (h *hostedCluster) getHostedClusterReadyNodeCount(npName string) (int, error) {
	cond := []string{"--kubeconfig=" + h.hostedClustersKubeconfigFile, "node", "--ignore-not-found", `-ojsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}'`}
	if len(npName) > 0 {
		cond = append(cond, "-l", "hypershift.openshift.io/nodePool="+npName)
	}
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args(cond...).Output()
	if er != nil {
		e2e.Logf(" get node status ready error: %v", er)
		return 0, er
	}
	return strings.Count(value, "True"), nil
}

func (h *hostedCluster) pollGetHostedClusterReadyNodeCount(npName string) func() int {
	return func() int {
		value, err := h.getHostedClusterReadyNodeCount(npName)
		o.Expect(err).ShouldNot(o.HaveOccurred())
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

func (h *hostedCluster) getResourceGroupName() (string, error) {
	infraId, err := h.getInfraID()
	if err != nil {
		return "", err
	}
	return h.name + "-" + infraId, nil
}

func (h *hostedCluster) getClustersDeletionTimestamp() (string, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("clusters", "-n", h.namespace+"-"+h.name, "--ignore-not-found", `-ojsonpath={.items[].metadata.deletionTimestamp}`).Output()
	if er != nil {
		e2e.Logf("get ClusterDeletionTimestamp, error occurred: %v", er)
		return "", er
	}
	return value, nil
}

func (h *hostedCluster) getHostedComponentNamespace() string {
	return fmt.Sprintf("%s-%s", h.namespace, h.name)
}

// Warning: the returned default SG ID could be empty
func (h *hostedCluster) getDefaultSgId() string {
	return doOcpReq(h.oc, OcpGet, false, "hc", h.name, "-n", h.namespace, "-o=jsonpath={.status.platform.aws.defaultWorkerSecurityGroupID}")
}

func (h *hostedCluster) getSvcPublishingStrategyType(svc hcService) hcServiceType {
	jsonPath := fmt.Sprintf(`-o=jsonpath={.spec.services[?(@.service=="%s")].servicePublishingStrategy.type}`, svc)
	return hcServiceType(doOcpReq(h.oc, OcpGet, true, "hc", h.name, "-n", h.namespace, jsonPath))
}

func (h *hostedCluster) getControlPlaneEndpointPort() string {
	return doOcpReq(h.oc, OcpGet, true, "hc", h.name, "-n", h.namespace, `-o=jsonpath={.status.controlPlaneEndpoint.port}`)
}

func (h *hostedCluster) hostedClustersReady() (bool, error) {
	value, er := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", "-n", h.namespace, "--ignore-not-found", h.name, `-ojsonpath='{.status.conditions[?(@.type=="Available")].status}'`).Output()
	if er != nil {
		e2e.Logf("error occurred to get Available: %v, try next round", er)
		return false, er
	}
	if !strings.Contains(value, "True") {
		return false, fmt.Errorf("Available != True")
	}

	value, er = h.oc.AsAdmin().WithoutNamespace().Run("get").Args("hostedclusters", "-n", h.namespace, "--ignore-not-found", h.name, `-ojsonpath={.status.version.history[?(@.state!="")].state}`).Output()
	if er != nil {
		e2e.Logf("error occurred to get PROGRESS: %v, try next round", er)
		return false, er
	}
	if !strings.Contains(value, "Completed") {
		return false, fmt.Errorf("PROGRESS != Completed")
	}

	return true, nil
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

func (h *hostedCluster) getAzureSubnetId() string {
	return doOcpReq(h.oc, OcpGet, false, "hc", h.name, "-n", h.namespace, "-o=jsonpath={.spec.platform.azure.subnetID}")
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
	iaasPlatform := exutil.CheckPlatform(h.oc)
	res, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("hostedcluster", h.name, "-n", h.namespace,
		`-ojsonpath={range .status.conditions[*]}{@.type}{" "}{@.status}{" "}{end}`).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	if iaasPlatform == "azure" {
		return checkSubstringWithNoExit(res,
			[]string{"ValidHostedControlPlaneConfiguration True", "ClusterVersionSucceeding True",
				"Degraded False", "EtcdAvailable True", "KubeAPIServerAvailable True", "InfrastructureReady True",
				"Available True", "ValidConfiguration True", "SupportedHostedCluster True",
				"ValidHostedControlPlaneConfiguration True", "IgnitionEndpointAvailable True", "ReconciliationActive True",
				"ValidReleaseImage True", "ReconciliationSucceeded True"})
	} else {
		return checkSubstringWithNoExit(res,
			[]string{"ValidHostedControlPlaneConfiguration True", "ClusterVersionSucceeding True",
				"Degraded False", "EtcdAvailable True", "KubeAPIServerAvailable True", "InfrastructureReady True",
				"Available True", "ValidConfiguration True", "SupportedHostedCluster True",
				"ValidHostedControlPlaneConfiguration True", "IgnitionEndpointAvailable True", "ReconciliationActive True",
				"ValidReleaseImage True", "ReconciliationSucceeded True"})
	}
}

func (h *hostedCluster) checkNodepoolAllConditions(npName string) func() bool {
	return func() bool {
		res := doOcpReq(h.oc, OcpGet, true, "nodepools", "-n", h.namespace, npName, `-ojsonpath={range .status.conditions[*]}{@.type}{" "}{@.status}{" "}{end}`)
		return checkSubstringWithNoExit(res, []string{"AutoscalingEnabled False", "UpdateManagementEnabled True", "ValidReleaseImage True", "ValidPlatformImage True", "AWSSecurityGroupAvailable True", "ValidMachineConfig True", "ValidGeneratedPayload True", "ReachedIgnitionEndpoint True", "ValidTuningConfig True", "ReconciliationActive True", "AllMachinesReady True", "AllNodesHealthy True", "Ready True"})
	}
}

// getHostedclusterConsoleInfo returns console url and password
// the first return is console url
// the second return is password of kubeadmin
func (h *hostedCluster) getHostedclusterConsoleInfo() (string, string) {
	url, cerr := h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpWhoami).Args("--show-console").Output()
	o.Expect(cerr).ShouldNot(o.HaveOccurred())

	pwdbase64, pswerr := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, "secret",
		"kubeadmin-password", "-ojsonpath={.data.password}").Output()
	o.Expect(pswerr).ShouldNot(o.HaveOccurred())

	pwd, err := base64.StdEncoding.DecodeString(pwdbase64)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return url, string(pwd)
}

func (h *hostedCluster) createAwsNodePool(name string, nodeCount int) {
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create nodepool aws --name %s --namespace %s --cluster-name %s --node-count %d",
		name, h.namespace, h.name, nodeCount)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (h *hostedCluster) createAwsInPlaceNodePool(name string, nodeCount int, dir string) {
	npFile := dir + "/np-inplace.yaml"
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create nodepool aws --name %s --namespace %s --cluster-name %s --node-count %d --render > %s", name, h.namespace, h.name, nodeCount, npFile)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	cmdSed := fmt.Sprintf("sed -i 's/Replace/InPlace/g' %s", npFile)
	_, err = bashClient.Run(cmdSed).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	err = h.oc.AsAdmin().WithoutNamespace().Run(OcpCreate).Args("-f", npFile).Execute()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (h *hostedCluster) deleteNodePool(name string) {
	_, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpDelete).Args("--ignore-not-found", "nodepool", "-n", h.namespace, name).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (h *hostedCluster) checkNodePoolReady(name string) bool {
	//check condition {Ready:True}
	readyCond := `-ojsonpath={.status.conditions[?(@.type=="Ready")].status}`
	isReady, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("np", "-n", h.namespace, name, readyCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if !strings.Contains(isReady, "True") {
		e2e.Logf("nodePool ready condition: %s", isReady)
		return false
	}

	//check condition {AutoscalingEnabled:True/False}
	autoScalCond := `-ojsonpath={.status.conditions[?(@.type=="AutoscalingEnabled")].status}`
	autoscaleEnabled, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("np", "-n", h.namespace, name, autoScalCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	//if not autoscaleEnabled, check replicas is as expected
	if autoscaleEnabled != "True" {
		desiredNodes, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("np", "-n", h.namespace, name, "-o=jsonpath={.spec.replicas}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		currentNodes, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("np", "-n", h.namespace, name, "-o=jsonpath={.status.replicas}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		return desiredNodes == currentNodes
	}
	return true
}

func (h *hostedCluster) pollCheckHostedClustersNodePoolReady(name string) func() bool {
	return func() bool {
		return h.checkNodePoolReady(name)
	}
}

func (h *hostedCluster) setNodepoolAutoScale(name, max, min string) {
	removeNpConfig := `[{"op": "remove", "path": "/spec/replicas"}]`
	autoscalConfig := fmt.Sprintf(`--patch={"spec": {"autoScaling": {"max": %s, "min":%s}}}`, max, min)

	_, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpPatch).Args("-n", h.namespace, "nodepools", name, "--type=json", "-p", removeNpConfig).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	_, err = h.oc.AsAdmin().WithoutNamespace().Run(OcpPatch).Args("-n", h.namespace, "nodepools", name, autoscalConfig, "--type=merge").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

// getNodepoolNodeName get hosted cluster node names by labels
// labelFileter: ${key1}={value1},${key2}={value2} e.g.hypershift.openshift.io/nodePool=hypershift-ci-22374-us-east-2a
func (h *hostedCluster) getHostedClusterNodeNameByLabelFilter(labelFilter string) string {
	nameCond := `-ojsonpath={.items[*].metadata.name}`
	nodesName, err := h.oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run(OcpGet).Args("node", "--ignore-not-found", "-l", labelFilter, nameCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return nodesName
}

func (h *hostedCluster) getHostedClusterNodeReadyStatus(nodeName string) string {
	labelFilter := "kubernetes.io/hostname=" + nodeName
	readyCond := `-ojsonpath={.items[].status.conditions[?(@.type=="Ready")].status}`
	status, err := h.oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run(OcpGet).Args("node", "--ignore-not-found", "-l", labelFilter, readyCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return status
}

// setNodepoolAutoRepair set spec.management.autoRepair value
// enabled: true or false
func (h *hostedCluster) setNodepoolAutoRepair(name, enabled string) {
	autoRepairConfig := fmt.Sprintf(`--patch={"spec": {"management": {"autoRepair": %s}}}`, enabled)
	_, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpPatch).Args("-n", h.namespace, "nodepools", name, autoRepairConfig, "--type=merge").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (h *hostedCluster) pollCheckNodepoolAutoRepairDisabled(name string) func() bool {
	return func() bool {
		return h.checkNodepoolAutoRepairDisabled(name)
	}
}

func (h *hostedCluster) checkNodepoolAutoRepairDisabled(name string) bool {
	//check nodeool status
	autoRepairCond := `-ojsonpath={.status.conditions[?(@.type=="AutorepairEnabled")].status}`
	rc, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "nodepools", name, autoRepairCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if strings.Contains(rc, "True") {
		return false
	}

	//check mhc should not exist
	mchCAPI := "machinehealthchecks.cluster.x-k8s.io"
	rc, err = h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, mchCAPI, name, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if len(rc) > 0 {
		return false
	}

	return true
}

func (h *hostedCluster) pollCheckNodepoolAutoRepairEnabled(name string) func() bool {
	return func() bool {
		return h.checkNodepoolAutoRepairEnabled(name)
	}
}

func (h *hostedCluster) checkNodepoolAutoRepairEnabled(name string) bool {
	//check nodeool status
	autoRepairCond := `-ojsonpath={.status.conditions[?(@.type=="AutorepairEnabled")].status}`
	rc, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "nodepools", name, autoRepairCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if !strings.Contains(rc, "True") {
		return false
	}

	//get np replica
	npReplica, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "nodepools", name, "-ojsonpath={.status.replicas}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	//check mhc currentHealthy, mch name is same with nodepool name
	mchCAPI := "machinehealthchecks.cluster.x-k8s.io"
	currentHealthyNum, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, mchCAPI, name, "-ojsonpath={.status.currentHealthy}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return npReplica == currentHealthyNum
}

func (h *hostedCluster) pollCheckNodeHealthByMHC(mhcName string) func() bool {
	return func() bool {
		return h.checkNodeHealthByMHC(mhcName)
	}
}

// checkNodeHealthByMHC checks if "Expected Machines" is same with "Current Healthy" in MHC
func (h *hostedCluster) checkNodeHealthByMHC(mhcName string) bool {
	mchCAPI := "machinehealthchecks.cluster.x-k8s.io"

	expectedMachineCond := `-ojsonpath={.status.expectedMachines}`
	expectedMachineNum, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, mchCAPI, mhcName, expectedMachineCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	currentHealthyCond := `-ojsonpath={.status.currentHealthy}`
	currentHealthyNum, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, mchCAPI, mhcName, currentHealthyCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return expectedMachineNum == currentHealthyNum
}

func (h *hostedCluster) pollCheckDeletedNodePool(npName string) func() bool {
	return func() bool {
		return h.checkDeletedNodePool(npName)
	}
}

func (h *hostedCluster) checkDeletedNodePool(npName string) bool {
	rc, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "np", npName, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if len(strings.TrimSpace(rc)) > 0 {
		return false
	}

	params := []string{"no", "--ignore-not-found", "-l", "hypershift.openshift.io/nodePool=" + npName}
	rc, err = h.oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run(OcpGet).Args(params...).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if len(strings.TrimSpace(rc)) > 0 {
		return false
	}

	return true
}

func (h *hostedCluster) pollCheckNodepoolCurrentNodes(name, expected string) func() bool {
	return func() bool {
		return h.checkNodepoolCurrentNodes(name, expected)
	}
}

func (h *hostedCluster) checkNodepoolCurrentNodes(name, expected string) bool {
	currentNodes, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", "-n", h.namespace, name, "-o=jsonpath={.status.replicas}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return currentNodes == expected
}

func (h *hostedCluster) isNodepoolAutosaclingEnabled(name string) bool {
	autoScalCond := `-ojsonpath={.status.conditions[?(@.type=="AutoscalingEnabled")].status}`
	autoscaleEnabled, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", "-n", h.namespace, name, autoScalCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return strings.Contains(autoscaleEnabled, "True")
}

func (h *hostedCluster) pollCheckAllNodepoolReady() func() bool {
	return func() bool {
		return h.checkAllNodepoolReady()
	}
}

func (h *hostedCluster) checkAllNodepoolReady() bool {
	nodeReadyCond := fmt.Sprintf(`-ojsonpath={.items[?(@.spec.clusterName=="%s")].status.conditions[?(@.type=="Ready")].status}`, h.name)
	nodesStatus, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", nodeReadyCond, "--namespace", h.namespace).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if len(nodesStatus) <= 0 {
		return true
	}
	if strings.Contains(nodesStatus, "False") {
		return false
	}
	return true
}

type nodePoolCondition struct {
	conditionsType         string
	conditionsTypeReq      string
	expectConditionsResult string
}

func (h *hostedCluster) pollCheckNodePoolConditions(npName string, conditions []nodePoolCondition) func() bool {
	return func() bool {
		return h.checkNodePoolConditions(npName, conditions)
	}
}

func (h *hostedCluster) checkNodePoolConditions(npName string, conditions []nodePoolCondition) bool {
	o.Expect(doOcpReq(h.oc, OcpGet, true, "nodepools", "-n", h.namespace, "-ojsonpath={.items[*].metadata.name}")).Should(o.ContainSubstring(npName))
	for _, condition := range conditions {
		res := doOcpReq(h.oc, OcpGet, false, "nodepools", npName, "-n", h.namespace, fmt.Sprintf(`-ojsonpath={.status.conditions[?(@.type=="%s")].%s}`, condition.conditionsType, condition.conditionsTypeReq))
		e2e.Logf("checkNodePoolStatus: %s, %s, expected: %s, res: %s", condition.conditionsType, condition.conditionsTypeReq, condition.expectConditionsResult, res)
		if !strings.Contains(res, condition.expectConditionsResult) {
			return false
		}
	}
	return true
}

func (h *hostedCluster) getNodepoolPayload(name string) string {
	return doOcpReq(h.oc, OcpGet, true, "nodepools", name, "-n", h.namespace, `-ojsonpath={.spec.release.image}`)
}

func (h *hostedCluster) getNodepoolStatusPayloadVersion(name string) string {
	payloadVersionCond := `-ojsonpath={.status.version}`
	version, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", name, "-n", h.namespace, payloadVersionCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return version
}

func (h *hostedCluster) upgradeNodepoolPayloadInPlace(name, payload string) {
	doOcpReq(h.oc, OcpPatch, true, "nodepools", name, "-n", h.namespace, "--type=json", fmt.Sprintf(`-p=[{"op": "replace", "path": "/spec/release/image","value": "%s"}]`, payload))
}

func (h *hostedCluster) pollCheckUpgradeNodepoolPayload(name, expectPayload, version string) func() bool {
	return func() bool {
		curPayload := h.getNodepoolPayload(name)
		if strings.Contains(curPayload, expectPayload) {
			v := h.getNodepoolStatusPayloadVersion(name)
			if strings.Contains(v, version) {
				return true
			}
		}

		return false
	}
}

// getCPReleaseImage return the .spec.release.image of hostedcluster
// it is set by user and can be treated as expected release
func (h *hostedCluster) getCPReleaseImage() string {
	payload, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "hostedcluster", h.name,
		`-ojsonpath={.spec.release.image}`).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return payload
}

// getCPPayload return current hosted cluster actual payload
func (h *hostedCluster) getCPPayloadTag() string {
	payload, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "hostedcluster", h.name,
		`-ojsonpath={.status.version.history[?(@.state=="Completed")].version}`).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	// for multi payloads just use the first one
	return strings.TrimSpace(strings.Split(payload, " ")[0])
}

// getCPDesiredPayload return desired payload in status
func (h *hostedCluster) getCPDesiredPayload() string {
	payload, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "hostedcluster", h.name,
		`-ojsonpath={.status.version.desired.image}`).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return payload
}

func (h *hostedCluster) upgradeCPPayload(payload string) {
	patchOption := fmt.Sprintf(`-p=[{"op": "replace", "path": "/spec/release/image","value": "%s"}]`, payload)
	_, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpPatch).Args("-n", h.namespace, "hostedcluster", h.name,
		"--type=json", patchOption).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (h *hostedCluster) pollCheckUpgradeCPPayload(payload string) func() bool {
	return func() bool {
		curPayload := h.getCPPayloadTag()
		if strings.Contains(payload, curPayload) {
			return true
		}

		return false
	}
}

func (h *hostedCluster) isFIPEnabled() bool {
	res, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "hostedcluster", h.name, "-ojsonpath={.spec.fips}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	enable, err := strconv.ParseBool(res)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return enable
}

// checkFIPInHostedCluster check FIP settings in hosted cluster nodes
func (h *hostedCluster) checkFIPInHostedCluster() bool {
	nodes, err := h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpGet).Args("no", "-ojsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	for _, nodename := range strings.Split(nodes, " ") {
		res, err := h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpDebug).Args("node/"+nodename, "-q", "--", "fips-mode-setup", "--check").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if !strings.Contains(res, "FIPS mode is enabled") {
			e2e.Logf("Warning: node %s fips-mode-setup check FIP false", nodename)
			return false
		}

		res, err = h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpDebug).Args("node/"+nodename, "-q", "--", "cat", "/proc/sys/crypto/fips_enabled").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if !strings.Contains(res, "1") {
			e2e.Logf("Warning: node %s /proc/sys/crypto/fips_enabled != 1", nodename)
			return false
		}

		res, err = h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpDebug).Args("node/"+nodename, "-q", "--", "sysctl", "crypto.fips_enabled").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if !strings.Contains(res, "crypto.fips_enabled = 1") {
			e2e.Logf("Warning: node %s crypto.fips_enabled != 1", nodename)
			return false
		}

	}
	return true
}

func (h *hostedCluster) isCPHighlyAvailable() bool {
	res, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "hostedcluster", h.name, "-ojsonpath={.spec.controllerAvailabilityPolicy}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return strings.Contains(res, HighlyAvailable)
}

// checkAWSRooVolumes check aws root-volume configurations,
// checkItems: iops, size, type
func (h *hostedCluster) checkAWSRootVolumes(name string, checkItem string, expected interface{}) bool {
	awsmachineVolumeJSONPathPtn := `-ojsonpath={.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].spec.rootVolume.%s}`
	awsmachineVolumeFilter := fmt.Sprintf(awsmachineVolumeJSONPathPtn, h.namespace, name, checkItem)
	nodepoolVolumeFilter := fmt.Sprintf("-ojsonpath={.spec.platform.aws.rootVolume.%s}", checkItem)

	var expectedVal string
	switch v := expected.(type) {
	case string:
		expectedVal = v
	case int64:
		expectedVal = strconv.FormatInt(v, 10)
	case *int64:
		expectedVal = strconv.FormatInt(*v, 10)
	default:
		e2e.Logf("Error: not supported expected value while checking aws nodepool root-volume config")
		return false
	}

	//check nodepool
	rootVolumeConfig, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("np", name, "-n", h.namespace, nodepoolVolumeFilter).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if strings.TrimSpace(rootVolumeConfig) != expectedVal {
		e2e.Logf("Error: nodepool %s rootVolume item %s not matched: return %s and expect %s, original expected %v", name, checkItem, rootVolumeConfig, expectedVal, expected)
		return false
	}

	//check awsmachine
	awsRootVolumeConfig, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("awsmachines", "-n", h.namespace+"-"+h.name, awsmachineVolumeFilter).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if strings.TrimSpace(awsRootVolumeConfig) != expectedVal {
		e2e.Logf("Error: awsmachine for nodepool %s rootVolume item %s not matched: return %s and expect %s, original expected %v", name, checkItem, awsRootVolumeConfig, expectedVal, expected)
		return false
	}
	return true
}

func (h *hostedCluster) checkAWSNodepoolRootVolumeSize(name string, expectedSize int64) bool {
	return h.checkAWSRootVolumes(name, "size", expectedSize)
}

func (h *hostedCluster) checkAWSNodepoolRootVolumeIOPS(name string, expectedIOPS int64) bool {
	return h.checkAWSRootVolumes(name, "iops", expectedIOPS)
}

func (h *hostedCluster) checkAWSNodepoolRootVolumeType(name string, expectedType string) bool {
	return h.checkAWSRootVolumes(name, "type", expectedType)
}

func (h *hostedCluster) setAWSNodepoolInstanceType(name, instanceType string) {
	cond := fmt.Sprintf(`--patch={"spec": {"platform": {"aws": {"instanceType":"%s"}}}}`, instanceType)
	_, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpPatch).Args("-n", h.namespace, "nodepools", name, cond, "--type=merge").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

func (h *hostedCluster) getAWSNodepoolInstanceType(name string) string {
	cond := `-ojsonpath={.spec.platform.aws.instanceType}`
	instanceType, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "nodepools", name, cond, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(instanceType).ShouldNot(o.BeEmpty())
	return instanceType
}

func (h *hostedCluster) getNodepoolUpgradeType(name string) string {
	cond := `-ojsonpath={.spec.management.upgradeType}`
	instanceType, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "nodepools", name, cond, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(instanceType).ShouldNot(o.BeEmpty())
	return instanceType
}

func (h *hostedCluster) pollCheckAWSNodepoolInstanceType(name, expected string) func() bool {
	return func() bool {
		return h.checkAWSNodepoolInstanceType(name, expected)
	}
}

func (h *hostedCluster) checkAWSNodepoolInstanceType(name, expected string) bool {
	// check nodepool instanceType
	instanceType := h.getAWSNodepoolInstanceType(name)
	if instanceType != expected {
		e2e.Logf("instanceType not matched, expected: %s, got: %s", expected, instanceType)
		return false
	}

	// check awsmachinetemplates instanceType
	cond := `-ojsonpath={.spec.template.spec.instanceType}`
	templateInstanceType, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, "awsmachinetemplates", name, cond, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(templateInstanceType).ShouldNot(o.BeEmpty())
	return templateInstanceType == expected
}

func (h *hostedCluster) pollCheckNodepoolRollingUpgradeIntermediateStatus(name string) func() bool {
	return func() bool {
		return h.checkNodepoolRollingUpgradeIntermediateStatus(name)
	}
}

func (h *hostedCluster) checkNodepoolRollingUpgradeIntermediateStatus(name string) bool {
	// check machinedeployment UNAVAILABLE nodes should not be zero
	infraID, err := h.getInfraID()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	cond := `-ojsonpath={.status.unavailableReplicas}`
	unavailableNum, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, "machinedeployment", name, cond, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(unavailableNum).ShouldNot(o.BeEmpty())
	num, err := strconv.Atoi(unavailableNum)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if num <= 0 {
		return false
	}

	// get machinesets.cluster.x-k8s.io according to nodepool
	machinesetCAPI := "machinesets.cluster.x-k8s.io"
	labelFilter := "cluster.x-k8s.io/cluster-name=" + infraID
	format := `-ojsonpath={.items[?(@.metadata.annotations.hypershift\.openshift\.io/nodePool=="%s/%s")].metadata.name}`
	cond = fmt.Sprintf(format, h.namespace, name)
	machinesets, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, machinesetCAPI, "-l", labelFilter, cond, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(machinesets).ShouldNot(o.BeEmpty())

	// a new machineset should be created, so number of machinesets should be 2
	if len(strings.Split(machinesets, " ")) <= 1 {
		return false
	}
	return true
}

func (h *hostedCluster) pollCheckNodepoolRollingUpgradeComplete(name string) func() bool {
	return func() bool {
		return h.checkNodepoolRollingUpgradeComplete(name)
	}
}

func (h *hostedCluster) checkNodepoolRollingUpgradeComplete(name string) bool {
	if !h.checkNodepoolRollingUpgradeCompleteByMachineDeployment(name) {
		e2e.Logf("checkNodepoolRollingUpgradeCompleteByMachineDeployment false")
		return false
	}

	if !h.checkNodePoolReady(name) {
		e2e.Logf("checkNodePoolReady false")
		return false
	}

	if !h.checkNodepoolHostedClusterNodeReady(name) {
		e2e.Logf("checkNodepoolHostedClusterNodeReady false")
		return false
	}
	return true
}

func (h *hostedCluster) getNodepoolReadyReplicas(name string) int {
	// get nodepool ready replics
	replicas, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace, "nodepools", name, "-ojsonpath={.status.replicas}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	replicasNum, err := strconv.Atoi(replicas)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return replicasNum
}

func (h *hostedCluster) getNodepoolHostedClusterReadyNodesNumber(name string) int {
	params := []string{"node", "--ignore-not-found", "-l", "hypershift.openshift.io/nodePool=" + name, `-ojsonpath={.items[*].status.conditions[?(@.type=="Ready")].status}`}
	status, err := h.oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run("get").Args(params...).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	readyNodeNum := strings.Count(status, "True")
	return readyNodeNum
}

// getNodepoolHostedClusterNodes gets hosted cluster ready nodes by nodepool label filer
// name: nodepool name
func (h *hostedCluster) getNodepoolHostedClusterNodes(name string) []string {
	params := []string{"node", "--ignore-not-found", "-l", "hypershift.openshift.io/nodePool=" + name, `-ojsonpath={.items[*].metadata.name}`}
	nameList, err := h.oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run(OcpGet).Args(params...).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if len(strings.TrimSpace(nameList)) <= 0 {
		return []string{}
	}

	return strings.Split(nameList, " ")
}

func (h *hostedCluster) getHostedClusterNodeInstanceType(nodeName string) string {
	params := []string{"node", nodeName, "--ignore-not-found", `-ojsonpath={.metadata.labels.beta\.kubernetes\.io/instance-type}`}
	instanceType, err := h.oc.AsGuestKubeconf().AsAdmin().WithoutNamespace().Run(OcpGet).Args(params...).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(instanceType).ShouldNot(o.BeEmpty())
	return instanceType
}

func (h *hostedCluster) checkNodepoolHostedClusterNodeReady(name string) bool {
	replicasNum := h.getNodepoolReadyReplicas(name)
	readyNodeNum := h.getNodepoolHostedClusterReadyNodesNumber(name)
	return replicasNum == readyNodeNum
}

func (h *hostedCluster) checkNodepoolRollingUpgradeCompleteByMachineDeployment(name string) bool {
	// check machinedeployment status
	cond := `-ojsonpath={.status}`
	statusStr, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpGet).Args("-n", h.namespace+"-"+h.name, "machinedeployment", name, cond, "--ignore-not-found").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Expect(statusStr).ShouldNot(o.BeEmpty())
	status := gjson.Parse(statusStr).Value().(map[string]interface{})

	var unavailable, replicas, ready, updated interface{}
	var ok bool

	//check unavailableReplicas should be zero
	unavailable, ok = status["unavailableReplicas"]
	o.Expect(ok).Should(o.BeTrue())
	unavailableNum, err := strconv.Atoi(fmt.Sprint(unavailable))
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if unavailableNum != 0 {
		return false
	}

	//check replicas == ready == updated
	replicas, ok = status["replicas"]
	o.Expect(ok).Should(o.BeTrue())
	replicaNum, err := strconv.Atoi(fmt.Sprint(replicas))
	o.Expect(err).ShouldNot(o.HaveOccurred())

	ready, ok = status["readyReplicas"]
	o.Expect(ok).Should(o.BeTrue())
	readyNum, err := strconv.Atoi(fmt.Sprint(ready))
	o.Expect(err).ShouldNot(o.HaveOccurred())

	updated, ok = status["updatedReplicas"]
	o.Expect(ok).Should(o.BeTrue())
	updatedNum, err := strconv.Atoi(fmt.Sprint(updated))
	o.Expect(err).ShouldNot(o.HaveOccurred())

	if replicaNum != readyNum || replicaNum != updatedNum {
		return false
	}

	return true
}

func (h *hostedCluster) checkNodepoolHostedClusterNodeInstanceType(npName string) bool {
	expected := h.getAWSNodepoolInstanceType(npName)
	replicas := h.getNodepoolReadyReplicas(npName)
	nodes := h.getNodepoolHostedClusterNodes(npName)
	o.Expect(len(nodes)).Should(o.Equal(replicas))
	for _, name := range nodes {
		instanceType := h.getHostedClusterNodeInstanceType(name)
		if instanceType != expected {
			e2e.Logf("hosted cluster node %s instanceType: %s is not expected %s", name, instanceType, expected)
			return false
		}
	}
	return true
}

// getEtcdLeader return etcd leader pod name and follower name list
func (h *hostedCluster) getCPEtcdLeaderAndFollowers() (string, []string, error) {
	var leader string
	var followers []string
	etcdEndpointStatusCmd := "ETCDCTL_API=3 /usr/bin/etcdctl --cacert /etc/etcd/tls/etcd-ca/ca.crt --cert /etc/etcd/tls/client/etcd-client.crt --key /etc/etcd/tls/client/etcd-client.key --endpoints=localhost:2379 endpoint status"
	replicas := doOcpReq(h.oc, OcpGet, true, "-n", h.namespace+"-"+h.name, "sts", "etcd", `-ojsonpath={.spec.replicas}`)
	totalNum, err := strconv.Atoi(replicas)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	for i := 0; i < totalNum; i++ {
		podName := "etcd-" + strconv.Itoa(i)
		res, err := exutil.RemoteShPodWithBashSpecifyContainer(h.oc, h.namespace+"-"+h.name, podName, "etcd", etcdEndpointStatusCmd)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		e2e.Logf("endpoint status %s", res)
		arr := strings.Split(res, ",")
		o.Expect(len(arr) > 5).Should(o.BeTrue())
		if strings.TrimSpace(arr[4]) == "true" {
			if leader != "" {
				return "", []string{}, fmt.Errorf("multiple leaders found error")
			}
			leader = podName
		} else {
			followers = append(followers, podName)
		}
	}
	if leader == "" {
		return "", []string{}, fmt.Errorf("no leader found error")
	}
	return leader, followers, nil
}

func (h *hostedCluster) getEtcdNodeMapping() map[string]string {
	replicas := doOcpReq(h.oc, OcpGet, true, "-n", h.namespace+"-"+h.name, "sts", "etcd", `-ojsonpath={.spec.replicas}`)
	totalNum, err := strconv.Atoi(replicas)
	o.Expect(err).ShouldNot(o.HaveOccurred())

	etcdNodeMap := make(map[string]string, 1)
	for i := 0; i < totalNum; i++ {
		etcdPod := "etcd-" + strconv.Itoa(i)
		node := doOcpReq(h.oc, OcpGet, true, "-n", h.namespace+"-"+h.name, "pod", etcdPod, `-ojsonpath={.spec.nodeName}`)
		etcdNodeMap[etcdPod] = node
	}
	return etcdNodeMap
}

func (h *hostedCluster) isCPEtcdPodHealthy(podName string) bool {
	etcdEndpointHealthCmd := "ETCDCTL_API=3 /usr/bin/etcdctl --cacert /etc/etcd/tls/etcd-ca/ca.crt --cert /etc/etcd/tls/client/etcd-client.crt --key /etc/etcd/tls/client/etcd-client.key --endpoints=localhost:2379 endpoint health"
	res, err := exutil.RemoteShPodWithBashSpecifyContainer(h.oc, h.namespace+"-"+h.name, podName, "etcd", etcdEndpointHealthCmd)
	if err != nil {
		e2e.Logf("CP ETCD %s is unhealthy with error : %s , \n res: %s", podName, err.Error(), res)
		return false
	}

	if strings.Contains(res, "unhealthy") {
		return false
	}
	return true
}

func (h *hostedCluster) getNodeNameByNodepool(npName string) []string {
	labelFilter := "hypershift.openshift.io/nodePool=" + npName
	nodes := h.getHostedClusterNodeNameByLabelFilter(labelFilter)
	return strings.Split(strings.TrimSpace(nodes), " ")
}

func (h *hostedCluster) getUnstructuredNodePoolByName(ctx context.Context, npName string) (*unstructured.Unstructured, error) {
	// Dynamically obtain the gvr to avoid version change in the future
	npRESTMapping, err := h.oc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: "hypershift.openshift.io",
		Kind:  "NodePool",
	})
	if err != nil {
		return nil, fmt.Errorf("error getting RESTMapping for hypershift.openshift.io/NodePool: %w", err)
	}
	npUnstructured, err := h.oc.AdminDynamicClient().Resource(npRESTMapping.Resource).Namespace(h.namespace).Get(ctx, npName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting NodePool/%s: %w", npName, err)
	}
	hcName, found, err := unstructured.NestedString(npUnstructured.Object, "spec", "clusterName")
	if err != nil || !found {
		return nil, fmt.Errorf("error extracting NodePool.spec.clusterName: %w", err)
	}
	if hcName != h.name {
		return nil, fmt.Errorf("expect NodePool.spec.clusterName to be %s but found to be %s", h.name, hcName)
	}
	return npUnstructured, nil
}

func (h *hostedCluster) getCurrentInfraMachineTemplatesByNodepool(ctx context.Context, npName string) (*unstructured.Unstructured, error) {
	npUnstructured, err := h.getUnstructuredNodePoolByName(ctx, npName)
	if err != nil {
		return nil, fmt.Errorf("error getting unstructured NodePool %s: %w", npName, err)
	}
	platform, found, err := unstructured.NestedString(npUnstructured.Object, "spec", "platform", "type")
	if err != nil || !found {
		return nil, fmt.Errorf("error extracting NodePool.spec.platform.type: %w", err)
	}
	e2e.Logf("Found NodePool/%s platform = %s", npName, platform)
	infraMachineTemplateKind, ok := platform2InfraMachineTemplateKind[platform]
	if !ok {
		return nil, fmt.Errorf("no infra machine template kind for platform %s. Available options: %v", platform, platform2InfraMachineTemplateKind)
	}
	e2e.Logf("Found infra machine template kind = %s", infraMachineTemplateKind)
	infraMachineTemplateRESTMapping, err := h.oc.RESTMapper().RESTMapping(schema.GroupKind{
		Group: capiInfraGroup,
		Kind:  infraMachineTemplateKind,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting RESTMapping for kind %s in group %s: %w", infraMachineTemplateKind, capiInfraGroup, err)
	}
	hcpNs := h.getHostedComponentNamespace()
	if len(hcpNs) == 0 {
		return nil, errors.New("empty hosted component namespace obtained from the hostedCluster object")
	}
	infraMachineTempName, ok := npUnstructured.GetAnnotations()[npInfraMachineTemplateAnnotationKey]
	if !ok {
		return nil, fmt.Errorf("annotation %s not found on NodePool %s", npInfraMachineTemplateAnnotationKey, npName)
	}
	e2e.Logf("Found infra machine template name = %s", infraMachineTempName)

	infraMachineTempUnstructured, err := h.oc.AdminDynamicClient().Resource(infraMachineTemplateRESTMapping.Resource).Namespace(hcpNs).Get(ctx, infraMachineTempName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting infra machine templates %s: %w", infraMachineTempName, err)
	}
	e2e.Logf("Found infra machine template %s", infraMachineTempUnstructured.GetName())
	return infraMachineTempUnstructured, nil
}

func (h *hostedCluster) DebugHostedClusterNodeWithChroot(caseID string, nodeName string, cmd ...string) (string, error) {
	newNamespace := names.SimpleNameGenerator.GenerateName(fmt.Sprintf("hypershift-%s-", caseID))
	defer func() {
		err := h.oc.AsAdmin().AsGuestKubeconf().Run("delete").Args("namespace", newNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}()

	_, err := h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpCreate).Args("namespace", newNamespace).Output()
	if err != nil {
		return "", err
	}

	res, err := h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpGet).Args("ns/"+newNamespace, `-o=jsonpath={.metadata.labels.pod-security\.kubernetes\.io/enforce}`).Output()
	if err != nil {
		return "", err
	}

	if !strings.Contains(res, "privileged") {
		_, err = h.oc.AsGuestKubeconf().WithoutNamespace().Run("label").Args("ns/"+newNamespace, `security.openshift.io/scc.podSecurityLabelSync=false`, `pod-security.kubernetes.io/enforce=privileged`, `pod-security.kubernetes.io/audit=privileged`, `pod-security.kubernetes.io/warn=privileged`, "--overwrite").Output()
		if err != nil {
			return "", err
		}
	}
	res, err = h.oc.AsGuestKubeconf().WithoutNamespace().Run(OcpDebug).Args(append([]string{"node/" + nodeName, "--to-namespace=" + newNamespace, "-q", "--", "chroot", "/host"}, cmd...)...).Output()
	return res, err
}

func (h *hostedCluster) updateHostedClusterAndCheck(oc *exutil.CLI, updateFunc func() error, deployment string) {
	oldVersion := doOcpReq(oc, OcpGet, true, "deployment", deployment, "-n", h.namespace+"-"+h.name, `-ojsonpath={.metadata.annotations.deployment\.kubernetes\.io/revision}`)
	err := updateFunc()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	o.Eventually(func() string {
		return doOcpReq(oc, OcpGet, true, "deployment", deployment, "-n", h.namespace+"-"+h.name, `-ojsonpath={.metadata.annotations.deployment\.kubernetes\.io/revision}`)
	}, DefaultTimeout, DefaultTimeout/10).ShouldNot(o.Equal(oldVersion), deployment+" not restart")
	o.Eventually(func() int {
		return strings.Compare(doOcpReq(oc, OcpGet, true, "deployment", deployment, "-n", h.namespace+"-"+h.name, `-ojsonpath={.status.replicas}`), doOcpReq(oc, OcpGet, true, "deployment", deployment, "-n", h.namespace+"-"+h.name, `-ojsonpath={.status.readyReplicas}`))
	}, LongTimeout, LongTimeout/10).Should(o.Equal(0), deployment+" is not ready")
}

// idpType: HTPasswd, GitLab, GitHub ...
func (h *hostedCluster) checkIDPConfigReady(idpType IdentityProviderType, idpName string, secretName string) bool {
	//check idpType by idpName
	if idpType != doOcpReq(h.oc, OcpGet, false, "hostedcluster", h.name, "-n", h.namespace, "--ignore-not-found", fmt.Sprintf(`-ojsonpath={.spec.configuration.oauth.identityProviders[?(@.name=="%s")].type}`, idpName)) {
		return false
	}

	//check configmap oauth-openshift
	configYaml := doOcpReq(h.oc, OcpGet, false, "configmap", "oauth-openshift", "-n", h.namespace+"-"+h.name, "--ignore-not-found", `-ojsonpath={.data.config\.yaml}`)
	if !strings.Contains(configYaml, fmt.Sprintf("name: %s", idpName)) {
		return false
	}
	if !strings.Contains(configYaml, fmt.Sprintf("kind: %sIdentityProvider", idpType)) {
		return false
	}

	//check secret name if secretName is not empty
	if secretName != "" {
		volumeName := doOcpReq(h.oc, OcpGet, false, "deploy", "oauth-openshift", "-n", h.namespace+"-"+h.name, "--ignore-not-found", fmt.Sprintf(`-ojsonpath={.spec.template.spec.volumes[?(@.secret.secretName=="%s")].name}`, secretName))
		if !strings.Contains(volumeName, "idp-secret") {
			return false
		}
	}
	return true
}

// idpType: HTPasswd, GitLab, GitHub ...
func (h *hostedCluster) pollCheckIDPConfigReady(idpType IdentityProviderType, idpName string, secretName string) func() bool {
	return func() bool {
		return h.checkIDPConfigReady(idpType, idpName, secretName)
	}
}

type etcdEndpointStatusResult []struct {
	Endpoint string                   `json:"Endpoint"`
	Status   *clientv3.StatusResponse `json:"Status"`
}

// getEtcdEndpointStatus gets status of the passed-in endpoints of the hosted cluster's ETCD.
// Omit the endpoints parameter to get status of all endpoints.
func (h *hostedCluster) getEtcdEndpointStatus(endpoints ...string) (etcdEndpointStatusResult, error) {
	var etcdEndpointStatusCmd string
	if len(endpoints) == 0 {
		etcdEndpointStatusCmd = etcdCmdPrefixForHostedCluster + " --endpoints " + etcdLocalClientReqEndpoint + " endpoint status --cluster -w json"
	} else {
		etcdEndpointStatusCmd = etcdCmdPrefixForHostedCluster + " --endpoints " + strings.Join(endpoints, ",") + " endpoint status -w json"
	}
	endpointStatus := doOcpReq(h.oc, OcpExec, true, "-n", h.getHostedComponentNamespace(), "etcd-0", "-c", "etcd", "--", "bash", "-c", etcdEndpointStatusCmd)
	e2e.Logf("Etcd endpoint status response = %s", endpointStatus)

	var res etcdEndpointStatusResult
	if err := json.Unmarshal([]byte(endpointStatus), &res); err != nil {
		return nil, err
	}
	return res, nil
}

// getEtcdEndpointDbStatsByIdx gets DB status of an ETCD endpoint
func (h *hostedCluster) getEtcdEndpointDbStatsByIdx(idx int) (dbSize, dbSizeInUse int64, dbFragRatio float64, err error) {
	var localEtcdEndpointStatus etcdEndpointStatusResult
	etcdEndpoint := h.getEtcdDiscoveryEndpointForClientReqByIdx(idx)
	if localEtcdEndpointStatus, err = h.getEtcdEndpointStatus(etcdEndpoint); err != nil {
		return -1, -1, 0, fmt.Errorf("error querying local ETCD endpoint status: %w", err)
	}

	dbSize, dbSizeInUse = localEtcdEndpointStatus[0].Status.DbSize, localEtcdEndpointStatus[0].Status.DbSizeInUse
	if dbSize == 0 {
		return -1, -1, 0, errors.New("zero dbSize obtained from ETCD server's response")
	}
	if dbSizeInUse == 0 {
		return -1, -1, 0, errors.New("zero dbSizeInUse obtained from ETCD server's response")
	}
	fragRatio := float64(dbSize-dbSizeInUse) / float64(dbSize)
	e2e.Logf("Found ETCD endpoint %s: dbSize = %d, dbSizeInUse = %d, fragmentation ratio = %.2f", etcdEndpoint, dbSize, dbSizeInUse, fragRatio)
	return dbSize, dbSizeInUse, fragRatio, nil
}

func (h *hostedCluster) getEtcdDiscoveryEndpointForClientReqByIdx(idx int) string {
	hcpNs := h.getHostedComponentNamespace()
	return fmt.Sprintf("etcd-%d.%s.%s.svc:%s", idx, etcdDiscoverySvcNameForHostedCluster, hcpNs, etcdClientReqPort)
}

func (h *hostedCluster) checkHCSpecForAzureEtcdEncryption(expected azureKMSKey, isBackupKey bool) {
	keyPath := "activeKey"
	if isBackupKey {
		keyPath = "backupKey"
	}

	keyName := doOcpReq(h.oc, OcpGet, true, "hc", h.name, "-n", h.namespace,
		fmt.Sprintf("-o=jsonpath={.spec.secretEncryption.kms.azure.%s.keyName}", keyPath))
	o.Expect(keyName).To(o.Equal(expected.keyName))
	keyVaultName := doOcpReq(h.oc, OcpGet, true, "hc", h.name, "-n", h.namespace,
		fmt.Sprintf("-o=jsonpath={.spec.secretEncryption.kms.azure.%s.keyVaultName}", keyPath))
	o.Expect(keyVaultName).To(o.Equal(expected.keyVaultName))
	keyVersion := doOcpReq(h.oc, OcpGet, true, "hc", h.name, "-n", h.namespace,
		fmt.Sprintf("-o=jsonpath={.spec.secretEncryption.kms.azure.%s.keyVersion}", keyPath))
	o.Expect(keyVersion).To(o.Equal(expected.keyVersion))
}

func (h *hostedCluster) checkKASEncryptionConfiguration() {
	kasSecretEncryptionConfigSecret := doOcpReq(h.oc, OcpExtract, true,
		fmt.Sprintf("secret/%s", kasEncryptionConfigSecretName), "-n", h.getHostedComponentNamespace(), "--to", "-")
	o.Expect(kasSecretEncryptionConfigSecret).To(o.And(
		o.ContainSubstring("secrets"),
		o.ContainSubstring("configmaps"),
		o.ContainSubstring("routes"),
		o.ContainSubstring("oauthaccesstokens"),
		o.ContainSubstring("oauthauthorizetokens"),
	))
}

func (h *hostedCluster) checkSecretEncryptionDecryption(isEtcdEncrypted bool) {
	var (
		secretName  = fmt.Sprintf("etcd-encryption-%s", strings.ToLower(exutil.RandStrDefault()))
		secretNs    = "default"
		secretKey   = "foo"
		secretValue = "bar"
	)

	e2e.Logf("Creating secret/%s within ns/%s of the hosted cluster", secretName, secretNs)
	doOcpReq(h.oc.AsGuestKubeconf(), OcpCreate, true, "secret", "generic", secretName,
		"-n", secretNs, fmt.Sprintf("--from-literal=%s=%s", secretKey, secretValue))

	e2e.Logf("Checking secret decryption")
	decryptedSecretContent := doOcpReq(h.oc.AsGuestKubeconf(), OcpExtract, true,
		fmt.Sprintf("secret/%s", secretName), "-n", secretNs, "--to", "-")
	o.Expect(decryptedSecretContent).To(o.And(
		o.ContainSubstring(secretKey),
		o.ContainSubstring(secretValue),
	))

	// Unencrypted secrets look like the following:
	// /kubernetes.io/secrets/default/test-secret.<secret-content>
	// Encrypted secrets look like the following:
	// /kubernetes.io/secrets/default/test-secret.k8s:enc:kms:v1:<EncryptionConfiguration-provider-name>:.<encrypted-content>
	if !isEtcdEncrypted {
		return
	}
	e2e.Logf("Checking ETCD encryption")
	etcdCmd := fmt.Sprintf("%s --endpoints %s get /kubernetes.io/secrets/%s/%s | hexdump -C | awk -F '|' '{print $2}' OFS= ORS=",
		etcdCmdPrefixForHostedCluster, etcdLocalClientReqEndpoint, secretNs, secretName)
	encryptedSecretContent, err := exutil.RemoteShPodWithBashSpecifyContainer(h.oc, h.getHostedComponentNamespace(),
		"etcd-0", "etcd", etcdCmd)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to get encrypted secret content within ETCD")
	o.Expect(encryptedSecretContent).NotTo(o.BeEmpty(), "obtained empty encrypted secret content")
	o.Expect(encryptedSecretContent).NotTo(o.ContainSubstring(secretValue))

	e2e.Logf("Deleting secret")
	_ = h.oc.AsGuestKubeconf().Run(OcpDelete).Args("secret", secretName, "-n", secretNs).Execute()
}

// Health checks an HC on Azure with ETCD encryption
func (h *hostedCluster) checkAzureEtcdEncryption(activeKey azureKMSKey, backupKey *azureKMSKey) {
	e2e.Logf("Checking hc.spec.secretEncryption.kms.azure.activeKey")
	h.checkHCSpecForAzureEtcdEncryption(activeKey, false)
	if backupKey != nil {
		e2e.Logf("Checking hc.spec.secretEncryption.kms.azure.backupKey")
		h.checkHCSpecForAzureEtcdEncryption(*backupKey, true)
	}

	e2e.Logf("Checking the ValidAzureKMSConfig condition of the hc")
	validAzureKMSConfigStatus := doOcpReq(h.oc, OcpGet, true, "hc", h.name, "-n", h.namespace,
		`-o=jsonpath={.status.conditions[?(@.type == "ValidAzureKMSConfig")].status}`)
	o.Expect(validAzureKMSConfigStatus).To(o.Equal("True"))

	e2e.Logf("Checking KAS EncryptionConfiguration")
	h.checkKASEncryptionConfiguration()

	e2e.Logf("Checking secret encryption/decryption within the hosted cluster")
	h.checkSecretEncryptionDecryption(true)
}

func (h *hostedCluster) waitForKASDeployUpdate(ctx context.Context, oldResourceVersion string) {
	kasDeployKindAndName := "deploy/kube-apiserver"
	err := exutil.WaitForResourceUpdate(ctx, h.oc, DefaultTimeout/20, DefaultTimeout,
		kasDeployKindAndName, h.getHostedComponentNamespace(), oldResourceVersion)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to wait for KAS deployment to be updated")
}

func (h *hostedCluster) waitForKASDeployReady(ctx context.Context) {
	kasDeployName := "kube-apiserver"
	exutil.WaitForDeploymentsReady(ctx, func(ctx context.Context) (*appsv1.DeploymentList, error) {
		return h.oc.AdminKubeClient().AppsV1().Deployments(h.getHostedComponentNamespace()).List(ctx, metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("metadata.name", kasDeployName).String(),
		})
	}, exutil.IsDeploymentReady, LongTimeout, LongTimeout/20, true)
}

func (h *hostedCluster) patchAzureKMS(activeKey, backupKey *azureKMSKey) {
	patch, err := getHCPatchForAzureKMS(activeKey, backupKey)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to get HC patch Azure KMS")
	doOcpReq(h.oc, OcpPatch, true, "hc", "-n", h.namespace, h.name, "--type=merge", "-p", patch)
}

func (h *hostedCluster) removeAzureKMSBackupKey() {
	doOcpReq(h.oc, OcpPatch, true, "hc", h.name, "-n", h.namespace, "--type=json",
		"-p", `[{"op": "remove", "path": "/spec/secretEncryption/kms/azure/backupKey"}]`)
}

// Re-encode all secrets within a hosted cluster namespace
func (h *hostedCluster) encodeSecretsNs(ctx context.Context, ns string) {
	guestKubeClient := h.oc.GuestKubeClient()
	secrets, err := guestKubeClient.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to list secrets")

	backoff := wait.Backoff{Steps: 10, Duration: 1 * time.Second}
	for _, secret := range secrets.Items {
		err = retry.RetryOnConflict(backoff, func() error {
			// Fetch the latest version of the secret
			latestSecret, getErr := guestKubeClient.CoreV1().Secrets(ns).Get(ctx, secret.Name, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			// Update the secret with the modified version
			_, updateErr := guestKubeClient.CoreV1().Secrets(ns).Update(ctx, latestSecret, metav1.UpdateOptions{})
			return updateErr
		})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to update secret with retry")
	}
}

// Re-encode all secrets within the hosted cluster
func (h *hostedCluster) encodeSecrets(ctx context.Context) {
	namespaces, err := h.oc.GuestKubeClient().CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to list namespaces")
	for _, ns := range namespaces.Items {
		h.encodeSecretsNs(ctx, ns.Name)
	}
}

// Re-encode all configmaps within a hosted cluster namespace
func (h *hostedCluster) encodeConfigmapsNs(ctx context.Context, ns string) {
	guestKubeClient := h.oc.GuestKubeClient()
	configmaps, err := guestKubeClient.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to list configmaps")

	backoff := wait.Backoff{Steps: 10, Duration: 1 * time.Second}
	for _, configmap := range configmaps.Items {
		err = retry.RetryOnConflict(backoff, func() error {
			// Fetch the latest version of the configmap
			latestConfigmap, getErr := guestKubeClient.CoreV1().ConfigMaps(ns).Get(ctx, configmap.Name, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			// Update the configmap with the modified version
			_, updateErr := guestKubeClient.CoreV1().ConfigMaps(ns).Update(ctx, latestConfigmap, metav1.UpdateOptions{})
			return updateErr
		})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to update configmap with retry")
	}
}

// Re-encode all configmaps within the hosted cluster
func (h *hostedCluster) encodeConfigmaps(ctx context.Context) {
	namespaces, err := h.oc.GuestKubeClient().CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to list namespaces")
	for _, ns := range namespaces.Items {
		h.encodeConfigmapsNs(ctx, ns.Name)
	}
}

func (h *hostedCluster) pollUntilReady() {
	o.Eventually(h.pollHostedClustersReady(), ClusterInstallTimeout, ClusterInstallTimeout/20).Should(o.BeTrue())
}

func (h *hostedCluster) getKASResourceVersion() string {
	return doOcpReq(h.oc, OcpGet, true, "deploy/kube-apiserver", "-n", h.getHostedComponentNamespace(), "-o=jsonpath={.metadata.resourceVersion}")
}
