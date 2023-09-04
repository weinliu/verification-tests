package node

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"k8s.io/apimachinery/pkg/util/wait"

	//e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-node] NODE initContainer policy,volume,readines,quota", func() {
	defer g.GinkgoRecover()

	var (
		oc                        = exutil.NewCLI("node-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir       = exutil.FixturePath("testdata", "node")
		customTemp                = filepath.Join(buildPruningBaseDir, "pod-modify.yaml")
		podTerminationTemp        = filepath.Join(buildPruningBaseDir, "pod-termination.yaml")
		podInitConTemp            = filepath.Join(buildPruningBaseDir, "pod-initContainer.yaml")
		podSleepTemp              = filepath.Join(buildPruningBaseDir, "sleepPod46306.yaml")
		kubeletConfigTemp         = filepath.Join(buildPruningBaseDir, "kubeletconfig-hardeviction.yaml")
		memHogTemp                = filepath.Join(buildPruningBaseDir, "mem-hog-ocp11600.yaml")
		podTwoContainersTemp      = filepath.Join(buildPruningBaseDir, "pod-with-two-containers.yaml")
		podUserNSTemp             = filepath.Join(buildPruningBaseDir, "pod-user-namespace.yaml")
		ctrcfgOverlayTemp         = filepath.Join(buildPruningBaseDir, "containerRuntimeConfig-overlay.yaml")
		podHelloTemp              = filepath.Join(buildPruningBaseDir, "pod-hello.yaml")
		podWkloadCpuTemp          = filepath.Join(buildPruningBaseDir, "pod-workload-cpu.yaml")
		podWkloadCpuNoAnTemp      = filepath.Join(buildPruningBaseDir, "pod-workload-cpu-without-anotation.yaml")
		podNoWkloadCpuTemp        = filepath.Join(buildPruningBaseDir, "pod-without-workload-cpu.yaml")
		runtimeTimeoutTemp        = filepath.Join(buildPruningBaseDir, "kubeletconfig-runReqTout.yaml")
		upgradeMachineConfigTemp1 = filepath.Join(buildPruningBaseDir, "custom-kubelet-test1.yaml")
		upgradeMachineConfigTemp2 = filepath.Join(buildPruningBaseDir, "custom-kubelet-test2.yaml")
		systemreserveTemp         = filepath.Join(buildPruningBaseDir, "kubeletconfig-defaultsysres.yaml")
		podLogLinkTemp            = filepath.Join(buildPruningBaseDir, "pod-loglink.yaml")

		podLogLink65404 = podLogLinkDescription{
			name:      "",
			namespace: "",
			template:  podLogLinkTemp,
		}

		podWkloadCpu52313 = podNoWkloadCpuDescription{
			name:      "",
			namespace: "",
			template:  podNoWkloadCpuTemp,
		}

		podWkloadCpu52326 = podWkloadCpuDescription{
			name:        "",
			namespace:   "",
			workloadcpu: "",
			template:    podWkloadCpuTemp,
		}

		podWkloadCpu52328 = podWkloadCpuDescription{
			name:        "",
			namespace:   "",
			workloadcpu: "",
			template:    podWkloadCpuTemp,
		}

		podWkloadCpu52329 = podWkloadCpuNoAnotation{
			name:        "",
			namespace:   "",
			workloadcpu: "",
			template:    podWkloadCpuNoAnTemp,
		}

		podHello = podHelloDescription{
			name:      "",
			namespace: "",
			template:  podHelloTemp,
		}

		podUserNS47663 = podUserNSDescription{
			name:      "",
			namespace: "",
			template:  podUserNSTemp,
		}

		podModify = podModifyDescription{
			name:          "",
			namespace:     "",
			mountpath:     "",
			command:       "",
			args:          "",
			restartPolicy: "",
			user:          "",
			role:          "",
			level:         "",
			template:      customTemp,
		}

		podTermination = podTerminationDescription{
			name:      "",
			namespace: "",
			template:  podTerminationTemp,
		}

		podInitCon38271 = podInitConDescription{
			name:      "",
			namespace: "",
			template:  podInitConTemp,
		}

		podSleep = podSleepDescription{
			namespace: "",
			template:  podSleepTemp,
		}

		kubeletConfig = kubeletConfigDescription{
			name:       "",
			labelkey:   "",
			labelvalue: "",
			template:   kubeletConfigTemp,
		}

		memHog = memHogDescription{
			name:       "",
			namespace:  "",
			labelkey:   "",
			labelvalue: "",
			template:   memHogTemp,
		}

		podTwoContainers = podTwoContainersDescription{
			name:      "",
			namespace: "",
			template:  podTwoContainersTemp,
		}

		ctrcfgOverlay = ctrcfgOverlayDescription{
			name:     "",
			overlay:  "",
			template: ctrcfgOverlayTemp,
		}
		runtimeTimeout = runtimeTimeoutDescription{
			name:       "",
			labelkey:   "",
			labelvalue: "",
			template:   runtimeTimeoutTemp,
		}

		upgradeMachineconfig1 = upgradeMachineconfig1Description{
			name:     "",
			template: upgradeMachineConfigTemp1,
		}
		upgradeMachineconfig2 = upgradeMachineconfig2Description{
			name:     "",
			template: upgradeMachineConfigTemp2,
		}
		systemReserveES = systemReserveESDescription{
			name:       "",
			labelkey:   "",
			labelvalue: "",
			template:   systemreserveTemp,
		}
	)
	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12893-Init containers with restart policy Always", func() {
		oc.SetupProject()
		podModify.name = "init-always-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "exit 1"
		podModify.restartPolicy = "Always"

		g.By("create FAILED init container with pod restartPolicy Always")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain CrashLoopBackOff")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with pod restartPolicy Always")

		podModify.name = "init-always-succ"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Always"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12894-Init containers with restart policy OnFailure", func() {
		oc.SetupProject()
		podModify.name = "init-onfailure-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "exit 1"
		podModify.restartPolicy = "OnFailure"

		g.By("create FAILED init container with pod restartPolicy OnFailure")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain CrashLoopBackOff")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with pod restartPolicy OnFailure")

		podModify.name = "init-onfailure-succ"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "OnFailure"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod ")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12896-Init containers with restart policy Never", func() {
		oc.SetupProject()
		podModify.name = "init-never-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "exit 1"
		podModify.restartPolicy = "Never"

		g.By("create FAILED init container with pod restartPolicy Never")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusterminatedReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain Error")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with pod restartPolicy Never")

		podModify.name = "init-never-succ"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Never"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod ")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12911-App container status depends on init containers exit code	", func() {
		oc.SetupProject()
		podModify.name = "init-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/false"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Never"

		g.By("create FAILED init container with exit code and command /bin/false")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusterminatedReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain Error")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with command /bin/true")
		podModify.name = "init-success"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/true"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Never"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod ")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12913-Init containers with volume work fine", func() {

		oc.SetupProject()
		podModify.name = "init-volume"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "echo This is OCP volume test > /work-dir/volume-test"
		podModify.restartPolicy = "Never"

		g.By("Create a pod with initContainer using volume\n")
		podModify.create(oc)
		g.By("Check pod status")
		err := podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Check Vol status\n")
		err = volStatus(oc)
		exutil.AssertWaitPollNoErr(err, "Init containers with volume do not work fine")
		g.By("Delete Pod\n")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("Author:pmali-Medium-30521-CRIO Termination Grace Period test", func() {

		oc.SetupProject()
		podTermination.name = "pod-termination"
		podTermination.namespace = oc.Namespace()

		g.By("Create a pod with termination grace period\n")
		podTermination.create(oc)
		g.By("Check pod status\n")
		err := podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Check container TimeoutStopUSec\n")
		err = podTermination.getTerminationGrace(oc)
		exutil.AssertWaitPollNoErr(err, "terminationGracePeriodSeconds is not valid")
		g.By("Delete Pod\n")
		podTermination.delete(oc)
	})

	// author: minmli@redhat.com
	g.It("Author:minmli-High-38271-Init containers should not restart when the exited init container is removed from node", func() {
		g.By("Test for case OCP-38271")
		oc.SetupProject()
		podInitCon38271.name = "initcon-pod"
		podInitCon38271.namespace = oc.Namespace()

		g.By("Create a pod with init container")
		podInitCon38271.create(oc)
		defer podInitCon38271.delete(oc)

		g.By("Check pod status")
		err := podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check init container exit normally")
		err = podInitCon38271.containerExit(oc)
		exutil.AssertWaitPollNoErr(err, "conainer not exit normally")

		g.By("Delete init container")
		_, err = podInitCon38271.deleteInitContainer(oc)
		exutil.AssertWaitPollNoErr(err, "fail to delete container")

		g.By("Check init container not restart again")
		err = podInitCon38271.initContainerNotRestart(oc)
		exutil.AssertWaitPollNoErr(err, "init container restart")
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-NonPreRelease-Longduration-Author:pmali-High-46306-Node should not becomes NotReady with error creating container storage layer not known[Disruptive][Slow]", func() {

		oc.SetupProject()
		podSleep.namespace = oc.Namespace()

		g.By("Get Worker Node and Add label app=sleep\n")
		workerNodeName := getSingleWorkerNode(oc)
		addLabelToNode(oc, "app=sleep", workerNodeName, "nodes")
		defer removeLabelFromNode(oc, "app-", workerNodeName, "nodes")

		g.By("Create a 50 pods on the same node\n")
		for i := 0; i < 50; i++ {
			podSleep.create(oc)
		}

		g.By("Check pod status\n")
		err := podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is NOT running")

		g.By("Delete project\n")
		go podSleep.deleteProject(oc)

		g.By("Reboot Worker node\n")
		go rebootNode(oc, workerNodeName)

		//g.By("****** Reboot Worker Node ****** ")
		//exutil.DebugNodeWithChroot(oc, workerNodeName, "reboot")

		g.By("Check Nodes Status\n")
		err = checkNodeStatus(oc, workerNodeName)
		exutil.AssertWaitPollNoErr(err, "node is not ready")

		g.By("Get Master node\n")
		masterNode := getSingleMasterNode(oc)

		g.By("Check Master Node Logs\n")
		err = masterNodeLog(oc, masterNode)
		exutil.AssertWaitPollNoErr(err, "Logs Found, Test Failed")
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Longduration-NonPreRelease-Author:pmali-Medium-11600-kubelet will evict pod immediately when met hard eviction threshold memory [Disruptive][Slow]", func() {

		oc.SetupProject()
		kubeletConfig.name = "kubeletconfig-ocp11600"
		kubeletConfig.labelkey = "custom-kubelet-ocp11600"
		kubeletConfig.labelvalue = "hard-eviction"

		memHog.name = "mem-hog-ocp11600"
		memHog.namespace = oc.Namespace()
		memHog.labelkey = kubeletConfig.labelkey
		memHog.labelvalue = kubeletConfig.labelvalue

		g.By("Get Worker Node and Add label custom-kubelet-ocp11600=hard-eviction\n")
		addLabelToNode(oc, "custom-kubelet-ocp11600=hard-eviction", "worker", "mcp")
		defer removeLabelFromNode(oc, "custom-kubelet-ocp11600-", "worker", "mcp")

		g.By("Create Kubelet config \n")
		kubeletConfig.create(oc)
		defer getmcpStatus(oc, "worker") // To check all the Nodes are in Ready State after deleteing kubeletconfig
		defer cleanupObjectsClusterScope(oc, objectTableRefcscope{"kubeletconfig", "kubeletconfig-ocp11600"})

		g.By("Make sure Worker mcp is Updated correctly\n")
		err := getmcpStatus(oc, "worker")
		exutil.AssertWaitPollNoErr(err, "mcp is not updated")

		g.By("Create a 10 pods on the same node\n")
		for i := 0; i < 10; i++ {
			memHog.create(oc)
		}
		defer cleanupObjectsClusterScope(oc, objectTableRefcscope{"ns", oc.Namespace()})

		g.By("Check worker Node events\n")
		workerNodeName := getSingleWorkerNode(oc)
		err = getWorkerNodeDescribe(oc, workerNodeName)
		exutil.AssertWaitPollNoErr(err, "Logs did not Found memory pressure, Test Failed")
	})

	// author: weinliu@redhat.com
	g.It("Author:weinliu-Critical-11055-/dev/shm can be automatically shared among all of a pod's containers", func() {
		g.By("Test for case OCP-11055")
		oc.SetupProject()
		podTwoContainers.name = "pod-twocontainers"
		podTwoContainers.namespace = oc.Namespace()
		g.By("Create a pod with two containers")
		podTwoContainers.create(oc)
		defer podTwoContainers.delete(oc)
		g.By("Check pod status")
		err := podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Enter container 1 and write files")
		_, err = exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift", "echo 'written_from_container1' > /dev/shm/c1")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Enter container 2 and check whether it can share container 1 shared files")
		containerFile1, err := exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift-fedora", "cat /dev/shm/c1")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Container1 File Content is: %v", containerFile1)
		o.Expect(containerFile1).To(o.Equal("written_from_container1"))
		g.By("Enter container 2 and write files")
		_, err = exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift-fedora", "echo 'written_from_container2' > /dev/shm/c2")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Enter container 1 and check whether it can share container 2 shared files")
		containerFile2, err := exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift", "cat /dev/shm/c2")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Container2 File Content is: %v", containerFile2)
		o.Expect(containerFile2).To(o.Equal("written_from_container2"))
	})

	// author: minmli@redhat.com
	g.It("Author:minmli-High-47663-run pods in user namespaces via crio workload annotation", func() {
		oc.SetupProject()
		g.By("Test for case OCP-47663")
		podUserNS47663.name = "userns-47663"
		podUserNS47663.namespace = oc.Namespace()

		g.By("Check workload of openshift-builder exist in crio config")
		err := podUserNS47663.crioWorkloadConfigExist(oc)
		exutil.AssertWaitPollNoErr(err, "crio workload config not exist")

		g.By("Check user containers exist in /etc/sub[ug]id")
		err = podUserNS47663.userContainersExistForNS(oc)
		exutil.AssertWaitPollNoErr(err, "user containers not exist for user namespace")

		g.By("Create a pod with annotation of openshift-builder workload")
		podUserNS47663.createPodUserNS(oc)
		defer podUserNS47663.deletePodUserNS(oc)

		g.By("Check pod status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check pod run in user namespace")
		err = podUserNS47663.podRunInUserNS(oc)
		exutil.AssertWaitPollNoErr(err, "pod not run in user namespace")
	})

	// author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-52328-set workload resource usage from pod level : pod should not take effect if not defaulted or specified in workload [Disruptive][Slow]", func() {
		oc.SetupProject()
		g.By("Test for case OCP-52328")

		g.By("Create a machine config for workload setting")
		mcCpuOverride := filepath.Join(buildPruningBaseDir, "machineconfig-cpu-override-52328.yaml")
		mcpName := "worker"
		defer func() {
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcCpuOverride).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcCpuOverride).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check mcp finish rolling out")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		g.By("Check workload setting is as expected")
		wkloadConfig := []string{"crio.runtime.workloads.management", "activation_annotation = \"io.openshift.manager\"", "annotation_prefix = \"io.openshift.workload.manager\"", "crio.runtime.workloads.management.resources", "cpushares = 512"}
		configPath := "/etc/crio/crio.conf.d/01-workload.conf"
		err = crioConfigExist(oc, wkloadConfig, configPath)
		exutil.AssertWaitPollNoErr(err, "workload setting is not set as expected")

		g.By("Create a pod only override the default cpushares in workload setting by annotation")
		defer podWkloadCpu52328.delete(oc)
		podWkloadCpu52328.name = "wkloadcpu-52328"
		podWkloadCpu52328.namespace = oc.Namespace()
		podWkloadCpu52328.workloadcpu = "{\"cpuset\": \"\", \"cpushares\": 1024}"
		podWkloadCpu52328.create(oc)

		g.By("Check pod status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check the pod only override cpushares")
		cpuset := ""
		cpushare := "1024"
		err = overrideWkloadCpu(oc, cpuset, cpushare, podWkloadCpu52328.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod not only override cpushares in workload setting")
	})

	// author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-52313-High-52326-High-52329-set workload resource usage from pod level : pod can get configured to defaults and override defaults and pod should not be set if annotation not specified [Disruptive][Slow]", func() {
		oc.SetupProject()
		g.By("Test for case OCP-52313, OCP-52326 and OCP-52329")

		g.By("Create a machine config for workload setting")
		mcCpuOverride := filepath.Join(buildPruningBaseDir, "machineconfig-cpu-override.yaml")
		defer func() {
			mcpName := "worker"
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcCpuOverride).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcCpuOverride).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check mcp finish rolling out")
		mcpName := "worker"
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		g.By("Check workload setting is as expected")
		wkloadConfig := []string{"crio.runtime.workloads.management", "activation_annotation = \"io.openshift.manager\"", "annotation_prefix = \"io.openshift.workload.manager\"", "crio.runtime.workloads.management.resources", "cpushares = 512", "cpuset = \"0\""}
		configPath := "/etc/crio/crio.conf.d/01-workload.conf"
		err = crioConfigExist(oc, wkloadConfig, configPath)
		exutil.AssertWaitPollNoErr(err, "workload setting is not set as expected")

		g.By("Create a pod with default workload setting by annotation")
		podWkloadCpu52313.name = "wkloadcpu-52313"
		podWkloadCpu52313.namespace = oc.Namespace()
		podWkloadCpu52313.create(oc)

		g.By("Check pod status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check the pod get configured to default workload setting")
		cpuset := "0"
		cpushare := "512"
		err = overrideWkloadCpu(oc, cpuset, cpushare, podWkloadCpu52313.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod is not configured to default workload setting")
		podWkloadCpu52313.delete(oc)

		g.By("Create a pod override the default workload setting by annotation")
		podWkloadCpu52326.name = "wkloadcpu-52326"
		podWkloadCpu52326.namespace = oc.Namespace()
		podWkloadCpu52326.workloadcpu = "{\"cpuset\": \"0-1\", \"cpushares\": 200}"
		podWkloadCpu52326.create(oc)

		g.By("Check pod status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check the pod override the default workload setting")
		cpuset = "0-1"
		cpushare = "200"
		err = overrideWkloadCpu(oc, cpuset, cpushare, podWkloadCpu52326.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod not override the default workload setting")
		podWkloadCpu52326.delete(oc)

		g.By("Create a pod without annotation but with prefix")
		defer podWkloadCpu52329.delete(oc)
		podWkloadCpu52329.name = "wkloadcpu-52329"
		podWkloadCpu52329.namespace = oc.Namespace()
		podWkloadCpu52329.workloadcpu = "{\"cpuset\": \"0-1\", \"cpushares\": 1800}"
		podWkloadCpu52329.create(oc)

		g.By("Check pod status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check the pod keep default workload setting")
		cpuset = "0-1"
		cpushare = "1800"
		err = defaultWkloadCpu(oc, cpuset, cpushare, podWkloadCpu52329.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod not keep efault workload setting")
	})

	// author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-46313-set overlaySize in containerRuntimeConfig should take effect in container [Disruptive][Slow]", func() {
		oc.SetupProject()
		g.By("Test for case OCP-46313")
		ctrcfgOverlay.name = "ctrcfg-46313"
		ctrcfgOverlay.overlay = "9G"

		g.By("Create a containerRuntimeConfig to set overlaySize")
		ctrcfgOverlay.create(oc)
		defer func() {
			g.By("Deleting configRuntimeConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"ContainerRuntimeConfig", "ctrcfg-46313"})
			g.By("Check mcp finish rolling out")
			err := getmcpStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()

		g.By("Check mcp finish rolling out")
		err := getmcpStatus(oc, "worker")
		exutil.AssertWaitPollNoErr(err, "mcp is not updated")

		g.By("Check overlaySize take effect in config file")
		err = checkOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "overlaySize not take effect")

		g.By("Create a pod")
		podTermination.name = "pod-46313"
		podTermination.namespace = oc.Namespace()
		podTermination.create(oc)
		defer podTermination.delete(oc)

		g.By("Check pod status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check in pod the root partition size for Overlay is correct.")
		err = checkPodOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "pod overlay size is not correct !!!")
	})

	g.It("Author:minmli-High-56266-kubelet/crio will delete netns when a pod is deleted", func() {
		g.By("Test for case OCP-56266")
		oc.SetupProject()

		g.By("Create a pod")
		podHello.name = "pod-56266"
		podHello.namespace = oc.Namespace()
		podHello.create(oc)

		g.By("Check pod status")
		err := podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Get Pod's Node name")
		hostname := getPodNodeName(oc, podHello.namespace)

		g.By("Get Pod's NetNS")
		netNsPath, err := getPodNetNs(oc, hostname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete the pod")
		podHello.delete(oc)

		g.By("Check the NetNs file was cleaned")
		err = checkNetNs(oc, hostname, netNsPath)
		exutil.AssertWaitPollNoErr(err, "the NetNs file is not cleaned !!!")
	})

	g.It("Author:minmli-High-55486-check not exist error MountVolume SetUp failed for volume serviceca object openshift-image-registry serviceca not registered", func() {
		g.By("Test for case OCP-55486")
		oc.SetupProject()

		g.By("Check events of each cronjob")
		err := checkEventsForErr(oc)
		exutil.AssertWaitPollNoErr(err, "Found error: MountVolume.SetUp failed for volume ... not registered ")
	})
	//author: asahay@redhat.com
	g.It("Author:asahay-Medium-55033-check KUBELET_LOG_LEVEL is 2", func() {
		g.By("Test for OCP-55033")
		g.By("check Kubelet Log Level\n")
		assertKubeletLogLevel(oc)
	})

	//author: asahay@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:asahay-High-52472-update runtimeRequestTimeout parameter using KubeletConfig CR [Disruptive][Slow]", func() {

		oc.SetupProject()
		runtimeTimeout.name = "kubeletconfig-52472"
		runtimeTimeout.labelkey = "custom-kubelet"
		runtimeTimeout.labelvalue = "test-timeout"

		g.By("Label mcp worker custom-kubelet as test-timeout \n")
		addLabelToNode(oc, "custom-kubelet=test-timeout", "worker", "mcp")
		defer removeLabelFromNode(oc, "custom-kubelet-", "worker", "mcp")

		g.By("Create KubeletConfig \n")
		defer func() {
			mcpName := "worker"
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer runtimeTimeout.delete(oc)
		runtimeTimeout.create(oc)

		g.By("Check mcp finish rolling out")
		mcpName := "worker"
		err := checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		g.By("Check Runtime Request Timeout")
		runTimeTimeout(oc)
	})

	//author :asahay@redhat.com

	g.It("NonHyperShiftHOST-NonPreRelease-PreChkUpgrade-Author:asahay-High-45436-Upgrading a cluster by making sure not keep duplicate machine config when it has multiple kubeletconfig [Disruptive][Slow]", func() {

		upgradeMachineconfig1.name = "max-pod"
		upgradeMachineconfig2.name = "max-pod-1"
		g.By("Create first KubeletConfig \n")
		upgradeMachineconfig1.create(oc)

		g.By("Check mcp finish rolling out")
		mcpName := "worker"
		err := checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		g.By("Create second KubeletConfig \n")
		upgradeMachineconfig2.create(oc)

		g.By("Check mcp finish rolling out")
		mcpName1 := "worker"
		err1 := checkMachineConfigPoolStatus(oc, mcpName1)
		exutil.AssertWaitPollNoErr(err1, "macineconfigpool worker update failed")

	})

	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:asahay-High-45436-post check Upgrading a cluster by making sure not keep duplicate machine config when it has multiple kubeletconfig [Disruptive][Slow]", func() {
		upgradeMachineconfig1.name = "max-pod"
		defer func() {
			g.By("Delete the KubeletConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"KubeletConfig", upgradeMachineconfig1.name})
			g.By("Check mcp finish rolling out")
			err := checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()

		upgradeMachineconfig2.name = "max-pod-1"
		defer func() {
			g.By("Delete the KubeletConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"KubeletConfig", upgradeMachineconfig2.name})
			g.By("Check mcp finish rolling out")
			err := checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()
		g.By("Checking no duplicate machine config")
		checkUpgradeMachineConfig(oc)

	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PreChkUpgrade-Author:minmli-High-45351-prepare to check crioConfig[Disruptive][Slow]", func() {
		g.By("1) oc debug one worker and edit /etc/crio/crio.conf")
		// we update log_level = "debug" in /etc/crio/crio.conf
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodename := nodeList.Items[0].Name
		_, err = exutil.DebugNodeWithChroot(oc, nodename, "/bin/bash", "-c", "sed -i 's/log_level = \"info\"/log_level = \"debug\"/g' /etc/crio/crio.conf")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2) create a ContainerRuntimeConfig to set overlaySize")
		ctrcfgOverlay.name = "ctrcfg-45351"
		ctrcfgOverlay.overlay = "35G"
		mcpName := "worker"
		ctrcfgOverlay.create(oc)

		g.By("3) check mcp finish rolling out")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "mcp update failed")

		g.By("4) check overlaySize update as expected")
		err = checkOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "overlaySize not update as expected")
	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:minmli-High-45351-post check crioConfig[Disruptive][Slow]", func() {
		g.By("1) check overlaySize don't change after upgrade")
		ctrcfgOverlay.name = "ctrcfg-45351"
		ctrcfgOverlay.overlay = "35G"

		defer func() {
			g.By("Delete the configRuntimeConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"ContainerRuntimeConfig", ctrcfgOverlay.name})
			g.By("Check mcp finish rolling out")
			err := checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()

		defer func() {
			g.By("Restore /etc/crio/crio.conf")
			nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, node := range nodeList.Items {
				nodename := node.Name
				_, err = exutil.DebugNodeWithChroot(oc, nodename, "/bin/bash", "-c", "sed -i 's/log_level = \"debug\"/log_level = \"info\"/g' /etc/crio/crio.conf")
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()

		err := checkOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "overlaySize change after upgrade")

		g.By("2) check conmon value from crio config")
		//we need check every node for the conmon = ""
		checkConmonForAllNode(oc)
	})

	g.It("Author:asahay-Medium-57332-collecting the audit log with must gather", func() {

		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-57332").Output()
		g.By("Running the must gather command \n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/tmp/must-gather-57332", "--", "/usr/bin/gather_audit_logs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("check the must-gather result")
		_, err = exec.Command("bash", "-c", "ls -l /tmp/must-gather-57332").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-57401-Create ImageDigestMirrorSet successfully [Disruptive][Slow]", func() {
		//If a cluster contains any ICSP or IDMS, it will skip the case
		if checkICSP(oc) || checkIDMS(oc) {
			g.Skip("This cluster contain ICSP or IDMS, skip the test.")
		}
		exutil.By("Create an ImageDigestMirrorSet")
		idms := filepath.Join(buildPruningBaseDir, "ImageDigestMirrorSet.yaml")
		defer func() {
			err := checkMachineConfigPoolStatus(oc, "master")
			exutil.AssertWaitPollNoErr(err, "macineconfigpool master update failed")
			err = checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + idms).Execute()

		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + idms).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the mcp finish updating")
		err = checkMachineConfigPoolStatus(oc, "master")
		exutil.AssertWaitPollNoErr(err, "macineconfigpool master update failed")
		err = checkMachineConfigPoolStatus(oc, "worker")
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check the ImageDigestMirrorSet apply to config")
		err = checkRegistryForIdms(oc)
		exutil.AssertWaitPollNoErr(err, "check registry config failed")

		exutil.By("The ImageContentSourcePolicy can't exist wiht ImageDigestMirrorSet or ImageTagMirrorSet")
		icsp := filepath.Join(buildPruningBaseDir, "ImageContentSourcePolicy.yaml")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", icsp).Output()
		o.Expect(strings.Contains(out, "Kind.ImageContentSourcePolicy: Forbidden: can't create ImageContentSourcePolicy when ImageDigestMirrorSet resources exist")).To(o.BeTrue())
	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-Author:minmli-Medium-59552-Enable image signature verification for Red Hat Container Registries [Serial]", func() {
		exutil.By("Apply a machine config to set image signature policy for worker nodes")
		mcImgSig := filepath.Join(buildPruningBaseDir, "machineconfig-image-signature-59552.yaml")
		mcpName := "worker"
		defer func() {
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcImgSig).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcImgSig).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the mcp finish updating")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check the signature configuration policy.json")
		err = checkImgSignature(oc)
		exutil.AssertWaitPollNoErr(err, "check signature configuration failed")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:asahay-Medium-62746-A default SYSTEM_RESERVED_ES value is applied if it is empty [Disruptive][Slow]", func() {

		exutil.By("set SYSTEM_RESERVED_ES as empty")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodename := nodeList.Items[0].Name
		_, err = exutil.DebugNodeWithChroot(oc, nodename, "/bin/bash", "-c", "sed -i 's/SYSTEM_RESERVED_ES=1Gi/SYSTEM_RESERVED_ES=/g' /etc/crio/crio.conf")
		o.Expect(err).NotTo(o.HaveOccurred())

		systemReserveES.name = "kubeletconfig-62746"
		systemReserveES.labelkey = "custom-kubelet"
		systemReserveES.labelvalue = "reserve-space"

		exutil.By("Label mcp worker custom-kubelet as reserve-space \n")
		addLabelToNode(oc, "custom-kubelet=reserve-space", "worker", "mcp")
		defer removeLabelFromNode(oc, "custom-kubelet-", "worker", "mcp")

		exutil.By("Create KubeletConfig \n")
		defer func() {
			mcpName := "worker"
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer systemReserveES.delete(oc)
		systemReserveES.create(oc)

		exutil.By("Check mcp finish rolling out")
		mcpName := "worker"
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check Default value")
		parameterCheck(oc)
	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-65404-log link inside pod via crio works well [Disruptive]", func() {
		exutil.By("Apply a machine config to enable log link via crio")
		mcLogLink := filepath.Join(buildPruningBaseDir, "machineconfig-log-link.yaml")
		mcpName := "worker"
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcLogLink).Execute()
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcLogLink).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the mcp finish updating")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check the crio config as expected")
		logLinkConfig := []string{"crio.runtime.workloads.linked", "activation_annotation = \"io.kubernetes.cri-o.LinkLogs\"", "allowed_annotations = [ \"io.kubernetes.cri-o.LinkLogs\" ]"}
		configPath := "/etc/crio/crio.conf.d/99-linked-log.conf"
		err = crioConfigExist(oc, logLinkConfig, configPath)
		exutil.AssertWaitPollNoErr(err, "crio config is not set as expected")

		exutil.By("Create a pod with LinkLogs annotation")
		podLogLink65404.name = "httpd"
		podLogLink65404.namespace = oc.Namespace()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer podLogLink65404.delete(oc)
		podLogLink65404.create(oc)

		exutil.By("Check pod status")
		err = podStatus(oc)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		exutil.By("Check log link successfully")
		checkLogLink(oc, podLogLink65404.namespace)
	})
})

