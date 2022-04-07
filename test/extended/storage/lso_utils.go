package storage

import (
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	qeCatalogSource     = "qe-app-registry"
	redhatCatalogSource = "redhat-operators"
	sourceNameSpace     = "openshift-marketplace"
)

// Define the localStorageOperator struct
type localStorageOperator struct {
	namespace      string
	channel        string
	source         string
	deployTemplate string
	currentCSV     string
}

// function option mode to change the default values of lso attributes
type lsoOption func(*localStorageOperator)

// Replace the default value of lso namespace
func setLsoNamespace(namespace string) lsoOption {
	return func(lso *localStorageOperator) {
		lso.namespace = namespace
	}
}

// Replace the default value of lso channel
func setLsoChannel(channel string) lsoOption {
	return func(lso *localStorageOperator) {
		lso.channel = channel
	}
}

// Replace the default value of lso source
func setLsoSource(source string) lsoOption {
	return func(lso *localStorageOperator) {
		lso.source = source
	}
}

// Replace the default value of lso deployTemplate
func setLsoTemplate(deployTemplate string) lsoOption {
	return func(lso *localStorageOperator) {
		lso.deployTemplate = deployTemplate
	}
}

//  Create a new customized lso object
func newLso(opts ...lsoOption) localStorageOperator {
	defaultLso := localStorageOperator{
		// namespace:      "local-storage-" + getRandomString(),
		namespace:      "",
		channel:        "4.11",
		source:         "qe-app-registry",
		deployTemplate: "/lso/lso-deploy-template.yaml",
	}
	for _, o := range opts {
		o(&defaultLso)
	}
	return defaultLso
}

// Install openshift local storage operator
func (lso *localStorageOperator) install(oc *exutil.CLI) {
	if lso.namespace == "" {
		lso.namespace = oc.Namespace()
	}
	// err := oc.AsAdmin().WithoutNamespace().Run("new-project").Args(lso.namespace).Execute()
	// o.Expect(err).NotTo(o.HaveOccurred())
	err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", lso.deployTemplate, "-p", "NAMESPACE="+lso.namespace, "CHANNEL="+lso.channel,
		"SOURCE="+lso.source)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Get openshift local storage operator currentCSV
func (lso *localStorageOperator) getCurrentCSV(oc *exutil.CLI) string {
	var (
		currentCSV string
		errinfo    error
	)
	err := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		currentCSV, errinfo = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lso.namespace, "Subscription", "-o=jsonpath={.items[?(@.metadata.name==\"local-storage-operator\")].status.currentCSV}").Output()
		if errinfo != nil {
			e2e.Logf("Get local storage operator currentCSV failed :%v, wait for next round get.", errinfo)
			return false, errinfo
		}
		if currentCSV != "" {
			e2e.Logf("The openshift local storage operator currentCSV is: \"%s\"", currentCSV)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Get local storage operator currentCSV in ns/%s timeout", lso.namespace))
	lso.currentCSV = currentCSV
	return currentCSV
}

// Check the cluster CatalogSource, use qeCatalogSource first
// If qeCatalogSource not exist check the redhatCatalogSource
// If both qeCatalogSource and redhatCatalogSource not exist skip the test
func (lso *localStorageOperator) checkClusterCatalogSource(oc *exutil.CLI) error {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sourceNameSpace, "catalogsource/"+qeCatalogSource).Output()
	if err != nil {
		if strings.Contains(output, "not found") {
			output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sourceNameSpace, "catalogsource/"+redhatCatalogSource).Output()
			if err != nil {
				if strings.Contains(output, "not found") {
					g.Skip("Skip for both qeCatalogSource and redhatCatalogSource don't exist !!!")
					return nil
				} else {
					e2e.Logf("Get redhatCatalogSource failed of: \"%v\"", err)
					return err
				}
			} else {
				lso.source = redhatCatalogSource
				lso.channel = "stable"
				e2e.Logf("Since qeCatalogSource doesn't exist, use offical: \"%s:%s\" instead", lso.source, lso.channel)
				return nil
			}
		} else {
			e2e.Logf("Get qeCatalogSource failed of: \"%v\"", err)
			return err
		}
	}
	e2e.Logf("qeCatalogSource exist, use qe catalogsource: \"%s:%s\" start test", lso.source, lso.channel)
	return nil
}

// Check openshift local storage operator install succeed
func (lso *localStorageOperator) checkInstallSucceed(oc *exutil.CLI) (bool, error) {
	lsoCSVinfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lso.namespace, "csv/"+lso.currentCSV, "-o", "json").Output()
	if err != nil {
		e2e.Logf("Check openshift local storage operator install phase failed : \"%v\".", err)
		return false, nil
	}
	if gjson.Get(lsoCSVinfo, `status.phase`).String() == "Succeeded" && gjson.Get(lsoCSVinfo, `status.reason`).String() == "InstallSucceeded" {
		e2e.Logf("openshift local storage operator:\"%s\" install succeed in ns/%s", lso.currentCSV, lso.namespace)
		return true, nil
	} else {
		return false, nil
	}
}

