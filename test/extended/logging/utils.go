package logging

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/structpb"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azarm "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	azpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	azto "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azlogs"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/operationalinsights/armoperationalinsights"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// SubscriptionObjects objects are used to create operators via OLM
type SubscriptionObjects struct {
	OperatorName       string
	Namespace          string
	OperatorGroup      string // the file used to create operator group
	Subscription       string // the file used to create subscription
	PackageName        string
	OperatorPodLabel   string               //The operator pod label which is used to select pod
	CatalogSource      CatalogSourceObjects `json:",omitempty"`
	SkipCaseWhenFailed bool                 // if true, the case will be skipped when operator is not ready, otherwise, the case will be marked as failed
}

// CatalogSourceObjects defines the source used to subscribe an operator
type CatalogSourceObjects struct {
	Channel         string `json:",omitempty"`
	SourceName      string `json:",omitempty"`
	SourceNamespace string `json:",omitempty"`
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

// containSubstring checks if b is a's element's substring
func containSubstring(a interface{}, b string) bool {
	switch reflect.TypeOf(a).Kind() {
	case reflect.Slice, reflect.Array:
		s := reflect.ValueOf(a)
		for i := 0; i < s.Len(); i++ {
			if strings.Contains(fmt.Sprintln(s.Index(i)), b) {
				return true
			}
		}
	}
	return false
}

func processTemplate(oc *exutil.CLI, parameters ...string) (string, error) {
	var configFile string
	filename := getRandomString() + ".json"
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 15*time.Second, true, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(filename)
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	if err != nil {
		return configFile, fmt.Errorf("failed to process template with the provided parameters")
	}
	return configFile, nil
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

func getClusterID(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-ojsonpath={.spec.clusterID}").Output()
}

func isFipsEnabled(oc *exutil.CLI) bool {
	nodes, err := exutil.GetSchedulableLinuxWorkerNodes(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	fips, err := exutil.DebugNodeWithChroot(oc, nodes[0].Name, "bash", "-c", "fips-mode-setup --check")
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Contains(fips, "FIPS mode is enabled.")
}

// waitForPackagemanifestAppear waits for the packagemanifest to appear in the cluster
// chSource: bool value, true means the packagemanifests' source name must match the so.CatalogSource.SourceName, e.g.: oc get packagemanifests xxxx -l catalog=$source-name
func (so *SubscriptionObjects) waitForPackagemanifestAppear(oc *exutil.CLI, chSource bool) {
	args := []string{"-n", so.CatalogSource.SourceNamespace, "packagemanifests"}
	if chSource {
		args = append(args, "-l", "catalog="+so.CatalogSource.SourceName)
	} else {
		args = append(args, so.PackageName)
	}
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		packages, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
		if err != nil {
			msg := fmt.Sprintf("%v", err)
			if strings.Contains(msg, "No resources found") || strings.Contains(msg, "NotFound") {
				return false, nil
			}
			return false, err
		}
		if strings.Contains(packages, so.PackageName) {
			return true, nil
		}
		e2e.Logf("Waiting for packagemanifest/%s to appear", so.PackageName)
		return false, nil
	})
	if err != nil {
		if so.SkipCaseWhenFailed {
			g.Skip(fmt.Sprintf("Skip the case for can't find packagemanifest/%s", so.PackageName))
		} else {
			e2e.Failf("Packagemanifest %s is not available", so.PackageName)
		}
	}
}

// setCatalogSourceObjects set the default values of channel, source namespace and source name if they're not specified
func (so *SubscriptionObjects) setCatalogSourceObjects(oc *exutil.CLI) {
	// set channel
	if so.CatalogSource.Channel == "" {
		so.CatalogSource.Channel = "stable"
	}

	// set source namespace
	if so.CatalogSource.SourceNamespace == "" {
		so.CatalogSource.SourceNamespace = "openshift-marketplace"
	}

	// set source and check if the packagemanifest exists or not
	if so.CatalogSource.SourceName != "" {
		so.waitForPackagemanifestAppear(oc, true)
	} else {
		catsrc, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("catsrc", "-n", so.CatalogSource.SourceNamespace, "qe-app-registry").Output()
		if catsrc != "" && !(strings.Contains(catsrc, "NotFound")) {
			so.CatalogSource.SourceName = "qe-app-registry"
			so.waitForPackagemanifestAppear(oc, true)
		} else {
			so.waitForPackagemanifestAppear(oc, false)
			source, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", so.PackageName, "-o", "jsonpath={.status.catalogSource}").Output()
			if err != nil {
				e2e.Logf("error getting catalog source name: %v", err)
			}
			so.CatalogSource.SourceName = source
		}
	}
}

// SubscribeOperator is used to subcribe the CLO and EO
func (so *SubscriptionObjects) SubscribeOperator(oc *exutil.CLI) {
	// check if the namespace exists, if it doesn't exist, create the namespace
	if so.OperatorPodLabel == "" {
		so.OperatorPodLabel = "name=" + so.OperatorName
	}
	_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), so.Namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			e2e.Logf("The project %s is not found, create it now...", so.Namespace)
			namespaceTemplate := exutil.FixturePath("testdata", "logging", "subscription", "namespace.yaml")
			namespaceFile, err := processTemplate(oc, "-f", namespaceTemplate, "-p", "NAMESPACE_NAME="+so.Namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer os.Remove(namespaceFile)
			err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
				output, err := oc.AsAdmin().Run("apply").Args("-f", namespaceFile).Output()
				if err != nil {
					if strings.Contains(output, "AlreadyExists") {
						return true, nil
					}
					return false, err
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can't create project %s", so.Namespace))
		}
	}

	// check the operator group, if no object found, then create an operator group in the project
	og, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "og").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	msg := fmt.Sprintf("%v", og)
	if strings.Contains(msg, "No resources found") {
		// create operator group
		ogFile, err := processTemplate(oc, "-n", so.Namespace, "-f", so.OperatorGroup, "-p", "OG_NAME="+so.Namespace, "NAMESPACE="+so.Namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(ogFile)
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
			output, err := oc.AsAdmin().Run("apply").Args("-f", ogFile, "-n", so.Namespace).Output()
			if err != nil {
				if strings.Contains(output, "AlreadyExists") {
					return true, nil
				}
				return false, err
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can't create operatorgroup %s in %s project", so.Namespace, so.Namespace))
	}

	// check subscription, if there is no subscription objets, then create one
	sub, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", so.Namespace, so.PackageName).Output()
	if err != nil {
		msg := fmt.Sprint("v%", sub)
		if strings.Contains(msg, "NotFound") {
			so.setCatalogSourceObjects(oc)
			//create subscription object
			currentPlatform := exutil.CheckPlatform(oc)
			if currentPlatform == "aws" && exutil.IsWorkloadIdentityCluster(oc) && so.PackageName == "loki-operator" {
				e2e.Logf("Deploying Loki in STS mode...")
				loggingBaseDir := exutil.FixturePath("testdata", "logging")
				so.Subscription = filepath.Join(loggingBaseDir, "subscription", "subscription-sts.yaml")
			}
			subscriptionFile, err := processTemplate(oc, "-n", so.Namespace, "-f", so.Subscription, "-p", "PACKAGE_NAME="+so.PackageName, "NAMESPACE="+so.Namespace, "CHANNEL="+so.CatalogSource.Channel, "SOURCE="+so.CatalogSource.SourceName, "SOURCE_NAMESPACE="+so.CatalogSource.SourceNamespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer os.Remove(subscriptionFile)
			err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
				output, err := oc.AsAdmin().Run("apply").Args("-f", subscriptionFile, "-n", so.Namespace).Output()
				if err != nil {
					if strings.Contains(output, "AlreadyExists") {
						return true, nil
					}
					return false, err
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can't create subscription %s in %s project", so.PackageName, so.Namespace))
			// check status in subscription
			err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(context.Context) (done bool, err error) {
				installPlanApproval, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "sub", so.PackageName, `-o=jsonpath={.spec.installPlanApproval}`).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if installPlanApproval == "Manual" {
					installPlanID, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "sub", so.PackageName, `-o=jsonpath={.status.installPlanRef.name}`).Output()
					installPlanApprovedStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "InstallPlans", installPlanID, `-o=jsonpath={.spec.approved}`).Output()
					if installPlanApprovedStatus == "false" {
						oc.AsAdmin().WithoutNamespace().Run("patch").Args("InstallPlans", installPlanID, "-n", so.Namespace, "-p", `{"spec":{"approved":true}}`, "--type=merge").Execute()
					}
				}
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "sub", so.PackageName, `-ojsonpath={.status.state}`).Output()
				if err != nil {
					e2e.Logf("error getting subscription/%s: %v", so.PackageName, err)
					return false, nil
				}
				return strings.Contains(output, "AtLatestKnown"), nil
			})
			if err != nil {
				out, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "sub", so.PackageName, `-ojsonpath={.status.conditions}`).Output()
				e2e.Logf("subscription/%s is not ready, conditions: %v", so.PackageName, out)
				if so.SkipCaseWhenFailed {
					g.Skip(fmt.Sprintf("Skip the case for the operator %s is not ready", so.OperatorName))
				} else {
					e2e.Failf("can't deploy operator %s", so.OperatorName)
				}
			}
		}
	}

	// check pod status
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		pods, err := oc.AdminKubeClient().CoreV1().Pods(so.Namespace).List(context.Background(), metav1.ListOptions{LabelSelector: so.OperatorPodLabel})
		if err != nil {
			e2e.Logf("Hit error %v when getting pods", err)
			return false, nil
		}
		if len(pods.Items) == 0 {
			e2e.Logf("Waiting for pod with label %s to appear\n", "name="+so.OperatorName)
			return false, nil
		}
		ready := true
		for _, pod := range pods.Items {
			if pod.Status.Phase != "Running" {
				ready = false
				e2e.Logf("Pod %s is not running: %v", pod.Name, pod.Status.Phase)
				break
			}
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if !containerStatus.Ready {
					ready = false
					e2e.Logf("Container %s in pod %s is not ready", &containerStatus.Name, pod.Name)
					break
				}
			}
		}
		return ready, nil
	})
	if err != nil {
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", so.Namespace, "-l", so.OperatorPodLabel).Execute()
		podStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", so.Namespace, "-l", so.OperatorPodLabel, "-ojsonpath={.items[*].status.conditions}").Output()
		containerStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", so.Namespace, "-l", so.OperatorPodLabel, "-ojsonpath={.items[*].status.containerStatuses}").Output()
		e2e.Logf("pod with label %s is not ready:\nconditions: %s\ncontainer status: %s", so.OperatorPodLabel, podStatus, containerStatus)
		if so.SkipCaseWhenFailed {
			g.Skip(fmt.Sprintf("Skip the case for the operator %s is not ready", so.OperatorName))
		} else {
			e2e.Failf("can't deploy operator %s", so.OperatorName)
		}
	}
}

func (so *SubscriptionObjects) uninstallOperator(oc *exutil.CLI) {
	//csv, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "sub/"+so.PackageName, "-ojsonpath={.status.installedCSV}").Output()
	resource{"subscription", so.PackageName, so.Namespace}.clear(oc)
	//_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", so.Namespace, "csv", csv).Execute()
	_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", so.Namespace, "csv", "-l", "operators.coreos.com/"+so.PackageName+"."+so.Namespace+"=").Execute()
	// do not remove namespace openshift-logging and openshift-operators-redhat, and preserve the operatorgroup as there may have several operators deployed in one namespace
	// for example: loki-operator and elasticsearch-operator
	if so.Namespace != "openshift-logging" && so.Namespace != "openshift-operators-redhat" && !strings.HasPrefix(so.Namespace, "e2e-test-") {
		deleteNamespace(oc, so.Namespace)
	}
}

func (so *SubscriptionObjects) getInstalledCSV(oc *exutil.CLI) string {
	installedCSV, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "sub", so.PackageName, "-ojsonpath={.status.installedCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return installedCSV
}

// WaitForDeploymentPodsToBeReady waits for the specific deployment to be ready
func WaitForDeploymentPodsToBeReady(oc *exutil.CLI, namespace string, name string) {
	var selectors map[string]string
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		deployment, err := oc.AdminKubeClient().AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for deployment/%s to appear\n", name)
				return false, nil
			}
			return false, err
		}
		selectors = deployment.Spec.Selector.MatchLabels
		if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas && deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas {
			e2e.Logf("Deployment %s available (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
			return true, nil
		}
		e2e.Logf("Waiting for full availability of %s deployment (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
		return false, nil
	})
	if err != nil && len(selectors) > 0 {
		var labels []string
		for k, v := range selectors {
			labels = append(labels, k+"="+v)
		}
		label := strings.Join(labels, ",")
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label).Execute()
		podStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label, "-ojsonpath={.items[*].status.conditions}").Output()
		containerStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label, "-ojsonpath={.items[*].status.containerStatuses}").Output()
		e2e.Failf("deployment %s is not ready:\nconditions: %s\ncontainer status: %s", name, podStatus, containerStatus)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("deployment %s is not available", name))
}

func waitForStatefulsetReady(oc *exutil.CLI, namespace string, name string) {
	var selectors map[string]string
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		ss, err := oc.AdminKubeClient().AppsV1().StatefulSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for statefulset/%s to appear\n", name)
				return false, nil
			}
			return false, err
		}
		selectors = ss.Spec.Selector.MatchLabels
		if ss.Status.ReadyReplicas == *ss.Spec.Replicas && ss.Status.UpdatedReplicas == *ss.Spec.Replicas {
			e2e.Logf("statefulset %s available (%d/%d)\n", name, ss.Status.ReadyReplicas, *ss.Spec.Replicas)
			return true, nil
		}
		e2e.Logf("Waiting for full availability of %s statefulset (%d/%d)\n", name, ss.Status.ReadyReplicas, *ss.Spec.Replicas)
		return false, nil
	})
	if err != nil && len(selectors) > 0 {
		var labels []string
		for k, v := range selectors {
			labels = append(labels, k+"="+v)
		}
		label := strings.Join(labels, ",")
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label).Execute()
		podStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label, "-ojsonpath={.items[*].status.conditions}").Output()
		containerStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", label, "-ojsonpath={.items[*].status.containerStatuses}").Output()
		e2e.Failf("statefulset %s is not ready:\nconditions: %s\ncontainer status: %s", name, podStatus, containerStatus)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("statefulset %s is not available", name))
}

