package disasterrecovery

import (
	"fmt"
	"strings"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type nutanixInstance struct {
	instance
	client *exutil.NutanixSession
}

// Get nodes and load clouds cred with the specified label.
func GetNutanixNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
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
	instanceState, err := nux.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := stopStates[strings.ToLower(instanceState)]; ok {
		err = nux.client.SetNutanixInstanceState("ON", instanceID)
		if err != nil {
			return fmt.Errorf("start instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to start instance %s from status %s", nux.nodeName, instanceState)
	}
	return nil
}

func (nux *nutanixInstance) Stop() error {
	instanceID, idErr := nux.client.GetNutanixInstanceID(nux.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	instanceState, err := nux.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := startStates[strings.ToLower(instanceState)]; ok {
		err = nux.client.SetNutanixInstanceState("OFF", instanceID)
		if err != nil {
			return fmt.Errorf("stop instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to stop instance %s from status %s", nux.nodeName, instanceState)
	}
	return nil
}

func (nux *nutanixInstance) State() (string, error) {
	return nux.client.GetNutanixInstanceState(nux.nodeName)
}
