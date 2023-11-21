package disasterrecovery

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/vmware/govmomi"

	o "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type vsphereInstance struct {
	instance
	vspObj    *exutil.Vmware
	vspClient *govmomi.Client
}

// Get nodes and load clouds cred with the specified label.
func GetVsphereNodes(oc *exutil.CLI, label string) ([]ComputeNode, func()) {
	nodeNames, err := exutil.GetClusterNodesBy(oc, label)
	o.Expect(err).NotTo(o.HaveOccurred())
	vspObj, vspClient := VsphereCloudClient(oc)
	var results []ComputeNode
	for _, nodeName := range nodeNames {
		results = append(results, newVsphereInstance(oc, vspObj, vspClient, nodeName))
	}
	return results, nil
}

// VsphereCloudClient pass env details to login function, and used to login
func VsphereCloudClient(oc *exutil.CLI) (*exutil.Vmware, *govmomi.Client) {
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
	cloudConfig, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm/cloud-provider-config", "-n", "openshift-config", "-o", `jsonpath={.data.config}`).OutputToFile("dr_vsphere_login_" + randomStr + "/server.ini")
	o.Expect(err3).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf(`grep -i server "%v" | awk -F '"' '{print $2}'`, cloudConfig)
	serverURL, err4 := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err4).NotTo(o.HaveOccurred())
	envUsername := string(accessKeyID)
	envPassword := string(secureKey)
	envURL := string(serverURL)
	envURL = strings.TrimSuffix(envURL, "\n")
	encodedPassword := url.QueryEscape(envPassword)
	govmomiURL := fmt.Sprintf("https://%s:%s@%s/sdk", envUsername, encodedPassword, envURL)
	vmware := exutil.Vmware{GovmomiURL: govmomiURL}
	return vmware.Login()
}

func newVsphereInstance(oc *exutil.CLI, vspObj *exutil.Vmware, vspClient *govmomi.Client, nodeName string) *vsphereInstance {
	return &vsphereInstance{
		instance: instance{
			nodeName: nodeName,
			oc:       oc,
		},
		vspObj:    vspObj,
		vspClient: vspClient,
	}
}

func (vs *vsphereInstance) GetInstanceID() (string, error) {
	var instanceID string
	var err error
	errVmId := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		instanceID, err = vs.vspObj.GetVspheresInstance(vs.vspClient, vs.nodeName)
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
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		vmState, _ := vs.vspObj.GetVspheresInstanceState(vs.vspClient, vs.nodeName)
		if vmState == "poweredOff" {
			err := vs.vspObj.StartVsphereInstance(vs.vspClient, vs.nodeName)
			if err != nil {
				e2e.Logf("Start instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if vmState == "poweredOn" {
			e2e.Logf("Instnace already running %s", vs.nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to poweron %s", vs.nodeName))
	return errVmstate
}

func (vs *vsphereInstance) Stop() error {
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		vmState, _ := vs.vspObj.GetVspheresInstanceState(vs.vspClient, vs.nodeName)
		if vmState == "poweredOn" {
			err := vs.vspObj.StopVsphereInstance(vs.vspClient, vs.nodeName)
			if err != nil {
				e2e.Logf("Stop instance failed with error :: %v.", err)
				return false, nil
			}
			return true, nil
		} else if vmState == "poweredOff" {
			e2e.Logf("Instnace already powerOff %s", vs.nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to powerOff %s", vs.nodeName))
	return errVmstate
}

func (vs *vsphereInstance) State() (string, error) {
	var nodeStatus string
	var statusErr error
	errVmstate := wait.Poll(10*time.Second, 200*time.Second, func() (bool, error) {
		nodeStatus, statusErr = vs.vspObj.GetVspheresInstanceState(vs.vspClient, vs.nodeName)
		if statusErr != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Failed to get VM instance state for node: %s, error: %s", vs.nodeName, statusErr))
	return nodeStatus, statusErr
}
