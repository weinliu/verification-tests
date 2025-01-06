package cvo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// JSONp defines a json struct
type JSONp struct {
	Oper string      `json:"op"`
	Path string      `json:"path"`
	Valu interface{} `json:"value,omitempty"`
}

type annotationCO struct {
	name       string
	annotation map[string]string
}

// GetDeploymentsYaml dumps out deployment in yaml format in specific namespace
func GetDeploymentsYaml(oc *exutil.CLI, deploymentName string, namespace string) (string, error) {
	e2e.Logf("Dumping deployments %s from namespace %s", deploymentName, namespace)
	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", deploymentName, "-n", namespace, "-o", "yaml").Output()
	if err != nil {
		e2e.Logf("Error dumping deployments: %v", err)
		return "", err
	}
	e2e.Logf(out)
	return out, err
}

// PodExec executes a single command or a bash script in the running pod. It returns the
// command output and error if the command finished with non-zero status code or the
// command took longer than 3 minutes to run.
func PodExec(oc *exutil.CLI, script string, namespace string, podName string) (string, error) {
	var out string
	waitErr := wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		var err error
		out, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, podName, "--", "/bin/bash", "-c", script).Output()
		return true, err
	})
	return out, waitErr
}

// WaitForAlert check if an alert appears
// Return value: bool: indicate if the alert is found
// Return value: map: annotation map which contains reason and message information
// Retrun value: error: any error
func waitForAlert(oc *exutil.CLI, alertString string, interval time.Duration, timeout time.Duration, state string) (bool, map[string]string, error) {
	if len(state) > 0 {
		if state != "pending" && state != "firing" {
			return false, nil, fmt.Errorf("state %s is not supported", state)
		}
	}
	e2e.Logf("Waiting for alert %s pending or firing...", alertString)
	url, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", "openshift-monitoring",
		"route", "prometheus-k8s",
		"-o=jsonpath={.spec.host}").Output()
	if err != nil || len(url) == 0 {
		return false, nil, fmt.Errorf("error getting the hostname of route prometheus-k8s %v", err)
	}
	token, err := exutil.GetSAToken(oc)
	if err != nil || len(token) == 0 {
		return false, nil, fmt.Errorf("error getting SA token %v", err)
	}

	alertCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/v1/alerts | jq -r '.data.alerts[] | select (.labels.alertname == \"%s\")'", token, url, alertString)
	alertAnnoCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/v1/alerts | jq -r '.data.alerts[] | select (.labels.alertname == \"%s\").annotations'", token, url, alertString)
	alertStateCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/v1/alerts | jq -r '.data.alerts[] | select (.labels.alertname == \"%s\").state'", token, url, alertString)

	// Poll returns timed out waiting for the condition when timeout is reached
	count := 0
	if pollErr := wait.Poll(interval*time.Second, timeout*time.Second, func() (bool, error) {
		count++
		metrics, err := exec.Command("bash", "-c", alertCMD).Output()
		if err != nil {
			e2e.Logf("Error retrieving prometheus alert metrics: %v, retry %d...", err, count)
			return false, nil
		}
		if len(string(metrics)) == 0 {
			e2e.Logf("Prometheus alert metrics nil, retry %d...", count)
			return false, nil
		}

		if len(state) > 0 {
			alertState, err := exec.Command("bash", "-c", alertStateCMD).Output()
			if err != nil {
				return false, fmt.Errorf("error getting alert state")
			}
			if state == "pending" && string(alertState) != "pending" {
				return false, fmt.Errorf("alert state is not expected, expected pending but actual is %s", string(alertState))
			}
			if state == "firing" {
				if int(interval)*count < int(timeout) {
					if string(alertState) == "pending" {
						e2e.Logf("Prometheus alert state is pending, waiting for firing, retry %d...", count)
						return false, nil
					}
					return false, fmt.Errorf("alert state is not expected, expected pending in the waiting time window but actual is %s", string(alertState))
				} else if string(alertState) == "firing" {
					return true, nil
				} else {
					return false, fmt.Errorf("alert state is not expected, expected firing when the waiting time is reached but actual is %s", string(alertState))
				}
			}
			return true, nil
		}
		return true, nil
	}); pollErr != nil {
		return false, nil, pollErr
	}
	e2e.Logf("Alert %s found", alertString)
	annotation, err := exec.Command("bash", "-c", alertAnnoCMD).Output()
	if err != nil || len(string(annotation)) == 0 {
		return true, nil, fmt.Errorf("error getting annotation for alert %s", alertString)
	}
	var annoMap map[string]string
	if err := json.Unmarshal(annotation, &annoMap); err != nil {
		return true, nil, fmt.Errorf("error converting annotation to map for alert %s", alertString)
	}

	return true, annoMap, nil
}

