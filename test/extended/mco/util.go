package mco

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/x509"
	b64 "encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/sjson"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// fixturePathCache to store fixture path mapping, key: dir name under testdata, value: fixture path
var fixturePathCache = make(map[string]string)

// PodDisruptionBudget struct is used to handle PodDisruptionBudget resources in OCP
type PodDisruptionBudget struct {
	name      string
	namespace string
	template  string
}

// ImageContentSourcePolicy struct is used to handle ImageContentSourcePolicy resources in OCP
type ImageContentSourcePolicy struct {
	name     string
	template string
}

// ImageDigestMirrorSet struct is used to handle ImageDigestMirrorSet resources in OCP
type ImageDigestMirrorSet struct {
	Resource
	Template
}

// NewImageDigestMirrorSet create a new ImageDigestMirrorSet struct
func NewImageDigestMirrorSet(oc *exutil.CLI, name string, t Template) *ImageDigestMirrorSet {
	return &ImageDigestMirrorSet{Resource: *NewResource(oc, "ImageDigestMirrorSet", name), Template: t}
}

// ImageTagMirrorSet struct is used to handle ImageTagMirrorSet resources in OCP
type ImageTagMirrorSet struct {
	Resource
	Template
}

// NewImageTagMirrorSet create a new ImageTagMirrorSet struct
func NewImageTagMirrorSet(oc *exutil.CLI, name string, t Template) *ImageTagMirrorSet {
	return &ImageTagMirrorSet{Resource: *NewResource(oc, "ImageTagMirrorSet", name), Template: t}
}

// TextToVerify is a helper struct to verify configurations using the `createMcAndVerifyMCValue` function
type TextToVerify struct {
	textToVerifyForMC   string
	textToVerifyForNode string
	needBash            bool
	needChroot          bool
}

func (pdb *PodDisruptionBudget) create(oc *exutil.CLI) {
	logger.Infof("Creating pod disruption budget: %s", pdb.name)
	exutil.CreateNsResourceFromTemplate(oc, pdb.namespace, "--ignore-unknown-parameters=true", "-f", pdb.template, "-p", "NAME="+pdb.name)
}

func (pdb *PodDisruptionBudget) delete(oc *exutil.CLI) {
	logger.Infof("Deleting pod disruption budget: %s", pdb.name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pdb", pdb.name, "-n", pdb.namespace, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (icsp *ImageContentSourcePolicy) create(oc *exutil.CLI) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", icsp.template, "-p", "NAME="+icsp.name)
	mcp := NewMachineConfigPool(oc.AsAdmin(), "worker")
	mcp.waitForComplete()
	mcp.name = "master"
	mcp.waitForComplete()
}

