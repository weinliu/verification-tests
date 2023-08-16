package workloads

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("ocmustgather", exutil.KubeConfigPath())
	)

	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-45694-Support to collect olm data in must-gather [Slow]", func() {
		g.By("create new namespace")
		oc.SetupProject()

		g.By("Check if operator installed or not")
		out, err := oc.AsAdmin().Run("get").Args("operators").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Now installed operator is %v", out)
		if matched, _ := regexp.MatchString("No resources found", out); matched {
			g.Skip("Skip for no operator installed")
		}

		g.By("run the must-gather")
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-45694").Output()
		msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("-n", oc.Namespace(), "must-gather", "--dest-dir=/tmp/must-gather-45694").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		mustGather := string(msg)
		checkMessage := []string{
			"operators.coreos.com/installplans",
			"operators.coreos.com/operatorconditions",
			"operators.coreos.com/operatorgroups",
			"operators.coreos.com/subscriptions",
		}
		for _, v := range checkMessage {
			if !strings.Contains(mustGather, v) {
				e2e.Failf("Failed to check the olm data: " + v)
			}
		}
	})
	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ARO-Author:yinzhou-Medium-56929-run the must-gather command with own name space [Slow]", func() {
		g.By("Set namespace as privileged namespace")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		err := oc.AsAdmin().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", "system:serviceaccount:"+oc.Namespace()+":default").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-56929").Output()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer g.GinkgoRecover()
			defer wg.Done()
			_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("--run-namespace", oc.Namespace(), "must-gather", "--source-dir=/must-gather/static-pods/", "--dest-dir=/tmp/must-gather-56929").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			output, err1 := oc.AsAdmin().Run("get").Args("pod", "-n", oc.Namespace(), "-l", "app=must-gather", "-o=jsonpath={.items[0].status.phase}").Output()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			if matched, _ := regexp.MatchString("Running", output); matched {
				e2e.Logf("Check the must-gather pod running in own namespace\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot find the must-gather pod in own namespace"))
		wg.Wait()
		err = wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
			output, err1 := oc.AsAdmin().Run("get").Args("pod", "-n", oc.Namespace()).Output()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			if matched, _ := regexp.MatchString("must-gather", output); !matched {
				e2e.Logf("Check the must-gather pod dispeared in own namespace\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Still find the must-gather pod in own namespace even wait for 10 mins"))
	})
	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:yinzhou-Low-51697-Fetch audit logs of login attempts via oc commands [Slow]", func() {
		g.By("run the must-gather")
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-51697").Output()
		msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/tmp/must-gather-51697", "--", "/usr/bin/gather_audit_logs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "audit_logs/oauth-server") {
			e2e.Failf("Failed to gather the oauth audit logs")
		}
		g.By("check the must-gather result")
		oauth_audit_files := getOauthAudit("/tmp/must-gather-51697")
		for _, file := range oauth_audit_files {
			headContent, err := exec.Command("bash", "-c", fmt.Sprintf("zcat %v | head -n 1", file)).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(headContent).To(o.ContainSubstring("auditID"), "Failed to read the oauth audit logs")
		}
	})
	// author: yinzhou@redhat.com
	g.It("NonPreRelease-Longduration-Author:yinzhou-Medium-60213-oc adm must-gather with node name option should run successfully on hypershift hosted cluster", func() {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructures.config.openshift.io", "cluster", "-o=jsonpath={.status.controlPlaneTopology}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		if matched, _ := regexp.MatchString("External", output); !matched {
			g.Skip("Non hypershift hosted cluster, skip test run")
		}
		nodes, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "hypershift.openshift.io/managed=true", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		nodeName := strings.Split(nodes, " ")[0]
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-60213").Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--node-name", nodeName, "--dest-dir=/tmp/must-gather-60213").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
	})
})
