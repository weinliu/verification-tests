package util

import (
	"context"
	"os"
	"strings"

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
