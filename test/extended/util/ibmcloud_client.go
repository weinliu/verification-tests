package util

import (
	"encoding/base64"
	"fmt"
	"os/exec"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/vpc-go-sdk/vpcv1"
	g "github.com/onsi/ginkgo/v2"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// IBMSession is an object representing an IBM session
type IBMSession struct {
	vpcv1 *vpcv1.VpcV1
}

// NewIBMSessionFromEnv creates a new IBM session from environment credentials
func NewIBMSessionFromEnv(ibmApiKey string) (*IBMSession, error) {
	// Create an IAM authenticator
	authenticator := &core.IamAuthenticator{
		ApiKey: ibmApiKey,
	}

	// Create a VPC service client
	vpcService, err := vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: authenticator,
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating VPC service client: %v", err)
	}

	session := &IBMSession{
		vpcv1: vpcService,
	}

	return session, nil
}

// GetIBMCredentialFromCluster gets IBM credentials like ibmapikey, ibmvpc and ibmregion from cluster
func GetIBMCredentialFromCluster(oc *CLI) (string, string, string, error) {
	var ibmClientID []byte
	credential, getSecErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/qe-ibmcloud-creds", "-n", "kube-system", "-o=jsonpath={.data.apiKey}").Output()
	if getSecErr != nil || credential == "" {
		// get creds in prow ci
		cmd := fmt.Sprintf("cat ${CLUSTER_PROFILE_DIR}/ibmcloud-api-key")
		credentialApiKey, getSecErr := exec.Command("bash", "-c", cmd).Output()
		credential = string(credentialApiKey)
		if getSecErr != nil || credential == "" {
			g.Skip("Failed to get credential to access IBM, skip the testing.")
		}
	} else {
		var err error
		ibmClientID, err = base64.StdEncoding.DecodeString(credential)
		if err != nil || string(ibmClientID) == "" {
			return "", "", "", fmt.Errorf("Error decoding IBM credentials: %s", err)
		}
	}

	ibmRegion, regionErr := GetIBMRegion(oc)
	if regionErr != nil {
		return "", "", "", regionErr
	}

	ibmResourceGrpName, ibmResourceGrpNameErr := GetIBMResourceGrpName(oc)
	if ibmResourceGrpNameErr != nil {
		return "", "", "", ibmResourceGrpNameErr
	}

	return string(ibmClientID), string(ibmRegion), string(ibmResourceGrpName) + "-vpc", nil
}

// StopIBMInstance stop the IBM instance
func StopIBMInstance(session *IBMSession, instanceID string) error {
	stopInstanceOptions := session.vpcv1.NewCreateInstanceActionOptions(instanceID, "stop")
	_, _, err := session.vpcv1.CreateInstanceAction(stopInstanceOptions)
	if err != nil {
		return fmt.Errorf("Unable to stop IBM instance: %v", err)
	}
	return nil
}

// StartIBMInstance start the IBM instance
func StartIBMInstance(session *IBMSession, instanceID string) error {
	startInstanceOptions := session.vpcv1.NewCreateInstanceActionOptions(instanceID, "start")
	_, _, err := session.vpcv1.CreateInstanceAction(startInstanceOptions)
	if err != nil {
		return fmt.Errorf("Unable to start IBM instance: %v", err)
	}
	return nil
}

// GetIBMInstanceID get IBM instance id
func GetIBMInstanceID(session *IBMSession, region string, vpcName string, instanceID string) (string, error) {
	err := SetVPCServiceURLForRegion(session, region)
	if err != nil {
		return "", fmt.Errorf("Failed to set vpc api service url :: %v", err)
	}

	// Retrieve the VPC ID based on the VPC name
	listVpcsOptions := session.vpcv1.NewListVpcsOptions()
	vpcs, _, err := session.vpcv1.ListVpcs(listVpcsOptions)
	if err != nil {
		return "", fmt.Errorf("Error listing VPCs: %v", err)
	}

	var vpcID string
	for _, vpc := range vpcs.Vpcs {
		if *vpc.Name == vpcName {
			vpcID = *vpc.ID
			e2e.Logf("VpcID found of VpcName %s :: %s", vpcName, vpcID)
			break
		}
	}

	if vpcID == "" {
		return "", fmt.Errorf("VPC not found: %s", vpcName)
	}

	// Set the VPC ID in the listInstancesOptions
	listInstancesOptions := session.vpcv1.NewListInstancesOptions()
	listInstancesOptions.SetVPCID(vpcID)

	// Retrieve the list of instances in the specified VPC
	instances, _, err := session.vpcv1.ListInstances(listInstancesOptions)
	if err != nil {
		return "", fmt.Errorf("Error listing instances: %v", err)
	}

	// Search for the instance by name
	for _, instance := range instances.Instances {
		if *instance.Name == instanceID {
			return *instance.ID, nil
		}
	}

	return "", fmt.Errorf("Instance not found for name: %s", instanceID)
}

// GetIBMInstanceStatus check IBM instance running status
func GetIBMInstanceStatus(session *IBMSession, instanceID string) (string, error) {
	getInstanceOptions := session.vpcv1.NewGetInstanceOptions(instanceID)
	instance, _, err := session.vpcv1.GetInstance(getInstanceOptions)
	if err != nil {
		return "", err
	}
	return *instance.Status, nil
}

// SetVPCServiceURLForRegion will set the VPC Service URL to a specific IBM Cloud Region, in order to access Region scoped resources
func SetVPCServiceURLForRegion(session *IBMSession, region string) error {
	regionOptions := session.vpcv1.NewGetRegionOptions(region)
	vpcRegion, _, err := session.vpcv1.GetRegion(regionOptions)
	if err != nil {
		return err
	}
	err = session.vpcv1.SetServiceURL(fmt.Sprintf("%s/v1", *vpcRegion.Endpoint))
	if err != nil {
		return err
	}
	return nil
}

// GetIBMRegion gets IBM cluster region
func GetIBMRegion(oc *CLI) (string, error) {
	ibmRegion, regionErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("Infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.ibmcloud.location}").Output()
	if regionErr != nil {
		return "", regionErr
	}
	return ibmRegion, nil
}

// GetIBMResourceGrpName get IBM cluster resource group name
func GetIBMResourceGrpName(oc *CLI) (string, error) {
	ibmResourceGrpName, ibmResourceGrpNameErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("Infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.ibmcloud.resourceGroupName}").Output()
	if ibmResourceGrpNameErr != nil {
		return "", ibmResourceGrpNameErr
	}
	return ibmResourceGrpName, nil
}
