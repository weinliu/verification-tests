package mco

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	bootstrap "github.com/openshift/openshift-tests-private/test/extended/util/bootstrap"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = g.Describe("[sig-mco] MCO", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("mco", exutil.KubeConfigPath())

	g.JustBeforeEach(func() {
		preChecks(oc)
	})

	g.It("NonHyperShiftHOST-Author:rioliu-Critical-42347-health check for machine-config-operator [Serial]", func() {
		g.By("checking mco status")
		co := NewResource(oc.AsAdmin(), "co", "machine-config")
		coStatus := co.GetOrFail(`{range .status.conditions[*]}{.type}{.status}{"\n"}{end}`)
		logger.Infof(coStatus)
		o.Expect(coStatus).Should(
			o.And(
				o.ContainSubstring("ProgressingFalse"),
				o.ContainSubstring("UpgradeableTrue"),
				o.ContainSubstring("DegradedFalse"),
				o.ContainSubstring("AvailableTrue"),
			))

		logger.Infof("machine config operator is healthy")

		g.By("checking mco pod status")
		pod := NewNamespacedResource(oc.AsAdmin(), "pods", MachineConfigNamespace, "")
		podStatus := pod.GetOrFail(`{.items[*].status.conditions[?(@.type=="Ready")].status}`)
		logger.Infof(podStatus)
		o.Expect(podStatus).ShouldNot(o.ContainSubstring("False"))
		logger.Infof("mco pods are healthy")

		g.By("checking mcp status")
		mcp := NewResource(oc.AsAdmin(), "mcp", "")
		mcpStatus := mcp.GetOrFail(`{.items[*].status.conditions[?(@.type=="Degraded")].status}`)
		logger.Infof(mcpStatus)
		o.Expect(mcpStatus).ShouldNot(o.ContainSubstring("True"))
		logger.Infof("mcps are not degraded")

	})

	g.It("Author:rioliu-Longduration-NonPreRelease-Critical-42361-add chrony systemd config [Disruptive]", func() {
		g.By("create new mc to apply chrony config on worker nodes")
		workerNode := NewNodeList(oc).GetAllCoreOsWokerNodesOrFail()[0]
		mcName := "change-workers-chrony-configuration"
		mcTemplate := "change-workers-chrony-configuration.yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)

		defer mc.delete()

		_, _ = workerNode.GetDate() // For debugging purposes. It will print the date in the logs.
		o.Expect(workerNode.IgnoreEventsBeforeNow()).NotTo(o.HaveOccurred(),
			"Error getting the latest event in node %s", workerNode.GetName())

		mc.create()

		g.By("verify that drain and reboot events were triggered")
		nodeEvents, eErr := workerNode.GetEvents()
		o.Expect(eErr).ShouldNot(o.HaveOccurred(), "Error getting drain events for node %s", workerNode.GetName())
		o.Expect(nodeEvents).To(HaveEventsSequence("Cordon", "Drain",
			"Reboot", "Uncordon"))

		g.By("get one worker node to verify the config changes")
		stdout, err := workerNode.DebugNodeWithChroot("cat", "/etc/chrony.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		logger.Infof(stdout)
		o.Expect(stdout).Should(
			o.And(
				o.ContainSubstring("pool 0.rhel.pool.ntp.org iburst"),
				o.ContainSubstring("driftfile /var/lib/chrony/drift"),
				o.ContainSubstring("makestep 1.0 3"),
				o.ContainSubstring("rtcsync"),
				o.ContainSubstring("logdir /var/log/chrony"),
			))

	})

	g.It("Author:rioliu-Longduration-NonPreRelease-High-42520-retrieve mc with large size from mcs [Disruptive]", func() {
		g.By("create new mc to add 100+ dummy files to /var/log")
		mcName := "bz1866117-add-dummy-files"
		mcTemplate := "bz1866117-add-dummy-files.yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("get one master node to do mc query")
		masterNode := NewNodeList(oc).GetAllMasterNodesOrFail()[0]
		stdout, err := masterNode.DebugNode("curl", "-w", "'Total: %{time_total}'", "-k", "-s", "-o", "/dev/null", "https://localhost:22623/config/worker")
		o.Expect(err).NotTo(o.HaveOccurred())

		var timecost float64
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if strings.Contains(line, "Total") {
				substr := line[8 : len(line)-1]
				timecost, _ = strconv.ParseFloat(substr, 64)
				break
			}
		}
		logger.Infof("api time cost is: %f", timecost)
		o.Expect(timecost).Should(o.BeNumerically("<", 10.0))
	})

	g.It("Author:mhanss-NonPreRelease-Critical-43048-Critical-43064-create/delete custom machine config pool [Disruptive]", func() {
		g.By("get worker node to change the label")
		nodeList := NewNodeList(oc)
		workerNode := nodeList.GetAllLinuxWorkerNodesOrFail()[0]

		g.By("Add label as infra to the existing node")
		infraLabel := "node-role.kubernetes.io/infra"
		labelOutput, err := workerNode.AddLabel(infraLabel, "")
		defer func() {
			// ignore output, just focus on error handling, if error is occurred, fail this case
			_, deletefailure := workerNode.DeleteLabel(infraLabel)
			o.Expect(deletefailure).NotTo(o.HaveOccurred())
		}()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(labelOutput).Should(o.ContainSubstring(workerNode.name))
		nodeLabel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes/" + workerNode.name).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeLabel).Should(o.ContainSubstring("infra"))

		g.By("Create custom infra mcp")
		mcpName := "infra"
		mcpTemplate := generateTemplateAbsolutePath("custom-machine-config-pool.yaml")
		mcp := NewMachineConfigPool(oc.AsAdmin(), mcpName)
		mcp.template = mcpTemplate
		defer mcp.delete()
		// We need to wait for the label to be delete before removing the MCP. Otherwise the worker pool
		// becomes Degraded.
		defer func() {
			// ignore output, just focus on error handling, if error is occurred, fail this case
			_, deletefailure := workerNode.DeleteLabel(infraLabel)
			o.Expect(deletefailure).NotTo(o.HaveOccurred())
			_ = workerNode.WaitForLabelRemoved(infraLabel)
		}()
		mcp.create()

		g.By("Check MCP status")
		o.Consistently(mcp.pollDegradedMachineCount(), "30s", "10s").Should(o.Equal("0"), "There are degraded nodes in pool")
		o.Eventually(mcp.pollDegradedStatus(), "1m", "20s").Should(o.Equal("False"), "The pool status is 'Degraded'")
		o.Eventually(mcp.pollUpdatedStatus(), "1m", "20s").Should(o.Equal("True"), "The pool is reporting that it is not updated")
		o.Eventually(mcp.pollMachineCount(), "1m", "10s").Should(o.Equal("1"), "The pool should report 1 machine count")
		o.Eventually(mcp.pollReadyMachineCount(), "1m", "10s").Should(o.Equal("1"), "The pool should report 1 machine ready")

		logger.Infof("Custom mcp is created successfully!")

		g.By("Remove custom label from the node")
		unlabeledOutput, err := workerNode.DeleteLabel(infraLabel)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(unlabeledOutput).Should(o.ContainSubstring(workerNode.name))
		o.Expect(workerNode.WaitForLabelRemoved(infraLabel)).Should(o.Succeed(),
			fmt.Sprintf("Label %s has not been removed from node %s", infraLabel, workerNode.GetName()))
		logger.Infof("Label removed")

		g.By("Check custom infra label is removed from the node")
		nodeList.ByLabel(infraLabel)
		infraNodes, err := nodeList.GetAll()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(infraNodes)).Should(o.Equal(0))

		g.By("Verify that the information is updated in MCP")
		o.Eventually(mcp.pollUpdatedStatus(), "5m", "20s").Should(o.Equal("True"), "The pool is reporting that it is not updated")
		o.Eventually(mcp.pollMachineCount(), "5m", "20s").Should(o.Equal("0"), "The pool should report 0 machine count")
		o.Eventually(mcp.pollReadyMachineCount(), "5m", "20s").Should(o.Equal("0"), "The pool should report 0 machine ready")
		o.Consistently(mcp.pollDegradedMachineCount(), "30s", "10s").Should(o.Equal("0"), "There are degraded nodes in pool")
		o.Eventually(mcp.pollDegradedStatus(), "5m", "20s").Should(o.Equal("False"), "The pool status is 'Degraded'")

		g.By("Remove custom infra mcp")
		mcp.delete()

		g.By("Check custom infra mcp is deleted")
		mcpOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp/" + mcpName).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(mcpOut).Should(o.ContainSubstring("NotFound"))
		logger.Infof("Custom mcp is deleted successfully!")
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Critical-42365-add real time kernel argument [Disruptive]", func() {
		exutil.SkipARM64(oc)
		platform := exutil.CheckPlatform(oc)
		if platform == GCPPlatform || platform == AWSPlatform {
			workerNode := skipTestIfOsIsNotCoreOs(oc)
			textToVerify := TextToVerify{
				textToVerifyForMC:   "realtime",
				textToVerifyForNode: "PREEMPT_RT",
				needBash:            true,
			}
			createMcAndVerifyMCValue(oc, "Kernel argument", "change-worker-kernel-argument", workerNode, textToVerify, "uname -a")
		} else {
			g.Skip("AWS or GCP platform is required to execute this test case as currently kernel real time argument is only supported by these platforms!")
		}
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Critical-42364-add selinux kernel argument [Disruptive]", func() {
		workerNode := skipTestIfOsIsNotCoreOs(oc)
		textToVerify := TextToVerify{
			textToVerifyForMC:   "enforcing=0",
			textToVerifyForNode: "enforcing=0",
		}
		createMcAndVerifyMCValue(oc, "Kernel argument", "change-worker-kernel-selinux", workerNode, textToVerify, "cat", "/rootfs/proc/cmdline")
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Critical-42367-add extension to RHCOS [Disruptive]", func() {
		workerNode := skipTestIfOsIsNotCoreOs(oc)
		textToVerify := TextToVerify{
			textToVerifyForMC:   "usbguard",
			textToVerifyForNode: "usbguard",
			needChroot:          true,
		}
		createMcAndVerifyMCValue(oc, "Usb Extension", "change-worker-extension-usbguard", workerNode, textToVerify, "rpm", "-q", "usbguard")
	})
	g.It("Author:sregidor-Longduration-NonPreRelease-Critical-56131-Install all extensions [Disruptive]", func() {
		exutil.SkipARM64(oc)

		var (
			workerNode                   = skipTestIfOsIsNotCoreOs(oc)
			stepText                     = "available extensions"
			mcName                       = "change-worker-all-extensions"
			allExtensions                = []string{"usbguard", "kerberos", "kernel-devel", "sandboxed-containers"}
			expectedRpmInstalledPackages = []string{"usbguard", "krb5-workstation", "libkadm5", "kernel-devel", "kernel-headers",
				"kata-containers"}
			textToVerify = TextToVerify{
				textToVerifyForMC:   "(?s)" + strings.Join(allExtensions, ".*"),
				textToVerifyForNode: "(?s)" + strings.Join(expectedRpmInstalledPackages, ".*"),
				needChroot:          true,
			}

			cmd = append([]string{"rpm", "-q"}, expectedRpmInstalledPackages...)
		)
		createMcAndVerifyMCValue(oc, stepText, mcName, workerNode, textToVerify, cmd...)

		g.By("Verify that extension packages where uninstalled after MC deletion")
		for _, pkg := range expectedRpmInstalledPackages {
			o.Expect(workerNode.RpmIsInstalled(pkg)).To(
				o.BeFalse(),
				"Package %s should be uninstalled when we remove the extensions MC", pkg)
		}

	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Critical-43310-add kernel arguments, kernel type and extension to the RHCOS and RHEL [Disruptive]", func() {
		nodeList := NewNodeList(oc)
		allRhelOs := nodeList.GetAllRhelWokerNodesOrFail()
		allCoreOs := nodeList.GetAllCoreOsWokerNodesOrFail()

		if len(allRhelOs) == 0 || len(allCoreOs) == 0 {
			g.Skip("Both Rhel and CoreOs are required to execute this test case!")
		}

		rhelOs := allRhelOs[0]
		coreOs := allCoreOs[0]

		g.By("Create new MC to add the kernel arguments, kernel type and extension")
		mcName := "change-worker-karg-ktype-extension"
		mcTemplate := mcName + ".yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("Check kernel arguments, kernel type and extension on the created machine config")
		mcOut, err := getMachineConfigDetails(oc, mc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(mcOut).Should(
			o.And(
				o.ContainSubstring("usbguard"),
				o.ContainSubstring("z=10"),
				o.ContainSubstring("realtime")))

		g.By("Check kernel arguments, kernel type and extension on the rhel worker node")
		rhelRpmOut, err := rhelOs.DebugNodeWithChroot("rpm", "-qa")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(rhelRpmOut).Should(o.And(
			o.MatchRegexp(".*kernel-tools-[0-9-.]+el[0-9]+.x86_64.*"),
			o.MatchRegexp(".*kernel-tools-libs-[0-9-.]+el[0-9]+.x86_64.*"),
			o.MatchRegexp(".*kernel-[0-9-.]+el[0-9]+.x86_64.*")))
		rhelUnameOut, err := rhelOs.DebugNodeWithChroot("uname", "-a")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(rhelUnameOut).Should(o.Not(o.ContainSubstring("PREEMPT_RT")))
		rhelCmdlineOut, err := rhelOs.DebugNodeWithChroot("cat", "/proc/cmdline")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(rhelCmdlineOut).Should(o.Not(o.ContainSubstring("z=10")))

		g.By("Check kernel arguments, kernel type and extension on the rhcos worker node")
		coreOsRpmOut, err := coreOs.DebugNodeWithChroot("rpm", "-qa")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(coreOsRpmOut).Should(o.And(
			o.MatchRegexp(".*kernel-rt-kvm-[0-9-.]+rt[0-9.]+el[0-9_]+.x86_64.*"),
			o.MatchRegexp(".*kernel-rt-core-[0-9-.]+rt[0-9.]+el[0-9_]+.x86_64.*"),
			o.MatchRegexp(".*kernel-rt-modules-core-[0-9-.]+rt[0-9.]+el[0-9_]+.x86_64.*"),
			o.MatchRegexp(".*kernel-rt-modules-extra-[0-9-.]+rt[0-9.]+el[0-9_]+.x86_64.*"),
			o.MatchRegexp(".*kernel-rt-modules-[0-9-.]+rt[0-9.]+el[0-9_]+.x86_64.*")))
		coreOsUnameOut, err := coreOs.DebugNodeWithChroot("uname", "-a")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(coreOsUnameOut).Should(o.ContainSubstring("PREEMPT_RT"))
		coreOsCmdlineOut, err := coreOs.DebugNodeWithChroot("cat", "/proc/cmdline")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(coreOsCmdlineOut).Should(o.ContainSubstring("z=10"))
		logger.Infof("Kernel argument, kernel type and extension changes are verified on both rhcos and rhel worker nodes!")
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Critical-42368-add max pods to the kubelet config [Disruptive]", func() {
		g.By("create kubelet config to add 500 max pods")
		kcName := "change-maxpods-kubelet-config"
		kcTemplate := generateTemplateAbsolutePath(kcName + ".yaml")
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		defer func() {
			kc.DeleteOrFail()
			mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			mcp.waitForComplete()
		}()
		kc.create()
		kc.waitUntilSuccess("10s")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()
		logger.Infof("Kubelet config is created successfully!")

		g.By("Check max pods in the created kubelet config")
		kcOut := kc.GetOrFail(`{.spec}`)
		o.Expect(kcOut).Should(o.ContainSubstring(`"maxPods":500`))
		logger.Infof("Max pods are verified in the created kubelet config!")

		g.By("Check kubelet config in the worker node")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		maxPods, err := workerNode.DebugNodeWithChroot("cat", "/etc/kubernetes/kubelet.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(maxPods).Should(o.ContainSubstring("\"maxPods\": 500"))
		logger.Infof("Max pods are verified in the worker node!")
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Critical-42369-add container runtime config [Disruptive]", func() {

		allRhelOs := NewNodeList(oc).GetAllRhelWokerNodesOrFail()
		if len(allRhelOs) > 0 {
			g.Skip("ctrcfg test cannot be executed on rhel node")
		}

		g.By("Create container runtime config")
		crName := "change-ctr-cr-config"
		crTemplate := generateTemplateAbsolutePath(crName + ".yaml")
		cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
		defer func() {
			cr.DeleteOrFail()
			mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			mcp.waitForComplete()
		}()
		cr.create()
		mcp := NewMachineConfigPool(cr.oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()
		logger.Infof("Container runtime config is created successfully!")

		g.By("Check container runtime config values in the created config")
		crOut := cr.GetOrFail(`{.spec}`)
		o.Expect(crOut).Should(
			o.And(
				o.ContainSubstring(`"logLevel":"debug"`),
				o.ContainSubstring(`"logSizeMax":"-1"`),
				o.ContainSubstring(`"pidsLimit":2048`),
				o.ContainSubstring(`"overlaySize":"8G"`)))
		logger.Infof("Container runtime config values are verified in the created config!")

		g.By("Check container runtime config values in the worker node")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		crStorageOut, err := workerNode.DebugNodeWithChroot("head", "-n", "7", "/etc/containers/storage.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(crStorageOut).Should(o.ContainSubstring("size = \"8G\""))
		crConfigOut, err := workerNode.DebugNodeWithChroot("crio", "config")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(crConfigOut).Should(
			o.And(
				o.ContainSubstring("log_level = \"debug\""),
				o.ContainSubstring("pids_limit = 2048")))
		logger.Infof("Container runtime config values are verified in the worker node!")
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Critical-42438-add journald systemd config [Disruptive]", func() {
		g.By("Create journald systemd config")
		encodedConf, err := exec.Command("bash", "-c", "cat "+generateTemplateAbsolutePath("journald.conf")+" | base64 | tr -d '\n'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		conf := string(encodedConf)
		jcName := "change-worker-jrnl-configuration"
		jcTemplate := jcName + ".yaml"
		journaldConf := []string{"CONFIGURATION=" + conf}
		jc := NewMachineConfig(oc.AsAdmin(), jcName, MachineConfigPoolWorker).SetMCOTemplate(jcTemplate)
		jc.parameters = journaldConf
		defer jc.delete()
		jc.create()
		logger.Infof("Journald systemd config is created successfully!")

		g.By("Check journald config value in the created machine config!")
		jcOut, err := getMachineConfigDetails(oc, jc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(jcOut).Should(o.ContainSubstring(conf))
		logger.Infof("Journald config is verified in the created machine config!")

		g.By("Check journald config values in the worker node")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		o.Expect(err).NotTo(o.HaveOccurred())
		journaldConfOut, err := workerNode.DebugNodeWithChroot("cat", "/etc/systemd/journald.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(journaldConfOut).Should(
			o.And(
				o.ContainSubstring("RateLimitInterval=1s"),
				o.ContainSubstring("RateLimitBurst=10000"),
				o.ContainSubstring("Storage=volatile"),
				o.ContainSubstring("Compress=no"),
				o.ContainSubstring("MaxRetentionSec=30s")))
		logger.Infof("Journald config values are verified in the worker node!")
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-High-43405-High-50508-node drain is not needed for mirror config change in container registry. Nodes not tainted. [Disruptive]", func() {
		g.By("Create image content source policy for mirror changes")
		icspName := "repository-mirror"
		icspTemplate := generateTemplateAbsolutePath(icspName + ".yaml")
		icsp := ImageContentSourcePolicy{name: icspName, template: icspTemplate}
		defer icsp.delete(oc)
		icsp.create(oc)

		g.By("Check registry changes in the worker node")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		registryOut, err := workerNode.DebugNodeWithChroot("cat", "/etc/containers/registries.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(registryOut).Should(
			o.And(
				o.ContainSubstring(`pull-from-mirror = "digest-only"`),
				o.ContainSubstring("example.com/example/ubi-minimal"),
				o.ContainSubstring("example.io/example/ubi-minimal")))

		g.By("Check MCD logs to make sure drain is skipped")
		podLogs, err := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "drain")
		o.Expect(err).NotTo(o.HaveOccurred())
		logger.Infof("Pod logs to skip node drain :\n %v", podLogs)
		o.Expect(podLogs).Should(
			o.And(
				o.ContainSubstring("/etc/containers/registries.conf: changes made are safe to skip drain"),
				o.ContainSubstring("Changes do not require drain, skipping")))

		g.By("Check that worker nodes are not tained after applying the MC")
		workerMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		workerNodes, err := workerMcp.GetNodes()
		o.Expect(err).ShouldNot(o.HaveOccurred(), "Error getting nodes linked to the worker pool")
		for _, node := range workerNodes {
			o.Expect(node.IsTainted()).To(o.BeFalse(), "% is tainted. There should be no tainted worker node after applying the configuration.",
				node.GetName())
		}

		g.By("Check that master nodes have only the NoSchedule taint after applying the the MC")
		masterMcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
		masterNodes, err := masterMcp.GetNodes()
		o.Expect(err).ShouldNot(o.HaveOccurred(), "Error getting nodes linked to the master pool")
		expectedTaint := `[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master"}]`
		for _, node := range masterNodes {
			taint, err := node.Get("{.spec.taints}")
			o.Expect(err).ShouldNot(o.HaveOccurred(), "Error getting taints in node %s", node.GetName())
			o.Expect(taint).Should(o.MatchJSON(expectedTaint), "%s is tainted. The only taint allowed in master nodes is NoSchedule.",
				node.GetName())
		}

	})

	g.It("Author:rioliu-NonPreRelease-High-42390-Critical-45318-add machine config without ignition version. Block the MCO upgrade rollout if any of the pools are Degraded [Serial]", func() {
		createMcAndVerifyIgnitionVersion(oc, "empty ign version", "change-worker-ign-version-to-empty", "")
	})

	g.It("Author:mhanss-NonPreRelease-High-43124-add machine config with invalid ignition version [Serial]", func() {
		createMcAndVerifyIgnitionVersion(oc, "invalid ign version", "change-worker-ign-version-to-invalid", "3.9.0")
	})

	g.It("Author:rioliu-NonPreRelease-High-42679-add new ssh authorized keys CoreOs [Serial]", func() {
		workerNode := skipTestIfOsIsNotCoreOs(oc)
		g.By("Create new machine config with new authorized key")
		mcName := TmplAddSSHAuthorizedKeyForWorker
		mcTemplate := mcName + ".yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("Check content of file authorized_keys to verify whether new one is added successfully")
		sshKeyOut, err := workerNode.DebugNodeWithChroot("cat", "/home/core/.ssh/authorized_keys.d/ignition")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sshKeyOut).Should(o.ContainSubstring("mco_test@redhat.com"))
	})

	g.It("Author:sregidor-NonPreRelease-High-46304-add new ssh authorized keys RHEL. OCP<4.10 [Serial]", func() {
		skipTestIfClusterVersion(oc, ">=", "4.10")
		workerNode := skipTestIfOsIsNotRhelOs(oc)

		g.By("Create new machine config with new authorized key")
		mcName := TmplAddSSHAuthorizedKeyForWorker
		mcTemplate := mcName + ".yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("Check content of file authorized_keys to verify whether new one is added successfully")
		sshKeyOut, err := workerNode.DebugNodeWithChroot("cat", "/home/core/.ssh/authorized_keys")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sshKeyOut).Should(o.ContainSubstring("mco_test@redhat.com"))
	})

	g.It("Author:sregidor-NonPreRelease-High-46897-add new ssh authorized keys RHEL. OCP>=4.10 [Serial]", func() {
		skipTestIfClusterVersion(oc, "<", "4.10")
		workerNode := skipTestIfOsIsNotRhelOs(oc)

		g.By("Create new machine config with new authorized key")
		mcName := TmplAddSSHAuthorizedKeyForWorker
		mcTemplate := mcName + ".yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("Check that the logs are reporting correctly that the 'core' user does not exist")
		errorString := "core user does not exist, and creating users is not supported, so ignoring configuration specified for core user"
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon,
			workerNode.GetMachineConfigDaemon(), "'"+errorString+"'")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podLogs).Should(o.ContainSubstring(errorString))

		g.By("Check that the authorized keys have not been created")
		rf := NewRemoteFile(workerNode, "/home/core/.ssh/authorized_keys")
		rferr := rf.Fetch().(*exutil.ExitError)
		// There should be no "/home/core" directory, so the result of trying to read the keys should be a failure
		o.Expect(rferr).To(o.HaveOccurred())
		o.Expect(rferr.StdErr).Should(o.ContainSubstring("No such file or directory"))

	})

	g.It("Author:mhanss-NonPreRelease-Medium-43084-shutdown machine config daemon with SIGTERM [Disruptive]", func() {
		g.By("Create new machine config to add additional ssh key")
		mcName := "add-additional-ssh-authorized-key"
		mcTemplate := mcName + ".yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("Check MCD logs to make sure shutdown machine config daemon with SIGTERM")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		podLogs, err := exutil.WaitAndGetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "SIGTERM")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podLogs).Should(
			o.And(
				o.ContainSubstring("Adding SIGTERM protection"),
				o.ContainSubstring("Removing SIGTERM protection")))

		g.By("Kill MCD process")
		mcdKillLogs, err := workerNode.DebugNodeWithChroot("pgrep", "-f", "machine-config-daemon_")
		o.Expect(err).NotTo(o.HaveOccurred())
		mcpPid := regexp.MustCompile("(?m)^[0-9]+").FindString(mcdKillLogs)
		_, err = workerNode.DebugNodeWithChroot("kill", mcpPid)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check MCD logs to make sure machine config daemon without SIGTERM")
		mcDaemon := workerNode.GetMachineConfigDaemon()

		exutil.AssertPodToBeReady(oc, mcDaemon, MachineConfigNamespace)
		mcdLogs, err := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, mcDaemon, "")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(mcdLogs).ShouldNot(o.ContainSubstring("SIGTERM"))
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-High-42682-change container registry config on ocp 4.6+ [Disruptive]", func() {

		skipTestIfClusterVersion(oc, "<", "4.6")

		registriesConfPath := "/etc/containers/registries.conf"
		newSearchRegistry := "quay.io"

		g.By("Generate new registries.conf information")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		registriesConf := NewRemoteFile(workerNode, registriesConfPath)
		o.Expect(registriesConf.Fetch()).Should(o.Succeed(), "Error getting %s file content", registriesConfPath)

		currentSearch, sErr := registriesConf.GetFilteredTextContent("unqualified-search-registries")
		o.Expect(sErr).ShouldNot(o.HaveOccurred())
		logger.Infof("Initial search configuration: %s", strings.Join(currentSearch, "\n"))

		logger.Infof("Adding %s registry to the initial search configuration", newSearchRegistry)
		currentConfig := registriesConf.GetTextContent()
		// add the "quay.io" registry to the unqualified-search-registries list defined in the registries.conf file
		// this regexp inserts `, "quay.io"` before  `]` in the unqualified-search-registries line.
		regx := regexp.MustCompile(`(unqualified-search-registries.*=.*\[.*)](.*)`)
		newConfig := regx.ReplaceAllString(currentConfig, fmt.Sprintf(`$1, "%s"] $2`, newSearchRegistry))

		g.By("Create new machine config to add quay.io to unqualified-search-registries list")
		mcName := "change-workers-container-reg"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		fileConfig := getURLEncodedFileConfig(registriesConfPath, newConfig, "420")
		errCreate := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(errCreate).NotTo(o.HaveOccurred(), "Error creating MachineConfig %s", mcName)

		g.By("Wait for MCP to be updated")
		mcpWorker := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcpWorker.waitForComplete()

		g.By("Check content of registries file to verify quay.io added to unqualified-search-registries list")
		regOut, errDebug := workerNode.DebugNodeWithChroot("cat", registriesConfPath)
		logger.Infof("File content of registries conf: %v", regOut)
		o.Expect(errDebug).NotTo(o.HaveOccurred(), "Error executing debug command on node %s", workerNode.GetName())
		o.Expect(regOut).Should(o.ContainSubstring(newSearchRegistry),
			"registry %s has not been added to the %s file", newSearchRegistry, registriesConfPath)

		g.By("Check MCD logs to make sure drain is successful and pods are evicted")
		podLogs, errLogs := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "\"evicted\\|drain\\|crio\"")
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error getting logs from node %s", workerNode.GetName())
		logger.Infof("Pod logs for node drain, pods evicted and crio service reload :\n %v", podLogs)
		// get clusterversion
		cv, _, cvErr := exutil.GetClusterVersion(oc)
		o.Expect(cvErr).NotTo(o.HaveOccurred())
		// check node drain is skipped for cluster 4.7+
		if CompareVersions(cv, ">", "4.7") {
			o.Expect(podLogs).Should(
				o.And(
					o.ContainSubstring("/etc/containers/registries.conf: changes made are safe to skip drain"),
					o.ContainSubstring("Changes do not require drain, skipping")))
		} else {
			// check node drain can be triggered for 4.6 & 4.7
			o.Expect(podLogs).Should(
				o.And(
					o.ContainSubstring("Update prepared; beginning drain"),
					o.ContainSubstring("Evicted pod openshift-image-registry/image-registry"),
					o.ContainSubstring("drain complete")))
		}
		// check whether crio.service is reloaded in 4.6+ env
		if CompareVersions(cv, ">", "4.6") {
			logger.Infof("cluster version is > 4.6, need to check crio service is reloaded or not")
			o.Expect(podLogs).Should(o.ContainSubstring("crio config reloaded successfully"))
		}

	})

	g.It("Author:rioliu-Longduration-NonPreRelease-High-42704-disable auto reboot for mco [Disruptive]", func() {
		g.By("pause mcp worker")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		defer mcp.pause(false)
		mcp.pause(true)

		g.By("create new mc")
		mcName := "change-workers-chrony-configuration"
		mcTemplate := "change-workers-chrony-configuration.yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		mc.skipWaitForMcp = true
		defer mc.delete()
		mc.create()

		g.By("compare config name b/w spec.configuration.name and status.configuration.name, they're different")
		specConf, specErr := mcp.getConfigNameOfSpec()
		o.Expect(specErr).NotTo(o.HaveOccurred())
		statusConf, statusErr := mcp.getConfigNameOfStatus()
		o.Expect(statusErr).NotTo(o.HaveOccurred())
		o.Expect(specConf).ShouldNot(o.Equal(statusConf))

		g.By("check mcp status condition, expected: UPDATED=False && UPDATING=False")
		var updated, updating string
		pollerr := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
			stdouta, erra := mcp.Get(`{.status.conditions[?(@.type=="Updated")].status}`)
			stdoutb, errb := mcp.Get(`{.status.conditions[?(@.type=="Updating")].status}`)
			updated = strings.Trim(stdouta, "'")
			updating = strings.Trim(stdoutb, "'")
			if erra != nil || errb != nil {
				logger.Errorf("error occurred %v%v", erra, errb)
				return false, nil
			}
			if updated != "" && updating != "" {
				logger.Infof("updated: %v, updating: %v", updated, updating)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollerr, "polling status conditions of mcp: [Updated,Updating] failed")
		o.Expect(updated).Should(o.Equal("False"))
		o.Expect(updating).Should(o.Equal("False"))

		g.By("unpause mcp worker, then verify whether the new mc can be applied on mcp/worker")
		mcp.pause(false)
		mcp.waitForComplete()
	})

	g.It("Author:rioliu-NonPreRelease-High-42681-rotate kubernetes certificate authority [Disruptive]", func() {
		g.By("patch secret to trigger CA rotation")
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "-p", `{"metadata": {"annotations": {"auth.openshift.io/certificate-not-after": null}}}`, "kube-apiserver-to-kubelet-signer", "-n", "openshift-kube-apiserver-operator").Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		g.By("monitor update progress of mcp master and worker, new configs should be applied successfully")
		mcpMaster := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
		mcpWorker := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcpMaster.waitForComplete()
		mcpWorker.waitForComplete()

		g.By("check new generated rendered configs for kuberlet CA")
		renderedConfs, renderedErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("mc", "--sort-by=metadata.creationTimestamp", "-o", "jsonpath='{.items[-2:].metadata.name}'").Output()
		o.Expect(renderedErr).NotTo(o.HaveOccurred())
		o.Expect(renderedConfs).NotTo(o.BeEmpty())
		slices := strings.Split(strings.Trim(renderedConfs, "'"), " ")
		var renderedMasterConf, renderedWorkerConf string
		for _, conf := range slices {
			if strings.Contains(conf, MachineConfigPoolMaster) {
				renderedMasterConf = conf
			} else if strings.Contains(conf, MachineConfigPoolWorker) {
				renderedWorkerConf = conf
			}
		}
		logger.Infof("new rendered config generated for master: %s", renderedMasterConf)
		logger.Infof("new rendered config generated for worker: %s", renderedWorkerConf)

		g.By("check logs of machine-config-daemon on master-n-worker nodes, make sure CA change is detected, drain and reboot are skipped")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		masterNode := NewNodeList(oc).GetAllMasterNodesOrFail()[0]

		commonExpectedStrings := []string{"File diff: detected change to /etc/kubernetes/kubelet-ca.crt", "Changes do not require drain, skipping"}
		expectedStringsForMaster := append(commonExpectedStrings, "Node has Desired Config "+renderedMasterConf+", skipping reboot")
		expectedStringsForWorker := append(commonExpectedStrings, "Node has Desired Config "+renderedWorkerConf+", skipping reboot")
		masterMcdLogs, masterMcdLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, masterNode.GetMachineConfigDaemon(), "")
		o.Expect(masterMcdLogErr).NotTo(o.HaveOccurred())
		workerMcdLogs, workerMcdLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "")
		o.Expect(workerMcdLogErr).NotTo(o.HaveOccurred())
		foundOnMaster := containsMultipleStrings(masterMcdLogs, expectedStringsForMaster)
		o.Expect(foundOnMaster).Should(o.BeTrue())
		logger.Infof("mcd log on master node %s contains expected strings: %v", masterNode.name, expectedStringsForMaster)
		foundOnWorker := containsMultipleStrings(workerMcdLogs, expectedStringsForWorker)
		o.Expect(foundOnWorker).Should(o.BeTrue())
		logger.Infof("mcd log on worker node %s contains expected strings: %v", workerNode.name, expectedStringsForWorker)
	})

	g.It("Author:rioliu-NonPreRelease-High-43085-check mcd crash-loop-back-off error in log [Serial]", func() {
		g.By("get master and worker nodes")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		masterNode := NewNodeList(oc).GetAllMasterNodesOrFail()[0]
		logger.Infof("master node %s", masterNode)
		logger.Infof("worker node %s", workerNode)

		g.By("check error messages in mcd logs for both master and worker nodes")
		expectedStrings := []string{"unable to update node", "cannot apply annotation for SSH access due to"}
		masterMcdLogs, masterMcdLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, masterNode.GetMachineConfigDaemon(), "")
		o.Expect(masterMcdLogErr).NotTo(o.HaveOccurred())
		workerMcdLogs, workerMcdLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "")
		o.Expect(workerMcdLogErr).NotTo(o.HaveOccurred())
		foundOnMaster := containsMultipleStrings(masterMcdLogs, expectedStrings)
		o.Expect(foundOnMaster).Should(o.BeFalse())
		logger.Infof("mcd log on master node %s does not contain error messages: %v", masterNode.name, expectedStrings)
		foundOnWorker := containsMultipleStrings(workerMcdLogs, expectedStrings)
		o.Expect(foundOnWorker).Should(o.BeFalse())
		logger.Infof("mcd log on worker node %s does not contain error messages: %v", workerNode.name, expectedStrings)
	})

	g.It("Author:mhanss-Longduration-NonPreRelease-Medium-43245-bump initial drain sleeps down to 1min [Disruptive]", func() {
		g.By("Start machine-config-controller logs capture")
		mcc := NewController(oc.AsAdmin())
		ignoreMccLogErr := mcc.IgnoreLogsBeforeNow()
		o.Expect(ignoreMccLogErr).NotTo(o.HaveOccurred(), "Ignore mcc log failed")

		g.By("Create a pod disruption budget to set minAvailable to 1")
		oc.SetupProject()
		nsName := oc.Namespace()
		pdbName := "dont-evict-43245"
		pdbTemplate := generateTemplateAbsolutePath("pod-disruption-budget.yaml")
		pdb := PodDisruptionBudget{name: pdbName, namespace: nsName, template: pdbTemplate}
		defer pdb.delete(oc)
		pdb.create(oc)

		g.By("Create new pod for pod disruption budget")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		hostname, err := workerNode.GetNodeHostname()
		o.Expect(err).NotTo(o.HaveOccurred())
		podName := "dont-evict-43245"
		podTemplate := generateTemplateAbsolutePath("create-pod.yaml")
		pod := exutil.Pod{Name: podName, Namespace: nsName, Template: podTemplate, Parameters: []string{"HOSTNAME=" + hostname}}
		defer func() { o.Expect(pod.Delete(oc)).NotTo(o.HaveOccurred()) }()
		pod.Create(oc)

		g.By("Create new mc to add new file on the node and trigger node drain")
		mcName := "test-file"
		mcTemplate := "add-mc-to-trigger-node-drain.yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		mc.skipWaitForMcp = true
		defer mc.delete()
		defer func() { o.Expect(pod.Delete(oc)).NotTo(o.HaveOccurred()) }()
		mc.create()

		g.By("Wait until node is cordoned")
		o.Eventually(workerNode.Poll(`{.spec.taints[?(@.effect=="NoSchedule")].effect}`),
			"20m", "1m").Should(o.Equal("NoSchedule"), fmt.Sprintf("Node %s was not cordoned", workerNode.name))

		g.By("Check MCC logs to see the early sleep interval b/w failed drains")
		var podLogs string
		// Wait until trying drain for 6 times
		waitErr := wait.Poll(1*time.Minute, 15*time.Minute, func() (bool, error) {
			logs, _ := mcc.GetFilteredLogsAsList(workerNode.GetName() + ".*Drain failed")
			if len(logs) > 5 {
				// Get only 6 lines to avoid flooding the test logs, ignore the rest if any.
				podLogs = strings.Join(logs[0:6], "\n")
				return true, nil
			}

			return false, nil
		})
		logger.Infof("Drain log lines for node %s:\n %s", workerNode.GetName(), podLogs)
		o.Expect(waitErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Cannot get 'Drain failed' log lines from controller for node %s", workerNode.GetName()))
		timestamps := filterTimestampFromLogs(podLogs, 6)
		logger.Infof("Timestamps %s", timestamps)
		// First 5 retries should be queued every 1 minute. We check 1 min < time < 2.7 min
		o.Expect(getTimeDifferenceInMinute(timestamps[0], timestamps[1])).Should(o.BeNumerically("<=", 2.7))
		o.Expect(getTimeDifferenceInMinute(timestamps[0], timestamps[1])).Should(o.BeNumerically(">=", 1))
		o.Expect(getTimeDifferenceInMinute(timestamps[3], timestamps[4])).Should(o.BeNumerically("<=", 2.7))
		o.Expect(getTimeDifferenceInMinute(timestamps[3], timestamps[4])).Should(o.BeNumerically(">=", 1))

		g.By("Check MCC logs to see the increase in the sleep interval b/w failed drains")
		lWaitErr := wait.Poll(1*time.Minute, 15*time.Minute, func() (bool, error) {
			logs, _ := mcc.GetFilteredLogsAsList(workerNode.GetName() + ".*Drain has been failing for more than 10 minutes. Waiting 5 minutes")
			if len(logs) > 1 {
				// Get only 2 lines to avoid flooding the test logs, ignore the rest if any.
				podLogs = strings.Join(logs[0:2], "\n")
				return true, nil
			}

			return false, nil
		})
		logger.Infof("Long wait drain log lines for node %s:\n %s", workerNode.GetName(), podLogs)
		o.Expect(lWaitErr).NotTo(o.HaveOccurred(),
			fmt.Sprintf("Cannot get 'Drain has been failing for more than 10 minutes. Waiting 5 minutes' log lines from controller for node %s",
				workerNode.GetName()))
		// Following developers' advice we dont check the time spam between long wait log lines. Read:
		// https://github.com/openshift/machine-config-operator/pull/3178
		// https://bugzilla.redhat.com/show_bug.cgi?id=2092442
	})

	g.It("Author:rioliu-NonPreRelease-High-43278-security fix for unsafe cipher [Serial]", func() {
		g.By("check go version >= 1.15")
		_, clusterVersion, cvErr := exutil.GetClusterVersion(oc)
		o.Expect(cvErr).NotTo(o.HaveOccurred())
		o.Expect(clusterVersion).NotTo(o.BeEmpty())
		logger.Infof("cluster version is %s", clusterVersion)
		commitID, commitErr := getCommitID(oc, "machine-config", clusterVersion)
		o.Expect(commitErr).NotTo(o.HaveOccurred())
		o.Expect(commitID).NotTo(o.BeEmpty())
		logger.Infof("machine config commit id is %s", commitID)
		goVersion, verErr := getGoVersion("machine-config-operator", commitID)
		o.Expect(verErr).NotTo(o.HaveOccurred())
		logger.Infof("go version is: %f", goVersion)
		o.Expect(goVersion).Should(o.BeNumerically(">", 1.15))

		g.By("verify TLS protocol version is 1.3")
		masterNode := NewNodeList(oc).GetAllMasterNodesOrFail()[0]
		sslOutput, sslErr := masterNode.DebugNodeWithChroot("bash", "-c", "openssl s_client -connect localhost:6443 2>&1|grep -A3 SSL-Session")
		logger.Infof("ssl protocol version is:\n %s", sslOutput)
		o.Expect(sslErr).NotTo(o.HaveOccurred())
		o.Expect(sslOutput).Should(o.ContainSubstring("TLSv1.3"))

		g.By("verify whether the unsafe cipher is disabled")
		cipherOutput, cipherErr := masterNode.DebugNodeWithOptions([]string{"--image=quay.io/openshifttest/testssl@sha256:ad6fb8002cb9cfce3ddc8829fd6e7e0d997aeb1faf972650f3e5d7603f90c6ef", "-n", MachineConfigNamespace}, "testssl.sh", "--quiet", "--sweet32", "localhost:6443")
		logger.Infof("test ssh script output:\n %s", cipherOutput)
		o.Expect(cipherErr).NotTo(o.HaveOccurred())
		o.Expect(cipherOutput).Should(o.ContainSubstring("not vulnerable (OK)"))
	})

	g.It("Author:sregidor-NonPreRelease-High-43151-add node label to service monitor [Serial]", func() {
		g.By("Get current mcd_ metrics from machine-config-daemon service")

		svcMCD := NewNamespacedResource(oc.AsAdmin(), "service", MachineConfigNamespace, MachineConfigDaemon)
		clusterIP, ipErr := WrapWithBracketsIfIpv6(svcMCD.GetOrFail("{.spec.clusterIP}"))
		o.Expect(ipErr).ShouldNot(o.HaveOccurred(), "No valid IP")
		port := svcMCD.GetOrFail("{.spec.ports[?(@.name==\"metrics\")].port}")

		token := getSATokenFromContainer(oc, "prometheus-k8s-0", "openshift-monitoring", "prometheus")

		statsCmd := fmt.Sprintf("curl -s -k  -H 'Authorization: Bearer %s' https://%s:%s/metrics | grep 'mcd_' | grep -v '#'", token, clusterIP, port)
		logger.Infof("stats output:\n %s", statsCmd)
		statsOut, err := exutil.RemoteShPod(oc, "openshift-monitoring", "prometheus-k8s-0", "sh", "-c", statsCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("mcd_host_os_and_version"))
		o.Expect(statsOut).Should(o.ContainSubstring("mcd_kubelet_state"))
		o.Expect(statsOut).Should(o.ContainSubstring("mcd_pivot_errors_total"))
		o.Expect(statsOut).Should(o.ContainSubstring("mcd_reboots_failed_total"))
		o.Expect(statsOut).Should(o.ContainSubstring("mcd_state"))
		o.Expect(statsOut).Should(o.ContainSubstring("mcd_update_state"))
		o.Expect(statsOut).Should(o.ContainSubstring("mcd_update_state"))

		g.By("Check relabeling section in machine-config-daemon")
		sourceLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("servicemonitor/machine-config-daemon", "-n", MachineConfigNamespace,
			"-o", "jsonpath='{.spec.endpoints[*].relabelings[*].sourceLabels}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceLabels).Should(o.ContainSubstring("__meta_kubernetes_pod_node_name"))

		g.By("Check node label in mcd_state metrics")
		stateQuery := getPrometheusQueryResults(oc, "mcd_state")
		logger.Infof("metrics:\n %s", stateQuery)
		firstMasterNode := NewNodeList(oc).GetAllMasterNodesOrFail()[0]
		firstWorkerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		o.Expect(stateQuery).Should(o.ContainSubstring(`"node":"` + firstMasterNode.name + `"`))
		o.Expect(stateQuery).Should(o.ContainSubstring(`"node":"` + firstWorkerNode.name + `"`))
	})

	g.It("Author:sregidor-NonPreRelease-High-43726-Azure ControllerConfig Infrastructure does not match cluster Infrastructure resource [Serial]", func() {
		g.By("Get machine-config-controller platform status.")
		mccPlatformStatus := NewResource(oc.AsAdmin(), "controllerconfig", "machine-config-controller").GetOrFail("{.spec.infra.status.platformStatus}")
		logger.Infof("test mccPlatformStatus:\n %s", mccPlatformStatus)

		if exutil.CheckPlatform(oc) == AzurePlatform {
			g.By("check cloudName field.")

			var jsonMccPlatformStatus map[string]interface{}
			errparseinfra := json.Unmarshal([]byte(mccPlatformStatus), &jsonMccPlatformStatus)
			o.Expect(errparseinfra).NotTo(o.HaveOccurred())
			o.Expect(jsonMccPlatformStatus).Should(o.HaveKey("azure"))

			azure := jsonMccPlatformStatus["azure"].(map[string]interface{})
			o.Expect(azure).Should(o.HaveKey("cloudName"))
		}

		g.By("Get infrastructure platform status.")
		infraPlatformStatus := NewResource(oc.AsAdmin(), "infrastructures", "cluster").GetOrFail("{.status.platformStatus}")
		logger.Infof("infraPlatformStatus:\n %s", infraPlatformStatus)

		g.By("Check same status in infra and machine-config-controller.")
		o.Expect(mccPlatformStatus).To(o.Equal(infraPlatformStatus))
	})

	g.It("Author:mhanss-NonPreRelease-High-42680-change pull secret in the openshift-config namespace [Serial]", func() {
		g.By("Add a dummy credential in pull secret")
		secretFile, err := getPullSecret(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		newSecretFile := generateTmpFile(oc, "pull-secret.dockerconfigjson")
		_, copyErr := exec.Command("bash", "-c", "cp "+secretFile+" "+newSecretFile).Output()
		o.Expect(copyErr).NotTo(o.HaveOccurred())
		newPullSecret, err := oc.AsAdmin().WithoutNamespace().Run("registry").Args("login", `--registry="quay.io"`, `--auth-basic="mhans-redhat:redhat123"`, "--to="+newSecretFile, "--skip-check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(newPullSecret).Should(o.Equal(`Saved credentials for "quay.io"`))
		setData, err := setDataForPullSecret(oc, newSecretFile)
		defer func() {
			_, err := setDataForPullSecret(oc, secretFile)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(setData).Should(o.Equal("secret/pull-secret data updated"))

		g.By("Wait for configuration to be applied in master and worker pools")
		mcpWorker := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcpMaster := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
		mcpWorker.waitForComplete()
		mcpMaster.waitForComplete()

		g.By("Check new generated rendered configs for newly added pull secret")
		renderedConfs, renderedErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("mc", "--sort-by=metadata.creationTimestamp", "-o", "jsonpath='{.items[-2:].metadata.name}'").Output()
		o.Expect(renderedErr).NotTo(o.HaveOccurred())
		o.Expect(renderedConfs).NotTo(o.BeEmpty())
		slices := strings.Split(strings.Trim(renderedConfs, "'"), " ")
		var renderedMasterConf, renderedWorkerConf string
		for _, conf := range slices {
			if strings.Contains(conf, MachineConfigPoolMaster) {
				renderedMasterConf = conf
			} else if strings.Contains(conf, MachineConfigPoolWorker) {
				renderedWorkerConf = conf
			}
		}
		logger.Infof("New rendered config generated for master: %s", renderedMasterConf)
		logger.Infof("New rendered config generated for worker: %s", renderedWorkerConf)

		g.By("Check logs of machine-config-daemon on master-n-worker nodes, make sure pull secret changes are detected, drain and reboot are skipped")
		masterNode := NewNodeList(oc).GetAllMasterNodesOrFail()[0]
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		commonExpectedStrings := []string{"File diff: detected change to /var/lib/kubelet/config.json", "Changes do not require drain, skipping"}
		expectedStringsForMaster := append(commonExpectedStrings, "Node has Desired Config "+renderedMasterConf+", skipping reboot")
		expectedStringsForWorker := append(commonExpectedStrings, "Node has Desired Config "+renderedWorkerConf+", skipping reboot")
		masterMcdLogs, masterMcdLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, masterNode.GetMachineConfigDaemon(), "")
		o.Expect(masterMcdLogErr).NotTo(o.HaveOccurred())
		workerMcdLogs, workerMcdLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "")
		o.Expect(workerMcdLogErr).NotTo(o.HaveOccurred())
		foundOnMaster := containsMultipleStrings(masterMcdLogs, expectedStringsForMaster)
		o.Expect(foundOnMaster).Should(o.BeTrue())
		logger.Infof("MCD log on master node %s contains expected strings: %v", masterNode.name, expectedStringsForMaster)
		foundOnWorker := containsMultipleStrings(workerMcdLogs, expectedStringsForWorker)
		o.Expect(foundOnWorker).Should(o.BeTrue())
		logger.Infof("MCD log on worker node %s contains expected strings: %v", workerNode.name, expectedStringsForWorker)
	})

	g.It("Author:sregidor-NonPreRelease-High-45239-KubeletConfig has a limit of 10 per cluster [Disruptive]", func() {
		kcsLimit := 10

		g.By("Pause mcp worker")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		defer mcp.pause(false)
		mcp.pause(true)

		g.By("Calculate number of existing KubeletConfigs")
		kcList := NewKubeletConfigList(oc.AsAdmin())
		kcs, kclErr := kcList.GetAll()
		o.Expect(kclErr).ShouldNot(o.HaveOccurred(), "Error getting existing KubeletConfig resources")
		existingKcs := len(kcs)
		logger.Infof("%d existing KubeletConfigs. We need to create %d KubeletConfigs to reach the %d configs limit",
			existingKcs, kcsLimit-existingKcs, kcsLimit)

		g.By(fmt.Sprintf("Create %d kubelet config to reach the limit", kcsLimit-existingKcs))
		createdKcs := []ResourceInterface{}
		kcTemplate := generateTemplateAbsolutePath("change-maxpods-kubelet-config.yaml")
		for n := existingKcs + 1; n <= kcsLimit; n++ {
			kcName := fmt.Sprintf("change-maxpods-kubelet-config-%d", n)
			kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
			defer kc.DeleteOrFail()
			kc.create()
			createdKcs = append(createdKcs, kc)
			logger.Infof("Created:\n %s", kcName)
		}

		g.By("Created kubeletconfigs must be successful")
		for _, kcItem := range createdKcs {
			kcItem.(*KubeletConfig).waitUntilSuccess("15s")
		}

		g.By(fmt.Sprintf("Check that %d machine configs were created", kcsLimit-existingKcs))
		renderedKcConfigsSuffix := "worker-generated-kubelet"
		verifyRenderedMcs(oc, renderedKcConfigsSuffix, createdKcs)

		g.By(fmt.Sprintf("Create a new Kubeletconfig. The %dth one", kcsLimit+1))
		kcName := fmt.Sprintf("change-maxpods-kubelet-config-%d", kcsLimit+1)
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		defer kc.DeleteOrFail()
		kc.create()

		g.By(fmt.Sprintf("Created kubeletconfigs over the limit must report a failure regarding the %d configs limit", kcsLimit))
		expectedMsg := fmt.Sprintf("could not get kubelet config key: max number of supported kubelet config (%d) has been reached. Please delete old kubelet configs before retrying", kcsLimit)
		kc.waitUntilFailure(expectedMsg, "10s")

		g.By("Created kubeletconfigs inside the limit must be successful")
		for _, kcItem := range createdKcs {
			kcItem.(*KubeletConfig).waitUntilSuccess("10s")
		}

		g.By("Check that only the right machine configs were created")
		// Check all ContainerRuntimeConfigs, the one created by this TC and the already existing ones too
		allKcs := []ResourceInterface{}
		allKcs = append(allKcs, createdKcs...)
		for _, kcItem := range kcs {
			key := kcItem
			allKcs = append(allKcs, &key)
		}
		allMcs := verifyRenderedMcs(oc, renderedKcConfigsSuffix, allKcs)

		kcCounter := 0
		for _, mc := range allMcs {
			if strings.HasPrefix(mc.name, "99-"+renderedKcConfigsSuffix) {
				kcCounter++
			}
		}
		o.Expect(kcCounter).Should(o.Equal(10), "Only %d Kubeletconfig resources should be generated", kcsLimit)

	})

	g.It("Author:sregidor-NonPreRelease-High-48468-ContainerRuntimeConfig has a limit of 10 per cluster [Disruptive]", func() {
		crsLimit := 10

		g.By("Pause mcp worker")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		defer mcp.pause(false)
		mcp.pause(true)

		g.By("Calculate number of existing ContainerRuntimeConfigs")
		crList := NewContainerRuntimeConfigList(oc.AsAdmin())
		crs, crlErr := crList.GetAll()
		o.Expect(crlErr).ShouldNot(o.HaveOccurred(), "Error getting existing ContainerRuntimeConfig resources")
		existingCrs := len(crs)
		logger.Infof("%d existing ContainerRuntimeConfig. We need to create %d ContainerRuntimeConfigs to reach the %d configs limit",
			existingCrs, crsLimit-existingCrs, crsLimit)

		g.By(fmt.Sprintf("Create %d container runtime configs to reach the limit", crsLimit-existingCrs))
		createdCrs := []ResourceInterface{}
		crTemplate := generateTemplateAbsolutePath("change-ctr-cr-config.yaml")
		for n := existingCrs + 1; n <= crsLimit; n++ {
			crName := fmt.Sprintf("change-ctr-cr-config-%d", n)
			cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
			defer cr.DeleteOrFail()
			cr.create()
			createdCrs = append(createdCrs, cr)
			logger.Infof("Created:\n %s", crName)
		}

		g.By("Created ContainerRuntimeConfigs must be successful")
		for _, crItem := range createdCrs {
			crItem.(*ContainerRuntimeConfig).waitUntilSuccess("10s")
		}

		g.By(fmt.Sprintf("Check that %d machine configs were created", crsLimit-existingCrs))
		renderedCrConfigsSuffix := "worker-generated-containerruntime"

		logger.Infof("Pre function res: %v", createdCrs)
		verifyRenderedMcs(oc, renderedCrConfigsSuffix, createdCrs)

		g.By(fmt.Sprintf("Create a new ContainerRuntimeConfig. The %dth one", crsLimit+1))
		crName := fmt.Sprintf("change-ctr-cr-config-%d", crsLimit+1)
		cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
		defer cr.DeleteOrFail()
		cr.create()

		g.By(fmt.Sprintf("Created container runtime configs over the limit must report a failure regarding the %d configs limit", crsLimit))
		expectedMsg := fmt.Sprintf("could not get ctrcfg key: max number of supported ctrcfgs (%d) has been reached. Please delete old ctrcfgs before retrying", crsLimit)
		cr.waitUntilFailure(expectedMsg, "10s")

		g.By("Created kubeletconfigs inside the limit must be successful")
		for _, crItem := range createdCrs {
			crItem.(*ContainerRuntimeConfig).waitUntilSuccess("10s")
		}

		g.By("Check that only the right machine configs were created")
		// Check all ContainerRuntimeConfigs, the one created by this TC and the already existing ones too
		allCrs := []ResourceInterface{}
		allCrs = append(allCrs, createdCrs...)
		for _, crItem := range crs {
			key := crItem
			allCrs = append(allCrs, &key)
		}

		allMcs := verifyRenderedMcs(oc, renderedCrConfigsSuffix, allCrs)

		crCounter := 0
		for _, mc := range allMcs {
			if strings.HasPrefix(mc.name, "99-"+renderedCrConfigsSuffix) {
				crCounter++
			}
		}
		o.Expect(crCounter).Should(o.Equal(10), "Only %d containerruntime resources should be generated", crsLimit)

	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-46314-Incorrect file contents if compression field is specified [Serial]", func() {
		g.By("Create a new MachineConfig to provision a config file in zipped format")

		fileContent := `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do
eiusmod tempor incididunt ut labore et dolore magna aliqua.  Ut
enim ad minim veniam, quis nostrud exercitation ullamco laboris
nisi ut aliquip ex ea commodo consequat.  Duis aute irure dolor in
reprehenderit in voluptate velit esse cillum dolore eu fugiat
nulla pariatur.  Excepteur sint occaecat cupidatat non proident,
sunt in culpa qui officia deserunt mollit anim id est laborum.


nulla pariatur.`

		mcName := "99-gzip-test"
		destPath := "/etc/test-file"
		fileConfig := getGzipFileJSONConfig(destPath, fileContent)

		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait until worker MachineConfigPool has finished the configuration")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()

		g.By("Verfiy that the file has been properly provisioned")
		node := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		rf := NewRemoteFile(node, destPath)
		err = rf.Fetch()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal("0644"))
		o.Expect(rf.GetUIDName()).To(o.Equal("root"))
		o.Expect(rf.GetGIDName()).To(o.Equal("root"))
	})

	g.It("NonHyperShiftHOST-Author:sregidor-High-46424-Check run level", func() {
		g.By("Validate openshift-machine-config-operator run level")
		mcoNs := NewResource(oc.AsAdmin(), "ns", MachineConfigNamespace)
		runLevel := mcoNs.GetOrFail(`{.metadata.labels.openshift\.io/run-level}`)

		logger.Debugf("Namespace definition:\n%s", mcoNs.PrettyString())
		o.Expect(runLevel).To(o.Equal(""), `openshift-machine-config-operator namespace should have run-level annotation equal to ""`)

		g.By("Validate machine-config-operator SCC")
		podsList := NewNamespacedResourceList(oc.AsAdmin(), "pods", mcoNs.name)
		podsList.ByLabel("k8s-app=machine-config-operator")
		mcoPods, err := podsList.GetAll()
		o.Expect(err).NotTo(o.HaveOccurred())
		logger.Infof("Validating that there is only one machine-config-operator pod")
		o.Expect(mcoPods).To(o.HaveLen(1))
		mcoPod := mcoPods[0]
		scc := mcoPod.GetOrFail(`{.metadata.annotations.openshift\.io/scc}`)

		logger.Infof("Validating that the operator pod has the right SCC")
		logger.Debugf("Machine-config-operator pod definition:\n%s", mcoPod.PrettyString())
		// on baremetal cluster, value of openshift.io/scc is nfs-provisioner, on AWS cluster it is hostmount-anyuid
		o.Expect(scc).Should(o.SatisfyAny(o.Equal("hostmount-anyuid"), o.Equal("nfs-provisioner")),
			`machine-config-operator pod is not using the right SCC`)

		g.By("Validate machine-config-daemon clusterrole")
		mcdCR := NewResource(oc.AsAdmin(), "clusterrole", "machine-config-daemon")
		mcdRules := mcdCR.GetOrFail(`{.rules[?(@.apiGroups[0]=="security.openshift.io")]}`)

		logger.Debugf("Machine-config-operator clusterrole definition:\n%s", mcdCR.PrettyString())
		o.Expect(mcdRules).Should(o.ContainSubstring("privileged"),
			`machine-config-daemon clusterrole has not the right configuration for ApiGroup "security.openshift.io"`)

		g.By("Validate machine-config-server clusterrole")
		mcsCR := NewResource(oc.AsAdmin(), "clusterrole", "machine-config-server")
		mcsRules := mcsCR.GetOrFail(`{.rules[?(@.apiGroups[0]=="security.openshift.io")]}`)
		logger.Debugf("Machine-config-server clusterrole definition:\n%s", mcdCR.PrettyString())
		o.Expect(mcsRules).Should(o.ContainSubstring("hostnetwork"),
			`machine-config-server clusterrole has not the right configuration for ApiGroup "security.openshift.io"`)

	})
	g.It("Author:sregidor-Longduration-NonPreRelease-High-46434-Mask service [Serial]", func() {
		activeString := "Active: active (running)"
		inactiveString := "Active: inactive (dead)"
		maskedString := "Loaded: masked"

		g.By("Validate that the chronyd service is active")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		svcOuput, err := workerNode.DebugNodeWithChroot("systemctl", "status", "chronyd")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svcOuput).Should(o.ContainSubstring(activeString))
		o.Expect(svcOuput).ShouldNot(o.ContainSubstring(inactiveString))

		g.By("Create a MachineConfig resource to mask the chronyd service")
		mcName := "99-test-mask-services"
		maskSvcConfig := getMaskServiceConfig("chronyd.service", true)
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err = mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("UNITS=[%s]", maskSvcConfig))
		o.Expect(err).NotTo(o.HaveOccurred())
		// if service is masked, but node drain is failed, unmask chronyd service on all worker nodes in this defer block
		// then clean up logic will delete this mc, node will be rebooted, when the system is back online, chronyd service
		// can be started automatically, unmask command can be executed w/o error with loaded & active service
		defer func() {
			workersNodes := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()
			for _, worker := range workersNodes {
				svcName := "chronyd"
				_, err := worker.UnmaskService(svcName)
				// just print out unmask op result here, make sure unmask op can be executed on all the worker nodes
				if err != nil {
					logger.Errorf("unmask %s failed on node %s: %v", svcName, worker.name, err)
				} else {
					logger.Infof("unmask %s success on node %s", svcName, worker.name)
				}
			}
		}()

		g.By("Wait until worker MachineConfigPool has finished the configuration")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()

		g.By("Validate that the chronyd service is masked")
		svcMaskedOuput, _ := workerNode.DebugNodeWithChroot("systemctl", "status", "chronyd")
		// Since the service is masked, the "systemctl status chronyd" command will return a value != 0 and an error will be reported
		// So we dont check the error, only the output
		o.Expect(svcMaskedOuput).ShouldNot(o.ContainSubstring(activeString))
		o.Expect(svcMaskedOuput).Should(o.ContainSubstring(inactiveString))
		o.Expect(svcMaskedOuput).Should(o.ContainSubstring(maskedString))

		g.By("Patch the MachineConfig resource to unmaskd the svc")
		// This part needs to be changed once we refactor MachineConfig to embed the Resource struct.
		// We will use here the 'mc' object directly
		mcresource := NewResource(oc.AsAdmin(), "mc", mc.name)
		err = mcresource.Patch("json", `[{ "op": "replace", "path": "/spec/config/systemd/units/0/mask", "value": false}]`)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait until worker MachineConfigPool has finished the configuration")
		mcp.waitForComplete()

		g.By("Validate that the chronyd service is unmasked")
		svcUnMaskedOuput, err := workerNode.DebugNodeWithChroot("systemctl", "status", "chronyd")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svcUnMaskedOuput).Should(o.ContainSubstring(activeString))
		o.Expect(svcUnMaskedOuput).ShouldNot(o.ContainSubstring(inactiveString))
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-46943-Config Drift. Config file. [Serial]", func() {
		g.By("Create a MC to deploy a config file")
		filePath := "/etc/mco-test-file"
		fileContent := "MCO test file\n"
		fileConfig := getURLEncodedFileConfig(filePath, fileContent, "")

		mcName := "mco-drift-test-file"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait until worker MCP has finished the configuration. No machine should be degraded.")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()

		g.By("Verfiy file content and permissions")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]

		defaultMode := "0644"
		rf := NewRemoteFile(workerNode, filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(defaultMode))

		g.By("Verify drift config behavior")
		defer func() {
			_ = rf.PushNewPermissions(defaultMode)
			_ = rf.PushNewTextContent(fileContent)
			_ = mcp.WaitForNotDegradedStatus()
		}()

		newMode := "0400"
		useForceFile := false
		verifyDriftConfig(mcp, rf, newMode, useForceFile)
	})

	g.It("Author:rioliu-NonPreRelease-High-46965-Avoid workload disruption for GPG Public Key Rotation [Serial]", func() {

		g.By("create new machine config with base64 encoded gpg public key")
		mcName := "add-gpg-pub-key"
		mcTemplate := "add-gpg-pub-key.yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("checkout machine config daemon logs to verify ")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		log, err := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(log).Should(o.ContainSubstring("/etc/machine-config-daemon/no-reboot/containers-gpg.pub"))
		o.Expect(log).Should(o.ContainSubstring("Changes do not require drain, skipping"))
		o.Expect(log).Should(o.ContainSubstring("crio config reloaded successfully"))
		o.Expect(log).Should(o.ContainSubstring("skipping reboot"))

		g.By("verify crio.service status")
		cmdOut, cmdErr := workerNode.DebugNodeWithChroot("systemctl", "is-active", "crio.service")
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).Should(o.ContainSubstring("active"))

	})

	g.It("Author:rioliu-NonPreRelease-High-47062-change policy.json on worker nodes [Serial]", func() {

		g.By("create new machine config to change /etc/containers/policy.json")
		mcName := "change-policy-json"
		mcTemplate := "change-policy-json.yaml"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		defer mc.delete()
		mc.create()

		g.By("verify file content changes")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
		fileContent, fileErr := workerNode.DebugNodeWithChroot("cat", "/etc/containers/policy.json")
		o.Expect(fileErr).NotTo(o.HaveOccurred())
		logger.Infof(fileContent)
		o.Expect(fileContent).Should(o.ContainSubstring(`{"default": [{"type": "insecureAcceptAnything"}]}`))
		o.Expect(fileContent).ShouldNot(o.ContainSubstring("transports"))

		g.By("checkout machine config daemon logs to make sure node drain/reboot are skipped")
		log, err := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigDaemon, workerNode.GetMachineConfigDaemon(), "")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(log).Should(o.ContainSubstring("/etc/containers/policy.json"))
		o.Expect(log).Should(o.ContainSubstring("Changes do not require drain, skipping"))
		o.Expect(log).Should(o.ContainSubstring("crio config reloaded successfully"))
		o.Expect(log).Should(o.ContainSubstring("skipping reboot"))

		g.By("verify crio.service status")
		cmdOut, cmdErr := workerNode.DebugNodeWithChroot("systemctl", "is-active", "crio.service")
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).Should(o.ContainSubstring("active"))

	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-46999-Config Drift. Config file permissions. [Serial]", func() {
		g.By("Create a MC to deploy a config file")
		filePath := "/etc/mco-test-file"
		fileContent := "MCO test file\n"
		fileMode := "0400" // decimal 256
		fileConfig := getURLEncodedFileConfig(filePath, fileContent, fileMode)

		mcName := "mco-drift-test-file-permissions"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait until worker MCP has finished the configuration. No machine should be degraded.")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()

		g.By("Verfiy file content and permissions")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]

		rf := NewRemoteFile(workerNode, filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(fileMode))

		g.By("Verify drift config behavior")
		defer func() {
			_ = rf.PushNewPermissions(fileMode)
			_ = rf.PushNewTextContent(fileContent)
			_ = mcp.WaitForNotDegradedStatus()
		}()

		newMode := "0644"
		useForceFile := true
		verifyDriftConfig(mcp, rf, newMode, useForceFile)
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-47045-Config Drift. Compressed files. [Serial]", func() {
		g.By("Create a MC to deploy a config file using compression")
		filePath := "/etc/mco-compressed-test-file"
		fileContent := "MCO test file\nusing compression"
		fileConfig := getGzipFileJSONConfig(filePath, fileContent)

		mcName := "mco-drift-test-compressed-file"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait until worker MCP has finished the configuration. No machine should be degraded.")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()

		g.By("Verfiy file content and permissions")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]

		rf := NewRemoteFile(workerNode, filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		defaultMode := "0644"
		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(defaultMode))

		g.By("Verfy drift config behavior")
		defer func() {
			_ = rf.PushNewPermissions(defaultMode)
			_ = rf.PushNewTextContent(fileContent)
			_ = mcp.WaitForNotDegradedStatus()
		}()

		newMode := "0400"
		useForceFile := true
		verifyDriftConfig(mcp, rf, newMode, useForceFile)
	})
	g.It("Author:sregidor-Longduration-NonPreRelease-High-47008-Config Drift. Dropin file. [Serial]", func() {
		g.By("Create a MC to deploy a unit with a dropin file")
		dropinFileName := "10-chrony-drop-test.conf"
		filePath := "/etc/systemd/system/chronyd.service.d/" + dropinFileName
		fileContent := "[Service]\nEnvironment=\"FAKE_OPTS=fake-value\""
		unitEnabled := true
		unitName := "chronyd.service"
		unitConfig := getDropinFileConfig(unitName, unitEnabled, dropinFileName, fileContent)

		mcName := "drifted-dropins-test"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("UNITS=[%s]", unitConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait until worker MCP has finished the configuration. No machine should be degraded.")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()

		g.By("Verfiy file content and permissions")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]

		rf := NewRemoteFile(workerNode, filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		defaultMode := "0644"
		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(defaultMode))

		g.By("Verify drift config behavior")
		defer func() {
			_ = rf.PushNewPermissions(defaultMode)
			_ = rf.PushNewTextContent(fileContent)
			_ = mcp.WaitForNotDegradedStatus()
		}()

		newMode := "0400"
		useForceFile := true
		verifyDriftConfig(mcp, rf, newMode, useForceFile)
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-47009-Config Drift. New Service Unit. [Serial]", func() {
		g.By("Create a MC to deploy a unit.")
		unitEnabled := true
		unitName := "example.service"
		filePath := "/etc/systemd/system/" + unitName
		fileContent := "[Service]\nType=oneshot\nExecStart=/usr/bin/echo Hello from MCO test service\n\n[Install]\nWantedBy=multi-user.target"
		unitConfig := getSingleUnitConfig(unitName, unitEnabled, fileContent)

		mcName := "drifted-new-service-test"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("UNITS=[%s]", unitConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait until worker MCP has finished the configuration. No machine should be degraded.")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()

		g.By("Verfiy file content and permissions")
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]

		rf := NewRemoteFile(workerNode, filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		defaultMode := "0644"
		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(defaultMode))

		g.By("Verfiy deployed unit")
		unitStatus, _ := workerNode.GetUnitStatus(unitName)
		// since it is a one-shot "hello world" service the execution will end
		// after the hello message and the unit will become inactive. So we dont check the error code.
		o.Expect(unitStatus).Should(
			o.And(
				o.ContainSubstring(unitName),
				o.ContainSubstring("Active: inactive (dead)"),
				o.ContainSubstring("Hello from MCO test service"),
				o.ContainSubstring("example.service: Deactivated successfully.")))

		g.By("Verify drift config behavior")
		defer func() {
			_ = rf.PushNewPermissions(defaultMode)
			_ = rf.PushNewTextContent(fileContent)
			_ = mcp.WaitForNotDegradedStatus()
		}()

		newMode := "0400"
		useForceFile := true
		verifyDriftConfig(mcp, rf, newMode, useForceFile)
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-51381-cordon node before node drain. OCP >= 4.11 [Serial]", func() {
		g.By("Capture initial migration-controller logs")
		ctrlerContainer := "machine-config-controller"
		ctrlerPod, podsErr := getMachineConfigControllerPod(oc)
		o.Expect(podsErr).NotTo(o.HaveOccurred())
		o.Expect(ctrlerPod).NotTo(o.BeEmpty())

		initialCtrlerLogs, initErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, ctrlerContainer, ctrlerPod, "")
		o.Expect(initErr).NotTo(o.HaveOccurred())

		g.By("Create a MC to deploy a config file")
		fileMode := "0644" // decimal 420
		filePath := "/etc/chrony.conf"
		fileContent := "pool 0.rhel.pool.ntp.org iburst\ndriftfile /var/lib/chrony/drift\nmakestep 1.0 3\nrtcsync\nlogdir /var/log/chrony"
		fileConfig := getBase64EncodedFileConfig(filePath, fileContent, fileMode)

		mcName := "cordontest-change-workers-chrony-configuration"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check MCD logs to make sure that the node is cordoned before being drained")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		workerNode := NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]

		o.Eventually(workerNode.PollIsCordoned(), fmt.Sprintf("%dm", mcp.estimateWaitTimeInMinutes()), "20s").Should(o.BeTrue(), "Worker node must be cordoned")

		searchRegexp := fmt.Sprintf("(?s)%s: initiating cordon.*node %s: Evicted pod", workerNode.GetName(), workerNode.GetName())
		o.Eventually(func() string {
			podAllLogs, _ := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, ctrlerContainer, ctrlerPod, "")
			// Remove the part of the log captured at the beginning of the test.
			// We only check the part of the log that this TC generates and ignore the previously generated logs
			return strings.Replace(podAllLogs, initialCtrlerLogs, "", 1)
		}, "5m", "10s").Should(o.MatchRegexp(searchRegexp), "Node should be cordoned before being drained")

		g.By("Wait until worker MCP has finished the configuration. No machine should be degraded.")
		mcp.waitForComplete()

		g.By("Verfiy file content and permissions")
		rf := NewRemoteFile(workerNode, filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(fileMode))
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-49568-Check nodes updating order maxUnavailable=1 [Serial]", func() {
		g.By("Scale machinesets and 1 more replica to make sure we have at least 2 nodes per machineset")
		platform := exutil.CheckPlatform(oc)
		logger.Infof("Platform is %s", platform)
		if platform != "none" && platform != "" {
			err := AddToAllMachineSets(oc, 1)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func() { o.Expect(AddToAllMachineSets(oc, -1)).NotTo(o.HaveOccurred()) }()
		} else {
			logger.Infof("Platform is %s, skipping the MachineSets replica configuration", platform)
		}

		g.By("Get the nodes in the worker pool sorted by update order")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		workerNodes, errGet := mcp.GetSortedNodes()
		o.Expect(errGet).NotTo(o.HaveOccurred())

		g.By("Create a MC to deploy a config file")
		filePath := "/etc/TC-49568-mco-test-file-order"
		fileContent := "MCO test file order\n"
		fileMode := "0400" // decimal 256
		fileConfig := getURLEncodedFileConfig(filePath, fileContent, fileMode)

		mcName := "mco-test-file-order"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Poll the nodes sorted by the order they are updated")
		maxUnavailable := 1
		updatedNodes := mcp.GetSortedUpdatedNodes(maxUnavailable)
		for _, n := range updatedNodes {
			logger.Infof("updated node: %s created: %s zone: %s", n.GetName(), n.GetOrFail(`{.metadata.creationTimestamp}`), n.GetOrFail(`{.metadata.labels.topology\.kubernetes\.io/zone}`))
		}

		g.By("Wait for the configuration to be applied in all nodes")
		mcp.waitForComplete()

		g.By("Check that nodes were updated in the right order")
		rightOrder := checkUpdatedLists(workerNodes, updatedNodes, maxUnavailable)
		o.Expect(rightOrder).To(o.BeTrue(), "Expected update order %s, but found order %s", workerNodes, updatedNodes)

		g.By("Verfiy file content and permissions")
		rf := NewRemoteFile(workerNodes[0], filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(fileMode))
	})

	g.It("Author:sregidor-Longduration-NonPreRelease-High-49672-Check nodes updating order maxUnavailable>1 [Serial]", func() {
		g.By("Scale machinesets and 1 more replica to make sure we have at least 2 nodes per machineset")
		platform := exutil.CheckPlatform(oc)
		logger.Infof("Platform is %s", platform)
		if platform != "none" && platform != "" {
			err := AddToAllMachineSets(oc, 1)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func() { o.Expect(AddToAllMachineSets(oc, -1)).NotTo(o.HaveOccurred()) }()
		} else {
			logger.Infof("Platform is %s, skipping the MachineSets replica configuration", platform)
		}

		// If the number of nodes is 2, since we are using maxUnavailable=2, all nodes will be cordoned at
		//  the same time and the eviction process will be stuck. In this case we need to skip the test case.
		numWorkers := len(NewNodeList(oc).GetAllLinuxWorkerNodesOrFail())
		if numWorkers <= 2 {
			g.Skip(fmt.Sprintf("The test case needs at least 3 worker nodes, because eviction will be stuck if not. Current num worker is %d, we skip the case",
				numWorkers))
		}

		g.By("Get the nodes in the worker pool sorted by update order")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		workerNodes, errGet := mcp.GetSortedNodes()
		o.Expect(errGet).NotTo(o.HaveOccurred())

		g.By("Set maxUnavailable value")
		maxUnavailable := 2
		mcp.SetMaxUnavailable(maxUnavailable)
		defer mcp.RemoveMaxUnavailable()

		g.By("Create a MC to deploy a config file")
		filePath := "/etc/TC-49672-mco-test-file-order"
		fileContent := "MCO test file order 2\n"
		fileMode := "0400" // decimal 256
		fileConfig := getURLEncodedFileConfig(filePath, fileContent, fileMode)

		mcName := "mco-test-file-order2"
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		defer mc.delete()

		err := mc.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Poll the nodes sorted by the order they are updated")
		updatedNodes := mcp.GetSortedUpdatedNodes(maxUnavailable)
		for _, n := range updatedNodes {
			logger.Infof("updated node: %s created: %s zone: %s", n.GetName(), n.GetOrFail(`{.metadata.creationTimestamp}`), n.GetOrFail(`{.metadata.labels.topology\.kubernetes\.io/zone}`))
		}

		g.By("Wait for the configuration to be applied in all nodes")
		mcp.waitForComplete()

		g.By("Check that nodes were updated in the right order")
		rightOrder := checkUpdatedLists(workerNodes, updatedNodes, maxUnavailable)
		o.Expect(rightOrder).To(o.BeTrue(), "Expected update order %s, but found order %s", workerNodes, updatedNodes)

		g.By("Verfiy file content and permissions")
		rf := NewRemoteFile(workerNodes[0], filePath)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())

		o.Expect(rf.GetTextContent()).To(o.Equal(fileContent))
		o.Expect(rf.GetNpermissions()).To(o.Equal(fileMode))
	})

	g.It("Author:rioliu-Longduration-NonPreRelease-High-49373-Send alert when MCO can't safely apply updated Kubelet CA on nodes in paused pool [Disruptive]", func() {

		g.By("create customized prometheus rule, change timeline and expression to trigger alert in a short period of time")
		ruleFile := generateTemplateAbsolutePath("prometheus-rule-mcc.yaml")
		defer func() {
			deleteErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", ruleFile).Execute()
			if deleteErr != nil {
				logger.Errorf("delete customized prometheus rule failed %v", deleteErr)
			}
		}()
		createRuleErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", ruleFile).Execute()
		o.Expect(createRuleErr).NotTo(o.HaveOccurred())

		g.By("check prometheus rule exists or not")
		mccRule, mccRuleErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", MachineConfigNamespace).Output()
		o.Expect(mccRuleErr).NotTo(o.HaveOccurred())
		o.Expect(mccRule).Should(o.ContainSubstring("machine-config-controller-test"))
		o.Expect(mccRule).Should(o.ContainSubstring("machine-config-controller")) // check the builtin rule exists or not

		g.By("pause mcp worker")
		workerPool := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		defer func() {
			workerPool.pause(false) // fallback solution, unpause worker pool if case is failed before last step.
			workerPool.waitForComplete()
		}()
		workerPool.pause(true)

		g.By("trigger kube-apiserver-to-kubelet-signer cert rotation")
		rotateErr := oc.AsAdmin().Run("patch").Args("secret/kube-apiserver-to-kubelet-signer", `-p={"metadata": {"annotations": {"auth.openshift.io/certificate-not-after": null}}}`, "-n", "openshift-kube-apiserver-operator").Execute()
		o.Expect(rotateErr).NotTo(o.HaveOccurred())

		g.By("check metrics for machine config controller")
		ctrlerPod, podsErr := getMachineConfigControllerPod(oc)
		o.Expect(podsErr).NotTo(o.HaveOccurred())
		o.Expect(ctrlerPod).NotTo(o.BeEmpty())
		// get sa token
		saToken, saErr := exutil.GetSAToken(oc)
		o.Expect(saErr).NotTo(o.HaveOccurred())
		o.Expect(saToken).NotTo(o.BeEmpty())
		// get metric value
		var metric string
		pollerr := wait.Poll(3*time.Second, 3*time.Minute, func() (bool, error) {
			cmd := "curl -k -H \"" + fmt.Sprintf("Authorization: Bearer %v", saToken) + "\" https://localhost:9001/metrics|grep machine_config_controller"
			metrics, metricErr := exutil.RemoteShPodWithBash(oc.AsAdmin(), MachineConfigNamespace, ctrlerPod, cmd)
			if metrics == "" || metricErr != nil {
				logger.Errorf("get mcc metrics failed in poller: %v, will try next round", metricErr)
				return false, nil
			}
			if len(metrics) > 0 {
				scanner := bufio.NewScanner(strings.NewReader(metrics))
				for scanner.Scan() {
					line := scanner.Text()
					if strings.Contains(line, `machine_config_controller_paused_pool_kubelet_ca{pool="worker"}`) {
						if !strings.HasSuffix(line, "0") {
							metric = line
							logger.Infof("found metric %s", line)
							return true, nil
						}
					}
				}
			}

			return false, nil
		})

		exutil.AssertWaitPollNoErr(pollerr, "got error when polling mcc metrics")
		o.Expect(metric).NotTo(o.BeEmpty())
		strArray := strings.Split(metric, " ") // split metric line with space, get metric value to verify, it should not be 0
		o.Expect(strArray[len(strArray)-1]).ShouldNot(o.Equal("0"))

		g.By("check pending alert")
		// wait 2 mins
		var counter int
		waiterErrX := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			if counter++; counter == 11 {
				return true, nil
			}
			logger.Infof("waiting for alert state change %s", strings.Repeat("=", counter))
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waiterErrX, "alert waiter poller is failed")

		pendingAlerts, pendingAlertErr := getAlertsByName(oc, "MachineConfigControllerPausedPoolKubeletCATest")
		o.Expect(pendingAlertErr).NotTo(o.HaveOccurred())
		o.Expect(pendingAlerts).ShouldNot(o.BeEmpty())

		for _, pendingAlert := range pendingAlerts {
			o.Expect(pendingAlert.Get("state").ToString()).Should(o.Equal("pending"))
			o.Expect(pendingAlert.Get("labels").Get("severity").ToString()).Should(o.Or(o.Equal("warning"), o.Equal("critical")))
		}

		g.By("check firing alert")
		// 5 mins later, all the pending alerts will be changed to firing
		counter = 0
		waiterErrY := wait.Poll(1*time.Minute, 7*time.Minute, func() (bool, error) {
			if counter++; counter == 6 {
				return true, nil
			}
			logger.Infof("waiting for alert state change %s", strings.Repeat("=", counter))
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waiterErrY, "alert waiter poller is failed")

		firingAlerts, firingAlertErr := getAlertsByName(oc, "MachineConfigControllerPausedPoolKubeletCATest")
		o.Expect(firingAlertErr).NotTo(o.HaveOccurred())
		o.Expect(firingAlerts).ShouldNot(o.BeEmpty())

		for _, firingAlert := range firingAlerts {
			o.Expect(firingAlert.Get("state").ToString()).Should(o.Equal("firing"))
			o.Expect(firingAlert.Get("labels").Get("severity").ToString()).Should(o.Or(o.Equal("warning"), o.Equal("critical")))
		}

		g.By("unpause mcp worker")
		workerPool.pause(false)
		workerPool.waitForComplete()

	})
	g.It("Author:sregidor-NonPreRelease-High-51219-Check ClusterRole rules", func() {
		expectedServiceAcc := MachineConfigDaemon
		eventsRoleBinding := MachineConfigDaemonEvents
		eventsClusterRole := MachineConfigDaemonEvents
		daemonClusterRoleBinding := MachineConfigDaemon
		daemonClusterRole := MachineConfigDaemon

		g.By(fmt.Sprintf("Check %s service account", expectedServiceAcc))
		serviceAccount := NewNamespacedResource(oc.AsAdmin(), "ServiceAccount", MachineConfigNamespace, expectedServiceAcc)
		o.Expect(serviceAccount.Exists()).To(o.BeTrue(), "Service account %s should exist in namespace %s", expectedServiceAcc, MachineConfigNamespace)

		g.By("Check service accounts in daemon pods")
		checkNodePermissions := func(node Node) {
			daemonPodName := node.GetMachineConfigDaemon()
			logger.Infof("Checking permissions in daemon pod %s", daemonPodName)
			daemonPod := NewNamespacedResource(node.oc, "pod", MachineConfigNamespace, daemonPodName)
			o.Expect(daemonPod.GetOrFail(`{.spec.containers[?(@.name == "oauth-proxy")].args}`)).
				Should(o.ContainSubstring(fmt.Sprintf("--openshift-service-account=%s", expectedServiceAcc)),
					"oauth-proxy in daemon pod %s should use service account %s", daemonPodName, expectedServiceAcc)

			o.Expect(daemonPod.GetOrFail(`{.spec.serviceAccount}`)).Should(o.Equal(expectedServiceAcc),
				"Pod %s should use service account: %s", daemonPodName, expectedServiceAcc)

			o.Expect(daemonPod.GetOrFail(`{.spec.serviceAccountName}`)).Should(o.Equal(expectedServiceAcc),
				"Pod %s should use service account name: %s", daemonPodName, expectedServiceAcc)

		}
		nodes, err := NewNodeList(oc.AsAdmin()).GetAllLinux()
		o.Expect(err).ShouldNot(o.HaveOccurred(), "Error getting the list of nodes")
		for _, node := range nodes {
			g.By(fmt.Sprintf("Checking node %s", node.GetName()))
			checkNodePermissions(node)
		}

		g.By("Check events rolebindings in default namespace")
		defaultEventsRoleBindings := NewNamespacedResource(oc.AsAdmin(), "RoleBinding", "default", "machine-config-daemon-events")
		o.Expect(defaultEventsRoleBindings.Exists()).Should(o.BeTrue(), "'%s' Rolebinding not found in 'default' namespace", eventsRoleBinding)
		// Check the bound SA
		machineConfigSubject := JSON(defaultEventsRoleBindings.GetOrFail(fmt.Sprintf(`{.subjects[?(@.name=="%s")]}`, expectedServiceAcc)))
		o.Expect(machineConfigSubject.ToMap()).Should(o.HaveKeyWithValue("name", expectedServiceAcc),
			"'%s' in 'default' namespace should bind %s SA in namespace %s", eventsRoleBinding, expectedServiceAcc, MachineConfigNamespace)
		o.Expect(machineConfigSubject.ToMap()).Should(o.HaveKeyWithValue("namespace", MachineConfigNamespace),
			"'%s' in 'default' namespace should bind %s SA in namespace %s", eventsRoleBinding, expectedServiceAcc, MachineConfigNamespace)

		// Check the ClusterRole
		machineConfigClusterRole := JSON(defaultEventsRoleBindings.GetOrFail(`{.roleRef}`))
		o.Expect(machineConfigClusterRole.ToMap()).Should(o.HaveKeyWithValue("kind", "ClusterRole"),
			"'%s' in 'default' namespace should bind a ClusterRole", eventsRoleBinding)
		o.Expect(machineConfigClusterRole.ToMap()).Should(o.HaveKeyWithValue("name", eventsClusterRole),
			"'%s' in 'default' namespace should bind %s ClusterRole", eventsRoleBinding, eventsClusterRole)

		g.By(fmt.Sprintf("Check events rolebindings in %s namespace", MachineConfigNamespace))
		mcoEventsRoleBindings := NewNamespacedResource(oc.AsAdmin(), "RoleBinding", MachineConfigNamespace, "machine-config-daemon-events")
		o.Expect(defaultEventsRoleBindings.Exists()).Should(o.BeTrue(), "'%s' Rolebinding not found in '%s' namespace", eventsRoleBinding, MachineConfigNamespace)
		// Check the bound SA
		machineConfigSubject = JSON(mcoEventsRoleBindings.GetOrFail(fmt.Sprintf(`{.subjects[?(@.name=="%s")]}`, expectedServiceAcc)))
		o.Expect(machineConfigSubject.ToMap()).Should(o.HaveKeyWithValue("name", expectedServiceAcc),
			"'%s' in '%s' namespace should bind %s SA in namespace %s", eventsRoleBinding, MachineConfigNamespace, expectedServiceAcc, MachineConfigNamespace)
		o.Expect(machineConfigSubject.ToMap()).Should(o.HaveKeyWithValue("namespace", MachineConfigNamespace),
			"'%s' in '%s' namespace should bind %s SA in namespace %s", eventsRoleBinding, MachineConfigNamespace, expectedServiceAcc, MachineConfigNamespace)

		// Check the ClusterRole
		machineConfigClusterRole = JSON(mcoEventsRoleBindings.GetOrFail(`{.roleRef}`))
		o.Expect(machineConfigClusterRole.ToMap()).Should(o.HaveKeyWithValue("kind", "ClusterRole"),
			"'%s' in '%s' namespace should bind a ClusterRole", eventsRoleBinding, MachineConfigNamespace)
		o.Expect(machineConfigClusterRole.ToMap()).Should(o.HaveKeyWithValue("name", eventsClusterRole),
			"'%s' in '%s' namespace should bind %s CLusterRole", eventsRoleBinding, MachineConfigNamespace, eventsClusterRole)

		g.By(fmt.Sprintf("Check MCO cluseterrolebindings in %s namespace", MachineConfigNamespace))
		mcoCRB := NewResource(oc.AsAdmin(), "ClusterRoleBinding", daemonClusterRoleBinding)
		o.Expect(mcoCRB.Exists()).Should(o.BeTrue(), "'%s' ClusterRolebinding not found.", daemonClusterRoleBinding)
		// Check the bound SA
		machineConfigSubject = JSON(mcoCRB.GetOrFail(fmt.Sprintf(`{.subjects[?(@.name=="%s")]}`, expectedServiceAcc)))
		o.Expect(machineConfigSubject.ToMap()).Should(o.HaveKeyWithValue("name", expectedServiceAcc),
			"'%s' ClusterRoleBinding should bind %s SA in namespace %s", daemonClusterRoleBinding, expectedServiceAcc, MachineConfigNamespace)
		o.Expect(machineConfigSubject.ToMap()).Should(o.HaveKeyWithValue("namespace", MachineConfigNamespace),
			"'%s' ClusterRoleBinding should bind %s SA in namespace %s", daemonClusterRoleBinding, expectedServiceAcc, MachineConfigNamespace)

		// Check the ClusterRole
		machineConfigClusterRole = JSON(mcoCRB.GetOrFail(`{.roleRef}`))
		o.Expect(machineConfigClusterRole.ToMap()).Should(o.HaveKeyWithValue("kind", "ClusterRole"),
			"'%s' ClusterRoleBinding should bind a ClusterRole", daemonClusterRoleBinding)
		o.Expect(machineConfigClusterRole.ToMap()).Should(o.HaveKeyWithValue("name", daemonClusterRole),
			"'%s' ClusterRoleBinding should bind %s CLusterRole", daemonClusterRoleBinding, daemonClusterRole)

		g.By("Check events clusterrole")
		eventsCR := NewResource(oc.AsAdmin(), "ClusterRole", eventsClusterRole)
		o.Expect(eventsCR.Exists()).To(o.BeTrue(), "ClusterRole %s should exist", eventsClusterRole)

		stringRules := eventsCR.GetOrFail(`{.rules}`)
		o.Expect(stringRules).ShouldNot(o.ContainSubstring("pod"),
			"ClusterRole %s should grant no pod permissions at all", eventsClusterRole)

		rules := JSON(stringRules)
		for _, rule := range rules.Items() {
			describesEvents := false
			resources := rule.Get("resources")
			for _, resource := range resources.Items() {
				if resource.ToString() == "events" {
					describesEvents = true
				}
			}

			if describesEvents {
				verbs := rule.Get("verbs").ToList()
				o.Expect(verbs).Should(o.ContainElement("create"), "In ClusterRole %s 'events' rule should have 'create' permissions", eventsClusterRole)
				o.Expect(verbs).Should(o.ContainElement("patch"), "In ClusterRole %s 'events' rule should have 'patch' permissions", eventsClusterRole)
				o.Expect(verbs).Should(o.HaveLen(2), "In ClusterRole %s 'events' rule should ONLY Have 'create' and 'patch' permissions", eventsClusterRole)
			}
		}

		g.By("Check daemon clusterrole")
		daemonCR := NewResource(oc.AsAdmin(), "ClusterRole", daemonClusterRole)
		stringRules = daemonCR.GetOrFail(`{.rules}`)
		o.Expect(stringRules).ShouldNot(o.ContainSubstring("pod"),
			"ClusterRole %s should grant no pod permissions at all", daemonClusterRole)
		o.Expect(stringRules).ShouldNot(o.ContainSubstring("daemonsets"),
			"ClusterRole %s should grant no daemonsets permissions at all", daemonClusterRole)

		rules = JSON(stringRules)
		for _, rule := range rules.Items() {
			describesNodes := false
			resources := rule.Get("resources")
			for _, resource := range resources.Items() {
				if resource.ToString() == "nodes" {
					describesNodes = true
				}
			}

			if describesNodes {
				verbs := rule.Get("verbs").ToList()
				o.Expect(verbs).Should(o.ContainElement("get"), "In ClusterRole %s 'nodes' rule should have 'get' permissions", daemonClusterRole)
				o.Expect(verbs).Should(o.ContainElement("list"), "In ClusterRole %s 'nodes' rule should have 'list' permissions", daemonClusterRole)
				o.Expect(verbs).Should(o.ContainElement("watch"), "In ClusterRole %s 'nodes' rule should have 'watch' permissions", daemonClusterRole)
				o.Expect(verbs).Should(o.HaveLen(3), "In ClusterRole %s 'events' rule should ONLY Have 'get', 'list' and 'watch' permissions", daemonClusterRole)
			}
		}

	})
	g.It("Author:sregidor-NonPreRelease-Medium-52373-Modify proxy configuration in paused pools [Disruptive]", func() {

		proxyValue := "http://user:pass@proxy-fake:1111"
		noProxyValue := "test.52373.no-proxy.com"

		g.By("Get current proxy configuration")
		proxy := NewResource(oc.AsAdmin(), "proxy", "cluster")
		proxyInitialConfig := proxy.GetOrFail(`{.spec}`)
		logger.Infof("Initial proxy configuration: %s", proxyInitialConfig)

		wmcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mmcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)

		defer func() {
			logger.Infof("Start TC defer block")

			logger.Infof("Restore original proxy config %s", proxyInitialConfig)
			_ = proxy.Patch("json", `[{ "op": "add", "path": "/spec", "value": `+proxyInitialConfig+`}]`)

			logger.Infof("Wait for new machine configs to be rendered and paused pools to report updated status")
			// We need to make sure that the config will NOT be applied, since the proxy is a fake one and if
			// we dont make sure that the config proxy is reverted, the nodes will be broken and go into
			// NotReady status
			_ = wmcp.WaitForUpdatedStatus()
			_ = mmcp.WaitForUpdatedStatus()

			logger.Infof("Unpause worker pool")
			wmcp.pause(false)

			logger.Infof("Unpause master pool")
			mmcp.pause(false)

			logger.Infof("End TC defer block")
		}()

		g.By("Pause MCPs")
		wmcp.pause(true)
		mmcp.pause(true)

		g.By("Configure new proxy")
		err := proxy.Patch("json",
			`[{ "op": "add", "path": "/spec/httpProxy", "value": "`+proxyValue+`" }]`)
		o.Expect(err).ShouldNot(o.HaveOccurred(), "Error patching http proxy")

		err = proxy.Patch("json",
			`[{ "op": "add", "path": "/spec/httpsProxy", "value": "`+proxyValue+`" }]`)
		o.Expect(err).ShouldNot(o.HaveOccurred(), "Error patching https proxy")

		err = proxy.Patch("json",
			`[{ "op": "add", "path": "/spec/noProxy", "value":  "`+noProxyValue+`" }]`)
		o.Expect(err).ShouldNot(o.HaveOccurred(), "Error patching noproxy")

		g.By("Verify that the proxy configuration was applied to daemonsets")
		mcoDs := NewNamespacedResource(oc.AsAdmin(), "DaemonSet", MachineConfigNamespace, "machine-config-daemon")
		// it should never take longer than 5 minutes to apply the proxy config under any circumstance,
		// it should be considered a bug.
		o.Eventually(mcoDs.Poll(`{.spec}`), "5m", "30s").Should(o.ContainSubstring(proxyValue),
			"machine-config-daemon is not using the new proxy configuration: %s", proxyValue)
		o.Eventually(mcoDs.Poll(`{.spec}`), "5m", "30s").Should(o.ContainSubstring(noProxyValue),
			"machine-config-daemon is not using the new no-proxy value: %s", noProxyValue)

		g.By("Check that the operator has been marked as degraded")
		mco := NewResource(oc.AsAdmin(), "co", "machine-config")
		o.Eventually(mco.Poll(`{.status.conditions[?(@.type=="Degraded")].status}`),
			"5m", "30s").Should(o.Equal("True"),
			"machine-config Operator should report degraded status")

		o.Eventually(mco.Poll(`{.status.conditions[?(@.type=="Degraded")].message}`),
			"5m", "30s").Should(o.ContainSubstring(`Required MachineConfigPool 'master' is paused and can not sync until it is unpaused`),
			"machine-config Operator is not reporting the right reason for degraded status")

		g.By("Restore original proxy configuration")
		err = proxy.Patch("json", `[{ "op": "add", "path": "/spec", "value": `+proxyInitialConfig+`}]`)
		o.Expect(err).ShouldNot(o.HaveOccurred(), "Error patching and restoring original proxy config")

		g.By("Verify that the new configuration is applied to the daemonset")
		// it should never take longer than 5 minutes to apply the proxy config under any circumstance,
		// it should be considered a bug.
		o.Eventually(mcoDs.Poll(`{.spec}`), "5m", "30s").ShouldNot(o.ContainSubstring(proxyValue),
			"machine-config-daemon has not restored the original proxy configuration")
		o.Eventually(mcoDs.Poll(`{.spec}`), "5m", "30s").ShouldNot(o.ContainSubstring(noProxyValue),
			"machine-config-daemon has not restored the original proxy configuration for 'no-proxy'")

		g.By("Check that the operator is not marked as degraded anymore")
		o.Eventually(mco.Poll(`{.status.conditions[?(@.type=="Degraded")].status}`),
			"5m", "30s").Should(o.Equal("False"),
			"machine-config Operator should not report degraded status anymore")

	})

	g.It("Author:sregidor-NonPreRelease-Medium-52520-Configure unqualified-search-registries in Image.config resource [Disruptive]", func() {
		expectedDropinFilePath := "/etc/containers/registries.conf.d/01-image-searchRegistries.conf"
		expectedDropinContent := "unqualified-search-registries = [\"quay.io\"]\nshort-name-mode = \"\"\n"

		g.By("Get current image.config cluster configuration")
		ic := NewResource(oc.AsAdmin(), "image.config", "cluster")
		icInitialConfig := ic.GetOrFail(`{.spec}`)
		logger.Infof("Initial image.config cluster configuration: %s", icInitialConfig)

		wmcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mmcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)

		workers, wsErr := wmcp.GetSortedNodes()
		o.Expect(wsErr).ShouldNot(o.HaveOccurred(), "Error getting the nodes in worker pool")

		masters, msErr := mmcp.GetSortedNodes()
		o.Expect(msErr).ShouldNot(o.HaveOccurred(), "Error getting the nodes in master pool")

		firstUpdatedWorker := workers[0]
		firstUpdatedMaster := masters[0]

		defer func() {
			logger.Infof("Start TC defer block")

			logger.Infof("Restore original image.config cluster config %s", icInitialConfig)
			_ = ic.Patch("json", `[{ "op": "add", "path": "/spec", "value": `+icInitialConfig+`}]`)

			logger.Infof("Wait for the original configuration to be applied")
			wmcp.waitForComplete()
			mmcp.waitForComplete()

			logger.Infof("End TC defer block")
		}()

		g.By("Add quay.io to unqualified-search-regisitries list in image.config cluster resource")
		startTime, dErr := firstUpdatedMaster.GetDate()
		o.Expect(dErr).ShouldNot(o.HaveOccurred(), "Error getting date in node %s", firstUpdatedMaster.GetName())

		o.Expect(firstUpdatedWorker.IgnoreEventsBeforeNow()).NotTo(o.HaveOccurred(),
			"Error getting the last event in node %s", firstUpdatedWorker.GetName())

		o.Expect(firstUpdatedMaster.IgnoreEventsBeforeNow()).NotTo(o.HaveOccurred(),
			"Error getting the last event in node %s", firstUpdatedMaster.GetName())

		patchErr := ic.Patch("merge", `{"spec": {"registrySources": {"containerRuntimeSearchRegistries":["quay.io"]}}}`)
		o.Expect(patchErr).ShouldNot(o.HaveOccurred(), "Error while partching the image.config cluster resource")

		g.By("Wait for first nodes to be configured")
		// Worker and master nodes should go into 'working' status
		o.Eventually(firstUpdatedWorker.IsUpdating, "5m", "20s").Should(o.BeTrue(),
			"Node %s is not in 'working' status after the new image.conig is configured")
		o.Eventually(firstUpdatedMaster.IsUpdating, "5m", "20s").Should(o.BeTrue(),
			"Node %s is not in 'working' status after the new image.conig is configured")

		// We dont actually wait for the whole configuration to be applied
		//  we will only wait for those nodes to be unpdated
		// Not waiting for the MCPs to finish the configuration makes this test case faster
		// If it causes unstability, just wait here for the MCPs to complete the configuration instead
		// Worker and master nodes should go into 'working' status
		o.Eventually(firstUpdatedWorker.IsUpdated, "10m", "20s").Should(o.BeTrue(),
			"Node %s is not in 'Done' status after the configuration is applied")
		o.Eventually(firstUpdatedMaster.IsUpdated, "10m", "20s").Should(o.BeTrue(),
			"Node %s is not in 'Done' status after the configuration is applied")

		g.By("Print all events for the verified worker node")
		el := NewEventList(oc.AsAdmin(), "default")
		el.ByFieldSelector(`involvedObject.name=` + firstUpdatedWorker.GetName())
		events, _ := el.GetAll()
		printString := ""
		for _, event := range events {
			printString += fmt.Sprintf("-  %s\n", event)
		}
		logger.Infof("All events for node %s:\n%s", firstUpdatedWorker.GetName(), printString)
		logger.Infof("OK!\n")

		g.By("Verify that a drain and reboot events were triggered for worker node")
		wEvents, weErr := firstUpdatedWorker.GetEvents()

		logger.Infof("All events for  node %s since: %s", firstUpdatedWorker.GetName(), firstUpdatedWorker.eventCheckpoint)
		for _, event := range wEvents {
			logger.Infof("-         %s", event)
		}
		o.Expect(weErr).ShouldNot(o.HaveOccurred(), "Error getting events for node %s", firstUpdatedWorker.GetName())
		o.Expect(wEvents).To(HaveEventsSequence("Drain", "Reboot"),
			"Error, the expected sequence of events is not found in node %s", firstUpdatedWorker.GetName())

		g.By("Verify that a drain and reboot events were triggered for master node")
		mEvents, meErr := firstUpdatedMaster.GetEvents()
		o.Expect(meErr).ShouldNot(o.HaveOccurred(), "Error getting drain events for node %s", firstUpdatedMaster.GetName())
		o.Expect(mEvents).To(HaveEventsSequence("Drain", "Reboot"),
			"Error, the expected sequence of events is not found in node %s", firstUpdatedWorker.GetName())

		g.By("Verify that the node was actually rebooted")
		o.Expect(firstUpdatedWorker.GetUptime()).Should(o.BeTemporally(">", startTime),
			"The node %s should have been rebooted after the configurion. Uptime didnt happen after start config time.")
		o.Expect(firstUpdatedMaster.GetUptime()).Should(o.BeTemporally(">", startTime),
			"The node %s should have been rebooted after the configurion. Uptime didnt happen after start config time.")

		g.By("Verify dropin file's content in worker node")
		wdropinFile := NewRemoteFile(firstUpdatedWorker, expectedDropinFilePath)
		wfetchErr := wdropinFile.Fetch()
		o.Expect(wfetchErr).ShouldNot(o.HaveOccurred(), "Error getting the content offile %s in node %s",
			expectedDropinFilePath, firstUpdatedWorker.GetName())

		o.Expect(wdropinFile.GetTextContent()).Should(o.Equal(expectedDropinContent))

		g.By("Verify dropin file's content in master node")
		mdropinFile := NewRemoteFile(firstUpdatedMaster, expectedDropinFilePath)
		mfetchErr := mdropinFile.Fetch()
		o.Expect(mfetchErr).ShouldNot(o.HaveOccurred(), "Error getting the content offile %s in node %s",
			expectedDropinFilePath, firstUpdatedMaster.GetName())

		o.Expect(mdropinFile.GetTextContent()).Should(o.Equal(expectedDropinContent))

	})

	g.It("Author:sregidor-NonPreRelease-High-52822-Create new config resources with 2.2.0 ignition boot image nodes [Disruptive]", func() {

		// Skip if not AWS
		platform := exutil.CheckPlatform(oc)
		if platform != AWSPlatform {
			g.Skip(fmt.Sprintf("Current platform is %s. AWS platform is required to execute this test case!.", platform))
		}

		// Skip if not east-2 zone
		infra := NewResource(oc.AsAdmin(), "infrastructure", "cluster")
		zone := infra.GetOrFail(`{.status.platformStatus.aws.region}`)
		if zone != "us-east-2" {
			g.Skip(fmt.Sprintf(`Current AWS zone is '%s'. AWS 'us-east-2' zone is required to execute this test case!.`, zone))
		}

		// Test execution
		newSecretName := "worker-user-data-modified-tc-52822"
		newMsName := "copied-machineset-modified-tc-52822"
		kcName := "change-maxpods-kubelet-config"
		kcTemplate := generateTemplateAbsolutePath(kcName + ".yaml")
		crName := "change-ctr-cr-config"
		crTemplate := generateTemplateAbsolutePath(crName + ".yaml")
		mcName := "generic-config-file-test-52822"
		mcpWorker := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)

		defer func() {
			logger.Infof("Start TC defer block")
			newMs := NewMachineSet(oc.AsAdmin(), MachineAPINamespace, newMsName)
			if newMs.Exists() {
				logger.Infof("Scaling %s machineset to zero", newMsName)
				_ = newMs.ScaleTo(0)

				logger.Infof("Waiting %s machineset for being ready", newMsName)
				_ = newMs.WaitUntilReady("15m")

				logger.Infof("Removing %s machineset", newMsName)
				_ = newMs.Delete()
			}

			newSecret := NewNamespacedResource(oc.AsAdmin(), "Secret", MachineAPINamespace, newSecretName)
			if newSecret.Exists() {
				logger.Infof("Removing %s secret", newSecretName)
				_ = newSecret.Delete()
			}

			cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
			if cr.Exists() {
				logger.Infof("Removing ContainerRuntimeConfig %s", cr.GetName())
				_ = cr.Delete()
			}
			kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
			if kc.Exists() {
				logger.Infof("Removing KubeletConfig %s", kc.GetName())
				_ = kc.Delete()
			}

			// MachineConfig struct has not been refactored to compose the "Resource" struct
			// so there is no "Exists" method available. Use it after refactoring MachineConfig
			mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
			logger.Infof("Removing machineconfig %s", mcName)
			mc.delete()

			logger.Infof("Waiting for worker pool to be updated")
			mcpWorker.waitForComplete()

			logger.Infof("End TC defer block")
		}()

		// Duplicate an existing MachineSet
		g.By("Duplicate a MachineSet resource")
		allMs, err := NewMachineSetList(oc, MachineAPINamespace).GetAll()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error getting a list of MachineSet resources")
		ms := allMs[0]
		newMs, dErr := ms.Duplicate(newMsName)
		o.Expect(dErr).NotTo(o.HaveOccurred(), "Error duplicating MachineSet %s -n %s", ms.GetName(), ms.GetNamespace())

		// Get current secret used by the MachineSet
		currentSecret := ms.GetOrFail(`{.spec.template.spec.providerSpec.value.userDataSecret.name}`)
		logger.Infof("Currently used secret: %s", currentSecret)

		// Create a new secret using "append" and "2.2.0" Ignition version
		g.By("Create a new secret with 2.2.0 ignition version and 'append' configuration")
		logger.Infof("Duplicating secret %s with new name %s", currentSecret, newSecretName)
		changes := msDuplicatedSecretChanges{Name: newSecretName, IgnitionVersion: "2.2.0", IgnitionConfigAction: "append"}
		_, sErr := duplicateMachinesetSecret(oc, currentSecret, changes)
		o.Expect(sErr).NotTo(o.HaveOccurred(), "Error duplicating machine-api secret")

		// Set the 4.5 boot image ami for east-2 zone.
		// the right ami should be selected from here https://github.com/openshift/installer/blob/release-4.5/data/data/rhcos.json
		g.By("Configure the duplicated MachineSet to use the 4.5 boot image")
		err = newMs.Patch("json", `[{ "op": "replace", "path": "/spec/template/spec/providerSpec/value/ami/id", "value": "ami-0ba8d5168e13bbcce" }]`)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error patching MachineSet %s to use the new 4.5 boot image", newMs.GetName())

		// Use new secret
		g.By("Configure the duplicated MachineSet to use the new secret")
		err = newMs.Patch("json", `[{ "op": "replace", "path": "/spec/template/spec/providerSpec/value/userDataSecret/name", "value": "`+newSecretName+`" }]`)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error patching MachineSet %s to use the new secret %s", newMs.GetName(), newSecretName)

		// KubeletConfig
		g.By("Create KubeletConfig")
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		kc.create()
		kc.waitUntilSuccess("10s")

		// ContainterRuntimeConfig
		g.By("Create ContainterRuntimeConfig")
		cr := NewContainerRuntimeConfig(oc.AsAdmin(), crName, crTemplate)
		cr.create()
		cr.waitUntilSuccess("10s")

		// Generic machineconfig
		g.By("Create generic config file")
		genericConfigFilePath := "/etc/test-52822"
		genericConfig := "config content for test case 52822"

		fileConfig := getURLEncodedFileConfig(genericConfigFilePath, genericConfig, "420")
		template := NewMCOTemplate(oc, "generic-machine-config-template.yml")
		errCreate := template.Create("-p", "NAME="+mcName, "-p", "POOL=worker", "-p", fmt.Sprintf("FILES=[%s]", fileConfig))
		o.Expect(errCreate).NotTo(o.HaveOccurred(), "Error creating MachineConfig %s", mcName)

		// Wait for all pools to apply the configs
		g.By("Wait for worker MCP to be updated")
		mcpWorker.waitForComplete()

		// Scale up the MachineSet
		g.By("Scale MachineSet up")
		logger.Infof("Scaling up machineset %s", newMs.GetName())
		scaleErr := newMs.ScaleTo(1)
		o.Expect(scaleErr).NotTo(o.HaveOccurred(), "Error scaling up MachineSet %s", newMs.GetName())

		logger.Infof("Waiting %s machineset for being ready", newMsName)
		o.Eventually(newMs.PollIsReady(), "15m", "30s").Should(o.BeTrue(), "MachineSet %s is not ready", newMs.GetName())

		// Verify that the scaled nodes has been configured properly
		g.By("Check config in the new node")
		newNodes, nErr := newMs.GetNodes()
		o.Expect(nErr).NotTo(o.HaveOccurred(), "Error getting the nodes created by MachineSet %s", newMs.GetName())
		o.Expect(newNodes).To(o.HaveLen(1), "Only one node should have been created by MachineSet %s", newMs.GetName())
		newNode := newNodes[0]
		logger.Infof("New node: %s", newNode.GetName())

		g.By("Check kubelet config")
		kcFile := NewRemoteFile(*newNode, "/etc/kubernetes/kubelet.conf")
		kcrErr := kcFile.Fetch()
		o.Expect(kcrErr).NotTo(o.HaveOccurred(), "Error reading kubelet config in node %s", newNode.GetName())
		o.Expect(kcFile.GetTextContent()).Should(o.ContainSubstring("\"maxPods\": 500"),
			"File /etc/kubernetes/kubelet.conf has not the expected content")

		g.By("Check container runtime config")
		crFile := NewRemoteFile(*newNode, "/etc/containers/storage.conf")
		crrErr := crFile.Fetch()
		o.Expect(crrErr).NotTo(o.HaveOccurred(), "Error reading container runtime config in node %s", newNode.GetName())
		o.Expect(crFile.GetTextContent()).Should(o.ContainSubstring("size = \"8G\""),
			"File /etc/containers/storage.conf has not the expected content")

		g.By("Check generic machine config")
		cFile := NewRemoteFile(*newNode, genericConfigFilePath)
		crErr := cFile.Fetch()
		o.Expect(crErr).NotTo(o.HaveOccurred(), "Error reading generic config file in node %s", newNode.GetName())
		o.Expect(cFile.GetTextContent()).Should(o.Equal(genericConfig),
			"File %s has not the expected content", genericConfigFilePath)
	})

	g.It("Author:rioliu-NonPreRelease-High-53668-when FIPS and realtime kernel are both enabled node should NOT be degraded [Disruptive]", func() {
		// skip if arm64. realtime kernel is not supported.
		exutil.SkipARM64(oc)
		// skip the test if fips is not enabled
		skipTestIfFIPSIsNotEnabled(oc)
		// skip the test if platform is not aws or gcp. realtime kargs currently supported on these platforms
		skipTestIfSupportedPlatformNotMatched(oc, AWSPlatform, GCPPlatform)

		g.By("create machine config to enable fips ")
		fipsMcName := "50-fips-bz-poc"
		fipsMcTemplate := "bz2096496-dummy-mc-for-fips.yaml"
		fipsMc := NewMachineConfig(oc.AsAdmin(), fipsMcName, MachineConfigPoolMaster).SetMCOTemplate(fipsMcTemplate)

		defer fipsMc.delete()
		fipsMc.create()

		g.By("create machine config to enable RT kernel")
		rtMcName := "50-realtime-kernel"
		rtMcTemplate := "change-worker-kernel-argument.yaml"
		rtMc := NewMachineConfig(oc.AsAdmin(), rtMcName, MachineConfigPoolMaster).SetMCOTemplate(rtMcTemplate)

		defer rtMc.delete()
		rtMc.create()

		masterNode := NewNodeList(oc).GetAllMasterNodesOrFail()[0]

		g.By("check whether fips is enabled")
		fipsEnabled, fipsErr := masterNode.IsFIPSEnabled()
		o.Expect(fipsErr).NotTo(o.HaveOccurred())
		o.Expect(fipsEnabled).Should(o.BeTrue(), "fips is not enabled on node %s", masterNode.GetName())

		g.By("check whether fips related kernel arg is enabled")
		fipsKarg := "trigger-fips-issue=1"
		fipsKargEnabled, fipsKargErr := masterNode.IsKernelArgEnabled(fipsKarg)
		o.Expect(fipsKargErr).NotTo(o.HaveOccurred())
		o.Expect(fipsKargEnabled).Should(o.BeTrue(), "fips related kernel arg %s is not enabled on node %s", fipsKarg, masterNode.GetName())

		g.By("check whether RT kernel is enabled")
		rtEnabled, rtErr := masterNode.IsKernelArgEnabled("PREEMPT_RT")
		o.Expect(rtErr).NotTo(o.HaveOccurred())
		o.Expect(rtEnabled).Should(o.BeTrue(), "RT kernel is not enabled on node %s", masterNode.GetName())
	})

	g.It("Author:sregidor-NonPreRelease-Critical-53960-No failed units in the bootstrap machine", func() {
		skipTestIfSupportedPlatformNotMatched(oc, AWSPlatform, AzurePlatform)

		failedUnitsCommand := "sudo systemctl list-units --failed --all"

		// If no bootstrap is found, we skip the case.
		// The  test can only be executed in deployments that didn't remove the bootstrap machine
		bs, err := bootstrap.GetBootstrap(oc)
		if err != nil {
			if _, notFound := err.(*bootstrap.InstanceNotFound); notFound {
				g.Skip("skip test because bootstrap machine does not exist in the current cluster")
			}
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify that there is no failed units in the bootstrap machine")
		// ssh client is a bit unstable, and it can return an empty string for no apparent reason every now and then.
		// Hence we use 'Eventually' to verify the command to make the test robust.
		o.Eventually(func() string {
			logger.Infof("Executing command in bootstrap: %s", failedUnitsCommand)
			failedUnits, err := bs.SSH.RunOutput(failedUnitsCommand)
			logger.Infof("Command output:\n%s", failedUnits)
			if err != nil {
				logger.Errorf("Command Error:\n%s", err)
			}
			return failedUnits
		}).Should(o.ContainSubstring("0 loaded units listed"),
			"There are failed units in the bootstrap machine")

	})
	g.It("Author:sregidor-NonPreRelease-Medium-55879-Don't allow creating the force file via MachineConfig [Disruptive]", func() {
		var (
			filePath    = "/run/machine-config-daemon-force"
			fileContent = ""
			fileMode    = "0420" // decimal 272
			fileConfig  = getURLEncodedFileConfig(filePath, fileContent, fileMode)
			mcName      = "mco-tc-55879-create-force-file"
			mcp         = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)

			expectedNDMessage = regexp.QuoteMeta(fmt.Sprintf("cannot create %s via Ignition", filePath)) // quotemeta to scape regex characters in the file path
			expectedNDReason  = "1 nodes are reporting degraded status on sync"
		)

		g.By("Create the force file using a MC")

		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf("FILES=[%s]", fileConfig)}
		mc.skipWaitForMcp = true

		validateMcpNodeDegraded(mc, mcp, expectedNDMessage, expectedNDReason)

	})

	g.It("NonHyperShiftHOST-Author:rioliu-Medium-54937-logs and events are flood with clusterrole and clusterrolebinding [Disruptive]", func() {

		g.By("get machine-config-operator pod name")
		mcoPod, getMcoPodErr := getMachineConfigOperatorPod(oc)
		o.Expect(getMcoPodErr).NotTo(o.HaveOccurred(), "get mco pod failed")

		if exutil.CheckPlatform(oc) == "vsphere" { // check platformStatus.VSphere related log on vpshere cluster only
			g.By("check infra/cluster info, make sure platformStatus.VSphere does not exist")
			infra := NewResource(oc.AsAdmin(), "infrastructure", "cluster")
			vsphereStatus, getStatusErr := infra.Get(`{.status.platformStatus.VSphere}`)
			o.Expect(getStatusErr).NotTo(o.HaveOccurred(), "check vsphere status failed")
			// check vsphereStatus exists or not, only check logs if it exists, otherwise skip the test
			if vsphereStatus == "" {
				g.By("check vsphere related log in machine-config-operator pod")
				filteredVsphereLog, filterVsphereLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigOperator, mcoPod, "PlatformStatus.VSphere")
				// if no platformStatus.Vsphere log found, the func will return error, that's expected
				logger.Debugf("filtered vsphere log:\n %s", filteredVsphereLog)
				o.Expect(filterVsphereLogErr).Should(o.HaveOccurred(), "found vsphere related log in mco pod")
			}
		}

		// check below logs for all platforms
		g.By("check clusterrole and clusterrolebinding related logs in machine-config-operator pod")
		filteredClusterRoleLog, filterClusterRoleLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigOperator, mcoPod, "ClusterRoleUpdated")
		logger.Debugf("filtered clusterrole log:\n %s", filteredClusterRoleLog)
		o.Expect(filterClusterRoleLogErr).Should(o.HaveOccurred(), "found ClusterRoleUpdated log in mco pod")

		filteredClusterRoleBindingLog, filterClusterRoleBindingLogErr := exutil.GetSpecificPodLogs(oc, MachineConfigNamespace, MachineConfigOperator, mcoPod, "ClusterRoleBindingUpdated")
		logger.Debugf("filtered clusterrolebinding log:\n %s", filteredClusterRoleBindingLog)
		o.Expect(filterClusterRoleBindingLogErr).Should(o.HaveOccurred(), "found ClusterRoleBindingUpdated log in mco pod")

	})

	g.It("Author:sregidor-NonPreRelease-Medium-54922-daemon: add check before updating kernelArgs [Disruptive]", func() {
		var (
			mcNameArg1         = "tc-54922-kernel-args-1"
			mcNameArg2         = "tc-54922-kernel-args-2"
			mcNameExt          = "tc-54922-extension"
			kernelArg1         = "test1"
			kernelArg2         = "test2"
			usbguardMCTemplate = "change-worker-extension-usbguard.yaml"

			expectedLogArg1Regex = regexp.QuoteMeta("Running rpm-ostree [kargs") + ".*" + regexp.QuoteMeta(fmt.Sprintf("--append=%s", kernelArg1)) +
				".*" + regexp.QuoteMeta("]")
			expectedLogArg2Regex = regexp.QuoteMeta("Running rpm-ostree [kargs") + ".*" + regexp.QuoteMeta(fmt.Sprintf("--delete=%s", kernelArg1)) +
				".*" + regexp.QuoteMeta(fmt.Sprintf("--append=%s", kernelArg1)) +
				".*" + regexp.QuoteMeta(fmt.Sprintf("--append=%s", kernelArg2)) +
				".*" + regexp.QuoteMeta("]")

			// Expr: "kargs .*--append|kargs .*--delete"
			// We need to scape the "--" characters
			expectedNotLogExtesionRegex = "kargs .*" + regexp.QuoteMeta("--") + "append|kargs .*" + regexp.QuoteMeta("--") + "delete"

			mcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		)

		skipTestIfSupportedPlatformNotMatched(oc, AWSPlatform, GCPPlatform)
		workerNode := skipTestIfOsIsNotCoreOs(oc)

		// Create MC to add kernel arg 'test1'
		g.By(fmt.Sprintf("Create a MC to add a kernel arg: %s", kernelArg1))
		mcArgs1 := NewMachineConfig(oc.AsAdmin(), mcNameArg1, MachineConfigPoolWorker)
		mcArgs1.parameters = []string{fmt.Sprintf(`KERNEL_ARGS=["%s"]`, kernelArg1)}
		mcArgs1.skipWaitForMcp = true

		defer mcArgs1.delete()
		mcArgs1.create()
		logger.Infof("OK!\n")

		g.By("Check that the MCD logs are tracing the new kernel argument")
		// We don't know if the selected node will be updated first or last, so we have to wait
		// the same time we would wait for the mcp to be updated. Aprox.
		timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
		logger.Infof("waiting time: %s", timeToWait.String())
		o.Expect(workerNode.CaptureMCDaemonLogsUntilRestartWithTimeout(timeToWait.String())).To(
			o.MatchRegexp(expectedLogArg1Regex),
			"A log line reporting new kernel arguments should be present in the MCD logs when we add a kernel argument via MC")
		logger.Infof("OK!\n")

		g.By("Wait for worker pool to be updated")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Check that the new kernel argument was added")
		o.Expect(workerNode.IsKernelArgEnabled(kernelArg1)).To(o.BeTrue(),
			"Kernel argument %s was not enabled in the node %s", kernelArg1, workerNode.GetName())
		logger.Infof("OK!\n")

		// Create MC to add kernel arg 'test2'
		g.By(fmt.Sprintf("Create a MC to add a kernel arg: %s", kernelArg2))
		mcArgs2 := NewMachineConfig(oc.AsAdmin(), mcNameArg2, MachineConfigPoolWorker)
		mcArgs2.parameters = []string{fmt.Sprintf(`KERNEL_ARGS=["%s"]`, kernelArg2)}
		mcArgs2.skipWaitForMcp = true

		defer mcArgs2.deleteNoWait()
		mcArgs2.create()
		logger.Infof("OK!\n")

		g.By("Check that the MCD logs are tracing both kernel arguments")
		// We don't know if the selected node will be updated first or last, so we have to wait
		// the same time we would wait for the mcp to be updated. Aprox.
		logger.Infof("waiting time: %s", timeToWait.String())
		o.Expect(workerNode.CaptureMCDaemonLogsUntilRestartWithTimeout(timeToWait.String())).To(
			o.MatchRegexp(expectedLogArg2Regex),
			"A log line reporting the new kernel arguments configuration should be present in MCD logs")
		logger.Infof("OK!\n")

		g.By("Wait for worker pool to be updated")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Check that the both kernel arguments were added")
		o.Expect(workerNode.IsKernelArgEnabled(kernelArg1)).To(o.BeTrue(),
			"Kernel argument %s was not enabled in the node %s", kernelArg1, workerNode.GetName())
		o.Expect(workerNode.IsKernelArgEnabled(kernelArg2)).To(o.BeTrue(),
			"Kernel argument %s was not enabled in the node %s", kernelArg2, workerNode.GetName())
		logger.Infof("OK!\n")

		// Create MC to deploy an usbguard extension
		g.By("Create MC to add usbguard extension")
		mcUsb := NewMachineConfig(oc.AsAdmin(), mcNameExt, MachineConfigPoolWorker).SetMCOTemplate(usbguardMCTemplate)
		mcUsb.skipWaitForMcp = true

		defer mcUsb.deleteNoWait()
		mcUsb.create()
		logger.Infof("OK!\n")

		g.By("Check that the MCD logs do not make any reference to add or delete kargs")
		o.Expect(workerNode.CaptureMCDaemonLogsUntilRestartWithTimeout(timeToWait.String())).NotTo(
			o.MatchRegexp(expectedNotLogExtesionRegex),
			"MCD logs should not make any reference to kernel arguments addition/deletion when no new kernel arg is added/deleted")

		logger.Infof("OK!\n")

		g.By("Wait for worker pool to be updated")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Check that the usbguard extension was added")
		o.Expect(workerNode.RpmIsInstalled("usbguard")).To(
			o.BeTrue(),
			"usbguard rpm should be installed in node %s", workerNode.GetName())
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-NonPreRelease-Medium-56123-Invalid extensions should degrade the machine config pool [Disruptive]", func() {
		var (
			validExtension   = "usbguard"
			invalidExtension = "zsh"
			mcName           = "mco-tc-56123-invalid-extension"
			mcp              = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)

			expectedNDMessage = regexp.QuoteMeta(fmt.Sprintf("invalid extensions found: [%s]", invalidExtension)) // quotemeta to scape regex characters
			expectedNDReason  = "1 nodes are reporting degraded status on sync"
		)

		g.By("Create a MC with invalid extensions")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf(`EXTENSIONS=["%s", "%s"]`, validExtension, invalidExtension)}
		mc.skipWaitForMcp = true

		validateMcpNodeDegraded(mc, mcp, expectedNDMessage, expectedNDReason)

	})

	g.It("NonHyperShiftHOST-Author:rioliu-Medium-54974-silence audit log events for container infra", func() {

		auditRuleFile := "/etc/audit/rules.d/mco-audit-quiet-containers.rules"
		auditLogFile := "/var/log/audit/audit.log"

		allCoreOsNodes := NewNodeList(oc.AsAdmin()).GetAllCoreOsNodesOrFail()
		for _, node := range allCoreOsNodes {
			g.By(fmt.Sprintf("log into node %s to check audit rule file exists or not", node.GetName()))
			o.Expect(node.DebugNodeWithChroot("stat", auditRuleFile)).ShouldNot(
				o.ContainSubstring("No such file or directory"),
				"The audit rules file %s should exist in the nodes", auditRuleFile)

			g.By("check expected msgtype in audit log rule file")
			grepOut, _ := node.DebugNodeWithOptions([]string{"--quiet"}, "chroot", "/host", "bash", "-c", fmt.Sprintf("grep -E 'NETFILTER_CFG|ANOM_PROMISCUOUS' %s", auditRuleFile))
			o.Expect(grepOut).NotTo(o.BeEmpty(), "expected excluded audit log msgtype not found")
			o.Expect(grepOut).Should(o.And(
				o.ContainSubstring("NETFILTER_CFG"),
				o.ContainSubstring("ANOM_PROMISCUOUS"),
			), "audit log rules does not have excluded msstype NETFILTER_CFG and ANOM_PROMISCUOUS")

			g.By(fmt.Sprintf("check audit log on node %s, make sure msg types NETFILTER_CFG and ANOM_PROMISCUOUS are excluded", node.GetName()))
			filteredLog, _ := node.DebugNodeWithChroot("bash", "-c", fmt.Sprintf("grep -E 'NETFILTER_CFG|ANOM_PROMISCUOUS' %s", auditLogFile))
			o.Expect(filteredLog).ShouldNot(o.Or(
				o.ContainSubstring("NETFILTER_CFG"),
				o.ContainSubstring("ANOM_PROMISCUOUS"),
			), "audit log contains excluded msgtype NETFILTER_CFG or ANOM_PROMISCUOUS")
		}
	})

	g.It("Author:sregidor-DEPRECATED-NonPreRelease-Medium-43279-Alert message of drain error contains pod info [Disruptive]", func() {
		var (
			mcp               = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			mcc               = NewController(oc.AsAdmin())
			nsName            = oc.Namespace()
			pdbName           = "dont-evict-43279"
			podName           = "dont-evict-43279"
			podTemplate       = generateTemplateAbsolutePath("create-pod.yaml")
			mcName            = "test-file"
			mcTemplate        = "add-mc-to-trigger-node-drain.yaml"
			expectedAlertName = "MCCDrainError"
		)
		// Get the first node that will be updated
		sortedWorkerNodes, sErr := mcp.GetSortedNodes()
		o.Expect(sErr).NotTo(o.HaveOccurred(), "Error getting the nodes that belong to %s pool", mcp.GetName())
		workerNode := sortedWorkerNodes[0]

		g.By("Start machine-config-controller logs capture")
		ignoreMccLogErr := mcc.IgnoreLogsBeforeNow()
		o.Expect(ignoreMccLogErr).NotTo(o.HaveOccurred(), "Ignore mcc log failed")
		logger.Infof("OK!\n")

		g.By("Create a pod disruption budget to set minAvailable to 1")
		pdbTemplate := generateTemplateAbsolutePath("pod-disruption-budget.yaml")
		pdb := PodDisruptionBudget{name: pdbName, namespace: nsName, template: pdbTemplate}
		defer pdb.delete(oc)
		pdb.create(oc)
		logger.Infof("OK!\n")

		g.By("Create new pod for pod disruption budget")
		hostname, err := workerNode.GetNodeHostname()
		o.Expect(err).NotTo(o.HaveOccurred())
		pod := exutil.Pod{Name: podName, Namespace: nsName, Template: podTemplate, Parameters: []string{"HOSTNAME=" + hostname}}
		defer func() { o.Expect(pod.Delete(oc)).NotTo(o.HaveOccurred()) }()
		pod.Create(oc)
		logger.Infof("OK!\n")

		g.By("Create new mc to add new file on the node and trigger node drain")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		mc.skipWaitForMcp = true
		defer mc.delete()
		defer func() {
			_ = pod.Delete(oc)
			mcp.WaitForNotDegradedStatus()
		}()
		mc.create()
		logger.Infof("OK!\n")

		g.By("Wait until node is cordoned")
		o.Eventually(workerNode.Poll(`{.spec.taints[?(@.effect=="NoSchedule")].effect}`),
			"20m", "1m").Should(o.Equal("NoSchedule"), fmt.Sprintf("Node %s was not cordoned", workerNode.name))
		logger.Infof("OK!\n")

		g.By("Verify that node is not degraded until the alarm timeout")
		o.Consistently(mcp.pollDegradedStatus(),
			"58m", "5m").Should(o.Equal("False"),
			"The worker MCP was degraded too soon. The worker MCP should not be degraded until 1 hour timeout happens")
		logger.Infof("OK!\n")

		g.By("Verify that node is degraded after the 1h timeout")
		o.Eventually(mcp.pollDegradedStatus(),
			"5m", "1m").Should(o.Equal("True"),
			"1 hour passed since the eviction problems were reported and the worker MCP has not been degraded")
		logger.Infof("OK!\n")

		g.By("Verify that the error is properly reported in the controller pod's logs")
		logger.Debugf("CONTROLLER LOGS BEGIN!\n")
		logger.Debugf(mcc.GetFilteredLogs(workerNode.GetName()))
		logger.Debugf("CONTROLLER LOGS END!\n")

		o.Expect(mcc.GetFilteredLogs(workerNode.GetName())).Should(
			o.ContainSubstring("node %s: drain exceeded timeout: 1h0m0s. Will continue to retry.",
				workerNode.GetName()),
			"The eviction problem is not properly reported in the MCController pod logs")
		logger.Infof("OK!\n")

		g.By("Verify that the error is properly reported in the MachineConfigPool status")
		nodeDegradedCondition := mcp.GetConditionByType("NodeDegraded")
		nodeDegradedConditionJSON := JSON(nodeDegradedCondition)
		nodeDegradedMessage := nodeDegradedConditionJSON.Get("message").ToString()
		expectedDegradedNodeMessage := fmt.Sprintf("failed to drain node: %s after 1 hour. Please see machine-config-controller logs for more information", workerNode.GetName())

		logger.Infof("MCP NodeDegraded condition: %s", nodeDegradedCondition)
		o.Expect(nodeDegradedMessage).To(o.ContainSubstring(expectedDegradedNodeMessage),
			"The error reported in the MCP NodeDegraded condition in not the expected one")
		logger.Infof("OK!\n")

		g.By("Verify that the alert is triggered with the right message")
		alertJSON, err := getAlertsByName(oc, expectedAlertName)

		logger.Infof("Found %s alerts: %s", expectedAlertName, alertJSON)

		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error trying to get the %s alert", expectedAlertName)
		o.Expect(alertJSON).To(o.HaveLen(1),
			"One and only one %s alert should be reported because of the eviction problems", expectedAlertName)

		expectedAnnotation := fmt.Sprintf("Drain failed on %s , updates may be blocked. For more details check MachineConfigController pod logs: oc logs -f -n openshift-machine-config-operator machine-config-controller-xxxxx -c machine-config-controller", workerNode.GetName())
		o.Expect(alertJSON[0].Get("annotations").Get("message")).Should(o.ContainSubstring(expectedAnnotation),
			"The error description should make a reference to the pod info")
		logger.Infof("OK!\n")

		g.By("Remove the  pod disruption budget")
		pdb.delete(oc)
		logger.Infof("OK!\n")

		g.By("Verfiy that the pool stops being degraded")
		o.Eventually(mcp.pollDegradedStatus(),
			"10m", "30s").Should(o.Equal("False"),
			"After removing the PodDisruptionBudget the eviction should have succeeded and the worker pool should stop being degraded")
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-NonPreRelease-Medium-57009-Kubeletconfig custom tlsSecurityProfile [Disruptive]", func() {
		var (
			kcName     = "tc-57009-set-kubelet-custom-tls-profile"
			kcTemplate = generateTemplateAbsolutePath("custom-tls-profile-kubelet-config.yaml")
			mcp        = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			workerNode = NewNodeList(oc.AsAdmin()).GetAllLinuxWorkerNodesOrFail()[0]

			expectedTLSMinVersion   = `"VersionTLS11"`
			expectedTLSCipherSuites = `[
						    "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
						    "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
						    "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
						    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
						  ]`
		)

		g.By("Create Kubeletconfig to configure a custom tlsSecurityProfile")
		kc := NewKubeletConfig(oc.AsAdmin(), kcName, kcTemplate)
		defer mcp.waitForComplete()
		defer kc.DeleteOrFail()
		kc.create()
		logger.Infof("KubeletConfig was created. Waiting for success.")
		kc.waitUntilSuccess("5m")
		logger.Infof("OK!\n")

		g.By("Wait for MachineConfigPools to be updated")
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Verify that the tls is configured properly")
		rf := NewRemoteFile(workerNode, "/etc/kubernetes/kubelet.conf")
		o.Expect(rf.Fetch()).To(o.Succeed(),
			"Error trying to get the kubelet config from node %s", workerNode.GetName())

		jsonConfig := JSON(rf.GetTextContent())
		logger.Infof("Verifying tlsMinVersion")
		o.Expect(jsonConfig.Get("tlsMinVersion")).To(
			o.MatchJSON(expectedTLSMinVersion),
			"The configured tlsMinVersion in /etc/kubernetes/kubelet.conf file is not the expected one")

		logger.Infof("Verifying tlsCipherSuites")
		o.Expect(jsonConfig.Get("tlsCipherSuites")).To(
			o.MatchJSON(expectedTLSCipherSuites),
			"The configured tlsCipherSuites in /etc/kubernetes/kubelet.conf file are not the expected one")
		logger.Infof("OK!\n")
	})
	g.It("Author:sregidor-NonPreRelease-Medium-56614-Create unit with content and mask=true[Disruptive]", func() {
		var (
			workerNode     = NewNodeList(oc).GetAllLinuxWorkerNodesOrFail()[0]
			maskedString   = "Loaded: masked"
			inactiveString = "Active: inactive (dead)"
			mcName         = "tc-56614-maks-and-contents"
			svcName        = "tc-56614-maks-and-contents.service"
			svcContents    = "[Unit]\nDescription=Just random content for test case 56614"
			maskSvcConfig  = getMaskServiceWithContentsConfig(svcName, true, svcContents)
		)

		g.By("Create a MachineConfig resource to mask the chronyd service")
		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf("UNITS=[%s]", maskSvcConfig)}
		defer mc.delete()

		mc.create()
		logger.Infof("OK!\n")

		g.By("Wait until worker MachineConfigPool has finished the configuration")
		mcp := NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Validate that the service is masked")
		output, _ := workerNode.DebugNodeWithChroot("systemctl", "status", svcName)
		// Since the service is masked, the "systemctl status" command will return a value != 0 and an error will be reported
		// So we dont check the error, only the output
		o.Expect(output).Should(o.And(
			o.ContainSubstring(inactiveString),
			o.ContainSubstring(maskedString),
		),
			"Service %s should be inactive and masked, but it is not.", svcName)
		logger.Infof("OK!\n")

		g.By("Validate the content")
		rf := NewRemoteFile(workerNode, "/etc/systemd/system/"+svcName)
		rferr := rf.Fetch()
		o.Expect(rferr).NotTo(o.HaveOccurred())
		o.Expect(rf.GetSymLink()).Should(o.Equal(fmt.Sprintf("'/etc/systemd/system/%s' -> '/dev/null'", svcName)),
			"The service is masked, so service's file should be a link to /dev/null")
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonPreRelease-Medium-57595-Use empty pull-secret[Disruptive]", func() {
		var (
			pullSecret = GetPullSecret(oc.AsAdmin())
			wMcp       = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			mMcp       = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
		)

		// If the cluster is using extensions, empty pull-secret will break the pools because images' validation is mandatory
		skipTestIfExtensionsAreUsed(oc.AsAdmin())
		// If RT kernel is enabled, empty pull-secrets will break the pools because the image's validation is mandatory
		skipTestIfRTKernel(oc.AsAdmin())

		g.By("Capture the current pull-secret value")
		// We don't use the pullSecret resource directly, instead we use auxiliary functions that will
		// extract and restore the secret's values using a file. Like that we can recover the value of the pull-secret
		// if our execution goes wrong, without printing it in the logs (for security reasons).
		secretFile, err := getPullSecret(oc)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error getting the pull-secret")
		logger.Debugf("Pull-secret content stored in file %s", secretFile)
		defer func() {
			logger.Infof("Start defer func")
			logger.Infof("Restoring initial pull-secret value")
			output, err := setDataForPullSecret(oc, secretFile)
			if err != nil {
				logger.Errorf("Error restoring the pull-secret's value. Error: %s\nOutput: %s", err, output)
			}
			wMcp.waitForComplete()
			mMcp.waitForComplete()
			logger.Infof("End defer func")
		}()
		logger.Infof("OK!\n")

		g.By("Set an empty pull-secret")
		o.Expect(pullSecret.SetDataValue(".dockerconfigjson", "{}")).To(o.Succeed(),
			"Error setting an empty pull-secret value")

		logger.Infof("OK!\n")

		g.By("Wait for machine config poools to be udated")
		logger.Infof("Wait for worker pool to be updated")
		wMcp.waitForComplete()
		logger.Infof("Wait for master pool to be updated")
		mMcp.waitForComplete()
		logger.Infof("OK!\n")
	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Medium-25819-enable FIPS by MCO not supported [Disruptive]", func() {
		var (
			mcTemplate = "change-fips.yaml"
			mcName     = "mco-tc-25819-master-fips"
			wMcp       = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			mMcp       = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)

			expectedNDMessage = regexp.QuoteMeta("detected change to FIPS flag; refusing to modify FIPS on a running cluster")
			expectedNDReason  = "1 nodes are reporting degraded status on sync"
		)

		g.By("Try to enable FIPS in master pool")

		mMc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolMaster).SetMCOTemplate(mcTemplate)
		mMc.parameters = []string{"FIPS=true"}
		mMc.skipWaitForMcp = true

		validateMcpNodeDegraded(mMc, mMcp, expectedNDMessage, expectedNDReason)
		logger.Infof("OK!\n")

		g.By("Try to enable FIPS in worker pool")
		wMc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
		wMc.parameters = []string{"FIPS=true"}
		wMc.skipWaitForMcp = true

		validateMcpNodeDegraded(wMc, wMcp, expectedNDMessage, expectedNDReason)
		logger.Infof("OK!\n")

	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Medium-59837-Use wrong user when creating a file [Disruptive]", func() {
		var (
			mcName              = "mco-tc-59837-create-file-with-wrong-user"
			wrongUserFileConfig = `{"contents": {"source": "data:text/plain;charset=utf-8;base64,dGVzdA=="},"mode": 420,"path": "/etc/wronguser-test-file.test","user": {"name": "wronguser"}}`
			mcp                 = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
			// quotemeta to scape regex characters in the file path
			expectedNDMessage = regexp.QuoteMeta(`failed to retrieve file ownership for file \"/etc/wronguser-test-file.test\": failed to retrieve UserID for username: wronguser`)
			expectedNDReason  = "1 nodes are reporting degraded status on sync"
		)

		g.By("Create the force file using a MC")

		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf("FILES=[%s]", wrongUserFileConfig)}
		mc.skipWaitForMcp = true

		validateMcpNodeDegraded(mc, mcp, expectedNDMessage, expectedNDReason)

	})

	g.It("Author:sregidor-NonHyperShiftHOST-NonPreRelease-Medium-59867-Create files specifying user and group [Disruptive]", func() {
		var (
			filesContent = "test"
			coreUserID   = 1000
			coreGroupID  = 1000
			rootUserID   = 0
			admGroupID   = 4

			allFiles = []ign32File{
				{
					Path: "/etc/core-core-name-test-file.test",
					Contents: ign32Contents{
						Source: GetBase64EncodedFileSourceContent(filesContent),
					},
					Mode: PtrInt(420), // decimal 0644
					User: ign32FileUser{
						Name: "core",
					},
					Group: ign32FileGroup{
						Name: "core",
					},
				},
				{
					Path: "/etc/core-core-id-test-file.test",
					Contents: ign32Contents{
						Source: GetBase64EncodedFileSourceContent(filesContent),
					},
					Mode: PtrInt(416), // decimal 0640
					User: ign32FileUser{
						ID: PtrInt(coreUserID), // core user ID number
					},
					Group: ign32FileGroup{
						ID: PtrInt(coreGroupID), // core group ID number
					},
				},
				{
					Path: "/etc/root-adm-id-test-file.test",
					Contents: ign32Contents{
						Source: GetBase64EncodedFileSourceContent(filesContent),
					},
					Mode: PtrInt(384), // decimal 0600
					User: ign32FileUser{
						ID: PtrInt(rootUserID),
					},
					Group: ign32FileGroup{
						ID: PtrInt(admGroupID),
					},
				},
				{
					Path: "/etc/nouser-test-file.test",
					Contents: ign32Contents{
						Source: GetBase64EncodedFileSourceContent(filesContent),
					},
					Mode: PtrInt(420), // decimal 0644
					User: ign32FileUser{
						ID: PtrInt(12343), // this user does not exist
					},
					Group: ign32FileGroup{
						ID: PtrInt(34321), // this group does not exist
					},
				},
			}
			workerNode = NewNodeList(oc).GetAllWorkerNodesOrFail()[0]
			wMcp       = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		)

		g.By("Create new machine config to create files with different users and groups")
		fileConfig := MarshalOrFail(allFiles)
		mcName := "tc-59867-create-files-with-users"

		mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker)
		mc.parameters = []string{fmt.Sprintf("FILES=%s", fileConfig)}
		defer mc.delete()

		mc.create()
		wMcp.waitForComplete()
		logger.Infof("OK!\n")

		g.By("Check that all files have been created with the right user, group, permissions and data")
		for _, file := range allFiles {
			logger.Infof("")
			logger.Infof("CHecking file: %s", file.Path)
			rf := NewRemoteFile(workerNode, file.Path)
			o.Expect(rf.Fetch()).NotTo(o.HaveOccurred(), "Error getting the file %s in node %s", file.Path, workerNode.GetName())

			logger.Infof("Checking content: %s", rf.GetTextContent())
			o.Expect(rf.GetTextContent()).To(o.Equal(filesContent),
				"The content of file %s is wrong!", file.Path)

			// Verify that the defined user name or user id (only one can be defined in the config) is the expected one
			if file.User.Name != "" {
				logger.Infof("Checking user name: %s", rf.GetUIDName())
				o.Expect(rf.GetUIDName()).To(o.Equal(file.User.Name),
					"The user who owns file %s is wrong!", file.Path)
			} else {
				logger.Infof("Checking user id: %s", rf.GetUIDNumber())
				o.Expect(rf.GetUIDNumber()).To(o.Equal(fmt.Sprintf("%d", *file.User.ID)),
					"The user id what owns file %s is wrong!", file.Path)
			}

			// Verify that if the user ID is defined and its value is the core user's one. Then the name should be "core"
			if file.User.ID != nil && *file.User.ID == coreUserID {
				logger.Infof("Checking core user name for ID: %s", rf.GetUIDNumber())
				o.Expect(rf.GetUIDName()).To(o.Equal("core"),
					"The user name who owns file %s is wrong! User name for Id %s should be 'core'",
					file.Path, rf.GetUIDNumber())
			}

			// Verify that if the user ID is defined and its value is the root user's one. Then the name should be "root"
			if file.User.ID != nil && *file.User.ID == rootUserID {
				logger.Infof("Checking root user name: %s", rf.GetUIDName())
				o.Expect(rf.GetUIDName()).To(o.Equal("root"),
					"The user name who owns file %s is wrong! User name for Id %s should be 'root'",
					file.Path, rf.GetUIDNumber())
			}

			// Verify that the defined group name or group id (only one can be defined in the config) is the expected one
			if file.Group.Name != "" {
				logger.Infof("Checking group name: %s", rf.GetGIDName())
				o.Expect(rf.GetGIDName()).To(o.Equal(file.Group.Name),
					"The group that owns file %s is wrong!", file.Path)
			} else {
				logger.Infof("Checking group id: %s", rf.GetGIDNumber())
				o.Expect(rf.GetGIDNumber()).To(o.Equal(fmt.Sprintf("%d", *file.Group.ID)),
					"The group id what owns file %s is wrong!", file.Path)
			}

			// Verify that if the group ID is defined and its value is the core group's one. Then the name should be "core"
			if file.Group.ID != nil && *file.Group.ID == coreGroupID {
				logger.Infof("Checking core group name for ID: %s", rf.GetUIDNumber())
				o.Expect(rf.GetGIDName()).To(o.Equal("core"),
					"The group name who owns file %s is wrong! Group name for Id %s should be 'core'",
					file.Path, rf.GetGIDNumber())
			}

			// Verify that if the group ID is defined and its value is the adm group's one. Then the name should be "adm"
			if file.Group.ID != nil && *file.Group.ID == admGroupID {
				logger.Infof("Checking adm group name: %s", rf.GetUIDNumber())
				o.Expect(rf.GetGIDName()).To(o.Equal("adm"),
					"The group name who owns file %s is wrong! Group name for Id %s should be 'adm'",
					file.Path, rf.GetGIDNumber())
			}

			logger.Infof("Checking file permissions: %s", rf.GetNpermissions())
			decimalPerm := ConvertOctalPermissionsToDecimalOrFail(rf.GetNpermissions())
			o.Expect(decimalPerm).To(o.Equal(*file.Mode),
				"The permssions of file %s are wrong", file.Path)

			logger.Infof("OK!\n")
		}

	})
})

