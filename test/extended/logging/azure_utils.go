package logging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	azRuntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	azTo "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
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
				Audiences: []*string{azTo.Ptr("api://AzureADTokenExchange")},
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
			PrincipalID:      azTo.Ptr(identityPrincipalID),
			PrincipalType:    azTo.Ptr(armauthorization.PrincipalTypeServicePrincipal),
			RoleDefinitionID: azTo.Ptr(roleDefinitionID),
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
		Location: azTo.Ptr(region),
		SKU: &armstorage.SKU{
			Name: azTo.Ptr(armstorage.SKUNameStandardLRS),
		},
		Kind: azTo.Ptr(armstorage.KindStorageV2),
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
		e2e.Logf("Error while deleting storage account. " + err.Error())
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
		e2e.Logf("Error deleting identity. " + err.Error())
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
	resourceGroup, err := getResourceGroupOnAzure(oc)
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

// Check logs under the container used/referenced under the loki object storage secret.
func validateAzureContainerContents(oc *exutil.CLI, storageAccountName, containerName string, tenants []string) {
	cred := createNewDefaultAzureCredential()
	_, storageAccountURISuffix := getStorageAccountURISuffixAndEnvForAzure(oc)
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
						e2e.Logf("Logs " + *blob.Name + " found under the container: " + containerName)
						return true, nil
					}
				}
			}
		}
		e2e.Logf("Waiting for data to be available under container: " + containerName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Timed out...No data is available under the container: "+containerName)
}