func (icsp *ImageContentSourcePolicy) delete(oc *exutil.CLI) {
	logger.Infof("deleting icsp config: %s", icsp.name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("imagecontentsourcepolicy", icsp.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	mcp := NewMachineConfigPool(oc.AsAdmin(), "worker")
	mcp.waitForComplete()
	mcp.name = "master"
	mcp.waitForComplete()
}

func getTimeDifferenceInMinute(oldTimestamp, newTimestamp string) float64 {
	oldTimeValues := strings.Split(oldTimestamp, ":")
	oldTimeHour, _ := strconv.Atoi(oldTimeValues[0])
	oldTimeMinute, _ := strconv.Atoi(oldTimeValues[1])
	oldTimeSecond, _ := strconv.Atoi(strings.Split(oldTimeValues[2], ".")[0])
	oldTimeNanoSecond, _ := strconv.Atoi(strings.Split(oldTimeValues[2], ".")[1])
	newTimeValues := strings.Split(newTimestamp, ":")
	newTimeHour, _ := strconv.Atoi(newTimeValues[0])
	newTimeMinute, _ := strconv.Atoi(newTimeValues[1])
	newTimeSecond, _ := strconv.Atoi(strings.Split(newTimeValues[2], ".")[0])
	newTimeNanoSecond, _ := strconv.Atoi(strings.Split(newTimeValues[2], ".")[1])
	y, m, d := time.Now().Date()
	oldTime := time.Date(y, m, d, oldTimeHour, oldTimeMinute, oldTimeSecond, oldTimeNanoSecond, time.UTC)
	newTime := time.Date(y, m, d, newTimeHour, newTimeMinute, newTimeSecond, newTimeNanoSecond, time.UTC)
	return newTime.Sub(oldTime).Minutes()
}

func filterTimestampFromLogs(logs string, numberOfTimestamp int) []string {
	return regexp.MustCompile("(?m)[0-9]{1,2}:[0-9]{1,2}:[0-9]{1,2}.[0-9]{1,6}").FindAllString(logs, numberOfTimestamp)
}

func getMachineConfigDetails(oc *exutil.CLI, mcName string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("mc", mcName, "-o", "yaml").Output()
}

// func getKubeletConfigDetails(oc *exutil.CLI, kcName string) (string, error) {
// 	return oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeletconfig", kcName, "-o", "yaml").Output()
// }

func getPullSecret(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/pull-secret", "-n", "openshift-config", `--template={{index .data ".dockerconfigjson" | base64decode}}`).OutputToFile("auth.dockerconfigjson")
}

func setDataForPullSecret(oc *exutil.CLI, configFile string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/pull-secret", "-n", "openshift-config", "--from-file=.dockerconfigjson="+configFile).Output()
}

func getCommitID(oc *exutil.CLI, component, clusterVersion string) (string, error) {
	secretFile, secretErr := getPullSecret(oc)
	if secretErr != nil {
		return "", secretErr
	}
	outFilePath, ocErr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config="+secretFile, "--commits", clusterVersion, "--insecure=true").OutputToFile("commitIdLogs.txt")
	if ocErr != nil {
		return "", ocErr
	}
	commitID, cmdErr := exec.Command("bash", "-c", "cat "+outFilePath+" | grep "+component+" | awk '{print $3}'").Output()
	return strings.TrimSuffix(string(commitID), "\n"), cmdErr
}

func getGoVersion(component, commitID string) (float64, error) {
	curlOutput, curlErr := exec.Command("bash", "-c", "curl -Lks https://raw.githubusercontent.com/openshift/"+component+"/"+commitID+"/go.mod | egrep '^go'").Output()
	if curlErr != nil {
		return 0, curlErr
	}
	goVersion := string(curlOutput)[3:]
	return strconv.ParseFloat(strings.TrimSuffix(goVersion, "\n"), 64)
}

func containsMultipleStrings(sourceString string, expectedStrings []string) bool {
	o.Expect(sourceString).NotTo(o.BeEmpty())
	o.Expect(expectedStrings).NotTo(o.BeEmpty())

	var count int
	for _, element := range expectedStrings {
		if strings.Contains(sourceString, element) {
			count++
		}
	}
	return len(expectedStrings) == count
}

// generateTemplateAbsolutePath manipulates absolute path of test file by
// cached fixture test data dir and file name
// because exutil.FixturePath will copy all test files to fixture path (tmp dir with prefix fixture-testdata-dir)
// this operation is very expensive, we don't want to call it for every case
func generateTemplateAbsolutePath(fileName string) string {
	mcoDirName := "mco"
	mcoBaseDir := ""
	if mcoBaseDir = fixturePathCache[mcoDirName]; mcoBaseDir == "" {
		logger.Infof("mco fixture dir is not initialized, start to create")
		mcoBaseDir = exutil.FixturePath("testdata", mcoDirName)
		fixturePathCache[mcoDirName] = mcoBaseDir
		logger.Infof("mco fixture dir is initialized: %s", mcoBaseDir)
	} else {
		mcoBaseDir = fixturePathCache[mcoDirName]
		logger.Debugf("mco fixture dir found in cache: %s", mcoBaseDir)
	}
	return filepath.Join(mcoBaseDir, fileName)
}

func getSATokenFromContainer(oc *exutil.CLI, podName, podNamespace, container string) string {
	podOut, err := exutil.RemoteShContainer(oc, podNamespace, podName, container, "cat", "/var/run/secrets/kubernetes.io/serviceaccount/token")
	o.Expect(err).NotTo(o.HaveOccurred())

	return podOut
}

func getHostFromRoute(oc *exutil.CLI, routeName, routeNamespace string) string {
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", routeName, "-n", routeNamespace, "-o", "jsonpath='{.spec.host}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	return stdout
}

// DEPRECATED, use generateTempFilePath instead
func generateTmpFile(oc *exutil.CLI, fileName string) string {
	return filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+fileName)
}

func getPrometheusQueryResults(oc *exutil.CLI, query string) string {

	token := getSATokenFromContainer(oc, "prometheus-k8s-0", "openshift-monitoring", "prometheus")

	routeHost := getHostFromRoute(oc, "prometheus-k8s", "openshift-monitoring")
	url := fmt.Sprintf("https://%s/api/v1/query?query=%s", routeHost, query)
	headers := fmt.Sprintf("Authorization: Bearer %s", token)

	curlCmd := fmt.Sprintf("curl -ks -H '%s' %s", headers, url)
	logger.Infof("curl cmd:\n %s", curlCmd)

	curlOutput, cmdErr := exec.Command("bash", "-c", curlCmd).Output()
	logger.Infof("curl output:\n%s", curlOutput)
	o.Expect(cmdErr).NotTo(o.HaveOccurred())

	return string(curlOutput)
}

func gZipData(data []byte) (compressedData []byte, err error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	defer func() {
		_ = gz.Close()
	}()

	_, err = gz.Write(data)
	if err != nil {
		return nil, err
	}

	if err := gz.Flush(); err != nil {
		return nil, err
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}

	compressedData = b.Bytes()

	return compressedData, nil
}

func jsonEncode(s string) string {
	e, err := json.Marshal(s)
	if err != nil {
		e2e.Failf("Error json encoding the string: %s", s)
	}
	return string(e)
}

// MarshalOrFail returns a marshalled interface or panics
func MarshalOrFail(input interface{}) []byte {
	bytes, err := json.Marshal(input)
	if err != nil {
		o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "The data cannot be marshaled. Data: %v", input)
	}
	return bytes
}

func getURLEncodedFileConfig(destinationPath, content, mode string) string {
	encodedContent := url.PathEscape(content)

	return getFileConfig(destinationPath, "data:,"+encodedContent, mode)
}

func getBase64EncodedFileConfig(destinationPath, content, mode string) string {
	return getFileConfig(destinationPath, GetBase64EncodedFileSourceContent(content), mode)
}

func getFileConfig(destinationPath, source, mode string) string {
	decimalMode := mode
	// if octal number we convert it to decimal. Json templates do not accept numbers with a leading zero (octal).
	// if we don't do this conversion the 'oc process' command will not be able to render the template because {"mode": 0666}
	//   is not a valid json. Numbers in json cannot start with a leading 0
	if mode != "" && mode[0] == '0' {
		// parse the octal string and conver to integer
		iMode, err := strconv.ParseInt(mode, 8, 64)
		// get a string with the decimal numeric representation of the mode
		decimalMode = fmt.Sprintf("%d", os.FileMode(iMode))
		if err != nil {
			e2e.Failf("Filer permissions %s cannot be converted to integer", mode)
		}
	}

	var fileConfig string
	if mode == "" {
		fileConfig = fmt.Sprintf(`{"contents": {"source": "%s"}, "path": "%s"}`, source, destinationPath)
	} else {
		fileConfig = fmt.Sprintf(`{"contents": {"source": "%s"}, "path": "%s", "mode": %s}`, source, destinationPath, decimalMode)
	}

	return fileConfig
}

