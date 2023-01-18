package util

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// GetClusterVersion returns the cluster version as string value (Ex: 4.8) and cluster build (Ex: 4.8.0-0.nightly-2021-09-28-165247)
func GetClusterVersion(oc *CLI) (string, string, error) {
	clusterBuild, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.version}").Output()
	if err != nil {
		return "", "", err
	}
	splitValues := strings.Split(clusterBuild, ".")
	clusterVersion := splitValues[0] + "." + splitValues[1]
	return clusterVersion, clusterBuild, err
}

// GetInfraID returns the infra id
func GetInfraID(oc *CLI) (string, error) {
	infraID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath='{.status.infrastructureName}'").Output()
	if err != nil {
		return "", err
	}
	return strings.Trim(infraID, "'"), err
}

// GetGcpProjectID returns the gcp project id
func GetGcpProjectID(oc *CLI) (string, error) {
	projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath='{.status.platformStatus.gcp.projectID}'").Output()
	if err != nil {
		return "", err
	}
	return strings.Trim(projectID, "'"), err
}

// GetClusterPrefixName return Cluster Prefix Name
func GetClusterPrefixName(oc *CLI) string {
	output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("route", "console", "-n", "openshift-console", "-o=jsonpath={.spec.host}").Output()
	if err != nil {
		e2e.Logf("Get cluster console route failed with err %v .", err)
		return ""
	}
	return strings.Split(output, ".")[2]
}

// GetClusterArchitecture return ClusterArchitecture
// If ClusterArchitecture is multi-arch, return Multi-Arch
func GetClusterArchitecture(oc *CLI) string {
	architecture := ""
	output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("nodes", "-o=jsonpath={.items[*].status.nodeInfo.architecture}").Output()
	if err != nil {
		e2e.Failf("get architecture failed err %v .", err)
	}
	if output == "" {
		e2e.Failf("get architecture failed")
	}
	architectureList := strings.Split(output, " ")
	architecture = architectureList[0]
	for _, architectureIndex := range architectureList {
		if architectureIndex != architecture {
			e2e.Logf("architecture %s", output)
			return "Multi-Arch"
		}
	}
	return architecture
}

// SkipARM64 skip the test if cluster is arm64
func SkipARM64(oc *CLI) {
	arch := GetClusterArchitecture(oc)
	e2e.Logf("architecture is " + arch)
	if arch == "arm64" {
		g.Skip("Skip for arm64")
	}
	if arch == "Multi-Arch" {
		g.Skip("Skip for Multi-Arch")
	}
}

// SkipBaselineCaps skip the test if cluster has no required resources.
// sets is comma separated list of baselineCapabilitySets to skip.
// for example: "None, v4.11"
func SkipBaselineCaps(oc *CLI, sets string) {
	baselineCapabilitySet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.spec.capabilities.baselineCapabilitySet}").Output()
	if err != nil {
		e2e.Failf("get baselineCapabilitySet failed err %v .", err)
	}
	sets = strings.ReplaceAll(sets, " ", "")
	for _, s := range strings.Split(sets, ",") {
		if strings.Contains(baselineCapabilitySet, s) {
			g.Skip("Skip for cluster with baselineCapabilitySet = " + s)
		}
	}
}

// GetAWSClusterRegion returns AWS region of the cluster
func GetAWSClusterRegion(oc *CLI) (string, error) {
	region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
	return region, err
}

// SkipNoDefaultSC skip the test if cluster has no default storageclass or has more than 1 default storageclass
func SkipNoDefaultSC(oc *CLI) {
	allSCRes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	defaultSCRes := gjson.Get(allSCRes, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
	e2e.Logf("The default storageclass list: %s", defaultSCRes)
	defaultSCNub := len(defaultSCRes.Array())
	if defaultSCNub != 1 {
		e2e.Logf("oc get sc:\n%s", allSCRes)
		g.Skip("Skip for unexpected default storageclass!")
	}
}

// SkipHypershift skip the test on a Hypershift cluster
func SkipHypershift(oc *CLI) {
	topology, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructures.config.openshift.io", "cluster", "-o=jsonpath={.status.controlPlaneTopology}").Output()
	if err != nil {
		e2e.Failf("get controlPlaneTopology failed err %v .", err)
	}
	if topology == "" {
		e2e.Failf("failure: controlPlaneTopology returned empty")
	}
	if topology == "External" {
		g.Skip("Skip for Hypershift cluster")
	}
}
