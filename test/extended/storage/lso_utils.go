package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Define the localStorageOperator struct
type localStorageOperator struct {
	subName          string
	namespace        string
	channel          string
	source           string
	deployTemplate   string
	currentCSV       string
	currentIteration int
	maxIteration     int
}

// function option mode to change the default values of lso attributes
type lsoOption func(*localStorageOperator)

// Replace the default value of lso subscription name
func setLsoSubName(subName string) lsoOption {
	return func(lso *localStorageOperator) {
		lso.subName = subName
	}
}

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

// Create a new customized lso object
func newLso(opts ...lsoOption) localStorageOperator {
	defaultLso := localStorageOperator{
		subName:          "lso-sub-" + getRandomString(),
		namespace:        "",
		channel:          "4.11",
		source:           "qe-app-registry",
		deployTemplate:   "/lso/lso-deploy-template.yaml",
		currentIteration: 1,
		maxIteration:     3,
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
	err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", lso.deployTemplate, "-p", "SUBNAME="+lso.subName, "NAMESPACE="+lso.namespace, "CHANNEL="+lso.channel,
		"SOURCE="+lso.source)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Get openshift local storage operator currentCSV
func (lso *localStorageOperator) getCurrentCSV(oc *exutil.CLI) string {
	var (
		currentCSV string
		errinfo    error
	)
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		currentCSV, errinfo = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lso.namespace, "Subscription", `-o=jsonpath={.items[?(@.metadata.name=="`+lso.subName+`")].status.currentCSV}`).Output()
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
	if err != nil {
		describeSubscription, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", lso.namespace, "Subscription/"+lso.subName).Output()
		e2e.Logf("The openshift local storage operator Subscription detail info is:\n \"%s\"", describeSubscription)
		// Temporarily avoid known issue https://issues.redhat.com/browse/OCPBUGS-19046
		// TODO: Revert the commit when OCPBUGS-19046 fixed
		if matched, _ := regexp.MatchString("clusterserviceversion local-storage-operator.*exists and is not referenced by a subscription", describeSubscription); matched && lso.currentIteration < lso.maxIteration {
			lso.currentIteration = lso.currentIteration + 1
			lsoCsv, getCsvErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "packagemanifests.packages.operators.coreos.com", "-l", "catalog="+lso.source, `-o=jsonpath={.items[?(@.metadata.name=="local-storage-operator")].status.channels[0].currentCSV}`).Output()
			o.Expect(getCsvErr).NotTo(o.HaveOccurred(), "Failed to get LSO csv from packagemanifests")
			deleteSpecifiedResource(oc, "sub", lso.subName, lso.namespace)
			deleteSpecifiedResource(oc, "csv", lsoCsv, lso.namespace)
			lso.install(oc)
			return lso.getCurrentCSV(oc)
		}
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Get local storage operator currentCSV in ns/%s timeout", lso.namespace))
	lso.currentCSV = currentCSV
	return currentCSV
}

// Check whether the local storage operator packagemanifests exist in cluster catalogs
func (lso *localStorageOperator) checkPackagemanifestsExistInClusterCatalogs(oc *exutil.CLI) {
	catalogs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "packagemanifests.packages.operators.coreos.com", "-o=jsonpath={.items[?(@.metadata.name==\"local-storage-operator\")].metadata.labels.catalog}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	catalogsList := strings.Fields(catalogs)
	// Check whether the preview catalogsource exists
	isPreviewExist, index := exutil.StringsSliceElementsHasPrefix(catalogsList, "oo-", true)
	switch {
	case catalogs == "":
		g.Skip("Skipped: Local storage Operator packagemanifests not exist in cluster catalogs")
	case isPreviewExist: // Used for lso presubmit test jobs
		lso.source = catalogsList[index]
		lso.channel = "preview"
	case strings.Contains(catalogs, autoReleaseCatalogSource):
		lso.source = autoReleaseCatalogSource
	case strings.Contains(catalogs, qeCatalogSource):
		lso.source = qeCatalogSource
	case strings.Contains(catalogs, redhatCatalogSource):
		lso.source = redhatCatalogSource
	default:
		lso.source = catalogsList[0]
	}
	e2e.Logf(`Local storage Operator exist in "catalogs: %s", use "channel: %s", "source: %s" start test`, catalogs, lso.channel, lso.source)
}

