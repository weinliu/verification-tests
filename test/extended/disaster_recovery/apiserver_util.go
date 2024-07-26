package disasterrecovery

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var (
	startStates = map[string]bool{
		"poweredon": true,
		"running":   true,
		"active":    true,
		"ready":     true,
	}
	stopStates = map[string]bool{
		"poweredoff":  true,
		"stopped":     true,
		"shutoff":     true,
		"terminated":  true,
		"paused":      true,
		"deallocated": true,
		"notready":    true,
	}
)

// ClusterSanitycheck do sanity check on cluster.
func ClusterSanitycheck(oc *exutil.CLI, projectName string) error {
	e2e.Logf("Running cluster sanity")

	// Helper function for polling
	pollAndLog := func(interval, timeout time.Duration, action func() error, successMsg, errorMsg string) error {
		err := wait.PollUntilContextTimeout(context.Background(), interval, timeout, false, func(cxt context.Context) (bool, error) {
			if errDo := action(); errDo != nil {
				return false, nil
			}
			e2e.Logf(successMsg)
			return true, nil
		})
		if err != nil {
			return fmt.Errorf(errorMsg)
		}
		return nil
	}

	// Create or switch to the project
	err := pollAndLog(10*time.Second, 600*time.Second, func() error {
		cmd := fmt.Sprintf(`oc new-project %s --skip-config-write || oc project %s`, projectName, projectName)
		_, err := exec.Command("bash", "-c", cmd).Output()
		return err
	}, fmt.Sprintf("oc new-project %s succeeded", projectName), fmt.Sprintf("oc new-project %s failed", projectName))
	if err != nil {
		return err
	}

	// Deploy the application
	err = pollAndLog(10*time.Second, 600*time.Second, func() error {
		return oc.AsAdmin().WithoutNamespace().Run("new-app").
			Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", projectName).Execute()
	}, "oc new app succeeded", "oc new app failed")
	if err != nil {
		return err
	}

	// Check deployment logs
	err = pollAndLog(15*time.Second, 900*time.Second, func() error {
		return oc.AsAdmin().WithoutNamespace().Run("logs").
			Args("deployment/hello-openshift", "-n", projectName).Execute()
	}, "oc log deployment succeeded", "oc log deployment failed")
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil {
		return err
	}

	// Get test pod name
	gettestpod, err := oc.AsAdmin().WithoutNamespace().Run("get").
		Args("pod", "-n", projectName, "--no-headers", "-o", `jsonpath={.items[0].metadata.name}`).Output()
	if err != nil {
		return err
	}

	// Execute command in test pod
	err = pollAndLog(5*time.Second, 60*time.Second, func() error {
		return oc.AsAdmin().WithoutNamespace().Run("exec").
			Args("-n", projectName, gettestpod, "--", "/bin/sh", "-c", `echo 'Test'`).Execute()
	}, "oc exec succeeded", "oc exec failed")
	return err
}

// ClusterHealthcheck do cluster health check like pod, node and operators
func ClusterHealthcheck(oc *exutil.CLI, dirname string) error {
	err := ClusterNodesHealthcheck(oc, 900, dirname)
	if err != nil {
		return fmt.Errorf("%s: %w", "Failed to cluster health check::Abnormal nodes found ", err)
	}
	err = ClusterOperatorHealthcheck(oc, 1500, dirname)
	if err != nil {
		return fmt.Errorf("%s: %w", "Failed to cluster health check::Abnormal cluster operators found", err)
	}
	// Check the load of cluster nodes
	err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 180*time.Second, false, func(cxt context.Context) (bool, error) {
		_, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes.metrics").Output()
		if err1 != nil {
			e2e.Logf("Nodes metrics are not ready!")
			return false, nil
		}
		e2e.Logf("Nodes metrics are ready!")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Unable to get nodes metrics!")
	outTop, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "nodes").Output()
	e2e.Logf("#### Output load of cluster nodes ####\n%s", outTop)

	err = ClusterPodsHealthcheck(oc, 900, dirname)
	if err != nil {
		return fmt.Errorf("%s: %w", "Failed to cluster health check::Abnormal pods found", err)
	}
	return nil
}

// ClusterOperatorHealthcheck check abnormal operators
func ClusterOperatorHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal operators")
	errCo := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		coLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "--no-headers").OutputToFile(dirname)
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -v '.True.*False.*False' || true`, coLogFile)
			coLogs, err := exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(coLogs) > 0 {
				return false, nil
			}
		} else {
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
	return errCo
}

// ClusterPodsHealthcheck check abnormal pods.
func ClusterPodsHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal pods")
	var podLogs []byte
	errPod := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		podLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-A").OutputToFile(dirname)
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -ivE 'Running|Completed|namespace|installer' || true`, podLogFile)
			podLogs, err = exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(podLogs) > 0 {
				return false, nil
			}
		} else {
			return false, nil
		}
		e2e.Logf("No abnormality found in pods...")
		return true, nil
	})
	if errPod != nil {
		e2e.Logf("%s", podLogs)
	}
	return errPod
}

// ClusterNodesHealthcheck check abnormal nodes
func ClusterNodesHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	errNode := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, time.Duration(waitTime)*time.Second, false, func(cxt context.Context) (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
		if err == nil {
			if strings.Contains(output, "NotReady") || strings.Contains(output, "SchedulingDisabled") {
				return false, nil
			}
		} else {
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
	return errNode
}

func isAzureStackCluster(oc *exutil.CLI) (bool, string) {
	cloudName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.ToLower(cloudName) == "azurestackcloud" {
		e2e.Logf("This is Azure Stack cluster.")
		return true, cloudName
	}
	return false, ""
}

// Get a random number of int32 type [m,n], n > m
func getRandomNum(m int32, n int32) int32 {
	rand.Seed(time.Now().UnixNano())
	return rand.Int31n(n-m+1) + m
}
