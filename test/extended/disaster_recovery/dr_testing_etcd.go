package disasterrecovery

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-disasterrecovery] DR_Testing", func() {
	defer g.GinkgoRecover()
	var (
		oc           = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())
		iaasPlatform string
	)

	g.BeforeEach(func() {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		iaasPlatform = strings.ToLower(output)
		if strings.Contains(iaasPlatform, "baremetal") || strings.Contains(iaasPlatform, "none") {
			g.Skip("IAAS platform: " + iaasPlatform + " is not supported yet for DR - skipping test ...")
		}
		if !IsCOHealthy(oc, "etcd") {
			g.Skip("PreCheck : etcd operator is degraded. Hence skipping the test.")
		}
		if !IsCOHealthy(oc, "kube-apiserver") {
			g.Skip("PreCheck : kube-apiserver operator is degraded. Hence skipping the test.")
		}
	})

	g.AfterEach(func() {
		if !healthyCheck(oc) {
			e2e.Failf("Cluster healthy check failed after the test.")
		}
	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-NonPreRelease-Longduration-Critical-42183-backup and restore should perform consistency checks on etcd snapshots [Disruptive]", func() {
		g.By("Test for case OCP-42183 backup and restore should perform consistency checks on etcd snapshots")

		g.By("select all the master node")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		g.By("Check etcd oprator status")
		checkOperator(oc, "etcd")
		g.By("Check kube-apiserver oprator status")
		checkOperator(oc, "kube-apiserver")

		g.By("Run the backup")
		masterN, etcdDb := runDRBackup(oc, masterNodeList)

		defer func() {
			_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterN, []string{"-q"}, "rm", "-rf", "/home/core/assets/backup")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Corrupt the etcd db file ")
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterN, []string{"-q"}, "truncate", "-s", "126k", etcdDb)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Run the restore")
		output, _ := exutil.DebugNodeWithOptionsAndChroot(oc, masterN, []string{"-q"}, "/usr/local/bin/cluster-restore.sh", "/home/core/assets/backup")
		o.Expect(output).To(o.ContainSubstring("Backup appears corrupted. Aborting!"))
	})

	// author: skundu@redhat.com
	g.It("Author:skundu-LEVEL0-Longduration-NonPreRelease-Critical-77921-workflow of quorum restoration. [Disruptive][Slow]", func() {

		var (
			bastionHost    = ""
			userForBastion = ""
		)
		g.By("check the platform is supported or not")
		supportedList := []string{"aws", "gcp", "azure", "vsphere", "nutanix", "ibmcloud"}
		platformListWithoutBastion := []string{"vsphere", "nutanix"}
		support := in(iaasPlatform, supportedList)
		if support != true {
			g.Skip("The platform is not supported now, skip the cases!!")
		}
		privateKeyForBastion := os.Getenv("SSH_CLOUD_PRIV_KEY")
		if privateKeyForBastion == "" {
			g.Skip("Failed to get the private key, skip the cases!!")
		}
		withoutBastion := in(iaasPlatform, platformListWithoutBastion)
		if !withoutBastion {
			bastionHost = os.Getenv("QE_BASTION_PUBLIC_ADDRESS")
			if bastionHost == "" {
				g.Skip("Failed to get the qe bastion public ip, skip the case !!")
			}
			userForBastion = getUserNameAndKeyonBationByPlatform(iaasPlatform)
			if userForBastion == "" {
				g.Skip("Failed to get the user for bastion host, hence skipping the case!!")
			}
		}

		g.By("make sure all the etcd pods are running")
		podAllRunning := checkEtcdPodStatus(oc)
		if podAllRunning != true {
			g.Skip("The ectd pods are not running")
		}
		defer o.Expect(checkEtcdPodStatus(oc)).To(o.BeTrue())

		g.By("select all the master node")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		masterNodeInternalIPList := getNodeInternalIPListByLabel(oc, "node-role.kubernetes.io/master=")

		e2e.Logf("bastion host is  : %v", bastionHost)
		e2e.Logf("platform is  : %v", iaasPlatform)
		e2e.Logf("user on bastion is  : %v", userForBastion)

		g.By("Make sure all the nodes are normal")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
		checkMessage := []string{
			"SchedulingDisabled",
			"NotReady",
		}
		for _, v := range checkMessage {
			if !o.Expect(out).ShouldNot(o.ContainSubstring(v)) {
				g.Skip("The cluster nodes is abnormal, skip this case")
			}
		}

		g.By("Check etcd oprator status")
		checkOperator(oc, "etcd")
		g.By("Check kube-apiserver oprator status")
		checkOperator(oc, "kube-apiserver")
		g.By("Make the two non-recovery control plane nodes NOT_READY")
		//if assert err the cluster will be unavailable
		for i := 1; i < len(masterNodeInternalIPList); i++ {
			_, err := runPSCommand(bastionHost, masterNodeInternalIPList[i], "sudo /usr/local/bin/disable-etcd.sh", privateKeyForBastion, userForBastion)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err1 := runPSCommand(bastionHost, masterNodeInternalIPList[i], "sudo rm -rf /var/lib/etcd", privateKeyForBastion, userForBastion)
			o.Expect(err1).NotTo(o.HaveOccurred())
			_, err2 := runPSCommand(bastionHost, masterNodeInternalIPList[i], "sudo systemctl stop kubelet.service", privateKeyForBastion, userForBastion)
			o.Expect(err2).NotTo(o.HaveOccurred())
		}

		g.By("Run the quorum-restore script on the recovery control plane host")
		msg, err := runPSCommand(bastionHost, masterNodeInternalIPList[0], "sudo -E /usr/local/bin/quorum-restore.sh", privateKeyForBastion, userForBastion)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("starting restore-etcd static pod"))

		g.By("Wait for the api server to come up after restore operation.")
		errW := wait.Poll(20*time.Second, 900*time.Second, func() (bool, error) {
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Output()
			if err != nil {
				e2e.Logf("Fail to get master, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString(masterNodeInternalIPList[0], out); matched {
				e2e.Logf("Api is back online:")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errW, "the Apiserver has not come up after quorum restore operation")

		g.By("Start the kubelet service on both the non-recovery control plane hosts")
		for i := 1; i < len(masterNodeList); i++ {
			_, _ = runPSCommand(bastionHost, masterNodeInternalIPList[i], "sudo systemctl start kubelet.service", privateKeyForBastion, userForBastion)

		}
		g.By("Wait for the nodes to be Ready.")
		for i := 0; i < len(masterNodeList); i++ {
			err := wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
				out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", masterNodeList[i]).Output()
				if err != nil {
					e2e.Logf("Fail to get master, error: %s. Trying again", err)
					return false, nil
				}
				if matched, _ := regexp.MatchString(" Ready", out); matched {
					e2e.Logf("Node %s is back online:\n%s", masterNodeList[i], out)
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "the kubelet start has not brought the node online and Ready")
		}
		defer checkOperator(oc, "etcd")
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", fmt.Sprintf("{\"spec\": {\"unsupportedConfigOverrides\": null}}")).Execute()
		g.By("Turn off quorum guard to ensure revision rollouts of static pods")
		errWait := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			errGrd := oc.AsAdmin().WithoutNamespace().Run("patch").Args("etcd", "cluster", "--type=merge", "-p", fmt.Sprintf("{\"spec\": {\"unsupportedConfigOverrides\": {\"useUnsupportedUnsafeNonHANonProductionUnstableEtcd\": true}}}")).Execute()
			if errGrd != nil {
				e2e.Logf("server is not ready yet, error: %s. Trying again ...", errGrd)
				return false, nil
			} else {
				e2e.Logf("successfully patched.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "unable to patch the server to turn off the quorum guard.")

		// both etcd and kube-apiserver operators start and end roll out almost simultaneously.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer g.GinkgoRecover()
			defer wg.Done()
			waitForOperatorRestart(oc, "etcd")
		}()
		waitForOperatorRestart(oc, "kube-apiserver")
		wg.Wait()
	})
	// author: geliu@redhat.com
	g.It("Author:geliu-NonPreRelease-Longduration-Critical-50205-lost master can be replaced by new one with machine config recreation in ocp 4.x [Disruptive][Slow]", func() {
		g.By("Test for case lost master can be replaced by new one with machine config recreation in ocp 4.x")

		g.By("Get all the master node name & count")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		masterNodeCount := len(masterNodeList)

		g.By("make sure all the etcd pods are running")
		defer o.Expect(checkEtcdPodStatus(oc)).To(o.BeTrue())
		podAllRunning := checkEtcdPodStatus(oc)
		if podAllRunning != true {
			g.Skip("The ectd pods are not running")
		}

		g.By("Export the machine config file for 1st master node")
		output, err := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		masterMachineNameList := strings.Fields(output)
		machineYmlFile := ""
		machineYmlFile, err = oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", masterMachineNameList[0], "-o", "yaml").OutputToFile("machine.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		newMachineConfigFile := strings.Replace(machineYmlFile, "machine.yaml", "machineUpd.yaml", -1)
		defer exec.Command("bash", "-c", "rm -f "+machineYmlFile).Output()
		defer exec.Command("bash", "-c", "rm -f "+newMachineConfigFile).Output()

		g.By("update machineYmlFile to newMachineYmlFile:")
		newMasterMachineNameSuffix := masterMachineNameList[0] + "00"
		o.Expect(updateMachineYmlFile(machineYmlFile, masterMachineNameList[0], newMasterMachineNameSuffix)).To(o.BeTrue())

		g.By("Create new machine")
		resultFile, _ := exec.Command("bash", "-c", "cat "+newMachineConfigFile).Output()
		e2e.Logf("####newMasterMachineNameSuffix is %s\n", string(resultFile))
		_, err = oc.AsAdmin().Run("create").Args("-n", "openshift-machine-api", "-f", newMachineConfigFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitMachineStatusRunning(oc, newMasterMachineNameSuffix)

		g.By("Delete machine of the unhealthy master node")
		_, err = oc.AsAdmin().Run("delete").Args("-n", "openshift-machine-api", "machine", masterMachineNameList[0]).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(getNodeListByLabel(oc, "node-role.kubernetes.io/master="))).To(o.Equal(masterNodeCount))
	})

	// author: skundu@redhat.com
	g.It("Longduration-Author:skundu-NonPreRelease-Critical-51109-Delete an existing machine at first and then add a new one. [Disruptive]", func() {

		g.By("Test for delete an existing machine at first and then add a new one")
		g.By("check the platform is supported or not")
		supportedList := []string{"aws", "gcp", "azure", "vsphere", "nutanix"}
		support := in(iaasPlatform, supportedList)
		if support != true {
			g.Skip("The platform is not supported now, skip the cases!!")
		}

		var (
			mMachineop          = ""
			machineStatusOutput = ""
		)

		g.By("Make sure all the etcd pods are running")
		defer o.Expect(checkEtcdPodStatus(oc)).To(o.BeTrue())
		podAllRunning := checkEtcdPodStatus(oc)
		if podAllRunning != true {
			g.Skip("The ectd pods are not running")
		}
		g.By("Get all the master node name & count")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		masterNodeCount := len(masterNodeList)

		g.By("Get master machine name list")
		output, errMachineConfig := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(errMachineConfig).NotTo(o.HaveOccurred())
		masterMachineNameList := strings.Fields(output)

		g.By("At first delete machine of the master node without adding new one")
		errMachineDelete := oc.AsAdmin().Run("delete").Args("-n", "openshift-machine-api", "--wait=false", "machine", masterMachineNameList[0]).Execute()
		o.Expect(errMachineDelete).NotTo(o.HaveOccurred())

		g.By("Verify that the machine is getting deleted and new machine is automatically created")
		waitforDesiredMachineCount(oc, masterNodeCount+1)

		errWait := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			mMachineopraw, errMachineConfig := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o=jsonpath={.items[*].metadata.name}").Output()
			if errMachineConfig != nil {
				e2e.Logf("Failed to get machine name: %s. Trying again", errMachineConfig)
				return false, nil
			} else {
				mMachineop = mMachineopraw
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Failed to get master machine names")
		mMachineNameList := strings.Fields(mMachineop)

		errSt := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			machineStatusraw, errStatus := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o", "jsonpath={.items[*].status.phase}").Output()
			if errStatus != nil {
				e2e.Logf("Failed to get machine status: %s. Trying again", errStatus)
				return false, nil
			} else {
				machineStatusOutput = machineStatusraw
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errSt, "Failed to get master machine status")
		mMachineStatus := strings.Fields(machineStatusOutput)

		e2e.Logf("masterMachineStatus after deletion is %v", mMachineStatus)
		o.Expect(in("Deleting", mMachineStatus)).To(o.Equal(true))

		newMasterMachine := getNewMastermachine(mMachineStatus, mMachineNameList, "Provision")
		g.By("Verify that the new machine is in running state.")
		waitMachineStatusRunning(oc, newMasterMachine)

		g.By("Verify that the old machine is deleted. The master machine count is same as initial one.")
		waitforDesiredMachineCount(oc, masterNodeCount)

	})

	// author: skundu@redhat.com
	g.It("Longduration-Author:skundu-NonPreRelease-Critical-59377-etcd-operator should not scale-down when all members are healthy. [Disruptive]", func() {
		g.By("etcd-operator should not scale-down when all members are healthy")
		g.By("check the platform is supported or not")
		supportedList := []string{"aws", "gcp", "azure", "vsphere", "nutanix"}
		support := in(iaasPlatform, supportedList)
		if support != true {
			g.Skip("The platform is not supported now, skip the cases!!")
		}

		var (
			mMachineop          = ""
			machineStatusOutput = ""
		)

		g.By("Make sure all the etcd pods are running")
		defer o.Expect(checkEtcdPodStatus(oc)).To(o.BeTrue())
		podAllRunning := checkEtcdPodStatus(oc)
		if podAllRunning != true {
			g.Skip("The ectd pods are not running")
		}
		g.By("Get all the master node name & count")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		masterNodeCount := len(masterNodeList)
		e2e.Logf("masterNodeCount is %v", masterNodeCount)

		g.By("Get master machine name list")
		output, errMachineConfig := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(errMachineConfig).NotTo(o.HaveOccurred())
		masterMachineNameList := strings.Fields(output)

		e2e.Logf("masterMachineNameList is %v", masterMachineNameList)
		g.By("Delete the CR")
		_, err := oc.AsAdmin().Run("delete").Args("-n", "openshift-machine-api", "controlplanemachineset.machine.openshift.io", "cluster").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForDesiredStateOfCR(oc, "Inactive")

		g.By("delete machine of the master node")
		errMachineDelete := oc.AsAdmin().Run("delete").Args("-n", "openshift-machine-api", "--wait=false", "machine", masterMachineNameList[0]).Execute()
		o.Expect(errMachineDelete).NotTo(o.HaveOccurred())

		errWait := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			machineStatusOutputraw, errStatus := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o", "jsonpath={.items[*].status.phase}").Output()
			if errStatus != nil {
				e2e.Logf("Failed to get master machine name: %s. Trying again", errStatus)
				return false, nil
			} else {
				machineStatusOutput = machineStatusOutputraw
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Failed to get master machine names")
		masterMachineStatus := strings.Fields(machineStatusOutput)
		e2e.Logf("masterMachineStatus after deletion is %v", masterMachineStatus)
		waitMachineDesiredStatus(oc, masterMachineNameList[0], "Deleting")

		g.By("enable the control plane machineset")
		errW := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			patch := `[{"op": "replace", "path": "/spec/state", "value": "Active"}]`
			patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", "openshift-machine-api", "controlplanemachineset.machine.openshift.io", "cluster", "--type=json", "-p", patch).Execute()
			if patchErr != nil {
				e2e.Logf("unable to apply patch the machineset, error: %s. Trying again ...", patchErr)
				return false, nil
			} else {
				e2e.Logf("successfully patched the machineset.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errW, "unable to enable the comtrol plane machineset.")
		waitForDesiredStateOfCR(oc, "Active")
		errSt := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			machineStatusraw, errStatus := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o", "jsonpath={.items[*].status.phase}").Output()
			if errStatus != nil {
				e2e.Logf("Failed to get machine status: %s. Trying again", errStatus)
				return false, nil
			}
			if match, _ := regexp.MatchString("Provision", machineStatusraw); match {
				e2e.Logf("machine status Provision showed up")
				machineStatusOutput = machineStatusraw
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errSt, "Failed to get master machine status")
		mMachineStatus := strings.Fields(machineStatusOutput)

		errWt := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			mMachineopraw, errMachineConfig := oc.AsAdmin().Run("get").Args(exutil.MapiMachine, "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=master", "-o=jsonpath={.items[*].metadata.name}").Output()
			if errMachineConfig != nil {
				e2e.Logf("Failed to get machine name: %s. Trying again", errMachineConfig)
				return false, nil
			} else {
				mMachineop = mMachineopraw
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWt, "Failed to get master machine names")
		mMachineNameList := strings.Fields(mMachineop)

		e2e.Logf("masterMachineStatus after enabling the CPMS is %v", mMachineStatus)
		newMasterMachine := getNewMastermachine(mMachineStatus, mMachineNameList, "Provision")
		g.By("Verify that the new machine is in running state.")
		waitMachineStatusRunning(oc, newMasterMachine)

		g.By("Verify that the old machine is deleted. The master machine count is same as initial one.")
		waitforDesiredMachineCount(oc, masterNodeCount)
	})

	// author: skundu@redhat.com
	g.It("Longduration-Author:skundu-NonPreRelease-Critical-53767-cluster-backup.sh exits with a non-zero code in case Etcd backup fails. [Disruptive]", func() {
		g.By("Test for case OCP-53767 - cluster-backup.sh exits with a non-zero code in case Etcd backup fails.")
		g.Skip("Skipping this test temporarily because it is redundant with OCP-42183")

		g.By("select all the master node")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")

		g.By("Check etcd oprator status")
		checkOperator(oc, "etcd")
		g.By("Check kube-apiserver oprator status")
		checkOperator(oc, "kube-apiserver")

		g.By("Run the backup")
		masterN, etcdDb := runDRBackup(oc, strings.Fields(masterNodeList[0]))

		defer func() {
			_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterN, []string{"-q"}, "rm", "-rf", "/home/core/assets/backup")
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Corrupt the etcd db file ")
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterN, []string{"-q"}, "truncate", "-s", "126k", etcdDb)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Run the restore")
		output, _ := exutil.DebugNodeWithOptionsAndChroot(oc, masterN, []string{"-q"}, "/usr/local/bin/cluster-restore.sh", "/home/core/assets/backup")
		o.Expect(strings.Contains(output, "Backup appears corrupted. Aborting!")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "non-zero exit code")).To(o.BeTrue())
	})

	// author: skundu@redhat.com
	g.It("Longduration-NonPreRelease-Author:skundu-Critical-68658-CEO prevents member deletion during revision rollout. [Disruptive]", func() {
		g.By("Test for case OCP-68658 - CEO prevents member deletion during revision rollout.")
		g.Skip("Skipping this test temporarily until the product bug OCPBUGS-17199 gets fixed.")

		var (
			mhcName      = "control-plane-health-68658"
			nameSpace    = "openshift-machine-api"
			maxUnhealthy = 1
		)

		g.By("1. Create MachineHealthCheck")
		baseDir := exutil.FixturePath("testdata", "etcd")
		pvcTemplate := filepath.Join(baseDir, "dr_mhc.yaml")
		params := []string{"-f", pvcTemplate, "-p", "NAME=" + mhcName, "NAMESPACE=" + nameSpace, "MAXUNHEALTHY=" + strconv.Itoa(maxUnhealthy)}
		defer oc.AsAdmin().Run("delete").Args("mhc", mhcName, "-n", nameSpace).Execute()
		exutil.CreateNsResourceFromTemplate(oc, nameSpace, params...)

		g.By("2. Verify MachineHealthCheck")
		mhcMaxUnhealthy, errStatus := oc.AsAdmin().Run("get").Args("-n", nameSpace, "mhc", "-o", "jsonpath={.spec.maxUnhealthy}").Output()
		o.Expect(errStatus).NotTo(o.HaveOccurred())
		if mhcMaxUnhealthy != strconv.Itoa(maxUnhealthy) {
			e2e.Failf("Failed to verify mhc newly created MHC %v", mhcName)
		}

		g.By("3. Get all the master nodes")
		masterNodeList := getNodeListByLabel(oc, "node-role.kubernetes.io/master=")
		masterNodeCount := len(masterNodeList)
		g.By("4. Stop the kubelet service on one of the master nodes")
		_, _ = exutil.DebugNodeWithOptionsAndChroot(oc, masterNodeList[0], []string{"-q"}, "systemctl", "stop", "kubelet")
		g.By("5. Ensure etcd oprator goes into degraded state and eventually recovers from it.")
		waitForOperatorRestart(oc, "etcd")
		waitforDesiredMachineCount(oc, masterNodeCount)
		g.By("6. Check kube-apiserver oprator status")
		checkOperator(oc, "kube-apiserver")
	})

})
