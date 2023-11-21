package workloads

import (
	"context"
	"fmt"
	"strconv"

	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// wait for daemonset to be ready
func waitForDaemonsetPodsToBeReady(oc *exutil.CLI, namespace string, name string) {
	err := wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
		daemonset, err := oc.AdminKubeClient().AppsV1().DaemonSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for availability of deployment/%s\n", name)
				return false, nil
			}
			return false, err
		}
		if daemonset.Status.DesiredNumberScheduled > 0 && daemonset.Status.NumberReady == daemonset.Status.DesiredNumberScheduled && daemonset.Status.UpdatedNumberScheduled == daemonset.Status.DesiredNumberScheduled && daemonset.Status.NumberReady == daemonset.Status.UpdatedNumberScheduled {
			e2e.Logf("Daemonset/%s is available (%d/%d)\n", name, daemonset.Status.NumberReady, daemonset.Status.DesiredNumberScheduled)
			return true, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("daemonset %s is not availabile", name))
}

// get the running pods number for special daemonset
func getDaemonsetDesiredNum(oc *exutil.CLI, namespace string, name string) int {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", name, "-n", namespace, "-o=jsonpath={.status.desiredNumberScheduled}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podNum, err := strconv.Atoi(output)
	o.Expect(err).NotTo(o.HaveOccurred())
	return podNum
}
