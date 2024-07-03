package storage

import (
	"fmt"
	"strings"
	"time"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

// Execute command in node
func execCommandInSpecificNode(oc *exutil.CLI, nodeHostName string, command string) (output string, err error) {
	debugNodeNamespace := oc.Namespace()
	executeCmd := append([]string{"/bin/sh", "-c"}, command)
	executeOption := []string{"-q"}
	// Check whether current namespace is Active
	nsState, err := oc.AsAdmin().Run("get").Args("ns/"+debugNodeNamespace, "-o=jsonpath={.status.phase}", "--ignore-not-found").Output()
	if nsState != "Active" || err != nil {
		debugNodeNamespace = "default"
		executeOption = append(executeOption, "--to-namespace="+debugNodeNamespace)
	}

	// Running oc debug node in normal projects
	// (normal projects mean projects that are not clusters default projects like: like "default", "openshift-xxx" et al)
	// need extra configuration on 4.12+ ocp test clusters
	// https://github.com/openshift/oc/blob/master/pkg/helpers/cmd/errors.go#L24-L29
	var stdOut, stdErr string
	// Retry to avoid system issue: "error: unable to create the debug pod ..."
	wait.Poll(10*time.Second, 30*time.Second, func() (bool, error) {
		stdOut, stdErr, err = exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, nodeHostName, executeOption, executeCmd...)
		debugLogf("Executed %q on node %q:\n*stdErr* :%q\nstdOut :%q\n*err*: %v", command, nodeHostName, stdErr, stdOut, err)
		if err != nil {
			e2e.Logf("Executed %q on node %q failed of %v", command, nodeHostName)
			return false, nil
		}
		return true, nil
	})

	// Adapt Pod Security changed on k8s v1.23+
	// https://kubernetes.io/docs/tutorials/security/cluster-level-pss/
	// Ignore the oc debug node output warning info: "Warning: would violate PodSecurity "restricted:latest": host namespaces (hostNetwork=true, hostPID=true), ..."
	if strings.Contains(strings.ToLower(stdErr), "warning") {
		output = stdOut
	} else {
		output = strings.TrimSpace(strings.Join([]string{stdErr, stdOut}, "\n"))
	}
	if err != nil {
		e2e.Logf("Execute \""+command+"\" on node \"%s\" *failed with* : \"%v\".", nodeHostName, err)
		return output, err
	}
	debugLogf("Executed \""+command+"\" on node \"%s\" *Output is* : \"%v\".", nodeHostName, output)
	e2e.Logf("Executed \""+command+"\" on node \"%s\" *Successfully* ", nodeHostName)
	return output, nil
}

// Check the Volume mounted on the Node
func checkVolumeMountOnNode(oc *exutil.CLI, volumeName string, nodeName string) {
	command := "mount | grep " + volumeName
	err := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		_, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" mount on node: \"%s\" failed", volumeName, nodeName))
}

// Check the Volume not mounted on the Node
func checkVolumeNotMountOnNode(oc *exutil.CLI, volumeName string, nodeName string) {
	command := "mount | grep -c \"" + volumeName + "\" || true"
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		count, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			e2e.Logf("Err Occurred: %v, trying again ...", err)
			return false, nil
		}
		if count == "0" {
			e2e.Logf("Volume: \"%s\" umount from node \"%s\" successfully", volumeName, nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" unmount from node: \"%s\" timeout", volumeName, nodeName))
}

// Check the Volume not detached from the Node
func checkVolumeDetachedFromNode(oc *exutil.CLI, volumeName string, nodeName string) {
	command := "lsblk | grep -c \"" + volumeName + "\" || true"
	err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
		count, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			e2e.Logf("Err Occurred: %v, trying again ...", err)
			return false, nil
		}
		if count == "0" {
			e2e.Logf("Volume: \"%s\" detached from node \"%s\" successfully", volumeName, nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" detached from node: \"%s\" timeout", volumeName, nodeName))
}

