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

type TestClientServerTemplate struct {
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
func (r resource) clear(oc *exutil.CLI) error {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", r.namespace, r.kind, r.name).Output()
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

func (r resource) waitForResourceToAppear(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, false, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", r.namespace, r.kind, r.name).Output()
		if err != nil {
			msg := fmt.Sprintf("%v", output)
			if strings.Contains(msg, "NotFound") {
				return false, nil
			}
			return false, err
		}
		e2e.Logf("Find %s %s", r.kind, r.name)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %s/%s is not appear", r.kind, r.name))
}

// WaitUntilResourceIsGone waits for the resource to be removed cluster
func (r resource) waitUntilResourceIsGone(oc *exutil.CLI) error {
	return wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, false, func(context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", r.namespace, r.kind, r.name).Output()
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

func (r resource) applyFromTemplate(oc *exutil.CLI, parameters ...string) error {
	parameters = append(parameters, "-n", r.namespace)
	file, err := processTemplate(oc, parameters...)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", r.namespace).Execute()
	r.waitForResourceToAppear(oc)
	return err
}

func waitForPodReadyWithLabel(oc *exutil.CLI, ns string, label string) {
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
func waitForDeploymentPodsToBeReady(oc *exutil.CLI, namespace string, name string) {
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
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("deployment %s is not availabile", name))
}

func waitForStatefulsetReady(oc *exutil.CLI, namespace string, name string) {
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
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("statefulset %s is not availabile", name))
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
func checkPodDeleted(oc *exutil.CLI, ns string, label string, checkValue string) {
	podCheck := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 240*time.Second, false, func(context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Output()
		if err != nil || strings.Contains(output, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(podCheck, fmt.Sprintf("found \"%s\" exist or not fully deleted", checkValue))
}

// For normal user to create resources in the specified namespace from the file (not template)
func createResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", file, "--namespace="+ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
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
				e2e.Logf("error closing body", err)
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
			e2e.Logf("error closing body", err)
		}
	}()
	return io.ReadAll(resp.Body)
}

func (testTemplate *TestClientServerTemplate) createTestClientServer(oc *exutil.CLI) error {
	configFile := exutil.ProcessTemplate(oc, "--ignore-unknown-parameters=true", "-f", testTemplate.Template, "-p", "SERVER_NS="+testTemplate.ServerNS, "-p", "CLIENT_NS="+testTemplate.ClientNS, "-p", "OBJECT_SIZE="+testTemplate.ObjectSize)

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