// WaitForDaemonsetPodsToBeReady waits for all the pods controlled by the ds to be ready
func WaitForDaemonsetPodsToBeReady(oc *exutil.CLI, ns string, name string) {
	var selectors map[string]string
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		daemonset, err := oc.AdminKubeClient().AppsV1().DaemonSets(ns).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for daemonset/%s to appear\n", name)
				return false, nil
			}
			return false, err
		}
		selectors = daemonset.Spec.Selector.MatchLabels
		if daemonset.Status.DesiredNumberScheduled > 0 && daemonset.Status.NumberReady == daemonset.Status.DesiredNumberScheduled && daemonset.Status.UpdatedNumberScheduled == daemonset.Status.DesiredNumberScheduled {
			e2e.Logf("Daemonset/%s is available (%d/%d)\n", name, daemonset.Status.NumberReady, daemonset.Status.DesiredNumberScheduled)
			return true, nil
		}
		e2e.Logf("Waiting for full availability of %s daemonset (%d/%d)\n", name, daemonset.Status.NumberReady, daemonset.Status.DesiredNumberScheduled)
		return false, nil
	})
	if err != nil && len(selectors) > 0 {
		var labels []string
		for k, v := range selectors {
			labels = append(labels, k+"="+v)
		}
		label := strings.Join(labels, ",")
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Execute()
		podStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.conditions}").Output()
		containerStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.containerStatuses}").Output()
		e2e.Failf("daemonset %s is not ready:\nconditions: %s\ncontainer status: %s", name, podStatus, containerStatus)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Daemonset %s is not available", name))
}

func waitForPodReadyWithLabel(oc *exutil.CLI, ns string, label string) {
	var count int
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		pods, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: label})
		if err != nil {
			return false, err
		}
		count = len(pods.Items)
		if count == 0 {
			e2e.Logf("Waiting for pod with label %s to appear\n", label)
			return false, nil
		}
		ready := true
		for _, pod := range pods.Items {
			if pod.Status.Phase != "Running" {
				ready = false
				break
			}
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
	if err != nil && count != 0 {
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Execute()
		podStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.conditions}").Output()
		containerStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.containerStatuses}").Output()
		e2e.Failf("pod with label %s is not ready:\nconditions: %s\ncontainer status: %s", label, podStatus, containerStatus)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod with label %s is not ready", label))
}

// getDeploymentsNameByLabel retruns a list of deployment name which have specific labels
func getDeploymentsNameByLabel(oc *exutil.CLI, ns string, label string) []string {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		deployList, err := oc.AdminKubeClient().AppsV1().Deployments(ns).List(context.Background(), metav1.ListOptions{LabelSelector: label})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Can't get deployment(s) match label(s) %s, retrying...\n", label)
				return false, nil
			}
			return false, err
		}
		if len(deployList.Items) > 0 {
			return true, nil
		}
		return false, nil
	})
	if err == nil {
		deployList, err := oc.AdminKubeClient().AppsV1().Deployments(ns).List(context.Background(), metav1.ListOptions{LabelSelector: label})
		o.Expect(err).NotTo(o.HaveOccurred())
		expectedDeployments := make([]string, 0, len(deployList.Items))
		for _, deploy := range deployList.Items {
			expectedDeployments = append(expectedDeployments, deploy.Name)
		}
		return expectedDeployments
	}
	e2e.Logf("No deployment matches label(s) %s in %s project", label, ns)
	return nil
}

func getPodNames(oc *exutil.CLI, ns, label string) ([]string, error) {
	var names []string
	pods, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return names, err
	}
	if len(pods.Items) == 0 {
		return names, fmt.Errorf("no pod(s) match label %s in namespace %s", label, ns)
	}
	for _, pod := range pods.Items {
		names = append(names, pod.Name)
	}
	return names, nil
}

// WaitForECKPodsToBeReady checks if the EFK pods could become ready or not
func WaitForECKPodsToBeReady(oc *exutil.CLI, ns string) {
	//wait for ES
	esDeployNames := getDeploymentsNameByLabel(oc, ns, "cluster-name=elasticsearch")
	for _, name := range esDeployNames {
		WaitForDeploymentPodsToBeReady(oc, ns, name)
	}
	// wait for Kibana
	WaitForDeploymentPodsToBeReady(oc, ns, "kibana")
	// wait for collector
	WaitForDaemonsetPodsToBeReady(oc, ns, "collector")
}

type resource struct {
	kind      string
	name      string
	namespace string
}

// WaitUntilResourceIsGone waits for the resource to be removed cluster
func (r resource) WaitUntilResourceIsGone(oc *exutil.CLI) error {
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
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
	if err != nil {
		return fmt.Errorf("can't remove %s/%s in %s project", r.kind, r.name, r.namespace)
	}
	return nil
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
	err = r.WaitUntilResourceIsGone(oc)
	return err
}

func (r resource) WaitForResourceToAppear(oc *exutil.CLI) {
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
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
	defer os.Remove(file)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", r.namespace).Output()
	if err != nil {
		return fmt.Errorf(output)
	}
	r.WaitForResourceToAppear(oc)
	return nil
}

// deleteClusterLogging deletes the clusterlogging instance which isn't created by `func (cl *clusterlogging) create(oc *exutil.CLI, optionalParameters ...string)`
// and ensures the related resources are removed
func deleteClusterLogging(oc *exutil.CLI, name, namespace string) {
	clOutput, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterlogging", name, "-n", namespace, "-ojson").Output()
	if len(clOutput) > 0 && !strings.Contains(clOutput, fmt.Sprint("clusterloggings.logging.openshift.io \""+name+"\" not found")) {
		err := resource{"clusterlogging", name, namespace}.clear(oc)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("could not delete clusterlogging/%s in %s project", name, namespace))

		cl := ClusterLogging{}
		json.Unmarshal([]byte(clOutput), &cl)
		//make sure other resources are removed
		resources := []resource{{"daemonset", "collector", namespace}}
		if *cl.Spec.LogStoreSpec.Type == "elasticsearch" {
			resources = append(resources, resource{"elasticsearches.logging.openshift.io", "elasticsearch", namespace})
			if len(cl.Spec.LogStoreSpec.ElasticsearchSpec.Storage.StorageClassName) > 0 {
				// remove all the pvcs in the namespace
				_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", namespace, "pvc", "-l", "logging-cluster=elasticsearch").Execute()
			}
		}
		if *cl.Spec.VisualizationSpec.Type == "kibana" {
			resources = append(resources, resource{"kibanas.logging.openshift.io", "kibana", namespace})
		} else if *cl.Spec.VisualizationSpec.Type == "ocp-console" {
			resources = append(resources, resource{"deployment", "logging-view-plugin", namespace})
		}
		for i := 0; i < len(resources); i++ {
			err = resources[i].WaitUntilResourceIsGone(oc)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s/%s is not deleted", resources[i].kind, resources[i].name))
		}
	}
}

type clusterlogging struct {
	name             string // default: instance
	namespace        string // default: openshift-logging
	collectorType    string // default: vector
	logStoreType     string // `elasticsearch` or `lokistack`, no default value
	esNodeCount      int    // if it's specified, parameter `ES_NODE_COUNT=${esNodeCount}` will be added when creating the CR
	storageClassName string // works when the logStoreType is elasticsearch
	storageSize      string // works when the logStoreType is elasticsearch and the storageClassName is specified
	lokistackName    string // required value when logStoreType is lokistack
	templateFile     string // the template used to create clusterlogging, no default value
	waitForReady     bool   // if true, will wait for all the logging pods to be ready after creating the CR
}

// create a clusterlogging CR from a template
func (cl *clusterlogging) create(oc *exutil.CLI, optionalParameters ...string) {
	if cl.name == "" {
		cl.name = "instance"
	}
	if cl.namespace == "" {
		cl.namespace = loggingNS
	}
	// In case of there is a clusterlogging in the namespace, add a step to check&remove the existing CR before creating it.
	deleteClusterLogging(oc, cl.name, cl.namespace)

	if cl.collectorType == "" {
		cl.collectorType = "vector"
	}

	if cl.storageClassName != "" && cl.storageSize == "" {
		cl.storageSize = "20Gi"
	}
	parameters := []string{"-p", "NAME=" + cl.name, "NAMESPACE=" + cl.namespace, "COLLECTOR=" + cl.collectorType}
	if cl.logStoreType == "elasticsearch" {
		if cl.esNodeCount > 0 {
			parameters = append(parameters, "ES_NODE_COUNT="+strconv.Itoa(cl.esNodeCount))
		}
		if cl.storageClassName != "" {
			parameters = append(parameters, "STORAGE_CLASS="+cl.storageClassName, "PVC_SIZE="+cl.storageSize)
		}
	} else if cl.logStoreType == "lokistack" {
		if cl.lokistackName == "" {
			e2e.Failf("lokistack name is not provided")
		}
		parameters = append(parameters, "LOKISTACKNAME="+cl.lokistackName)
	}
	if len(optionalParameters) > 0 {
		parameters = append(parameters, optionalParameters...)
	}
	//parameters = append(parameters, "-f", cl.templateFile, "--ignore-unknown-parameters=true")
	parameters = append(parameters, "-f", cl.templateFile)
	file, processErr := processTemplate(oc, parameters...)
	defer os.Remove(file)
	if processErr != nil {
		e2e.Failf("error processing file: %v", processErr)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", cl.namespace).Execute()
	if err != nil {
		e2e.Failf("error creating clusterlogging: %v", err)
	}
	resource{"clusterlogging", cl.name, cl.namespace}.WaitForResourceToAppear(oc)

	if cl.waitForReady {
		cl.waitForLoggingReady(oc)
	}
}

// update clusterlogging CR
// if template is specified, then run command `oc process -f template -p patches | oc apply -f -`
// if template is not specified, then run command `oc patch clusterlogging/${cl.name} -p patches`
// if use patch, should add `--type=xxxx` in the end of patches
func (cl *clusterlogging) update(oc *exutil.CLI, template string, patches ...string) {
	var err error
	if template != "" {
		//parameters := []string{"-f", template, "--ignore-unknown-parameters=true", "-p", "NAME=" + cl.name, "NAMESPACE=" + cl.namespace}
		parameters := []string{"-f", template, "-p", "NAME=" + cl.name, "NAMESPACE=" + cl.namespace}
		if len(patches) > 0 {
			parameters = append(parameters, patches...)
		}
		file, processErr := processTemplate(oc, parameters...)
		defer os.Remove(file)
		if processErr != nil {
			e2e.Failf("error processing file: %v", processErr)
		}
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", cl.namespace).Execute()
	} else {
		parameters := []string{"cl/" + cl.name, "-n", cl.namespace, "-p"}
		parameters = append(parameters, patches...)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(parameters...).Execute()
	}
	if err != nil {
		e2e.Failf("error updating clusterlogging: %v", err)
	}
}

// delete clusterlogging CR
func (cl *clusterlogging) delete(oc *exutil.CLI) {
	err := resource{"clusterlogging", cl.name, cl.namespace}.clear(oc)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("could not delete clusterlogging/%s in %s project", cl.name, cl.namespace))
	var resources []resource
	if cl.name == "instance" && cl.namespace == loggingNS {
		resources = append(resources, resource{"daemonset", "collector", cl.namespace})
	}
	if cl.logStoreType == "elasticsearch" {
		resources = append(resources, resource{"elasticsearches.logging.openshift.io", "elasticsearch", cl.namespace}, resource{"kibanas.logging.openshift.io", "kibana", cl.namespace})
		if cl.storageClassName != "" {
			// remove all the pvcs in the namespace
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", cl.namespace, "pvc", "-l", "logging-cluster=elasticsearch").Execute()
		}
	} else if cl.logStoreType == "lokistack" {
		resources = append(resources, resource{"deployment", "logging-view-plugin", cl.namespace})
	}
	for i := 0; i < len(resources); i++ {
		err = resources[i].WaitUntilResourceIsGone(oc)
		if err != nil {
			e2e.Logf("%s/%s is not deleted", resources[i].kind, resources[i].name)
		}
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s/%s is not deleted", resources[i].kind, resources[i].name))
	}
}

// wait for the logging pods to be ready
func (cl *clusterlogging) waitForLoggingReady(oc *exutil.CLI) {
	if cl.logStoreType == "elasticsearch" {
		var esDeployNames []string
		err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(context.Context) (done bool, err error) {
			esDeployNames = getDeploymentsNameByLabel(oc, cl.namespace, "cluster-name=elasticsearch")
			if len(esDeployNames) != cl.esNodeCount {
				e2e.Logf("expect %d ES deployments, but only find %d, try next time...", cl.esNodeCount, len(esDeployNames))
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "some ES deployments are not created")

		for _, name := range esDeployNames {
			WaitForDeploymentPodsToBeReady(oc, cl.namespace, name)
		}
		// wait for Kibana
		WaitForDeploymentPodsToBeReady(oc, cl.namespace, "kibana")
	} else if cl.logStoreType == "lokistack" {
		WaitForDeploymentPodsToBeReady(oc, cl.namespace, "logging-view-plugin")
	}
	// wait for collector
	if cl.name == "instance" && cl.namespace == cloNS {
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, "collector")
	} else {
		WaitForDaemonsetPodsToBeReady(oc, cl.namespace, cl.name)
	}
}

