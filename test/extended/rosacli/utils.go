package rosacli

import (
	"os"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	rosacli "github.com/openshift/openshift-tests-private/test/extended/util/rosacli"
)

// Get the clusterID env.
func getClusterIDENVExisted() string {
	return os.Getenv("CLUSTER_ID")
}

// Check if the cluster is hosted-cp cluster
func isHostedCPCluster(clusterID string) (bool, error) {
	var rosaClient = rosacli.NewClient()
	rosaClient.Runner.Format("json")
	clusterService := rosaClient.Cluster
	output, err := clusterService.DescribeCluster(clusterID)
	if err != nil {
		logger.Errorf("it met error when describeCluster in isHostedCPCluster is %v", err)
		return false, err
	}
	rosaClient.Runner.CloseFormat()
	rosaClient.Parser.JsonData.Input(output)
	jsonData := rosaClient.Parser.JsonData.Input(output).Parse()
	return jsonData.DigBool("hypershift", "enabled"), nil
}
