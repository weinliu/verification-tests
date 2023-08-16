package etcd

import (
	"fmt"
	"os/exec"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-etcd] ETCD", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-Critical-43330-Ensure a safety net for the 3.4 to 3.5 etcd upgrade", func() {
		var (
			err error
			msg string
		)
		g.By("Test for case OCP-43330 Ensure a safety net for the 3.4 to 3.5 etcd upgrade")
		oc.SetupProject()

		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")

		e2e.Logf("verify whether etcd version is 3.5")
		output, err := exutil.RemoteShPod(oc, "openshift-etcd", etcdPodList[0], "etcdctl")
		o.Expect(err).NotTo(o.HaveOccurred())

		o.Expect(output).To(o.ContainSubstring("3.5"))

		e2e.Logf("get the Kubernetes version")
		version, err := exec.Command("bash", "-c", "oc version | grep Kubernetes |awk '{print $3}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sVersion := string(version)
		kubeVer := strings.Split(sVersion, "+")[0]

		e2e.Logf("retrieve all the master node")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("verify the kubelet version in node details")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("node", masterNodeList[0], "-o", "custom-columns=VERSION:.status.nodeInfo.kubeletVersion").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring(kubeVer))

	})
	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-Author:geliu-Medium-52418-Add new parameter to avoid Potential etcd inconsistent revision and data occurs", func() {
		g.By("Test for case OCP-52418-Add new parameter to avoid Potential etcd inconsistent revision and data occurs")
		oc.SetupProject()

		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")

		e2e.Logf("get the expected parameter from etcd member pod")
		output, err := oc.AsAdmin().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.spec.containers[*].command[*]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("experimental-initial-corrupt-check=true"))
	})
	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-NonPreRelease-Longduration-Critical-52312-cluster-backup.sh script has a conflict to use /etc/kubernetes/static-pod-certs folder [Serial]", func() {
		g.By("Test for case OCP-52312 cluster-backup.sh script has a conflict to use /etc/kubernetes/static-pod-certs folder.")
		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		defer func() {
			e2e.Logf("Remove the certs directory")
			_, errCert := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodeList[0], []string{"-q"}, "rm", "-rf", "/etc/kubernetes/static-pod-certs")
			o.Expect(errCert).NotTo(o.HaveOccurred())
		}()
		e2e.Logf("Create the certs directory")
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodeList[0], []string{"-q"}, "mkdir", "/etc/kubernetes/static-pod-certs")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Remove the backup directory")
			_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodeList[0], []string{"-q"}, "rm", "-rf", "/home/core/assets/backup")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		firstMNode := []string{masterNodeList[0]}
		e2e.Logf("Run the backup")
		masterN, _ := runDRBackup(oc, firstMNode)
		e2e.Logf("Etcd db successfully backed up on node %v", masterN)

	})
	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-NonPreRelease-Longduration-Critical-57119-SSL/TLS: Birthday attack against 64 bit block ciphers (SWEET32) etcd metrics port 9979 [Serial]", func() {
		g.By("Test for case OCP-57119 SSL/TLS: Birthday attack against 64 bit block ciphers (SWEET32) etcd metrics port 9979 .")
		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		e2e.Logf("get the ip of the first node")
		ipOfNode := getIpOfMasterNode(oc, "node-role.kubernetes.io/master=")
		e2e.Logf("Node IP %v", ipOfNode)
		e2e.Logf("Verify the SSL Health of port 9979")
		res := verifySSLHealth(oc, ipOfNode, masterNodeList[0])
		if res {
			e2e.Logf("SSL health on port 9979 is healthy.")
		} else {
			e2e.Failf("SSL health on port 9979 is vulnerable")
		}

	})
	// author: geliu@redhat.com
	g.It("NonHyperShiftHOST-Author:geliu-Critical-54129-New etcd alerts to be added to the monitoring stack in ocp 4.10.", func() {
		g.By("Test for case OCP-54129-New etcd alerts to be added to the monitoring stack in ocp 4.10.")
		e2e.Logf("Check new alert msg have been updated")
		output, err := exec.Command("bash", "-c", "oc -n openshift-monitoring get cm prometheus-k8s-rulefiles-0 -oyaml | grep \"alert: etcd\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("etcdHighFsyncDurations"))
		o.Expect(output).To(o.ContainSubstring("etcdDatabaseQuotaLowSpace"))
		o.Expect(output).To(o.ContainSubstring("etcdExcessiveDatabaseGrowth"))
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-PstChkUpgrade-ConnectedOnly-Author:skundu-NonPreRelease-Critical-22665-Check etcd image have been update to target release value after upgrade [Serial]", func() {
		g.By("Test for case OCP-22665 Check etcd image have been update to target release value after upgrade.")
		g.By("Check if it's a proxy cluster")

		e2e.Logf("Discover all the etcd pods")
		etcdPodList := getPodListByLabel(oc, "etcd=true")

		e2e.Logf("get the image id from the etcd pod")
		etcdImageID, errImg := oc.AsAdmin().Run("get").Args("-n", "openshift-etcd", "pod", etcdPodList[0], "-o=jsonpath={.status.containerStatuses[?(@.name==\"etcd\")].imageID}").Output()
		o.Expect(errImg).NotTo(o.HaveOccurred())
		e2e.Logf("etcd imagid is %v", etcdImageID)

		e2e.Logf("select all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("get the clusterVersion")
		clusterVersion, errClvr := oc.AsAdmin().Run("get").Args("clusterversions", "version", "-o=jsonpath={.status.desired.image}").Output()
		o.Expect(errClvr).NotTo(o.HaveOccurred())
		e2e.Logf("clusterVersion is %v", clusterVersion)

		httpProxy, httpsProxy := getGlobalProxy(oc)
		if strings.Contains(httpProxy, "http") || strings.Contains(httpsProxy, "https") {
			e2e.Logf("It's a  proxy platform.")
			ret := verifyImageIDwithProxy(oc, masterNodeList, httpProxy, httpsProxy, etcdImageID, clusterVersion)
			if ret {
				e2e.Logf("Image version of etcd successfully updated to the target release on all the node(s) of cluster with proxy")
			} else {
				e2e.Failf("etcd Image update to target release on proxy cluster failed")
			}

		} else {
			g.By("Run the command on node(s)")
			res := verifyImageIDInDebugNode(oc, masterNodeList, etcdImageID, clusterVersion)
			if res {
				e2e.Logf("Image version of etcd successfully updated to the target release on all the node(s)")
			} else {
				e2e.Failf("etcd Image update to target release failed")
			}
		}
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-NonPreRelease-Longduration-Critical-64148-Verify etcd-bootstrap member is removed properly [Serial]", func() {
		g.By("Test for case OCP-64148 Verify etcd-bootstrap member is removed properly.")

		g.By("Verifying etcd cluster message and status")
		res := verifyEtcdClusterMsgStatus(oc, "etcd-bootstrap member is already removed", "True")
		if res {
			e2e.Logf("etcd bootstrap member successfully removed")
		} else {
			e2e.Failf("failed to remove the etcd bootstrap member")
		}
	})

	// author: skundu@redhat.com
	g.It("NonHyperShiftHOST-Author:skundu-NonPreRelease-Longduration-Critical-54999-Verify ETCD is not degraded in dual-stack networking cluster.[Serial]", func() {
		g.By("Test for case OCP-54999 Verify ETCD is not degraded in dual-stack networking cluster.")
		ipStackType := getIPStackType(oc)
		g.By("Skip testing on ipv4 or ipv6 single stack cluster")
		if ipStackType == "ipv4single" || ipStackType == "ipv6single" {
			g.Skip("The case only can be run on dualstack cluster , skip for single stack cluster!!!")
		}
		g.By("Verifying etcd status on dualstack cluster")
		if ipStackType == "dualstack" {
			g.By("Check etcd oprator status")
			checkOperator(oc, "etcd")
			podAllRunning := checkEtcdPodStatus(oc)
			if podAllRunning != true {
				e2e.Failf("etcd pods are not in running state")
			}

		}
	})

})
var _ = g.Describe("[sig-etcd] ETCD Microshift", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("default")
	// author: geliu@redhat.com
	g.It("MicroShiftOnly-Author:geliu-Medium-62738-[ETCD] Build Microshift prototype to launch etcd as an transient systemd unit", func() {
		g.By("1. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		masterNode := masterNodes[0]
		g.By("2. Check microshift version")
		output, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "microshift version")
		o.Expect(err).NotTo(o.HaveOccurred())

		if strings.Contains(output, "MicroShift Version") {
			e2e.Logf("Micorshift version is %v ", output)
		} else {
			e2e.Failf("Test Failed to get MicroShift Version.")
		}
		g.By("3. Check etcd version")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "microshift-etcd version")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "MicroShift-etcd Version: 4") {
			e2e.Logf("micorshift-etcd version is %v ", output)
		} else {
			e2e.Failf("Test Failed to get MicroShift-etcd Version.")
		}
		g.By("4. Check etcd run as an transient systemd unit")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", output)
		} else {
			e2e.Failf("Test Failed to get microshift-etcd.scope status.")
		}
		g.By("5. Check etcd log")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "journalctl -u microshift-etcd.scope -o cat")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Running scope as unit: microshift-etcd.scope") {
			e2e.Logf("micorshift-etcd log is %v ", output)
		} else {
			e2e.Failf("Test Failed to get micorshift-etcd log.")
		}
	})

	// author: skundu@redhat.com
	g.It("MicroShiftOnly-Author:skundu-Medium-62547-[ETCD] verify etcd quota size is configurable. [Disruptive]", func() {
		var (
			e2eTestNamespace = "microshift-ocp62547"
			valCfg           = 180
			MemoryHighValue  = valCfg * 1024 * 1024
		)
		g.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		g.By("2. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		masterNode := masterNodes[0]
		g.By("3. Check microshift is running actively")
		output, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift status is: %v ", output)
		} else {
			e2e.Failf("Test Failed to get microshift status.")
		}
		g.By("4. Check etcd status is running and active")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", output)
		} else {
			e2e.Failf("Test Failed to get microshift-etcd.scope status.")
		}

		g.By("5. Configure the memoryLimitMB field")
		configYaml := "/etc/microshift/config.yaml"
		etcdConfigCMD := fmt.Sprintf(`cat > %v << EOF
etcd:
  memoryLimitMB: %v`, configYaml, valCfg)

		defer waitForMicroshiftAfterRestart(oc, masterNodes[0])
		defer exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "rm -f /etc/microshift/config.yaml")
		_, etcdConfigcmdErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", etcdConfigCMD)
		o.Expect(etcdConfigcmdErr).NotTo(o.HaveOccurred())

		g.By("6. Restart microshift")
		waitForMicroshiftAfterRestart(oc, masterNodes[0])
		g.By("7. Check etcd status is running and active, after successful restart")
		opStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opStatus, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", opStatus)
		} else {
			e2e.Failf("Test Failed to get microshift-etcd.scope status.")
		}

		g.By("8. Verify the value of memoryLimitMB field is corrcetly configured")
		opConfig, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "/usr/bin/microshift show-config --mode effective")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opConfig, "memoryLimitMB: "+fmt.Sprint(valCfg)) {
			e2e.Logf("memoryLimitMB is successfully verified")
		} else {
			e2e.Failf("Test Failed to set memoryLimitMB field")
		}

		g.By("9. Verify the value of memoryLimitMB field is corrcetly configured")
		opStat, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl show microshift-etcd.scope | grep MemoryHigh")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opStat, fmt.Sprint(MemoryHighValue)) {
			e2e.Logf("stat MemoryHigh is successfully verified")
		} else {
			e2e.Failf("Failed to verify stat MemoryHigh")
		}

	})

	// author: skundu@redhat.com
	g.It("MicroShiftOnly-Author:skundu-Medium-60945-[ETCD] etcd should start stop automatically when microshift is started or stopped. [Disruptive]", func() {
		g.By("1. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		masterNode := masterNodes[0]
		g.By("2. Check microshift is running actively")
		output, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift status is: %v ", output)
		} else {
			e2e.Failf("Failed to get microshift status.")
		}
		g.By("3. Check etcd status is running and active")
		output, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(output, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", output)
		} else {
			e2e.Failf("Failed to get microshift-etcd.scope status.")
		}
		g.By("4. Restart microshift")
		waitForMicroshiftAfterRestart(oc, masterNodes[0])
		g.By("5. Check etcd status is running and active, after successful restart")
		opStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNode, []string{"-q"}, "bash", "-c", "systemctl status microshift-etcd.scope")
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(opStatus, "Active: active (running)") {
			e2e.Logf("microshift-etcd.scope status is: %v ", opStatus)
		} else {
			e2e.Failf("Failed to get microshift-etcd.scope status.")
		}
	})

})
