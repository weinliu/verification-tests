package util

import (
	"strings"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// GetFirstLinuxWorkerNode returns the first linux worker node in the cluster
func GetFirstLinuxWorkerNode(oc *CLI) (string, error) {
	var (
		workerNode string
		err        error
	)
	workerNode, err = getFirstNodeByOsID(oc, "worker", "rhcos")
	if len(workerNode) == 0 {
		workerNode, err = getFirstNodeByOsID(oc, "worker", "rhel")
	}
	return workerNode, err
}

// GetAllNodesbyOSType returns a list of the names of all linux/windows nodes in the cluster have both linux and windows node
func GetAllNodesbyOSType(oc *CLI, ostype string) ([]string, error) {
	var nodesArray []string
	nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "kubernetes.io/os="+ostype, "-o", "jsonpath='{.items[*].metadata.name}'").Output()
	nodesStr := strings.Trim(nodes, "'")
	//If split an empty string to string array, the default length string array is 1
	//So need to check if string is empty.
	if len(nodesStr) == 0 {
		return nodesArray, err
	}
	nodesArray = strings.Split(nodesStr, " ")
	return nodesArray, err
}

// GetAllNodes returns a list of the names of all nodes in the cluster
func GetAllNodes(oc *CLI) ([]string, error) {
	nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o", "jsonpath='{.items[*].metadata.name}'").Output()
	return strings.Split(strings.Trim(nodes, "'"), " "), err
}

// GetFirstWorkerNode returns a first worker node
func GetFirstWorkerNode(oc *CLI) (string, error) {
	workerNodes, err := GetClusterNodesBy(oc, "worker")
	return workerNodes[0], err
}

// GetFirstMasterNode returns a first master node
func GetFirstMasterNode(oc *CLI) (string, error) {
	masterNodes, err := GetClusterNodesBy(oc, "master")
	return masterNodes[0], err
}

// GetClusterNodesBy returns the cluster nodes by role
func GetClusterNodesBy(oc *CLI, role string) ([]string, error) {
	nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/"+role, "-o", "jsonpath='{.items[*].metadata.name}'").Output()
	return strings.Split(strings.Trim(nodes, "'"), " "), err
}

// DebugNodeWithChroot creates a debugging session of the node with chroot
func DebugNodeWithChroot(oc *CLI, nodeName string, cmd ...string) (string, error) {
	stdOut, stdErr, err := debugNode(oc, nodeName, []string{}, true, true, cmd...)
	return strings.Join([]string{stdOut, stdErr}, "\n"), err
}

// DebugNodeWithOptions launch debug container with options e.g. --image
func DebugNodeWithOptions(oc *CLI, nodeName string, options []string, cmd ...string) (string, error) {
	stdOut, stdErr, err := debugNode(oc, nodeName, options, false, true, cmd...)
	return strings.Join([]string{stdOut, stdErr}, "\n"), err
}

// DebugNodeWithOptionsAndChroot launch debug container using chroot and with options e.g. --image
func DebugNodeWithOptionsAndChroot(oc *CLI, nodeName string, options []string, cmd ...string) (string, error) {
	stdOut, stdErr, err := debugNode(oc, nodeName, options, true, true, cmd...)
	return strings.Join([]string{stdOut, stdErr}, "\n"), err
}

// DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel launch debug container using chroot and with options e.g. --image
// WithoutRecoverNsLabel which will not recover the labels that added for debug node container adapt the podSecurity changed on 4.12+ test clusters
// "security.openshift.io/scc.podSecurityLabelSync=false" And "pod-security.kubernetes.io/enforce=privileged"
func DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc *CLI, nodeName string, options []string, cmd ...string) (stdOut string, stdErr string, err error) {
	return debugNode(oc, nodeName, options, true, false, cmd...)
}

// DebugNode creates a debugging session of the node
func DebugNode(oc *CLI, nodeName string, cmd ...string) (string, error) {
	stdOut, stdErr, err := debugNode(oc, nodeName, []string{}, false, true, cmd...)
	return strings.Join([]string{stdOut, stdErr}, "\n"), err
}

func debugNode(oc *CLI, nodeName string, cmdOptions []string, needChroot bool, recoverNsLabels bool, cmd ...string) (stdOut string, stdErr string, err error) {
	var (
		debugNodeNamespace string
		isNsPrivileged     bool
		cargs              []string
		outputError        error
	)
	cargs = []string{"node/" + nodeName}
	debugNodeNamespace = oc.Namespace()
	if debugNodeNamespace == "" {
		debugNodeNamespace = "default"
	}
	// Running oc debug node in normal projects
	// (normal projects mean projects that are not clusters default projects like: like "default", "openshift-xxx" et al)
	// need extra configuration on 4.12+ ocp test clusters
	// https://github.com/openshift/oc/blob/master/pkg/helpers/cmd/errors.go#L24-L29
	if debugNodeNamespace != "default" && !strings.HasPrefix(debugNodeNamespace, "openshift-") {
		isNsPrivileged, outputError = IsDebugNodeNamespacePrivileged(oc, debugNodeNamespace)
		if outputError != nil {
			return "", "", outputError
		}
		if !isNsPrivileged {
			if recoverNsLabels {
				defer RecoverDebugNodeNamespaceRestricted(oc, debugNodeNamespace)
			}
			outputError = SetDebugNodeNamespacePrivileged(oc, debugNodeNamespace)
			if outputError != nil {
				return "", "", outputError
			}
		}
	}
	if len(cmdOptions) > 0 {
		cargs = append(cargs, cmdOptions...)
	}
	// Overwrite the debug node namespace add the "--to-namespace=" between cmdOptions and cmd
	// Make sure to use the privileged namespace to setup the debugPod
	// E.g. $ oc debug -n xx node/ip-10-0-142-72.us-east-2.compute.internal --to-namespace=de  --to-namespace=des -n openshift-tests
	// Error from server (NotFound): namespaces "des" not found
	// "oc debug node" command will use the last "--to-namespace=$(namespace)" as the final namespace even if has more than 1 this options
	cargs = append(cargs, "--to-namespace="+debugNodeNamespace)
	if needChroot {
		cargs = append(cargs, "--", "chroot", "/host")
	} else {
		cargs = append(cargs, "--")
	}
	cargs = append(cargs, cmd...)
	return oc.AsAdmin().WithoutNamespace().Run("debug").Args(cargs...).Outputs()
}

