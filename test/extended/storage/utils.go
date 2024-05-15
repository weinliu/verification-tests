package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Define the global cloudProvider
var cloudProvider, provisioner string

// Define test waiting time const
const (
	defaultMaxWaitingTime    = 300 * time.Second
	defaultIterationTimes    = 20
	longerMaxWaitingTime     = 15 * time.Minute
	moreLongerMaxWaitingTime = 30 * time.Minute
	longestMaxWaitingTime    = 1 * time.Hour
)

// Kubeadmin user use oc client apply yaml template
func applyResourceFromTemplateAsAdmin(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	if isCRDSpecificFieldExist(oc, "template.apiVersion") {
		err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "config.json")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			configFile = output
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))
	} else {
		configFile = parameterizedTemplateByReplaceToFile(oc, parameters...)
	}

	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

// Common user use oc client apply yaml template
func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	if isCRDSpecificFieldExist(oc, "template.apiVersion") {
		err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
			output, err := oc.Run("process").Args(parameters...).OutputToFile(getRandomString() + "config.json")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			configFile = output
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))
	} else {
		configFile = parameterizedTemplateByReplaceToFile(oc, parameters...)
	}
	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

// Common user use oc client apply yaml template and return output
func applyResourceFromTemplateWithOutput(oc *exutil.CLI, parameters ...string) (string, error) {
	var configFile string
	if isCRDSpecificFieldExist(oc, "template.apiVersion") {
		err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
			output, err := oc.Run("process").Args(parameters...).OutputToFile(getRandomString() + "config.json")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			configFile = output
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))
	} else {
		configFile = parameterizedTemplateByReplaceToFile(oc, parameters...)
	}

	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Output()
}

// parameterizedTemplateByReplaceToFile parameterize template to new file
func parameterizedTemplateByReplaceToFile(oc *exutil.CLI, parameters ...string) string {
	isParameterExist, pIndex := exutil.StringsSliceElementsHasPrefix(parameters, "-f", true)
	o.Expect(isParameterExist).Should(o.BeTrue())
	templateFileName := parameters[pIndex+1]
	templateContentByte, readFileErr := ioutil.ReadFile(templateFileName)
	o.Expect(readFileErr).ShouldNot(o.HaveOccurred())
	templateContentStr := string(templateContentByte)
	isParameterExist, pIndex = exutil.StringsSliceElementsHasPrefix(parameters, "-p", true)
	o.Expect(isParameterExist).Should(o.BeTrue())
	for i := pIndex + 1; i < len(parameters); i++ {
		if strings.Contains(parameters[i], "=") {
			tempSlice := strings.Split(parameters[i], "=")
			o.Expect(tempSlice).Should(o.HaveLen(2))
			templateContentStr = strings.ReplaceAll(templateContentStr, "${"+tempSlice[0]+"}", tempSlice[1])
		}
	}
	templateContentJSON, convertErr := yaml.YAMLToJSON([]byte(templateContentStr))
	o.Expect(convertErr).NotTo(o.HaveOccurred())
	configFile := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+getRandomString()+"config.json")
	o.Expect(ioutil.WriteFile(configFile, pretty.Pretty(templateContentJSON), 0644)).ShouldNot(o.HaveOccurred())
	return configFile
}

// Get a random string of 8 byte
func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
// Define valid devMap for aws instance ebs volume device "/dev/sd[f-p]"
var devMaps = map[string]bool{"f": false, "g": false, "h": false, "i": false, "j": false,
	"k": false, "l": false, "m": false, "n": false, "o": false, "p": false}

// Get a valid device for EFS volume attach
func getValidDeviceForEbsVol() string {
	var validStr string
	for k, v := range devMaps {
		if !v {
			devMaps[k] = true
			validStr = k
			break
		}
	}
	e2e.Logf("validDevice: \"/dev/sd%s\", devMaps: \"%+v\"", validStr, devMaps)
	return "/dev/sd" + validStr
}

// Get the cloud provider type of the test environment
func getCloudProvider(oc *exutil.CLI) string {
	var (
		errMsg error
		output string
	)
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, errMsg = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if errMsg != nil {
			e2e.Logf("Get cloudProvider *failed with* :\"%v\",wait 5 seconds retry.", errMsg)
			return false, errMsg
		}
		e2e.Logf("The test cluster cloudProvider is :\"%s\".", strings.ToLower(output))
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Waiting for get cloudProvider timeout")
	return strings.ToLower(output)
}

// Get the cluster infrastructureName(ClusterID)
func getClusterID(oc *exutil.CLI) (string, error) {
	clusterID, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
	if err != nil || clusterID == "" {
		e2e.Logf("Get infrastructureName(ClusterID) failed with \"%v\", Or infrastructureName(ClusterID) is null:\"%s\"", err, clusterID)
	} else {
		debugLogf("The infrastructureName(ClusterID) is:\"%s\"", clusterID)
	}
	return clusterID, err
}

