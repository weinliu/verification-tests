package disasterrecovery

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ibmInstance struct {
	instance
	ibmRegion  string
	ibmVpcName string
	client     *exutil.IBMSession
}

// Get nodes and load clouds cred with the specified label.
func GetIbmNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	ibmApiKey, ibmRegion, ibmVpcName, credErr := exutil.GetIBMCredentialFromCluster(oc)
	o.Expect(credErr).NotTo(o.HaveOccurred())
	ibmSession, sessErr := exutil.NewIBMSessionFromEnv(ibmApiKey)
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newIbmInstance(oc, ibmSession, ibmRegion, ibmVpcName, nodeName))
	}
	return results, nil
}

func newIbmInstance(oc *exutil.CLI, client *exutil.IBMSession, ibmRegion, ibmVpcName, nodeName string) *ibmInstance {
	return &ibmInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		client:     client,
		ibmRegion:  ibmRegion,
		ibmVpcName: ibmVpcName,
	}
}

func (ibm *ibmInstance) GetInstanceID() (string, error) {
	instanceID, err := exutil.GetIBMInstanceID(ibm.client, ibm.oc, ibm.ibmRegion, ibm.ibmVpcName, ibm.nodeName)
	if err == nil {
		e2e.Logf("VM instance name: %s", instanceID)
		return instanceID, nil
	}
	return "", err
}

func (ibm *ibmInstance) Start() error {
	instanceID, idErr := exutil.GetIBMInstanceID(ibm.client, ibm.oc, ibm.ibmRegion, ibm.ibmVpcName, ibm.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	return exutil.StartIBMInstance(ibm.client, instanceID)

}

func (ibm *ibmInstance) Stop() error {
	instanceID, idErr := exutil.GetIBMInstanceID(ibm.client, ibm.oc, ibm.ibmRegion, ibm.ibmVpcName, ibm.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	return exutil.StopIBMInstance(ibm.client, instanceID)
}

func (ibm *ibmInstance) State() (string, error) {
	instanceID, idErr := exutil.GetIBMInstanceID(ibm.client, ibm.oc, ibm.ibmRegion, ibm.ibmVpcName, ibm.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	return exutil.GetIBMInstanceStatus(ibm.client, instanceID)
}
