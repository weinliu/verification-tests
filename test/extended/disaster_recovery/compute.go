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

// GetNodes get master nodes according to platform and creds with the specified label.
func GetNodes(oc *exutil.CLI, label string) (ComputeNodes, func()) {
	platform := exutil.CheckPlatform(oc)
	switch platform {
	case "aws":
		e2e.Logf("\n AWS is detected, running the case on AWS\n")
		return GetAwsNodes(oc, label)
	case "gcp":
		e2e.Logf("\n GCP is detected, running the case on gcp\n")
		return GetGcpNodes(oc, label)
	case "vsphere":
		e2e.Logf("\n vsphere is detected, running the case on vsphere\n")
		return GetVsphereNodes(oc, label)
	case "openstack":
		e2e.Logf("\n OSP is detected, running the case on osp\n")
		return GetOspNodes(oc, label)
	case "azure":
		e2e.Logf("\n Azure is detected, running the case on azure\n")
		return GetAzureNodes(oc, label)
	case "baremetal":
		e2e.Logf("\n IPI Baremetal is detected, running the case on baremetal\n")
		return GetBaremetalNodes(oc, label)
	case "none":
		e2e.Logf("\n UPI Baremetal is detected, running the case on baremetal\n")
		return GetUPIBaremetalNodes(oc, label)
	case "ibmcloud":
		e2e.Logf("\n IBM is detected, running the case on IBM\n")
		return GetIbmNodes(oc, label)
	case "nutanix":
		e2e.Logf("\n Nutanix is detected, running the case on nutanix\n")
		return GetNutanixNodes(oc, label)
	case "powervs":
		e2e.Logf("\n IBM Powervs is detected, running the case on PowerVs\n")
		return GetIBMPowerNodes(oc, label)
	default:
		g.Skip("Not support cloud provider for DR cases for now. Test cases should be run on IBM or vsphere or aws or gcp or openstack or azure or baremetal, skip for other platforms!!")
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
	if exutil.CheckPlatform(oc) == "gcp" || exutil.CheckPlatform(oc) == "openstack" {
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
