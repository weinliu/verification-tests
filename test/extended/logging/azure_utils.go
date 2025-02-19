package logging

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azarm "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	azpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	azRuntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	azto "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azlogs"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/operationalinsights/armoperationalinsights"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/google/uuid"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Creates a new default Azure credential
func createNewDefaultAzureCredential() *azidentity.DefaultAzureCredential {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to obtain a credential")
	return cred
}

// Function to create a managed identity on Azure
func createManagedIdentityOnAzure(defaultAzureCred *azidentity.DefaultAzureCredential, azureSubscriptionID, lokiStackName, resourceGroup, region string) (string, string) {
	// Create the MSI client
	client, err := armmsi.NewUserAssignedIdentitiesClient(azureSubscriptionID, defaultAzureCred, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create MSI client")

	// Configure the managed identity
	identity := armmsi.Identity{
		Location: &region,
	}

	// Create the identity
	result, err := client.CreateOrUpdate(context.Background(), resourceGroup, lokiStackName, identity, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create or update the identity")
	return *result.Properties.ClientID, *result.Properties.PrincipalID
}

// Function to create Federated Credentials on Azure
func createFederatedCredentialforLoki(defaultAzureCred *azidentity.DefaultAzureCredential, azureSubscriptionID, managedIdentityName, lokiServiceAccount, lokiStackNS, federatedCredentialName, serviceAccountIssuer, resourceGroup string) {
	subjectName := "system:serviceaccount:" + lokiStackNS + ":" + lokiServiceAccount

	// Create the Federated Identity Credentials client
	client, err := armmsi.NewFederatedIdentityCredentialsClient(azureSubscriptionID, defaultAzureCred, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create federated identity credentials client")

	// Create or update the federated identity credential
	result, err := client.CreateOrUpdate(
		context.Background(),
		resourceGroup,
		managedIdentityName,
		federatedCredentialName,
		armmsi.FederatedIdentityCredential{
			Properties: &armmsi.FederatedIdentityCredentialProperties{
				Issuer:    &serviceAccountIssuer,
				Subject:   &subjectName,
				Audiences: []*string{azto.Ptr("api://AzureADTokenExchange")},
			},
		},
		nil,
	)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create or update the federated credential: "+federatedCredentialName)
	e2e.Logf("Federated credential created/updated successfully: %s\n", *result.Name)
}

// Assigns role to a Azure Managed Identity on subscription level scope
func createRoleAssignmentForManagedIdentity(defaultAzureCred *azidentity.DefaultAzureCredential, azureSubscriptionID, identityPrincipalID string) {
	clientFactory, err := armauthorization.NewClientFactory(azureSubscriptionID, defaultAzureCred, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create instance of ClientFactory")

	scope := "/subscriptions/" + azureSubscriptionID
	// Below is standard role definition ID for Storage Blob Data Contributor built-in role
	roleDefinitionID := scope + "/providers/Microsoft.Authorization/roleDefinitions/ba92f5b4-2d11-453d-a403-e96b0029c9fe"

	// Create or update a role assignment by scope and name
	_, err = clientFactory.NewRoleAssignmentsClient().Create(context.Background(), scope, uuid.NewString(), armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      azto.Ptr(identityPrincipalID),
			PrincipalType:    azto.Ptr(armauthorization.PrincipalTypeServicePrincipal),
			RoleDefinitionID: azto.Ptr(roleDefinitionID),
		},
	}, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "Role Assignment operation failure....")
}

// Creates Azure storage account
func createStorageAccountOnAzure(defaultAzureCred *azidentity.DefaultAzureCredential, azureSubscriptionID, resourceGroup, region string) string {
	storageAccountName := "aosqelogging" + getRandomString()
	// Create the storage account
	storageClient, err := armstorage.NewAccountsClient(azureSubscriptionID, defaultAzureCred, nil)
	o.Expect(err).NotTo(o.HaveOccurred())
	result, err := storageClient.BeginCreate(context.Background(), resourceGroup, storageAccountName, armstorage.AccountCreateParameters{
		Location: azto.Ptr(region),
		SKU: &armstorage.SKU{
			Name: azto.Ptr(armstorage.SKUNameStandardLRS),
		},
		Kind: azto.Ptr(armstorage.KindStorageV2),
	}, nil)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Poll until the Storage account is ready
	_, err = result.PollUntilDone(context.Background(), &azRuntime.PollUntilDoneOptions{
		Frequency: 10 * time.Second,
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "Storage account is not ready...")
	os.Setenv("LOKI_OBJECT_STORAGE_STORAGE_ACCOUNT", storageAccountName)
	return storageAccountName
}

// Returns the Azure environment and storage account URI suffixes
func getStorageAccountURISuffixAndEnvForAzure(oc *exutil.CLI) (string, string) {
	// To return account URI suffix and env
	cloudName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
	storageAccountURISuffix := ".blob.core.windows.net"
	environment := "AzureGlobal"
	// Currently we don't have template support for STS/WIF on Azure Government
	// The below code should be ok to run when support is added for WIF
	if strings.ToLower(cloudName) == "azureusgovernmentcloud" {
		storageAccountURISuffix = ".blob.core.usgovcloudapi.net"
		environment = "AzureUSGovernment"
	}
	if strings.ToLower(cloudName) == "azurechinacloud" {
		storageAccountURISuffix = ".blob.core.chinacloudapi.cn"
		environment = "AzureChinaCloud"
	}
	if strings.ToLower(cloudName) == "azuregermancloud" {
		environment = "AzureGermanCloud"
		storageAccountURISuffix = ".blob.core.cloudapi.de"
	}
	return environment, storageAccountURISuffix
}

// Creates a blob container under the provided storageAccount
func createBlobContaineronAzure(defaultAzureCred *azidentity.DefaultAzureCredential, storageAccountName, storageAccountURISuffix, containerName string) {
	blobServiceClient, err := azblob.NewClient(fmt.Sprintf("https://%s%s", storageAccountName, storageAccountURISuffix), defaultAzureCred, nil)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = blobServiceClient.CreateContainer(context.Background(), containerName, nil)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("%s container created successfully: ", containerName)
}

// Creates Loki object storage secret required on Azure STS/WIF clusters
func createLokiObjectStorageSecretForWIF(oc *exutil.CLI, lokiStackNS, objectStorageSecretName, environment, containerName, storageAccountName string) error {
	return oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "-n", lokiStackNS, objectStorageSecretName, "--from-literal=environment="+environment, "--from-literal=container="+containerName, "--from-literal=account_name="+storageAccountName).Execute()
}

// Deletes a storage account in Microsoft Azure
func deleteAzureStorageAccount(defaultAzureCred *azidentity.DefaultAzureCredential, azureSubscriptionID, resourceGroupName, storageAccountName string) {
	clientFactory, err := armstorage.NewClientFactory(azureSubscriptionID, defaultAzureCred, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to create instance of ClientFactory for storage account deletion")

	_, err = clientFactory.NewAccountsClient().Delete(context.Background(), resourceGroupName, storageAccountName, nil)
	if err != nil {
		e2e.Logf("Error while deleting storage account: %s", err.Error())
	} else {
		e2e.Logf("storage account deleted successfully..")
	}
}

// Deletes the Azure Managed identity
func deleteManagedIdentityOnAzure(defaultAzureCred *azidentity.DefaultAzureCredential, azureSubscriptionID, resourceGroupName, identityName string) {
	client, err := armmsi.NewUserAssignedIdentitiesClient(azureSubscriptionID, defaultAzureCred, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to create MSI client for identity deletion")

	_, err = client.Delete(context.Background(), resourceGroupName, identityName, nil)
	if err != nil {
		e2e.Logf("Error deleting identity: %s", err.Error())
	} else {
		e2e.Logf("managed identity deleted successfully...")
	}
}

// patches CLIENT_ID, SUBSCRIPTION_ID, TENANT_ID AND REGION into Loki subscription on Azure WIF clusters
func patchLokiConfigIntoLokiSubscription(oc *exutil.CLI, azureSubscriptionID, identityClientID, region string) {
	patchConfig := `{
		"spec": {
			"config": {
				"env": [
					{
						"name": "CLIENTID",
						"value": "%s"
					},
					{
						"name": "TENANTID",
						"value": "%s"
					},
					{
						"name": "SUBSCRIPTIONID",
						"value": "%s"
					},
					{
						"name": "REGION",
						"value": "%s"
					}
				]
			}
		}
	}`

	err := oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("patch").Args("sub", "loki-operator", "-n", loNS, "-p", fmt.Sprintf(patchConfig, identityClientID, os.Getenv("AZURE_TENANT_ID"), azureSubscriptionID, region), "--type=merge").Execute()
	o.Expect(err).NotTo(o.HaveOccurred(), "Patching Loki Operator failed...")
	waitForPodReadyWithLabel(oc, loNS, "name=loki-operator-controller-manager")
}

// Performs creation of Managed Identity, Associated Federated credentials, Role assignment to the managed identity and object storage creation on Azure
func performManagedIdentityAndSecretSetupForAzureWIF(oc *exutil.CLI, lokistackName, lokiStackNS, azureContainerName, lokiStackStorageSecretName string) {
	region, err := getAzureClusterRegion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	serviceAccountIssuer, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("authentication.config", "cluster", "-o=jsonpath={.spec.serviceAccountIssuer}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	resourceGroup, err := getAzureResourceGroupFromCluster(oc)
	o.Expect(err).NotTo(o.HaveOccurred())

	azureSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	cred := createNewDefaultAzureCredential()

	identityClientID, identityPrincipalID := createManagedIdentityOnAzure(cred, azureSubscriptionID, lokistackName, resourceGroup, region)
	createFederatedCredentialforLoki(cred, azureSubscriptionID, lokistackName, lokistackName, lokiStackNS, "openshift-logging-"+lokistackName, serviceAccountIssuer, resourceGroup)
	createFederatedCredentialforLoki(cred, azureSubscriptionID, lokistackName, lokistackName+"-ruler", lokiStackNS, "openshift-logging-"+lokistackName+"-ruler", serviceAccountIssuer, resourceGroup)
	createRoleAssignmentForManagedIdentity(cred, azureSubscriptionID, identityPrincipalID)
	patchLokiConfigIntoLokiSubscription(oc, azureSubscriptionID, identityClientID, region)
	storageAccountName := createStorageAccountOnAzure(cred, azureSubscriptionID, resourceGroup, region)
	environment, storageAccountURISuffix := getStorageAccountURISuffixAndEnvForAzure(oc)
	createBlobContaineronAzure(cred, storageAccountName, storageAccountURISuffix, azureContainerName)
	err = createLokiObjectStorageSecretForWIF(oc, lokiStackNS, lokiStackStorageSecretName, environment, azureContainerName, storageAccountName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Function to check if tenant logs are present under the Azure blob Container.
// Use getStorageAccountURISuffixAndEnvForAzure() to get the storage account URI suffix.
// Returns success if any one of the tenants under tenants[] are found.
func validatesIfLogsArePushedToAzureContainer(storageAccountURISuffix, storageAccountName, containerName string, tenants []string) {
	cred := createNewDefaultAzureCredential()
	// Create a new Blob service client
	serviceClient, err := azblob.NewClient("https://"+storageAccountName+storageAccountURISuffix, cred, nil)
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to create service client..")

	// Poll to check log streams are flushed to container referenced under loki object storage secret
	err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, true, func(context.Context) (done bool, err error) {
		// Create a client to interact with the container and List blobs in the container
		pager := serviceClient.NewListBlobsFlatPager(containerName, nil)
		for pager.More() {
			// advance to the next page
			page, err := pager.NextPage(context.TODO())
			o.Expect(err).NotTo(o.HaveOccurred())

			// check the blob names for this page
			for _, blob := range page.Segment.BlobItems {
				for _, tenantName := range tenants {
					if strings.Contains(*blob.Name, tenantName) {
						e2e.Logf("Logs %s found under the container: %s", *blob.Name, containerName)
						return true, nil
					}
				}
			}
		}
		e2e.Logf("Waiting for data to be available under container: %s", containerName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Timed out...No data is available under the container: "+containerName)
}

func getAzureResourceGroupFromCluster(oc *exutil.CLI) (string, error) {
	resourceGroup, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructures", "cluster", "-o=jsonpath={.status.platformStatus.azure.resourceGroupName}").Output()
	return resourceGroup, err
}

// Get region/location of cluster running on Azure Cloud
func getAzureClusterRegion(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("node", `-ojsonpath={.items[].metadata.labels.topology\.kubernetes\.io/region}`).Output()
}

// Define the function to create a resource group.
func createAzureResourceGroup(resourceGroupName, subscriptionId, location string, credential azcore.TokenCredential) (armresources.ResourceGroupsClientCreateOrUpdateResponse, error) {
	rgClient, _ := armresources.NewResourceGroupsClient(subscriptionId, credential, nil)

	param := armresources.ResourceGroup{
		Location: azto.Ptr(location),
	}

	return rgClient.CreateOrUpdate(context.Background(), resourceGroupName, param, nil)
}

// Delete a resource group.
func deleteAzureResourceGroup(resourceGroupName, subscriptionId string, credential azcore.TokenCredential) error {
	rgClient, _ := armresources.NewResourceGroupsClient(subscriptionId, credential, nil)

	poller, err := rgClient.BeginDelete(context.Background(), resourceGroupName, nil)
	if err != nil {
		return err
	}
	if _, err := poller.PollUntilDone(context.Background(), nil); err != nil {
		return err
	}
	e2e.Logf("Successfully deleted resource group: %s", resourceGroupName)
	return nil
}

type azureCredentials struct {
	SubscriptionID string `json:"subscriptionId"`
	ClientID       string `json:"clientId"`
	ClientSecret   string `json:"clientSecret"`
	TenantID       string `json:"tenantId"`
}

// To read Azure subscription json file from local disk.
// Also injects ENV vars needed to perform certain operations on Managed Identities.
func readAzureCredentials() bool {
	var (
		azureCredFile string
		azureCred     azureCredentials
	)
	authFile, present := os.LookupEnv("AZURE_AUTH_LOCATION")
	if present {
		azureCredFile = authFile
	} else {
		envDir, present := os.LookupEnv("CLUSTER_PROFILE_DIR")
		if present {
			azureCredFile = filepath.Join(envDir, "osServicePrincipal.json")
		}
	}
	if len(azureCredFile) > 0 {
		fileContent, err := os.ReadFile(azureCredFile)
		if err != nil {
			e2e.Logf("can't read file %s: %v", azureCredFile, err)
			return false
		}
		json.Unmarshal(fileContent, &azureCred)
		os.Setenv("AZURE_SUBSCRIPTION_ID", azureCred.SubscriptionID)
		os.Setenv("AZURE_TENANT_ID", azureCred.TenantID)
		os.Setenv("AZURE_CLIENT_ID", azureCred.ClientID)
		os.Setenv("AZURE_CLIENT_SECRET", azureCred.ClientSecret)
		return true
	}
	return false
}

type azureMonitorLog struct {
	azCred            *azidentity.DefaultAzureCredential
	clientOpts        azpolicy.ClientOptions
	customerID        string
	host              string
	location          string
	primaryKey        string
	resourceGroupName string
	secondaryKey      string
	subscriptionID    string
	tPrefixOrName     string // Depend on how we defined the logType in CLF template, it can be the table name or the table name name prefix.
	workspaceID       string
	workspaceName     string
}

// checkout the cloudType of this cluster's platform
func getAzureCloudName(oc *exutil.CLI) string {
	cloudName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
	if err == nil && len(cloudName) > 0 {
		return strings.ToLower(cloudName)
	}
	return ""
}

func (azLog *azureMonitorLog) getSourceGroupLocation() error {
	resourceGroupClient, err := armresources.NewResourceGroupsClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}

	ctx := context.Background()
	resourceGroupGetResponse, err := resourceGroupClient.Get(
		ctx,
		azLog.resourceGroupName,
		nil,
	)
	if err != nil {
		return err
	}
	azLog.location = *resourceGroupGetResponse.ResourceGroup.Location
	return nil
}

func (azLog *azureMonitorLog) createLogWorkspace() error {
	e2e.Logf("Creating workspace")
	workspacesClient, err := armoperationalinsights.NewWorkspacesClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}
	ctx := context.Background()
	pollerResp, err := workspacesClient.BeginCreateOrUpdate(
		ctx,
		azLog.resourceGroupName,
		azLog.workspaceName,
		armoperationalinsights.Workspace{
			Location:   azto.Ptr(azLog.location),
			Properties: &armoperationalinsights.WorkspaceProperties{},
		},
		nil,
	)
	if err != nil {
		return err
	}
	workspace, err := pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}
	azLog.workspaceID = *workspace.ID
	azLog.workspaceName = *workspace.Name
	azLog.customerID = *workspace.Properties.CustomerID

	shareKeyClient, err := armoperationalinsights.NewSharedKeysClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}
	resp, err := shareKeyClient.GetSharedKeys(ctx, azLog.resourceGroupName, azLog.workspaceName, nil)
	if err != nil {
		return err
	}
	azLog.primaryKey = *resp.PrimarySharedKey
	azLog.secondaryKey = *resp.SecondarySharedKey
	return nil
}

// Get azureMonitoring from Envs. CreateOrUpdate Log Analytics workspace.
func newAzureLog(oc *exutil.CLI, location, resouceGroupName, workspaceName, tPrefixOrName string) (azureMonitorLog, error) {
	var (
		azLog azureMonitorLog
		err   error
	)

	azLog.tPrefixOrName = tPrefixOrName
	azLog.workspaceName = workspaceName
	azLog.resourceGroupName = resouceGroupName
	//  The workspace name must be between 4 and 63 characters.
	//  The workspace name can contain only letters, numbers and '-'. The '-' shouldn't be the first or the last symbol.

	azLog.subscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
	if len(azLog.subscriptionID) == 0 {
		dat, err := oc.AsAdmin().WithoutNamespace().Run("get", "-n", "kube-system", "secret/azure-credentials", "-ojsonpath={.data.azure_subscription_id}").Output()
		if err != nil {
			return azLog, fmt.Errorf("failed to get secret/azure-credentials")
		}
		data, err := base64.StdEncoding.DecodeString(dat)
		if err != nil {
			return azLog, fmt.Errorf("failed to decode subscription_id from secret/azure-credentials")
		}

		azLog.subscriptionID = string(data)
		if len(azLog.subscriptionID) == 0 {
			return azLog, fmt.Errorf("failed as subscriptionID is empty")
		}
	}
	platform := exutil.CheckPlatform(oc)
	if platform == "azure" {
		cloudName := getAzureCloudName(oc)
		switch cloudName {
		case "azurepubliccloud":
			azLog.clientOpts = azcore.ClientOptions{Cloud: azcloud.AzurePublic}
			azLog.host = "ods.opinsights.azure.com"
		case "azureusgovernmentcloud":
			azLog.clientOpts = azcore.ClientOptions{Cloud: azcloud.AzureGovernment}
			azLog.host = "ods.opinsights.azure.us"
		case "azurechinacloud":
			//azLog.clientOpts = azcore.ClientOptions{Cloud: azcloud.AzureChina}
			return azLog, fmt.Errorf("skip on AzureChinaCloud")
		case "azuregermancloud":
			return azLog, fmt.Errorf("skip on AzureGermanCloud")
		case "azurestackcloud":
			return azLog, fmt.Errorf("skip on AzureStackCloud")
		default:
			return azLog, fmt.Errorf("skip on %s", cloudName)
		}
	} else {
		//TODO: get az cloud type from env vars
		azLog.clientOpts = azcore.ClientOptions{Cloud: azcloud.AzurePublic}
		azLog.host = "ods.opinsights.azure.com"
	}
	azLog.azCred, err = azidentity.NewDefaultAzureCredential(
		&azidentity.DefaultAzureCredentialOptions{ClientOptions: azLog.clientOpts},
	)
	if err != nil {
		return azLog, err
	}

	if location != "" {
		azLog.location = location
	} else {
		err = azLog.getSourceGroupLocation()
		if err != nil {
			return azLog, err
		}
	}

	err = azLog.createLogWorkspace()
	if err != nil {
		return azLog, err
	}
	return azLog, nil
}

// Create a secret for collector pods to forward logs to Log Analytics workspaces.
func (azLog *azureMonitorLog) createSecret(oc *exutil.CLI, name, namespace string) error {
	return oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", name, "-n", namespace, "--from-literal=shared_key="+azLog.primaryKey).Execute()
}

// query logs per table in Log Analytics workspaces.
func (azLog *azureMonitorLog) getLogByTable(logTable string) ([]azlogs.Row, error) {
	queryString := logTable + "| where TimeGenerated > ago(5m)|top 10 by TimeGenerated"
	e2e.Logf("query %v", queryString)
	var entries []azlogs.Row

	client, err := azlogs.NewClient(azLog.azCred,
		&azlogs.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return entries, err
	}

	//https://learn.microsoft.com/en-us/cli/azure/monitor/log-analytics?view=azure-cli-latest
	//https://learn.microsoft.com/en-us/azure/data-explorer/kusto/query/
	err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		res, err1 := client.QueryWorkspace(
			context.TODO(),
			azLog.customerID,
			azlogs.QueryBody{
				Query: azto.Ptr(queryString),
			},
			nil)
		if err1 != nil {
			e2e.Logf("azlogs QueryWorkspace error: %v. continue", err1)
			return false, nil
		}
		if res.Error != nil {
			e2e.Logf("azlogs QueryWorkspace response error: %v, continue", res.Error)
			return false, nil
		}
		for _, table := range res.Tables {
			entries = append(entries, table.Rows...)
		}
		return len(entries) > 0, nil
	})

	return entries, err
}

// Delete LogWorkspace
func (azLog *azureMonitorLog) deleteWorkspace() error {
	e2e.Logf("Delete workspace %v", azLog.workspaceName)
	ctx := context.Background()
	workspacesClient, err := armoperationalinsights.NewWorkspacesClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}
	workspacesClient.BeginDelete(ctx, azLog.resourceGroupName, azLog.workspaceName, &armoperationalinsights.WorkspacesClientBeginDeleteOptions{Force: new(bool)})
	return nil
}