// validate that the machine config 'mc' degrades machineconfigpool 'mcp', due to NodeDegraded error matching xpectedNDStatus, expectedNDMessage, expectedNDReason
func validateMcpNodeDegraded(mc *MachineConfig, mcp *MachineConfigPool, expectedNDMessage, expectedNDReason string) {

	oc := mcp.oc
	defer mcp.RecoverFromDegraded()
	defer mc.deleteNoWait()
	mc.create()
	logger.Infof("OK!\n")

	g.By("Wait until MCP becomes degraded")
	o.Eventually(mcp.pollDegradedStatus(), "10m", "30s").Should(o.Equal("True"),
		"The '%s' MCP should become degraded when we try to create an invalid MC, but it didn't.", mcp.GetName())
	o.Eventually(mcp.pollDegradedMachineCount(), "5m", "30s").Should(o.Equal("1"),
		"The '%s' MCP should report '1' degraded machine count, but it doesn't.", mcp.GetName())

	logger.Infof("OK!\n")

	g.By("Validate the reported error")
	nodeDegradedCondition := mcp.GetConditionByType("NodeDegraded")

	nodeDegradedConditionJSON := JSON(nodeDegradedCondition)

	nodeDegradedStatus := nodeDegradedConditionJSON.Get("status").ToString()
	nodeDegradedMessage := nodeDegradedConditionJSON.Get("message").ToString()
	nodeDegradedReason := nodeDegradedConditionJSON.Get("reason").ToString()

	o.ExpectWithOffset(1, nodeDegradedStatus).Should(o.Equal("True"),
		"'worker' MCP should report degraded status in the NodeDegraded condition: %s", nodeDegradedCondition)
	o.ExpectWithOffset(1, nodeDegradedMessage).Should(o.MatchRegexp(expectedNDMessage),
		"'worker' MCP is not reporting the expected message in the NodeDegraded condition: %s", nodeDegradedCondition)
	o.ExpectWithOffset(1, nodeDegradedReason).Should(o.MatchRegexp(expectedNDReason),
		"'worker' MCP is not reporting the expected reason in the NodeDegraded condition: %s", nodeDegradedCondition)
	logger.Infof("OK!\n")

	//
	g.By("Get co machine config to verify status and reason for Upgradeable type")
	var mcDataMap map[string]interface{}

	// If the degraded node is true, then co/machine-config should not be upgradeable
	// It's unlikely, but it can happen that the MCP is degraded, but the CO has not been already updated with the right error message.
	// So we need to poll for the right reason
	o.Eventually(func() string {
		var err error
		mcDataMap, err = getStatusCondition(oc, "co/machine-config", "Upgradeable")
		if err != nil {
			return ""
		}
		return mcDataMap["reason"].(string)
	}, "5m", "10s").Should(o.Equal("DegradedPool"),
		"co/machine-config Upgradeable condition reason is not the expected one: %s", mcDataMap)

	o.ExpectWithOffset(1, mcDataMap["status"].(string)).Should(
		o.Equal("False"),
		"co/machine-config Upgradeable condition status is not the expected one: %s", mcDataMap)
	o.ExpectWithOffset(1, mcDataMap["message"].(string)).Should(
		o.ContainSubstring("One or more machine config pools are degraded, please see `oc get mcp` for further details and resolve before upgrading"),
		"co/machine-config Upgradeable condition message is not the expected one: %s", mcDataMap)

	logger.Infof("OK!\n")

}

