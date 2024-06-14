package clusterinfrastructure

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ingressControllerDescription struct {
	template string
	name     string
}

type loadBalancerServiceDescription struct {
	template      string
	name          string
	awssubnet     string
	awslabel      string
	gcptype       string
	azureinternal bool
	azuresubnet   string
	namespace     string
}

type podDescription struct {
	template  string
	name      string
	namespace string
}

func (ingressController *ingressControllerDescription) createIngressController(oc *exutil.CLI) {
	e2e.Logf("Creating ingressController ...")
	exutil.CreateNsResourceFromTemplate(oc, "openshift-ingress-operator", "--ignore-unknown-parameters=true", "-f", ingressController.template, "-p", "NAME="+ingressController.name)
}

func (ingressController *ingressControllerDescription) deleteIngressController(oc *exutil.CLI) error {
	e2e.Logf("Deleting ingressController ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("ingressController", ingressController.name, "-n", "openshift-ingress-operator").Execute()
}

func (loadBalancerService *loadBalancerServiceDescription) createLoadBalancerService(oc *exutil.CLI) {
	e2e.Logf("Creating loadBalancerService ...")
	var err error
	if strings.Contains(loadBalancerService.template, "annotations") {
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", loadBalancerService.template, "-p", "NAME="+loadBalancerService.name, "NAMESPACE="+loadBalancerService.namespace, "AWSSUBNET="+loadBalancerService.awssubnet, "AWSLABEL="+loadBalancerService.awslabel, "GCPTYPE="+loadBalancerService.gcptype, "AZUREINTERNAL="+strconv.FormatBool(loadBalancerService.azureinternal), "AZURESUNBET="+loadBalancerService.azuresubnet)
	} else {
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", loadBalancerService.template, "-p", "NAME="+loadBalancerService.name, "NAMESPACE="+loadBalancerService.namespace)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (loadBalancerService *loadBalancerServiceDescription) deleteLoadBalancerService(oc *exutil.CLI) error {
	e2e.Logf("Deleting loadBalancerService ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", loadBalancerService.name, "-n", loadBalancerService.namespace).Execute()
}

func (pod *podDescription) createPod(oc *exutil.CLI) {
	e2e.Logf("Creating pod ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pod *podDescription) deletePod(oc *exutil.CLI) error {
	e2e.Logf("Deleting pod ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod.name, "-n", pod.namespace).Execute()
}

// waitForClusterHealthy check if new machineconfig is applied successfully
func waitForClusterHealthy(oc *exutil.CLI) {
	e2e.Logf("Waiting for the cluster healthy ...")
	// sleep for 5 minites to make sure related mcp start to update
	time.Sleep(5 * time.Minute)
	timeToWait := time.Duration(getNodeCount(oc)*5) * time.Minute
	pollErr := wait.Poll(1*time.Minute, timeToWait-5, func() (bool, error) {
		master, errMaster := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", "master", "-o", "jsonpath='{.status.conditions[?(@.type==\"Updated\")].status}'").Output()
		worker, errWorker := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", "worker", "-o", "jsonpath='{.status.conditions[?(@.type==\"Updated\")].status}'").Output()
		if errMaster != nil || errWorker != nil {
			e2e.Logf("the err:%v,%v, and try next round", errMaster, errWorker)
			return false, nil
		}
		if strings.Contains(master, "True") && strings.Contains(worker, "True") {
			e2e.Logf("mc operation is completed on mcp")
			return true, nil
		}
		return false, nil
	})
	if pollErr != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Failf("Expected cluster is not healthy after waiting up to %s minutes ...", timeToWait)
	}
	e2e.Logf("Cluster is healthy ...")
}

func getNodeCount(oc *exutil.CLI) int {
	nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeCount := int(strings.Count(nodes, "Ready")) + int(strings.Count(nodes, "NotReady"))
	return nodeCount
}

// SkipIfCloudControllerManagerNotDeployed check if ccm is deployed
func SkipIfCloudControllerManagerNotDeployed(oc *exutil.CLI) {
	var ccm string
	var err error
	ccm, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", "openshift-cloud-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
	if err == nil {
		if len(ccm) == 0 {
			g.Skip("Skip for cloud-controller-manager is not deployed!")
		}
	}
}

// wait for the named resource is disappeared, e.g. used while router deployment rolled out
func waitForResourceToDisappear(oc *exutil.CLI, ns, rsname string) error {
	return wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(rsname, "-n", ns).Output()
		e2e.Logf("check resource %v and got: %v", rsname, status)
		primary := false
		if err != nil {
			if strings.Contains(status, "NotFound") {
				e2e.Logf("the resource is disappeared!")
				primary = true
			} else {
				e2e.Logf("failed to get the resource: %v, retrying...", err)
			}
		} else {
			e2e.Logf("the resource is still there, retrying...")
		}
		return primary, nil
	})
}

func waitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.conditions[?(@.type==\"Ready\")].status}").Output()
		e2e.Logf("the Ready status of pod is %v", status)
		if err != nil || status == "" {
			e2e.Logf("failed to get pod status: %v, retrying...", err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("the pod Ready status not met; wanted True but got %v, retrying...", status)
			return false, nil
		}
		return true, nil
	})
}

