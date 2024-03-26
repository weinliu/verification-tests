package util

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"

	o "github.com/onsi/gomega"
)

// AzureClientSet encapsulates Azure account information and multiple clients.
type AzureClientSet struct {
	// Account information
	SubscriptionID  string
	tokenCredential azcore.TokenCredential

	// Clients
	resourceGroupsClient *armresources.ResourceGroupsClient
}

func NewAzureClientSet(subscriptionId string, tokenCredential azcore.TokenCredential) *AzureClientSet {
	return &AzureClientSet{
		SubscriptionID:  subscriptionId,
		tokenCredential: tokenCredential,
	}
}

// NewAzureClientSetWithRootCreds constructs an AzureClientSet with info gleaned from the in-cluster root credential.
func NewAzureClientSetWithRootCreds(oc *CLI) *AzureClientSet {
	azCreds := NewEmptyAzureCredentials()
	o.Expect(azCreds.GetFromClusterAndDecode(oc)).NotTo(o.HaveOccurred())
	o.Expect(azCreds.SetSdkEnvVars()).NotTo(o.HaveOccurred())
	azureCredentials, err := azidentity.NewDefaultAzureCredential(nil)
	o.Expect(err).NotTo(o.HaveOccurred())
	return NewAzureClientSet(azCreds.AzureSubscriptionID, azureCredentials)
}

// GetResourceGroupClient gets the resource group client from the AzureClientSet, constructs it if necessary.
// Concurrent invocation of this method is safe only when AzureClientSet.resourceGroupsClient is non-nil,
// which is the case when the resourceGroupsClient is eagerly initialized.
func (cs *AzureClientSet) GetResourceGroupClient(options *arm.ClientOptions) *armresources.ResourceGroupsClient {
	if cs.resourceGroupsClient == nil {
		rgClient, err := armresources.NewResourceGroupsClient(cs.SubscriptionID, cs.tokenCredential, options)
		o.Expect(err).NotTo(o.HaveOccurred())
		cs.resourceGroupsClient = rgClient
	}
	return cs.resourceGroupsClient
}