func createMcAndVerifyMCValue(oc *exutil.CLI, stepText, mcName string, workerNode Node, textToVerify TextToVerify, cmd ...string) {
	g.By(fmt.Sprintf("Create new MC to add the %s", stepText))
	mcTemplate := mcName + ".yaml"
	mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
	defer mc.delete()
	mc.create()
	logger.Infof("Machine config is created successfully!")

	g.By(fmt.Sprintf("Check %s in the created machine config", stepText))
	mcOut, err := getMachineConfigDetails(oc, mc.name)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(mcOut).Should(o.MatchRegexp(textToVerify.textToVerifyForMC))
	logger.Infof("%s is verified in the created machine config!", stepText)

	g.By(fmt.Sprintf("Check %s in the machine config daemon", stepText))
	var podOut string
	if textToVerify.needBash {
		podOut, err = exutil.RemoteShPodWithBash(oc, MachineConfigNamespace, workerNode.GetMachineConfigDaemon(), cmd...)
	} else if textToVerify.needChroot {
		podOut, err = exutil.RemoteShPodWithChroot(oc, MachineConfigNamespace, workerNode.GetMachineConfigDaemon(), cmd...)
	} else {
		podOut, err = exutil.RemoteShPod(oc, MachineConfigNamespace, workerNode.GetMachineConfigDaemon(), cmd...)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podOut).Should(o.MatchRegexp(textToVerify.textToVerifyForNode))
	logger.Infof("%s is verified in the machine config daemon!", stepText)
}

