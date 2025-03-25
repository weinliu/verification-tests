package workloads

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cli] Workloads oc command upgrade works fine", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("oc", exutil.KubeConfigPath())
	)

	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-NonPreRelease-PreChkUpgrade-Medium-33209-Check some container related oc commands still work after upgrade", func() {
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "workloads-upgrade").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--import-mode", "PreserveOriginal", "-n", "workloads-upgrade", "--name=example-ocupgrade").Execute()
			if err != nil {
				e2e.Logf("failed to use new-app command: %s. Trying again", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot create the oc-upgrade test app"))
	})

	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-NonPreRelease-PstChkUpgrade-Medium-33209-Check some container related oc commands still work after upgrade", func() {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "workloads-upgrade").Output()
		if err != nil || strings.Contains(output, "not found") {
			g.Skip("Could not found the workloads-upgrade on the running OCP cluster, skipping")
		}

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "workloads-upgrade").Execute()
		podStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "deployment=example-ocupgrade", "-n", "workloads-upgrade", "-o=jsonpath={.items[*].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(podStatus, "Running")).To(o.BeTrue())
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "deployment=example-ocupgrade", "-n", "workloads-upgrade", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "workloads-upgrade", podName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Run exec command")
		err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "workloads-upgrade", podName, "--", "date").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer exec.Command("kill", "-9", `lsof -t -i:40039`).Output()
		cmd2, _, _, err := oc.AsAdmin().Run("port-forward").Args("-n", "workloads-upgrade", podName, "40039:8080").Background()
		defer cmd2.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			checkOutput, err := exec.Command("bash", "-c", "curl http://127.0.0.1:40039 --noproxy \"127.0.0.1\"").Output()
			if err != nil {
				e2e.Logf("failed to execute the curl: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("Hello OpenShift", string(checkOutput)); matched {
				e2e.Logf("Check the port-forward command succeeded\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get the port-forward result"))
	})
})