func getGzipFileJSONConfig(destinationPath, fileContent string) string {
	compressedContent, err := gZipData([]byte(fileContent))
	o.Expect(err).NotTo(o.HaveOccurred())
	encodedContent := b64.StdEncoding.EncodeToString(compressedContent)
	fileConfig := fmt.Sprintf(`{"contents": {"compression": "gzip", "source": "data:;base64,%s"}, "path": "%s"}`, encodedContent, destinationPath)
	return fileConfig
}

func getMaskServiceWithContentsConfig(name string, mask bool, unitContents string) string {
	// Escape not valid characters in json from the file content
	escapedContent := jsonEncode(unitContents)
	return fmt.Sprintf(`{"name": "%s", "mask": %t, "contents": %s}`, name, mask, escapedContent)
}

func getMaskServiceConfig(name string, mask bool) string {
	return fmt.Sprintf(`{"name": "%s", "mask": %t}`, name, mask)
}

func getDropinFileConfig(unitName string, enabled bool, fileName, fileContent string) string {
	// Escape not valid characters in json from the file content
	escapedContent := jsonEncode(fileContent)
	return fmt.Sprintf(`{"name": "%s", "enabled": %t, "dropins": [{"name": "%s", "contents": %s}]}`, unitName, enabled, fileName, escapedContent)
}

func getSingleUnitConfig(unitName string, unitEnabled bool, unitContents string) string {
	// Escape not valid characters in json from the file content
	escapedContent := jsonEncode(unitContents)
	return fmt.Sprintf(`{"name": "%s", "enabled": %t, "contents": %s}`, unitName, unitEnabled, escapedContent)
}

// AddToAllMachineSets adds a delta to all MachineSets replicas and wait for the MachineSets to be ready
func AddToAllMachineSets(oc *exutil.CLI, delta int) error {
	allMs, err := NewMachineSetList(oc, "openshift-machine-api").GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())

	var addErr error
	modifiedMSs := []MachineSet{}
	for _, ms := range allMs {
		addErr = ms.AddToScale(delta)
		if addErr == nil {
			modifiedMSs = append(modifiedMSs, ms)
		} else {
			break
		}
	}

	if addErr != nil {
		logger.Infof("Error reconfiguring MachineSets. Restoring original replicas.")
		for _, ms := range modifiedMSs {
			_ = ms.AddToScale(-1 * delta)
		}

		return addErr
	}

	var waitErr error
	for _, ms := range allMs {
		immediate := true
		waitErr = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 20*time.Minute, immediate, func(ctx context.Context) (bool, error) { return ms.GetIsReady(), nil })
		if waitErr != nil {
			logger.Errorf("MachineSet %s is not ready. Restoring original replicas.", ms.GetName())
			for _, ms := range modifiedMSs {
				_ = ms.AddToScale(-1 * delta)
			}
			break
		}
	}

	return waitErr
}

func sortNodeList(nodes []Node) []Node {
	sort.Slice(nodes, func(l, r int) bool {
		lMetadata := JSON(nodes[l].GetOrFail("{.metadata}"))
		rMetadata := JSON(nodes[r].GetOrFail("{.metadata}"))

		lLabels := &JSONData{nil}
		if lMetadata.Get("labels").Exists() {
			lLabels = lMetadata.Get("labels")
		}
		rLabels := &JSONData{nil}
		if rMetadata.Get("labels").Exists() {
			rLabels = rMetadata.Get("labels")
		}

		lZone := lLabels.Get("topology.kubernetes.io/zone")
		rZone := rLabels.Get("topology.kubernetes.io/zone")
		// if both nodes have zone label, sort by zone, push nodes without label to end of list
		if lZone.Exists() && rZone.Exists() {
			if lZone.ToString() != rZone.ToString() {
				return lZone.ToString() < rZone.ToString()
			}
		} else if rZone.Exists() {
			return false
		} else if lZone.Exists() {
			return true
		}

		// if nodes are in the same zone or they have no labels sortby creationTime oldest to newest
		dateLayout := "2006-01-02T15:04:05Z"
		lDate, err := time.Parse(dateLayout, lMetadata.Get("creationTimestamp").ToString())
		if err != nil {
			e2e.Failf("Cannot parse creationTimestamp %s in node %s", lMetadata.Get("creationTimestamp").ToString(), nodes[l].GetName())

		}
		rDate, err := time.Parse(dateLayout, rMetadata.Get("creationTimestamp").ToString())
		if err != nil {
			e2e.Failf("Cannot parse creationTimestamp %s in node %s", rMetadata.Get("creationTimestamp").ToString(), nodes[r].GetName())

		}
		return lDate.Before(rDate)

	})
	return nodes
}

// sortMasterNodeList returns the list of nodes sorted by the order used to updated them in MCO master pool.
//
//	Master pool will use the same order as the rest of the pools, but the node running the operator pod will be the last one to be updated.
func sortMasterNodeList(oc *exutil.CLI, nodes []Node) ([]Node, error) {
	masterSortedNodes := []Node{}
	operatorNode, err := GetOperatorNode(oc)
	if err != nil {
		return nil, err
	}

	logger.Infof("MCO operator pod running in node: %s", operatorNode)

	var latestNode Node
	for _, item := range sortNodeList(nodes) {
		node := item
		if node.GetName() == operatorNode.GetName() {
			latestNode = node
			continue
		}
		masterSortedNodes = append(masterSortedNodes, node)
	}

	masterSortedNodes = append(masterSortedNodes, latestNode)

	logger.Infof("Sorted master nodes: %s", masterSortedNodes)

	return masterSortedNodes, nil
}