// Check if operator's condition is expected until timeout or return true or an error happened.
func waitForCondition(oc *exutil.CLI, interval time.Duration, timeout time.Duration, expectedCondition string, args ...string) error {
	e2e.Logf("Checking condition for: oc %v", args)
	err := wait.Poll(interval*time.Second, timeout*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run(args[0]).Args(args[1:]...).Output()
		if err != nil {
			e2e.Logf("Checking condition error:%v", err)
			return false, err
		}
		condition := strings.Replace(string(output), "\n", "", -1)
		if strings.Compare(condition, expectedCondition) != 0 {
			e2e.Logf("Current condition is: '%s' Waiting for condition to be '%s'...", condition, expectedCondition)
			return false, nil
		}
		e2e.Logf("Current condition is: %v", condition)
		return true, nil
	})
	if err != nil {
		return err
	}
	return nil
}

// Get detail alert info by selector
func getAlert(oc *exutil.CLI, alertSelector string) map[string]interface{} {
	var alertInfo map[string]interface{}
	url, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", "openshift-monitoring",
		"route", "prometheus-k8s",
		"-o=jsonpath={.spec.host}").Output()
	if err != nil || len(url) == 0 {
		e2e.Logf("error getting the hostname of route prometheus-k8s %v", err)
		return nil
	}
	token, err := exutil.GetSAToken(oc)
	if err != nil || len(token) == 0 {
		e2e.Logf("error getting SA token %v", err)
		return nil
	}
	command := fmt.Sprintf("curl -skH \"Authorization: Bearer %s\" https://%s/api/v1/alerts"+
		" | jq -r '[.data.alerts[]|select(%s)][0]'", token, url, alertSelector)
	output, err := exec.Command("bash", "-c", command).Output()
	if err != nil {
		e2e.Logf("Getting alert error:%v for %s", err, strings.ReplaceAll(command, token[5:], "*****"))
		return nil
	}
	if len(output) == 0 {
		e2e.Logf("No alert found for %v", alertSelector)
		return nil
	}
	err = json.Unmarshal(output, &alertInfo)
	if err != nil {
		e2e.Logf("Unmarshal alert error:%v in %s for %s", err, output, strings.ReplaceAll(command, token[5:], "*****"))
		return nil
	}
	e2e.Logf("Alert found: %v", alertInfo)
	return alertInfo
}

// Get detail alert info by alertname
func getAlertByName(oc *exutil.CLI, alertName string, name string) map[string]interface{} {
	return getAlert(oc, fmt.Sprintf(".labels.alertname == \"%s\" and .labels.name == \"%s\"", alertName, name))
}

// CreateBucket creates a new bucket in the gcs
// projectID := "my-project-id"
// bucketName := "bucket-name"
// return value: error: any error
func CreateBucket(client *storage.Client, projectID, bucketName string) error {
	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	if err := client.Bucket(bucketName).Create(ctx, projectID, nil); err != nil {
		return err
	}
	return nil
}

// UploadFile uploads a gcs object
// bucket := "bucket-name"
// object := "object-name"
// return value: error: any error
func UploadFile(client *storage.Client, bucket, object, file string) error {
	// Open local file
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("os.Open: %v", err)
	}
	defer f.Close()

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()

	// Upload an object with storage.Writer.
	wc := client.Bucket(bucket).Object(object).NewWriter(ctx)
	if _, err = io.Copy(wc, f); err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("Writer.Close: %v", err)
	}
	return nil
}

// MakePublic makes a gcs object public
// bucket := "bucket-name"
// object := "object-name"
// return value: error: any error
func MakePublic(client *storage.Client, bucket, object string) error {
	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	acl := client.Bucket(bucket).Object(object).ACL()
	if err := acl.Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
		return err
	}
	return nil
}

// DeleteObject deletes the gcs object
// return value: error: any error
func DeleteObject(client *storage.Client, bucket, object string) error {
	if object == "" {
		return nil
	}

	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	o := client.Bucket(bucket).Object(object)
	if err := o.Delete(ctx); err != nil {
		return err
	}
	e2e.Logf("Object: %v deleted", object)
	return nil
}

// DeleteBucket deletes gcs bucket
// return value: error: any error
func DeleteBucket(client *storage.Client, bucketName string) error {
	if bucketName == "" {
		return nil
	}

	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	if err := client.Bucket(bucketName).Delete(ctx); err != nil {
		return err
	}
	e2e.Logf("Bucket: %v deleted", bucketName)
	return nil
}

// GenerateReleaseVersion generates a fake release version based on source release version
func GenerateReleaseVersion(oc *exutil.CLI) string {
	sourceVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-o=jsonpath={.status.desired.version}").Output()
	if err != nil {
		return ""
	}
	splits := strings.Split(sourceVersion, ".")
	if len(splits) > 1 {
		if sourceMinorNum, err := strconv.Atoi(splits[1]); err == nil {
			targeMinorNum := sourceMinorNum + 1
			splits[1] = strconv.Itoa(targeMinorNum)
			return strings.Join(splits, ".")
		}
	}
	return ""
}

