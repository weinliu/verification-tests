package disasterrecovery

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// ClusterSanitycheck do sanity check on cluster.
func ClusterSanitycheck(oc *exutil.CLI, projectName string) error {
	e2e.Logf("Running cluster sanity")
	errProject := wait.Poll(15*time.Second, 300*time.Second, func() (bool, error) {
		// Added like this to handle project exist error.
		cmd := fmt.Sprintf(`oc new-project %s || oc project %s`, projectName, projectName)
		_, err := exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			return false, nil
		}
		e2e.Logf("oc new-project %s succeeded", projectName)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errProject, fmt.Sprintf("oc new-project %s failed", projectName))
	errApp := wait.Poll(15*time.Second, 300*time.Second, func() (bool, error) {
		err := oc.AsAdmin().WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", projectName).Execute()
		if err != nil {
			return false, nil
		}
		e2e.Logf("oc new app succeeded")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errApp, "oc new app failed")
	errDeployment := wait.Poll(15*time.Second, 600*time.Second, func() (bool, error) {
		err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment/hello-openshift", "-n", projectName).Execute()
		if err != nil {
			return false, nil
		}
		e2e.Logf("oc log deployment succeeded")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errDeployment, "oc log deployment failed")
	gettestpod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", projectName, "--no-headers", "-o", `jsonpath={.items[0].metadata.name}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	errExec := wait.Poll(15*time.Second, 300*time.Second, func() (bool, error) {
		err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", projectName, gettestpod, "--", "/bin/sh", "-c", `echo 'Test'`).Execute()
		if err != nil {
			return false, nil
		}
		e2e.Logf("oc exec succeeded")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errExec, "oc exec failed")
	return err
}

// ClusterHealthcheck do cluster health check like pod, node and operators
func ClusterHealthcheck(oc *exutil.CLI, dirname string) error {
	err := ClusterNodesHealthcheck(oc, 300, dirname)
	if err != nil {
		return fmt.Errorf("Cluster nodes health check failed")
	}
	err = ClusterOperatorHealthcheck(oc, 900, dirname)
	if err != nil {
		return fmt.Errorf("Cluster operators health check failed")
	}
	err = ClusterPodsHealthcheck(oc, 600, dirname)
	if err != nil {
		return fmt.Errorf("Cluster pods health check failed")
	}
	return nil
}

// ClusterOperatorHealthcheck check abnormal operators
func ClusterOperatorHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal operators")
	errCo := wait.Poll(10*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		coLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "--no-headers").OutputToFile(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`cat %v | grep -v '.True.*False.*False' || true`, coLogFile)
		coLogs, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(coLogs) > 0 {
			return false, nil
		}
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("No abnormality found in cluster operators...")
		return true, nil
	})
	if errCo != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	exutil.AssertWaitPollNoErr(errCo, "Abnormality found in cluster operators.")
	return errCo
}

// ClusterPodsHealthcheck check abnormal pods.
func ClusterPodsHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal pods")
	var podLogs []byte
	errPod := wait.Poll(5*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		podLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-A").OutputToFile(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`cat %v | grep -ivE 'Running|Completed|namespace|installer' || true`, podLogFile)
		podLogs, err = exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(podLogs) > 0 {
			return false, nil
		}
		e2e.Logf("No abnormality found in pods...")
		return true, nil
	})
	if errPod != nil {
		e2e.Logf("%s", podLogs)
	}
	exutil.AssertWaitPollNoErr(errPod, "Abnormality found in pods.")
	return errPod
}

// ClusterNodesHealthcheck check abnormal nodes
func ClusterNodesHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	errNode := wait.Poll(5*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "NotReady") || strings.Contains(output, "SchedulingDisabled") {
			return false, nil
		}
		e2e.Logf("Nodes are normal...")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		return true, nil
	})
	if errNode != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	exutil.AssertWaitPollNoErr(errNode, "Abnormality found in nodes.")
	return errNode
}
