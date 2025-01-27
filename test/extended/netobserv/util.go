package netobserv

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type TestServerTemplate struct {
	ServerNS    string
	LargeBlob   string
	ServiceType string
	Template    string
}

type TestClientTemplate struct {
	ServerNS   string
	ClientNS   string
	ObjectSize string
	Template   string
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

// contain checks if b is an elememt of a
func contain(a []string, b string) bool {
	for _, c := range a {
		if c == b {
			return true
		}
	}
	return false
}

func getProxyFromEnv() string {
	var proxy string
	if os.Getenv("http_proxy") != "" {
		proxy = os.Getenv("http_proxy")
	} else if os.Getenv("http_proxy") != "" {
		proxy = os.Getenv("https_proxy")
	}
	return proxy
}

func getRouteAddress(oc *exutil.CLI, ns, routeName string) string {
	route, err := oc.AdminRouteClient().RouteV1().Routes(ns).Get(context.Background(), routeName, metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	return route.Spec.Host
}

func processTemplate(oc *exutil.CLI, parameters ...string) (string, error) {
	var configFile string
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 15*time.Second, false, func(context.Context) (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + ".json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	return configFile, err
}

// delete the objects in the cluster
func (r Resource) clear(oc *exutil.CLI) error {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", r.Namespace, r.Kind, r.Name).Output()
	if err != nil {
		errstring := fmt.Sprintf("%v", msg)
		if strings.Contains(errstring, "NotFound") || strings.Contains(errstring, "the server doesn't have a resource type") {
			return nil
		}
		return err
	}
	err = r.waitUntilResourceIsGone(oc)
	return err
}

// expect: true means we want the resource contain/compare with the expectedContent, false means the resource is expected not to compare with/contain the expectedContent;
// compare: true means compare the expectedContent with the resource content, false means check if the resource contains the expectedContent;
// args are the arguments used to execute command `oc.AsAdmin.WithoutNamespace().Run("get").Args(args...).Output()`;
func checkResource(oc *exutil.CLI, expect, compare bool, expectedContent string, args []string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, false, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
		if err != nil {
			if strings.Contains(output, "NotFound") {
				return false, nil
			}
			return false, err
		}
		if compare {
			res := strings.Compare(output, expectedContent)
			if (res == 0 && expect) || (res != 0 && !expect) {
				return true, nil
			}
			return false, nil
		}
		res := strings.Contains(output, expectedContent)
		if (res && expect) || (!res && !expect) {
			return true, nil
		}
		return false, nil
	})
	if expect {
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The content doesn't match/contain %s", expectedContent))
	} else {
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The %s still exists in the resource", expectedContent))
	}
}

// return the infrastructureName. For example:  anli922-jglp4
func getInfrastructureName(oc *exutil.CLI) string {
	infrastructureName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.infrastructureName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return infrastructureName
}

func patchResourceAsAdmin(oc *exutil.CLI, ns, resource, rsname, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, rsname, "--type=json", "-p", patch, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (r Resource) waitForResourceToAppear(oc *exutil.CLI) error {
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, false, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", r.Namespace, r.Kind, r.Name).Output()
		if err != nil {
			msg := fmt.Sprintf("%v", output)
			if strings.Contains(msg, "NotFound") {
				return false, nil
			}
			return false, err
		}
		e2e.Logf("Find %s %s", r.Kind, r.Name)
		return true, nil
	})
	return err
}

// WaitUntilResourceIsGone waits for the resource to be removed cluster
func (r Resource) waitUntilResourceIsGone(oc *exutil.CLI) error {
	return wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, false, func(context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", r.Namespace, r.Kind, r.Name).Output()
		if err != nil {
			errstring := fmt.Sprintf("%v", output)
			if strings.Contains(errstring, "NotFound") || strings.Contains(errstring, "the server doesn't have a resource type") || strings.Contains(errstring, "not found") {
				return true, nil
			}
			return true, err
		}
		return false, nil
	})
}

func (r Resource) applyFromTemplate(oc *exutil.CLI, parameters ...string) error {
	parameters = append(parameters, "-n", r.Namespace)
	file, err := processTemplate(oc, parameters...)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", r.Namespace).Execute()
	r.waitForResourceToAppear(oc)
	return err
}