// Check openshift local storage operator install succeed
func (lso *localStorageOperator) checkInstallSucceed(oc *exutil.CLI) (bool, error) {
	lsoCSVinfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lso.namespace, "csv/"+lso.currentCSV, "-o", "json").Output()
	if err != nil {
		e2e.Logf("Check openshift local storage operator install phase failed : \"%v\".", err)
		return false, nil
	}
	if gjson.Get(lsoCSVinfo, `status.phase`).String() == "Succeeded" && gjson.Get(lsoCSVinfo, `status.reason`).String() == "InstallSucceeded" {
		e2e.Logf("openshift local storage operator:\"%s\" In channel:\"%s\" install succeed in ns/%s", lso.currentCSV, lso.channel, lso.namespace)
		return true, nil
	}
	return false, nil
}

// Waiting for openshift local storage operator install succeed
func (lso *localStorageOperator) waitInstallSucceed(oc *exutil.CLI) {
	lso.getCurrentCSV(oc)
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		return lso.checkInstallSucceed(oc)
	})
	if err != nil {
		e2e.Logf("LSO *%s* install failed", lso.currentCSV)
		getOcDescribeInfo(oc, lso.namespace, "csv", lso.currentCSV)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for local storage operator:\"%s\" install succeed in ns/%s timeout", lso.currentCSV, lso.namespace))
}

// Uninstall specified openshift local storage operator
func (lso *localStorageOperator) uninstall(oc *exutil.CLI) error {
	var (
		err           error
		errs          []error
		resourceTypes = []string{"localvolume", "localvolumeset", "localvolumediscovery", "deployment", "ds", "pod", "pvc"}
	)
	for _, resourceType := range resourceTypes {
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", lso.namespace, resourceType, "--all", "--ignore-not-found").Execute()
		if err != nil {
			e2e.Logf("Clean \"%s\" resources failed of %v", resourceType, err)
			errs = append(errs, err)
		}
	}
	o.Expect(errs).Should(o.HaveLen(0))
	e2e.Logf("LSO uninstall Succeed")
	return nil
}

// Get the diskmaker-manager log content
func (lso *localStorageOperator) getDiskManagerLoginfo(oc *exutil.CLI, extraParameters ...string) (string, error) {
	cmdArgs := []string{"-n", lso.namespace, "-l", "app=diskmaker-manager", "-c", "diskmaker-manager"}
	cmdArgs = append(cmdArgs, extraParameters...)
	return oc.AsAdmin().WithoutNamespace().Run("logs").Args(cmdArgs...).Output()
}

