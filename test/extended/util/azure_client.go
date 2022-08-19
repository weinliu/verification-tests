package util

import (
	"context"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2019-06-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// AzureSession is an object representing session for subscription
type AzureSession struct {
	SubscriptionID string
	Authorizer     autorest.Authorizer
}

// NewAzureSessionFromEnv new azure session from env credentials
func NewAzureSessionFromEnv() (*AzureSession, error) {
	authorizer, azureSessErr := auth.NewAuthorizerFromEnvironment()
	if azureSessErr != nil {
		e2e.Logf("New Azure Session from ENV error: %v", azureSessErr)
		return nil, azureSessErr
	}
	sess := AzureSession{
		SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
		Authorizer:     authorizer,
	}
	return &sess, nil
}

// getNicClient get nic client
func getNicClient(sess *AzureSession) network.InterfacesClient {
	nicClient := network.NewInterfacesClient(sess.SubscriptionID)
	nicClient.Authorizer = sess.Authorizer
	return nicClient
}

// getIPClient get publicIP client
func getIPClient(sess *AzureSession) network.PublicIPAddressesClient {
	ipClient := network.NewPublicIPAddressesClient(sess.SubscriptionID)
	ipClient.Authorizer = sess.Authorizer
	return ipClient
}

// GetAzureVMPrivateIP  get Azure vm private IP
func GetAzureVMPrivateIP(sess *AzureSession, rg, vmName string) (string, error) {
	nicClient := getNicClient(sess)
	privateIP := ""

	//find private IP
	for iter, err := nicClient.ListComplete(context.Background(), rg); iter.NotDone(); err = iter.Next() {
		if err != nil {
			return "", err
		}
		if strings.Contains(*iter.Value().Name, vmName) {
			e2e.Logf("Found int-svc VM with name %s", *iter.Value().Name)
			intF := *iter.Value().InterfacePropertiesFormat.IPConfigurations
			privateIP = *intF[0].InterfaceIPConfigurationPropertiesFormat.PrivateIPAddress
			e2e.Logf("The private IP for vm %s is %s,", vmName, privateIP)
			break
		}
	}

	return privateIP, nil

}

// GetAzureVMPublicIP  get azure vm public IP
func GetAzureVMPublicIP(sess *AzureSession, rg, vmName string) (string, error) {
	publicIPName := vmName + "PublicIP"
	publicIP := ""
	//find public IP
	ipClient := getIPClient(sess)
	publicIPAtt, getIPErr := ipClient.Get(context.Background(), rg, publicIPName, "")
	if getIPErr != nil {
		return "", getIPErr
	}
	publicIP = *publicIPAtt.IPAddress
	e2e.Logf("The public IP for vm %s is %s,", vmName, publicIP)
	return publicIP, nil

}

// StartAzureVM starts the selected VM
func StartAzureVM(sess *AzureSession, vmName string, resourceGroupName string) (osr autorest.Response, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vmClient := compute.NewVirtualMachinesClient(sess.SubscriptionID)
	vmClient.Authorizer = sess.Authorizer
	future, vmErr := vmClient.Start(ctx, resourceGroupName, vmName)
	if err != nil {
		e2e.Logf("cannot start vm: %v", vmErr)
		return osr, vmErr
	}

	err = future.WaitForCompletionRef(ctx, vmClient.Client)
	if err != nil {
		e2e.Logf("cannot get the vm start future response: %v", err)
		return osr, err
	}
	return future.Result(vmClient)
}

// StopAzureVM stops the selected VM
func StopAzureVM(sess *AzureSession, vmName string, resourceGroupName string) (osr autorest.Response, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vmClient := compute.NewVirtualMachinesClient(sess.SubscriptionID)
	vmClient.Authorizer = sess.Authorizer
	var skipShutdown bool = true
	// skipShutdown parameter is optional, we are taking its true value here
	future, vmErr := vmClient.PowerOff(ctx, resourceGroupName, vmName, &skipShutdown)
	if err != nil {
		e2e.Logf("cannot power off vm: %v", vmErr)
		return osr, vmErr
	}

	err = future.WaitForCompletionRef(ctx, vmClient.Client)
	if err != nil {
		e2e.Logf("cannot get the vm power off future response: %v", err)
		return osr, err
	}
	return future.Result(vmClient)
}

// GetAzureVMInstance get vm instance
func GetAzureVMInstance(sess *AzureSession, vmName string, resourceGroupName string) (string, error) {
	vmClient := compute.NewVirtualMachinesClient(sess.SubscriptionID)
	vmClient.Authorizer = sess.Authorizer
	for vm, vmErr := vmClient.ListComplete(context.Background(), resourceGroupName); vm.NotDone(); vmErr = vm.Next() {
		if vmErr != nil {
			e2e.Logf("got error while traverising RG list: %v", vmErr)
			return "", vmErr
		}
		instanceName := vm.Value()
		if *instanceName.Name == vmName {
			e2e.Logf("Azure instance found :: %s", vmName)
			break
		}
	}
	return vmName, nil
}

// GetAzureVMInstanceState get vm instance state
func GetAzureVMInstanceState(sess *AzureSession, vmName string, resourceGroupName string) (string, error) {
	var vmErr error
	vmClient := compute.NewVirtualMachinesClient(sess.SubscriptionID)
	vmClient.Authorizer = sess.Authorizer
	vmStatus, vmErr := vmClient.Get(context.Background(), resourceGroupName, vmName, compute.InstanceView)
	if vmErr != nil {
		e2e.Logf("Failed to get vm status :: %v", vmErr)
		return "", vmErr
	}
	status1 := *vmStatus.VirtualMachineProperties.InstanceView.Statuses
	status2 := *status1[1].DisplayStatus
	newStatus := strings.Split(status2, " ")
	e2e.Logf("Azure instance status found :: %v", newStatus[1])
	return string(newStatus[1]), nil
}
