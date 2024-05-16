package osus

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type operatorGroup struct {
	name      string
	namespace string
	template  string
}

type subscription struct {
	name            string
	namespace       string
	channel         string
	approval        string
	operatorName    string
	sourceName      string
	sourceNamespace string
	startingCSV     string
	template        string
}

type resource struct {
	oc               *exutil.CLI
	asAdmin          bool
	withoutNamespace bool
	kind             string
	name             string
	requireNS        bool
	namespace        string
}

type updateService struct {
	name      string
	namespace string
	graphdata string
	releases  string
	template  string
	replicas  int
}

type supportedMap struct {
	osusver string
	ocpver  []string
}

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var cfgFileJson string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "osus-resource-cfg.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		cfgFileJson = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", cfgFileJson)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", cfgFileJson).Execute()
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

func (og *operatorGroup) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (sub *subscription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"APPROVAL="+sub.approval, "OPERATORNAME="+sub.operatorName, "SOURCENAME="+sub.sourceName, "SOURCENAMESPACE="+sub.sourceNamespace, "STARTINGCSV="+sub.startingCSV)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func removeResource(oc *exutil.CLI, parameters ...string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(parameters...).Output()
	if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
		e2e.Logf("No resource found!")
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (og *operatorGroup) delete(oc *exutil.CLI) {
	removeResource(oc, "-n", og.namespace, "operatorgroup", og.name)
}

func (sub *subscription) delete(oc *exutil.CLI) {
	removeResource(oc, "-n", sub.namespace, "subscription", sub.name)
}

func (us *updateService) create(oc *exutil.CLI) (err error) {
	err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", us.template, "-p", "NAME="+us.name, "NAMESPACE="+us.namespace, "GRAPHDATA="+us.graphdata, "RELEASES="+us.releases, "REPLICAS="+strconv.Itoa(us.replicas))
	return
}

// Check if pod is running
func waitForPodReady(oc *exutil.CLI, pod string, ns string) {
	e2e.Logf("Waiting for %s pod creating...", pod)
	pollErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		cmdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector="+pod, "-n", ns).Output()
		if err != nil || strings.Contains(cmdOut, "No resources found") {
			e2e.Logf("error: %v, keep trying!", err)
			return false, nil
		}
		return true, nil
	})

	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("pod with name=%s is not found", pod))

	e2e.Logf("Waiting for %s pod ready and running...", pod)
	pollErr = wait.Poll(30*time.Second, 600*time.Second, func() (bool, error) {
		stateOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector="+pod, "-n", ns, "-o=jsonpath={.items[*].status.phase}").Output()
		if err != nil {
			e2e.Logf("pod phase status: %s with error %v, try again", stateOut, err)
			return false, nil
		}
		state := strings.Split(stateOut, " ")
		for _, s := range state {
			if strings.Compare(s, "Running") != 0 {
				e2e.Logf("pod status: %s, try again", s)
				return false, nil
			}
		}
		readyOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector="+pod, "-n", ns, "-o=jsonpath={.items[*].status.conditions[?(@.type==\"Ready\")].status}").Output()
		if err != nil {
			e2e.Logf("pod ready condition: %s with error %v, try again", readyOut, err)
			return false, nil
		}
		ready := strings.Split(readyOut, " ")
		for _, s := range ready {
			if strings.Compare(s, "True") != 0 {
				e2e.Logf("pod ready condition: %s, try again", s)
				return false, nil
			}
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("pod %s is not running", pod))
}

func copyFile(source string, dest string) {
	bytesRead, err := ioutil.ReadFile(source)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = ioutil.WriteFile(dest, bytesRead, 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Set ENV for oc-mirror credential
func locatePodmanCred(oc *exutil.CLI, dst string) (dirname string, err error) {
	e2e.Logf("Setting env for oc-mirror credential")
	if err = exutil.GetPullSec(oc, dst); err != nil {
		return "", fmt.Errorf("extract pull-secret failed: %v", err)
	}
	if os.Getenv("CLUSTER_PROFILE_DIR") != "" {
		cmd := fmt.Sprintf("jq -s '.[0]*.[1]' %s %s > %s", dst+"/.dockerconfigjson", os.Getenv("CLUSTER_PROFILE_DIR")+"/pull-secret", dst+"/auth.json")
		if _, err = exec.Command("bash", "-c", cmd).CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s failed: %v", cmd, err)
		}
	} else {
		copyFile(dst+"/.dockerconfigjson", dst+"/auth.json")
	}
	envDir := filepath.Join("/tmp/", fmt.Sprintf("ota-%s", getRandomString()))
	containerDir := envDir + "/containers/"
	key := "XDG_RUNTIME_DIR"
	currentRuntime, ex := os.LookupEnv(key)
	if !ex {
		if err = os.MkdirAll(containerDir, 0700); err != nil {
			return "", fmt.Errorf("make dir failed: %v", err)
		}
		os.Setenv(key, envDir)
		copyFile(dst+"/auth.json", containerDir+"auth.json")
		return containerDir, nil
	}
	runtimeContainerDir := currentRuntime + "/containers/"
	_, err = os.Stat(runtimeContainerDir + "auth.json")
	if os.IsNotExist(err) {
		if err = os.MkdirAll(runtimeContainerDir, 0700); err != nil {
			return "", fmt.Errorf("make dir failed: %v", err)
		}
		copyFile(dst+"/auth.json", runtimeContainerDir+"auth.json")
	}
	return runtimeContainerDir, nil
}

// Mirror OCP release and graph data image to local registry
// Return the output direcotry which contains the manifests
func ocmirror(oc *exutil.CLI, registry string, dirname string, imageset string) (string, error) {
	var imagesetTemplate string
	if imageset == "" {
		imagesetTemplate = exutil.FixturePath("testdata", "ota", "osus", "imageset-config.yaml")
	} else {
		imagesetTemplate = imageset
	}
	sedCmd := fmt.Sprintf("sed -i 's|REGISTRY|%s|g' %s", registry, imagesetTemplate)
	// e2e.Logf(sedCmd)
	if err := exec.Command("bash", "-c", sedCmd).Run(); err != nil {
		e2e.Logf("Update the imageset template failed: %v", err.Error())
		return "", err
	}
	// file, _ := os.Open(imagesetTemplate)
	// b, _ := ioutil.ReadAll(file)
	// e2e.Logf(string(b))

	if err := os.Chdir(dirname); err != nil {
		e2e.Logf("Failed to cd %s: %v", dirname, err.Error())
		return "", err
	}
	output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imagesetTemplate, "--ignore-history", "docker://"+registry, "--dest-skip-tls").Output()
	if err != nil {
		e2e.Logf("Mirror images failed: %v", err.Error())
		return "", err
	}
	e2e.Logf("output of oc-mirror is %s", output)
	substrings := strings.Split(output, " ")
	outdir := dirname + "/" + substrings[len(substrings)-1]
	return outdir, nil
}

// Check if image-registry is healthy
func checkCOHealth(oc *exutil.CLI, co string) bool {
	e2e.Logf("Checking CO %s is healthy...", co)
	status := "TrueFalseFalse"
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", co, "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	if err != nil {
		e2e.Logf("Get co status failed: %v", err.Error())
		return false
	}
	return strings.Contains(output, status)
}

// Configure the Registry Certificate as trusted for cincinnati
func trustCert(oc *exutil.CLI, registry string, cert string) (err error) {
	var output string
	certRegistry := registry
	before, after, found := strings.Cut(registry, ":")
	if found {
		certRegistry = before + ".." + after
	}

	if err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-config", "configmap", "trusted-ca", "--from-file="+certRegistry+"="+cert, "--from-file=updateservice-registry="+cert).Execute(); err != nil {
		err = fmt.Errorf("create trust-ca configmap failed: %v", err)
		return
	}
	if err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config.openshift.io/cluster", "-p", `{"spec": {"additionalTrustedCA": {"name": "trusted-ca"}}}`, "--type=merge").Execute(); err != nil {
		err = fmt.Errorf("patch image.config.openshift.io/cluster failed: %v", err)
		return
	}
	waitErr := wait.Poll(30*time.Second, 10*time.Minute, func() (bool, error) {
		registryHealth := checkCOHealth(oc, "image-registry")
		if registryHealth {
			return true, nil
		}
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].message}").Output()
		e2e.Logf("Waiting for image-registry coming ready...")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Image registry is not ready with info %s\n", output))
	return nil
}

// Install OSUS instance using manifests generated by oc-mirror
func installOSUSAppOCMirror(oc *exutil.CLI, outdir string) (err error) {
	e2e.Logf("Install OSUS instance")
	if err = oc.AsAdmin().Run("apply").Args("-f", outdir).Execute(); err != nil {
		err = fmt.Errorf("install osus instance failed: %v", err)
		return
	}
	waitForPodReady(oc, "app=update-service-oc-mirror", oc.Namespace())
	return nil
}

// Returns OSUS instance name
func getOSUSApp(oc *exutil.CLI) (instance string, err error) {
	e2e.Logf("Get OSUS instance")
	instance, err = oc.AsAdmin().Run("get").Args("updateservice", "-o=jsonpath={.items[].metadata.name}").Output()
	if err != nil {
		err = fmt.Errorf("get OSUS instance failed: %v", err)
	}
	return
}

// Uninstall OSUS instance
func uninstallOSUSApp(oc *exutil.CLI) (err error) {
	e2e.Logf("Uninstall OSUS instance")
	instance, err := getOSUSApp(oc)
	if err != nil {
		return
	}
	_, err = oc.AsAdmin().Run("delete").Args("updateservice", instance).Output()
	if err != nil {
		err = fmt.Errorf("uninstall OSUS instance failed: %v", err)
		return
	}
	return nil
}

// Verify the OSUS application works
func verifyOSUS(oc *exutil.CLI) (err error) {
	e2e.Logf("Verify the OSUS works")
	instance, err := getOSUSApp(oc)
	if err != nil {
		return
	}
	PEURI, err := oc.AsAdmin().Run("get").Args("-o", "jsonpath={.status.policyEngineURI}", "updateservice", instance).Output()
	if err != nil {
		return fmt.Errorf("get policy engine URI failed: %v", err)
	}
	graphURI := PEURI + "/api/upgrades_info/v1/graph"

	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transCfg}

	response, err := client.Get(graphURI + "?channel=stable-4.13")

	if err != nil {
		return fmt.Errorf("reach graph URI failed: %v", err)
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("graph URI is not active, response code is %v", response.StatusCode)
	}
	return
}