// For admin user to create resources in the specified namespace from the file (not template)
func applyResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// For normal user to create resources in the specified namespace from the file (not template)
func createResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func WaitForPodsReadyWithLabel(oc *exutil.CLI, ns, label string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, false, func(context.Context) (done bool, err error) {
		pods, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: label})
		if err != nil {
			return false, err
		}
		if len(pods.Items) == 0 {
			e2e.Logf("Waiting for pod with label %s to appear\n", label)
			return false, nil
		}
		ready := true
		for _, pod := range pods.Items {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if !containerStatus.Ready {
					ready = false
					break
				}
			}
		}
		if !ready {
			e2e.Logf("Waiting for pod with label %s to be ready...\n", label)
		}
		return ready, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The pod with label %s is not availabile", label))
}

// WaitForDeploymentPodsToBeReady waits for the specific deployment to be ready
func waitForDeploymentPodsToBeReady(oc *exutil.CLI, namespace, name string) error {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, false, func(context.Context) (done bool, err error) {
		deployment, err := oc.AdminKubeClient().AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for availability of deployment/%s\n", name)
				return false, nil
			}
			return false, err
		}
		if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas && deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas {
			e2e.Logf("Deployment %s available (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
			return true, nil
		}
		e2e.Logf("Waiting for full availability of %s deployment (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
		return false, nil
	})
	return err
}

func waitForStatefulsetReady(oc *exutil.CLI, namespace, name string) error {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, false, func(context.Context) (done bool, err error) {
		ss, err := oc.AdminKubeClient().AppsV1().StatefulSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for availability of %s statefulset\n", name)
				return false, nil
			}
			return false, err
		}
		if ss.Status.ReadyReplicas == *ss.Spec.Replicas && ss.Status.UpdatedReplicas == *ss.Spec.Replicas {
			e2e.Logf("statefulset %s available (%d/%d)\n", name, ss.Status.ReadyReplicas, *ss.Spec.Replicas)
			return true, nil
		}
		e2e.Logf("Waiting for full availability of %s statefulset (%d/%d)\n", name, ss.Status.ReadyReplicas, *ss.Spec.Replicas)
		return false, nil
	})
	return err
}