// skipTestIfClusterVersion skips the test case if the provided version matches the constraints.
func skipTestIfClusterVersion(oc *exutil.CLI, operator, constraintVersion string) {
	clusterVersion, _, err := exutil.GetClusterVersion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())

	if CompareVersions(clusterVersion, operator, constraintVersion) {
		g.Skip(fmt.Sprintf("Test case skipped because current cluster version %s %s %s",
			clusterVersion, operator, constraintVersion))
	}
}

// skipTestIfOsIsNotCoreOs it will either skip the test case in case of worker node is not CoreOS or will return the CoreOS worker node
func skipTestIfOsIsNotCoreOs(oc *exutil.CLI) Node {
	allCoreOs := NewNodeList(oc).GetAllCoreOsWokerNodesOrFail()
	if len(allCoreOs) == 0 {
		g.Skip("CoreOs is required to execute this test case!")
	}
	return allCoreOs[0]
}

// skipTestIfOsIsNotCoreOs it will either skip the test case in case of worker node is not CoreOS or will return the CoreOS worker node
func skipTestIfOsIsNotRhelOs(oc *exutil.CLI) Node {
	allRhelOs := NewNodeList(oc).GetAllRhelWokerNodesOrFail()
	if len(allRhelOs) == 0 {
		g.Skip("RhelOs is required to execute this test case!")
	}
	return allRhelOs[0]
}