func getMachineConfigControllerPod(oc *exutil.CLI) (string, error) {
	pods, err := exutil.GetAllPodsWithLabel(oc.AsAdmin(), MachineConfigNamespace, "k8s-app=machine-config-controller")
	logger.Infof("machine-config-controller pod name is %s", pods[0])
	return pods[0], err
}

func getMachineConfigOperatorPod(oc *exutil.CLI) (string, error) {
	pods, err := exutil.GetAllPodsWithLabel(oc.AsAdmin(), MachineConfigNamespace, "k8s-app=machine-config-operator")
	logger.Infof("machine-config-operator pod name is %s", pods[0])
	return pods[0], err
}

func getAlertsByName(oc *exutil.CLI, alertName string) ([]JSONData, error) {

	mon, monErr := exutil.NewPrometheusMonitor(oc.AsAdmin())
	if monErr != nil {
		return nil, monErr
	}

	allAerts, allAlertErr := mon.GetAlerts()

	if allAlertErr != nil {
		return nil, allAlertErr
	}

	logger.Infof("get all alerts: %s\n", allAerts)

	jsonObj := JSON(allAerts)
	filteredAlerts, filteredAlertErr := jsonObj.GetJSONPath(fmt.Sprintf(`{.data.alerts[?(@.labels.alertname=="%s")]}`, alertName))

	if filteredAlertErr != nil {
		return nil, filteredAlertErr
	}

	for _, alert := range filteredAlerts {
		logger.Infof("filtered alert %s\n", alert.String())
	}

	return filteredAlerts, nil
}

// WrapWithBracketsIfIpv6 wraps the ip with brackets if it is an IPV6 address.
// In order to use IPV6 addresses with curl commands we need to wrap them between brackets.
func WrapWithBracketsIfIpv6(ip string) (string, error) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", fmt.Errorf("The string %s is not a valid IP", ip)
	}

	// If it is an IPV6 address, wrap it
	if parsedIP.To4() == nil {
		return "[" + ip + "]", nil
	}

	return ip, nil
}

func isFIPSEnabledInClusterConfig(oc *exutil.CLI) bool {
	cc := NewNamespacedResource(oc.AsAdmin(), "cm", "kube-system", "cluster-config-v1")
	ic := cc.GetOrFail("{.data.install-config}")
	return strings.Contains(ic, "fips: true")
}

// preChecks executes some basic checks to make sure the the cluster is healthy enough to run MCO test cases
func preChecks(oc *exutil.CLI) {
	exutil.By("MCO Preconditions Checks")

	allMCPs, err := NewMachineConfigPoolList(oc.AsAdmin()).GetAll()
	o.Expect(err).NotTo(o.HaveOccurred(), "Cannot get the list of MachineConfigPools")

	for _, pool := range allMCPs {
		logger.Infof("Check that %s pool is ready for testing", pool.GetName())
		if err := pool.SanityCheck(); err != nil { // Check that it is not degraded nor updating
			logger.Errorf("MCP is not ready for testing (it is updating or degraded):\n%s", pool.PrettyString())
			g.Skip(fmt.Sprintf("%s pool is not ready for testing. %s", pool.GetName(), err))
		}

	}

	logger.Infof("Check that all nodes are running")
	nodes, err := NewNodeList(oc).GetAllLinux()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "It is not possible to get the list of nodes in the cluster")

	for _, node := range nodes {
		o.EventuallyWithOffset(1, node.IsReady, "2m", "20s").Should(o.BeTrue(), "Node %s is not Ready. We can't continue testing.", node.GetName())
	}
	logger.Infof("End of MCO Preconditions\n")
}

// helper func to generate a temp file path with target dir
// and file name pattern, * is reserved keyword, will be replaced
// by random string
func generateTempFilePath(dir, pattern string) string {
	return filepath.Join(dir, strings.ReplaceAll(pattern, "*", exutil.GetRandomString()))
}

// create a temp dir with e2e namespace for every test in JustBeforeEach
// fail the test, if dir creation is failed
func createTmpDir() string {
	// according to review comment, dir name should not depend on temp ns
	// it is empty when initialized by `NewCLIWithoutNamespace`
	tmpdir := filepath.Join(e2e.TestContext.OutputDir, fmt.Sprintf("mco-test-%s", exutil.GetRandomString()))
	err := os.MkdirAll(tmpdir, 0o755)
	o.Expect(err).NotTo(o.HaveOccurred())
	logger.Infof("create test dir %s", tmpdir)
	return tmpdir
}

// parse base domain from dns config. format is like $clustername.$basedomain
// this func does not support negative case, if info cannot be retrieved, fail the test
func getBaseDomain(oc *exutil.CLI) string {
	baseDomain := NewResource(oc.AsAdmin(), "dns", "cluster").GetOrFail("{.spec.baseDomain}")
	return baseDomain[strings.Index(baseDomain, ".")+1:]
}

// check whether hypershift operator is installed and hostedcluster is created
func isHypershiftEnabled(oc *exutil.CLI) bool {
	guestClusterName, guestClusterKubeconfigFile, _ := exutil.ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc)
	return (guestClusterName != "" && guestClusterKubeconfigFile != "")
}

// get first hostedcluster name
func getFirstHostedCluster(oc *exutil.CLI) string {
	hostedClusterName, _, _ := exutil.ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc)
	logger.Infof("first hostedcluster name is %s", hostedClusterName)
	return hostedClusterName
}

