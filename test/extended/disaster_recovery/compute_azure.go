package disasterrecovery

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type azureInstance struct {
	instance
	azureRGname string
	client      *exutil.AzureSession
}

// GetAzureMasterNodes get master nodes and load clouds cred.
func GetAzureMasterNodes(oc *exutil.CLI) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, "master")
	o.Expect(err).NotTo(o.HaveOccurred())
	azureRGname, rgerr := exutil.GetAzureCredentialFromCluster(oc)
	o.Expect(rgerr).NotTo(o.HaveOccurred())
	azureSession, sessErr := exutil.NewAzureSessionFromEnv()
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newAzureInstance(oc, azureSession, azureRGname, nodeName))
	}
	return results, nil
}

func newAzureInstance(oc *exutil.CLI, client *exutil.AzureSession, azureRGname, nodeName string) *azureInstance {
	return &azureInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		client:      client,
		azureRGname: azureRGname,
	}
}

func (az *azureInstance) GetInstanceID() (string, error) {
	instanceID, err := exutil.GetAzureVMInstance(az.client, az.nodeName, az.azureRGname)
	if err == nil {
		e2e.Logf("VM instance name: %s", instanceID)
		return instanceID, nil
	}
	return "", err
}

func (az *azureInstance) Start() error {
	_, err := exutil.StartAzureVM(az.client, az.nodeName, az.azureRGname)
	o.Expect(err).NotTo(o.HaveOccurred())
	return err
}

func (az *azureInstance) Stop() error {
	_, err := exutil.StopAzureVM(az.client, az.nodeName, az.azureRGname)
	o.Expect(err).NotTo(o.HaveOccurred())
	return err
}

func (az *azureInstance) State() (string, error) {
	instanceState, err := exutil.GetAzureVMInstanceState(az.client, az.nodeName, az.azureRGname)
	o.Expect(err).NotTo(o.HaveOccurred())
	return instanceState, err
}