// skipTestIfFIPSIsNotEnabled skip the test if fips is not enabled
func skipTestIfFIPSIsNotEnabled(oc *exutil.CLI) {
	if !isFIPSEnabledInClusterConfig(oc) {
		g.Skip("fips is not enabled, skip this test")
	}
}

func createMcAndVerifyIgnitionVersion(oc *exutil.CLI, stepText, mcName, ignitionVersion string) {
	g.By(fmt.Sprintf("Create machine config with %s", stepText))
	mcTemplate := "change-worker-ign-version.yaml"
	mc := NewMachineConfig(oc.AsAdmin(), mcName, MachineConfigPoolWorker).SetMCOTemplate(mcTemplate)
	mc.parameters = []string{"IGNITION_VERSION=" + ignitionVersion}
	defer mc.delete()
	mc.create()

	g.By("Get mcp/worker status to check whether it is degraded")
	mcpDataMap, err := getStatusCondition(oc, "mcp/worker", "RenderDegraded")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(mcpDataMap).NotTo(o.BeNil())
	o.Expect(mcpDataMap["status"].(string)).Should(o.Equal("True"))
	o.Expect(mcpDataMap["message"].(string)).Should(o.ContainSubstring("parsing Ignition config failed: unknown version. Supported spec versions: 2.2, 3.0, 3.1, 3.2"))

	g.By("Get co machine config to verify status and reason for Upgradeable type")
	mcDataMap, err := getStatusCondition(oc, "co/machine-config", "Upgradeable")
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(mcDataMap).NotTo(o.BeNil())
	o.Expect(mcDataMap["status"].(string)).Should(o.Equal("False"))
	o.Expect(mcDataMap["message"].(string)).Should(o.ContainSubstring("One or more machine config pools are degraded, please see `oc get mcp` for further details and resolve before upgrading"))
}

