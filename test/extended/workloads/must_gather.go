package workloads

import (
	"encoding/json"
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
	g.It("ARO-Author:yinzhou-Medium-56929-run the must-gather command with own name space [Slow]", func() {
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
		if err != nil {
			oc.Run("get").Args("events", "-n", oc.Namespace()).Execute()
		}
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

	// author: yinzhou@redhat.com
	g.It("NonPreRelease-Longduration-NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:yinzhou-High-70982-must-gather support since and since-time flags", func() {
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-70982").Output()
		exutil.By("Set namespace as privileged namespace")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		exutil.By("1. Test must-gather with correct since format should succeed.\n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--since=1m", "--dest-dir=/tmp/must-gather-70982").Output()
		if err != nil {
			e2e.Failf("Must-gather falied with error %v", err)
		}

		exutil.By("2. Test must-gather with correct since format and special logs should succeed.\n")
		workerNodeList, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())

		timeNow := getTimeFromNode(oc, workerNodeList[0], oc.Namespace())
		e2e.Logf("The time now is  %v", timeNow)
		timeStampOne := timeNow.Add(time.Minute * -5).Format("15:04:05")
		e2e.Logf("The time stamp is  %v", timeStampOne)
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--since=2m", "--dest-dir=/tmp/must-gather-70982/mustgather2", "--", "/usr/bin/gather_network_logs").Output()
		if err != nil {
			e2e.Failf("Must-gather falied with error %v", err)
		}

		checkMustgatherLogTime("/tmp/must-gather-70982/mustgather2", workerNodeList[0], timeStampOne)

		exutil.By("3. Test must-gather with correct since-time format should succeed.\n")
		now := getTimeFromNode(oc, workerNodeList[0], oc.Namespace())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--since-time="+now.Add(time.Minute*-2).Format("2006-01-02T15:04:05Z"), "--dest-dir=/tmp/must-gather-70982").Output()
		if err != nil {
			e2e.Failf("Must-gather falied with error %v", err)
		}
		exutil.By("4. Test must-gather with correct since-time format and specidal logs should succeed.\n")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--since-time="+now.Add(time.Minute*-1).Format("2006-01-02T15:04:05Z"), "--dest-dir=/tmp/must-gather-70982", "--", "/usr/bin/gather_network_logs").Output()
		if err != nil {
			e2e.Failf("Must-gather falied with error %v", err)
		}
		exutil.By("5. Test must-gather with wrong since-time format should falied.\n")
		_, warningErr, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--since-time="+now.Format("2006-01-02"), "--dest-dir=/tmp/must-gather-70982", "--", "/usr/bin/gather_network_logs").Outputs()
		o.Expect(err).To(o.HaveOccurred())
		exutil.By("6. Test must-gather with wrong since-time format should falied.\n")
		o.Expect(strings.Contains(warningErr, "since-time only accepts times matching RFC3339")).To(o.BeTrue())
		_, warningErr, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--since-time="+now.Format("2006-01-02T15:04:05"), "--dest-dir=/tmp/must-gather-70982", "--", "/usr/bin/gather_network_logs").Outputs()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(warningErr, "since-time only accepts times matching RFC3339")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-71212-oc adm inspect should support since and sincetime", func() {
		defer exec.Command("bash", "-c", "rm -rf /tmp/inspect71212").Output()
		exutil.By("Set namespace as privileged namespace")
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		exutil.By("1. Test inspect with correct since-time format should succeed and gather correct logs.\n")
		workerNodeList, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())

		now := getTimeFromNode(oc, workerNodeList[0], oc.Namespace())
		timeStamp := now.Add(time.Minute * -5).Format("2006-01-02T15:04:05Z")

		podname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-multus", "-l", "app=multus", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("inspect", "ns", "openshift-multus", "--since-time="+now.Add(time.Minute*-2).Format("2006-01-02T15:04:05Z"), "--dest-dir=/tmp/inspect71212").Output()
		if err != nil {
			e2e.Failf("Inspect falied with error %v", err)
		}
		checkInspectLogTime("/tmp/inspect71212", podname, timeStamp)
		exutil.By("2. Test inspect with wrong since-time format should failed.\n")
		_, warningErr, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("inspect", "ns", "openshift-multus", "--since-time="+now.Format("2006-01-02T15:04:05"), "--dest-dir=/tmp/inspect71212").Outputs()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(warningErr, "--since-time only accepts times matching RFC3339")).To(o.BeTrue())
		exutil.By("3. Test inspect with wrong since-time format should failed.\n")
		_, warningErr, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("inspect", "ns", "openshift-multus", "--since-time="+now.Format("2006-01-02"), "--dest-dir=/tmp/inspect71212").Outputs()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(warningErr, "--since-time only accepts times matching RFC3339")).To(o.BeTrue())
		exutil.By("4. Test inspect with correct since format should succeed.\n")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("inspect", "ns", "openshift-multus", "--since=1m", "--dest-dir=/tmp/inspect71212").Output()
		if err != nil {
			e2e.Failf("Inspect falied with error %v", err)
		}
		exutil.By("5. Test inspect with wrong since format should falied.\n")
		_, warningErr, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("inspect", "ns", "openshift-multus", "--since=1", "--dest-dir=/tmp/inspect71212").Outputs()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(warningErr, "time: missing unit")).To(o.BeTrue())
	})

	// author: knarra@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:knarra-Critical-73054-Verify version of oc binary is included into the must-gather directory when running oc adm must-gather command [Slow]", func() {
		exutil.By("Get oc client version")
		clientVersion, clientVersionErr := oc.Run("version").Args("-o", "json").Output()
		o.Expect(clientVersionErr).NotTo(o.HaveOccurred())
		versionInfo := &VersionInfo{}
		if err := json.Unmarshal([]byte(clientVersion), &versionInfo); err != nil {
			e2e.Failf("unable to decode version with error: %v", err)
		}
		e2e.Logf("Version output is %s", versionInfo.ClientInfo.GitVersion)

		exutil.By("Run the must-gather")
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-73054").Output()
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/tmp/must-gather-73054", "--", "/usr/bin/gather_audit_logs").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("check the must-gather and verify that oc binary version is included")
		headContent, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/must-gather-73054/must-gather.logs| head -n 5")).Output()
		e2e.Logf("headContent is %s", headContent)
		if err != nil {
			e2e.Logf("Error is %s", err.Error())
		}
		o.Expect(headContent).To(o.ContainSubstring(versionInfo.ClientInfo.GitVersion))
	})

	// author: knarra@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Critical-73055-Verify logs generated are included in the must-gather directory when running the oc adm must-gather command [Slow]", func() {
		exutil.By("Run the must-gather")
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-73055").Output()
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-73055-1").Output()
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-73055-2").Output()
		mustGatherOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/tmp/must-gather-73055", "--", "/usr/bin/gather_audit_logs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("check logs generated are included in the must-gather directory when must-gather is run")
		fileContent, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/must-gather-73055/must-gather.logs | head -n 10")).Output()
		if err != nil {
			e2e.Logf("Error reading file must-gather.logs:", err)
		}
		fileContentStr := string(fileContent)
		if !strings.Contains(mustGatherOutput, fileContentStr) {
			e2e.Failf("contains output")
		}

		// Check if gather.logs exists in the directory for default must-gather image
		checkGatherLogsForImage(oc, "/tmp/must-gather-73055")

		// Check if gather.logs exists in the directory for CNV image
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--image=registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.15.0", "--dest-dir=/tmp/must-gather-73055-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkGatherLogsForImage(oc, "/tmp/must-gather-73055-1")

		// Check if gather.logs exists for both the images when passed to must-gather
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--image-stream=openshift/must-gather", "--image=registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.15.0", "--dest-dir=/tmp/must-gather-73055-2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkGatherLogsForImage(oc, "/tmp/must-gather-73055-2")
	})

})
