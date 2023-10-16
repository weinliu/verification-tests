package disasterrecovery

import (
	"fmt"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type awsInstance struct {
	instance
	client *exutil.AwsClient
}

// GetAwsNodes get nodes and load clouds cred with the specified label.
func GetAwsNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	exutil.GetAwsCredentialFromCluster(oc)
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
	errVMState := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
		vmState, err := a.State()
		o.Expect(err).NotTo(o.HaveOccurred())
		if vmState == "stopped" {
			err = a.client.StartInstance(instanceID)
			if err != nil {
				e2e.Logf("Start instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if vmState == "running" {
			e2e.Logf("%s already running", a.nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVMState, fmt.Sprintf("Not able to restart %s", a.nodeName))
	return errVMState
}

func (a *awsInstance) Stop() error {
	instanceID, err := a.client.GetAwsInstanceIDFromHostname(a.nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	vmState, err := a.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if vmState == "stopped" {
		e2e.Logf("%s already Stopped", a.nodeName)
	} else {
		err = a.client.StopInstance(instanceID)
		if err != nil {
			e2e.Logf("Stop instance failed with error :: %v.", err)
			return err
		}
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
	return instanceState, nil
}