type clusterlogforwarder struct {
	name                      string // default: instance
	namespace                 string // default: openshift-logging
	templateFile              string // the template used to create clusterlogforwarder, no default value
	secretName                string // optional, if it's specified, when creating CLF, the parameter `"SECRET_NAME="+clf.secretName` will be added automatically
	serviceAccountName        string // optional, only required when !(clf.name == "instance" && clf.namespace == "openshift-logging")
	collectApplicationLogs    bool   // optional, if true, will add cluster-role/collect-application-logs to the serviceAccount when !(clf.name == "instance" && clf.namespace == "openshift-logging")
	collectAuditLogs          bool   // optional, if true, will add cluster-role/collect-audit-logs to the serviceAccount when !(clf.name == "instance" && clf.namespace == "openshift-logging")
	collectInfrastructureLogs bool   // optional, if true, will add cluster-role/collect-infrastructure-logs to the serviceAccount when !(clf.name == "instance" && clf.namespace == "openshift-logging")
	waitForPodReady           bool   // optional, if true, will check daemonset stats when !(clf.name == "instance" && clf.namespace == "openshift-logging")
	enableMonitoring          bool   // optional, if true, will add label `openshift.io/cluster-monitoring: "true"` to the project, and create role/prometheus-k8s rolebinding/prometheus-k8s in the namespace, works when when !(clf.namespace == "openshift-operators-redhat" || clf.namespace == "openshift-logging")
}

// create clusterlogforwarder CR from a template
func (clf *clusterlogforwarder) create(oc *exutil.CLI, optionalParameters ...string) {
	if clf.name == "" {
		clf.name = "instance"
	}
	if clf.namespace == "" {
		clf.namespace = loggingNS
	}

	//parameters := []string{"-f", clf.templateFile, "--ignore-unknown-parameters=true", "-p", "NAME=" + clf.name, "NAMESPACE=" + clf.namespace}
	parameters := []string{"-f", clf.templateFile, "-p", "NAME=" + clf.name, "NAMESPACE=" + clf.namespace}
	if clf.secretName != "" {
		parameters = append(parameters, "SECRET_NAME="+clf.secretName)
	}
	if !(clf.name == "instance" && clf.namespace == cloNS) && len(clf.serviceAccountName) > 0 {
		clf.createServiceAccount(oc)
		parameters = append(parameters, "SERVICE_ACCOUNT_NAME="+clf.serviceAccountName)
	}
	if len(optionalParameters) > 0 {
		parameters = append(parameters, optionalParameters...)
	}

	file, processErr := processTemplate(oc, parameters...)
	defer os.Remove(file)
	if processErr != nil {
		e2e.Failf("error processing file: %v", processErr)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", file, "-n", clf.namespace).Execute()
	if err != nil {
		e2e.Failf("error creating clusterlogforwarder: %v", err)
	}
	resource{"clusterlogforwarder", clf.name, clf.namespace}.WaitForResourceToAppear(oc)

	if !(clf.name == "instance" && clf.namespace == cloNS) && clf.waitForPodReady {
		WaitForDaemonsetPodsToBeReady(oc, clf.namespace, clf.name)
	}

	if clf.namespace != cloNS && clf.namespace != "openshift-operators-redhat" && clf.enableMonitoring {
		enableClusterMonitoring(oc, clf.namespace)
	}
}

// createServiceAccount creates the serviceaccount and add the required clusterroles to the serviceaccount
func (clf *clusterlogforwarder) createServiceAccount(oc *exutil.CLI) {
	_, err := oc.AdminKubeClient().CoreV1().ServiceAccounts(clf.namespace).Get(context.Background(), clf.serviceAccountName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		err = createServiceAccount(oc, clf.namespace, clf.serviceAccountName)
		if err != nil {
			e2e.Failf("can't create the serviceaccount: %v", err)
		}
	}
	if clf.collectApplicationLogs {
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-application-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	if clf.collectInfrastructureLogs {
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-infrastructure-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	if clf.collectAuditLogs {
		err = addClusterRoleToServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-audit-logs")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func createServiceAccount(oc *exutil.CLI, namespace, name string) error {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("serviceaccount", name, "-n", namespace).Execute()
	return err
}

func addClusterRoleToServiceAccount(oc *exutil.CLI, namespace, serviceAccountName, clusterRole string) error {
	return oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", clusterRole, fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccountName)).Execute()
}

func removeClusterRoleFromServiceAccount(oc *exutil.CLI, namespace, serviceAccountName, clusterRole string) error {
	return oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", clusterRole, fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccountName)).Execute()
}

// update existing clusterlogforwarder CR
// if template is specified, then run command `oc process -f template -p patches | oc apply -f -`
// if template is not specified, then run command `oc patch clusterlogforwarder/${clf.name} -p patches`
// if use patch, should add `--type=` in the end of patches
func (clf *clusterlogforwarder) update(oc *exutil.CLI, template string, patches ...string) {
	var err error
	if template != "" {
		//parameters := []string{"-f", template, "--ignore-unknown-parameters=true", "-p", "NAME=" + clf.name, "NAMESPACE=" + clf.namespace}
		parameters := []string{"-f", template, "-p", "NAME=" + clf.name, "NAMESPACE=" + clf.namespace}
		if clf.secretName != "" {
			parameters = append(parameters, "SECRET_NAME="+clf.secretName)
		}
		if !(clf.name == "instance" && clf.namespace == cloNS) && len(clf.serviceAccountName) > 0 {
			parameters = append(parameters, "SERVICE_ACCOUNT_NAME="+clf.serviceAccountName)
		}
		if len(patches) > 0 {
			parameters = append(parameters, patches...)
		}
		file, processErr := processTemplate(oc, parameters...)
		defer os.Remove(file)
		if processErr != nil {
			e2e.Failf("error processing file: %v", processErr)
		}
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", clf.namespace).Execute()
	} else {
		parameters := []string{"clf/" + clf.name, "-n", clf.namespace, "-p"}
		parameters = append(parameters, patches...)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(parameters...).Execute()
	}
	if err != nil {
		e2e.Failf("error updating clusterlogforwarder: %v", err)
	}
}

// delete the clusterlogforwarder CR
func (clf *clusterlogforwarder) delete(oc *exutil.CLI) {
	err := resource{"clusterlogforwarder", clf.name, clf.namespace}.clear(oc)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("clusterlogforwarder/%s in project/%s is not deleted", clf.name, clf.namespace))
	if !(clf.name == "instance" && clf.namespace == cloNS) {
		if len(clf.serviceAccountName) > 0 {
			if clf.collectApplicationLogs {
				err = removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-application-logs")
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			if clf.collectInfrastructureLogs {
				err = removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-infrastructure-logs")
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			if clf.collectAuditLogs {
				err = removeClusterRoleFromServiceAccount(oc, clf.namespace, clf.serviceAccountName, "collect-audit-logs")
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			resource{"serviceaccount", clf.serviceAccountName, clf.namespace}.clear(oc)
		}
		if clf.waitForPodReady {
			err = resource{"daemonset", clf.name, clf.namespace}.WaitUntilResourceIsGone(oc)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("daemonset/%s in project/%s is not deleted", clf.name, clf.namespace))
		}
	}
}

type logFileMetricExporter struct {
	name          string
	namespace     string
	template      string
	waitPodsReady bool
}

func (lfme *logFileMetricExporter) create(oc *exutil.CLI, optionalParameters ...string) {
	if lfme.name == "" {
		lfme.name = "instance"
	}
	if lfme.namespace == "" {
		lfme.namespace = loggingNS
	}
	if lfme.template == "" {
		lfme.template = exutil.FixturePath("testdata", "logging", "logfilemetricexporter", "lfme.yaml")
	}

	parameters := []string{"-f", lfme.template, "-p", "NAME=" + lfme.name, "NAMESPACE=" + lfme.namespace}
	if len(optionalParameters) > 0 {
		parameters = append(parameters, optionalParameters...)
	}

	file, processErr := processTemplate(oc, parameters...)
	defer os.Remove(file)
	if processErr != nil {
		e2e.Failf("error processing file: %v", processErr)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", lfme.namespace).Execute()
	if err != nil {
		e2e.Failf("error creating logfilemetricexporter: %v", err)
	}
	resource{"logfilemetricexporter", lfme.name, lfme.namespace}.WaitForResourceToAppear(oc)
	if lfme.waitPodsReady {
		WaitForDaemonsetPodsToBeReady(oc, lfme.namespace, "logfilesmetricexporter")
	}
}

func (lfme *logFileMetricExporter) delete(oc *exutil.CLI) {
	err := resource{"logfilemetricexporter", lfme.name, lfme.namespace}.clear(oc)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("logfilemetricexporter/%s in project/%s is not deleted", lfme.name, lfme.namespace))
	err = resource{"daemonset", "logfilesmetricexporter", lfme.namespace}.WaitUntilResourceIsGone(oc)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ds/logfilesmetricexporter in project/%s is not deleted", lfme.namespace))
}

func deleteNamespace(oc *exutil.CLI, ns string) {
	err := oc.AdminKubeClient().CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		_, err = oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Namespace %s is not deleted in 3 minutes", ns))
}

// WaitForIMCronJobToAppear checks if the cronjob exists or not
func WaitForIMCronJobToAppear(oc *exutil.CLI, ns string, name string) {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		_, err = oc.AdminKubeClient().BatchV1().CronJobs(ns).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for availability of cronjob\n")
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cronjob %s is not available", name))
}

func waitForIMJobsToComplete(oc *exutil.CLI, ns string, timeout time.Duration) {
	// wait for jobs to appear
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true, func(context.Context) (done bool, err error) {
		jobList, err := oc.AdminKubeClient().BatchV1().Jobs(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "component=indexManagement"})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for availability of jobs\n")
				return false, nil
			}
			return false, err
		}
		if len(jobList.Items) > 0 {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("jobs with label %s are not exist", "component=indexManagement"))
	// wait for jobs to complete
	jobList, err := oc.AdminKubeClient().BatchV1().Jobs(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "component=indexManagement"})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, job := range jobList.Items {
		err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
			job, err := oc.AdminKubeClient().BatchV1().Jobs(ns).Get(context.Background(), job.Name, metav1.GetOptions{})
			//succeeded, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "job", job.Name, "-o=jsonpath={.status.succeeded}").Output()
			if err != nil {
				return false, err
			}
			if job.Status.Succeeded == 1 {
				e2e.Logf("job %s completed successfully", job.Name)
				return true, nil
			}
			e2e.Logf("job %s is not completed yet", job.Name)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("job %s is not completed yet", job.Name))
	}
}

func getStorageClassName(oc *exutil.CLI) (string, error) {
	scs, err := oc.AdminKubeClient().StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	if len(scs.Items) == 0 {
		return "", fmt.Errorf("there is no storageclass in the cluster")
	}
	for _, sc := range scs.Items {
		if sc.ObjectMeta.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return sc.Name, nil
		}
	}
	return scs.Items[0].Name, nil
}

