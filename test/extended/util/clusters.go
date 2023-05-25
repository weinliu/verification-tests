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

// GetReleaseImage returns the release image as string value (Ex: registry.ci.openshift.org/ocp/release@sha256:b13971e61312f5dddd6435ccf061ac1a8447285a85828456edcd4fc2504cfb8f)
func GetReleaseImage(oc *CLI) (string, error) {
	releaseImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.image}").Output()
	if err != nil {
		return "", err
	}
	return releaseImage, nil
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

// SkipIfPlatformTypeNot skips all platforms other than supported
// platforms is comma separated list of allowed platforms
// for example: "gcp, aws"
func SkipIfPlatformTypeNot(oc *CLI, platforms string) {
	platformType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
	if err != nil {
		e2e.Failf("get infrastructure platformStatus type failed err %v .", err)
	}
	if !strings.Contains(strings.ToLower(platforms), strings.ToLower(platformType)) {
		g.Skip("Skip for non-" + platforms + " cluster: " + platformType)
	}
}

// IsHypershiftHostedCluster
func IsHypershiftHostedCluster(oc *CLI) bool {
	topology, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructures.config.openshift.io", "cluster", "-o=jsonpath={.status.controlPlaneTopology}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Compare(topology, "External") == 0
}

// SkipHypershiftHostedCluster skip the test on a Hypershift cluster
// NOTE: according to @kuiwang02 the preffered method is NonHyperShiftHOST label
// leaving this one as a second line of defence just in case
func SkipHypershiftHostedCluster(oc *CLI) {
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

// IsSTSCluster judges whether the test cluster is using the STS mode
func IsSTSCluster(oc *CLI) bool {
	tempCredentials, extractErr := oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", "openshift-image-registry", "secret/installer-cloud-credentials", "--keys=credentials", "--to=-").Output()
	o.Expect(extractErr).ShouldNot(o.HaveOccurred(), "Failed to extract the temp credentials for checking whether the cluster is using STS mode")
	return strings.Contains(tempCredentials, "web_identity_token_file")
}