// Check diskmaker-manager log contains specified content
func (lso *localStorageOperator) checkDiskManagerLogContains(oc *exutil.CLI, expectedContent string, checkFlag bool) {
	logContent, err := lso.getDiskManagerLoginfo(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	if os.Getenv("STORAGE_LOG_LEVEL") == "DEBUG" {
		path := filepath.Join(e2e.TestContext.OutputDir, "lso-diskmaker-manager-log-"+getRandomString()+".log")
		ioutil.WriteFile(path, []byte(logContent), 0644)
		debugLogf("The diskmaker-manager log is %s", path)
	}
	o.Expect(strings.Contains(logContent, expectedContent)).Should(o.Equal(checkFlag))
}

// Define LocalVolume CR
type localVolume struct {
	name       string
	namespace  string
	deviceID   string
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

// Replace the default value of localVolume deviceID
func setLvDeviceID(deviceID string) localVolumeOption {
	return func(lv *localVolume) {
		lv.deviceID = deviceID
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

// Create a new customized localVolume object
func newLocalVolume(opts ...localVolumeOption) localVolume {
	defaultLocalVolume := localVolume{
		name:       "lv-" + getRandomString(),
		namespace:  "",
		deviceID:   "",
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
	o.Expect(lv.deviceID).NotTo(o.BeEmpty())
	if lv.namespace == "" {
		lv.namespace = oc.Namespace()
	}
	var deletePaths = make([]string, 0, 5)
	if lv.volumeMode == "Block" {
		deletePaths = []string{`items.0.spec.storageClassDevices.0.fsType`}
	}
	err := applyResourceFromTemplateDeleteParametersAsAdmin(oc, deletePaths, "--ignore-unknown-parameters=true", "-f", lv.template, "-p", "NAME="+lv.name, "NAMESPACE="+lv.namespace, "DEVICEID="+lv.deviceID,
		"FSTYPE="+lv.fsType, "SCNAME="+lv.scname, "VOLUMEMODE="+lv.volumeMode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create localVolume CR with extra parameters
func (lv *localVolume) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	o.Expect(lv.deviceID).NotTo(o.BeEmpty())
	if lv.namespace == "" {
		lv.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", lv.template, "-p", "NAME="+lv.name, "NAMESPACE="+lv.namespace, "DEVICEID="+lv.deviceID,
		"FSTYPE="+lv.fsType, "SCNAME="+lv.scname, "VOLUMEMODE="+lv.volumeMode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete localVolume CR
func (lv *localVolume) deleteAsAdmin(oc *exutil.CLI) {
	lvPvs, _ := getPvNamesOfSpecifiedSc(oc, lv.scname)
	// Delete the localvolume CR
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("localvolume/"+lv.name, "-n", lv.namespace, "--ignore-not-found").Execute()
	for _, pv := range lvPvs {
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv/"+pv, "--ignore-not-found").Execute()
	}
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("sc/"+lv.scname, "--ignore-not-found").Execute()
	command := "rm -rf /mnt/local-storage/" + lv.scname
	workers := getWorkersList(oc)
	for _, worker := range workers {
		execCommandInSpecificNode(oc, worker, command)
	}
}

// Waiting for the localVolume CR become "Available"
func (lv *localVolume) waitAvailable(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		availableFlag, errinfo := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lv.namespace, "localvolume/"+lv.name, "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}").Output()
		if errinfo != nil {
			e2e.Logf("Failed to get LV status: %v, wait for next round to get.", errinfo)
			return false, nil
		}
		if availableFlag == "True" {
			e2e.Logf("The localVolume \"%s\" have already become available to use", lv.name)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", lv.namespace, "localvolume/"+lv.name).Output()
		debugLogf("***$ oc describe localVolume/%s\n***%s", lv.name, output)
		output, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", lv.namespace, "-l", "app=diskmaker-manager", "-c", "diskmaker-manager", "--tail=100").Output()
		e2e.Logf("***$ oc logs -l app=diskmaker-manager -c diskmaker-manager --tail=100\n***%s", output)
		e2e.Logf("**************************************************************************")
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for the localVolume \"%s\" become available to use timeout", lv.name))
}

// Define LocalVolumeSet CR
type localVolumeSet struct {
	name           string
	namespace      string
	fsType         string
	maxDeviceCount int64
	scname         string
	volumeMode     string
	template       string
}

// function option mode to change the default values of localVolumeSet attributes
type localVolumeSetOption func(*localVolumeSet)

// Replace the default value of localVolumeSet name
func setLvsName(name string) localVolumeSetOption {
	return func(lvs *localVolumeSet) {
		lvs.name = name
	}
}

// Replace the default value of localVolumeSet namespace
func setLvsNamespace(namespace string) localVolumeSetOption {
	return func(lvs *localVolumeSet) {
		lvs.namespace = namespace
	}
}

// Replace the default value of localVolumeSet storageclass name
func setLvsScname(scname string) localVolumeSetOption {
	return func(lvs *localVolumeSet) {
		lvs.scname = scname
	}
}

// Replace the default value of localVolumeSet fsType
func setLvsFstype(fsType string) localVolumeSetOption {
	return func(lvs *localVolumeSet) {
		lvs.fsType = fsType
	}
}

// Replace the default value of localVolumeSet maxDeviceCount
func setLvsMaxDeviceCount(maxDeviceCount int64) localVolumeSetOption {
	return func(lvs *localVolumeSet) {
		lvs.maxDeviceCount = maxDeviceCount
	}
}

// Replace the default value of localVolumeSet volumeMode
func setLvsVolumeMode(volumeMode string) localVolumeSetOption {
	return func(lvs *localVolumeSet) {
		lvs.volumeMode = volumeMode
	}
}

// Replace the default value of localVolumeSet template
func setLvsTemplate(template string) localVolumeSetOption {
	return func(lvs *localVolumeSet) {
		lvs.template = template
	}
}

// Create a new customized localVolumeSet object
func newLocalVolumeSet(opts ...localVolumeSetOption) localVolumeSet {
	defaultLocalVolumeSet := localVolumeSet{
		name:           "lvs-" + getRandomString(),
		namespace:      "",
		fsType:         "ext4",
		maxDeviceCount: 10,
		scname:         "lvs-sc-" + getRandomString(),
		volumeMode:     "Filesystem",
		template:       "/lso/localvolumeset-template.yaml",
	}
	for _, o := range opts {
		o(&defaultLocalVolumeSet)
	}
	return defaultLocalVolumeSet
}

// Create localVolumeSet CR
func (lvs *localVolumeSet) create(oc *exutil.CLI) {
	if lvs.namespace == "" {
		lvs.namespace = oc.Namespace()
	}
	var deletePaths = make([]string, 0, 5)
	if lvs.volumeMode == "Block" {
		deletePaths = []string{`items.0.spec.storageClassDevices.0.fsType`}
	}
	err := applyResourceFromTemplateDeleteParametersAsAdmin(oc, deletePaths, "--ignore-unknown-parameters=true", "-f", lvs.template, "-p", "NAME="+lvs.name, "NAMESPACE="+lvs.namespace,
		"FSTYPE="+lvs.fsType, "MAXDEVICECOUNT="+strconv.FormatInt(lvs.maxDeviceCount, 10), "SCNAME="+lvs.scname, "VOLUMEMODE="+lvs.volumeMode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create localVolumeSet CR with extra parameters
func (lvs *localVolumeSet) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if lvs.namespace == "" {
		lvs.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", lvs.template, "-p", "NAME="+lvs.name, "NAMESPACE="+lvs.namespace,
		"FSTYPE="+lvs.fsType, "MAXDEVICECOUNT="+strconv.FormatInt(lvs.maxDeviceCount, 10), "SCNAME="+lvs.scname, "VOLUMEMODE="+lvs.volumeMode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create localVolumeSet CR with extra parameters
func (lvs *localVolumeSet) createWithSpecifiedDeviceTypes(oc *exutil.CLI, specifiedDeviceTypes []string) {
	if lvs.namespace == "" {
		lvs.namespace = oc.Namespace()
	}
	specifiedDeviceTypesExtraParameters := map[string]interface{}{
		"jsonPath":    `items.0.spec.deviceInclusionSpec.`,
		"deviceTypes": specifiedDeviceTypes,
	}
	lvs.createWithExtraParameters(oc, specifiedDeviceTypesExtraParameters)
}

// Delete localVolumeSet CR
func (lvs *localVolumeSet) deleteAsAdmin(oc *exutil.CLI) {
	lvsPvs, _ := getPvNamesOfSpecifiedSc(oc, lvs.scname)
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("localvolumeSet/"+lvs.name, "-n", lvs.namespace, "--ignore-not-found").Execute()
	for _, pv := range lvsPvs {
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv/"+pv, "--ignore-not-found").Execute()
	}
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("sc/"+lvs.scname, "--ignore-not-found").Execute()
	command := "rm -rf /mnt/local-storage/" + lvs.scname
	workers := getWorkersList(oc)
	for _, worker := range workers {
		execCommandInSpecificNode(oc, worker, command)
	}
}

// Get the localVolumeSet CR totalProvisionedDeviceCount
func (lvs *localVolumeSet) getTotalProvisionedDeviceCount(oc *exutil.CLI) (int64, error) {
	var (
		output                 string
		provisionedDeviceCount int64
		err                    error
	)
	output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lvs.namespace, "localvolumeSet/"+lvs.name, "-o=jsonpath={.status.totalProvisionedDeviceCount}").Output()
	if err == nil {
		provisionedDeviceCount, err = strconv.ParseInt(output, 10, 64)
		if err != nil {
			e2e.Logf("The localVolumeSet CR totalProvisionedDeviceCount is: \"%d\"", provisionedDeviceCount)
		}
	}
	return provisionedDeviceCount, err
}

// Poll get the localVolumeSet CR totalProvisionedDeviceCount returns func() satisfy the Eventually assert
func (lvs *localVolumeSet) pollGetTotalProvisionedDeviceCount(oc *exutil.CLI) func() int64 {
	return func() int64 {
		provisionedDeviceCount, _ := lvs.getTotalProvisionedDeviceCount(oc)
		return provisionedDeviceCount
	}
}

// Waiting for the localVolumeSet CR have already provisioned Device
func (lvs *localVolumeSet) waitDeviceProvisioned(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		provisionedDeviceCount, errinfo := lvs.getTotalProvisionedDeviceCount(oc)
		if errinfo != nil {
			e2e.Logf("Get LVS provisionedDeviceCount failed :%v, wait for trying next round get.", errinfo)
			return false, nil
		}
		if provisionedDeviceCount > 0 {
			e2e.Logf("The localVolumeSet \"%s\" have already provisioned Device [provisionedDeviceCount: %d]", lvs.name, provisionedDeviceCount)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", lvs.namespace, "localvolumeSet/"+lvs.name).Output()
		debugLogf("***$ oc describe localVolumeSet/%s\n***%s", lvs.name, output)
		output, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", lvs.namespace, "-l", "app=diskmaker-manager", "-c", "diskmaker-manager", "--tail=100").Output()
		e2e.Logf("***$ oc logs -l app=diskmaker-manager -c diskmaker-manager --tail=100\n***%s", output)
		e2e.Logf("**************************************************************************")
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for the localVolumeSet \"%s\" have already provisioned Device timeout", lvs.name))
}

// Define the localVolumeDiscovery struct
type localVolumeDiscovery struct {
	name             string
	namespace        string
	template         string
	discoverNodes    []string
	discoveryResults map[string]string
}

// function option mode to change the default values of localVolumeDiscovery attributes
type localVolumeDiscoveryOption func(*localVolumeDiscovery)

// Replace the default value of localVolumeDiscovery name
func setLvdName(name string) localVolumeDiscoveryOption {
	return func(lvd *localVolumeDiscovery) {
		lvd.name = name
	}
}

// Replace the default value of localVolumeDiscovery namespace
func setLvdNamespace(namespace string) localVolumeDiscoveryOption {
	return func(lvd *localVolumeDiscovery) {
		lvd.namespace = namespace
	}
}

// Replace the default value of localVolumeDiscovery discoverNodes
func setLvdDiscoverNodes(discoverNodes []string) localVolumeDiscoveryOption {
	return func(lvd *localVolumeDiscovery) {
		lvd.discoverNodes = discoverNodes
	}
}

// Replace the default value of localVolumeDiscovery template
func setLvdTemplate(template string) localVolumeDiscoveryOption {
	return func(lvd *localVolumeDiscovery) {
		lvd.template = template
	}
}

// Create a new customized localVolumeDiscovery object
func newlocalVolumeDiscovery(opts ...localVolumeDiscoveryOption) localVolumeDiscovery {
	initDiscoverResults := make(map[string]string, 10)
	defaultlocalVolumeDiscovery := localVolumeDiscovery{
		// The LocalVolumeDiscovery "autodetect-a" is invalid: metadata.name: Unsupported value: "autodetect-a": supported values: "auto-discover-devices"
		// TODO: Seems CR name must be "auto-discover-devices" will double check the code later
		name:             "auto-discover-devices",
		namespace:        "",
		discoverNodes:    []string{},
		discoveryResults: initDiscoverResults,
		template:         "/lso/localvolumediscovery-template.yaml",
	}
	for _, o := range opts {
		o(&defaultlocalVolumeDiscovery)
	}
	return defaultlocalVolumeDiscovery
}

// Create localVolumeDiscovery CR
func (lvd *localVolumeDiscovery) create(oc *exutil.CLI) {
	if lvd.namespace == "" {
		lvd.namespace = oc.Namespace()
	}
	if len(lvd.discoverNodes) > 0 {
		lvd.ApplyWithSpecificNodes(oc, `kubernetes.io/hostname`, "In", lvd.discoverNodes)
	} else {
		err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", lvd.template, "-p", "NAME="+lvd.name, "NAMESPACE="+lvd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Create localVolumeDiscovery CR with extra parameters
func (lvd *localVolumeDiscovery) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) {
	if lvd.namespace == "" {
		lvd.namespace = oc.Namespace()
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", lvd.template, "-p", "NAME="+lvd.name, "NAMESPACE="+lvd.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create localVolumeDiscovery CR with specific nodes
func (lvd *localVolumeDiscovery) ApplyWithSpecificNodes(oc *exutil.CLI, filterKey string, filterOperator string, filterValues []string) {
	if lvd.namespace == "" {
		lvd.namespace = oc.Namespace()
	}
	extraParameters := map[string]interface{}{
		"jsonPath": `items.0.spec.nodeSelector.nodeSelectorTerms.0.matchExpressions.0.`,
		"key":      filterKey,
		"operator": filterOperator,
		"values":   filterValues,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", lvd.template, "-p", "NAME="+lvd.name, "NAMESPACE="+lvd.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete localVolumeDiscovery CR
func (lvd *localVolumeDiscovery) deleteAsAdmin(oc *exutil.CLI) {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("localVolumeDiscovery/"+lvd.name, "-n", lvd.namespace, "--ignore-not-found").Execute()
}

func (lvd *localVolumeDiscovery) waitDiscoveryAvailable(oc *exutil.CLI) {
	err := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		lvdAvailableStatus, getLvdStatusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lvd.namespace, "localVolumeDiscovery/"+lvd.name, "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}").Output()
		if getLvdStatusErr != nil {
			e2e.Logf("Failed to get localvolumediscovery: %v", getLvdStatusErr)
			return false, getLvdStatusErr
		}
		if lvdAvailableStatus == "True" {
			e2e.Logf("Localvolumediscovery is Available now")
			return true, nil
		}
		e2e.Logf("Localvolumediscovery status is still \"%s\" try the next round", lvdAvailableStatus)
		return false, nil
	})
	if err != nil {
		getOcDescribeInfo(oc.AsAdmin(), lvd.namespace, "localVolumeDiscovery", "auto-discover-devices")
		diskmakerDiscoveryLogs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", lvd.namespace, "-l", "app=diskmaker-discovery", "-c", "diskmaker-discovery", "--tail=100").Output()
		e2e.Logf("***$ oc logs -l app=diskmaker-discovery -c diskmaker-discovery --tail=100***\n%s", diskmakerDiscoveryLogs)
		e2e.Logf("**************************************************************************")
	}
	exutil.AssertWaitPollNoErr(err, "Wait Localvolumediscovery become Available timeout")
}

// Create localVolumeDiscovery CR with specific nodes
func (lvd *localVolumeDiscovery) waitDiscoveryResultsGenerated(oc *exutil.CLI) {
	lvd.waitDiscoveryAvailable(oc)
	lvd.syncDiscoveryResults(oc)
}

// Get localVolumeDiscoveryResults from specified node
func (lvd *localVolumeDiscovery) getSpecifiedNodeDiscoveryResults(oc *exutil.CLI, nodeName string) (nodeVolumeDiscoveryResults string) {
	var getDiscoveryResultsErr error
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		nodeVolumeDiscoveryResults, getDiscoveryResultsErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", lvd.namespace, "localVolumeDiscoveryResults/discovery-result-"+nodeName, "-o", "json", "--ignore-not-found").Output()
		if getDiscoveryResultsErr != nil {
			e2e.Logf("Failed to get node \"%s\" volume discoveryResults: %v", nodeName, getDiscoveryResultsErr)
			return false, getDiscoveryResultsErr
		}
		if nodeVolumeDiscoveryResults != "" {
			e2e.Logf("Get Node \"%s\" volume discoveryResults succeed", nodeName)
			debugLogf("The node/%s volume discoveryResults is\n %s", nodeName, nodeVolumeDiscoveryResults)
			return true, nil
		}
		e2e.Logf("Get node \"%s\" volume discoveryResults is empty try the next round", nodeName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Get node \"%s\" volume discoveryResults timeout", nodeName))
	return nodeVolumeDiscoveryResults
}

// Create localVolumeDiscovery CR with specific nodes
func (lvd *localVolumeDiscovery) syncDiscoveryResults(oc *exutil.CLI) {
	for _, discoverNode := range lvd.discoverNodes {
		lvd.discoveryResults[discoverNode] = lvd.getSpecifiedNodeDiscoveryResults(oc, discoverNode)
	}
	e2e.Logf("DiscoveryResults Sync Succeed")
}

// waitSpecifiedDeviceStatusAsExpected waits specified device status become expected status
func (lvd *localVolumeDiscovery) waitSpecifiedDeviceStatusAsExpected(oc *exutil.CLI, nodeName string, devicePath string, expectedStatus string) {
	var deviceStatus, nodeDevicesDiscoverResults string
	err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
		nodeDevicesDiscoverResults = lvd.getSpecifiedNodeDiscoveryResults(oc, nodeName)
		if strings.HasPrefix(devicePath, "/dev/disk/by-id") {
			deviceStatus = gjson.Get(nodeDevicesDiscoverResults, `status.discoveredDevices.#(deviceID==`+devicePath+`).status.state`).String()
		} else {
			deviceStatus = gjson.Get(nodeDevicesDiscoverResults, `status.discoveredDevices.#(path==`+devicePath+`).status.state`).String()
		}
		debugLogf(`Device: "%s" on node/%s status is "%s" now`, devicePath, nodeName, deviceStatus)
		return strings.EqualFold(deviceStatus, expectedStatus), nil
	})
	if err != nil {
		e2e.Logf(`Node/%s's LocalVolumeDiscoveryResult is`, nodeName, nodeDevicesDiscoverResults)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf(`Waitting for device: "%s" on node/%s become "%s" timeout, last actual status is "%s".`, devicePath, nodeName, expectedStatus, deviceStatus))
}