// get image url of latest nightly build
// command is like `oc image info registry.ci.openshift.org/ocp/release:4.12 -a /tmp/config.json -ojson|jq -r '.config.config.Labels."io.openshift.release"'`
// this func does not support negative scenario, if any error occurred, fail the test directly
// return imageURL and build version
func getLatestImageURL(oc *exutil.CLI, release string) (string, string) {
	if release == "" {
		release = "4.12" // TODO: need to update default major version to 4.13 when 4.12 is GA
	}
	imageURLFormat := "%s:%s"
	registryBaseURL := "registry.ci.openshift.org/ocp/release"
	registryQueryURL := fmt.Sprintf(imageURLFormat, registryBaseURL, release)
	registryConfig, extractErr := getPullSecret(oc)
	defer os.Remove(registryConfig)
	o.Expect(extractErr).NotTo(o.HaveOccurred(), "extract registry config from pull secret error")

	imageInfo, getImageInfoErr := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", registryQueryURL, "-a", registryConfig, "-ojson").Output()
	o.Expect(getImageInfoErr).NotTo(o.HaveOccurred(), "get image info error")
	o.Expect(imageInfo).NotTo(o.BeEmpty())

	imageJSON := JSON(imageInfo)
	buildVersion := imageJSON.Get("config").Get("config").Get("Labels").Get(`io.openshift.release`).ToString()
	o.Expect(buildVersion).NotTo(o.BeEmpty(), "nightly build version is empty")
	imageDigest := imageJSON.Get("digest").ToString()
	o.Expect(imageDigest).NotTo(o.BeEmpty(), "image digest is empty")

	imageURL := fmt.Sprintf("%s@%s", registryBaseURL, imageDigest)
	logger.Infof("Get latest nigthtly build of %s: %s", release, imageURL)

	return imageURL, buildVersion

}

// skipTestIfSupportedPlatformNotMatched skip the test if supported platforms are not matched
func skipTestIfSupportedPlatformNotMatched(oc *exutil.CLI, supported ...string) {
	var match bool
	p := exutil.CheckPlatform(oc)
	for _, sp := range supported {
		if strings.EqualFold(sp, p) {
			match = true
			break
		}
	}

	if !match {
		g.Skip(fmt.Sprintf("skip test because current platform %s is not in supported list %v", p, supported))
	}
}

// IsAROCluster check cluster is ARO or not
func IsAROCluster(oc *exutil.CLI) bool {
	return NewResource(oc.AsAdmin(), "clusters.aro.openshift.io", "cluster").Exists()
}

// skipTestIfRTKernel skips the current test if the cluster is using real time kernel
func skipTestIfRTKernel(oc *exutil.CLI) {
	wMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
	mMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)

	isWorkerRT, err := wMcp.IsRealTimeKernel()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error trying to know if realtime kernel is active worker pool")

	isMasterRT, err := mMcp.IsRealTimeKernel()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error trying to know if realtime kernel is active master pool")

	if isWorkerRT || isMasterRT {
		g.Skip("Pools are using real time kernel configuration. This test cannot be executed if the cluster is using RT kernel.")
	}
}

// skipTestIfExtensionsAreUsed skips the current test if any extension has been deployed in the nodes
func skipTestIfExtensionsAreUsed(oc *exutil.CLI) {
	wMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
	mMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)

	wCurrentMC, err := wMcp.GetConfiguredMachineConfig()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error trying to get the current MC configured in worker pool")

	mCurrentMC, err := mMcp.GetConfiguredMachineConfig()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error trying to get the current MC configured in master pool")

	wExtensions, err := wCurrentMC.GetExtensions()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error trying to get the extensions configured in MC: %s", wCurrentMC.GetName())

	mExtensions, err := mCurrentMC.GetExtensions()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error trying to get the extensions configured in MC: %s", mCurrentMC.GetName())

	if wExtensions != "[]" || mExtensions != "[]" {
		g.Skip("Current cluster is using extensions. This test cannot be execute in a cluster using extensions")
	}

}

// skipTestIfCloudImagesCannotBeModified skips the current test if the machinesets cannot be modified to use other cloud images. Currently we can only do that in AWS and east-2 zone.
func skipTestIfCloudImagesCannotBeModified(oc *exutil.CLI) {
	// Skip if not AWS or GCP
	skipTestIfSupportedPlatformNotMatched(oc, AWSPlatform, GCPPlatform)

	// Skip if not east-2 zone
	infra := NewResource(oc.AsAdmin(), "infrastructure", "cluster")
	zone := infra.GetOrFail(`{.status.platformStatus.aws.region}`)
	if zone != "us-east-2" {
		g.Skip(fmt.Sprintf(`Current AWS zone is '%s'. AWS 'us-east-2' zone is required to execute this test case!.`, zone))
	}
}

// skipTestIfWorkersCannotBeScaled skips the current test if the worker pool cannot be scaled via machineset
func skipTestIfWorkersCannotBeScaled(oc *exutil.CLI) {
	logger.Infof("Checking if in this cluster workers can be scaled using machinesets")
	msl, err := NewMachineSetList(oc.AsAdmin(), MachineAPINamespace).GetAll()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error getting a list of MachineSet resources")

	// If there is no machineset then clearly we can't use them to scale the workers
	if len(msl) == 0 {
		g.Skip("There is no machineset available in current cluster. This test cannot be execute if workers cannot be scaled via machineset")
	}

	totalworkers := 0
	for _, ms := range msl {
		replicas := ms.GetOrFail(`{.spec.replicas}`)
		if replicas != "" {
			intReplicas, err := strconv.Atoi(replicas)
			if err == nil {
				totalworkers += intReplicas
			}
		}
	}

	// In some UPI/SNO/Compact clusters machineset resources exist, but they are all configured with 0 replicas
	// If all machinesets have 0 replicas, then it means that we need to skip the test case
	if totalworkers == 0 {
		g.Skip("There are machinesets in this cluster, but they are not available to scale workers. This test cannot be execute if workers cannot be scaled via machineset")
	}
}

// skipTestIfBaselineCapabilitySetIsNone skips the test cases if no enabledCapabilities found in resource clusterversion
func skipTestIfBaselineCapabilitySetIsNone(oc *exutil.CLI) {
	if isBaselineCapabilitySetNone(oc) {
		// i.e. enabledCapabilities not found, skip the test
		g.Skip("Skip this test because enabledCapabilities not found in resource clusterversion")
	}
}