// Assert the status of a resource
func assertResourceStatus(oc *exutil.CLI, kind, name, namespace, jsonpath, exptdStatus string) {
	parameters := []string{kind, name, "-o", "jsonpath=" + jsonpath}
	if namespace != "" {
		parameters = append(parameters, "-n", namespace)
	}
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(parameters...).Output()
		if err != nil {
			return false, err
		}
		if strings.Compare(status, exptdStatus) != 0 {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s/%s value for %s is not %s", kind, name, jsonpath, exptdStatus))
}

func getRouteAddress(oc *exutil.CLI, ns, routeName string) string {
	route, err := oc.AdminRouteClient().RouteV1().Routes(ns).Get(context.Background(), routeName, metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	return route.Spec.Host
}

func getSAToken(oc *exutil.CLI, name, ns string) string {
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", name, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return token
}

// enableClusterMonitoring add label `openshift.io/cluster-monitoring: "true"` to the project, and create role/prometheus-k8s rolebinding/prometheus-k8s in the namespace
func enableClusterMonitoring(oc *exutil.CLI, namespace string) {
	err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", namespace, "openshift.io/cluster-monitoring=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	file := exutil.FixturePath("testdata", "logging", "prometheus-k8s-rbac.yaml")
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-n", namespace, "-f", file).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// queryPrometheus returns the promtheus metrics which match the query string
// token: the user token used to run the http request, if it's not specified, it will use the token of sa/prometheus-k8s in openshift-monitoring project
// path: the api path, for example: /api/v1/query?
// query: the metric/alert you want to search, e.g.: es_index_namespaces_total
// action: it can be "GET", "get", "Get", "POST", "post", "Post"
func queryPrometheus(oc *exutil.CLI, token string, path string, query string, action string) (*prometheusQueryResult, error) {
	var bearerToken string
	var err error
	if token == "" {
		bearerToken = getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
	} else {
		bearerToken = token
	}
	address := "https://" + getRouteAddress(oc, "openshift-monitoring", "prometheus-k8s")

	h := make(http.Header)
	h.Add("Content-Type", "application/json")
	h.Add("Authorization", "Bearer "+bearerToken)

	params := url.Values{}
	if len(query) > 0 {
		params.Add("query", query)
	}

	var p prometheusQueryResult
	resp, err := doHTTPRequest(h, address, path, params.Encode(), action, false, 5, nil, 200)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(resp, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func getMetric(oc *exutil.CLI, token, query string) ([]metric, error) {
	res, err := queryPrometheus(oc, token, "/api/v1/query", query, "GET")
	if err != nil {
		return []metric{}, err
	}
	return res.Data.Result, nil
}

func checkMetric(oc *exutil.CLI, token, query string, timeInMinutes int) {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, time.Duration(timeInMinutes)*time.Minute, true, func(context.Context) (done bool, err error) {
		metrics, err := getMetric(oc, token, query)
		if err != nil {
			return false, err
		}
		if len(metrics) == 0 {
			e2e.Logf("no metrics found by query: %s, try next time", query)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can't find metrics by %s in %d minutes", query, timeInMinutes))
}

func getAlert(oc *exutil.CLI, token, alertSelector string) ([]alert, error) {
	var al []alert
	alerts, err := queryPrometheus(oc, token, "/api/v1/alerts", "", "GET")
	if err != nil {
		return al, err
	}
	for i := 0; i < len(alerts.Data.Alerts); i++ {
		if alerts.Data.Alerts[i].Labels.AlertName == alertSelector {
			al = append(al, alerts.Data.Alerts[i])
		}
	}
	return al, nil
}

func checkAlert(oc *exutil.CLI, token, alertName, status string, timeInMinutes int) {
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, time.Duration(timeInMinutes)*time.Minute, true, func(context.Context) (done bool, err error) {
		alerts, err := getAlert(oc, token, alertName)
		if err != nil {
			return false, err
		}
		for _, alert := range alerts {
			s, _ := regexp.Compile(status)
			if s.MatchString(alert.State) {
				return true, nil
			}
		}
		e2e.Logf("Waiting for alert %s to be in state %s...", alertName, status)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s alert is not %s in %d minutes", alertName, status, timeInMinutes))
}

// WaitUntilPodsAreGone waits for pods selected with labelselector to be removed
func WaitUntilPodsAreGone(oc *exutil.CLI, namespace string, labelSelector string) {
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector="+labelSelector, "-n", namespace).Output()
		if err != nil {
			return false, err
		}
		errstring := fmt.Sprintf("%v", output)
		if strings.Contains(errstring, "No resources found") {
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Error waiting for pods to be removed using label selector %s", labelSelector))
}

// Check logs from resource
func (r resource) checkLogsFromRs(oc *exutil.CLI, expected string, containerName string) {
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(r.kind+`/`+r.name, "-n", r.namespace, "-c", containerName).Output()
		if err != nil {
			e2e.Logf("Can't get logs from resource, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.Match(expected, []byte(output)); !matched {
			e2e.Logf("Can't find the expected string\n")
			return false, nil
		}
		e2e.Logf("Check the logs succeed!!\n")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%s is not expected for %s", expected, r.name))
}

func getCurrentCSVFromPackage(oc *exutil.CLI, source, channel, packagemanifest string) string {
	var currentCSV string
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", "openshift-marketplace", "-l", "catalog="+source, "-ojsonpath={.items}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	packMS := []PackageManifest{}
	json.Unmarshal([]byte(output), &packMS)
	for _, pm := range packMS {
		if pm.Name == packagemanifest {
			for _, channels := range pm.Status.Channels {
				if channels.Name == channel {
					currentCSV = channels.CurrentCSV
					break
				}
			}
		}
	}
	return currentCSV
}

func chkMustGather(oc *exutil.CLI, ns string, clin string) {
	cloImg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "deployment.apps/cluster-logging-operator", "-o", "jsonpath={.spec.template.spec.containers[?(@.name == \"cluster-logging-operator\")].image}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The cloImg is: " + cloImg)

	cloPodList, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "name=cluster-logging-operator"})
	o.Expect(err).NotTo(o.HaveOccurred())
	cloImgID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pods", cloPodList.Items[0].Name, "-o", "jsonpath={.status.containerStatuses[0].imageID}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The cloImgID is: " + cloImgID)

	mgDest := "must-gather-" + getRandomString()
	baseDir := exutil.FixturePath("testdata", "logging")
	TestDataPath := filepath.Join(baseDir, mgDest)
	defer exec.Command("rm", "-r", TestDataPath).Output()
	err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", ns, "must-gather", "--image="+cloImg, "--dest-dir="+TestDataPath).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	replacer := strings.NewReplacer(".", "-", "/", "-", ":", "-", "@", "-")
	cloImgDir := replacer.Replace(cloImgID)
	var checkPath []string
	if clin == "collector" {
		checkPath = []string{
			"timestamp",
			"event-filter.html",
			cloImgDir + "/timestamp",
			cloImgDir + "/gather-debug.log",
			cloImgDir + "/event-filter.html",
			cloImgDir + "/cluster-scoped-resources",
			cloImgDir + "/namespaces",
			cloImgDir + "/cluster-logging/clo",
			cloImgDir + "/cluster-logging/clo/openshift-logging/daemonsets.txt",
			cloImgDir + "/cluster-logging/collectors",
			cloImgDir + "/cluster-logging/install",
			cloImgDir + "/cluster-logging/install/install_plan-clo",
			cloImgDir + "/cluster-logging/install/install_plan-eo",
			cloImgDir + "/cluster-logging/install/subscription-clo",
			cloImgDir + "/cluster-logging/install/subscription-eo",
			cloImgDir + "/namespaces/openshift-logging/core/configmaps/collector-config.yaml",
		}
	} else {
		checkPath = []string{
			"timestamp",
			"event-filter.html",
			cloImgDir + "/timestamp",
			cloImgDir + "/gather-debug.log",
			cloImgDir + "/event-filter.html",
			cloImgDir + "/cluster-scoped-resources",
			cloImgDir + "/namespaces",
			cloImgDir + "/cluster-logging/clo",
			cloImgDir + "/namespaces/openshift-logging/core/configmaps/collector-config.yaml",
			cloImgDir + "/cluster-logging/clo/openshift-logging/deployments.txt",
			cloImgDir + "/cluster-logging/clo/openshift-logging/daemonsets.txt",
			cloImgDir + "/cluster-logging/clo/openshift-logging/elasticsearch.crt",
			cloImgDir + "/cluster-logging/clo/openshift-logging/elasticsearch.key",
			cloImgDir + "/cluster-logging/clo/openshift-logging/logging-es.crt",
			cloImgDir + "/cluster-logging/clo/openshift-logging/logging-es.key",
			cloImgDir + "/cluster-logging/eo",
			cloImgDir + "/cluster-logging/eo/eo-deployment.describe",
			cloImgDir + "/cluster-logging/es",
			cloImgDir + "/cluster-logging/es/cluster-elasticsearch",
			cloImgDir + "/cluster-logging/es/elasticsearch_cr.yaml",
			cloImgDir + "/cluster-logging/collectors",
			cloImgDir + "/cluster-logging/install",
			cloImgDir + "/cluster-logging/install/install_plan-clo",
			cloImgDir + "/cluster-logging/install/install_plan-eo",
			cloImgDir + "/cluster-logging/install/subscription-clo",
			cloImgDir + "/cluster-logging/install/subscription-eo",
		}
	}

	for _, v := range checkPath {
		pathStat, err := os.Stat(filepath.Join(TestDataPath, v))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pathStat.Size() > 0).To(o.BeTrue(), "The path %s is empty", v)
	}
}

func checkNetworkType(oc *exutil.CLI) string {
	output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.type}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.ToLower(output)
}

func getAppDomain(oc *exutil.CLI) (string, error) {
	subDomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingresses.config/cluster", "-ojsonpath={.spec.domain}").Output()
	if err != nil {
		return "", err
	}
	return subDomain, nil
}

type certsConf struct {
	serverName string
	namespace  string
	passPhrase string //client private key passphrase
}

func (certs certsConf) generateCerts(oc *exutil.CLI, keysPath string) {
	generateCertsSH := exutil.FixturePath("testdata", "logging", "external-log-stores", "cert_generation.sh")
	domain, err := getAppDomain(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd := []string{generateCertsSH, keysPath, certs.namespace, certs.serverName, domain}
	if certs.passPhrase != "" {
		cmd = append(cmd, certs.passPhrase)
	}
	err = exec.Command("sh", cmd...).Run()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// expect: true means we want the resource contain/compare with the expectedContent, false means the resource is expected not to compare with/contain the expectedContent;
// compare: true means compare the expectedContent with the resource content, false means check if the resource contains the expectedContent;
// args are the arguments used to execute command `oc.AsAdmin.WithoutNamespace().Run("get").Args(args...).Output()`;
func checkResource(oc *exutil.CLI, expect bool, compare bool, expectedContent string, args []string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
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

type rsyslog struct {
	serverName          string //the name of the rsyslog server, it's also used to name the svc/cm/sa/secret
	namespace           string //the namespace where the rsyslog server deployed in
	tls                 bool
	secretName          string //the name of the secret for the collector to use
	loggingNS           string //the namespace where the collector pods deployed in
	clientKeyPassphrase string //client private key passphrase
}

func (r rsyslog) createPipelineSecret(oc *exutil.CLI, keysPath string) {
	secret := resource{"secret", r.secretName, r.loggingNS}
	cmd := []string{"secret", "generic", secret.name, "-n", secret.namespace, "--from-file=ca-bundle.crt=" + keysPath + "/ca.crt"}
	if r.clientKeyPassphrase != "" {
		cmd = append(cmd, "--from-file=tls.key="+keysPath+"/client.key", "--from-file=tls.crt="+keysPath+"/client.crt", "--from-literal=passphrase="+r.clientKeyPassphrase)
	}

	err := oc.AsAdmin().WithoutNamespace().Run("create").Args(cmd...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	resource{"secret", r.secretName, r.loggingNS}.WaitForResourceToAppear(oc)
}

func (r rsyslog) deploy(oc *exutil.CLI) {
	// create SA
	sa := resource{"serviceaccount", r.serverName, r.namespace}
	err := oc.WithoutNamespace().Run("create").Args("serviceaccount", sa.name, "-n", sa.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	sa.WaitForResourceToAppear(oc)
	err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "privileged", fmt.Sprintf("system:serviceaccount:%s:%s", r.namespace, r.serverName), "-n", r.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	filePath := []string{"testdata", "logging", "external-log-stores", "rsyslog"}
	// create secrets if needed
	if r.tls {
		o.Expect(r.secretName).NotTo(o.BeEmpty())
		// create a temporary directory
		baseDir := exutil.FixturePath("testdata", "logging")
		keysPath := filepath.Join(baseDir, "temp"+getRandomString())
		defer exec.Command("rm", "-r", keysPath).Output()
		err = os.MkdirAll(keysPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		cert := certsConf{r.serverName, r.namespace, r.clientKeyPassphrase}
		cert.generateCerts(oc, keysPath)
		// create pipelinesecret
		r.createPipelineSecret(oc, keysPath)
		// create secret for rsyslog server
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", r.serverName, "-n", r.namespace, "--from-file=server.key="+keysPath+"/server.key", "--from-file=server.crt="+keysPath+"/server.crt", "--from-file=ca_bundle.crt="+keysPath+"/ca.crt").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		filePath = append(filePath, "secure")
	} else {
		filePath = append(filePath, "insecure")
	}

	// create configmap/deployment/svc
	cm := resource{"configmap", r.serverName, r.namespace}
	cmFilePath := append(filePath, "configmap.yaml")
	cmFile := exutil.FixturePath(cmFilePath...)
	err = cm.applyFromTemplate(oc, "-f", cmFile, "-n", r.namespace, "-p", "NAMESPACE="+r.namespace, "-p", "NAME="+r.serverName)
	o.Expect(err).NotTo(o.HaveOccurred())

	deploy := resource{"deployment", r.serverName, r.namespace}
	deployFilePath := append(filePath, "deployment.yaml")
	deployFile := exutil.FixturePath(deployFilePath...)
	err = deploy.applyFromTemplate(oc, "-f", deployFile, "-n", r.namespace, "-p", "NAMESPACE="+r.namespace, "-p", "NAME="+r.serverName)
	o.Expect(err).NotTo(o.HaveOccurred())
	WaitForDeploymentPodsToBeReady(oc, r.namespace, r.serverName)

	svc := resource{"svc", r.serverName, r.namespace}
	svcFilePath := append(filePath, "svc.yaml")
	svcFile := exutil.FixturePath(svcFilePath...)
	err = svc.applyFromTemplate(oc, "-f", svcFile, "-n", r.namespace, "-p", "NAMESPACE="+r.namespace, "-p", "NAME="+r.serverName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (r rsyslog) remove(oc *exutil.CLI) {
	resource{"serviceaccount", r.serverName, r.namespace}.clear(oc)
	if r.tls {
		resource{"secret", r.serverName, r.namespace}.clear(oc)
		resource{"secret", r.secretName, r.loggingNS}.clear(oc)
	}
	resource{"configmap", r.serverName, r.namespace}.clear(oc)
	resource{"deployment", r.serverName, r.namespace}.clear(oc)
	resource{"svc", r.serverName, r.namespace}.clear(oc)
}

func (r rsyslog) getPodName(oc *exutil.CLI) string {
	pods, err := oc.AdminKubeClient().CoreV1().Pods(r.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=" + r.serverName})
	o.Expect(err).NotTo(o.HaveOccurred())
	var names []string
	for i := 0; i < len(pods.Items); i++ {
		names = append(names, pods.Items[i].Name)
	}
	return names[0]
}

func (r rsyslog) checkData(oc *exutil.CLI, expect bool, filename string) {
	cmd := "ls -l /var/log/clf/" + filename
	if expect {
		err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
			stdout, err := e2eoutput.RunHostCmdWithRetries(r.namespace, r.getPodName(oc), cmd, 3*time.Second, 15*time.Second)
			if err != nil {
				if strings.Contains(err.Error(), "No such file or directory") {
					return false, nil
				}
				return false, err
			}
			return strings.Contains(stdout, filename), nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The %s doesn't exist", filename))
	} else {
		err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
			stdout, err := e2eoutput.RunHostCmdWithRetries(r.namespace, r.getPodName(oc), cmd, 3*time.Second, 15*time.Second)
			if err != nil {
				if strings.Contains(err.Error(), "No such file or directory") {
					return true, nil
				}
				return false, err
			}
			return strings.Contains(stdout, "No such file or directory"), nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The %s exists", filename))
	}

}

type fluentdServer struct {
	serverName                 string //the name of the fluentd server, it's also used to name the svc/cm/sa/secret
	namespace                  string //the namespace where the fluentd server deployed in
	serverAuth                 bool
	clientAuth                 bool   // only can be set when serverAuth is true
	clientPrivateKeyPassphrase string //only can be set when clientAuth is true
	sharedKey                  string //if it's not empty, means the shared_key is set, only works when serverAuth is true
	secretName                 string //the name of the secret for the collector to use
	loggingNS                  string //the namespace where the collector pods deployed in
	inPluginType               string //forward or http
}

func (f fluentdServer) createPipelineSecret(oc *exutil.CLI, keysPath string) {
	secret := resource{"secret", f.secretName, f.loggingNS}
	cmd := []string{"secret", "generic", secret.name, "-n", secret.namespace, "--from-file=ca-bundle.crt=" + keysPath + "/ca.crt"}
	if f.clientAuth {
		cmd = append(cmd, "--from-file=tls.key="+keysPath+"/client.key", "--from-file=tls.crt="+keysPath+"/client.crt")
	}
	if f.clientPrivateKeyPassphrase != "" {
		cmd = append(cmd, "--from-literal=passphrase="+f.clientPrivateKeyPassphrase)
	}
	if f.sharedKey != "" {
		cmd = append(cmd, "--from-literal=shared_key="+f.sharedKey)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args(cmd...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	secret.WaitForResourceToAppear(oc)
}

func (f fluentdServer) deploy(oc *exutil.CLI) {
	// create SA
	sa := resource{"serviceaccount", f.serverName, f.namespace}
	err := oc.WithoutNamespace().Run("create").Args("serviceaccount", sa.name, "-n", sa.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	sa.WaitForResourceToAppear(oc)
	//err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "privileged", fmt.Sprintf("system:serviceaccount:%s:%s", f.namespace, f.serverName), "-n", f.namespace).Execute()
	//o.Expect(err).NotTo(o.HaveOccurred())
	filePath := []string{"testdata", "logging", "external-log-stores", "fluentd"}

	// create secrets if needed
	if f.serverAuth {
		o.Expect(f.secretName).NotTo(o.BeEmpty())
		filePath = append(filePath, "secure")
		// create a temporary directory
		baseDir := exutil.FixturePath("testdata", "logging")
		keysPath := filepath.Join(baseDir, "temp"+getRandomString())
		defer exec.Command("rm", "-r", keysPath).Output()
		err = os.MkdirAll(keysPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		//generate certs
		cert := certsConf{f.serverName, f.namespace, f.clientPrivateKeyPassphrase}
		cert.generateCerts(oc, keysPath)
		//create pipelinesecret
		f.createPipelineSecret(oc, keysPath)
		//create secret for fluentd server
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", f.serverName, "-n", f.namespace, "--from-file=ca-bundle.crt="+keysPath+"/ca.crt", "--from-file=tls.key="+keysPath+"/server.key", "--from-file=tls.crt="+keysPath+"/server.crt", "--from-file=ca.key="+keysPath+"/ca.key").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

	} else {
		filePath = append(filePath, "insecure")
	}

	// create configmap/deployment/svc
	cm := resource{"configmap", f.serverName, f.namespace}
	//when prefix is http-, the fluentdserver using http inplugin.
	cmFilePrefix := ""
	if f.inPluginType == "http" {
		cmFilePrefix = "http-"
	}

	var cmFileName string
	if !f.serverAuth {
		cmFileName = cmFilePrefix + "configmap.yaml"
	} else {
		if f.clientAuth {
			if f.sharedKey != "" {
				cmFileName = "cm-mtls-share.yaml"
			} else {
				cmFileName = cmFilePrefix + "cm-mtls.yaml"
			}
		} else {
			if f.sharedKey != "" {
				cmFileName = "cm-serverauth-share.yaml"
			} else {
				cmFileName = cmFilePrefix + "cm-serverauth.yaml"
			}
		}
	}
	cmFilePath := append(filePath, cmFileName)
	cmFile := exutil.FixturePath(cmFilePath...)
	cCmCmd := []string{"-f", cmFile, "-n", f.namespace, "-p", "NAMESPACE=" + f.namespace, "-p", "NAME=" + f.serverName}
	if f.sharedKey != "" {
		cCmCmd = append(cCmCmd, "-p", "SHARED_KEY="+f.sharedKey)
	}
	err = cm.applyFromTemplate(oc, cCmCmd...)
	o.Expect(err).NotTo(o.HaveOccurred())

	deploy := resource{"deployment", f.serverName, f.namespace}
	deployFilePath := append(filePath, "deployment.yaml")
	deployFile := exutil.FixturePath(deployFilePath...)
	err = deploy.applyFromTemplate(oc, "-f", deployFile, "-n", f.namespace, "-p", "NAMESPACE="+f.namespace, "-p", "NAME="+f.serverName)
	o.Expect(err).NotTo(o.HaveOccurred())
	WaitForDeploymentPodsToBeReady(oc, f.namespace, f.serverName)

	err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("-n", f.namespace, "deployment", f.serverName, "--name="+f.serverName).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (f fluentdServer) remove(oc *exutil.CLI) {
	resource{"serviceaccount", f.serverName, f.namespace}.clear(oc)
	if f.serverAuth {
		resource{"secret", f.serverName, f.namespace}.clear(oc)
		resource{"secret", f.secretName, f.loggingNS}.clear(oc)
	}
	resource{"configmap", f.serverName, f.namespace}.clear(oc)
	resource{"deployment", f.serverName, f.namespace}.clear(oc)
	resource{"svc", f.serverName, f.namespace}.clear(oc)
}

func (f fluentdServer) getPodName(oc *exutil.CLI) string {
	pods, err := oc.AdminKubeClient().CoreV1().Pods(f.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=" + f.serverName})
	o.Expect(err).NotTo(o.HaveOccurred())
	var names []string
	for i := 0; i < len(pods.Items); i++ {
		names = append(names, pods.Items[i].Name)
	}
	return names[0]
}

// check the data in fluentd server
// filename is the name of a file you want to check
// expect true means you expect the file to exist, false means the file is not expected to exist
func (f fluentdServer) checkData(oc *exutil.CLI, expect bool, filename string) {
	cmd := "ls -l /fluentd/log/" + filename
	if expect {
		err := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
			stdout, err := e2eoutput.RunHostCmdWithRetries(f.namespace, f.getPodName(oc), cmd, 3*time.Second, 15*time.Second)
			if err != nil {
				if strings.Contains(err.Error(), "No such file or directory") {
					return false, nil
				}
				return false, err
			}
			return strings.Contains(stdout, filename), nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The %s doesn't exist", filename))
	} else {
		err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
			stdout, err := e2eoutput.RunHostCmdWithRetries(f.namespace, f.getPodName(oc), cmd, 3*time.Second, 15*time.Second)
			if err != nil {
				if strings.Contains(err.Error(), "No such file or directory") {
					return true, nil
				}
				return false, err
			}
			return strings.Contains(stdout, "No such file or directory"), nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The %s exists", filename))
	}

}

type logstash struct {
	name      string
	namespace string
}

func (l logstash) deploy(oc *exutil.CLI) {
	cmFile := exutil.FixturePath("testdata", "logging", "external-log-stores", "logstash", "configmap.yaml")
	deployFile := exutil.FixturePath("testdata", "logging", "external-log-stores", "logstash", "deployment.yaml")

	deploy := resource{"deployment", l.name, l.namespace}
	configmap := resource{"configmap", l.name, l.namespace}
	svc := resource{"svc", l.name, l.namespace}

	err := configmap.applyFromTemplate(oc, "-f", cmFile, "-n", l.namespace, "-p", "NAMESPACE="+l.namespace, "-p", "NAME="+l.name)
	if err != nil {
		e2e.Failf("can't create configmap %s in %s project: %v", l.name, l.namespace, err)
	}

	err = deploy.applyFromTemplate(oc, "-f", deployFile, "-n", l.namespace, "-p", "NAMESPACE="+l.namespace, "-p", "NAME="+l.name)
	if err != nil {
		e2e.Failf("can't create deployment %s in %s project: %v", l.name, l.namespace, err)
	}
	svc.WaitForResourceToAppear(oc)
	WaitForDeploymentPodsToBeReady(oc, l.namespace, l.name)
}

func (l logstash) remove(oc *exutil.CLI) {
	for _, k := range []string{"deployment", "configmap", "svc"} {
		resource{k, l.name, l.namespace}.clear(oc)
	}
}

func (l logstash) checkData(oc *exutil.CLI, expect bool, filename string) {
	pods, err := oc.AdminKubeClient().CoreV1().Pods(l.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "component=" + l.name})
	if err != nil {
		e2e.Failf("can't get pod with label component=%s in %s project: %v", l.name, l.namespace, err)
	}

	cmd := "ls -l /usr/share/logstash/data/" + filename
	err = wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 60*time.Second, true, func(context.Context) (done bool, err error) {
		stdout, err := e2eoutput.RunHostCmdWithRetries(l.namespace, pods.Items[0].Name, cmd, 3*time.Second, 15*time.Second)
		if err != nil {
			return false, err
		}
		if (strings.Contains(stdout, filename) && expect) || (!strings.Contains(stdout, filename) && !expect) {
			return true, nil
		}
		return false, nil
	})
	if expect {
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The %s doesn't exist", filename))
	} else {
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The %s exists", filename))
	}
}

// return the infrastructureName. For example:  anli922-jglp4
func getInfrastructureName(oc *exutil.CLI) string {
	infrastructureName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.infrastructureName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return infrastructureName
}

// return the nodeNames
func getNodeNames(oc *exutil.CLI, nodeLabel string) []string {
	var nodeNames []string
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", nodeLabel, "-o=jsonpath={.items[*].metadata.name}").Output()
	if err == nil {
		nodeNames = strings.Split(output, " ")
	} else {
		e2e.Logf("Warning: failed to get nodes names ")
	}
	return nodeNames
}

func getDataFromKafkaConsumerPod(oc *exutil.CLI, kafkaNS, consumerPod string) ([]LogEntity, error) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", kafkaNS, consumerPod, "--since=30s", "--tail=30").Output()
	if err != nil {
		return nil, fmt.Errorf("get error when checking data in kafka consumer: %v", err)
	}
	var logs []LogEntity
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		var log LogEntity
		err = json.Unmarshal([]byte(line), &log)
		if err != nil {
			return nil, nil
		}
		logs = append(logs, log)
	}
	return logs, nil
}

func getDataFromKafkaByLogType(oc *exutil.CLI, kafkaNS, consumerPod, logType string) ([]LogEntity, error) {
	data, err := getDataFromKafkaConsumerPod(oc, kafkaNS, consumerPod)
	if err != nil {
		return nil, err
	}
	var logs []LogEntity
	for _, log := range data {
		if log.LogType == logType {
			logs = append(logs, log)
		}
	}
	return logs, nil
}

func getDataFromKafkaByNamespace(oc *exutil.CLI, kafkaNS, consumerPod, namespace string) ([]LogEntity, error) {
	data, err := getDataFromKafkaConsumerPod(oc, kafkaNS, consumerPod)
	if err != nil {
		return nil, err
	}
	var logs []LogEntity
	for _, log := range data {
		if log.Kubernetes.NamespaceName == namespace {
			logs = append(logs, log)
		}
	}
	return logs, nil
}

type kafka struct {
	namespace      string
	kafkasvcName   string
	zoosvcName     string
	authtype       string //Name the kafka folders under testdata same as the authtype (options: plaintext-ssl, sasl-ssl, sasl-plaintext)
	pipelineSecret string //the name of the secret for collectors to use
	collectorType  string //must be specified when auth type is sasl-ssl/sasl-plaintext
	loggingNS      string //the namespace where the collector pods are deployed in
}

func (k kafka) deployZookeeper(oc *exutil.CLI) {
	zookeeperFilePath := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka", "zookeeper")
	//create zookeeper configmap/svc/StatefulSet
	configTemplate := filepath.Join(zookeeperFilePath, "configmap.yaml")
	if k.authtype == "plaintext-ssl" {
		configTemplate = filepath.Join(zookeeperFilePath, "configmap-ssl.yaml")
	}
	err := resource{"configmap", k.zoosvcName, k.namespace}.applyFromTemplate(oc, "-n", k.namespace, "-f", configTemplate, "-p", "NAME="+k.zoosvcName, "NAMESPACE="+k.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())

	zoosvcFile := filepath.Join(zookeeperFilePath, "zookeeper-svc.yaml")
	zoosvc := resource{"Service", k.zoosvcName, k.namespace}
	err = zoosvc.applyFromTemplate(oc, "-n", k.namespace, "-f", zoosvcFile, "-p", "NAME="+k.zoosvcName, "-p", "NAMESPACE="+k.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())

	zoosfsFile := filepath.Join(zookeeperFilePath, "zookeeper-statefulset.yaml")
	zoosfs := resource{"StatefulSet", k.zoosvcName, k.namespace}
	err = zoosfs.applyFromTemplate(oc, "-n", k.namespace, "-f", zoosfsFile, "-p", "NAME="+k.zoosvcName, "-p", "NAMESPACE="+k.namespace, "-p", "SERVICENAME="+zoosvc.name, "-p", "CM_NAME="+k.zoosvcName)
	o.Expect(err).NotTo(o.HaveOccurred())
	waitForPodReadyWithLabel(oc, k.namespace, "app="+k.zoosvcName)
}

func (k kafka) deployKafka(oc *exutil.CLI) {
	kafkaFilePath := exutil.FixturePath("testdata", "logging", "external-log-stores", "kafka")
	kafkaConfigmapTemplate := filepath.Join(kafkaFilePath, k.authtype, "kafka-configmap.yaml")
	consumerConfigmapTemplate := filepath.Join(kafkaFilePath, k.authtype, "consumer-configmap.yaml")

	var keysPath string
	if k.authtype == "sasl-ssl" || k.authtype == "plaintext-ssl" {
		baseDir := exutil.FixturePath("testdata", "logging")
		keysPath = filepath.Join(baseDir, "temp"+getRandomString())
		defer exec.Command("rm", "-r", keysPath).Output()
		err := os.MkdirAll(keysPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		generateCertsSH := filepath.Join(kafkaFilePath, "cert_generation.sh")
		stdout, err := exec.Command("sh", generateCertsSH, keysPath, k.namespace).Output()
		if err != nil {
			e2e.Logf("error generating certs: %s", string(stdout))
			e2e.Failf("error generating certs: %v", err)
		}
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "kafka-cluster-cert", "-n", k.namespace, "--from-file=ca_bundle.jks="+keysPath+"/ca/ca_bundle.jks", "--from-file=cluster.jks="+keysPath+"/cluster/cluster.jks").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	pipelineSecret := resource{"secret", k.pipelineSecret, k.loggingNS}
	kafkaClientCert := resource{"secret", "kafka-client-cert", k.namespace}
	//create kafka secrets and confimap
	cmdPipeline := []string{"secret", "generic", pipelineSecret.name, "-n", pipelineSecret.namespace}
	cmdClient := []string{"secret", "generic", kafkaClientCert.name, "-n", kafkaClientCert.namespace}
	switch k.authtype {
	case "sasl-plaintext":
		{
			cmdClient = append(cmdClient, "--from-literal=username=admin", "--from-literal=password=admin-secret")
			cmdPipeline = append(cmdPipeline, "--from-literal=username=admin", "--from-literal=password=admin-secret")
			if k.collectorType == "vector" {
				cmdPipeline = append(cmdPipeline, "--from-literal=sasl.enable=True", "--from-literal=sasl.mechanisms=PLAIN")
			}
		}
	case "sasl-ssl":
		{
			cmdClient = append(cmdClient, "--from-file=ca-bundle.jks="+keysPath+"/ca/ca_bundle.jks", "--from-file=ca-bundle.crt="+keysPath+"/ca/ca_bundle.crt", "--from-file=tls.crt="+keysPath+"/client/client.crt", "--from-file=tls.key="+keysPath+"/client/client.key", "--from-literal=username=admin", "--from-literal=password=admin-secret")
			cmdPipeline = append(cmdPipeline, "--from-file=ca-bundle.crt="+keysPath+"/ca/ca_bundle.crt", "--from-literal=username=admin", "--from-literal=password=admin-secret")
			switch k.collectorType {
			case "fluentd":
				{
					cmdPipeline = append(cmdPipeline, "--from-literal=sasl_over_ssl=true")
				}
			case "vector":
				{
					cmdPipeline = append(cmdPipeline, "--from-literal=sasl.enable=True", "--from-literal=sasl.mechanisms=PLAIN", "--from-file=tls.crt="+keysPath+"/client/client.crt", "--from-file=tls.key="+keysPath+"/client/client.key")
				}
			}
		}
	case "plaintext-ssl":
		{
			cmdClient = append(cmdClient, "--from-file=ca-bundle.jks="+keysPath+"/ca/ca_bundle.jks", "--from-file=ca-bundle.crt="+keysPath+"/ca/ca_bundle.crt", "--from-file=tls.crt="+keysPath+"/client/client.crt", "--from-file=tls.key="+keysPath+"/client/client.key")
			cmdPipeline = append(cmdPipeline, "--from-file=ca-bundle.crt="+keysPath+"/ca/ca_bundle.crt", "--from-file=tls.crt="+keysPath+"/client/client.crt", "--from-file=tls.key="+keysPath+"/client/client.key")
		}
	}
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args(cmdClient...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	kafkaClientCert.WaitForResourceToAppear(oc)
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args(cmdPipeline...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	pipelineSecret.WaitForResourceToAppear(oc)

	consumerConfigmap := resource{"configmap", "kafka-client", k.namespace}
	err = consumerConfigmap.applyFromTemplate(oc, "-n", k.namespace, "-f", consumerConfigmapTemplate, "-p", "NAME="+consumerConfigmap.name, "NAMESPACE="+consumerConfigmap.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())

	kafkaConfigmap := resource{"configmap", k.kafkasvcName, k.namespace}
	err = kafkaConfigmap.applyFromTemplate(oc, "-n", k.namespace, "-f", kafkaConfigmapTemplate, "-p", "NAME="+kafkaConfigmap.name, "NAMESPACE="+kafkaConfigmap.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())

	//create ClusterRole and ClusterRoleBinding
	rbacFile := filepath.Join(kafkaFilePath, "kafka-rbac.yaml")
	output, err := oc.AsAdmin().WithoutNamespace().Run("process").Args("-n", k.namespace, "-f", rbacFile, "-p", "NAMESPACE="+k.namespace).OutputToFile(getRandomString() + ".json")
	o.Expect(err).NotTo(o.HaveOccurred())
	oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", output, "-n", k.namespace).Execute()

	//create kafka svc
	svcFile := filepath.Join(kafkaFilePath, "kafka-svc.yaml")
	svc := resource{"Service", k.kafkasvcName, k.namespace}
	err = svc.applyFromTemplate(oc, "-f", svcFile, "-n", svc.namespace, "-p", "NAME="+svc.name, "NAMESPACE="+svc.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())

	//create kafka StatefulSet
	sfsFile := filepath.Join(kafkaFilePath, k.authtype, "kafka-statefulset.yaml")
	sfs := resource{"StatefulSet", k.kafkasvcName, k.namespace}
	err = sfs.applyFromTemplate(oc, "-f", sfsFile, "-n", k.namespace, "-p", "NAME="+sfs.name, "-p", "NAMESPACE="+sfs.namespace, "-p", "CM_NAME="+k.kafkasvcName)
	o.Expect(err).NotTo(o.HaveOccurred())
	waitForStatefulsetReady(oc, sfs.namespace, sfs.name)

	//create kafka-consumer deployment
	deployFile := filepath.Join(kafkaFilePath, k.authtype, "kafka-consumer-deployment.yaml")
	deploy := resource{"deployment", "kafka-consumer-" + k.authtype, k.namespace}
	err = deploy.applyFromTemplate(oc, "-f", deployFile, "-n", deploy.namespace, "-p", "NAMESPACE="+deploy.namespace, "NAME="+deploy.name, "CM_NAME=kafka-client")
	o.Expect(err).NotTo(o.HaveOccurred())
	WaitForDeploymentPodsToBeReady(oc, deploy.namespace, deploy.name)
}

func (k kafka) removeZookeeper(oc *exutil.CLI) {
	resource{"configmap", k.zoosvcName, k.namespace}.clear(oc)
	resource{"svc", k.zoosvcName, k.namespace}.clear(oc)
	resource{"statefulset", k.zoosvcName, k.namespace}.clear(oc)
}

func (k kafka) removeKafka(oc *exutil.CLI) {
	resource{"secret", "kafka-client-cert", k.namespace}.clear(oc)
	resource{"secret", "kafka-cluster-cert", k.namespace}.clear(oc)
	resource{"secret", k.pipelineSecret, k.loggingNS}.clear(oc)
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrole/kafka-node-reader").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding/kafka-node-reader").Execute()
	resource{"configmap", k.kafkasvcName, k.namespace}.clear(oc)
	resource{"svc", k.kafkasvcName, k.namespace}.clear(oc)
	resource{"statefulset", k.kafkasvcName, k.namespace}.clear(oc)
	resource{"configmap", "kafka-client", k.namespace}.clear(oc)
	resource{"deployment", "kafka-consumer-" + k.authtype, k.namespace}.clear(oc)
}

func deleteEventRouter(oc *exutil.CLI, namespace string) {
	e2e.Logf("Deleting Event Router and its resources")
	r := []resource{{"deployment", "", namespace}, {"configmaps", "", namespace}, {"serviceaccounts", "", namespace}}
	for i := 0; i < len(r); i++ {
		rName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, r[i].kind, "-l", "app=eventrouter", "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			errstring := fmt.Sprintf("%v", rName)
			if strings.Contains(errstring, "NotFound") || strings.Contains(errstring, "the server doesn't have a resource type") || strings.Contains(errstring, "array index out of bounds") {
				e2e.Logf("%s not found for Event Router", r[i].kind)
				continue
			}
		}
		r[i].name = rName
		err = r[i].clear(oc)
		if err != nil {
			e2e.Logf("could not delete %s/%s", r[i].kind, r[i].name)
		}
	}
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrole", "-l", "app=eventrouter").Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebindings", "-l", "app=eventrouter").Execute()
}

func (r resource) createEventRouter(oc *exutil.CLI, parameters ...string) {
	// delete Event Router first.
	deleteEventRouter(oc, r.namespace)
	parameters = append(parameters, "-l", "app=eventrouter", "-p", "EVENT_ROUTER_NAME="+r.name)
	err := r.applyFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// createSecretForGCL creates a secret for collector pods to forward logs to Google Cloud Logging
func createSecretForGCL(oc *exutil.CLI, name, namespace string) error {
	// for GCP STS clusters, get gcp-credentials from env var GOOGLE_APPLICATION_CREDENTIALS
	_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "gcp-credentials", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		gcsCred := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", name, "-n", namespace, "--from-file=google-application-credentials.json="+gcsCred).Execute()
	}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err = os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/gcp-credentials", "-n", "kube-system", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", name, "-n", namespace, "--from-file=google-application-credentials.json="+dirname+"/service_account.json").Execute()
}

type googleCloudLogging struct {
	projectID string
	logName   string
}

// listLogEntries gets the most recent 5 entries
// example: https://cloud.google.com/logging/docs/reference/libraries#list_log_entries
// https://github.com/GoogleCloudPlatform/golang-samples/blob/HEAD/logging/simplelog/simplelog.go
func (gcl googleCloudLogging) listLogEntries(queryString string) ([]*logging.Entry, error) {
	ctx := context.Background()

	adminClient, err := logadmin.NewClient(ctx, gcl.projectID)
	if err != nil {
		e2e.Logf("Failed to create logadmin client: %v", err)
	}
	defer adminClient.Close()

	var entries []*logging.Entry
	lastHour := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)

	filter := fmt.Sprintf(`logName = "projects/%s/logs/%s" AND timestamp > "%s"`, gcl.projectID, gcl.logName, lastHour)
	if len(queryString) > 0 {
		filter += queryString
	}

	iter := adminClient.Entries(ctx,
		logadmin.Filter(filter),
		// Get most recent entries first.
		logadmin.NewestFirst(),
	)

	// Fetch the most recent 5 entries.
	for len(entries) < 5 {
		entry, err := iter.Next()
		if err == iterator.Done {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (gcl googleCloudLogging) getLogByType(logType string) ([]*logging.Entry, error) {
	searchString := " AND jsonPayload.log_type = \"" + logType + "\""
	return gcl.listLogEntries(searchString)
}

func (gcl googleCloudLogging) getLogByNamespace(namespace string) ([]*logging.Entry, error) {
	searchString := " AND jsonPayload.kubernetes.namespace_name = \"" + namespace + "\""
	return gcl.listLogEntries(searchString)
}

func extractGoogleCloudLoggingLogs(gclLogs []*logging.Entry) ([]LogEntity, error) {
	var (
		logs []LogEntity
		log  LogEntity
	)
	for _, item := range gclLogs {
		if value, ok := item.Payload.(*structpb.Struct); ok {
			v, err := value.MarshalJSON()
			if err != nil {
				return nil, err
			}
			//e2e.Logf("\noriginal log:\n%s\n\n", string(v))
			err = json.Unmarshal(v, &log)
			if err != nil {
				return nil, err
			}
			logs = append(logs, log)
		}
	}
	return logs, nil
}

func (gcl googleCloudLogging) removeLogs() error {
	ctx := context.Background()

	adminClient, err := logadmin.NewClient(ctx, gcl.projectID)
	if err != nil {
		e2e.Logf("Failed to create logadmin client: %v", err)
	}
	defer adminClient.Close()

	return adminClient.DeleteLog(ctx, gcl.logName)
}

func (gcl googleCloudLogging) waitForLogsAppearByType(logType string) error {
	return wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		logs, err := gcl.getLogByType(logType)
		if err != nil {
			return false, err
		}
		return len(logs) > 0, nil
	})
}

func (gcl googleCloudLogging) waitForLogsAppearByNamespace(namespace string) error {
	return wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		logs, err := gcl.getLogByNamespace(namespace)
		if err != nil {
			return false, err
		}
		return len(logs) > 0, nil
	})
}

// getIndexImageTag retruns a tag of index image
// this is desigend for logging upgrade test, the logging packagemanifests in the cluster may only have the testing version
// to provide a previous version for upgrade test, use clusterversion - 1 as the tag,
// for example: in OCP 4.12, use 4.11 as the tag
// index image: quay.io/openshift-qe-optional-operators/aosqe-index
func getIndexImageTag(oc *exutil.CLI) (string, error) {
	version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-ojsonpath={.status.desired.version}").Output()
	if err != nil {
		return "", err
	}
	major := strings.Split(version, ".")[0]
	minor := strings.Split(version, ".")[1]

	newMinor, err := strconv.Atoi(minor)
	if err != nil {
		return "", err
	}
	return major + "." + strconv.Itoa(newMinor-1), nil
}

func getExtLokiSecret() (string, string, error) {
	glokiUser := os.Getenv("GLOKIUSER")
	glokiPwd := os.Getenv("GLOKIPWD")
	if glokiUser == "" || glokiPwd == "" {
		return "", "", fmt.Errorf("GLOKIUSER or GLOKIPWD environment variable is not set")
	}
	return glokiUser, glokiPwd, nil
}

func checkCiphers(oc *exutil.CLI, tlsVer string, ciphers []string, server string, caFile string, cloNS string, timeInSec int) error {
	delay := time.Duration(timeInSec) * time.Second
	for _, cipher := range ciphers {
		e2e.Logf("Testing %s...", cipher)

		clPod, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "name=cluster-logging-operator"})
		if err != nil {
			return fmt.Errorf("failed to get pods: %w", err)
		}

		cmd := fmt.Sprintf("openssl s_client -%s -cipher %s -CAfile %s -connect %s", tlsVer, cipher, caFile, server)
		result, err := e2eoutput.RunHostCmdWithRetries(cloNS, clPod.Items[0].Name, cmd, 3*time.Second, 30*time.Second)

		if err != nil {
			return fmt.Errorf("failed to run command: %w", err)
		}

		if strings.Contains(string(result), ":error:") {
			errorStr := strings.Split(string(result), ":")[5]
			e2e.Logf("NOT SUPPORTED (%s)\n", errorStr)
			return fmt.Errorf(errorStr)
		} else if strings.Contains(string(result), fmt.Sprintf("Cipher is %s", cipher)) || strings.Contains(string(result), "Cipher    :") {
			e2e.Logf("SUPPORTED")
		} else {
			e2e.Logf("UNKNOWN RESPONSE")
			errorStr := string(result)
			return fmt.Errorf(errorStr)
		}

		time.Sleep(delay)
	}

	return nil
}

func checkTLSVer(oc *exutil.CLI, tlsVer string, server string, caFile string, cloNS string) error {

	e2e.Logf("Testing TLS %s ", tlsVer)

	clPod, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(context.Background(), metav1.ListOptions{LabelSelector: "name=cluster-logging-operator"})
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	cmd := fmt.Sprintf("openssl s_client -%s -CAfile %s -connect %s", tlsVer, caFile, server)
	result, err := e2eoutput.RunHostCmdWithRetries(cloNS, clPod.Items[0].Name, cmd, 3*time.Second, 30*time.Second)

	if err != nil {
		return fmt.Errorf("failed to run command: %w", err)
	}

	if strings.Contains(string(result), ":error:") {
		errorStr := strings.Split(string(result), ":")[5]
		e2e.Logf("NOT SUPPORTED (%s)\n", errorStr)
		return fmt.Errorf(errorStr)
	} else if strings.Contains(string(result), "Cipher is ") || strings.Contains(string(result), "Cipher    :") {
		e2e.Logf("SUPPORTED")
	} else {
		e2e.Logf("UNKNOWN RESPONSE")
		errorStr := string(result)
		return fmt.Errorf(errorStr)
	}

	return nil
}

func checkTLSProfile(oc *exutil.CLI, profile string, algo string, server string, caFile string, cloNS string, timeInSec int) bool {
	var ciphers []string
	var tlsVer string

	if profile == "modern" {
		e2e.Logf("Modern profile is currently not supported, please select from old, intermediate, custom")
		return false
	}

	if isFipsEnabled(oc) {
		switch profile {
		case "old":
			e2e.Logf("Checking old profile with TLS v1.3")
			tlsVer = "tls1_3"
			err := checkTLSVer(oc, tlsVer, server, caFile, cloNS)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking old profile with TLS v1.2")
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384", "ECDHE-ECDSA-CHACHA20-POLY1305", "ECDHE-ECDSA-AES128-SHA256", "ECDHE-ECDSA-AES128-SHA", "ECDHE-ECDSA-AES256-SHA384", "ECDHE-ECDSA-AES256-SHA"}
			} else if algo == "RSA" {
				ciphers = []string{"ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES128-GCM-SHA256"}
			}
			tlsVer = "tls1_2"
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).NotTo(o.HaveOccurred())

		case "intermediate":
			e2e.Logf("Setting alogorith to %s", algo)
			e2e.Logf("Checking intermediate profile with TLS v1.3")
			tlsVer = "tls1_3"
			err := checkTLSVer(oc, tlsVer, server, caFile, cloNS)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking intermediate ciphers with TLS v1.3")
			//  as openssl-3.0.7-24.el9 in CLO pod failed as below, no such issue in openssl-3.0.9-2.fc38.  use TLS 1.3 to test TSL 1.2 here.
			//  openssl s_client -tls1_2 -cipher ECDHE-RSA-AES128-GCM-SHA256 -CAfile /run/secrets/kubernetes.io/serviceaccount/service-ca.crt -connect lokistack-sample-gateway-http:8081
			//  20B4A391FFFF0000:error:1C8000E9:Provider routines:kdf_tls1_prf_derive:ems not enabled:providers/implementations/kdfs/tls1_prf.c:200:
			//  20B4A391FFFF0000:error:0A08010C:SSL routines:tls1_PRF:unsupported:ssl/t1_enc.c:83:
			tlsVer = "tls1_3"
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384"}
			} else if algo == "RSA" {
				ciphers = []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"}
			}
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking intermediate profile with TLS v1.1")
			tlsVer = "tls1_1"
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).To(o.HaveOccurred())

		case "custom":
			e2e.Logf("Setting alogorith to %s", algo)
			e2e.Logf("Checking custom profile with TLS v1.3")
			tlsVer = "tls1_3"
			err := checkTLSVer(oc, tlsVer, server, caFile, cloNS)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking custom profile ciphers with TLS v1.3")
			//  as openssl-3.0.7-24.el9 in CLO pod failed as below, no such issue in openssl-3.0.9-2.fc38.  use TLS 1.3 to test TSL 1.2 here.
			//  openssl s_client -tls1_2 -cipher ECDHE-RSA-AES128-GCM-SHA256 -CAfile /run/secrets/kubernetes.io/serviceaccount/service-ca.crt -connect lokistack-sample-gateway-http:8081
			//  20B4A391FFFF0000:error:1C8000E9:Provider routines:kdf_tls1_prf_derive:ems not enabled:providers/implementations/kdfs/tls1_prf.c:200:
			//  20B4A391FFFF0000:error:0A08010C:SSL routines:tls1_PRF:unsupported:ssl/t1_enc.c:83:
			tlsVer = "tls1_3"
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-CHACHA20-POLY1305", "ECDHE-ECDSA-AES128-GCM-SHA256"}
			} else if algo == "RSA" {
				ciphers = []string{"ECDHE-RSA-AES128-GCM-SHA256"}
			}
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking ciphers on in custom profile with TLS v1.3")
			tlsVer = "tls1_3"
			if algo == "ECDSA" {
				ciphers = []string{"TLS_AES_128_GCM_SHA256"}
			} else if algo == "RSA" {
				ciphers = []string{"TLS_AES_128_GCM_SHA256"}
			}
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).To(o.HaveOccurred())
		}

	} else {
		switch profile {
		case "old":
			e2e.Logf("Checking old profile with TLS v1.3")
			tlsVer = "tls1_3"
			err := checkTLSVer(oc, tlsVer, server, caFile, cloNS)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking old profile with TLS v1.2")
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384", "ECDHE-ECDSA-CHACHA20-POLY1305", "ECDHE-ECDSA-AES128-SHA256", "ECDHE-ECDSA-AES128-SHA", "ECDHE-ECDSA-AES256-SHA384", "ECDHE-ECDSA-AES256-SHA"}
			} else if algo == "RSA" {
				ciphers = []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES128-SHA256", "ECDHE-RSA-AES128-SHA", "ECDHE-RSA-AES256-SHA", "AES128-GCM-SHA256", "AES256-GCM-SHA384", "AES128-SHA256", "AES128-SHA", "AES256-SHA"}
			}
			tlsVer = "tls1_2"
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking old profile with TLS v1.1")
			//  remove these ciphers as openssl-3.0.7-24.el9  s_client -tls1_1 -cipher <ciphers> failed.
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-AES128-SHA", "ECDHE-ECDSA-AES256-SHA"}
			} else if algo == "RSA" {
				ciphers = []string{"AES128-SHA", "AES256-SHA"}
			}
			tlsVer = "tls1_1"
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).NotTo(o.HaveOccurred())

		case "intermediate":
			e2e.Logf("Setting alogorith to %s", algo)
			e2e.Logf("Checking intermediate profile with TLS v1.3")
			tlsVer = "tls1_3"
			err := checkTLSVer(oc, tlsVer, server, caFile, cloNS)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking intermediate profile ciphers with TLS v1.2")
			tlsVer = "tls1_2"
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384", "ECDHE-ECDSA-CHACHA20-POLY1305"}
			} else if algo == "RSA" {
				ciphers = []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"}
			}
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking intermediate profile with TLS v1.1")
			// replace checkCiphers with checkTLSVer as we needn't check all v1.1 Ciphers
			tlsVer = "tls1_1"
			err = checkTLSVer(oc, tlsVer, server, caFile, cloNS)
			o.Expect(err).To(o.HaveOccurred())

		case "custom":
			e2e.Logf("Setting alogorith to %s", algo)

			e2e.Logf("Checking custom profile with TLS v1.3")
			tlsVer = "tls1_3"
			err := checkTLSVer(oc, tlsVer, server, caFile, cloNS)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking custom profile with TLS v1.2")
			tlsVer = "tls1_2"
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-AES128-GCM-SHA256"}
			} else if algo == "RSA" {
				ciphers = []string{"ECDHE-RSA-AES128-GCM-SHA256"}
			}
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking ciphers not in custom profile with TLS v1.3")
			tlsVer = "tls1_3"
			if algo == "ECDSA" {
				ciphers = []string{"ECDHE-ECDSA-AES128-GCM-SHA256"}
			} else if algo == "RSA" {
				ciphers = []string{"TLS_AES_128_GCM_SHA256"}
			}
			err = checkCiphers(oc, tlsVer, ciphers, server, caFile, cloNS, timeInSec)
			o.Expect(err).To(o.HaveOccurred())
		}
	}
	return true
}

