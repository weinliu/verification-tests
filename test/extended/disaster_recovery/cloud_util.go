package disasterrecovery

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/vmware/govmomi"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ocpInstance struct {
	platform string
	nodeName string
}

// GetVsphereCloudClient pass env details to login function, and used to login
func GetVsphereCloudClient(oc *exutil.CLI) (*exutil.Vmware, *govmomi.Client) {
	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/vsphere-creds", "-n", "kube-system", "-o", `jsonpath={.data}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	output := gjson.Parse(credential).Value().(map[string]interface{})
	var accessKeyIDBase64 string
	var secureKeyBase64 string
	for key, value := range output {
		if strings.Contains(key, "username") {
			accessKeyIDBase64 = fmt.Sprint(value)
		} else if strings.Contains(key, "password") {
			secureKeyBase64 = fmt.Sprint(value)
		}
	}
	accessKeyID, err1 := base64.StdEncoding.DecodeString(accessKeyIDBase64)
	o.Expect(err1).NotTo(o.HaveOccurred())
	secureKey, err2 := base64.StdEncoding.DecodeString(secureKeyBase64)
	o.Expect(err2).NotTo(o.HaveOccurred())
	cloudConfig, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/cloud-provider-config", "-n", "openshift-config", "-o", `jsonpath={.data.config}`).OutputToFile("OCP-19941/server.ini")
	o.Expect(err3).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf(`grep -i server "%v" | awk -F '"' '{print $2}'`, cloudConfig)
	serverURL, err4 := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err4).NotTo(o.HaveOccurred())
	envUsername := string(accessKeyID)
	envPassword := string(secureKey)
	envURL := string(serverURL)
	envURL = strings.TrimSuffix(envURL, "\n")
	govmomiURL := "https://" + envUsername + ":" + envPassword + "@" + envURL + "/sdk"
	vmware := exutil.Vmware{GovmomiURL: govmomiURL}
	return vmware.Login()
}

// GetVsphereInstance to list vmware instance.
func GetVsphereInstance(oc *exutil.CLI, nodeName string) (string, error) {
	vmware, c := GetVsphereCloudClient(oc)
	defer func() {
		errLogout := vmware.GetVsphereConnectionLogout(c)
		o.Expect(errLogout).NotTo(o.HaveOccurred())
	}()
	return vmware.GetVspheresInstance(c, nodeName)
}

// GetVsphereInstanceState to check state of vm instance
func GetVsphereInstanceState(oc *exutil.CLI, nodeName string) (string, error) {
	vmware, c := GetVsphereCloudClient(oc)
	defer func() {
		errLogout := vmware.GetVsphereConnectionLogout(c)
		o.Expect(errLogout).NotTo(o.HaveOccurred())
	}()
	return vmware.GetVspheresInstanceState(c, nodeName)
}