// isBaselineCapabilitySetNone check value of enabledCapabilities in clusterversion, return true if it is empty or does not exist
func isBaselineCapabilitySetNone(oc *exutil.CLI) bool {
	return len(getEnabledCapabilities(oc)) == 0
}

// getEnabledCapabilities get enabled capability list, the invoker can check expected capability is enabled or not
func getEnabledCapabilities(oc *exutil.CLI) []interface{} {
	jsonStr := NewResource(oc.AsAdmin(), "clusterversion", "version").GetOrFail(`{.status.capabilities.enabledCapabilities}`)
	logger.Infof("enabled capabilities: %s", jsonStr)
	enabledCapabilities := make([]interface{}, 0)
	jsonData := JSON(jsonStr)
	if jsonData.Exists() {
		enabledCapabilities = jsonData.ToList()
	}

	return enabledCapabilities
}

// IsCapabilityEnabled check whether capability is in enabledCapabilities
func IsCapabilityEnabled(oc *exutil.CLI, capability string) bool {
	enabledCapabilities := getEnabledCapabilities(oc)
	enabled := false
	for _, ec := range enabledCapabilities {
		if ec == capability {
			enabled = true
			break
		}
	}
	logger.Infof("Capability [%s] is enabled: %v", capability, enabled)

	return enabled
}

// GetCurrentTestPolarionIDNumber inspects the name of the test case and return the number of the polarion ID linked to this automated test case. It returns an empty string if no ID found.
func GetCurrentTestPolarionIDNumber() string {
	name := g.CurrentSpecReport().FullText()

	r := regexp.MustCompile(`\d+`)
	return r.FindString(name)
}

// GetBase64EncodedFileSourceContent returns the ignition config "source" value for a content file encoded in base64
func GetBase64EncodedFileSourceContent(fileContent string) string {

	encodedContent := b64.StdEncoding.EncodeToString([]byte(fileContent))

	return "data:text/plain;charset=utf-8;base64," + encodedContent
}

// ConvertOctalPermissionsToDecimalOrFail transfomr an octal permission (0640) to its decimal form (416)
func ConvertOctalPermissionsToDecimalOrFail(octalPerm string) int {

	o.ExpectWithOffset(1, octalPerm).To(o.And(
		o.Not(o.BeEmpty()),
		o.HavePrefix("0")),
		"Error the octal permissions %s should not be empty and should start with a '0' character")

	// parse the octal string and conver to integer
	iMode, err := strconv.ParseInt(octalPerm, 8, 64)
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error parsing string %s to ocatl", octalPerm)

	return int(iMode)
}

// PtrInt returns the pointer to an integer
func PtrInt(a int) *int {
	return &a
}

// PtrStr returns the pointer to a string
func PtrStr(a string) *string {
	return &a
}

// PtrTo returns the pointer to the element passed as parameter
func PtrTo[T any](v T) *T {
	return &v
}

// RemoveAllMCOPods removes all MCO pods in openshift-machine-config-operator namespace
func RemoveAllMCOPods(oc *exutil.CLI) error {
	return removeMCOPods(oc, "--all")
}

// RemoveAllMCDPods removes all MCD pods in openshift-machine-config-operator namespace
func RemoveAllMCDPods(oc *exutil.CLI) error {
	return removeMCOPods(oc, "-l", "k8s-app=machine-config-daemon")
}

// removeAllMCOPods removes all MCO pods in openshift-machine-config-operator namespace matching the given selector args
func removeMCOPods(oc *exutil.CLI, argsSelector ...string) error {
	args := append([]string{"pods", "-n", MachineConfigNamespace}, argsSelector...)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(args...).Execute()

	if err != nil {
		logger.Errorf("Cannot delete the pods in %s namespace", MachineConfigNamespace)
		return err
	}

	logger.Infof("Waiting for MCO pods to be runnging and ready in namespace %s", MachineConfigNamespace)
	immediate := false
	waitErr := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 15*time.Minute, immediate,
		func(ctx context.Context) (bool, error) {
			status, err := NewNamespacedResourceList(oc.AsAdmin(), "pod", MachineConfigNamespace).
				Get(`{.items[*].status.conditions[?(@.type=="Ready")].status}`)

			if err != nil {
				return false, err
			}

			if strings.Contains(status, "False") {
				return false, nil
			}

			return true, nil
		})

	if waitErr != nil {
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", MachineConfigNamespace).Execute()
		return fmt.Errorf("MCO pods were deleted in namespace %s, but they did not become ready", MachineConfigNamespace)
	}

	return nil
}

// OCCreate executes "oc create -f" command using a file. No "-n" parameter is provided, so the resources will be created in the current namespace or in the namespace defined in the resources' definitions
func OCCreate(oc *exutil.CLI, fileName string) error {
	return oc.WithoutNamespace().Run("create").Args("-f", fileName).Execute()
}

// GetMCSPodNames returns a list of string containing the names of the MCS pods
func GetMCSPodNames(oc *exutil.CLI) ([]string, error) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", MachineConfigNamespace,
		"-l", "k8s-app=machine-config-server", "-o", "jsonpath={.items[*].metadata.name }").Output()
	if err != nil {
		return nil, err
	}

	if strings.Trim(output, " \n") == "" {
		return []string{}, nil
	}

	return strings.Split(output, " "), nil
}

