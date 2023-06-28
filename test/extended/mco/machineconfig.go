package mco

import (
	"context"
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

// MachineConfig struct is used to handle MachineConfig resources in OCP
type MachineConfig struct {
	Resource
	Template
	pool           string
	parameters     []string
	skipWaitForMcp bool
}

// NewMachineConfig create a NewMachineConfig struct
func NewMachineConfig(oc *exutil.CLI, name, pool string) *MachineConfig {
	mc := &MachineConfig{Resource: *NewResource(oc, "mc", name), pool: pool}
	return mc.SetTemplate(*NewMCOTemplate(oc, GenericMCTemplate))
}

// SetTemplate sets the template that will be used by the "create" method in order to create the MC
func (mc *MachineConfig) SetTemplate(template Template) *MachineConfig {
	mc.Template = template
	return mc
}

// SetMCOTemplate set a template defined in the MCO testdata folder
func (mc *MachineConfig) SetMCOTemplate(templateName string) *MachineConfig {
	mc.Template = *NewMCOTemplate(mc.oc, templateName)
	return mc
}

func (mc *MachineConfig) create() {
	mc.name = mc.name + "-" + exutil.GetRandomString()
	params := []string{"-p", "NAME=" + mc.name, "POOL=" + mc.pool}
	params = append(params, mc.parameters...)
	mc.Create(params...)

	immediate := false
	pollerr := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 1*time.Minute, immediate, func(ctx context.Context) (bool, error) {
		stdout, err := mc.Get(`{.metadata.name}`)
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, mc.name) {
			logger.Infof("mc %s is created successfully", mc.name)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(pollerr, fmt.Sprintf("create machine config %v failed", mc.name))

	if !mc.skipWaitForMcp {
		mcp := NewMachineConfigPool(mc.oc, mc.pool)
		if mc.GetKernelTypeSafe() != "" {
			mcp.SetWaitingTimeForRTKernel() // Since we configure realtime kernel we wait longer for completion
		}
		mcp.waitForComplete()
	}

}

// we need this method to be able to delete the MC without waiting for success.
// TODO: This method should be deleted when we refactor the MC struct to embed the Resource struct. But right now we have no other choice.
func (mc *MachineConfig) deleteNoWait() error {
	return mc.Delete()
}

func (mc *MachineConfig) delete() {
	mcp := NewMachineConfigPool(mc.oc, mc.pool)
	if mc.GetKernelTypeSafe() != "" {
		mcp.SetWaitingTimeForRTKernel() // If the MC is configuring realtime kernel, we increase the waiting period
	}

	err := mc.oc.AsAdmin().WithoutNamespace().Run("delete").Args("mc", mc.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	mcp.waitForComplete()
}

// GetExtensions returns all the extensions configured in this MC
func (mc *MachineConfig) GetExtensions() (string, error) {
	return mc.Get(`{.spec.extensions}`)
}

// GetAuthorizedKeysByUser returns the authorizedkeys that this MC defines for the given user in a json list format
func (mc *MachineConfig) GetAuthorizedKeysByUser(user string) (string, error) {
	return mc.Get(fmt.Sprintf(`{.spec.config.passwd.users[?(@.name=="%s")].sshAuthorizedKeys}`, user))
}

// Get the kernelType configured in this MC. If any arror happens it returns an empty string
func (mc *MachineConfig) GetKernelTypeSafe() string {
	return mc.GetSafe(`{.spec.kernelType}`, "")
}

// GetAuthorizedKeysByUserAsList returns the authorizedkeys that this MC defines for the given user as a list of strings
func (mc *MachineConfig) GetAuthorizedKeysByUserAsList(user string) ([]string, error) {
	listKeys := []string{}

	keys, err := mc.Get(fmt.Sprintf(`{.spec.config.passwd.users[?(@.name=="%s")].sshAuthorizedKeys}`, user))
	if err != nil {
		return nil, err
	}

	if keys == "" {
		return listKeys, nil
	}

	jKeys := JSON(keys)
	for _, key := range jKeys.Items() {
		listKeys = append(listKeys, key.ToString())
	}

	return listKeys, err
}

// GetIgnitionVersion returns the ignition version used in the MC
func (mc *MachineConfig) GetIgnitionVersion() (string, error) {
	return mc.Get(`{.spec.config.ignition.version}`)
}