// Get the cluster version channel x.x (e.g. 4.11)
func getClusterVersionChannel(oc *exutil.CLI) string {
	// clusterbot env don't have ".spec.channel", So change to use desire version
	clusterVersion, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clusterversion", "-o=jsonpath={.items[?(@.kind==\"ClusterVersion\")].status.desired.version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	tempSlice := strings.Split(clusterVersion, ".")
	clusterVersion = tempSlice[0] + "." + tempSlice[1]
	e2e.Logf("The Cluster version is belong to channel: \"%s\"", clusterVersion)
	return clusterVersion
}

// Strings contain sub string check
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// Strings slice contains duplicate string check
func containsDuplicate(strings []string) bool {
	elemMap := make(map[string]bool)
	for _, value := range strings {
		if _, ok := elemMap[value]; ok {
			return true
		}
		elemMap[value] = true
	}
	return false
}

// Convert interface type to string
func interfaceToString(value interface{}) string {
	var key string
	if value == nil {
		return key
	}

	switch value.(type) {
	case float64:
		ft := value.(float64)
		key = strconv.FormatFloat(ft, 'f', -1, 64)
	case float32:
		ft := value.(float32)
		key = strconv.FormatFloat(float64(ft), 'f', -1, 64)
	case int:
		it := value.(int)
		key = strconv.Itoa(it)
	case uint:
		it := value.(uint)
		key = strconv.Itoa(int(it))
	case int8:
		it := value.(int8)
		key = strconv.Itoa(int(it))
	case uint8:
		it := value.(uint8)
		key = strconv.Itoa(int(it))
	case int16:
		it := value.(int16)
		key = strconv.Itoa(int(it))
	case uint16:
		it := value.(uint16)
		key = strconv.Itoa(int(it))
	case int32:
		it := value.(int32)
		key = strconv.Itoa(int(it))
	case uint32:
		it := value.(uint32)
		key = strconv.Itoa(int(it))
	case int64:
		it := value.(int64)
		key = strconv.FormatInt(it, 10)
	case uint64:
		it := value.(uint64)
		key = strconv.FormatUint(it, 10)
	case string:
		key = value.(string)
	case []byte:
		key = string(value.([]byte))
	default:
		newValue, _ := json.Marshal(value)
		key = string(newValue)
	}

	return key
}

// Json add extra parameters to jsonfile
func jsonAddExtraParametersToFile(jsonInput string, extraParameters map[string]interface{}) (string, error) {
	var (
		jsonPath string
		err      error
	)
	if _, ok := extraParameters["jsonPath"]; !ok && gjson.Get(jsonInput, `items.0`).Exists() {
		jsonPath = `items.0.`
	} else {
		jsonPath = interfaceToString(extraParameters["jsonPath"])
	}
	for extraParametersKey, extraParametersValue := range extraParameters {
		if extraParametersKey != "jsonPath" {
			jsonInput, err = sjson.Set(jsonInput, jsonPath+extraParametersKey, extraParametersValue)
			debugLogf("Process jsonPath: \"%s\" Value: \"%s\"", jsonPath+extraParametersKey, extraParametersValue)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	}
	if cloudProvider == "ibmcloud" && !gjson.Get(jsonInput, `items.0.parameters.profile`).Bool() && strings.EqualFold(gjson.Get(jsonInput, `items.0.kind`).String(), "storageclass") {
		jsonInput, err = sjson.Set(jsonInput, jsonPath+"parameters.profile", "10iops-tier")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	path := filepath.Join(e2e.TestContext.OutputDir, "storageConfig"+"-"+getRandomString()+".json")
	return path, ioutil.WriteFile(path, pretty.Pretty([]byte(jsonInput)), 0644)
}

// Batch process jsonPaths with new values and save to json file
func jsonPathsBatchProcessToFile(jsonInput string, jsonPathsAndActions []map[string]string, multiExtraParameters []map[string]interface{}) (string, error) {
	var err error
	for i := 0; i < len(jsonPathsAndActions); i++ {
		for jsonPath, action := range jsonPathsAndActions[i] {
			switch action {
			case "set":
				for extraParametersKey, extraParametersValue := range multiExtraParameters[i] {
					jsonInput, err = sjson.Set(jsonInput, jsonPath+extraParametersKey, extraParametersValue)
					debugLogf("Process jsonPath: \"%s\" Value: \"%s\"", jsonPath+extraParametersKey, extraParametersValue)
					o.Expect(err).NotTo(o.HaveOccurred())
				}
			case "delete":
				jsonInput, err = sjson.Delete(jsonInput, jsonPath)
				debugLogf("Delete jsonPath: \"%s\"", jsonPath)
				o.Expect(err).NotTo(o.HaveOccurred())
			default:
				e2e.Logf("Unknown JSON process action: \"%s\"", action)
			}
		}
	}
	path := filepath.Join(e2e.TestContext.OutputDir, "storageConfig"+"-"+getRandomString()+".json")
	return path, ioutil.WriteFile(path, pretty.Pretty([]byte(jsonInput)), 0644)
}

// Json delete paths to jsonfile
func jsonDeletePathsToFile(jsonInput string, deletePaths []string) (string, error) {
	var err error
	if len(deletePaths) != 0 {
		for _, path := range deletePaths {
			jsonInput, err = sjson.Delete(jsonInput, path)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	}
	path := filepath.Join(e2e.TestContext.OutputDir, "storageConfig"+"-"+getRandomString()+".json")
	return path, ioutil.WriteFile(path, pretty.Pretty([]byte(jsonInput)), 0644)
}

// Kubeadmin user use oc client apply yaml template delete parameters
func applyResourceFromTemplateDeleteParametersAsAdmin(oc *exutil.CLI, deletePaths []string, parameters ...string) error {
	var configFile, tempJSONOutput string
	if isCRDSpecificFieldExist(oc, "template.apiVersion") {
		err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().Run("process").Args(parameters...).Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			tempJSONOutput = output
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))
	} else {
		tempByte, readErr := ioutil.ReadFile(parameterizedTemplateByReplaceToFile(oc, parameters...))
		o.Expect(readErr).NotTo(o.HaveOccurred())
		tempJSONOutput = string(tempByte)
	}

	configFile, _ = jsonDeletePathsToFile(tempJSONOutput, deletePaths)
	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

// Kubeadmin user use oc client apply yaml template with extra parameters
func applyResourceFromTemplateWithExtraParametersAsAdmin(oc *exutil.CLI, extraParameters map[string]interface{}, parameters ...string) error {
	var configFile, tempJSONOutput string
	if isCRDSpecificFieldExist(oc, "template.apiVersion") {
		err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().Run("process").Args(parameters...).Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			tempJSONOutput = output
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))
	} else {
		tempByte, readErr := ioutil.ReadFile(parameterizedTemplateByReplaceToFile(oc, parameters...))
		o.Expect(readErr).NotTo(o.HaveOccurred())
		tempJSONOutput = string(tempByte)
		// Make methods compatible with microshift test cluster
		if _, ok := extraParameters["jsonPath"]; ok && strings.HasPrefix(fmt.Sprintf("%s", extraParameters["jsonPath"]), "items.0.") {
			extraParameters["jsonPath"] = strings.TrimPrefix(fmt.Sprintf("%s", extraParameters["jsonPath"]), "items.0.")
		}
	}
	configFile, _ = jsonAddExtraParametersToFile(tempJSONOutput, extraParameters)
	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

// Use oc client apply yaml template with multi extra parameters
func applyResourceFromTemplateWithMultiExtraParameters(oc *exutil.CLI, jsonPathsAndActions []map[string]string, multiExtraParameters []map[string]interface{}, parameters ...string) (string, error) {
	var configFile, tempJSONOutput string
	if isCRDSpecificFieldExist(oc, "template.apiVersion") {
		err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().Run("process").Args(parameters...).Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			tempJSONOutput = output
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))
	} else {
		tempByte, readErr := ioutil.ReadFile(parameterizedTemplateByReplaceToFile(oc, parameters...))
		o.Expect(readErr).NotTo(o.HaveOccurred())
		tempJSONOutput = string(tempByte)
	}

	configFile, _ = jsonPathsBatchProcessToFile(tempJSONOutput, jsonPathsAndActions, multiExtraParameters)
	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Output()
}

// None duplicate element slice intersect
func sliceIntersect(slice1, slice2 []string) []string {
	m := make(map[string]int)
	sliceResult := make([]string, 0)
	for _, value1 := range slice1 {
		m[value1]++
	}

	for _, value2 := range slice2 {
		appearTimes := m[value2]
		if appearTimes == 1 {
			sliceResult = append(sliceResult, value2)
		}
	}
	return sliceResult
}

// Convert String Slice to Map: map[string]struct{}
func convertStrSliceToMap(strSlice []string) map[string]struct{} {
	set := make(map[string]struct{}, len(strSlice))
	for _, v := range strSlice {
		set[v] = struct{}{}
	}
	return set
}

// Judge whether the map contains specified key
func isInMap(inputMap map[string]struct{}, inputString string) bool {
	_, ok := inputMap[inputString]
	return ok
}

// Judge whether the String Slice contains specified element, return bool
func strSliceContains(sl []string, element string) bool {
	return isInMap(convertStrSliceToMap(sl), element)
}

// Check if component is listed in clusterversion.status.capabilities.knownCapabilities
func isKnownCapability(oc *exutil.CLI, component string) bool {
	knownCapabilities, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].status.capabilities.knownCapabilities}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Cluster known capability parameters: %v\n", knownCapabilities)
	return strings.Contains(knownCapabilities, component)
}

