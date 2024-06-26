package util

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	armcompute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	o "github.com/onsi/gomega"
)

// AzureClientSet encapsulates Azure account information and multiple clients.
type AzureClientSet struct {
	// Account information
	SubscriptionID  string
	tokenCredential azcore.TokenCredential

	// Clients
	resourceGroupsClient           *armresources.ResourceGroupsClient
	capacityReservationGroupClient *armcompute.CapacityReservationGroupsClient
	capacityReservationsClient     *armcompute.CapacityReservationsClient
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

// GetCapacityReservationGroupClient get capacity reservation group Client
func (cs *AzureClientSet) GetCapacityReservationGroupClient(options *arm.ClientOptions) *armcompute.CapacityReservationGroupsClient {
	if cs.capacityReservationGroupClient == nil {
		capacityReservationGroupClient, err := armcompute.NewCapacityReservationGroupsClient(cs.SubscriptionID, cs.tokenCredential, options)
		o.Expect(err).NotTo(o.HaveOccurred())
		cs.capacityReservationGroupClient = capacityReservationGroupClient
	}
	return cs.capacityReservationGroupClient
}

// GetCapacityReservationsClient get capacity reservations client
func (cs *AzureClientSet) GetCapacityReservationsClient(options *arm.ClientOptions) *armcompute.CapacityReservationsClient {
	if cs.capacityReservationsClient == nil {
		capacityReservationsClient, err := armcompute.NewCapacityReservationsClient(cs.SubscriptionID, cs.tokenCredential, options)
		o.Expect(err).NotTo(o.HaveOccurred())
		cs.capacityReservationsClient = capacityReservationsClient
	}
	return cs.capacityReservationsClient
}

// CreateCapacityReservationGroup create a capacity reservation group
func (cs *AzureClientSet) CreateCapacityReservationGroup(ctx context.Context, capacityReservationGroupName string, resourceGroupName string, location string, zone string) (string, error) {
	capacityReservationGroupClient := cs.GetCapacityReservationGroupClient(nil)
	capacityReservationGroup := armcompute.CapacityReservationGroup{
		Location: &location,
		Zones:    []*string{&zone},
	}

	resp, err := capacityReservationGroupClient.CreateOrUpdate(
		ctx,
		resourceGroupName,
		capacityReservationGroupName,
		capacityReservationGroup,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("Failed to create Capacity Reservation Group: %v", err)
	}
	e2e.Logf("Capacity Reservation Group created successfully, capacity reservation group ID: %s", *resp.ID)
	return *resp.ID, err
}

// CreateCapacityReservation create a capacity reservation
func (cs *AzureClientSet) CreateCapacityReservation(ctx context.Context, capacityReservationGroupName string, capacityReservationName string, location string, resourceGroupName string, skuName string, zone string) error {
	capacityReservationsClient := cs.GetCapacityReservationsClient(nil)
	instanceCapacity := int64(1)
	capacityReservation := armcompute.CapacityReservation{
		Location: &location,
		SKU: &armcompute.SKU{
			Capacity: &instanceCapacity,
			Name:     &skuName,
		},
		Zones: []*string{&zone},
	}
	resp, err := capacityReservationsClient.BeginCreateOrUpdate(
		ctx,
		resourceGroupName,
		capacityReservationGroupName,
		capacityReservationName,
		capacityReservation,
		nil,
	)
	if err != nil {
		return fmt.Errorf("Failed to create Capacity Reservation: %v", err)
	}
	finalResp, err := resp.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("Failed to wait for the Capacity Reservation creation to complete: %v", err)
	}
	e2e.Logf("Capacity Reservation created successfully %s", *finalResp.ID)
	return nil
}

// DeleteCapacityReservationGroup delete capacity reservation group
func (cs *AzureClientSet) DeleteCapacityReservationGroup(ctx context.Context, capacityReservationGroupName string, resourceGroupName string) error {
	capacityReservationGroupClient := cs.GetCapacityReservationGroupClient(nil)
	_, err := capacityReservationGroupClient.Delete(
		ctx,
		resourceGroupName,
		capacityReservationGroupName,
		nil,
	)
	if err != nil {
		return fmt.Errorf("Failed to delete Capacity Reservation Group: %v", err)
	}
	e2e.Logf("Capacity Reservation Group deleted successfully")
	return nil
}

// DeleteCapacityReservation delete capacity reservation
func (cs *AzureClientSet) DeleteCapacityReservation(ctx context.Context, capacityReservationGroupName string, capacityReservationName string, resourceGroupName string) error {
	capacityReservationsClient := cs.GetCapacityReservationsClient(nil)
	resp, err := capacityReservationsClient.BeginDelete(
		ctx,
		resourceGroupName,
		capacityReservationGroupName,
		capacityReservationName,
		nil,
	)
	if err != nil {
		return fmt.Errorf("Failed to delete Capacity Reservation: %v", err)
	}
	_, err = resp.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("Failed to wait for the capacity reservation deletation to complete: %v", err)
	}
	e2e.Logf("Capacity Reservation deleted successfully")
	return nil
}