func checkCollectorConfiguration(oc *exutil.CLI, ns, secretName string, searchString ...string) (bool, error) {
	if ns == "" {
		ns = loggingNS
	}
	if secretName == "" {
		secretName = "collector-config"
	}
	// Parse the vector.toml file
	dirname := "/tmp/" + oc.Namespace() + "-vectortoml"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	if err != nil {
		return false, err
	}

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+secretName, "-n", ns, "--confirm", "--to="+dirname).Output()
	if err != nil {
		return false, err
	}

	filename := filepath.Join(dirname, "vector.toml")
	file, err := os.Open(filename)
	if err != nil {
		return false, err
	}
	defer file.Close()

	content, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}

	for _, s := range searchString {
		if !strings.Contains(string(content), s) {
			return false, nil
		}
	}
	return true, nil
}

func checkOperatorsRunning(oc *exutil.CLI) (bool, error) {
	jpath := `{range .items[*]}{.metadata.name}:{.status.conditions[?(@.type=='Available')].status}{':'}{.status.conditions[?(@.type=='Degraded')].status}{'\n'}{end}`
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperators.config.openshift.io", "-o", "jsonpath="+jpath).Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute 'oc get clusteroperators.config.openshift.io' command: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		e2e.Logf("%s", line)
		parts := strings.Split(line, ":")
		available := parts[1] == "True"
		degraded := parts[2] == "False"

		if !available || !degraded {
			return false, nil
		}
	}

	return true, nil
}

