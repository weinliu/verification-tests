package util

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2019-06-01/network"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
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

// getStorageClient get storage client
func getStorageClient(sess *AzureSession) storage.AccountsClient {
	storageClient := storage.NewAccountsClient(sess.SubscriptionID)
	storageClient.Authorizer = sess.Authorizer
	return storageClient
}

// GetAzureStorageAccount get Azure Storage Account
func GetAzureStorageAccount(sess *AzureSession, resourceGroupName string) (string, error) {
	storageClient := getStorageClient(sess)
	listGroupAccounts, err := storageClient.ListByResourceGroup(context.Background(), resourceGroupName)
	if err != nil {
		return "", err
	}
	for _, acc := range *listGroupAccounts.Value {
		fmt.Printf("\t%s\n", *acc.Name)
		match, err := regexp.MatchString("cluster", *acc.Name)
		if err != nil {
			return "", err
		}

		if match {
			e2e.Logf("The storage account name is %s,", *acc.Name)
			return *acc.Name, nil
		}
	}
	e2e.Logf("There is no storage account name matching regex : cluster")
	return "", nil
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

// GetAzureVMPublicIPByNameRegex  returns the first public IP whose name matches the given regex
func GetAzureVMPublicIPByNameRegex(sess *AzureSession, rg, publicIPNameRegex string) (string, error) {
	//find public IP
	e2e.Logf("Looking for publicIP with name matching %s", publicIPNameRegex)
	ipClient := getIPClient(sess)

	for iter, err := ipClient.ListAll(context.Background()); iter.NotDone(); err = iter.Next() {
		if err != nil {
			return "", err
		}

		for _, value := range iter.Values() {
			match, err := regexp.MatchString(publicIPNameRegex, *value.Name)
			if err != nil {
				return "", err
			}

			if match {
				e2e.Logf("The public IP with name %s is %s,", *value.Name, *value.IPAddress)
				return *value.IPAddress, nil
			}
		}
	}

	e2e.Logf("There is no public IP with its name matching regex : %s", publicIPNameRegex)
	return "", nil

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
			return vmName, nil
		}
	}
	return "", nil
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

// GetAzureCredentialFromCluster gets Azure credentials from cluster and loads them as environment variables
func GetAzureCredentialFromCluster(oc *CLI) (string, error) {
	credential, getSecErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "-o=jsonpath={.data}").Output()
	if getSecErr != nil {
		return "", nil
	}

	type azureCredentials struct {
		AzureClientID       string `json:"azure_client_id,omitempty"`
		AzureClientSecret   string `json:"azure_client_secret,omitempty"`
		AzureSubscriptionID string `json:"azure_subscription_id,omitempty"`
		AzureTenantID       string `json:"azure_tenant_id,omitempty"`
		AzureResourceGroup  string `json:"azure_resourcegroup,omitempty"`
	}
	azureCreds := azureCredentials{}
	if err := json.Unmarshal([]byte(credential), &azureCreds); err != nil {
		return "", err
	}

	azureClientID, err := base64.StdEncoding.DecodeString(azureCreds.AzureClientID)
	if err != nil {
		return "", err
	}

	azureClientSecret, err := base64.StdEncoding.DecodeString(azureCreds.AzureClientSecret)
	if err != nil {
		return "", err
	}

	azureSubscriptionID, err := base64.StdEncoding.DecodeString(azureCreds.AzureSubscriptionID)
	if err != nil {
		return "", err
	}

	azureTenantID, err := base64.StdEncoding.DecodeString(azureCreds.AzureTenantID)
	if err != nil {
		return "", err
	}

	azureResourceGroup, err := base64.StdEncoding.DecodeString(azureCreds.AzureResourceGroup)
	if err != nil {
		return "", err
	}
	os.Setenv("AZURE_CLIENT_ID", string(azureClientID))
	os.Setenv("AZURE_CLIENT_SECRET", string(azureClientSecret))
	os.Setenv("AZURE_SUBSCRIPTION_ID", string(azureSubscriptionID))
	os.Setenv("AZURE_TENANT_ID", string(azureTenantID))
	e2e.Logf("Azure credentials successfully loaded.")
	return string(azureResourceGroup), nil
}
