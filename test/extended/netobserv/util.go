package netobserv

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
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

type flowlogsPipelineDescription struct {
	serviceNs string
	name      string
	cmname    string
	template  string
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
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
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

// return the infrastructureName. For example:  anli922-jglp4
func getInfrastructureName(oc *exutil.CLI) string {
	infrastructureName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.infrastructureName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return infrastructureName
}

func (flowlogsPipeline *flowlogsPipelineDescription) create(oc *exutil.CLI, ns string, flowlogsPipelineDeploymenTemplate string) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", flowlogsPipelineDeploymenTemplate, "-p", "NAMESPACE="+ns)
}

func waitPodReady(oc *exutil.CLI, ns string, label string) {
	podName := getFlowlogsPipelinePod(oc, ns, label)
	exutil.AssertPodToBeReady(oc, podName, ns)
}

func patchResourceAsAdmin(oc *exutil.CLI, ns, resource, rsname, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, rsname, "--type=json", "-p", patch, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getFlowlogsPipelineCollector(oc *exutil.CLI, resource string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("get flowCollector: %v", output)
	return output
}

// get name of flowlogsPipeline pod by label
func getFlowlogsPipelinePod(oc *exutil.CLI, ns string, name string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", "app="+name, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of podname:%v", podName)
	return podName
}

// Verify some key and deterministic fields and their values
func verifyFlowRecord(podLog string) {
	re := regexp.MustCompile(`{\"Bytes\":.*}`)
	//e2e.Logf("the logs of flowlogs-pipeline pods are: %v", podLog)
	flowRecords := re.FindAllString(podLog, -3)
	//e2e.Logf("The flowRecords %v\n\n\n", flowRecords)
	for i, flow := range flowRecords {
		e2e.Logf("The %d th flow record is: %v\n\n\n", i, flow)
		o.Expect(flow).Should(o.And(
			o.MatchRegexp("Bytes.:[0-9]+"),
			o.MatchRegexp("TimeFlowEndMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowStartMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeReceived.:[1-9][0-9]+")))
	}
}

// Verify metrics by doing curl commands
func verifyCurl(oc *exutil.CLI, podName string, ns string, curlDest string, CertPath string) {
	command := []string{"exec", "-n", ns, podName, "--", "curl", "-s", "-v", "-L", curlDest, "--cacert", CertPath}
	output, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().OutputToFile("metrics.txt")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).NotTo(o.BeEmpty(), "No Metrics found")

	// grep the HTTPS Code
	metric1, _ := exec.Command("bash", "-c", "cat "+output+" | grep -o \"HTTP/2.*\"| tail -1 | awk '{print $2}'").Output()
	httpCode := strings.TrimSpace(string(metric1))
	e2e.Logf("The http code is : %v", httpCode)
	o.Expect(httpCode).NotTo(o.BeEmpty(), "HTTP Code not found")

	// grep the number of flows processed
	metric2, _ := exec.Command("bash", "-c", "cat "+output+" | grep  -o \"ingest_flows_processed.*\" | tail -1 | awk '{print $2}'").Output()
	flowLogsProcessed := strings.TrimSpace(string(metric2))
	e2e.Logf("The number of flowslogs processed are : %v", flowLogsProcessed)
	o.Expect(flowLogsProcessed).NotTo(o.BeEmpty(), "The number of flowlogs processed is empty")

	// grep the number of loki records written
	metric3, _ := exec.Command("bash", "-c", "cat "+output+" | grep -o \"records_written.*\" | tail -1 | awk '{print $2}'").Output()
	lokiRecordsWritten := strings.TrimSpace(string(metric3))
	e2e.Logf("The number of loki records written are : %v", lokiRecordsWritten)
	o.Expect(lokiRecordsWritten).NotTo(o.BeEmpty(), "The number of loki records written is empty")

	flowsProcessedInt, err := strconv.ParseInt(flowLogsProcessed, 10, 64)
	if err == nil {
		e2e.Logf("%d of type %T", flowsProcessedInt, flowsProcessedInt)
	}

	lokiRecordsWrittenInt, err := strconv.ParseInt(lokiRecordsWritten, 10, 64)
	if err == nil {
		e2e.Logf("%d of type %T", lokiRecordsWrittenInt, lokiRecordsWrittenInt)
	}

	// verify all the metrics
	o.Expect(httpCode).To(o.Equal("200"))
	o.Expect(flowsProcessedInt).Should(o.BeNumerically(">", 0))
	o.Expect(lokiRecordsWrittenInt).Should(o.BeNumerically(">", 0))
}

func verifyTime(oc *exutil.CLI, namespace string, lokiStackName string, lokiStackNS string) {
	var s string
	bearerToken := getSAToken(oc, "flowlogs-pipeline", namespace)
	route := "https://" + getRouteAddress(oc, lokiStackNS, lokiStackName)
	lc := newLokiClient(route).withToken(bearerToken).retry(5)
	res, err := lc.searchLogsInLoki("network", "{app=\"netobserv-flowcollector\"}")
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(res.Data.Result) == 0 {
		exutil.AssertWaitPollNoErr(err, "network logs are not found")
	}

	for _, r := range res.Data.Result {
		e2e.Logf("\nlog is : %v\n", r.Values[0])
		s = fmt.Sprint(r.Values[0])
	}
	l := strings.Split(s, " ")

	ltime := strings.Replace(l[0], "[", "", 1)

	logtime, err := strconv.ParseInt(ltime, 10, 64)
	if err == nil {
		e2e.Logf("%d of type %T", logtime, logtime)
	}

	now := time.Now().UnixNano()

	timeminus := now - logtime
	o.Expect(timeminus).Should(o.BeNumerically(">", 0))
	o.Expect(timeminus).Should(o.BeNumerically("<=", 300000000000))
}

func (r resource) waitForResourceToAppear(oc *exutil.CLI) {
	err := wait.Poll(3*time.Second, 180*time.Second, func() (done bool, err error) {
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

func (r resource) applyFromTemplate(oc *exutil.CLI, parameters ...string) error {
	parameters = append(parameters, "-n", r.namespace)
	file, err := processTemplate(oc, parameters...)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", r.namespace).Execute()
	r.waitForResourceToAppear(oc)
	return err
}

func waitForPodReadyWithLabel(oc *exutil.CLI, ns string, label string) {
	err := wait.Poll(5*time.Second, 180*time.Second, func() (done bool, err error) {
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

// WaitUntilResourceIsGone waits for the resource to be removed cluster
func (r resource) waitUntilResourceIsGone(oc *exutil.CLI) error {
	return wait.Poll(3*time.Second, 180*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", r.namespace, r.kind, r.name).Output()
		if err != nil {
			errstring := fmt.Sprintf("%v", output)
			if strings.Contains(errstring, "NotFound") || strings.Contains(errstring, "the server doesn't have a resource type") {
				return true, nil
			}
			return true, err
		}
		return false, nil
	})
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

// WaitForDeploymentPodsToBeReady waits for the specific deployment to be ready
func waitForDeploymentPodsToBeReady(oc *exutil.CLI, namespace string, name string) {
	err := wait.Poll(5*time.Second, 180*time.Second, func() (done bool, err error) {
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
	err := wait.Poll(5*time.Second, 180*time.Second, func() (done bool, err error) {
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