// verifyRenderedMcs verifies that the resources provided in the parameter "allRes" have created a
// a new MachineConfig owned by those resources
func verifyRenderedMcs(oc *exutil.CLI, renderSuffix string, allRes []ResourceInterface) []Resource {
	// TODO: Use MachineConfigList when MC code is refactored
	allMcs, err := NewResourceList(oc.AsAdmin(), "mc").GetAll()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(allMcs).NotTo(o.BeEmpty())

	// cache all MCs owners to avoid too many oc binary executions while searching
	mcOwners := make(map[Resource]*JSONData, len(allMcs))
	for _, mc := range allMcs {
		owners := JSON(mc.GetOrFail(`{.metadata.ownerReferences}`))
		mcOwners[mc] = owners
	}

	// Every resource should own one MC
	for _, res := range allRes {
		var ownedMc *Resource
		for mc, owners := range mcOwners {
			if owners.Exists() {
				for _, owner := range owners.Items() {
					if !(strings.EqualFold(owner.Get("kind").ToString(), res.GetKind()) && strings.EqualFold(owner.Get("name").ToString(), res.GetName())) {
						continue
					}
					logger.Infof("Resource '%s' '%s' owns MC '%s'", res.GetKind(), res.GetName(), mc.GetName())
					// Each resource can only own one MC
					o.Expect(ownedMc).To(o.BeNil(), "Resource %s owns more than 1 MC: %s and %s", res.GetName(), mc.GetName(), ownedMc)
					// we need to do this to avoid the loop variable to override our value
					key := mc
					ownedMc = &key
					break
				}
			} else {
				logger.Infof("MC '%s' has no owner.", mc.name)
			}

		}
		o.Expect(ownedMc).NotTo(o.BeNil(), fmt.Sprintf("Resource '%s' '%s' should have generated a MC but it has not. It owns no MC.", res.GetKind(), res.GetName()))
		o.Expect(ownedMc.name).To(o.ContainSubstring(renderSuffix), "Mc '%s' is owned by '%s' '%s' but its name does not contain the expected substring '%s'",
			ownedMc.GetName(), res.GetKind(), res.GetName(), renderSuffix)
	}

	return allMcs
}

