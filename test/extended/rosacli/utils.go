package rosacli

import "os"

// Get the clusterID env.
func getClusterIDENVExisted() string {
	return os.Getenv("CLUSTER_ID")
}