func restoreAddCA(oc *exutil.CLI, addCA string) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-config", "configmap", "trusted-ca").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	var message string
	if addCA == "" {
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config.openshift.io/cluster", "--type=json", "-p", "[{\"op\":\"remove\", \"path\":\"/spec/additionalTrustedCA\"}]").Execute()
	} else {
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config.openshift.io/cluster", "--type=merge", "--patch", fmt.Sprintf("{\"spec\":{\"additionalTrustedCA\":%s}}", addCA)).Execute()
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	waitErr := wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
		registryHealth := checkCOHealth(oc, "image-registry")
		if registryHealth {
			return true, nil
		}
		message, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].message}").Output()
		e2e.Logf("Wait for image-registry coming ready")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Image registry is not ready with info %s\n", message))
}

// Build graph-data image by using podman
func buildPushGraphImage(oc *exutil.CLI, tag string, dirname string) (err error) {
	e2e.Logf("Build graph-data image")
	dockerFile := exutil.FixturePath("testdata", "ota", "osus", "Dockerfile")
	cmd := fmt.Sprintf("podman build -f %s -t %s", dockerFile, tag)
	var out []byte
	if out, err = exec.Command("bash", "-c", cmd).CombinedOutput(); err != nil {
		err = fmt.Errorf("%s failed: %v\n%s", cmd, err, string(out))
		return
	}

	cmd = fmt.Sprintf("podman push --trace --authfile %s --tls-verify=false %s", dirname+"/.dockerconfigjson", tag)
	if out, err = exec.Command("bash", "-c", cmd).CombinedOutput(); err != nil {
		err = fmt.Errorf("%s failed: %v\n%s", cmd, err, string(out))
		return
	}
	return
}

