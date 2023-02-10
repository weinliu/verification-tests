package mco

import (
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

	pollerr := wait.Poll(5*time.Second, 1*time.Minute, func() (bool, error) {
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
		mcp.waitForComplete()
	}

}

// we need this method to be able to delete the MC without waiting for success.
// TODO: This method should be deleted when we refactor the MC struct to embed the Resource struct. But right now we have no other choice.
func (mc *MachineConfig) deleteNoWait() error {
	return mc.Delete()
}

func (mc *MachineConfig) delete() {
	err := mc.oc.AsAdmin().WithoutNamespace().Run("delete").Args("mc", mc.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	mcp := NewMachineConfigPool(mc.oc, mc.pool)
	mcp.waitForComplete()
}

func (mc *MachineConfig) GetExtensions() (string, error) {
	return mc.Get(`{.spec.extensions}`)
}