// Check the mounted volume on the Node contains content by cmd
func checkVolumeMountCmdContain(oc *exutil.CLI, volumeName string, nodeName string, content string) {
	command := "mount | grep " + volumeName
	err := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		msg, err := execCommandInSpecificNode(oc, nodeName, command)
		if err != nil {
			e2e.Logf("Err Occurred: %v, trying again ...", err)
			return false, nil
		}
		return strings.Contains(msg, content), nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check volume: \"%s\" mount in node : \"%s\" contains  \"%s\" failed", volumeName, nodeName, content))
}

// Get the Node List for pod with label
func getNodeListForPodByLabel(oc *exutil.CLI, namespace string, labelName string) ([]string, error) {
	podsList, err := getPodsListByLabel(oc, namespace, labelName)
	o.Expect(err).NotTo(o.HaveOccurred())
	var nodeList []string
	for _, pod := range podsList {
		nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", pod, "-n", namespace, "-o=jsonpath={.spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%s is on Node:\"%s\"", pod, nodeName)
		nodeList = append(nodeList, nodeName)
	}
	return nodeList, err
}

// GetNodeNameByPod gets the pod located node's name
func getNodeNameByPod(oc *exutil.CLI, namespace string, podName string) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The nodename in namespace %s for pod %s is %s", namespace, podName, nodeName)
	return nodeName
}

// Get the cluster worker nodes info
func getWorkersInfo(oc *exutil.CLI) string {
	workersInfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker", "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return workersInfo
}

func getWorkersList(oc *exutil.CLI) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Fields(output)
}

// Get the compact node list, compact node has both master and worker role on it
func getCompactNodeList(oc *exutil.CLI) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/master,node-role.kubernetes.io/worker", "--ignore-not-found", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Fields(output)
}

// Get the cluster schedulable worker nodes names with the same available zone or without the available zone
func getTwoSchedulableWorkersWithSameAz(oc *exutil.CLI) (schedulableWorkersWithSameAz []string, azName string) {
	var (
		allNodes                   = getAllNodesInfo(oc)
		allSchedulableLinuxWorkers = getSchedulableLinuxWorkers(allNodes)
		schedulableWorkersWithAz   = make(map[string]string)
	)
	for _, schedulableLinuxWorker := range allSchedulableLinuxWorkers {
		azName = schedulableLinuxWorker.availableZone
		if azName == "" {
			azName = "noneAzCluster"
		}
		if _, ok := schedulableWorkersWithAz[azName]; ok {
			e2e.Logf("Schedulable workers %s,%s in the same az %s", schedulableLinuxWorker.name, schedulableWorkersWithAz[azName], azName)
			return append(schedulableWorkersWithSameAz, schedulableLinuxWorker.name, schedulableWorkersWithAz[azName]), azName
		}
		schedulableWorkersWithAz[azName] = schedulableLinuxWorker.name
	}
	e2e.Logf("*** The test cluster has less than two schedulable linux workers in each available zone! ***")
	return nil, azName
}

