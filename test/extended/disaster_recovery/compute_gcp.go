package disasterrecovery

import (
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type gcpInstance struct {
	instance
	projectID string
	client    *exutil.Gcloud
}

// GetGcpNodes get nodes and load clouds cred with the specified label.
func GetGcpNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	projectID, err := exutil.GetGcpProjectID(oc)
	o.Expect(err).ToNot(o.HaveOccurred())
	client := client(projectID)
	var results []ComputeNode
	for _, node := range nodeNames {
		results = append(results, newGcpInstance(oc, client, projectID, strings.Split(node, ".")[0]))
	}
	return results, nil
}

func (g *gcpInstance) GetInstanceID() (string, error) {
	instanceID, err := g.client.GetGcpInstanceByNode(g.nodeName)
	if err == nil {
		e2e.Logf("VM instance name: %s", instanceID)
		return instanceID, nil
	}
	return "", err
}

func newGcpInstance(oc *exutil.CLI, client *exutil.Gcloud, projectID, nodeName string) *gcpInstance {
	return &gcpInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		client:    client,
		projectID: projectID,
	}
}

// GetGcloudClient to login on gcloud platform
func client(projectID string) *exutil.Gcloud {
	if projectID != "openshift-qe" {
		g.Skip("openshift-qe project is needed to execute this test case!")
	}
	gcloud := exutil.Gcloud{ProjectID: projectID}
	return gcloud.Login()
}

func (g *gcpInstance) Start() error {
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		instanceState, err := g.State()
		o.Expect(err).NotTo(o.HaveOccurred())
		if instanceState == "terminated" {
			nodeInstance := strings.Split(g.nodeName, ".")
			zoneName, err := g.client.GetZone(g.nodeName, nodeInstance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			err = g.client.StartInstance(nodeInstance[0], zoneName)
			if err != nil {
				e2e.Logf("Start instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if instanceState == "running" {
			e2e.Logf("%v already running", g.nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to start %s", g.nodeName))
	return errVmstate
}

func (g *gcpInstance) Stop() error {
	nodeInstance := strings.Split(g.nodeName, ".")
	zoneName, err := g.client.GetZone(g.nodeName, nodeInstance[0])
	o.Expect(err).NotTo(o.HaveOccurred())
	instanceState, err := g.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if instanceState == "terminated" {
		e2e.Logf("%v already stopped", g.nodeName)
	} else {
		err = g.client.StopInstanceAsync(nodeInstance[0], zoneName)
		if err != nil {
			e2e.Logf("Stop instance failed with error :: %v.", err)
			return err
		}
	}
	return nil
}

func (g *gcpInstance) State() (string, error) {
	instanceState, err := g.client.GetGcpInstanceStateByNode(g.nodeName)
	if err == nil {
		e2e.Logf("VM %s is : %s", g.nodeName, strings.ToLower(instanceState))
		return strings.ToLower(instanceState), nil
	}
	return "", err
}
