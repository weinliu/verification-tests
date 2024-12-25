package winc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
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

	oc := exutil.NewCLIWithoutNamespace("default")

	// Struct used to define a service in the windows-services
	type Service struct {
		Name         string   `json:"name"`
		Path         string   `json:"path"`
		Bootstrap    bool     `json:"bootstrap"`
		Priority     int      `json:"priority"`
		Dependencies []string `json:"dependencies,omitempty"`
	}

	g.BeforeEach(func() {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		iaasPlatform = strings.ToLower(output)
		var err error
		privateKey, err = exutil.GetPrivateKey()
		o.Expect(err).NotTo(o.HaveOccurred())
		publicKey, err = exutil.GetPublicKey()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-Critical-33612-Windows node basic check", func() {
		g.By("Check Windows worker nodes run the same kubelet version as other Linux worker nodes")
		linuxKubeletVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l=kubernetes.io/os=linux", "-o=jsonpath={.items[0].status.nodeInfo.kubeletVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		windowsKubeletVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l=kubernetes.io/os=windows", "-o=jsonpath={.items[0].status.nodeInfo.kubeletVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if !matchKubeletVersion(oc, linuxKubeletVersion, windowsKubeletVersion) {
			e2e.Failf("failed to check Windows %s and Linux %s kubelet version should be the same", windowsKubeletVersion, linuxKubeletVersion)
		}

		g.By("Check worker label is applied to Windows nodes")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--no-headers", "-l=kubernetes.io/os=windows").Output()
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
				expected: "azure-cloud-node-manager.exe cni containerd csi-proxy ecr-credential-provider.exe generated hybrid-overlay-node.exe kube-node powershell windows-exporter windows-instance-config-daemon.exe",
			},
			{
				folder:   "/payload/windows-exporter",
				expected: "windows-exporter-webconfig.yaml windows_exporter.exe",
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
				expected: "kube-log-runner.exe kube-proxy.exe kubelet.exe",
			},
			{
				folder:   "/payload/powershell",
				expected: "gcp-get-hostname.ps1 hns.psm1 windows-defender-exclusion.ps1",
			},
			{
				folder:   "/payload/generated",
				expected: "network-conf.ps1",
			},
		}
		for _, checkFolder := range checkFolders {
			g.By("Check required files in" + checkFolder.folder)
			command := []string{"exec", "-n", wmcoNamespace, "deployment.apps/windows-machine-config-operator", "--", "ls", checkFolder.folder}
			msg, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
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
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "windows-user-data", "-n", mcoNamespace, "-o=jsonpath={.data.userData}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		decodedUserData, _ := base64.StdEncoding.DecodeString(msg)
		if !strings.Contains(string(decodedUserData), publicKeyContent) {
			e2e.Failf("Failed to check public key in windows-user-data secret %s", string(decodedUserData))
		}
		g.By("Check delete secret windows-user-data")
		// May fail other cases in parallel, so run it in serial
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "windows-user-data", "-n", mcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pollErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", mcoNamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(msg, "windows-user-data") {
				e2e.Logf("Secret windows-user-data does not exist yet and wait up to 1 minute ...")
				return false, nil
			}
			e2e.Logf("Secret windows-user-data exist now")
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "windows-user-data", "-o=jsonpath={.data.userData}", "-n", mcoNamespace).Output()
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
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "windows-user-data", "-p", `{"data":{"userData":"aW52YWxpZAo="}}`, "-n", mcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pollErr = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "windows-user-data", "-o=jsonpath={.data.userData}", "-n", mcoNamespace).Output()
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
	g.It("Author:sgao-Smokerun-Low-32554-wmco run in a pod with HostNetwork", func() {
		winInternalIP := getWindowsInternalIPs(oc)[0]
		curlDest := winInternalIP + ":22"
		command := []string{"exec", "-n", wmcoNamespace, "deployment.apps/windows-machine-config-operator", "--", "curl", "--http0.9", curlDest}
		msg, _ := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
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
		msg, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux web server from Windows pod")
		}
		command = []string{"exec", "-n", defaultNamespace, linuxPodNames[0], "--", "curl", windPodIPs[0]}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Linux pod")
		}
	})

	// author: sgao@redhat.com refactored:v1
	g.It("Smokerun-Author:sgao-Critical-32273-Configure kube proxy and external networking check", func() {
		if iaasPlatform == "vsphere" || iaasPlatform == "nutanix" || iaasPlatform == "none" {
			g.Skip(fmt.Sprintf("Platform %s does not support Load balancer, skipping", iaasPlatform))
		}
		namespace := "winc-32273"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWorkload(oc, namespace, windowsWebserverFile, map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, true, windowsWorkloads)
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
		if iaasPlatform == "none" {
			g.Skip("platform none does not support changing namespace and scaling up machines")
		}
		namespace := "winc-42047"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		zone := getAvailabilityZone(oc)
		machinesetName := getWindowsMachineSetName(oc, "winc", iaasPlatform, zone)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, machinesetName, "-n", mcoNamespace).Output()

		g.By("Creating Windows machineset with 1")
		setMachineset(oc, iaasPlatform, getConfigMapData(oc, wincTestCM, "primary_windows_image", defaultNamespace))
		waitForMachinesetReady(oc, machinesetName, 25, 1)

		g.By("Creating cluster and machine autoscaller")
		defer destroyWindowsAutoscaller(oc)
		createWindowsAutoscaller(oc, machinesetName)

		g.By("Creating Windows workloads")
		createWorkload(oc, namespace, "windows_web_server_scaler.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, true, windowsWorkloads)

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

	g.It("Longduration-Author:rrasouli-NonPreRelease-High-37096-Schedule Windows workloads with cluster running multiple Windows OS variants [Slow][Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip("platform none does not support scaling up machines")
		}
		if iaasPlatform != "azure" && iaasPlatform != "aws" {
			// Currently vSphere and GCP supports only Windows 2022
			g.Skip("Only Azure and AWS are supporting multiple operating systems, skipping")
		}
		// we assume 2 Windows Nodes created with the default server 2019 image, here we create new server
		namespace := "winc-37096"
		zone := getAvailabilityZone(oc)
		machinesetName := getWindowsMachineSetName(oc, "winsecond", iaasPlatform, zone)
		machinesetMultiOSFileName := iaasPlatform + "_windows_machineset.yaml"
		err := configureMachineset(oc, iaasPlatform, "winsecond", machinesetMultiOSFileName, getConfigMapData(oc, wincTestCM, "secondary_windows_image", defaultNamespace))
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, machinesetName, "-n", mcoNamespace).Output()

		// here we provision 1 webserver with a runtime class ID, up to 20 minutes due to pull image on AWS
		waitForMachinesetReady(oc, machinesetName, 20, 1)
		// Here we fetch machine IP from machineset
		machineIP := fetchAddress(oc, "InternalIP", machinesetName)
		nodeName := getNodeNameFromIP(oc, machineIP[0])

		defer deleteProject(oc, namespace)
		createProject(oc, namespace)

		buildID, err := getWindowsBuildID(oc, nodeName)
		o.Expect(err).NotTo(o.HaveOccurred())
		replacement := map[string]string{
			"<windows_container_image>": getConfigMapData(oc, wincTestCM, "secondary_windows_container_image", defaultNamespace),
			"<kernelID>":                buildID,
		}
		createWorkload(oc, namespace, "windows_webserver_secondary_os.yaml", replacement, true, windowsWorkloads)
		e2e.Logf("-------- Windows workload scaled on node IP %v -------------", machineIP[0])
		e2e.Logf("-------- Scaling up workloads to 5 -------------")
		scaleDeployment(oc, windowsWorkloads, 5, namespace)

		// we get a list of all hostIPs all should be on the same host
		pods, err := getWorkloadsHostIP(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		// we check that all workloads hostIP are similar for all pods
		// Iterate through the pods and compare their host IPs with the provided node IP
		similarHostIP := true
		for _, pod := range pods {
			e2e.Logf("Pod host IP is %v, of node IP, %v", pod, machineIP[0])
			if pod != machineIP[0] {
				similarHostIP = false
				break
			}
		}

		// Check if all pods have similar host IPs
		o.Expect(similarHostIP).To(o.BeTrue(), "Windows workloads did not bootstrap on the same host...")
		e2e.Logf("Windows workloads successfully bootstrapped on the same host %v", nodeName)
	})

	// author rrasouli@redhat.com
	g.It("Author:rrasouli-NonPreRelease-Longduration-Critical-42496-byoh-Configure Windows instance with DNS [Slow] [Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip("platform none does not support BYOH tests")
		}
		zone := getAvailabilityZone(oc)
		byohMachineSetName := getWindowsMachineSetName(oc, "byoh", iaasPlatform, zone)
		defer deleteResource(oc, "configmap", "windows-instances", wmcoNamespace, "--ignore-not-found")
		defer deleteResource(oc, exutil.MapiMachineset, byohMachineSetName, mcoNamespace)
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		address := setBYOH(oc, iaasPlatform, []string{"InternalDNS"}, byohMachineSetName, getConfigMapData(oc, wincTestCM, "primary_windows_image", defaultNamespace))

		// removing the config map
		g.By("Delete the BYOH configmap for node deconfiguration")
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "windows-instances", "-n", wmcoNamespace).Output()

		// log entry 'instance has been deconfigured' after removing services
		waitUntilWMCOStatusChanged(oc, "instance has been deconfigured")

		// check services are not running
		g.By("Check services are not running after deleting the Windows Node")
		runningServices, err := getWinSVCs(bastionHost, address[0], privateKey, iaasPlatform)
		o.Expect(err).NotTo(o.HaveOccurred())
		svcBool, svc := checkRunningServicesOnWindowsNode(svcs, runningServices)
		o.Expect(svcBool).To(o.BeFalse(), "Service %v still running on Windows node after deconfiguration", svc)

		// Check folders do not exist after deleting the Windows Node
		g.By("Check folders do not exist after deleting the Windows Node")
		for _, folder := range folders {
			o.Expect(checkFoldersDoNotExist(bastionHost, address[0], folder, privateKey, iaasPlatform)).To(o.BeTrue(), "Folder %v still exists on a deleted node", folder)
		}
	})

	// author rrasouli@redhat.com
	g.It("Author:rrasouli-NonPreRelease-Longduration-Critical-42484-byoh-Configure Windows instance with IP [Slow][Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip("platform none does not support BYOH tests")
		}
		namespace := "winc-42484"
		zone := getAvailabilityZone(oc)
		byohMachineSetName := getWindowsMachineSetName(oc, "byoh", iaasPlatform, zone)
		defer waitWindowsNodesReady(oc, 2, 15*time.Minute)
		defer deleteResource(oc, exutil.MapiMachineset, byohMachineSetName, mcoNamespace)
		defer deleteResource(oc, "configmap", "windows-instances", wmcoNamespace)
		defer deleteProject(oc, namespace)

		byohIP := setBYOH(oc, iaasPlatform, []string{"InternalIP"}, byohMachineSetName, getConfigMapData(oc, wincTestCM, "primary_windows_image", defaultNamespace))
		createProject(oc, namespace)
		createWorkload(oc, namespace, "windows_web_server_byoh.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, true, windowsWorkloads)
		scaleDeployment(oc, windowsWorkloads, 5, namespace)
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)

		byohNode := getNodeNameFromIP(oc, byohIP[0])

		// change version annotation on node
		oc.AsAdmin().WithoutNamespace().Run("annotate").Args("node", byohNode, "--overwrite", "windowsmachineconfig.openshift.io/version=invalidVersion").Output()
		waitVersionAnnotationReady(oc, byohNode, 30*time.Second, 600*time.Second)
		waitUntilWMCOStatusChanged(oc, "instance has been deconfigured")
		waitWindowsNodeReady(oc, byohNode, 15*time.Minute)

		// deleting the BYOH node
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("node", byohNode).Output()
		// wait the byoh node is back
		waitUntilWMCOStatusChanged(oc, "transferring files")
		waitWindowsNodeReady(oc, byohNode, 5*time.Minute)
	})

	g.It("Author:rrasouli-NonPreRelease-Longduration-Critical-42516-byoh-Configure a Windows instance with both IP and DNS [Slow][Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip("platform none does not support BYOH tests")
		}
		namespace := "winc-42516"
		zone := getAvailabilityZone(oc)
		byohMachineSetName := getWindowsMachineSetName(oc, "byoh", iaasPlatform, zone)
		defer waitWindowsNodesReady(oc, 2, 15*time.Minute)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, byohMachineSetName, "-n", mcoNamespace).Output()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "windows-instances", "-n", wmcoNamespace).Output()

		setBYOH(oc, iaasPlatform, []string{"InternalIP", "InternalDNS"}, byohMachineSetName, getConfigMapData(oc, wincTestCM, "primary_windows_image", defaultNamespace))
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWorkload(oc, namespace, "windows_web_server_byoh.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, true, windowsWorkloads)
		scaleDeployment(oc, windowsWorkloads, 5, namespace)
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
	})

	// author rrasouli@redhat.com
	g.It("Smokerun-Author:rrasouli-NonPreRelease-Longduration-High-39451-Access Windows workload through clusterIP [Slow][Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip("platform none does not support scaling up machineset tests")
		}
		namespace := "winc-39451"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWorkload(oc, namespace, windowsWebserverFile, map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, true, windowsWorkloads)
		var linuxWebserverImage string
		if isDisconnectedCluster(oc) {
			linuxWebserverImage = getConfigMapData(oc, wincTestCM, "linux_container_disconnected_image", defaultNamespace)
		} else {
			linuxWebserverImage = linuxNoTagsImage
		}
		createWorkload(oc, namespace, linuxWebserverFile, map[string]string{"<linux_webserver_image>": linuxWebserverImage}, true, linuxWorkloads)
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

		// we query the Linux ClusterIP by a windows pod
		command := []string{"exec", "-n", namespace, winPodArray[0], "--", "curl", linuxClusterIP + ":8080"}

		msg, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux ClusterIP from a windows pod")
		}
		e2e.Logf("***** Now testing Windows node from a linux host : ****")
		command = []string{"exec", "-n", namespace, linuxPodArray[0], "--", "curl", windowsClusterIP}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
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
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows ClusterIP from a Linux pod")
		}

		command = []string{"exec", "-n", namespace, winPodArray[1], "--", "curl", linuxClusterIP + ":8080"}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux ClusterIP from a windows pod")
		}

		g.By("Scale up the MachineSet")
		if iaasPlatform != "none" {
			e2e.Logf("Scalling up the Windows node to 3")
			zone := getAvailabilityZone(oc)
			windowsMachineSetName := getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone)
			defer scaleWindowsMachineSet(oc, windowsMachineSetName, 10, 2, false)
			scaleWindowsMachineSet(oc, windowsMachineSetName, 15, 3, false)
			waitWindowsNodesReady(oc, 3, 1200*time.Second)
			// Testing the Windows server is reachable via Linux pod
			command = []string{"exec", "-n", namespace, linuxPodArray[0], "--", "curl", windowsClusterIP}
			msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
			e2e.Logf("msg: %v", msg)
			o.Expect(strings.Contains(msg, "Windows Container Web Server")).To(o.BeTrue(), fmt.Sprintf("Failed to curl Windows ClusterIP from a Linux pod:  %v", err))
			// Testing the Linux server is reachable with the second windows pod created
			command = []string{"exec", "-n", namespace, winPodArray[1], "--", "curl", linuxClusterIP + ":8080"}
			msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
			e2e.Logf("msg: %v", msg)
			o.Expect(strings.Contains(msg, "Windows Container Web Server")).To(o.BeTrue(), fmt.Sprintf("Failed to curl Linux ClusterIP from Windows pod:    %v", err))
			// Testing the Linux server is reachable with the first Windows pod created.
			command = []string{"exec", "-n", namespace, winPodArray[0], "--", "curl", linuxClusterIP + ":8080"}
			msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
			e2e.Logf("msg: %v", msg)
			o.Expect(strings.Contains(msg, "Windows Container Web Server")).To(o.BeTrue(), fmt.Sprintf("Failed to curl Windows ClusterIP from Windows pod:  %v", err))
		}
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-Critical-31276-Configure CNI and internal networking check", func() {
		namespace := "winc-31276"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)

		// Create Windows workload
		createWorkload(oc, namespace, windowsWebserverFile,
			map[string]string{
				"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace),
			},
			true,
			windowsWorkloads,
		)

		// Get Linux container image for disconnected environment
		linuxWebserverImage := getConfigMapData(oc, wincTestCM, "linux_container_disconnected_image", defaultNamespace)

		// Create Linux workload based on environment type
		if isDisconnectedCluster(oc) {
			if linuxWebserverImage != "<linux_container_disconnected_image>" {
				// Use disconnected Linux image
				e2e.Logf("Using disconnected Linux webserver image: %v", linuxWebserverImage)
				createWorkload(oc, namespace, linuxWebserverFileDisconnected,
					map[string]string{
						"<linux_webserver_image>": linuxWebserverImage,
					},
					true,
					linuxWorkloads,
				)
			} else {
				e2e.Logf("Warning: No valid Linux image config found in disconnected environment")
				// Fallback to default Linux image
				createWorkload(oc, namespace, linuxWebserverFile,
					map[string]string{},
					true,
					linuxWorkloads,
				)
			}
		} else {
			// Use default Linux image for connected environment
			createWorkload(oc, namespace, linuxWebserverFile,
				map[string]string{},
				true,
				linuxWorkloads,
			)
		}

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
		msg, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Linux pod")
		}
		linuxSVC := linuxPodIPArray[0] + ":8080"
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", linuxSVC}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl Linux web server from Windows pod")
		}

		g.By("Check communication: Windows pod <--> Windows pod in the same node")
		if hostIPArray[0] != hostIPArray[1] {
			e2e.Failf("Failed to get Windows pod in the same node")
		}
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", winPodIPArray[0]}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod in the same node")
		}
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", winPodIPArray[1]}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod in the same node")
		}

		g.By("Test connectivity from Linux pod to a Windows pod by DNS")
		command = []string{"exec", "-n", namespace, linuxPodNameArray[0], "--", "curl", windowsServiceDNS}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl by DNS a Windows workload from a Linux pod")
		}

		g.By("Test connectivity from last Windows pod to a Windows by DNS")
		command = []string{"exec", "-n", namespace, winPodNameArray[len(winPodNameArray)-1], "--", "curl", windowsServiceDNS}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl by DNS a Windows workload from last Windows pod pod")
		}

		g.By("Test connectivity from Windows pod to a Linux pod by DNS")
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", linuxServiceDNS}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Linux Container Web Server") {
			e2e.Failf("Failed to curl by DNS a Linux web server from Windows pod")
		}

		g.By("Check communication: Windows pod <--> Windows pod across different Windows nodes")
		lastHostIP := hostIPArray[len(hostIPArray)-1]
		if hostIPArray[0] == lastHostIP {
			e2e.Failf("Failed to get Windows pod across different Windows nodes")
		}
		lastIP := winPodIPArray[len(winPodIPArray)-1]
		command = []string{"exec", "-n", namespace, winPodNameArray[0], "--", "curl", lastIP}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod across different Windows nodes")
		}

		lastPodName := winPodNameArray[len(winPodNameArray)-1]
		command = []string{"exec", "-n", namespace, lastPodName, "--", "curl", winPodIPArray[0]}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, "Windows Container Web Server") {
			e2e.Failf("Failed to curl Windows web server from Windows pod across different Windows nodes")
		}
	})

	// author: sgao@redhat.com
	g.It("Author:sgao-Smokerun-Medium-33768-NodeWithoutOVNKubeNodePodRunning alert ignore Windows nodes", func() {
		g.By("Check NodeWithoutOVNKubeNodePodRunning alert ignore Windows nodes")
		// Retrieve the Prometheus' pod id
		prometheusPod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-monitoring", "-l=app.kubernetes.io/name=prometheus", "-o", "jsonpath='{.items[0].metadata.name}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Run locally, in the Prometheus container the API query to /api/v1/rules
		// saving us from having to perform port-forwarding, which does not work
		// with intermediate bastion hosts.
		getAlertCMD, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", strings.Trim(prometheusPod, `'`), "--", "curl", "localhost:9090/api/v1/rules").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Search for required string in the rules output
		if !strings.Contains(string(getAlertCMD), "kube_node_labels{label_kubernetes_io_os=\\\"windows\\\"}") {
			e2e.Failf("Failed to check NodeWithoutOVNKubeNodePodRunning alert ignore Windows nodes")
		}
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-Critical-33779-Retrieving Windows node logs", func() {
		g.By("Check a cluster-admin can retrieve kubelet logs")
		windowsHostNames := getWindowsHostNames(oc)
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve kubelet log on: " + winHostName)
			msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", winHostName, "--path=kubelet/kubelet.log").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(string(msg), "Running kubelet as a Windows service!") {
				e2e.Failf("Failed to retrieve kubelet log on: " + winHostName)
			}
		}

		g.By("Check a cluster-admin can retrieve kube-proxy logs")
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve kube-proxy log on: " + winHostName)
			msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", winHostName, "--path=kube-proxy/kube-proxy.log").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(string(msg), "Running kube-proxy as a Windows service!") {
				e2e.Failf("Failed to retrieve kube-proxy log on: " + winHostName)
			}
		}

		g.By("Check a cluster-admin can retrieve hybrid-overlay logs")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=hybrid-overlay/hybrid-overlay.log").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve hybrid-overlay log on: " + winHostName)
			if !strings.Contains(string(msg), winHostName) {
				e2e.Failf("Failed to retrieve hybrid-overlay log on: " + winHostName)
			}
		}

		g.By("Check a cluster-admin can retrieve container runtime logs")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=containerd/containerd.log").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Retrieve container runtime logs")
		if !strings.Contains(string(msg), "starting containerd") {
			e2e.Failf("Failed to retrieve container runtime logs")
		}

		g.By("Check a cluster-admin can retrieve wicd runtime logs")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=wicd/windows-instance-config-daemon.exe.INFO").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve wicd runtime log on: " + winHostName)
			if !strings.Contains(string(msg), winHostName+" Log file created at:") {
				e2e.Failf("Failed to retrieve wicd log on: " + winHostName)
			}
		}

		g.By("Check a cluster-admin can retrieve csi-proxy logs")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "-l=kubernetes.io/os=windows", "--path=csi-proxy/csi-proxy.log").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, winHostName := range windowsHostNames {
			e2e.Logf("Retrieve csi-proxy runtime log on: " + winHostName)
			if !strings.Contains(string(msg), winHostName+" Log file created at:") {
				e2e.Failf("Failed to retrieve csi-proxy log on: " + winHostName)
			}
		}
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-NonPreRelease-Longduration-Critical-33783-Enable must gather on Windows node [Slow][Disruptive]", func() {
		g.By("Check must-gather on Windows node")
		// Note: Marked as [Disruptive] in case of /tmp folder full
		msg, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/tmp/must-gather-33783").Output()
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
			"host_service_logs/windows/log_files/kube-proxy/kube-proxy.log",
			"host_service_logs/windows/log_files/kubelet/",
			"host_service_logs/windows/log_files/kubelet/kubelet.log",
			"host_service_logs/windows/log_files/containerd/containerd.log",
			"host_service_logs/windows/log_files/wicd/windows-instance-config-daemon.exe.ERROR",
			"host_service_logs/windows/log_files/wicd/windows-instance-config-daemon.exe.INFO",
			"host_service_logs/windows/log_files/wicd/windows-instance-config-daemon.exe.WARNING",
			"host_service_logs/windows/log_files/csi-proxy/",
			"host_service_logs/windows/log_files/csi-proxy/csi-proxy.log",
		}
		for _, v := range checkMessage {
			if !strings.Contains(mustGather, v) {
				e2e.Failf("Failed to check must-gather on Windows node: " + v)
			}
		}
	})

	// author: rrasouli@redhat.com
	g.It("Smokerun-Author:rrasouli-NonPreRelease-Longduration-High-33794-Watch cloud private key secret [Slow][Disruptive]", func() {
		// vSphere contains a builtin private and public key with it's template, currently changing its private key is super challenging
		// it implies generating a new template with a different key.
		if iaasPlatform == "none" {
			g.Skip("platform none does not support changing namespace and scaling up machines")
		}
		if iaasPlatform == "vsphere" || iaasPlatform == "none" {
			g.Skip(fmt.Sprintf("%s does not support key replacement, skipping", iaasPlatform))
		}

		g.By("Scale WMCO to 0")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)

		g.By("Deleting the private key and user data")
		defer oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "windows-user-data", "-n", mcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scale WMCO to 1")
		scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)

		g.By("Creating Windows machineset with 1")
		zone := getAvailabilityZone(oc)
		machinesetName := getWindowsMachineSetName(oc, "winc", iaasPlatform, zone)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args(exutil.MapiMachineset, machinesetName, "-n", mcoNamespace).Output()
		setMachineset(oc, iaasPlatform, getConfigMapData(oc, wincTestCM, "primary_windows_image", defaultNamespace))

		g.By("Check Windows machine should be in Provisioning phase and not reconciled without cloud-private-key and windows-user-data")
		pollErr := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
			events, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", mcoNamespace).Output()
			status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(exutil.MapiMachine, "-ojsonpath={.items[?(@.metadata.labels.machine\\.openshift\\.io\\/cluster-api-machineset==\""+machinesetName+"\")].status.phase}", "-n", "openshift-machine-api").Output()
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
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		waitForMachinesetReady(oc, machinesetName, 25, 1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scale down the machinset that the number of the existing Windows machines will be 0")
		scaleWindowsMachineSet(oc, machinesetName, 5, 0, false)

		g.By("Delete the existing private key secret")
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a new secret with a wrong key name.")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=wrong-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scale up the machinset that the number of the existing Windows machines will be 1")
		// since we don't need to wait until the machineset is in ready state there is no need for a long timeout until the machine is ready
		scaleWindowsMachineSet(oc, machinesetName, 2, 1, true)
		waitUntilWMCOStatusChanged(oc, "cloud-private-key missing")

		g.By("Replace private key during Windows machine configuration")
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForMachinesetReady(oc, machinesetName, 25, 1)
	})

	// author: sgao@redhat.com
	g.It("Smokerun-Author:sgao-NonPreRelease-Longduration-Medium-37472-Idempotent check of service running in Windows node [Slow][Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip("platform none does not load balancer nor external IP tests")
		}
		namespace := "winc-37472"
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWorkload(oc, namespace, windowsWebserverFile, map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, true, windowsWorkloads)
		windowsHostName := getWindowsHostNames(oc)[0]
		oc.AsAdmin().WithoutNamespace().Run("annotate").Args("node", windowsHostName, "windowsmachineconfig.openshift.io/version-").Output()

		g.By("Check after reconciled Windows node should be Ready")
		waitVersionAnnotationReady(oc, windowsHostName, 60*time.Second, 1200*time.Second)
		g.By("Check LB service works well")
		if iaasPlatform != "vsphere" && iaasPlatform != "nutanix" {
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
			e2e.Logf(fmt.Sprintf("Platform %s does not support Load balancer, skipping", iaasPlatform))
		}
	})

	// author: sgao@redhat.com
	g.It("Author:sgao-NonPreRelease-Longduration-Smokerun-Medium-39030-Re queue on Windows machine's edge cases [Slow][Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip("platform none does not support scaling up Windows machines")
		}
		g.By("Scale down WMCO")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)

		g.By("Scale up the MachineSet")
		zone := getAvailabilityZone(oc)
		windowsMachineSetName := getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone)
		defer waitWindowsNodesReady(oc, 2, time.Second*1000)
		defer scaleWindowsMachineSet(oc, windowsMachineSetName, 10, 2, false)
		scaleWindowsMachineSet(oc, windowsMachineSetName, 10, 3, true)
		g.By("Scale up WMCO")
		scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		waitForMachinesetReady(oc, windowsMachineSetName, 15, 3)

		g.By("Check Windows machines created before WMCO starts are successfully reconciling and Windows nodes added")
		waitWindowsNodesReady(oc, 3, 1200*time.Second)
	})

	// author: rrasouli@redhat.com
	g.It("Smokerun-Author:rrasouli-Medium-37362-[wmco] wmco using correct golang version", func() {
		g.By("Fetch the correct golang version")
		// get the golang version
		getCMD := "oc version -ojson | jq '.serverVersion.goVersion'"
		goVersion, _ := exec.Command("bash", "-c", getCMD).Output()
		s := string(goVersion)
		tVersion := truncatedVersion(s)
		e2e.Logf("Golang version is: %s", s)
		e2e.Logf("Golang version truncated is: %s", tVersion)
		g.By("Compare fetched version with WMCO log version")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deployment.apps/windows-machine-config-operator", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(msg, tVersion) {
			e2e.Failf("Unmatching golang version")
		}
	})
	// author: rrasouli@redhat.com
	g.It("Smokerun-Author:rrasouli-High-38186-[wmco] Windows LB service [Slow]", func() {
		if iaasPlatform == "vsphere" || iaasPlatform == "nutanix" || iaasPlatform == "none" {
			g.Skip(fmt.Sprintf("Platform %s does not support Load balancer, skipping", iaasPlatform))
		}
		namespace := "winc-38186"
		// defer cancel to avoid leaving a zombie goroutine
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		defer deleteProject(oc, namespace)
		createProject(oc, namespace)
		createWorkload(oc, namespace, windowsWebserverFile, map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, true, windowsWorkloads)
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
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l=kubernetes.io/os=windows", "-o=jsonpath={.items[0].spec.taints[0].key}={.items[0].spec.taints[0].value}:{.items[0].spec.taints[0].effect}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if msg != "os=Windows:NoSchedule" {
			e2e.Failf("Failed to check Windows node have taint os=Windows:NoSchedule")
		}
		g.By("Check deployment without tolerations would not land on Windows nodes")
		createWorkload(oc, namespace, "windows_web_server_no_taint.yaml", map[string]string{"<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)}, false, windowsWorkloads)
		poolErr := wait.Poll(20*time.Second, 60*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l=app=win-webserver", "-o=jsonpath={.items[].status.conditions[].message}", "-n", namespace).Output()
			if strings.Contains(msg, "had untolerated taint") {
				return true, nil
			}
			return false, nil
		})
		if poolErr != nil {
			e2e.Failf("Failed to check deployment without tolerations would not land on Windows nodes")
		}
		g.By("Check deployment with tolerations already covered in function createWorkload()")
		g.By("Check none of core/optional operators/operands would land on Windows nodes")
		for _, winHostName := range getWindowsHostNames(oc) {
			e2e.Logf("Check pods running on Windows node: " + winHostName)
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--all-namespaces", "-o=jsonpath={.items[*].metadata.namespace}", "-l=app=win-webserver", "--field-selector", "spec.nodeName="+winHostName, "--no-headers").Output()
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
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "user", "--from-file=username=username-42204.txt", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "pass", "--from-file=password=password-42204.txt", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create Windows Pod with Projected Volume")
		// TODO replace to nano server image as soon as it supported
		// change the powershell commands to pwsh.exe and in the windows_webserver_projected_volume change to pwsh.exe
		image := "mcr.microsoft.com/windows/servercore:ltsc2019"
		deployedImage := getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace)
		if strings.Contains(deployedImage, "ltsc2022") {
			image = "mcr.microsoft.com/windows/servercore:ltsc2022"
		}
		createWorkload(oc, namespace, "windows_webserver_projected_volume.yaml", map[string]string{"<windows_container_image>": image}, true, windowsWorkloads)
		winpod, err := getWorkloadsNames(oc, windowsWorkloads, namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check in Windows pod, the projected-volume directory contains your projected sources")
		command := []string{"exec", winpod[0], "-n", namespace, "--", "powershell.exe", " ls .\\projected-volume", ";", "ls C:\\var\\run\\secrets\\kubernetes.io\\serviceaccount"}
		msg, err := oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Projected volume output is:\n %v", msg)

		g.By("Check username and password exist on projected volume pod")
		command = []string{"exec", winpod[0], "-n", namespace, "--", "powershell.exe", "cat C:\\projected-volume\\username"}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Username output is:\n %v", msg)
		command = []string{"exec", winpod[0], "-n", namespace, "--", "powershell.exe", "cat C:\\projected-volume\\password"}
		msg, err = oc.AsAdmin().WithoutNamespace().Run(command...).Args().Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Password output is:\n %v", msg)
	})

	// author: rrasouli@redhat.com refactored:v1
	g.It("Smokerun-Author:rrasouli-Critical-48873-Add description OpenShift managed to Openshift services", func() {
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		address := getWindowsInternalIPs(oc)
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
		if iaasPlatform == "none" {
			g.Skip("platform none is not supporting scaling up machineset tests")
		}
		g.By("Get Endpoints and service monitor values")
		// need to fetch service monitor age
		serviceMonitorAge1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("endpoints", "-n", wmcoNamespace, "-o=jsonpath={.items[].metadata.creationTimestamp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// here we fetch a list of endpoints
		endpointsIPsBefore, err := getEndpointsIPs(oc, wmcoNamespace)
		if err != nil {
			e2e.Failf("Error retrieving endpoint IPs: %v", err)
		}
		// restarting the WMCO deployment
		g.By("Restart WMCO pod by deleting")
		wmcoID, err := getWorkloadsNames(oc, wmcoDeployment, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		wmcoStartTime, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("endpoints", "-n", wmcoNamespace, "-o=jsonpath={.status.StartTime}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("WMCO start time before restart %v", wmcoStartTime)
		oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", wmcoID[0], "-n", wmcoNamespace).Output()
		// checking that the WMCO has no errors and restarted properly
		poolErr := wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			return checkWorkloadCreated(oc, wmcoDeployment, wmcoNamespace, 1), nil
		})
		if poolErr != nil {
			e2e.Failf("Error restarting WMCO up to 3 minutes ...")
		}
		g.By("Test endpoints IPs survives a WMCO restart")
		waitForEndpointsReady(oc, wmcoNamespace, 5, len(strings.Split(endpointsIPsBefore, " ")))

		endpointsIPsAfter, err := getEndpointsIPs(oc, wmcoNamespace)
		if err != nil {
			e2e.Failf("Error retrieving endpoint IPs: %v", err)
		}
		endpointsIPsBeforeArray := strings.Split(endpointsIPsBefore, " ")
		sort.Strings(endpointsIPsBeforeArray)
		endpointsIPsAfterArray := strings.Split(endpointsIPsAfter, " ")
		sort.Strings(endpointsIPsAfterArray)
		if !reflect.DeepEqual(endpointsIPsBeforeArray, endpointsIPsAfterArray) {
			e2e.Failf("Endpoints list mismatch after WMCO restart %v, %v", endpointsIPsBeforeArray, endpointsIPsAfterArray)
		}
		g.By("Test service-monitor restarted")
		serviceMonitorAge2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("endpoints", "-n", wmcoNamespace, "-o=jsonpath={.items[].metadata.creationTimestamp}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		timeOriginal, err := time.Parse(time.RFC3339, serviceMonitorAge1)
		o.Expect(err).NotTo(o.HaveOccurred())
		timeLast, err := time.Parse(time.RFC3339, serviceMonitorAge2)
		o.Expect(err).NotTo(o.HaveOccurred())
		if timeOriginal.Unix() >= timeLast.Unix() {
			e2e.Failf("Service monitor %v did not restart, bigger than %v new service monitor age", serviceMonitorAge1, serviceMonitorAge2)
		}
		g.By("Scale down nodes")
		defer waitWindowsNodesReady(oc, 2, time.Second*3000)
		zone := getAvailabilityZone(oc)
		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 20, 2, false)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 5, 0, false)
		g.By("Test endpoints IP are deleted after scalling down")
		waitForEndpointsReady(oc, wmcoNamespace, 5, 0)
		endpointsIPsLast, err := getEndpointsIPs(oc, wmcoNamespace)
		if err != nil {
			e2e.Failf("Error retrieving endpoint IPs: %v", err)
		}
		if endpointsIPsLast != "" {
			e2e.Failf("Endpoints %v are still exists after scalling down Windows nodes", endpointsIPsLast)
		}
	})

	g.It("Smokerun-Author:jfrancoa-Critical-50924-Windows instances react to kubelet CA rotation [Disruptive]", func() {
		const (
			namespace = "openshift-kube-apiserver-operator"
			configmap = "kube-apiserver-to-kubelet-client-ca"
		)

		// Get the expected number of Windows nodes
		winInternalIP := getWindowsInternalIPs(oc)
		expectedWindowsNodes := 2
		if len(winInternalIP) != expectedWindowsNodes {
			e2e.Failf("Expected Ready Windows nodes does not match %d", expectedWindowsNodes)
		}

		g.By("Ensure Windows nodes are Ready before proceeding")
		waitWindowsNodesReady(oc, expectedWindowsNodes, 10*time.Minute)

		initialCertNotBefore, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "kube-apiserver-to-kubelet-signer", "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.metadata.annotations.auth\\.openshift\\.io\\/certificate-not-before}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		initialCertNotBeforeParsed, err := time.Parse(time.RFC3339, strings.Trim(initialCertNotBefore, `'`))
		o.Expect(err).NotTo(o.HaveOccurred())

		// CA rotation
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret", "-p", `{"metadata": {"annotations": {"auth.openshift.io/certificate-not-after": null}}}`, "kube-apiserver-to-kubelet-signer", "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Wait for WMCO status to change
		waitUntilWMCOStatusChanged(oc, "updating kubelet CA client certificates in")

		// Confirm rotation
		rotatedCertNotBefore, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secrets", "kube-apiserver-to-kubelet-signer", "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.metadata.annotations.auth\\.openshift\\.io\\/certificate-not-before}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		rotatedCertNotBeforeParsed, err := time.Parse(time.RFC3339, strings.Trim(rotatedCertNotBefore, `'`))
		o.Expect(err).NotTo(o.HaveOccurred())
		if initialCertNotBeforeParsed.Equal(rotatedCertNotBeforeParsed) {
			e2e.Failf("Kubelet CA rotation did not happen. certificate-not-before: %v", rotatedCertNotBefore)
		}

		// Force the expired certificate deletion and update configmap
		// ... (existing code for updating configmap)

		g.By("Verify kubelet client CA is updated in Windows workers")
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		caBundlePath := folders[1] + "\\kubelet-ca.crt"

		for _, winhost := range winInternalIP {
			g.By(fmt.Sprintf("Verify kubelet client CA content is included in Windows worker %v ", winhost))

			poolErr := wait.Poll(30*time.Second, 15*time.Minute, func() (bool, error) {
				// Get kubelet client CA content from configmap
				kubeletCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", configmap, "-n", "openshift-kube-apiserver-operator", "-o=jsonpath='{.data.ca\\-bundle\\.crt}'").Output()
				if err != nil {
					e2e.Logf("Error getting kubelet client CA from ConfigMap: %v", err)
					return false, nil
				}
				if kubeletCA == "" {
					e2e.Logf("Kubelet client CA from ConfigMap is empty, retrying...")
					return false, nil
				}

				// Fetch CA bundle from Windows worker node
				bundleContent, err := runPSCommand(bastionHost, winhost, fmt.Sprintf("Get-Content -Raw -Path %s", caBundlePath), privateKey, iaasPlatform)
				if err != nil {
					e2e.Logf("Failed fetching CA bundle from Windows node %v: %v", winhost, err)
					return false, nil
				}
				if bundleContent == "" {
					e2e.Logf("CA bundle from Windows node %v is empty, retrying...", winhost)
					return false, nil
				}

				kubeletCAFormatted := strings.Trim(strings.TrimSpace(kubeletCA), "'")
				bundleContentSplit := strings.Split(bundleContent, "\r\n-----BEGIN CERTIFICATE-----")

				if len(bundleContentSplit) < 2 {
					e2e.Logf("Unexpected bundle content format from Windows node %v, retrying...", winhost)
					return false, nil
				}

				if strings.Contains(bundleContentSplit[1], kubeletCAFormatted) {
					e2e.Logf("Kubelet CA found in Windows worker node %v bundle", winhost)
					return true, nil
				}

				e2e.Logf("Kubelet CA not found in Windows worker node %v bundle. Retrying...", winhost)
				return false, nil
			})

			if poolErr != nil {
				e2e.Failf("Failed to verify that the kubelet client CA is included in Windows worker %v bundle: %v", winhost, poolErr)
			}
		}

		g.By("Ensure that none of the Windows Workers got restarted and that we have communication to the Windows workers")
		// for ip, up := range getWindowsNodesUptime(oc, privateKey, iaasPlatform) {
		// 	// If the timestamp from the moment when the certificate got rotated
		// 	// is older than the worker's uptime timestamp it means that
		// 	// a restart took place
		// 	if rotatedCertNotBeforeParsed.Before(up) {
		// 		e2e.Failf("windows worker %v got restarted after CA rotation", ip)
		// 	}
		// }
		uptimes, err := getWindowsNodesUptime(oc, privateKey, iaasPlatform)
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get Windows nodes uptime")

		for ip, up := range uptimes {
			if rotatedCertNotBeforeParsed.Before(up) {
				e2e.Failf("Windows worker %v got restarted after CA rotation", ip)
			}
		}
	})

	g.It("Smokerun-Author:rrasouli-Medium-54711- [WICD] wmco services are running from ConfigMap", func() {
		g.By("Check configmap services running on Windows workers")
		windowsServicesCM, err := popItemFromList(oc, "cm", wicdConfigMap, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		payload, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath={.data.services}").Output()
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
		windowsServicesCM, err := popItemFromList(oc, "configmap", wicdConfigMap, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		if cmVersionFromLog != windowsServicesCM {
			e2e.Failf("Configmap of windows services mismatch with Logs version")
		}

		g.By("Check windowsmachineconfig/desired-version annotation")
		for _, winHostName := range getWindowsHostNames(oc) {
			desiredVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", winHostName, "-o=jsonpath='{.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/desired-version}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Trim(desiredVersion, `'`) != wmcoLogVersion {
				e2e.Failf("desired-version annotation missmatch, expected %v and got %v for host %v", wmcoLogVersion, desiredVersion, winHostName)
			}
		}

		g.By("Check that windows-instance-config-daemon serviceaccount exists")
		_, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("serviceaccount", "windows-instance-config-daemon", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete windows-services configmap and wait for its recreation")
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", windowsServicesCM, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForCM(oc, windowsServicesCM, wicdConfigMap, wmcoNamespace)

		g.By("Attempt to modify the windows-services configmap")
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("configmap", windowsServicesCM, "-p", `{"data":{"services":"[]"}}`, "-n", wmcoNamespace).Output()
		if err == nil {
			e2e.Failf("It should not be possible to modify configmap %v", windowsServicesCM)
		}
		// Try to modify the inmutable field in the configmap should not have effect
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("configmap", windowsServicesCM, "-p", `{"inmutable":false}`, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		cmInmutable, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath='{.immutable}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Trim(cmInmutable, `'`) != "true" {
			e2e.Failf("Inmutable field inside %v configmap has been modified when it should not be possible", windowsServicesCM)
		}

		g.By("Stop WMCO, delete existing windows-services configmap and create new dummy ones. WMCO should delete all and leave only the valid one")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", windowsServicesCM, "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		manifestFile, err := exutil.GenerateManifestFile(oc, "winc", "wicd_configmap.yaml", map[string]string{"<version>": "8.8.8-55657c8"})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", manifestFile, "--ignore-not-found").Execute()
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "wicd_configmap.yaml", map[string]string{"<version>": "0.0.1-55657c8"})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", manifestFile, "--ignore-not-found").Execute()
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Create also one with the same id as the existing
		manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "wicd_configmap.yaml", map[string]string{"<version>": wmcoLogVersion})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", manifestFile, "--ignore-not-found").Execute()
		defer os.Remove(manifestFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// scaling WMCO pod back to 1
		scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		waitForCM(oc, windowsServicesCM, wicdConfigMap, wmcoNamespace)
	})

	g.It("Longduration-Author:rrasouli-NonPreRelease-High-35707-Re-create Windows nodes not matching wmco version annotation [Slow][Serial][Disruptive]", func() {
		if iaasPlatform == "none" {
			g.Skip(fmt.Sprintf("%s does not support LB nor machineset, skipping", iaasPlatform))
		}
		// go routine parameters
		var ctx context.Context
		var cancel context.CancelFunc
		if iaasPlatform != "vsphere" && iaasPlatform != "nutanix" {
			externalIP, err := getExternalIP(iaasPlatform, oc, windowsWorkloads, defaultNamespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			ctx, cancel = context.WithCancel(context.Background())
			// defer cancel to avoid leaving a zombie goroutine
			defer cancel()
			runInBackground(ctx, cancel, checkConnectivity, externalIP, 60)
		}

		g.By("Scalling machines to 3")
		defer waitWindowsNodesReady(oc, 2, time.Second*1000)
		zone := getAvailabilityZone(oc)
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
			oc.AsAdmin().WithoutNamespace().Run("annotate").Args("node", node, "--overwrite", "windowsmachineconfig.openshift.io/version=invalidVersion").Output()
			waitVersionAnnotationReady(oc, node, 30*time.Second, 600*time.Second)
		}
		// scaling WMCO back to 1 we can expect to have 3 new nodes instead of wrong version annotated nodes
		err = scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-owide", "-n", defaultNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(msg)
		for ok := true; ok; ok = (getNumNodesWithAnnotation(oc, "invalidVersion") > 0) {
			waitForMachinesetReady(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 28, 3)
		}

		if iaasPlatform != "vsphere" && iaasPlatform != "nutanix" {
			// Context was cancelled due to an error
			if ctx.Err() != nil {
				e2e.Failf("Connectivity check failed")
			} else {
				cancel() // Stop go routine
				e2e.Logf("Ending checkConnectivity")
			}
		}
	})

	g.It("Author:jfrancoa-Smokerun-Medium-56354-Stop dependent services before stopping a service in WICD [Disruptive]", func() {
		targetService := "containerd"

		g.By("Check configmap services running on Windows workers")

		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		winInternalIP := getWindowsInternalIPs(oc)
		pollIntervalSeconds := 10
		maxRetries := 60 // This allows for a 10 minute timeout

		for _, winhost := range winInternalIP {
			g.By(fmt.Sprintf("Modify %v service binPath and check that it gets restored in host with IP %v", targetService, winhost))

			initialBinPath := getServiceProperty(oc, winhost, privateKey, iaasPlatform, targetService, "PathName")

			// Add --service-name containerd as argument to containerd service
			cmd := fmt.Sprintf("sc.exe config %v binPath=\\\"%v --service-name containerd\\\"", targetService, initialBinPath)
			msg, _ := runPSCommand(bastionHost, winhost, cmd, privateKey, iaasPlatform)
			o.Expect(msg).Should(o.ContainSubstring("SUCCESS"))

			// Poll for the service binPath to be restored
			success := false
			for i := 0; i < maxRetries; i++ {
				afterReconciliationBinPath := getServiceProperty(oc, winhost, privateKey, iaasPlatform, targetService, "PathName")
				if afterReconciliationBinPath == initialBinPath {
					success = true
					break
				}
				time.Sleep(time.Duration(pollIntervalSeconds) * time.Second)
			}
			o.Expect(success).Should(o.BeTrue(), "Service binPath did not return to its initial state within the expected time frame")

			afterReconciliationBinPath := getServiceProperty(oc, winhost, privateKey, iaasPlatform, targetService, "PathName")
			o.Expect(afterReconciliationBinPath).Should(o.Equal(initialBinPath))
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
			telemetryConfig, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "openshift-monitoring", "telemetry-config", "-o=jsonpath={.data}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(telemetryConfig).To(o.ContainSubstring(expectedExposedMetric),
				"Metric %s, is not exposed to telemetry", metricQuery)

			g.By(fmt.Sprintf("Verify the metric %s displays the right value", metricQuery))

			queryResult, err := mon.SimpleQuery(metricQuery + "{label_node_openshift_io_os_id=\"Windows\"}")
			metricValue := extractMetricValue(queryResult)
			o.Expect(err).NotTo(o.HaveOccurred(),
				"Error querying metric: %s: %s", metricQuery, metricValue)

			metricValue = extractMetricValue(queryResult)

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
		if iaasPlatform == "vsphere" || iaasPlatform == "none" {
			g.Skip(fmt.Sprintf("%s does not support key replacement, skipping", iaasPlatform))
		}
		g.By("Scalling down the machineset to 1")
		// defer
		zone := getAvailabilityZone(oc)
		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 45, 2, false)
		defer waitWindowsNodesReady(oc, 2, 3000*time.Second)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 18, 1, false)

		g.By(" Scaling down WMCO to 0 ")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		err := scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(" Replacing the private key with a newly created key during machine scale up ")
		defer oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Scalling up the machineset to 2")
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 18, 2, true)

		defer os.Remove("mykey")
		defer os.Remove("mykey.pub")
		cmd := "ssh-keygen  -N '' -C 'test key' -f mykey"
		_, err = exec.Command("bash", "-c", cmd).CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem=mykey", "-n", wmcoNamespace).Output()
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
		if iaasPlatform == "none" {
			g.Skip("platform none does not support BYOH tests")
		}
		g.By(" Creating new BYOH node ")
		zone := getAvailabilityZone(oc)
		byohMachineSetName := getWindowsMachineSetName(oc, "byoh", iaasPlatform, zone)

		defer waitWindowsNodesReady(oc, 2, 3000*time.Second)
		defer waitForMachinesetReady(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 45, 2)
		defer deleteResource(oc, exutil.MapiMachineset, byohMachineSetName, mcoNamespace, "--ignore-not-found")
		defer deleteResource(oc, "configmap", "windows-instances", wmcoNamespace, "--ignore-not-found")
		byohMachine := setBYOH(oc, iaasPlatform, []string{"InternalIP"}, byohMachineSetName, getConfigMapData(oc, wincTestCM, "primary_windows_image", defaultNamespace))
		waitWindowsNodesReady(oc, 3, 1000*time.Second)
		defer os.Remove("mykey")
		defer os.Remove("mykey.pub")
		oldPubKey, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0]), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/pub-key-hash}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		oldUsernameHash, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0]), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/username}").Output()
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
		defer oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "cloud-private-key", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(" Replacing the private key with the a new one previously created ")
		defer deleteResource(oc, "secret", "cloud-private-key", wmcoNamespace, "--ignore-not-found")
		_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem=mykey", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if iaasPlatform != "vsphere" {
			waitUntilWMCOStatusChanged(oc, "\"unhealthy\":0")
		}
		g.By(" Comparing username public keys hash changed ")
		newPubkey, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0]), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/pub-key-hash}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newUsernameHash, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", getNodeNameFromIP(oc, byohMachine[0]), "-o=jsonpath={.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/username}").Output()
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
		waitUntilWMCOStatusChanged(oc, "instance has been configured as a worker node")
		// teardown the test to restore its original status
		deleteResource(oc, exutil.MapiMachineset, byohMachineSetName, mcoNamespace)
		deleteResource(oc, "secret", "cloud-private-key", wmcoNamespace)
		oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "cloud-private-key", "--from-file=private-key.pem="+privateKey, "-n", wmcoNamespace).Output()
		if iaasPlatform != "vsphere" {
			waitUntilWMCOStatusChanged(oc, "\"unhealthy\":0")
		}
		waitForMachinesetReady(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 40, 2)
	})

	// author jfrancoa@redhat.com
	g.It("Longduration-Author:jfrancoa-NonPreRelease-Medium-37086-Install wmco in a namespace other than recommended [Serial][Disruptive]", func() {
		//TODO remove this line as soon as OCPBUGS-23121 fixed
		g.Skip("This test skipped due OCPBUGS-23121 isn't fixed yet")
		if iaasPlatform == "none" {
			g.Skip("platform none does not support changing namespace and scaling up machines")
		}

		customNamespace := "winc-namespace-test"
		zone := getAvailabilityZone(oc)

		g.By("Scalling down the machineset to 0")
		defer waitWindowsNodesReady(oc, 2, 3000*time.Second)
		defer scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 30, 2, false)
		scaleWindowsMachineSet(oc, getWindowsMachineSetName(oc, defaultWindowsMS, iaasPlatform, zone), 20, 0, false)

		subsSource, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("subscription", "-n", wmcoNamespace, "-o=jsonpath={.items[?(@.spec.name=='"+wmcoDeployment+"')].spec.source}").Output()
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

	g.It("Smokerun-Author:rrasouli-Medium-60814-Check containerd version is properly reported", func() {
		// get the latest version hash from WMCO logs
		versionHash := strings.Split(getWMCOVersionFromLogs(oc), "-")[1]
		resp, err := http.Get("https://raw.githubusercontent.com/openshift/windows-machine-config-operator/" + versionHash + "/Makefile")
		if err != nil {
			e2e.Failf("unable to get http with error: %v", err)
		}
		body, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			e2e.Failf("unable to parse the http result with error: %v", err)
		}

		submoduleContainerdVersion := getValueFromText(body, "CONTAINERD_GIT_VERSION=")
		for _, winhost := range getWindowsHostNames(oc) {
			if strings.Compare(string(submoduleContainerdVersion), getContainerdVersion(oc, winhost)) != 0 {
				e2e.Failf("Containerd version mismatch expected %s actual %s", submoduleContainerdVersion, getContainerdVersion(oc, winhost))
			}
		}
	})

	g.It("Author:jfrancoa-Smokerun-Medium-60944-WICD controller periodically reconciles state of Windows services [Disruptive]", func() {
		targetService := "windows_exporter"
		winInternalIP := getWindowsInternalIPs(oc)
		pollIntervalSeconds := 10
		maxRetries := 60 // This allows for a 10 minute timeout

		for _, winhost := range winInternalIP {
			g.By(fmt.Sprintf("Stop service %v in host with IP %v", targetService, winhost))
			// In case something goes wrong and the service does not get reconciled, make sure to
			// restore back the service using defer
			defer setServiceState(oc, winhost, privateKey, iaasPlatform, "start", targetService)
			setServiceState(oc, winhost, privateKey, iaasPlatform, "stop", targetService)

			// Poll for the service state using getServiceProperty
			success := false
			for i := 0; i < maxRetries; i++ {
				state := getServiceProperty(oc, winhost, privateKey, iaasPlatform, targetService, "State")
				if strings.TrimSpace(state) == "Running" {
					success = true
					break
				}
				time.Sleep(time.Duration(pollIntervalSeconds) * time.Second)
			}
			o.Expect(success).Should(o.BeTrue(), "Service did not return to 'Running' state within the expected time frame")

			status := getServiceProperty(oc, winhost, privateKey, iaasPlatform, targetService, "State")
			o.Expect(status).Should(o.Equal("Running"))
		}
	})

	g.It("Smokerun-Author:rrasouli-Critical-65980-[node-proxy]-Cluster-wide proxy acceptance test [Serial][Disruptive]", func() {
		// checking whether cluster proxy is configured at all, otherwise skip the test
		if !isProxy(oc) {
			g.Skip("Cluster proxy not detected, skipping")
		}

		// here we are creating a new cluster proxy map that contains similar keys as in WICD
		clusterEnvVars := getEnvVarProxyMap(oc)
		initialProxySpec := getProxySpec(oc)
		// here we retrieve the proxy env vars from WICD CM
		windowsServicesCM, err := popItemFromList(oc, "cm", wicdConfigMap, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		wicdPayload, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath={.data.environmentVars}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		wicdProxies := getPayloadMap(wicdPayload)
		// comparing between 2 proxy settings, cluster and WICD
		if !compareMaps(clusterEnvVars, wicdProxies) {
			e2e.Failf("Cluster proxy settings are not equal to WICD proxy settings")
		}
		// here checking all proxy values exist on worker nodes
		g.By("Check if proxy exists on all Windows worker")
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		winInternalIP := getWindowsInternalIPs(oc)
		checkProxyVarsExistsOnWindowsNode(winInternalIP, wicdProxies, bastionHost, privateKey, iaasPlatform)
		// verify that trusted-ca ConfigMap exists in the cluster
		g.By("Ensure that trusted-ca exists")
		_, err = popItemFromList(oc, "cm", proxyCAConfigMap, wmcoNamespace)
		e2e.ExpectNoError(err, "Couldn't find trusted-ca configmap")
		defer restoreEnvironmentFiles(oc, initialProxySpec)

		// remove here trustedCA
		g.By("remove proxy trusted CA")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("proxy/cluster", "--type=json", "-p", `[{"op": "replace", "path": "/spec/trustedCA/name", "value": ""}]`).Execute()
		if err != nil {
			e2e.Failf("It should not be possible to modify ConfigMap %v, error print is %v", trustedCACM, err)
		}
		// remove noProxy vars and check invoked on each Windows nodes
		g.By("remove no_proxy vars and check invoked on each Windows nodes")
		timeNoProxy := getWMCOTimestamp(oc)

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("proxy/cluster", "--type=json", "-p", `[{"op": "remove", "path": "/spec/noProxy", "value": "noProxy"}]`).Execute()
		if err != nil {
			e2e.Failf("It should not be possible to modify ConfigMap %v, error print is %v", trustedCACM, err)
		}
		// rebooting instance references should appear
		checkWMCORestarted(oc, timeNoProxy)
		waitUntilWMCOStatusChanged(oc, "rebooting instance")
		time.Sleep(20 * time.Second)
		waitWindowsNodesReady(oc, 2, 6*time.Minute)

		g.By("remove https_proxy vars and check invoked on each Windows nodes")
		testNoHttpsClusterEnvVars := getEnvVarProxyMap(oc, map[string]string{"NO_PROXY": "status.noProxy", "HTTP_PROXY": "status.httpProxy"})
		timeNoHttps := getWMCOTimestamp(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("proxy/cluster", "--type=json", "-p", `[{"op": "remove", "path": "/spec/httpsProxy", "value": "httpsProxy"}]`).Execute()
		if err != nil {
			e2e.Failf("It should not be possible to modify ConfigMap %v, error print is %v", trustedCACM, err)
		}
		// rebooting instance references should appear
		time.Sleep(20 * time.Second)
		checkWMCORestarted(oc, timeNoHttps)
		waitUntilWMCOStatusChanged(oc, "rebooting instance")
		waitWindowsNodesReady(oc, 2, 6*time.Minute)
		// for stability purpose we need this sleep waiting WMCO to copy all proxy vars
		time.Sleep(60 * time.Second)
		wicdPayload, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath={.data.environmentVars}").Output()
		if wicdPayload == "" {
			e2e.Failf("WMCO did not copy proxy variables properly")
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		wicdProxies = getPayloadMap(wicdPayload)
		if !compareMaps(testNoHttpsClusterEnvVars, wicdProxies) {
			e2e.Failf("Cluster proxy settings are not equal to WICD proxy settings, https proxy was not removed from WICD CM")
		}
		checkProxyVarsExistsOnWindowsNode(winInternalIP, wicdProxies, bastionHost, privateKey, iaasPlatform)
		// remove http_proxy vars as well and check invoked on each Windows nodes
		g.By("remove https_proxy vars and check invoked on each Windows nodes")
		timeNoHttp := getWMCOTimestamp(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("proxy/cluster", "--type=json", "-p", `[{"op": "remove", "path": "/spec/httpProxy", "value": "httpProxy"}]`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// rebooting instance references should appear
		time.Sleep(20 * time.Second)
		checkWMCORestarted(oc, timeNoHttp)
		waitUntilWMCOStatusChanged(oc, "rebooting instance")
		waitWindowsNodesReady(oc, 2, 6*time.Minute)
		wicdPayload, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath={.data.environmentVars}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		wicdProxies = getPayloadMap(wicdPayload)
		checkProxyVarsExistsOnWindowsNode(winInternalIP, wicdProxies, bastionHost, privateKey, iaasPlatform)
	})

	g.It("Smokerun-Author:rrasouli-Critical-66670-[node-proxy]-Cluster-wide proxy trusted-ca configmap tests [Serial][Disruptive]", func() {
		// verify with a boolean function isProxyEnabled - skip if not
		if !isProxy(oc) {
			g.Skip("Cluster proxy not detected, skipping")
		}
		// here we are creating a new cluster proxy map that contains similar keys as in WICD
		initialProxySpec := getProxySpec(oc)
		g.By("Check whether the configured cluster proxy exists in windows-services (WICD) CM")

		g.By("add another record to cluster proxy, example.com to no-proxy")
		// before patching the cluster we determine WMCO start time

		wmcoStartTime := getWMCOTimestamp(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("proxy/cluster", "--type=json", "-p",
			"[{\"op\": \"add\", \"path\":\"/spec/noProxy\", \"value\":\""+noProxy+",example.com\"}]").Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "could not patch proxy with new noProxy value")
		// wait here for WMCO to restart and copy the env vars
		defer restoreEnvironmentFiles(oc, initialProxySpec)
		checkWMCORestarted(oc, wmcoStartTime)
		time.Sleep(60 * time.Second)

		g.By("verify that newly added noProxy record exist on WICD Windows Services")
		windowsServicesCM, err := popItemFromList(oc, "cm", wicdConfigMap, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		json, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-ojsonpath={.data.environmentVars}", "-n", wmcoNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		noProxyWICD := gjson.Get(json, "NO_PROXY")
		if !strings.Contains(fmt.Sprint(noProxyWICD), "example.com") {
			o.Expect(err).NotTo(o.HaveOccurred(), "WICD proxy string does not contains example.com")
		}
		g.By("verify that each of newly added record exist on each of Windows workers")
		o.Expect(err).NotTo(o.HaveOccurred())
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		winInternalIP := getWindowsInternalIPs(oc)
		for _, winhost := range winInternalIP {
			e2e.Logf(fmt.Sprintf("Check Added no-proxy value exist on worker %v", winhost))
			// Use PowerShell to get the NO_PROXY environment variable value
			msg, _ := runPSCommand(bastionHost, winhost, "get-childitem -Path env: |  Where-Object -Property Name -eq NO_PROXY | Format-List Value", privateKey, iaasPlatform)
			// Check if "example.com" is present in the NO_PROXY value
			if !strings.Contains(msg, "example.com") {
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to find example.com value on winworker %v", winhost))
			}
		}

		g.By("Verify that the trusted-ca cm exists on the WMCO namespace")
		trustedCA, err := popItemFromList(oc, "cm", trustedCACM, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		if trustedCA != trustedCACM {
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("trusted CA %v CM does not exist on %v namespace", trustedCACM, wmcoNamespace))
		}

		g.By("Validate the content of trusted-ca copied to each Windows workers")
		caBundlePath := folders[2] + "\\ca-bundle.crt"
		e2e.Logf("Path is %v", caBundlePath)
		trustedCA, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", trustedCACM, "-n", wmcoNamespace, "-o=jsonpath='{.data.ca\\-bundle\\.crt}'").Output()
		if err != nil {
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("error getting trusted CA from ConfigMap: %v", err))
		}
		for _, winhost := range winInternalIP {
			e2e.Logf(fmt.Sprintf("Verify trusted CA content is included in Windows worker %v ", winhost))
			// Fetch CA bundle from Windows worker node
			bundleContent, err := runPSCommand(bastionHost, winhost, fmt.Sprintf("Get-Content -Raw -Path %s", caBundlePath), privateKey, iaasPlatform)
			if err != nil {
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("failed fetching CA bundle from Windows node %v: %v", winhost, err))
			}
			CAFormatted := strings.Trim(strings.TrimSpace(trustedCA), "'")
			if strings.Contains(bundleContent, CAFormatted) {
				o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Trusted CA not found in Windows worker node bundle %v", winhost))
			}
		}

		g.By("Check that trusted-ca configmap cannot get deleted")
		deleteResource(oc, "cm", trustedCACM, wmcoNamespace)
		waitForCM(oc, trustedCACM, trustedCACM, wmcoNamespace)

		g.By("Check that trusted-ca configmap cannot get tampered")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("configmap", trustedCACM, "--type=json", "-p", `[{"op": "remove", "path": "/metadata/labels"}]`, "-n", wmcoNamespace).Execute()
		if err != nil {
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("It should not be possible to modify ConfigMap %v, error print is %v", trustedCACM, err))
		}
		waitForCM(oc, trustedCACM, trustedCACM, wmcoNamespace)
	})

	g.It("Smokerun-Author:rrasouli-Critical-68320-[node-proxy]-Import custom CA certificates into Windows node system store [Serial][Disruptive]", func() {
		// verify with a boolean function isProxyEnabled - skip if not
		if !isProxy(oc) {
			g.Skip("Cluster proxy not detected, skipping")
		}

		const (
			name                        = "OCP-68320-custom"
			validity                    = "3650"
			caSubj                      = "/OU=openshift/CN=test-custom-self-cert-signer"
			userSelfSignedCommonName    = "CN=test-custom-self-cert-signer, OU=openshift"
			userInstalledCertCommonName = "CN=Installer-QE-CA, OU=Installer-QE, O=OCP, S=Beijing, C=CN"
			namespace                   = "openshift-config"
			configmap                   = "user-ca-bundle"
		)
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		g.By("Verify that user certificate installed on each Windows worker")
		// Verify that user certificate is installed on each Windows worker before the change
		checkUserCertificatesOnWindowsWorkers(oc, bastionHost, userInstalledCertCommonName, privateKey, 1, iaasPlatform)

		g.By("Create a self-signed certificate and paste it to the content of the cm -n openshift-config user-ca-bundle")
		keyPath := fmt.Sprintf("%s-ca.key", name)
		crtPath := fmt.Sprintf("%s-ca.crt", name)
		defer os.Remove(keyPath)
		cmd := fmt.Sprintf("openssl genrsa -out %s-ca.key 4096", name)
		output, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		if err != nil {
			e2e.Failf("Failed to execute command: %s. Output:\n%s", cmd, output)
		}
		defer os.Remove(crtPath)
		cmd = fmt.Sprintf("openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %s -out %s-ca.crt -subj %s", name, validity, name, caSubj)
		output, err = exec.Command("bash", "-c", cmd).CombinedOutput()
		if err != nil {
			e2e.Failf("Failed to execute command: %s. Output:\n%s", cmd, output)
		}
		e2e.Logf("Storing existing ConfigMap content")
		initalConfigMapContent := getConfigMapData(oc, configmap, "ca\\-bundle\\.crt", namespace)
		initalConfigMapContent = removeOuterQuotes(initalConfigMapContent)
		defer func() {
			configureCertificateToJSONPatch(oc, initalConfigMapContent, configmap, namespace)
		}()
		e2e.Logf("Appending the new certificate to the existing content")
		newCertificateContent, err := readCertificateContent(fmt.Sprintf("%s-ca.crt", name))
		o.Expect(err).NotTo(o.HaveOccurred())
		// Combine the ConfigMap content and new certificate content
		combinedContent := fmt.Sprintf("%s\n%s", initalConfigMapContent, newCertificateContent)
		configureCertificateToJSONPatch(oc, combinedContent, configmap, namespace)

		g.By("Verify that user certificate installed on each Windows worker")
		checkUserCertificatesOnWindowsWorkers(oc, bastionHost, userInstalledCertCommonName, privateKey, 1, iaasPlatform)

		g.By("Creating certificate rotation")
		rotationValidity := "1"

		cmd = fmt.Sprintf("openssl req -x509 -new -nodes -key %s-ca.key -sha256 -days %s -out %s-ca.crt -subj %s", name, rotationValidity, name, caSubj)
		output, err = exec.Command("bash", "-c", cmd).CombinedOutput()
		if err != nil {
			e2e.Failf("Failed to execute command: %s. Output:\n%s", cmd, output)
		}
		e2e.Logf("Storing existing ConfigMap content")
		newCertificateContent, err = readCertificateContent(fmt.Sprintf("%s-ca.crt", name))
		if err != nil {
			e2e.Failf("Failed to read certificate")
		}
		// newCertificateContent = removeOuterQuotes(newCertificateContent)
		combinedContent = fmt.Sprintf("%s\n%s", initalConfigMapContent, newCertificateContent)
		configureCertificateToJSONPatch(oc, combinedContent, configmap, namespace)

		g.By("Verify that after certificate rotation certificates installed on each Windows worker")
		checkUserCertificatesOnWindowsWorkers(oc, bastionHost, userInstalledCertCommonName, privateKey, 1, iaasPlatform)

		g.By("Verify that added self signed certificate has been removed from each Windows node")

		configureCertificateToJSONPatch(oc, initalConfigMapContent, configmap, namespace)

		checkUserCertificatesOnWindowsWorkers(oc, bastionHost, userSelfSignedCommonName, privateKey, 0, iaasPlatform)
	})

	// author rrasouli@redhat.com
	g.It("Author:rrasouli-NonPreRelease-Longduration-Critical-43832-[upgrade]-Seamless upgrade with BYOH Windows instances [Serial][Disruptive]", func() {
		upgrade_index_to := getConfigMapData(oc, wincTestCM, "wmco_upgrade_index_image", defaultNamespace)
		trimmedValue := removeOuterQuotes(strings.TrimSpace(upgrade_index_to))
		if trimmedValue == "" {
			g.Skip("Upgrade index image hasn't been configured")
		}

		// Get current version from the deployment
		currentVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
			"deployment",
			wmcoDeployment,
			"-n", wmcoNamespace,
			"-o=jsonpath={.spec.template.spec.containers[0].image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Get Windows nodes and ensure they're ready
		windowsHosts := getWindowsHostNames(oc)
		o.Expect(len(windowsHosts)).To(o.BeNumerically(">", 0), "No Windows nodes found")

		// Wait for Windows nodes to be ready
		waitWindowsNodesReady(oc, len(windowsHosts), 5*time.Minute)

		// Get bastion host for SSH connections
		bastionHost := getSSHBastionHost(oc, iaasPlatform)

		g.By("Verifying initial AWS route configuration")
		for _, windowsHost := range windowsHosts {
			e2e.Logf("Checking routes for node: %s", windowsHost)
			err := verifyAWSRoutePersistence(bastionHost, windowsHost, privateKey, iaasPlatform)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to verify initial route persistence for node %s", windowsHost))
		}
		g.By("Uninstalling existing WMCO")
		uninstallWMCO(oc, wmcoNamespace, true)

		g.By("Removing existing catalog source")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args(
			"catalogsource",
			"wmco",
			"-n", "openshift-marketplace").Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to delete existing catalog source")

		// Wait for the catalog source to be fully removed
		err = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
				"catalogsource",
				"wmco",
				"-n", "openshift-marketplace").Output()
			if err != nil {
				return true, nil
			}
			return false, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred(), "Catalog source failed to be removed")

		g.By("Creating new catalog source with upgrade index")
		replacement := map[string]string{
			"<index_image>": upgrade_index_to,
		}
		manifestFile, err := exutil.GenerateManifestFile(oc, "winc", "catalogsource.yaml", replacement)
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to generate catalog source manifest")
		defer os.Remove(manifestFile)

		_, err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", manifestFile).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to apply catalog source")

		// Wait for catalog source to be ready
		g.By("Waiting for catalog source to be ready")
		poolErr := wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
				"catalogsources.operators.coreos.com",
				"wmco",
				"-o=jsonpath={.status.connectionState.lastObservedState}",
				"-n",
				"openshift-marketplace").Output()
			if err != nil {
				e2e.Logf("Command failed with error: %s .Retrying...", err)
				return false, nil
			}
			if msg == "READY" {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(poolErr, "WMCO deployment did not start up after waiting up to 5 minutes ...")

		g.By("Installing new WMCO")
		installWMCO(oc, wmcoNamespace, "wmco", privateKey)

		// Wait for operator upgrade
		g.By("Waiting for operator upgrade to complete")
		err = wait.Poll(30*time.Second, 15*time.Minute, func() (bool, error) {
			newVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
				"deployment",
				wmcoDeployment,
				"-n", wmcoNamespace,
				"-o=jsonpath={.spec.template.spec.containers[0].image}").Output()
			if err != nil {
				return false, nil
			}
			e2e.Logf("Current version: %s, New version: %s", currentVersion, newVersion)
			return currentVersion != newVersion, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred(), "Operator failed to upgrade within timeout")

		numberOfWindowsNodes := len(windowsHosts)
		e2e.Logf("Number of Windows nodes to upgrade %v", numberOfWindowsNodes)
		for _, node := range windowsHosts {
			e2e.Logf("Waiting for node to be configured as worker node %s", node)
			waitUntilWMCOStatusChanged(oc, "instance has been configured as a worker node")
		}
		waitWindowsNodesReady(oc, numberOfWindowsNodes, 20*time.Minute)

		exutil.By("Check service configmap got created with a names with the new version")
		wmcoLogVersion := getWMCOVersionFromLogs(oc)
		cmVersionFromLog := "windows-services-" + wmcoLogVersion
		windowsServicesCM, err := popItemFromList(oc, "configmap", wicdConfigMap, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		if cmVersionFromLog != windowsServicesCM {
			e2e.Failf("Configmap of windows services mismatch with Logs version")
		}

		exutil.By("Check windowsmachineconfig/desired-version annotation")
		for _, winHostName := range getWindowsHostNames(oc) {
			desiredVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
				"nodes",
				winHostName,
				"-o=jsonpath='{.metadata.annotations.windowsmachineconfig\\.openshift\\.io\\/desired-version}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Trim(desiredVersion, `'`) != wmcoLogVersion {
				e2e.Failf("desired-version annotation mismatch, expected %v and got %v for host %v",
					wmcoLogVersion, desiredVersion, winHostName)
			}
		}

		g.By("Verifying AWS route persistence after upgrade")
		for _, windowsHost := range windowsHosts {
			err := verifyAWSRoutePersistence(bastionHost, windowsHost, privateKey, iaasPlatform)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to verify route persistence after upgrade for node %s", windowsHost))
		}

		exutil.By("Set wmco_upgrade_index_image to empty")
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(
			"configmap",
			wincTestCM,
			"-n",
			defaultNamespace,
			"-p",
			`{"data":{"wmco_upgrade_index_image": ""}}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to reset configmap winc-test-config")
	})

	g.It("Smokerun-Author:weinliu-Medium-70922-Monitor CPU, Memory, and Filesystem graphs for Windows Pods managed by wmco", func() {
		// Define the metrics queries for CPU, Memory, and Filesystem
		cpuMetricQuery := "pod:container_cpu_usage:sum"
		memoryMetricQuery := "pod:container_memory_usage_bytes:sum"
		filesystemMetricQuery := "pod:container_fs_usage_bytes:sum"

		// Create Prometheus monitor instance
		mon, err := exutil.NewPrometheusMonitor(oc.AsAdmin())
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating new Prometheus monitor")

		// Define the namespace where the Windows workloads are deployed
		namespace := wmcoNamespace

		// Check if the WMCO deployment is ready
		ready := checkWorkloadCreated(oc, wmcoDeployment, namespace, 1)
		if !ready {
			e2e.Failf("WMCO deployment %s is not ready", wmcoDeployment)
		}

		g.By("Checking CPU, Memory, and Filesystem graphs for Windows pods managed by WMCO")
		// Get the pods managed by WMCO
		podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app="+wmcoDeployment, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error getting pods managed by WMCO")

		// Split the pod list
		podNames := strings.Fields(podList)

		for _, podName := range podNames {
			// Check CPU metric
			g.By(fmt.Sprintf("Verify CPU metric availability for pod: %s", podName))
			cpuQueryResult, err := mon.SimpleQuery(cpuMetricQuery + "{pod=\"" + podName + "\"}")
			cpuMetricValue := extractMetricValue(cpuQueryResult)
			o.Expect(err).NotTo(o.HaveOccurred(), "Error querying CPU metric for pod %s: %s", podName, cpuMetricValue)

			// Check Memory metric
			g.By(fmt.Sprintf("Verify Memory metric availability for pod: %s", podName))
			memoryQueryResult, err := mon.SimpleQuery(memoryMetricQuery + "{pod=\"" + podName + "\"}")
			memoryMetricValue := extractMetricValue(memoryQueryResult)
			o.Expect(err).NotTo(o.HaveOccurred(), "Error querying Memory metric for pod %s: %s", podName, memoryMetricValue)

			// Check Filesystem metric
			g.By(fmt.Sprintf("Verify Filesystem metric availability for pod: %s", podName))
			filesystemQueryResult, err := mon.SimpleQuery(filesystemMetricQuery + "{pod=\"" + podName + "\"}")
			filesystemMetricValue := extractMetricValue(filesystemQueryResult)
			o.Expect(err).NotTo(o.HaveOccurred(), "Error querying Filesystem metric for pod %s: %s", podName, filesystemMetricValue)

			// Add your expectations/assertions to validate the metrics
		}
	})

	g.It("Author:rrasouli-DisconnectedOnly-Smokerun-Critical-74760-Windows workloads with disconnected registry image", func() {
		var (
			sourceNamespace   = "openshift-config"
			namespace         = "winc-74760"
			pull_secret_name  = "pull-secret"
			primary_image_key = primary_disconnected_image_key
		)

		g.By("Create a Windows workload using the disconnected registry image")

		// Step 1: Retrieve the primary_windows_container_disconnected_image from the ConfigMap
		g.By("Retrieving the primary disconnected image from the ConfigMap")
		primaryDisconnectedImage := getConfigMapData(oc, wincTestCM, primary_image_key, defaultNamespace)
		if !isDisconnectedCluster(oc) {
			g.Skip("Skipping test not a disconnected test")
		}
		defer deleteProject(oc, namespace)
		createProject(oc, namespace)

		// Step 2: Apply the pull-secret to the namespace
		g.By("Applying the pull-secret to the namespace")
		pullSecretData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", pull_secret_name, "-n", sourceNamespace, "-o=jsonpath={.data.\\.dockerconfigjson}").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Unable to retrieve pull-secret from %v namespace", sourceNamespace)

		decodedPullSecretData, err := base64.StdEncoding.DecodeString(pullSecretData)
		o.Expect(err).NotTo(o.HaveOccurred(), "Unable to decode pull-secret data")

		_, err = oc.AsAdmin().NotShowInfo().WithoutNamespace().Run("create").Args("secret", "generic", pull_secret_name, fmt.Sprintf("--from-literal=.dockerconfigjson=%s", decodedPullSecretData), "--type=kubernetes.io/dockerconfigjson", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Unable to create pull-secret in %v namespace", namespace)

		// Step 3: Create workloads pulling from a disconnected registry
		g.By("Creating workloads pulling from a disconnected registry")
		createWorkload(oc, namespace, "windows_web_server_disconnected.yaml", map[string]string{"<windows_container_image>": primaryDisconnectedImage}, true, windowsWorkloads)

		// Step 4: Scale the deployment up
		g.By("Scaling the deployment up")
		err = scaleDeployment(oc, windowsWorkloads, 5, namespace)
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to scale up the deployment")

		// Step 5: Scale the deployment back down
		g.By("Scaling the deployment back down")
		err = scaleDeployment(oc, windowsWorkloads, 1, namespace)
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to scale down the deployment")
	})

	g.It("Author:weinliu-Smokerun-Medium-73595-Verify Display of Filesystem Graphs (metrics) for Windows Nodes [Serial]", func() {
		windowsHostNames := getWindowsHostNames(oc)
		o.Expect(len(windowsHostNames)).To(o.BeNumerically(">", 0), "Test requires at least one Windows node to run")

		for _, winHostName := range windowsHostNames {
			e2e.Logf("Verifying resource usage for node: %s", winHostName)

			// Retrieve node ephemeral storage using `oc` command
			nodeStorage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", winHostName, "-o=jsonpath={.status.allocatable['ephemeral-storage']}").Output()
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get node %s ephemeral storage", winHostName))

			// Convert the storage value from string to int64
			storageValue, err := strconv.ParseInt(strings.TrimSuffix(nodeStorage, "Ki"), 10, 64)
			o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to parse storage value for node %s: %s", winHostName, nodeStorage))

			o.Expect(storageValue).To(o.BeNumerically(">", 0), fmt.Sprintf("Expected strictly positive storage value but got %d", storageValue))
		}
	})

	g.It("Author:weinliu-Smokerun-Medium-73752-Monitor Network In, and Network Out graphs for Windows Pods managed by wmco", func() {
		// Define the metrics queries for Network In, and Network Out
		networkInMetricQuery := "pod:network_receive_bytes_total:sum"
		networkOutMetricQuery := "pod:network_transmit_bytes_total:sum"

		// Create Prometheus monitor instance
		mon, err := exutil.NewPrometheusMonitor(oc.AsAdmin())
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating new Prometheus monitor")

		// Define the namespace where the Windows workloads are deployed
		namespace := wmcoNamespace

		// Check if the WMCO deployment is ready
		ready := checkWorkloadCreated(oc, wmcoDeployment, namespace, 1)
		if !ready {
			e2e.Failf("WMCO deployment %s is not ready", wmcoDeployment)
		}

		exutil.By("Checking Network In, and Network Out graphs for Windows pods managed by WMCO")

		// Get the pods managed by WMCO
		podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app="+wmcoDeployment, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Error getting pods managed by WMCO")

		// Split the pod list
		podNames := strings.Fields(podList)

		for _, podName := range podNames {

			// Check Network In metric
			g.By(fmt.Sprintf("Verify Network In metric availability for pod: %s", podName))
			networkInQueryResult, err := mon.SimpleQuery(networkInMetricQuery + "{pod=\"" + podName + "\"}")
			networkInMetricValue := extractMetricValue(networkInQueryResult)
			o.Expect(err).NotTo(o.HaveOccurred(), "Error querying Network In metric for pod %s: %s", podName, networkInMetricValue)

			// Check Network Out metric
			g.By(fmt.Sprintf("Verify Network Out metric availability for pod: %s", podName))
			networkOutQueryResult, err := mon.SimpleQuery(networkOutMetricQuery + "{pod=\"" + podName + "\"}")
			networkOutMetricValue := extractMetricValue(networkOutQueryResult)
			o.Expect(err).NotTo(o.HaveOccurred(), "Error querying Network Out metric for pod %s: %s", podName, networkOutMetricValue)

			// Add your expectations/assertions to validate the metrics
		}
	})

	g.It("Author:rrasouli-Smokerun-Medium-76765-WICD-Remove-Services [Disruptive]", func() {
		wmcoLogVersion := getWMCOVersionFromLogs(oc)
		g.By("Step 1: Fetch the WICD ConfigMap and verify its existence")
		windowsServicesCM, err := popItemFromList(oc, "cm", wicdConfigMap, wmcoNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(windowsServicesCM).NotTo(o.BeEmpty(), "Expected to find a WICD ConfigMap")

		g.By("Step 2: Extract services from the ConfigMap and ensure they are properly defined")
		payload, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", windowsServicesCM, "-n", wmcoNamespace, "-o=jsonpath={.data.services}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(payload).NotTo(o.BeEmpty(), "Expected non-empty services payload in ConfigMap")

		var configMapServices []Service
		err = json.Unmarshal([]byte(payload), &configMapServices)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(configMapServices).NotTo(o.BeEmpty(), "Expected to find services defined in the ConfigMap")

		g.By("Step 3: Retrieve Windows worker information")
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		o.Expect(bastionHost).NotTo(o.BeEmpty(), "Expected to get a bastion host")

		winInternalIP := getWindowsInternalIPs(oc)
		o.Expect(winInternalIP).NotTo(o.BeEmpty(), "Expected to find Windows worker nodes")

		g.By("Step 4: Scale WMCO to 0 and remove the existing windows-services ConfigMap")
		defer scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace) // Ensure scaling WMCO back to 1 at the end
		scaleDeployment(oc, wmcoDeployment, 0, wmcoNamespace)       // Scale down the WMCO to 0

		// Delete the old windows-services ConfigMap
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("cm", windowsServicesCM, "-n", wmcoNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to delete windows-services %v ConfigMap", windowsServicesCM)

		g.By("Step 5: Generate new service ConfigMap adding fake new services here")
		var manifestFile string
		manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "wicd_configmap.yaml", map[string]string{
			"<version>": wmcoLogVersion, // Replace the version dynamically
			"<new_services>": `[
				{"name":"new-service-1","path":"C:\\k\\new-service-1.exe --logfile C:\\var\\log\\new-service-1.log","bootstrap":false,"priority":2},
				{"name":"new-service-2","path":"C:\\k\\new-service-2.exe --logfile C:\\var\\log\\new-service-2.log","bootstrap":false,"priority":3}
			]`, // New services added dynamically
		})
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to find manifest file")
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", manifestFile, "--ignore-not-found").Execute()
			os.Remove(manifestFile)
		}()

		// Create the new windows-services ConfigMap using the generated manifest
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to create a new windows-services ConfigMap")

		// Wait for the ConfigMap to be applied correctly
		waitForCM(oc, windowsServicesCM, wicdConfigMap, wmcoNamespace)

		g.By("Step 6: Scale WMCO back to 1 and wait for node reconfiguration")
		scaleDeployment(oc, wmcoDeployment, 1, wmcoNamespace)
		waitWindowsNodesReady(oc, len(winInternalIP), 15*time.Minute)

		g.By("Step 7: Verify the initial state of services (all should be running)")
		for _, winhost := range winInternalIP {
			for _, svc := range configMapServices {
				msg, err := runPSCommand(bastionHost, winhost, fmt.Sprintf("Get-Service %v", svc.Name), privateKey, iaasPlatform)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(msg).To(o.ContainSubstring("Running"), "Expected service %s to be running initially on %s", svc.Name, winhost)
			}
		}

		g.By("Step 8: Simulate service removal (in reverse order to respect priority)")
		removedServices := make([]string, 0)
		for i := len(configMapServices) - 1; i >= 0; i-- {
			serviceName := configMapServices[i].Name
			removedServices = append(removedServices, serviceName)
			e2e.Logf("Simulating removal of service: %s", serviceName)
		}

		g.By("Step 9: Verify service removal order")
		servicesByPriority := make(map[int][]string)
		for _, svc := range configMapServices {
			servicesByPriority[svc.Priority] = append(servicesByPriority[svc.Priority], svc.Name)
		}

		// Build a map to track when each service was removed
		serviceRemovalOrder := make(map[string]int)
		for pos, serviceName := range removedServices {
			serviceRemovalOrder[serviceName] = pos
		}

		priorities := make([]int, 0, len(servicesByPriority))
		for priority := range servicesByPriority {
			priorities = append(priorities, priority)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

		for i := 0; i < len(priorities)-1; i++ {
			currentPriority := priorities[i]
			nextPriority := priorities[i+1]
			currentServices := servicesByPriority[currentPriority]
			nextServices := servicesByPriority[nextPriority]

			// Get the earliest removal position for each priority group
			currentEarliestPos := -1
			nextEarliestPos := -1

			for _, svc := range currentServices {
				pos, exists := serviceRemovalOrder[svc]
				if exists && (currentEarliestPos == -1 || pos < currentEarliestPos) {
					currentEarliestPos = pos
				}
			}

			for _, svc := range nextServices {
				pos, exists := serviceRemovalOrder[svc]
				if exists && (nextEarliestPos == -1 || pos < nextEarliestPos) {
					nextEarliestPos = pos
				}
			}

			o.Expect(currentEarliestPos).To(o.BeNumerically("<", nextEarliestPos),
				"Expected services with priority %d to be removed before services with priority %d",
				currentPriority, nextPriority)
		}

		g.By("Step 10: Verify services are no longer running on Windows workers and attempt to stop them if necessary")
		maxRetries := 3
		retryInterval := 30 * time.Second

		type ServiceStatus struct {
			Name    string
			Status  string
			Retries int
		}
		defer waitWindowsNodesReady(oc, len(winInternalIP), 15*time.Minute)
		serviceStatuses := make(map[string][]ServiceStatus)

		for _, winhost := range winInternalIP {
			e2e.Logf("Checking services on Windows host: %s", winhost)
			serviceStatuses[winhost] = []ServiceStatus{}

			for _, serviceName := range removedServices {
				var serviceStatus string
				var err error
				retries := 0

				for i := 0; i < maxRetries; i++ {
					retries++
					e2e.Logf("Attempt %d to stop service %s on %s", i+1, serviceName, winhost)

					// Attempt to stop the service forcefully
					stopCmd := fmt.Sprintf("Stop-Service %v -Force -ErrorAction SilentlyContinue; (Get-Service %v).Status", serviceName, serviceName)
					serviceStatus, err = runPSCommand(bastionHost, winhost, stopCmd, privateKey, iaasPlatform)
					if err != nil {
						e2e.Logf("Error stopping service %s on %s: %v", serviceName, winhost, err)
						continue
					}

					serviceStatus = strings.TrimSpace(serviceStatus)
					e2e.Logf("Service %s status on %s: %s", serviceName, winhost, serviceStatus)

					if serviceStatus != "Running" {
						break
					}

					e2e.Logf("Service %s is still running on %s. Retrying in %v...", serviceName, winhost, retryInterval)
					time.Sleep(retryInterval)
				}

				serviceStatuses[winhost] = append(serviceStatuses[winhost], ServiceStatus{Name: serviceName, Status: serviceStatus, Retries: retries})

				if err != nil {
					o.Expect(err).NotTo(o.HaveOccurred(), "Failed to stop service %s on host %s after retries", serviceName, winhost)
				}
			}
		}

		g.By("Step 11: Verify no unexpected services are still running and gather summarized information about services")
		unexpectedServices := []string{"unwanted-service-1", "unwanted-service-2"} // List of unwanted services

		for _, winhost := range winInternalIP {
			for _, service := range unexpectedServices {
				serviceExists := false
				retries := 0

				// Check if service exists on the node
				serviceExistsCmd := fmt.Sprintf("Get-Service %v -ErrorAction SilentlyContinue", service)
				serviceExistsOutput, err := runPSCommand(bastionHost, winhost, serviceExistsCmd, privateKey, iaasPlatform)

				if err == nil && !strings.Contains(serviceExistsOutput, "Cannot find") {
					serviceExists = true
				}

				if serviceExists {
					// Retry checking if the service is stopped (only if it exists)
					for retries < maxRetries {
						serviceCheckCmd := fmt.Sprintf("Get-Service %v | Select-Object -ExpandProperty Status", service)
						serviceStatusOutput, err := runPSCommand(bastionHost, winhost, serviceCheckCmd, privateKey, iaasPlatform)

						if err == nil && strings.Contains(serviceStatusOutput, "Stopped") {
							e2e.Logf("Service %s is stopped on host %s", service, winhost)
							break
						}

						retries++
						e2e.Logf("Service %s is not stopped yet on host %s. Attempt %d/%d", service, winhost, retries, maxRetries)
						if retries < maxRetries {
							time.Sleep(retryInterval)
						}
					}

					if retries == maxRetries {
						// Fail if the service did not stop after retries
						e2e.Failf("Service %s is still running on host %s after retries", service, winhost)
					}
				}
			}
			e2e.Logf("Finished checking for unexpected services.")
		}
	})

})