// RotateMCSCertificates it executes the "oc adm ocp-certificates regenerate-machine-config-server-serving-cert" command in a master node
// When we execute the command in the master node we make sure that in FIPS clusters we are running the command from a FIPS enabled machine.
func RotateMCSCertificates(oc *exutil.CLI) error {
	wMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
	master := wMcp.GetNodesOrFail()[0]

	remoteAdminKubeConfig := fmt.Sprintf("/root/remoteKubeConfig-%s", exutil.GetRandomString())
	adminKubeConfig := exutil.KubeConfigPath()

	defer master.RemoveFile(remoteAdminKubeConfig)
	err := master.CopyFromLocal(adminKubeConfig, remoteAdminKubeConfig)

	if err != nil {
		return err
	}

	command := fmt.Sprintf("oc --kubeconfig=%s --insecure-skip-tls-verify adm ocp-certificates regenerate-machine-config-server-serving-cert",
		remoteAdminKubeConfig)

	logger.Infof("RUN: %s", command)
	stdout, err := master.DebugNodeWithChroot(strings.Split(command, " ")...)

	logger.Infof(stdout)

	return err
}

func GetCertificatesInfoFromPemBundle(bundleName string, pemBundle []byte) ([]CertificateInfo, error) {
	var certificatesInfo []CertificateInfo

	if pemBundle == nil {
		return nil, fmt.Errorf("Provided pem bundle is nil")
	}

	if len(pemBundle) == 0 {
		logger.Infof("Empty pem bundle")
		return certificatesInfo, nil
	}

	for {
		block, rest := pem.Decode(pemBundle)
		if block == nil {
			return nil, fmt.Errorf("failed to parse certificate PEM:\n%s", string(pemBundle))
		}

		logger.Infof("FOUND: %s", block.Type)
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("Only CERTIFICATES are expected in the bundle, but a type %s was found in it", block.Type)
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}

		certificatesInfo = append(certificatesInfo,
			CertificateInfo{
				BundleFile: bundleName,
				NotAfter:   cert.NotAfter.Format(time.RFC3339),
				NotBefore:  cert.NotBefore.Format(time.RFC3339),
				Signer:     cert.Issuer.String(),
				Subject:    cert.Subject.String(),
			},
		)

		pemBundle = rest
		if len(rest) == 0 {
			break
		}

	}
	return certificatesInfo, nil
}

// GetImageRegistryCertificates returns a map with the image registry certificates content. Key=certificate file name, Value=certificate content
func GetImageRegistryCertificates(oc *exutil.CLI) (map[string]string, error) {
	return GetDataFromConfigMap(oc.AsAdmin(), "openshift-config-managed", "image-registry-ca")
}

// GetManagedMergedTrustedImageRegistryCertificates returns a map with the merged trusted image registry certificates content. Key=certificate file name, Value=certificate content
func GetManagedMergedTrustedImageRegistryCertificates(oc *exutil.CLI) (map[string]string, error) {
	return GetDataFromConfigMap(oc.AsAdmin(), "openshift-config-managed", "merged-trusted-image-registry-ca")
}

// GetDataFromConfigMap returns a map[string]string with the information of the ".data" section of a configmap
func GetDataFromConfigMap(oc *exutil.CLI, namespace, name string) (map[string]string, error) {
	data := map[string]string{}
	cm := NewNamespacedResource(oc.AsAdmin(), "ConfigMap", namespace, name)
	dataJSON, err := cm.Get(`{.data}`)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return nil, err
	}

	return data, nil
}

// createCertificate creates a CA and returns: (path to the key used to sign the CA, path to the CA, error)
func createCA(tmpDir, caFileName string) (keyPath, caPath string, err error) {
	var (
		keyFileName = "privateKey.pem"
	)
	caPath = filepath.Join(tmpDir, caFileName)
	keyPath = filepath.Join(tmpDir, keyFileName)

	logger.Infof("Creating CA in directory %s", tmpDir)
	logger.Infof("Create key")
	keyArgs := []string{"genrsa", "-out", keyFileName, "4096"}
	cmd := exec.Command("openssl", keyArgs...)
	cmd.Dir = tmpDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Errorf(string(output))
		return "", "", err
	}

	logger.Infof("Create CA")

	caArgs := []string{"req", "-new", "-x509", "-nodes", "-days", "3600", "-key", "privateKey.pem", "-out", caFileName, "-subj", "/OU=MCO QE/CN=example.com"}

	cmd = exec.Command("openssl", caArgs...)
	cmd.Dir = tmpDir

	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Errorf(string(output))
		return "", "", err
	}

	return keyPath, caPath, nil
}

// splitCommandString splits a string taking into account double quotes and single quotes, unscaping the quotes if necessary
// Example. Split this:
//	command "with \"escaped\" double quotes" and 'with \'escaped\' single quotes' and simple params
// Result:
// - command
// - with "escaped" double quotes
// - and
// - with 'escaped' single quotes
// - and
// - simple
// - params

func splitCommandString(strCommand string) []string {
	command := []string{}
	insideDoubleQuote := false
	insideSingleQuote := false

	isSingleQuote := func(b byte) bool {
		return b == '\'' && !insideDoubleQuote
	}

	isDoubleQuote := func(b byte) bool {
		return b == '"' && !insideSingleQuote
	}

	arg := []byte{}
	for _, char := range []byte(strings.TrimSpace(strCommand)) {
		if isDoubleQuote(char) {

			// skip the first character of the quote
			if !insideDoubleQuote {
				insideDoubleQuote = true
				continue
			}
			// we are inside a quote
			// if the new double quote is scaped we unscape it and continue inside a quote
			if arg[len(arg)-1] == '\\' {
				arg[len(arg)-1] = '"'
				continue
			}

			// If there is no scaped char the we get out of the quote state ignoring the last character of the quote
			insideDoubleQuote = false
			continue

		}

		if isSingleQuote(char) {
			// skip the first character of the quote
			if !insideSingleQuote {
				insideSingleQuote = true
				continue
			}
			// we are inside a quote
			// if the new single quote is scaped we unscape it and continue inside a quote
			if arg[len(arg)-1] == '\\' {
				arg[len(arg)-1] = '\''
				continue
			}

			// If there is no scaped char the we get out of the quote state ignoring the last character of the quote
			insideSingleQuote = false
			continue

		}

		if char == ' ' && !insideDoubleQuote && !insideSingleQuote {
			command = append(command, string(arg))
			arg = []byte{}
			continue
		}

		arg = append(arg, char)
	}
	if len(arg) > 0 {
		command = append(command, string(arg))
	}

	return command

}

