package disasterrecovery

import (
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
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
	instanceState, err := g.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := stopStates[instanceState]; ok {
		nodeInstance := strings.Split(g.nodeName, ".")
		zoneName, err := g.client.GetZone(g.nodeName, nodeInstance[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		err = g.client.StartInstance(nodeInstance[0], zoneName)
		if err != nil {
			return fmt.Errorf("start instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to start instance %s from status %s", g.nodeName, instanceState)
	}
	return nil
}

func (g *gcpInstance) Stop() error {
	instanceState, err := g.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := startStates[instanceState]; ok {
		nodeInstance := strings.Split(g.nodeName, ".")
		zoneName, err := g.client.GetZone(g.nodeName, nodeInstance[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		err = g.client.StopInstanceAsync(nodeInstance[0], zoneName)
		if err != nil {
			return fmt.Errorf("stop instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to stop instance %s from status %s", g.nodeName, instanceState)
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
