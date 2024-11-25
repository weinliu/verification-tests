package imageregistry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

const (
	asAdmin          = true
	withoutNamespace = true
	contain          = false
	ok               = true
	pvcType          = "pvc"
	swiftType        = "swift"
	emptyDir         = "emptyDir"
)

type prometheusResponse struct {
	Status string                 `json:"status"`
	Error  string                 `json:"error"`
	Data   prometheusResponseData `json:"data"`
}

type prometheusResponseData struct {
	ResultType string       `json:"resultType"`
	Result     model.Vector `json:"result"`
}

// tbuskey@redhat.com for OCP-22056
type prometheusImageregistryQueryHTTP struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name      string `json:"__name__"`
				Container string `json:"container"`
				Endpoint  string `json:"endpoint"`
				Instance  string `json:"instance"`
				Job       string `json:"job"`
				Namespace string `json:"namespace"`
				Pod       string `json:"pod"`
				Service   string `json:"service"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
		ResultType string `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

func gatherMetricsResult(oc *exutil.CLI, token, prometheusURL string, metrics []string) map[string]int {
	var (
		data   prometheusImageregistryQueryHTTP
		err    error
		l      int
		msg    string
		result = make(map[string]int)
	)
	for _, query := range metrics {
		prometheusURLQuery := fmt.Sprintf("%v/query?query=%v", prometheusURL, query)
		err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), prometheusURLQuery).Outputs()
			if err != nil || msg == "" {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the query %v is getting failed", query))
		json.Unmarshal([]byte(msg), &data)
		l = len(data.Data.Result) - 1
		result[query], _ = strconv.Atoi(data.Data.Result[l].Value[1].(string))
		e2e.Logf("The query %v result ==  %v", query, result[query])
	}
	return result
}

func listPodStartingWith(prefix string, oc *exutil.CLI, namespace string) (pod []corev1.Pod) {
	podsToAll := []corev1.Pod{}
	podList, err := oc.AdminKubeClient().CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		e2e.Logf("Error listing pods: %v", err)
		return nil
	}
	for _, pod := range podList.Items {
		if strings.HasPrefix(pod.Name, prefix) {
			podsToAll = append(podsToAll, pod)
		}
	}
	return podsToAll
}

func dePodLogs(pods []corev1.Pod, oc *exutil.CLI, matchlogs string) bool {
	for _, pod := range pods {
		depOutput, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+pod.Name, "-n", pod.Namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(depOutput, matchlogs) {
			return true
		}
	}
	return false
}

func getBearerTokenURLViaPod(ns, execPodName, url, bearer string) (string, error) {
	g.By("Get token via pod")
	cmd := fmt.Sprintf("curl --retry 15 --max-time 4 --retry-delay 1 -s -k -H 'Authorization: Bearer %s' %s", bearer, url)
	output, err := e2eoutput.RunHostCmd(ns, execPodName, cmd)
	if err != nil {
		return "", fmt.Errorf("host command failed: %v\n%s", err, output)
	}
	return output, nil
}

type bcSource struct {
	outname   string
	name      string
	namespace string
	template  string
}

type authRole struct {
	namespace string
	rolename  string
	template  string
}

func (bcsrc *bcSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", bcsrc.template, "-p", "OUTNAME="+bcsrc.outname, "NAME="+bcsrc.name, "NAMESPACE="+bcsrc.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (authrole *authRole) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", authrole.template, "-p", "NAMESPACE="+authrole.namespace, "ROLE_NAME="+authrole.rolename)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func parseToJSON(oc *exutil.CLI, parameters []string) string {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Applying resources from template is failed")
	e2e.Logf("the file of resource is %s", configFile)
	return configFile
}

func createResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	configFile := parseToJSON(oc, parameters)
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
}

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	configFile := parseToJSON(oc, parameters)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

// the method is to get something from resource. it is "oc get xxx" actually
func getResource(oc *exutil.CLI, asAdmin, withoutNamespace bool, parameters ...string) string {
	var result string
	var err error
	err = wait.Poll(3*time.Second, 150*time.Second, func() (bool, error) {
		result, err = doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", result, err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to get %v", parameters))
	e2e.Logf("$oc get %v, the returned resource:%v", parameters, result)
	return result
}

