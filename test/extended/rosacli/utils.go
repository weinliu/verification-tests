package rosacli

import (
	"os"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

// Get the clusterID env.
func getClusterIDENVExisted() string {
	return os.Getenv("CLUSTER_ID")
}

// Check if the cluster is hosted-cp cluster
func isHostedCPCluster(clusterID string) (bool, error) {
	var rosaClient = NewClient()
	rosaClient.Runner.format = "json"
	clusterService := rosaClient.Cluster
	output, err := clusterService.describeCluster(clusterID)
	if err != nil {
		logger.Errorf("it met error when describeCluster in isHostedCPCluster is %v", err)
		return false, err
	}
	rosaClient.Runner.CloseFormat()
	jsonData := rosaClient.Parser.jsonData.Input(output).Parse()
	return jsonData.digBool("hypershift", "enabled"), nil
}