// IsDebugNodeNamespacePrivileged returns whether the input ns has the security label
// Privileged label : "pod-security.kubernetes.io/enforce=privileged"
func IsDebugNodeNamespacePrivileged(oc *CLI, namespace string) (bool, error) {
	nsSecurityLabelValue, err := GetResourceSpecificLabelValue(oc, "ns/"+namespace, "", "pod-security\\.kubernetes\\.io/enforce")
	if err != nil {
		e2e.Logf("Failed to get label \"pod-security.kubernetes.io/enforce\" value from ns/%s:\"%v\"", namespace, err)
		return false, err
	}
	return strings.Contains(nsSecurityLabelValue, "privileged"), nil
}

// SetDebugNodeNamespacePrivileged adds the privileged labels to the input ns
// Privileged labels : "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged"
func SetDebugNodeNamespacePrivileged(oc *CLI, namespace string) error {
	_, labeledError := AddLabelToSpecificResource(oc, "ns/"+namespace, "", "security.openshift.io/scc.podSecurityLabelSync", "false")
	if labeledError != nil {
		e2e.Logf("Failed to add label \"security.openshift.io/scc.podSecurityLabelSync=false\" to ns/%s :\"%v\"", namespace, labeledError)
		return labeledError
	}
	_, labeledError = AddLabelToSpecificResource(oc, "ns/"+namespace, "", "pod-security.kubernetes.io/enforce", "privileged")
	if labeledError != nil {
		e2e.Logf("Failed to add label \"pod-security.kubernetes.io/enforce=privileged\" to ns/%s :\"%v\"", namespace, labeledError)
		return labeledError
	}
	return nil
}

// RecoverDebugNodeNamespaceRestricted removes the privileged labels from the input ns
// Privileged labels : "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged"
func RecoverDebugNodeNamespaceRestricted(oc *CLI, namespace string) error {
	_, unlabeledError := DeleteLabelFromSpecificResource(oc, "ns/"+namespace, "", "security.openshift.io/scc.podSecurityLabelSync")
	if unlabeledError != nil {
		e2e.Logf("Failed to recover label \"security.openshift.io/scc.podSecurityLabelSync\" for ns/%s :\"%v\"", namespace, unlabeledError)
		return unlabeledError
	}
	_, unlabeledError = DeleteLabelFromSpecificResource(oc, "ns/"+namespace, "", "pod-security.kubernetes.io/enforce")
	if unlabeledError != nil {
		e2e.Logf("Failed to recover label \"pod-security.kubernetes.io/enforce\" for ns/%s :\"%v\"", namespace, unlabeledError)
		return unlabeledError
	}
	return nil
}

// DeleteLabelFromNode delete the custom label from the node
func DeleteLabelFromNode(oc *CLI, node string, label string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node, label+"-").Output()
}

// AddLabelToNode add the custom label to the node
func AddLabelToNode(oc *CLI, node string, label string, value string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("label").Args("node", node, label+"="+value).Output()
}

// GetFirstCoreOsWorkerNode returns the first CoreOS worker node
func GetFirstCoreOsWorkerNode(oc *CLI) (string, error) {
	return getFirstNodeByOsID(oc, "worker", "rhcos")
}

// GetFirstRhelWorkerNode returns the first rhel worker node
func GetFirstRhelWorkerNode(oc *CLI) (string, error) {
	return getFirstNodeByOsID(oc, "worker", "rhel")
}

// getFirstNodeByOsID returns the cluster node by role and os id
func getFirstNodeByOsID(oc *CLI, role string, osID string) (string, error) {
	nodes, err := GetClusterNodesBy(oc, role)
	for _, node := range nodes {
		stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node/"+node, "-o", "jsonpath=\"{.metadata.labels.node\\.openshift\\.io/os_id}\"").Output()
		if strings.Trim(stdout, "\"") == osID {
			return node, err
		}
	}
	return "", err
}

// GetNodeHostname returns the cluster node hostname
func GetNodeHostname(oc *CLI, node string) (string, error) {
	hostname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o", "jsonpath='{..kubernetes\\.io/hostname}'").Output()
	return strings.Trim(hostname, "'"), err
}
