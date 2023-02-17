package winc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-windows] Windows_Containers", func() {
	defer g.GinkgoRecover()

	var (
		oc               = exutil.NewCLIWithoutNamespace("default")
		mcoNamespace     = "openshift-machine-api"
		wmcoNamespace    = "openshift-windows-machine-config-operator"
		wmcoDeployment   = "windows-machine-config-operator"
		privateKey       = "../internal/config/keys/openshift-qe.pem"
		publicKey        = "../internal/config/keys/openshift-qe.pub"
		windowsWorkloads = "win-webserver"
		linuxWorkloads   = "linux-webserver"
		defaultWindowsMS = "windows"
		defaultNamespace = "winc-test"
		iaasPlatform     string
		zone             string
		svcs             = map[int]string{
			0: "windows_exporter",
			1: "kubelet",
			2: "hybrid-overlay-node",
			3: "kube-proxy",
			4: "containerd",
			5: "windows-instance-config-daemon",
		}
		folders = map[int]string{
			1: "c:\\k",
			2: "c:\\temp",
			3: "c:\\var\\log",
		}
	)

	// Struct used to define a service in the windows-services
	type Service struct {
		Name         string   `json:"name"`
		Path         string   `json:"path"`
		Bootstrap    bool     `json:"bootstrap"`
		Priority     int      `json:"priority"`
		Dependencies []string `json:"dependencies,omitempty"`
	}

	g.BeforeEach(func() {
		output, _ := oc.WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		iaasPlatform = strings.ToLower(output)
		zone, _ = oc.WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-n", mcoNamespace, "-l", "machine.openshift.io/os-id=Windows", "-o=jsonpath={.items[0].metadata.labels.machine\\.openshift\\.io\\/zone}").Output()
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-Critical-33612-Windows node basic check", func() {
		g.By("Check Windows worker nodes run the same kubelet version as other Linux worker nodes")
		linuxKubeletVersion, err := oc.WithoutNamespace().Run("get").Args("nodes", "-l=kubernetes.io/os=linux", "-o=jsonpath={.items[0].status.nodeInfo.kubeletVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		windowsKubeletVersion, err := oc.WithoutNamespace().Run("get").Args("nodes", "-l=kubernetes.io/os=windows", "-o=jsonpath={.items[0].status.nodeInfo.kubeletVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if windowsKubeletVersion[0:5] != linuxKubeletVersion[0:5] {
			e2e.Failf("Failed to check Windows %s and Linux %s kubelet version should be the same", windowsKubeletVersion, linuxKubeletVersion)
		}

		g.By("Check worker label is applied to Windows nodes")
		msg, err := oc.WithoutNamespace().Run("get").Args("nodes", "--no-headers", "-l=kubernetes.io/os=windows").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, node := range strings.Split(msg, "\n") {
			if !strings.Contains(node, "worker") {
				e2e.Failf("Failed to check worker label is applied to Windows node %s", node)
			}
		}

		g.By("Check version annotation is applied to Windows nodes")
		// Note: Case 33536 also is covered
		windowsHostName := getWindowsHostNames(oc)
		for _, host := range windowsHostName {
			retcode, err := checkVersionAnnotationReady(oc, host)
			o.Expect(err).NotTo(o.HaveOccurred())
			if !retcode {
				e2e.Failf("Failed to check version annotation is applied to Windows node %s", host)
			}
		}

		g.By("Check dockerfile prepare required binaries in operator image")
		checkFolders := []struct {
			folder   string
			expected string
		}{
			{
				folder:   "/payload",
				expected: "azure-cloud-node-manager.exe cni containerd generated hybrid-overlay-node.exe kube-node powershell windows-instance-config-daemon.exe windows_exporter.exe",
			},
			{
				folder:   "/payload/containerd",
				expected: "containerd-shim-runhcs-v1.exe containerd.exe containerd_conf.toml",
			},
			{
				folder:   "/payload/cni",
				expected: "host-local.exe win-bridge.exe win-overlay.exe",
			},
			{
				folder:   "/payload/kube-node",
				expected: "kube-proxy.exe kubelet.exe",
			},
			{
				folder:   "/payload/powershell",
				expected: "gcp-get-hostname.ps1 hns.psm1",
			},
			{
				folder:   "/payload/generated",
				expected: "network-conf.ps1",
			},
		}
		for _, checkFolder := range checkFolders {
			g.By("Check required files in" + checkFolder.folder)
			command := []string{"exec", "-n", wmcoNamespace, "deployment.apps/windows-machine-config-operator", "--", "ls", checkFolder.folder}
			msg, err := oc.WithoutNamespace().Run(command...).Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			actual := strings.ReplaceAll(msg, "\n", " ")
			if actual != checkFolder.expected {
				e2e.Failf("Failed to check required files in %s, expected: %s actual: %s", checkFolder.folder, checkFolder.expected, actual)
			}
		}

		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		winInternalIP := getWindowsInternalIPs(oc)
		for _, winhost := range winInternalIP {
			for _, svc := range svcs {
				g.By(fmt.Sprintf("Check %v service is running in worker %v", svc, winhost))
				msg, _ = runPSCommand(bastionHost, winhost, fmt.Sprintf("Get-Service %v", svc), privateKey, iaasPlatform)
				if !strings.Contains(msg, "Running") {
					e2e.Failf("Failed to check %v service is running in %v: %s", svc, winhost, msg)
				}
			}
		}
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-Critical-32615-Generate userData secret [Serial]", func() {
		g.By("Check secret windows-user-data generated and contain correct public key")
		output, err := exec.Command("bash", "-c", "cat "+publicKey+"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		publicKeyContent := strings.Split(string(output), " ")[1]
		msg, err := oc.WithoutNamespace().Run("get").Args("secret", "windows-user-data", "-n", mcoNamespace, "-o=jsonpath={.data.userData}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		decodedUserData, _ := base64.StdEncoding.DecodeString(msg)
		if !strings.Contains(string(decodedUserData), publicKeyContent) {
			e2e.Failf("Failed to check public key in windows-user-data secret %s", string(decodedUserData))
		}
		g.By("Check delete secret windows-user-data")
		// May fail other cases in parallel, so run it in serial
		_, err = oc.WithoutNamespace().Run("delete").Args("secret", "windows-user-data", "-n", mcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pollErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			msg, err := oc.WithoutNamespace().Run("get").Args("secret", "-n", mcoNamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(msg, "windows-user-data") {
				e2e.Logf("Secret windows-user-data does not exist yet and wait up to 1 minute ...")
				return false, nil
			}
			e2e.Logf("Secret windows-user-data exist now")
			msg, err = oc.WithoutNamespace().Run("get").Args("secret", "windows-user-data", "-o=jsonpath={.data.userData}", "-n", mcoNamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			decodedUserData, _ := base64.StdEncoding.DecodeString(msg)
			if !strings.Contains(string(decodedUserData), publicKeyContent) {
				e2e.Failf("Failed to check public key in windows-user-data secret %s", string(decodedUserData))
			}
			return true, nil
		})
		if pollErr != nil {
			e2e.Failf("Secret windows-user-data does not exist after waiting up to 1 minutes ...")
		}
		g.By("Check update secret windows-user-data")
		// May fail other cases in parallel, so run it in serial
		// Update userData to "aW52YWxpZAo=" which is base64 encoded "invalid"
		_, err = oc.WithoutNamespace().Run("patch").Args("secret", "windows-user-data", "-p", `{"data":{"userData":"aW52YWxpZAo="}}`, "-n", mcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pollErr = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			msg, err := oc.WithoutNamespace().Run("get").Args("secret", "windows-user-data", "-o=jsonpath={.data.userData}", "-n", mcoNamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			decodedUserData, _ := base64.StdEncoding.DecodeString(msg)
			if !strings.Contains(string(decodedUserData), publicKeyContent) {
				e2e.Logf("Secret windows-user-data is not updated yet and wait up to 1 minute ...")
				return false, nil
			}
			e2e.Logf("Secret windows-user-data is updated")
			return true, nil
		})
		if pollErr != nil {
			e2e.Failf("Secret windows-user-data is not updated after waiting up to 1 minutes ...")
		}
	})

	// author: sgao@redhat.com
	g.It("Author:sgao-Low-32554-wmco run in a pod with HostNetwork", func() {
		winInternalIP := getWindowsInternalIPs(oc)[0]
		curlDest := winInternalIP + ":22"
		command := []string{"exec", "-n", wmcoNamespace, "deployment.apps/windows-machine-config-operator", "--", "curl", curlDest}
		msg, _ := oc.WithoutNamespace().Run(command...).Args().Output()
		if !strings.Contains(msg, "SSH-2.0-OpenSSH") {
			e2e.Failf("Failed to check WMCO run in a pod with HostNetwork: %s", msg)
		}
	})

	// author: sgao@redhat.com refactored:v1
	g.It("Smokerun-Author:sgao-Critical-28632-Windows and Linux east west network during a long time", func() {
		// Note: Flexy alredy created workload in winc-test, here we check it still works after a long time
		g.By("Check communication: Windows pod <--> Linux pod")
		winPodNames, err := getWorkloadsNames(oc, windowsWorkloads, defaultNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		windPodIPs, err := getWorkloadsIP(oc, windowsWorkloads, defaultNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		linuxPodNames, err := getWorkloadsNames(oc, linuxWorkloads, defaultNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		linuxPodIPs, err := getWorkloadsIP(oc, linuxWorkloads, defaultNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		command := []string{"exec", "-n", defaultNamespace, winPodNames[0], "--", "curl", linuxPodIPs[0] + ":8080"}
		msg, err := oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux web server from Windows pod")
		}
		command = []string{"exec", "-n", defaultNamespace, linuxPodNames[0], "--", "curl", windPodIPs[0]}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Linux pod")
		}
	})

	// author: sgao@redhat.com refactored:v1
	g.It("Smokerun-Author:sgao-Critical-32273-Configure kube proxy and external networking check", func() {
		if iaasPlatform == "vsphere" {
			g.Skip("vSphere does not support Load balancer, skipping")
		}
		namespace := "winc-32273"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWindowsWorkload(oc, namespace, "windows_web_server.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, true)
		externalIP, err := getExternalIP(iaasPlatform, oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Load balancer takes about 3 minutes to work, set timeout as 5 minutes
		pollErr := wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
			msg, _ := exec.Command("bash", "-c", "curl "+externalIP).Output()
			if !strings.Contains(string(msg), "Windows Container Web Server") {
				e2e.Logf("Load balancer is not ready yet and waiting up to 5 minutes ...")
				return false, nil
			}
			e2e.Logf("Load balancer is ready")
			return true, nil
		})
		if pollErr != nil {
			e2e.Failf("Load balancer is not ready after waiting up to 5 minutes ...")
		}
	})

	// author: rrasouli@redhat.com
	g.It("Author:rrasouli-NonPreRelease-Longduration-Medium-42047-Cluster autoscaling with Windows nodes [Slow][Disruptive]", func() {
		namespace := "winc-42047"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)

		machinesetName := getWindowsMachineSetName(oc, "winc", iaasPlatform, zone)
		defer oc.WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, machinesetName, "-n", mcoNamespace).Output()

		g.By("Creating Windows machineset with 1")
		setMachineset(oc, iaasPlatform, machinesetName)
		waitForMachinesetReady(oc, machinesetName, 25, 1)

		g.By("Creating cluster and machine autoscaller")
		defer destroyWindowsAutoscaller(oc)
		createWindowsAutoscaller(oc, machinesetName, namespace)

		g.By("Creating Windows workloads")
		createWindowsWorkload(oc, namespace, "windows_web_server_scaler.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, true)

		if iaasPlatform == "gcp" || iaasPlatform == "vsphere" {
			g.By("Scalling up the Windows workload to 4")
			scaleDeployment(oc, windowsWorkloads, 4, namespace)

			// now we need to test check whether the machines auto scalled to 2
			g.By("Waiting for Windows nodes to auto scale to 2")
			waitForMachinesetReady(oc, machinesetName, 20, 2)
		} else {
			g.By("Scalling up the Windows workload to 2")
			scaleDeployment(oc, windowsWorkloads, 2, namespace)

			// now we need to test check whether the machines auto scalled to 2
			g.By("Waiting for Windows nodes to auto scale to 2")
			waitForMachinesetReady(oc, machinesetName, 20, 2)
		}
		g.By("Scalling down the Windows workload to 1")
		scaleDeployment(oc, windowsWorkloads, 1, namespace)
		waitForMachinesetReady(oc, machinesetName, 10, 1)
	})
	// author rrasouli@redhat.com

	g.It("Smokerun-Longduration-Author:rrasouli-NonPreRelease-High-37096-Schedule Windows workloads with cluster running multiple Windows OS variants [Slow][Disruptive]", func() {
		if iaasPlatform != "azure" {
			// Currently vSphere and GCP supports only Windows 2022, AWS support for Windows 2022
			// was dropped. Support for AWS will be added in the next release.
			g.Skip("Only Azure supports multiple operating systems, skipping")
		}
		// we assume 2 Windows Nodes created with the default server 2019 image, here we create new server
		namespace := "winc-37096"
		machinesetName := getWindowsMachineSetName(oc, "winsecond", iaasPlatform, zone)
		machinesetMultiOSFileName := iaasPlatform + "_windows_machineset.yaml"
		machinesetFileName, err := getMachinesetFileName(oc, iaasPlatform, winVersion, machinesetName, machinesetMultiOSFileName, "secondary")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, machinesetName, "-n", mcoNamespace).Output()
		defer os.Remove(machinesetFileName)
		createMachineset(oc, machinesetFileName)
		// here we provision 1 webservers with a runtime class ID, up to 20 minutes due to pull image on AWS
		waitForMachinesetReady(oc, machinesetName, 20, 1)
		// Here we fetch machine IP from machineset
		machineIP := fetchAddress(oc, "InternalIP", machinesetName)
		nodeName := getNodeNameFromIP(oc, machineIP[0], iaasPlatform)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer deleteProject(oc, namespace)
		createProject(oc, namespace)

		buildID, err := getWindowsBuildID(oc, nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		replacement := map[string]string{
			"<windows_container_image>": getConfigMapData(oc, "secondary_windows_container_image"),
			"<kernelID>":                buildID,
		}
		createWindowsWorkload(oc, namespace, "windows_webserver_secondary_os.yaml", replacement, true)
		e2e.Logf("-------- Windows workload scaled on node IP %v -------------", machineIP[0])
		e2e.Logf("-------- Scaling up workloads to 5 -------------")
		scaleDeployment(oc, windowsWorkloads, 5, namespace)
		// we get a list of all hostIPs all should be on the same host
		pods, err := getWorkloadsHostIP(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		// we check that all workloads hostIP are similar for all pods
		if !checkPodsHaveSimilarHostIP(oc, pods, machineIP[0]) {
			e2e.Failf("Windows workloads did not bootstrap on the same host...")
		} else {
			e2e.Logf("Windows workloads succesfully bootstrap on the same host %v", nodeName)
		}
	})

	// author rrasouli@redhat.com
	g.It("Author:rrasouli-NonPreRelease-Longduration-Critical-42496-byoh-Configure Windows instance with DNS [Slow] [Disruptive]", func() {
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		byohMachineSetName := getWindowsMachineSetName(oc, "byoh", iaasPlatform, zone)
		address := setBYOH(oc, iaasPlatform, "InternalDNS", byohMachineSetName)
		defer oc.WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, byohMachineSetName, "-n", mcoNamespace).Output()
		defer oc.WithoutNamespace().Run("delete").Args("configmap", "windows-instances", "-n", wmcoNamespace).Output()
		// removing the config map
		g.By("Delete the BYOH congigmap for node deconfiguration")
		oc.WithoutNamespace().Run("delete").Args("configmap", "windows-instances", "-n", wmcoNamespace).Output()
		// here we need to add 2 status change values since the log is indicating
		// log entry 'instance has been deconfigured' after removing services
		waitUntilWMCOStatusChanged(oc, "removing directories")
		waitUntilWMCOStatusChanged(oc, "instance has been deconfigured")
		// check services are not running
		g.By("Check services are not running after deleting the Windows Node")
		runningServices, err := getWinSVCs(bastionHost, address[0], privateKey, iaasPlatform)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcBool, svc := checkRunningServicesOnWindowsNode(svcs, runningServices)
		if svcBool {
			e2e.Failf("Service %v still running on Windows node after deconfiguration", svc)
		}
		g.By("Check folder do not exist after deleting the Windows Node")
		for _, folder := range folders {
			if checkFoldersDoNotExist(bastionHost, address[0], fmt.Sprintf("%v", folder), privateKey, iaasPlatform) {
				e2e.Failf("Folders still exists on a deleted node %v", fmt.Sprintf("%v", folder))
			}
		}
		// TODO check network removal test

	})

	// author rrasouli@redhat.com
	g.It("Author:rrasouli-NonPreRelease-Longduration-Critical-42516-byoh-Configure Windows instance with IP [Slow][Disruptive]", func() {
		namespace := "winc-42516"
		byohMachineSetName := getWindowsMachineSetName(oc, "byoh", iaasPlatform, zone)
		defer waitWindowsNodesReady(oc, 2, 15*time.Minute)
		defer oc.WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, byohMachineSetName, "-n", mcoNamespace).Output()
		defer oc.WithoutNamespace().Run("delete").Args("configmap", "windows-instances", "-n", wmcoNamespace).Output()
		defer deleteProject(oc, namespace)

		byohIP := setBYOH(oc, iaasPlatform, "InternalIP", byohMachineSetName)
		createProject(oc, namespace)
		createWindowsWorkload(oc, namespace, "windows_web_server_byoh.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, true)
		scaleDeployment(oc, windowsWorkloads, 5, namespace)
		msg, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)

		byohNode := getNodeNameFromIP(oc, byohIP[0], iaasPlatform)

		// change version annotation on node
		oc.WithoutNamespace().Run("annotate").Args("node", byohNode, "--overwrite", "windowsmachineconfig.openshift.io/version=invalidVersion").Output()
		waitVersionAnnotationReady(oc, byohNode, 30*time.Second, 600*time.Second)
		waitUntilWMCOStatusChanged(oc, "instance has been deconfigured")
		waitWindowsNodeReady(oc, byohNode, 15*time.Minute)

		// deleting the BYOH node
		oc.WithoutNamespace().Run("delete").Args("node", byohNode).Output()
		// wait the byoh node is back
		waitUntilWMCOStatusChanged(oc, "transferring files")
		waitWindowsNodeReady(oc, byohNode, 5*time.Minute)
	})

	// author rrasouli@redhat.com
	g.It("Smokerun-Author:rrasouli-NonPreRelease-High-39451-Access Windows workload through clusterIP [Slow][Disruptive]", func() {
		namespace := "winc-39451"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWindowsWorkload(oc, namespace, "windows_web_server.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, true)
		createLinuxWorkload(oc, namespace)
		g.By("Check access through clusterIP from Linux and Windows pods")
		windowsClusterIP, err := getServiceClusterIP(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		linuxClusterIP, err := getServiceClusterIP(oc, linuxWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		winPodArray, err := getWorkloadsNames(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		linuxPodArray, err := getWorkloadsNames(oc, linuxWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("windows cluster IP: " + windowsClusterIP)
		e2e.Logf("Linux cluster IP: " + linuxClusterIP)

		//we query the Linux ClusterIP by a windows pod
		command := []string{"exec", "-n", namespace, winPodArray[0], "--", "curl", linuxClusterIP + ":8080"}

		msg, err := oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux ClusterIP from a windows pod")
		}
		e2e.Logf("***** Now testing Windows node from a linux host : ****")
		command = []string{"exec", "-n", namespace, linuxPodArray[0], "--", "curl", windowsClusterIP}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows ClusterIP from a linux pod")
		}

		g.By("Scale up the Deployment Windows pod continue to be available to curl Linux web server")
		e2e.Logf("Scalling up the Deployment to 2")
		scaleDeployment(oc, windowsWorkloads, 2, namespace)

		o.Expect(err).NotTo(o.HaveOccurred())
		winPodArray, err = getWorkloadsNames(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		command = []string{"exec", "-n", namespace, linuxPodArray[0], "--", "curl", windowsClusterIP}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows ClusterIP from a Linux pod")
		}

		command = []string{"exec", "-n", namespace, winPodArray[1], "--", "curl", linuxClusterIP + ":8080"}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux ClusterIP from a windows pod")
		}

		g.By("Scale up the MachineSet")
		e2e.Logf("Scalling up the Windows node to 3")
		windowsMachineSetName := getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone)
		scaleWindowsMachineSet(oc, windowsMachineSetName, 15, 3, false)
		defer scaleWindowsMachineSet(oc, windowsMachineSetName, 10, 2, false)
		waitWindowsNodesReady(oc, 3, 1200*time.Second)
		// Testing the Windows server is reachable via Linux pod
		command = []string{"exec", "-n", namespace, linuxPodArray[0], "--", "curl", windowsClusterIP}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows ClusterIP from a Linux pod")
		}
		// Testing the Linux server is reachable with the second windows pod created
		command = []string{"exec", "-n", namespace, winPodArray[1], "--", "curl", linuxClusterIP + ":8080"}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux ClusterIP from a windows pod")
		}
		// Testing the Linux server is reachable with the first Windows pod created.
		command = []string{"exec", "-n", namespace, winPodArray[0], "--", "curl", linuxClusterIP + ":8080"}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux ClusterIP from a windows pod")
		}
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-Critical-31276-Configure CNI and internal networking check", func() {
		namespace := "winc-31276"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWindowsWorkload(oc, namespace, "windows_web_server.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, true)
		createLinuxWorkload(oc, namespace)
		// we scale the deployment to 5 windows pods
		scaleDeployment(oc, windowsWorkloads, 5, namespace)
		hostIPArray, err := getWorkloadsHostIP(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check communication: Windows pod <--> Linux pod")
		winPodNameArray, err := getWorkloadsNames(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		linuxPodNameArray, err := getWorkloadsNames(oc, linuxWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		winPodIPArray, err := getWorkloadsIP(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		linuxPodIPArray, err := getWorkloadsIP(oc, linuxWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		command := []string{"exec", "-n", namespace, linuxPodNameArray[0], "--", "curl", winPodIPArray[0]}
		msg, err := oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Linux pod")
		}
		linuxSVC := linuxPodIPArray[0] + ":8080"
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", linuxSVC}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux web server from Windows pod")
		}

		g.By("Check communication: Windows pod <--> Windows pod in the same node")
		if hostIPArray[0] != hostIPArray[1] {
			e2e.Failf("Failed to get Windows pod in the same node")
		}
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", winPodIPArray[0]}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod in the same node")
		}
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", winPodIPArray[1]}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod in the same node")
		}

		g.By("Check communication: Windows pod <--> Windows pod across different Windows nodes")
		lastHostIP := hostIPArray[len(hostIPArray)-1]
		if hostIPArray[0] == lastHostIP {
			e2e.Failf("Failed to get Windows pod across different Windows nodes")
		}
		lastIP := winPodIPArray[len(winPodIPArray)-1]
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", lastIP}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod across different Windows nodes")
		}
		lastPodName := winPodNameArray[len(winPodNameArray)-1]
		command = []string{"exec", "-n", namespace, lastPodName, "--", "curl", winPodIPArray[0]}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod across different Windows nodes")
		}
	})

	// author: sgao@redhat.com
	g.It("Author:sgao-Medium-33768-NodeWithoutOVNKubeNodePodRunning alert ignore Windows nodes", func() {
		g.By("Check NodeWithoutOVNKubeNodePodRunning alert ignore Windows nodes")
		// Retrieve the Prometheus' pod id
		prometheusPod, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", "openshift-monitoring", "-l=app.kubernetes.io/name=prometheus", "-o", "jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Run locally, in the Prometheus container the API query to /api/v1/rules
		// saving us from having to perform port-forwarding, which does not work
		// with intermediate bastion hosts.
		getAlertCMD, err := oc.WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", strings.Trim(prometheusPod, `'`), "--", "curl", "localhost:9090/api/v1/rules").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Search for required string in the rules output
		if !strings.Contains(string(getAlertCMD), "kube_node_labels{label_kubernetes_io_os=\\\"windows\\\"}") {
			e2e.Failf("Failed to check NodeWithoutOVNKubeNodePodRunning alert ignore Windows nodes")
		}
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-Critical-33779-Retrieving Windows node logs", func() {
		g.By("Check a cluster-admin can retrieve kubelet logs")
		msg, err := oc.WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=kubelet/kubelet.log").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		windowsHostNames := getWindowsHostNames(oc)
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve kubelet log on: " + winHostName)
			if !strings.Contains(string(msg), winHostName+" Log file created at:") {
				e2e.Failf("Failed to retrieve kubelet log on: " + winHostName)
			}
		}

		g.By("Check a cluster-admin can retrieve kube-proxy logs")
		msg, err = oc.WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=kube-proxy/kube-proxy.exe.WARNING").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve kube-proxy log on: " + winHostName)
			if !strings.Contains(string(msg), winHostName+" Log file created at:") {
				e2e.Failf("Failed to retrieve kube-proxy log on: " + winHostName)
			}
		}

		g.By("Check a cluster-admin can retrieve hybrid-overlay logs")
		msg, err = oc.WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=hybrid-overlay/hybrid-overlay.log").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve hybrid-overlay log on: " + winHostName)
			if !strings.Contains(string(msg), winHostName) {
				e2e.Failf("Failed to retrieve hybrid-overlay log on: " + winHostName)
			}
		}

		g.By("Check a cluster-admin can retrieve container runtime logs")
		msg, err = oc.WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=containerd/containerd.log").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Retrieve container runtime logs")
		if !strings.Contains(string(msg), "starting containerd") {
			e2e.Failf("Failed to retrieve container runtime logs")
		}

		g.By("Check a cluster-admin can retrieve wicd runtime logs")
		msg, err = oc.WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=wicd/windows-instance-config-daemon.exe.INFO").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve wicd runtime log on: " + winHostName)
			if !strings.Contains(string(msg), winHostName+" Log file created at:") {
				e2e.Failf("Failed to retrieve wicd log on: " + winHostName)
			}
		}
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-NonPreRelease-Critical-33783-Enable must gather on Windows node [Slow][Disruptive]", func() {
		g.By("Check must-gather on Windows node")
		// Note: Marked as [Disruptive] in case of /tmp folder full
		msg, err := oc.WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/tmp/must-gather-33783").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-33783").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		mustGather := string(msg)
		checkMessage := []string{
			"host_service_logs/windows/",
			"host_service_logs/windows/log_files/",
			"host_service_logs/windows/log_files/hybrid-overlay/",
			"host_service_logs/windows/log_files/hybrid-overlay/hybrid-overlay.log",
			"host_service_logs/windows/log_files/kube-proxy/",
			"host_service_logs/windows/log_files/kube-proxy/kube-proxy.exe.ERROR",
			"host_service_logs/windows/log_files/kube-proxy/kube-proxy.exe.INFO",
			"host_service_logs/windows/log_files/kube-proxy/kube-proxy.exe.WARNING",
			"host_service_logs/windows/log_files/kubelet/",
			"host_service_logs/windows/log_files/kubelet/kubelet.log",
			"host_service_logs/windows/log_files/containerd/containerd.log",
			"host_service_logs/windows/log_files/wicd/windows-instance-config-daemon.exe.ERROR",
			"host_service_logs/windows/log_files/wicd/windows-instance-config-daemon.exe.INFO",
			"host_service_logs/windows/log_files/wicd/windows-instance-config-daemon.exe.WARNING",
		}
		for _, v := range checkMessage {
			if !strings.Contains(mustGather, v) {
				e2e.Failf("Failed to check must-gather on Windows node: " + v)
			}
		}
	})

	// author: rrasouli@redhat.com
	g.It("Smokerun-Author:rrasouli-NonPreRelease-High-33794-Watch cloud private key secret [Slow][Disruptive]", func() {
		// vSphere contains a builtin private and public key with it's template, currently changing its private key is super challenging
		// it implies generating a new template with a different key.
		if iaasPlatform == "vsphere" {
			g.Skip("vSphere does not support key replacement, skipping")
		}

		g.By("Scale WMCO to 0")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)

		g.By("Deleting the private key and user data")
		defer oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		_, err := oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().Run("delete").Args("secret", "windows-user-data", "-n", mcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scale WMCO to 1")
		scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)

		g.By("Creating Windows machineset with 1")
		machinesetName := getWindowsMachineSetName(oc, "winc", iaasPlatform, zone)
		defer oc.WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, machinesetName, "-n", mcoNamespace).Output()
		setMachineset(oc, iaasPlatform, machinesetName)

		g.By("Check Windows machine should be in Provisioning phase and not reconciled without cloud-private-key and windows-user-data")
		pollErr := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
			events, _ := oc.WithoutNamespace().Run("get").Args("events", "-n", mcoNamespace).Output()
			status, err := oc.WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-ojsonpath={.items[?(@.metadata.labels.machine\\.openshift\\.io\\/cluster-api-machineset==\""+machinesetName+"\")].status.phase}", "-n", "openshift-machine-api").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(events, "Secret \"windows-user-data\" not found") && strings.EqualFold(status, "Provisioning") {
				return true, nil
			}
			return false, nil
		})
		if pollErr != nil {
			e2e.Failf("Failed to check Windows machine should be in Provisioning phase and not reconciled after waiting up to 5 minutes ...")
		}

		g.By("Create the private key so machine can be reconciled with a valid secret")
		_, err = oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		waitForMachinesetReady(oc, machinesetName, 25, 1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scale down the machinset that the number of the existing Windows machines will be 0")
		scaleWindowsMachineSet(oc, machinesetName, 5, 0, false)

		g.By("Delete the existing private key secret")
		_, err = oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new secret with a wrong key name.")
		defer oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		_, err = oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=wrong-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scale up the machinset that the number of the existing Windows machines will be 1")
		// since we don't need to wait until the machineset is in ready state there is no need for a long timeout until the machine is ready
		scaleWindowsMachineSet(oc, machinesetName, 2, 1, true)
		waitUntilWMCOStatusChanged(oc, "cloud-private-key missing")

		g.By("Replace private key during Windows machine configuration")
		_, err = oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForMachinesetReady(oc, machinesetName, 25, 1)
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-NonPreRelease-Medium-37472-Idempotent check of service running in Windows node [Slow][Disruptive]", func() {
		namespace := "winc-37472"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWindowsWorkload(oc, namespace, "windows_web_server.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, true)
		windowsHostName := getWindowsHostNames(oc)[0]
		oc.WithoutNamespace().Run("annotate").Args("node", windowsHostName, "windowsmachineconfig.openshift.io/version-").Output()

		g.By("Check after reconciled Windows node should be Ready")
		waitVersionAnnotationReady(oc, windowsHostName, 60*time.Second, 1200*time.Second)
		g.By("Check LB service works well")
		if iaasPlatform != "vsphere" {
			externalIP, err := getExternalIP(iaasPlatform, oc, windowsWorkloads, namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			// Load balancer takes about 3 minutes to work, set timeout as 5 minutes
			pollErr := wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
				msg, _ := exec.Command("bash", "-c", "curl "+externalIP).Output()
				if !strings.Contains(string(msg), "Windows Container Web Server") {
					e2e.Logf("Load balancer is not ready yet and waiting up to 5 minutes ...")
					return false, nil
				}
				e2e.Logf("Load balancer is ready")
				return true, nil
			})
			if pollErr != nil {
				e2e.Failf("Load balancer is not ready after waiting up to 5 minutes ...")
			}
		} else {
			e2e.Logf("Skipped step Check LB service works, not supported on vSphere")
		}
	})

	// author: sgao@redhat.com
	g.It("Author:sgao-NonPreRelease-Medium-39030-Re queue on Windows machine's edge cases [Slow][Disruptive]", func() {
		g.By("Scale down WMCO")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)

		g.By("Scale up the MachineSet")
		windowsMachineSetName := getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone)
		defer scaleWindowsMachineSet(oc, windowsMachineSetName, 10, 2, false)
		scaleWindowsMachineSet(oc, windowsMachineSetName, 10, 3, true)
		g.By("Scale up WMCO")
		scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		waitForMachinesetReady(oc, windowsMachineSetName, 15, 3)

		g.By("Check Windows machines created before WMCO starts are successfully reconciling and Windows nodes added")
		waitWindowsNodesReady(oc, 3, 1200*time.Second)
	})

	// author: rrasouli@redhat.com
	g.It("Author:rrasouli-Medium-37362-[wmco] wmco using correct golang version", func() {
		g.By("Fetch the correct golang version")
		// get the golang version
		getCMD := "oc version -ojson | jq '.serverVersion.goVersion'"
		goVersion, _ := exec.Command("bash", "-c", getCMD).Output()
		s := string(goVersion)
		tVersion := truncatedVersion(s)
		e2e.Logf("Golang version is: %s", s)
		e2e.Logf("Golang version truncated is: %s", tVersion)
		g.By("Compare fetched version with WMCO log version")
		msg, err := oc.WithoutNamespace().Run("logs").Args("deployment.apps/windows-machine-config-operator", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, tVersion) {
			e2e.Failf("Unmatching golang version")
		}

	})
	// author: rrasouli@redhat.com
	g.It("Smokerun-Author:rrasouli-High-38186-[wmco] Windows LB service [Slow]", func() {
		if iaasPlatform == "vsphere" {
			g.Skip("vSphere does not support Load balancer, skipping")
		}
		namespace := "winc-38186"
		// defer cancel to avoid leaving a zombie goroutine
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWindowsWorkload(oc, namespace, "windows_web_server.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, true)
		// fetching here the external IP
		externalIP, err := getExternalIP(iaasPlatform, oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Wait for the Windows server to come up
		time.Sleep(100 * time.Second)
		g.By("Test LB " + externalIP + " connectivity")
		// Execute checkConnectivity(externalIP, 5) in the background
		runInBackground(ctx, cancel, checkConnectivity, externalIP, 5)

		g.By("2 Windows node + N Windows pods, N >= 2 and Windows pods should be landed on different nodes, we scale to 5 Windows workloads")
		scaleDeployment(oc, windowsWorkloads, 6, namespace)

		// Context was cancelled due to an error
		if ctx.Err() != nil {
			e2e.Failf("Connectivity check failed")
		} else {
			cancel()
			e2e.Logf("Ending checkConnectivity")
		}
	})

	// author: sgao@redhat.com refactored:v1
	g.It("Smokerun-Author:sgao-Critical-25593-Prevent scheduling non Windows workloads on Windows nodes", func() {
		namespace := "winc-25593"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		g.By("Check Windows node have a taint 'os=Windows:NoSchedule'")
		msg, err := oc.WithoutNamespace().Run("get").Args("nodes", "-l=kubernetes.io/os=windows", "-o=jsonpath={.items[0].spec.taints[0].key}={.items[0].spec.taints[0].value}:{.items[0].spec.taints[0].effect}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if msg != "os=Windows:NoSchedule" {
			e2e.Failf("Failed to check Windows node have taint os=Windows:NoSchedule")
		}
		g.By("Check deployment without tolerations would not land on Windows nodes")
		createWindowsWorkload(oc, namespace, "windows_web_server_no_taint.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, "primary_windows_container_image")}, false)
		poolErr := wait.Poll(20*time.Second, 60*time.Second, func() (bool, error) {
			msg, err = oc.WithoutNamespace().Run("get").Args("pod", "-l=app=win-webserver", "-o=jsonpath={.items[].status.conditions[].message}", "-n", namespace).Output()
			if strings.Contains(msg, "had untolerated taint") {
				return true, nil
			}
			return false, nil
		})
		if poolErr != nil {
			e2e.Failf("Failed to check deployment without tolerations would not land on Windows nodes")
		}
		g.By("Check deployment with tolerations already covered in function createWindowsWorkload()")
		g.By("Check none of core/optional operators/operands would land on Windows nodes")
		for _, winHostName := range getWindowsHostNames(oc) {
			e2e.Logf("Check pods running on Windows node: " + winHostName)
			msg, err = oc.WithoutNamespace().Run("get").Args("pods", "--all-namespaces", "-o=jsonpath={.items[*].metadata.namespace}", "-l=app=win-webserver", "--field-selector", "spec.nodeName="+winHostName, "--no-headers").Output()
			for _, namespace := range strings.Split(msg, " ") {
				e2e.Logf("Found pods running in namespace: " + namespace)
				if namespace != "" && !strings.Contains(namespace, "winc") {
					e2e.Failf("Failed to check none of core/optional operators/operands would land on Windows nodes")
				}
			}
		}
	})

	// author: rrasouli@redhat.com refactored:v1
	g.It("Smokerun-Author:rrasouli-Medium-42204-Create Windows pod with a Projected Volume", func() {
		namespace := "winc-42204"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		username := "admin"
		password := getRandomString(8)

		// we write files with the content of username/password
		os.WriteFile("username-42204.txt", []byte(username), 0644)
		defer os.Remove("username-42204.txt")
		os.WriteFile("password-42204.txt", []byte(password), 0644)
		defer os.Remove("password-42204.txt")

		g.By("Create username and password secrets")
		_, err := oc.WithoutNamespace().Run("create").Args("secret", "generic", "user", "--from-file=username=username-42204.txt", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().Run("create").Args("secret", "generic", "pass", "--from-file=password=password-42204.txt", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create Windows Pod with Projected Volume")
		// TODO replace to nano server image as soon as it supported
		// change the powershell commands to pwsh.exe and in the windows_webserver_projected_volume change to pwsh.exe
		image := "mcr.microsoft.com/windows/servercore:ltsc2019"
		deployedImage := getConfigMapData(oc, "primary_windows_container_image")
		if strings.Contains(deployedImage, "ltsc2022") {
			image = "mcr.microsoft.com/windows/servercore:ltsc2022"
		}
		createWindowsWorkload(oc, namespace, "windows_webserver_projected_volume.yaml", map[string]string{"<windows_container_image>": image}, true)
		winpod, err := getWorkloadsNames(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check in Windows pod, the projected-volume directory contains your projected sources")
		command := []string{"exec", winpod[0], "-n", namespace, "--", "powershell.exe", " ls .\\projected-volume", ";", "ls C:\\var\\run\\secrets\\kubernetes.io\\serviceaccount"}
		msg, err := oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Projected volume output is:\n %v", msg)

		g.By("Check username and password exist on projected volume pod")
		command = []string{"exec", winpod[0], "-n", namespace, "--", "powershell.exe", "cat C:\\projected-volume\\username"}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Username output is:\n %v", msg)
		command = []string{"exec", winpod[0], "-n", namespace, "--", "powershell.exe", "cat C:\\projected-volume\\password"}
		msg, err = oc.WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Password output is:\n %v", msg)
	})

	// author: rrasouli@redhat.com refactored:v1
	g.It("Smokerun-Author:rrasouli-Critical-48873-Add description OpenShift managed to Openshift services", func() {
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		// use config map to fetch the actual Windows version
		machineset := getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone)
		address := fetchAddress(oc, "InternalIP", machineset)
		for _, machineIP := range address {
			svcDescription, err := getSVCsDescription(bastionHost, machineIP, privateKey, iaasPlatform)
			o.Expect(err).NotTo(o.HaveOccurred())

			for _, svc := range svcs {
				svcDesc := svcDescription[svc]
				e2e.Logf("Service is %v", svcDesc)

				if !strings.Contains(svcDesc, "OpenShift managed") {
					e2e.Failf("Description is missing on service %v", svc)
				}
			}
		}
	})

	g.It("Longduration-Smokerun-Author:rrasouli-NonPreRelease-Critical-39858-Windows servicemonitor and endpoints check [Slow][Serial][Disruptive]", func() {

		g.By("Get Endpoints and service monitor values")
		// need to fetch service monitor age
		serviceMonitorAge1, err := oc.WithoutNamespace().Run("get").Args("endpoints", "-n", wmcoNamespace, "-o=jsonpath={.items[].metadata.creationTimestamp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// here we fetch a list of endpoints
		endpointsIPsBefore := getEndpointsIPs(oc, wmcoNamespace)
		// restarting the WMCO deployment
		g.By("Restart WMCO pod by deleting")
		wmcoID, err := getWorkloadsNames(oc, wmcoDeployment, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		wmcoStartTime, err := oc.WithoutNamespace().Run("get").Args("endpoints", "-n", wmcoNamespace, "-o=jsonpath={.status.StartTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("WMCO start time before restart", wmcoStartTime)
		oc.WithoutNamespace().Run("delete").Args("pod", wmcoID[0], "-n", wmcoNamespace).Output()
		// checking that the WMCO has no errors and restarted properly
		poolErr := wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			return checkWorkloadCreated(oc, wmcoDeployment, wmcoNamespace, 1), nil
		})
		if poolErr != nil {
			e2e.Failf("Error restarting WMCO up to 3 minutes ...")
		}
		g.By("Test endpoints IPs survives a WMCO restart")
		waitForEndpointsReady(oc, wmcoNamespace, 5, len(strings.Split(endpointsIPsBefore, " ")))

		endpointsIPsAfter := getEndpointsIPs(oc, wmcoNamespace)
		endpointsIPsBeforeArray := strings.Split(endpointsIPsBefore, " ")
		sort.Strings(endpointsIPsBeforeArray)
		endpointsIPsAfterArray := strings.Split(endpointsIPsAfter, " ")
		sort.Strings(endpointsIPsAfterArray)
		if !reflect.DeepEqual(endpointsIPsBeforeArray, endpointsIPsAfterArray) {
			e2e.Failf("Endpoints list mismatch after WMCO restart %v, %v", endpointsIPsBeforeArray, endpointsIPsAfterArray)
		}
		g.By("Test service-monitor restarted")
		serviceMonitorAge2, err := oc.WithoutNamespace().Run("get").Args("endpoints", "-n", wmcoNamespace, "-o=jsonpath={.items[].metadata.creationTimestamp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		timeOriginal, err := time.Parse(time.RFC3339, serviceMonitorAge1)
		o.Expect(err).NotTo(o.HaveOccurred())
		timeLast, err := time.Parse(time.RFC3339, serviceMonitorAge2)
		o.Expect(err).NotTo(o.HaveOccurred())
		if timeOriginal.Unix() >= timeLast.Unix() {
			e2e.Failf("Service monitor %v did not restart, bigger than %v new service monitor age", serviceMonitorAge1, serviceMonitorAge2)
		}
		g.By("Scale down nodes")
		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 20, 2, false)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 5, 0, false)
		g.By("Test endpoints IP are deleted after scalling down")
		waitForEndpointsReady(oc, wmcoNamespace, 5, 0)
		endpointsIPsLast := getEndpointsIPs(oc, wmcoNamespace)
		if endpointsIPsLast != "" {
			e2e.Failf("Endpoints %v are still exists after scalling down Windows nodes", endpointsIPsLast)
		}
	})

	g.It("Smokerun-Author:jfrancoa-Critical-50924-Windows instances react to kubelet CA rotation [Disruptive]", func() {

		// Retrieve previous certificate which will get rotated.
		certToExpire, err := oc.WithoutNamespace().Run("get").Args("configmap", "kube-apiserver-to-kubelet-client-ca", "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.data.ca\\-bundle\\.crt}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Force the kubelet CA rotation")

		initialCertNotBefore, err := oc.WithoutNamespace().Run("get").Args("secrets", "kube-apiserver-to-kubelet-signer", "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.metadata.annotations.auth\\.openshift\\.io\\/certificate-not-before}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		initialCertNotBeforeParsed, err := time.Parse(time.RFC3339, strings.Trim(initialCertNotBefore, `'`))
		o.Expect(err).NotTo(o.HaveOccurred())

		// CA rotation
		err = oc.WithoutNamespace().Run("patch").Args("secret", "-p", `{"metadata": {"annotations": {"auth.openshift.io/certificate-not-after": null}}}`, "kube-apiserver-to-kubelet-signer", "-n", "openshift-kube-apiserver-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Confirm that the rotation has taken place by
		// comparing initial certificate-not-before with the certificate-not-before annotation
		// after forcing the rotation
		rotatedCertNotBefore, err := oc.WithoutNamespace().Run("get").Args("secrets", "kube-apiserver-to-kubelet-signer", "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.metadata.annotations.auth\\.openshift\\.io\\/certificate-not-before}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		rotatedCertNotBeforeParsed, err := time.Parse(time.RFC3339, strings.Trim(rotatedCertNotBefore, `'`))
		o.Expect(err).NotTo(o.HaveOccurred())
		if initialCertNotBeforeParsed.Equal(rotatedCertNotBeforeParsed) {
			e2e.Failf("Kubelet CA rotation did not happen. certificate-not-before: %v", rotatedCertNotBefore)
		}

		// Force the expired certificate deletion from kubelet's client CA
		// First we get the current content on kubelet's client CA
		cmOutput, err := oc.WithoutNamespace().Run("get").Args("configmap", "kube-apiserver-to-kubelet-client-ca", "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.data.ca\\-bundle\\.crt}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Delete the expired certificate (stored at the beggining of test) by using ReplaceAll
		formattedCertToExpire := strings.Trim(strings.TrimSpace(certToExpire), "'")
		cmWithoutExpired := strings.ReplaceAll(cmOutput, formattedCertToExpire, "")
		formattedcmWithoutExpired := strings.ReplaceAll(strings.Trim(strings.TrimSpace(cmWithoutExpired), "'"), "\n", "\\n")
		// Patch the data.ca-bundle.crt field with the new config map content
		// without the expired certificate
		_, err = oc.WithoutNamespace().Run("patch").Args("configmap", "kube-apiserver-to-kubelet-client-ca", "-n", "openshift-kube-apiserver-operator", "-p", `{"data":{"ca-bundle.crt": "`+formattedcmWithoutExpired+`"}}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Verify kubelet client CA is updated in Windows workers")
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		caBundlePath := folders[1] + "\\kubelet-ca.crt"
		winInternalIP := getWindowsInternalIPs(oc)
		for _, winhost := range winInternalIP {

			g.By(fmt.Sprintf("Verify kubelet client CA content is include in Windows worker %v ", winhost))

			poolErr := wait.Poll(15*time.Second, 10*time.Minute, func() (bool, error) {
				// Get kubelet client CA content from confimap
				kubeletCA, err := oc.WithoutNamespace().Run("get").Args("configmap", "kube-apiserver-to-kubelet-client-ca", "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.data.ca\\-bundle\\.crt}'").Output()
				if err != nil {
					e2e.Logf("error getting kubelet client CA from ConfigMap: %v", err)
					return false, nil
				}

				// Fetch CA bundle from Windows worker node
				bundleContent, err := runPSCommand(bastionHost, winhost, fmt.Sprintf("Get-Content -Raw -Path %s", caBundlePath), privateKey, iaasPlatform)
				if err != nil {
					e2e.Logf("failed fetching CA bundle from Windows node %v: %v", winhost, err)
					return false, nil
				}

				kubeletCAFormatted := strings.Trim(strings.TrimSpace(kubeletCA), "'")
				// Check that the kubelet client CA is included in bundleContent variable
				// We need to split bundleContent by \r\n as it contains both Stdout and Stderr
				// and we are just interested on the Stdout
				if strings.Contains(strings.Split(bundleContent, "\r\n-----BEGIN CERTIFICATE-----")[1], kubeletCAFormatted) {
					return true, nil
				}
				e2e.Logf("kubelet CA not found in Windows worker node bundle %v. Retrying...", winhost)
				return false, nil
			})
			if poolErr != nil {
				e2e.Failf("failed to verify that the kubelet client CA is included in Windows worker %v bundle", winhost)
			}

		}

		g.By("Ensure that none of the Windows Workers got restarted and that we have communication to the Windows workers")
		for ip, up := range getWindowsNodesUptime(oc, privateKey, iaasPlatform) {
			// If the timestamp from the moment when the certificate got rotated
			// is older than the worker's uptime timestamp it means that
			// a restart took place
			if rotatedCertNotBeforeParsed.Before(up) {
				e2e.Failf("windows worker %v got restarted after CA rotation", ip)
			}
		}

	})

	g.It("Smokerun-Author:rrasouli-Medium-54711- [WICD] wmco services are running from ConfigMap", func() {

		g.By("Check configmap services running on Windows workers")
		windowsServicesCM, err := popItemFromList(oc, "cm", "windows-services", wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		payload, err := oc.WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath={.data.services}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		var services []Service
		json.Unmarshal([]byte(payload), &services)
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		winInternalIP := getWindowsInternalIPs(oc)
		for _, winhost := range winInternalIP {
			for _, svc := range services {
				g.By(fmt.Sprintf("Check %v service is running in worker %v", svc.Name, winhost))
				msg, _ := runPSCommand(bastionHost, winhost, fmt.Sprintf("Get-Service %v", svc.Name), privateKey, iaasPlatform)
				o.Expect(msg).Should(o.ContainSubstring("Running"), "Failed to check %v service is running in %v: %s", svc.Name, winhost, msg)
			}
		}

	})

	g.It("Smokerun-Author:jfrancoa-Medium-50403-wmco creates and maintains Windows services ConfigMap [Disruptive]", func() {

		g.By("Check service configmap exists")
		wmcoLogVersion := getWMCOVersionFromLogs(oc)
		cmVersionFromLog := "windows-services-" + wmcoLogVersion
		windowsServicesCM, err := popItemFromList(oc, "configmap", "windows-services", wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		if cmVersionFromLog != windowsServicesCM {
			e2e.Failf("Configmap of windows services mismatch with Logs version")
		}

		g.By("Check windowsmachineconfig/desired-version annotation")
		for _, winHostName := range getWindowsHostNames(oc) {
			desiredVersion, err := oc.WithoutNamespace().Run("get").Args("nodes", winHostName, "-o=jsonpath='{.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/desired-version}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Trim(desiredVersion, `'`) != wmcoLogVersion {
				e2e.Failf("desired-version annotation missmatch, expected %v and got %v for host %v", wmcoLogVersion, desiredVersion, winHostName)
			}
		}

		g.By("Check that windows-instance-config-daemon serviceaccount exists")
		_, err = oc.WithoutNamespace().Run("get").Args("serviceaccount", "windows-instance-config-daemon", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete windows-services configmap and wait for its recreation")
		_, err = oc.WithoutNamespace().Run("delete").Args("configmap", windowsServicesCM, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForServicesCM(oc, windowsServicesCM)

		g.By("Attempt to modify the windows-services configmap")
		_, err = oc.WithoutNamespace().Run("patch").Args("configmap", windowsServicesCM, "-p", `{"data":{"services":"[]"}}`, "-n", wmcoNamespace).Output()
		if err == nil {
			e2e.Failf("It should not be possible to modify configmap %v", windowsServicesCM)
		}
		// Try to modify the inmutable field in the configmap should not have effect
		_, err = oc.WithoutNamespace().Run("patch").Args("configmap", windowsServicesCM, "-p", `{"inmutable":false}`, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmInmutable, err := oc.WithoutNamespace().Run("get").Args("configmap", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath='{.immutable}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Trim(cmInmutable, `'`) != "true" {
			e2e.Failf("Inmutable field inside %v configmap has been modified when it should not be possible", windowsServicesCM)
		}

		g.By("Stop WMCO, delete existing windows-services configmap and create new dummy ones. WMCO should delete all and leave only the valid one")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)
		_, err = oc.WithoutNamespace().Run("delete").Args("configmap", windowsServicesCM, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = createManifestFile(oc, "wicd_configmap.yaml", map[string]string{"<version>": "8.8.8-55657c8"})
		o.Expect(err).NotTo(o.HaveOccurred())
		err = createManifestFile(oc, "wicd_configmap.yaml", map[string]string{"<version>": "0.0.1-55657c8"})
		o.Expect(err).NotTo(o.HaveOccurred())
		// Create also one with the same id as the existing
		err = createManifestFile(oc, "wicd_configmap.yaml", map[string]string{"<version>": wmcoLogVersion})
		o.Expect(err).NotTo(o.HaveOccurred())
		scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		waitForServicesCM(oc, windowsServicesCM)

	})

	g.It("Longduration-Author:rrasouli-NonPreRelease-High-35707-Re-create Windows nodes not matching wmco version annotation [Slow][Serial][Disruptive]", func() {

		// go routine parameters
		var ctx context.Context
		var cancel context.CancelFunc
		if iaasPlatform != "vsphere" {
			externalIP, err := getExternalIP(iaasPlatform, oc, windowsWorkloads, defaultNamespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			ctx, cancel = context.WithCancel(context.Background())
			// defer cancel to avoid leaving a zombie goroutine
			defer cancel()
			runInBackground(ctx, cancel, checkConnectivity, externalIP, 60)
		}

		g.By("Scalling machines to 3")
		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 18, 2, false)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 18, 3, false)
		// Wait for the added node to be in Ready state, otherwise workloads won't get
		// scheduled into it.
		waitWindowsNodesReady(oc, 3, 300*time.Second)
		g.By("Scalling workloads to 9")
		defer scaleDeployment(oc, windowsWorkloads, 5, defaultNamespace)
		scaleDeployment(oc, windowsWorkloads, 9, defaultNamespace)
		// TODO fetch nodes age
		// getWindowsNodesUptime(oc, privateKey, iaasPlatform)
		g.By("Tampering 3 Window machines and service continue ")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		err := scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, node := range getWindowsHostNames(oc) {
			oc.WithoutNamespace().Run("annotate").Args("node", node, "--overwrite", "windowsmachineconfig.openshift.io/version=invalidVersion").Output()
			waitVersionAnnotationReady(oc, node, 30*time.Second, 600*time.Second)
		}
		// scaling WMCO back to 1 we can expect to have 3 new nodes instead of wrong version annotated nodes
		err = scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		msg, err := oc.WithoutNamespace().Run("get").Args("pods", "-owide", "-n", defaultNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		for ok := true; ok; ok = (getNumNodesWithAnnotation(oc, "invalidVersion") > 0) {
			waitForMachinesetReady(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 28, 3)
		}

		if iaasPlatform != "vsphere" {
			// Context was cancelled due to an error
			if ctx.Err() != nil {
				e2e.Failf("Connectivity check failed")
			} else {
				cancel() // Stop go routine
				e2e.Logf("Ending checkConnectivity")
			}
		}

	})

	g.It("Author:jfrancoa-Medium-56354-Stop dependent services before stopping a service in WICD [Disruptive]", func() {

		targetService := "kubelet"

		g.By("Check configmap services running on Windows workers")
		windowsServicesCM, err := popItemFromList(oc, "cm", "windows-services", wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		payload, err := oc.WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath={.data.services}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		var services []Service
		json.Unmarshal([]byte(payload), &services)

		g.By("Retrieve dependent services from windows-services configmap")

		// Using an annonymous function to obtain the dependent services
		// in a recursive way.
		var recursiveDependencies func([]Service, string) []string
		recursiveDependencies = func(svcs []Service, service string) []string {
			for _, svc := range services {
				for _, dep := range svc.Dependencies {
					if dep == service {
						return append(recursiveDependencies(services, svc.Name), service)
					}
				}
			}
			return []string{service}
		}

		deps := recursiveDependencies(services, targetService)

		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		winInternalIP := getWindowsInternalIPs(oc)
		winHostNames := getWindowsHostNames(oc)

		for idx, winhost := range winInternalIP {

			g.By(fmt.Sprintf("Modify %v service binPath and check dependents stopped timestamp in host with IP %v", targetService, winhost))
			cmd := fmt.Sprintf("Get-WmiObject win32_service | Where-Object { $_.Name -eq \\\"%v\\\" } | select -ExpandProperty PathName", targetService)
			msg, _ := runPSCommand(bastionHost, winhost, cmd, privateKey, iaasPlatform)
			listOut := strings.Split(msg, "\r\n")
			initialBinPath := strings.TrimSpace(listOut[len(listOut)-2])

			// Add --log-file-max-size 2000 as argument to kubelet service
			cmd = fmt.Sprintf("sc.exe config %v binPath=\\\"%v --log-file-max-size 2000\\\"", targetService, initialBinPath)
			msg, _ = runPSCommand(bastionHost, winhost, cmd, privateKey, iaasPlatform)
			o.Expect(msg).Should(o.ContainSubstring("SUCCESS"))

			// TODO: Remove once https://issues.redhat.com/browse/WINC-736 is finished
			// This is a workaround to force the WICD reconciliation
			winHostName := winHostNames[idx]
			forceWicdReconciliation(oc, winHostName)

			// Ensure that the binPath command was restored
			time.Sleep(60 * time.Second) // Give time to WICD to reconcile
			cmd = fmt.Sprintf("Get-WmiObject win32_service | Where-Object { $_.Name -eq \\\"%v\\\" } | select -ExpandProperty PathName", targetService)
			msg, _ = runPSCommand(bastionHost, winhost, cmd, privateKey, iaasPlatform)
			listOut = strings.Split(msg, "\r\n")
			afterReconciliationBinPath := strings.TrimSpace(listOut[len(listOut)-2])

			o.Expect(afterReconciliationBinPath).Should(o.Equal(initialBinPath))

			g.By(fmt.Sprintf("Verifying that dependant services got stopped first for node %v", winHostName))
			// kube-proxy stopped < hybrid-overlay-node stopped < kubelet stopped
			var previousTS time.Time = time.Time{}
			for i, svc := range deps {
				serviceTimeStamp := getServiceTimeStamp(oc, winhost, privateKey, iaasPlatform, "stopped", svc)
				if !previousTS.IsZero() {
					if serviceTimeStamp.Before(previousTS) {
						e2e.Failf("Service %v was stopped before service %v", svc, deps[i-1])
					}
				}
				previousTS = serviceTimeStamp
			}

		}
	})
	// author jfrancoa@redhat.com
	g.It("Smokerun-Author:jfrancoa-Medium-38188-Get Windows instance/core number and CPU arch", func() {

		winMetrics := []string{"cluster:node_instance_type_count:sum", "cluster:capacity_cpu_cores:sum"}

		mon, err := exutil.NewPrometheusMonitor(oc.AsAdmin())
		o.Expect(err).NotTo(o.HaveOccurred(),
			"Error creating new thanos monitor")

		for _, metricQuery := range winMetrics {

			g.By(fmt.Sprintf("Check that the metric %s is exposed to telemetry", metricQuery))

			expectedExposedMetric := fmt.Sprintf(`{__name__=\"%s\"}`, metricQuery)
			telemetryConfig, err := oc.WithoutNamespace().Run("get").Args("configmap", "-n", "openshift-monitoring", "telemetry-config", "-o=jsonpath={.data}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(telemetryConfig).To(o.ContainSubstring(expectedExposedMetric),
				"Metric %s, is not exposed to telemetry", metricQuery)

			g.By(fmt.Sprintf("Verify the metric %s displays the right value", metricQuery))

			queryResult, err := mon.SimpleQuery(metricQuery + "{label_node_openshift_io_os_id=\"Windows\"}")
			o.Expect(err).NotTo(o.HaveOccurred(),
				"Error querying metric: %s", metricQuery)

			jsonResult := gjson.Parse(queryResult)
			status := jsonResult.Get("status").String()
			o.Expect(status).Should(o.Equal("success"),
				"Query %s execution failed: %s", metricQuery, status)

			metricValue := gjson.Parse(queryResult).Get("data.result.0.value.1").String()

			valueFromCluster := getMetricsFromCluster(oc, metricQuery)

			e2e.Logf("Query %s value: %s", metricQuery, metricValue)
			o.Expect(metricValue).Should(o.Equal(valueFromCluster),
				"Prometheus metric %s does not match the value %s obtained from the cluster", metricValue, valueFromCluster)

		}

	})

	// author rrasouli@redhat.com
	g.It("Longduration-Author:rrasouli-NonPreRelease-High-39640-Replace private key during Windows machine configuration [Slow][Serial][Disruptive]", func() {
		// vSphere contains a builtin private and public key with it's template, currently changing its private key is super challenging
		// it implies generating a new template with a different key.
		if iaasPlatform == "vsphere" {
			g.Skip("vSphere does not support key replacement, skipping")
		}
		g.By("Scalling down the machineset to 1")
		// defer
		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 45, 2, false)
		defer waitWindowsNodesReady(oc, 2, 3000*time.Second)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 18, 1, false)

		g.By(" Scaling down WMCO to 0 ")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		err := scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(" Replacing the private key with a newly created key during machine scale up ")
		defer oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		_, err = oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scalling up the machineset to 2")
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 18, 2, true)

		defer os.Remove("mykey")
		defer os.Remove("mykey.pub")
		cmd := "ssh-keygen  -N '' -C 'test key' -f mykey"
		_, err = exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		_, err = oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem=mykey", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// scaling WMCO back to 1 we can expect to have 2 new nodes instead of wrong version annotated nodes
		g.By(" Waiting for nodes to be in a Ready status ")
		err = scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		// A delay waiting for machine upgrade to be completed
		waitUntilWMCOStatusChanged(oc, "\"unhealthy\":0")

	})

	// author rrasouli@redhat.com
	g.It("Longduration-Author:rrasouli-NonPreRelease-Medium-44099-Secure Windows workers username annotation [Disruptive]", func() {
		g.By(" Creating new BYOH node ")
		byohMachineSetName := getWindowsMachineSetName(oc, "byoh", iaasPlatform, zone)

		waitWindowsNodesReady(oc, 2, 3000*time.Second)
		defer waitForMachinesetReady(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 45, 2)
		defer oc.WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, byohMachineSetName, "-n", mcoNamespace).Output()
		defer oc.WithoutNamespace().Run("delete").Args("configmap", "windows-instances", "-n", wmcoNamespace).Output()
		byohMachine := setBYOH(oc, iaasPlatform, "InternalIP", byohMachineSetName)
		waitWindowsNodesReady(oc, 3, 1000*time.Second)
		defer os.Remove("mykey")
		defer os.Remove("mykey.pub")
		oldPubKey, err := oc.WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0], iaasPlatform), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/pub-key-hash}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oldUsernameHash, err := oc.WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0], iaasPlatform), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/username}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By(" Creating new SSL keys ")
		cmd := "ssh-keygen  -N '' -C 'test' -f mykey"
		_, err = exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(" Open public key file ")
		content, err := os.ReadFile("mykey.pub")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By(" Appending public key into BYOH node ssh administrators_authorized_keys ")
		cmd = fmt.Sprintf("Add-Content -Value \\\"%q\\\" -Path C:\\ProgramData\\ssh\\administrators_authorized_keys", content)
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		_, err = runPSCommand(bastionHost, byohMachine[0], cmd, privateKey, iaasPlatform)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(" Deleting the private key ")
		defer oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		_, err = oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(" Replacing the private key with the a new one previously created ")
		defer oc.WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		_, err = oc.WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem=mykey", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if iaasPlatform != "vsphere" {
			waitUntilWMCOStatusChanged(oc, "\"unhealthy\":0")
		}
		g.By(" Comparing username public keys hash changed ")
		newPubkey, err := oc.WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0], iaasPlatform), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/pub-key-hash}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newUsernameHash, err := oc.WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0], iaasPlatform), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/username}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(oldPubKey).ShouldNot(o.Equal(newPubkey), "Content of old pub key is similar as new pub key")
		o.Expect(oldUsernameHash).ShouldNot(o.Equal(newUsernameHash), "Old username hash is similiar as new username hash")
		myPrivateKey, err := filepath.Abs("./mykey")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(" Checking that services are running on the BYOH node with the new private key ")
		for _, svc := range svcs {
			g.By(fmt.Sprintf("Check %v service is running in worker %v", svc, byohMachine[0]))
			msg, err := runPSCommand(bastionHost, byohMachine[0], fmt.Sprintf("Get-Service %v", svc), myPrivateKey, iaasPlatform)
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(msg, "Running") {
				e2e.Failf("Failed to check %v service is running in %v: %s", svc, byohMachine[0], msg)
			}
		}

	})

	// author jfrancoa@redhat.com
	g.It("Longduration-Author:jfrancoa-NonPreRelease-Medium-37086-Install wmco in a namespace other than recommended [Disruptive]", func() {

		customNamespace := "winc-namespace-test"

		g.By("Scalling down the machineset to 0")
		defer waitWindowsNodesReady(oc, 2, 3000*time.Second)
		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 30, 2, false)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 20, 0, false)

		subsSource, err := oc.WithoutNamespace().Run("get").Args("subscription", "-n", wmcoNamespace, "-o=jsonpath={.items[?(@.spec.name=='"+wmcoDeployment+"')].spec.source}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("Uninstall WMCO from current namespace %v", wmcoNamespace))
		defer installWMCO(oc, wmcoNamespace, subsSource, privateKey)
		uninstallWMCO(oc, wmcoNamespace)

		g.By(fmt.Sprintf("Install WMCO in new namespace %v", customNamespace))
		defer uninstallWMCO(oc, customNamespace)
		installWMCO(oc, customNamespace, subsSource, privateKey)

		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 20, 0, false)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 30, 2, false)
		waitWindowsNodesReady(oc, 2, 3000*time.Second)

		g.By("Scalling workloads to 10")
		defer scaleDeployment(oc, windowsWorkloads, 5, defaultNamespace)
		scaleDeployment(oc, windowsWorkloads, 10, defaultNamespace)
	})

})