var _ = g.Describe("[sig-node] NODE keda", func() {

	defer g.GinkgoRecover()
	var (
		oc                        = exutil.NewCLI("keda-operator", exutil.KubeConfigPath())
		cmaKedaControllerTemplate string
	)
	g.BeforeEach(func() {
		// skip ARM64 arch
		architecture.SkipNonAmd64SingleArch(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "node")
		cmaKedaControllerTemplate = filepath.Join(buildPruningBaseDir, "cma-keda-controller-template.yaml")
		exutil.SkipMissingQECatalogsource(oc)
		createKedaOperator(oc)
	})
	// author: weinliu@redhat.com
	g.It("StagerunBoth-Author:weinliu-High-52383-Keda Install", func() {
		g.By("CMA (Keda) operator has been installed successfully")
	})

	// author: weinliu@redhat.com
	g.It("StagerunBoth-Author:weinliu-High-62570-Verify must-gather tool works with CMA", func() {
		var (
			mustgatherName = "mustgather" + getRandomString()
			mustgatherDir  = "/tmp/" + mustgatherName
			mustgatherLog  = mustgatherName + ".log"
			logFile        string
		)
		g.By("Get the mustGatherImage")
		mustGatherImage, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("packagemanifest", "-n=openshift-marketplace", "openshift-custom-metrics-autoscaler-operator", "-o=jsonpath={.status.channels[?(.name=='stable')].currentCSVDesc.annotations.containerImage}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Running the must gather command \n")
		defer os.RemoveAll(mustgatherDir)
		logFile, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir="+mustgatherDir, "--image="+mustGatherImage).Output()
		if err != nil {
			e2e.Logf("mustgather created from image %v in %v logged to %v,%v %v", mustGatherImage, mustgatherDir, mustgatherLog, logFile, err)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	})
	// author: weinliu@redhat.com
	g.It("Author:weinliu-High-60961-Audit logging test - stdout Metadata[Serial]", func() {
		g.By("Create KedaController with log level Metadata")
		g.By("Create CMA Keda Controller ")
		cmaKedaController := cmaKedaControllerDescription{
			level:     "Metadata",
			template:  cmaKedaControllerTemplate,
			name:      "keda",
			namespace: "openshift-keda",
		}
		defer cmaKedaController.delete(oc)
		cmaKedaController.create(oc)
		metricsApiserverPodName := getPodNameByLabel(oc, "openshift-keda", "app=keda-metrics-apiserver")
		waitPodReady(oc, "openshift-keda", "app=keda-metrics-apiserver")
		g.By("Check the Audit Logged as configed")
		log, err := exutil.GetSpecificPodLogs(oc, "openshift-keda", "", metricsApiserverPodName[0], "")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(log, "\"level\":\"Metadata\"")).Should(o.BeTrue())
	})
})