func waitForOperatorsRunning(oc *exutil.CLI) {
	e2e.Logf("Wait a minute to allow the cluster to reconcile the config changes.")
	time.Sleep(1 * time.Minute)
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Minute, 21*time.Minute, true, func(context.Context) (done bool, err error) {
		return checkOperatorsRunning(oc)
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to wait for operators to be running: %v", err))
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
			// sleep 5 second before doing next request
			time.Sleep(5 * time.Second)
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

// buildURL concats a url `http://foo/bar` with a path `/buzz`.
func buildURL(u, p, q string) (string, error) {
	url, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	url.Path = path.Join(url.Path, p)
	url.RawQuery = q
	return url.String(), nil
}

// GetIPVersionStackType gets IP-version Stack type of the cluster
func GetIPVersionStackType(oc *exutil.CLI) (ipvStackType string) {
	svcNetwork, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.serviceNetwork}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Count(svcNetwork, ":") >= 2 && strings.Count(svcNetwork, ".") >= 2 {
		ipvStackType = "dualstack"
	} else if strings.Count(svcNetwork, ":") >= 2 {
		ipvStackType = "ipv6single"
	} else if strings.Count(svcNetwork, ".") >= 2 {
		ipvStackType = "ipv4single"
	}
	e2e.Logf("The test cluster IP-version Stack type is :\"%s\".", ipvStackType)
	return ipvStackType
}

// convertInterfaceToArray converts interface{} to []string
/*
	example of interface{}:
	  [
	    timestamp,
		log data
	  ],
	  [
	    timestamp,
		count
	  ]
*/
func convertInterfaceToArray(t interface{}) []string {
	var data []string
	switch reflect.TypeOf(t).Kind() {
	case reflect.Slice, reflect.Array:
		s := reflect.ValueOf(t)
		for i := 0; i < s.Len(); i++ {
			data = append(data, fmt.Sprint(s.Index(i)))
		}
	}
	return data
}

// send logs over http
func postDataToHttpserver(oc *exutil.CLI, clfNS string, httpURL string, postJsonString string) bool {
	CollectorPods, err := oc.AdminKubeClient().CoreV1().Pods(clfNS).List(context.Background(), metav1.ListOptions{LabelSelector: "component=collector"})
	if err != nil || len(CollectorPods.Items) < 1 {
		e2e.Logf("failed to get pods: component=collector")
		return false
	}
	//ToDo, send logs to httpserver using service ca, oc get cm/openshift-service-ca.crt -o json |jq '.data."service-ca.crt"'
	cmd := `curl -s -k -w "%{http_code}" ` + httpURL + " -d '" + postJsonString + "'"
	result, err := e2eoutput.RunHostCmdWithRetries(clfNS, CollectorPods.Items[0].Name, cmd, 3*time.Second, 30*time.Second)
	if err != nil {
		e2e.Logf("Show more status as data can not be sent to httpserver")
		oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", clfNS, "endpoints").Output()
		oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", clfNS, "pods").Output()
		return false
	}
	if result == "200" {
		return true
	} else {
		e2e.Logf("Show result as return code is not 200")
		e2e.Logf("result=%v", result)
		return false
	}
}

// create job for rapiddast test
// Run a job to do rapiddast, the scan result will be written into pod logs and store in artifactdirPath
func rapidastScan(oc *exutil.CLI, ns, configFile string, scanPolicyFile string, apiGroupName string) (bool, error) {
	//update the token and create a new config file
	content, err := os.ReadFile(configFile)
	if err != nil {
		return false, err
	}
	defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", ns)).Execute()
	oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", fmt.Sprintf("system:serviceaccount:%s:default", ns)).Execute()
	token := getSAToken(oc, "default", ns)
	originConfig := string(content)
	targetConfig := strings.Replace(originConfig, "Bearer sha256~xxxxxxxx", "Bearer "+token, -1)
	newConfigFile := "/tmp/logdast" + getRandomString()
	f, err := os.Create(newConfigFile)
	if err != nil {
		return false, err
	}
	defer f.Close()
	defer exec.Command("rm", newConfigFile).Output()
	f.WriteString(targetConfig)

	//Create configmap
	err = oc.WithoutNamespace().Run("create").Args("-n", ns, "configmap", "rapidast-configmap", "--from-file=rapidastconfig.yaml="+newConfigFile, "--from-file=customscan.policy="+scanPolicyFile).Execute()
	if err != nil {
		return false, err
	}

	//Create job
	loggingBaseDir := exutil.FixturePath("testdata", "logging")
	jobTemplate := filepath.Join(loggingBaseDir, "rapidast/job_rapidast.yaml")
	err = oc.WithoutNamespace().Run("create").Args("-n", ns, "-f", jobTemplate).Execute()
	if err != nil {
		return false, err
	}
	//Waiting up to 10 minutes until pod Failed or Success
	err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, true, func(context.Context) (done bool, err error) {
		jobStatus, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", "job-name=rapidast-job", "-ojsonpath={.items[0].status.phase}").Output()
		e2e.Logf(" rapidast Job status %s ", jobStatus)
		if err1 != nil {
			return false, nil
		}
		if jobStatus == "Pending" || jobStatus == "Running" {
			return false, nil
		}
		if jobStatus == "Failed" {
			return true, fmt.Errorf("rapidast-job status failed")
		}
		if jobStatus == "Succeeded" {
			return true, nil
		}
		return false, nil
	})
	//return if the pod status is not Succeeded
	if err != nil {
		return false, err
	}
	// Get the rapidast pod name
	jobPods, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=rapidast-job"})
	if err != nil {
		return false, err
	}
	podLogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, jobPods.Items[0].Name).Output()
	//return if failed to get logs
	if err != nil {
		return false, err
	}

	// Copy DAST Report into $ARTIFACT_DIR
	artifactAvaiable := true
	artifactdirPath := os.Getenv("ARTIFACT_DIR")
	if artifactdirPath == "" {
		artifactAvaiable = false
	}
	info, err := os.Stat(artifactdirPath)
	if err != nil {
		e2e.Logf("%s doesn't exist", artifactdirPath)
		artifactAvaiable = false
	} else if !info.IsDir() {
		e2e.Logf("%s isn't a directory", artifactdirPath)
		artifactAvaiable = false
	}

	if artifactAvaiable {
		rapidastResultsSubDir := artifactdirPath + "/rapiddastresultslogging"
		err = os.MkdirAll(rapidastResultsSubDir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		artifactFile := rapidastResultsSubDir + "/" + apiGroupName + "_rapidast.result"
		e2e.Logf("Write report into %s", artifactFile)
		f1, err := os.Create(artifactFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f1.Close()

		_, err = f1.WriteString(podLogs)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		// print pod logs if artifactdirPath is not writable
		e2e.Logf("#oc logs -n %s %s \n %s", jobPods.Items[0].Name, ns, podLogs)
	}

	//return false, if high risk is reported
	podLogA := strings.Split(podLogs, "\n")
	riskHigh := 0
	riskMedium := 0
	re1 := regexp.MustCompile(`"riskdesc": .*High`)
	re2 := regexp.MustCompile(`"riskdesc": .*Medium`)
	for _, item := range podLogA {
		if re1.MatchString(item) {
			riskHigh++
		}
		if re2.MatchString(item) {
			riskMedium++
		}
	}
	e2e.Logf("rapidast result: riskHigh=%v riskMedium=%v", riskHigh, riskMedium)

	if riskHigh > 0 {
		return false, fmt.Errorf("high risk alert, please check the scan result report")
	}
	return true, nil
}

// Get OIDC provider for the cluster
func getOIDC(oc *exutil.CLI) (string, error) {
	oidc, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("authentication.config", "cluster", "-o=jsonpath={.spec.serviceAccountIssuer}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(oidc, "https://"), nil
}

func getPoolID(oc *exutil.CLI) (string, error) {
	// pool_id="$(oc get authentication cluster -o json | jq -r .spec.serviceAccountIssuer | sed 's/.*\/\([^\/]*\)-oidc/\1/')"
	issuer, err := getOIDC(oc)
	if err != nil {
		return "", err
	}

	return strings.Split(strings.Split(issuer, "/")[1], "-oidc")[0], nil
}

type azureMonitorLog struct {
	subscriptionID    string
	resourceGroupName string
	customerID        string
	workspaceID       string
	workspaceName     string
	primaryKey        string
	secondaryKey      string
	tPrefixOrName     string // Depend on how we defined the logType in CLF template, it can be the table name or the table name name prefix.
	location          string
	azCred            *azidentity.DefaultAzureCredential
	clientOpts        azpolicy.ClientOptions
	host              string
}

// checkout the cloudType of this cluster's platform
func getAzureCloudType(oc *exutil.CLI) string {
	var err error
	cloudType := exutil.CheckPlatform(oc)
	if cloudType == "azure" {
		cloudType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		if err != nil {
			cloudType = "azure"
		}
	}
	return cloudType
}

func (azLog *azureMonitorLog) getSourceGroupLocation() error {
	resourceGroupClient, err := armresources.NewResourceGroupsClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}

	ctx := context.Background()
	resourceGroupGetResponse, err := resourceGroupClient.Get(
		ctx,
		azLog.resourceGroupName,
		nil,
	)
	if err != nil {
		return err
	}
	azLog.location = *resourceGroupGetResponse.ResourceGroup.Location
	return nil
}

func (azLog *azureMonitorLog) createLogWorkspace() error {
	e2e.Logf("Create workspace ")
	workspacesClient, err := armoperationalinsights.NewWorkspacesClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}
	ctx := context.Background()
	pollerResp, err := workspacesClient.BeginCreateOrUpdate(
		ctx,
		azLog.resourceGroupName,
		azLog.workspaceName,
		armoperationalinsights.Workspace{
			Location:   azto.Ptr(azLog.location),
			Properties: &armoperationalinsights.WorkspaceProperties{},
		},
		nil,
	)
	if err != nil {
		return err
	}
	workspace, err := pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}
	azLog.workspaceID = *workspace.ID
	azLog.workspaceName = *workspace.Name
	azLog.customerID = *workspace.Properties.CustomerID

	shareKeyClient, err := armoperationalinsights.NewSharedKeysClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}
	resp, err := shareKeyClient.GetSharedKeys(ctx, azLog.resourceGroupName, azLog.workspaceName, nil)
	if err != nil {
		return err
	}
	azLog.primaryKey = *resp.PrimarySharedKey
	azLog.secondaryKey = *resp.SecondarySharedKey
	return nil
}