// GetInternalIgnitionConfigURL return the API server internal uri without any port and without the protocol
func GetAPIServerInternalURI(oc *exutil.CLI) (string, error) {
	infra := NewResource(oc, "infrastructure", "cluster")
	apiServerInternalURI, err := infra.Get(`{.status.apiServerInternalURI}`)
	if err != nil {
		return "", err
	}

	return regexp.MustCompile(`^https*:\/\/(.*):\d+$`).ReplaceAllString(strings.TrimSpace(apiServerInternalURI), `$1`), nil
}

// IsCompactOrSNOCluster returns true if the current cluster is a Compact cluster or a SNO cluster
func IsCompactOrSNOCluster(oc *exutil.CLI) bool {
	var (
		wMcp    = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcpList = NewMachineConfigPoolList(oc.AsAdmin())
	)

	return wMcp.IsEmpty() && len(mcpList.GetAllOrFail()) == 2
}

// IsInstalledWithAssistedInstaller returns true if the assisted-installer was involved in the installation of the cluster. If any error happens, it fails the test.
//
//	Remember that the agent installer is using assisted-installer for the installation too.
func IsInstalledWithAssistedInstallerOrFail(oc *exutil.CLI) bool {
	logger.Infof("Checking if the cluster was installed using assisted-installer")

	podsList := NewNamespacedResourceList(oc, "pods", "assisted-installer")
	podsList.ByLabel("app=assisted-installer-controller")

	podsList.PrintDebugCommand()

	pods, err := podsList.GetAll()
	if err != nil {
		e2e.Failf("Error checking if the cluster was installed with assisted-installer: %s", err)
	}

	return len(pods) > 0
}

func IsOnPremPlatform(platform string) bool {
	switch platform {
	case BaremetalPlatform, OvirtPlatform, OpenstackPlatform, VspherePlatform, NutanixPlatform:
		return true
	default:
		return false
	}
}

func SkipIfNotOnPremPlatform(oc *exutil.CLI) {
	platform := exutil.CheckPlatform(oc)
	if !IsOnPremPlatform(platform) {
		g.Skip(fmt.Sprintf("Current platform: %s. This test can only be execute in OnPrem platforms.", platform))
	}
}

// GetClonedResourceJSONString takes the json data of a given resource and clone it using a new name and namespace, removing unnecessary fields
// Sometimes we need to apply extra changes, in order to do so we can provide an function using the extraModifications parameter
func GetClonedResourceJSONString(res ResourceInterface, newName, newNamespace string, extraModifications func(string) (string, error)) (string, error) {
	jsonRes, err := res.GetCleanJSON()
	if err != nil {
		return "", err
	}

	jsonRes, err = sjson.Delete(jsonRes, "status")
	if err != nil {
		return "", err
	}

	jsonRes, err = sjson.Delete(jsonRes, "metadata.creationTimestamp")
	if err != nil {
		return "", err
	}
	jsonRes, err = sjson.Delete(jsonRes, "metadata.resourceVersion")
	if err != nil {
		return "", err
	}
	jsonRes, err = sjson.Delete(jsonRes, "metadata.uid")
	if err != nil {
		return "", err
	}

	jsonRes, err = sjson.Delete(jsonRes, "metadata.generation")
	if err != nil {
		return "", err
	}

	jsonRes, err = sjson.Set(jsonRes, "metadata.name", newName)
	if err != nil {
		return "", err
	}

	if newNamespace != "" {
		jsonRes, err = sjson.Set(jsonRes, "metadata.namespace", newNamespace)
		if err != nil {
			return "", err
		}
	}

	if extraModifications != nil {
		logger.Infof("Executing extra modifications")
		return extraModifications(jsonRes)
	}

	return jsonRes, nil

}

// CloneResource will clone the given resource with the new name and the new namespace. If new namespace is an empty strng, it is ignored and the namespace will not be changed.
// Sometimes we need to apply extra changes to the cloned resource before it is created, in order to do so we can provide an function using the extraModifications parameter
func CloneResource(res ResourceInterface, newName, newNamespace string, extraModifications func(string) (string, error)) (*Resource, error) {
	logger.Infof("Cloning resource %s with name %s and namespace %s", res, newName, newNamespace)

	jsonRes, err := GetClonedResourceJSONString(res, newName, newNamespace, extraModifications)
	if err != nil {
		return nil, err
	}

	if newNamespace == "" {
		newNamespace = res.GetNamespace()
	}

	filename := "cloned-" + res.GetKind() + "-" + newName + "-" + uuid.NewString()
	if newNamespace != "" {
		filename += "-namespace"
	}
	filename += ".json"

	tmpFile := generateTmpFile(res.GetOC(), filename)

	wErr := os.WriteFile(tmpFile, []byte(jsonRes), 0o644)
	if wErr != nil {
		return nil, wErr
	}

	logger.Infof("New resource created using definition file %s", tmpFile)

	_, cErr := res.GetOC().AsAdmin().WithoutNamespace().Run("create").Args("-f", tmpFile).Output()

	if cErr != nil {
		return nil, cErr
	}

	return NewNamespacedResource(res.GetOC(), res.GetKind(), newNamespace, newName), nil
}

func skipIfNoTechPreview(oc *exutil.CLI) {
	if !exutil.IsTechPreviewNoUpgrade(oc) {
		g.Skip("featureSet: TechPreviewNoUpgrade is required for this test")
	}
}