func verifyDriftConfig(mcp *MachineConfigPool, rf *RemoteFile, newMode string, forceFile bool) {
	workerNode := rf.node
	origContent := rf.content
	origMode := rf.GetNpermissions()

	g.By("Modify file content and check degraded status")
	newContent := origContent + "Extra Info"
	o.Expect(rf.PushNewTextContent(newContent)).NotTo(o.HaveOccurred())
	rferr := rf.Fetch()
	o.Expect(rferr).NotTo(o.HaveOccurred())

	o.Expect(rf.GetTextContent()).To(o.Equal(newContent), "File content should be updated")
	o.Eventually(mcp.pollDegradedMachineCount(), "10m", "30s").Should(o.Equal("1"), "There should be 1 degraded machine")
	o.Eventually(mcp.pollDegradedStatus(), "10m", "30s").Should(o.Equal("True"), "The worker MCP should report a True Degraded status")
	o.Eventually(mcp.pollUpdatedStatus(), "10m", "30s").Should(o.Equal("False"), "The worker MCP should report a False Updated status")

	g.By("Verify that node annotations describe the reason for the Degraded status")
	reason := workerNode.GetAnnotationOrFail("machineconfiguration.openshift.io/reason")
	o.Expect(reason).To(o.Equal(fmt.Sprintf(`content mismatch for file "%s"`, rf.fullPath)))

	if forceFile {
		g.By("Restore original content using the ForceFile and wait until pool is ready again")
		o.Expect(workerNode.ForceReapplyConfiguration()).NotTo(o.HaveOccurred())
	} else {
		g.By("Restore original content manually and wait until pool is ready again")
		o.Expect(rf.PushNewTextContent(origContent)).NotTo(o.HaveOccurred())
	}

	o.Eventually(mcp.pollDegradedMachineCount(), "10m", "30s").Should(o.Equal("0"), "There should be no degraded machines")
	o.Eventually(mcp.pollDegradedStatus(), "10m", "30s").Should(o.Equal("False"), "The worker MCP should report a False Degraded status")
	o.Eventually(mcp.pollUpdatedStatus(), "10m", "30s").Should(o.Equal("True"), "The worker MCP should report a True Updated status")
	rferr = rf.Fetch()
	o.Expect(rferr).NotTo(o.HaveOccurred())
	o.Expect(rf.GetTextContent()).To(o.Equal(origContent), "Original file content should be restored")

	g.By("Verify that node annotations have been cleaned")
	reason = workerNode.GetAnnotationOrFail("machineconfiguration.openshift.io/reason")
	o.Expect(reason).To(o.Equal(``))

	g.By(fmt.Sprintf("Manually modify the file permissions to %s", newMode))
	o.Expect(rf.PushNewPermissions(newMode)).NotTo(o.HaveOccurred())
	rferr = rf.Fetch()
	o.Expect(rferr).NotTo(o.HaveOccurred())

	o.Expect(rf.GetNpermissions()).To(o.Equal(newMode), "%s File permissions should be %s", rf.fullPath, newMode)
	o.Eventually(mcp.pollDegradedMachineCount(), "10m", "30s").Should(o.Equal("1"), "There should be 1 degraded machine")
	o.Eventually(mcp.pollDegradedStatus(), "10m", "30s").Should(o.Equal("True"), "The worker MCP should report a True Degraded status")
	o.Eventually(mcp.pollUpdatedStatus(), "10m", "30s").Should(o.Equal("False"), "The worker MCP should report a False Updated status")

	g.By("Verify that node annotations describe the reason for the Degraded status")
	reason = workerNode.GetAnnotationOrFail("machineconfiguration.openshift.io/reason")
	o.Expect(reason).To(o.MatchRegexp(fmt.Sprintf(`mode mismatch for file: "%s"; expected: .+/%s; received: .+/%s`, rf.fullPath, origMode, newMode)))

	g.By("Restore the original file permissions")
	o.Expect(rf.PushNewPermissions(origMode)).NotTo(o.HaveOccurred())
	rferr = rf.Fetch()
	o.Expect(rferr).NotTo(o.HaveOccurred())

	o.Expect(rf.GetNpermissions()).To(o.Equal(origMode), "%s File permissions should be %s", rf.fullPath, origMode)
	o.Eventually(mcp.pollDegradedMachineCount(), "10m", "30s").Should(o.Equal("0"), "There should be no degraded machines")
	o.Eventually(mcp.pollDegradedStatus(), "10m", "30s").Should(o.Equal("False"), "The worker MCP should report a False Degraded status")
	o.Eventually(mcp.pollUpdatedStatus(), "10m", "30s").Should(o.Equal("True"), "The worker MCP should report a True Updated status")

	g.By("Verify that node annotations have been cleaned")
	reason = workerNode.GetAnnotationOrFail("machineconfiguration.openshift.io/reason")
	o.Expect(reason).To(o.Equal(``))
}

// checkUpdatedLists Compares that 2 lists are ordered in steps.
// when we update nodes with maxUnavailable>1, since we are polling, we cannot make sure
// that the sorted lists have the same order one by one. We can only make sure that the steps
// defined by maxUnavailable have the right order.
// If step=1, it is the same as comparing that both lists are equal.
func checkUpdatedLists(l, r []Node, step int) bool {
	if len(l) != len(r) {
		logger.Errorf("Compared lists have different size")
		return false
	}

	indexStart := 0
	for i := 0; i < len(l); i += step {
		indexEnd := i + step
		if (i + step) > (len(l)) {
			indexEnd = len(l)
		}

		// Create 2 sublists with the size of the step
		stepL := l[indexStart:indexEnd]
		stepR := r[indexStart:indexEnd]
		indexStart += step

		// All elements in one sublist should exist in the other one
		// but they dont have to be in the same order.
		for _, nl := range stepL {
			found := false
			for _, nr := range stepR {
				if nl.GetName() == nr.GetName() {
					found = true
					break
				}

			}
			if !found {
				logger.Errorf("Nodes were not updated in the right order. Comparing steps %s and %s\n", stepL, stepR)
				return false
			}
		}

	}
	return true

}