var _ = g.Describe("[sig-node] NODE VPA Vertical Pod Autoscaler", func() {

	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("vpa-operator", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipMissingQECatalogsource(oc)
		createVpaOperator(oc)
	})
	// author: weinliu@redhat.com
	g.It("StagerunBoth-Author:weinliu-High-60991-VPA Install", func() {
		g.By("VPA operator is installed successfully")
	})
})

var _ = g.Describe("[sig-node] NODE Install and verify Cluster Resource Override Admission Webhook", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("clusterresourceoverride-operator", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {

		g.By("Skip test when precondition not meet !!!")
		exutil.SkipMissingQECatalogsource(oc)
		installOperatorClusterresourceoverride(oc)

	})
	// author: asahay@redhat.com

	g.It("StagerunBoth-Author:asahay-High-27070-Cluster Resource Override Operator. [Serial]", func() {
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterResourceOverride", "cluster", "-n", "clusterresourceoverride-operator").Execute()
		createCRClusterresourceoverride(oc)
		var err error
		var croCR string
		errCheck := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			croCR, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterResourceOverride", "cluster", "-n", "clusterresourceoverride-operator").Output()
			if err != nil {
				e2e.Logf("error  %v, please try next round", err)
				return false, nil
			}
			if !strings.Contains(croCR, "cluster") {
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("can not get cluster with output %v, the error is %v", croCR, err))
		e2e.Logf("Operator is installed successfully")
	})

	g.It("Author:asahay-Medium-27075-Testing the config changes. [Serial]", func() {

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterResourceOverride", "cluster").Execute()
		createCRClusterresourceoverride(oc)
		var err error
		var croCR string
		errCheck := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			croCR, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterResourceOverride", "cluster", "-n", "clusterresourceoverride-operator").Output()
			if err != nil {
				e2e.Logf("error  %v, please try next round", err)
				return false, nil
			}
			if !strings.Contains(croCR, "cluster") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("can not get cluster with output %v, the error is %v", croCR, err))
		e2e.Logf("Operator is installed successfully")

		g.By("Testing the changes\n")
		testCRClusterresourceoverride(oc)

	})

})