// Drain specified node
func drainSpecificNode(oc *exutil.CLI, nodeName string) {
	e2e.Logf("oc adm drain nodes/" + nodeName + " --ignore-daemonsets --delete-emptydir-data --force --timeout=600s")
	err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("drain", "nodes/"+nodeName, "--ignore-daemonsets", "--delete-emptydir-data", "--force", "--timeout=600s").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func drainNodeWithPodLabel(oc *exutil.CLI, nodeName string, podLabel string) {
	e2e.Logf("oc adm drain nodes/" + nodeName + " --pod-selector" + podLabel + " --ignore-daemonsets --delete-emptydir-data --force --timeout=600s")
	err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("drain", "nodes/"+nodeName, "--pod-selector", "app="+podLabel, "--ignore-daemonsets", "--delete-emptydir-data", "--force", "--timeout=600s").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Uncordon specified node
func uncordonSpecificNode(oc *exutil.CLI, nodeName string) error {
	e2e.Logf("oc adm uncordon nodes/" + nodeName)
	return oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", "nodes/"+nodeName).Execute()
}

// Waiting specified node available: scheduleable and ready
func waitNodeAvailable(oc *exutil.CLI, nodeName string) {
	err := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		nodeInfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes/"+nodeName, "-o", "json").Output()
		if err != nil {
			e2e.Logf("Get node status Err Occurred: \"%v\", try next round", err)
			return false, nil
		}
		if !gjson.Get(nodeInfo, `spec.unschedulable`).Exists() && gjson.Get(nodeInfo, `status.conditions.#(type=Ready).status`).String() == "True" {
			e2e.Logf("Node: \"%s\" is ready to use", nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting Node: \"%s\" become ready to use timeout", nodeName))
}

// Get Region info
func getClusterRegion(oc *exutil.CLI) string {
	node := getWorkersList(oc)[0]
	region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o=jsonpath={.metadata.labels.failure-domain\\.beta\\.kubernetes\\.io\\/region}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return region
}

// Check zoned or un-zoned nodes in cluster, currently works for azure only
func checkNodeZoned(oc *exutil.CLI) bool {
	// https://kubernetes-sigs.github.io/cloud-provider-azure/topics/availability-zones/#node-labels
	if cloudProvider == "azure" {
		node := getWorkersList(oc)[0]
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o=jsonpath={.metadata.labels.failure-domain\\.beta\\.kubernetes\\.io\\/zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		region := getClusterRegion(oc)
		e2e.Logf("The zone is %s", zone)
		e2e.Logf("The region is %s", region)
		//if len(zone) == 1 {
		if !strings.Contains(zone, region) {
			return false
		}
	}
	return true
}

type node struct {
	name                        string
	instanceID                  string
	availableZone               string
	osType                      string
	osImage                     string
	osID                        string
	role                        []string
	scheduleable                bool
	readyStatus                 string // "True", "Unknown"(Node is poweroff or disconnect), "False"
	architecture                string
	allocatableEphemeralStorage string
	ephemeralStorageCapacity    string
	instanceType                string
	isNoScheduleTaintsEmpty     bool
}

// Get cluster all node information
func getAllNodesInfo(oc *exutil.CLI) []node {
	var (
		// nodes []node
		nodes    = make([]node, 0, 10)
		zonePath = `metadata.labels.topology\.kubernetes\.io\/zone`
	)
	nodesInfoJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodesList := strings.Split(strings.Trim(strings.Trim(gjson.Get(nodesInfoJSON, "items.#.metadata.name").String(), "["), "]"), ",")
	for _, nodeName := range nodesList {
		nodeName = strings.Trim(nodeName, "\"")
		nodeRole := make([]string, 0, 4)

		labelList := strings.Split(strings.Trim(gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").metadata.labels").String(), "\n "), ",")
		for i := 0; i < len(labelList); i++ {
			if strings.Contains(labelList[i], `node-role.kubernetes.io/`) {
				nodeRole = append(nodeRole, strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(strings.Trim(labelList[i], "\n")), "\": \"\""), "\"node-role.kubernetes.io/"))
			}
		}

		nodeAvailableZone := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+")."+zonePath).String()
		// Enhancement: It seems sometimes aws worker node miss kubernetes az label, maybe caused by other parallel cases
		if nodeAvailableZone == "" && cloudProvider == "aws" {
			e2e.Logf("The node \"%s\" kubernetes az label not exist, retry get from csi az label", nodeName)
			zonePath = `metadata.labels.topology\.ebs\.csi\.aws\.com\/zone`
			nodeAvailableZone = gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+")."+zonePath).String()
		}
		readyStatus := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").status.conditions.#(type=Ready).status").String()
		scheduleFlag := !gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").spec.unschedulable").Exists()
		nodeOsType := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").metadata.labels.kubernetes\\.io\\/os").String()
		nodeOsID := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").metadata.labels.node\\.openshift\\.io\\/os_id").String()
		nodeOsImage := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").status.nodeInfo.osImage").String()
		nodeArch := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").status.nodeInfo.architecture").String()
		nodeEphemeralStorageCapacity := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").status.capacity.ephemeral-storage").String()
		nodeAllocatableEphemeralStorage := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").status.allocatable.ephemeral-storage").String()
		tempSlice := strings.Split(gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+")."+"spec.providerID").String(), "/")
		nodeInstanceID := tempSlice[len(tempSlice)-1]
		nodeInstanceType := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").metadata.labels.node\\.kubernetes\\.io\\/instance-type").String()

		// taints: [{"effect":"PreferNoSchedule","key":"UpdateInProgress"}] could be tolerance since it is a prefer strategy the workloads still could schedule to these nodes
		nodeTaints := gjson.Get(nodesInfoJSON, "items.#(metadata.name="+nodeName+").spec.taints|@ugly|@flatten").String()
		isNoScheduleTaintsEmpty := !strings.Contains(nodeTaints, `"effect":"NoSchedule"`)
		if strSliceContains(nodeRole, "worker") && nodeTaints != "" {
			e2e.Logf("The worker node %q has taints: %s", nodeName, nodeTaints)
		}

		nodes = append(nodes, node{
			name:                        nodeName,
			instanceID:                  nodeInstanceID,
			instanceType:                nodeInstanceType,
			availableZone:               nodeAvailableZone,
			osType:                      nodeOsType,
			osID:                        nodeOsID,
			osImage:                     nodeOsImage,
			role:                        nodeRole,
			scheduleable:                scheduleFlag,
			architecture:                nodeArch,
			readyStatus:                 readyStatus,
			allocatableEphemeralStorage: nodeAllocatableEphemeralStorage,
			ephemeralStorageCapacity:    nodeEphemeralStorageCapacity,
			isNoScheduleTaintsEmpty:     isNoScheduleTaintsEmpty,
		})
	}
	e2e.Logf("*** The \"%s\" Cluster nodes info is ***:\n \"%+v\"", cloudProvider, nodes)
	return nodes
}

