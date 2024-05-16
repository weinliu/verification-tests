package nto

import (
	"fmt"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-node] PSAP should", func() {
	defer g.GinkgoRecover()

	var (
		oc                            = exutil.NewCLI("nto-test", exutil.KubeConfigPath())
		ntoNamespace                  = "openshift-cluster-node-tuning-operator"
		overrideFile                  = exutil.FixturePath("testdata", "psap", "nto", "override.yaml")
		podTestFile                   = exutil.FixturePath("testdata", "psap", "nto", "pod_test.yaml")
		podNginxFile                  = exutil.FixturePath("testdata", "psap", "nto", "pod-nginx.yaml")
		tunedNFConntrackMaxFile       = exutil.FixturePath("testdata", "psap", "nto", "tuned-nf-conntrack-max.yaml")
		hPPerformanceProfileFile      = exutil.FixturePath("testdata", "psap", "nto", "hp-performanceprofile.yaml")
		hpPerformanceProfilePatchFile = exutil.FixturePath("testdata", "psap", "nto", "hp-performanceprofile-patch.yaml")

		cgroupSchedulerBacklist      = exutil.FixturePath("testdata", "psap", "nto", "cgroup-scheduler-blacklist.yaml")
		cgroupSchedulerBestEffortPod = exutil.FixturePath("testdata", "psap", "nto", "cgroup-scheduler-besteffor-pod.yaml")
		ntoTunedDebugFile            = exutil.FixturePath("testdata", "psap", "nto", "nto-tuned-debug.yaml")
		ntoIRQSMPFile                = exutil.FixturePath("testdata", "psap", "nto", "default-irq-smp-affinity.yaml")
		ntoRealtimeFile              = exutil.FixturePath("testdata", "psap", "nto", "realtime.yaml")
		ntoMCPFile                   = exutil.FixturePath("testdata", "psap", "nto", "machine-config-pool.yaml")
		IPSFile                      = exutil.FixturePath("testdata", "psap", "nto", "ips.yaml")
		workerStackFile              = exutil.FixturePath("testdata", "psap", "nto", "worker-stack-tuned.yaml")
		paoPerformanceFile           = exutil.FixturePath("testdata", "psap", "pao", "pao-performanceprofile.yaml")
		paoPerformancePatchFile      = exutil.FixturePath("testdata", "psap", "pao", "pao-performance-patch.yaml")
		paoPerformanceFixpatchFile   = exutil.FixturePath("testdata", "psap", "pao", "pao-performance-fixpatch.yaml")
		paoPerformanceOptimizeFile   = exutil.FixturePath("testdata", "psap", "pao", "pao-performance-optimize.yaml")
		paoIncludePerformanceProfile = exutil.FixturePath("testdata", "psap", "pao", "pao-include-performance-profile.yaml")
		paoWorkerCnfMCPFile          = exutil.FixturePath("testdata", "psap", "pao", "pao-workercnf-mcp.yaml")
		paoWorkerOptimizeMCPFile     = exutil.FixturePath("testdata", "psap", "pao", "pao-workeroptimize-mcp.yaml")
		hugepage100MPodFile          = exutil.FixturePath("testdata", "psap", "nto", "hugepage-100m-pod.yaml")
		hugepageMCPfile              = exutil.FixturePath("testdata", "psap", "nto", "hugepage-mcp.yaml")
		hugepageTunedBoottimeFile    = exutil.FixturePath("testdata", "psap", "nto", "hugepage-tuned-boottime.yaml")
		stalldTunedFile              = exutil.FixturePath("testdata", "psap", "nto", "stalld-tuned.yaml")
		openshiftNodePostgresqlFile  = exutil.FixturePath("testdata", "psap", "nto", "openshift-node-postgresql.yaml")
		netPluginFile                = exutil.FixturePath("testdata", "psap", "nto", "net-plugin-tuned.yaml")
		cloudProviderFile            = exutil.FixturePath("testdata", "psap", "nto", "cloud-provider-profile.yaml")
		nodeDiffCPUsTunedBootFile    = exutil.FixturePath("testdata", "psap", "nto", "node-diffcpus-tuned-bootloader.yaml")
		nodeDiffCPUsMCPFile          = exutil.FixturePath("testdata", "psap", "nto", "node-diffcpus-mcp.yaml")
		tuningMaxPidFile             = exutil.FixturePath("testdata", "psap", "nto", "tuning-maxpid.yaml")

		isNTO              bool
		isPAOInstalled     bool
		paoNamespace       = "openshift-performance-addon-operator"
		iaasPlatform       string
		ManualPickup       bool
		podShippedFile     string
		podSysctlFile      string
		ntoTunedPidMax     string
		customTunedProfile string
		tunedNodeName      string
		ntoSysctlTemplate  string

		err error
	)

	g.BeforeEach(func() {
		// ensure NTO operator is installed
		isNTO = isNTOPodInstalled(oc, ntoNamespace)
		// get IaaS platform
		iaasPlatform = exutil.CheckPlatform(oc)
		e2e.Logf("Cloud provider is: %v", iaasPlatform)
		ManualPickup = false

		podShippedFile = exutil.FixturePath("testdata", "psap", "nto", "pod-shipped.yaml")
		podSysctlFile = exutil.FixturePath("testdata", "psap", "nto", "nto-sysctl-pod.yaml")
		ntoTunedPidMax = exutil.FixturePath("testdata", "psap", "nto", "nto-tuned-pidmax.yaml")
		customTunedProfile = exutil.FixturePath("testdata", "psap", "nto", "custom-tuned-profiles.yaml")
		ntoSysctlTemplate = exutil.FixturePath("testdata", "psap", "nto", "nto-sysctl-template.yaml")
	})

	// author: liqcui@redhat.com
	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-29789-Sysctl parameters that set by tuned can be overwritten by parameters set via /etc/sysctl [Flaky]", func() {

		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		exutil.By("Pick one worker node and one tuned pod on same node")
		workerNodeName, err := exutil.GetFirstLinuxWorkerNode(oc)
		o.Expect(workerNodeName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Worker Node: %v", workerNodeName)
		tunedPodName, err := exutil.GetPodName(oc, ntoNamespace, "openshift-app=tuned", workerNodeName)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		exutil.By("Check values set by /etc/sysctl on node and store the values")
		inotify, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "cat", "/etc/sysctl.d/inotify.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(inotify).To(o.And(
			o.ContainSubstring("fs.inotify.max_user_watches"),
			o.ContainSubstring("fs.inotify.max_user_instances")))
		maxUserWatchesValue := getMaxUserWatchesValue(inotify)
		maxUserInstancesValue := getMaxUserInstancesValue(inotify)
		e2e.Logf("fs.inotify.max_user_watches has value of: %v", maxUserWatchesValue)
		e2e.Logf("fs.inotify.max_user_instances has value of: %v", maxUserInstancesValue)

		exutil.By("Mount /etc/sysctl on node")
		_, err = exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "mount")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check sysctl kernel.pid_max on node and store the value")
		kernel, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "sysctl", "kernel.pid_max")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kernel).To(o.ContainSubstring("kernel.pid_max"))
		pidMaxValue := getKernelPidMaxValue(kernel)
		e2e.Logf("kernel.pid_max has value of: %v", pidMaxValue)

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoNamespace, "tuneds.tuned.openshift.io", "override").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", workerNodeName, "tuned.openshift.io/override-").Execute()

		//tuned can not override parameters set via /etc/sysctl{.conf,.d} when reapply_sysctl=true
		//  The settings in /etc/sysctl.d/inotify.conf as below
		//      fs.inotify.max_user_watches = 65536     =>Try to override to 163840 by tuned, expect the old value 65536
		//      fs.inotify.max_user_instances = 8192    =>Not override by tuned, expect the old value 8192
		//      kernel.pid_max = 4194304                =>Default value is 4194304
		//  The settings in custom tuned profile as below
		//      fs.inotify.max_user_watches = 163840    =>Try to override to 163840 by tuned, expect the old value 65536
		//      kernel.pid_max = 1048576                =>Override by tuned, expect the new value 1048576
		exutil.By("Create new NTO CR with reapply_sysctl=true and label the node")
		//reapply_sysctl=true tuned can not override parameters set via /etc/sysctl{.conf,.d}
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", workerNodeName, "tuned.openshift.io/override=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", overrideFile, "REAPPLY_SYSCTL=true")

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, workerNodeName, "override")

		exutil.By("Check value of fs.inotify.max_user_instances on node (set by sysctl, should be the same as before), expected value is 8192")
		maxUserInstanceCheck, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "sysctl", "fs.inotify.max_user_instances")
		e2e.Logf("fs.inotify.max_user_instances has value of: %v", maxUserInstanceCheck)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(maxUserInstanceCheck).To(o.ContainSubstring(maxUserInstancesValue))

		exutil.By("Check value of fs.inotify.max_user_watches on node (set by sysctl, should be the same as before),expected value is 65536")
		maxUserWatchesCheck, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "sysctl", "fs.inotify.max_user_watches")
		e2e.Logf("fs.inotify.max_user_watches has value of: %v", maxUserWatchesCheck)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(maxUserWatchesCheck).To(o.ContainSubstring(maxUserWatchesValue))

		exutil.By("Check value of kernel.pid_max on node (set by override tuned, should be the same value of override custom profile), expected value is 1048576")
		pidMaxCheck, _, err := exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "sysctl", "kernel.pid_max")
		e2e.Logf("kernel.pid_max has value of: %v", pidMaxCheck)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pidMaxCheck).To(o.ContainSubstring("kernel.pid_max = 1048576"))

		//tuned can override parameters set via /etc/sysctl{.conf,.d} when reapply_sysctl=false
		//  The settings in /etc/sysctl.d/inotify.conf as below
		//      fs.inotify.max_user_watches = 65536     =>Try to override to 163840 by tuned, expect the old value 163840
		//      fs.inotify.max_user_instances = 8192    =>Not override by tuned, expect the old value 8192
		//      kernel.pid_max = 4194304                =>Default value is 4194304
		//  The settings in custom tuned profile as below
		//      fs.inotify.max_user_watches = 163840    =>Try to override to 163840 by tuned, expect the old value 163840
		//      kernel.pid_max = 1048576                =>Override by tuned, expect the new value 1048576

		exutil.By("Create new CR with reapply_sysctl=true")
		//reapply_sysctl=true tuned can not override parameters set via /etc/sysctl{.conf,.d}
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", overrideFile, "REAPPLY_SYSCTL=false")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check value of fs.inotify.max_user_instances on node (set by sysctl, should be the same as before),expected value is 8192")
		maxUserInstanceCheck, _, err = exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "sysctl", "fs.inotify.max_user_instances")
		e2e.Logf("fs.inotify.max_user_instances has value of: %v", maxUserInstanceCheck)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(maxUserInstanceCheck).To(o.ContainSubstring(maxUserInstanceCheck))

		exutil.By("Check value of fs.inotify.max_user_watches on node (set by sysctl, should be the same value of override custom profile), expected value is 163840")
		maxUserWatchesCheck, _, err = exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "sysctl", "fs.inotify.max_user_watches")
		e2e.Logf("fs.inotify.max_user_watches has value of: %v", maxUserWatchesCheck)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(maxUserWatchesCheck).To(o.ContainSubstring("fs.inotify.max_user_watches = 163840"))

		exutil.By("Check value of kernel.pid_max on node (set by override tuned, should be the same value of override custom profile), expected value is 1048576")
		pidMaxCheck, _, err = exutil.DebugNodeWithOptionsAndChrootWithoutRecoverNsLabel(oc, workerNodeName, []string{"-q"}, "sysctl", "kernel.pid_max")
		e2e.Logf("kernel.pid_max has value of: %v", pidMaxCheck)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pidMaxCheck).To(o.ContainSubstring("kernel.pid_max = 1048576"))

	})

	// author: nweinber@redhat.com
	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-33237-Test NTO support for operatorapi Unmanaged state [Flaky]", func() {

		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		defer func() {
			exutil.By("Remove custom profile (if not already removed) and patch default tuned back to Managed")
			_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoNamespace, "tuned", "nf-conntrack-max", "--ignore-not-found").Execute()
			_ = patchTunedState(oc, ntoNamespace, "default", "Managed")
		}()

		isSNO := exutil.IsSNOCluster(oc)
		is3Master := exutil.Is3MasterNoDedicatedWorkerNode(oc)
		var profileCheck string

		masterNodeName := getFirstMasterNodeName(oc)
		defaultMasterProfileName := getDefaultProfileNameOnMaster(oc, masterNodeName)

		exutil.By("Create logging namespace")
		oc.SetupProject()
		loggingNamespace := oc.Namespace()

		exutil.By("Patch default tuned to 'Unmanaged'")
		err := patchTunedState(oc, ntoNamespace, "default", "Unmanaged")
		o.Expect(err).NotTo(o.HaveOccurred())
		state, err := getTunedState(oc, ntoNamespace, "default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.Equal("Unmanaged"))

		exutil.By("Create new pod from CR and label it")
		exutil.CreateNsResourceFromTemplate(oc, loggingNamespace, "--ignore-unknown-parameters=true", "-f", podTestFile)
		err = exutil.LabelPod(oc, loggingNamespace, "web", "tuned.openshift.io/elasticsearch=")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Wait for pod web is ready")
		exutil.AssertPodToBeReady(oc, "web", loggingNamespace)

		exutil.By("Get the tuned node and pod names")
		tunedNodeName, err := exutil.GetPodNodeName(oc, loggingNamespace, "web")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Tuned Node: %v", tunedNodeName)
		tunedPodName, err := exutil.GetPodName(oc, ntoNamespace, "openshift-app=tuned", tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		exutil.By("Create new profile from CR")
		exutil.CreateNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", tunedNFConntrackMaxFile)

		exutil.By("All node's current profile is:")
		stdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Profile Name Per Nodes: %v", stdOut)

		logsCheck, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ntoNamespace, "--tail=9", tunedPodName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(logsCheck).NotTo(o.ContainSubstring("nf-conntrack-max"))

		if isSNO || is3Master {
			profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(profileCheck).To(o.Equal(defaultMasterProfileName))
		} else {
			profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(profileCheck).To(o.Equal("openshift-node"))
		}

		nodeList, err := exutil.GetAllNodesbyOSType(oc, "linux")
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeListSize := len(nodeList)
		for i := 0; i < nodeListSize; i++ {
			output, err := exutil.DebugNodeWithChroot(oc, nodeList[i], "sysctl", "net.netfilter.nf_conntrack_max")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("net.netfilter.nf_conntrack_max = 1048576"))
		}

		exutil.By("Remove custom profile and pod and patch default tuned back to Managed")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoNamespace, "tuned", "nf-conntrack-max").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", loggingNamespace, "pod", "web").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = patchTunedState(oc, ntoNamespace, "default", "Managed")
		o.Expect(err).NotTo(o.HaveOccurred())
		state, err = getTunedState(oc, ntoNamespace, "default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.Equal("Managed"))

		exutil.By("Create new pod from CR and label it")
		exutil.CreateNsResourceFromTemplate(oc, loggingNamespace, "--ignore-unknown-parameters=true", "-f", podTestFile)
		err = exutil.LabelPod(oc, loggingNamespace, "web", "tuned.openshift.io/elasticsearch=")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get the tuned node and pod names")
		tunedNodeName, err = exutil.GetPodNodeName(oc, loggingNamespace, "web")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Tuned Node: %v", tunedNodeName)
		tunedPodName, err = exutil.GetPodName(oc, ntoNamespace, "openshift-app=tuned", tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Tuned Pod: %v", tunedPodName)

		exutil.By("Create new profile from CR")
		exutil.CreateNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", tunedNFConntrackMaxFile)

		exutil.By("All node's current profile is:")
		stdOut, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Profile Name Per Nodes: %v", stdOut)

		exutil.By("Assert nf-conntrack-max applied to the node that web application run on it.")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "nf-conntrack-max")

		profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("nf-conntrack-max"))

		exutil.By("All node's current profile is:")
		stdOut, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Profile Name Per Nodes: %v", stdOut)

		// tuned nodes should have value of 1048578, others should be 1048576
		for i := 0; i < nodeListSize; i++ {
			output, err := exutil.DebugNodeWithChroot(oc, nodeList[i], "sysctl", "net.netfilter.nf_conntrack_max")
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodeList[i] == tunedNodeName {
				o.Expect(output).To(o.ContainSubstring("net.netfilter.nf_conntrack_max = 1048578"))
			} else {
				o.Expect(output).To(o.ContainSubstring("net.netfilter.nf_conntrack_max = 1048576"))
			}
		}

		exutil.By("Change tuned state back to Unmanaged and delete custom tuned")
		err = patchTunedState(oc, ntoNamespace, "default", "Unmanaged")
		o.Expect(err).NotTo(o.HaveOccurred())
		state, err = getTunedState(oc, ntoNamespace, "default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.Equal("Unmanaged"))
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoNamespace, "tuned", "nf-conntrack-max").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("nf-conntrack-max"))

		logsCheck, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ntoNamespace, "--tail=9", tunedPodName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(logsCheck).To(o.ContainSubstring("tuned.daemon.daemon: static tuning from profile 'nf-conntrack-max' applied"))

		exutil.By("All node's current profile is:")
		stdOut, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Profile Name Per Nodes: %v", stdOut)

		// tuned nodes should have value of 1048578, others should be 1048576
		for i := 0; i < nodeListSize; i++ {
			output, err := exutil.DebugNodeWithChroot(oc, nodeList[i], "sysctl", "net.netfilter.nf_conntrack_max")
			o.Expect(err).NotTo(o.HaveOccurred())
			if nodeList[i] == tunedNodeName {
				o.Expect(output).To(o.ContainSubstring("net.netfilter.nf_conntrack_max = 1048578"))
			} else {
				o.Expect(output).To(o.ContainSubstring("net.netfilter.nf_conntrack_max = 1048576"))
			}
		}

		exutil.By("Changed tuned state back to Managed")
		err = patchTunedState(oc, ntoNamespace, "default", "Managed")
		o.Expect(err).NotTo(o.HaveOccurred())
		state, err = getTunedState(oc, ntoNamespace, "default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.Equal("Managed"))

		if isSNO || is3Master {
			assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, defaultMasterProfileName)
			profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(profileCheck).To(o.Equal(defaultMasterProfileName))
		} else {
			assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node")
			profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(profileCheck).To(o.Equal("openshift-node"))
		}

		exutil.By("All node's current profile is:")
		stdOut, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Profile Name Per Nodes: %v", stdOut)

		for i := 0; i < nodeListSize; i++ {
			output, err := exutil.DebugNodeWithChroot(oc, nodeList[i], "sysctl", "net.netfilter.nf_conntrack_max")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("net.netfilter.nf_conntrack_max = 1048576"))
		}
	})

	// author: nweinber@redhat.com
	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-36881-Node Tuning Operator will provide machine config for the master machine config pool [Disruptive] [Slow]", func() {

		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)
		isOneMasterwithNWorker := exutil.IsOneMasterWithNWorkerNodes(oc)

		if !isNTO || isSNO || isOneMasterwithNWorker {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		if ManualPickup {
			g.Skip("This is the test case that execute mannually in shared cluster ...")
		}

		defer func() {
			exutil.By("Remove new tuning profile after test completion")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoNamespace, "tuneds.tuned.openshift.io", "openshift-node-performance-hp-performanceprofile").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("Add new tuning profile from CR")
		exutil.CreateNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", hPPerformanceProfileFile)

		exutil.By("Verify new tuned profile was created")
		profiles, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profiles).To(o.ContainSubstring("openshift-node-performance-hp-performanceprofile"))

		exutil.By("Get NTO pod name and check logs for priority warning")
		ntoPodName, err := getNTOPodName(oc, ntoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("NTO pod name: %v", ntoPodName)
		//ntoPodLogs, err := exutil.GetSpecificPodLogs(oc, ntoNamespace, "", ntoPodName, "")
		assertNTOPodLogsLastLines(oc, ntoNamespace, ntoPodName, "10", 180, `openshift-node-performance-hp-performanceprofile have the same priority 30.*please use a different priority for your custom profiles`)

		//o.Expect(err).NotTo(o.HaveOccurred())
		//o.Expect(ntoPodLogs).To(o.ContainSubstring("profiles openshift-control-plane/openshift-node-performance-hp-performanceprofile have the same priority 30, please use a different priority for your custom profiles!"))

		exutil.By("Patch priority for openshift-node-performance-hp-performanceprofile tuned to 18")
		err = patchTunedProfile(oc, ntoNamespace, "openshift-node-performance-hp-performanceprofile", hpPerformanceProfilePatchFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		tunedPriority, err := getTunedPriority(oc, ntoNamespace, "openshift-node-performance-hp-performanceprofile")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedPriority).To(o.Equal("18"))

		exutil.By("Check Nodes for expected changes")
		masterNodeName := assertIfNodeSchedulingDisabled(oc)
		e2e.Logf("The master node %v has been rebooted", masterNodeName)

		exutil.By("Check MachineConfigPool for expected changes")
		exutil.AssertIfMCPChangesAppliedByName(oc, "master", 1800)

		exutil.By("Ensure the settings took effect on the master nodes, only check the first rebooted nodes")
		assertIfMasterNodeChangesApplied(oc, masterNodeName)

		exutil.By("Check MachineConfig kernel arguments for expected changes")
		mcCheck, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mc").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(mcCheck).To(o.ContainSubstring("50-nto-master"))
		mcKernelArgCheck, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("mc/50-nto-master").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(mcKernelArgCheck).To(o.ContainSubstring("default_hugepagesz=2M"))
	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-23959-Test NTO for remove pod in daemon mode [Disruptive]", func() {

		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		ntoRes := ntoResource{
			name:        "kernel-pid-max",
			namespace:   ntoNamespace,
			template:    customTunedProfile,
			sysctlparm:  "kernel.pid_max",
			sysctlvalue: "128888",
		}
		defer func() {
			exutil.By("Remove custom profile (if not already removed) and patch default tuned back to Managed")
			ntoRes.delete(oc)
			_ = patchTunedState(oc, ntoNamespace, "default", "Managed")
		}()

		isSNO := exutil.IsSNOCluster(oc)
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		defer func() {
			exutil.By("Forcily delete labeled pod on first worker node after test case executed in case compareSysctlDifferentFromSpecifiedValueByName step failure")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", tunedPodName, "-n", ntoNamespace, "--ignore-not-found").Execute()
		}()

		exutil.By("Apply new profile from CR")
		ntoRes.createTunedProfileIfNotExist(oc)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check all nodes for kernel.pid_max value, all node should different from 128888")
		compareSysctlDifferentFromSpecifiedValueByName(oc, "kernel.pid_max", "128888")

		exutil.By("Label tuned pod as tuned.openshift.io/elasticsearch=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", tunedPodName, "-n", ntoNamespace, "tuned.openshift.io/elasticsearch=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check current profile for each node")
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)

		exutil.By("Compare if the value kernel.pid_max in on node with labeled pod, should be 128888")
		compareSysctlValueOnSepcifiedNodeByName(oc, tunedNodeName, "kernel.pid_max", "", "128888")

		exutil.By("Delete labeled tuned pod by name")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", tunedPodName, "-n", ntoNamespace).Execute()

		exutil.By("Check all nodes for kernel.pid_max value, all node should different from 128888")
		compareSysctlDifferentFromSpecifiedValueByName(oc, "kernel.pid_max", "128888")

	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-23958-Test NTO for label pod in daemon mode [Disruptive]", func() {

		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		ntoRes := ntoResource{
			name:        "user-max-ipc-namespaces",
			namespace:   ntoNamespace,
			template:    customTunedProfile,
			sysctlparm:  "user.max_ipc_namespaces",
			sysctlvalue: "121112",
		}
		defer func() {
			exutil.By("Remove custom profile (if not already removed) and patch default tuned back to Managed")
			ntoRes.delete(oc)
		}()

		isSNO := exutil.IsSNOCluster(oc)
		//Prior to choose worker nodes with machineset
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		defer func() {
			exutil.By("Forcily remove label from the pod on first worker node in case compareSysctlDifferentFromSpecifiedValueByName step failure")
			err = exutil.LabelPod(oc, ntoNamespace, tunedPodName, "tuned.openshift.io/elasticsearch-")
		}()

		exutil.By("Apply new profile from CR")
		ntoRes.createTunedProfileIfNotExist(oc)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check all nodes for user.max_ipc_namespaces value, all node should different from 121112")
		compareSysctlDifferentFromSpecifiedValueByName(oc, "user.max_ipc_namespaces", "121112")

		exutil.By("Label tuned pod as tuned.openshift.io/elasticsearch=")
		err = exutil.LabelPod(oc, ntoNamespace, tunedPodName, "tuned.openshift.io/elasticsearch=")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check current profile for each node")
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)

		exutil.By("Compare if the value user.max_ipc_namespaces in on node with labeled pod, should be 121112")
		compareSysctlValueOnSepcifiedNodeByName(oc, tunedNodeName, "user.max_ipc_namespaces", "", "121112")

		exutil.By("Remove label from tuned pod as tuned.openshift.io/elasticsearch-")
		err = exutil.LabelPod(oc, ntoNamespace, tunedPodName, "tuned.openshift.io/elasticsearch-")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check all nodes for user.max_ipc_namespaces value, all node should different from 121112")
		compareSysctlDifferentFromSpecifiedValueByName(oc, "user.max_ipc_namespaces", "121112")

	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-43173-NTO Cgroup Blacklist Pod should affine to default cpuset.[Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		isSNO := exutil.IsSNOCluster(oc)
		//Prior to choose worker nodes with machineset
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		//Get how many cpus on the specified worker node
		exutil.By("Get how many cpus cores on the labeled worker node")
		nodeCPUCores, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.capacity.cpu}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeCPUCores).NotTo(o.BeEmpty())

		nodeCPUCoresInt, err := strconv.Atoi(nodeCPUCores)
		o.Expect(err).NotTo(o.HaveOccurred())
		if nodeCPUCoresInt <= 1 {
			g.Skip("the worker node don't have enough cpus - skipping test ...")
		}

		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		exutil.By("Remove custom profile (if not already removed) and remove node label")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "-n", ntoNamespace, "cgroup-scheduler-affinecpuset").Execute()

		defer func() {
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned-scheduler-node-").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		exutil.By("Label the specified linux node with label tuned-scheduler-node")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned-scheduler-node=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// setting cgroup_ps_blacklist=/kubepods\.slice/
		// the process belong the /kubepods\.slice/ can consume all cpuset
		// The expected Cpus_allowed_list in /proc/$PID/status should be 0-N
		// the process doesn't belong the /kubepods\.slice/ can consume all cpuset
		// The expected Cpus_allowed_list in /proc/$PID/status should be 0 or 0,2-N

		exutil.By("Create NTO custom tuned profile cgroup-scheduler-affinecpuset")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", cgroupSchedulerBacklist, "-p", "PROFILE_NAME=cgroup-scheduler-affinecpuset", `CGROUP_BLACKLIST=/kubepods\.slice/`)

		exutil.By("Check if NTO custom tuned profile cgroup-scheduler-affinecpuset was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "cgroup-scheduler-affinecpuset")

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		// The expected Cpus_allowed_list in /proc/$PID/status should be 0-N
		exutil.By("Verified the cpu allow list in cgroup black list for tuned ...")
		o.Expect(assertProcessInCgroupSchedulerBlacklist(oc, tunedNodeName, ntoNamespace, "tuned", nodeCPUCoresInt)).To(o.Equal(true))

		// The expected Cpus_allowed_list in /proc/$PID/status should be 0-N
		exutil.By("Verified the cpu allow list in cgroup black list for chronyd ...")
		o.Expect(assertProcessNOTInCgroupSchedulerBlacklist(oc, tunedNodeName, ntoNamespace, "chronyd", nodeCPUCoresInt)).To(o.Equal(true))

	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-27491-Add own custom profile to tuned operator [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		ntoRes := ntoResource{
			name:        "user-max-mnt-namespaces",
			namespace:   ntoNamespace,
			template:    customTunedProfile,
			sysctlparm:  "user.max_mnt_namespaces",
			sysctlvalue: "142214",
		}

		masterNodeName := getFirstMasterNodeName(oc)
		defaultMasterProfileName := getDefaultProfileNameOnMaster(oc, masterNodeName)

		oc.SetupProject()
		ntoTestNS := oc.Namespace()

		is3CPNoWorker := exutil.Is3MasterNoDedicatedWorkerNode(oc)
		//Clean up the custom profile user-max-mnt-namespaces and unlabel the nginx pod
		defer ntoRes.delete(oc)

		//First choice to use [tests] image, the image mirrored by default in disconnected cluster
		//if don't have [tests] image in some environment, we can use hello-openshift as image
		//usually test imagestream shipped in all ocp and mirror the image in disconnected cluster by default
		// AppImageName := exutil.GetImagestreamImageName(oc, "tests")
		// if len(AppImageName) == 0 {
		AppImageName := "quay.io/openshifttest/nginx-alpine@sha256:04f316442d48ba60e3ea0b5a67eb89b0b667abf1c198a3d0056ca748736336a0"
		// }

		//Create a nginx web application pod
		exutil.By("Create a nginx web pod in nto temp namespace")
		exutil.ApplyNsResourceFromTemplate(oc, ntoTestNS, "--ignore-unknown-parameters=true", "-f", podShippedFile, "-p", "IMAGENAME="+AppImageName)

		//Check if nginx pod is ready
		exutil.AssertPodToBeReady(oc, "nginx", ntoTestNS)

		//Get the node name in the same node as nginx app
		tunedNodeName, err := exutil.GetPodNodeName(oc, ntoTestNS, "nginx")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("tunedNodeName is %v", tunedNodeName)

		//Get the tuned pod name in the same node as nginx app
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		//Label pod nginx with tuned.openshift.io/elasticsearch=
		exutil.By("Label nginx pod as tuned.openshift.io/elasticsearch=")
		err = exutil.LabelPod(oc, ntoTestNS, "nginx", "tuned.openshift.io/elasticsearch=")
		o.Expect(err).NotTo(o.HaveOccurred())

		//Apply new profile that match label tuned.openshift.io/elasticsearch=
		exutil.By("Apply new profile from CR")
		ntoRes.createTunedProfileIfNotExist(oc)

		exutil.By("Check if new profile  user-max-mnt-namespaces applied to labeled node")
		//Verify if the new profile is applied
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "user-max-mnt-namespaces")
		profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("user-max-mnt-namespaces"))

		exutil.By("Assert static tuning from profile 'user-max-mnt-namespaces' applied in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 180, `static tuning from profile 'user-max-mnt-namespaces' applied|active and recommended profile \(user-max-mnt-namespaces\) match`)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Compare if the value user.max_mnt_namespaces in on node with labeled pod, should be 142214")
		compareSysctlValueOnSepcifiedNodeByName(oc, tunedNodeName, "user.max_mnt_namespaces", "", "142214")

		exutil.By("Delete custom tuned profile user.max_mnt_namespaces")
		ntoRes.delete(oc)

		//Check if restore to default profile.
		isSNO := exutil.IsSNOCluster(oc)
		if isSNO || is3CPNoWorker {
			exutil.By("The cluster is SNO or Compact Cluster")
			assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, defaultMasterProfileName)
			exutil.By("Assert default profile applied in tuned pod log")
			assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 180, "'"+defaultMasterProfileName+"' applied|("+defaultMasterProfileName+") match")
			profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(profileCheck).To(o.Equal(defaultMasterProfileName))
		} else {
			exutil.By("The cluster is regular OCP Cluster")
			assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node")
			exutil.By("Assert profile 'openshift-node' applied in tuned pod log")
			assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 180, `static tuning from profile 'openshift-node' applied|active and recommended profile \(openshift-node\) match`)
			profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(profileCheck).To(o.Equal("openshift-node"))
		}

		exutil.By("Check all nodes for user.max_mnt_namespaces value, all node should different from 142214")
		compareSysctlDifferentFromSpecifiedValueByName(oc, "user.max_mnt_namespaces", "142214")
	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-NonPreRelease-Longduration-Author:liqcui-Medium-37125-Turning on debugging for tuned containers.[Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		ntoRes := ntoResource{
			name:        "user-max-net-namespaces",
			namespace:   ntoNamespace,
			template:    ntoTunedDebugFile,
			sysctlparm:  "user.max_net_namespaces",
			sysctlvalue: "101010",
		}

		var (
			isEnableDebug bool
			isDebugInLog  bool
		)

		//Clean up the custom profile user-max-mnt-namespaces
		defer ntoRes.delete(oc)

		//Create a temp namespace to deploy nginx pod
		oc.SetupProject()
		ntoTestNS := oc.Namespace()

		//First choice to use [tests] image, the image mirrored by default in disconnected cluster
		//if don't have [tests] image in some environment, we can use hello-openshift as image
		//usually test imagestream shipped in all ocp and mirror the image in disconnected cluster by default
		// AppImageName := exutil.GetImagestreamImageName(oc, "tests")
		// if len(AppImageName) == 0 {
		AppImageName := "quay.io/openshifttest/nginx-alpine@sha256:04f316442d48ba60e3ea0b5a67eb89b0b667abf1c198a3d0056ca748736336a0"
		// }

		//Create a nginx web application pod
		exutil.By("Create a nginx web pod in nto temp namespace")
		exutil.ApplyNsResourceFromTemplate(oc, ntoTestNS, "--ignore-unknown-parameters=true", "-f", podNginxFile, "-p", "IMAGENAME="+AppImageName)

		//Check if nginx pod is ready
		exutil.AssertPodToBeReady(oc, "nginx", ntoTestNS)

		//Get the node name in the same node as nginx app
		tunedNodeName, err := exutil.GetPodNodeName(oc, ntoTestNS, "nginx")
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node as nginx app
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		//To reset tuned pod log, forcily to delete tuned pod
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", tunedPodName, "-n", ntoNamespace, "--ignore-not-found=true").Execute()

		//Label pod nginx with tuned.openshift.io/elasticsearch=
		exutil.By("Label nginx pod as tuned.openshift.io/elasticsearch=")
		err = exutil.LabelPod(oc, ntoTestNS, "nginx", "tuned.openshift.io/elasticsearch=")
		o.Expect(err).NotTo(o.HaveOccurred())

		//Verify if debug was disabled by default
		exutil.By("Check node profile debug settings, it should be debug: false")
		isEnableDebug = assertDebugSettings(oc, tunedNodeName, ntoNamespace, "false")
		o.Expect(isEnableDebug).To(o.Equal(true))

		//Apply new profile that match label tuned.openshift.io/elasticsearch=
		exutil.By("Apply new profile from CR with debug setting is false")
		ntoRes.createDebugTunedProfileIfNotExist(oc, false)

		//Verify if the new profile is applied
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)
		profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("user-max-net-namespaces"))

		//Verify nto tuned logs
		exutil.By("Check NTO tuned pod logs to confirm if user-max-net-namespaces applied")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 180, `user-max-net-namespaces applied|\(user-max-net-namespaces\) match`)
		//Verify if debug is false by CR setting
		exutil.By("Check node profile debug settings, it should be debug: false")
		isEnableDebug = assertDebugSettings(oc, tunedNodeName, ntoNamespace, "false")
		o.Expect(isEnableDebug).To(o.Equal(true))

		//Check if the log contain debug, the expected result should be none
		exutil.By("Check if tuned pod log contains debug key word, the expected result should be no DEBUG")
		isDebugInLog = exutil.AssertOprPodLogsbyFilter(oc, tunedPodName, ntoNamespace, "DEBUG", 2)
		o.Expect(isDebugInLog).To(o.Equal(false))

		exutil.By("Delete custom profile and will apply a new one ...")
		ntoRes.delete(oc)

		exutil.By("Apply new profile from CR with debug setting is true")
		ntoRes.createDebugTunedProfileIfNotExist(oc, true)

		//Verify if the new profile is applied
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)
		profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("user-max-net-namespaces"))

		//Verify nto tuned logs
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 180, `user-max-net-namespaces applied|\(user-max-net-namespaces\) match`)

		//Verify if debug was enabled by CR setting
		exutil.By("Check if the debug is true in node profile, the expected result should be true")
		isEnableDebug = assertDebugSettings(oc, tunedNodeName, ntoNamespace, "true")
		o.Expect(isEnableDebug).To(o.Equal(true))

		//The log shouldn't contain debug in log
		exutil.By("Check if tuned pod log contains debug key word, the log should contain DEBUG")
		exutil.AssertOprPodLogsbyFilterWithDuration(oc, tunedPodName, ntoNamespace, "DEBUG", 60, 2)
	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-37415-Allow setting isolated_cores without touching the default_irq_affinity [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		isSNO := exutil.IsSNOCluster(oc)
		//Prior to choose worker nodes with machineset
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned.openshift.io/default-irq-smp-affinity-").Execute()

		exutil.By("Label the node with default-irq-smp-affinity ")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned.openshift.io/default-irq-smp-affinity=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the default values of /proc/irq/default_smp_affinity on worker nodes")

		//Replace exutil.DebugNodeWithOptionsAndChroot with oc.AsAdmin().WithoutNamespace due to throw go warning even if set --quiet=true
		//This test case must got the value of default_smp_affinity without warning information
		defaultSMPAffinity, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "cat", "/proc/irq/default_smp_affinity").Output()
		e2e.Logf("the default value of /proc/irq/default_smp_affinity without cpu affinity is: %v", defaultSMPAffinity)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultSMPAffinity).NotTo(o.BeEmpty())
		defaultSMPAffinity = strings.ReplaceAll(defaultSMPAffinity, ",", "")
		defaultSMPAffinityMask := getDefaultSMPAffinityBitMaskbyCPUCores(oc, tunedNodeName)
		o.Expect(defaultSMPAffinity).To(o.ContainSubstring(defaultSMPAffinityMask))

		e2e.Logf("the value of /proc/irq/default_smp_affinity: %v", defaultSMPAffinityMask)
		cpuBitsMask := convertCPUBitMaskToByte(defaultSMPAffinityMask)
		o.Expect(cpuBitsMask).NotTo(o.BeEmpty())

		ntoRes1 := ntoResource{
			name:        "default-irq-smp-affinity",
			namespace:   ntoNamespace,
			template:    ntoIRQSMPFile,
			sysctlparm:  "#default_irq_smp_affinity",
			sysctlvalue: "1",
		}

		defer ntoRes1.delete(oc)

		exutil.By("Create default-irq-smp-affinity profile to enable isolated_cores=1")
		ntoRes1.createIRQSMPAffinityProfileIfNotExist(oc)

		exutil.By("Check if new NTO profile was applied")
		ntoRes1.assertTunedProfileApplied(oc, tunedNodeName)

		exutil.By("Check values of /proc/irq/default_smp_affinity on worker nodes after enabling isolated_cores=1")
		isolatedcoresSMPAffinity, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "cat", "/proc/irq/default_smp_affinity").Output()
		isolatedcoresSMPAffinity = strings.ReplaceAll(isolatedcoresSMPAffinity, ",", "")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(isolatedcoresSMPAffinity).NotTo(o.BeEmpty())
		e2e.Logf("the value of default_smp_affinity after setting isolated_cores=1 is: %v", isolatedcoresSMPAffinity)

		exutil.By("Verify if the value of /proc/irq/default_smp_affinity is affected by isolated_cores=1")
		//Isolate the second cpu cores, the default_smp_affinity should be changed
		isolatedCPU := convertIsolatedCPURange2CPUList("1")
		o.Expect(isolatedCPU).NotTo(o.BeEmpty())

		newSMPAffinityMask := assertIsolateCPUCoresAffectedBitMask(cpuBitsMask, isolatedCPU)
		o.Expect(newSMPAffinityMask).NotTo(o.BeEmpty())
		o.Expect(isolatedcoresSMPAffinity).To(o.ContainSubstring(newSMPAffinityMask))

		exutil.By("Remove the old profile and create a new one later ...")
		ntoRes1.delete(oc)

		ntoRes2 := ntoResource{
			name:        "default-irq-smp-affinity",
			namespace:   ntoNamespace,
			template:    ntoIRQSMPFile,
			sysctlparm:  "default_irq_smp_affinity",
			sysctlvalue: "1",
		}

		defer ntoRes2.delete(oc)
		exutil.By("Create default-irq-smp-affinity profile to enable default_irq_smp_affinity=1")
		ntoRes2.createIRQSMPAffinityProfileIfNotExist(oc)

		exutil.By("Check if new NTO profile was applied")
		ntoRes2.assertTunedProfileApplied(oc, tunedNodeName)

		exutil.By("Check values of /proc/irq/default_smp_affinity on worker nodes")
		//We only need to return the value /proc/irq/default_smp_affinity without stdErr
		IRQSMPAffinity, _, err := exutil.DebugNodeRetryWithOptionsAndChrootWithStdErr(oc, tunedNodeName, []string{"--quiet=true", "--to-namespace=" + ntoNamespace}, "cat", "/proc/irq/default_smp_affinity")
		IRQSMPAffinity = strings.ReplaceAll(IRQSMPAffinity, ",", "")
		o.Expect(IRQSMPAffinity).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		//Isolate the second cpu cores, the default_smp_affinity should be changed
		e2e.Logf("the value of default_smp_affinity after setting default_irq_smp_affinity=1 is: %v", IRQSMPAffinity)
		isMatch := assertDefaultIRQSMPAffinityAffectedBitMask(cpuBitsMask, isolatedCPU, string(IRQSMPAffinity))
		o.Expect(isMatch).To(o.Equal(true))
	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-44650-NTO profiles provided with TuneD [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//Get the tuned pod name that run on first worker node
		tunedNodeName, err := exutil.GetFirstLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		exutil.By("Check kernel version of worker nodes ...")
		kernelVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.nodeInfo.kernelVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kernelVersion).NotTo(o.BeEmpty())

		exutil.By("Check default tuned profile list, should contain openshift-control-plane and openshift-node")
		defaultTunedOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned", "default", "-ojsonpath={.spec.recommend}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultTunedOutput).NotTo(o.BeEmpty())
		o.Expect(defaultTunedOutput).To(o.And(
			o.ContainSubstring("openshift-control-plane"),
			o.ContainSubstring("openshift-node")))

		exutil.By("Check content of tuned file /usr/lib/tuned/openshift/tuned.conf to match default NTO settings")
		openshiftTunedConf, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "cat", "/usr/lib/tuned/openshift/tuned.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(openshiftTunedConf).NotTo(o.BeEmpty())
		if strings.Contains(kernelVersion, "el8") || strings.Contains(kernelVersion, "el7") {
			o.Expect(openshiftTunedConf).To(o.And(
				o.ContainSubstring("avc_cache_threshold=8192"),
				o.ContainSubstring("kernel.pid_max=>4194304"),
				o.ContainSubstring("net.netfilter.nf_conntrack_max=1048576"),
				o.ContainSubstring("net.ipv4.conf.all.arp_announce=2"),
				o.ContainSubstring("net.ipv4.neigh.default.gc_thresh1=8192"),
				o.ContainSubstring("net.ipv4.neigh.default.gc_thresh2=32768"),
				o.ContainSubstring("net.ipv4.neigh.default.gc_thresh3=65536"),
				o.ContainSubstring("net.ipv6.neigh.default.gc_thresh1=8192"),
				o.ContainSubstring("net.ipv6.neigh.default.gc_thresh2=32768"),
				o.ContainSubstring("net.ipv6.neigh.default.gc_thresh3=65536"),
				o.ContainSubstring("vm.max_map_count=262144"),
				o.ContainSubstring("/sys/module/nvme_core/parameters/io_timeout=4294967295"),
				o.ContainSubstring(`cgroup_ps_blacklist=/kubepods\.slice/`),
				o.ContainSubstring("runtime=0")))
		} else {
			o.Expect(openshiftTunedConf).To(o.And(
				o.ContainSubstring("avc_cache_threshold=8192"),
				o.ContainSubstring("nf_conntrack_hashsize=1048576"),
				o.ContainSubstring("kernel.pid_max=>4194304"),
				o.ContainSubstring("fs.aio-max-nr=>1048576"),
				o.ContainSubstring("net.netfilter.nf_conntrack_max=1048576"),
				o.ContainSubstring("net.ipv4.conf.all.arp_announce=2"),
				o.ContainSubstring("net.ipv4.neigh.default.gc_thresh1=8192"),
				o.ContainSubstring("net.ipv4.neigh.default.gc_thresh2=32768"),
				o.ContainSubstring("net.ipv4.neigh.default.gc_thresh3=65536"),
				o.ContainSubstring("net.ipv6.neigh.default.gc_thresh1=8192"),
				o.ContainSubstring("net.ipv6.neigh.default.gc_thresh2=32768"),
				o.ContainSubstring("net.ipv6.neigh.default.gc_thresh3=65536"),
				o.ContainSubstring("vm.max_map_count=262144"),
				o.ContainSubstring("/sys/module/nvme_core/parameters/io_timeout=4294967295"),
				o.ContainSubstring(`cgroup_ps_blacklist=/kubepods\.slice/`),
				o.ContainSubstring("runtime=0")))
		}

		exutil.By("Check content of tuned file /usr/lib/tuned/openshift-control-plane/tuned.conf to match default NTO settings")
		openshiftControlPlaneTunedConf, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "cat", "/usr/lib/tuned/openshift-control-plane/tuned.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(openshiftControlPlaneTunedConf).NotTo(o.BeEmpty())
		o.Expect(openshiftControlPlaneTunedConf).To(o.ContainSubstring("include=openshift"))

		if strings.Contains(kernelVersion, "el8") || strings.Contains(kernelVersion, "el7") {
			o.Expect(openshiftControlPlaneTunedConf).To(o.And(
				o.ContainSubstring("sched_wakeup_granularity_ns=4000000"),
				o.ContainSubstring("sched_migration_cost_ns=5000000")))
		} else {
			o.Expect(openshiftControlPlaneTunedConf).NotTo(o.And(
				o.ContainSubstring("sched_wakeup_granularity_ns=4000000"),
				o.ContainSubstring("sched_migration_cost_ns=5000000")))
		}

		exutil.By("Check content of tuned file /usr/lib/tuned/openshift-node/tuned.conf to match default NTO settings")
		openshiftNodeTunedConf, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "cat", "/usr/lib/tuned/openshift-node/tuned.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(openshiftNodeTunedConf).To(o.And(
			o.ContainSubstring("include=openshift"),
			o.ContainSubstring("net.ipv4.tcp_fastopen=3"),
			o.ContainSubstring("fs.inotify.max_user_watches=65536"),
			o.ContainSubstring("fs.inotify.max_user_instances=8192")))
	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-33238-Test NTO support for operatorapi Removed state [Disruptive]", func() {

		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		exutil.By("Remove custom profile (if not already removed) and patch default tuned back to Managed")
		//Cleanup tuned and change back to managed state
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoNamespace, "tuned", "tuning-pidmax", "--ignore-not-found").Execute()
		defer patchTunedState(oc, ntoNamespace, "default", "Managed")

		ntoRes := ntoResource{
			name:        "tuning-pidmax",
			namespace:   ntoNamespace,
			template:    customTunedProfile,
			sysctlparm:  "kernel.pid_max",
			sysctlvalue: "182218",
		}

		oc.SetupProject()
		ntoTestNS := oc.Namespace()
		//Clean up the custom profile user-max-mnt-namespaces and unlabel the nginx pod
		defer ntoRes.delete(oc)

		//First choice to use [tests] image, the image mirrored by default in disconnected cluster
		//if don't have [tests] image in some environment, we can use hello-openshift as image
		//usually test imagestream shipped in all ocp and mirror the image in disconnected cluster by default
		// AppImageName := exutil.GetImagestreamImageName(oc, "tests")
		// if len(AppImageName) == 0 {
		AppImageName := "quay.io/openshifttest/nginx-alpine@sha256:04f316442d48ba60e3ea0b5a67eb89b0b667abf1c198a3d0056ca748736336a0"
		// }

		//Create a nginx web application pod
		exutil.By("Create a nginx web pod in nto temp namespace")
		exutil.ApplyNsResourceFromTemplate(oc, ntoTestNS, "--ignore-unknown-parameters=true", "-f", podNginxFile, "-p", "IMAGENAME="+AppImageName)

		//Check if nginx pod is ready
		exutil.AssertPodToBeReady(oc, "nginx", ntoTestNS)

		//Get the node name in the same node as nginx app
		tunedNodeName, err := exutil.GetPodNodeName(oc, ntoTestNS, "nginx")
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node as nginx app
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		e2e.Logf("the tuned name on node %v is %v", tunedNodeName, tunedPodName)
		//Label pod nginx with tuned.openshift.io/elasticsearch=
		exutil.By("Label nginx pod as tuned.openshift.io/elasticsearch=")
		err = exutil.LabelPod(oc, ntoTestNS, "nginx", "tuned.openshift.io/elasticsearch=")
		o.Expect(err).NotTo(o.HaveOccurred())

		//Apply new profile that match label tuned.openshift.io/elasticsearch=
		exutil.By("Apply new profile from CR")
		ntoRes.createTunedProfileIfNotExist(oc)

		//Verify if the new profile is applied
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)
		profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("tuning-pidmax"))

		exutil.By("Check logs, profile changes SHOULD be applied since tuned is MANAGED")
		logsCheck, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ntoNamespace, "--tail=9", tunedPodName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(logsCheck).To(o.ContainSubstring("tuning-pidmax"))

		exutil.By("Compare if the value user.max_ipc_namespaces in on node with labeled pod, should be 182218")
		compareSysctlValueOnSepcifiedNodeByName(oc, tunedNodeName, "kernel.pid_max", "", "182218")

		exutil.By("Patch default tuned to 'Removed'")
		err = patchTunedState(oc, ntoNamespace, "default", "Removed")
		o.Expect(err).NotTo(o.HaveOccurred())
		state, err := getTunedState(oc, ntoNamespace, "default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.Equal("Removed"))

		exutil.By("Check logs, profiles, and nodes (profile changes SHOULD NOT be applied since tuned is REMOVED)")

		exutil.By("Check pod status, all tuned pod should be terminated since tuned is REMOVED")
		exutil.WaitForNoPodsAvailableByKind(oc, "daemonset", "tuned", ntoNamespace)
		podCheck, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "pods").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podCheck).NotTo(o.ContainSubstring("tuned"))

		exutil.By("Check profile status, all node profile should be removed since tuned is REMOVED)")
		profileCheck, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.ContainSubstring("No resources"))

		exutil.By("Change tuned state back to managed ...")
		err = patchTunedState(oc, ntoNamespace, "default", "Managed")
		o.Expect(err).NotTo(o.HaveOccurred())
		state, err = getTunedState(oc, ntoNamespace, "default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(state).To(o.Equal("Managed"))

		exutil.By("Get the tuned node and pod names")
		//Get the node name in the same node as nginx app
		tunedNodeName, err = exutil.GetPodNodeName(oc, ntoTestNS, "nginx")
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node as nginx app
		tunedPodName = getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		exutil.By("Check logs, profiles, and nodes (profile changes SHOULD be applied since tuned is MANAGED)")
		//Verify if the new profile is applied
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)
		profileCheck, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("tuning-pidmax"))

		exutil.By("Check logs, profile changes SHOULD be applied since tuned is MANAGED)")
		logsCheck, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ntoNamespace, "--tail=9", tunedPodName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(logsCheck).To(o.ContainSubstring("tuning-pidmax"))

		exutil.By("Compare if the value user.max_ipc_namespaces in on node with labeled pod, should be 182218")
		compareSysctlValueOnSepcifiedNodeByName(oc, tunedNodeName, "kernel.pid_max", "", "182218")
	})

	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-30589-NTO Use MachineConfigs to lay down files needed for tuned [Disruptive] [Slow]", func() {

		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)
		if !isNTO || isSNO {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		//Use the last worker node as labeled node
		tunedNodeName, err := exutil.GetLastLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node that labeled node
		//tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		//Re-delete mcp,mc, performance and unlabel node, just in case the test case broken before clean up steps
		defer exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-rt", "worker-rt", 300)
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-rt-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-realtime", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Create machine config pool")
		exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ntoMCPFile, "-p", "MCP_NAME=worker-rt")

		exutil.By("Label the node with node-role.kubernetes.io/worker-rt=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-rt=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create openshift-realtime profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, ntoRealtimeFile)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Assert if machine config pool applied for worker nodes")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker", 300)
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-rt", 300)

		exutil.By("Assert if openshift-realtime profile was applied ...")
		//Verify if the new profile is applied
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-realtime")
		profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("openshift-realtime"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Assert if isolcpus was applied in machineconfig...")
		AssertTunedAppliedMC(oc, "nto-worker-rt", "isolcpus=")

		exutil.By("Assert if isolcpus was applied in labled node...")
		isMatch := AssertTunedAppliedToNode(oc, tunedNodeName, "isolcpus=")
		o.Expect(isMatch).To(o.Equal(true))

		exutil.By("Delete openshift-realtime tuned in labled node...")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-realtime", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Check Nodes for expected changes")
		assertIfNodeSchedulingDisabled(oc)

		exutil.By("Assert if machine config pool applied for worker nodes")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-rt", 300)

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Assert if isolcpus was applied in labled node...")
		isMatch = AssertTunedAppliedToNode(oc, tunedNodeName, "isolcpus=")
		o.Expect(isMatch).To(o.Equal(false))

		//The custom mc and mcp must be deleted by correct sequence, unlabel first and labeled node return to worker mcp, then delete mc and mcp
		//otherwise the mcp will keep degrade state, it will affected other test case that use mcp
		exutil.By("Delete custom MC and MCP by following right way...")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-rt-").Execute()
		exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-rt", "worker-rt", 300)
	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-29804-Tuned profile is updated after incorrect tuned CR is fixed [Disruptive]", func() {
		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		is3Master := exutil.Is3MasterNoDedicatedWorkerNode(oc)
		var (
			tunedNodeName string
			err           error
		)

		//Use the last worker node as labeled node
		//Support 3 master/worker node, no dedicated worker nodes
		if !is3Master && !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		e2e.Logf("tunedNodeName is:\n%v", tunedNodeName)

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "ips", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Label the node with tuned=ips")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned=ips", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create ips-host profile, new tuned should automatically handle duplicate sysctl settings")
		//Define duplicated parameter and value
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", IPSFile, "-p", "SYSCTLPARM1=kernel.pid_max", "SYSCTLVALUE1=1048575", "SYSCTLPARM2=kernel.pid_max", "SYSCTLVALUE2=1048575")

		exutil.By("Assert recommended profile (ips-host) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "5", 180, `recommended profile \(ips-host\) matches current configuration`)

		exutil.By("Check if new custom profile applied to label node")
		o.Expect(assertNTOCustomProfileStatus(oc, ntoNamespace, tunedNodeName, "ips-host", "True", "False")).To(o.Equal(true))

		//Only used for debug info
		exutil.By("Check current profile for each node")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		e2e.Logf("Current profile for each node: \n%v", output)

		//New tuned can automatically de-duplicate value of sysctl, no duplicate error anymore
		exutil.By("Assert if the duplicate value of sysctl kernel.pid_max take effective on target node, expected value should be 1048575")
		compareSpecifiedValueByNameOnLabelNode(oc, tunedNodeName, "kernel.pid_max", "1048575")

		exutil.By("Get default value of fs.mount-max on label node")
		defaultMaxMapCount := getValueOfSysctlByName(oc, ntoNamespace, tunedNodeName, "fs.mount-max")
		o.Expect(defaultMaxMapCount).NotTo(o.BeEmpty())
		e2e.Logf("The default value of sysctl fs.mount-max is %v", defaultMaxMapCount)

		//setting an invalid value for ips-host profile
		exutil.By("Update ips-host profile with invalid value of fs.mount-max = -1")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", IPSFile, "-p", "SYSCTLPARM1=fs.mount-max", "SYSCTLVALUE1=-1", "SYSCTLPARM2=kernel.pid_max", "SYSCTLVALUE2=1048575")

		exutil.By("Assert static tuning from profile 'ips-host' applied in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "5", 180, `static tuning from profile 'ips-host' applied`)

		exutil.By("Check if new custom profile applied to label node")
		o.Expect(assertNTOCustomProfileStatus(oc, ntoNamespace, tunedNodeName, "ips-host", "True", "True")).To(o.Equal(true))

		exutil.By("Check current profile for each node")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		e2e.Logf("Current profile for each node: \n%v", output)

		//The invalid value won't impact default value of fs.mount-max
		exutil.By("Assert if the value of sysctl fs.mount-max still use default value")
		compareSpecifiedValueByNameOnLabelNode(oc, tunedNodeName, "fs.mount-max", defaultMaxMapCount)

		//setting an new value of fs.mount-max for ips-host profile
		exutil.By("Update ips-host profile with new value of fs.mount-max = 868686")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", IPSFile, "-p", "SYSCTLPARM1=fs.mount-max", "SYSCTLVALUE1=868686", "SYSCTLPARM2=kernel.pid_max", "SYSCTLVALUE2=1048575")

		exutil.By("Assert recommended profile (ips-host) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "5", 180, `recommended profile \(ips-host\) matches current configuration`)

		exutil.By("Check if new custom profile applied to label node")
		o.Expect(assertNTOCustomProfileStatus(oc, ntoNamespace, tunedNodeName, "ips-host", "True", "False")).To(o.Equal(true))

		exutil.By("Check current profile for each node")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		e2e.Logf("Current profile for each node: \n%v", output)

		//The invalid value won't impact default value of fs.mount-max
		exutil.By("Assert if the new value of sysctl fs.mount-max take effective, expected value is 868686")
		compareSpecifiedValueByNameOnLabelNode(oc, tunedNodeName, "fs.mount-max", "868686")
	})
	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-39123-NTO Operator will update tuned after changing included profile [Disruptive] [Slow]", func() {
		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)
		if !isNTO || isSNO {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		if ManualPickup {
			g.Skip("This is the test case that execute mannually in shared cluster ...")
		}

		skipPAODeploy := skipDeployPAO(oc)
		isPAOInstalled = exutil.IsPAOInstalled(oc)
		if skipPAODeploy || isPAOInstalled {
			e2e.Logf("PAO has been installed and continue to execute test case")
		} else {
			isPAOInOperatorHub := exutil.IsPAOInOperatorHub(oc)
			if !isPAOInOperatorHub {
				g.Skip("PAO is not in OperatorHub - skipping test ...")
			}
			exutil.InstallPAO(oc, paoNamespace)
		}

		//Re-delete mcp,mc, performance and unlabel node, just in case the test case broken before clean up steps
		defer exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-cnf", "worker-cnf", 300)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "performance-patch", "-n", ntoNamespace, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("PerformanceProfile", "performance", "--ignore-not-found").Execute()

		//Use the last worker node as labeled node
		tunedNodeName, err := exutil.GetLastLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-cnf-").Execute()

		exutil.By("Label the node with node-role.kubernetes.io/worker-cnf=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-cnf=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform == "aws" || iaasPlatform == "gcp" {
			//Only GCP and AWS support realtime-kenel
			exutil.By("Apply performance profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoPerformanceFile, "-p", "ISENABLED=true")
		} else {
			exutil.By("Apply performance profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoPerformanceFile, "-p", "ISENABLED=false")
		}

		exutil.By("Apply worker-cnf machineconfigpool")
		exutil.ApplyOperatorResourceByYaml(oc, paoNamespace, paoWorkerCnfMCPFile)

		exutil.By("Assert if the MCP worker-cnf has been successfully applied ...")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-cnf", 750)

		exutil.By("Check if new NTO profile openshift-node-performance-performance was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node-performance-performance")

		exutil.By("Check if profile openshift-node-performance-performance applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node-performance-performance"))

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if tuned pod logs contains openshift-node-performance-performance on labeled nodes")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "2", 60, "openshift-node-performance-performance")

		exutil.By("Check if the linux kernel parameter as vm.stat_interval = 10")
		compareSpecifiedValueByNameOnLabelNode(oc, tunedNodeName, "vm.stat_interval", "10")

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Apply performance-patch profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, paoPerformancePatchFile)

		exutil.By("Assert if the MCP worker-cnf is ready after node rebooted ...")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-cnf", 750)

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if profile what's active profile applied on nodes")
		nodeProfileName, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node-performance-performance"))

		exutil.By("Check if tuned pod logs contains Cannot find profile 'openshift-node-performance-example-performanceprofile' on labeled nodes")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 60, "Cannot find profile")

		exutil.By("Check if the linux kernel parameter as vm.stat_interval = 1")
		compareSpecifiedValueByNameOnLabelNode(oc, tunedNodeName, "vm.stat_interval", "1")

		exutil.By("Patch include to include=openshift-node-performance-performance")
		err = patchTunedProfile(oc, ntoNamespace, "performance-patch", paoPerformanceFixpatchFile)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Assert if the MCP worker-cnf is ready after node rebooted ...")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-cnf", 600)

		exutil.By("Check if new NTO profile performance-patch was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "performance-patch")

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if contains static tuning from profile 'performance-patch' applied in tuned pod logs on labeled nodes")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 60, "static tuning from profile 'performance-patch' applied")

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if the linux kernel parameter as vm.stat_interval = 10")
		compareSpecifiedValueByNameOnLabelNode(oc, tunedNodeName, "vm.stat_interval", "10")

		//The custom mc and mcp must be deleted by correct sequence, unlabel first and labeled node return to worker mcp, then delete mc and mcp
		//otherwise the mcp will keep degrade state, it will affected other test case that use mcp
		exutil.By("Delete custom MC and MCP by following right way...")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-cnf-").Execute()
		exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-cnf", "worker-cnf", 480)
	})

	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-45686-NTO Creating tuned profile with references to not yet existing Performance Profile configuration.[Disruptive] [Slow]", func() {
		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)
		if !isNTO || isSNO {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		if ManualPickup {
			g.Skip("This is the test case that execute mannually in shared cluster ...")
		}

		skipPAODeploy := skipDeployPAO(oc)
		isPAOInstalled = exutil.IsPAOInstalled(oc)
		if skipPAODeploy || isPAOInstalled {
			e2e.Logf("PAO has been installed and continue to execute test case")
		} else {
			isPAOInOperatorHub := exutil.IsPAOInOperatorHub(oc)
			if !isPAOInOperatorHub {
				g.Skip("PAO is not in OperatorHub - skipping test ...")
			}
			exutil.InstallPAO(oc, paoNamespace)
		}

		defer exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-optimize", "worker-optimize", 360)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "include-performance-profile", "-n", ntoNamespace, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("PerformanceProfile", "optimize", "--ignore-not-found").Execute()

		//Use the last worker node as labeled node
		tunedNodeName, err := exutil.GetLastLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		//Re-delete mcp,mc, performance and unlabel node, just in case the test case broken before clean up steps
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-optimize-").Execute()

		exutil.By("Label the node with node-role.kubernetes.io/worker-optimize=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-optimize=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Apply worker-optimize machineconfigpool")
		exutil.ApplyOperatorResourceByYaml(oc, paoNamespace, paoWorkerOptimizeMCPFile)

		exutil.By("Assert if the MCP has been successfully applied ...")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-optimize", 600)

		isSNO = exutil.IsSNOCluster(oc)
		if isSNO {
			exutil.By("Apply include-performance-profile tuned profile")
			exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", paoIncludePerformanceProfile, "-p", "ROLENAME=master")
			exutil.By("Assert if the mcp is ready after server has been successfully rebooted...")
			exutil.AssertIfMCPChangesAppliedByName(oc, "master", 600)

		} else {
			exutil.By("Apply include-performance-profile tuned profile")
			exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", paoIncludePerformanceProfile, "-p", "ROLENAME=worker-optimize")

			exutil.By("Assert if the mcp is ready after server has been successfully rebooted...")
			exutil.AssertIfMCPChangesAppliedByName(oc, "worker-optimize", 600)
		}

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if profile what's active profile applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isSNO {
			o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-control-plane"))
		} else {
			o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node"))
		}

		exutil.By("Check if tuned pod logs contains Cannot find profile 'openshift-node-performance-optimize' on labeled nodes")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 60, "Cannot find profile 'openshift-node-performance-optimize'")

		if isSNO {
			exutil.By("Apply performance optimize profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoPerformanceOptimizeFile, "-p", "ROLENAME=master")
			exutil.By("Assert if the mcp is ready after server has been successfully rebooted...")
			exutil.AssertIfMCPChangesAppliedByName(oc, "master", 600)
		} else {
			exutil.By("Apply performance optimize profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoPerformanceOptimizeFile, "-p", "ROLENAME=worker-optimize")
			exutil.By("Assert if the mcp is ready after server has been successfully rebooted...")
			exutil.AssertIfMCPChangesAppliedByName(oc, "worker-optimize", 600)
		}

		exutil.By("Check performance profile tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-node-performance-optimize"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if new NTO profile performance-patch was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "include-performance-profile")

		exutil.By("Check if profile what's active profile applied on nodes")
		nodeProfileName, err = getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("include-performance-profile"))

		exutil.By("Check if contains static tuning from profile 'include-performance-profile' applied in tuned pod logs on labeled nodes")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "2", 60, "static tuning from profile 'include-performance-profile' applied")

		//The custom mc and mcp must be deleted by correct sequence, unlabel first and labeled node return to worker mcp, then delete mc and mcp
		//otherwise the mcp will keep degrade state, it will affected other test case that use mcp
		exutil.By("Delete custom MC and MCP by following right way...")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-optimize-").Execute()
		exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-optimize", "worker-optimize", 480)
	})

	g.It("NonHyperShiftHOST-Author:liqcui-Medium-36152-NTO Get metrics and alerts", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//get metric information that require ssl auth
		sslKey := "/etc/prometheus/secrets/metrics-client-certs/tls.key"
		sslCrt := "/etc/prometheus/secrets/metrics-client-certs/tls.crt"

		//Get NTO metrics data
		exutil.By("Get NTO metrics informaton without ssl, should be denied access, throw error...")
		metricsOutput, metricsError := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "sts/prometheus-k8s", "-c", "prometheus", "--", "curl", "-k", "https://node-tuning-operator.openshift-cluster-node-tuning-operator.svc:60000/metrics").Output()
		o.Expect(metricsError).Should(o.HaveOccurred())
		o.Expect(metricsOutput).NotTo(o.BeEmpty())
		o.Expect(metricsOutput).To(o.Or(
			o.ContainSubstring("bad certificate"),
			o.ContainSubstring("errno = 104"),
			o.ContainSubstring("certificate required"),
			o.ContainSubstring("error:1409445C"),
			o.ContainSubstring("exit code 56"),
			o.ContainSubstring("errno = 32")))

		exutil.By("Get NTO metrics informaton with ssl key and crt, should be access, get the metric information...")
		metricsOutput, metricsError = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "sts/prometheus-k8s", "-c", "prometheus", "--", "curl", "-k", "--key", sslKey, "--cert", sslCrt, "https://node-tuning-operator.openshift-cluster-node-tuning-operator.svc:60000/metrics").Output()
		o.Expect(metricsOutput).NotTo(o.BeEmpty())
		o.Expect(metricsError).NotTo(o.HaveOccurred())

		e2e.Logf("The metrics information of NTO as below: \n%v", metricsOutput)

		//Assert the key metrics
		exutil.By("Check if all metrics exist as expected...")
		o.Expect(metricsOutput).To(o.And(
			o.ContainSubstring("nto_build_info"),
			o.ContainSubstring("nto_pod_labels_used_info"),
			o.ContainSubstring("nto_degraded_info"),
			o.ContainSubstring("nto_profile_calculated_total")))
	})

	g.It("NonPreRelease-Longduration-Author:liqcui-Medium-49265-NTO support automatically rotate ssl certificate. [Disruptive]", func() {
		// test requires NTO to be installed
		is3CPNoWorker := exutil.Is3MasterNoDedicatedWorkerNode(oc)
		isSNO := exutil.IsSNOCluster(oc)

		if !isNTO || is3CPNoWorker || isSNO {
			g.Skip("NTO is not installed or No need to test on compact cluster - skipping test ...")
		}

		//Use the last worker node as labeled node
		tunedNodeName, err = exutil.GetLastLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("The tuned node name is: \n%v", tunedNodeName)

		//Get NTO operator pod name
		ntoOperatorPod, err := getNTOPodName(oc, ntoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The tuned operator pod name is: \n%v", ntoOperatorPod)

		metricEndpoint := getServiceENDPoint(oc, ntoNamespace)

		exutil.By("Get information about the certificate the metrics server in NTO")
		openSSLOutputBefore, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "/bin/bash", "-c", "/bin/openssl s_client -connect "+metricEndpoint+" 2>/dev/null </dev/null").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get information about the creation and expiration date of the certificate")
		openSSLExpireDateBefore, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "/bin/bash", "-c", "/bin/openssl s_client -connect "+metricEndpoint+" 2>/dev/null </dev/null | /bin/openssl x509 -noout -dates").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The openSSL Expired Date information of NTO openSSL before rotate as below: \n%v", openSSLExpireDateBefore)

		encodeBase64OpenSSLOutputBefore := exutil.StringToBASE64(openSSLOutputBefore)
		encodeBase64OpenSSLExpireDateBefore := exutil.StringToBASE64(openSSLExpireDateBefore)

		//To improve the sucessful rate, execute oc delete secret/node-tuning-operator-tls instead of oc -n openshift-service-ca secret/signing-key
		//The last one "oc -n openshift-service-ca secret/signing-key" take more time to complete, but need to manually execute once failed.
		exutil.By("Delete secret/node-tuning-operator-tls to automate to create a new one certificate")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ntoNamespace, "secret/node-tuning-operator-tls").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Assert NTO logs to match key words restarting metrics server to rotate certificates")
		assertNTOPodLogsLastLines(oc, ntoNamespace, ntoOperatorPod, "4", 240, "restarting metrics server to rotate certificates")

		exutil.By("Assert if NTO rotate certificates ...")
		AssertNTOCertificateRotate(oc, ntoNamespace, tunedNodeName, encodeBase64OpenSSLOutputBefore, encodeBase64OpenSSLExpireDateBefore)

		exutil.By("The certificate extracted from the openssl command should match the first certificate from the tls.crt file in the secret")
		compareCertificateBetweenOpenSSLandTLSSecret(oc, ntoNamespace, tunedNodeName)
	})

	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-49371-NTO will not restart tuned daemon when profile application take too long [Disruptive] [Slow]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//The restart tuned has removed due to timeout in the bug https://issues.redhat.com/browse/OCPBUGS-30647
		//Use the first worker node as labeled node
		tunedNodeName, err := exutil.GetFirstLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "worker-stuck-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-profile-stuck", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Label the node with worker-stack=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "worker-stuck=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create openshift-profile-stuck profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, workerStackFile)

		exutil.By("Check openshift-profile-stuck tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-profile-stuck"))

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Assert recommended profile (openshift-profile-stuck) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "12", 300, `recommended profile \(openshift-profile-stuck\) matches current configuration`)

		exutil.By("Check if new NTO profile openshift-profile-stuck was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-profile-stuck")

		exutil.By("Check if profile what's active profile applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-profile-stuck"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("The log shouldn't contain [ timeout (120) to apply TuneD profile; restarting TuneD daemon ] in tuned pod log")
		ntoPodLogs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ntoNamespace, tunedPodName, "--tail=10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ntoPodLogs).NotTo(o.ContainSubstring("timeout (120) to apply TuneD profile; restarting TuneD daemon"))

		exutil.By("The log shouldn't contain [ error waiting for tuned: signal: terminated ] in tuned pod log")
		ntoPodLogs, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ntoNamespace, tunedPodName, "--tail=10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ntoPodLogs).NotTo(o.ContainSubstring("error waiting for tuned: signal: terminated"))
	})

	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-49370-NTO add huge pages to boot time via bootloader [Disruptive] [Slow]", func() {
		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)
		if !isNTO || isSNO {
			g.Skip("NTO is not installed or it's Single Node Cluster- skipping test ...")
		}

		//Use the last worker node as labeled node
		tunedNodeName, err := exutil.GetLastLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node that labeled node
		//tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		//Re-delete mcp,mc, performance and unlabel node, just in case the test case broken before clean up steps
		defer exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-hp", "worker-hp", 300)
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-hp-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "hugepages", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Label the node with node-role.kubernetes.io/worker-hp=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-hp=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create hugepages tuned profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, hugepageTunedBoottimeFile)

		exutil.By("Check hugepages tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("hugepages"))

		exutil.By("Create worker-hp machineconfigpool ...")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, hugepageMCPfile)

		exutil.By("Assert if the MCP has been successfully applied ...")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-hp", 720)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node-hugepages")

		exutil.By("Check if profile openshift-node-hugepages applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node-hugepages"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check value of allocatable.hugepages-2Mi in labled node ")
		nodeHugePagesOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.allocatable.hugepages-2Mi}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeHugePagesOutput).To(o.ContainSubstring("100M"))

		oc.SetupProject()
		ntoTestNS := oc.Namespace()

		//First choice to use [tests] image, the image mirrored by default in disconnected cluster
		//if don't have [tests] image in some environment, we can use hello-openshift as image
		//usually test imagestream shipped in all ocp and mirror the image in disconnected cluster by default
		AppImageName := exutil.GetImagestreamImageName(oc, "tests")
		if len(AppImageName) == 0 {
			AppImageName = "quay.io/openshifttest/nginx-alpine@sha256:04f316442d48ba60e3ea0b5a67eb89b0b667abf1c198a3d0056ca748736336a0"
		}

		//Create a hugepages-app application pod
		exutil.By("Create a hugepages-app pod to consume hugepage in nto temp namespace")
		exutil.ApplyNsResourceFromTemplate(oc, ntoTestNS, "--ignore-unknown-parameters=true", "-f", hugepage100MPodFile, "-p", "IMAGENAME="+AppImageName)

		//Check if hugepages-appis ready
		exutil.By("Check if a hugepages-app pod is ready ...")
		exutil.AssertPodToBeReady(oc, "hugepages-app", ntoTestNS)

		exutil.By("Check the value of /etc/podinfo/hugepages_2M_request, the value expected is 105 ...")
		podInfo, err := exutil.RemoteShPod(oc, ntoTestNS, "hugepages-app", "cat", "/etc/podinfo/hugepages_2M_request")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podInfo).To(o.ContainSubstring("105"))

		exutil.By("Check the value of REQUESTS_HUGEPAGES in env on pod ...")
		envInfo, err := exutil.RemoteShPodWithBash(oc, ntoTestNS, "hugepages-app", "env | grep REQUESTS_HUGEPAGES")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(envInfo).To(o.ContainSubstring("REQUESTS_HUGEPAGES_2Mi=104857600"))

		exutil.By("The right way to delete custom MC and MCP...")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-hp-").Execute()
		exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-hp", "worker-hp", 480)
	})

	g.It("NonPreRelease-Longduration-Author:liqcui-Medium-49439-NTO can start and stop stalld when relying on Tuned '[service]' plugin.[Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		if ManualPickup {
			g.Skip("This is the test case that execute mannually in shared cluster ...")
		}

		//Use the first rhcos worker node as labeled node
		tunedNodeName, err := exutil.GetFirstCoreOsWorkerNode(oc)
		e2e.Logf("tunedNodeName is [ %v ]", tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())

		if len(tunedNodeName) == 0 {
			g.Skip("Skip Testing on RHEL worker or windows node")
		}

		//Get the tuned pod name in the same node that labeled node
		//tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-stalld-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-stalld", "-n", ntoNamespace, "--ignore-not-found").Execute()
		defer exutil.DebugNodeWithChroot(oc, tunedNodeName, "/usr/bin/throttlectl", "on")

		exutil.By("Set off for /usr/bin/throttlectl before enable stalld")
		switchThrottlectlOnOff(oc, ntoNamespace, tunedNodeName, "off", 30)

		exutil.By("Label the node with node-role.kubernetes.io/worker-stalld=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-stalld=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create openshift-stalld tuned profile")
		exutil.CreateNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", stalldTunedFile, "-p", "STALLD_STATUS=start,enable")

		exutil.By("Check openshift-stalld tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-stalld"))

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-stalld")

		exutil.By("Check if profile openshift-stalld applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-stalld"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if stalld service is running ...")
		stalldStatus, err := exutil.DebugNodeWithChroot(oc, tunedNodeName, "systemctl", "status", "stalld")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stalldStatus).To(o.ContainSubstring("active (running)"))

		exutil.By("Apply openshift-stalld with stop,disable tuned profile")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", stalldTunedFile, "-p", "STALLD_STATUS=stop,disable")

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-stalld")

		exutil.By("Check if stalld service is inactive and stopped ...")
		//Return an error when the systemctl status stalld is inactive, so err for o.Expect as expected.
		stalldStatus, _ = exutil.DebugNodeWithChroot(oc, tunedNodeName, "systemctl", "status", "stalld")
		e2e.Logf("The service stalld status:\n%v", stalldStatus)
		o.Expect(stalldStatus).To(o.ContainSubstring("inactive (dead)"))

		exutil.By("Apply openshift-stalld with start,enable tuned profile")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", stalldTunedFile, "-p", "STALLD_STATUS=start,enable")

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-stalld")

		exutil.By("Check if stalld service is running again ...")
		stalldStatus, err = exutil.DebugNodeWithChroot(oc, tunedNodeName, "systemctl", "status", "stalld")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stalldStatus).To(o.ContainSubstring("active (running)"))
	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-49441-NTO Applying a profile with multiple inheritance where parents include a common ancestor. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		//trying to include two profiles that share the same parent profile "throughput-performance". An example of such profiles
		// are the openshift-node --> openshift --> (virtual-guest) --> throughput-performance and postgresql profiles.
		//Use the first worker node as labeled node

		isSNO := exutil.IsSNOCluster(oc)
		//Prior to choose worker nodes with machineset
		if exutil.IsMachineSetExist(oc) && !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned.openshift.io/openshift-node-postgresql-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-node-postgresql", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Label the node with tuned.openshift.io/openshift-node-postgresql=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned.openshift.io/openshift-node-postgresql=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check postgresql profile /usr/lib/tuned/postgresql/tuned.conf include throughput-performance profile")
		postGreSQLProfile, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "cat", "/usr/lib/tuned/postgresql/tuned.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(postGreSQLProfile).To(o.ContainSubstring("throughput-performance"))

		exutil.By("Check postgresql profile /usr/lib/tuned/openshift-node/tuned.conf include openshift profile")
		openshiftNodeProfile, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "cat", "/usr/lib/tuned/openshift-node/tuned.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(openshiftNodeProfile).To(o.ContainSubstring(`include=openshift`))

		exutil.By("Check postgresql profile /usr/lib/tuned/openshift/tuned.conf include throughput-performance profile")
		openshiftProfile, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "cat", "/usr/lib/tuned/openshift/tuned.conf")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(openshiftProfile).To(o.ContainSubstring("throughput-performance"))

		exutil.By("Create openshift-node-postgresql tuned profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, openshiftNodePostgresqlFile)

		exutil.By("Check openshift-node-postgresql tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-node-postgresql"))

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node-postgresql")

		exutil.By("Check if profile openshift-node-postgresql applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node-postgresql"))

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Assert recommended profile (openshift-node-postgresql) matches current configuration in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "10", 300, `recommended profile \(openshift-node-postgresql\) matches current configuration|static tuning from profile 'openshift-node-postgresql' applied`)
	})

	g.It("NonHyperShiftHOST-Author:liqcui-Medium-49705-Tuned net plugin handle net devices with n/a value for a channel. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed or hosted cluster - skipping test ...")
		}

		if iaasPlatform == "vsphere" || iaasPlatform == "openstack" || iaasPlatform == "none" || iaasPlatform == "powervs" {
			g.Skip("IAAS platform: " + iaasPlatform + " doesn't support cloud provider profile - skipping test ...")
		}

		isSNO := exutil.IsSNOCluster(oc)

		//Prior to choose worker nodes with machineset
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		exutil.By("Check default channel for host network adapter, not expected Combined: 1, if so, skip testing ...")
		//assertIFChannelQueuesStatus is used for checking if match Combined: 1
		//If match <Combined: 1>, skip testing
		isMatch := assertIFChannelQueuesStatus(oc, ntoNamespace, tunedNodeName)
		if isMatch {
			g.Skip("Only one NIC queues or Unsupported NIC - skipping test ...")
		}

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", tunedPodName, "-n", ntoNamespace, "node-role.kubernetes.io/netplugin-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "net-plugin", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Label the node with node-role.kubernetes.io/netplugin=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("pod", tunedPodName, "-n", ntoNamespace, "node-role.kubernetes.io/netplugin=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create net-plugin tuned profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, netPluginFile)

		exutil.By("Check net-plugin tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(tunedNames).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("net-plugin"))

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(output).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Assert tuned.plugins.base: instance net: assigning devices match in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "180", 300, "tuned.plugins.base: instance net: assigning devices")

		exutil.By("Assert active and recommended profile (net-plugin) match in tuned pod log")
		assertNTOPodLogsLastLines(oc, ntoNamespace, tunedPodName, "180", 300, `profile 'net-plugin' applied|profile \(net-plugin\) match`)

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "net-plugin")

		exutil.By("Check if profile net-plugin applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(nodeProfileName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("net-plugin"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(output).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check channel for host network adapter, expected Combined: 1")
		o.Expect(assertIFChannelQueuesStatus(oc, ntoNamespace, tunedNodeName)).To(o.BeTrue())

		exutil.By("Delete tuned net-plugin and check channel for host network adapater again")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "net-plugin", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Check if profile openshift-node|openshift-control-plane applied on nodes")
		if isSNO {
			assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-control-plane")
		} else {
			assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node")
		}

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(output).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check channel for host network adapter, not expected Combined: 1")
		o.Expect(assertIFChannelQueuesStatus(oc, ntoNamespace, tunedNodeName)).To(o.BeFalse())

	})

	g.It("ROSA-OSD_CCS-NonHyperShiftHOST-Author:liqcui-Medium-49617-NTO support cloud-provider specific profiles for NTO/TuneD. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		if iaasPlatform == "none" {
			g.Skip("IAAS platform: " + iaasPlatform + " doesn't support cloud provider profile - skipping test ...")
		}

		isSNO := exutil.IsSNOCluster(oc)
		//Prior to choose worker nodes with machineset
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		exutil.By("Get cloud provider name ...")
		providerName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("profiles.tuned.openshift.io", tunedNodeName, "-n", ntoNamespace, "-ojsonpath={.spec.config.providerName}").Output()
		o.Expect(providerName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "provider-"+providerName, "-n", ntoNamespace, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "provider-abc", "-n", ntoNamespace, "--ignore-not-found").Execute()

		providerID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.spec.providerID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(providerID).NotTo(o.BeEmpty())
		o.Expect(providerID).To(o.ContainSubstring(providerName))

		exutil.By("Check /var/lib/tuned/provider on target nodes")
		openshiftProfile, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "cat", "/var/lib/ocp-tuned/provider")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(openshiftProfile).NotTo(o.BeEmpty())
		o.Expect(openshiftProfile).To(o.ContainSubstring(providerName))

		exutil.By("Check the value of vm.admin_reserve_kbytes on target nodes, the expected value should be 8192")
		sysctlOutput, err := exutil.RemoteShPod(oc, ntoNamespace, tunedPodName, "sysctl", "vm.admin_reserve_kbytes")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sysctlOutput).NotTo(o.BeEmpty())
		o.Expect(sysctlOutput).To(o.ContainSubstring("vm.admin_reserve_kbytes = 8192"))

		exutil.By("Apply cloud-provider profile ...")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", cloudProviderFile, "-p", "PROVIDER_NAME="+providerName)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(output).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check tuned for NTO")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned.tuned.openshift.io").Output()
		o.Expect(output).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current tuned for NTO: \n%v", output)

		exutil.By("Check provider + providerName profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).NotTo(o.BeEmpty())
		o.Expect(tunedNames).To(o.ContainSubstring("provider-" + providerName))

		exutil.By("Check the value of vm.admin_reserve_kbytes on target nodes, the expected value is 16386")
		compareSpecifiedValueByNameOnLabelNodewithRetry(oc, ntoNamespace, tunedNodeName, "vm.admin_reserve_kbytes", "16386")

		exutil.By("Remove cloud-provider profile, the value of vm.admin_reserve_kbytes rollback to 8192")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "provider-"+providerName, "-n", ntoNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the value of vm.admin_reserve_kbytes on target nodes, the expected value should be 8192")
		compareSpecifiedValueByNameOnLabelNodewithRetry(oc, ntoNamespace, tunedNodeName, "vm.admin_reserve_kbytes", "8192")

		exutil.By("Apply cloud-provider-abc profile,the abc doesn't belong to any cloud provider ...")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", cloudProviderFile, "-p", "PROVIDER_NAME=abc")

		exutil.By("Check the value of vm.admin_reserve_kbytes on target nodes, the expected value should be no change, still is 8192")
		compareSpecifiedValueByNameOnLabelNodewithRetry(oc, ntoNamespace, tunedNodeName, "vm.admin_reserve_kbytes", "8192")
	})

	g.It("Author:liqcui-Medium-45593-NTO Operator set io_timeout for AWS Nitro instances in correct way.[Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		// currently test is only supported on AWS
		if iaasPlatform == "aws" {
			exutil.By("Expected /sys/module/nvme_core/parameters/io_timeout value on each node is: 4294967295")
			assertIOTimeOutandMaxRetries(oc, ntoNamespace)
		} else {
			g.Skip("Test Case 45593 doesn't support on other cloud platform, only support aws - skipping test ...")
		}

	})

	g.It("Author:liqcui-Medium-27420-NTO Operator is providing default tuned.[Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		defaultTunedCreateTimeBefore, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", "default", "-n", ntoNamespace, "-ojsonpath={.metadata.creationTimestamp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultTunedCreateTimeBefore).NotTo(o.BeEmpty())

		exutil.By("Delete the default tuned ...")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "default", "-n", ntoNamespace).Execute()
		exutil.By("The make sure the tuned default created and ready")
		confirmedTunedReady(oc, ntoNamespace, "default", 60)

		defaultTunedCreateTimeAfter, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", "default", "-n", ntoNamespace, "-ojsonpath={.metadata.creationTimestamp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultTunedCreateTimeAfter).NotTo(o.BeEmpty())
		o.Expect(defaultTunedCreateTimeAfter).NotTo(o.ContainSubstring(defaultTunedCreateTimeBefore))

		defaultTunedCreateTimeBefore, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", "default", "-n", ntoNamespace, "-ojsonpath={.metadata.creationTimestamp}").Output()
		o.Expect(defaultTunedCreateTimeBefore).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		defaultTunedCreateTimeAfter, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", "default", "-n", ntoNamespace, "-ojsonpath={.metadata.creationTimestamp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(defaultTunedCreateTimeAfter).NotTo(o.BeEmpty())
		o.Expect(defaultTunedCreateTimeAfter).To(o.ContainSubstring(defaultTunedCreateTimeBefore))

		e2e.Logf("defaultTunedCreateTimeBefore is : %v defaultTunedCreateTimeAfter is: %v", defaultTunedCreateTimeBefore, defaultTunedCreateTimeAfter)

	})
	g.It("NonHyperShiftHOST-Author:liqcui-Medium-41552-NTO Operator Report per-node Tuned profile application status[Disruptive].", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		isSNO := exutil.IsSNOCluster(oc)
		is3Master := exutil.Is3MasterNoDedicatedWorkerNode(oc)
		masterNodeName := getFirstMasterNodeName(oc)
		defaultMasterProfileName := getDefaultProfileNameOnMaster(oc, masterNodeName)

		//NTO will provides two default tuned, one is openshift-control-plane, another is openshift-node
		exutil.By("Check the default tuned profile list per nodes")
		profileOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("profiles.tuned.openshift.io", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileOutput).NotTo(o.BeEmpty())
		if isSNO || is3Master {
			o.Expect(profileOutput).To(o.ContainSubstring(defaultMasterProfileName))
		} else {
			o.Expect(profileOutput).To(o.ContainSubstring("openshift-control-plane"))
			o.Expect(profileOutput).To(o.ContainSubstring("openshift-node"))
		}

	})

	g.It("NonHyperShiftHOST-Author:liqcui-Medium-50052-NTO RHCOS-shipped stalld systemd units should use SCHED_FIFO to run stalld[Disruptive].", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		if iaasPlatform == "vsphere" || iaasPlatform == "none" {
			g.Skip("IAAS platform: " + iaasPlatform + " doesn't support cloud provider profile - skipping test ...")
		}

		isSNO := exutil.IsSNOCluster(oc)
		//Prior to choose worker nodes with machineset
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		e2e.Logf("tunedNodeName is [ %v ]", tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())

		if len(tunedNodeName) == 0 {
			g.Skip("Skip Testing on RHEL worker or windows node")
		}

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-stalld-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-stalld", "-n", ntoNamespace, "--ignore-not-found").Execute()
		defer exutil.DebugNodeRetryWithOptionsAndChroot(oc, tunedNodeName, []string{"-q"}, "/usr/bin/throttlectl", "on")

		//Switch off throttlectl to improve sucessfull rate of stalld starting
		exutil.By("Set off for /usr/bin/throttlectl before enable stalld")
		switchThrottlectlOnOff(oc, ntoNamespace, tunedNodeName, "off", 30)

		exutil.By("Label the node with node-role.kubernetes.io/worker-stalld=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-stalld=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create openshift-stalld tuned profile")
		exutil.CreateNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", stalldTunedFile, "-p", "STALLD_STATUS=start,enable")

		exutil.By("Check openshift-stalld tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).NotTo(o.BeEmpty())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-stalld"))

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.BeEmpty())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if new NTO profile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-stalld")

		exutil.By("Check if profile openshift-stalld applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).NotTo(o.BeEmpty())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-stalld"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.BeEmpty())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if stalld service is running ...")
		stalldStatus, _, err := exutil.DebugNodeRetryWithOptionsAndChrootWithStdErr(oc, tunedNodeName, []string{"-q"}, "systemctl", "status", "stalld")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stalldStatus).NotTo(o.BeEmpty())
		o.Expect(stalldStatus).To(o.ContainSubstring("active (running)"))

		exutil.By("Get stalld PID on labeled node ...")
		stalldPIDStatus, _, err := exutil.DebugNodeRetryWithOptionsAndChrootWithStdErr(oc, tunedNodeName, []string{"-q"}, "/bin/bash", "-c", "ps -efZ | grep stalld | grep -v grep")
		e2e.Logf("stalldPIDStatus is :\n%v", stalldPIDStatus)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stalldPIDStatus).NotTo(o.BeEmpty())
		o.Expect(stalldPIDStatus).NotTo(o.ContainSubstring("unconfined_service_t"))
		o.Expect(stalldPIDStatus).To(o.ContainSubstring("-t 20"))

		exutil.By("Get stalld PID on labeled node ...")
		stalldPID, _, err := exutil.DebugNodeRetryWithOptionsAndChrootWithStdErr(oc, tunedNodeName, []string{"-q"}, "/bin/bash", "-c", "ps -efL| grep stalld | grep -v grep | awk '{print $2}'")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(stalldPID).NotTo(o.BeEmpty())

		exutil.By("Get status of chrt -p stalld PID on labeled node ...")
		chrtStalldPIDOutput, _, err := exutil.DebugNodeRetryWithOptionsAndChrootWithStdErr(oc, tunedNodeName, []string{"-q"}, "/bin/bash", "-c", "chrt -ap "+stalldPID)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(chrtStalldPIDOutput).NotTo(o.BeEmpty())
		o.Expect(chrtStalldPIDOutput).To(o.ContainSubstring("SCHED_FIFO"))
		e2e.Logf("chrtStalldPIDOutput is :\n%v", chrtStalldPIDOutput)
	})
	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-51495-NTO PAO Shipped into NTO with basic function verification.[Disruptive][Slow].", func() {

		var (
			paoBaseProfileMCP = exutil.FixturePath("testdata", "psap", "pao", "pao-baseprofile-mcp.yaml")
			paoBaseProfile    = exutil.FixturePath("testdata", "psap", "pao", "pao-baseprofile.yaml")
			paoBaseQoSPod     = exutil.FixturePath("testdata", "psap", "pao", "pao-baseqos-pod.yaml")
		)
		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)
		if !isNTO || isSNO {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		if ManualPickup {
			g.Skip("This is the test case that execute mannually in shared cluster ...")
		}

		skipPAODeploy := skipDeployPAO(oc)
		isPAOInstalled = exutil.IsPAOInstalled(oc)
		if skipPAODeploy || isPAOInstalled {
			e2e.Logf("PAO has been installed and continue to execute test case")
		} else {
			isPAOInOperatorHub := exutil.IsPAOInOperatorHub(oc)
			if !isPAOInOperatorHub {
				g.Skip("PAO is not in OperatorHub - skipping test ...")
			}
			exutil.InstallPAO(oc, paoNamespace)
		}

		defer exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-pao", "worker-pao", 480)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("performanceprofile", "pao-baseprofile", "--ignore-not-found").Execute()

		//Use the last worker node as labeled node
		tunedNodeName, err := exutil.GetLastLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNodeName).NotTo(o.BeEmpty())

		//Get how many cpus on the specified worker node
		exutil.By("Get how many cpus cores on the labeled worker node")
		nodeCPUCores, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.capacity.cpu}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeCPUCores).NotTo(o.BeEmpty())

		nodeCPUCoresInt, err := strconv.Atoi(nodeCPUCores)
		o.Expect(err).NotTo(o.HaveOccurred())
		if nodeCPUCoresInt <= 1 {
			g.Skip("the worker node don't have enough cpus - skipping test ...")
		}
		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		//Re-delete mcp,mc, performance and unlabel node, just in case the test case broken before clean up steps
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-pao-").Execute()

		exutil.By("Label the node with node-role.kubernetes.io/worker-pao=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-pao=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform == "aws" || iaasPlatform == "gcp" {
			//Only GCP and AWS support realtime-kenel
			exutil.By("Apply pao-baseprofile performance profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoBaseProfile, "-p", "ISENABLED=true")
		} else {
			exutil.By("Apply pao-baseprofile performance profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoBaseProfile, "-p", "ISENABLED=false")
		}

		exutil.By("Check Performance Profile pao-baseprofile was created automatically")
		paoBasePerformanceProfile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("performanceprofile").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(paoBasePerformanceProfile).NotTo(o.BeEmpty())
		o.Expect(paoBasePerformanceProfile).To(o.ContainSubstring("pao-baseprofile"))

		exutil.By("Create machine config pool worker-pao")
		exutil.ApplyOperatorResourceByYaml(oc, "", paoBaseProfileMCP)

		exutil.By("Assert if machine config pool applied for worker nodes")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-pao", 1200)

		exutil.By("Check openshift-node-performance-pao-baseprofile tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-node-performance-pao-baseprofile"))

		exutil.By("Check current profile openshift-node-performance-pao-baseprofile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if new NTO profile openshift-node-performance-pao-baseprofile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node-performance-pao-baseprofile")

		exutil.By("Check if profile openshift-node-performance-pao-baseprofile applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node-performance-pao-baseprofile"))

		exutil.By("Check value of allocatable.hugepages-1Gi in labled node ")
		nodeHugePagesOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.allocatable.hugepages-1Gi}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeHugePagesOutput).To(o.ContainSubstring("1Gi"))

		exutil.By("Check Settings of CPU Manager policy created by PAO in labled node ")
		cpuManagerConfOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep cpuManager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cpuManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("cpuManagerPolicy"))
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("cpuManagerReconcilePeriod"))
		e2e.Logf("The settings of CPU Manager Policy on labeled nodes: \n%v", cpuManagerConfOutput)

		exutil.By("Check Settings of CPU Manager for reservedSystemCPUs created by PAO in labled node ")
		cpuManagerConfOutput, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep reservedSystemCPUs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cpuManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("reservedSystemCPUs"))
		e2e.Logf("The settings of CPU Manager reservedSystemCPUs on labeled nodes: \n%v", cpuManagerConfOutput)

		exutil.By("Check Settings of Topology Manager for topologyManagerPolicy created by PAO in labled node ")
		topologyManagerConfOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep topologyManagerPolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(topologyManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(topologyManagerConfOutput).To(o.ContainSubstring("topologyManagerPolicy"))
		e2e.Logf("The settings of CPU Manager topologyManagerPolicy on labeled nodes: \n%v", topologyManagerConfOutput)

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform == "aws" || iaasPlatform == "gcp" {
			exutil.By("Check realTime kernel setting that created by PAO in labled node ")
			realTimekernalOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-owide").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(realTimekernalOutput).NotTo(o.BeEmpty())
			o.Expect(realTimekernalOutput).To(o.Or(o.ContainSubstring("rt")))
		} else {
			exutil.By("Check realTime kernel setting that created by PAO in labled node ")
			realTimekernalOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-owide").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(realTimekernalOutput).NotTo(o.BeEmpty())
			o.Expect(realTimekernalOutput).NotTo(o.Or(o.ContainSubstring("rt")))
		}

		exutil.By("Check runtimeClass setting that created by PAO ... ")
		runtimeClassOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("performanceprofile", "pao-baseprofile", "-ojsonpath={.status.runtimeClass}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runtimeClassOutput).NotTo(o.BeEmpty())
		o.Expect(runtimeClassOutput).To(o.ContainSubstring("performance-pao-baseprofile"))
		e2e.Logf("The settings of runtimeClass on labeled nodes: \n%v", runtimeClassOutput)

		oc.SetupProject()
		ntoTestNS := oc.Namespace()

		//Create a guaranteed-pod application pod
		exutil.By("Create a guaranteed-pod pod into temp namespace")
		exutil.ApplyOperatorResourceByYaml(oc, ntoTestNS, paoBaseQoSPod)

		//Check if guaranteed-pod is ready
		exutil.By("Check if a guaranteed-pod pod is ready ...")
		exutil.AssertPodToBeReady(oc, "guaranteed-pod", ntoTestNS)

		exutil.By("Check the cpu bind to isolation CPU zone for a guaranteed-pod")
		cpuManagerStateOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /var/lib/kubelet/cpu_manager_state").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cpuManagerStateOutput).NotTo(o.BeEmpty())
		o.Expect(cpuManagerStateOutput).To(o.ContainSubstring("guaranteed-pod"))
		e2e.Logf("The settings of CPU Manager cpuManagerState on labeled nodes: \n%v", cpuManagerStateOutput)

		//The custom mc and mcp must be deleted by correct sequence, unlabel first and labeled node return to worker mcp, then delete mc and mcp
		//otherwise the mcp will keep degrade state, it will affected other test case that use mcp
		exutil.By("Delete custom MC and MCP by following correct logic ...")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-pao-").Execute()
		exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-pao", "worker-pao", 480)
	})

	g.It("NonHyperShiftHOST-Author:liqcui-Medium-53053-NTO will automatically delete profile with unknown/stuck state. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		if iaasPlatform == "none" {
			g.Skip("IAAS platform: " + iaasPlatform + " doesn't support cloud provider profile - skipping test ...")
		}

		var (
			ntoUnknownProfile = exutil.FixturePath("testdata", "psap", "nto", "nto-unknown-profile.yaml")
		)

		//Get NTO operator pod name
		ntoOperatorPod, err := getNTOPodName(oc, ntoNamespace)
		o.Expect(ntoOperatorPod).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		isSNO := exutil.IsSNOCluster(oc)
		//Prior to choose worker nodes with machineset
		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		exutil.By("Get cloud provider name ...")
		providerName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("profiles.tuned.openshift.io", tunedNodeName, "-n", ntoNamespace, "-ojsonpath={.spec.config.providerName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(providerName).NotTo(o.BeEmpty())

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("profiles.tuned.openshift.io", "worker-does-not-exist-openshift-node", "-n", ntoNamespace, "--ignore-not-found").Execute()

		exutil.By("Apply worker-does-not-exist-openshift-node profile ...")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", ntoUnknownProfile, "-p", "PROVIDER_NAME="+providerName)

		exutil.By("The profile worker-does-not-exist-openshift-node will be deleted automatically once created.")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(tunedNames).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).NotTo(o.ContainSubstring("worker-does-not-exist-openshift-node"))

		exutil.By("Assert NTO logs to match key words  Node 'worker-does-not-exist-openshift-node' not found")
		assertNTOPodLogsLastLines(oc, ntoNamespace, ntoOperatorPod, "4", 120, " Node \"worker-does-not-exist-openshift-node\" not found")

	})

	g.It("NonPreRelease-Longduration-Author:liqcui-Medium-59884-NTO Cgroup Blacklist multiple regular expression. [Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		oc.SetupProject()
		ntoTestNS := oc.Namespace()

		//Get the tuned pod name that run on first worker node
		tunedNodeName, err := exutil.GetFirstLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNodeName).NotTo(o.BeEmpty())

		//First choice to use [tests] image, the image mirrored by default in disconnected cluster
		//if don't have [tests] image in some environment, we can use hello-openshift as image
		//usually test imagestream shipped in all ocp and mirror the image in disconnected cluster by default
		// AppImageName := exutil.GetImagestreamImageName(oc, "tests")
		// if len(AppImageName) == 0 {
		AppImageName := "quay.io/openshifttest/nginx-alpine@sha256:04f316442d48ba60e3ea0b5a67eb89b0b667abf1c198a3d0056ca748736336a0"
		// }

		//Get how many cpus on the specified worker node
		exutil.By("Get how many cpus cores on the labeled worker node")
		nodeCPUCores, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.capacity.cpu}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeCPUCores).NotTo(o.BeEmpty())

		nodeCPUCoresInt, err := strconv.Atoi(nodeCPUCores)
		o.Expect(err).NotTo(o.HaveOccurred())
		if nodeCPUCoresInt <= 1 {
			g.Skip("the worker node don't have enough cpus - skipping test ...")
		}

		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		exutil.By("Remove custom profile (if not already removed) and remove node label")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "-n", ntoNamespace, "cgroup-scheduler-blacklist").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned-scheduler-node-").Execute()

		exutil.By("Label the specified linux node with label tuned-scheduler-node")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "tuned-scheduler-node=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// setting cgroup_ps_blacklist=/kubepods\.slice/kubepods-burstable\.slice/;/system\.slice/
		// the process belong the /kubepods\.slice/kubepods-burstable\.slice/ or /system\.slice/ can consume all cpuset
		// The expected Cpus_allowed_list in /proc/$PID/status should be 0-N
		// the process doesn't belong the /kubepods\.slice/kubepods-burstable\.slice/ or /system\.slice/ can consume all cpuset
		// The expected Cpus_allowed_list in /proc/$PID/status should be 0 or 0,2-N

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "-n", ntoTestNS, "app-web", "--ignore-not-found").Execute()

		exutil.By("Create pod that deletect the value of kernel.pid_max ")
		exutil.ApplyNsResourceFromTemplate(oc, ntoTestNS, "--ignore-unknown-parameters=true", "-f", cgroupSchedulerBestEffortPod, "-p", "IMAGE_NAME="+AppImageName)

		//Check if nginx pod is ready
		exutil.By("Check if best effort pod is ready...")
		exutil.AssertPodToBeReady(oc, "app-web", ntoTestNS)

		exutil.By("Create NTO custom tuned profile cgroup-scheduler-blacklist")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", cgroupSchedulerBacklist, "-p", "PROFILE_NAME=cgroup-scheduler-blacklist", `CGROUP_BLACKLIST=/kubepods\.slice/kubepods-burstable\.slice/;/system\.slice/`)

		exutil.By("Check if NTO custom tuned profile cgroup-scheduler-blacklist was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "cgroup-scheduler-blacklist")

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		// The expected Cpus_allowed_list in /proc/$PID/status should be 0-N
		exutil.By("Verified the cpu allow list in cgroup black list for tuned ...")
		o.Expect(assertProcessInCgroupSchedulerBlacklist(oc, tunedNodeName, ntoNamespace, "tuned", nodeCPUCoresInt)).To(o.Equal(true))

		// The expected Cpus_allowed_list in /proc/$PID/status should be 0-N
		exutil.By("Verified the cpu allow list in cgroup black list for chronyd ...")
		o.Expect(assertProcessInCgroupSchedulerBlacklist(oc, tunedNodeName, ntoNamespace, "chronyd", nodeCPUCoresInt)).To(o.Equal(true))

		// The expected Cpus_allowed_list in /proc/$PID/status should be 0 or 0,2-N
		exutil.By("Verified the cpu allow list in cgroup black list for nginx process...")
		o.Expect(assertProcessNOTInCgroupSchedulerBlacklist(oc, tunedNodeName, ntoNamespace, "nginx| tail -1", nodeCPUCoresInt)).To(o.Equal(true))
	})
	g.It("Longduration-NonPreRelease-Author:liqcui-Medium-60743-NTO No race to update MC when nodes with different number of CPUs are in the same MCP. [Disruptive] [Slow]", func() {

		// test requires NTO to be installed
		isSNO := exutil.IsSNOCluster(oc)

		if !isNTO || isSNO {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		haveMachineSet := exutil.IsMachineSetExist(oc)

		if !haveMachineSet {
			g.Skip("No machineset found, skipping test ...")
		}

		// currently test is only supported on AWS, GCP, Azure, ibmcloud, alibabacloud
		supportPlatforms := []string{"aws", "gcp", "azure", "ibmcloud", "alibabacloud"}

		if !implStringArrayContains(supportPlatforms, iaasPlatform) {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		//Use the last worker node as labeled node
		tunedNodeName, err := exutil.GetLastLinuxWorkerNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get NTO Operator Pod Name
		ntoOperatorPodName := getNTOOperatorPodName(oc, ntoNamespace)

		//Re-delete mcp,mc, performance and unlabel node, just in case the test case broken before clean up steps
		defer exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-diffcpus", "worker-diffcpus", 480)
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-diffcpus-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-bootcmdline-cpu", "-n", ntoNamespace, "--ignore-not-found").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("machineset", "ocp-psap-qe-diffcpus", "-n", "openshift-machine-api", "--ignore-not-found").Execute()

		exutil.By("Create openshift-bootcmdline-cpu tuned profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, nodeDiffCPUsTunedBootFile)

		exutil.By("Create machine config pool")
		exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", nodeDiffCPUsMCPFile, "-p", "MCP_NAME=worker-diffcpus")

		exutil.By("Label the last node with node-role.kubernetes.io/worker-diffcpus=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-diffcpus=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a new machineset with different instance type.")
		newMachinesetInstanceType := exutil.SpecifyMachinesetWithDifferentInstanceType(oc)
		o.Expect(newMachinesetInstanceType).NotTo(o.BeEmpty())
		exutil.CreateMachinesetbyInstanceType(oc, "ocp-psap-qe-diffcpus", newMachinesetInstanceType)

		exutil.By("Wait for new node is ready when machineset created")
		//1 means replicas=1
		clusterinfra.WaitForMachinesRunning(oc, 1, "ocp-psap-qe-diffcpus")

		exutil.By("Label the second node with node-role.kubernetes.io/worker-diffcpus=")
		secondTunedNodeName := exutil.GetNodeNameByMachineset(oc, "ocp-psap-qe-diffcpus")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", secondTunedNodeName, "node-role.kubernetes.io/worker-diffcpus-", "--overwrite").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", secondTunedNodeName, "node-role.kubernetes.io/worker-diffcpus=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Assert if the status of adding the two worker node into worker-diffcpus mcp, mcp applied")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-diffcpus", 480)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Assert if openshift-bootcmdline-cpu profile was applied ...")
		//Verify if the new profile is applied
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-bootcmdline-cpu")
		profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("openshift-bootcmdline-cpu"))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		assertNTOPodLogsLastLines(oc, ntoNamespace, ntoOperatorPodName, "25", 180, "Nodes in MCP worker-diffcpus agree on bootcmdline: cpus=")

		//Comment out with an known issue, until it was fixed
		exutil.By("Assert if cmdline was applied in machineconfig...")
		AssertTunedAppliedMC(oc, "nto-worker-diffcpus", "cpus=")

		exutil.By("Assert if cmdline was applied in labled node...")
		o.Expect(AssertTunedAppliedToNode(oc, tunedNodeName, "cpus=")).To(o.Equal(true))

		exutil.By("<Profiles with bootcmdline conflict> warn message will show in oc get co/node-tuning")
		assertCoStatusWithKeywords(oc, "Profiles with bootcmdline conflict")

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		//Verify if the <Profiles with bootcmdline conflict> warn message disapper after removing custom tuned profile
		exutil.By("Delete openshift-bootcmdline-cpu tuned in labled node...")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "openshift-bootcmdline-cpu", "-n", ntoNamespace, "--ignore-not-found").Execute()

		//The custom mc and mcp must be deleted by correct sequence, unlabel first and labeled node return to worker mcp, then delete mc and mcp
		//otherwise the mcp will keep degrade state, it will affected other test case that use mcp
		exutil.By("Removing custom MC and MCP from mcp worker-diffcpus...")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-diffcpus-").Execute()

		//remove node from mcp worker-diffcpus
		//To reduce time using delete machineset instead of unlabel secondTunedNodeName node
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("machineset", "ocp-psap-qe-diffcpus", "-n", "openshift-machine-api", "--ignore-not-found").Execute()
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", secondTunedNodeName, "node-role.kubernetes.io/worker-diffcpus-").Execute()

		exutil.By("Assert if first worker node return to worker mcp")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker", 480)

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.BeEmpty())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("<Profiles with bootcmdline conflict> warn message will disappear after removing worker node from mcp worker-diffcpus")
		assertCONodeTuningStatusWithoutWARNWithRetry(oc, 180, "Profiles with bootcmdline conflict")

		exutil.By("Assert if isolcpus was applied in labled node...")
		o.Expect(AssertTunedAppliedToNode(oc, tunedNodeName, "cpus=")).To(o.Equal(false))
	})

	g.It("ROSA--NonHyperShiftHOST-Author:liqcui-Medium-63223-NTO support tuning sysctl and kernel bools that applied to all nodes of nodepool-level settings in hypershift. [Disruptive]", func() {
		//Only execute on ROSA hosted cluster
		isROSA := isROSAHostedCluster(oc)
		if !isROSA {
			g.Skip("It's not ROSA hosted cluster - skipping test ...")
		}

		//For ROSA Environment, we are unable to access management cluster, so discussed with ROSA team,
		//ROSA team create pre-defined configmap and applied to specified nodepool with hardcode profile name.
		//NTO will only check if all setting applied to the worker node on hosted cluster.
		exutil.By("Check if the tuned hc-nodepool-vmdratio is created in hosted cluster nodepool")
		tunedNameList, err := oc.AsAdmin().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNameList).NotTo(o.BeEmpty())
		e2e.Logf("The list of tuned tunedNameList is: \n%v", tunedNameList)
		o.Expect(tunedNameList).To(o.And(o.ContainSubstring("hc-nodepool-vmdratio"),
			o.ContainSubstring("tuned-hugepages")))

		appliedProfileList, err := oc.AsAdmin().Run("get").Args("profiles.tuned.openshift.io", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(appliedProfileList).NotTo(o.BeEmpty())
		o.Expect(appliedProfileList).To(o.And(o.ContainSubstring("hc-nodepool-vmdratio"),
			o.ContainSubstring("openshift-node-hugepages")))

		exutil.By("Get the node name that applied to the profile hc-nodepool-vmdratio")
		tunedNodeNameStdOut, err := oc.AsAdmin().Run("get").Args("profiles.tuned.openshift.io", "-n", ntoNamespace, `-ojsonpath='{.items[?(@..status.tunedProfile=="hc-nodepool-vmdratio")].metadata.name}'`).Output()
		tunedNodeName := strings.Trim(tunedNodeNameStdOut, "'")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNodeName).NotTo(o.BeEmpty())

		exutil.By("Assert the value of sysctl vm.dirty_ratio, the expecte value should be 55")
		debugNodeStdout, err := oc.AsAdmin().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "sysctl", "vm.dirty_ratio").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The value of sysctl vm.dirty_ratio on node %v is: \n%v\n", tunedNodeName, debugNodeStdout)
		o.Expect(debugNodeStdout).To(o.ContainSubstring("vm.dirty_ratio = 55"))

		exutil.By("Get the node name that applied to the profile openshift-node-hugepages")
		tunedNodeNameStdOut, err = oc.AsAdmin().Run("get").Args("profiles.tuned.openshift.io", "-n", ntoNamespace, `-ojsonpath='{.items[?(@..status.tunedProfile=="openshift-node-hugepages")].metadata.name}'`).Output()
		tunedNodeName = strings.Trim(tunedNodeNameStdOut, "'")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNodeName).NotTo(o.BeEmpty())

		exutil.By("Assert the value of cat /proc/cmdline, the expecte value should be hugepagesz=2M hugepages=50")
		debugNodeStdout, err = oc.AsAdmin().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "cat", "/proc/cmdline").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The value of /proc/cmdline on node %v is: \n%v\n", tunedNodeName, debugNodeStdout)
		o.Expect(debugNodeStdout).To(o.ContainSubstring("hugepagesz=2M hugepages=50"))
	})

	g.It("ROSA-NonHyperShiftHOST-Author:sahshah-Medium-64908-NTO Expose tuned socket interface.[Disruptive]", func() {
		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}

		exutil.By("Pick one worker node to label")
		tunedNodeName, err := exutil.GetFirstLinuxWorkerNode(oc)
		o.Expect(tunedNodeName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		//Clean up resources
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "-n", ntoNamespace, "tuning-maxpid").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-tuning-").Execute()

		//Label the node with node-role.kubernetes.io/worker-tuning
		exutil.By("Label the node with node-role.kubernetes.io/worker-tuning=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-tuning=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		//Apply new profile that match label node-role.kubernetes.io/worker-tuning=
		exutil.By("Create tuning-maxpid profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, tuningMaxPidFile)

		//NTO will provides two default tuned, one is default
		exutil.By("Check the default tuned list, expected tuning-maxpid")
		allTuneds, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("tuned", "-n", ntoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(allTuneds).To(o.ContainSubstring("tuning-maxpid"))

		exutil.By("Check if new profile tuning-maxpid applied to labeled node")
		//Verify if the new profile is applied
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "tuning-maxpid")
		profileCheck, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(profileCheck).To(o.Equal("tuning-maxpid"))

		exutil.By("Get current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check the custom profile as expected by debugging the node ")
		printfString := fmt.Sprintf(`printf '{"jsonrpc": "2.0", "method": "active_profile", "id": 1}' | nc -U /run/tuned/tuned.sock`)
		printfStringStdOut, err := exutil.RemoteShPodWithBash(oc, ntoNamespace, tunedPodName, printfString)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(printfStringStdOut).NotTo(o.BeEmpty())
		o.Expect(printfStringStdOut).To(o.ContainSubstring("tuning-maxpid"))
		e2e.Logf("printfStringStdOut is :\n%v", printfStringStdOut)

	})

	g.It("ROSA-NonHyperShiftHOST-Author:liqcui-Medium-65371-NTO TuneD prevent from reverting node level profiles on termination [Disruptive]", func() {

		// test requires NTO to be installed
		if !isNTO {
			g.Skip("NTO is not installed - skipping test ...")
		}
		//Use the last worker node as labeled node
		var (
			tunedNodeName string
			err           error
		)

		isSNO := exutil.IsSNOCluster(oc)

		if !isSNO {
			tunedNodeName = choseOneWorkerNodeToRunCase(oc, 0)
		} else {
			tunedNodeName, err = exutil.GetFirstLinuxWorkerNode(oc)
			o.Expect(tunedNodeName).NotTo(o.BeEmpty())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)

		oc.SetupProject()
		ntoTestNS := oc.Namespace()

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-tuning-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "tuning-pidmax", "-n", ntoNamespace, "--ignore-not-found").Execute()

		ntoRes := ntoResource{
			name:        "tuning-pidmax",
			namespace:   ntoNamespace,
			template:    ntoTunedPidMax,
			sysctlparm:  "kernel.pid_max",
			sysctlvalue: "181818",
		}

		exutil.By("Label the node with node-role.kubernetes.io/worker-tuning=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-tuning=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create tuning-pidmax profile")
		exutil.ApplyOperatorResourceByYaml(oc, ntoNamespace, ntoTunedPidMax)

		exutil.By("Create tuning-pidmax profile tuning-pidmax applied to nodes")
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		AppImageName := exutil.GetImagestreamImageName(oc, "tests")

		clusterVersion, _, err := exutil.GetClusterVersion(oc)
		e2e.Logf("Current clusterVersion is [ %v ]", clusterVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterVersion).NotTo(o.BeEmpty())

		exutil.By("Create pod that deletect the value of kernel.pid_max ")
		exutil.ApplyNsResourceFromTemplate(oc, ntoTestNS, "--ignore-unknown-parameters=true", "-f", podSysctlFile, "-p", "IMAGE_NAME="+AppImageName, "RUNASNONROOT=true")

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		//Check if sysctlpod pod is ready
		exutil.AssertPodToBeReady(oc, "sysctlpod", ntoTestNS)

		exutil.By("Get the sysctlpod status")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoTestNS, "pods").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The status of pod sysctlpod: \n%v", output)

		exutil.By("Check the the value of kernel.pid_max in the pod sysctlpod, the expected value should be kernel.pid_max = 181818")
		podLogStdout, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("sysctlpod", "--tail=1", "-n", ntoTestNS).Output()
		e2e.Logf("Logs of sysctlpod before delete tuned pod is [ %v ]", podLogStdout)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podLogStdout).NotTo(o.BeEmpty())
		o.Expect(podLogStdout).To(o.ContainSubstring("kernel.pid_max = 181818"))

		exutil.By("Delete tuned pod on the labeled node, and make sure the kernel.pid_max don't revert to origin value")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", tunedPodName, "-n", ntoNamespace).Execute()).NotTo(o.HaveOccurred())

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check tuned pod status after delete tuned pod")
		//Get the tuned pod name in the same node that labeled node
		tunedPodName = getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		//Check if tuned pod that deleted is ready
		exutil.AssertPodToBeReady(oc, tunedPodName, ntoNamespace)

		exutil.By("Check the the value of kernel.pid_max in the pod sysctlpod again, the expected value still be kernel.pid_max = 181818")
		podLogStdout, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("sysctlpod", "--tail=2", "-n", ntoTestNS).Output()
		e2e.Logf("Logs of sysctlpod after delete tuned pod is [ %v ]", podLogStdout)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podLogStdout).NotTo(o.BeEmpty())
		o.Expect(podLogStdout).To(o.ContainSubstring("kernel.pid_max = 181818"))
		o.Expect(podLogStdout).NotTo(o.ContainSubstring("kernel.pid_max not equal 181818"))
	})
	g.It("Longduration-NonPreRelease-PreChkUpgrade-Author:liqcui-Medium-49618-Telco N-1 - Pre Check for PAO shipped with NTO to support upgrade.[Telco][Disruptive][Slow].", func() {

		var (
			paoBaseProfileMCP = exutil.FixturePath("testdata", "psap", "pao", "pao-baseprofile-mcp.yaml")
			paoBaseProfile    = exutil.FixturePath("testdata", "psap", "pao", "pao-baseprofile.yaml")
		)
		isSNO := exutil.IsSNOCluster(oc)

		if !isNTO || isSNO {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		// currently test is only supported on AWS, GCP, Azure, ibmcloud, alibabacloud
		supportPlatforms := []string{"aws", "gcp", "azure", "ibmcloud", "alibabacloud"}

		if !implStringArrayContains(supportPlatforms, iaasPlatform) || isSNO {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		tunedNodeName := choseOneWorkerNodeToRunCase(oc, 0)

		//Get how many cpus on the specified worker node
		exutil.By("Get the number of cpus cores on the labeled worker node")
		nodeCPUCores, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.capacity.cpu}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeCPUCores).NotTo(o.BeEmpty())

		nodeCPUCoresInt, err := strconv.Atoi(nodeCPUCores)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current cpus cores of worker node is %v", nodeCPUCoresInt)
		if nodeCPUCoresInt < 4 {
			g.Skip("the worker node doesn't have enough cpus - skipping test ...")
		}

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		exutil.By("Label the node with node-role.kubernetes.io/worker-pao=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-pao=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create machine config pool worker-pao")
		exutil.ApplyOperatorResourceByYaml(oc, "", paoBaseProfileMCP)
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-pao", 300)

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform == "aws" || iaasPlatform == "gcp" {
			//Only GCP and AWS support realtime-kenel
			exutil.By("Apply pao-baseprofile performance profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoBaseProfile, "-p", "ISENABLED=true")
		} else {
			exutil.By("Apply pao-baseprofile performance profile")
			exutil.ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoBaseProfile, "-p", "ISENABLED=false")
		}

		exutil.By("Check Performance Profile pao-baseprofile was created automatically")
		paoBasePerformanceProfile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("performanceprofile").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(paoBasePerformanceProfile).NotTo(o.BeEmpty())
		o.Expect(paoBasePerformanceProfile).To(o.ContainSubstring("pao-baseprofile"))

		exutil.By("Assert if machine config pool applied to worker nodes that label with worker-pao")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-pao", 1800)
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker", 300)
		exutil.AssertIfMCPChangesAppliedByName(oc, "master", 720)

		exutil.By("Check openshift-node-performance-pao-baseprofile tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-node-performance-pao-baseprofile"))

		exutil.By("Check current profile openshift-node-performance-pao-baseprofile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if new NTO profile openshift-node-performance-pao-baseprofile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node-performance-pao-baseprofile")

		exutil.By("Check if profile openshift-node-performance-pao-baseprofile applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node-performance-pao-baseprofile"))

		exutil.By("Check value of allocatable.hugepages-1Gi in labled node ")
		nodeHugePagesOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.allocatable.hugepages-1Gi}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeHugePagesOutput).To(o.ContainSubstring("1Gi"))

		exutil.By("Check Settings of CPU Manager policy created by PAO in labled node ")
		cpuManagerConfOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep cpuManager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cpuManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("cpuManagerPolicy"))
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("cpuManagerReconcilePeriod"))
		e2e.Logf("The settings of CPU Manager Policy on labeled nodes: \n%v", cpuManagerConfOutput)

		exutil.By("Check Settings of CPU Manager for reservedSystemCPUs created by PAO in labled node ")
		cpuManagerConfOutput, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep reservedSystemCPUs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cpuManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("reservedSystemCPUs"))
		e2e.Logf("The settings of CPU Manager reservedSystemCPUs on labeled nodes: \n%v", cpuManagerConfOutput)

		exutil.By("Check Settings of Topology Manager for topologyManagerPolicy created by PAO in labled node ")
		topologyManagerConfOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep topologyManagerPolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(topologyManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(topologyManagerConfOutput).To(o.ContainSubstring("topologyManagerPolicy"))
		e2e.Logf("The settings of CPU Manager topologyManagerPolicy on labeled nodes: \n%v", topologyManagerConfOutput)

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform == "aws" || iaasPlatform == "gcp" {
			exutil.By("Check realTime kernel setting that created by PAO in labled node ")
			realTimekernalOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-owide").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(realTimekernalOutput).NotTo(o.BeEmpty())
			o.Expect(realTimekernalOutput).To(o.Or(o.ContainSubstring("rt")))
		} else {
			exutil.By("Check realTime kernel setting that created by PAO in labled node ")
			realTimekernalOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-owide").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(realTimekernalOutput).NotTo(o.BeEmpty())
			o.Expect(realTimekernalOutput).NotTo(o.Or(o.ContainSubstring("rt")))
		}

		exutil.By("Check runtimeClass setting that created by PAO ... ")
		runtimeClassOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("performanceprofile", "pao-baseprofile", "-ojsonpath={.status.runtimeClass}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runtimeClassOutput).NotTo(o.BeEmpty())
		o.Expect(runtimeClassOutput).To(o.ContainSubstring("performance-pao-baseprofile"))
		e2e.Logf("The settings of runtimeClass on labeled nodes: \n%v", runtimeClassOutput)

		exutil.By("Check Kernel boot settings passed into /proc/cmdline in labled node ")
		kernelCMDLineStdout, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "cat", "/proc/cmdline").Output()
		e2e.Logf("The settings of Kernel boot passed into /proc/cmdline  on labeled nodes: \n%v", kernelCMDLineStdout)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kernelCMDLineStdout).NotTo(o.BeEmpty())
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("tsc=reliable"))
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("isolcpus="))
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("hugepagesz=1G"))
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("nosmt"))
	})

	g.It("Longduration-NonPreRelease-PstChkUpgrade-Author:liqcui-Medium-49618-Telco N-1 - Post Check for PAO shipped with NTO to support upgrade.[Telco][Disruptive][Slow].", func() {

		if !isNTO {
			g.Skip("NTO is not installed or is Single Node Cluster- skipping test ...")
		}

		isSNO := exutil.IsSNOCluster(oc)

		// currently test is only supported on AWS, GCP, Azure, ibmcloud, alibabacloud
		supportPlatforms := []string{"aws", "gcp", "azure", "ibmcloud", "alibabacloud"}

		if !implStringArrayContains(supportPlatforms, iaasPlatform) || isSNO {
			g.Skip("IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		tunedNodeName, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker-pao", "-ojsonpath={.items[*].metadata.name}").Output()
		if len(tunedNodeName) == 0 {
			g.Skip("No labeled node was found, skipping testing ...")
		} else {
			defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-pao-").Execute()
		}

		defer exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-pao", "worker-pao", 480)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("performanceprofile", "pao-baseprofile", "--ignore-not-found").Execute()

		exutil.By("Check If Performance Profile pao-baseprofile and cloud-provider exist during Post Check Phase")
		paoBasePerformanceProfile, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("performanceprofile").Output()

		if !strings.Contains(paoBasePerformanceProfile, "pao-baseprofile") {
			g.Skip("No Performancerofile found skipping test ...")
		}

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		exutil.By("Assert if machine config pool applied for worker nodes")
		exutil.AssertIfMCPChangesAppliedByName(oc, "worker-pao", 1200)

		exutil.By("Check openshift-node-performance-pao-baseprofile tuned profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).To(o.ContainSubstring("openshift-node-performance-pao-baseprofile"))

		exutil.By("Check current profile openshift-node-performance-pao-baseprofile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check if new NTO profile openshift-node-performance-pao-baseprofile was applied")
		assertIfTunedProfileApplied(oc, ntoNamespace, tunedNodeName, "openshift-node-performance-pao-baseprofile")

		exutil.By("Check if profile openshift-node-performance-pao-baseprofile applied on nodes")
		nodeProfileName, err := getTunedProfile(oc, ntoNamespace, tunedNodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeProfileName).To(o.ContainSubstring("openshift-node-performance-pao-baseprofile"))

		exutil.By("Check value of allocatable.hugepages-1Gi in labled node ")
		nodeHugePagesOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.status.allocatable.hugepages-1Gi}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodeHugePagesOutput).To(o.ContainSubstring("1Gi"))

		exutil.By("Check Settings of CPU Manager policy created by PAO in labled node ")
		cpuManagerConfOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep cpuManager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cpuManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("cpuManagerPolicy"))
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("cpuManagerReconcilePeriod"))
		e2e.Logf("The settings of CPU Manager Policy on labeled nodes: \n%v", cpuManagerConfOutput)

		exutil.By("Check Settings of CPU Manager for reservedSystemCPUs created by PAO in labled node ")
		cpuManagerConfOutput, err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep reservedSystemCPUs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cpuManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(cpuManagerConfOutput).To(o.ContainSubstring("reservedSystemCPUs"))
		e2e.Logf("The settings of CPU Manager reservedSystemCPUs on labeled nodes: \n%v", cpuManagerConfOutput)

		exutil.By("Check Settings of Topology Manager for topologyManagerPolicy created by PAO in labled node ")
		topologyManagerConfOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "/bin/bash", "-c", "cat /etc/kubernetes/kubelet.conf  |grep topologyManagerPolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(topologyManagerConfOutput).NotTo(o.BeEmpty())
		o.Expect(topologyManagerConfOutput).To(o.ContainSubstring("topologyManagerPolicy"))
		e2e.Logf("The settings of CPU Manager topologyManagerPolicy on labeled nodes: \n%v", topologyManagerConfOutput)

		// currently test is only supported on AWS, GCP, and Azure
		if iaasPlatform == "aws" || iaasPlatform == "gcp" {
			exutil.By("Check realTime kernel setting that created by PAO in labled node ")
			realTimekernalOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-owide").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(realTimekernalOutput).NotTo(o.BeEmpty())
			o.Expect(realTimekernalOutput).To(o.Or(o.ContainSubstring("rt")))
		} else {
			exutil.By("Check realTime kernel setting that created by PAO in labled node ")
			realTimekernalOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-owide").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(realTimekernalOutput).NotTo(o.BeEmpty())
			o.Expect(realTimekernalOutput).NotTo(o.Or(o.ContainSubstring("rt")))
		}

		exutil.By("Check runtimeClass setting that created by PAO ... ")
		runtimeClassOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("performanceprofile", "pao-baseprofile", "-ojsonpath={.status.runtimeClass}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(runtimeClassOutput).NotTo(o.BeEmpty())
		o.Expect(runtimeClassOutput).To(o.ContainSubstring("performance-pao-baseprofile"))
		e2e.Logf("The settings of runtimeClass on labeled nodes: \n%v", runtimeClassOutput)

		exutil.By("Check Kernel boot settings passed into /proc/cmdline in labled node ")
		kernelCMDLineStdout, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", ntoNamespace, "--quiet=true", "node/"+tunedNodeName, "--", "chroot", "/host", "cat", "/proc/cmdline").Output()
		e2e.Logf("The settings of Kernel boot passed into /proc/cmdline  on labeled nodes: \n%v", kernelCMDLineStdout)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(kernelCMDLineStdout).NotTo(o.BeEmpty())
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("tsc=reliable"))
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("isolcpus="))
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("hugepagesz=1G"))
		o.Expect(kernelCMDLineStdout).To(o.ContainSubstring("nosmt"))

		//The custom mc and mcp must be deleted by correct sequence, unlabel first and labeled node return to worker mcp, then delete mc and mcp
		//otherwise the mcp will keep degrade state, it will affected other test case that use mcp
		exutil.By("Delete custom MC and MCP by following correct logic ...")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-pao-").Execute()
		exutil.DeleteMCAndMCPByName(oc, "50-nto-worker-pao", "worker-pao", 480)
	})

	g.It("NonPreRelease-PreChkUpgrade-Author:liqcui-Medium-21995-Pre Check for basic NTO function to Upgrade OCP Cluster[Disruptive].", func() {

		// currently test is only supported on AWS, GCP, Azure, ibmcloud, alibabacloud
		supportPlatforms := []string{"aws", "gcp", "azure", "ibmcloud", "alibabacloud"}

		if !implStringArrayContains(supportPlatforms, iaasPlatform) || !isNTO {
			g.Skip("NTO is not installed or IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		tunedNodeName := choseOneWorkerNodeToRunCase(oc, 1)

		paoNodeName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker-pao", "-ojsonpath={.items[*].metadata.name}").Output()
		if len(tunedNodeName) == 0 || tunedNodeName == paoNodeName {
			g.Skip("No suitable worker node was found in : " + iaasPlatform + " - skipping test ...")
		}

		exutil.By("Label the node with node-role.kubernetes.io/worker-tuning=")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-tuning=", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		ntoRes := ntoResource{
			name:        "tuning-pidmax",
			namespace:   ntoNamespace,
			template:    ntoSysctlTemplate,
			sysctlparm:  "kernel.pid_max",
			sysctlvalue: "282828",
		}

		exutil.By("Create tuning-pidmax profile")
		ntoRes.applyNTOTunedProfile(oc)

		exutil.By("Create tuning-pidmax profile tuning-pidmax applied to nodes")
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Compare if the value kernel.pid_max in on labeled node, should be 282828")
		compareSpecifiedValueByNameOnLabelNodewithRetry(oc, ntoNamespace, tunedNodeName, "kernel.pid_max", "282828")

		exutil.By("Get cloud provider name ...")
		providerName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("profiles.tuned.openshift.io", tunedNodeName, "-n", ntoNamespace, "-ojsonpath={.spec.config.providerName}").Output()
		o.Expect(providerName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		providerID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.spec.providerID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(providerID).NotTo(o.BeEmpty())
		o.Expect(providerID).To(o.ContainSubstring(providerName))

		exutil.By("Apply cloud-provider profile ...")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", cloudProviderFile, "-p", "PROVIDER_NAME="+providerName)

		exutil.By("Check provider + providerName profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).NotTo(o.BeEmpty())
		o.Expect(tunedNames).To(o.ContainSubstring("provider-" + providerName))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check the value of vm.admin_reserve_kbytes on target nodes, the expected value is 16386")
		compareSpecifiedValueByNameOnLabelNodewithRetry(oc, ntoNamespace, tunedNodeName, "vm.admin_reserve_kbytes", "16386")
	})
	g.It("NonPreRelease-PstChkUpgrade-Author:liqcui-Medium-21995-Post Check for basic NTO function to Upgrade OCP Cluster[Disruptive]", func() {

		// currently test is only supported on AWS, GCP, Azure, ibmcloud, alibabacloud
		supportPlatforms := []string{"aws", "gcp", "azure", "ibmcloud", "alibabacloud"}

		if !implStringArrayContains(supportPlatforms, iaasPlatform) || !isNTO {
			g.Skip("NTO is not installed or IAAS platform: " + iaasPlatform + " is not automated yet - skipping test ...")
		}

		tunedNodeName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker-tuning", "-ojsonpath={.items[*].metadata.name}").Output()
		if len(tunedNodeName) == 0 {
			g.Skip("No suitable worker node was found in : " + iaasPlatform + " - skipping test ...")
		}

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-tuning-").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "tuning-pidmax", "-n", ntoNamespace, "--ignore-not-found").Execute()

		//Get the tuned pod name in the same node that labeled node
		tunedPodName := getTunedPodNamebyNodeName(oc, tunedNodeName, ntoNamespace)
		o.Expect(tunedPodName).NotTo(o.BeEmpty())

		exutil.By("Get cloud provider name ...")
		providerName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("profiles.tuned.openshift.io", tunedNodeName, "-n", ntoNamespace, "-ojsonpath={.spec.config.providerName}").Output()
		o.Expect(providerName).NotTo(o.BeEmpty())
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "provider-"+providerName, "-n", ntoNamespace, "--ignore-not-found").Execute()

		ntoRes := ntoResource{
			name:        "tuning-pidmax",
			namespace:   ntoNamespace,
			template:    ntoSysctlTemplate,
			sysctlparm:  "kernel.pid_max",
			sysctlvalue: "282828",
		}

		exutil.By("Create tuning-pidmax profile and apply it to nodes")
		ntoRes.assertTunedProfileApplied(oc, tunedNodeName)

		exutil.By("Check current profile for each node")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Compare if the value kernel.pid_max in on labeled node, should be 282828")
		compareSpecifiedValueByNameOnLabelNodewithRetry(oc, ntoNamespace, tunedNodeName, "kernel.pid_max", "282828")

		providerID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", tunedNodeName, "-ojsonpath={.spec.providerID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(providerID).NotTo(o.BeEmpty())
		o.Expect(providerID).To(o.ContainSubstring(providerName))

		exutil.By("Apply cloud-provider profile ...")
		exutil.ApplyNsResourceFromTemplate(oc, ntoNamespace, "--ignore-unknown-parameters=true", "-f", cloudProviderFile, "-p", "PROVIDER_NAME="+providerName)

		exutil.By("Check provider + providerName profile should be automatically created")
		tunedNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "tuned").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(tunedNames).NotTo(o.BeEmpty())
		o.Expect(tunedNames).To(o.ContainSubstring("provider-" + providerName))

		exutil.By("Check current profile for each node")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ntoNamespace, "profiles.tuned.openshift.io").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Current profile for each node: \n%v", output)

		exutil.By("Check the value of vm.admin_reserve_kbytes on target nodes, the expected value is 16386")
		compareSpecifiedValueByNameOnLabelNodewithRetry(oc, ntoNamespace, tunedNodeName, "vm.admin_reserve_kbytes", "16386")

		//Clean nto resource after upgrade
		oc.AsAdmin().WithoutNamespace().Run("label").Args("node", tunedNodeName, "node-role.kubernetes.io/worker-tuning-").Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "tuning-pidmax", "-n", ntoNamespace, "--ignore-not-found").Execute()
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("tuned", "provider-"+providerName, "-n", ntoNamespace, "--ignore-not-found").Execute()

	})
})