// Waiting for openshift local storage operator install succeed
func (lso *localStorageOperator) waitInstallSucceed(oc *exutil.CLI) {
	lso.getCurrentCSV(oc)
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		return lso.checkInstallSucceed(oc)
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for local storage operator:\"%s\" install succeed in ns/%s timeout", lso.currentCSV, lso.namespace))
}

// Uninstall specified openshift local storage operator
func (lso *localStorageOperator) uninstall(oc *exutil.CLI) error {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("localvolume", "--all", "-n", lso.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("localvolumeset", "--all", "-n", lso.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("localvolumediscovery", "--all", "-n", lso.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment/"+"local-storage-operator", "-n", lso.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("ds/"+"diskmaker-manager", "-n", lso.namespace).Execute()
	debugLogf("LSO uninstall Succeed")
	return nil
}

// Define LocalVolume CR
type localVolume struct {
	name       string
	namespace  string
	deviceId   string
	fsType     string
	scname     string
	volumeMode string
	template   string
}

// function option mode to change the default values of localVolume attributes
type localVolumeOption func(*localVolume)

// Replace the default value of localVolume name
func setLvName(name string) localVolumeOption {
	return func(lv *localVolume) {
		lv.name = name
	}
}

// Replace the default value of localVolume namespace
func setLvNamespace(namespace string) localVolumeOption {
	return func(lv *localVolume) {
		lv.namespace = namespace
	}
}

// Replace the default value of localVolume deviceId
func setLvDeviceId(deviceId string) localVolumeOption {
	return func(lv *localVolume) {
		lv.deviceId = deviceId
	}
}

// Replace the default value of localVolume scname
func setLvScname(scname string) localVolumeOption {
	return func(lv *localVolume) {
		lv.scname = scname
	}
}

// Replace the default value of localVolume volumeMode
func setLvVolumeMode(volumeMode string) localVolumeOption {
	return func(lv *localVolume) {
		lv.volumeMode = volumeMode
	}
}

// Replace the default value of localVolume fsType
func setLvFstype(fsType string) localVolumeOption {
	return func(lv *localVolume) {
		lv.fsType = fsType
	}
}

// Replace the default value of localVolume template
func setLvTemplate(template string) localVolumeOption {
	return func(lv *localVolume) {
		lv.template = template
	}
}

//  Create a new customized localVolume object
func newLocalVolume(opts ...localVolumeOption) localVolume {
	defaultLocalVolume := localVolume{
		name:       "lv-" + getRandomString(),
		namespace:  "",
		deviceId:   "",
		fsType:     "ext4",
		scname:     "lvsc-" + getRandomString(),
		volumeMode: "Filesystem",
		template:   "/lso/localvolume-template.yaml",
	}
	for _, o := range opts {
		o(&defaultLocalVolume)
	}
	return defaultLocalVolume
}

// Create localVolume CR
func (lv *localVolume) create(oc *exutil.CLI) {
	var deletePaths = make([]string, 0, 5)
	if lv.volumeMode == "Block" {
		deletePaths = []string{`items.0.spec.storageClassDevices.0.fsType`}
	}
	err := applyResourceFromTemplateDeleteParametersAsAdmin(oc, deletePaths, "--ignore-unknown-parameters=true", "-f", lv.template, "-p", "NAME="+lv.name, "NAMESPACE="+lv.namespace, "DEVICEID="+lv.deviceId,
		"FSTYPE="+lv.fsType, "SCNAME="+lv.scname, "VOLUMEMODE="+lv.volumeMode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create localVolume CR with extra parameters
func (lv *localVolume) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", lv.template, "-p", "NAME="+lv.name, "NAMESPACE="+lv.namespace, "DEVICEID="+lv.deviceId,
		"FSTYPE="+lv.fsType, "SCNAME="+lv.scname, "VOLUMEMODE="+lv.volumeMode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete localVolume CR
func (lv *localVolume) deleteAsAdmin(oc *exutil.CLI) {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("sc/" + lv.scname).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("localvolume/"+lv.name, "-n", lv.namespace).Execute()
	lvPvs, _ := getPvNamesOfSpecifiedSc(oc, lv.scname)
	for _, pv := range lvPvs {
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv/" + pv).Execute()
	}
	command := "rm -rf /mnt/local-storage/" + lv.scname
	workers := getWorkersList(oc)
	for _, worker := range workers {
		execCommandInSpecificNode(oc, worker, command)
	}
}