// Get all schedulable linux wokers
func getSchedulableLinuxWorkers(allNodes []node) (linuxWorkers []node) {
	linuxWorkers = make([]node, 0, 6)
	if len(allNodes) == 1 { // In case of SNO cluster
		linuxWorkers = append(linuxWorkers, allNodes...)
	} else {
		for _, myNode := range allNodes {
			if myNode.scheduleable && myNode.osType == "linux" && strSliceContains(myNode.role, "worker") && !strSliceContains(myNode.role, "infra") && !strSliceContains(myNode.role, "edge") && myNode.isNoScheduleTaintsEmpty && myNode.readyStatus == "True" {
				linuxWorkers = append(linuxWorkers, myNode)
			}
		}
	}
	e2e.Logf("The schedulable linux workers are: \"%+v\"", linuxWorkers)
	return linuxWorkers
}

// Get all schedulable rhel wokers
func getSchedulableRhelWorkers(allNodes []node) []node {
	schedulableRhelWorkers := make([]node, 0, 6)
	if len(allNodes) == 1 { // In case of SNO cluster
		schedulableRhelWorkers = append(schedulableRhelWorkers, allNodes...)
	} else {
		for _, myNode := range allNodes {
			if myNode.scheduleable && myNode.osID == "rhel" && strSliceContains(myNode.role, "worker") && !strSliceContains(myNode.role, "infra") && !strSliceContains(myNode.role, "edge") && myNode.isNoScheduleTaintsEmpty && myNode.readyStatus == "True" {
				schedulableRhelWorkers = append(schedulableRhelWorkers, myNode)
			}
		}
	}
	e2e.Logf("The schedulable RHEL workers are: \"%+v\"", schedulableRhelWorkers)
	return schedulableRhelWorkers
}

