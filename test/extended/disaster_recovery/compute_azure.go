package disasterrecovery

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type azureInstance struct {
	instance
	azureRGname    string
	client         *exutil.AzureSession
	azureCloudType string
}

// GetAzureNodes get nodes and load clouds cred with the specified label.
func GetAzureNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	azureRGname, rgerr := exutil.GetAzureCredentialFromCluster(oc)
	o.Expect(rgerr).NotTo(o.HaveOccurred())
	azureSession, sessErr := exutil.NewAzureSessionFromEnv()
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	isAzureStack, cloudName := isAzureStackCluster(oc)
	if isAzureStack {
		var filePath string
		filePath = os.Getenv("SHARED_DIR") + "/azurestack-login-script.sh"
		if _, err := os.Stat(filePath); err == nil {
			e2e.Logf("File %s exists.\n", filePath)
		} else if os.IsNotExist(err) {
			g.Skip(fmt.Sprintf("File %s does not exist.\n", filePath))
		} else {
			g.Skip(fmt.Sprintf("Error checking file: %v\n", err))
		}
		cmd := fmt.Sprintf(`source %s`, filePath)
		_, cmdErr := exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
	}
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newAzureInstance(oc, azureSession, azureRGname, nodeName, strings.ToLower(cloudName)))
	}
	return results, nil
}

func newAzureInstance(oc *exutil.CLI, client *exutil.AzureSession, azureRGname, nodeName string, azureCloudType string) *azureInstance {
	return &azureInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		client:         client,
		azureRGname:    azureRGname,
		azureCloudType: azureCloudType,
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
	if az.azureCloudType == "azurestackcloud" {
		err := exutil.StartAzureStackVM(az.azureRGname, az.nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		return err
	}
	_, err := exutil.StartAzureVM(az.client, az.nodeName, az.azureRGname)
	o.Expect(err).NotTo(o.HaveOccurred())
	return err
}

func (az *azureInstance) Stop() error {
	if az.azureCloudType == "azurestackcloud" {
		err := exutil.StopAzureStackVM(az.azureRGname, az.nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		return err
	}
	_, err := exutil.StopAzureVM(az.client, az.nodeName, az.azureRGname)
	o.Expect(err).NotTo(o.HaveOccurred())
	return err
}

func (az *azureInstance) State() (string, error) {
	if az.azureCloudType == "azurestackcloud" {
		instanceState, err := exutil.GetAzureStackVMStatus(az.azureRGname, az.nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		return instanceState, err
	}
	instanceState, err := exutil.GetAzureVMInstanceState(az.client, az.nodeName, az.azureRGname)
	o.Expect(err).NotTo(o.HaveOccurred())
	return instanceState, err
}
