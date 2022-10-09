package util

import (
	"strings"

	g "github.com/onsi/ginkgo"
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
