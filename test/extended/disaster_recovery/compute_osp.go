package disasterrecovery

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ospInstance struct {
	instance
	ospObj exutil.Osp
}

// Get nodes and load clouds cred with the specified label.
func GetOspNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	OspCredentials(oc)
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newOspInstance(oc, nodeName))
	}
	return results, nil
}

// OspCredentials get creds of osp platform
func OspCredentials(oc *exutil.CLI) {
	credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/openstack-credentials", "-n", "kube-system", "-o", `jsonpath={.data.clouds\.yaml}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	credential, err := base64.StdEncoding.DecodeString(credentials)
	o.Expect(err).NotTo(o.HaveOccurred())
	var (
		username       string
		password       string
		projectID      string
		authURL        string
		userDomainName string
		regionName     string
		projectName    string
	)
	credVars := []string{"auth_url", "username", "password", "project_id", "user_domain_name", "region_name", "project_name"}
	for _, s := range credVars {
		r, _ := regexp.Compile(`` + s + `:.*`)
		match := r.FindAllString(string(credential), -1)
		if strings.Contains(s, "username") {
			username = strings.Split(match[0], " ")[1]
			os.Setenv("OSP_DR_USERNAME", username)
		}
		if strings.Contains(s, "password") {
			password = strings.Split(match[0], " ")[1]
			os.Setenv("OSP_DR_PASSWORD", password)
		}
		if strings.Contains(s, "auth_url") {
			authURL = strings.Split(match[0], " ")[1]
			os.Setenv("OSP_DR_AUTH_URL", authURL)
		}
		if strings.Contains(s, "project_id") {
			projectID = strings.Split(match[0], " ")[1]
			os.Setenv("OSP_DR_PROJECT_ID", projectID)
		}
		if strings.Contains(s, "user_domain_name") {
			userDomainName = strings.Split(match[0], " ")[1]
			os.Setenv("OSP_DR_USER_DOMAIN_NAME", userDomainName)
		}
		if strings.Contains(s, "region_name") {
			regionName = strings.Split(match[0], " ")[1]
			os.Setenv("OSP_DR_REGION_NAME", regionName)
		}
		if strings.Contains(s, "project_name") {
			projectName = strings.Split(match[0], " ")[1]
			os.Setenv("OSP_DR_PROJECT_NAME", projectName)
		}
	}
}

func newOspInstance(oc *exutil.CLI, nodeName string) *ospInstance {
	OspCredentials(oc)
	return &ospInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		ospObj: exutil.Osp{},
	}
}

func (osp *ospInstance) GetInstanceID() (string, error) {
	instanceID, err := osp.ospObj.GetOspInstance(osp.nodeName)
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
		err = osp.ospObj.GetStartOspInstance(osp.nodeName)
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
		err = osp.ospObj.GetStopOspInstance(osp.nodeName)
		if err != nil {
			return fmt.Errorf("stop instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to stop instance %s from status %s", osp.nodeName, instanceState)
	}
	return nil
}

func (osp *ospInstance) State() (string, error) {
	instanceState, err := osp.ospObj.GetOspInstanceState(osp.nodeName)
	if err == nil {
		e2e.Logf("VM %s is : %s", osp.nodeName, strings.ToLower(instanceState))
		return strings.ToLower(instanceState), nil
	}
	return "", err
}