// GenerateReleasePayload generates a fake release payload based on source release payload by default
func GenerateReleasePayload(oc *exutil.CLI) string {
	var targetDigest string
	sourcePayload, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-o=jsonpath={.status.desired.image}").Output()
	if err != nil {
		return ""
	}
	data := make([]byte, 10)
	if _, err := rand.Read(data); err == nil {
		sh256Bytes := sha256.Sum256(data)
		targetDigest = hex.EncodeToString(sh256Bytes[:])
	} else {
		return ""
	}

	splits := strings.Split(sourcePayload, ":")
	if len(splits) > 1 {
		splits[1] = targetDigest
		return strings.Join(splits, ":")
	}
	return ""
}

// updateGraph updates the cincy.json
// return value: string: graph json filename
// return value: string: target version
// return value: string: target payload
// return value: error: any error
func updateGraph(oc *exutil.CLI, graphName string) (string, string, string, error) {
	graphDataDir := exutil.FixturePath("testdata", "ota/cvo")
	graphTemplate := filepath.Join(graphDataDir, graphName)

	e2e.Logf("Graph Template: %v", graphTemplate)

	// Assume the cluster is not being upgraded, then desired version will be the current version
	sourceVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-o=jsonpath={.status.desired.version}").Output()
	if err != nil {
		return "", "", "", err
	}
	sourcePayload, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-o=jsonpath={.status.desired.image}").Output()
	if err != nil {
		return "", "", "", err
	}

	targetVersion := GenerateReleaseVersion(oc)
	if targetVersion == "" {
		return "", "", "", fmt.Errorf("error get target version")
	}
	targetPayload := GenerateReleasePayload(oc)
	if targetPayload == "" {
		return "", "", "", fmt.Errorf("error get target payload")
	}

	// Give the new graph a unique name
	// graphFile := fmt.Sprintf("cincy-%d", time.Now().Unix())

	sedCmd := fmt.Sprintf("sed -i -e 's|sourceversion|%s|g; s|sourcepayload|%s|g; s|targetversion|%s|g; s|targetpayload|%s|g' %s", sourceVersion, sourcePayload, targetVersion, targetPayload, graphTemplate)
	//fmt.Println(sedCmd)
	if err := exec.Command("bash", "-c", sedCmd).Run(); err == nil {
		return graphTemplate, targetVersion, targetPayload, nil
	}
	e2e.Logf("Error on sed command: %v", err.Error())
	return "", "", "", err
}

// buildGraph creates a gcs bucket, upload the graph file and make it public for CVO to use
// projectID := "projectID"
// return value: string: the public url of the object
// return value: string: the bucket name
// return value: string: the object name
// return value: string: the target version
// return value: string: the target payload
// return value: error: any error
func buildGraph(client *storage.Client, oc *exutil.CLI, projectID string, graphName string) (
	url string, bucket string, object string, targetVersion string, targetPayload string, err error) {
	var graphFile string
	var resp *http.Response
	var body []byte

	if graphFile, targetVersion, targetPayload, err = updateGraph(oc, graphName); err != nil {
		return
	}
	e2e.Logf("Graph file: %v updated", graphFile)

	// Give the bucket a unique name
	bucket = fmt.Sprintf("ocp-ota-%d", time.Now().Unix())
	if err = CreateBucket(client, projectID, bucket); err != nil {
		return
	}
	e2e.Logf("Bucket: %v created", bucket)

	// Give the object a unique name
	object = fmt.Sprintf("graph-%d", time.Now().Unix())
	if err = UploadFile(client, bucket, object, graphFile); err != nil {
		return
	}
	e2e.Logf("Object: %v uploaded", object)

	// Make the object public
	if err = MakePublic(client, bucket, object); err != nil {
		return
	}
	e2e.Logf("Object: %v public", object)

	url = fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, object)
	// testing endpoint accessible and logging graph contents
	if resp, err = http.Get(url); err == nil {
		defer resp.Body.Close()
		if body, err = io.ReadAll(resp.Body); err == nil {
			e2e.Logf(string(body))
		}
	}
	return
}

// restoreCVSpec restores upstream and channel of clusterversion
// if no need to restore, pass "nochange" to the argument(s)
func restoreCVSpec(upstream string, channel string, oc *exutil.CLI) {
	e2e.Logf("Restoring upgrade graph to '%s' channel to '%s'", upstream, channel)
	if channel != "nochange" {
		_ = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "channel", "--allow-explicit-channel", channel).Execute()
		time.Sleep(5 * time.Second)
		currChannel, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[].spec.channel}").Output()
		if currChannel != channel {
			e2e.Logf("Error on channel recovery, expected %s, but got %s", channel, currChannel)
		}
	}

	if upstream != "nochange" {
		if upstream == "" {
			_ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterversion/version", "--type=json", "-p", "[{\"op\":\"remove\", \"path\":\"/spec/upstream\"}]").Execute()
		} else {
			_ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterversion/version", "--type=merge", "--patch", fmt.Sprintf("{\"spec\":{\"upstream\":\"%s\"}}", upstream)).Execute()
		}
		currUpstream, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[].spec.upstream}").Output()
		if currUpstream != upstream {
			e2e.Logf("Error on upstream recovery, expected %s, but got %s", upstream, currUpstream)
		}
	}
}