// StopVsphereInstance to stop vvm instance
func StopVsphereInstance(oc *exutil.CLI, nodeName string) error {
	vmware, c := GetVsphereCloudClient(oc)
	defer func() {
		errLogout := vmware.GetVsphereConnectionLogout(c)
		o.Expect(errLogout).NotTo(o.HaveOccurred())
	}()
	err := vmware.StopVsphereInstance(c, nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		vmState, err := vmware.GetVspheresInstanceState(c, nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if vmState == "poweredOn" {
			return false, nil
		} else if vmState == "poweredOff" {
			e2e.Logf("%s already poweroff", nodeName)
			return true, nil
		}
		e2e.Logf("%s has been poweroff", nodeName)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to poweroff %s", nodeName))
	return errVmstate
}

// StartVsphereInstance to start vm instance
func StartVsphereInstance(oc *exutil.CLI, nodeName string) error {
	vmware, c := GetVsphereCloudClient(oc)
	defer func() {
		errLogout := vmware.GetVsphereConnectionLogout(c)
		o.Expect(errLogout).NotTo(o.HaveOccurred())
	}()
	err := vmware.StartVsphereInstance(c, nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		vmState, err := vmware.GetVspheresInstanceState(c, nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if vmState == "poweredOff" {
			return false, nil
		} else if vmState == "poweredOn" {
			e2e.Logf("%s already poweron", nodeName)
			return true, nil
		}
		e2e.Logf("%s has been poweron", nodeName)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to poweron %s", nodeName))
	return errVmstate
}

// GetAwsInstanceID AWS instance ID
func GetAwsInstanceID(oc *exutil.CLI, nodeName string) (string, error) {
	exutil.GetAwsCredentialFromCluster(oc)
	a := exutil.InitAwsSession()
	instanceID, err := a.GetAwsInstanceIDFromHostname(nodeName)
	if err != nil {
		e2e.Logf("Get instance id failed with error :: %v.", err)
		return "", err
	}
	return instanceID, nil
}

// GetAwsInstanceState to get instance state
func GetAwsInstanceState(oc *exutil.CLI, nodeName string) (string, error) {
	exutil.GetAwsCredentialFromCluster(oc)
	a := exutil.InitAwsSession()
	instanceID, err := a.GetAwsInstanceIDFromHostname(nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	instanceState, err := a.GetAwsInstanceState(instanceID)
	if err != nil {
		e2e.Logf("Get instance state failed with error :: %v.", err)
		return "", err
	}
	return instanceState, nil
}

// StopAwsInstance to stop aws instance
func StopAwsInstance(oc *exutil.CLI, nodeName string) error {
	exutil.GetAwsCredentialFromCluster(oc)
	a := exutil.InitAwsSession()
	instanceID, err := a.GetAwsInstanceIDFromHostname(nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	vmState, err := GetAwsInstanceState(oc, nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	if vmState == "stopped" {
		e2e.Logf("%s already Stopped", nodeName)
	} else {
		err = a.StopInstance(instanceID)
		if err != nil {
			e2e.Logf("Stop instance failed with error :: %v.", err)
			return err
		}
	}
	return nil
}

// StartAwsInstance to start aws instance
func StartAwsInstance(oc *exutil.CLI, nodeName string) error {
	exutil.GetAwsCredentialFromCluster(oc)
	a := exutil.InitAwsSession()
	instanceID, err := a.GetAwsInstanceIDFromHostname(nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	errVmstate := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
		vmState, err := GetAwsInstanceState(oc, nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if vmState == "stopped" {
			err = a.StartInstance(instanceID)
			if err != nil {
				e2e.Logf("Start instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if vmState == "runnimg" {
			e2e.Logf("%s already running", nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to restart %s", nodeName))
	return errVmstate
}

// GetInstance to get instance from cloud platform
func (envPlatform ocpInstance) GetInstance(oc *exutil.CLI) (string, error) {
	var (
		vmInstance string
		err        error
	)
	switch envPlatform.platform {
	case "vsphere":
		vmInstance, err = GetVsphereInstance(oc, envPlatform.nodeName)
	case "aws":
		vmInstance, err = GetAwsInstanceID(oc, envPlatform.nodeName)
	case "gcp":
		vmInstance, err = GetGcpInstance(oc, envPlatform.nodeName)
	case "openstack":
		vmInstance, err = GetOspInstance(oc, envPlatform.nodeName)
	case "azure":
		vmInstance, err = GetAzureInstance(oc, envPlatform.nodeName)
	case "baremetal":
		vmInstance, err = GetBmhNodeMachineConfig(oc, envPlatform.nodeName)
	default:
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return vmInstance, nil
}

// GetInstanceState to get instance from cloud platform
func (envPlatform ocpInstance) GetInstanceState(oc *exutil.CLI) (string, error) {
	var (
		vmState string
		err     error
	)
	switch envPlatform.platform {
	case "vsphere":
		vmState, err = GetVsphereInstanceState(oc, envPlatform.nodeName)
	case "aws":
		vmState, err = GetAwsInstanceState(oc, envPlatform.nodeName)
	case "gcp":
		vmState, err = GetGcpInstanceState(oc, envPlatform.nodeName)
	case "openstack":
		vmState, err = GetOspInstanceState(oc, envPlatform.nodeName)
	case "azure":
		vmState, err = GetAzureInstanceState(oc, envPlatform.nodeName)
	case "baremetal":
		vmState, err = GetNodestatus(oc, envPlatform.nodeName)
	default:
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return vmState, nil
}

// StopInstance instance on cloud platform
func (envPlatform ocpInstance) StopInstance(oc *exutil.CLI) error {
	var err error
	switch envPlatform.platform {
	case "vsphere":
		err = StopVsphereInstance(oc, envPlatform.nodeName)
	case "aws":
		err = StopAwsInstance(oc, envPlatform.nodeName)
		// after stopping VM wait for some time because error got delay in aws while running case in prow ci.
		time.Sleep(time.Second * 10)
	case "gcp":
		err = GetGcpInstanceStopAsync(oc, envPlatform.nodeName)
	case "openstack":
		err = GetOspInstancesStop(oc, envPlatform.nodeName)
	case "azure":
		err = StoptAzureVMInstance(oc, envPlatform.nodeName)
	case "baremetal":
		err = StopIPIBaremetalNode(oc, envPlatform.nodeName)
	default:
		return nil
	}
	return err
}

// StartInstance on cloud platform
func (envPlatform ocpInstance) StartInstance(oc *exutil.CLI) error {
	var err error
	switch envPlatform.platform {
	case "vsphere":
		err = StartVsphereInstance(oc, envPlatform.nodeName)
	case "aws":
		err = StartAwsInstance(oc, envPlatform.nodeName)
	case "gcp":
		err = GetGcpInstanceStart(oc, envPlatform.nodeName)
	case "openstack":
		err = GetOspInstancesStart(oc, envPlatform.nodeName)
	case "azure":
		err = StartAzureVMInstance(oc, envPlatform.nodeName)
	case "baremetal":
		err = StartIPIBaremetalNode(oc, envPlatform.nodeName)
	default:
		return nil
	}
	return err
}

// GetGcloudClient to login on gcloud platform
func GetGcloudClient(oc *exutil.CLI) *exutil.Gcloud {
	projectID, err := exutil.GetGcpProjectID(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	if projectID != "openshift-qe" {
		g.Skip("openshift-qe project is needed to execute this test case!")
	}
	gcloud := exutil.Gcloud{ProjectID: projectID}
	return gcloud.Login()
}

// GetGcpInstance from gcp cloud
func GetGcpInstance(oc *exutil.CLI, infraID string) (string, error) {
	instanceID, err := GetGcloudClient(oc).GetGcpInstanceByNode(infraID)
	if err == nil {
		e2e.Logf("VM instance name: %s", instanceID)
		return instanceID, nil
	}
	return "", err
}

// GetGcpInstanceState from gcp cloud
func GetGcpInstanceState(oc *exutil.CLI, infraID string) (string, error) {
	instanceState, err := GetGcloudClient(oc).GetGcpInstanceStateByNode(infraID)
	if err == nil {
		e2e.Logf("VM %s is : %s", infraID, strings.ToLower(instanceState))
		return strings.ToLower(instanceState), nil
	}
	return "", err
}

// GetGcpInstanceStart on gcp cloud
func GetGcpInstanceStart(oc *exutil.CLI, infraID string) error {
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		instanceState, err := GetGcpInstanceState(oc, infraID)
		o.Expect(err).NotTo(o.HaveOccurred())
		if instanceState == "terminated" {
			nodeInstance := strings.Split(infraID, ".")
			zoneName, err := GetGcloudClient(oc).GetZone(infraID, nodeInstance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			err = GetGcloudClient(oc).StartInstance(nodeInstance[0], zoneName)
			if err != nil {
				e2e.Logf("Start instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if instanceState == "running" {
			e2e.Logf("%v already running", infraID)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to start %s", infraID))
	return errVmstate
}

// GetGcpInstanceStopAsync on cloud platform
func GetGcpInstanceStopAsync(oc *exutil.CLI, infraID string) error {
	nodeInstance := strings.Split(infraID, ".")
	zoneName, err := GetGcloudClient(oc).GetZone(infraID, nodeInstance[0])
	o.Expect(err).NotTo(o.HaveOccurred())
	instanceState, err := GetGcpInstanceState(oc, infraID)
	o.Expect(err).NotTo(o.HaveOccurred())
	if instanceState == "terminated" {
		e2e.Logf("%v already stopped", infraID)
	} else {
		err = GetGcloudClient(oc).StopInstanceAsync(nodeInstance[0], zoneName)
		if err != nil {
			e2e.Logf("Stop instance failed with error :: %v.", err)
			return err
		}
	}
	return nil
}

// GetOspCredentials get creds of osp platform
func GetOspCredentials(oc *exutil.CLI) {
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

// GetOspInstance from osp platform
func GetOspInstance(oc *exutil.CLI, infraID string) (string, error) {
	GetOspCredentials(oc)
	osp := exutil.Osp{}
	instanceName, err := osp.GetOspInstance(infraID)
	if err == nil {
		e2e.Logf("VM instance name: %s", instanceName)
		return instanceName, nil
	}
	return "", err
}

// GetOspInstanceState on osp platform
func GetOspInstanceState(oc *exutil.CLI, infraID string) (string, error) {
	GetOspCredentials(oc)
	osp := exutil.Osp{}
	instanceState, err := osp.GetOspInstanceState(infraID)
	if err == nil {
		e2e.Logf("VM %s is : %s", infraID, strings.ToLower(instanceState))
		return strings.ToLower(instanceState), nil
	}
	return "", err
}

// GetOspInstancesStart on osp platform
func GetOspInstancesStart(oc *exutil.CLI, infraID string) error {
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		instanceState, err := GetOspInstanceState(oc, infraID)
		o.Expect(err).NotTo(o.HaveOccurred())
		if instanceState == "paused" {
			GetOspCredentials(oc)
			osp := exutil.Osp{}
			err := osp.GetStartOspInstance(infraID)
			if err != nil {
				e2e.Logf("Start instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if instanceState == "active" {
			e2e.Logf("Instnace already running %s", infraID)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to start %s", infraID))
	return errVmstate
}

// GetOspInstancesStop on osp platfrom
func GetOspInstancesStop(oc *exutil.CLI, infraID string) error {
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		instanceState, err := GetOspInstanceState(oc, infraID)
		o.Expect(err).NotTo(o.HaveOccurred())
		if instanceState == "active" {
			GetOspCredentials(oc)
			osp := exutil.Osp{}
			err := osp.GetStopOspInstance(infraID)
			if err != nil {
				e2e.Logf("Stop instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if instanceState == "paused" {
			e2e.Logf("Instance already stopped %v", infraID)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to stop %s", infraID))
	return errVmstate
}

// GetAzureInstanceState get azure vm instance state
func GetAzureInstanceState(oc *exutil.CLI, vmInstance string) (string, error) {
	resourceGroupName, rgerr := exutil.GetAzureCredentialFromCluster(oc)
	o.Expect(rgerr).NotTo(o.HaveOccurred())
	session, sessErr := exutil.NewAzureSessionFromEnv()
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	instanceState, instanceErr := exutil.GetAzureVMInstanceState(session, vmInstance, resourceGroupName)
	o.Expect(instanceErr).NotTo(o.HaveOccurred())
	return instanceState, instanceErr
}

// GetAzureInstance get azure vm instance
func GetAzureInstance(oc *exutil.CLI, vmInstance string) (string, error) {
	resourceGroupName, rgerr := exutil.GetAzureCredentialFromCluster(oc)
	o.Expect(rgerr).NotTo(o.HaveOccurred())
	session, sessErr := exutil.NewAzureSessionFromEnv()
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	instanceName, instanceErr := exutil.GetAzureVMInstance(session, vmInstance, resourceGroupName)
	o.Expect(instanceErr).NotTo(o.HaveOccurred())
	return instanceName, instanceErr
}

// StartAzureVMInstance start azure vm
func StartAzureVMInstance(oc *exutil.CLI, vmInstance string) error {
	resourceGroupName, rgerr := exutil.GetAzureCredentialFromCluster(oc)
	o.Expect(rgerr).NotTo(o.HaveOccurred())
	session, sessErr := exutil.NewAzureSessionFromEnv()
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	_, instanceErr := exutil.StartAzureVM(session, vmInstance, resourceGroupName)
	o.Expect(instanceErr).NotTo(o.HaveOccurred())
	return instanceErr
}

// StoptAzureVMInstance stop azure vm
func StoptAzureVMInstance(oc *exutil.CLI, vmInstance string) error {
	resourceGroupName, rgerr := exutil.GetAzureCredentialFromCluster(oc)
	o.Expect(rgerr).NotTo(o.HaveOccurred())
	session, sessErr := exutil.NewAzureSessionFromEnv()
	o.Expect(sessErr).NotTo(o.HaveOccurred())
	_, instanceErr := exutil.StopAzureVM(session, vmInstance, resourceGroupName)
	o.Expect(instanceErr).NotTo(o.HaveOccurred())
	return instanceErr
}

// GetBmhNodeMachineConfig get bmh machineconfig name
func GetBmhNodeMachineConfig(oc *exutil.CLI, vmInstance string) (string, error) {
	var masterNodeMachineConfig string
	bmhOutput, bmhErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("bmh", "-n", "openshift-machine-api", "-o", `jsonpath='{.items[*].metadata.name}'`).Output()
	o.Expect(bmhErr).NotTo(o.HaveOccurred())
	machineConfigOutput := strings.Fields(bmhOutput)
	for i := 0; i < len(machineConfigOutput); i++ {
		if strings.Contains(machineConfigOutput[i], vmInstance) {
			masterNodeMachineConfig = strings.ReplaceAll(machineConfigOutput[i], "'", "")
		}
	}
	return masterNodeMachineConfig, bmhErr
}

// GetNodestatus get node status
func GetNodestatus(oc *exutil.CLI, vmInstance string) (string, error) {
	nodeStatus, statusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", vmInstance, "-o", `jsonpath={.status.conditions[3].type}`).Output()
	o.Expect(statusErr).NotTo(o.HaveOccurred())
	return strings.ToLower(nodeStatus), statusErr
}

// StopIPIBaremetalNode stop ipi baremetal node
func StopIPIBaremetalNode(oc *exutil.CLI, vmInstance string) error {
	errVM := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		vmInstance, err := GetBmhNodeMachineConfig(oc, vmInstance)
		if err != nil {
			return false, nil
		}
		patch := `[{"op": "replace", "path": "/spec/online", "value": false}]`
		stopErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", "openshift-machine-api", vmInstance, "--type=json", "-p", patch).Execute()
		if stopErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVM, fmt.Sprintf("Not able to stop %s", vmInstance))
	return errVM
}

// StartIPIBaremetalNode stop ipi baremetal node
func StartIPIBaremetalNode(oc *exutil.CLI, vmInstance string) error {
	errVM := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		vmInstance, err := GetBmhNodeMachineConfig(oc, vmInstance)
		if err != nil {
			return false, nil
		}
		patch := `[{"op": "replace", "path": "/spec/online", "value": true}]`
		startErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("bmh", "-n", "openshift-machine-api", vmInstance, "--type=json", "-p", patch).Execute()
		if startErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVM, fmt.Sprintf("Not able to start %s", vmInstance))
	return errVM
}
