package hypershift

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	o "github.com/onsi/gomega"
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
		return strings.Contains(desiredNodes, currentNodes)
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

func (h *hostedCluster) getNodepoolPayload(name string) string {
	payloadCond := `-ojsonpath={.spec.release.image}`
	payload, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", name, "-n", h.namespace, payloadCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return payload
}

func (h *hostedCluster) getNodepoolStatusPayloadVersion(name string) string {
	payloadVersionCond := `-ojsonpath={.status.version}`
	version, err := h.oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", name, "-n", h.namespace, payloadVersionCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return version
}

func (h *hostedCluster) upgradeNodepoolPayloadInPlace(name, payload string) {
	inplacePatchCond := `-p=[{"op": "replace", "path": "/spec/management/upgradeType", "value": "InPlace"}]`
	_, err := h.oc.AsAdmin().WithoutNamespace().Run(OcpPatch).Args("-n", h.namespace, "np", name,
		"--type=json", inplacePatchCond).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	patchOption := fmt.Sprintf(`-p=[{"op": "replace", "path": "/spec/release/image","value": "%s"}]`, payload)
	_, err = h.oc.AsAdmin().WithoutNamespace().Run(OcpPatch).Args("-n", h.namespace, "np", name,
		"--type=json", patchOption).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
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