// Run "oc adm release extract" cmd to extract manifests from current live cluster
func extractManifest(oc *exutil.CLI) (tempDataDir string, err error) {
	tempDataDir = filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))

	if err = os.Mkdir(tempDataDir, 0755); err != nil {
		err = fmt.Errorf("failed to create directory: %v", err)
		return
	}

	if err = oc.AsAdmin().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--confirm", "--to="+tempDataDir).Execute(); err != nil {
		err = fmt.Errorf("failed to extract dockerconfig: %v", err)
		return
	}

	manifestDir := filepath.Join(tempDataDir, "manifest")
	if err = oc.AsAdmin().Run("adm").Args("release", "extract", "--to", manifestDir, "-a", tempDataDir+"/.dockerconfigjson").Execute(); err != nil {
		e2e.Logf("warning: release extract failed once with:\n\"%v\"", err)

		//Workaround disconnected baremental clusters that don't have cert for the registry
		platform := exutil.CheckPlatform(oc)
		if strings.Contains(platform, "baremetal") || strings.Contains(platform, "none") {
			var mirror_registry string
			mirror_registry, err = exutil.GetMirrorRegistry(oc)
			if mirror_registry != "" {
				if err != nil {
					err = fmt.Errorf("error out getting mirror registry: %v", err)
					return
				}
				if err = oc.AsAdmin().Run("adm").Args("release", "extract", "--insecure", "--to", manifestDir, "-a", tempDataDir+"/.dockerconfigjson").Execute(); err != nil {
					err = fmt.Errorf("warning: insecure release extract for disconnected baremetal failed with:\n\"%v\"", err)
				}
				return
			}
		}

		//Workaround c2s/cs2s clusters that only have token to the mirror in pull secret
		var region, image, mirror string
		if region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure",
			"cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output(); err != nil {
			err = fmt.Errorf("failed to get cluster region: %v", err)
			return
		}

		// region us-iso-* represent C2S, us-isob-* represent SC2S
		if !strings.Contains(region, "us-iso-") && !strings.Contains(region, "us-isob-") {
			err = fmt.Errorf("oc adm release failed, and no retry for non-c2s/cs2s region: %s", region)
			return
		}

		if image, err = exutil.GetReleaseImage(oc); err != nil {
			err = fmt.Errorf("failed to get cluster release image: %v", err)
			return
		}

		if mirror, err = oc.AsAdmin().Run("get").Args("ImageContentSourcePolicy",
			"-o", "jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}").Output(); err != nil {
			err = fmt.Errorf("failed to acquire mirror from ICSP: %v", err)
			return
		}

		if err = oc.AsAdmin().Run("adm").Args("release", "extract",
			"--from", fmt.Sprintf("%s@%s", mirror, strings.Split(image, "@")[1]),
			"--to", manifestDir, "-a", tempDataDir+"/.dockerconfigjson", "--insecure").Execute(); err != nil {
			err = fmt.Errorf("failed to extract manifests: %v", err)
			return
		}
	}
	return
}

// Run "oc adm release extract --included --install-config" cmd to extract manifests
func extractIncludedManifestWithInstallcfg(oc *exutil.CLI, creds bool, cfg string, image string, cloud string) (tempDataDir string, err error) {
	tempDataDir = filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))
	var out string
	if err = os.Mkdir(tempDataDir, 0755); err != nil {
		err = fmt.Errorf("failed to create directory: %v", err)
		return
	}
	if creds && cloud != "" {
		out, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--install-config", cfg, "--included", "--credentials-requests", "--cloud", cloud, "--from", image, "--to", tempDataDir).Output()
	} else if creds {
		out, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--install-config", cfg, "--included", "--credentials-requests", "--from", image, "--to", tempDataDir).Output()
	} else if cloud != "" {
		err = fmt.Errorf("--cloud only works with --credentials-requests,creds_var: %v,cloud_var: %v", creds, cloud)
	} else {
		out, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--install-config", cfg, "--included", "--from", image, "--to", tempDataDir).Output()
	}
	if err != nil {
		err = fmt.Errorf("failed to extract manifest: %v, command output:%v", err, out)
		return
	}
	return
}

