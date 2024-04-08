package disasterrecovery

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/vmware/govmomi"
	"gopkg.in/ini.v1"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type vsphereInstance struct {
	instance
	vspObj         *exutil.Vmware
	vspClient      *govmomi.Client
	vmRelativePath string
}

// Get nodes and load clouds cred with the specified label.
func GetVsphereNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	vspObj, vspClient, vmRelativePath := VsphereCloudClient(oc)
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newVsphereInstance(oc, vspObj, vspClient, nodeName, vmRelativePath))
	}
	return results, nil
}

// VsphereCloudClient pass env details to login function, and used to login
func VsphereCloudClient(oc *exutil.CLI) (*exutil.Vmware, *govmomi.Client, string) {
	randomStr := exutil.GetRandomString()
	dirname := fmt.Sprintf("/tmp/-dr_vsphere_login_%s/", randomStr)
	defer os.RemoveAll(dirname)
	os.MkdirAll(dirname, 0755)
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
	vSphereConfigFile, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/cloud-provider-config", "-n", "openshift-config", "-o", `jsonpath={.data.config}`).OutputToFile("dr_vsphere_login_" + randomStr + "/server.ini")
	o.Expect(err3).NotTo(o.HaveOccurred())
	envURL, vmRelativePath, err4 := getvSphereServerConfig(vSphereConfigFile)
	o.Expect(err4).NotTo(o.HaveOccurred())
	envUsername := string(accessKeyID)
	envPassword := string(secureKey)
	encodedPassword := url.QueryEscape(envPassword)
	govmomiURL := fmt.Sprintf("https://%s:%s@%s/sdk", envUsername, encodedPassword, envURL)
	vmware := exutil.Vmware{GovmomiURL: govmomiURL}
	vm, client := vmware.Login()
	return vm, client, vmRelativePath
}

func newVsphereInstance(oc *exutil.CLI, vspObj *exutil.Vmware, vspClient *govmomi.Client, nodeName string, vmRelativePath string) *vsphereInstance {
	return &vsphereInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		vspObj:         vspObj,
		vspClient:      vspClient,
		vmRelativePath: vmRelativePath,
	}
}

func (vs *vsphereInstance) GetInstanceID() (string, error) {
	var instanceID string
	var err error
	errVmId := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		instanceID, err = vs.vspObj.GetVspheresInstance(vs.vspClient, vs.vmRelativePath+vs.nodeName)
		if err == nil {
			e2e.Logf("VM instance name: %s", instanceID)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmId, fmt.Sprintf("Failed to get VM instance ID for node: %s, error: %s", vs.nodeName, err))
	return instanceID, err
}

func (vs *vsphereInstance) Start() error {
	instanceState, err := vs.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := stopStates[instanceState]; ok {
		err = vs.vspObj.StartVsphereInstance(vs.vspClient, vs.vmRelativePath+vs.nodeName)
		if err != nil {
			return fmt.Errorf("start instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to start instance %s from status %s", vs.nodeName, instanceState)
	}
	return nil
}

func (vs *vsphereInstance) Stop() error {
	instanceState, err := vs.State()
	o.Expect(err).NotTo(o.HaveOccurred())
	if _, ok := startStates[instanceState]; ok {
		err = vs.vspObj.StopVsphereInstance(vs.vspClient, vs.vmRelativePath+vs.nodeName)
		if err != nil {
			return fmt.Errorf("stop instance failed with error :: %v", err)
		}
	} else {
		return fmt.Errorf("unalbe to stop instance %s from status %s", vs.nodeName, instanceState)
	}
	return nil
}

func (vs *vsphereInstance) State() (string, error) {
	instanceState, statusErr := vs.vspObj.GetVspheresInstanceState(vs.vspClient, vs.vmRelativePath+vs.nodeName)
	return strings.ToLower(instanceState), statusErr
}

func getvSphereServerConfig(vSphereConfigFile string) (string, string, error) {
	// Load the INI configuration file
	cfg, err := ini.Load(vSphereConfigFile)
	if err != nil {
		return "", "", fmt.Errorf("Error loading configuration: %s", err)
	}

	// Retrieve the server URL from the [Workspace] section
	serverURL := cfg.Section("Workspace").Key("server").String()

	// Retrieve the folder from the [Workspace] section
	vmRelativePath := cfg.Section("Workspace").Key("folder").String()
	return serverURL, vmRelativePath + "/", nil
}
