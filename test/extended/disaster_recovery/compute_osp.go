package disasterrecovery

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gophercloud/gophercloud"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ospInstance struct {
	instance
	ospObj exutil.Osp
	client *gophercloud.ServiceClient
}

// Get nodes and load clouds cred with the specified label.
func GetOspNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	cred, err1 := exutil.GetOpenStackCredentials(oc)
	o.Expect(err1).NotTo(o.HaveOccurred())
	client := exutil.NewOpenStackClient(cred, "compute")

	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newOspInstance(oc, client, nodeName))
	}
	return results, nil
}

// OspCredentials get creds of osp platform
func OspCredentials(oc *exutil.CLI) {
	credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/openstack-credentials", "-n", "kube-system", "-o", `jsonpath={.data.clouds\.yaml}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	// Decode the base64 credentials
	credential, err := base64.StdEncoding.DecodeString(credentials)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Define variables for the credentials
	var (
		username                    string
		password                    string
		projectID                   string
		authURL                     string
		userDomainName              string
		regionName                  string
		projectName                 string
		authType                    string
		applicationCredentialId     string
		applicationCredentialSecret string
	)

	// Define mappings for credentials to environment variables
	credMap := map[string]*string{
		"auth_url":                      &authURL,
		"username":                      &username,
		"password":                      &password,
		"project_id":                    &projectID,
		"user_domain_name":              &userDomainName,
		"region_name":                   &regionName,
		"project_name":                  &projectName,
		"auth_type":                     &authType,
		"application_credential_id":     &applicationCredentialId,
		"application_credential_secret": &applicationCredentialSecret,
	}

	// Extract and set each credential variable using regex
	for yamlKey, credVar := range credMap {
		r := regexp.MustCompile(yamlKey + `:\s*([^\n]+)`)
		match := r.FindStringSubmatch(string(credential))
		if len(match) == 2 {
			*credVar = strings.TrimSpace(match[1])
		}

		// Set environment variable
		envVarName := fmt.Sprintf("OSP_DR_%s", strings.ToUpper(yamlKey))
		os.Setenv(envVarName, *credVar)
	}
}

func newOspInstance(oc *exutil.CLI, client *gophercloud.ServiceClient, nodeName string) *ospInstance {
	return &ospInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		ospObj: exutil.Osp{},
		client: client,
	}
}

func (osp *ospInstance) GetInstanceID() (string, error) {
	instanceID, err := osp.ospObj.GetOspInstance(osp.client, osp.nodeName)
	if err != nil {
		e2e.Logf("Get instance id failed with error :: %v.", err)
		return "", err
	}
	return instanceID, nil
}

func (osp *ospInstance) Start() error {
	instanceState, err := osp.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := stopStates[instanceState]; ok {
		err = osp.ospObj.GetStartOspInstance(osp.client, osp.nodeName)
		if err != nil {
			return fmt.Errorf("start instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to start instance %s from status %s", osp.nodeName, instanceState)
	}
	return nil
}

func (osp *ospInstance) Stop() error {
	instanceState, err := osp.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := startStates[instanceState]; ok {
		err = osp.ospObj.GetStopOspInstance(osp.client, osp.nodeName)
		if err != nil {
			return fmt.Errorf("stop instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to stop instance %s from status %s", osp.nodeName, instanceState)
	}
	return nil
}

func (osp *ospInstance) State() (string, error) {
	instanceState, err := osp.ospObj.GetOspInstanceState(osp.client, osp.nodeName)
	if err == nil {
		e2e.Logf("VM %s is : %s", osp.nodeName, strings.ToLower(instanceState))
		return strings.ToLower(instanceState), nil
	}
	return "", err
}
