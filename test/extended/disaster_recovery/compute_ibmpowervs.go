package disasterrecovery

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ibmPowerVsInstance struct {
	instance
	ibmRegion     string
	ibmVpcName    string
	clientPowerVs *exutil.IBMPowerVsSession
}

// Get nodes and load clouds cred with the specified label.
func GetIBMPowerNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	ibmApiKey, ibmRegion, ibmVpcName, credErr := exutil.GetIBMCredentialFromCluster(oc)
	o.Expect(credErr).NotTo(o.HaveOccurred())
	cloudID := exutil.GetIBMPowerVsCloudID(oc, nodeNames[0])
	ibmSession, sessErr := exutil.LoginIBMPowerVsCloud(ibmApiKey, ibmRegion, ibmVpcName, cloudID)
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newIBMPowerInstance(oc, ibmSession, ibmRegion, ibmVpcName, nodeName))
	}
	return results, nil
}

func newIBMPowerInstance(oc *exutil.CLI, clientPowerVs *exutil.IBMPowerVsSession, ibmRegion, ibmVpcName, nodeName string) *ibmPowerVsInstance {
	return &ibmPowerVsInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		clientPowerVs: clientPowerVs,
		ibmRegion:     ibmRegion,
		ibmVpcName:    ibmVpcName,
	}
}

func (ibmPws *ibmPowerVsInstance) GetInstanceID() (string, error) {
	instanceID, _, err := exutil.GetIBMPowerVsInstanceInfo(ibmPws.clientPowerVs, ibmPws.nodeName)
	if err == nil {
		e2e.Logf("VM instance ID: %s", instanceID)
		return instanceID, nil
	}
	return "", err
}

func (ibmPws *ibmPowerVsInstance) Start() error {
	instanceID, _, idErr := exutil.GetIBMPowerVsInstanceInfo(ibmPws.clientPowerVs, ibmPws.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	return exutil.PerformInstanceActionOnPowerVs(ibmPws.clientPowerVs, instanceID, "start")
}

func (ibmPws *ibmPowerVsInstance) Stop() error {
	instanceID, _, idErr := exutil.GetIBMPowerVsInstanceInfo(ibmPws.clientPowerVs, ibmPws.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	return exutil.PerformInstanceActionOnPowerVs(ibmPws.clientPowerVs, instanceID, "stop")
}

func (ibmPws *ibmPowerVsInstance) State() (string, error) {
	_, status, idErr := exutil.GetIBMPowerVsInstanceInfo(ibmPws.clientPowerVs, ibmPws.nodeName)
	o.Expect(idErr).NotTo(o.HaveOccurred())
	return status, idErr
}
