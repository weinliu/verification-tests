package disasterrecovery

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ospInstance struct {
	instance
	ospObj exutil.Osp
}

// GetOspMasterNodes get master nodes and load clouds cred.
func GetOspMasterNodes(oc *exutil.CLI) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, "master")
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
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		instanceState, err := osp.ospObj.GetOspInstanceState(osp.nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(instanceState) == "paused" {
			err := osp.ospObj.GetStartOspInstance(osp.nodeName)
			if err != nil {
				e2e.Logf("Start instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if strings.ToLower(instanceState) == "active" {
			e2e.Logf("Instnace already running %s", osp.nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to start %s", osp.nodeName))
	return errVmstate
}

func (osp *ospInstance) Stop() error {
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		instanceState, err := osp.ospObj.GetOspInstanceState(osp.nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.ToLower(instanceState) == "active" {
			err := osp.ospObj.GetStopOspInstance(osp.nodeName)
			if err != nil {
				e2e.Logf("Stop instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if strings.ToLower(instanceState) == "paused" {
			e2e.Logf("Instance already stopped %v", osp.nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to stop %s", osp.nodeName))
	return errVmstate
}

func (osp *ospInstance) State() (string, error) {
	instanceState, err := osp.ospObj.GetOspInstanceState(osp.nodeName)
	if err == nil {
		e2e.Logf("VM %s is : %s", osp.nodeName, strings.ToLower(instanceState))
		return strings.ToLower(instanceState), nil
	}
	return "", err
}