func waitForClusterOperatorsReady(oc *exutil.CLI, clusterOperators ...string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		for _, co := range clusterOperators {
			coState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator/"+co, "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
			if err != nil || coState == "" {
				e2e.Logf("failed to get co state: %v, retrying...", err)
				return false, nil
			}
			if !strings.Contains(coState, "TrueFalseFalse") {
				e2e.Logf("the co: %v status not met; wanted TrueFalseFalse but got %v, retrying...", co, coState)
				return false, nil
			}
		}
		return true, nil
	})
}

// getLBSvcIP get Load Balancer service IP/Hostname
func getLBSvcIP(oc *exutil.CLI, loadBalancerService loadBalancerServiceDescription) string {
	e2e.Logf("Getting the Load Balancer service IP ...")
	iaasPlatform := clusterinfra.CheckPlatform(oc)
	var jsonString string
	if iaasPlatform == clusterinfra.AWS || iaasPlatform == clusterinfra.IBMCloud {
		jsonString = "-o=jsonpath={.status.loadBalancer.ingress[0].hostname}"
	} else {
		jsonString = "-o=jsonpath={.status.loadBalancer.ingress[0].ip}"
	}
	err := wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
		svcStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("svc", loadBalancerService.name, "-n", loadBalancerService.namespace, jsonString).Output()
		if err != nil || svcStatus == "pending" || svcStatus == "" {
			e2e.Logf("External-IP is not assigned and waiting up to 20 seconds ...")
			return false, nil
		}
		e2e.Logf("External-IP is assigned: %s" + svcStatus)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "External-IP is not assigned in 5 minite")
	svcStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("svc", "-n", loadBalancerService.namespace, loadBalancerService.name, jsonString).Output()
	e2e.Logf("The %s lb service ip/hostname is %q", loadBalancerService.name, svcStatus)
	return svcStatus
}

func waitForLoadBalancerReady(oc *exutil.CLI, externalIP string) {
	e2e.Logf("Getting the Load Balancer service IP ...")
	errWait := wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
		msg, err := exec.Command("bash", "-c", "curl "+externalIP).Output()
		if err != nil {
			e2e.Logf("failed to execute the curl: %s. Trying again", err)
			return false, nil
		}
		e2e.Logf("msg -->: %s", msg)
		if !strings.Contains(string(msg), "Hello-OpenShift") {
			e2e.Logf("Load balancer is not ready yet and waiting up to 5 minutes ...")
			return false, nil
		}
		e2e.Logf("Load balancer is ready")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errWait, "Load balancer is not ready after waiting up to 5 minutes ...")
}