// the method is to do something with oc.
func doAction(oc *exutil.CLI, action string, asAdmin, withoutNamespace bool, parameters ...string) (string, error) {
	if asAdmin && withoutNamespace {
		return oc.AsAdmin().WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if asAdmin && !withoutNamespace {
		return oc.AsAdmin().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && withoutNamespace {
		return oc.WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && !withoutNamespace {
		return oc.Run(action).Args(parameters...).Output()
	}
	return "", nil
}

func comparePodHostIP(oc *exutil.CLI) (int, int) {
	var hostsIP = []string{}
	var numi, numj int
	podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(context.Background(), metav1.ListOptions{LabelSelector: "docker-registry=default"})
	for _, pod := range podList.Items {
		hostsIP = append(hostsIP, pod.Status.HostIP)
	}
	for i := 0; i < len(hostsIP)-1; i++ {
		for j := i + 1; j < len(hostsIP); j++ {
			if hostsIP[i] == hostsIP[j] {
				numi++
			} else {
				numj++
			}
		}
	}
	return numi, numj
}

// Check the latest image pruner pod logs
func imagePruneLog(oc *exutil.CLI, matchLogs, notMatchLogs string) {
	podsOfImagePrune := []corev1.Pod{}
	err := wait.Poll(10*time.Second, 5*time.Minute, func() (bool, error) {
		podsOfImagePrune = listPodStartingWith("image-pruner", oc, "openshift-image-registry")
		if len(podsOfImagePrune) == 0 {
			e2e.Logf("Can't get pruner pods, go to next round")
			return false, nil
		}
		pod := podsOfImagePrune[len(podsOfImagePrune)-1]
		e2e.Logf("the pod status is %s", pod.Status.Phase)
		if pod.Status.Phase != "ContainerCreating" && pod.Status.Phase != "Pending" {
			depOutput, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+pod.Name, "-n", pod.Namespace).Output()
			if strings.Contains(depOutput, matchLogs) && !strings.Contains(depOutput, notMatchLogs) {
				return true, nil
			}
		}
		e2e.Logf("The image pruner log doesn't contain %v or contain %v", matchLogs, notMatchLogs)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can't get the image pruner log or image pruner log doesn't contain %v or contain %v", matchLogs, notMatchLogs))
}

func configureRegistryStorageToEmptyDir(oc *exutil.CLI) {
	emptydirstorage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.status.storage.emptyDir}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if emptydirstorage == "{}" {
		g.By("Image registry is using EmptyDir now")
	} else {
		g.By("Set registry to use EmptyDir storage")
		storagetype, _ := getRegistryStorageConfig(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"`+storagetype+`":null,"emptyDir":{}}, "replicas":1}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(30*time.Second, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(context.Background(), metav1.ListOptions{LabelSelector: "docker-registry=default"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(podList.Items) == 1 && podList.Items[0].Status.Phase == corev1.PodRunning {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry pod list is not 1")
		err = oc.AsAdmin().WithoutNamespace().Run("wait").Args("configs.imageregistry/cluster", "--for=condition=Available").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func recoverRegistryStorageConfig(oc *exutil.CLI) {
	g.By("Set image registry storage to default value")
	platformtype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.type}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if platformtype != "VSphere" {
		if platformtype != "None" {
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":null}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Image registry will be auto-recovered to default storage")
		}
	}
}

func recoverRegistryDefaultReplicas(oc *exutil.CLI) {
	g.By("Set image registry to default replicas")
	platforms := map[string]bool{
		"VSphere": true,
		"None":    true,
		"oVirt":   true,
	}
	expectedStatus1 := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	platformtype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.type}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !platforms[platformtype] {
		g.By("Check if cluster is sno")
		workerNodes, _ := exutil.GetClusterNodesBy(oc, "worker")
		if len(workerNodes) == 1 {
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("config.imageregistry/cluster", "-p", `{"spec":{"replicas":1}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("config.imageregistry/cluster", "-p", `{"spec":{"replicas":2}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		err = waitCoBecomes(oc, "image-registry", 240, expectedStatus1)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func getRegistryStorageConfig(oc *exutil.CLI) (string, string) {
	var storagetype, storageinfo string
	g.By("Get image registry storage info")
	platformtype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.type}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	switch platformtype {
	case "AWS":
		storagetype = "s3"
		storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.s3.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	case "Azure":
		storagetype = "azure"
		storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.azure.container}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	case "GCP":
		storagetype = "gcs"
		storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.gcs.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	case "OpenStack":
		storagetype = swiftType
		storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.swift.container}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// On disconnect & openstack, the registry configure to use persistent volume
		if storageinfo == "" {
			storagetype = pvcType
			storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.pvc.claim}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	case "AlibabaCloud":
		storagetype = "oss"
		storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.oss.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	case "IBMCloud":
		storagetype = "ibmcos"
		storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.ibmcos.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	case "BareMetal", "None", "VSphere", "Nutanix", "External":
		storagetype = pvcType
		storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.pvc.claim}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if storageinfo == "" {
			storagetype = emptyDir
			storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.emptyDir}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if storageinfo == "" {
				storagetype = "s3"
				storageinfo, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage.s3.bucket}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}
	default:
		e2e.Logf("Image Registry is using unknown storage type")
	}
	return storagetype, storageinfo
}

/*
func waitRegistryDefaultPodsReady(oc *exutil.CLI) {
	storagetype, _ := getRegistryStorageConfig(oc)
	if storagetype == pvcType || storagetype == emptyDir {
		podNum := getImageRegistryPodNumber(oc)
		o.Expect(podNum).Should(o.Equal(1))
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)
	} else {
		podNum := getImageRegistryPodNumber(oc)
		o.Expect(podNum).Should(o.Equal(2))
		checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", podNum)

	}
}
*/

func checkRegistrypodsRemoved(oc *exutil.CLI) {
	err := wait.Poll(25*time.Second, 3*time.Minute, func() (bool, error) {
		podList, err := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(context.Background(), metav1.ListOptions{LabelSelector: "docker-registry=default"})
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(podList.Items) == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Image registry pods are not removed")
}

type staSource struct {
	name      string
	namespace string
	image     string
	template  string
}

func (stafulsrc *staSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", stafulsrc.template, "-p", "NAME="+stafulsrc.name, "NAMESPACE="+stafulsrc.namespace, "IMAGE="+stafulsrc.image)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func checkPodsRunningWithLabel(oc *exutil.CLI, namespace, label string, number int) {
	err := wait.Poll(25*time.Second, 6*time.Minute, func() (bool, error) {
		podList, _ := oc.AdminKubeClient().CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: label})
		if len(podList.Items) != number {
			e2e.Logf("the pod number is not %d, Continue to next round", number)
			return false, nil
		}
		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				e2e.Logf("Continue to next round")
				return false, nil
			}
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pods list are not %d", number))
}

type icspSource struct {
	name     string
	mirrors  string
	source   string
	template string
}

func (icspsrc *icspSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", icspsrc.template, "-p", "NAME="+icspsrc.name, "MIRRORS="+icspsrc.mirrors, "SOURCE="+icspsrc.source)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (icspsrc *icspSource) delete(oc *exutil.CLI) {
	e2e.Logf("deleting icsp: %s", icspsrc.name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("imagecontentsourcepolicy", icspsrc.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getRegistryDefaultRoute(oc *exutil.CLI) (defaultroute string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		defroute, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("route", "-n", "openshift-image-registry", "default-route", "-o=jsonpath={.spec.host}").Output()
		if defroute == "" || err != nil {
			e2e.Logf("Continue to next round")
			return false, nil
		}
		defaultroute = defroute
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Did not find registry route")
	return defaultroute
}

func setImageregistryConfigs(oc *exutil.CLI, pathinfo, matchlogs string) bool {
	foundInfo := false
	defer recoverRegistrySwiftSet(oc)
	err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"swift":{`+pathinfo+`}}}}`, "--type=merge").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("co/image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, matchlogs) {
			foundInfo = true
			return true, nil
		}
		e2e.Logf("Continue to next round")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "No image registry error info found")
	return foundInfo
}

func recoverRegistrySwiftSet(oc *exutil.CLI) {
	matchInfo := "True False False"
	err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"swift":{"authURL":null, "regionName":null, "regionID":null, "domainID":null, "domain":null, "tenantID":null}}}}`, "--type=merge").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.Poll(4*time.Second, 20*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[*].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, matchInfo) {
			return true, nil
		}
		e2e.Logf("Continue to next round")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Image registry is degrade")
}

type podSource struct {
	name      string
	namespace string
	image     string
	template  string
}

func (podsrc *podSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", podsrc.template, "-p", "NAME="+podsrc.name, "NAMESPACE="+podsrc.namespace, "IMAGE="+podsrc.image)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func checkRegistryUsingFSVolume(oc *exutil.CLI) bool {
	storageinfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("config.image", "cluster", "-o=jsonpath={.spec.storage}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(storageinfo, pvcType) || strings.Contains(storageinfo, emptyDir) {
		return true
	}
	return false
}

func saveImageMetadataName(oc *exutil.CLI, image string) string {
	imagemetadata, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("images").OutputToFile("imagemetadata.txt")
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove("imagemetadata.txt")
	manifest, err := exec.Command("bash", "-c", "cat "+imagemetadata+" | grep "+image+" | awk '{print $1}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.TrimSuffix(string(manifest), "\n")
}

func checkRegistryFunctionFine(oc *exutil.CLI, bcname, namespace string) {
	// Check if could push images to image registry
	err := oc.AsAdmin().WithoutNamespace().Run("new-build").Args("-D", "FROM quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--to="+bcname, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(namespace), bcname+"-1", nil, nil, nil)
	if err != nil {
		exutil.DumpBuildLogs(bcname, oc)
	}
	exutil.AssertWaitPollNoErr(err, "build is not complete")
	err = exutil.WaitForAnImageStreamTag(oc, namespace, bcname, "latest")
	o.Expect(err).NotTo(o.HaveOccurred())

	// Check if could pull images from image registry
	imagename := "image-registry.openshift-image-registry.svc:5000/" + namespace + "/" + bcname + ":latest"
	err = oc.AsAdmin().WithoutNamespace().Run("run").Args(bcname, "--image", imagename, `--overrides={"spec":{"securityContext":{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}}}`, "-n", namespace, "--command", "--", "/bin/sleep", "120").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	var output string
	errWait := wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", bcname, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, `Successfully pulled image`) {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("Image registry is broken, can't pull image. the log:\n %v", output))
}

func checkRegistryDegraded(oc *exutil.CLI) bool {
	status := "TrueFalseFalse"
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return !strings.Contains(output, status)
}

func getCreditFromCluster(oc *exutil.CLI) (string, string, string) {
	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	accessKeyIDBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).Str, gjson.Get(credential, `data.aws_secret_access_key`).Str
	accessKeyID, err1 := base64.StdEncoding.DecodeString(accessKeyIDBase64)
	o.Expect(err1).NotTo(o.HaveOccurred())
	secureKey, err2 := base64.StdEncoding.DecodeString(secureKeyBase64)
	o.Expect(err2).NotTo(o.HaveOccurred())
	clusterRegion, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
	o.Expect(err3).NotTo(o.HaveOccurred())
	return string(accessKeyID), string(secureKey), clusterRegion
}

func getAWSClient(oc *exutil.CLI) *s3.Client {
	accessKeyID, secureKey, clusterRegion := getCreditFromCluster(oc)
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secureKey, "")),
		config.WithRegion(clusterRegion))

	o.Expect(err).NotTo(o.HaveOccurred())
	return s3.NewFromConfig(cfg)
}

func awsGetBucketTagging(client *s3.Client, bucket string) (string, error) {
	tagOutput, err := client.GetBucketTagging(context.TODO(), &s3.GetBucketTaggingInput{Bucket: &bucket})
	if err != nil {
		outputGetTag := fmt.Sprintf("Got an error GetBucketTagging for %s: %v", bucket, err)
		return outputGetTag, err
	}
	outputGetTag := ""
	for _, t := range tagOutput.TagSet {
		outputGetTag += *t.Key + " " + *t.Value + "\n"
	}
	return outputGetTag, nil
}

// the method is to make newCheck object.
// the method parameter is expect, it will check something is expceted or not
// the method parameter is present, it will check something exists or not
// the executor is asAdmin, it will exectue oc with Admin
// the executor is asUser, it will exectue oc with User
// the inlineNamespace is withoutNamespace, it will execute oc with WithoutNamespace()
// the inlineNamespace is withNamespace, it will execute oc with WithNamespace()
// the expectAction take effective when method is expect, if it is contain, it will check if the strings contain substring with expectContent parameter
//
//	if it is compare, it will check the strings is samme with expectContent parameter
//
// the expectContent is the content we expected
// the expect is ok, contain or compare result is OK for method == expect, no error raise. if not OK, error raise
// the expect is nok, contain or compare result is NOK for method == expect, no error raise. if OK, error raise
// the expect is ok, resource existing is OK for method == present, no error raise. if resource not existing, error raise
// the expect is nok, resource not existing is OK for method == present, no error raise. if resource existing, error raise
func newCheck(method string, executor bool, inlineNamespace bool, expectAction bool,
	expectContent string, expect bool, resource []string) checkDescription {
	return checkDescription{
		method:          method,
		executor:        executor,
		inlineNamespace: inlineNamespace,
		expectAction:    expectAction,
		expectContent:   expectContent,
		expect:          expect,
		resource:        resource,
	}
}

type checkDescription struct {
	method          string
	executor        bool
	inlineNamespace bool
	expectAction    bool
	expectContent   string
	expect          bool
	resource        []string
}

// the method is to check the resource per definition of the above described newCheck.
func (ck checkDescription) check(oc *exutil.CLI) {
	switch ck.method {
	case "present":
		ok := isPresentResource(oc, ck.executor, ck.inlineNamespace, ck.expectAction, ck.resource...)
		o.Expect(ok).To(o.BeTrue())
	case "expect":
		err := expectedResource(oc, ck.executor, ck.inlineNamespace, ck.expectAction, ck.expectContent, ck.expect, ck.resource...)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("expected content %s not found by %v", ck.expectContent, ck.resource))
	default:
		err := fmt.Errorf("unknown method")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// the method is to check the presence of the resource
// asAdmin means if taking admin to check it
// withoutNamespace means if take WithoutNamespace() to check it.
// present means if you expect the resource presence or not. if it is ok, expect presence. if it is nok, expect not present.
func isPresentResource(oc *exutil.CLI, asAdmin, withoutNamespace, present bool, parameters ...string) bool {
	parameters = append(parameters, "--ignore-not-found")
	err := wait.Poll(3*time.Second, 70*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		if !present && strings.Compare(output, "") == 0 {
			return true, nil
		}
		if present && strings.Compare(output, "") != 0 {
			return true, nil
		}
		return false, nil
	})
	return err == nil
}

// the method is to check one resource's attribution is expected or not.
// asAdmin means if taking admin to check it
// withoutNamespace means if take WithoutNamespace() to check it.
// isCompare means if containing or exactly comparing. if it is contain, it check result contain content. if it is compare, it compare the result with content exactly.
// content is the substing to be expected
// the expect is ok, contain or compare result is OK for method == expect, no error raise. if not OK, error raise
// the expect is nok, contain or compare result is NOK for method == expect, no error raise. if OK, error raise
func expectedResource(oc *exutil.CLI, asAdmin, withoutNamespace, isCompare bool, content string, expect bool, parameters ...string) error {
	expectMap := map[bool]string{
		true:  "do",
		false: "do not",
	}

	cc := func(a, b string, ic bool) bool {
		bs := strings.Split(b, "+2+")
		ret := false
		for _, s := range bs {
			if (ic && strings.Compare(a, s) == 0) || (!ic && strings.Contains(a, s)) {
				ret = true
			}
		}
		return ret
	}
	e2e.Logf("Running: oc get asAdmin(%t) withoutNamespace(%t) %s", asAdmin, withoutNamespace, strings.Join(parameters, " "))
	return wait.Poll(3*time.Second, 150*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		e2e.Logf("---> we %v expect value: %s, in returned value: %s", expectMap[expect], content, output)
		if isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s matches one of the content %s, expected", output, content)
			return true, nil
		}
		if isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not matche the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s contains one of the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not contain the content %s, expected", output, content)
			return true, nil
		}
		e2e.Logf("---> Not as expected! Return false")
		return false, nil
	})
}
func exposeService(oc *exutil.CLI, ns, resource, name, port string) {
	err := oc.AsAdmin().WithoutNamespace().Run("expose").Args(resource, "--name="+name, "--port="+port, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func exposeRouteFromSVC(oc *exutil.CLI, rType, ns, route, service string) string {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("route", rType, route, "--service="+service, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	regRoute, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", route, "-n", ns, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return regRoute
}

func listRepositories(regRoute, expect string) {
	curlCmd := fmt.Sprintf("curl -ks  https://%s/v2/_catalog | grep %s", regRoute, expect)
	result, err := exec.Command("bash", "-c", curlCmd).CombinedOutput()
	if err != nil {
		e2e.Logf("Command: \"%s\" returned: %v", curlCmd, string(result))
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	o.Expect(string(result)).To(o.ContainSubstring(expect))
}

func setSecureRegistryWithoutAuth(oc *exutil.CLI, ns, regName, image, port string) string {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("deploy", regName, "--image="+image, "--port=5000", "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	checkPodsRunningWithLabel(oc, ns, "app="+regName, 1)
	exposeService(oc, ns, "deploy/"+regName, regName, port)
	regRoute := exposeRouteFromSVC(oc, "edge", ns, regName, regName)
	checkDnsCO(oc)
	waitRouteReady(regRoute)
	listRepositories(regRoute, "repositories")
	return regRoute
}

func setSecureRegistryEnableAuth(oc *exutil.CLI, ns, regName, htpasswdFile, image string) string {
	regRoute := setSecureRegistryWithoutAuth(oc, ns, regName, image, "5000")
	ge1 := saveGeneration(oc, ns, "deployment/"+regName)
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "htpasswd", "--from-file="+htpasswdFile, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.WithoutNamespace().Run("set").Args("volume", "deployment/"+regName, "--add", "--mount-path=/auth", "--type=secret", "--secret-name=htpasswd", "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.WithoutNamespace().Run("set").Args("env", "deployment/"+regName, "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd", "REGISTRY_AUTH_HTPASSWD_REALM=Registry Realm", "REGISTRY_AUTH=htpasswd", "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		ge2 := saveGeneration(oc, ns, "deployment/"+regName)
		if ge2 == ge1 {
			e2e.Logf("Continue to next round")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Custom registry does not update")
	checkPodsRunningWithLabel(oc, ns, "app="+regName, 1)
	return regRoute
}

func generateHtpasswdFile(tempDataDir, user, pass string) (string, error) {
	htpasswdFile := filepath.Join(tempDataDir, "htpasswd")
	generateCMD := fmt.Sprintf("htpasswd -Bbn %s %s > %s", user, pass, htpasswdFile)
	_, err := exec.Command("bash", "-c", generateCMD).Output()
	if err != nil {
		e2e.Logf("Fail to generate htpasswd file: %v", err)
		return htpasswdFile, err
	}
	return htpasswdFile, nil
}

func extractPullSecret(oc *exutil.CLI) (string, error) {
	tempDataDir := filepath.Join("/tmp/", fmt.Sprintf("ir-%s", getRandomString()))
	err := os.Mkdir(tempDataDir, 0o755)
	if err != nil {
		e2e.Logf("Fail to create directory: %v", err)
		return tempDataDir, err
	}
	err = oc.AsAdmin().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--confirm", "--to="+tempDataDir).Execute()
	if err != nil {
		e2e.Logf("Fail to extract dockerconfig: %v", err)
		return tempDataDir, err
	}
	return tempDataDir, nil
}

func appendPullSecretAuth(authFile, regRouter, newRegUser, newRegPass string) (string, error) {
	fieldValue := newRegUser + ":" + newRegPass
	regToken := base64.StdEncoding.EncodeToString([]byte(fieldValue))
	authDir, _ := filepath.Split(authFile)
	newAuthFile := filepath.Join(authDir, fmt.Sprintf("%s.json", getRandomString()))
	jqCMD := fmt.Sprintf(`cat %s | jq '.auths += {"%s":{"auth":"%s"}}' > %s`, authFile, regRouter, regToken, newAuthFile)
	_, err := exec.Command("bash", "-c", jqCMD).Output()
	if err != nil {
		e2e.Logf("Fail to extract dockerconfig: %v", err)
		return newAuthFile, err
	}
	return newAuthFile, nil
}

func updatePullSecret(oc *exutil.CLI, authFile string) {
	err := oc.AsAdmin().WithoutNamespace().Run("set").Args("data", "secret/pull-secret", "-n", "openshift-config", "--from-file=.dockerconfigjson="+authFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func foundAffinityRules(oc *exutil.CLI, affinityRules string) bool {
	podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(context.Background(), metav1.ListOptions{LabelSelector: "docker-registry=default"})
	for _, pod := range podList.Items {
		out, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("pod/"+pod.Name, "-n", pod.Namespace, "-o=jsonpath={.spec.affinity.podAntiAffinity}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out, affinityRules) {
			return false
		}
	}
	return true
}

func saveGlobalProxy(oc *exutil.CLI) (string, string, string) {
	httpProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpProxy}")
	httpsProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpsProxy}")
	noProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.noProxy}")
	return httpProxy, httpsProxy, noProxy
}

func createSimpleRunPod(oc *exutil.CLI, image, expectInfo string) {
	podName := getRandomString()
	err := oc.AsAdmin().WithoutNamespace().Run("run").Args(podName, "--image="+image, "-n", oc.Namespace(), `--overrides={"spec":{"securityContext":{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}}}`, "--image-pull-policy=Always", "--", "sleep", "300").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", podName, "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, expectInfo) {
			return true, nil
		}
		e2e.Logf("Continue to next round")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Pod doesn't show expected log %v", expectInfo))
}

func newAppUseImageStream(oc *exutil.CLI, ns, imagestream, expectInfo string) {
	appName := getRandomString()
	err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("-i", imagestream, "--name="+appName, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-l", "deployment="+appName, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, expectInfo) {
			return true, nil
		}
		e2e.Logf("Continue to next round")
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Pod doesn't pull expected image")
}

// Save deployment or daemonset generation to judge if update applied
func saveGeneration(oc *exutil.CLI, ns, resource string) string {
	num, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, "-n", ns, "-o=jsonpath={.metadata.generation}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return num
}

// Create route to expose the registry
func createRouteExposeRegistry(oc *exutil.CLI) {
	// Don't forget to restore the environment use func restoreRouteExposeRegistry
	output, err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"defaultRoute":true}}`, "--type=merge").Output()
	if err != nil {
		e2e.Logf(output)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("patched"))
}

func restoreRouteExposeRegistry(oc *exutil.CLI) {
	output, err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"defaultRoute":false}}`, "--type=merge").Output()
	if err != nil {
		e2e.Logf(output)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("patched"))
}

func getPodNodeListByLabel(oc *exutil.CLI, namespace, labelKey string) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-o", "wide", "-n", namespace, "-l", labelKey, "-o=jsonpath={.items[*].spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeNameList := strings.Fields(output)
	return nodeNameList
}

func getImageRegistryPodNumber(oc *exutil.CLI) int {
	podNum, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.replicas}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	intPodNum, _ := strconv.Atoi(podNum)
	return intPodNum
}

func saveImageRegistryAuth(oc *exutil.CLI, sa, regRoute, ns string) (string, error) {
	tempDataFile := filepath.Join("/tmp/", fmt.Sprintf("ir-auth-%s", getRandomString()))
	token, err := getSAToken(oc, sa, ns)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("registry").Args("login", "--registry="+regRoute, "--auth-basic=anyuser:"+token, "--to="+tempDataFile, "--insecure", "-n", ns).Execute()
	if err != nil {
		e2e.Logf("Fail to login image registry: %v", err)
		return tempDataFile, err
	}
	return tempDataFile, nil
}

func getSAToken(oc *exutil.CLI, sa, ns string) (string, error) {
	e2e.Logf("Getting a token assgined to specific serviceaccount from %s namespace...", ns)
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", sa, "-n", ns).Output()
	if err != nil {
		if strings.Contains(token, "unknown command") { // oc client is old version, create token is not supported
			e2e.Logf("oc create token is not supported by current client, use oc sa get-token instead")
			token, err = oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", sa, "-n", ns).Output()
		} else {
			return "", err
		}
	}

	return token, err
}

type machineConfig struct {
	name       string
	pool       string
	source     string
	path       string
	template   string
	parameters []string
}

func (mc *machineConfig) waitForMCPComplete(oc *exutil.CLI) {
	machineCount, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mc.pool, "-ojsonpath={.status.machineCount}").Output()
	e2e.Logf("machineCount: %v", machineCount)
	o.Expect(err).NotTo(o.HaveOccurred())
	mcCount, _ := strconv.Atoi(machineCount)
	timeToWait := time.Duration(10*mcCount) * time.Minute
	e2e.Logf("Waiting %s for MCP %s to be completed.", timeToWait, mc.pool)
	err = wait.Poll(1*time.Minute, timeToWait, func() (bool, error) {
		mcpStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mc.pool, `-ojsonpath={.status.conditions[?(@.type=="Updated")].status}`).Output()
		e2e.Logf("mcpStatus: %v", mcpStatus)
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(mcpStatus, "True") {
			// i.e. mcp updated=true, mc is applied successfully
			e2e.Logf("mc operation is completed on mcp %s", mc.pool)
			return true, nil
		}
		return false, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("mc operation is not completed on mcp %s", mc.pool))

}

