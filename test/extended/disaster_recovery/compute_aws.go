package disasterrecovery

import (
	"fmt"
	"strings"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type awsInstance struct {
	instance
	client *exutil.AwsClient
}

// GetAwsNodes get nodes and load clouds cred with the specified label.
func GetAwsNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	clusterinfra.GetAwsCredentialFromCluster(oc)
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newAwsInstance(oc, exutil.InitAwsSession(), nodeName))
	}
	return results, nil
}

func (a *awsInstance) GetInstanceID() (string, error) {
	instanceID, err := a.client.GetAwsInstanceIDFromHostname(a.nodeName)
	if err != nil {
		e2e.Logf("Get instance id failed with error :: %v.", err)
		return "", err
	}
	return instanceID, nil
}

func newAwsInstance(oc *exutil.CLI, client *exutil.AwsClient, nodeName string) *awsInstance {
	return &awsInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		client: client,
	}
}

func (a *awsInstance) Start() error {
	instanceID, err := a.client.GetAwsInstanceIDFromHostname(a.nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	instanceState, err := a.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := stopStates[instanceState]; ok {
		err = a.client.StartInstance(instanceID)
		if err != nil {
			return fmt.Errorf("start instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to start instance %s from status %s", a.nodeName, instanceState)
	}
	return nil
}

func (a *awsInstance) Stop() error {
	instanceID, err := a.client.GetAwsInstanceIDFromHostname(a.nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	instanceState, err := a.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := startStates[instanceState]; ok {
		err = a.client.StopInstance(instanceID)
		if err != nil {
			return fmt.Errorf("stop instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to stop instance %s from status %s", a.nodeName, instanceState)
	}
	return nil
}

func (a *awsInstance) State() (string, error) {
	instanceID, err := a.client.GetAwsInstanceIDFromHostname(a.nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	instanceState, err := a.client.GetAwsInstanceState(instanceID)
	if err != nil {
		e2e.Logf("Get instance state failed with error :: %v.", err)
		return "", err
	}
	return strings.ToLower(instanceState), nil
}
