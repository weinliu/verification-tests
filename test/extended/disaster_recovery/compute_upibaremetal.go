package disasterrecovery

import (
	"fmt"
	"os/exec"
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
	upiObj    exutil.Upirdusession
	upiClient *ipmi.Client
	buildid   string
}

// Get nodes and load clouds cred with the specified label.
func GetUPIBaremetalNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	// Currently DR on UPI baremetal only supported for RDU intances.
	if !strings.Contains(nodeNames[0], "qeclusters.arm.eng.rdu2.redhat.com") {
		g.Skip("Currently DR on UPI baremetal only supported for RDU intances.")
	}
	configid, configErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("get-contexts", "--no-headers").Output()
	o.Expect(configErr).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf(`echo '%v' | awk '{print $3}'| head -1 | tr -d '\n\t'`, configid)
	buildid, builderr := exec.Command("bash", "-c", cmd).Output()
	o.Expect(builderr).NotTo(o.HaveOccurred())
	var results []ComputeNode
	url := fmt.Sprintf(`http://openshift-qe-bastion.arm.eng.rdu2.redhat.com:7788/%v/`, string(buildid))
	for _, nodeName := range nodeNames {
		ipmiSesssion, err := exutil.FetchUPIBareMetalCredentials(url, nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		results = append(results, newUPIbaremetalInstance(oc, nodeName, string(buildid), ipmiSesssion))
	}
	return results, nil
}

func newUPIbaremetalInstance(oc *exutil.CLI, nodeName string, buildid string, ipmiSession exutil.Upirdusession) *UPIInstance {
	return &UPIInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		upiObj:  ipmiSession,
		buildid: buildid,
	}
}

func (upi *UPIInstance) GetInstanceID() (string, error) {
	var instanceID string
	var err error
	errVmId := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		instanceID, err = upi.upiObj.GetUPIbaremetalInstance(upi.buildid, upi.nodeName)
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmId, fmt.Sprintf("Failed to get VM instance ID for node: %s, error: %s", upi.nodeName, err))
	return instanceID, err
}

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

func (upi *UPIInstance) State() (string, error) {
	var (
		instanceState string
		statusErr     error
	)
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		instanceState, statusErr = upi.upiObj.GetUPIbaremetalInstanceState()
		if statusErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Failed to get VM instance state for master node: %s, error: %s", upi.nodeName, statusErr))
	return strings.ToLower(instanceState), statusErr
}
