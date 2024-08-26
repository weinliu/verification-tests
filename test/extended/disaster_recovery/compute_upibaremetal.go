package disasterrecovery

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path"
	"strings"
	"time"

	ipmi "github.com/vmware/goipmi"
	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type UPIInstance struct {
	instance
	upiObj    *exutil.RDU2Host
	upiClient *ipmi.Client
	buildid   string
}

const RDU2BaseDomain = "arm.eng.rdu2.redhat.com"

// GetUPIBaremetalNodes get nodes by label and returns a list of ComputeNode objects with the required information to
// control the nodes.
func GetUPIBaremetalNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	if !strings.Contains(nodeNames[0], RDU2BaseDomain) {
		g.Skip("Currently, UPI baremetal DR is only supported on RDU2 clusters.")
	}
	// Get Bastion Host Address
	bastionHost := os.Getenv("QE_BASTION_PUBLIC_ADDRESS")
	if bastionHost == "" {
		g.Fail("Failed to get the RDU2 bastion address, failing.")
	}

	// Parse the hosts.yaml file to get the RDU2Host objects.
	hostsFilePath := path.Join(os.Getenv("SHARED_DIR"), "hosts.yaml")
	_, err = os.Stat(hostsFilePath)
	o.Expect(err).NotTo(o.HaveOccurred())
	yamlBytes, err := os.ReadFile(hostsFilePath)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Unmarshal the yaml into a slice of RDU2Host objects
	var yamlData []exutil.RDU2Host
	err = yaml.Unmarshal(yamlBytes, &yamlData)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Convert slice to map of name to RDU2Host objects to allow lookup by name
	var hostsMap = make(map[string]*exutil.RDU2Host)
	for n := range yamlData {
		yamlData[n].JumpHost = bastionHost
		hostsMap[yamlData[n].Name] = &yamlData[n]
	}

	// Create the UPIInstance objects and the results slice
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		o.Expect(err).NotTo(o.HaveOccurred())
		results = append(results, newUPIbaremetalInstance(oc, nodeName,
			hostsMap[strings.Split(nodeName, ".")[0]]))
	}

	return results, nil
}

func newUPIbaremetalInstance(oc *exutil.CLI, nodeName string, host *exutil.RDU2Host) *UPIInstance {
	return &UPIInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		upiObj: host,
	}
}

// GetInstanceID returns the instance ID of the node (the host key's value), but it's not used for UPI baremetal.
// Previously, it was returning the bmc_address, but it is not the instance ID and is now meaningless/unreachable.
func (upi *UPIInstance) GetInstanceID() (string, error) {
	return upi.upiObj.Host, nil
}

// Start starts the instance
func (upi *UPIInstance) Start() error {
	instanceState, err := upi.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := stopStates[instanceState]; ok {
		err = upi.upiObj.StartUPIbaremetalInstance()
		if err != nil {
			return fmt.Errorf("start instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to start instance %s from status %s", upi.nodeName, instanceState)
	}
	return nil
}

// Stop stops the instance
func (upi *UPIInstance) Stop() error {
	instanceState, err := upi.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := startStates[instanceState]; ok {
		err = upi.upiObj.StopUPIbaremetalInstance()
		if err != nil {
			return fmt.Errorf("stop instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to stop instance %s from status %s", upi.nodeName, instanceState)
	}
	return nil
}

// State returns the state of the instance
func (upi *UPIInstance) State() (string, error) {
	var (
		instanceState string
		statusErr     error
	)
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		instanceState, statusErr = upi.upiObj.GetMachinePowerStatus()
		if statusErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate,
		fmt.Sprintf("Failed to get power status for master node: %s, error: %s", upi.nodeName, statusErr))
	return strings.ToLower(instanceState), statusErr
}