func (mc *machineConfig) createWithCheck(oc *exutil.CLI) {
	mc.name = mc.name + "-" + exutil.GetRandomString()
	params := []string{"--ignore-unknown-parameters=true", "-f", mc.template, "-p", "NAME=" + mc.name, "POOL=" + mc.pool, "SOURCE=" + mc.source, "PATH=" + mc.path}
	params = append(params, mc.parameters...)
	exutil.CreateClusterResourceFromTemplate(oc, params...)

	pollerr := wait.Poll(5*time.Second, 1*time.Minute, func() (bool, error) {
		stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mc/"+mc.name, "-o", "jsonpath='{.metadata.name}'").Output()
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, mc.name) {
			e2e.Logf("mc %s is created successfully", mc.name)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(pollerr, fmt.Sprintf("create machine config %v failed", mc.name))

	mc.waitForMCPComplete(oc)

}

func (mc *machineConfig) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("mc", mc.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	mc.waitForMCPComplete(oc)
}

type runtimeClass struct {
	name     string
	handler  string
	template string
}

func (rtc *runtimeClass) createWithCheck(oc *exutil.CLI) {
	rtc.name = rtc.name + "-" + exutil.GetRandomString()
	params := []string{"--ignore-unknown-parameters=true", "-f", rtc.template, "-p", "NAME=" + rtc.name, "HANDLER=" + rtc.handler}
	exutil.CreateClusterResourceFromTemplate(oc, params...)

	rtcerr := wait.Poll(5*time.Second, 1*time.Minute, func() (bool, error) {
		stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("runtimeclass/"+rtc.name, "-o", "jsonpath='{.metadata.name}'").Output()
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, rtc.name) {
			e2e.Logf("runtimeClass %s is created successfully", rtc.name)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(rtcerr, fmt.Sprintf("create runtimeClass %v failed", rtc.name))

}

func (rtc *runtimeClass) delete(oc *exutil.CLI) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("runtimeclass", rtc.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

type prometheusImageregistryOperations struct {
	Data struct {
		Result []struct {
			Metric []struct {
				Name      string `json:"__name__"`
				Operation string `json:"operation"`
				Resource  string `json:"resource_type"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
		ResultType string `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

type prometheusImageregistryStorageType struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name      string `json:"__name__"`
				Container string `json:"container"`
				Endpoint  string `json:"endpoint"`
				Instance  string `json:"instance"`
				Job       string `json:"job"`
				Namespace string `json:"namespace"`
				Pod       string `json:"pod"`
				Service   string `json:"service"`
				Storage   string `json:"storage"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
		ResultType string `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

type limitSource struct {
	name      string
	namespace string
	size      string
	template  string
}

func (limitsrc *limitSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", limitsrc.template, "-p", "NAME="+limitsrc.name, "NAMESPACE="+limitsrc.namespace, "SIZE="+limitsrc.size)
	o.Expect(err).NotTo(o.HaveOccurred())
}
func checkDnsCO(oc *exutil.CLI) {
	expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	err := waitCoBecomes(oc, "ingress", 240, expectedStatus)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = waitCoBecomes(oc, "dns", 240, expectedStatus)
	o.Expect(err).NotTo(o.HaveOccurred())

}

func waitRouteReady(route string) {
	curlCmd := "curl -k https://" + route
	var output []byte
	var curlErr error
	pollErr := wait.Poll(5*time.Second, 1*time.Minute, func() (bool, error) {
		output, curlErr = exec.Command("bash", "-c", curlCmd).CombinedOutput()
		if curlErr != nil {
			e2e.Logf("the route is not ready, go to next round")
			return false, nil
		}
		return true, nil
	})
	if pollErr != nil {
		e2e.Logf("output is: %v with error %v", string(output), curlErr.Error())
	}
	exutil.AssertWaitPollNoErr(pollErr, "The route can't be used")
}

type signatureSource struct {
	name     string
	imageid  string
	title    string
	content  string
	template string
}

func (signsrc *signatureSource) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", signsrc.template, "-p", "NAME="+signsrc.name, "IMAGEID="+signsrc.imageid, "TITLE="+signsrc.title, "CONTENT="+signsrc.content)
	o.Expect(err).NotTo(o.HaveOccurred())
}

type isSource struct {
	name      string
	namespace string
	repo      string
	tagname   string
	image     string
	template  string
}

func (issrc *isSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", issrc.template, "-p", "NAME="+issrc.name, "REPO="+issrc.repo, "NAMESPACE="+issrc.namespace, "TAGNAME="+issrc.tagname, "IMAGE="+issrc.image)
	o.Expect(err).NotTo(o.HaveOccurred())
}

/*
func setWaitForAnImageStreamTag(oc *exutil.CLI, namespace, name, tag string, timeout time.Duration) error {
	return exutil.TimedWaitForAnImageStreamTag(oc, namespace, name, tag, timeout)
}
*/

func waitForAnImageStreamTag(oc *exutil.CLI, namespace, name, tag string) error {
	return exutil.TimedWaitForAnImageStreamTag(oc, namespace, name, tag, time.Second*420)
}

func waitCoBecomes(oc *exutil.CLI, coName string, waitTime int, expectedStatus map[string]string) error {
	var gottenStatus map[string]string
	err := wait.Poll(15*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		gottenStatus := getCoStatus(oc, coName, expectedStatus)
		eq := reflect.DeepEqual(expectedStatus, gottenStatus)
		if eq {
			e2e.Logf("Given operator %s becomes %s", coName, gottenStatus)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		for key, value := range gottenStatus {
			e2e.Logf("\ncheck the %v status %v is %v\n", coName, key, value)
		}
	}
	return err
}

func getCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) map[string]string {
	newStatusToCompare := make(map[string]string)
	for key := range statusToCompare {
		args := fmt.Sprintf(`-o=jsonpath={.status.conditions[?(@.type == "%s")].status}`, key)
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", args, coName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newStatusToCompare[key] = status
	}
	return newStatusToCompare
}

func checkPodsRemovedWithLabel(oc *exutil.CLI, namespace, label string) {
	err := wait.Poll(25*time.Second, 3*time.Minute, func() (bool, error) {
		podList, err := oc.AdminKubeClient().CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: label})
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(podList.Items) == 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "Pods are not removed")
}

type dsSource struct {
	name      string
	namespace string
	image     string
	template  string
}

func (dssrc *dsSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", dssrc.template, "-p", "NAME="+dssrc.name, "NAMESPACE="+dssrc.namespace, "IMAGE="+dssrc.image)
	o.Expect(err).NotTo(o.HaveOccurred())
}

type isImportSource struct {
	name      string
	namespace string
	image     string
	policy    string
	mode      string
	template  string
}

func (issrc *isImportSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", issrc.template, "-p", "NAME="+issrc.name, "NAMESPACE="+issrc.namespace, "IMAGE="+issrc.image, "POLICY="+issrc.policy, "MODE="+issrc.mode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func pruneImage(oc *exutil.CLI, isName, imageName, refRoute, token string, num int) {
	g.By("Check image object and sub-manifest created for the manifest list")
	isOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("is/"+isName, "-n", oc.Namespace()).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(strings.Contains(isOut, isName)).To(o.BeTrue())
	imageOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("images", "-n", oc.Namespace()).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	imageCount := strings.Count(imageOut, imageName)
	o.Expect(imageCount).To(o.Equal(num))

	g.By("Prune image")
	out, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "images", "--token="+token, "--registry-url="+refRoute, "--confirm").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(strings.Contains(out, "Summary: deleted")).To(o.BeTrue())
	imageOut, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("images", "-n", oc.Namespace()).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	imageCount = strings.Count(imageOut, imageName)
	o.Expect(imageCount).To(o.Equal(num))
}

func doPrometheusQuery(oc *exutil.CLI, token, url string) int {
	var (
		data  prometheusImageregistryQueryHTTP
		count int
	)

	msg, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(
		"-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--",
		"curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token),
		fmt.Sprintf("%s", url)).Outputs()
	if err != nil {
		e2e.Failf("Failed Prometheus query, error: %v", err)
	}
	o.Expect(msg).NotTo(o.BeEmpty())
	json.Unmarshal([]byte(msg), &data)
	err = wait.Poll(60*time.Second, 120*time.Second, func() (bool, error) {
		if len(data.Data.Result) != 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "cannot get query result")
	count, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))
	o.Expect(err).NotTo(o.HaveOccurred())
	return count
}

func copyFile(source string, dest string) {
	bytesRead, err := ioutil.ReadFile(source)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = ioutil.WriteFile(dest, bytesRead, 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
}

type imageObject struct {
	architecture []string
	digest       []string
	os           []string
}

func (c *imageObject) getManifestObject(oc *exutil.CLI, resource, name, namespace string) *imageObject {
	archList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, name, "-n", namespace, "-ojsonpath={..dockerImageManifests[*].architecture}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	c.architecture = strings.Split(archList, " ")
	digestList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, name, "-n", namespace, "-ojsonpath={..dockerImageManifests[*].digest}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	c.digest = strings.Split(digestList, " ")
	osList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, name, "-n", namespace, "-ojsonpath={..dockerImageManifests[*].os}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	c.os = strings.Split(osList, " ")
	return c
}

func getManifestList(oc *exutil.CLI, image, auth string) string {
	jqCMD := fmt.Sprintf(`oc image info %s -a %s --insecure --show-multiarch -o json| jq -r '.[0].listDigest'`, image, auth)
	manifestList, err := exec.Command("bash", "-c", jqCMD).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.TrimSuffix(string(manifestList), "\n")
}

func checkOptionalOperatorInstalled(oc *exutil.CLI, operator string) bool {
	installedOperators, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.capabilities.enabledCapabilities}").Output()
	if err != nil {
		e2e.Failf("get enabledCapabilities failed err %v .", err)
	}
	if strings.Contains(installedOperators, operator) {
		e2e.Logf("The %v operator is installed", operator)
		return true
	}
	e2e.Logf("The %v operator is not installed", operator)
	return false
}

func checkICSP(oc *exutil.CLI) bool {
	icsp, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(icsp, "No resources found") {
		e2e.Logf("there is no ImageContentSourcePolicy in this cluster")
		return false
	}
	return true
}

type idmsSource struct {
	name     string
	mirrors  string
	source   string
	template string
}

func (idmssrc *idmsSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", idmssrc.template, "-p", "NAME="+idmssrc.name, "MIRRORS="+idmssrc.mirrors, "SOURCE="+idmssrc.source)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (idmssrc *idmsSource) delete(oc *exutil.CLI) {
	e2e.Logf("deleting idms: %s", idmssrc.name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("idms", idmssrc.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

type itmsSource struct {
	name     string
	mirrors  string
	source   string
	template string
}

func (itmssrc *itmsSource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", itmssrc.template, "-p", "NAME="+itmssrc.name, "MIRRORS="+itmssrc.mirrors, "SOURCE="+itmssrc.source)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (itmssrc *itmsSource) delete(oc *exutil.CLI) {
	e2e.Logf("deleting itms: %s", itmssrc.name)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("itms", itmssrc.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

type isStruct struct {
	name      string
	namespace string
	repo      string
	template  string
}

func (issrc *isStruct) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", issrc.template, "-p", "NAME="+issrc.name, "REPO="+issrc.repo, "NAMESPACE="+issrc.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// GetMirrorRegistry returns mirror registry from idms
func GetMirrorRegistry(oc *exutil.CLI) (registry string) {
	registry, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("idms", "-o", "jsonpath={.items[0].spec.imageDigestMirrors[0].mirrors[0]}").Output()
	if err != nil {
		e2e.Failf("failed to acquire mirror registry from IDMS: %v", err)
	} else {
		registry, _, _ = strings.Cut(registry, "/")
	}
	return registry
}

func checkImagePruners(oc *exutil.CLI) bool {
	impr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("imagepruners").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(impr, "No resources found") {
		e2e.Logf("there is no imagepruners in this cluster")
		return false
	}
	return true
}

func get_osp_authurl(oc *exutil.CLI) string {
	g.By("get authurl")
	var authURL string
	credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/openstack-credentials", "-n", "kube-system", "-o", `jsonpath={.data.clouds\.yaml}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	credential, err := base64.StdEncoding.DecodeString(credentials)
	o.Expect(err).NotTo(o.HaveOccurred())
	r, _ := regexp.Compile("auth_url:.*")
	match := r.FindAllString(string(credential), -1)
	if strings.Contains(match[0], "auth_url") {
		authURL = strings.Split(match[0], " ")[1]
		return authURL
	}
	return ""
}

func getgcloudClient(oc *exutil.CLI) *exutil.Gcloud {
	if exutil.CheckPlatform(oc) != "gcp" {
		g.Skip("it is not gcp platform!")
	}
	projectID, err := exutil.GetGcpProjectID(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	gcloud := exutil.Gcloud{ProjectID: projectID}
	return gcloud.Login()
}

func filterTimestampFromLogs(logs string, numberOfTimestamp int) []string {
	return regexp.MustCompile("(?m)[0-9]{1,2}:[0-9]{1,2}:[0-9]{1,2}.[0-9]{1,6}").FindAllString(logs, numberOfTimestamp)
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

func validateResourceEnv(oc *exutil.CLI, namespace, resource, value string) {
	result, err := oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "-n", namespace, resource, "--list").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(strings.Contains(result, value)).To(o.BeTrue())
}

func checkDiscPolicy(oc *exutil.CLI) (string, bool) {
	sites := [3]string{"ImageContentSourcePolicy", "idms", "itms"}
	for _, policy := range sites {
		result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(policy).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(result, "No resources found") {
			return policy, true
		}
	}
	return "", false
}
func checkMirrorRegistry(oc *exutil.CLI, repo string) string {
	policy, dis := checkDiscPolicy(oc)
	switch dis {
	case policy == "ImageContentSourcePolicy":
		mirrorReg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy/image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[?(@.source==\""+repo+"\")].mirrors[]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		return mirrorReg
	case policy == "idms":
		mirrorReg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("idms/image-policy-aosqe", "-o=jsonpath={.spec.imageDigestMirrors[?(@.source==\""+repo+"\")].mirrors[]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		return mirrorReg
	case policy == "itms":
		mirrorReg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("itms/image-policy-aosqe", "-o=jsonpath={.spec.imageTagMirrors[?(@.source==\""+repo+"\")].mirrors[]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		return mirrorReg
	}
	return ""
}

func SkipDnsFailure(oc *exutil.CLI) {
	expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	err := waitCoBecomes(oc, "ingress", 240, expectedStatus)
	if err != nil {
		g.Skip("Ingress is not ready, skip the case test!")
	}
	err = waitCoBecomes(oc, "dns", 240, expectedStatus)
	if err != nil {
		g.Skip("Dns is not ready, skip the case test!")
	}
}

// Upi install on azure is based on azure arm template
// Ipi install on azure is based on cluster api since 4.17
func isIPIAzure(oc *exutil.CLI) bool {
	result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "openshift-install", "-n", "openshift-config").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if !strings.Contains(result, "NotFound") {
		return true
	}
	return false
}

func hasDuplicate(slice []string, value string) bool {
	countMap := make(map[string]int)
	for _, v := range slice {
		if v == value {
			countMap[v]++
			if countMap[v] > 1 {
				return true
			}
		}
	}
	return false
}

func configureRegistryStorageToPvc(oc *exutil.CLI, pvcName string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":null, "managementState":"Unmanaged"}}`, "--type=merge").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	patchInfo := fmt.Sprintf("{\"spec\":{\"managementState\":\"Managed\",\"replicas\":1,\"rolloutStrategy\":\"Recreate\",\"storage\":{\"managementState\":\"Managed\",\"pvc\":{\"claim\":\"%s\"}}}}", pvcName)
	err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("configs.imageregistry/cluster", "-p", patchInfo, "--type=merge").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	checkPodsRunningWithLabel(oc, "openshift-image-registry", "docker-registry=default", 1)
}

type persistentVolumeClaim struct {
	name             string
	namespace        string
	accessmode       string
	memorysize       string
	storageclassname string
	template         string
}

func (pvc *persistentVolumeClaim) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pvc.template, "-p", "NAME="+pvc.name, "NAMESPACE="+pvc.namespace, "MEMORYSIZE="+pvc.memorysize, "STORAGECLASSNAME="+pvc.storageclassname, "ACCESSMODE="+pvc.accessmode)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func waitForPvcStatus(oc *exutil.CLI, namespace string, pvcname string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		pvStatus, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pvc", pvcname, "-o=jsonpath='{.status.phase}'").Output()
		if err != nil {
			return false, err
		}
		if match, _ := regexp.MatchString("Bound", pvStatus); match {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The PVC is not Bound as expected")
}

func checkMetric(oc *exutil.CLI, url, token, metricString string, timeout time.Duration) {
	var metrics string
	var err error
	getCmd := "curl -G -k -s -H \"Authorization:Bearer " + token + "\" " + url
	err = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, timeout*time.Second, false, func(context.Context) (bool, error) {
		metrics, err = exutil.RemoteShPod(oc, "openshift-monitoring", "prometheus-k8s-0", "sh", "-c", getCmd)
		if err != nil || !strings.Contains(metrics, metricString) {
			return false, nil
		}
		return true, err
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The metrics %s failed to contain %s", metrics, metricString))
}