func getDefaultCapsInCR(version string) []string {
	switch version {
	case "4.19":
		return []string{"CloudCredential", "CloudCredential+CloudControllerManager", "CloudCredential+Ingress", "MachineAPI+CloudCredential", "ImageRegistry+CloudCredential", "Storage+CloudCredential"}
	case "4.18":
		return []string{"CloudCredential", "CloudCredential+CloudControllerManager", "CloudCredential+Ingress", "MachineAPI+CloudCredential", "ImageRegistry+CloudCredential", "Storage+CloudCredential"}
	case "4.17":
		return []string{"CloudCredential", "CloudCredential+CloudControllerManager", "CloudCredential+Ingress", "MachineAPI+CloudCredential", "ImageRegistry+CloudCredential", "Storage+CloudCredential"}
	case "4.16":
		return []string{"CloudCredential", "CloudCredential+CloudControllerManager", "CloudCredential+Ingress", "MachineAPI+CloudCredential", "ImageRegistry+CloudCredential", "Storage+CloudCredential"}
	case "4.15":
		return []string{"CloudCredential", "MachineAPI+CloudCredential", "ImageRegistry+CloudCredential", "Storage+CloudCredential"}
	case "4.14":
		return []string{"Storage", "MachineAPI"}
	default:
		e2e.Logf("Unknown version:%s detected!", version)
		return nil
	}
}

func getRandomPlatform() string {
	types := [...]string{"aws", "azure", "gcp", "vsphere"}
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	index := seed.Intn(len(types) - 1)
	return types[index]
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

// get clusterversion version object values by jsonpath.
// Returns: object_value(string), error
func getCVObyJP(oc *exutil.CLI, jsonpath string) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").
		Args("clusterversion", "version",
			"-o", fmt.Sprintf("jsonpath={%s}", jsonpath)).Output()
}

// find argument index in CVO container args in deployment (by arg name).
// Returns: arg_value(string), arg_index(int), error
func getCVOcontArg(oc *exutil.CLI, argQuery string) (string, int, error) {
	depArgs, err := oc.AsAdmin().WithoutNamespace().Run("get").
		Args("-n", "openshift-cluster-version",
			"deployment", "cluster-version-operator",
			"-o", "jsonpath={.spec.template.spec.containers[0].args}").Output()
	if err != nil {
		e2e.Logf("Error getting cvo deployment args: %v", err)
		return "", -1, err
	}

	var result []string
	err = json.Unmarshal([]byte(depArgs), &result)
	if err != nil {
		e2e.Logf("Error Unmarshal cvo deployment args: %v", err)
		return "", -1, err
	}

	for index, arg := range result {
		if strings.Contains(arg, argQuery) {
			e2e.Logf("query '%s' found '%s' at Index: %d", argQuery, arg, index)
			val := strings.Split(arg, "=")
			if len(val) > 1 {
				return val[1], index, nil
			}
			return val[0], index, nil
		}
	}
	return "", -1, fmt.Errorf("error: cvo deployment arg %s not found", argQuery)
}

// patch resource (namespace - use "" if none, resource_name, patch).
// Returns: result(string), error
func ocJSONPatch(oc *exutil.CLI, namespace string, resource string, patch []JSONp) (patchOutput string, err error) {
	p, err := json.Marshal(patch)
	if err != nil {
		e2e.Logf("ocJSONPatch Error - json.Marshal: '%v'", err)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	if namespace != "" {
		patchOutput, err = oc.AsAdmin().WithoutNamespace().Run("patch").
			Args("-n", namespace, resource, "--type=json", "--patch", string(p)).Output()
	} else {
		patchOutput, err = oc.AsAdmin().WithoutNamespace().Run("patch").
			Args(resource, "--type=json", "--patch", string(p)).Output()
	}
	e2e.Logf("patching '%s'\nwith '%s'\nresult '%s'", resource, string(p), patchOutput)
	return
}

// patch CVO container argument (arg_index, arg_value)
// Returns: result(string), error
func patchCVOcontArg(oc *exutil.CLI, index int, value string) (string, error) {
	patch := []JSONp{
		{"replace",
			fmt.Sprintf("/spec/template/spec/containers/0/args/%d", index),
			value},
	}
	return ocJSONPatch(oc,
		"openshift-cluster-version",
		"deployment/cluster-version-operator",
		patch)
}

// Get updates by using "oc adm upgrade ..." command in the given timeout
// Check expStrings in the result of the updates
// Returns: true - found, false - not found
func checkUpdates(oc *exutil.CLI, conditional bool, interval time.Duration, timeout time.Duration, expStrings ...string) bool {
	var (
		cmdOut string
		err    error
	)

	if pollErr := wait.Poll(interval*time.Second, timeout*time.Second, func() (bool, error) {
		if conditional {
			cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--include-not-recommended").Output()
		} else {
			cmdOut, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
		}
		for _, str := range expStrings {
			if !strings.Contains(cmdOut, str) || err != nil {
				return false, err
			}
		}
		return true, nil
	}); pollErr != nil {
		e2e.Logf("last oc adm upgrade returned:\n%s\nstderr: %v\nexpecting:\n%s\n", cmdOut, err, strings.Join(expStrings, "\n\n"))
		return false
	}
	return true
}

// change the spec.capabilities
// if base==true, change the baselineCapabilitySet, otherwise, change the additionalEnabledCapabilities
func changeCap(oc *exutil.CLI, base bool, cap interface{}) (string, error) {
	var spec string
	if base {
		spec = "/spec/capabilities/baselineCapabilitySet"
	} else {
		spec = "/spec/capabilities/additionalEnabledCapabilities"
	}
	if cap == nil {
		return ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"remove", spec, nil}})
	}
	// if spec.capabilities is not present, patch to add capabilities
	orgCap, err := getCVObyJP(oc, ".spec.capabilities")
	if err != nil {
		return "", err
	}
	if orgCap == "" {
		value := make(map[string]interface{})
		_, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", "/spec/capabilities", value}})
		if err != nil {
			return "", err
		}
	}
	return ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"add", spec, cap}})
}