// Mirror OCP images using oc adm release mirror
func mirror(oc *exutil.CLI, registry string, payload string, dirname string) (err error) {
	e2e.Logf("Mirror OCP images by oc adm release mirror")
	_, tag, found := strings.Cut(payload, ":")
	if !found {
		err = fmt.Errorf("the payload is invalid")
		return
	}
	cmd := fmt.Sprintf("oc adm release mirror -a %s --insecure=true --from %s --to=%s --to-release-image=%s", dirname+"/.dockerconfigjson", payload, registry+"/ocp-image", registry+"/ocp-release:"+tag)
	var out []byte
	if out, err = exec.Command("bash", "-c", cmd).CombinedOutput(); err != nil {
		err = fmt.Errorf("%s failed: %v\n%s", cmd, err, string(out))
		return
	}
	return
}

// Install OSUS instance using oc
func installOSUSAppOC(oc *exutil.CLI, us updateService) (err error) {
	e2e.Logf("Install OSUS instance")
	if err = us.create(oc); err != nil {
		err = fmt.Errorf("install osus instance failed: %v", err)
		return
	}
	waitForPodReady(oc, "app="+us.name, oc.Namespace())
	return nil
}

func installOSUSOperator(oc *exutil.CLI, version string, mode string) {
	e2e.Logf("Install OSUS operator")
	testDataDir := exutil.FixturePath("testdata", "ota/osus")
	ogTemp := filepath.Join(testDataDir, "operatorgroup.yaml")
	subTemp := filepath.Join(testDataDir, "subscription.yaml")
	var csv string
	if version == "" {
		csv = version
	} else {
		csv = fmt.Sprintf("update-service-operator.v%s", version)
	}

	og := operatorGroup{
		name:      "osus-og",
		namespace: oc.Namespace(),
		template:  ogTemp,
	}

	sub := subscription{
		name:            "osus-sub",
		namespace:       oc.Namespace(),
		channel:         "v1",
		approval:        mode,
		operatorName:    "cincinnati-operator",
		sourceName:      "qe-app-registry",
		sourceNamespace: "openshift-marketplace",
		startingCSV:     csv,
		template:        subTemp,
	}

	e2e.Logf("Create OperatorGroup...")
	og.create(oc)

	e2e.Logf("Create Subscription...")
	sub.create(oc)

	if mode == "Manual" && version != "" {
		e2e.Logf("Approve installplan manually...")
		jsonpath := fmt.Sprintf("-o=jsonpath={.items[?(@.spec.clusterServiceVersionNames[]=='%s')].metadata.name}", csv)
		o.Eventually(func() string {
			osusIP, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("installplan", jsonpath, "-n", oc.Namespace()).Output()
			e2e.Logf("waiting for ip: %s", osusIP)
			return osusIP
		}, 3*time.Minute, 1*time.Minute).ShouldNot(o.BeEmpty(), "Fail to generate installplan!")
		osusIP, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("installplan", jsonpath, "-n", oc.Namespace()).Output()
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("installplan", osusIP, "--type=json", "-p", "[{\"op\": \"replace\", \"path\": \"/spec/approved\", \"value\": true}]", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	waitForPodReady(oc, "name=updateservice-operator", oc.Namespace())
}

func upgradeOSUS(oc *exutil.CLI, usname string, version string) error {
	e2e.Logf("Check installplan available...")
	ips, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("installplan", "-o=jsonpath={.items[*].metadata.name}", "-n", oc.Namespace()).Output()
	if err != nil {
		return err
	}
	if len(strings.Fields(ips)) != 2 {
		return fmt.Errorf("unexpected installplan found: %s", ips)
	}
	preOPName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector=name=updateservice-operator", "-o=jsonpath={.items[*].metadata.name}", "-n", oc.Namespace()).Output()
	if err != nil {
		return err
	}
	preAPPName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector=app="+usname, "-o=jsonpath={.items[*].metadata.name}", "-n", oc.Namespace()).Output()
	if err != nil {
		return err
	}
	e2e.Logf("Manually approve new installplan for update...")
	jsonpath := fmt.Sprintf("-o=jsonpath={.items[?(@.spec.clusterServiceVersionNames[]=='update-service-operator.v%s')].metadata.name}", version)
	osusIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("installplan", jsonpath, "-n", oc.Namespace()).Output()
	if err != nil {
		return err
	}
	err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("installplan", osusIP, "--type=json", "-p", "[{\"op\": \"replace\", \"path\": \"/spec/approved\", \"value\": true}]", "-n", oc.Namespace()).Execute()
	if err != nil {
		return err
	}
	e2e.Logf("Waiting for operator and operand pods rolling...")
	var (
		postOPName string
		errOP      error
	)
	preAppList := strings.Fields(preAPPName)
	err = wait.PollUntilContextTimeout(context.Background(), 1*time.Minute, 5*time.Minute, true, func(context.Context) (bool, error) {
		postOPName, errOP = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector=name=updateservice-operator", "-o=jsonpath={.items[*].metadata.name}", "-n", oc.Namespace()).Output()
		postAPPName, errAPP := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector=app="+usname, "-o=jsonpath={.items[*].metadata.name}", "-n", oc.Namespace()).Output()
		if errOP != nil || errAPP != nil {
			return false, nil
		}
		if strings.Compare(postOPName, preOPName) == 0 {
			e2e.Logf("waiting: operator pods after upgrade: %s; while operator pods before upgrade: %s", postOPName, preOPName)
			return false, nil
		}
		for _, pre := range preAppList {
			if strings.Contains(postAPPName, pre) {
				e2e.Logf("waiting: app pods after upgrade: %s; while app pods before upgrade: %s", postAPPName, preAPPName)
				return false, nil
			}
		}
		if len(strings.Fields(postAPPName)) != len(preAppList) {
			e2e.Logf("waiting for pods [%s] to expected number %d", postAPPName, len(preAppList))
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("pod is not rolling successfully after upgrade: %v", err)
	}
	csvInPostPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", postOPName, "-o=jsonpath={.spec.containers[].env[?(@.name=='OPERATOR_CONDITION_NAME')].value}", "-n", oc.Namespace()).Output()
	if err != nil {
		return err
	}
	if !strings.Contains(csvInPostPod, version) {
		return fmt.Errorf("unexpected operator version upgraded: %s", csvInPostPod)
	}
	return nil
}

func skipUnsupportedOCPVer(oc *exutil.CLI, version string) {
	mapTest := supportedMap{
		osusver: "4.9.1",
		ocpver:  []string{"4.8", "4.9", "4.10", "4.11"},
	}
	clusterVersion, _, err := exutil.GetClusterVersion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	if version != mapTest.osusver {
		g.Skip(fmt.Sprintf("Skip test for cluster with unrecoginzed old osus version %s!", version))
	}
	skip := true
	for _, ver := range mapTest.ocpver {
		if clusterVersion == ver {
			skip = false
			break
		}
	}
	if skip {
		g.Skip("Skip test for cluster with old osus on unsupported ocp version!")
	}
}

// Check metadata in the OSUS application works and return digests
func checkMetadata(oc *exutil.CLI, podName string) (digests []string, err error) {
	e2e.Logf("Check the metadata service works...")
	// Workaound OCPBUGS-33292, will restore it after the bug fix
	// instance, err := getOSUSApp(oc)
	// if err != nil {
	// 	 return
	// }
	// MetadataURI, err := oc.AsAdmin().Run("get").Args("-o", "jsonpath={.status.MetadataURI}", "updateservice", instance).Output()

	host, err := oc.AsAdmin().Run("get").Args("route", "update-service-oc-mirror-meta-route", "-o", "jsonpath={.spec.host}").Output()
	if err != nil || host == "" {
		return nil, fmt.Errorf("fail to get metadata URI: %v", err)
	}
	MetadataURI := "https://" + host
	result, err := oc.AsAdmin().Run("exec").Args(podName, "-c", "graph-builder", "--", "ls", "/var/lib/cincinnati/graph-data/signatures/sha256/").Output()
	if err != nil || result == "" {
		return nil, fmt.Errorf("fail to get signatures info: %v", err)
	}
	digests = strings.Fields(result)
	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transCfg}
	for _, digest := range digests {
		api := MetadataURI + "/api/upgrades_info/signatures/sha256=" + digest + "/signature-1"
		response, err := client.Get(api)
		if err != nil {
			return nil, fmt.Errorf("fail to access metadataURI through %s: %v", api, err)
		}
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("no signature found for %s, return: %d", digest, response.StatusCode)
		}
	}
	return
}

// check if osus instance re-deployed sucessfully
func verifyAppRolling(oc *exutil.CLI, usname string, prelist []string) (postlist []string, err error) {
	e2e.Logf("Waiting for operand pods rolling...")
	err = wait.PollUntilContextTimeout(context.Background(), 1*time.Minute, 5*time.Minute, true, func(context.Context) (bool, error) {
		postAPPName, err := oc.AsAdmin().Run("get").Args("pods", "--selector=app="+usname, "-o=jsonpath={.items[?(@.status.phase==\"Running\")].metadata.name}").Output()
		if err != nil {
			return false, nil
		}
		for _, pre := range prelist {
			if strings.Contains(postAPPName, pre) {
				e2e.Logf("waiting: current app pods: %s; while app pods before rolling: %s", postAPPName, prelist)
				return false, nil
			}
		}
		postlist = strings.Fields(postAPPName)
		if len(postlist) != len(prelist) {
			e2e.Logf("waiting for pods [%s] to expected number %d", postlist, len(prelist))
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("pod is not rolling successfully: %v", err)
	}
	return
}
