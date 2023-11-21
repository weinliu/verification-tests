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
	e2e "k8s.io/kubernetes/test/e2e/framework"
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
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		instanceState, err := upi.upiObj.GetUPIbaremetalInstanceState()
		if err != nil {
			e2e.Logf("Start instance failed with error :: %v.", err)
			return false, nil
		} else if instanceState == "poweredOn" {
			e2e.Logf("VM instance of master node has already started up :: %v", upi.nodeName)
			return true, nil
		} else if instanceState == "poweredOff" {
			err := upi.upiObj.StartUPIbaremetalInstance()
			if err == nil {
				e2e.Logf("VM Instance of master node has been started :: %v", upi.nodeName)
				return true, nil
			} else {
				return false, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to start VM instance of master node :: %s", upi.nodeName))
	return errVmstate
}

func (upi *UPIInstance) Stop() error {
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		instanceState, err := upi.upiObj.GetUPIbaremetalInstanceState()
		if err != nil {
			e2e.Logf("Stop instance failed with error :: %v.", err)
			return false, nil
		} else if instanceState == "poweredOff" {
			e2e.Logf("VM Instance of master node has been already down :: %v", upi.nodeName)
			return true, nil
		} else if instanceState == "poweredOn" {
			err := upi.upiObj.StopUPIbaremetalInstance()
			if err == nil {
				e2e.Logf("VM Instance of master node has been brought down:: %v", upi.nodeName)
				return true, nil
			} else {
				return false, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to stop VM instance of master node :: %s", upi.nodeName))
	return errVmstate
}

func (upi *UPIInstance) State() (string, error) {
	var nodeStatus string
	var statusErr error
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		nodeStatus, statusErr = upi.upiObj.GetUPIbaremetalInstanceState()
		if statusErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Failed to get VM instance state for master node: %s, error: %s", upi.nodeName, statusErr))
	return nodeStatus, statusErr
}
