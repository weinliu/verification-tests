package hypershift

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
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
				"ValidReleaseImage True", "ValidOIDCConfiguration True", "ReconciliationSucceeded True"})
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

func (h *hostedCluster) createAwsNodePoolWithoutDefaultSG(name string, nodeCount int, dir string) {
	npFile := dir + "/np.yaml"
	var bashClient = NewCmdClient().WithShowInfo(true)
	cmd := fmt.Sprintf("hypershift create nodepool aws --name %s --namespace %s --cluster-name %s --node-count %d --render > %s", name, h.namespace, h.name, nodeCount, npFile)
	_, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	//delete default sg sed '/securityGroups/, +1d'
	cmdSed := fmt.Sprintf("sed -i '/securityGroups/, +1d' %s", npFile)
	_, err = bashClient.Run(cmdSed).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	err = h.oc.AsAdmin().WithoutNamespace().Run(OcpApply).Args("-f", npFile).Execute()
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

	//if not autoscaleEnabled, check repicas is as expected
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
		res := doOcpReq(h.oc, OcpGet, true, "nodepools", npName, "-n", h.namespace, fmt.Sprintf(`-ojsonpath={.status.conditions[?(@.type=="%s")].%s}`, condition.conditionsType, condition.conditionsTypeReq))
		e2e.Logf("checkNodePoolStatus: %s, %s, %s", condition.conditionsType, condition.conditionsTypeReq, res)
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

func (h *hostedCluster) upgradeNodepoolPayloadInPlace(name, payload string, isInPlace bool) {
	if isInPlace {
		doOcpReq(h.oc, OcpPatch, true, "nodepools", name, "-n", h.namespace, "--type=json", `-p=[{"op": "replace", "path": "/spec/management/upgradeType", "value": "InPlace"}]`)
	}
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
	awsmachineVolumeJSONPathPtn := `-ojsonpath={.items[?(@.metadata.annotations.cluster\.x-k8s\.io/cloned-from-name=="%s")].spec.rootVolume.%s}`
	awsmachineVolumeFilter := fmt.Sprintf(awsmachineVolumeJSONPathPtn, name, checkItem)
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

	// a new machineset should be created, so number of machinsets should be 2
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