// Get azureMonitoring from Envs. CreateOrUpdate Log Analytics workspace.
func newAzureLog(oc *exutil.CLI, resouceGroupName string, workspaceName string, tPrefixOrName string) (azureMonitorLog, error) {
	var azLog azureMonitorLog
	var err error
	azLog.resourceGroupName = resouceGroupName
	//  The workspace name must be between 4 and 63 characters.
	//  The workspace name can contain only letters, numbers and '-'. The '-' shouldn't be the first or the last symbol.
	azLog.tPrefixOrName = tPrefixOrName
	azLog.workspaceName = workspaceName
	azLog.subscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
	if len(azLog.subscriptionID) == 0 {
		dat, err := oc.AsAdmin().WithoutNamespace().Run("get", "-n", "kube-system", "secret/azure-credentials", "-ojsonpath={.data.azure_subscription_id}").Output()
		if err != nil {
			return azLog, fmt.Errorf("failed to get secret/azure-credentials")
		}
		data, err := base64.StdEncoding.DecodeString(dat)
		if err != nil {
			return azLog, fmt.Errorf("failed to decode subscription_id from secret/azure-credentials")
		}

		azLog.subscriptionID = string(data)
		if len(azLog.subscriptionID) == 0 {
			return azLog, fmt.Errorf("failed as subscriptionID is empty")
		}
	}
	cloudType := getAzureCloudType(oc)
	switch strings.ToLower(cloudType) {
	case "azurepubliccloud":
		azLog.clientOpts = azcore.ClientOptions{Cloud: azcloud.AzurePublic}
		azLog.host = "ods.opinsights.azure.com"
	case "azureusgovernmentcloud":
		azLog.clientOpts = azcore.ClientOptions{Cloud: azcloud.AzureGovernment}
		azLog.host = "ods.opinsights.azure.us"
	case "azurechinacloud":
		azLog.clientOpts = azcore.ClientOptions{Cloud: azcloud.AzureChina}
		return azLog, fmt.Errorf("skip on AzureChinaCloud")
	case "azuregermancloud":
		return azLog, fmt.Errorf("skip on AzureGermanCloud")
	case "azurestackcloud":
		return azLog, fmt.Errorf("skip on AzureStackCloud")
	default:
		return azLog, fmt.Errorf("skip on %s", cloudType)
	}
	azLog.azCred, err = azidentity.NewDefaultAzureCredential(
		&azidentity.DefaultAzureCredentialOptions{ClientOptions: azLog.clientOpts},
	)
	if err != nil {
		return azLog, err
	}

	err = azLog.getSourceGroupLocation()
	if err != nil {
		return azLog, err
	}

	err = azLog.createLogWorkspace()
	if err != nil {
		return azLog, err
	}
	return azLog, nil
}