// verifies that the capabilities list passed to this func have resources enabled in a cluster
func verifyCaps(oc *exutil.CLI, caps []string) (err error) {
	// Important! this map should be updated each version with new capabilities, as they added to openshift.
	capability_operators := map[string]string{
		"baremetal":                "baremetal",
		"Console":                  "console",
		"Insights":                 "insights",
		"marketplace":              "marketplace",
		"Storage":                  "storage",
		"openshift-samples":        "openshift-samples",
		"CSISnapshot":              "csi-snapshot-controller",
		"NodeTuning":               "node-tuning",
		"MachineAPI":               "machine-api",
		"Build":                    "build",
		"DeploymentConfig":         "dc",
		"ImageRegistry":            "image-registry",
		"OperatorLifecycleManager": "operator-lifecycle-manager",
		"CloudCredential":          "cloud-credential",
		"Ingress":                  "ingress",
		"CloudControllerManager":   "cloud-controller-manager",
	}
	for _, cap := range caps {
		prefix := "co"
		if cap == "Build" || cap == "DeploymentConfig" {
			prefix = "-A" // special case for caps that isn't co but a resource
		}
		// if there's a new cap missing in capability_operators - return error
		if capability_operators[cap] == "" {
			return fmt.Errorf("new unknown capability '%v'. please update automation: capability_operators in utils.go", cap)
		}
		if _, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(prefix, capability_operators[cap]).Output(); err != nil {
			return
		}
	}
	return
}

// waits for string 'message' to appear in CVO 'jsonpath'.
// or waits for message to disappear if waitingToAppear=false.
// returns error if any.
func waitForCVOStatus(oc *exutil.CLI, interval time.Duration, timeout time.Duration, message string, jsonpath string, waitingToAppear bool) (err error) {
	var prefix, out string
	if !waitingToAppear {
		prefix = "not "
	}
	e2e.Logf("Waiting for CVO '%s' %sto contain '%s'", jsonpath, prefix, message)
	err = wait.Poll(interval*time.Second, timeout*time.Second, func() (bool, error) {
		out, err = getCVObyJP(oc, jsonpath)
		return strings.Contains(out, message) == waitingToAppear, err
	})
	if err != nil {
		if strings.Compare(err.Error(), "timed out waiting for the condition") == 0 {
			out, _ = getCVObyJP(oc, ".status.conditions")
			err = fmt.Errorf("reached time limit of %s waiting for CVO %s %sto contain '%s', dumping conditions:\n%s",
				timeout*time.Second, strings.NewReplacer(".status.conditions[?(.type=='", "", "')].", " ").Replace(jsonpath), prefix, message, out)
			return
		}
		err = fmt.Errorf("while waiting for CVO %sto contain '%s', an error was received: %s %s", prefix, message, out, err.Error())
		e2e.Logf(err.Error())
	}
	return
}

func setCVOverrides(oc *exutil.CLI, resourceKind string, resourceName string, resourceNamespace string) (err error) {
	type ovrd struct {
		Ki string `json:"kind"`
		Na string `json:"name"`
		Ns string `json:"namespace"`
		Un bool   `json:"unmanaged"`
		Gr string `json:"group"`
	}
	var ovPatch string
	if ovPatch, err = ocJSONPatch(oc, "", "clusterversion/version", []JSONp{
		{"add", "/spec/overrides", []ovrd{{resourceKind, resourceName, resourceNamespace, true, "apps"}}}}); err != nil {
		return fmt.Errorf("patching /spec/overrides failed with: %s %v", ovPatch, err)
	}

	// upgradeable .reason may be ClusterVersionOverridesSet or MultipleReasons, but .message have to contain "overrides"
	e2e.Logf("Waiting for Upgradeable to contain overrides message...")
	if err = waitForCVOStatus(oc, 30, 8*60,
		"Disabling ownership via cluster version overrides prevents upgrades",
		".status.conditions[?(.type=='Upgradeable')].message", true); err != nil {
		return
	}

	e2e.Logf("Waiting for ClusterVersionOverridesSet in oc adm upgrade...")
	if !checkUpdates(oc, false, 30, 8*60, "ClusterVersionOverridesSet") {
		return fmt.Errorf("no overrides message in oc adm upgrade within 8m")
	}

	e2e.Logf("Waiting for Progressing=false...")
	//to workaround the fake upgrade by cv.overrrides, refer to https://issues.redhat.com/browse/OTA-586
	err = waitForCVOStatus(oc, 30, 8*60, "False",
		".status.conditions[?(.type=='Progressing')].status", true)
	return
}