// Check if component is listed in clusterversion.status.capabilities.enabledCapabilities
func isEnabledCapability(oc *exutil.CLI, component string) bool {
	enabledCapabilities, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].status.capabilities.enabledCapabilities}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Cluster enabled capability parameters: %v\n", enabledCapabilities)
	return strings.Contains(enabledCapabilities, component)
}

// Check if component is a disabled capability by knownCapabilities and enabledCapabilities list, skip if it is disabled.
func checkOptionalCapability(oc *exutil.CLI, component string) {
	if isKnownCapability(oc, component) && !isEnabledCapability(oc, component) {
		g.Skip("Skip for " + component + " not enabled optional capability")
	}
}

// Common csi cloud provider support check
func generalCsiSupportCheck(cloudProvider string) {
	generalCsiSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportPlatformsBool := gjson.GetBytes(generalCsiSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+")|@flatten").Exists()
	e2e.Logf("%s * %v * %v", cloudProvider, gjson.GetBytes(generalCsiSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten"), supportPlatformsBool)
	if !supportPlatformsBool {
		g.Skip("Skip for non-supported cloud provider: " + cloudProvider + "!!!")
	}
}

// Common Intree cloud provider support check
func generalIntreeSupportCheck(cloudProvider string) {
	generalIntreeSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-intree-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportPlatformsBool := gjson.GetBytes(generalIntreeSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+")|@flatten").Exists()
	e2e.Logf("%s * %v * %v", cloudProvider, gjson.GetBytes(generalIntreeSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten"), supportPlatformsBool)
	if !supportPlatformsBool {
		g.Skip("Skip for non-supported cloud provider: " + cloudProvider + "!!!")
	}
}

// Get common csi provisioners by cloudplatform
func getSupportProvisionersByCloudProvider(oc *exutil.CLI) []string {
	csiCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportProvisioners := []string{}
	supportProvisionersResult := gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten").Array()
	e2e.Logf("%s support provisioners are : %v", cloudProvider, supportProvisionersResult)
	for i := 0; i < len(supportProvisionersResult); i++ {
		supportProvisioners = append(supportProvisioners, gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten."+strconv.Itoa(i)).String())
	}
	if cloudProvider == "aws" && !checkCSIDriverInstalled(oc, []string{"efs.csi.aws.com"}) || checkFips(oc) {
		supportProvisioners = deleteElement(supportProvisioners, "efs.csi.aws.com")
		e2e.Logf("***%s \"AWS-EFS CSI Driver\" not installed OR it installed in FIPS enabled cluster, updating support provisioners to: %v***", cloudProvider, supportProvisioners)
	}
	if cloudProvider == "gcp" && !checkCSIDriverInstalled(oc, []string{"filestore.csi.storage.gke.io"}) {
		supportProvisioners = deleteElement(supportProvisioners, "filestore.csi.storage.gke.io")
		e2e.Logf("***%s \"GCP File store CSI Driver\" not installed, updating support provisioners to: %v***", cloudProvider, supportProvisioners)
	}
	// AzureStack test clusters don't support azure file storage
	// Ref: https://learn.microsoft.com/en-us/azure-stack/user/azure-stack-acs-differences?view=azs-2108
	if cloudProvider == "azure" && (isAzureStackCluster(oc) || checkFips(oc)) {
		e2e.Logf("***%s \"Azure-file CSI Driver\" don't support AzureStackCluster or FIPS enabled env, updating support provisioners to: %v***", cloudProvider, supportProvisioners)
		supportProvisioners = deleteElement(supportProvisioners, "file.csi.azure.com")
	}
	return supportProvisioners
}

// Get common csi volumetypes by cloudplatform
func getSupportVolumesByCloudProvider() []string {
	csiCommonSupportVolumeMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportVolumes := []string{}
	supportVolumesResult := gjson.GetBytes(csiCommonSupportVolumeMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").volumetypes|@flatten").Array()
	e2e.Logf("%s support volumes are : %v", cloudProvider, supportVolumesResult)
	for i := 0; i < len(supportVolumesResult); i++ {
		supportVolumes = append(supportVolumes, gjson.GetBytes(csiCommonSupportVolumeMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").volumetypes|@flatten."+strconv.Itoa(i)).String())
	}
	return supportVolumes
}

// Get common Intree provisioners by cloudplatform
func getIntreeSupportProvisionersByCloudProvider(oc *exutil.CLI) []string {
	csiCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-intree-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportProvisioners := []string{}
	supportProvisionersResult := gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten").Array()
	e2e.Logf("%s support provisioners are : %v", cloudProvider, supportProvisionersResult)
	for i := 0; i < len(supportProvisionersResult); i++ {
		supportProvisioners = append(supportProvisioners, gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten."+strconv.Itoa(i)).String())
	}
	return supportProvisioners
}

// Get pre-defined storageclass by cloudplatform and provisioner
func getPresetStorageClassNameByProvisioner(oc *exutil.CLI, cloudProvider string, provisioner string) string {
	scList := getPresetStorageClassListByProvisioner(oc, cloudProvider, provisioner)
	if len(scList) < 1 {
		return ""
	}
	return scList[0]
}

// Get pre-defined storageclass list by cloudplatform and provisioner
func getPresetStorageClassListByProvisioner(oc *exutil.CLI, cloudProvider string, provisioner string) (scList []string) {
	// TODO: Adaptation for known product issue https://issues.redhat.com/browse/OCPBUGS-1964
	// we need to remove the condition after the issue is solved
	if isGP2volumeSupportOnly(oc) && provisioner == "ebs.csi.aws.com" {
		return append(scList, "gp2-csi")
	}
	csiCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	scArray := gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#(name="+provisioner+").preset_scname").Array()
	for _, sc := range scArray {
		scList = append(scList, sc.Str)
	}
	return scList
}

// Get pre-defined storageclass by cloudplatform and provisioner
func getIntreePresetStorageClassNameByProvisioner(cloudProvider string, provisioner string) string {
	intreeCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-intree-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	return gjson.GetBytes(intreeCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#(name="+provisioner+").preset_scname").String()
}

// Get pre-defined volumesnapshotclass by cloudplatform and provisioner
func getPresetVolumesnapshotClassNameByProvisioner(cloudProvider string, provisioner string) string {
	csiCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	return gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#(name="+provisioner+").preset_vscname").String()
}

// Get the now timestamp mil second
func nowStamp() string {
	return time.Now().Format(time.StampMilli)
}

// Log output the storage debug info
func debugLogf(format string, args ...interface{}) {
	if logLevel := os.Getenv("STORAGE_LOG_LEVEL"); logLevel == "DEBUG" {
		e2e.Logf(fmt.Sprintf(nowStamp()+": *STORAGE_DEBUG*:\n"+format, args...))
	}
}

func getZonesFromWorker(oc *exutil.CLI) []string {
	var workerZones []string
	workerNodes, err := exutil.GetClusterNodesBy(oc, "worker")
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, workerNode := range workerNodes {
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes/"+workerNode, "-o=jsonpath={.metadata.labels.failure-domain\\.beta\\.kubernetes\\.io\\/zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !contains(workerZones, zone) {
			workerZones = append(workerZones, zone)
		}
	}

	return workerZones
}

// Get and print the oc describe info, set namespace as "" for cluster-wide resource
func getOcDescribeInfo(oc *exutil.CLI, namespace string, resourceKind string, resourceName string) {
	var ocDescribeInfo string
	var err error
	if namespace != "" {
		ocDescribeInfo, err = oc.WithoutNamespace().Run("describe").Args("-n", namespace, resourceKind, resourceName).Output()
	} else {
		ocDescribeInfo, err = oc.WithoutNamespace().Run("describe").Args(resourceKind, resourceName).Output()
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("***$ oc describe %s %s\n%s", resourceKind, resourceName, ocDescribeInfo)
	e2e.Logf("**************************************************************************")
}

// Get a random number of int64 type [m,n], n > m
func getRandomNum(m int64, n int64) int64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Int63n(n-m+1) + m
}

// Restore the credential of vSphere CSI driver
func restoreVsphereCSIcredential(oc *exutil.CLI, originKey string, originValue string) {
	e2e.Logf("****** Restore the credential of vSphere CSI driver and make sure the CSO recover healthy ******")
	output, err := oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("patch").Args("secret/vmware-vsphere-cloud-credentials", "-n", "openshift-cluster-csi-drivers", `-p={"data":{"`+originKey+`":"`+originValue+`"}}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("patched"))
	e2e.Logf("The vSphere CSI secret recovered")
	vSphereDriverController.waitReady(oc.AsAdmin())
}

// Delete string list's specified string element
func deleteElement(list []string, element string) []string {
	result := make([]string, 0)
	for _, v := range list {
		if v != element {
			result = append(result, v)
		}
	}
	return result
}

// Get Cluster Storage Operator specified status value
func getCSOspecifiedStatusValue(oc *exutil.CLI, specifiedStatus string) (string, error) {
	status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/storage", "-o=jsonpath={.status.conditions[?(.type=='"+specifiedStatus+"')].status}").Output()
	debugLogf("CSO \"%s\" status value is \"%s\"", specifiedStatus, status)
	return status, err
}

// Wait for Cluster Storage Operator specified status value as expected
func waitCSOspecifiedStatusValueAsExpected(oc *exutil.CLI, specifiedStatus string, expectedValue string) {
	pollErr := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		realValue, err := getCSOspecifiedStatusValue(oc, specifiedStatus)
		if err != nil {
			e2e.Logf("Get CSO \"%s\" status value failed of: \"%v\"", err)
			return false, nil
		}
		if realValue == expectedValue {
			e2e.Logf("CSO \"%s\" status value become expected \"%s\"", specifiedStatus, expectedValue)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Waiting for CSO \"%s\" status value become expected \"%s\" timeout", specifiedStatus, expectedValue))
}

// checkCSOspecifiedStatusValueAsExpectedConsistently checks Cluster Storage Operator specified status value as expected consistently
func checkCSOspecifiedStatusValueAsExpectedConsistently(oc *exutil.CLI, specifiedStatus string, expectedValue string) {
	o.Consistently(func() string {
		actualStatusValue, _ := getCSOspecifiedStatusValue(oc, specifiedStatus)
		return actualStatusValue
	}, 60*time.Second, 5*time.Second).Should(o.ContainSubstring(expectedValue))
}

// Check Cluster Storage Operator healthy
func checkCSOhealthy(oc *exutil.CLI) (bool, error) {
	// CSO healthyStatus:[degradedStatus:False, progressingStatus:False, availableStatus:True, upgradeableStatus:True]
	var healthyStatus = []string{"False", "False", "True", "True"}
	csoStatusJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/storage", "-o", "json").Output()
	degradedStatus := gjson.Get(csoStatusJSON, `status.conditions.#(type=Degraded).status`).String()
	progressingStatus := gjson.Get(csoStatusJSON, `status.conditions.#(type=Progressing).status`).String()
	availableStatus := gjson.Get(csoStatusJSON, `status.conditions.#(type=Available).status`).String()
	upgradeableStatus := gjson.Get(csoStatusJSON, `status.conditions.#(type=Upgradeable).status`).String()
	e2e.Logf("CSO degradedStatus:%s, progressingStatus:%v, availableStatus:%v, upgradeableStatus:%v", degradedStatus, progressingStatus, availableStatus, upgradeableStatus)
	return reflect.DeepEqual([]string{degradedStatus, progressingStatus, availableStatus, upgradeableStatus}, healthyStatus), err
}

// Wait for Cluster Storage Operator become healthy
func waitCSOhealthy(oc *exutil.CLI) {
	pollErr := wait.Poll(defaultMaxWaitingTime/defaultIterationTimes, defaultMaxWaitingTime, func() (bool, error) {
		healthyBool, err := checkCSOhealthy(oc)
		if err != nil {
			e2e.Logf("Get CSO status failed of: \"%v\"", err)
			return false, err
		}
		if healthyBool {
			e2e.Logf("CSO status become healthy")
			return true, nil
		}
		return false, nil
	})
	if pollErr != nil {
		getOcDescribeInfo(oc.AsAdmin(), "", "co", "storage")
		ClusterStorageOperatorLogs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-cluster-storage-operator", "-l", "name=cluster-storage-operator", "--tail=100").Output()
		e2e.Logf("***$ oc logs -n openshift-cluster-storage-operator -l name=cluster-storage-operator --tail=100***\n%s", ClusterStorageOperatorLogs)
		e2e.Logf("**************************************************************************")
	}
	exutil.AssertWaitPollNoErr(pollErr, "Waiting for CSO become healthy timeout")
}

// Check specific Cluster Operator healthy by clusteroperator.status.conditions
// In storage test, we usually don't care the upgrade condition, some Cluster Operator might be always upgradeableStatus:False in some releases
func checkCOHealthy(oc *exutil.CLI, coName string) (bool, error) {
	// CO healthyStatus:[degradedStatus:False, progressingStatus:False, availableStatus:True]
	var healthyStatus = []string{"False", "False", "True"}
	coStatusJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", coName, "-o", "json").Output()
	degradedStatus := gjson.Get(coStatusJSON, `status.conditions.#(type=Degraded).status`).String()
	progressingStatus := gjson.Get(coStatusJSON, `status.conditions.#(type=Progressing).status`).String()
	availableStatus := gjson.Get(coStatusJSON, `status.conditions.#(type=Available).status`).String()
	e2e.Logf("Checking the %v CO status, degradedStatus:%v, progressingStatus:%v, availableStatus:%v.", coName, degradedStatus, progressingStatus, availableStatus)
	return reflect.DeepEqual([]string{degradedStatus, progressingStatus, availableStatus}, healthyStatus), err
}

// Wait for Cluster Storage Operator become healthy using checkCOHealthy()
// Using 20 Second as polling time, consider it when define the maxWaitingSeconds
func waitCOHealthy(oc *exutil.CLI, coName string, maxWaitingSeconds int) {
	pollErr := wait.Poll(20*time.Second, time.Duration(maxWaitingSeconds)*time.Second, func() (bool, error) {
		healthyBool, err := checkCOHealthy(oc, coName)
		if err != nil {
			e2e.Logf("Get Cluster status failed of: \"%v\"", err)
			return false, err
		}
		if healthyBool {
			e2e.Logf("Cluster Operator %v status become healthy", coName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Waiting for Cluster Operator %v become healthy timeout after %v seconds.", coName, maxWaitingSeconds))
}

// Check CSI driver successfully installed or no
func checkCSIDriverInstalled(oc *exutil.CLI, supportProvisioners []string) bool {
	var provisioner string
	for _, provisioner = range supportProvisioners {
		csiDriver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clustercsidrivers", provisioner).Output()
		if err != nil || strings.Contains(csiDriver, "not found") {
			e2e.Logf("Error to get CSI driver:%v", err)
			return false
		}
	}
	e2e.Logf("CSI driver got successfully installed for provisioner '%s'", provisioner)
	return true
}

// waitResourceSpecifiedEventsOccurred waits for specified resource event occurred
func waitResourceSpecifiedEventsOccurred(oc *exutil.CLI, namespace string, resourceName string, events ...string) {
	o.Eventually(func() bool {
		Info, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", namespace, "--field-selector=involvedObject.name="+resourceName).Output()
		if err != nil {
			e2e.Logf("Failed to get resource %s events caused by \n %v \n Trying next round.", resourceName, err)
			return false
		}
		for count := range events {
			if !strings.Contains(Info, events[count]) {
				e2e.Logf("The events of %s are: \n %s", resourceName, Info)
				return false
			}
		}
		return true
	}, 60*time.Second, 10*time.Second).Should(o.BeTrue())
}

// Get the Resource Group id value
// https://bugzilla.redhat.com/show_bug.cgi?id=2110899
// If skip/empty value it will create in default resource group id
// Currently adding other than default rgid value to check if it really works other than default rgid
func getResourceGroupID(oc *exutil.CLI) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "cluster-config-v1", "-n", "kube-system", "-o=jsonpath={.data.install-config}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	jsonOutput, err := yaml.YAMLToJSON([]byte(output))
	o.Expect(err).NotTo(o.HaveOccurred())
	rgid := gjson.Get(string(jsonOutput), `platform.`+cloudProvider+`.resourceGroupID`).String()
	//o.Expect(rgid).NotTo(o.BeEmpty())
	if rgid == "" {
		return "rg-aek2u7zroz6ggyy"
	}
	return rgid
}

// Check if FIPS is enabled
// Azure-file doesn't work on FIPS enabled cluster
func checkFips(oc *exutil.CLI) bool {
	node, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "--selector=node-role.kubernetes.io/worker,kubernetes.io/os=linux", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	fipsInfo, err := execCommandInSpecificNode(oc, node, "fips-mode-setup --check")
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(fipsInfo, "FIPS mode is disabled.") {
		e2e.Logf("FIPS is not enabled.")
		return false
	}
	e2e.Logf("FIPS is enabled.")
	return true
}

// Convert strings slice to integer slice
func stringSliceToIntSlice(strSlice []string) ([]int, []error) {
	var (
		intSlice = make([]int, 0, len(strSlice))
		errSlice = make([]error, 0, len(strSlice))
	)
	for _, strElement := range strSlice {
		intElement, err := strconv.Atoi(strElement)
		if err != nil {
			errSlice = append(errSlice, err)
		}
		intSlice = append(intSlice, intElement)
	}
	return intSlice, errSlice
}

// mergeMaps merges map objects to one map object
// the same key's value will be covered by the newest value (the last map key's value)
// no mutexes, doesn't support for concurrent operations
func mergeMaps(mObj ...map[string]interface{}) map[string]interface{} {
	resultObj := make(map[string]interface{}, 10)
	for i := 0; i < len(mObj); i++ {
		for k, v := range mObj[i] {
			resultObj[k] = v
		}
	}
	return resultObj
}

// Compare cluster versions
// versionA, versionB should be the same length
// E.g. [{versionA: "4.10.1", versionB: "4.10.12"}, {versionA: "4.10", versionB: "4.11}]
// IF versionA above versionB return "bool:true"
// ELSE return "bool:false" (Contains versionA = versionB)
func versionIsAbove(versionA, versionB string) bool {
	var (
		subVersionStringA, subVersionStringB []string
		subVersionIntA, subVersionIntB       []int
		errList                              []error
	)
	subVersionStringA = strings.Split(versionA, ".")
	subVersionIntA, errList = stringSliceToIntSlice(subVersionStringA)
	o.Expect(errList).Should(o.HaveLen(0))
	subVersionStringB = strings.Split(versionB, ".")
	subVersionIntB, errList = stringSliceToIntSlice(subVersionStringB)
	o.Expect(errList).Should(o.HaveLen(0))
	o.Expect(len(subVersionIntA)).Should(o.Equal(len(subVersionIntB)))
	var minusRes int
	for i := 0; i < len(subVersionIntA); i++ {
		minusRes = subVersionIntA[i] - subVersionIntB[i]
		if minusRes > 0 {
			e2e.Logf("Version:\"%s\" is above Version:\"%s\"", versionA, versionB)
			return true
		}
		if minusRes == 0 {
			continue
		}
		e2e.Logf("Version:\"%s\" is below Version:\"%s\"", versionA, versionB)
		return false
	}
	e2e.Logf("Version:\"%s\" is the same with Version:\"%s\"", versionA, versionB)
	return false
}

// Patch a specified resource
// E.g. oc patch -n <namespace> <resourceKind> <resourceName> -p <JSONPatch> --type=<patchType>
// type parameter that you can set to one of these values:
// Parameter value	Merge type
// 1. json	JSON Patch, RFC 6902
// 2. merge	JSON Merge Patch, RFC 7386
// 3. strategic	Strategic merge patch
func patchResourceAsAdmin(oc *exutil.CLI, namespace, resourceKindAndName, JSONPatch, patchType string) {
	if namespace == "" {
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceKindAndName, "-p", JSONPatch, "--type="+patchType).Output()).To(o.ContainSubstring("patched"))
	} else {
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", namespace, resourceKindAndName, "-p", JSONPatch, "--type="+patchType).Output()).To(o.ContainSubstring("patched"))
	}
}

// Get the oc client version major.minor x.x (e.g. 4.11)
func getClientVersion(oc *exutil.CLI) string {
	output, err := oc.WithoutNamespace().AsAdmin().Run("version").Args("-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	clientVersion := gjson.Get(output, `clientVersion.gitVersion`).String()
	o.Expect(clientVersion).NotTo(o.BeEmpty())
	tempSlice := strings.Split(clientVersion, ".")
	clientVersion = tempSlice[0] + "." + tempSlice[1]
	e2e.Logf("The oc client version is : \"%s\"", clientVersion)
	return clientVersion
}

// Get the cluster history versions
func getClusterHistoryVersions(oc *exutil.CLI) []string {
	historyVersionOp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].status.history[*].version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	historyVersions := strings.Split(historyVersionOp, " ")
	e2e.Logf("Cluster history versions are %s", historyVersions)
	return historyVersions
}

// isClusterHistoryVersionsContains checks whether the cluster history versions contains specified version
func isClusterHistoryVersionsContains(oc *exutil.CLI, version string) bool {
	clusterHistoryVersions := getClusterHistoryVersions(oc)
	if len(clusterHistoryVersions) > 0 {
		for _, historyVersion := range clusterHistoryVersions {
			if strings.HasPrefix(historyVersion, version) {
				return true
			}
		}
	}
	return false
}

// Get valid volume size by cloudProvider
func getValidVolumeSize() (validVolSize string) {
	switch cloudProvider {
	// AlibabaCloud minimum volume size is 20Gi
	case "alibabacloud":
		validVolSize = strconv.FormatInt(getRandomNum(20, 30), 10) + "Gi"
	// IBMCloud minimum volume size is 10Gi
	case "ibmcloud":
		validVolSize = strconv.FormatInt(getRandomNum(10, 20), 10) + "Gi"
	// Other Clouds(AWS GCE Azure OSP vSphere) minimum volume size is 1Gi
	default:
		validVolSize = strconv.FormatInt(getRandomNum(1, 10), 10) + "Gi"
	}
	return validVolSize
}

// isAwsOutpostCluster judges whether the aws test cluster has outpost workers
func isAwsOutpostCluster(oc *exutil.CLI) bool {
	if cloudProvider != "aws" {
		return false
	}
	return strings.Contains(getWorkersInfo(oc), `topology.ebs.csi.aws.com/outpost-id`)
}

// isAwsLocalZoneCluster judges whether the aws test cluster has edge(local zone) workers
func isAwsLocalZoneCluster(oc *exutil.CLI) bool {
	if cloudProvider != "aws" {
		return false
	}
	return strings.Contains(getWorkersInfo(oc), `node-role.kubernetes.io/edge`)
}

// isGP2volumeSupportOnly judges whether the aws test cluster only support gp2 type volumes
func isGP2volumeSupportOnly(oc *exutil.CLI) bool {
	return isAwsLocalZoneCluster(oc) || isAwsOutpostCluster(oc)
}

// Definition of int64Slice type
// Used for expand sort.Sort() method
type int64Slice []int64

// Len is the number of elements in the collection.
func (p int64Slice) Len() int {
	return len(p)
}

// Less describe a transitive ordering.
func (p int64Slice) Less(i, j int) bool {
	return p[i] < p[j]
}

// Swap swaps the elements with indexes i and j.
func (p int64Slice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// isSpecifiedResourceExist checks whether the specified resource exist, returns bool
func isSpecifiedResourceExist(oc *exutil.CLI, resourceKindAndName string, resourceNamespace string) bool {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName, "--ignore-not-found")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(cargs...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return !strings.EqualFold(output, "")
}

// expectSpecifiedResourceExist one-time checks whether the specified resource exist or not, case will fail if it is not expected
func expectSpecifiedResourceExist(oc *exutil.CLI, resourceKindAndName string, resourceNamespace string, checkFlag bool) {
	o.Expect(isSpecifiedResourceExist(oc, resourceKindAndName, resourceNamespace)).Should(o.Equal(checkFlag))
}

// isSpecifiedAPIExist checks whether the specified api exist, returns bool
func isSpecifiedAPIExist(oc *exutil.CLI, apiNameAndVersion string) bool {
	output, err := oc.AsAdmin().WithoutNamespace().Run("api-resources").Args().Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Contains(output, apiNameAndVersion)
}

// isCRDSpecificFieldExist checks whether the CRD specified field exist, returns bool
func isCRDSpecificFieldExist(oc *exutil.CLI, crdFieldPath string) bool {
	var (
		crdFieldInfo string
		getInfoErr   error
	)
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		crdFieldInfo, getInfoErr = oc.AsAdmin().WithoutNamespace().Run("explain").Args(crdFieldPath).Output()
		if getInfoErr != nil && strings.Contains(crdFieldInfo, "the server doesn't have a resource type") {
			if strings.Contains(crdFieldInfo, "the server doesn't have a resource type") {
				e2e.Logf("The test cluster specified crd field: %s is not exist.", crdFieldPath)
				return true, nil
			}
			// TODO: The "couldn't find resource" error info sometimes(very low frequency) happens in few cases but I couldn't reproduce it, this retry solution should be an enhancement
			if strings.Contains(getInfoErr.Error(), "couldn't find resource") {
				e2e.Logf("Failed to check whether the specified crd field: %s exist, try again. Err:\n%v", crdFieldPath, getInfoErr)
				return false, nil
			}
			return false, getInfoErr
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Check whether the specified: %s crd field exist timeout.", crdFieldPath))
	return !strings.Contains(crdFieldInfo, "the server doesn't have a resource type")
}

// isMicroShiftCluster judges whether the test cluster is microshift cluster
// TODO: It's not a precise judgment, just a temp solution, need to do more research and enhance later
func isMicroshiftCluster(oc *exutil.CLI) bool {
	return !isCRDSpecificFieldExist(oc, "template.apiVersion")
}

// Set the specified csi driver ManagementState
func setSpecifiedCSIDriverManagementState(oc *exutil.CLI, driverName string, managementState string) {
	patchResourceAsAdmin(oc, "", "clustercsidriver/"+driverName, `{"spec":{"managementState": "`+managementState+`"}}`, "merge")
}

// Add specified annotation to specified resource
func addAnnotationToSpecifiedResource(oc *exutil.CLI, resourceNamespace string, resourceKindAndName string, annotationKeyAndValue string) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, "--overwrite", resourceKindAndName, annotationKeyAndValue)
	o.Expect(oc.WithoutNamespace().Run("annotate").Args(cargs...).Execute()).NotTo(o.HaveOccurred())
}

// Remove specified annotation from specified resource
func removeAnnotationFromSpecifiedResource(oc *exutil.CLI, resourceNamespace string, resourceKindAndName string, annotationKey string) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName, annotationKey+"-")
	o.Expect(oc.WithoutNamespace().Run("annotate").Args(cargs...).Execute()).NotTo(o.HaveOccurred())
}

// getByokKeyIDFromClusterCSIDriver gets the kms key id info from BYOK clusters clustercsidriver
func getByokKeyIDFromClusterCSIDriver(oc *exutil.CLI, driverProvisioner string) (keyID string) {
	clustercsidriverJSONContent, getContentError := oc.AsAdmin().WithoutNamespace().Run("get").Args("clustercsidriver/"+driverProvisioner, "-ojson").Output()
	o.Expect(getContentError).ShouldNot(o.HaveOccurred())
	if !gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.`+cloudProvider).Exists() {
		e2e.Logf("None kms key settings in clustercsidriver/%s, the test cluster is not a BYOK cluster", driverProvisioner)
		return keyID
	}
	switch cloudProvider {
	case "aws":
		keyID = gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.aws.kmsKeyARN`).String()
	case "azure":
		diskEncryptionSetName := gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.azure.diskEncryptionSet.name`).String()
		diskEncryptionSetResourceGroup := gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.azure.diskEncryptionSet.resourceGroup`).String()
		diskEncryptionSetSubscriptionID := gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.azure.diskEncryptionSet.subscriptionID`).String()
		keyID = "/subscriptions/" + diskEncryptionSetSubscriptionID + "/resourceGroups/" + diskEncryptionSetResourceGroup + "/providers/Microsoft.Compute/diskEncryptionSets/" + diskEncryptionSetName
	case "gcp":
		keyRing := gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.gcp.kmsKey.keyRing`).String()
		location := gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.gcp.kmsKey.location`).String()
		name := gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.gcp.kmsKey.name`).String()
		projectID := gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.gcp.kmsKey.projectID`).String()
		keyID = "projects/" + projectID + "/locations/" + location + "/keyRings/" + keyRing + "/cryptoKeys/" + name
	case "ibmcloud":
		keyID = gjson.Get(clustercsidriverJSONContent, `spec.driverConfig.ibmcloud.encryptionKeyCRN`).String()
	default:
		return keyID
	}
	e2e.Logf(`The BYOK test cluster driverProvisioner/%s kms keyID is: "%s"`, driverProvisioner, keyID)
	return keyID
}

// get vSphere infrastructure.spec.platformSpec.vsphere.failureDomains, which is supported from 4.12
// 4.13+ even if there's no failureDomains set when install, it will auto generate the "generated-failure-domain"
func getVsphereFailureDomainsNum(oc *exutil.CLI) (fdNum int64) {
	fdNames, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o", "jsonpath={.spec.platformSpec.vsphere.failureDomains[*].name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The vSphere cluster failureDomains are: [%s]", fdNames)
	return int64(len(strings.Fields(fdNames)))
}

// isVsphereTopologyConfigured judges whether the vSphere cluster configured infrastructure.spec.platformSpec.vsphere.failureDomains when install
func isVsphereTopologyConfigured(oc *exutil.CLI) bool {
	return getVsphereFailureDomainsNum(oc) >= 2
}

// isTechPreviewNoUpgrade checks if a cluster is a TechPreviewNoUpgrade cluster
func isTechPreviewNoUpgrade(oc *exutil.CLI) bool {
	featureGate, err := oc.AdminConfigClient().ConfigV1().FeatureGates().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		e2e.Failf("could not retrieve feature-gate: %v", err)
	}

	return featureGate.Spec.FeatureSet == configv1.TechPreviewNoUpgrade
}

// parseCapacityToBytes parses capacity with unit to int64 bytes size
func parseCapacityToBytes(capacityWithUnit string) int64 {
	bytesSize, parseBytesErr := resource.ParseQuantity(capacityWithUnit)
	o.Expect(parseBytesErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to parse capacity size with unit: %q", capacityWithUnit))
	e2e.Logf("%q bytes size is %d bytes", capacityWithUnit, bytesSize.Value())
	return bytesSize.Value()
}