// Get one cluster schedulable linux worker, rhel linux worker first
// If no schedulable linux worker found, skip directly
func getOneSchedulableWorker(allNodes []node) (expectedWorker node) {
	expectedWorker = getOneSchedulableWorkerWithoutAssert(allNodes)
	if len(expectedWorker.name) == 0 {
		g.Skip("Skipped: Currently no schedulable workers could satisfy the tests")
	}
	return expectedWorker
}

// Get one cluster schedulable linux worker, rhel linux worker first
// If no schedulable linux worker found, failed directly
func getOneSchedulableWorkerWithAssert(allNodes []node) (expectedWorker node) {
	expectedWorker = getOneSchedulableWorkerWithoutAssert(allNodes)
	if len(expectedWorker.name) == 0 {
		e2e.Failf("Currently no schedulable workers could satisfy the tests")
	}
	return expectedWorker
}

// Get one cluster schedulable linux worker, rhel linux worker first
func getOneSchedulableWorkerWithoutAssert(allNodes []node) (expectedWorker node) {
	schedulableRhelWorkers := getSchedulableRhelWorkers(allNodes)
	if len(schedulableRhelWorkers) != 0 {
		expectedWorker = schedulableRhelWorkers[0]
	} else {
		if len(allNodes) == 1 { // In case of SNO cluster
			expectedWorker = allNodes[0]
		} else {
			for _, myNode := range allNodes {
				// For aws local zones cluster the default LOCALZONE_WORKER_SCHEDULABLE value is no in both official document and CI configuration
				if myNode.scheduleable && myNode.osType == "linux" && strSliceContains(myNode.role, "worker") && !strSliceContains(myNode.role, "infra") && !strSliceContains(myNode.role, "edge") && myNode.isNoScheduleTaintsEmpty && myNode.readyStatus == "True" {
					expectedWorker = myNode
					break
				}
			}
		}
	}
	e2e.Logf("Get the schedulableWorker is \"%+v\"", expectedWorker)
	return expectedWorker
}

// Get one cluster schedulable master worker
func getOneSchedulableMaster(allNodes []node) (expectedMater node) {
	if len(allNodes) == 1 { // In case of SNO cluster
		expectedMater = allNodes[0]
	} else {
		for _, myNode := range allNodes {
			if myNode.scheduleable && myNode.osType == "linux" && strSliceContains(myNode.role, "master") && myNode.readyStatus == "True" {
				expectedMater = myNode
				break
			}
		}
	}
	e2e.Logf("Get the schedulableMaster is \"%+v\"", expectedMater)
	o.Expect(expectedMater.name).NotTo(o.BeEmpty())
	return expectedMater
}

// Get 2 schedulable worker nodes with different available zones
func getTwoSchedulableWorkersWithDifferentAzs(oc *exutil.CLI) []node {
	var (
		expectedWorkers            = make([]node, 0, 2)
		allNodes                   = getAllNodesInfo(oc)
		allSchedulableLinuxWorkers = getSchedulableLinuxWorkers(allNodes)
	)
	if len(allSchedulableLinuxWorkers) < 2 {
		e2e.Logf("Schedulable workers less than 2")
		return expectedWorkers
	}
	for i := 1; i < len(allSchedulableLinuxWorkers); i++ {
		if allSchedulableLinuxWorkers[0].availableZone != allSchedulableLinuxWorkers[i].availableZone {
			e2e.Logf("2 Schedulable workers with different available zones are: [%v|%v]", allSchedulableLinuxWorkers[0], allSchedulableLinuxWorkers[i])
			return append(expectedWorkers, allSchedulableLinuxWorkers[0], allSchedulableLinuxWorkers[i])
		}
	}
	e2e.Logf("All Schedulable workers are the same az")
	return expectedWorkers
}