func unsetCVOverrides(oc *exutil.CLI) {
	e2e.Logf("Unset /spec/overrides...")
	_, err := ocJSONPatch(oc, "", "clusterversion/version", []JSONp{{"remove", "/spec/overrides", nil}})
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Waiting overrides to disappear from cluster conditions...")
	err = waitForCVOStatus(oc, 30, 8*60,
		"Disabling ownership via cluster version overrides prevents upgrades",
		".status.conditions[?(.type=='Upgradeable')].message", false)
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Check no ClusterVersionOverridesSet in `oc adm upgrade` msg...")
	upgStatusOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(upgStatusOutput).NotTo(o.ContainSubstring("ClusterVersionOverridesSet"))
}

// Check if a non-namespace resource existed
func isGlobalResourceExist(oc *exutil.CLI, resourceType string) bool {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resourceType).Output()
	o.Expect(err).NotTo(o.HaveOccurred(), "fail to get resource %s", resourceType)
	if strings.Contains(output, "No resources found") {
		e2e.Logf("there is no %s in this cluster!", resourceType)
		return false
	}
	return true
}

// Check ICSP or IDMS to get mirror registry info
func getMirrorRegistry(oc *exutil.CLI) (registry string, err error) {
	if isGlobalResourceExist(oc, "ImageContentSourcePolicy") {
		if registry, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy",
			"-o", "jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}").Output(); err == nil {
			registry, _, _ = strings.Cut(registry, "/")
		} else {
			err = fmt.Errorf("failed to acquire mirror registry from ICSP: %v", err)
		}
		return
	} else if isGlobalResourceExist(oc, "ImageDigestMirrorSet") {
		if registry, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageDigestMirrorSet",
			"-o", "jsonpath={.items[0].spec.imageDigestMirrors[0].mirrors[0]}").Output(); err == nil {
			registry, _, _ = strings.Cut(registry, "/")
		} else {
			err = fmt.Errorf("failed to acquire mirror registry from IDMS: %v", err)
		}
		return
	} else {
		err = fmt.Errorf("no ICSP or IDMS found!")
		return
	}
}

// Run "oc adm release info" cmd to get release info of the current release
func getReleaseInfo(oc *exutil.CLI) (output string, err error) {
	tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))
	err = os.Mkdir(tempDataDir, 0755)
	defer os.RemoveAll(tempDataDir)
	if err != nil {
		err = fmt.Errorf("failed to create tempdir %s: %v", tempDataDir, err)
		return
	}

	if err = oc.AsAdmin().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--confirm", "--to="+tempDataDir).Execute(); err != nil {
		err = fmt.Errorf("failed to extract dockerconfig: %v", err)
		return
	}

	if output, err = oc.AsAdmin().Run("adm").Args("release", "info", "-a", tempDataDir+"/.dockerconfigjson", "-ojson").Output(); err != nil {
		e2e.Logf("warning: release info failed once with:\n\"%v\"", err)

		//Workaround disconnected baremental clusters that don't have cert for the registry
		platform := exutil.CheckPlatform(oc)
		if strings.Contains(platform, "baremetal") || strings.Contains(platform, "none") {
			var mirror_registry string
			mirror_registry, err = getMirrorRegistry(oc)
			if mirror_registry != "" {
				if err != nil {
					err = fmt.Errorf("error out getting mirror registry: %v", err)
					return
				}
				if err = oc.AsAdmin().Run("adm").Args("release", "info", "--insecure", "-a", tempDataDir+"/.dockerconfigjson", "-ojson").Execute(); err != nil {
					err = fmt.Errorf("warning: insecure release info for disconnected baremetal failed with:\n\"%v\"", err)
				}
				return
			}
		}

		//Workaround c2s/cs2s clusters that only have token to the mirror in pull secret
		var region, image, mirror string
		if region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure",
			"cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output(); err != nil {
			err = fmt.Errorf("failed to get cluster region: %v", err)
			return
		}

		// region us-iso-* represent C2S, us-isob-* represent SC2S
		if !strings.Contains(region, "us-iso-") && !strings.Contains(region, "us-isob-") {
			err = fmt.Errorf("oc adm release failed, and no retry for non-c2s/cs2s region: %s", region)
			return
		}

		if image, err = exutil.GetReleaseImage(oc); err != nil {
			err = fmt.Errorf("failed to get cluster release image: %v", err)
			return
		}

		if mirror, err = oc.AsAdmin().Run("get").Args("ImageContentSourcePolicy",
			"-o", "jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}").Output(); err != nil {
			err = fmt.Errorf("failed to acquire mirror from ICSP: %v", err)
			return
		}

		if output, err = oc.AsAdmin().Run("adm").Args("release", "info",
			"--insecure", "-a", tempDataDir+"/.dockerconfigjson",
			fmt.Sprintf("%s@%s", mirror, strings.Split(image, "@")[1])).Output(); err != nil {
			err = fmt.Errorf("failed to get release info: %v", err)
			return
		}
	}
	return
}

