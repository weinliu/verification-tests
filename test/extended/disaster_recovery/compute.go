package disasterrecovery

import (
	"fmt"
	"os/exec"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	cvers "github.com/openshift/openshift-tests-private/test/extended/mco"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// ComputeNode interface to handle compute node e.g. start or stop
type ComputeNode interface {
	GetName() string
	GetInstanceID() (string, error)
	Start() error
	Stop() error
	State() (string, error)
}

type instance struct {
	nodeName string
	oc       *exutil.CLI
}

func (i *instance) GetName() string {
	return i.nodeName
}

// ComputeNodes handles ComputeNode interface
type ComputeNodes []ComputeNode

// GetDrMasterNodes list nodes according to plaforn and creds.
func GetDrMasterNodes(oc *exutil.CLI) (ComputeNodes, func()) {
	platform := exutil.CheckPlatform(oc)
	switch platform {
	case "aws":
		e2e.Logf("\n AWS is detected, running the case on AWS\n")
		return GetAwsMasterNodes(oc)
	case "gcp":
		e2e.Logf("\n GCP is detected, running the case on gcp\n")
		return GetGcpMasterNodes(oc)
	case "vsphere":
		e2e.Logf("\n vsphere is detected, running the case on vsphere\n")
		return GetVsphereMasterNodes(oc)
	case "openstack":
		e2e.Logf("\n OSP is detected, running the case on osp\n")
		return GetOspMasterNodes(oc)
	case "azure":
		e2e.Logf("\n Azure is detected, running the case on azure\n")
		return GetAzureMasterNodes(oc)
	case "baremetal":
		e2e.Logf("\n IPI Baremetal is detected, running the case on baremetal\n")
		return GetBaremetalMasterNodes(oc)
	case "none":
		e2e.Logf("\n UPI Baremetal is detected, running the case on baremetal\n")
		return GetUPIBaremetalMasterNodes(oc)
	default:
		g.Skip("Not support cloud provider for DR cases for now. Test cases should be run on vsphere or aws or gcp or openstack or azure or IPI baremetal, skip for other platforms!!")
	}
	return nil, nil
}

func (n ComputeNodes) leaderMasterNodeName(oc *exutil.CLI) ComputeNode {
	// get clusterversion
	e2e.Logf("Checking clusterversion")
	clusterVersion, _, err := exutil.GetClusterVersion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Get leader node")
	var leaderNode string
	if cvers.CompareVersions(clusterVersion, ">", "4.9") {
		leaderNode, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("leases", "kube-controller-manager", "-n", "kube-system", "-o=jsonpath={.spec.holderIdentity}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		masterStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "kube-controller-manager", "-n", "kube-system", "-o", "jsonpath='{.metadata.annotations}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		jqCmd := fmt.Sprintf(`echo %s | jq -r .'"control-plane.alpha.kubernetes.io/leader"'| jq -r  .holderIdentity`, masterStr)
		masterNode, err := exec.Command("bash", "-c", jqCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		leaderNode = string(masterNode)
	}
	masterNodeStr := strings.Split(leaderNode, "_")
	//Changing format for gcp
	if exutil.CheckPlatform(oc) == "gcp" {
		masterNodeStr = strings.Split(masterNodeStr[0], ".")
	}
	for _, node := range n {
		if strings.Contains(node.GetName(), masterNodeStr[0]) {
			e2e.Logf("Leader master node is :: %v", node.GetName())
			return node
		}
	}
	return nil
}