// Create a secret for collector pods to forward logs to Log Analytics workspaces.
func (azLog *azureMonitorLog) createSecret(oc *exutil.CLI, name, namespace string) error {
	return oc.NotShowInfo().AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", name, "-n", namespace, "--from-literal=shared_key="+azLog.primaryKey).Execute()
}

// query logs per table in Log Analytics workspaces.
func (azLog *azureMonitorLog) getLogByTable(logTable string) ([]azlogs.Row, error) {
	queryString := logTable + "| where TimeGenerated > ago(5m)|top 10 by TimeGenerated"
	e2e.Logf("query %v", queryString)
	var entries []azlogs.Row

	client, err := azlogs.NewClient(azLog.azCred,
		&azlogs.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return entries, err
	}

	//https://learn.microsoft.com/en-us/cli/azure/monitor/log-analytics?view=azure-cli-latest
	//https://learn.microsoft.com/en-us/azure/data-explorer/kusto/query/
	err = wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 180*time.Second, true, func(context.Context) (done bool, err error) {
		res, err1 := client.QueryWorkspace(
			context.TODO(),
			azLog.customerID,
			azlogs.QueryBody{
				Query: azto.Ptr(queryString),
			},
			nil)
		if err1 != nil {
			e2e.Logf("azlogs QueryWorkspace error: %v. continue", err1)
			return false, nil
		}
		if res.Error != nil {
			e2e.Logf("azlogs QueryWorkspace response error: %v, continue", res.Error)
			return false, nil
		}
		for _, table := range res.Tables {
			entries = append(entries, table.Rows...)
		}
		return len(entries) > 0, nil
	})

	return entries, err
}

// Delete LogWorkspace
func (azLog *azureMonitorLog) deleteWorkspace() error {
	e2e.Logf("Delete workspace %v", azLog.workspaceName)
	ctx := context.Background()
	workspacesClient, err := armoperationalinsights.NewWorkspacesClient(azLog.subscriptionID, azLog.azCred,
		&azarm.ClientOptions{
			ClientOptions: azLog.clientOpts,
		},
	)
	if err != nil {
		return err
	}
	workspacesClient.BeginDelete(ctx, azLog.resourceGroupName, azLog.workspaceName, &armoperationalinsights.WorkspacesClientBeginDeleteOptions{Force: new(bool)})
	return nil
}

// Create a linux audit policy to generate audit logs in one schedulable worker
func genLinuxAuditLogsOnWorker(oc *exutil.CLI) (string, error) {
	workerNodes, err := exutil.GetSchedulableLinuxWorkerNodes(oc)
	if err != nil || len(workerNodes) == 0 {
		return "", fmt.Errorf("can not find schedulable worker to enable audit policy")
	}
	result, err := exutil.DebugNodeWithChroot(oc, workerNodes[0].Name, "bash", "-c", "auditctl -w /var/log/pods/ -p rwa -k logging-qe-test-read-write-pod-logs")
	if err != nil && strings.Contains(result, "Rule exists") {
		//Node: we still provide the nodeName here, the policy will be deleted if defer deleteLinuxAuditPolicyFromNodes is called.
		return workerNodes[0].Name, nil
	}
	return workerNodes[0].Name, err
}

// delete the linux audit policy
func deleteLinuxAuditPolicyFromNode(oc *exutil.CLI, nodeName string) error {
	if nodeName == "" {
		return fmt.Errorf("nodeName can not be empty")
	}
	_, err := exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", "auditctl -W /var/log/pods/ -p rwa -k logging-qe-test-read-write-pod-logs")
	return err
}

// Create a linux journald logs in one schedulable worker, it is best try function
func genLinuxJournalOnWorker(oc *exutil.CLI) {
	workerNodes, err := exutil.GetSchedulableLinuxWorkerNodes(oc)
	if err != nil || len(workerNodes) == 0 {
		return
	}
	exutil.DebugNodeWithChroot(oc, workerNodes[0].Name, "bash", "-c", "logger -i -p local0.warning logging qe journald message1")
	exutil.DebugNodeWithChroot(oc, workerNodes[0].Name, "bash", "-c", "logger -i -p local0.warning logging qe journald message2")
	exutil.DebugNodeWithChroot(oc, workerNodes[0].Name, "bash", "-c", "logger -i -p local0.warning logging qe journald message3")
	return
}
