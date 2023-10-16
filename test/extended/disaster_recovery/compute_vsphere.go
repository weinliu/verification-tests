package disasterrecovery

import (
	"encoding/base64"
	"fmt"
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
	govmomiURL := "https://" + envUsername + ":" + envPassword + "@" + envURL + "/sdk"
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
	instanceID, err := vs.vspObj.GetVspheresInstance(vs.vspClient, vs.nodeName)
	if err == nil {
		e2e.Logf("VM instance name: %s", instanceID)
		return instanceID, nil
	}
	return "", err
}

func (vs *vsphereInstance) Start() error {
	err := vs.vspObj.StartVsphereInstance(vs.vspClient, vs.nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		vmState, err := vs.vspObj.GetVspheresInstanceState(vs.vspClient, vs.nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if vmState == "poweredOff" {
			return false, nil
		} else if vmState == "poweredOn" {
			e2e.Logf("%s already poweron", vs.nodeName)
			return true, nil
		}
		e2e.Logf("%s has been poweron", vs.nodeName)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to poweron %s", vs.nodeName))
	return errVmstate
}

func (vs *vsphereInstance) Stop() error {
	err := vs.vspObj.StopVsphereInstance(vs.vspClient, vs.nodeName)
	o.Expect(err).NotTo(o.HaveOccurred())
	errVmstate := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		vmState, err := vs.vspObj.GetVspheresInstanceState(vs.vspClient, vs.nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if vmState == "poweredOn" {
			return false, nil
		} else if vmState == "poweredOff" {
			e2e.Logf("%s already poweroff", vs.nodeName)
			return true, nil
		}
		e2e.Logf("%s has been poweroff", vs.nodeName)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errVmstate, fmt.Sprintf("Not able to poweroff %s", vs.nodeName))
	return errVmstate
}

func (vs *vsphereInstance) State() (string, error) {
	return vs.vspObj.GetVspheresInstanceState(vs.vspClient, vs.nodeName)
}