// Get CVO pod object values by jsonpath
// Returns: object_value(map), error
func getCVOPod(oc *exutil.CLI, jsonpath string) (map[string]interface{}, error) {
	var objectValue map[string]interface{}
	pod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-cluster-version", "-o=jsonpath={.items[].metadata.name}").Output()
	if err != nil {
		return nil, fmt.Errorf("getting CVO pod name failed: %v", err)
	}
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").
		Args("pod", pod, "-n", "openshift-cluster-version",
			"-o", fmt.Sprintf("jsonpath={%s}", jsonpath)).Output()
	if err != nil {
		return nil, fmt.Errorf("getting CVO pod object values failed: %v", err)
	}
	err = json.Unmarshal([]byte(output), &objectValue)
	if err != nil {
		return nil, fmt.Errorf("unmarshal release info error: %v", err)
	}
	return objectValue, nil
}

// clearing fake upgrade and waiting for ReleaseAccepted recovery
func recoverReleaseAccepted(oc *exutil.CLI) (err error) {
	var out string
	if out, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("upgrade", "--clear").Output(); err != nil {
		err = fmt.Errorf("clearing upgrade failed with: %s\n%v", out, err)
		e2e.Logf(err.Error())
		return err
	}
	if err = waitForCondition(oc, 30, 480, "True",
		"get", "clusterversion", "version", "-o", "jsonpath={.status.conditions[?(@.type=='ReleaseAccepted')].status}"); err != nil {
		if strings.Compare(err.Error(), "timed out waiting for the condition") == 0 {
			err = fmt.Errorf("ReleaseAccepted condition is not back to True within 8m")
		} else {
			err = fmt.Errorf("waiting for ReleaseAccepted returned error: %s", err.Error())
		}
		e2e.Logf(err.Error())
	}
	return err
}

func getTargetPayload(oc *exutil.CLI, imageType string) (releasePayload string, err error) {
	switch imageType {
	case "stable":
		latest4StableImage, err := exutil.GetLatest4StableImage()
		if err != nil {
			return "", err
		}
		imageInfo, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", latest4StableImage, "-ojson").Output()
		if err != nil {
			return "", err
		}
		imageDigest := gjson.Get(imageInfo, "digest").String()
		return fmt.Sprintf("quay.io/openshift-release-dev/ocp-release@%s", imageDigest), nil
	case "nightly":
		clusterVersion, _, err := exutil.GetClusterVersion(oc)
		if err != nil {
			return "", err
		}
		latest4NightlyImage, err := exutil.GetLatestNightlyImage(clusterVersion)
		if err != nil {
			return "", err
		}
		tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))
		err = os.Mkdir(tempDataDir, 0755)
		defer os.RemoveAll(tempDataDir)
		if err != nil {
			return "", err
		}
		err = exutil.GetPullSec(oc, tempDataDir)
		if err != nil {
			return "", err
		}
		authFile := tempDataDir + "/.dockerconfigjson"
		imageInfo, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "-a", authFile, latest4NightlyImage, "-ojson").Output()
		if err != nil {
			return "", err
		}
		imageDigest := gjson.Get(imageInfo, "digest").String()
		return fmt.Sprintf("registry.ci.openshift.org/ocp/release@%s", imageDigest), nil
	default:
		return "", fmt.Errorf("unrecognized imageType")
	}
}

// included==true, means check expected string should be included in events
// included==false, means check expected string should not be included in events
func checkCVOEvents(oc *exutil.CLI, included bool, expected []string) (err error) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", "openshift-cluster-version").Output()
	if err != nil {
		return err
	}
	e2e.Logf("the cvo event: %s", output)
	if included {
		for _, exp := range expected {
			matched, _ := regexp.MatchString(exp, output)
			if !matched {
				return fmt.Errorf("msg: %s is not found in events", exp)
			}
		}
	} else {
		for _, exp := range expected {
			matched, _ := regexp.MatchString(exp, output)
			if matched {
				return fmt.Errorf("msg: %s is found in events", exp)
			}
		}
	}
	return nil
}