func getSecrets(oc *exutil.CLI, namespace string) (string, error) {
	var secrets string
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 360*time.Second, false, func(context.Context) (done bool, err error) {
		secrets, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "-n", namespace, "-o", "jsonpath='{range .items[*]}{.metadata.name}{\" \"}'").Output()

		if err != nil {
			return false, err
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Secrets not available")
	return secrets, err
}

// check pods with label that are fully deleted
func checkPodDeleted(oc *exutil.CLI, ns, label, checkValue string) {
	podCheck := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 240*time.Second, false, func(context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Output()
		if err != nil || strings.Contains(output, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(podCheck, fmt.Sprintf("Pod \"%s\" exists or not fully deleted", checkValue))
}

func getSAToken(oc *exutil.CLI, name, ns string) string {
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", name, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return token
}

func doHTTPRequest(header http.Header, address, path, query, method string, quiet bool, attempts int, requestBody io.Reader, expectedStatusCode int) ([]byte, error) {
	us, err := buildURL(address, path, query)
	if err != nil {
		return nil, err
	}
	if !quiet {
		e2e.Logf(us)
	}

	req, err := http.NewRequest(strings.ToUpper(method), us, requestBody)
	if err != nil {
		return nil, err
	}

	req.Header = header

	var tr *http.Transport
	proxy := getProxyFromEnv()
	if len(proxy) > 0 {
		proxyURL, err := url.Parse(proxy)
		o.Expect(err).NotTo(o.HaveOccurred())
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy:           http.ProxyURL(proxyURL),
		}
	} else {
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	client := &http.Client{Transport: tr}

	var resp *http.Response
	success := false

	for attempts > 0 {
		attempts--

		resp, err = client.Do(req)
		if err != nil {
			e2e.Logf("error sending request %v", err)
			continue
		}
		if resp.StatusCode != expectedStatusCode {
			buf, _ := io.ReadAll(resp.Body) // nolint
			e2e.Logf("Error response from server: %s %s (%v), attempts remaining: %d", resp.Status, string(buf), err, attempts)
			if err := resp.Body.Close(); err != nil {
				e2e.Logf("error closing body %v", err)
			}
			continue
		}
		success = true
		break
	}
	if !success {
		return nil, fmt.Errorf("run out of attempts while querying the server")
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			e2e.Logf("error closing body %v", err)
		}
	}()
	return io.ReadAll(resp.Body)
}

func (testTemplate *TestServerTemplate) createServer(oc *exutil.CLI) error {
	templateParams := []string{"--ignore-unknown-parameters=true", "-f", testTemplate.Template, "-p", "SERVER_NS=" + testTemplate.ServerNS}

	if testTemplate.LargeBlob != "" {
		templateParams = append(templateParams, "-p", "LARGE_BLOB="+testTemplate.LargeBlob)
	}
	if testTemplate.ServiceType != "" {
		templateParams = append(templateParams, "-p", "SERVICE_TYPE="+testTemplate.ServiceType)
	}
	configFile := exutil.ProcessTemplate(oc, templateParams...)

	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
	if err != nil {
		return err
	}
	return nil
}

func (testTemplate *TestClientTemplate) createClient(oc *exutil.CLI) error {
	templateParams := []string{"--ignore-unknown-parameters=true", "-f", testTemplate.Template, "-p", "SERVER_NS=" + testTemplate.ServerNS, "-p", "CLIENT_NS=" + testTemplate.ClientNS}

	if testTemplate.ObjectSize != "" {
		templateParams = append(templateParams, "-p", "OBJECT_SIZE="+testTemplate.ObjectSize)
	}
	configFile := exutil.ProcessTemplate(oc, templateParams...)

	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
	if err != nil {
		return err
	}
	return nil
}

// wait until DaemonSet is Ready
func waitUntilDaemonSetReady(oc *exutil.CLI, daemonset, namespace string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(context.Context) (done bool, err error) {
		desiredNumber, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", daemonset, "-n", namespace, "-o", "jsonpath='{.status.desiredNumberScheduled}'").Output()

		if err != nil {
			// loop until daemonset is found or until timeout
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			return false, err
		}
		numberReady, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", daemonset, "-n", namespace, "-o", "jsonpath='{.status.numberReady}'").Output()
		if err != nil {
			return false, err
		}
		numberReadyi, err := strconv.Atoi(strings.Trim(numberReady, "'"))
		if err != nil {
			return false, err
		}

		desiredNumberi, err := strconv.Atoi(strings.Trim(desiredNumber, "'"))
		if err != nil {
			return false, err
		}
		if numberReadyi != desiredNumberi {
			return false, nil
		}
		updatedNumber, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", daemonset, "-n", namespace, "-o", "jsonpath='{.status.updatedNumberScheduled}'").Output()
		if err != nil {
			return false, err
		}
		updatedNumberi, err := strconv.Atoi(strings.Trim(updatedNumber, "'"))
		if err != nil {
			return false, err
		}
		if updatedNumberi != desiredNumberi {
			return false, nil
		}

		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Daemonset %s did not become Ready", daemonset))
}

// wait until Deployment is Ready
func waitUntilDeploymentReady(oc *exutil.CLI, deployment, ns string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(context.Context) (done bool, err error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", deployment, "-n", ns, "-o", "jsonpath='{.status.conditions[0].type}'").Output()

		if err != nil {
			// loop until deployment is found or until timeout
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			return false, err
		}

		if strings.Trim(status, "'") != "Available" {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Deployment %s did not become Available", deployment))
}

func getResourceGeneration(oc *exutil.CLI, resource, name, ns string) (int, error) {
	gen, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, name, "-o=jsonpath='{.metadata.generation}'", "-n", ns).Output()
	if err != nil {
		return -1, err
	}
	genI, err := strconv.Atoi(strings.Trim(gen, "'"))
	if err != nil {
		return -1, err
	}
	return genI, nil

}

func getResourceVersion(oc *exutil.CLI, resource, name, ns string) (int, error) {
	resV, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, name, "-o=jsonpath='{.metadata.resourceVersion}'", "-n", ns).Output()
	if err != nil {
		return -1, err
	}
	vers, err := strconv.Atoi(strings.Trim(resV, "'"))
	if err != nil {
		return -1, err
	}
	return vers, nil
}

func waitForResourceGenerationUpdate(oc *exutil.CLI, resource, name, field string, prev int, ns string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, false, func(context.Context) (done bool, err error) {
		var cur int
		if field == "generation" {
			cur, err = getResourceGeneration(oc, resource, name, ns)

		} else if field == "resourceVersion" {
			cur, err = getResourceVersion(oc, resource, name, ns)

		}
		if err != nil {
			return false, err
		}
		if cur != prev {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s/%s generation did not update", resource, name))
}

func checkResourceExists(oc *exutil.CLI, resource, name, ns string) (bool, error) {
	stdout, stderr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, name, "-n", ns).Outputs()
	if err != nil {
		return false, err
	}
	if strings.Contains(stderr, "NotFound") {
		return false, nil
	}
	if strings.Contains(stdout, name) {
		return true, nil
	}
	return false, nil
}

// get pod logs absolute path
func getPodLogs(oc *exutil.CLI, namespace, podname string) (string, error) {
	cargs := []string{"-n", namespace, podname}
	var podLogs string
	var err error

	// add polling as logs could be rotated
	err = wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
		podLogs, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args(cargs...).OutputToFile("podLogs.txt")

		if err != nil {
			e2e.Logf("unable to get the pod (%s) logs", podname)
			return false, err
		}
		podLogsf, err := os.Stat(podLogs)

		if err != nil {
			return false, err
		}
		return podLogsf.Size() > 0, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s pod logs were not collected", podname))

	e2e.Logf("pod logs file is %s", podLogs)
	return filepath.Abs(podLogs)
}

// check if NetworkAttachDefinition is created
func checkNAD(oc *exutil.CLI, nad, ns string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(context.Context) (done bool, err error) {
		nadOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("net-attach-def", nad, "-n", ns).Output()
		if err != nil {
			// loop until NAD is found or until timeout
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			return false, err
		}
		if !strings.Contains(nadOutput, nad) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Network Attach Definition %s did not become Available", nad))
}

// wait until hyperconverged is ready
func waitUntilHyperConvergedReady(oc *exutil.CLI, hc, ns string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(context.Context) (done bool, err error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hyperconverged", hc, "-n", ns, "-o", "jsonpath='{.status.conditions[0].status}'").Output()

		if err != nil {
			// loop until hyperconverged is found or until timeout
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			return false, err
		}

		if strings.Trim(status, "'") != "True" {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("HyperConverged %s did not become Available", hc))
}

// wait until virtual machine is Ready
func waitUntilVMReady(oc *exutil.CLI, vm, ns string) {
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 1200*time.Second, false, func(context.Context) (done bool, err error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("virtualmachine", vm, "-n", ns, "-o", "jsonpath='{.status.conditions[0].status}'").Output()

		if err != nil {
			// loop until virtual machine is found or until timeout
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			return false, err
		}

		if strings.Trim(status, "'") != "True" {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Virtual machine %s did not become Available", vm))
}

// wait until catalogSource is Ready
func WaitUntilCatSrcReady(oc *exutil.CLI, catSrc string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 600*time.Second, false, func(context.Context) (done bool, err error) {
		state, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", catSrc, "-n", "openshift-marketplace", "-o", "jsonpath='{.status.connectionState.lastObservedState}'").Output()
		if err != nil {
			// loop until virtual machine is found or until timeout
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			return false, err
		}

		if strings.Trim(state, "'") != "READY" {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Catalog Source %s did not become Ready", catSrc))
}

// check if cluster has baremetal workers
func hasMetalWorkerNodes(oc *exutil.CLI) bool {
	workers, err := exutil.GetClusterNodesBy(oc, "worker")
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, w := range workers {
		Output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", w, "-o", "jsonpath='{.metadata.labels.node\\.kubernetes\\.io/instance-type}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(Output, "metal") {
			e2e.Logf("Cluster does not have metal worker nodes")
			return false
		}
	}
	return true
}

// check resource is fully deleted
func checkResourceDeleted(oc *exutil.CLI, resourceType, resourceName, namespace string) {
	resourceCheck := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 600*time.Second, false, func(context.Context) (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(resourceType, resourceName, "-n", namespace).Output()
		if !strings.Contains(output, "NotFound") {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(resourceCheck, fmt.Sprintf("found %s \"%s\" exist or not fully deleted", resourceType, resourceName))
}

// delete a resource
func deleteResource(oc *exutil.CLI, resourceType, resourceName, namespace string, optionalParameters ...string) {
	cmdArgs := []string{resourceType, resourceName, "-n", namespace}
	cmdArgs = append(cmdArgs, optionalParameters...)
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(cmdArgs...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	checkResourceDeleted(oc, resourceType, resourceName, namespace)
}

// get kubeadmin token of the cluster
func getKubeAdminToken(oc *exutil.CLI, kubeAdminPasswd, serverUrl, currentContext string) string {
	longinErr := oc.WithoutNamespace().Run("login").Args("-u", "kubeadmin", "-p", kubeAdminPasswd, serverUrl).NotShowInfo().Execute()
	o.Expect(longinErr).NotTo(o.HaveOccurred())
	kubeadminToken, kubeadminTokenErr := oc.WithoutNamespace().Run("whoami").Args("-t").Output()
	o.Expect(kubeadminTokenErr).NotTo(o.HaveOccurred())

	rollbackCtxErr := oc.WithoutNamespace().Run("config").Args("set", "current-context", currentContext).Execute()
	o.Expect(rollbackCtxErr).NotTo(o.HaveOccurred())
	return kubeadminToken
}
