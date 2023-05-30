package disasterrecovery

import (
	"fmt"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type nutanixInstance struct {
	instance
	client *exutil.NutanixSession
}

// GetNutanixMasterNodes get master nodes and load clouds cred.
func GetNutanixMasterNodes(oc *exutil.CLI) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, "master")
	o.Expect(err).NotTo(o.HaveOccurred())
	nutanixUsername, nutanixPassword, nutanixEndpointURL, credErr := exutil.GetNutanixCredentialFromCluster(oc)
	o.Expect(credErr).NotTo(o.HaveOccurred())
	nutanixSession, sessErr := exutil.NewNutanixSession(nutanixUsername, nutanixPassword, nutanixEndpointURL)
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newNutanixInstance(oc, nutanixSession, nodeName))
	}
	return results, nil
}

func newNutanixInstance(oc *exutil.CLI, client *exutil.NutanixSession, nodeName string) *nutanixInstance {
	return &nutanixInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		client: client,
	}
}

func (nux *nutanixInstance) GetInstanceID() (string, error) {
	instanceID, err := nux.client.GetNutanixInstanceID(nux.nodeName)
	if err == nil {
		e2e.Logf("VM instance name: %s", instanceID)
		return instanceID, nil
	}
	return "", err
}

func (nux *nutanixInstance) Start() error {
	instanceID, idErr := nux.client.GetNutanixInstanceID(nux.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	err := nux.client.SetNutanixInstanceState("ON", instanceID)
	o.Expect(err).NotTo(o.HaveOccurred())
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		vmState, err := nux.client.GetNutanixInstanceState(nux.nodeName)
		if err != nil {
			return false, err
		} else if vmState != "running" {
			return false, nil
		}
		e2e.Logf("%s has been restarted", nux.nodeName)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to start %s", nux.nodeName))
	return errVmstate
}

func (nux *nutanixInstance) Stop() error {
	instanceID, idErr := nux.client.GetNutanixInstanceID(nux.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	err := nux.client.SetNutanixInstanceState("OFF", instanceID)
	o.Expect(err).NotTo(o.HaveOccurred())
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		vmState, err := nux.client.GetNutanixInstanceState(nux.nodeName)
		if err != nil {
			return false, err
		} else if vmState != "stopped" {
			return false, nil
		}
		e2e.Logf("%s has been stopped", nux.nodeName)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to poweroff %s", nux.nodeName))
	return errVmstate
}

func (nux *nutanixInstance) State() (string, error) {
	return nux.client.GetNutanixInstanceState(nux.nodeName)
}
