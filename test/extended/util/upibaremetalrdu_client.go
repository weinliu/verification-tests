package util

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gebn/bmc"
	"github.com/gebn/bmc/pkg/ipmi"
	"github.com/ghodss/yaml"
	"github.com/tidwall/gjson"

	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// A Upirdu represents object ...
type Upirdusession struct {
	NodeName    string `json:"name"`
	BMCHOSTNAME string `json:"bmc_address"`
	BMCUSERNAME string `json:"bmc_user"`
	BMCPASSWORD string `json:"bmc_pass"`
}

func FetchUPIBareMetalCredentials(url, nodeName string) (Upirdusession, error) {
	resp, err := http.Get(url)
	if err != nil {
		return Upirdusession{}, fmt.Errorf("error downloading creds: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Upirdusession{}, fmt.Errorf("error downloading creds: response is not %d", http.StatusOK)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Upirdusession{}, fmt.Errorf("error downloading creds: %w", err)
	}

	data := []Upirdusession{}

	if err := yaml.Unmarshal(body, &data); err != nil {
		return Upirdusession{}, fmt.Errorf("error parsing creds: %w", err)
	}
	instanceName := strings.Split(nodeName, ".")
	for _, node := range data {
		if node.NodeName == instanceName[0] {
			return node, nil
		}
	}
	return Upirdusession{}, fmt.Errorf("no credentials found for node %q", nodeName)
}

// StopUPIbaremetalInstance represents to stop upi baremetal rdu instance ...
func (upirdu *Upirdusession) StopUPIbaremetalInstance() error {
	argBMCAddr := upirdu.BMCHOSTNAME
	username := upirdu.BMCUSERNAME
	password := upirdu.BMCPASSWORD
	cmd := ipmi.ChassisControlPowerOff

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	machine, err := bmc.Dial(ctx, argBMCAddr)
	if err != nil {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	defer machine.Close()
	e2e.Logf("connected to %v over IPMI v%v", machine.Address(), machine.Version())
	sess, err := machine.NewSession(ctx, &bmc.SessionOpts{
		Username:          username,
		Password:          []byte(password),
		MaxPrivilegeLevel: ipmi.PrivilegeLevelOperator,
	})
	if err != nil {
		e2e.Logf("%v", err)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	defer sess.Close(ctx)
	if err := sess.ChassisControl(ctx, cmd); err != nil {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return err
}

// StartUPIbaremetalInstance represents to start upi baremetal rdu instance ...
func (upirdu *Upirdusession) StartUPIbaremetalInstance() error {
	argBMCAddr := upirdu.BMCHOSTNAME
	username := upirdu.BMCUSERNAME
	password := upirdu.BMCPASSWORD
	cmd := ipmi.ChassisControlPowerOn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	machine, err := bmc.Dial(ctx, argBMCAddr)
	if err != nil {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	defer machine.Close()
	e2e.Logf("connected to %v over IPMI v%v", machine.Address(), machine.Version())
	sess, err := machine.NewSession(ctx, &bmc.SessionOpts{
		Username:          username,
		Password:          []byte(password),
		MaxPrivilegeLevel: ipmi.PrivilegeLevelOperator,
	})
	if err != nil {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	defer sess.Close(ctx)
	if err := sess.ChassisControl(ctx, cmd); err != nil {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return err
}

// GetUPIbaremetalInstance represents upi baremetal rdu instance ...
func (upirdu *Upirdusession) GetUPIbaremetalInstance(buildId string, instanceID string) (string, error) {
	cmd := fmt.Sprintf(`curl http://openshift-qe-bastion.arm.eng.rdu2.redhat.com:7788/%v/`, buildId)
	buildconfig, builderr := exec.Command("bash", "-c", cmd).Output()
	o.Expect(builderr).NotTo(o.HaveOccurred())
	instanceName := strings.Split(instanceID, ".")
	yamlToJson, err := yaml.YAMLToJSON([]byte(buildconfig))
	o.Expect(err).NotTo(o.HaveOccurred())
	gjsonOutput := gjson.Get(string(yamlToJson), `#(name=="`+instanceName[0]+`").bmc_address`).String()
	o.Expect(string(gjsonOutput)).NotTo(o.BeEmpty())
	e2e.Logf("Found baremetal instance :: %v", string(gjsonOutput))
	return gjsonOutput, err
}

// GetUPIbaremetalInstanceState( represents to state of upi baremetal rdu instance ...
func (upirdu *Upirdusession) GetUPIbaremetalInstanceState() (string, error) {
	argBMCAddr := upirdu.BMCHOSTNAME
	username := upirdu.BMCUSERNAME
	password := upirdu.BMCPASSWORD
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	machine, err := bmc.Dial(ctx, argBMCAddr)
	if err != nil {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	defer machine.Close()
	e2e.Logf("connected to %v over IPMI v%v", machine.Address(), machine.Version())
	sess, err := machine.NewSession(ctx, &bmc.SessionOpts{
		Username:          username,
		Password:          []byte(password),
		MaxPrivilegeLevel: ipmi.PrivilegeLevelOperator,
	})
	if err != nil {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	defer sess.Close(ctx)
	var instanceState string
	if status, err := sess.GetChassisStatus(ctx); err != nil {
		e2e.Failf("failed to get chassis status:: %v :: %v", argBMCAddr, err)
	} else if status.PoweredOn == true {
		e2e.Logf("UPI baremetal instance :: %v :: poweredOn", argBMCAddr)
		instanceState = "poweredOn"
	} else {
		e2e.Logf("UPI baremetal instance :: %v :: poweredOff", argBMCAddr)
		instanceState = "poweredOff"
	}
	return instanceState, err
}
