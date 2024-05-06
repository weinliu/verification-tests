package util

import (
	"strings"

	g "github.com/onsi/ginkgo/v2"
)

// We are no longer updating this file because we deprecated it,
// the new file is test/extended/util/clusterinfra/machine_helpers.go
// This file is not deleted because there are some old dependencies
const (
	MachineAPINamespace = "openshift-machine-api"
	//MapiMachineset means the fullname of mapi machineset
	MapiMachineset = "machinesets.machine.openshift.io"
	//MapiMachine means the fullname of mapi machine
	MapiMachine = "machines.machine.openshift.io"
)

// CheckPlatform check the cluster's platform
func CheckPlatform(oc *CLI) string {
	output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
	return strings.ToLower(output)
}

// SkipForSNOCluster skip for SNO cluster
func SkipForSNOCluster(oc *CLI) {
	//Only 1 master, 1 worker node and with the same hostname.
	masterNodes, _ := GetClusterNodesBy(oc, "master")
	workerNodes, _ := GetClusterNodesBy(oc, "worker")
	if len(masterNodes) == 1 && len(workerNodes) == 1 && masterNodes[0] == workerNodes[0] {
		g.Skip("Skip for SNO cluster.")
	}
}
