package storage

import (
	"testing"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/stretchr/testify/assert"
)

// Self test for storage methods
func TestGererateCsiScExtraParametersAndValidRandomCapacityByVolType(t *testing.T) {
	var (
		oc = exutil.NewCLI("storage-self-test", exutil.KubeConfigPath())
		as = assert.New(t)
	)

	csiVolumeTypes := []string{"io1", "io2", "gp2", "gp3", "sc1", "st1", "standard"}
	for _, volumeType := range csiVolumeTypes {
		scParameters := gererateCsiScExtraParametersByVolType(oc, ebsCsiDriverPrivisioner, volumeType)
		validCapacity := getValidRandomCapacityByCsiVolType(ebsCsiDriverPrivisioner, volumeType)
		as.Contains(interfaceToString(scParameters), volumeType)
		debugLogf("*csiProvisioner:\"%s\"*volType:\"%s\"*Parameters:\"%+v\"*Capacty:\"%s\"",
			ebsCsiDriverPrivisioner, volumeType, scParameters, validCapacity)
	}
}
